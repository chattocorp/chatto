package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/internal/testutil"
	"hmans.de/chatto/pkg/events"
)

func TestOAuthClientAccessDeniedWatchersAreClientTargetedAndRaceFree(t *testing.T) {
	projection := NewOAuthClientProjection()
	const (
		alpha = "https://alpha.example/oauth/client-metadata.json"
		bravo = "https://bravo.example/oauth/client-metadata.json"
	)
	for index, clientID := range []string{alpha, bravo} {
		if err := projection.Apply(newEvent("member", &evtv1.Event{
			Event: &evtv1.Event_OauthClientAuthorizationRecorded{
				OauthClientAuthorizationRecorded: &evtv1.OAuthClientAuthorizationRecordedEvent{
					ClientId: clientID,
					Source:   evtv1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD,
				},
			},
		}), uint64(index+1)); err != nil {
			t.Fatalf("apply authorization for %s: %v", clientID, err)
		}
	}

	alphaBlocked, stopAlpha := projection.watchAccessDenied(alpha)
	defer stopAlpha()
	bravoBlocked, stopBravo := projection.watchAccessDenied(bravo)
	if err := projection.Apply(newEvent("admin", &evtv1.Event{
		Event: &evtv1.Event_OauthClientPolicyChanged{
			OauthClientPolicyChanged: &evtv1.OAuthClientPolicyChangedEvent{
				ClientId: alpha,
				Policy:   evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED,
			},
		},
	}), 3); err != nil {
		t.Fatalf("apply block: %v", err)
	}

	select {
	case <-alphaBlocked:
	default:
		t.Fatal("matching client watcher remained open after block")
	}
	select {
	case <-bravoBlocked:
		t.Fatal("unrelated client watcher closed after block")
	default:
	}

	stopBravo()
	projection.RLock()
	_, retained := projection.accessDeniedWatchersByClient[bravo]
	projection.RUnlock()
	if retained {
		t.Fatal("cancelled client watcher remained registered")
	}

	alreadyBlocked, stopAlreadyBlocked := projection.watchAccessDenied(alpha)
	defer stopAlreadyBlocked()
	select {
	case <-alreadyBlocked:
	default:
		t.Fatal("watcher registered after block was not closed atomically")
	}
}

func TestOAuthClientAuthorizationPolicyAndTokenRevocation(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	admin := invitationAdmin(t, c)
	member, err := c.CreateUser(ctx, SystemActorID, "oauth-client-member", "OAuth Client Member", "password123")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	clientID := "https://remote.example/oauth/client-metadata.json"

	if clients, err := c.ListOAuthClients(ctx, member.Id); !errors.Is(err, ErrPermissionDenied) || clients != nil {
		t.Fatalf("member ListOAuthClients = %+v, %v; want permission denied", clients, err)
	}
	if clients, err := c.ListOAuthClients(ctx, admin); err != nil || len(clients) != 0 {
		t.Fatalf("initial ListOAuthClients = %+v, %v", clients, err)
	}

	if err := c.RecordOAuthClientAuthorization(ctx, member.Id, clientID, "Remote Chatto", "https://remote.example", "https://remote.example", evtv1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD); err != nil {
		t.Fatalf("RecordOAuthClientAuthorization: %v", err)
	}
	firstAuthorization, err := c.GetOAuthClient(ctx, admin, clientID)
	if err != nil {
		t.Fatalf("GetOAuthClient after first authorization: %v", err)
	}
	if err := c.RecordOAuthClientAuthorization(ctx, member.Id, clientID, "Remote Chatto", "https://remote.example", "https://remote.example", evtv1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD); err != nil {
		t.Fatalf("repeat RecordOAuthClientAuthorization: %v", err)
	}
	repeatAuthorization, err := c.GetOAuthClient(ctx, admin, clientID)
	if err != nil {
		t.Fatalf("GetOAuthClient after repeat authorization: %v", err)
	}
	if !repeatAuthorization.LastAuthorizationAt.After(firstAuthorization.LastAuthorizationAt) {
		t.Fatalf("repeat authorization timestamp = %v, want after %v", repeatAuthorization.LastAuthorizationAt, firstAuthorization.LastAuthorizationAt)
	}
	if err := c.RecordOAuthClientAuthorization(ctx, admin, clientID, "Remote Chatto", "https://remote.example", "https://other.example", evtv1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD); err != nil {
		t.Fatalf("second-user RecordOAuthClientAuthorization: %v", err)
	}
	state, err := c.GetOAuthClient(ctx, admin, clientID)
	if err != nil {
		t.Fatalf("GetOAuthClient: %v", err)
	}
	if state.Policy != evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_DEFAULT || state.Source != evtv1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD || state.AuthorizedUserCount != 2 || len(state.RedirectOrigins) != 2 || state.FirstAuthorizationAt.IsZero() || state.LastAuthorizationAt.IsZero() {
		t.Fatalf("recorded OAuth client = %+v", state)
	}

	generation := mustCurrentAuthGeneration(t, c, member.Id)
	token, err := c.CreateOAuthAccessTokenForClient(ctx, member.Id, clientID, generation)
	if err != nil {
		t.Fatalf("CreateOAuthAccessTokenForClient: %v", err)
	}
	blocked, err := c.UpdateOAuthClientPolicy(ctx, admin, clientID, evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED)
	if err != nil {
		t.Fatalf("UpdateOAuthClientPolicy blocked: %v", err)
	}
	if blocked.Policy != evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED {
		t.Fatalf("blocked policy = %v", blocked.Policy)
	}
	if _, err := c.ValidateAuthToken(ctx, token); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("ValidateAuthToken after block = %v, want not found", err)
	}
	if _, err := c.CreateOAuthAccessTokenForClient(ctx, member.Id, clientID, generation); !errors.Is(err, ErrOAuthClientBlocked) {
		t.Fatalf("CreateOAuthAccessTokenForClient after block = %v, want blocked", err)
	}
	if err := c.RecordOAuthClientAuthorization(ctx, member.Id, clientID, "Remote Chatto", "https://remote.example", "https://remote.example", evtv1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD); !errors.Is(err, ErrOAuthClientBlocked) {
		t.Fatalf("RecordOAuthClientAuthorization after block = %v, want blocked", err)
	}

	trusted, err := c.UpdateOAuthClientPolicy(ctx, admin, clientID, evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_TRUSTED)
	if err != nil || trusted.Policy != evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_TRUSTED {
		t.Fatalf("UpdateOAuthClientPolicy trusted = %+v, %v", trusted, err)
	}
	consented, err := c.HasOAuthClientConsent(ctx, admin, clientID, "https://remote.example")
	if err != nil || consented {
		t.Fatalf("trusted client consent = %v, %v; trust must not grant user consent", consented, err)
	}
}

func TestOAuthClientAuthorizationCodeFailureDoesNotRecordClient(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	admin := invitationAdmin(t, c)
	member, err := c.CreateUser(ctx, SystemActorID, "oauth-code-failure-member", "OAuth Code Failure Member", "password123")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	clientID := "https://code-failure.example/oauth/client-metadata.json"
	request := OAuthClientAuthorization{
		UserID:         member.Id,
		ClientID:       clientID,
		ClientName:     "Code Failure Client",
		ClientOrigin:   "https://code-failure.example",
		RedirectOrigin: "https://code-failure.example",
		Source:         evtv1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD,
	}

	publisher := c.EventPublisher
	c.EventPublisher = nil // Fail the durable auth-code issuance audit append.
	if code, err := c.CreateOAuthClientAuthorizationCode(ctx, request, "https://code-failure.example/callback", GenerateCodeChallenge("verifier"), "S256", mustCurrentAuthGeneration(t, c, member.Id)); err == nil || code != "" {
		t.Fatalf("CreateOAuthClientAuthorizationCode = %q, %v; want failed issuance", code, err)
	}
	c.EventPublisher = publisher

	if clients, err := c.ListOAuthClients(ctx, admin); err != nil || len(clients) != 0 {
		t.Fatalf("ListOAuthClients after failed issuance = %+v, %v; want empty", clients, err)
	}
	grantCount, err := countTestKVKeys(ctx, c.storage.runtimeStateKV, "grant.*")
	if err != nil {
		t.Fatalf("count grant keys: %v", err)
	}
	if grantCount != 0 {
		t.Fatalf("failed issuance retained %d authorization codes", grantCount)
	}
}

func TestOAuthClientAuthorizationRecordFailureDiscardsCode(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	admin := invitationAdmin(t, c)
	member, err := c.CreateUser(ctx, SystemActorID, "oauth-record-failure-member", "OAuth Record Failure Member", "password123")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	clientID := "https://record-failure.example/oauth/client-metadata.json"
	request := OAuthClientAuthorization{
		UserID:         member.Id,
		ClientID:       clientID,
		ClientName:     "Record Failure Client",
		ClientOrigin:   "https://record-failure.example",
		RedirectOrigin: "https://record-failure.example",
		Source:         evtv1.OAuthClientSource_OAUTH_CLIENT_SOURCE_UNSPECIFIED,
	}

	if code, err := c.CreateOAuthClientAuthorizationCode(ctx, request, "https://record-failure.example/callback", GenerateCodeChallenge("verifier"), "S256", mustCurrentAuthGeneration(t, c, member.Id)); !errors.Is(err, ErrInvalidArgument) || code != "" {
		t.Fatalf("CreateOAuthClientAuthorizationCode = %q, %v; want invalid record", code, err)
	}
	if clients, err := c.ListOAuthClients(ctx, admin); err != nil || len(clients) != 0 {
		t.Fatalf("ListOAuthClients after failed record = %+v, %v; want empty", clients, err)
	}
	grantCount, err := countTestKVKeys(ctx, c.storage.runtimeStateKV, "grant.*")
	if err != nil {
		t.Fatalf("count grant keys: %v", err)
	}
	if grantCount != 0 {
		t.Fatalf("failed authorization record retained %d authorization codes", grantCount)
	}
}

func TestOAuthClientAuthorizationPostCommitWaitFailureKeepsCode(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	admin := invitationAdmin(t, c)
	member, err := c.CreateUser(ctx, SystemActorID, "oauth-wait-failure-member", "OAuth Wait Failure Member", "password123")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	clientID := "https://wait-failure.example/oauth/client-metadata.json"
	request := OAuthClientAuthorization{
		UserID:         member.Id,
		ClientID:       clientID,
		ClientName:     "Wait Failure Client",
		ClientOrigin:   "https://wait-failure.example",
		RedirectOrigin: "https://wait-failure.example",
		Source:         evtv1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD,
	}
	waitErr := errors.New("forced post-commit projection wait failure")

	code, err := c.createOAuthClientAuthorizationCode(ctx, request, "", nil, "https://wait-failure.example/callback", GenerateCodeChallenge("verifier"), "S256", mustCurrentAuthGeneration(t, c, member.Id), func(context.Context, events.StreamPosition) error {
		return waitErr
	})
	if err != nil || code == "" {
		t.Fatalf("createOAuthClientAuthorizationCode = %q, %v; want committed authorization", code, err)
	}
	if _, err := c.storage.runtimeStateKV.Get(ctx, c.authCodeKey(code)); err != nil {
		t.Fatalf("authorization code missing after post-commit wait failure: %v", err)
	}
	state, err := c.GetOAuthClient(ctx, admin, clientID)
	if err != nil {
		t.Fatalf("GetOAuthClient after post-commit wait failure: %v", err)
	}
	if state.AuthorizedUserCount != 1 || state.ClientName != request.ClientName {
		t.Fatalf("recorded OAuth client = %+v", state)
	}
}

func TestOAuthClientPolicyRejectsUnknownClientAndInvalidPolicy(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	admin := invitationAdmin(t, c)
	clientID := "https://missing.example/oauth/client-metadata.json"
	if _, err := c.UpdateOAuthClientPolicy(ctx, admin, clientID, evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown client policy error = %v, want not found", err)
	}
	if _, err := c.UpdateOAuthClientPolicy(ctx, admin, clientID, evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_UNSPECIFIED); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("invalid policy error = %v, want invalid argument", err)
	}
}

func TestOAuthClientBlockEventInvalidatesTokenOnAnotherReplicaBeforeCleanup(t *testing.T) {
	ns, firstNC := testutil.StartNATS(t)
	secondNC, err := nats.Connect(nats.DefaultURL, nats.InProcessServer(ns))
	if err != nil {
		t.Fatalf("connect second replica to NATS: %v", err)
	}
	t.Cleanup(secondNC.Close)

	ctx := testContext(t)
	cfg := config.CoreConfig{
		SecretKey: "oauth-client-replica-secret",
		Assets:    config.AssetsConfig{SigningSecret: "oauth-client-replica-signing-secret"},
	}
	first, err := NewChattoCore(ctx, firstNC, cfg)
	if err != nil {
		t.Fatalf("create first replica: %v", err)
	}
	startCoreServices(t, first)
	second, err := NewChattoCore(ctx, secondNC, cfg)
	if err != nil {
		t.Fatalf("create second replica: %v", err)
	}
	startCoreServices(t, second)

	admin := invitationAdmin(t, first)
	member, err := first.CreateUser(ctx, SystemActorID, "oauth-client-replica-member", "OAuth Client Replica Member", "password123")
	if err != nil {
		t.Fatalf("create member: %v", err)
	}
	const clientID = "https://replica-client.example/oauth/client-metadata.json"
	if err := first.RecordOAuthClientAuthorization(ctx, member.Id, clientID, "Replica Client", "https://replica-client.example", "https://replica-client.example", evtv1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD); err != nil {
		t.Fatalf("record OAuth client authorization: %v", err)
	}
	generation := mustCurrentAuthGeneration(t, first, member.Id)
	token, err := first.CreateOAuthAccessTokenForClient(ctx, member.Id, clientID, generation)
	if err != nil {
		t.Fatalf("create OAuth access token: %v", err)
	}
	if userID, err := second.ValidateAuthToken(ctx, token); err != nil || userID != member.Id {
		t.Fatalf("second replica pre-block validation = %q, %v", userID, err)
	}
	blockedOnSecond, stopBlockWatch := second.WatchOAuthClientAccessDenied(clientID)
	defer stopBlockWatch()

	// Publish only the durable policy fact, as if another replica committed the
	// block but its best-effort runtime-token cleanup had not run yet. The
	// validating replica must wait for that fact and reject the still-present
	// token independently of cleanup.
	aggregate := evtstream.OAuthClientAggregate(clientID)
	sequence, err := first.EventPublisher.LastSubjectSeq(ctx, aggregate.AllEventsFilter())
	if err != nil {
		t.Fatalf("read OAuth client aggregate sequence: %v", err)
	}
	event := newEvent(admin, &evtv1.Event{Event: &evtv1.Event_OauthClientPolicyChanged{
		OauthClientPolicyChanged: &evtv1.OAuthClientPolicyChangedEvent{
			ClientId: clientID,
			Policy:   evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED,
		},
	}})
	if _, err := first.EventPublisher.AppendAtFilter(ctx, aggregate.SubjectFor(event), event, aggregate.AllEventsFilter(), sequence); err != nil {
		t.Fatalf("publish block event: %v", err)
	}
	if _, err := first.storage.runtimeStateKV.Get(ctx, first.authTokenKey(token)); err != nil {
		t.Fatalf("token disappeared before replica validation: %v", err)
	}

	if _, err := second.ValidateAuthToken(ctx, token); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("second replica validation after block = %v, want token not found", err)
	}
	select {
	case <-blockedOnSecond:
	case <-time.After(5 * time.Second):
		t.Fatal("second replica did not notify active client watchers after block")
	}
	state, err := second.GetOAuthClient(ctx, admin, clientID)
	if err != nil {
		t.Fatalf("read blocked client from second replica: %v", err)
	}
	if state.Policy != evtv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED {
		t.Fatalf("second replica policy = %v, want blocked", state.Policy)
	}
	if _, err := first.storage.runtimeStateKV.Get(ctx, first.authTokenKey(token)); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("rejected token still exists after validation: %v", err)
	}
}

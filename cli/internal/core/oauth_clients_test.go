package core

import (
	"errors"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

func TestOAuthClientObservationPolicyAndTokenRevocation(t *testing.T) {
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

	if err := c.ObserveOAuthClient(ctx, member.Id, clientID, "Remote Chatto", "https://remote.example", "https://remote.example", corev1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD); err != nil {
		t.Fatalf("ObserveOAuthClient: %v", err)
	}
	firstObservation, err := c.GetOAuthClient(ctx, admin, clientID)
	if err != nil {
		t.Fatalf("GetOAuthClient after first observation: %v", err)
	}
	if err := c.ObserveOAuthClient(ctx, member.Id, clientID, "Remote Chatto", "https://remote.example", "https://remote.example", corev1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD); err != nil {
		t.Fatalf("repeat ObserveOAuthClient: %v", err)
	}
	repeatObservation, err := c.GetOAuthClient(ctx, admin, clientID)
	if err != nil {
		t.Fatalf("GetOAuthClient after repeat observation: %v", err)
	}
	if !repeatObservation.LastObservedAt.After(firstObservation.LastObservedAt) {
		t.Fatalf("repeat observation timestamp = %v, want after %v", repeatObservation.LastObservedAt, firstObservation.LastObservedAt)
	}
	if err := c.ObserveOAuthClient(ctx, admin, clientID, "Remote Chatto", "https://remote.example", "https://other.example", corev1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD); err != nil {
		t.Fatalf("second-user ObserveOAuthClient: %v", err)
	}
	state, err := c.GetOAuthClient(ctx, admin, clientID)
	if err != nil {
		t.Fatalf("GetOAuthClient: %v", err)
	}
	if state.Policy != corev1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_DEFAULT || state.Source != corev1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD || state.AuthorizedUserCount != 2 || len(state.RedirectOrigins) != 2 || state.FirstObservedAt.IsZero() || state.LastObservedAt.IsZero() {
		t.Fatalf("observed OAuth client = %+v", state)
	}

	generation := mustCurrentAuthGeneration(t, c, member.Id)
	token, err := c.CreateOAuthAccessTokenForClient(ctx, member.Id, clientID, generation)
	if err != nil {
		t.Fatalf("CreateOAuthAccessTokenForClient: %v", err)
	}
	blocked, err := c.UpdateOAuthClientPolicy(ctx, admin, clientID, corev1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED)
	if err != nil {
		t.Fatalf("UpdateOAuthClientPolicy blocked: %v", err)
	}
	if blocked.Policy != corev1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED {
		t.Fatalf("blocked policy = %v", blocked.Policy)
	}
	if _, err := c.ValidateAuthToken(ctx, token); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("ValidateAuthToken after block = %v, want not found", err)
	}
	if _, err := c.CreateOAuthAccessTokenForClient(ctx, member.Id, clientID, generation); !errors.Is(err, ErrOAuthClientBlocked) {
		t.Fatalf("CreateOAuthAccessTokenForClient after block = %v, want blocked", err)
	}
	if err := c.ObserveOAuthClient(ctx, member.Id, clientID, "Remote Chatto", "https://remote.example", "https://remote.example", corev1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD); !errors.Is(err, ErrOAuthClientBlocked) {
		t.Fatalf("ObserveOAuthClient after block = %v, want blocked", err)
	}

	trusted, err := c.UpdateOAuthClientPolicy(ctx, admin, clientID, corev1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_TRUSTED)
	if err != nil || trusted.Policy != corev1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_TRUSTED {
		t.Fatalf("UpdateOAuthClientPolicy trusted = %+v, %v", trusted, err)
	}
	consented, err := c.HasOAuthClientConsent(ctx, admin, clientID, "https://remote.example")
	if err != nil || consented {
		t.Fatalf("trusted client consent = %v, %v; trust must not grant user consent", consented, err)
	}
}

func TestOAuthClientPolicyRejectsUnknownClientAndInvalidPolicy(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	admin := invitationAdmin(t, c)
	clientID := "https://missing.example/oauth/client-metadata.json"
	if _, err := c.UpdateOAuthClientPolicy(ctx, admin, clientID, corev1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown client policy error = %v, want not found", err)
	}
	if _, err := c.UpdateOAuthClientPolicy(ctx, admin, clientID, corev1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_UNSPECIFIED); !errors.Is(err, ErrInvalidArgument) {
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
	if err := first.ObserveOAuthClient(ctx, member.Id, clientID, "Replica Client", "https://replica-client.example", "https://replica-client.example", corev1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD); err != nil {
		t.Fatalf("observe OAuth client: %v", err)
	}
	generation := mustCurrentAuthGeneration(t, first, member.Id)
	token, err := first.CreateOAuthAccessTokenForClient(ctx, member.Id, clientID, generation)
	if err != nil {
		t.Fatalf("create OAuth access token: %v", err)
	}
	if userID, err := second.ValidateAuthToken(ctx, token); err != nil || userID != member.Id {
		t.Fatalf("second replica pre-block validation = %q, %v", userID, err)
	}

	// Publish only the durable policy fact, as if another replica committed the
	// block but its best-effort runtime-token cleanup had not run yet. The
	// validating replica must wait for that fact and reject the still-present
	// token independently of cleanup.
	aggregate := evtstream.OAuthClientAggregate(clientID)
	sequence, err := first.EventPublisher.LastSubjectSeq(ctx, aggregate.AllEventsFilter())
	if err != nil {
		t.Fatalf("read OAuth client aggregate sequence: %v", err)
	}
	event := newEvent(admin, &corev1.Event{Event: &corev1.Event_OauthClientPolicyChanged{
		OauthClientPolicyChanged: &corev1.OAuthClientPolicyChangedEvent{
			ClientId: clientID,
			Policy:   corev1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED,
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
	state, err := second.GetOAuthClient(ctx, admin, clientID)
	if err != nil {
		t.Fatalf("read blocked client from second replica: %v", err)
	}
	if state.Policy != corev1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_BLOCKED {
		t.Fatalf("second replica policy = %v, want blocked", state.Policy)
	}
	if _, err := first.storage.runtimeStateKV.Get(ctx, first.authTokenKey(token)); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Fatalf("rejected token still exists after validation: %v", err)
	}
}

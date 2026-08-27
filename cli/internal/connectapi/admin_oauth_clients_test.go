package connectapi

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/evtstream"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestAdminOAuthClientServiceLifecycleAndAuthorization(t *testing.T) {
	env := newConnectAPITestEnv(t)
	clientID := "https://remote.example/oauth/client-metadata.json"
	if err := env.core.RecordOAuthClientAuthorization(env.ctx, env.viewer.Id, clientID, "Remote Chatto", "https://remote.example", "https://remote.example", corev1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD); err != nil {
		t.Fatalf("RecordOAuthClientAuthorization: %v", err)
	}
	if _, err := env.adminOAuthClients.ListOAuthClients(withCaller(env.ctx, env.viewer), connect.NewRequest(&adminv1.ListOAuthClientsRequest{})); err == nil || connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("regular ListOAuthClients error = %v, want permission denied", err)
	}
	if err := env.core.AssignAdminRole(env.ctx, env.viewer.Id); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}
	ctx := withCaller(env.ctx, env.viewer)
	listed, err := env.adminOAuthClients.ListOAuthClients(ctx, connect.NewRequest(&adminv1.ListOAuthClientsRequest{}))
	if err != nil {
		t.Fatalf("ListOAuthClients: %v", err)
	}
	if len(listed.Msg.GetOauthClients()) != 1 || listed.Msg.GetOauthClients()[0].GetClientId() != clientID || listed.Msg.GetOauthClients()[0].GetClientOrigin() != "https://remote.example" || listed.Msg.GetOauthClients()[0].GetAuthorizedUserCount() != 1 || !listed.Msg.GetOauthClients()[0].GetFirstAuthorizationAt().IsValid() || !listed.Msg.GetOauthClients()[0].GetLastAuthorizationAt().IsValid() {
		t.Fatalf("listed OAuth clients = %+v", listed.Msg.GetOauthClients())
	}
	updated, err := env.adminOAuthClients.UpdateOAuthClientPolicy(ctx, connect.NewRequest(&adminv1.UpdateOAuthClientPolicyRequest{
		ClientId: clientID,
		Policy:   adminv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_TRUSTED,
	}))
	if err != nil {
		t.Fatalf("UpdateOAuthClientPolicy: %v", err)
	}
	if updated.Msg.GetOauthClient().GetPolicy() != adminv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_TRUSTED {
		t.Fatalf("updated client = %+v", updated.Msg.GetOauthClient())
	}
	got, err := env.adminOAuthClients.GetOAuthClient(ctx, connect.NewRequest(&adminv1.GetOAuthClientRequest{ClientId: clientID}))
	if err != nil || got.Msg.GetOauthClient().GetPolicy() != adminv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_TRUSTED {
		t.Fatalf("GetOAuthClient = %+v, %v", got, err)
	}
	if _, err := env.adminOAuthClients.GetOAuthClient(ctx, connect.NewRequest(&adminv1.GetOAuthClientRequest{ClientId: "https://missing.example/client.json"})); err == nil || connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("missing GetOAuthClient error = %v, want not found", err)
	}
}

func TestAdminOAuthClientServicePreservesFutureEnumValues(t *testing.T) {
	env := newConnectAPITestEnv(t)
	const clientID = "https://future.example/oauth/client-metadata.json"
	if err := env.core.RecordOAuthClientAuthorization(
		env.ctx,
		env.viewer.Id,
		clientID,
		"Future Client",
		"https://future.example",
		"https://future.example",
		corev1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD,
	); err != nil {
		t.Fatalf("RecordOAuthClientAuthorization: %v", err)
	}
	if err := env.core.AssignAdminRole(env.ctx, env.viewer.Id); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}

	aggregate := evtstream.OAuthClientAggregate(clientID)
	sequence, err := env.core.EventPublisher.LastSubjectSeq(env.ctx, aggregate.AllEventsFilter())
	if err != nil {
		t.Fatalf("read OAuth client aggregate sequence: %v", err)
	}
	futureSource := corev1.OAuthClientSource(99)
	sourceEvent := &corev1.Event{
		Id:        core.NewEventID(),
		ActorId:   env.viewer.Id,
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_OauthClientAuthorizationRecorded{
			OauthClientAuthorizationRecorded: &corev1.OAuthClientAuthorizationRecordedEvent{
				ClientId:       clientID,
				ClientName:     "Future Client",
				ClientUri:      "https://future.example",
				RedirectOrigin: "https://future.example",
				Source:         futureSource,
			},
		},
	}
	if _, err := env.core.EventPublisher.AppendAtFilter(
		env.ctx,
		aggregate.SubjectFor(sourceEvent),
		sourceEvent,
		aggregate.AllEventsFilter(),
		sequence,
	); err != nil {
		t.Fatalf("append future OAuth source: %v", err)
	}
	sequence, err = env.core.EventPublisher.LastSubjectSeq(env.ctx, aggregate.AllEventsFilter())
	if err != nil {
		t.Fatalf("refresh OAuth client aggregate sequence: %v", err)
	}
	accessDenied, stopAccessWatch := env.core.WatchOAuthClientAccessDenied(clientID)
	defer stopAccessWatch()
	futurePolicy := corev1.OAuthClientPolicy(101)
	event := &corev1.Event{
		Id:        core.NewEventID(),
		ActorId:   env.viewer.Id,
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_OauthClientPolicyChanged{
			OauthClientPolicyChanged: &corev1.OAuthClientPolicyChangedEvent{
				ClientId: clientID,
				Policy:   futurePolicy,
			},
		},
	}
	if _, err := env.core.EventPublisher.AppendAtFilter(
		env.ctx,
		aggregate.SubjectFor(event),
		event,
		aggregate.AllEventsFilter(),
		sequence,
	); err != nil {
		t.Fatalf("append future OAuth policy: %v", err)
	}

	got, err := env.adminOAuthClients.GetOAuthClient(
		withCaller(env.ctx, env.viewer),
		connect.NewRequest(&adminv1.GetOAuthClientRequest{ClientId: clientID}),
	)
	if err != nil {
		t.Fatalf("GetOAuthClient: %v", err)
	}
	if got.Msg.GetOauthClient().GetPolicy() != adminv1.OAuthClientPolicy(futurePolicy) {
		t.Fatalf(
			"future policy = %d, want preserved %d",
			got.Msg.GetOauthClient().GetPolicy(),
			futurePolicy,
		)
	}
	if got.Msg.GetOauthClient().GetSource() != adminv1.OAuthClientSource(futureSource) {
		t.Fatalf(
			"future source = %d, want preserved %d",
			got.Msg.GetOauthClient().GetSource(),
			futureSource,
		)
	}
	if err := env.core.RequireOAuthClientAllowed(env.ctx, clientID); !errors.Is(err, core.ErrOAuthClientBlocked) {
		t.Fatalf("future policy authorization = %v, want access denied", err)
	}
	select {
	case <-accessDenied:
	default:
		t.Fatal("future policy did not terminate active client watchers")
	}
	if _, err := env.adminOAuthClients.UpdateOAuthClientPolicy(
		withCaller(env.ctx, env.viewer),
		connect.NewRequest(&adminv1.UpdateOAuthClientPolicyRequest{
			ClientId: clientID,
			Policy:   adminv1.OAuthClientPolicy_OAUTH_CLIENT_POLICY_DEFAULT,
		}),
	); err == nil || connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("overwrite future policy error = %v, want invalid argument", err)
	}
}

func TestAdminOAuthClientServicePaginationDoesNotReshuffleOnAuthorization(t *testing.T) {
	env := newConnectAPITestEnv(t)
	clientIDs := []string{
		"https://alpha.example/oauth/client-metadata.json",
		"https://bravo.example/oauth/client-metadata.json",
		"https://charlie.example/oauth/client-metadata.json",
		"https://delta.example/oauth/client-metadata.json",
	}
	for _, clientID := range clientIDs {
		origin := strings.TrimSuffix(clientID, "/oauth/client-metadata.json")
		if err := env.core.RecordOAuthClientAuthorization(env.ctx, env.viewer.Id, clientID, clientID, origin, origin, corev1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD); err != nil {
			t.Fatalf("RecordOAuthClientAuthorization(%q): %v", clientID, err)
		}
	}
	if err := env.core.AssignAdminRole(env.ctx, env.viewer.Id); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}
	ctx := withCaller(env.ctx, env.viewer)

	first, err := env.adminOAuthClients.ListOAuthClients(ctx, connect.NewRequest(&adminv1.ListOAuthClientsRequest{
		Page: &apiv1.PageRequest{Limit: 2},
	}))
	if err != nil {
		t.Fatalf("first ListOAuthClients: %v", err)
	}

	// Reauthorizing the oldest client changes its last-authorization timestamp,
	// but must not move any client across the offset boundary between requests.
	if err := env.core.RecordOAuthClientAuthorization(env.ctx, env.viewer.Id, clientIDs[0], clientIDs[0], "https://alpha.example", "https://alpha.example", corev1.OAuthClientSource_OAUTH_CLIENT_SOURCE_CIMD); err != nil {
		t.Fatalf("reauthorize client: %v", err)
	}
	second, err := env.adminOAuthClients.ListOAuthClients(ctx, connect.NewRequest(&adminv1.ListOAuthClientsRequest{
		Page: &apiv1.PageRequest{Limit: 2, Offset: 2},
	}))
	if err != nil {
		t.Fatalf("second ListOAuthClients: %v", err)
	}
	got := append(oauthClientIDs(first.Msg.GetOauthClients()), oauthClientIDs(second.Msg.GetOauthClients())...)
	if !slices.Equal(got, clientIDs) {
		t.Fatalf("combined page client IDs = %v, want each client once in %v", got, clientIDs)
	}
}

func oauthClientIDs(clients []*adminv1.OAuthClient) []string {
	ids := make([]string, 0, len(clients))
	for _, client := range clients {
		ids = append(ids, client.GetClientId())
	}
	return ids
}

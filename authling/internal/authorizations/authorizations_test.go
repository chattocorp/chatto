package authorizations

import (
	"bytes"
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "hmans.de/authling/internal/pb/authling/core/v1"
)

func TestProjectionRejectsForkedAndReusedGrantHistory(t *testing.T) {
	projection := NewProjection()
	if err := projection.Apply(accountCreated("evt_account"), 1); err != nil {
		t.Fatal(err)
	}
	if err := projection.Apply(grantAuthorized("evt_grant", "grant_one", ""), 2); err != nil {
		t.Fatal(err)
	}
	if err := projection.Apply(grantAuthorized("evt_fork", "grant_one", "evt_other"), 3); err == nil || !strings.Contains(err.Error(), "another active authorization") {
		t.Fatalf("forked renewal error = %v", err)
	}
	if err := projection.Apply(grantRevoked("evt_revoke", "grant_one", "evt_other"), 4); err == nil || !strings.Contains(err.Error(), "another active authorization") {
		t.Fatalf("forked revocation error = %v", err)
	}
	if err := projection.Apply(grantRevoked("evt_revoke", "grant_one", "evt_grant"), 5); err != nil {
		t.Fatal(err)
	}
	if err := projection.Apply(grantAuthorized("evt_reuse", "grant_one", ""), 6); err == nil || !strings.Contains(err.Error(), "reused after its generation ended") {
		t.Fatalf("reused grant id error = %v", err)
	}
}

func TestProjectionRejectsGrantForAbsentAccount(t *testing.T) {
	projection := NewProjection()
	if err := projection.Apply(grantAuthorized("evt_grant", "grant_one", ""), 1); err == nil || !strings.Contains(err.Error(), "absent account") {
		t.Fatalf("absent-account grant error = %v", err)
	}
}

func TestClientDigestIsDeploymentKeyed(t *testing.T) {
	clientID := "https://client.example/metadata?tenant=private"
	first := (&Service{indexKey: []byte("first deployment key")}).clientDigest(clientID)
	second := (&Service{indexKey: []byte("second deployment key")}).clientDigest(clientID)
	if len(first) != 32 || first == second || bytes.Contains([]byte(first), []byte(clientID)) {
		t.Fatalf("client digests do not provide a keyed opaque index")
	}
}

func accountCreated(eventID string) *corev1.Event {
	return &corev1.Event{Id: eventID, CreatedAt: timestamppb.Now(), Event: &corev1.Event_AccountCreated{AccountCreated: &corev1.AccountCreatedEvent{AccountId: "acc_one"}}}
}

func grantAuthorized(eventID, grantID, priorEventID string) *corev1.Event {
	return &corev1.Event{Id: eventID, CreatedAt: timestamppb.Now(), Event: &corev1.Event_OidcGrantAuthorized{OidcGrantAuthorized: &corev1.OIDCGrantAuthorizedEvent{
		AccountId: "acc_one", GrantId: grantID, ClientIdDigest: make([]byte, 32), ClientName: "Client One", ClientHost: "client.example",
		Scopes: []string{"openid"}, PriorAuthorizationEventId: priorEventID,
	}}}
}

func grantRevoked(eventID, grantID, authorizationEventID string) *corev1.Event {
	return &corev1.Event{Id: eventID, CreatedAt: timestamppb.Now(), Event: &corev1.Event_OidcGrantRevoked{OidcGrantRevoked: &corev1.OIDCGrantRevokedEvent{
		AccountId: "acc_one", GrantId: grantID, AuthorizationEventId: authorizationEventID,
	}}}
}

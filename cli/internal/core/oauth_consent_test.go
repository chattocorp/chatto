package core

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"hmans.de/chatto/internal/events"
)

func TestChattoCore_OAuthConsentGrantIsProjectedAndIdempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "consent-user", "Consent User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	origin := "https://client.example"

	consented, err := core.HasOAuthConsent(ctx, user.Id, origin)
	if err != nil {
		t.Fatalf("HasOAuthConsent before grant: %v", err)
	}
	if consented {
		t.Fatalf("expected no consent before grant")
	}

	if err := core.GrantOAuthConsent(ctx, user.Id, origin); err != nil {
		t.Fatalf("GrantOAuthConsent: %v", err)
	}
	if err := core.GrantOAuthConsent(ctx, user.Id, origin); err != nil {
		t.Fatalf("duplicate GrantOAuthConsent: %v", err)
	}

	consented, err = core.HasOAuthConsent(ctx, user.Id, origin)
	if err != nil {
		t.Fatalf("HasOAuthConsent after grant: %v", err)
	}
	if !consented {
		t.Fatalf("expected consent after grant")
	}

	published, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventOAuthConsentGranted))
	if err != nil {
		t.Fatalf("SubjectEvents: %v", err)
	}
	if len(published) != 1 {
		t.Fatalf("expected one consent grant event, got %d", len(published))
	}
	payload := published[0].GetOauthConsentGranted()
	if payload.GetRedirectOrigin() != origin {
		t.Fatalf("origin = %q, want %q", payload.GetRedirectOrigin(), origin)
	}
	jsonPayload, err := protojson.Marshal(published[0])
	if err != nil {
		t.Fatalf("marshal grant event: %v", err)
	}
	if !strings.Contains(string(jsonPayload), origin) {
		t.Fatalf("grant event should include canonical origin for user-visible approvals: %s", jsonPayload)
	}
	if strings.Contains(string(jsonPayload), "/servers/callback") {
		t.Fatalf("grant event leaked full redirect URI path: %s", jsonPayload)
	}
}

func TestChattoCore_OAuthConsentDeniedIsAuditOnly(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "deny-consent-user", "Deny Consent User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	origin := "https://client.example"

	if err := core.RecordOAuthConsentDenied(ctx, user.Id, origin); err != nil {
		t.Fatalf("RecordOAuthConsentDenied: %v", err)
	}
	consented, err := core.HasOAuthConsent(ctx, user.Id, origin)
	if err != nil {
		t.Fatalf("HasOAuthConsent: %v", err)
	}
	if consented {
		t.Fatalf("denial should not grant consent")
	}

	published, _, err := core.EventPublisher.SubjectEvents(ctx, events.UserAggregate(user.Id).Subject(events.EventOAuthConsentDenied))
	if err != nil {
		t.Fatalf("SubjectEvents: %v", err)
	}
	if len(published) != 1 {
		t.Fatalf("expected one consent denial event, got %d", len(published))
	}
	payload := published[0].GetOauthConsentDenied()
	if payload.GetRedirectOrigin() != origin {
		t.Fatalf("origin = %q, want %q", payload.GetRedirectOrigin(), origin)
	}
}

func TestChattoCore_OAuthConsentClearedByAccountDeletion(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, SystemActorID, "delete-consent-user", "Delete Consent User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	origin := "https://client.example"

	if err := core.GrantOAuthConsent(ctx, user.Id, origin); err != nil {
		t.Fatalf("GrantOAuthConsent: %v", err)
	}
	if err := core.DeleteUser(ctx, user.Id, user.Id); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	consented, err := core.HasOAuthConsent(ctx, user.Id, origin)
	if err != nil {
		t.Fatalf("HasOAuthConsent: %v", err)
	}
	if consented {
		t.Fatalf("expected account deletion to clear projected consent")
	}
}

package core

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	"hmans.de/chatto/internal/evtstream"
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

	published, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.Id).Subject(evtstream.EventOAuthConsentGranted))
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

	published, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.Id).Subject(evtstream.EventOAuthConsentDenied))
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

func TestChattoCore_OAuthConsentUsesClientIdentifier(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "client-consent-user", "Client Consent User", "password123")
	if err != nil {
		t.Fatal(err)
	}
	const clientID = "https://client.example/oauth/metadata.json"
	const origin = "https://client.example"
	if err := core.GrantOAuthClientConsent(ctx, user.Id, clientID, "Example Client", origin, origin); err != nil {
		t.Fatal(err)
	}
	consented, err := core.HasOAuthClientConsent(ctx, user.Id, clientID, "https://different.example")
	if err != nil {
		t.Fatal(err)
	}
	if !consented {
		t.Fatal("expected consent to follow the stable client identifier")
	}
	if legacy, err := core.HasOAuthConsent(ctx, user.Id, origin); err != nil || legacy {
		t.Fatalf("origin-only consent = %v, err = %v", legacy, err)
	}

	published, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.Id).Subject(evtstream.EventOAuthConsentGranted))
	if err != nil {
		t.Fatal(err)
	}
	payload := published[0].GetOauthConsentGranted()
	if payload.GetClientId() != clientID || payload.GetClientName() != "Example Client" || payload.GetClientUri() != origin {
		t.Fatalf("consent metadata = %#v", payload)
	}
}

func TestChattoCore_OAuthConsentEventsStripClientURIPrivateData(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "private-client-uri", "Private Client URI", "password123")
	if err != nil {
		t.Fatal(err)
	}
	const clientID = "https://client.example/oauth/metadata.json"
	const clientURI = "https://client.example/products/chatto?account=person@example.com&token=secret"
	const origin = "https://client.example"

	if err := core.GrantOAuthClientConsent(ctx, user.Id, clientID, "Example Client", clientURI, origin); err != nil {
		t.Fatal(err)
	}
	if err := core.RecordOAuthClientConsentDenied(ctx, user.Id, clientID, "Example Client", clientURI, origin); err != nil {
		t.Fatal(err)
	}

	for _, eventType := range []string{evtstream.EventOAuthConsentGranted, evtstream.EventOAuthConsentDenied} {
		published, _, err := core.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.Id).Subject(eventType))
		if err != nil {
			t.Fatal(err)
		}
		if len(published) != 1 {
			t.Fatalf("%s events = %d, want 1", eventType, len(published))
		}
		encoded, err := protojson.Marshal(published[0])
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(encoded), "person@example.com") || strings.Contains(string(encoded), "secret") || strings.Contains(string(encoded), "/products/chatto") {
			t.Fatalf("%s event leaked client URI private data: %s", eventType, encoded)
		}
		if !strings.Contains(string(encoded), origin) {
			t.Fatalf("%s event omitted canonical client origin: %s", eventType, encoded)
		}
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

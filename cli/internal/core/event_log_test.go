package core

import (
	"bytes"
	"strings"
	"testing"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestMarshalEventLogPayloadJSONRedactsPasswordHash(t *testing.T) {
	hash := []byte("$2a$10$password-verifier")
	event := &evtv1.Event{
		Id:      "event-password-changed",
		ActorId: "audited-actor",
		Event: &evtv1.Event_UserPasswordHashChanged{
			UserPasswordHashChanged: &evtv1.UserPasswordHashChangedEvent{
				UserId:                      "target-user",
				PasswordHash:                append([]byte(nil), hash...),
				PreserveExistingCredentials: true,
			},
		},
	}

	payload, err := marshalEventLogPayloadJSON(event)
	if err != nil {
		t.Fatalf("marshalEventLogPayloadJSON: %v", err)
	}
	if strings.Contains(string(payload), "passwordHash") {
		t.Fatalf("audit payload contains passwordHash: %s", payload)
	}
	for _, want := range []string{"userPasswordHashChanged", "target-user", "preserveExistingCredentials"} {
		if !strings.Contains(string(payload), want) {
			t.Fatalf("audit payload = %s, want retained field %q", payload, want)
		}
	}
	if !bytes.Equal(event.GetUserPasswordHashChanged().GetPasswordHash(), hash) {
		t.Fatalf("durable event password hash was mutated: %q", event.GetUserPasswordHashChanged().GetPasswordHash())
	}
}

func TestMarshalEventLogPayloadJSONRedactsBotAPIKeyVerifier(t *testing.T) {
	verifier := []byte("bot-api-key-verifier")
	event := &evtv1.Event{Event: &evtv1.Event_BotApiKeyRotated{
		BotApiKeyRotated: &evtv1.BotApiKeyRotatedEvent{UserId: "bot-user", Verifier: append([]byte(nil), verifier...)},
	}}
	payload, err := marshalEventLogPayloadJSON(event)
	if err != nil {
		t.Fatalf("marshalEventLogPayloadJSON: %v", err)
	}
	if strings.Contains(string(payload), "verifier") || strings.Contains(string(payload), string(verifier)) {
		t.Fatalf("audit payload contains bot verifier: %s", payload)
	}
	if !bytes.Equal(event.GetBotApiKeyRotated().GetVerifier(), verifier) {
		t.Fatal("durable bot verifier was mutated")
	}
}

func TestMarshalEventLogPayloadJSONRedactsBotIncomingWebhookVerifier(t *testing.T) {
	verifier := []byte("bot-incoming-webhook-verifier")
	event := &evtv1.Event{Event: &evtv1.Event_BotIncomingWebhookCreated{
		BotIncomingWebhookCreated: &evtv1.BotIncomingWebhookCreatedEvent{UserId: "bot-user", WebhookId: "webhook", Verifier: append([]byte(nil), verifier...)},
	}}
	payload, err := marshalEventLogPayloadJSON(event)
	if err != nil {
		t.Fatalf("marshalEventLogPayloadJSON: %v", err)
	}
	if strings.Contains(string(payload), "verifier") || strings.Contains(string(payload), string(verifier)) {
		t.Fatalf("audit payload contains webhook verifier: %s", payload)
	}
	if !bytes.Equal(event.GetBotIncomingWebhookCreated().GetVerifier(), verifier) {
		t.Fatal("durable webhook verifier was mutated")
	}
}

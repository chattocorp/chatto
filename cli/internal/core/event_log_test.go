package core

import (
	"bytes"
	"strings"
	"testing"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

func TestMarshalEventLogPayloadJSONRedactsInvitationNote(t *testing.T) {
	t.Parallel()
	event := &evtv1.Event{Event: &evtv1.Event_InvitationCreated{
		InvitationCreated: &evtv1.InvitationCreatedEvent{InvitationId: "invite", Note: "Private recipient note"},
	}}
	data, err := marshalEventLogPayloadJSON(event)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "Private recipient note") || strings.Contains(string(data), `"note"`) {
		t.Fatal("audit payload exposed an invitation note")
	}
	if event.GetInvitationCreated().GetNote() != "Private recipient note" {
		t.Fatal("redaction changed the durable event")
	}
}

func TestMarshalEventLogPayloadJSONRedactsPasswordHash(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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

func TestMarshalEventLogPayloadJSONRedactsAddedBotAPIKeyVerifier(t *testing.T) {
	t.Parallel()

	verifier := []byte("named-bot-api-key-verifier")
	event := &evtv1.Event{Event: &evtv1.Event_BotApiKeyAdded{
		BotApiKeyAdded: &evtv1.BotApiKeyAddedEvent{
			UserId: "bot-user", KeyId: "Kcredential", Name: "Production", Verifier: append([]byte(nil), verifier...),
		},
	}}
	payload, err := marshalEventLogPayloadJSON(event)
	if err != nil {
		t.Fatalf("marshalEventLogPayloadJSON: %v", err)
	}
	if strings.Contains(string(payload), "verifier") || strings.Contains(string(payload), string(verifier)) {
		t.Fatalf("audit payload contains named bot verifier: %s", payload)
	}
	if !bytes.Equal(event.GetBotApiKeyAdded().GetVerifier(), verifier) {
		t.Fatal("durable named bot verifier was mutated")
	}
}

func TestMarshalEventLogPayloadJSONRedactsBotIncomingWebhookVerifier(t *testing.T) {
	t.Parallel()

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

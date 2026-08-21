package core

import (
	"bytes"
	"strings"
	"testing"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestMarshalEventLogPayloadJSONRedactsPasswordHash(t *testing.T) {
	hash := []byte("$2a$10$password-verifier")
	event := &corev1.Event{
		Id:      "event-password-changed",
		ActorId: "audited-actor",
		Event: &corev1.Event_UserPasswordHashChanged{
			UserPasswordHashChanged: &corev1.UserPasswordHashChangedEvent{
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
	event := &corev1.Event{Event: &corev1.Event_BotApiKeyRotated{
		BotApiKeyRotated: &corev1.BotApiKeyRotatedEvent{UserId: "bot-user", Verifier: append([]byte(nil), verifier...)},
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

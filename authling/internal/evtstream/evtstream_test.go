package evtstream

import (
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/authling/internal/config"
	"hmans.de/authling/internal/logging"
	"hmans.de/authling/internal/natsruntime"
	"hmans.de/authling/internal/pb/authling/core/v1"
	"hmans.de/authling/internal/storage"
	"hmans.de/chatto/pkg/events"
)

func TestAppendAccountCreatedUsesAggregateOCC(t *testing.T) {
	connection, err := natsruntime.Open(t.Context(), config.NATSConfig{
		Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()},
	})
	if err != nil {
		t.Fatalf("open NATS: %v", err)
	}
	t.Cleanup(func() {
		if err := connection.Close(); err != nil {
			t.Errorf("close NATS: %v", err)
		}
	})
	js, stream, err := storage.Open(t.Context(), connection.NATS, 1)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	logger := logging.Events{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	publisher := NewPublisher(events.NewEncodedEventLog(js, stream, logger))

	if _, err := publisher.AppendAccountCreated(t.Context(), accountCreatedEvent("evt_first")); err != nil {
		t.Fatalf("append first account creation: %v", err)
	}
	if _, err := publisher.AppendAccountCreated(t.Context(), accountCreatedEvent("evt_second")); !errors.Is(err, events.ErrConflict) {
		t.Fatalf("append duplicate account creation error = %v, want events.ErrConflict", err)
	}
}

func TestAppendPasswordChangedUsesObservedAccountTail(t *testing.T) {
	connection, err := natsruntime.Open(t.Context(), config.NATSConfig{
		Embedded: config.EmbeddedNATSConfig{Enabled: true, DataDir: t.TempDir()},
	})
	if err != nil {
		t.Fatalf("open NATS: %v", err)
	}
	t.Cleanup(func() {
		if err := connection.Close(); err != nil {
			t.Errorf("close NATS: %v", err)
		}
	})
	js, stream, err := storage.Open(t.Context(), connection.NATS, 1)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	logger := logging.Events{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
	publisher := NewPublisher(events.NewEncodedEventLog(js, stream, logger))
	if _, err := publisher.AppendAccountCreated(t.Context(), accountCreatedEvent("evt_created")); err != nil {
		t.Fatalf("append account creation: %v", err)
	}
	tail, err := publisher.AccountTail(t.Context(), "acc_test")
	if err != nil || tail == 0 {
		t.Fatalf("account tail = %d, %v", tail, err)
	}
	request := passwordResetRequestedEvent("evt_reset_request")
	if _, err := publisher.AppendPasswordResetRequested(t.Context(), request, tail); err != nil {
		t.Fatalf("append password reset request: %v", err)
	}
	requestTail, err := publisher.AccountTail(t.Context(), "acc_test")
	if err != nil || requestTail <= tail {
		t.Fatalf("account tail after reset request = %d, %v", requestTail, err)
	}
	first := passwordChangedEvent("evt_password_first")
	first.GetPasswordChanged().PasswordResetRequestEventId = request.GetId()
	if _, err := publisher.AppendPasswordChanged(t.Context(), first, requestTail); err != nil {
		t.Fatalf("append password change: %v", err)
	}
	if _, err := publisher.AppendPasswordChanged(t.Context(), passwordChangedEvent("evt_password_stale"), requestTail); !errors.Is(err, events.ErrConflict) {
		t.Fatalf("stale password change error = %v, want events.ErrConflict", err)
	}
}

func TestDecodeRejectsMalformedEvents(t *testing.T) {
	if _, err := Decode([]byte("not protobuf")); err == nil {
		t.Fatal("decode malformed event succeeded")
	}
	if _, err := Decode(nil); err == nil || !strings.Contains(err.Error(), "id is required") {
		t.Fatalf("decode empty event error = %v, want missing id", err)
	}
	malformed := passwordResetRequestedEvent("evt_reset_request")
	malformed.GetPasswordResetRequested().CredentialEventId = ""
	data, err := proto.Marshal(malformed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data); err == nil || !strings.Contains(err.Error(), "password reset request is incomplete") {
		t.Fatalf("decode incomplete password reset request error = %v", err)
	}
}

func TestAccountSubjectRejectsUnsafeTokens(t *testing.T) {
	if _, err := AccountSubject("account.with.dots"); err == nil {
		t.Fatal("account subject accepted a multi-token account ID")
	}
}

func accountCreatedEvent(eventID string) *corev1.Event {
	return &corev1.Event{
		Id:        eventID,
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_AccountCreated{
			AccountCreated: &corev1.AccountCreatedEvent{AccountId: "acc_test"},
		},
	}
}

func passwordChangedEvent(eventID string) *corev1.Event {
	return &corev1.Event{
		Id:        eventID,
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_PasswordChanged{PasswordChanged: &corev1.PasswordChangedEvent{
			AccountId:                  "acc_test",
			UserKeyRef:                 "key_user",
			CredentialKeyRef:           "key_credential",
			CredentialEnvelopeVersion:  1,
			PasswordVerifierNonce:      []byte("nonce"),
			PasswordVerifierCiphertext: []byte("ciphertext"),
		}},
	}
}

func passwordResetRequestedEvent(eventID string) *corev1.Event {
	return &corev1.Event{
		Id:        eventID,
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_PasswordResetRequested{PasswordResetRequested: &corev1.PasswordResetRequestedEvent{
			AccountId:         "acc_test",
			CredentialEventId: "evt_created",
		}},
	}
}

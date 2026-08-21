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

func TestAppendEmailChangedUsesAccountAndRegistryOCC(t *testing.T) {
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
	if err != nil {
		t.Fatal(err)
	}
	request := emailChangeRequestedEvent("evt_email_request")
	if _, err := publisher.AppendEmailChangeRequested(t.Context(), request, tail); err != nil {
		t.Fatalf("append email change request: %v", err)
	}
	requestTail, _ := publisher.AccountTail(t.Context(), "acc_test")
	registryTail, _ := publisher.AccountRegistryTail(t.Context())
	change, claim := emailChangedEvents("evt_email_changed", "evt_email_claimed")
	if _, err := publisher.AppendEmailChanged(t.Context(), change, claim, requestTail, registryTail); err != nil {
		t.Fatalf("append email change: %v", err)
	}
	staleChange, staleClaim := emailChangedEvents("evt_email_stale", "evt_email_stale_claim")
	if _, err := publisher.AppendEmailChanged(t.Context(), staleChange, staleClaim, requestTail, registryTail); !errors.Is(err, events.ErrConflict) {
		t.Fatalf("stale email change error = %v, want events.ErrConflict", err)
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
	malformed = emailChangeRequestedEvent("evt_email_request")
	malformed.GetEmailChangeRequested().CredentialEventId = ""
	data, err = proto.Marshal(malformed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data); err == nil || !strings.Contains(err.Error(), "email change request is incomplete") {
		t.Fatalf("decode incomplete email change request error = %v", err)
	}
	change, _ := emailChangedEvents("evt_email_changed", "evt_email_claimed")
	change.GetEmailChanged().EmailCiphertext = nil
	data, err = proto.Marshal(change)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data); err == nil || !strings.Contains(err.Error(), "email credential envelope is incomplete") {
		t.Fatalf("decode incomplete email change error = %v", err)
	}
	malformed = passwordChangedEvent("evt_signed_in_change")
	malformed.GetPasswordChanged().Kind = corev1.PasswordChangeKind_PASSWORD_CHANGE_KIND_SIGNED_IN
	data, err = proto.Marshal(malformed)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data); err == nil || !strings.Contains(err.Error(), "signed-in password change correlation is invalid") {
		t.Fatalf("decode uncorrelated signed-in password change error = %v", err)
	}
	profile := profileUpdatedEvent("evt_profile")
	profile.GetProfileUpdated().FullNameCiphertext = nil
	data, err = proto.Marshal(profile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data); err == nil || !strings.Contains(err.Error(), "profile envelope is incomplete") {
		t.Fatalf("decode incomplete profile update error = %v", err)
	}
	malformedGrant := &corev1.Event{Id: "evt_grant", CreatedAt: timestamppb.Now(), Event: &corev1.Event_OidcGrantAuthorized{OidcGrantAuthorized: &corev1.OIDCGrantAuthorizedEvent{
		AccountId: "acc_test", GrantId: "grant_test", ClientIdDigest: make([]byte, 32), ClientName: "Client", ClientHost: "client.example", Scopes: []string{"openid", "openid"},
	}}}
	data, err = proto.Marshal(malformedGrant)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data); err == nil || !strings.Contains(err.Error(), "OIDC grant authorization is incomplete or invalid") {
		t.Fatalf("decode duplicate grant scopes error = %v", err)
	}
	malformedRevocation := &corev1.Event{Id: "evt_revoke", CreatedAt: timestamppb.Now(), Event: &corev1.Event_OidcGrantRevoked{OidcGrantRevoked: &corev1.OIDCGrantRevokedEvent{
		AccountId: "acc_test", GrantId: "grant_test",
	}}}
	data, err = proto.Marshal(malformedRevocation)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decode(data); err == nil || !strings.Contains(err.Error(), "OIDC grant revocation is incomplete or invalid") {
		t.Fatalf("decode uncorrelated grant revocation error = %v", err)
	}
}

func profileUpdatedEvent(eventID string) *corev1.Event {
	return &corev1.Event{Id: eventID, CreatedAt: timestamppb.Now(), Event: &corev1.Event_ProfileUpdated{ProfileUpdated: &corev1.ProfileUpdatedEvent{
		AccountId: "acc_test", UserKeyRef: "key_user", CredentialKeyRef: "key_credential", ProfileEnvelopeVersion: 1,
		PreferredUsernameNonce: []byte("nonce"), PreferredUsernameCiphertext: []byte("ciphertext"),
		FullNameNonce: []byte("nonce"), FullNameCiphertext: []byte("ciphertext"),
	}}}
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

func emailChangeRequestedEvent(eventID string) *corev1.Event {
	return &corev1.Event{
		Id:        eventID,
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_EmailChangeRequested{EmailChangeRequested: &corev1.EmailChangeRequestedEvent{
			AccountId:         "acc_test",
			CredentialEventId: "evt_created",
		}},
	}
}

func emailChangedEvents(changeEventID, claimEventID string) (*corev1.Event, *corev1.Event) {
	createdAt := timestamppb.Now()
	change := &corev1.Event{
		Id:        changeEventID,
		CreatedAt: createdAt,
		Event: &corev1.Event_EmailChanged{EmailChanged: &corev1.EmailChangedEvent{
			AccountId:                 "acc_test",
			UserKeyRef:                "key_user",
			CredentialKeyRef:          "key_credential",
			CredentialEnvelopeVersion: 1,
			EmailNonce:                []byte("nonce"),
			EmailCiphertext:           []byte("ciphertext"),
			EmailChangeRequestEventId: "evt_email_request",
			PriorCredentialEventId:    "evt_created",
		}},
	}
	claim := &corev1.Event{
		Id:        claimEventID,
		CreatedAt: createdAt,
		Event: &corev1.Event_EmailClaimed{EmailClaimed: &corev1.EmailClaimedEvent{
			AccountId:         "acc_test",
			CredentialEventId: changeEventID,
		}},
	}
	return change, claim
}

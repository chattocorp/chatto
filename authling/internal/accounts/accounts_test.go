package accounts

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "hmans.de/authling/internal/pb/authling/core/v1"
	"hmans.de/chatto/pkg/datacrypto"
)

func TestPasswordChangedVerifierRejectsAADSubstitution(t *testing.T) {
	key, err := datacrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	aad := passwordChangedAAD("evt_change", "acc_example", "key_user", "key_credential")
	sealed, err := datacrypto.Seal(key, []byte("argon2 verifier"), aad)
	if err != nil {
		t.Fatal(err)
	}
	for name, substituted := range map[string][]byte{
		"event type": credentialAAD("evt_change", "acc_example", "key_user", "key_credential", "password-verifier"),
		"event ID":   passwordChangedAAD("evt_other", "acc_example", "key_user", "key_credential"),
		"account":    passwordChangedAAD("evt_change", "acc_other", "key_user", "key_credential"),
		"user key":   passwordChangedAAD("evt_change", "acc_example", "key_other", "key_credential"),
		"data key":   passwordChangedAAD("evt_change", "acc_example", "key_user", "key_other"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := datacrypto.Open(key, sealed.Ciphertext, sealed.Nonce, substituted); !errors.Is(err, datacrypto.ErrDecryptionFailed) {
				t.Fatalf("substitution error = %v, want ErrDecryptionFailed", err)
			}
		})
	}
}

func TestEmailChangedRequiresAppliedReauthenticationRequest(t *testing.T) {
	projection := NewProjection(nil, []byte("index key"))
	projection.accounts = map[string]Account{"acc_example": {ID: "acc_example"}}
	projection.credentials = map[string]protectedCredential{"acc_example": {
		accountID: "acc_example", eventID: "evt_prior", userKeyRef: "key_user", credentialKeyRef: "key_credential",
	}}
	event := &corev1.Event{
		Id: "evt_change", CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_EmailChanged{EmailChanged: &corev1.EmailChangedEvent{
			AccountId: "acc_example", UserKeyRef: "key_user", CredentialKeyRef: "key_credential",
			CredentialEnvelopeVersion: 1, EmailNonce: []byte("nonce"), EmailCiphertext: []byte("ciphertext"),
			EmailChangeRequestEventId: "evt_request", PriorCredentialEventId: "evt_prior",
		}},
	}
	if err := projection.Apply(event, 1); err == nil || !strings.Contains(err.Error(), "reauthentication request") {
		t.Fatalf("uncorrelated email change error = %v", err)
	}
}

func TestEmailReplacementClaimRequiresCorrelationButHistoricalCreationDoesNot(t *testing.T) {
	claim := func() *corev1.Event {
		return &corev1.Event{Id: "evt_claim", CreatedAt: timestamppb.Now(), Event: &corev1.Event_EmailClaimed{EmailClaimed: &corev1.EmailClaimedEvent{AccountId: "acc_example"}}}
	}

	replacement := NewProjection(nil, []byte("index key"))
	replacement.pendingEmails = map[string]pendingEmail{"acc_example": {eventID: "evt_change", replaces: true}}
	if err := replacement.Apply(claim(), 1); err == nil || !strings.Contains(err.Error(), "missing credential correlation") {
		t.Fatalf("uncorrelated replacement claim error = %v", err)
	}

	historical := NewProjection(nil, []byte("index key"))
	digestValue := digest(historical.indexKey, "person@example.com")
	historical.pendingEmails = map[string]pendingEmail{"acc_example": {eventID: "evt_created", digest: digestValue}}
	if err := historical.Apply(claim(), 1); err != nil {
		t.Fatalf("historical uncorrelated creation claim: %v", err)
	}
}

func TestEmailChangedAddressRejectsAADSubstitution(t *testing.T) {
	key, err := datacrypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	defer clear(key)
	aad := emailChangedAAD("evt_change", "acc_example", "key_user", "key_credential", "evt_request", "evt_prior")
	sealed, err := datacrypto.Seal(key, []byte("person@example.com"), aad)
	if err != nil {
		t.Fatal(err)
	}
	for name, substituted := range map[string][]byte{
		"event type":       credentialAAD("evt_change", "acc_example", "key_user", "key_credential", "email"),
		"event ID":         emailChangedAAD("evt_other", "acc_example", "key_user", "key_credential", "evt_request", "evt_prior"),
		"account":          emailChangedAAD("evt_change", "acc_other", "key_user", "key_credential", "evt_request", "evt_prior"),
		"user key":         emailChangedAAD("evt_change", "acc_example", "key_other", "key_credential", "evt_request", "evt_prior"),
		"data key":         emailChangedAAD("evt_change", "acc_example", "key_user", "key_other", "evt_request", "evt_prior"),
		"request event":    emailChangedAAD("evt_change", "acc_example", "key_user", "key_credential", "evt_other", "evt_prior"),
		"prior credential": emailChangedAAD("evt_change", "acc_example", "key_user", "key_credential", "evt_request", "evt_other"),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := datacrypto.Open(key, sealed.Ciphertext, sealed.Nonce, substituted); !errors.Is(err, datacrypto.ErrDecryptionFailed) {
				t.Fatalf("substitution error = %v, want ErrDecryptionFailed", err)
			}
		})
	}
}

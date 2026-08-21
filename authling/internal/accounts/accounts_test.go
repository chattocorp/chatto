package accounts

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "hmans.de/authling/internal/pb/authling/core/v1"
	"hmans.de/chatto/pkg/datacrypto"
)

func TestPreferredUsernameFromEmail(t *testing.T) {
	for email, want := range map[string]string{
		"Alice.Example@example.com": "alice.example",
		"a+tag@example.com":         "atag",
		"--Person--@example.com":    "person",
	} {
		if got := preferredUsernameFromEmail(email); got != want {
			t.Errorf("preferredUsernameFromEmail(%q) = %q, want %q", email, got, want)
		}
	}
}

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

func TestEmailChangedRejectsRequestBoundToAnotherCredential(t *testing.T) {
	projection := NewProjection(nil, []byte("index key"))
	projection.accounts = map[string]Account{"acc_example": {ID: "acc_example"}}
	projection.credentials = map[string]protectedCredential{"acc_example": {
		accountID: "acc_example", eventID: "evt_prior", userKeyRef: "key_user", credentialKeyRef: "key_credential",
	}}
	projection.emailChanges = map[string]map[string]emailChangeRequest{"acc_example": {
		"evt_request": {credentialEventID: "evt_other", sequence: 1},
	}}
	event := &corev1.Event{
		Id: "evt_change", CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_EmailChanged{EmailChanged: &corev1.EmailChangedEvent{
			AccountId: "acc_example", UserKeyRef: "key_user", CredentialKeyRef: "key_credential",
			CredentialEnvelopeVersion: 1, EmailNonce: []byte("nonce"), EmailCiphertext: []byte("ciphertext"),
			EmailChangeRequestEventId: "evt_request", PriorCredentialEventId: "evt_prior",
		}},
	}
	if err := projection.Apply(event, 2); err == nil || !strings.Contains(err.Error(), "another reauthentication request") {
		t.Fatalf("wrong-credential request error = %v", err)
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
	wrongCorrelation := claim()
	wrongCorrelation.GetEmailClaimed().CredentialEventId = "evt_other"
	if err := replacement.Apply(wrongCorrelation, 1); err == nil || !strings.Contains(err.Error(), "another staged credential") {
		t.Fatalf("wrong replacement claim correlation error = %v", err)
	}

	historical := NewProjection(nil, []byte("index key"))
	digestValue := digest(historical.indexKey, "person@example.com")
	historical.pendingEmails = map[string]pendingEmail{"acc_example": {eventID: "evt_created", digest: digestValue}}
	if err := historical.Apply(claim(), 1); err != nil {
		t.Fatalf("historical uncorrelated creation claim: %v", err)
	}
}

func TestPasswordMutationRejectsTheSplitEmailChangeBatch(t *testing.T) {
	projection := NewProjection(nil, []byte("index key"))
	projection.credentials = map[string]protectedCredential{"acc_example": {
		accountID: "acc_example", eventID: "evt_prior", userKeyRef: "key_user", credentialKeyRef: "key_credential",
	}}
	projection.pendingEmails = map[string]pendingEmail{"acc_example": {
		eventID: "evt_email_change", replaces: true,
		credential: protectedCredential{accountID: "acc_example", eventID: "evt_email_change"},
	}}
	if credential, ok := projection.credentialForPasswordMutation("acc_example"); ok {
		t.Fatalf("password mutation observed staged email-change credential: %+v", credential)
	}
	delete(projection.pendingEmails, "acc_example")
	if credential, ok := projection.credentialForPasswordMutation("acc_example"); !ok || credential.eventID != "evt_prior" {
		t.Fatalf("password mutation credential after registry claim = %+v, %v", credential, ok)
	}
}

func TestPasswordChangeInvalidatesAcceptedEmailChangeRequests(t *testing.T) {
	projection := NewProjection(nil, []byte("index key"))
	projection.accounts = map[string]Account{"acc_example": {ID: "acc_example"}}
	projection.credentials = map[string]protectedCredential{"acc_example": {
		accountID: "acc_example", eventID: "evt_prior", userKeyRef: "key_user", credentialKeyRef: "key_credential",
	}}
	projection.emailChanges = map[string]map[string]emailChangeRequest{"acc_example": {
		"evt_request": {credentialEventID: "evt_prior", sequence: 1},
	}}
	event := &corev1.Event{Id: "evt_password", CreatedAt: timestamppb.Now(), Event: &corev1.Event_PasswordChanged{PasswordChanged: &corev1.PasswordChangedEvent{
		AccountId: "acc_example", UserKeyRef: "key_user", CredentialKeyRef: "key_credential",
		PasswordVerifierNonce: []byte("nonce"), PasswordVerifierCiphertext: []byte("ciphertext"),
	}}}
	if err := projection.Apply(event, 2); err != nil {
		t.Fatal(err)
	}
	if _, ok := projection.emailChanges["acc_example"]; ok {
		t.Fatal("password change retained accepted email-change requests")
	}
}

func TestSignedInPasswordChangeRequiresCurrentCredentialCorrelation(t *testing.T) {
	projection := NewProjection(nil, []byte("index key"))
	projection.accounts = map[string]Account{"acc_example": {ID: "acc_example"}}
	projection.credentials = map[string]protectedCredential{"acc_example": {
		accountID: "acc_example", eventID: "evt_current", userKeyRef: "key_user", credentialKeyRef: "key_credential",
	}}
	event := &corev1.Event{Id: "evt_password", CreatedAt: timestamppb.Now(), Event: &corev1.Event_PasswordChanged{PasswordChanged: &corev1.PasswordChangedEvent{
		AccountId: "acc_example", UserKeyRef: "key_user", CredentialKeyRef: "key_credential",
		CredentialEnvelopeVersion: 1, PasswordVerifierNonce: []byte("nonce"), PasswordVerifierCiphertext: []byte("ciphertext"),
		PriorCredentialEventId: "evt_stale", Kind: corev1.PasswordChangeKind_PASSWORD_CHANGE_KIND_SIGNED_IN,
	}}}
	if err := projection.Apply(event, 2); err == nil || !strings.Contains(err.Error(), "prior credential") {
		t.Fatalf("stale signed-in password change error = %v", err)
	}
}

func TestRecoveryPasswordChangeRequiresAppliedRequest(t *testing.T) {
	projection := NewProjection(nil, []byte("index key"))
	projection.accounts = map[string]Account{"acc_example": {ID: "acc_example"}}
	projection.credentials = map[string]protectedCredential{"acc_example": {
		accountID: "acc_example", eventID: "evt_current", userKeyRef: "key_user", credentialKeyRef: "key_credential",
	}}
	event := &corev1.Event{Id: "evt_password", CreatedAt: timestamppb.Now(), Event: &corev1.Event_PasswordChanged{PasswordChanged: &corev1.PasswordChangedEvent{
		AccountId: "acc_example", UserKeyRef: "key_user", CredentialKeyRef: "key_credential",
		CredentialEnvelopeVersion: 1, PasswordVerifierNonce: []byte("nonce"), PasswordVerifierCiphertext: []byte("ciphertext"),
		PasswordResetRequestEventId: "evt_request", PriorCredentialEventId: "evt_current",
		Kind: corev1.PasswordChangeKind_PASSWORD_CHANGE_KIND_RECOVERY,
	}}}
	if err := projection.Apply(event, 2); err == nil || !strings.Contains(err.Error(), "recovery request") {
		t.Fatalf("uncorrelated recovery password change error = %v", err)
	}
}

func TestCompletedEmailChangeRequiresItsExactCredentialGeneration(t *testing.T) {
	projection := NewProjection(nil, []byte("index key"))
	projection.accounts = map[string]Account{"acc_example": {ID: "acc_example", AuthenticationVersion: 2}}
	projection.credentials = map[string]protectedCredential{"acc_example": {
		accountID:                 "acc_example",
		eventID:                   "evt_email_change",
		emailChangeEventID:        "evt_email_change",
		emailChangeRequestEventID: "evt_request",
		emailDigest:               digest(projection.indexKey, "new@example.com"),
	}}
	target := EmailChangeTarget{AccountID: "acc_example", RequestEventID: "evt_request"}
	if account, ok := projection.completedEmailChange(target, "new@example.com"); !ok || account.AuthenticationVersion != 2 {
		t.Fatalf("current email change completion = %+v, %v", account, ok)
	}
	credential := projection.credentials["acc_example"]
	credential.eventID = "evt_later_password_change"
	projection.credentials["acc_example"] = credential
	projection.accounts["acc_example"] = Account{ID: "acc_example", AuthenticationVersion: 3}
	if account, ok := projection.completedEmailChange(target, "new@example.com"); ok {
		t.Fatalf("email change completion crossed a later credential generation: %+v", account)
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

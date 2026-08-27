package issuer

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "hmans.de/authling/internal/pb/authling/core/v1"
)

func TestProjectionRejectsOutOfOrderSigningKeyHistory(t *testing.T) {
	now := time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)
	projection := NewProjection()
	prepared := &corev1.Event{Id: "evt_prepared", CreatedAt: timestamppb.New(now), Event: &corev1.Event_OidcSigningKeyPrepared{OidcSigningKeyPrepared: &corev1.OIDCSigningKeyPreparedEvent{
		SigningKeyRef: "system.oidc-signing.key_next", SigningKeyId: "sig_next", ActivateAt: timestamppb.New(now.Add(time.Minute)),
	}}}
	if err := projection.Apply(prepared, 1); err == nil || !strings.Contains(err.Error(), "out of order") {
		t.Fatalf("preparation before issuer error = %v", err)
	}

	established := &corev1.Event{Id: "evt_established", CreatedAt: timestamppb.New(now), Event: &corev1.Event_IssuerEstablished{IssuerEstablished: &corev1.IssuerEstablishedEvent{
		Issuer: "https://auth.example", SigningKeyRef: "system.oidc-signing.v1", SigningKeyId: "sig_initial",
	}}}
	if err := projection.Apply(established, 1); err != nil {
		t.Fatal(err)
	}
	request := &corev1.Event{Id: "evt_request", CreatedAt: timestamppb.New(now), Event: &corev1.Event_OidcSigningKeyRotationRequested{OidcSigningKeyRotationRequested: &corev1.OIDCSigningKeyRotationRequestedEvent{SigningKeyRef: "system.oidc-signing.key_next"}}}
	if err := projection.Apply(request, 2); err != nil {
		t.Fatal(err)
	}
	if err := projection.Apply(prepared, 3); err != nil {
		t.Fatal(err)
	}
	earlyActivation := &corev1.Event{Id: "evt_activated", CreatedAt: timestamppb.New(now), Event: &corev1.Event_OidcSigningKeyActivated{OidcSigningKeyActivated: &corev1.OIDCSigningKeyActivatedEvent{
		SigningKeyRef: "system.oidc-signing.key_next", SigningKeyId: "sig_next",
		PreviousSigningKeyRef: "system.oidc-signing.v1", PreviousSigningKeyId: "sig_initial",
		RetireAfter: timestamppb.New(now.Add(time.Hour)),
	}}}
	if err := projection.Apply(earlyActivation, 4); err == nil || !strings.Contains(err.Error(), "out of order") {
		t.Fatalf("early activation error = %v", err)
	}

	activationTime := now.Add(time.Minute)
	activation := &corev1.Event{Id: "evt_activated", CreatedAt: timestamppb.New(activationTime), Event: &corev1.Event_OidcSigningKeyActivated{OidcSigningKeyActivated: &corev1.OIDCSigningKeyActivatedEvent{
		SigningKeyRef: "system.oidc-signing.key_next", SigningKeyId: "sig_next",
		PreviousSigningKeyRef: "system.oidc-signing.v1", PreviousSigningKeyId: "sig_initial",
		RetireAfter: timestamppb.New(activationTime.Add(time.Hour)),
	}}}
	if err := projection.Apply(activation, 4); err != nil {
		t.Fatal(err)
	}
	earlyRetirement := &corev1.Event{Id: "evt_retire", CreatedAt: timestamppb.New(activationTime), Event: &corev1.Event_OidcSigningKeyRetirementRequested{OidcSigningKeyRetirementRequested: &corev1.OIDCSigningKeyRetirementRequestedEvent{
		SigningKeyRef: "system.oidc-signing.v1", SigningKeyId: "sig_initial",
	}}}
	if err := projection.Apply(earlyRetirement, 5); err == nil || !strings.Contains(err.Error(), "out of order") {
		t.Fatalf("early retirement error = %v", err)
	}
}

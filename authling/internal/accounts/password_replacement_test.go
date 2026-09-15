package accounts

import (
	"context"
	"errors"
	"testing"

	"google.golang.org/protobuf/proto"
	"hmans.de/authling/internal/evtstream"
	corev1 "hmans.de/authling/internal/pb/authling/core/v1"
	"hmans.de/chatto/pkg/events"
)

type passwordPublicationFault struct {
	*evtstream.Publisher
	appendPassword func(context.Context, *corev1.Event, uint64) (events.StreamPosition, error)
}

func (p passwordPublicationFault) AppendPasswordChanged(ctx context.Context, event *corev1.Event, tail uint64) (events.StreamPosition, error) {
	return p.appendPassword(ctx, event, tail)
}

func TestPasswordReplacementPublicationBoundaries(t *testing.T) {
	for _, recovery := range []bool{false, true} {
		name := "signed-in"
		if recovery {
			name = "recovery"
		}
		for _, boundary := range []string{"audit conflict", "credential conflict", "lost acknowledgement"} {
			t.Run(name+"/"+boundary, func(t *testing.T) {
				fixture := newSafetyFixture(t, t.TempDir(), nil)
				service := fixture.service
				ctx := accountTestContext(t)
				const email = "replacement@example.invalid"
				const original = "an uncommon original password"
				const replacement = "an uncommon replacement password"
				account, err := service.CreateLocal(ctx, email, original)
				if err != nil {
					t.Fatal(err)
				}
				var requestID, credentialID string
				var command func() (Account, error)
				kind := corev1.PasswordChangeKind_PASSWORD_CHANGE_KIND_SIGNED_IN
				if recovery {
					target, exists, err := service.RecordPasswordResetRequested(ctx, email)
					if err != nil || !exists {
						t.Fatalf("prepare recovery: %v", err)
					}
					requestID, credentialID = target.RequestEventID, target.CredentialEventID
					kind = corev1.PasswordChangeKind_PASSWORD_CHANGE_KIND_RECOVERY
					command = func() (Account, error) { return service.ResetPassword(ctx, target, replacement) }
				} else {
					target, err := service.PreparePasswordChange(ctx, account.ID, original, replacement)
					if err != nil {
						t.Fatal(err)
					}
					credentialID = target.CredentialEventID
					command = func() (Account, error) { return service.ChangePassword(ctx, target) }
				}
				other, _, _ := newSafetyReplica(t, fixture.js, fixture.stream, fixture.keys)
				lostAck := errors.New("injected password acknowledgement loss")
				calls := 0
				var first *corev1.Event
				var committed events.StreamPosition
				service.publisher = passwordPublicationFault{Publisher: fixture.publisher, appendPassword: func(ctx context.Context, event *corev1.Event, tail uint64) (events.StreamPosition, error) {
					calls++
					if calls == 1 {
						first = proto.Clone(event).(*corev1.Event)
						switch boundary {
						case "audit conflict":
							if _, _, err := other.RecordPasswordResetRequested(ctx, email); err != nil {
								t.Fatal(err)
							}
						case "credential conflict":
							target, err := other.PreparePasswordChange(ctx, account.ID, original, "a concurrent uncommon password")
							if err != nil {
								t.Fatal(err)
							}
							if _, err := other.ChangePassword(ctx, target); err != nil {
								t.Fatal(err)
							}
						}
					} else if !proto.Equal(first, event) {
						t.Fatal("retry rebuilt the event or encrypted verifier")
					}
					position, err := fixture.publisher.AppendPasswordChanged(ctx, event, tail)
					if err == nil {
						committed = position
					}
					if boundary == "lost acknowledgement" && err == nil {
						return events.StreamPosition{}, lostAck
					}
					return position, err
				}}
				updated, err := command()
				switch boundary {
				case "audit conflict":
					if err != nil || calls != 2 || updated.ID != account.ID || updated.AuthenticationVersion != account.AuthenticationVersion+1 {
						t.Fatalf("audit retry: calls=%d err=%v", calls, err)
					}
				case "credential conflict":
					if !errors.Is(err, ErrCredentialChanged) || calls != 1 {
						t.Fatalf("stale replacement: calls=%d err=%v", calls, err)
					}
				case "lost acknowledgement":
					if !errors.Is(err, lostAck) || calls != 1 {
						t.Fatalf("unknown commit retried: calls=%d err=%v", calls, err)
					}
				}
				payload := first.GetPasswordChanged()
				if payload.GetKind() != kind || payload.GetPasswordResetRequestEventId() != requestID || payload.GetPriorCredentialEventId() != credentialID || payload.GetAccountId() != account.ID || payload.GetCredentialEnvelopeVersion() != 1 {
					t.Fatal("password replacement changed its event contract")
				}
				if boundary != "credential conflict" {
					if err := service.handle.Projector().WaitFor(ctx, committed); err != nil {
						t.Fatal(err)
					}
					if _, err := service.AuthenticateLocal(ctx, email, replacement); err != nil {
						t.Fatalf("new password: %v", err)
					}
					if _, err := service.AuthenticateLocal(ctx, email, original); !errors.Is(err, ErrInvalidCredentials) {
						t.Fatalf("old password: %v", err)
					}
				}
				// A new replica must replay the exact same credential and event kind.
				replayed, _, _ := newSafetyReplica(t, fixture.js, fixture.stream, fixture.keys)
				want := replacement
				if boundary == "credential conflict" {
					want = "a concurrent uncommon password"
				}
				if _, err := replayed.AuthenticateLocal(ctx, email, want); err != nil {
					t.Fatalf("replayed credential: %v", err)
				}
			})
		}
	}
}

func TestPasswordReplacementRejectsIncompleteTargetsBeforePasswordWork(t *testing.T) {
	service := &Service{}
	for _, target := range []PasswordChangeTarget{
		{CredentialEventID: "evt_test", newPassword: "x"},
		{AccountID: "acc_test", newPassword: "x"},
		{AccountID: "acc_test", CredentialEventID: "evt_test"},
	} {
		if _, err := service.ChangePassword(t.Context(), target); !errors.Is(err, ErrCredentialChanged) {
			t.Fatalf("incomplete signed-in target: %v", err)
		}
	}
	for _, target := range []PasswordResetTarget{
		{CredentialEventID: "evt_test", RequestEventID: "evt_request"},
		{AccountID: "acc_test", RequestEventID: "evt_request"},
		{AccountID: "acc_test", CredentialEventID: "evt_test"},
	} {
		if _, err := service.ResetPassword(t.Context(), target, "x"); !errors.Is(err, ErrCredentialChanged) {
			t.Fatalf("incomplete recovery target: %v", err)
		}
	}
}

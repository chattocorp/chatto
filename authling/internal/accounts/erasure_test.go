package accounts

import (
	"context"
	"errors"
	"testing"
	"time"

	"hmans.de/authling/internal/evtstream"
	corev1 "hmans.de/authling/internal/pb/authling/core/v1"
	"hmans.de/chatto/pkg/events"
)

type erasurePublicationFault struct {
	*evtstream.Publisher
	committed bool
}

func (p erasurePublicationFault) AppendAccountErasure(ctx context.Context, request, release *corev1.Event, tail, registry uint64) (events.StreamPosition, error) {
	if p.committed {
		if _, err := p.Publisher.AppendAccountErasure(ctx, request, release, tail, registry); err != nil {
			return events.StreamPosition{}, err
		}
	}
	return events.StreamPosition{}, errors.New("injected lost acknowledgement")
}

func TestErasureRequestNeverPurgesKeysOnUnknownCommit(t *testing.T) {
	for _, committed := range []bool{false, true} {
		t.Run(map[bool]string{false: "not committed", true: "committed"}[committed], func(t *testing.T) {
			fixture := newSafetyFixture(t, t.TempDir(), nil)
			account, err := fixture.service.CreateLocal(t.Context(), "erasure@example.invalid", "a deliberately uncommon password")
			if err != nil {
				t.Fatal(err)
			}
			credential, _ := fixture.service.handle.Projection().credentialForAccount(account.ID)
			fixture.service.publisher = erasurePublicationFault{fixture.publisher, committed}
			if err := fixture.service.RequestErasure(t.Context(), account.ID, account.AuthenticationVersion); err == nil {
				t.Fatal("fault not observed")
			}
			for _, ref := range []string{credential.userKeyRef, credential.credentialKeyRef} {
				if _, err := fixture.keys.Get(t.Context(), ref); err != nil {
					t.Fatal("request path purged keys", err)
				}
			}
			activeErr := fixture.service.RequireActive(t.Context(), account.ID)
			if committed && !errors.Is(activeErr, ErrInvalidCredentials) {
				t.Fatalf("committed denial: %v", activeErr)
			}
			if !committed && activeErr != nil {
				t.Fatalf("uncommitted request denied account: %v", activeErr)
			}
		})
	}
}

type erasureDuringProfileAppend struct {
	*evtstream.Publisher
	erase func() error
}

func (p *erasureDuringProfileAppend) AppendProfileUpdated(ctx context.Context, event *corev1.Event, tail uint64) (events.StreamPosition, error) {
	if p.erase != nil {
		f := p.erase
		p.erase = nil
		if err := f(); err != nil {
			return events.StreamPosition{}, err
		}
	}
	return p.Publisher.AppendProfileUpdated(ctx, event, tail)
}
func TestProfileCannotCommitAfterConcurrentErasure(t *testing.T) {
	fixture := newSafetyFixture(t, t.TempDir(), nil)
	account, err := fixture.service.CreateLocal(t.Context(), "profile@example.invalid", "a deliberately uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	fixture.service.publisher = &erasureDuringProfileAppend{Publisher: fixture.publisher, erase: func() error {
		return fixture.service.RequestErasure(t.Context(), account.ID, account.AuthenticationVersion)
	}}
	if _, err := fixture.service.UpdateProfile(t.Context(), account.ID, "changed", "Changed"); !errors.Is(err, ErrCredentialChanged) {
		t.Fatalf("profile raced erasure: %v", err)
	}
	if err := fixture.service.handle.Projector().Err(); err != nil {
		t.Fatal("invalid history", err)
	}
}

func TestReplayRechecksErasureAfterKeyDisappears(t *testing.T) {
	fixture := newSafetyFixture(t, t.TempDir(), nil)
	account, err := fixture.service.CreateLocal(t.Context(), "replay-race@example.invalid", "a deliberately uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	credential, _ := fixture.service.handle.Projection().credentialForAccount(account.ID)
	projection := NewProjection(fixture.service.vault, fixture.service.handle.Projection().indexKey)
	checks := 0
	projection.SetErasureChecker(func(ctx context.Context, id string) (bool, error) {
		checks++
		if checks == 1 {
			// Commit and destroy after capturing a stale negative erasure result.
			if err := fixture.service.RequestErasure(ctx, id, account.AuthenticationVersion); err != nil {
				return false, err
			}
			if err := fixture.service.vault.DestroyAccountKeys(ctx, credential.userKeyRef, credential.credentialKeyRef); err != nil {
				return false, err
			}
			return false, nil
		}
		return errors.Is(fixture.service.RequireActive(ctx, id), ErrInvalidCredentials), nil
	})
	_, erased, err := projection.replayEmailDigest(account.ID, credential.userKeyRef, credential.credentialKeyRef, credential.emailCiphertext, credential.emailNonce, credential.emailAAD)
	if err != nil || !erased || checks != 2 {
		t.Fatalf("erasure replay checks=%d erased=%v error=%v", checks, erased, err)
	}
}

func TestLaggingReplicaCannotReadDeletedAccount(t *testing.T) {
	fixture := newSafetyFixture(t, t.TempDir(), nil)
	account, err := fixture.service.CreateLocal(t.Context(), "replica@example.invalid", "a deliberately uncommon password")
	if err != nil {
		t.Fatal(err)
	}
	replica, _, stop := newSafetyReplica(t, fixture.js, fixture.stream, fixture.keys)
	stop()
	if err := fixture.service.RequestErasure(t.Context(), account.ID, account.AuthenticationVersion); err != nil {
		t.Fatal(err)
	}
	// The stopped replica still has the old encrypted account and the keys exist.
	if _, ok := replica.Get(account.ID); !ok {
		t.Fatal("fixture has no stale account")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer cancel()
	if _, err := replica.Profile(ctx, account.ID); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("stale replica disclosed profile")
	}
	sessionCtx, sessionCancel := context.WithTimeout(t.Context(), 150*time.Millisecond)
	defer sessionCancel()
	if _, ok, err := replica.AuthenticationVersion(sessionCtx, account.ID); ok || !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("stale replica accepted session generation")
	}
}

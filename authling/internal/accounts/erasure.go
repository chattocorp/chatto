package accounts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/authling/internal/evtstream"
	"hmans.de/authling/internal/ids"
	corev1 "hmans.de/authling/internal/pb/authling/core/v1"
	"hmans.de/chatto/pkg/datacrypto"
	"hmans.de/chatto/pkg/events"
)

// SetErasureChecker configures the key-free replay barrier before Run starts.
func (p *Projection) SetErasureChecker(check func(context.Context, string) (bool, error)) {
	p.checkErasure = check
}

// replayEmailDigest only omits decryption when a committed erasure proves the
// loss is intentional. Structural event validation still runs for erased data.
func (p *Projection) replayEmailDigest(id, userRef, dataRef string, ciphertext, nonce, aad []byte) ([32]byte, bool, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	check := func() (bool, error) {
		if p.checkErasure == nil {
			return false, nil
		}
		return p.checkErasure(ctx, id)
	}
	erased, err := check()
	if err != nil {
		return [32]byte{}, false, err
	}
	if erased {
		return [32]byte{}, true, nil
	}
	key, err := p.vault.ResolveDataKey(ctx, dataRef, userRef)
	if err != nil {
		// Another replica can commit deletion and purge keys after the first check.
		erased, checkErr := check()
		if checkErr != nil {
			return [32]byte{}, false, checkErr
		}
		if erased {
			return [32]byte{}, true, nil
		}
		return [32]byte{}, false, fmt.Errorf("resolve active account key: %w", err)
	}
	defer clear(key)
	plain, err := datacrypto.Open(key, ciphertext, nonce, aad)
	if err != nil {
		return [32]byte{}, false, fmt.Errorf("decrypt account email: %w", err)
	}
	defer clear(plain)
	if len(plain) == 0 {
		return [32]byte{}, false, fmt.Errorf("empty account email")
	}
	return digest(p.indexKey, string(plain)), false, nil
}

// RequireActive crosses the durable account tail before exposing protected data.
func (s *Service) RequireActive(ctx context.Context, id string) error {
	tail, err := s.publisher.AccountTail(ctx, id)
	if err != nil {
		return err
	}
	if tail == 0 {
		return ErrInvalidCredentials
	}
	subject, err := evtstream.AccountSubject(id)
	if err != nil {
		return err
	}
	if err := s.handle.Projector().WaitFor(ctx, events.SubjectPosition(subject, tail)); err != nil {
		return err
	}
	if _, ok := s.handle.Projection().Get(id); !ok {
		return ErrInvalidCredentials
	}
	return nil
}

// RequestErasure permanently denies access and releases the email claim. The
// version must be from a fresh successful password check for this account.
// Only the durable worker destroys keys, including after unknown commit results.
func (s *Service) RequestErasure(ctx context.Context, id string, version uint64) error {
	for range 5 {
		if err := s.RequireActive(ctx, id); err != nil {
			return err
		}
		account, ok := s.handle.Projection().Get(id)
		if !ok || account.AuthenticationVersion != version {
			return ErrCredentialChanged
		}
		credential, ok := s.handle.Projection().credentialForIdentityMutation(id)
		if !ok {
			return ErrCredentialChanged
		}
		tail, _, err := s.identityCredentialAtTail(ctx, id, credential.eventID)
		if err != nil {
			return err
		}
		registryTail, err := s.publisher.AccountRegistryTail(ctx)
		if err != nil {
			return err
		}
		if err := s.handle.Projector().WaitFor(ctx, events.SubjectPosition(evtstream.AccountRegistrySubject(), registryTail)); err != nil {
			return err
		}
		// Re-evaluate after both barriers: a staged email change can advance version.
		current, ok := s.handle.Projection().accountAtCredential(id, credential.eventID)
		if !ok || current.AuthenticationVersion != version {
			return ErrCredentialChanged
		}
		requestID, err := ids.New("evt")
		if err != nil {
			return err
		}
		releaseID, err := ids.New("evt")
		if err != nil {
			return err
		}
		request := &corev1.Event{Id: requestID, CreatedAt: timestamppb.Now(), Event: &corev1.Event_AccountErasureRequested{AccountErasureRequested: &corev1.AccountErasureRequestedEvent{AccountId: id, PriorCredentialEventId: credential.eventID, UserKeyRef: credential.userKeyRef, CredentialKeyRef: credential.credentialKeyRef}}}
		release := &corev1.Event{Id: releaseID, CreatedAt: timestamppb.Now(), Event: &corev1.Event_EmailReleased{EmailReleased: &corev1.EmailReleasedEvent{AccountId: id, ErasureRequestEventId: requestID}}}
		position, err := s.publisher.AppendAccountErasure(ctx, request, release, tail, registryTail)
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		if err != nil {
			return err
		}
		return s.handle.Projector().WaitFor(ctx, position)
	}
	return fmt.Errorf("account erasure conflict")
}

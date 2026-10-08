// Package authentication coordinates local credential checks and online
// guessing defenses.
package authentication

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/storage"
)

const (
	attemptWindow         = 15 * time.Minute
	maxFailedAttempts     = 10
	maxConcurrentPassword = 4
)

// ErrBusy indicates that this process has no password-verification capacity.
var ErrBusy = errors.New("authentication capacity exhausted")

type accountAuthenticator interface {
	AuthenticateLocal(context.Context, string, string) (accounts.Account, error)
	PreparePasswordChange(context.Context, string, string, string) (accounts.PasswordChangeTarget, error)
	ChangePassword(context.Context, accounts.PasswordChangeTarget) (accounts.Account, error)
	PrepareEmailChange(context.Context, string, string, string) (accounts.EmailChangeTarget, error)
}

type limitState struct {
	key      string
	revision uint64
	limited  bool
}

// Service applies distributed attempt limits around local credentials.
type Service struct {
	kv       storage.KeyValue
	key      []byte
	accounts accountAuthenticator
	slots    chan struct{}
}

// New constructs the local authentication boundary.
func New(kv storage.KeyValue, key []byte, accountService accountAuthenticator) *Service {
	return &Service{
		kv:       kv,
		key:      append([]byte(nil), key...),
		accounts: accountService,
		slots:    make(chan struct{}, maxConcurrentPassword),
	}
}

// Login verifies one local credential. Every identifier follows the same
// durable throttling and password-hashing path.
func (s *Service) Login(ctx context.Context, email, password string) (accounts.Account, error) {
	release, err := s.acquirePasswordSlot(ctx)
	if err != nil {
		return accounts.Account{}, err
	}
	defer release()

	normalized := accounts.NormalizeEmail(email)
	key := s.attemptKey("login-attempt", normalized)
	limit, err := s.readLimit(ctx, key)
	if err != nil {
		return accounts.Account{}, fmt.Errorf("read login attempt limit: %w", err)
	}
	account, authErr := s.accounts.AuthenticateLocal(ctx, normalized, password)
	if authErr != nil {
		if !errors.Is(authErr, accounts.ErrInvalidCredentials) {
			return accounts.Account{}, authErr
		}
		if !limit.limited {
			if err := s.recordFailure(ctx, key); err != nil {
				return accounts.Account{}, fmt.Errorf("record failed login: %w", err)
			}
		}
		return accounts.Account{}, accounts.ErrInvalidCredentials
	}
	if limit.limited {
		return accounts.Account{}, accounts.ErrInvalidCredentials
	}
	if limit.revision > 0 {
		// Delete only the state observed before password verification. A
		// concurrent failure advances the revision and must remain recorded.
		_ = s.kv.Delete(ctx, limit.key, jetstream.LastRevision(limit.revision))
	}
	return account, nil
}

// ReauthenticateEmailChange applies the same distributed guessing and local
// Argon2 concurrency defenses as login while binding the proof to an account.
func (s *Service) ReauthenticateEmailChange(ctx context.Context, accountID, password, newEmail string) (accounts.EmailChangeTarget, error) {
	release, err := s.acquirePasswordSlot(ctx)
	if err != nil {
		return accounts.EmailChangeTarget{}, err
	}
	defer release()

	key := s.attemptKey("email-change-reauth-attempt", accountID)
	limit, err := s.readLimit(ctx, key)
	if err != nil {
		return accounts.EmailChangeTarget{}, fmt.Errorf("read email change reauthentication limit: %w", err)
	}
	target, authErr := s.accounts.PrepareEmailChange(ctx, accountID, password, newEmail)
	passwordAccepted := authErr == nil || errors.Is(authErr, accounts.ErrEmailUnchanged)
	if !passwordAccepted {
		if !errors.Is(authErr, accounts.ErrInvalidCredentials) {
			return accounts.EmailChangeTarget{}, authErr
		}
		if !limit.limited {
			if err := s.recordFailure(ctx, key); err != nil {
				return accounts.EmailChangeTarget{}, fmt.Errorf("record failed email change reauthentication: %w", err)
			}
		}
		return accounts.EmailChangeTarget{}, accounts.ErrInvalidCredentials
	}
	if limit.limited {
		return accounts.EmailChangeTarget{}, accounts.ErrInvalidCredentials
	}
	if limit.revision > 0 {
		_ = s.kv.Delete(ctx, limit.key, jetstream.LastRevision(limit.revision))
	}
	return target, authErr
}

// ChangePassword reauthenticates a signed-in account under the same
// distributed guessing and Argon2 concurrency defenses as login, then commits
// the replacement while that exact credential remains current.
func (s *Service) ChangePassword(ctx context.Context, accountID, currentPassword, newPassword string) (accounts.Account, error) {
	release, err := s.acquirePasswordSlot(ctx)
	if err != nil {
		return accounts.Account{}, err
	}
	defer release()

	key := s.attemptKey("password-change-reauth-attempt", accountID)
	limit, err := s.readLimit(ctx, key)
	if err != nil {
		return accounts.Account{}, fmt.Errorf("read password change reauthentication limit: %w", err)
	}
	target, authErr := s.accounts.PreparePasswordChange(ctx, accountID, currentPassword, newPassword)
	passwordAccepted := authErr == nil || errors.Is(authErr, accounts.ErrInvalidPassword) || errors.Is(authErr, accounts.ErrPasswordUnchanged)
	if !passwordAccepted {
		if !errors.Is(authErr, accounts.ErrInvalidCredentials) {
			return accounts.Account{}, authErr
		}
		if !limit.limited {
			if err := s.recordFailure(ctx, key); err != nil {
				return accounts.Account{}, fmt.Errorf("record failed password change reauthentication: %w", err)
			}
		}
		return accounts.Account{}, accounts.ErrInvalidCredentials
	}
	if limit.limited {
		return accounts.Account{}, accounts.ErrInvalidCredentials
	}
	if limit.revision > 0 {
		_ = s.kv.Delete(ctx, limit.key, jetstream.LastRevision(limit.revision))
	}
	if authErr != nil {
		return accounts.Account{}, authErr
	}
	return s.accounts.ChangePassword(ctx, target)
}

func (s *Service) acquirePasswordSlot(ctx context.Context) (func(), error) {
	select {
	case s.slots <- struct{}{}:
		return func() { <-s.slots }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
		return nil, ErrBusy
	}
}

func (s *Service) readLimit(ctx context.Context, key string) (limitState, error) {
	count, revision, err := storage.ReadCounter(ctx, s.kv, key)
	if err != nil {
		return limitState{}, err
	}
	return limitState{key: key, revision: revision, limited: count >= maxFailedAttempts}, nil
}

// recordFailure counts one failed attempt. A counter at the limit stays
// unchanged. A storage failure is returned without a retry.
func (s *Service) recordFailure(ctx context.Context, key string) error {
	_, err := storage.IncrementCounter(ctx, s.kv, key, maxFailedAttempts, attemptWindow)
	return err
}

func (s *Service) attemptKey(namespace, identifier string) string {
	return storage.DigestKey("login-limit.", s.key, namespace+"\x00"+identifier)
}

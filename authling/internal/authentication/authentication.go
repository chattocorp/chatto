// Package authentication coordinates local credential checks and online
// guessing defenses.
package authentication

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
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

type attemptCounter struct {
	Count int `json:"count"`
}

type reservation struct {
	key      string
	revision uint64
	limited  bool
}

// Service applies distributed attempt limits around local credentials.
type Service struct {
	kv       jetstream.KeyValue
	js       jetstream.JetStream
	key      []byte
	accounts *accounts.Service
	slots    chan struct{}
}

// New constructs the local authentication boundary.
func New(kv jetstream.KeyValue, js jetstream.JetStream, key []byte, accountService *accounts.Service) *Service {
	return &Service{
		kv:       kv,
		js:       js,
		key:      append([]byte(nil), key...),
		accounts: accountService,
		slots:    make(chan struct{}, maxConcurrentPassword),
	}
}

// Login verifies one local credential. Every identifier follows the same
// durable throttling and password-hashing path.
func (s *Service) Login(ctx context.Context, email, password string) (accounts.Account, error) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	case <-ctx.Done():
		return accounts.Account{}, ctx.Err()
	default:
		return accounts.Account{}, ErrBusy
	}

	normalized := accounts.NormalizeEmail(email)
	reserved, err := s.reserveAttempt(ctx, normalized)
	if err != nil {
		return accounts.Account{}, fmt.Errorf("reserve login attempt: %w", err)
	}
	account, authErr := s.accounts.AuthenticateLocal(ctx, normalized, password)
	if authErr != nil {
		if !errors.Is(authErr, accounts.ErrInvalidCredentials) {
			_ = s.rollbackAttempt(ctx, reserved)
			return accounts.Account{}, authErr
		}
		return accounts.Account{}, accounts.ErrInvalidCredentials
	}
	if reserved.limited {
		return accounts.Account{}, accounts.ErrInvalidCredentials
	}
	// Clear only the revision reserved by this successful request. A newer
	// concurrent attempt must keep its own failure budget intact.
	_ = s.kv.Delete(ctx, reserved.key, jetstream.LastRevision(reserved.revision))
	return account, nil
}

func (s *Service) reserveAttempt(ctx context.Context, email string) (reservation, error) {
	key := s.attemptKey(email)
	for range 16 {
		entry, err := s.kv.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			data, _ := json.Marshal(attemptCounter{Count: 1})
			revision, createErr := s.kv.Create(ctx, key, data, jetstream.KeyTTL(attemptWindow))
			if createErr == nil {
				return reservation{key: key, revision: revision}, nil
			}
			continue
		}
		if err != nil {
			return reservation{}, err
		}
		var counter attemptCounter
		if json.Unmarshal(entry.Value(), &counter) != nil || counter.Count < 1 {
			return reservation{}, fmt.Errorf("decode login attempt counter")
		}
		if counter.Count >= maxFailedAttempts {
			return reservation{key: key, revision: entry.Revision(), limited: true}, nil
		}
		counter.Count++
		data, _ := json.Marshal(counter)
		revision, updateErr := storage.UpdateKeyWithTTL(ctx, s.js, storage.RuntimeStateBucket, key, data, entry.Revision(), attemptWindow)
		if updateErr == nil {
			return reservation{key: key, revision: revision}, nil
		}
	}
	return reservation{}, fmt.Errorf("update login attempt counter after repeated conflicts")
}

func (s *Service) rollbackAttempt(ctx context.Context, reserved reservation) error {
	if reserved.limited {
		return nil
	}
	entry, err := s.kv.Get(ctx, reserved.key)
	if err != nil || entry.Revision() != reserved.revision {
		return nil
	}
	var counter attemptCounter
	if json.Unmarshal(entry.Value(), &counter) != nil {
		return nil
	}
	if counter.Count <= 1 {
		return s.kv.Delete(ctx, reserved.key, jetstream.LastRevision(entry.Revision()))
	}
	counter.Count--
	data, _ := json.Marshal(counter)
	_, err = storage.UpdateKeyWithTTL(ctx, s.js, storage.RuntimeStateBucket, reserved.key, data, entry.Revision(), attemptWindow)
	return err
}

func (s *Service) attemptKey(email string) string {
	digest := hmac.New(sha256.New, s.key)
	_, _ = digest.Write([]byte("login-attempt\x00" + email))
	return "login-limit." + base64.RawURLEncoding.EncodeToString(digest.Sum(nil))
}

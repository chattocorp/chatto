// Package sessions owns Authling's first-party browser session lifecycle.
package sessions

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/storage"
	"hmans.de/chatto/pkg/datacrypto"
)

const (
	// AbsoluteLifetime forces a full authentication ceremony each day.
	AbsoluteLifetime = 24 * time.Hour
	// InactivityLifetime expires a session that has not served a request.
	InactivityLifetime = time.Hour
	touchInterval      = 5 * time.Minute
	tokenBytes         = 32
)

// ErrNotFound deliberately combines absent, expired, and malformed browser
// sessions so untrusted cookies never reveal server-side state.
var ErrNotFound = errors.New("session not found")

// Session is the authenticated server-side browser state.
type Session struct {
	AccountID             string    `json:"account_id"`
	AuthenticationVersion uint64    `json:"authentication_version,omitempty"`
	CreatedAt             time.Time `json:"created_at"`
	LastSeenAt            time.Time `json:"last_seen_at"`
	ExpiresAt             time.Time `json:"expires_at"`
}

// AuthenticationVersionResolver returns the durable credential generation for
// an account. A changed generation invalidates sessions issued before it.
type AuthenticationVersionResolver func(accountID string) (uint64, bool)

type sealedState struct {
	Version    int    `json:"version"`
	Nonce      []byte `json:"nonce"`
	Ciphertext []byte `json:"ciphertext"`
}

// Service stores encrypted session state in Authling's runtime KV bucket.
type Service struct {
	kv                    jetstream.KeyValue
	js                    jetstream.JetStream
	key                   []byte
	now                   func() time.Time
	authenticationVersion AuthenticationVersionResolver
}

// New constructs the browser-session boundary.
func New(kv jetstream.KeyValue, js jetstream.JetStream, key []byte, authenticationVersion AuthenticationVersionResolver) *Service {
	return &Service{kv: kv, js: js, key: append([]byte(nil), key...), now: time.Now, authenticationVersion: authenticationVersion}
}

// Create starts a new authenticated browser session and returns its bearer
// token. Only the token belongs in the browser cookie.
func (s *Service) Create(ctx context.Context, accountID string) (string, Session, error) {
	return s.create(ctx, accountID, nil)
}

// CreateAtAuthenticationVersion starts a browser session only for the exact
// credential generation that authorized the surrounding operation. A
// concurrent password or email change makes the new session stale instead of
// silently upgrading it to the later generation.
func (s *Service) CreateAtAuthenticationVersion(ctx context.Context, accountID string, expectedVersion uint64) (string, Session, error) {
	return s.create(ctx, accountID, &expectedVersion)
}

func (s *Service) create(ctx context.Context, accountID string, expectedVersion *uint64) (string, Session, error) {
	if strings.TrimSpace(accountID) == "" {
		return "", Session{}, fmt.Errorf("account id is required")
	}
	if expectedVersion != nil && s.authenticationVersion == nil {
		return "", Session{}, fmt.Errorf("create generation-bound session without an authentication version resolver")
	}
	random := make([]byte, tokenBytes)
	if _, err := rand.Read(random); err != nil {
		return "", Session{}, fmt.Errorf("generate session token: %w", err)
	}
	token := base64.RawURLEncoding.EncodeToString(random)
	clear(random)
	now := s.now().UTC()
	state := Session{AccountID: accountID, CreatedAt: now, LastSeenAt: now, ExpiresAt: now.Add(AbsoluteLifetime)}
	if s.authenticationVersion != nil {
		version, ok := s.authenticationVersion(accountID)
		if !ok {
			return "", Session{}, fmt.Errorf("create session for absent account")
		}
		if expectedVersion != nil && version != *expectedVersion {
			return "", Session{}, fmt.Errorf("create session after authentication generation changed")
		}
		state.AuthenticationVersion = version
	}
	key := s.sessionKey(token)
	data, err := s.seal(key, state)
	if err != nil {
		return "", Session{}, err
	}
	revision, err := s.kv.Create(ctx, key, data, jetstream.KeyTTL(AbsoluteLifetime))
	if err != nil {
		return "", Session{}, fmt.Errorf("store session: %w", err)
	}
	if expectedVersion != nil {
		version, ok := s.authenticationVersion(accountID)
		if !ok || version != *expectedVersion {
			cleanupContext, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = s.kv.Delete(cleanupContext, key, jetstream.LastRevision(revision))
			return "", Session{}, fmt.Errorf("create session while authentication generation changed")
		}
	}
	return token, state, nil
}

// Validate authenticates a bearer and advances its inactivity frontier at a
// bounded cadence. Absolute expiry never slides.
func (s *Service) Validate(ctx context.Context, token string) (Session, error) {
	return s.validate(ctx, token, true)
}

// Inspect authenticates a bearer without extending its inactivity lifetime.
// Long-lived connections use it for passive revocation and expiry checks.
func (s *Service) Inspect(ctx context.Context, token string) (Session, error) {
	return s.validate(ctx, token, false)
}

func (s *Service) validate(ctx context.Context, token string, touch bool) (Session, error) {
	if !validToken(token) {
		return Session{}, ErrNotFound
	}
	key := s.sessionKey(token)
	for range 3 {
		entry, state, err := s.read(ctx, key)
		if err != nil {
			return Session{}, err
		}
		now := s.now().UTC()
		if !now.Before(state.ExpiresAt) || now.Sub(state.LastSeenAt) >= InactivityLifetime {
			_ = s.kv.Delete(ctx, key)
			return Session{}, ErrNotFound
		}
		if s.authenticationVersion != nil {
			version, ok := s.authenticationVersion(state.AccountID)
			if !ok || version != state.AuthenticationVersion {
				_ = s.kv.Delete(ctx, key)
				return Session{}, ErrNotFound
			}
		}
		if !touch {
			return state, nil
		}
		if now.Sub(state.LastSeenAt) < touchInterval {
			return state, nil
		}
		state.LastSeenAt = now
		remaining := state.ExpiresAt.Sub(now)
		data, err := s.seal(key, state)
		if err != nil {
			return Session{}, err
		}
		if _, err := storage.UpdateKeyWithTTL(ctx, s.js, storage.RuntimeStateBucket, key, data, entry.Revision(), remaining); err == nil {
			return state, nil
		}
		// A concurrent request may have touched or revoked the session. Re-read
		// rather than allowing an older record to resurrect it.
	}
	return Session{}, fmt.Errorf("update session activity after repeated conflicts")
}

// Revoke invalidates a browser session server-side. It is idempotent for
// absent or syntactically invalid tokens.
func (s *Service) Revoke(ctx context.Context, token string) error {
	if !validToken(token) {
		return nil
	}
	key := s.sessionKey(token)
	for range 4 {
		entry, err := s.kv.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read session for revocation: %w", err)
		}
		if err := s.kv.Delete(ctx, key, jetstream.LastRevision(entry.Revision())); err == nil {
			return nil
		}
		// A concurrent activity update may have advanced the revision. Retry so
		// logout cannot leave the newly touched session alive.
	}
	return fmt.Errorf("revoke session after repeated conflicts")
}

func (s *Service) read(ctx context.Context, key string) (jetstream.KeyValueEntry, Session, error) {
	entry, err := s.kv.Get(ctx, key)
	if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
		return nil, Session{}, ErrNotFound
	}
	if err != nil {
		return nil, Session{}, fmt.Errorf("read session: %w", err)
	}
	var sealed sealedState
	if err := json.Unmarshal(entry.Value(), &sealed); err != nil || sealed.Version != 1 {
		return nil, Session{}, fmt.Errorf("decode session envelope")
	}
	plain, err := datacrypto.Open(s.key, sealed.Ciphertext, sealed.Nonce, sessionAAD(key))
	if err != nil {
		return nil, Session{}, fmt.Errorf("decrypt session: %w", err)
	}
	defer clear(plain)
	var state Session
	if err := json.Unmarshal(plain, &state); err != nil || state.AccountID == "" || state.CreatedAt.IsZero() || state.LastSeenAt.Before(state.CreatedAt) || state.LastSeenAt.After(state.ExpiresAt) || !state.ExpiresAt.After(state.CreatedAt) {
		return nil, Session{}, fmt.Errorf("decode session state")
	}
	return entry, state, nil
}

func (s *Service) seal(key string, state Session) ([]byte, error) {
	plain, err := json.Marshal(state)
	if err != nil {
		return nil, fmt.Errorf("encode session state: %w", err)
	}
	sealed, err := datacrypto.Seal(s.key, plain, sessionAAD(key))
	clear(plain)
	if err != nil {
		return nil, fmt.Errorf("encrypt session: %w", err)
	}
	data, err := json.Marshal(sealedState{Version: 1, Nonce: sealed.Nonce, Ciphertext: sealed.Ciphertext})
	if err != nil {
		return nil, fmt.Errorf("encode session envelope: %w", err)
	}
	return data, nil
}

func (s *Service) sessionKey(token string) string {
	digest := hmac.New(sha256.New, s.key)
	_, _ = digest.Write([]byte("session\x00" + token))
	return "session." + base64.RawURLEncoding.EncodeToString(digest.Sum(nil))
}

func sessionAAD(key string) []byte {
	return []byte("authling:runtime-session:v1\x00" + key)
}

func validToken(token string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil && len(decoded) == tokenBytes
}

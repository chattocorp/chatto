// Package passwordreset owns Authling's verified-email password recovery workflow.
package passwordreset

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/email"
	"hmans.de/authling/internal/storage"
	"hmans.de/chatto/pkg/datacrypto"
)

const FlowTTL = 15 * time.Minute
const maxWrongAttempts = 5
const maxCompletionAttempts = 5
const maxDeliveredCodes = 10
const maxGlobalDeliveredCodes = 1000
const maxConcurrentDeliveries = 8
const maxConcurrentCompletions = 4

var (
	ErrInvalidEmail   = errors.New("enter a valid email address")
	ErrInvalidCode    = errors.New("the code is invalid or has expired")
	ErrInvalidFlow    = errors.New("the password reset has expired; start again")
	errTooManyCodes   = errors.New("too many password reset codes requested")
	errCompletionBusy = errors.New("password reset completion capacity exhausted")
)

type flowState struct {
	Target             accounts.PasswordResetTarget `json:"target"`
	CodeDigest         []byte                       `json:"code_digest"`
	WrongAttempts      int                          `json:"wrong_attempts"`
	Verified           bool                         `json:"verified"`
	Completing         bool                         `json:"completing"`
	CompletionAttempts int                          `json:"completion_attempts"`
	ExpiresAt          time.Time                    `json:"expires_at"`
}

type sealedState struct {
	Version           int `json:"version"`
	Nonce, Ciphertext []byte
}

// Service coordinates expiring recovery state, email delivery, and durable
// password changes without exposing whether an address has an account.
type Service struct {
	kv              jetstream.KeyValue
	js              jetstream.JetStream
	key             []byte
	sender          email.Sender
	accounts        *accounts.Service
	deliverySlots   chan struct{}
	completionSlots chan struct{}
}

// New constructs the password-reset workflow.
func New(kv jetstream.KeyValue, js jetstream.JetStream, key []byte, sender email.Sender, accountService *accounts.Service) *Service {
	return &Service{kv: kv, js: js, key: append([]byte(nil), key...), sender: sender, accounts: accountService, deliverySlots: make(chan struct{}, maxConcurrentDeliveries), completionSlots: make(chan struct{}, maxConcurrentCompletions)}
}

// PasswordMinimumLength returns the active local password policy for form rendering.
func (s *Service) PasswordMinimumLength() int { return s.accounts.PasswordMinimumLength() }

// Start creates an opaque recovery flow and sends a six-digit code. Existing
// and absent addresses take the same externally observable delivery path.
func (s *Service) Start(ctx context.Context, rawEmail string) (string, error) {
	normalized, err := normalizeAndValidateEmail(rawEmail)
	if err != nil {
		return "", err
	}
	token, err := randomToken(32)
	if err != nil {
		return "", err
	}
	code, err := verificationCode()
	if err != nil {
		return "", err
	}
	if err := s.reserveDelivery(ctx, normalized); err != nil {
		return "", err
	}
	reserved := true
	delivered := false
	defer func() {
		if reserved && !delivered {
			cleanupContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.rollbackDelivery(cleanupContext, normalized)
		}
	}()
	target, _, err := s.accounts.RecordPasswordResetRequested(ctx, normalized)
	if err != nil {
		return "", fmt.Errorf("record password reset request: %w", err)
	}
	state := flowState{Target: target, CodeDigest: keyedDigest(s.key, "code\x00"+token+"\x00"+code), ExpiresAt: time.Now().UTC().Add(FlowTTL)}
	key := s.flowKey(token)
	data, err := s.seal(key, state)
	if err != nil {
		return "", err
	}
	if _, err := s.kv.Create(ctx, key, data, jetstream.KeyTTL(FlowTTL)); err != nil {
		return "", fmt.Errorf("store password reset flow: %w", err)
	}
	body := fmt.Sprintf("Your Authling password reset code is %s.\n\nIt expires in 15 minutes. If you did not request this, you can ignore this message.\n", code)
	if err := s.send(ctx, email.Message{To: normalized, Subject: "Your Authling password reset code", Body: body}); err != nil {
		_ = s.kv.Delete(ctx, key)
		return "", fmt.Errorf("deliver password reset code: %w", err)
	}
	delivered = true
	return token, nil
}

// Verify consumes a correct OTP logically. Wrong and repeated attempts share
// one bounded counter and one public error.
func (s *Service) Verify(ctx context.Context, token, code string) error {
	key := s.flowKey(token)
	entry, state, err := s.read(ctx, key)
	if err != nil || !time.Now().Before(state.ExpiresAt) || state.Verified || state.WrongAttempts >= maxWrongAttempts {
		return ErrInvalidCode
	}
	want := keyedDigest(s.key, "code\x00"+token+"\x00"+strings.TrimSpace(code))
	if !hmac.Equal(state.CodeDigest, want) {
		state.WrongAttempts++
		if _, updateErr := s.update(ctx, key, entry.Revision(), state); updateErr != nil {
			return ErrInvalidCode
		}
		return ErrInvalidCode
	}
	state.Verified = true
	state.CodeDigest = nil
	_, err = s.update(ctx, key, entry.Revision(), state)
	return err
}

// Complete replaces the password only when the credential bound to the flow
// is still current. The flow is consumed after success or a stale binding.
func (s *Service) Complete(ctx context.Context, token, password string) (accounts.Account, error) {
	key := s.flowKey(token)
	entry, state, err := s.read(ctx, key)
	if err != nil || !time.Now().Before(state.ExpiresAt) || !state.Verified || state.Completing || state.CompletionAttempts >= maxCompletionAttempts {
		return accounts.Account{}, ErrInvalidFlow
	}
	select {
	case s.completionSlots <- struct{}{}:
		defer func() { <-s.completionSlots }()
	case <-ctx.Done():
		return accounts.Account{}, ctx.Err()
	default:
		return accounts.Account{}, errCompletionBusy
	}
	state.Completing = true
	state.CompletionAttempts++
	completionRevision, err := s.update(ctx, key, entry.Revision(), state)
	if err != nil {
		return accounts.Account{}, ErrInvalidFlow
	}
	if state.Target.AccountID == "" || state.Target.CredentialEventID == "" {
		_ = s.kv.Delete(ctx, key, jetstream.LastRevision(completionRevision))
		return accounts.Account{}, ErrInvalidFlow
	}
	account, err := s.accounts.ResetPassword(ctx, state.Target, password)
	if err != nil {
		if errors.Is(err, accounts.ErrCredentialChanged) {
			_ = s.kv.Delete(ctx, key, jetstream.LastRevision(completionRevision))
			return accounts.Account{}, ErrInvalidFlow
		}
		state.Completing = false
		_, _ = s.update(ctx, key, completionRevision, state)
		return accounts.Account{}, err
	}
	_ = s.kv.Delete(ctx, key, jetstream.LastRevision(completionRevision))
	return account, nil
}

func (s *Service) flowKey(token string) string {
	return "password-reset." + base64.RawURLEncoding.EncodeToString(keyedDigest(s.key, "flow\x00"+token))
}

func (s *Service) seal(key string, state flowState) ([]byte, error) {
	plain, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	defer clear(plain)
	sealed, err := datacrypto.Seal(s.key, plain, []byte("authling:password-reset-runtime:v1\x00"+key))
	if err != nil {
		return nil, err
	}
	return json.Marshal(sealedState{Version: 1, Nonce: sealed.Nonce, Ciphertext: sealed.Ciphertext})
}

func (s *Service) read(ctx context.Context, key string) (jetstream.KeyValueEntry, flowState, error) {
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		return nil, flowState{}, err
	}
	var sealed sealedState
	if err := json.Unmarshal(entry.Value(), &sealed); err != nil || sealed.Version != 1 {
		return nil, flowState{}, ErrInvalidFlow
	}
	plain, err := datacrypto.Open(s.key, sealed.Ciphertext, sealed.Nonce, []byte("authling:password-reset-runtime:v1\x00"+key))
	if err != nil {
		return nil, flowState{}, ErrInvalidFlow
	}
	defer clear(plain)
	var state flowState
	if err := json.Unmarshal(plain, &state); err != nil {
		return nil, flowState{}, ErrInvalidFlow
	}
	return entry, state, nil
}

func (s *Service) update(ctx context.Context, key string, revision uint64, state flowState) (uint64, error) {
	data, err := s.seal(key, state)
	if err != nil {
		return 0, err
	}
	remaining := time.Until(state.ExpiresAt)
	if remaining <= 0 {
		return 0, ErrInvalidFlow
	}
	updated, err := storage.UpdateKeyWithTTL(ctx, s.js, storage.RuntimeStateBucket, key, data, revision, remaining)
	if err != nil {
		return 0, ErrInvalidFlow
	}
	return updated, nil
}

type deliveryCounter struct {
	Count int `json:"count"`
}

func (s *Service) deliveryKey(address string) string {
	return "password-reset-limit." + base64.RawURLEncoding.EncodeToString(keyedDigest(s.key, "delivery\x00"+address))
}

func (s *Service) reserveDelivery(ctx context.Context, address string) error {
	if err := s.reserveCounter(ctx, "password-reset-limit.global", maxGlobalDeliveredCodes); err != nil {
		return err
	}
	if err := s.reserveCounter(ctx, s.deliveryKey(address), maxDeliveredCodes); err != nil {
		_ = s.rollbackCounter(ctx, "password-reset-limit.global")
		return err
	}
	return nil
}

func (s *Service) reserveCounter(ctx context.Context, key string, limit int) error {
	for range 16 {
		entry, err := s.kv.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			data, _ := json.Marshal(deliveryCounter{Count: 1})
			if _, err := s.kv.Create(ctx, key, data, jetstream.KeyTTL(FlowTTL)); err == nil {
				return nil
			}
			continue
		}
		if err != nil {
			return fmt.Errorf("read password reset delivery limit: %w", err)
		}
		var counter deliveryCounter
		if json.Unmarshal(entry.Value(), &counter) != nil {
			return fmt.Errorf("decode password reset delivery limit")
		}
		if counter.Count >= limit {
			return errTooManyCodes
		}
		counter.Count++
		data, _ := json.Marshal(counter)
		if _, err := storage.UpdateKeyWithTTL(ctx, s.js, storage.RuntimeStateBucket, key, data, entry.Revision(), FlowTTL); err == nil {
			return nil
		}
	}
	return fmt.Errorf("update password reset delivery limit")
}

func (s *Service) rollbackDelivery(ctx context.Context, address string) error {
	return errors.Join(s.rollbackCounter(ctx, s.deliveryKey(address)), s.rollbackCounter(ctx, "password-reset-limit.global"))
}

func (s *Service) rollbackCounter(ctx context.Context, key string) error {
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		return nil
	}
	var counter deliveryCounter
	if json.Unmarshal(entry.Value(), &counter) != nil {
		return nil
	}
	if counter.Count <= 1 {
		return s.kv.Delete(ctx, key, jetstream.LastRevision(entry.Revision()))
	}
	counter.Count--
	data, _ := json.Marshal(counter)
	_, err = storage.UpdateKeyWithTTL(ctx, s.js, storage.RuntimeStateBucket, key, data, entry.Revision(), FlowTTL)
	return err
}

func (s *Service) send(ctx context.Context, message email.Message) error {
	select {
	case s.deliverySlots <- struct{}{}:
		defer func() { <-s.deliverySlots }()
	case <-ctx.Done():
		return ctx.Err()
	default:
		return fmt.Errorf("email delivery capacity exhausted")
	}
	return s.sender.SendContext(ctx, message)
}

func normalizeAndValidateEmail(raw string) (string, error) {
	value := accounts.NormalizeEmail(raw)
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value || strings.ContainsAny(value, "\r\n") {
		return "", ErrInvalidEmail
	}
	return value, nil
}

func keyedDigest(key []byte, value string) []byte {
	h := hmac.New(sha256.New, key)
	_, _ = h.Write([]byte(value))
	return h.Sum(nil)
}

func randomToken(size int) (string, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(data), nil
}

func verificationCode() (string, error) {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}
	n := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	return fmt.Sprintf("%06d", n%1000000), nil
}

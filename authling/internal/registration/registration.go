// Package registration owns Authling's verified-email signup workflow.
package registration

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/authling/internal/accounts"
	"hmans.de/authling/internal/email"
	"hmans.de/authling/internal/runtimejson"
	"hmans.de/authling/internal/storage"
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
	ErrInvalidFlow    = errors.New("the signup has expired; start again")
	errCompletionBusy = errors.New("signup completion capacity exhausted")
)

type flowState struct {
	Email              string    `json:"email"`
	CodeDigest         []byte    `json:"code_digest"`
	WrongAttempts      int       `json:"wrong_attempts"`
	Verified           bool      `json:"verified"`
	Completing         bool      `json:"completing"`
	CompletionAttempts int       `json:"completion_attempts"`
	ExpiresAt          time.Time `json:"expires_at"`
}

// Service coordinates expiring flow state, email delivery, and durable account creation.
type Service struct {
	kv              jetstream.KeyValue
	js              jetstream.JetStream
	key             []byte
	sender          email.Sender
	siteName        string // Public service name used only in email copy.
	accounts        *accounts.Service
	deliveryBudget  *storage.DeliveryBudget
	deliverySlots   chan struct{}
	completionSlots chan struct{}
}

// New constructs the signup workflow with a resolved public site name for email.
func New(kv jetstream.KeyValue, js jetstream.JetStream, key []byte, sender email.Sender, accountService *accounts.Service, siteName string) *Service {
	return &Service{
		kv:       kv,
		js:       js,
		key:      append([]byte(nil), key...),
		sender:   sender,
		siteName: siteName,
		accounts: accountService,
		deliveryBudget: storage.NewDeliveryBudget(kv, js, storage.DeliveryPolicy{
			GlobalKey:      "signup-limit.global",
			GlobalLimit:    maxGlobalDeliveredCodes,
			RecipientLimit: maxDeliveredCodes,
			Window:         FlowTTL,
		}),
		deliverySlots:   make(chan struct{}, maxConcurrentDeliveries),
		completionSlots: make(chan struct{}, maxConcurrentCompletions),
	}
}

// PasswordMinimumLength returns the active local password policy for signup
// form rendering.
func (s *Service) PasswordMinimumLength() int { return s.accounts.PasswordMinimumLength() }

// Start validates an address, creates an opaque flow, and delivers a six-digit code.
// Claimed and available addresses take the same delivery path. Uniqueness is
// revealed to neither the sender nor the browser response.
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
	state := flowState{Email: normalized, CodeDigest: keyedDigest(s.key, "code\x00"+token+"\x00"+code), ExpiresAt: time.Now().UTC().Add(FlowTTL)}
	if err := s.deliveryBudget.Reserve(ctx, s.deliveryKey(normalized)); err != nil {
		return "", err
	}
	delivered := false
	defer func() {
		if !delivered {
			cleanupContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.deliveryBudget.Rollback(cleanupContext, s.deliveryKey(normalized))
		}
	}()
	key := s.flowKey(token)
	data, err := s.seal(key, state)
	if err != nil {
		return "", err
	}
	if _, err := s.kv.Create(ctx, key, data, jetstream.KeyTTL(FlowTTL)); err != nil {
		return "", fmt.Errorf("store signup flow: %w", err)
	}
	body := fmt.Sprintf("Your %s verification code is %s.\n\nIt expires in 15 minutes. If you did not request this, you can ignore this message.\n", s.siteName, code)
	if err := s.send(ctx, email.Message{To: normalized, Subject: "Your " + s.siteName + " verification code", Body: body}); err != nil {
		_ = s.kv.Delete(ctx, key)
		return "", fmt.Errorf("deliver verification code: %w", err)
	}
	delivered = true
	return token, nil
}

// Verify consumes a correct OTP logically by transitioning the flow into its
// verified state. Repeated or wrong codes share a bounded attempt counter.
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

// Complete creates the durable account and consumes the verified flow only
// after account creation succeeds.
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
	account, err := s.accounts.CreateLocal(ctx, state.Email, password)
	if err != nil {
		state.Completing = false
		_, _ = s.update(ctx, key, completionRevision, state)
		return accounts.Account{}, err
	}
	_ = s.kv.Delete(ctx, key, jetstream.LastRevision(completionRevision))
	return account, nil
}

func (s *Service) flowKey(token string) string {
	return "signup." + base64.RawURLEncoding.EncodeToString(keyedDigest(s.key, "flow\x00"+token))
}

func (s *Service) seal(key string, state flowState) ([]byte, error) {
	return runtimejson.Seal(s.key, []byte("authling:runtime:v1\x00"+key), state, runtimejson.CapitalizedFields)
}

func (s *Service) read(ctx context.Context, key string) (jetstream.KeyValueEntry, flowState, error) {
	entry, err := s.kv.Get(ctx, key)
	if err != nil {
		return nil, flowState{}, err
	}
	var state flowState
	if err := runtimejson.Open(s.key, []byte("authling:runtime:v1\x00"+key), entry.Value(), &state); err != nil {
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

func (s *Service) deliveryKey(address string) string {
	return "signup-limit." + base64.RawURLEncoding.EncodeToString(keyedDigest(s.key, "delivery\x00"+address))
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

// Package emailchange owns Authling's signed-in verified email-address change workflow.
package emailchange

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
	"hmans.de/authling/internal/authentication"
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
const completionLeaseWriteTimeout = 5 * time.Second
const completionWorkTimeout = 15 * time.Second
const oldAddressNotificationTimeout = 10 * time.Second
const completionCleanupTimeout = 5 * time.Second
const completionLeaseLifetime = 45 * time.Second

var (
	ErrInvalidEmail   = errors.New("enter a valid email address")
	ErrInvalidCode    = errors.New("the code is invalid or has expired")
	ErrInvalidFlow    = errors.New("the email change has expired; start again")
	errTooManyCodes   = errors.New("too many email change codes requested")
	errCompletionBusy = errors.New("email change completion capacity exhausted")
)

type flowState struct {
	Target               accounts.EmailChangeTarget `json:"target"`
	NewEmail             string                     `json:"new_email"`
	CodeDigest           []byte                     `json:"code_digest"`
	WrongAttempts        int                        `json:"wrong_attempts"`
	Verified             bool                       `json:"verified"`
	CompletionAttempts   int                        `json:"completion_attempts"`
	CompletionLeaseUntil time.Time                  `json:"completion_lease_until"`
	ExpiresAt            time.Time                  `json:"expires_at"`
}

type sealedState struct {
	Version           int `json:"version"`
	Nonce, Ciphertext []byte
}

// Completion reports a committed identity change and whether the best-effort
// security notice to the previous address could be delivered.
type Completion struct {
	Account                      accounts.Account
	AuthenticationVersion        uint64
	OldAddressNotificationFailed bool
}

// Service coordinates reauthentication, expiring mailbox verification state,
// durable address replacement, and the old-address security notification.
type Service struct {
	kv              jetstream.KeyValue
	js              jetstream.JetStream
	key             []byte
	sender          email.Sender
	accounts        *accounts.Service
	authentication  *authentication.Service
	deliverySlots   chan struct{}
	completionSlots chan struct{}
	now             func() time.Time
}

// Option customizes the email-change service's operational dependencies.
type Option func(*Service)

// WithClock supplies the clock used for expiring workflow state and completion
// leases. It is primarily useful for deterministic lifecycle verification.
func WithClock(now func() time.Time) Option {
	return func(service *Service) {
		if now != nil {
			service.now = now
		}
	}
}

// New constructs the verified email-change workflow.
func New(kv jetstream.KeyValue, js jetstream.JetStream, key []byte, sender email.Sender, accountService *accounts.Service, authenticationService *authentication.Service, options ...Option) *Service {
	service := &Service{kv: kv, js: js, key: append([]byte(nil), key...), sender: sender, accounts: accountService, authentication: authenticationService, deliverySlots: make(chan struct{}, maxConcurrentDeliveries), completionSlots: make(chan struct{}, maxConcurrentCompletions), now: time.Now}
	for _, option := range options {
		option(service)
	}
	return service
}

// Start reauthenticates the account, records the accepted request, and sends a
// six-digit code to the requested address. Claimed and available addresses
// follow the same delivery path.
func (s *Service) Start(ctx context.Context, accountID, password, rawNewEmail string) (string, error) {
	newEmail, err := normalizeAndValidateEmail(rawNewEmail)
	if err != nil {
		return "", err
	}
	target, err := s.authentication.ReauthenticateEmailChange(ctx, accountID, password, newEmail)
	if err != nil {
		return "", err
	}
	if err := s.reserveDelivery(ctx, newEmail); err != nil {
		return "", err
	}
	reserved := true
	delivered := false
	defer func() {
		if reserved && !delivered {
			cleanupContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = s.rollbackDelivery(cleanupContext, newEmail)
		}
	}()
	target, err = s.accounts.RecordEmailChangeRequested(ctx, target)
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
	state := flowState{Target: target, NewEmail: newEmail, CodeDigest: keyedDigest(s.key, "code\x00"+token+"\x00"+code), ExpiresAt: s.now().UTC().Add(FlowTTL)}
	key := s.flowKey(token)
	data, err := s.seal(key, state)
	if err != nil {
		return "", err
	}
	if _, err := s.kv.Create(ctx, key, data, jetstream.KeyTTL(FlowTTL)); err != nil {
		return "", fmt.Errorf("store email change flow: %w", err)
	}
	body := fmt.Sprintf("Your Authling email change code is %s.\n\nIt expires in 15 minutes. If you did not request this, you can ignore this message.\n", code)
	if err := s.send(ctx, email.Message{To: newEmail, Subject: "Your Authling email change code", Body: body}); err != nil {
		_ = s.kv.Delete(ctx, key)
		return "", fmt.Errorf("deliver email change code: %w", err)
	}
	delivered = true
	return token, nil
}

// Verify proves control of the requested mailbox for the signed-in account.
func (s *Service) Verify(ctx context.Context, accountID, token, code string) error {
	key := s.flowKey(token)
	entry, state, err := s.read(ctx, key)
	if err != nil || state.Target.AccountID != accountID || !s.now().Before(state.ExpiresAt) || state.Verified || state.WrongAttempts >= maxWrongAttempts {
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

// Complete claims the verified address if the reauthenticated credential is
// still current, then attempts a security notification to the previous address.
func (s *Service) Complete(ctx context.Context, accountID, token string) (Completion, error) {
	key := s.flowKey(token)
	entry, state, err := s.read(ctx, key)
	if err != nil || state.Target.AccountID != accountID || !s.now().Before(state.ExpiresAt) || !state.Verified {
		return Completion{}, ErrInvalidFlow
	}
	now := s.now().UTC()
	if state.CompletionLeaseUntil.After(now) {
		return Completion{}, errCompletionBusy
	}
	committed, wasCommitted := s.accounts.CompletedEmailChange(state.Target, state.NewEmail)
	if state.CompletionAttempts >= maxCompletionAttempts && !wasCommitted {
		return Completion{}, ErrInvalidFlow
	}
	select {
	case s.completionSlots <- struct{}{}:
		defer func() { <-s.completionSlots }()
	case <-ctx.Done():
		return Completion{}, ctx.Err()
	default:
		return Completion{}, errCompletionBusy
	}
	if !wasCommitted {
		state.CompletionAttempts++
	}
	state.CompletionLeaseUntil = now.Add(completionLeaseLifetime)
	leaseContext, cancelLease := context.WithTimeout(ctx, completionLeaseWriteTimeout)
	completionRevision, err := s.update(leaseContext, key, entry.Revision(), state)
	cancelLease()
	if err != nil {
		return Completion{}, errCompletionBusy
	}
	workContext, cancelWork := context.WithTimeout(ctx, completionWorkTimeout)
	defer cancelWork()
	committed, wasCommitted = s.accounts.CompletedEmailChange(state.Target, state.NewEmail)
	if wasCommitted {
		return s.finishCommitted(key, completionRevision, state, committed)
	}
	account, err := s.accounts.ChangeEmail(workContext, state.Target, state.NewEmail)
	if err != nil {
		if committed, ok := s.accounts.CompletedEmailChange(state.Target, state.NewEmail); ok && workContext.Err() == nil {
			return s.finishCommitted(key, completionRevision, state, committed)
		}
		if errors.Is(err, accounts.ErrEmailClaimed) || errors.Is(err, accounts.ErrCredentialChanged) {
			_ = s.kv.Delete(ctx, key, jetstream.LastRevision(completionRevision))
			return Completion{}, ErrInvalidFlow
		}
		state.CompletionLeaseUntil = time.Time{}
		cleanupContext, cancel := context.WithTimeout(context.Background(), completionCleanupTimeout)
		defer cancel()
		_, _ = s.update(cleanupContext, key, completionRevision, state)
		return Completion{}, err
	}
	if err := workContext.Err(); err != nil {
		return Completion{}, err
	}
	return s.finishCommitted(key, completionRevision, state, account)
}

func (s *Service) finishCommitted(key string, revision uint64, state flowState, account accounts.Account) (Completion, error) {
	message := email.Message{
		To:      state.Target.OldEmail,
		Subject: "Your Authling email address changed",
		Body:    "The email address for your Authling account was changed. If you did not make this change, contact the operator of this Authling service immediately.\n",
	}
	notificationContext, cancel := context.WithTimeout(context.Background(), oldAddressNotificationTimeout)
	defer cancel()
	notificationErr := s.send(notificationContext, message)
	cleanupContext, cleanupCancel := context.WithTimeout(context.Background(), completionCleanupTimeout)
	defer cleanupCancel()
	_ = s.kv.Delete(cleanupContext, key, jetstream.LastRevision(revision))
	return Completion{Account: account, AuthenticationVersion: account.AuthenticationVersion, OldAddressNotificationFailed: notificationErr != nil}, nil
}

func (s *Service) flowKey(token string) string {
	return "email-change." + base64.RawURLEncoding.EncodeToString(keyedDigest(s.key, "flow\x00"+token))
}

func (s *Service) seal(key string, state flowState) ([]byte, error) {
	plain, err := json.Marshal(state)
	if err != nil {
		return nil, err
	}
	defer clear(plain)
	sealed, err := datacrypto.Seal(s.key, plain, []byte("authling:email-change-runtime:v1\x00"+key))
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
	plain, err := datacrypto.Open(s.key, sealed.Ciphertext, sealed.Nonce, []byte("authling:email-change-runtime:v1\x00"+key))
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
	remaining := state.ExpiresAt.Sub(s.now())
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
	return "email-change-limit." + base64.RawURLEncoding.EncodeToString(keyedDigest(s.key, "delivery\x00"+address))
}

func (s *Service) reserveDelivery(ctx context.Context, address string) error {
	if err := s.reserveCounter(ctx, "email-change-limit.global", maxGlobalDeliveredCodes); err != nil {
		return err
	}
	if err := s.reserveCounter(ctx, s.deliveryKey(address), maxDeliveredCodes); err != nil {
		_ = s.rollbackCounter(ctx, "email-change-limit.global")
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
			return fmt.Errorf("read email change delivery limit: %w", err)
		}
		var counter deliveryCounter
		if json.Unmarshal(entry.Value(), &counter) != nil {
			return fmt.Errorf("decode email change delivery limit")
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
	return fmt.Errorf("update email change delivery limit")
}

func (s *Service) rollbackDelivery(ctx context.Context, address string) error {
	return errors.Join(s.rollbackCounter(ctx, s.deliveryKey(address)), s.rollbackCounter(ctx, "email-change-limit.global"))
}

func (s *Service) rollbackCounter(ctx context.Context, key string) error {
	for range 16 {
		entry, err := s.kv.Get(ctx, key)
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			return nil
		}
		if err != nil {
			return err
		}
		var counter deliveryCounter
		if json.Unmarshal(entry.Value(), &counter) != nil || counter.Count < 1 {
			return fmt.Errorf("decode email change delivery limit for rollback")
		}
		if counter.Count == 1 {
			if err := s.kv.Delete(ctx, key, jetstream.LastRevision(entry.Revision())); err == nil {
				return nil
			}
			continue
		}
		counter.Count--
		data, _ := json.Marshal(counter)
		if _, err := storage.UpdateKeyWithTTL(ctx, s.js, storage.RuntimeStateBucket, key, data, entry.Revision(), FlowTTL); err == nil {
			return nil
		}
	}
	return fmt.Errorf("rollback email change delivery limit after repeated conflicts")
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

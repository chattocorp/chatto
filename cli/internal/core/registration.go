package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// ============================================================================
// Registration Code Constants and Errors
// ============================================================================

const (
	// RegistrationCodeTTL is the duration a registration verification code is valid.
	RegistrationCodeTTL = 15 * time.Minute

	// RegistrationCompletionTokenTTL is the duration a code-exchanged completion token is valid.
	RegistrationCompletionTokenTTL = 15 * time.Minute
)

var (
	// ErrRegistrationCodeNotFound is returned when the registration code record doesn't exist.
	ErrRegistrationCodeNotFound = errors.New("registration code not found")

	// ErrRegistrationCodeExpired is returned when the registration code has expired.
	ErrRegistrationCodeExpired = errors.New("registration code has expired")

	// ErrRegistrationCodeInvalid is returned when a submitted registration code is wrong.
	ErrRegistrationCodeInvalid = errors.New("invalid registration code")

	// ErrRegistrationCodeExhausted is returned when too many invalid attempts were made.
	ErrRegistrationCodeExhausted = errors.New("registration code exhausted")

	// ErrRegistrationCodeLimitExceeded is returned when too many codes are active for an email.
	ErrRegistrationCodeLimitExceeded = errors.New("too many active registration codes")

	// ErrRegistrationTokenNotFound is returned when the completion token doesn't exist.
	ErrRegistrationTokenNotFound = errors.New("registration completion token not found")

	// ErrRegistrationTokenExpired is returned when the completion token has expired.
	ErrRegistrationTokenExpired = errors.New("registration completion token has expired")
)

var verificationCodePattern = regexp.MustCompile(`^\d{6}$`)

// ============================================================================
// Registration Types
// ============================================================================

// RegistrationCode represents a pending email-first registration OTP.
type RegistrationCode struct {
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// RegistrationToken represents a token used to complete email-first registration
// after the email code has already been verified. Unlike password reset tokens,
// this has no UserID — the user doesn't exist yet.
type RegistrationToken struct {
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// ============================================================================
// KV Key Functions
// ============================================================================

const (
	registrationOTPScope                 = "registration"
	registrationCompletionTokenKeyPrefix = "registration_completion."
)

// registrationCodeKey returns the HMAC-derived KV key for one registration code.
func (c *ChattoCore) registrationCodeKey(email, code string) string {
	return c.emailOTPCodeKey(registrationOTPScope, normalizeRegistrationEmail(email), code)
}

func (c *ChattoCore) registrationCodeChallengeKey(email string) string {
	return c.emailOTPChallengeKey(registrationOTPScope, normalizeRegistrationEmail(email))
}

// registrationTokenKey returns the HMAC-derived KV key for a registration completion token.
func (c *ChattoCore) registrationTokenKey(token string) string {
	return c.runtimeTokenKey(registrationCompletionTokenKeyPrefix, token)
}

func normalizeRegistrationEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

// ============================================================================
// Registration Code Operations
// ============================================================================

// CreateRegistrationCode creates a short-lived registration code for an email.
// It returns the raw six-digit code so the caller can send it by email. The code
// itself is never stored; only an HMAC-derived lookup key is kept in RUNTIME_STATE.
func (c *ChattoCore) CreateRegistrationCode(ctx context.Context, email string) (string, error) {
	email = normalizeRegistrationEmail(email)
	if email == "" {
		return "", fmt.Errorf("email is required")
	}

	code, err := c.createEmailOTP(ctx, registrationOTPScope, email, RegistrationCodeTTL, func(createdAt time.Time) ([]byte, error) {
		return json.Marshal(RegistrationCode{
			Email:     email,
			CreatedAt: createdAt,
		})
	}, func(createdAt time.Time) error {
		return c.recordRegistrationCodeIssued(ctx, email, createdAt)
	})
	if errors.Is(err, errEmailOTPExhausted) {
		return "", ErrRegistrationCodeExhausted
	}
	if errors.Is(err, errEmailOTPTooManyCodes) {
		return "", ErrRegistrationCodeLimitExceeded
	}
	return code, err
}

// VerifyRegistrationCode validates an email registration code and returns a
// short-lived completion token for the account-creation form.
func (c *ChattoCore) VerifyRegistrationCode(ctx context.Context, email, code string) (string, error) {
	email = normalizeRegistrationEmail(email)
	code = strings.TrimSpace(code)
	if email == "" || !verificationCodePattern.MatchString(code) {
		return "", ErrRegistrationCodeInvalid
	}

	entry, err := c.getEmailOTPCode(ctx, registrationOTPScope, email, code, RegistrationCodeTTL)
	if err != nil {
		switch {
		case errors.Is(err, errEmailOTPNotFound):
			return "", ErrRegistrationCodeNotFound
		case errors.Is(err, errEmailOTPExpired):
			return "", ErrRegistrationCodeExpired
		case errors.Is(err, errEmailOTPInvalid):
			return "", ErrRegistrationCodeInvalid
		case errors.Is(err, errEmailOTPExhausted):
			return "", ErrRegistrationCodeExhausted
		default:
			return "", err
		}
	}

	var codeData RegistrationCode
	if err := json.Unmarshal(entry.value, &codeData); err != nil {
		return "", fmt.Errorf("failed to unmarshal registration code: %w", err)
	}

	if time.Since(codeData.CreatedAt) > RegistrationCodeTTL {
		_ = c.storage.runtimeStateKV.Delete(ctx, entry.key, jetstream.LastRevision(entry.revision))
		return "", ErrRegistrationCodeExpired
	}
	if codeData.Email != email {
		return "", ErrRegistrationCodeInvalid
	}
	if err := c.consumeEmailOTPCode(ctx, registrationOTPScope, email, entry); err != nil {
		if errors.Is(err, errEmailOTPNotFound) {
			return "", ErrRegistrationCodeNotFound
		}
		return "", err
	}

	token, err := c.CreateRegistrationCompletionToken(ctx, email)
	if err != nil {
		return "", err
	}
	return token, nil
}

func isRuntimeStateRevisionConflict(err error) bool {
	return errors.Is(err, jetstream.ErrKeyExists)
}

func isRuntimeStateKeyAbsent(err error) bool {
	return errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted)
}

// ============================================================================
// Registration Completion Token Operations
// ============================================================================

// CreateRegistrationCompletionToken creates a high-entropy token after the email
// code has been verified. This token is used by /auth/register/complete and is
// not sent by email.
func (c *ChattoCore) CreateRegistrationCompletionToken(ctx context.Context, email string) (string, error) {
	email = normalizeRegistrationEmail(email)
	if email == "" {
		return "", fmt.Errorf("email is required")
	}

	token := NewRegistrationToken()
	createdAt := time.Now()
	tokenData := RegistrationToken{
		Email:     email,
		CreatedAt: createdAt,
	}

	data, err := json.Marshal(tokenData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal registration completion token: %w", err)
	}

	_, err = c.storage.runtimeStateKV.Create(ctx, c.registrationTokenKey(token), data, jetstream.KeyTTL(RegistrationCompletionTokenTTL))
	if err != nil {
		return "", fmt.Errorf("failed to store registration completion token: %w", err)
	}

	return token, nil
}

// CreateRegistrationToken is kept as a test/back-compat helper for callers that
// need to bypass email delivery and code entry.
func (c *ChattoCore) CreateRegistrationToken(ctx context.Context, email string) (string, error) {
	return c.CreateRegistrationCompletionToken(ctx, email)
}

// GetRegistrationToken retrieves and validates a registration completion token.
// Returns the token data including the email address.
func (c *ChattoCore) GetRegistrationToken(ctx context.Context, token string) (*RegistrationToken, error) {
	key := c.registrationTokenKey(token)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, ErrRegistrationTokenNotFound
		}
		return nil, fmt.Errorf("failed to get registration completion token: %w", err)
	}

	var tokenData RegistrationToken
	if err := json.Unmarshal(entry.Value(), &tokenData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal registration completion token: %w", err)
	}

	if time.Since(tokenData.CreatedAt) > RegistrationCompletionTokenTTL {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return nil, ErrRegistrationTokenExpired
	}

	return &tokenData, nil
}

// DeleteRegistrationToken removes a completion token after successful account creation.
func (c *ChattoCore) DeleteRegistrationToken(ctx context.Context, token string) error {
	key := c.registrationTokenKey(token)
	err := c.storage.runtimeStateKV.Delete(ctx, key)
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to delete registration completion token: %w", err)
	}
	return nil
}

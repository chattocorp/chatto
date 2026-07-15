package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// Email Verification Constants and Errors
// ============================================================================

const (
	// EmailVerificationCodeTTL is the duration a verification code is valid.
	EmailVerificationCodeTTL = 15 * time.Minute
)

var (
	// ErrTokenNotFound is returned when the verification code doesn't exist or has expired.
	ErrTokenNotFound = errors.New("verification code not found or expired")

	// ErrTokenExpired is returned when the verification code has expired.
	ErrTokenExpired = errors.New("verification code has expired")

	// ErrEmailVerificationCodeInvalid is returned when a submitted email verification code is wrong.
	ErrEmailVerificationCodeInvalid = errors.New("invalid email verification code")

	// ErrEmailVerificationCodeExhausted is returned when too many invalid attempts were made.
	ErrEmailVerificationCodeExhausted = errors.New("email verification code exhausted")

	// ErrEmailVerificationCodeLimitExceeded is returned when too many codes are active for an email verification challenge.
	ErrEmailVerificationCodeLimitExceeded = errors.New("too many active email verification codes")

	// ErrEmailAlreadyVerified is returned when trying to verify an email that's already verified by another user.
	ErrEmailAlreadyVerified = errors.New("email address is already verified by another account")

	errVerifiedEmailNoop = errors.New("verified email mutation is a no-op")
)

// ============================================================================
// Email Verification Types
// ============================================================================

// EmailVerificationCode represents a pending code used to verify an email address.
// Stored as JSON (short-lived, auto-expires via KV TTL — not worth proto).
type EmailVerificationCode struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// VerifiedEmail is the read-time plaintext shape returned by
// GetVerifiedEmails. UserProjection retains only its encrypted value and
// materialises this shape while hydrating a request.
type VerifiedEmail struct {
	Email      string    `json:"email"`
	VerifiedAt time.Time `json:"verified_at"`
}

// ============================================================================
// KV Key Functions
// ============================================================================

const (
	emailVerificationOTPScope = "email_verification"
)

func (c *ChattoCore) emailVerificationCodeTTL() time.Duration {
	return c.config.EmailOTP.TTLOrDefault()
}

// emailVerificationCodeKey returns the HMAC-derived KV key for one email verification code.
func (c *ChattoCore) emailVerificationCodeKey(userID, email, code string) string {
	return c.emailOTPCodeKey(emailVerificationOTPScope, emailVerificationOTPSubject(userID, email), code)
}

func (c *ChattoCore) emailVerificationCodeChallengeKey(userID, email string) string {
	return c.emailOTPChallengeKey(emailVerificationOTPScope, emailVerificationOTPSubject(userID, email))
}

func emailVerificationOTPSubject(userID, email string) string {
	return strings.TrimSpace(userID) + "\x00" + strings.ToLower(strings.TrimSpace(email))
}

// emailHash returns the stable lowercase-SHA256 hex digest used in both
// the per-email key and the user_by_email index. Centralised so the
// index and the per-email entries can never drift apart.
func emailHash(email string) string {
	return userPIILookupHash(email)
}

// verifiedEmailKey returns the KV key for a single verified email.
// Format: verified_emails.{userID}.{sha256(lowercase(email))}
//
// One entry per (user, email) pair lets us add a new email with a single
// Put (no read-modify-write), list a user's emails with a prefix scan
// (`verified_emails.{userID}.*`), and decode only the entries we need.
func verifiedEmailKey(userID, email string) string {
	return fmt.Sprintf("verified_emails.%s.%s", userID, emailHash(email))
}

// verifiedEmailPrefix returns the prefix-scan pattern for one user's
// verified emails.
func verifiedEmailPrefix(userID string) string {
	return fmt.Sprintf("verified_emails.%s.*", userID)
}

// userByEmailKey returns the KV key for the email-to-user index.
// Uses SHA256 hash of the lowercase email to ensure valid NATS subject characters
// and case-insensitive uniqueness. Created when an email is verified.
func userByEmailKey(email string) string {
	return fmt.Sprintf("user_by_email.%s", emailHash(email))
}

// ============================================================================
// Email Verification Code Operations
// ============================================================================

// CreateEmailVerificationCode creates a short-lived verification code for an email.
// The returned raw code is intended to be sent by email and is never stored.
func (c *ChattoCore) CreateEmailVerificationCode(ctx context.Context, userID, email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if userID == "" {
		return "", fmt.Errorf("userID is required")
	}
	if email == "" {
		return "", fmt.Errorf("email is required")
	}
	subject := emailVerificationOTPSubject(userID, email)
	ttl := c.emailVerificationCodeTTL()
	code, err := c.createEmailOTP(ctx, emailVerificationOTPScope, subject, ttl, func(createdAt time.Time) ([]byte, error) {
		return json.Marshal(EmailVerificationCode{
			UserID:    userID,
			Email:     email,
			CreatedAt: createdAt,
		})
	}, func(createdAt time.Time) error {
		return c.recordEmailVerificationCodeIssued(ctx, userID, email, createdAt)
	})
	if errors.Is(err, errEmailOTPExhausted) {
		return "", ErrEmailVerificationCodeExhausted
	}
	if errors.Is(err, errEmailOTPTooManyCodes) {
		return "", ErrEmailVerificationCodeLimitExceeded
	}
	return code, err
}

// CancelEmailVerificationCode removes an email-verification OTP that was
// created but not delivered, so failed email sends do not consume throttle slots.
func (c *ChattoCore) CancelEmailVerificationCode(ctx context.Context, userID, email, code string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if userID == "" || email == "" {
		return nil
	}
	return c.cancelEmailOTP(ctx, emailVerificationOTPScope, emailVerificationOTPSubject(userID, email), code, c.emailVerificationCodeTTL())
}

// VerifyEmailCode verifies an email using a submitted code.
func (c *ChattoCore) VerifyEmailCode(ctx context.Context, userID, email, code string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	code = strings.TrimSpace(code)
	if userID == "" || email == "" || !verificationCodePattern.MatchString(code) {
		return "", ErrEmailVerificationCodeInvalid
	}

	subject := emailVerificationOTPSubject(userID, email)
	ttl := c.emailVerificationCodeTTL()
	entry, err := c.getEmailOTPCode(ctx, emailVerificationOTPScope, subject, code, ttl)
	if err != nil {
		switch {
		case errors.Is(err, errEmailOTPNotFound):
			return "", ErrTokenNotFound
		case errors.Is(err, errEmailOTPExpired):
			return "", ErrTokenExpired
		case errors.Is(err, errEmailOTPInvalid):
			return "", ErrEmailVerificationCodeInvalid
		case errors.Is(err, errEmailOTPExhausted):
			return "", ErrEmailVerificationCodeExhausted
		default:
			return "", err
		}
	}

	var codeData EmailVerificationCode
	if err := json.Unmarshal(entry.value, &codeData); err != nil {
		return "", fmt.Errorf("failed to unmarshal email verification code: %w", err)
	}

	if time.Since(codeData.CreatedAt) > ttl {
		_ = c.storage.runtimeStateKV.Delete(ctx, entry.key, jetstream.LastRevision(entry.revision))
		return "", ErrTokenExpired
	}
	if codeData.UserID != userID || codeData.Email != email {
		return "", ErrEmailVerificationCodeInvalid
	}
	if err := c.consumeEmailOTPCode(ctx, emailVerificationOTPScope, subject, entry); err != nil {
		if errors.Is(err, errEmailOTPNotFound) {
			return "", ErrTokenNotFound
		}
		return "", err
	}
	if err := c.addVerifiedEmailAs(ctx, userID, userID, email); err != nil {
		return "", err
	}
	return userID, nil
}

// requireVerifiedAccountCapacity enforces the user cap at points where an
// account gains its first verified sign-in factor.
func (c *ChattoCore) requireVerifiedAccountCapacity(ctx context.Context, userID string) error {
	if max := c.config.Limits.MaxUsersOrDefault(); max >= 0 {
		if userID != "" && c.Users.HasVerifiedFactor(userID) {
			return nil
		}
		count, err := c.CountVerifiedAccounts(ctx)
		if err != nil {
			return fmt.Errorf("failed to count verified accounts: %w", err)
		}
		if count >= max {
			return ErrLimitExceeded
		}
	}
	return nil
}

// addVerifiedEmail appends a durable verified-email event for the user.
// Idempotent: rewriting the same (user, email) pair just overwrites the
// existing entry with identical content.
func (c *ChattoCore) addVerifiedEmailAs(ctx context.Context, actorID, userID, email string) error {
	email = strings.ToLower(strings.TrimSpace(email))
	if email == "" {
		return ErrInvalidArgument
	}
	if _, err := c.GetUser(ctx, userID); err != nil {
		return fmt.Errorf("user not found: %w", err)
	}

	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_UserVerifiedEmailAdded{
		UserVerifiedEmailAdded: &corev1.UserVerifiedEmailAddedEvent{
			UserId: userID,
		},
	}})
	encryptedEmail, err := c.encryptUserPIIString(ctx, event.GetId(), userID, events.EventUserVerifiedEmailAdded, "email", email)
	if err != nil {
		return fmt.Errorf("encrypt verified email: %w", err)
	}
	event.GetUserVerifiedEmailAdded().EncryptedEmail = encryptedEmail
	if _, err := c.appendUserEvent(ctx, userID, event, events.UserSubjectFilter(), func() error {
		if _, err := c.GetUser(ctx, userID); err != nil {
			return fmt.Errorf("user not found: %w", err)
		}
		if ownerID, ok := c.Users.EmailOwnerID(email); ok {
			if ownerID == userID {
				return errVerifiedEmailNoop
			}
			return ErrEmailAlreadyVerified
		}
		if err := c.requireVerifiedAccountCapacity(ctx, userID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		if errors.Is(err, errVerifiedEmailNoop) {
			// Already verified for this user. Keep going so owner-email
			// auto-promotion below still catches config changes.
		} else if errors.Is(err, ErrEmailAlreadyVerified) {
			return ErrEmailAlreadyVerified
		} else {
			return fmt.Errorf("failed to store verified email: %w", err)
		}
	}

	// Auto-promote on config-owner email match. This is what closes the
	// chicken-and-egg gap on fresh deployments: as soon as the operator's
	// account verifies their email, they pick up the `owner` role without
	// waiting for the next boot-time owner sync.
	if c.config.Owners.IsServerOwnerEmail(email) {
		if err := c.AssignServerRoleToExistingUser(ctx, SystemActorID, userID, RoleOwner); err != nil {
			c.logger.Warn("Failed to auto-assign owner role on email verification",
				"user_id", userID, "error", err)
		} else {
			c.logger.Info("Auto-promoted user to owner via owners.emails match",
				"user_id", userID)
		}
	}

	return nil
}

// GetVerifiedEmails returns all verified emails for a user from the user projection.
func (c *ChattoCore) GetVerifiedEmails(ctx context.Context, userID string) ([]VerifiedEmail, error) {
	return c.Users.VerifiedEmailsContext(ctx, userID)
}

// HasVerifiedEmail checks if a user has at least one verified email.
func (c *ChattoCore) HasVerifiedEmail(ctx context.Context, userID string) (bool, error) {
	return c.Users.HasVerifiedEmail(userID), nil
}

// IsEmailClaimed checks if an email address is already verified by any user.
// Used to prevent registration with an email that's already in use.
func (c *ChattoCore) IsEmailClaimed(ctx context.Context, email string) (bool, error) {
	return c.Users.EmailClaimed(email), nil
}

// GetUserByVerifiedEmail looks up a user by their verified email address.
// Returns the user if found, or an error if not found.
func (c *ChattoCore) GetUserByVerifiedEmail(ctx context.Context, email string) (*corev1.User, error) {
	user, ok, err := c.Users.GetByEmailContext(ctx, email)
	if err != nil {
		return nil, err
	}
	if ok {
		return user, nil
	}
	return nil, fmt.Errorf("%w: verified email", ErrNotFound)
}

// CountVerifiedAccounts returns the number of distinct users with at least one
// verified sign-in factor: a verified email or linked external identity.
func (c *ChattoCore) CountVerifiedAccounts(ctx context.Context) (int, error) {
	return len(c.Users.VerifiedAccountIDs()), nil
}

// CountVerifiedUsers returns the number of distinct users with at least
// one verified email.
func (c *ChattoCore) CountVerifiedUsers(ctx context.Context) (int, error) {
	return len(c.Users.VerifiedUserIDs()), nil
}

// ListUsersWithVerifiedEmail returns all user IDs that have at least one verified email.
func (c *ChattoCore) ListUsersWithVerifiedEmail(ctx context.Context) ([]string, error) {
	return c.Users.VerifiedUserIDs(), nil
}

// applyConfigOwners materializes owners.emails as durable owner-role
// assignments for users who already have matching verified emails. It is
// intentionally additive: config cannot distinguish owner roles it granted
// from owner roles assigned manually, so removed config emails are not revoked
// here.
func (c *ChattoCore) applyConfigOwners(ctx context.Context) error {
	if len(c.config.Owners.Emails) == 0 {
		return nil
	}

	promoted := 0
	for _, userID := range c.Users.VerifiedUserIDs() {
		emails, err := c.Users.VerifiedEmailsContext(ctx, userID)
		if err != nil {
			return err
		}
		for _, ve := range emails {
			if !c.config.Owners.IsServerOwnerEmail(ve.Email) {
				continue
			}
			if c.RBAC.HasRole(userID, RoleOwner) {
				break
			}
			if err := c.AssignServerRoleToExistingUser(ctx, SystemActorID, userID, RoleOwner); err != nil {
				return fmt.Errorf("assign owner role to %s: %w", userID, err)
			}
			promoted++
			c.logger.Info("Applied owners.emails owner role", "user_id", userID)
			break
		}
	}
	if promoted > 0 {
		c.logger.Info("Applied config owners", "owners_promoted", promoted)
	}
	return nil
}

// AddVerifiedEmailDirect adds an email as verified without requiring token verification.
// Used for OAuth flows where the email is already verified by the provider.
func (c *ChattoCore) AddVerifiedEmailDirect(ctx context.Context, userID, email string) error {
	return c.AddVerifiedEmailDirectAs(ctx, userID, userID, email)
}

// AddVerifiedEmailDirectAs adds an email as verified with explicit actor
// attribution. Operator/admin flows should pass SystemActorID.
func (c *ChattoCore) AddVerifiedEmailDirectAs(ctx context.Context, actorID, userID, email string) error {
	return c.addVerifiedEmailAs(ctx, actorID, userID, email)
}

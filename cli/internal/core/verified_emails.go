package core

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

	// EmailVerificationCodeMaxAttempts is the number of invalid attempts before a code is exhausted.
	EmailVerificationCodeMaxAttempts = 5
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
	CodeHash  string    `json:"code_hash"`
	Attempts  int       `json:"attempts"`
	CreatedAt time.Time `json:"created_at"`
}

// VerifiedEmail is the in-memory shape returned by GetVerifiedEmails.
// On disk each entry lives in its own KV key as a proto-encoded
// corev1.VerifiedEmail; this Go struct exists for back-compat with the
// callers that already use it.
type VerifiedEmail struct {
	Email      string    `json:"email"`
	VerifiedAt time.Time `json:"verified_at"`
}

// ============================================================================
// KV Key Functions
// ============================================================================

const (
	emailVerificationCodeKeyPrefix  = "email_verification_code."
	emailVerificationCodeHashPrefix = "email_verification_code_verifier."
)

// emailVerificationCodeKey returns the HMAC-derived KV key for an email verification flow.
func (c *ChattoCore) emailVerificationCodeKey(userID, email string) string {
	return c.runtimeTokenKey(emailVerificationCodeKeyPrefix, userID+"\x00"+strings.ToLower(strings.TrimSpace(email)))
}

// emailVerificationCodeHash returns the HMAC-derived verifier stored for a submitted code.
func (c *ChattoCore) emailVerificationCodeHash(userID, email, code string) string {
	return c.runtimeTokenKey(emailVerificationCodeHashPrefix, userID+"\x00"+strings.ToLower(strings.TrimSpace(email))+"\x00"+code)
}

// emailHash returns the stable lowercase-SHA256 hex digest used in both
// the per-email key and the user_by_email index. Centralised so the
// index and the per-email entries can never drift apart.
func emailHash(email string) string {
	sum := sha256.Sum256([]byte(strings.ToLower(email)))
	return hex.EncodeToString(sum[:])
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

// CreateEmailVerificationCode creates or replaces the current verification code for an email.
// The returned raw code is intended to be sent by email and is never stored.
func (c *ChattoCore) CreateEmailVerificationCode(ctx context.Context, userID, email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if userID == "" {
		return "", fmt.Errorf("userID is required")
	}
	if email == "" {
		return "", fmt.Errorf("email is required")
	}
	code, err := NewVerificationCode()
	if err != nil {
		return "", err
	}
	createdAt := time.Now()

	codeData := EmailVerificationCode{
		UserID:    userID,
		Email:     email,
		CodeHash:  c.emailVerificationCodeHash(userID, email, code),
		CreatedAt: createdAt,
	}

	data, err := json.Marshal(codeData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal email verification code: %w", err)
	}

	key := c.emailVerificationCodeKey(userID, email)
	_ = c.storage.runtimeStateKV.Delete(ctx, key)
	_, err = c.storage.runtimeStateKV.Create(ctx, key, data, jetstream.KeyTTL(EmailVerificationCodeTTL))
	if err != nil {
		return "", fmt.Errorf("failed to store email verification code: %w", err)
	}

	if err := c.recordEmailVerificationCodeIssued(ctx, userID, email, createdAt); err != nil {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return "", err
	}

	return code, nil
}

// VerifyEmailCode verifies an email using a submitted code.
func (c *ChattoCore) VerifyEmailCode(ctx context.Context, userID, email, code string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	code = strings.TrimSpace(code)
	if userID == "" || email == "" || !verificationCodePattern.MatchString(code) {
		return "", ErrEmailVerificationCodeInvalid
	}

	key := c.emailVerificationCodeKey(userID, email)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return "", ErrTokenNotFound
		}
		return "", fmt.Errorf("failed to get email verification code: %w", err)
	}

	var codeData EmailVerificationCode
	if err := json.Unmarshal(entry.Value(), &codeData); err != nil {
		return "", fmt.Errorf("failed to unmarshal email verification code: %w", err)
	}

	if time.Since(codeData.CreatedAt) > EmailVerificationCodeTTL {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return "", ErrTokenExpired
	}
	if codeData.UserID != userID || codeData.Email != email {
		return "", ErrEmailVerificationCodeInvalid
	}
	if codeData.Attempts >= EmailVerificationCodeMaxAttempts {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return "", ErrEmailVerificationCodeExhausted
	}
	if codeData.CodeHash != c.emailVerificationCodeHash(userID, email, code) {
		codeData.Attempts++
		if codeData.Attempts >= EmailVerificationCodeMaxAttempts {
			_ = c.storage.runtimeStateKV.Delete(ctx, key)
			return "", ErrEmailVerificationCodeExhausted
		}
		if err := c.replaceEmailVerificationCodeRecord(ctx, key, codeData); err != nil {
			return "", err
		}
		return "", ErrEmailVerificationCodeInvalid
	}

	if err := c.addVerifiedEmail(ctx, userID, email); err != nil {
		return "", err
	}
	_ = c.storage.runtimeStateKV.Delete(ctx, key)
	return userID, nil
}

func (c *ChattoCore) replaceEmailVerificationCodeRecord(ctx context.Context, key string, codeData EmailVerificationCode) error {
	data, err := json.Marshal(codeData)
	if err != nil {
		return fmt.Errorf("failed to marshal email verification code: %w", err)
	}
	_ = c.storage.runtimeStateKV.Delete(ctx, key)
	remaining := EmailVerificationCodeTTL - time.Since(codeData.CreatedAt)
	if remaining <= 0 {
		return ErrTokenExpired
	}
	if _, err := c.storage.runtimeStateKV.Create(ctx, key, data, jetstream.KeyTTL(remaining)); err != nil {
		return fmt.Errorf("failed to store email verification code attempt: %w", err)
	}
	return nil
}

// addVerifiedEmail appends a durable verified-email event for the user.
// Idempotent: rewriting the same (user, email) pair just overwrites the
// existing entry with identical content.
func (c *ChattoCore) addVerifiedEmail(ctx context.Context, userID, email string) error {
	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserVerifiedEmailAdded{
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
		if user, ok := c.Users.GetByEmail(email); ok {
			if user.GetId() == userID {
				return errVerifiedEmailNoop
			}
			return ErrEmailAlreadyVerified
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
	// requiring a server restart or `chatto reset rbac` run.
	if c.config.Owners.IsServerOwnerEmail(email) {
		if err := c.AssignServerRole(ctx, SystemActorID, userID, RoleOwner); err != nil {
			c.logger.Warn("Failed to auto-assign owner role on email verification",
				"user_id", userID, "email", email, "error", err)
		} else {
			c.logger.Info("Auto-promoted user to owner via owners.emails match",
				"user_id", userID, "email", email)
		}
	}

	return nil
}

// GetVerifiedEmails returns all verified emails for a user from the user projection.
func (c *ChattoCore) GetVerifiedEmails(ctx context.Context, userID string) ([]VerifiedEmail, error) {
	return c.Users.VerifiedEmails(userID), nil
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
	if user, ok := c.Users.GetByEmail(email); ok {
		return user, nil
	}
	return nil, fmt.Errorf("no user found with verified email")
}

// CountVerifiedUsers returns the number of distinct users with at least
// one verified email.
//
// Implemented by listing the email-to-user index (`user_by_email.*`),
// which has one entry per verified email — not per user — and
// deduplicating the userIDs in the values. This is the only path that
// gives a tombstone-free count; see the comment on ListKeysFiltered in
// the older version of this file for the JetStream subject-count
// pitfall.
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
		emails := c.Users.VerifiedEmails(userID)
		for _, ve := range emails {
			if !c.config.Owners.IsServerOwnerEmail(ve.Email) {
				continue
			}
			if c.RBAC.HasRole(userID, RoleOwner) {
				break
			}
			if err := c.AssignServerRole(ctx, SystemActorID, userID, RoleOwner); err != nil {
				return fmt.Errorf("assign owner role to %s: %w", userID, err)
			}
			promoted++
			c.logger.Info("Applied owners.emails owner role", "user_id", userID, "email", ve.Email)
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
	return c.addVerifiedEmail(ctx, userID, email)
}

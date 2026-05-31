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
	// EmailVerificationTokenTTL is the duration a verification token is valid.
	EmailVerificationTokenTTL = 24 * time.Hour
)

var (
	// ErrTokenNotFound is returned when the verification token doesn't exist or has expired.
	ErrTokenNotFound = errors.New("verification token not found or expired")

	// ErrTokenExpired is returned when the verification token has expired.
	ErrTokenExpired = errors.New("verification token has expired")

	// ErrEmailAlreadyVerified is returned when trying to verify an email that's already verified by another user.
	ErrEmailAlreadyVerified = errors.New("email address is already verified by another account")

	errVerifiedEmailNoop = errors.New("verified email mutation is a no-op")
)

// ============================================================================
// Email Verification Types
// ============================================================================

// EmailVerificationToken represents a token used to verify an email address.
// Stored as JSON (short-lived, auto-expires via KV TTL — not worth proto).
type EmailVerificationToken struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
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

// emailVerificationTokenKey returns the KV key for an email verification token.
// Format: email_verification.{token}
func emailVerificationTokenKey(token string) string {
	return fmt.Sprintf("email_verification.%s", token)
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
// Email Verification Token Operations
// ============================================================================

// CreateEmailVerificationToken creates a new verification token for an email.
// The token contains all info needed for verification (userID, email).
func (c *ChattoCore) CreateEmailVerificationToken(ctx context.Context, userID, email string) (string, error) {
	token := NewEmailVerificationToken()
	createdAt := time.Now()

	tokenData := EmailVerificationToken{
		UserID:    userID,
		Email:     email,
		CreatedAt: createdAt,
	}

	data, err := json.Marshal(tokenData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal token: %w", err)
	}

	_, err = c.storage.serverKV.Create(ctx, emailVerificationTokenKey(token), data, jetstream.KeyTTL(EmailVerificationTokenTTL))
	if err != nil {
		return "", fmt.Errorf("failed to store verification token: %w", err)
	}

	if err := c.recordEmailVerificationLinkIssued(ctx, userID, email, createdAt); err != nil {
		_ = c.deleteEmailVerificationToken(ctx, token)
		return "", err
	}

	return token, nil
}

// getEmailVerificationToken retrieves and validates a verification token.
// Returns the token data including userID and email.
func (c *ChattoCore) getEmailVerificationToken(ctx context.Context, token string) (*EmailVerificationToken, error) {
	entry, err := c.storage.serverKV.Get(ctx, emailVerificationTokenKey(token))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, ErrTokenNotFound
		}
		return nil, fmt.Errorf("failed to get verification token: %w", err)
	}

	var tokenData EmailVerificationToken
	if err := json.Unmarshal(entry.Value(), &tokenData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %w", err)
	}

	// Check if token has expired
	if time.Since(tokenData.CreatedAt) > EmailVerificationTokenTTL {
		// Clean up expired token
		c.storage.serverKV.Delete(ctx, emailVerificationTokenKey(token))
		return nil, ErrTokenExpired
	}

	return &tokenData, nil
}

// deleteEmailVerificationToken removes a verification token.
func (c *ChattoCore) deleteEmailVerificationToken(ctx context.Context, token string) error {
	err := c.storage.serverKV.Delete(ctx, emailVerificationTokenKey(token))
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to delete verification token: %w", err)
	}
	return nil
}

// ============================================================================
// Email Verification
// ============================================================================

// VerifyEmail verifies an email using a token.
// This will:
// 1. Validate the token (O(1) direct lookup)
// 2. Check that the email isn't already claimed by another user
// 3. Claim the email
// 4. Add it to the user's verified emails
// 5. Delete the verification token
func (c *ChattoCore) VerifyEmail(ctx context.Context, token string) (userID string, err error) {
	// Direct O(1) lookup - token contains userID and email in value
	tokenData, err := c.getEmailVerificationToken(ctx, token)
	if err != nil {
		return "", err
	}

	// Add to user's verified emails (idempotent - won't duplicate)
	if err := c.addVerifiedEmail(ctx, tokenData.UserID, tokenData.Email); err != nil {
		return "", err
	}

	// Clean up: delete the verification token
	c.deleteEmailVerificationToken(ctx, token)

	return tokenData.UserID, nil
}

// addVerifiedEmail appends a durable verified-email event for the user.
// Idempotent: rewriting the same (user, email) pair just overwrites the
// existing entry with identical content.
func (c *ChattoCore) addVerifiedEmail(ctx context.Context, userID, email string) error {
	event := newEvent(userID, &corev1.Event{Event: &corev1.Event_UserVerifiedEmailAdded{
		UserVerifiedEmailAdded: &corev1.UserVerifiedEmailAddedEvent{
			UserId: userID,
			Email:  email,
		},
	}})
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

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
// Password Reset Constants and Errors
// ============================================================================

const (
	// PasswordResetTokenTTL is the duration a password reset token is valid.
	PasswordResetTokenTTL = 1 * time.Hour
)

var (
	// ErrPasswordResetTokenNotFound is returned when the reset token doesn't exist.
	ErrPasswordResetTokenNotFound = errors.New("password reset token not found")

	// ErrPasswordResetTokenExpired is returned when the reset token has expired.
	ErrPasswordResetTokenExpired = errors.New("password reset token has expired")
)

// ============================================================================
// Password Reset Types
// ============================================================================

// PasswordResetToken represents a token used to reset a user's password.
type PasswordResetToken struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// ============================================================================
// KV Key Functions
// ============================================================================

const passwordResetTokenKeyPrefix = "password_reset."

// passwordResetTokenKey returns the HMAC-derived KV key for a password reset token.
func (c *ChattoCore) passwordResetTokenKey(token string) string {
	return c.runtimeTokenKey(passwordResetTokenKeyPrefix, token)
}

// ============================================================================
// Password Reset Token Operations
// ============================================================================

// CreatePasswordResetToken creates a new password reset token for a verified email.
// If the email is not found or not verified, returns ("", nil) to prevent email enumeration.
// Only verified emails can trigger password reset.
func (c *ChattoCore) CreatePasswordResetToken(ctx context.Context, email string) (string, error) {
	// Normalize email
	normalizedEmail := strings.ToLower(strings.TrimSpace(email))
	if normalizedEmail == "" {
		return "", fmt.Errorf("email is required")
	}

	// Look up user by verified email
	user, err := c.GetUserByVerifiedEmail(ctx, normalizedEmail)
	if err != nil {
		// User not found or email not verified - return empty string, no error
		// This prevents email enumeration attacks
		return "", nil
	}

	// Generate token
	token := NewPasswordResetToken()
	createdAt := time.Now()

	tokenData := PasswordResetToken{
		UserID:    user.Id,
		Email:     normalizedEmail,
		CreatedAt: createdAt,
	}

	data, err := json.Marshal(tokenData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal token: %w", err)
	}

	_, err = c.storage.runtimeStateKV.Create(ctx, c.passwordResetTokenKey(token), data, jetstream.KeyTTL(PasswordResetTokenTTL))
	if err != nil {
		return "", fmt.Errorf("failed to store password reset token: %w", err)
	}

	if err := c.recordPasswordResetLinkIssued(ctx, user.Id, normalizedEmail, createdAt); err != nil {
		_ = c.deletePasswordResetToken(ctx, token)
		return "", err
	}

	return token, nil
}

// getPasswordResetToken retrieves and validates a password reset token.
// Returns the token data including userID and email.
func (c *ChattoCore) getPasswordResetToken(ctx context.Context, token string) (*PasswordResetToken, error) {
	key := c.passwordResetTokenKey(token)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, ErrPasswordResetTokenNotFound
		}
		return nil, fmt.Errorf("failed to get password reset token: %w", err)
	}

	var tokenData PasswordResetToken
	if err := json.Unmarshal(entry.Value(), &tokenData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal token: %w", err)
	}

	// Check if token has expired
	if time.Since(tokenData.CreatedAt) > PasswordResetTokenTTL {
		// Clean up expired token
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return nil, ErrPasswordResetTokenExpired
	}

	return &tokenData, nil
}

// deletePasswordResetToken removes a password reset token.
func (c *ChattoCore) deletePasswordResetToken(ctx context.Context, token string) error {
	key := c.passwordResetTokenKey(token)
	err := c.storage.runtimeStateKV.Delete(ctx, key)
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to delete password reset token: %w", err)
	}
	return nil
}

// ValidatePasswordResetToken validates a token and returns the associated userID.
// This is useful for checking if a token is valid before showing the reset form.
func (c *ChattoCore) ValidatePasswordResetToken(ctx context.Context, token string) (string, error) {
	tokenData, err := c.getPasswordResetToken(ctx, token)
	if err != nil {
		return "", err
	}
	return tokenData.UserID, nil
}

// ResetPassword validates the token, updates the user's password, and deletes the token.
// The newPasswordHash should already be bcrypt-hashed by the caller.
// This is atomic: the token is deleted regardless of password update outcome.
func (c *ChattoCore) ResetPassword(ctx context.Context, token string, newPasswordHash string) error {
	// Validate token first
	tokenData, err := c.getPasswordResetToken(ctx, token)
	if err != nil {
		return err
	}

	// Delete token immediately to prevent reuse (even if password update fails)
	defer c.deletePasswordResetToken(ctx, token)

	passwordChanged := newEvent(tokenData.UserID, &corev1.Event{Event: &corev1.Event_UserPasswordHashChanged{
		UserPasswordHashChanged: &corev1.UserPasswordHashChangedEvent{
			UserId:       tokenData.UserID,
			PasswordHash: []byte(newPasswordHash),
		},
	}})
	agg := events.UserAggregate(tokenData.UserID)
	entries := []events.BatchEntry{
		{
			Subject: agg.SubjectFor(passwordChanged),
			Event:   passwordChanged,
		},
		{
			Subject: agg.Subject(events.EventPasswordResetCompleted),
			Event:   passwordResetCompletedEvent(ctx, tokenData.UserID),
		},
	}
	if _, err := c.appendUserBatch(ctx, tokenData.UserID, entries, "", nil); err != nil {
		return fmt.Errorf("failed to update password: %w", err)
	}
	if _, err := c.RevokeRuntimeCredentialsForUser(ctx, tokenData.UserID, "password_reset"); err != nil {
		c.logger.Warn("Failed to clean up runtime credentials after password reset", "user_id", tokenData.UserID, "error", err)
	}
	if err := c.PublishSessionTerminated(ctx, tokenData.UserID, "password_reset"); err != nil {
		c.logger.Warn("Failed to publish SessionTerminatedEvent", "user_id", tokenData.UserID, "reason", "password_reset", "error", err)
	}

	return nil
}

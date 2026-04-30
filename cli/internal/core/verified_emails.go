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
)

// ============================================================================
// Email Verification Types
// ============================================================================

// EmailVerificationToken represents a token used to verify an email address.
type EmailVerificationToken struct {
	UserID    string    `json:"user_id"`
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// VerifiedEmail represents a verified email address attached to a user.
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

// userVerifiedEmailsKey returns the KV key for a user's verified emails.
func userVerifiedEmailsKey(userID string) string {
	return fmt.Sprintf("user.%s.verified_emails", userID)
}

// userByEmailKey returns the KV key for the email-to-user index.
// Uses SHA256 hash of the lowercase email to ensure valid NATS subject characters
// and case-insensitive uniqueness. Created when an email is verified.
func userByEmailKey(email string) string {
	hash := sha256.Sum256([]byte(strings.ToLower(email)))
	return fmt.Sprintf("user_by_email.%s", hex.EncodeToString(hash[:]))
}

// ============================================================================
// Email Verification Token Operations
// ============================================================================

// CreateEmailVerificationToken creates a new verification token for an email.
// The token contains all info needed for verification (userID, email).
func (c *ChattoCore) CreateEmailVerificationToken(ctx context.Context, userID, email string) (string, error) {
	token := NewEmailVerificationToken()

	tokenData := EmailVerificationToken{
		UserID:    userID,
		Email:     email,
		CreatedAt: time.Now(),
	}

	data, err := json.Marshal(tokenData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal token: %w", err)
	}

	_, err = c.storage.instanceKV.Put(ctx, emailVerificationTokenKey(token), data)
	if err != nil {
		return "", fmt.Errorf("failed to store verification token: %w", err)
	}

	return token, nil
}

// getEmailVerificationToken retrieves and validates a verification token.
// Returns the token data including userID and email.
func (c *ChattoCore) getEmailVerificationToken(ctx context.Context, token string) (*EmailVerificationToken, error) {
	entry, err := c.storage.instanceKV.Get(ctx, emailVerificationTokenKey(token))
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
		c.storage.instanceKV.Delete(ctx, emailVerificationTokenKey(token))
		return nil, ErrTokenExpired
	}

	return &tokenData, nil
}

// deleteEmailVerificationToken removes a verification token.
func (c *ChattoCore) deleteEmailVerificationToken(ctx context.Context, token string) error {
	err := c.storage.instanceKV.Delete(ctx, emailVerificationTokenKey(token))
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

	// Try to claim the email (atomic operation to prevent duplicates)
	normalizedEmail := strings.ToLower(tokenData.Email)
	claimKey := userByEmailKey(normalizedEmail)

	// Track whether we created the claim (for rollback purposes)
	claimCreated := false
	_, err = c.storage.instanceKV.Create(ctx, claimKey, []byte(tokenData.UserID))
	if err != nil {
		// Claim failed - check if it's already claimed by this same user (idempotent)
		entry, getErr := c.storage.instanceKV.Get(ctx, claimKey)
		if getErr != nil || string(entry.Value()) != tokenData.UserID {
			// Email claimed by another user
			return "", ErrEmailAlreadyVerified
		}
		// Claim exists for this user - continue to ensure email is in verified list
	} else {
		claimCreated = true
	}

	// Add to user's verified emails (idempotent - won't duplicate)
	if err := c.addVerifiedEmail(ctx, tokenData.UserID, tokenData.Email); err != nil {
		// Only rollback if we just created the claim
		if claimCreated {
			c.storage.instanceKV.Delete(ctx, claimKey)
		}
		return "", err
	}

	// Clean up: delete the verification token
	c.deleteEmailVerificationToken(ctx, token)

	return tokenData.UserID, nil
}

// addVerifiedEmail adds an email to a user's verified emails list.
// Idempotent - won't add duplicates.
func (c *ChattoCore) addVerifiedEmail(ctx context.Context, userID, email string) error {
	// Get existing verified emails
	emails, err := c.GetVerifiedEmails(ctx, userID)
	if err != nil {
		return err
	}

	// Check if email already exists (case-insensitive)
	normalizedEmail := strings.ToLower(email)
	for _, e := range emails {
		if strings.ToLower(e.Email) == normalizedEmail {
			// Already in list, nothing to do
			return nil
		}
	}

	// Add new email
	emails = append(emails, VerifiedEmail{
		Email:      email,
		VerifiedAt: time.Now(),
	})

	// Store updated list
	data, err := json.Marshal(emails)
	if err != nil {
		return fmt.Errorf("failed to marshal verified emails: %w", err)
	}

	_, err = c.storage.instanceKV.Put(ctx, userVerifiedEmailsKey(userID), data)
	if err != nil {
		return fmt.Errorf("failed to store verified emails: %w", err)
	}

	return nil
}

// GetVerifiedEmails returns all verified emails for a user.
func (c *ChattoCore) GetVerifiedEmails(ctx context.Context, userID string) ([]VerifiedEmail, error) {
	entry, err := c.storage.instanceKV.Get(ctx, userVerifiedEmailsKey(userID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return []VerifiedEmail{}, nil
		}
		return nil, fmt.Errorf("failed to get verified emails: %w", err)
	}

	var emails []VerifiedEmail
	if err := json.Unmarshal(entry.Value(), &emails); err != nil {
		return nil, fmt.Errorf("failed to unmarshal verified emails: %w", err)
	}

	return emails, nil
}

// HasVerifiedEmail checks if a user has at least one verified email.
func (c *ChattoCore) HasVerifiedEmail(ctx context.Context, userID string) (bool, error) {
	emails, err := c.GetVerifiedEmails(ctx, userID)
	if err != nil {
		return false, err
	}
	return len(emails) > 0, nil
}

// IsEmailClaimed checks if an email address is already verified by any user.
// Used to prevent registration with an email that's already in use.
func (c *ChattoCore) IsEmailClaimed(ctx context.Context, email string) (bool, error) {
	normalizedEmail := strings.ToLower(email)
	claimKey := userByEmailKey(normalizedEmail)

	_, err := c.storage.instanceKV.Get(ctx, claimKey)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return false, nil // Email is not claimed
		}
		return false, fmt.Errorf("failed to check email claim: %w", err)
	}
	return true, nil // Email is claimed
}

// GetUserByVerifiedEmail looks up a user by their verified email address.
// Returns the user if found, or an error if not found.
func (c *ChattoCore) GetUserByVerifiedEmail(ctx context.Context, email string) (*corev1.User, error) {
	normalizedEmail := strings.ToLower(email)
	claimKey := userByEmailKey(normalizedEmail)

	entry, err := c.storage.instanceKV.Get(ctx, claimKey)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, fmt.Errorf("no user found with verified email")
		}
		return nil, fmt.Errorf("failed to lookup user by email: %w", err)
	}

	userID := string(entry.Value())
	return c.GetUser(ctx, userID)
}

// CountVerifiedUsers returns the number of users with at least one verified email.
//
// One KV entry exists per user under user.{userID}.verified_emails, so this is a
// key scan without value fetches. ListKeysFiltered is used (rather than the
// faster server-side stream.Info(WithSubjectFilter)) because the latter counts
// tombstones from deleted users — see CountSpaces for the full reasoning.
func (c *ChattoCore) CountVerifiedUsers(ctx context.Context) (int, error) {
	keyLister, err := c.storage.instanceKV.ListKeysFiltered(ctx, "user.*.verified_emails")
	if err != nil {
		return 0, nil
	}
	count := 0
	for range keyLister.Keys() {
		count++
	}
	return count, nil
}

// ListUsersWithVerifiedEmail returns all user IDs that have at least one verified email.
func (c *ChattoCore) ListUsersWithVerifiedEmail(ctx context.Context) ([]string, error) {
	// List all keys matching user.*.verified_emails pattern
	pattern := "user.*.verified_emails"
	keyLister, err := c.storage.instanceKV.ListKeysFiltered(ctx, pattern)
	if err != nil {
		// No keys found is not an error
		return []string{}, nil
	}

	const prefix = "user."
	const suffix = ".verified_emails"

	var users []string
	// Keys have format: user.{userID}.verified_emails
	for key := range keyLister.Keys() {
		// Extract userID from key using TrimPrefix/TrimSuffix for clarity
		if strings.HasPrefix(key, prefix) && strings.HasSuffix(key, suffix) {
			userID := strings.TrimPrefix(key, prefix)
			userID = strings.TrimSuffix(userID, suffix)
			if userID != "" {
				users = append(users, userID)
			}
		}
	}
	return users, nil
}

// AddVerifiedEmailDirect adds an email as verified without requiring token verification.
// Used for OAuth flows where the email is already verified by the provider.
func (c *ChattoCore) AddVerifiedEmailDirect(ctx context.Context, userID, email string) error {
	// Try to claim the email
	normalizedEmail := strings.ToLower(email)
	claimKey := userByEmailKey(normalizedEmail)

	_, err := c.storage.instanceKV.Create(ctx, claimKey, []byte(userID))
	if err != nil {
		// Email already claimed - check if it's by this user
		entry, getErr := c.storage.instanceKV.Get(ctx, claimKey)
		if getErr == nil && string(entry.Value()) == userID {
			// Already verified by this user, nothing to do
			return nil
		}
		// Claimed by another user
		return ErrEmailAlreadyVerified
	}

	// Add to verified emails
	if err := c.addVerifiedEmail(ctx, userID, email); err != nil {
		// Rollback: delete the claim
		c.storage.instanceKV.Delete(ctx, claimKey)
		return err
	}

	return nil
}

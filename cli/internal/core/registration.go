package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// ============================================================================
// Registration Token Constants and Errors
// ============================================================================

const (
	// RegistrationTokenTTL is the duration a registration token is valid.
	RegistrationTokenTTL = 24 * time.Hour
)

var (
	// ErrRegistrationTokenNotFound is returned when the registration token doesn't exist.
	ErrRegistrationTokenNotFound = errors.New("registration token not found")

	// ErrRegistrationTokenExpired is returned when the registration token has expired.
	ErrRegistrationTokenExpired = errors.New("registration token has expired")
)

// ============================================================================
// Registration Token Types
// ============================================================================

// RegistrationToken represents a token used to complete email-first registration.
// Unlike password reset tokens, this has no UserID — the user doesn't exist yet.
type RegistrationToken struct {
	Email     string    `json:"email"`
	CreatedAt time.Time `json:"created_at"`
}

// ============================================================================
// KV Key Functions
// ============================================================================

// registrationTokenKey returns the KV key for a registration token.
// Format: registration.{token}
func registrationTokenKey(token string) string {
	return fmt.Sprintf("registration.%s", token)
}

// ============================================================================
// Registration Token Operations
// ============================================================================

// CreateRegistrationToken creates a new registration token for an email address.
// The token is stored in serverKV and can be used to complete account creation.
//
// Email is expected to already be normalized (trimmed, lowercased) by the caller —
// the HTTP handlers do this at the request boundary.
func (c *ChattoCore) CreateRegistrationToken(ctx context.Context, email string) (string, error) {
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
		return "", fmt.Errorf("failed to marshal registration token: %w", err)
	}

	_, err = c.storage.serverKV.Create(ctx, registrationTokenKey(token), data, jetstream.KeyTTL(RegistrationTokenTTL))
	if err != nil {
		return "", fmt.Errorf("failed to store registration token: %w", err)
	}

	if err := c.recordRegistrationLinkIssued(ctx, email, createdAt); err != nil {
		_ = c.DeleteRegistrationToken(ctx, token)
		return "", err
	}

	return token, nil
}

// GetRegistrationToken retrieves and validates a registration token.
// Returns the token data including the email address.
func (c *ChattoCore) GetRegistrationToken(ctx context.Context, token string) (*RegistrationToken, error) {
	entry, err := c.storage.serverKV.Get(ctx, registrationTokenKey(token))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, ErrRegistrationTokenNotFound
		}
		return nil, fmt.Errorf("failed to get registration token: %w", err)
	}

	var tokenData RegistrationToken
	if err := json.Unmarshal(entry.Value(), &tokenData); err != nil {
		return nil, fmt.Errorf("failed to unmarshal registration token: %w", err)
	}

	// Check if token has expired
	if time.Since(tokenData.CreatedAt) > RegistrationTokenTTL {
		// Clean up expired token
		c.storage.serverKV.Delete(ctx, registrationTokenKey(token))
		return nil, ErrRegistrationTokenExpired
	}

	return &tokenData, nil
}

// DeleteRegistrationToken removes a registration token after successful account creation.
func (c *ChattoCore) DeleteRegistrationToken(ctx context.Context, token string) error {
	err := c.storage.serverKV.Delete(ctx, registrationTokenKey(token))
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to delete registration token: %w", err)
	}
	return nil
}

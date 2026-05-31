package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// ============================================================================
// Auth Token Errors
// ============================================================================

var (
	// ErrAuthTokenNotFound is returned when a bearer auth token doesn't exist or has expired.
	ErrAuthTokenNotFound = errors.New("auth token not found")
)

// authTokenKeyPrefix is the KV key prefix for bearer session tokens.
const authTokenKeyPrefix = "session."

// ============================================================================
// Auth Token Types
// ============================================================================

// AuthTokenData is the JSON value stored in RUNTIME_STATE for bearer tokens.
type AuthTokenData struct {
	UserID    string    `json:"user_id"`
	CreatedAt time.Time `json:"created_at"`
}

// ============================================================================
// Auth Token Operations
// ============================================================================

func (c *ChattoCore) authTokenTTL() time.Duration {
	if c.config.AuthTokenTTL != 0 {
		return c.config.AuthTokenTTL
	}
	return 90 * 24 * time.Hour
}

func (c *ChattoCore) authTokenKey(token string) string {
	return c.runtimeTokenKey(authTokenKeyPrefix, token)
}

// CreateAuthToken creates a new opaque bearer token for the given user.
// The token is stored in RUNTIME_STATE and can be used for API authentication.
// Token expiry is handled by NATS KV TTL.
func (c *ChattoCore) CreateAuthToken(ctx context.Context, userID string) (string, error) {
	token := NewAuthToken()

	data, err := json.Marshal(AuthTokenData{
		UserID:    userID,
		CreatedAt: time.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("failed to marshal auth token: %w", err)
	}

	_, err = c.storage.runtimeStateKV.Create(ctx, c.authTokenKey(token), data, jetstream.KeyTTL(c.authTokenTTL()))
	if err != nil {
		return "", fmt.Errorf("failed to store auth token: %w", err)
	}

	return token, nil
}

// ValidateAuthToken checks if a bearer token is valid and returns the associated user ID.
// Returns ErrAuthTokenNotFound if the token doesn't exist (or has expired via NATS TTL).
//
// Sliding window: each successful validation rewrites the entry to reset the NATS KV TTL.
// This means the token only expires after the configured TTL of *inactivity* — active
// users are never logged out.
func (c *ChattoCore) ValidateAuthToken(ctx context.Context, token string) (string, error) {
	key := c.authTokenKey(token)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return "", ErrAuthTokenNotFound
		}
		return "", fmt.Errorf("failed to get auth token: %w", err)
	}

	var tokenData AuthTokenData
	if err := json.Unmarshal(entry.Value(), &tokenData); err != nil {
		return "", fmt.Errorf("failed to unmarshal auth token: %w", err)
	}

	// Rewrite to reset TTL (sliding window expiry).
	// Fire-and-forget — validation succeeds even if the re-put fails.
	_, _ = c.updateRuntimeStateTokenTTL(ctx, key, entry.Value(), entry.Revision(), c.authTokenTTL())

	return tokenData.UserID, nil
}

// RevokeAuthToken deletes a bearer token, immediately invalidating it.
// This is idempotent — revoking a non-existent token is not an error.
func (c *ChattoCore) RevokeAuthToken(ctx context.Context, token string) error {
	err := c.storage.runtimeStateKV.Delete(ctx, c.authTokenKey(token))
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to revoke auth token: %w", err)
	}
	return nil
}

func (c *ChattoCore) updateRuntimeStateTokenTTL(ctx context.Context, key string, value []byte, revision uint64, ttl time.Duration) (uint64, error) {
	msg := nats.NewMsg("$KV.RUNTIME_STATE." + key)
	msg.Data = value
	ack, err := c.js.PublishMsg(ctx, msg,
		jetstream.WithExpectLastSequencePerSubject(revision),
		jetstream.WithMsgTTL(ttl),
	)
	if err != nil {
		return 0, err
	}
	return ack.Sequence, nil
}

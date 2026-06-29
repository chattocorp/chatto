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
	UserID          string    `json:"user_id"`
	CreatedAt       time.Time `json:"created_at"`
	AuthGeneration  uint64    `json:"auth_generation,omitempty"`
	FreshAuthAt     time.Time `json:"fresh_auth_at,omitempty"`
	FreshAuthMethod string    `json:"fresh_auth_method,omitempty"`
	FreshAuthSource string    `json:"fresh_auth_source,omitempty"`
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
	return c.CreateAuthTokenWithSource(ctx, userID, "unknown")
}

// CreateAuthTokenWithSource creates a new opaque bearer token and records the
// security-safe issuance fact in EVT. The raw token remains only in the return
// value and the HMAC-derived RUNTIME_STATE key.
func (c *ChattoCore) CreateAuthTokenWithSource(ctx context.Context, userID, source string) (string, error) {
	authGeneration, err := c.CurrentAuthGeneration(ctx, userID)
	if err != nil {
		return "", err
	}
	return c.CreateAuthTokenWithSourceGeneration(ctx, userID, source, authGeneration)
}

// CreateAuthTokenWithSourceGeneration creates a bearer token for an
// authentication that proved credentials against authGeneration.
func (c *ChattoCore) CreateAuthTokenWithSourceGeneration(ctx context.Context, userID, source string, authGeneration uint64) (string, error) {
	if err := c.RequireAuthenticationAllowed(ctx, userID, authGeneration); err != nil {
		if errors.Is(err, ErrAuthenticationRevoked) {
			return "", ErrAuthTokenNotFound
		}
		return "", err
	}

	token := NewAuthToken()
	createdAt := time.Now()
	key := c.authTokenKey(token)
	tokenData := AuthTokenData{
		UserID:         userID,
		CreatedAt:      createdAt,
		AuthGeneration: authGeneration,
	}
	if sourceGrantsInitialFreshAuth(source) {
		tokenData.FreshAuthAt = createdAt
		tokenData.FreshAuthMethod = freshAuthMethodForSource(source)
		tokenData.FreshAuthSource = source
	}

	data, err := json.Marshal(tokenData)
	if err != nil {
		return "", fmt.Errorf("failed to marshal auth token: %w", err)
	}

	_, err = c.storage.runtimeStateKV.Create(ctx, key, data, jetstream.KeyTTL(c.authTokenTTL()))
	if err != nil {
		return "", fmt.Errorf("failed to store auth token: %w", err)
	}
	if err := c.recordBearerTokenIssued(ctx, userID, createdAt, source); err != nil {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return "", err
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
	validation, err := c.ValidateRuntimeCredential(ctx, RuntimeCredential{
		UserID:         tokenData.UserID,
		CreatedAt:      tokenData.CreatedAt,
		AuthGeneration: tokenData.AuthGeneration,
	})
	if err != nil {
		if !errors.Is(err, ErrAuthenticationRevoked) {
			return "", err
		}
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return "", ErrAuthTokenNotFound
	}
	value := entry.Value()
	if validation.ShouldPersistAuthGeneration {
		tokenData.AuthGeneration = validation.AuthGeneration
		if upgraded, err := json.Marshal(tokenData); err == nil {
			value = upgraded
		}
	}

	// Rewrite to reset TTL (sliding window expiry).
	// Fire-and-forget — validation succeeds even if the re-put fails.
	_, _ = c.updateRuntimeStateTokenTTL(ctx, key, value, entry.Revision(), c.authTokenTTL())

	return validation.UserID, nil
}

// RevokeAuthToken deletes a bearer token, immediately invalidating it.
// This is idempotent — revoking a non-existent token is not an error.
func (c *ChattoCore) RevokeAuthToken(ctx context.Context, token string) error {
	return c.RevokeAuthTokenWithReason(ctx, token, "explicit")
}

// RevokeAuthTokenWithReason deletes a bearer token and records the revocation
// audit fact when the token existed and could be associated with a user.
func (c *ChattoCore) RevokeAuthTokenWithReason(ctx context.Context, token, reason string) error {
	key := c.authTokenKey(token)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil
		}
		return fmt.Errorf("failed to get auth token for revocation: %w", err)
	}

	var tokenData AuthTokenData
	if err := json.Unmarshal(entry.Value(), &tokenData); err != nil {
		if deleteErr := c.storage.runtimeStateKV.Delete(ctx, key); deleteErr != nil && !errors.Is(deleteErr, jetstream.ErrKeyNotFound) {
			return fmt.Errorf("failed to revoke malformed auth token after unmarshal error %v: %w", err, deleteErr)
		}
		return fmt.Errorf("failed to unmarshal auth token for revocation: %w", err)
	}

	if tokenData.UserID != "" {
		if err := c.recordBearerTokenRevoked(ctx, tokenData.UserID, reason); err != nil {
			return err
		}
	}

	err = c.storage.runtimeStateKV.Delete(ctx, key)
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to revoke auth token: %w", err)
	}
	return nil
}

// RevokeAllAuthTokensForUser deletes all bearer tokens for a user. It is used
// by password changes/resets and account deletion flows that need immediate
// bearer-token revocation across clients.
func (c *ChattoCore) RevokeAllAuthTokensForUser(ctx context.Context, userID string) (int, error) {
	return c.RevokeAllAuthTokensForUserWithReason(ctx, userID, "explicit")
}

// RevokeAllAuthTokensForUserWithReason deletes all bearer tokens for a user and
// records a revocation audit fact for each token that existed.
func (c *ChattoCore) RevokeAllAuthTokensForUserWithReason(ctx context.Context, userID, reason string) (int, error) {
	if userID == "" {
		return 0, nil
	}

	lister, err := c.storage.runtimeStateKV.ListKeysFiltered(ctx, authTokenKeyPrefix+"*")
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to list auth tokens: %w", err)
	}

	var keys []string
	for key := range lister.Keys() {
		keys = append(keys, key)
	}

	revoked := 0
	for _, key := range keys {
		entry, err := c.storage.runtimeStateKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return revoked, fmt.Errorf("failed to get auth token for revoke-all: %w", err)
		}

		var tokenData AuthTokenData
		if err := json.Unmarshal(entry.Value(), &tokenData); err != nil {
			c.logger.Warn("Skipping malformed auth token during revoke-all", "key", key, "error", err)
			continue
		}
		if tokenData.UserID != userID {
			continue
		}

		if err := c.recordBearerTokenRevoked(ctx, userID, reason); err != nil {
			return revoked, err
		}
		if err := c.storage.runtimeStateKV.Delete(ctx, key); err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return revoked, fmt.Errorf("failed to revoke auth token: %w", err)
		}
		revoked++
	}
	return revoked, nil
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

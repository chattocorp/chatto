package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// Auth Token Errors
// ============================================================================

var (
	// ErrAuthTokenNotFound is returned when a bearer auth token doesn't exist or has expired.
	ErrAuthTokenNotFound = errors.New("auth token not found")
)

// authTokenKeyPrefix is the KV key prefix for opaque runtime credentials.
const authTokenKeyPrefix = "session."

// ============================================================================
// Auth Token Types
// ============================================================================

// AuthTokenKind identifies the security class of an opaque runtime credential.
type AuthTokenKind string

const (
	AuthTokenKindFirstPartySession AuthTokenKind = "first_party_session"
	AuthTokenKindOAuthAccessToken  AuthTokenKind = "oauth_access_token"
)

// AuthTokenPresentation identifies how an opaque runtime token is intended to
// be presented by clients.
type AuthTokenPresentation string

const (
	AuthTokenPresentationBearer AuthTokenPresentation = "bearer"
	AuthTokenPresentationCookie AuthTokenPresentation = "cookie"
)

// AuthTokenData is the JSON value stored in RUNTIME_STATE under session.{hmac}.
// New bearer tokens and same-origin cookie session handles share this record
// shape so validators can reject a credential presented through the wrong
// transport. The name is kept for compatibility with the existing auth-token
// service API.
type AuthTokenData struct {
	UserID          string                       `json:"user_id"`
	Kind            AuthTokenKind                `json:"kind,omitempty"`
	Presentation    AuthTokenPresentation        `json:"presentation,omitempty"`
	Source          string                       `json:"source,omitempty"`
	Request         *corev1.AuditRequestMetadata `json:"request,omitempty"`
	CreatedAt       time.Time                    `json:"created_at"`
	AuthGeneration  uint64                       `json:"auth_generation,omitempty"`
	FreshAuthAt     time.Time                    `json:"fresh_auth_at,omitempty"`
	FreshAuthMethod string                       `json:"fresh_auth_method,omitempty"`
	FreshAuthSource string                       `json:"fresh_auth_source,omitempty"`
}

// ValidatedRuntimeCredential is the normalized result of validating an opaque
// runtime credential handle from a specific presentation channel.
type ValidatedRuntimeCredential struct {
	Handle          string
	UserID          string
	Kind            AuthTokenKind
	Presentation    AuthTokenPresentation
	Source          string
	Request         *corev1.AuditRequestMetadata
	CreatedAt       time.Time
	AuthGeneration  uint64
	FreshAuthAt     time.Time
	FreshAuthMethod string
	FreshAuthSource string
}

func authTokenKindForSource(source string) AuthTokenKind {
	if source == "oauth_code_exchange" {
		return AuthTokenKindOAuthAccessToken
	}
	return AuthTokenKindFirstPartySession
}

func (d AuthTokenData) kindOrDefault() AuthTokenKind {
	if d.Kind != "" {
		return d.Kind
	}
	return AuthTokenKindFirstPartySession
}

func (d AuthTokenData) presentationOrDefault() AuthTokenPresentation {
	if d.Presentation != "" {
		return d.Presentation
	}
	return AuthTokenPresentationBearer
}

func validatedRuntimeCredentialFromAuthToken(handle string, data AuthTokenData) ValidatedRuntimeCredential {
	return ValidatedRuntimeCredential{
		Handle:          handle,
		UserID:          data.UserID,
		Kind:            data.kindOrDefault(),
		Presentation:    data.presentationOrDefault(),
		Source:          data.Source,
		Request:         data.Request,
		CreatedAt:       data.CreatedAt,
		AuthGeneration:  data.AuthGeneration,
		FreshAuthAt:     data.FreshAuthAt,
		FreshAuthMethod: data.FreshAuthMethod,
		FreshAuthSource: data.FreshAuthSource,
	}
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

func (c *ChattoCore) runtimeCredentialTTL(presentation AuthTokenPresentation) time.Duration {
	if presentation == AuthTokenPresentationCookie {
		return c.cookieSessionTTL()
	}
	return c.authTokenTTL()
}

// ValidatePresentedRuntimeCredential validates an opaque runtime credential
// handle as presented over a specific transport. Bearer and same-origin cookie
// auth both use session.{hmac} records; the presentation check prevents a
// handle minted for one channel from being replayed through another.
func (c *ChattoCore) ValidatePresentedRuntimeCredential(ctx context.Context, handle string, presentation AuthTokenPresentation) (ValidatedRuntimeCredential, error) {
	if handle == "" {
		return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
	}

	key := c.authTokenKey(handle)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
		}
		return ValidatedRuntimeCredential{}, fmt.Errorf("failed to get runtime credential: %w", err)
	}

	var tokenData AuthTokenData
	if err := json.Unmarshal(entry.Value(), &tokenData); err != nil {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
	}
	if tokenData.presentationOrDefault() != presentation {
		return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
	}
	if tokenData.UserID == "" {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
	}
	if presentation == AuthTokenPresentationCookie && tokenData.kindOrDefault() != AuthTokenKindFirstPartySession {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
	}

	validation, err := c.ValidateRuntimeCredential(ctx, RuntimeCredential{
		UserID:         tokenData.UserID,
		CreatedAt:      tokenData.CreatedAt,
		AuthGeneration: tokenData.AuthGeneration,
	})
	if err != nil {
		if !errors.Is(err, ErrAuthenticationRevoked) {
			return ValidatedRuntimeCredential{}, err
		}
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
	}
	if validation.ShouldPersistAuthGeneration {
		tokenData.AuthGeneration = validation.AuthGeneration
		if value, err := json.Marshal(tokenData); err == nil {
			_, _ = c.updateRuntimeStateTokenTTL(ctx, key, value, entry.Revision(), c.runtimeCredentialTTL(presentation))
		}
	} else {
		_, _ = c.updateRuntimeStateTokenTTL(ctx, key, entry.Value(), entry.Revision(), c.runtimeCredentialTTL(presentation))
	}

	return validatedRuntimeCredentialFromAuthToken(handle, tokenData), nil
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
		Kind:           authTokenKindForSource(source),
		Presentation:   AuthTokenPresentationBearer,
		Source:         source,
		Request:        auditRequestMetadata(ctx),
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
	credential, err := c.ValidatePresentedRuntimeCredential(ctx, token, AuthTokenPresentationBearer)
	if err != nil {
		return "", err
	}
	return credential.UserID, nil
}

// RevokeAuthToken deletes a bearer token, immediately invalidating it.
// This is idempotent — revoking a non-existent token is not an error.
func (c *ChattoCore) RevokeAuthToken(ctx context.Context, token string) error {
	return c.RevokeAuthTokenWithReason(ctx, token, "explicit")
}

// RevokeAuthTokenWithReason deletes a bearer token and records the revocation
// audit fact when the token existed and could be associated with a user.
func (c *ChattoCore) RevokeAuthTokenWithReason(ctx context.Context, token, reason string) error {
	_, _, err := c.RevokePresentedRuntimeCredentialWithReason(ctx, token, AuthTokenPresentationBearer, reason)
	return err
}

// RevokePresentedRuntimeCredentialWithReason deletes one opaque runtime
// credential for the requested presentation channel. It returns the owning user
// ID when the credential existed so HTTP-edge logout can apply one audit and
// live-session termination flow for bearer and cookie presentations.
func (c *ChattoCore) RevokePresentedRuntimeCredentialWithReason(ctx context.Context, token string, presentation AuthTokenPresentation, reason string) (string, bool, error) {
	if token == "" {
		return "", false, nil
	}
	key := c.authTokenKey(token)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("failed to get runtime credential for revocation: %w", err)
	}

	var tokenData AuthTokenData
	if err := json.Unmarshal(entry.Value(), &tokenData); err != nil {
		if deleteErr := c.storage.runtimeStateKV.Delete(ctx, key); deleteErr != nil && !errors.Is(deleteErr, jetstream.ErrKeyNotFound) {
			return "", false, fmt.Errorf("failed to revoke malformed runtime credential after unmarshal error %v: %w", err, deleteErr)
		}
		return "", true, fmt.Errorf("failed to unmarshal runtime credential for revocation: %w", err)
	}
	if tokenData.presentationOrDefault() != presentation {
		return "", false, nil
	}

	if presentation == AuthTokenPresentationBearer && tokenData.UserID != "" {
		if err := c.recordBearerTokenRevoked(ctx, tokenData.UserID, reason); err != nil {
			return tokenData.UserID, false, err
		}
	}

	err = c.storage.runtimeStateKV.Delete(ctx, key)
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return tokenData.UserID, false, fmt.Errorf("failed to revoke runtime credential: %w", err)
	}
	return tokenData.UserID, true, nil
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
		if tokenData.presentationOrDefault() != AuthTokenPresentationBearer {
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

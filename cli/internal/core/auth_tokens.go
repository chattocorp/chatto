package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
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
	// AuthTokenPresentationResourceBearer identifies access credentials that
	// are bound to an OAuth resource and scope set. General bearer validators
	// must reject this presentation instead of ignoring the grant boundary.
	AuthTokenPresentationResourceBearer AuthTokenPresentation = "resource_bearer"
	AuthTokenPresentationCookie         AuthTokenPresentation = "cookie"
)

func isBearerPresentation(presentation AuthTokenPresentation) bool {
	return presentation == AuthTokenPresentationBearer || presentation == AuthTokenPresentationResourceBearer
}

// AuthTokenData is the JSON value stored in RUNTIME_STATE under session.{hmac}.
// New bearer tokens and same-origin cookie session handles share this record
// shape so validators can reject a credential presented through the wrong
// transport. The name is kept for compatibility with the existing auth-token
// service API.
type AuthTokenData struct {
	UserID             string                      `json:"user_id"`
	ClientID           string                      `json:"client_id,omitempty"`
	Resource           string                      `json:"resource,omitempty"`
	Scopes             []string                    `json:"scopes,omitempty"`
	Kind               AuthTokenKind               `json:"kind,omitempty"`
	Presentation       AuthTokenPresentation       `json:"presentation,omitempty"`
	Source             string                      `json:"source,omitempty"`
	Request            *evtv1.AuditRequestMetadata `json:"request,omitempty"`
	CreatedAt          time.Time                   `json:"created_at"`
	ExpiresAt          time.Time                   `json:"expires_at,omitempty"`
	AuthGeneration     uint64                      `json:"auth_generation,omitempty"`
	RenewableSessionID string                      `json:"renewable_session_id,omitempty"`
	AccessGeneration   uint64                      `json:"access_generation,omitempty"`
	FreshAuthAt        time.Time                   `json:"fresh_auth_at,omitempty"`
	FreshAuthMethod    string                      `json:"fresh_auth_method,omitempty"`
	FreshAuthSource    string                      `json:"fresh_auth_source,omitempty"`
}

// ValidatedRuntimeCredential is the normalized result of validating an opaque
// runtime credential handle from a specific presentation channel.
type ValidatedRuntimeCredential struct {
	Handle             string
	UserID             string
	ClientID           string
	Resource           string
	Scopes             []string
	Kind               AuthTokenKind
	Presentation       AuthTokenPresentation
	Source             string
	Request            *evtv1.AuditRequestMetadata
	CreatedAt          time.Time
	ExpiresAt          time.Time
	AuthGeneration     uint64
	RenewableSessionID string
	AccessGeneration   uint64
	FreshAuthAt        time.Time
	FreshAuthMethod    string
	FreshAuthSource    string
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
		Handle:             handle,
		UserID:             data.UserID,
		ClientID:           data.ClientID,
		Resource:           data.Resource,
		Scopes:             append([]string(nil), data.Scopes...),
		Kind:               data.kindOrDefault(),
		Presentation:       data.presentationOrDefault(),
		Source:             data.Source,
		Request:            data.Request,
		CreatedAt:          data.CreatedAt,
		ExpiresAt:          data.ExpiresAt,
		AuthGeneration:     data.AuthGeneration,
		RenewableSessionID: data.RenewableSessionID,
		AccessGeneration:   data.AccessGeneration,
		FreshAuthAt:        data.FreshAuthAt,
		FreshAuthMethod:    data.FreshAuthMethod,
		FreshAuthSource:    data.FreshAuthSource,
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

// ValidatePresentedRuntimeCredential validates an opaque runtime credential
// handle as presented over a specific transport. General bearer,
// resource-bound bearer, and same-origin cookie auth use session.{hmac}
// records. The presentation check prevents a handle minted for one channel or
// grant class from being replayed through another.
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
		_ = c.deleteRuntimeStateKey(ctx, key)
		return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
	}
	if tokenData.presentationOrDefault() != presentation {
		return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
	}
	if tokenData.UserID == "" {
		_ = c.deleteRuntimeStateKey(ctx, key)
		return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
	}
	if presentation == AuthTokenPresentationCookie && tokenData.kindOrDefault() != AuthTokenKindFirstPartySession {
		_ = c.deleteRuntimeStateKey(ctx, key)
		return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
	}
	if presentation == AuthTokenPresentationCookie {
		if tokenData.CreatedAt.IsZero() || tokenData.ExpiresAt.IsZero() || !time.Now().Before(tokenData.ExpiresAt) {
			_ = c.deleteRuntimeStateKey(ctx, key)
			return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
		}
	}
	if isBearerPresentation(presentation) {
		now := time.Now()
		if tokenData.RenewableSessionID == "" || tokenData.ExpiresAt.IsZero() || !now.Before(tokenData.ExpiresAt) {
			_ = c.storage.runtimeStateKV.Delete(ctx, key)
			return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
		}
		session, _, err := c.validateRenewableSession(ctx, tokenData.RenewableSessionID, now)
		if err != nil {
			if errors.Is(err, ErrRefreshTokenNotFound) {
				_ = c.storage.runtimeStateKV.Delete(ctx, key)
				return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
			}
			return ValidatedRuntimeCredential{}, err
		}
		if session.UserID != tokenData.UserID || session.ClientID != tokenData.ClientID || session.Resource != tokenData.Resource || !slices.Equal(session.Scopes, tokenData.Scopes) || session.Kind != tokenData.kindOrDefault() || session.AuthGeneration != tokenData.AuthGeneration || tokenData.AccessGeneration > session.CurrentGeneration {
			_ = c.storage.runtimeStateKV.Delete(ctx, key)
			return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
		}
		// Fresh authentication belongs to the renewable session so a rotation
		// racing a password re-verification cannot strand the newly issued access
		// generation with stale copied metadata.
		tokenData.FreshAuthAt = session.FreshAuthAt
		tokenData.FreshAuthMethod = session.FreshAuthMethod
		tokenData.FreshAuthSource = session.FreshAuthSource
		return validatedRuntimeCredentialFromAuthToken(handle, tokenData), nil
	}
	if tokenData.kindOrDefault() == AuthTokenKindOAuthAccessToken {
		if err := c.RequireOAuthClientAllowed(ctx, tokenData.ClientID); err != nil {
			if !errors.Is(err, ErrOAuthClientBlocked) {
				return ValidatedRuntimeCredential{}, err
			}
			_ = c.storage.runtimeStateKV.Delete(ctx, key)
			return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
		}
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
		_ = c.deleteRuntimeStateKey(ctx, key)
		return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
	}
	if validation.ShouldPersistAuthGeneration {
		tokenData.AuthGeneration = validation.AuthGeneration
		if value, err := json.Marshal(tokenData); err == nil {
			_, _ = c.updateRuntimeStateUntil(ctx, key, value, entry.Revision(), tokenData.ExpiresAt, time.Now())
		}
	}

	return validatedRuntimeCredentialFromAuthToken(handle, tokenData), nil
}

// ValidatePublicBearerCredential validates a bearer credential for Chatto's
// general public API and realtime transports. A resource-bound credential is
// valid only at its resource server and must not become general account
// authority through these transports.
func (c *ChattoCore) ValidatePublicBearerCredential(ctx context.Context, handle string) (ValidatedRuntimeCredential, error) {
	credential, err := c.ValidatePresentedRuntimeCredential(ctx, handle, AuthTokenPresentationBearer)
	if err != nil {
		return ValidatedRuntimeCredential{}, err
	}
	if credential.Resource != "" || len(credential.Scopes) != 0 {
		return ValidatedRuntimeCredential{}, ErrAuthTokenNotFound
	}
	return credential, nil
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
	credentials, err := c.CreateBearerSessionWithSource(ctx, userID, source)
	if err != nil {
		return "", err
	}
	return credentials.AccessToken, nil
}

// CreateAuthTokenWithSourceGeneration creates a bearer token for an
// authentication that proved credentials against authGeneration.
func (c *ChattoCore) CreateAuthTokenWithSourceGeneration(ctx context.Context, userID, source string, authGeneration uint64) (string, error) {
	credentials, err := c.CreateBearerSessionWithSourceGeneration(ctx, userID, source, authGeneration)
	if err != nil {
		return "", err
	}
	return credentials.AccessToken, nil
}

// CreateOAuthAccessTokenForClient creates a bearer token bound to the public
// OAuth client that completed the authorization-code flow.
func (c *ChattoCore) CreateOAuthAccessTokenForClient(ctx context.Context, userID, clientID string, authGeneration uint64) (string, error) {
	credentials, err := c.CreateOAuthBearerSessionForClient(ctx, userID, clientID, authGeneration)
	if err != nil {
		return "", err
	}
	return credentials.AccessToken, nil
}

// ValidateAuthToken checks if a bearer token is valid and returns the associated user ID.
// Returns ErrAuthTokenNotFound if the token doesn't exist, has reached its
// fixed expiry, or its renewable session is no longer valid.
func (c *ChattoCore) ValidateAuthToken(ctx context.Context, token string) (string, error) {
	credential, err := c.ValidatePublicBearerCredential(ctx, token)
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
		deleteErr := c.deleteRuntimeStateKey(ctx, key)
		if deleteErr != nil && !errors.Is(deleteErr, jetstream.ErrKeyNotFound) {
			return "", false, fmt.Errorf("failed to revoke malformed runtime credential after unmarshal error %v: %w", err, deleteErr)
		}
		return "", true, fmt.Errorf("failed to unmarshal runtime credential for revocation: %w", err)
	}
	if tokenData.presentationOrDefault() != presentation {
		return "", false, nil
	}

	if isBearerPresentation(presentation) {
		if tokenData.RenewableSessionID == "" {
			_ = c.storage.runtimeStateKV.Delete(ctx, key)
			return tokenData.UserID, true, nil
		}
		if err := c.revokeRenewableSession(ctx, tokenData.RenewableSessionID, reason); err != nil {
			return tokenData.UserID, false, err
		}
	}

	err = c.deleteRuntimeStateKey(ctx, key)
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

// RevokeAllAuthTokensForUserWithReason deletes every renewable bearer session
// for a user and records one revocation audit fact per session.
func (c *ChattoCore) RevokeAllAuthTokensForUserWithReason(ctx context.Context, userID, reason string) (int, error) {
	if userID == "" {
		return 0, nil
	}

	lister, err := c.storage.runtimeStateKV.ListKeysFiltered(ctx, renewableSessionKeyPrefix+"*")
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
			return revoked, fmt.Errorf("failed to get renewable session for revoke-all: %w", err)
		}

		var session RenewableSession
		if err := json.Unmarshal(entry.Value(), &session); err != nil {
			c.logger.Warn("Skipping malformed renewable session during revoke-all", "key", key, "error", err)
			continue
		}
		if session.UserID != userID {
			continue
		}

		if err := c.deleteRuntimeStateKey(ctx, key); err != nil {
			return revoked, fmt.Errorf("failed to revoke renewable session: %w", err)
		}
		if err := c.recordBearerTokenRevoked(ctx, userID, reason); err != nil {
			c.logger.Warn("Failed to append bearer-token revocation audit event", "error", err)
		}
		revoked++
	}
	return revoked, nil
}

// RevokeOAuthClientTokens removes all bearer credentials issued to a public
// OAuth client. Policy enforcement remains authoritative even if this
// defense-in-depth cleanup is interrupted.
func (c *ChattoCore) RevokeOAuthClientTokens(ctx context.Context, clientID string) (int, error) {
	if clientID == "" {
		return 0, nil
	}
	lister, err := c.storage.runtimeStateKV.ListKeysFiltered(ctx, renewableSessionKeyPrefix+"*")
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to list OAuth client tokens: %w", err)
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
			return revoked, fmt.Errorf("failed to get OAuth client renewable session: %w", err)
		}
		var session RenewableSession
		if err := json.Unmarshal(entry.Value(), &session); err != nil {
			continue
		}
		if session.Kind != AuthTokenKindOAuthAccessToken || session.ClientID != clientID {
			continue
		}
		if err := c.deleteRuntimeStateKey(ctx, key); err != nil {
			return revoked, fmt.Errorf("failed to revoke OAuth client token: %w", err)
		}
		if session.UserID != "" {
			if err := c.recordBearerTokenRevoked(ctx, session.UserID, "oauth_client_blocked"); err != nil {
				c.logger.Warn("Failed to append OAuth token revocation audit event", "error", err)
			}
		}
		revoked++
	}
	return revoked, nil
}

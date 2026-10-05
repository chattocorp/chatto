package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const FreshAuthWindow = 30 * time.Minute

var ErrFreshAuthRequired = errors.New("fresh authentication is required")

func freshAuthMethodForSource(source string) string {
	switch {
	case strings.Contains(source, "password"):
		return "password"
	case strings.Contains(source, "oidc"),
		strings.Contains(source, "oauth"),
		strings.Contains(source, "external_identity"),
		strings.Contains(source, "github"),
		strings.Contains(source, "gitlab"),
		strings.Contains(source, "discord"):
		return "external_identity"
	default:
		return "login"
	}
}

func sourceGrantsInitialFreshAuth(source string) bool {
	if source == "oauth_code_exchange" || source == "unknown" {
		return false
	}
	return source == "external_identity_create" ||
		source == "registration" ||
		source == "registration_complete" ||
		strings.HasSuffix(source, "_login")
}

func isFreshAuthAt(at time.Time, now time.Time) bool {
	return !at.IsZero() && now.Sub(at) >= 0 && now.Sub(at) <= FreshAuthWindow
}

// RequireFreshAuthForBearerToken reports ErrFreshAuthRequired unless the
// bearer token's session re-verified within FreshAuthWindow.
func (c *ChattoCore) RequireFreshAuthForBearerToken(ctx context.Context, token string) error {
	return c.requireFreshCredential(ctx, token, AuthTokenPresentationBearer, ErrAuthTokenNotFound)
}

// RequireFreshAuthForCookieSession reports ErrFreshAuthRequired unless the
// cookie session re-verified within FreshAuthWindow.
func (c *ChattoCore) RequireFreshAuthForCookieSession(ctx context.Context, sessionID string) error {
	return c.requireFreshCredential(ctx, sessionID, AuthTokenPresentationCookie, ErrCookieSessionNotFound)
}

// requireFreshCredential validates a credential through the stream leader, so
// a re-verification on another replica is always visible. notFound is the
// caller's error for an unknown credential.
func (c *ChattoCore) requireFreshCredential(ctx context.Context, handle string, presentation AuthTokenPresentation, notFound error) error {
	credential, err := c.validateRuntimeCredential(ctx, handle, presentation, authoritativeCredentialRead)
	if errors.Is(err, ErrAuthTokenNotFound) {
		return notFound
	}
	if err != nil {
		return err
	}
	if credential.Kind != AuthTokenKindFirstPartySession || !isFreshAuthAt(credential.FreshAuthAt, time.Now()) {
		return ErrFreshAuthRequired
	}
	return nil
}

// MarkBearerTokenFresh records a re-verification on the bearer token's
// session. It validates the token through the stream leader.
func (c *ChattoCore) MarkBearerTokenFresh(ctx context.Context, token, method, source string) error {
	credential, err := c.validateRuntimeCredential(ctx, token, AuthTokenPresentationBearer, authoritativeCredentialRead)
	if err != nil {
		return err
	}
	if credential.Kind != AuthTokenKindFirstPartySession {
		return ErrFreshAuthRequired
	}
	return c.markRenewableSessionFresh(ctx, credential.RenewableSessionID, method, source, time.Now())
}

func (c *ChattoCore) MarkCookieSessionFresh(ctx context.Context, sessionID, method, source string) error {
	if sessionID == "" {
		return ErrCookieSessionNotFound
	}
	key := c.authTokenKey(sessionID)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return ErrCookieSessionNotFound
		}
		return fmt.Errorf("failed to get cookie session token: %w", err)
	}

	var tokenData AuthTokenData
	if err := json.Unmarshal(entry.Value(), &tokenData); err != nil {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return ErrCookieSessionNotFound
	}
	if tokenData.UserID == "" ||
		tokenData.kindOrDefault() != AuthTokenKindFirstPartySession ||
		tokenData.presentationOrDefault() != AuthTokenPresentationCookie ||
		tokenData.CreatedAt.IsZero() ||
		tokenData.ExpiresAt.IsZero() {
		_ = c.deleteRuntimeStateKey(ctx, key)
		return ErrCookieSessionNotFound
	}
	expiresAt := tokenData.ExpiresAt
	now := time.Now()
	if !now.Before(expiresAt) {
		_ = c.deleteRuntimeStateKey(ctx, key)
		return ErrCookieSessionNotFound
	}
	if err := c.RequireAuthenticationAllowed(ctx, tokenData.UserID, tokenData.AuthGeneration); err != nil {
		if errors.Is(err, ErrAuthenticationRevoked) {
			_ = c.deleteRuntimeStateKey(ctx, key)
			return ErrCookieSessionNotFound
		}
		return err
	}
	tokenData.FreshAuthAt = now
	tokenData.FreshAuthMethod = method
	tokenData.FreshAuthSource = source
	value, err := json.Marshal(tokenData)
	if err != nil {
		return fmt.Errorf("failed to marshal cookie session token: %w", err)
	}
	_, err = c.updateRuntimeStateUntil(ctx, key, value, entry.Revision(), expiresAt, now)
	if err != nil {
		return fmt.Errorf("failed to mark cookie session fresh: %w", err)
	}
	return nil
}

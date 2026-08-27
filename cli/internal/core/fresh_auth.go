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

func (c *ChattoCore) RequireFreshAuthForBearerToken(ctx context.Context, token string) error {
	data, _, err := c.authTokenData(ctx, token)
	if err != nil {
		return err
	}
	if !data.canSatisfyFreshAuth() {
		return ErrFreshAuthRequired
	}
	if isFreshAuthAt(data.FreshAuthAt, time.Now()) {
		return nil
	}
	return ErrFreshAuthRequired
}

func (c *ChattoCore) MarkBearerTokenFresh(ctx context.Context, token, method, source string) error {
	data, _, err := c.authTokenData(ctx, token)
	if err != nil {
		return err
	}
	if data.Kind != AuthTokenKindFirstPartySession {
		return ErrFreshAuthRequired
	}
	now := time.Now()
	if err := c.markRenewableSessionFresh(ctx, data.RenewableSessionID, method, source, now); err != nil {
		return err
	}
	return nil
}

func (d AuthTokenData) canSatisfyFreshAuth() bool {
	if d.Kind != "" {
		return d.Kind == AuthTokenKindFirstPartySession
	}
	return d.FreshAuthSource != "" && d.FreshAuthSource != "oauth_code_exchange"
}

func (c *ChattoCore) authTokenData(ctx context.Context, token string) (AuthTokenData, jetstream.KeyValueEntry, error) {
	if token == "" {
		return AuthTokenData{}, nil, ErrAuthTokenNotFound
	}
	key := c.authTokenKey(token)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return AuthTokenData{}, nil, ErrAuthTokenNotFound
		}
		return AuthTokenData{}, nil, fmt.Errorf("failed to get auth token: %w", err)
	}

	var tokenData AuthTokenData
	if err := json.Unmarshal(entry.Value(), &tokenData); err != nil {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return AuthTokenData{}, nil, ErrAuthTokenNotFound
	}
	if tokenData.presentationOrDefault() != AuthTokenPresentationBearer {
		return AuthTokenData{}, nil, ErrAuthTokenNotFound
	}
	if tokenData.UserID == "" {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return AuthTokenData{}, nil, ErrAuthTokenNotFound
	}
	if tokenData.RenewableSessionID == "" || tokenData.ExpiresAt.IsZero() || !time.Now().Before(tokenData.ExpiresAt) {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return AuthTokenData{}, nil, ErrAuthTokenNotFound
	}
	session, _, err := c.validateRenewableSession(ctx, tokenData.RenewableSessionID, time.Now())
	if err != nil {
		if errors.Is(err, ErrRefreshTokenNotFound) {
			_ = c.storage.runtimeStateKV.Delete(ctx, key)
			return AuthTokenData{}, nil, ErrAuthTokenNotFound
		}
		return AuthTokenData{}, nil, err
	}
	if session.UserID != tokenData.UserID || session.ClientID != tokenData.ClientID || session.Kind != tokenData.kindOrDefault() || session.AuthGeneration != tokenData.AuthGeneration || tokenData.AccessGeneration > session.CurrentGeneration {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return AuthTokenData{}, nil, ErrAuthTokenNotFound
	}
	tokenData.FreshAuthAt = session.FreshAuthAt
	tokenData.FreshAuthMethod = session.FreshAuthMethod
	tokenData.FreshAuthSource = session.FreshAuthSource
	if _, err := c.ValidateRuntimeCredential(ctx, RuntimeCredential{
		UserID:         tokenData.UserID,
		CreatedAt:      tokenData.CreatedAt,
		AuthGeneration: tokenData.AuthGeneration,
	}); err != nil {
		if errors.Is(err, ErrAuthenticationRevoked) {
			_ = c.storage.runtimeStateKV.Delete(ctx, key)
			return AuthTokenData{}, nil, ErrAuthTokenNotFound
		}
		return AuthTokenData{}, nil, err
	}
	return tokenData, entry, nil
}

func (c *ChattoCore) RequireFreshAuthForCookieSession(ctx context.Context, sessionID string) error {
	record, err := c.ValidateCookieCredential(ctx, sessionID)
	if err != nil {
		return err
	}
	if record.GetFreshAuthAt() != nil && isFreshAuthAt(record.GetFreshAuthAt().AsTime(), time.Now()) {
		return nil
	}
	return ErrFreshAuthRequired
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
	validation, err := c.ValidateRuntimeCredential(ctx, RuntimeCredential{
		UserID:         tokenData.UserID,
		CreatedAt:      tokenData.CreatedAt,
		AuthGeneration: tokenData.AuthGeneration,
	})
	if err != nil {
		if errors.Is(err, ErrAuthenticationRevoked) {
			_ = c.deleteRuntimeStateKey(ctx, key)
			return ErrCookieSessionNotFound
		}
		return err
	}
	if validation.ShouldPersistAuthGeneration {
		tokenData.AuthGeneration = validation.AuthGeneration
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

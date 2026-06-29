package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
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
	if isFreshAuthAt(data.FreshAuthAt, time.Now()) {
		return nil
	}
	return ErrFreshAuthRequired
}

func (c *ChattoCore) MarkBearerTokenFresh(ctx context.Context, token, method, source string) error {
	data, entry, err := c.authTokenData(ctx, token)
	if err != nil {
		return err
	}
	data.FreshAuthAt = time.Now()
	data.FreshAuthMethod = method
	data.FreshAuthSource = source
	value, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal auth token: %w", err)
	}
	_, err = c.updateRuntimeStateTokenTTL(ctx, c.authTokenKey(token), value, entry.Revision(), c.authTokenTTL())
	if err != nil {
		return fmt.Errorf("failed to mark auth token fresh: %w", err)
	}
	return nil
}

func (c *ChattoCore) authTokenData(ctx context.Context, token string) (AuthTokenData, jetstream.KeyValueEntry, error) {
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
		return AuthTokenData{}, nil, fmt.Errorf("failed to unmarshal auth token: %w", err)
	}
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

func (c *ChattoCore) RequireFreshAuthForCookieSession(ctx context.Context, userID, sessionID string) error {
	record, err := c.ValidateCookieSession(ctx, userID, sessionID)
	if err != nil {
		return err
	}
	if record.GetFreshAuthAt() != nil && isFreshAuthAt(record.GetFreshAuthAt().AsTime(), time.Now()) {
		return nil
	}
	return ErrFreshAuthRequired
}

func (c *ChattoCore) MarkCookieSessionFresh(ctx context.Context, userID, sessionID, method, source string) error {
	if userID == "" || sessionID == "" {
		return ErrCookieSessionNotFound
	}
	key := c.cookieSessionKey(userID, sessionID)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return ErrCookieSessionNotFound
		}
		return fmt.Errorf("failed to get cookie session: %w", err)
	}
	var record corev1.CookieSession
	if err := proto.Unmarshal(entry.Value(), &record); err != nil {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return ErrCookieSessionNotFound
	}
	if record.GetUserId() != userID || record.GetExpiresAt() == nil || !time.Now().Before(record.GetExpiresAt().AsTime()) {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return ErrCookieSessionNotFound
	}
	if record.GetCreatedAt() == nil {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return ErrCookieSessionNotFound
	}
	validation, err := c.ValidateRuntimeCredential(ctx, RuntimeCredential{
		UserID:         userID,
		CreatedAt:      record.GetCreatedAt().AsTime(),
		AuthGeneration: record.GetAuthGeneration(),
	})
	if err != nil {
		if errors.Is(err, ErrAuthenticationRevoked) {
			_ = c.storage.runtimeStateKV.Delete(ctx, key)
			return ErrCookieSessionNotFound
		}
		return err
	}
	if validation.ShouldPersistAuthGeneration {
		record.AuthGeneration = validation.AuthGeneration
	}
	record.FreshAuthAt = timestamppb.Now()
	record.FreshAuthMethod = method
	record.FreshAuthSource = source
	value, err := proto.Marshal(&record)
	if err != nil {
		return fmt.Errorf("failed to marshal cookie session: %w", err)
	}
	ttl := time.Until(record.GetExpiresAt().AsTime())
	if ttl <= 0 {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return ErrCookieSessionNotFound
	}
	_, err = c.updateRuntimeStateTokenTTL(ctx, key, value, entry.Revision(), ttl)
	if err != nil {
		return fmt.Errorf("failed to mark cookie session fresh: %w", err)
	}
	return nil
}

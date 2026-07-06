package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

var (
	// ErrCookieSessionNotFound is returned when a cookie session does not exist,
	// has expired, is malformed, or does not belong to the supplied user.
	ErrCookieSessionNotFound = errors.New("cookie session not found")
)

// cookieSessionKeyPrefix is the legacy KV key prefix for protobuf-backed
// browser cookie sessions.
//
// Deprecated: current login flows write cookie-presentation runtime credentials
// to session.{hmac}. Keep this prefix readable/deletable only until legacy
// sessions have exceeded the configured auth token TTL after rollout.
const cookieSessionKeyPrefix = "cookie_session."

func (c *ChattoCore) cookieSessionTTL() time.Duration {
	return c.authTokenTTL()
}

func cookieSessionUserKeyFilter(userID string) string {
	return cookieSessionKeyPrefix + userID + ".*"
}

func (c *ChattoCore) cookieSessionKey(userID, sessionID string) string {
	return c.runtimeTokenKey(cookieSessionKeyPrefix+userID+".", sessionID)
}

// CreateCookieSession creates a first-party runtime credential for same-origin
// cookie presentation and returns the opaque handle that should be stored in the
// signed browser cookie.
func (c *ChattoCore) CreateCookieSession(ctx context.Context, userID, source string) (string, *corev1.CookieSession, error) {
	authGeneration, err := c.CurrentAuthGeneration(ctx, userID)
	if err != nil {
		return "", nil, err
	}
	return c.CreateCookieSessionForGeneration(ctx, userID, source, authGeneration)
}

// CreateCookieSessionForGeneration creates a first-party cookie-presentation
// runtime credential for an authentication that proved credentials against
// authGeneration.
func (c *ChattoCore) CreateCookieSessionForGeneration(ctx context.Context, userID, source string, authGeneration uint64) (string, *corev1.CookieSession, error) {
	now := time.Now()
	return c.createCookieSessionForGeneration(ctx, userID, source, authGeneration, now, freshAuthMethodForSource(source), source)
}

func (c *ChattoCore) CreateCookieSessionForGenerationPreservingFreshAuth(ctx context.Context, userID, source string, authGeneration uint64, previous *corev1.CookieSession) (string, *corev1.CookieSession, error) {
	var freshAuthAt time.Time
	var freshAuthMethod, freshAuthSource string
	if previous != nil {
		if previous.GetFreshAuthAt() != nil {
			freshAuthAt = previous.GetFreshAuthAt().AsTime()
		}
		freshAuthMethod = previous.GetFreshAuthMethod()
		freshAuthSource = previous.GetFreshAuthSource()
	}
	return c.createCookieSessionForGeneration(ctx, userID, source, authGeneration, freshAuthAt, freshAuthMethod, freshAuthSource)
}

func (c *ChattoCore) createCookieSessionForGeneration(ctx context.Context, userID, source string, authGeneration uint64, freshAuthAt time.Time, freshAuthMethod, freshAuthSource string) (string, *corev1.CookieSession, error) {
	if userID == "" {
		return "", nil, ErrCookieSessionNotFound
	}
	if err := c.RequireAuthenticationAllowed(ctx, userID, authGeneration); err != nil {
		if !errors.Is(err, ErrAuthenticationRevoked) {
			return "", nil, err
		}
		return "", nil, ErrCookieSessionNotFound
	}

	sessionID := NewAuthToken()
	now := time.Now()
	tokenData := AuthTokenData{
		UserID:         userID,
		Kind:           AuthTokenKindFirstPartySession,
		Presentation:   AuthTokenPresentationCookie,
		Source:         source,
		Request:        auditRequestMetadata(ctx),
		CreatedAt:      now,
		AuthGeneration: authGeneration,
	}
	if !freshAuthAt.IsZero() {
		tokenData.FreshAuthAt = freshAuthAt
		tokenData.FreshAuthMethod = freshAuthMethod
		tokenData.FreshAuthSource = freshAuthSource
	}

	data, err := json.Marshal(tokenData)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal cookie session: %w", err)
	}

	key := c.authTokenKey(sessionID)
	if _, err := c.storage.runtimeStateKV.Create(ctx, key, data, jetstream.KeyTTL(c.cookieSessionTTL())); err != nil {
		return "", nil, fmt.Errorf("failed to store cookie session: %w", err)
	}

	return sessionID, c.cookieSessionRecordFromAuthTokenData(tokenData), nil
}

// ValidateCookieSession validates a cookie-backed server-side session and
// returns its runtime-state record. Callers must still load the current user
// projection before authenticating the request.
func (c *ChattoCore) ValidateCookieSession(ctx context.Context, userID, sessionID string) (*corev1.CookieSession, error) {
	if userID == "" || sessionID == "" {
		return nil, ErrCookieSessionNotFound
	}

	if record, err := c.validateTokenBackedCookieSession(ctx, userID, sessionID); err == nil {
		return record, nil
	} else if !errors.Is(err, ErrCookieSessionNotFound) {
		return nil, err
	}

	return c.validateLegacyCookieSession(ctx, userID, sessionID)
}

// ValidateCookieCredential validates a typed cookie-presentation runtime
// credential. Unlike deprecated cookie_session.* records, typed credentials
// carry their user ID in the runtime-state record, so callers do not need to
// trust or duplicate the user ID in the signed browser cookie.
func (c *ChattoCore) ValidateCookieCredential(ctx context.Context, sessionID string) (*corev1.CookieSession, error) {
	if sessionID == "" {
		return nil, ErrCookieSessionNotFound
	}
	return c.validateTokenBackedCookieSession(ctx, "", sessionID)
}

func (c *ChattoCore) validateTokenBackedCookieSession(ctx context.Context, userID, sessionID string) (*corev1.CookieSession, error) {
	credential, err := c.ValidatePresentedRuntimeCredential(ctx, sessionID, AuthTokenPresentationCookie)
	if err != nil {
		if errors.Is(err, ErrAuthTokenNotFound) {
			return nil, ErrCookieSessionNotFound
		}
		return nil, err
	}
	if credential.CreatedAt.IsZero() || credential.Kind != AuthTokenKindFirstPartySession {
		return nil, ErrCookieSessionNotFound
	}
	if userID != "" && credential.UserID != userID {
		return nil, ErrCookieSessionNotFound
	}

	return c.cookieSessionRecordFromValidatedCredential(credential), nil
}

// validateLegacyCookieSession reads cookie_session.* records created before
// cookie sessions moved to typed session.{hmac} runtime credentials.
//
// Deprecated: this exists only to avoid signing users out during the migration
// window. Remove with cookieSessionKeyPrefix after the compatibility cutoff.
func (c *ChattoCore) validateLegacyCookieSession(ctx context.Context, userID, sessionID string) (*corev1.CookieSession, error) {
	key := c.cookieSessionKey(userID, sessionID)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, ErrCookieSessionNotFound
		}
		return nil, fmt.Errorf("failed to get cookie session: %w", err)
	}

	var record corev1.CookieSession
	if err := proto.Unmarshal(entry.Value(), &record); err != nil {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return nil, ErrCookieSessionNotFound
	}
	if record.GetUserId() != userID {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return nil, ErrCookieSessionNotFound
	}
	if record.GetCreatedAt() == nil {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return nil, ErrCookieSessionNotFound
	}
	expiresAtPB := record.GetExpiresAt()
	if expiresAtPB == nil || !time.Now().Before(expiresAtPB.AsTime()) {
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return nil, ErrCookieSessionNotFound
	}
	validation, err := c.ValidateRuntimeCredential(ctx, RuntimeCredential{
		UserID:         userID,
		CreatedAt:      record.GetCreatedAt().AsTime(),
		AuthGeneration: record.GetAuthGeneration(),
	})
	if err != nil {
		if !errors.Is(err, ErrAuthenticationRevoked) {
			return nil, err
		}
		_ = c.storage.runtimeStateKV.Delete(ctx, key)
		return nil, ErrCookieSessionNotFound
	}
	if validation.ShouldPersistAuthGeneration {
		record.AuthGeneration = validation.AuthGeneration
		if data, err := proto.Marshal(&record); err == nil {
			_, _ = c.updateRuntimeStateTokenTTL(ctx, key, data, entry.Revision(), time.Until(expiresAtPB.AsTime()))
		}
	}

	return &record, nil
}

func (c *ChattoCore) cookieSessionRecordFromAuthTokenData(tokenData AuthTokenData) *corev1.CookieSession {
	return c.cookieSessionRecordFromValidatedCredential(validatedRuntimeCredentialFromAuthToken("", tokenData))
}

func (c *ChattoCore) cookieSessionRecordFromValidatedCredential(credential ValidatedRuntimeCredential) *corev1.CookieSession {
	record := &corev1.CookieSession{
		UserId:         credential.UserID,
		CreatedAt:      timestamppb.New(credential.CreatedAt),
		ExpiresAt:      timestamppb.New(credential.CreatedAt.Add(c.cookieSessionTTL())),
		Source:         credential.Source,
		Request:        credential.Request,
		AuthGeneration: credential.AuthGeneration,
	}
	if !credential.FreshAuthAt.IsZero() {
		record.FreshAuthAt = timestamppb.New(credential.FreshAuthAt)
		record.FreshAuthMethod = credential.FreshAuthMethod
		record.FreshAuthSource = credential.FreshAuthSource
	}
	return record
}

// RevokeCookieSession deletes one cookie session. It is idempotent.
// It deletes both current and deprecated legacy cookie-session storage shapes;
// keep the legacy delete until validateLegacyCookieSession is removed.
func (c *ChattoCore) RevokeCookieSession(ctx context.Context, userID, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	if err := c.storage.runtimeStateKV.Delete(ctx, c.authTokenKey(sessionID)); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to revoke cookie session token: %w", err)
	}
	if userID == "" {
		return nil
	}
	err := c.storage.runtimeStateKV.Delete(ctx, c.cookieSessionKey(userID, sessionID))
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to revoke cookie session: %w", err)
	}
	return nil
}

// RevokeCookieSessionsForUser deletes all cookie sessions for a user. Used by
// password changes/resets and account deletion flows that need immediate
// revocation across browser sessions.
func (c *ChattoCore) RevokeCookieSessionsForUser(ctx context.Context, userID string) (int, error) {
	if userID == "" {
		return 0, nil
	}

	deleted := 0
	tokenLister, err := c.storage.runtimeStateKV.ListKeysFiltered(ctx, authTokenKeyPrefix+"*")
	if err != nil && !errors.Is(err, jetstream.ErrNoKeysFound) {
		return 0, fmt.Errorf("failed to list cookie session tokens: %w", err)
	}
	if err == nil {
		var tokenKeys []string
		for key := range tokenLister.Keys() {
			tokenKeys = append(tokenKeys, key)
		}
		for _, key := range tokenKeys {
			entry, err := c.storage.runtimeStateKV.Get(ctx, key)
			if err != nil {
				if errors.Is(err, jetstream.ErrKeyNotFound) {
					continue
				}
				return deleted, fmt.Errorf("failed to get cookie session token for revoke-all: %w", err)
			}
			var tokenData AuthTokenData
			if err := json.Unmarshal(entry.Value(), &tokenData); err != nil {
				c.logger.Warn("Skipping malformed auth token during cookie session revoke-all", "key", key, "error", err)
				continue
			}
			if tokenData.UserID != userID ||
				tokenData.kindOrDefault() != AuthTokenKindFirstPartySession ||
				tokenData.presentationOrDefault() != AuthTokenPresentationCookie {
				continue
			}
			if err := c.storage.runtimeStateKV.Delete(ctx, key); err != nil {
				if errors.Is(err, jetstream.ErrKeyNotFound) {
					continue
				}
				return deleted, fmt.Errorf("failed to revoke cookie session token: %w", err)
			}
			deleted++
		}
	}

	lister, err := c.storage.runtimeStateKV.ListKeysFiltered(ctx, cookieSessionUserKeyFilter(userID))
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return deleted, nil
		}
		return deleted, fmt.Errorf("failed to list cookie sessions: %w", err)
	}

	var keys []string
	for key := range lister.Keys() {
		keys = append(keys, key)
	}

	for _, key := range keys {
		if err := c.storage.runtimeStateKV.Delete(ctx, key); err != nil {
			if !errors.Is(err, jetstream.ErrKeyNotFound) {
				c.logger.Warn("Failed to revoke cookie session", "key", key, "error", err)
			}
			continue
		}
		deleted++
	}
	return deleted, nil
}

// PublishSessionTerminated publishes a SessionTerminatedEvent for the given user.
// This notifies all of the user's active subscriptions (across tabs/devices) that
// their session has been terminated. The subscription handler closes the stream
// after forwarding this event, tearing down the WebSocket connection server-side.
//
// Reasons: "logout", "admin_boot", "account_deleted"
func (c *ChattoCore) PublishSessionTerminated(ctx context.Context, userID, reason string) error {
	event := newLiveEvent(userID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_SessionTerminated{
			SessionTerminated: &corev1.SessionTerminatedEvent{
				Reason: reason,
			},
		},
	})
	subject := subjects.LiveSyncUserEvent(userID, "session_terminated")
	return c.publishLiveEvent(ctx, subject, event)
}

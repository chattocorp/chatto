package core

import (
	"context"
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

// CreateCookieSession creates a server-side cookie session record in
// RUNTIME_STATE and returns the opaque session ID that should be stored in the
// signed browser cookie.
func (c *ChattoCore) CreateCookieSession(ctx context.Context, userID, source string) (string, *corev1.CookieSession, error) {
	authGeneration, err := c.CurrentAuthGeneration(ctx, userID)
	if err != nil {
		return "", nil, err
	}
	return c.CreateCookieSessionForGeneration(ctx, userID, source, authGeneration)
}

// CreateCookieSessionForGeneration creates a server-side cookie session for an
// authentication that proved credentials against authGeneration.
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
	if err := c.RequireAuthenticationAllowed(ctx, userID, authGeneration); err != nil {
		if !errors.Is(err, ErrAuthenticationRevoked) {
			return "", nil, err
		}
		return "", nil, ErrCookieSessionNotFound
	}

	sessionID := NewCookieSessionID()
	now := time.Now()
	expiresAt := now.Add(c.cookieSessionTTL())

	record := &corev1.CookieSession{
		UserId:         userID,
		CreatedAt:      timestamppb.New(now),
		ExpiresAt:      timestamppb.New(expiresAt),
		Source:         source,
		Request:        auditRequestMetadata(ctx),
		AuthGeneration: authGeneration,
	}
	if !freshAuthAt.IsZero() {
		record.FreshAuthAt = timestamppb.New(freshAuthAt)
		record.FreshAuthMethod = freshAuthMethod
		record.FreshAuthSource = freshAuthSource
	}

	data, err := proto.Marshal(record)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal cookie session: %w", err)
	}

	key := c.cookieSessionKey(userID, sessionID)
	if _, err := c.storage.runtimeStateKV.Create(ctx, key, data, jetstream.KeyTTL(c.cookieSessionTTL())); err != nil {
		return "", nil, fmt.Errorf("failed to store cookie session: %w", err)
	}

	return sessionID, record, nil
}

// ValidateCookieSession validates a cookie-backed server-side session and
// returns its runtime-state record. Callers must still load the current user
// projection before authenticating the request.
func (c *ChattoCore) ValidateCookieSession(ctx context.Context, userID, sessionID string) (*corev1.CookieSession, error) {
	if userID == "" || sessionID == "" {
		return nil, ErrCookieSessionNotFound
	}

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

// RevokeCookieSession deletes one cookie session. It is idempotent.
func (c *ChattoCore) RevokeCookieSession(ctx context.Context, userID, sessionID string) error {
	if userID == "" || sessionID == "" {
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

	lister, err := c.storage.runtimeStateKV.ListKeysFiltered(ctx, cookieSessionUserKeyFilter(userID))
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return 0, nil
		}
		return 0, fmt.Errorf("failed to list cookie sessions: %w", err)
	}

	var keys []string
	for key := range lister.Keys() {
		keys = append(keys, key)
	}

	deleted := 0
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

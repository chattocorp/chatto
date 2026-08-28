package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/live/v1"
	"hmans.de/chatto/internal/pb/chatto/core/runtime_state/v1"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core/subjects"
)

var (
	// ErrCookieSessionNotFound is returned when a cookie session does not exist,
	// has expired, is malformed, or does not belong to the supplied user.
	ErrCookieSessionNotFound = errors.New("cookie session not found")
)

// CookieSessionStoreEntry is the revisioned value loaded by an HTTP session
// store adapter. The revision must be supplied when a request changes or
// conditionally deletes the value so replicas cannot overwrite one another.
type CookieSessionStoreEntry struct {
	Value    []byte
	Revision uint64
}

func (c *ChattoCore) cookieSessionTTL() time.Duration {
	return c.authTokenTTL()
}

// CreateCookieSession creates a first-party runtime credential for same-origin
// cookie presentation and returns the opaque handle that should be stored in the
// signed browser cookie.
func (c *ChattoCore) CreateCookieSession(ctx context.Context, userID, source string) (string, *runtimestatev1.CookieSession, error) {
	authGeneration, err := c.CurrentAuthGeneration(ctx, userID)
	if err != nil {
		return "", nil, err
	}
	return c.CreateCookieSessionForGeneration(ctx, userID, source, authGeneration)
}

// CreateCookieSessionForGeneration creates a first-party cookie-presentation
// runtime credential for an authentication that proved credentials against
// authGeneration.
func (c *ChattoCore) CreateCookieSessionForGeneration(ctx context.Context, userID, source string, authGeneration uint64) (string, *runtimestatev1.CookieSession, error) {
	now := time.Now()
	return c.createCookieSessionForGeneration(ctx, userID, source, authGeneration, now, freshAuthMethodForSource(source), source)
}

func (c *ChattoCore) createCookieSessionForGeneration(ctx context.Context, userID, source string, authGeneration uint64, freshAuthAt time.Time, freshAuthMethod, freshAuthSource string) (string, *runtimestatev1.CookieSession, error) {
	tokenData, err := c.newCookieSessionDataForGeneration(ctx, userID, source, authGeneration, freshAuthAt, freshAuthMethod, freshAuthSource)
	if err != nil {
		return "", nil, err
	}

	sessionID := NewAuthToken()
	data, err := json.Marshal(tokenData)
	if err != nil {
		return "", nil, fmt.Errorf("failed to marshal cookie session: %w", err)
	}
	if err := c.CreateCookieSessionValue(ctx, sessionID, data, time.Now()); err != nil {
		return "", nil, err
	}

	return sessionID, c.cookieSessionRecordFromAuthTokenData(tokenData), nil
}

// NewCookieSessionData validates a browser authentication and returns the
// typed record that a session-store adapter can commit under its own opaque
// token.
func (c *ChattoCore) NewCookieSessionData(ctx context.Context, userID, source string) (AuthTokenData, error) {
	authGeneration, err := c.CurrentAuthGeneration(ctx, userID)
	if err != nil {
		return AuthTokenData{}, err
	}
	return c.NewCookieSessionDataForGeneration(ctx, userID, source, authGeneration)
}

// NewCookieSessionDataForGeneration returns a typed browser-session record for
// an authentication that proved credentials against authGeneration.
func (c *ChattoCore) NewCookieSessionDataForGeneration(ctx context.Context, userID, source string, authGeneration uint64) (AuthTokenData, error) {
	now := time.Now()
	return c.newCookieSessionDataForGeneration(ctx, userID, source, authGeneration, now, freshAuthMethodForSource(source), source)
}

func (c *ChattoCore) newCookieSessionDataForGeneration(ctx context.Context, userID, source string, authGeneration uint64, freshAuthAt time.Time, freshAuthMethod, freshAuthSource string) (AuthTokenData, error) {
	if userID == "" {
		return AuthTokenData{}, ErrCookieSessionNotFound
	}
	if err := c.requireHumanUser(ctx, userID); err != nil {
		if errors.Is(err, ErrHumanAccountRequired) || errors.Is(err, ErrNotFound) {
			return AuthTokenData{}, ErrCookieSessionNotFound
		}
		return AuthTokenData{}, err
	}
	if err := c.RequireAuthenticationAllowed(ctx, userID, authGeneration); err != nil {
		if !errors.Is(err, ErrAuthenticationRevoked) {
			return AuthTokenData{}, err
		}
		return AuthTokenData{}, ErrCookieSessionNotFound
	}

	now := freshAuthAt
	if now.IsZero() {
		now = time.Now()
	}
	tokenData := AuthTokenData{
		UserID:         userID,
		Kind:           AuthTokenKindFirstPartySession,
		Presentation:   AuthTokenPresentationCookie,
		Source:         source,
		Request:        auditRequestMetadata(ctx),
		CreatedAt:      now,
		ExpiresAt:      now.Add(c.cookieSessionTTL()),
		AuthGeneration: authGeneration,
	}
	if !freshAuthAt.IsZero() {
		tokenData.FreshAuthAt = freshAuthAt
		tokenData.FreshAuthMethod = freshAuthMethod
		tokenData.FreshAuthSource = freshAuthSource
	}
	return tokenData, nil
}

// LoadCookieSessionValue returns a well-formed, unexpired cookie-session value
// together with the exact KV revision observed by the caller. Malformed or
// expired records fail closed and are removed with a revision fence.
func (c *ChattoCore) LoadCookieSessionValue(ctx context.Context, sessionID string, now time.Time) (CookieSessionStoreEntry, error) {
	if sessionID == "" {
		return CookieSessionStoreEntry{}, ErrCookieSessionNotFound
	}
	key := c.authTokenKey(sessionID)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
			return CookieSessionStoreEntry{}, ErrCookieSessionNotFound
		}
		return CookieSessionStoreEntry{}, fmt.Errorf("failed to load cookie session: %w", err)
	}
	if _, err := decodeCookieSessionValue(entry.Value(), now); err != nil {
		_ = c.deleteRuntimeStateKey(ctx, key, jetstream.LastRevision(entry.Revision()))
		return CookieSessionStoreEntry{}, ErrCookieSessionNotFound
	}
	return CookieSessionStoreEntry{Value: append([]byte(nil), entry.Value()...), Revision: entry.Revision()}, nil
}

// MigrateLegacyCookieSession adds the explicit expiry required by 0.5 to the
// typed cookie-session record written by 0.4. The opaque handle and authority
// stay unchanged. The expected revision makes concurrent upgrade requests
// converge on one record, and a completed migration is returned without
// extending its expiry again.
//
// Deprecated: remove this 0.4-to-0.5 compatibility bridge in 0.6.
func (c *ChattoCore) MigrateLegacyCookieSession(ctx context.Context, sessionID string, now time.Time) (*runtimestatev1.CookieSession, error) {
	if sessionID == "" {
		return nil, ErrCookieSessionNotFound
	}
	ttl := c.cookieSessionTTL()
	if ttl <= 0 {
		return nil, fmt.Errorf("cookie session TTL must be positive")
	}

	key := c.authTokenKey(sessionID)
	for attempt := 0; attempt < 8; attempt++ {
		entry, err := c.storage.runtimeStateKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
				return nil, ErrCookieSessionNotFound
			}
			return nil, fmt.Errorf("failed to get legacy cookie session: %w", err)
		}

		var tokenData AuthTokenData
		if err := json.Unmarshal(entry.Value(), &tokenData); err != nil ||
			tokenData.UserID == "" ||
			tokenData.kindOrDefault() != AuthTokenKindFirstPartySession ||
			tokenData.presentationOrDefault() != AuthTokenPresentationCookie ||
			tokenData.CreatedAt.IsZero() {
			_ = c.deleteRuntimeStateKey(ctx, key, jetstream.LastRevision(entry.Revision()))
			return nil, ErrCookieSessionNotFound
		}
		if !tokenData.ExpiresAt.IsZero() && !now.Before(tokenData.ExpiresAt) {
			_ = c.deleteRuntimeStateKey(ctx, key, jetstream.LastRevision(entry.Revision()))
			return nil, ErrCookieSessionNotFound
		}
		if err := c.requireHumanUser(ctx, tokenData.UserID); err != nil {
			if errors.Is(err, ErrHumanAccountRequired) || errors.Is(err, ErrNotFound) || errors.Is(err, ErrNotAuthenticated) {
				_ = c.deleteRuntimeStateKey(ctx, key, jetstream.LastRevision(entry.Revision()))
				return nil, ErrCookieSessionNotFound
			}
			return nil, err
		}

		validation, err := c.ValidateRuntimeCredential(ctx, RuntimeCredential{
			UserID:         tokenData.UserID,
			CreatedAt:      tokenData.CreatedAt,
			AuthGeneration: tokenData.AuthGeneration,
		})
		if err != nil {
			if errors.Is(err, ErrAuthenticationRevoked) {
				_ = c.deleteRuntimeStateKey(ctx, key, jetstream.LastRevision(entry.Revision()))
				return nil, ErrCookieSessionNotFound
			}
			return nil, err
		}

		changed := false
		if validation.ShouldPersistAuthGeneration {
			tokenData.AuthGeneration = validation.AuthGeneration
			changed = true
		}
		if tokenData.ExpiresAt.IsZero() {
			tokenData.ExpiresAt = now.Add(ttl)
			changed = true
		}
		if !changed {
			return c.cookieSessionRecordFromAuthTokenData(tokenData), nil
		}

		value, err := json.Marshal(tokenData)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal migrated cookie session: %w", err)
		}
		if _, err := c.updateRuntimeStateUntil(ctx, key, value, entry.Revision(), tokenData.ExpiresAt, now); err != nil {
			if isRuntimeStateRevisionConflict(err) {
				continue
			}
			return nil, fmt.Errorf("failed to migrate legacy cookie session: %w", err)
		}
		return c.cookieSessionRecordFromAuthTokenData(tokenData), nil
	}
	return nil, fmt.Errorf("migrate legacy cookie session: too much contention")
}

// CreateCookieSessionValue commits a new opaque cookie session with physical
// retention bounded by the record's authoritative expiry.
func (c *ChattoCore) CreateCookieSessionValue(ctx context.Context, sessionID string, value []byte, now time.Time) error {
	tokenData, err := decodeCookieSessionValue(value, now)
	if err != nil {
		return err
	}
	if _, err := c.storage.runtimeStateKV.Create(ctx, c.authTokenKey(sessionID), value, jetstream.KeyTTL(tokenData.ExpiresAt.Sub(now))); err != nil {
		return fmt.Errorf("failed to store cookie session: %w", err)
	}
	return nil
}

// UpdateCookieSessionValue changes one cookie session only when the caller
// still owns the revision it loaded. The updated message receives a fresh
// per-message TTL that matches the authoritative expiry in the value.
func (c *ChattoCore) UpdateCookieSessionValue(ctx context.Context, sessionID string, value []byte, revision uint64, now time.Time) error {
	tokenData, err := decodeCookieSessionValue(value, now)
	if err != nil {
		return err
	}
	if _, err := c.updateRuntimeStateUntil(ctx, c.authTokenKey(sessionID), value, revision, tokenData.ExpiresAt, now); err != nil {
		return fmt.Errorf("failed to update cookie session: %w", err)
	}
	return nil
}

// DeleteCookieSessionValue removes a cookie session only if it still has the
// revision loaded by the request. This is intended for conditional cleanup;
// explicit logout uses RevokeCookieSession so it fences every earlier writer.
func (c *ChattoCore) DeleteCookieSessionValue(ctx context.Context, sessionID string, revision uint64) error {
	if sessionID == "" {
		return nil
	}
	return c.deleteRuntimeStateKey(ctx, c.authTokenKey(sessionID), jetstream.LastRevision(revision))
}

func decodeCookieSessionValue(value []byte, now time.Time) (AuthTokenData, error) {
	var tokenData AuthTokenData
	if err := json.Unmarshal(value, &tokenData); err != nil ||
		tokenData.UserID == "" ||
		tokenData.kindOrDefault() != AuthTokenKindFirstPartySession ||
		tokenData.presentationOrDefault() != AuthTokenPresentationCookie ||
		tokenData.CreatedAt.IsZero() ||
		tokenData.ExpiresAt.IsZero() ||
		!now.Before(tokenData.ExpiresAt) {
		return AuthTokenData{}, ErrCookieSessionNotFound
	}
	return tokenData, nil
}

// ValidateCookieCredential validates a typed cookie-presentation runtime
// credential. The credential carries its user ID in the runtime-state record,
// so callers do not need to trust or duplicate a user ID in the signed browser
// cookie.
func (c *ChattoCore) ValidateCookieCredential(ctx context.Context, sessionID string) (*runtimestatev1.CookieSession, error) {
	if sessionID == "" {
		return nil, ErrCookieSessionNotFound
	}
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

	return c.cookieSessionRecordFromValidatedCredential(credential), nil
}

// RenewCookieSession advances one cookie session's expiry in place when its
// current window is in the final quarter. The expected KV revision serializes
// renewal with other replicas and makes an unrevisioned logout delete fence a
// concurrent renewal without changing the browser's opaque handle.
func (c *ChattoCore) RenewCookieSession(ctx context.Context, sessionID string, now time.Time) (*runtimestatev1.CookieSession, bool, error) {
	if sessionID == "" {
		return nil, false, ErrCookieSessionNotFound
	}
	ttl := c.cookieSessionTTL()
	if ttl <= 0 {
		return nil, false, fmt.Errorf("cookie session TTL must be positive")
	}

	key := c.authTokenKey(sessionID)
	for attempt := 0; attempt < 8; attempt++ {
		entry, err := c.storage.runtimeStateKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
				return nil, false, ErrCookieSessionNotFound
			}
			return nil, false, fmt.Errorf("failed to get cookie session for renewal: %w", err)
		}

		var tokenData AuthTokenData
		if err := json.Unmarshal(entry.Value(), &tokenData); err != nil ||
			tokenData.UserID == "" ||
			tokenData.kindOrDefault() != AuthTokenKindFirstPartySession ||
			tokenData.presentationOrDefault() != AuthTokenPresentationCookie ||
			tokenData.CreatedAt.IsZero() ||
			tokenData.ExpiresAt.IsZero() {
			_ = c.deleteRuntimeStateKey(ctx, key, jetstream.LastRevision(entry.Revision()))
			return nil, false, ErrCookieSessionNotFound
		}
		if !now.Before(tokenData.ExpiresAt) {
			_ = c.deleteRuntimeStateKey(ctx, key, jetstream.LastRevision(entry.Revision()))
			return nil, false, ErrCookieSessionNotFound
		}

		validation, err := c.ValidateRuntimeCredential(ctx, RuntimeCredential{
			UserID:         tokenData.UserID,
			CreatedAt:      tokenData.CreatedAt,
			AuthGeneration: tokenData.AuthGeneration,
		})
		if err != nil {
			if errors.Is(err, ErrAuthenticationRevoked) {
				_ = c.deleteRuntimeStateKey(ctx, key, jetstream.LastRevision(entry.Revision()))
				return nil, false, ErrCookieSessionNotFound
			}
			return nil, false, err
		}
		if validation.ShouldPersistAuthGeneration {
			tokenData.AuthGeneration = validation.AuthGeneration
		}

		if tokenData.ExpiresAt.Sub(now) > ttl/4 {
			return c.cookieSessionRecordFromAuthTokenData(tokenData), false, nil
		}

		tokenData.ExpiresAt = now.Add(ttl)
		value, err := json.Marshal(tokenData)
		if err != nil {
			return nil, false, fmt.Errorf("failed to marshal renewed cookie session: %w", err)
		}
		if _, err := c.updateRuntimeStateUntil(ctx, key, value, entry.Revision(), tokenData.ExpiresAt, now); err != nil {
			if isRuntimeStateRevisionConflict(err) {
				continue
			}
			return nil, false, fmt.Errorf("failed to renew cookie session: %w", err)
		}
		return c.cookieSessionRecordFromAuthTokenData(tokenData), true, nil
	}
	return nil, false, fmt.Errorf("renew cookie session: too much contention")
}

func (c *ChattoCore) cookieSessionRecordFromAuthTokenData(tokenData AuthTokenData) *runtimestatev1.CookieSession {
	return c.cookieSessionRecordFromValidatedCredential(validatedRuntimeCredentialFromAuthToken("", tokenData))
}

func (c *ChattoCore) cookieSessionRecordFromValidatedCredential(credential ValidatedRuntimeCredential) *runtimestatev1.CookieSession {
	record := &runtimestatev1.CookieSession{
		UserId:         credential.UserID,
		CreatedAt:      timestamppb.New(credential.CreatedAt),
		ExpiresAt:      timestamppb.New(credential.ExpiresAt),
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

// RevokeCookieSession deletes one typed cookie session. It is idempotent.
func (c *ChattoCore) RevokeCookieSession(ctx context.Context, sessionID string) error {
	if sessionID == "" {
		return nil
	}
	if err := c.deleteRuntimeStateKey(ctx, c.authTokenKey(sessionID)); err != nil {
		return fmt.Errorf("failed to revoke cookie session token: %w", err)
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
			if err := c.deleteRuntimeStateKey(ctx, key); err != nil {
				return deleted, fmt.Errorf("failed to revoke cookie session token: %w", err)
			}
			deleted++
		}
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
	event := newLiveEvent(userID, &livev1.LiveEvent{
		Event: &livev1.LiveEvent_SessionTerminated{
			SessionTerminated: &livev1.SessionTerminatedEvent{
				Reason: reason,
			},
		},
	})
	subject := subjects.LiveSyncUserEvent(userID, "session_terminated")
	return c.publishLiveEvent(ctx, subject, event)
}

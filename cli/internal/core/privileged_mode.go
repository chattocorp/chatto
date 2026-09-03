package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// PrivilegedModeWindow is the fixed lifetime of one explicit activation.
// Ordinary use does not extend the deadline.
const PrivilegedModeWindow = 15 * time.Minute

// SetBearerPrivilegedMode activates or deactivates privileged mode on the
// renewable session behind one current public bearer token. Activation is
// idempotent and does not extend an activation that is already current.
func (c *ChattoCore) SetBearerPrivilegedMode(ctx context.Context, token string, active bool) (time.Time, error) {
	credential, err := c.ValidatePublicBearerCredential(ctx, token)
	if err != nil {
		return time.Time{}, err
	}
	if credential.RenewableSessionID == "" {
		return time.Time{}, ErrAuthTokenNotFound
	}
	return c.setRenewableSessionPrivilegedMode(ctx, credential.RenewableSessionID, active, time.Now())
}

func (c *ChattoCore) setRenewableSessionPrivilegedMode(ctx context.Context, sessionID string, active bool, now time.Time) (time.Time, error) {
	for attempt := 0; attempt < 8; attempt++ {
		session, entry, err := c.validateRenewableSession(ctx, sessionID, now)
		if err != nil {
			if errors.Is(err, ErrRefreshTokenNotFound) {
				return time.Time{}, ErrAuthTokenNotFound
			}
			return time.Time{}, err
		}
		if active && now.Before(session.PrivilegedModeExpiresAt) {
			return session.PrivilegedModeExpiresAt, nil
		}
		wasActive := now.Before(session.PrivilegedModeExpiresAt)
		deadline := time.Time{}
		if active {
			deadline = now.Add(PrivilegedModeWindow)
			if session.ExpiresAt.Before(deadline) {
				deadline = session.ExpiresAt
			}
		}
		session.PrivilegedModeExpiresAt = deadline
		value, err := json.Marshal(session)
		if err != nil {
			return time.Time{}, fmt.Errorf("marshal privileged renewable session: %w", err)
		}
		if _, err := c.updateRuntimeStateUntil(ctx, c.renewableSessionKey(sessionID), value, entry.Revision(), session.ExpiresAt, now); err != nil {
			if isRuntimeStateRevisionConflict(err) {
				continue
			}
			return time.Time{}, fmt.Errorf("set renewable-session privileged mode: %w", err)
		}
		if active {
			if err := c.recordPrivilegedModeActivated(ctx, session.UserID, deadline); err != nil {
				c.logger.Warn("Failed to append privileged-mode activation audit event", "error", err)
			}
		} else if wasActive {
			if err := c.recordPrivilegedModeDeactivated(ctx, session.UserID); err != nil {
				c.logger.Warn("Failed to append privileged-mode deactivation audit event", "error", err)
			}
		}
		return deadline, nil
	}
	return time.Time{}, fmt.Errorf("set renewable-session privileged mode: too much contention")
}

// SetCookiePrivilegedMode activates or deactivates privileged mode on one
// same-origin browser session. Activation is idempotent and does not slide.
func (c *ChattoCore) SetCookiePrivilegedMode(ctx context.Context, sessionID string, active bool) (time.Time, error) {
	if sessionID == "" {
		return time.Time{}, ErrCookieSessionNotFound
	}
	key := c.authTokenKey(sessionID)
	for attempt := 0; attempt < 8; attempt++ {
		entry, err := c.storage.runtimeStateKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted) {
				return time.Time{}, ErrCookieSessionNotFound
			}
			return time.Time{}, fmt.Errorf("get cookie session for privileged mode: %w", err)
		}
		var tokenData AuthTokenData
		if err := json.Unmarshal(entry.Value(), &tokenData); err != nil ||
			tokenData.UserID == "" ||
			tokenData.kindOrDefault() != AuthTokenKindFirstPartySession ||
			tokenData.presentationOrDefault() != AuthTokenPresentationCookie ||
			tokenData.CreatedAt.IsZero() {
			return time.Time{}, ErrCookieSessionNotFound
		}
		now := time.Now()
		if tokenData.ExpiresAt.IsZero() || !now.Before(tokenData.ExpiresAt) {
			return time.Time{}, ErrCookieSessionNotFound
		}
		validation, err := c.ValidateRuntimeCredential(ctx, RuntimeCredential{
			UserID:         tokenData.UserID,
			CreatedAt:      tokenData.CreatedAt,
			AuthGeneration: tokenData.AuthGeneration,
		})
		if err != nil {
			if errors.Is(err, ErrAuthenticationRevoked) {
				_ = c.deleteRuntimeStateKey(ctx, key, jetstream.LastRevision(entry.Revision()))
				return time.Time{}, ErrCookieSessionNotFound
			}
			return time.Time{}, err
		}
		if validation.ShouldPersistAuthGeneration {
			tokenData.AuthGeneration = validation.AuthGeneration
		}
		if active && now.Before(tokenData.PrivilegedModeExpiresAt) {
			return tokenData.PrivilegedModeExpiresAt, nil
		}
		wasActive := now.Before(tokenData.PrivilegedModeExpiresAt)
		deadline := time.Time{}
		if active {
			deadline = now.Add(PrivilegedModeWindow)
			if tokenData.ExpiresAt.Before(deadline) {
				deadline = tokenData.ExpiresAt
			}
		}
		tokenData.PrivilegedModeExpiresAt = deadline
		value, err := json.Marshal(tokenData)
		if err != nil {
			return time.Time{}, fmt.Errorf("marshal privileged cookie session: %w", err)
		}
		if _, err := c.updateRuntimeStateUntil(ctx, key, value, entry.Revision(), tokenData.ExpiresAt, now); err != nil {
			if isRuntimeStateRevisionConflict(err) {
				continue
			}
			return time.Time{}, fmt.Errorf("set cookie-session privileged mode: %w", err)
		}
		if active {
			if err := c.recordPrivilegedModeActivated(ctx, tokenData.UserID, deadline); err != nil {
				c.logger.Warn("Failed to append privileged-mode activation audit event", "error", err)
			}
		} else if wasActive {
			if err := c.recordPrivilegedModeDeactivated(ctx, tokenData.UserID); err != nil {
				c.logger.Warn("Failed to append privileged-mode deactivation audit event", "error", err)
			}
		}
		return deadline, nil
	}
	return time.Time{}, fmt.Errorf("set cookie-session privileged mode: too much contention")
}

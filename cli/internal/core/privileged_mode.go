package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/chattocorp/chatto/pkg/jetstreamutil"
	"github.com/nats-io/nats.go/jetstream"
)

// PrivilegedModeWindow is the fixed lifetime of one explicit activation.
// Ordinary use does not extend the deadline.
const PrivilegedModeWindow = 15 * time.Minute

// ProtectedRoles returns the roles the user can select for Shield Mode.
func (c *ChattoCore) ProtectedRoles(userID string) []string {
	return c.rbacModel.rbac.Projection().ProtectedRoles(userID)
}

// SelectPrivilegedRoles validates a session activation against current role
// assignments. The legacy activation command selects all available roles.
// Every subsequent authorization still resolves current grants and denies.
func (c *ChattoCore) SelectPrivilegedRoles(userID string, requested []string, allRoles bool) (available, selected []string, err error) {
	available = c.ProtectedRoles(userID)
	if len(available) == 0 {
		return nil, nil, ErrPrivilegedModeUnavailable
	}
	if allRoles {
		return available, available, nil
	}
	if len(requested) == 0 {
		return nil, nil, fmt.Errorf("%w: select at least one protected role", ErrInvalidArgument)
	}
	selected = slices.Clone(requested)
	slices.Sort(selected)
	selected = slices.Compact(selected)
	for _, name := range selected {
		if _, ok := slices.BinarySearch(available, name); !ok {
			return nil, nil, ErrPermissionDenied
		}
	}
	return available, selected, nil
}

// SetBearerPrivilegedMode activates or deactivates privileged mode on the
// renewable session behind one current public bearer token. Activation is
// idempotent and does not extend an activation that is already current.
func (c *ChattoCore) SetBearerPrivilegedMode(ctx context.Context, token string, active bool, roles ...string) (time.Time, error) {
	credential, err := c.ValidatePublicBearerCredential(ctx, token)
	if err != nil {
		return time.Time{}, err
	}
	if credential.RenewableSessionID == "" {
		return time.Time{}, ErrAuthTokenNotFound
	}
	return c.setRenewableSessionPrivilegedMode(ctx, credential.RenewableSessionID, active, time.Now(), roles...)
}

func (c *ChattoCore) setRenewableSessionPrivilegedMode(ctx context.Context, sessionID string, active bool, now time.Time, roles ...string) (time.Time, error) {
	for range 8 {
		session, entry, err := c.validateRenewableSession(ctx, sessionID, now)
		if err != nil {
			if errors.Is(err, ErrRefreshTokenNotFound) {
				return time.Time{}, ErrAuthTokenNotFound
			}
			return time.Time{}, err
		}
		if active && now.Before(session.PrivilegedModeExpiresAt) {
			if !slices.Equal(session.PrivilegedRoles, roles) {
				return time.Time{}, fmt.Errorf("%w: deactivate before changing selected roles", ErrInvalidArgument)
			}
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
		session.PrivilegedRoles = roles
		session.PrivilegedModeExpiresAt = deadline
		value, err := json.Marshal(session)
		if err != nil {
			return time.Time{}, fmt.Errorf("marshal privileged renewable session: %w", err)
		}
		if _, err := c.updateRuntimeStateUntil(ctx, c.renewableSessionKey(sessionID), value, entry.Revision(), session.ExpiresAt, now); err != nil {
			if jetstreamutil.IsSequenceConflict(err) {
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
func (c *ChattoCore) SetCookiePrivilegedMode(ctx context.Context, sessionID string, active bool, roles ...string) (time.Time, error) {
	if sessionID == "" {
		return time.Time{}, ErrCookieSessionNotFound
	}
	key := c.authTokenKey(sessionID)
	for range 8 {
		entry, err := c.storage.runtimeStateKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
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
		if err := c.RequireAuthenticationAllowed(ctx, tokenData.UserID, tokenData.AuthGeneration); err != nil {
			if errors.Is(err, ErrAuthenticationRevoked) {
				_ = c.deleteRuntimeStateKey(ctx, key, jetstream.LastRevision(entry.Revision()))
				return time.Time{}, ErrCookieSessionNotFound
			}
			return time.Time{}, err
		}
		if active && now.Before(tokenData.PrivilegedModeExpiresAt) {
			if !slices.Equal(tokenData.PrivilegedRoles, roles) {
				return time.Time{}, fmt.Errorf("%w: deactivate before changing selected roles", ErrInvalidArgument)
			}
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
		tokenData.PrivilegedRoles = roles
		tokenData.PrivilegedModeExpiresAt = deadline
		value, err := json.Marshal(tokenData)
		if err != nil {
			return time.Time{}, fmt.Errorf("marshal privileged cookie session: %w", err)
		}
		if _, err := c.updateRuntimeStateUntil(ctx, key, value, entry.Revision(), tokenData.ExpiresAt, now); err != nil {
			if jetstreamutil.IsSequenceConflict(err) {
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

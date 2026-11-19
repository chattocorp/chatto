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

// PrivilegedModeAction distinguishes an explicit role selection from the legacy
// command to activate all eligible roles.
type PrivilegedModeAction int

const (
	DeactivatePrivilegedMode PrivilegedModeAction = iota
	ActivateAllPrivilegedRoles
	ActivateSelectedPrivilegedRoles
)

// PrivilegedModeActivation is the selection and deadline from one session revision.
type PrivilegedModeActivation struct {
	Roles     []string
	ExpiresAt time.Time
}

// nextActivation runs against the session revision used for the write. A legacy
// retry keeps that revision's selection even if the user's assignments changed.
func (c *ChattoCore) nextActivation(userID string, action PrivilegedModeAction, requested, stored []string, deadline, sessionEnd, now time.Time) (PrivilegedModeActivation, error) {
	if action == DeactivatePrivilegedMode {
		return PrivilegedModeActivation{}, nil
	}
	current := PrivilegedModeActivation{Roles: stored, ExpiresAt: deadline}
	active := now.Before(deadline)
	if active && action == ActivateAllPrivilegedRoles {
		return current, nil
	}
	available := c.ProtectedRoles(userID)
	if len(available) == 0 {
		return PrivilegedModeActivation{}, ErrPrivilegedModeUnavailable
	}
	var selected []string
	switch action {
	case ActivateAllPrivilegedRoles:
		selected = available
	case ActivateSelectedPrivilegedRoles:
		if len(requested) == 0 {
			return PrivilegedModeActivation{}, fmt.Errorf("%w: select at least one protected role", ErrInvalidArgument)
		}
		selected = slices.Clone(requested)
		slices.Sort(selected)
		selected = slices.Compact(selected)
		for _, name := range selected {
			if _, ok := slices.BinarySearch(available, name); !ok {
				return PrivilegedModeActivation{}, ErrPermissionDenied
			}
		}
	default:
		return PrivilegedModeActivation{}, ErrInvalidArgument
	}
	if active {
		if !slices.Equal(stored, selected) {
			return PrivilegedModeActivation{}, fmt.Errorf("%w: deactivate before changing selected roles", ErrInvalidArgument)
		}
		return current, nil
	}
	deadline = now.Add(PrivilegedModeWindow)
	if sessionEnd.Before(deadline) {
		deadline = sessionEnd
	}
	return PrivilegedModeActivation{Roles: selected, ExpiresAt: deadline}, nil
}

// SetBearerPrivilegedMode activates or deactivates privileged mode on the
// renewable session behind one current public bearer token. Activation is
// idempotent and does not extend an activation that is already current.
func (c *ChattoCore) SetBearerPrivilegedMode(ctx context.Context, token string, action PrivilegedModeAction, roles ...string) (PrivilegedModeActivation, error) {
	credential, err := c.ValidatePublicBearerCredential(ctx, token)
	if err != nil {
		return PrivilegedModeActivation{}, err
	}
	if credential.RenewableSessionID == "" {
		return PrivilegedModeActivation{}, ErrAuthTokenNotFound
	}
	return c.setRenewableSessionPrivilegedMode(ctx, credential.RenewableSessionID, action, roles...)
}

func (c *ChattoCore) setRenewableSessionPrivilegedMode(ctx context.Context, sessionID string, action PrivilegedModeAction, roles ...string) (PrivilegedModeActivation, error) {
	for range 8 {
		now := time.Now()
		session, entry, err := c.validateRenewableSession(ctx, sessionID, now)
		if err != nil {
			if errors.Is(err, ErrRefreshTokenNotFound) {
				return PrivilegedModeActivation{}, ErrAuthTokenNotFound
			}
			return PrivilegedModeActivation{}, err
		}
		wasActive := now.Before(session.PrivilegedModeExpiresAt)
		next, err := c.nextActivation(session.UserID, action, roles, session.PrivilegedRoles, session.PrivilegedModeExpiresAt, session.ExpiresAt, now)
		if err != nil {
			return PrivilegedModeActivation{}, err
		}
		if wasActive && action != DeactivatePrivilegedMode {
			return next, nil
		}
		session.PrivilegedRoles = next.Roles
		session.PrivilegedModeExpiresAt = next.ExpiresAt
		value, err := json.Marshal(session)
		if err != nil {
			return PrivilegedModeActivation{}, fmt.Errorf("marshal privileged renewable session: %w", err)
		}
		if _, err := c.updateRuntimeStateUntil(ctx, c.renewableSessionKey(sessionID), value, entry.Revision(), session.ExpiresAt, now); err != nil {
			if jetstreamutil.IsSequenceConflict(err) {
				continue
			}
			return PrivilegedModeActivation{}, fmt.Errorf("set renewable-session privileged mode: %w", err)
		}
		if action != DeactivatePrivilegedMode {
			if err := c.recordPrivilegedModeActivated(ctx, session.UserID, next.ExpiresAt); err != nil {
				c.logger.Warn("Failed to append privileged-mode activation audit event", "error", err)
			}
		} else if wasActive {
			if err := c.recordPrivilegedModeDeactivated(ctx, session.UserID); err != nil {
				c.logger.Warn("Failed to append privileged-mode deactivation audit event", "error", err)
			}
		}
		return next, nil
	}
	return PrivilegedModeActivation{}, fmt.Errorf("set renewable-session privileged mode: too much contention")
}

// SetCookiePrivilegedMode activates or deactivates privileged mode on one
// same-origin browser session. Activation is idempotent and does not slide.
func (c *ChattoCore) SetCookiePrivilegedMode(ctx context.Context, sessionID string, action PrivilegedModeAction, roles ...string) (PrivilegedModeActivation, error) {
	if sessionID == "" {
		return PrivilegedModeActivation{}, ErrCookieSessionNotFound
	}
	key := c.authTokenKey(sessionID)
	for range 8 {
		entry, err := c.storage.runtimeStateKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				return PrivilegedModeActivation{}, ErrCookieSessionNotFound
			}
			return PrivilegedModeActivation{}, fmt.Errorf("get cookie session for privileged mode: %w", err)
		}
		var tokenData AuthTokenData
		if err := json.Unmarshal(entry.Value(), &tokenData); err != nil ||
			tokenData.UserID == "" ||
			tokenData.kindOrDefault() != AuthTokenKindFirstPartySession ||
			tokenData.presentationOrDefault() != AuthTokenPresentationCookie ||
			tokenData.CreatedAt.IsZero() {
			return PrivilegedModeActivation{}, ErrCookieSessionNotFound
		}
		now := time.Now()
		if tokenData.ExpiresAt.IsZero() || !now.Before(tokenData.ExpiresAt) {
			return PrivilegedModeActivation{}, ErrCookieSessionNotFound
		}
		if err := c.RequireAuthenticationAllowed(ctx, tokenData.UserID, tokenData.AuthGeneration); err != nil {
			if errors.Is(err, ErrAuthenticationRevoked) {
				_ = c.deleteRuntimeStateKey(ctx, key, jetstream.LastRevision(entry.Revision()))
				return PrivilegedModeActivation{}, ErrCookieSessionNotFound
			}
			return PrivilegedModeActivation{}, err
		}
		wasActive := now.Before(tokenData.PrivilegedModeExpiresAt)
		next, err := c.nextActivation(tokenData.UserID, action, roles, tokenData.PrivilegedRoles, tokenData.PrivilegedModeExpiresAt, tokenData.ExpiresAt, now)
		if err != nil {
			return PrivilegedModeActivation{}, err
		}
		if wasActive && action != DeactivatePrivilegedMode {
			return next, nil
		}
		tokenData.PrivilegedRoles = next.Roles
		tokenData.PrivilegedModeExpiresAt = next.ExpiresAt
		value, err := json.Marshal(tokenData)
		if err != nil {
			return PrivilegedModeActivation{}, fmt.Errorf("marshal privileged cookie session: %w", err)
		}
		if _, err := c.updateRuntimeStateUntil(ctx, key, value, entry.Revision(), tokenData.ExpiresAt, now); err != nil {
			if jetstreamutil.IsSequenceConflict(err) {
				continue
			}
			return PrivilegedModeActivation{}, fmt.Errorf("set cookie-session privileged mode: %w", err)
		}
		if action != DeactivatePrivilegedMode {
			if err := c.recordPrivilegedModeActivated(ctx, tokenData.UserID, next.ExpiresAt); err != nil {
				c.logger.Warn("Failed to append privileged-mode activation audit event", "error", err)
			}
		} else if wasActive {
			if err := c.recordPrivilegedModeDeactivated(ctx, tokenData.UserID); err != nil {
				c.logger.Warn("Failed to append privileged-mode deactivation audit event", "error", err)
			}
		}
		return next, nil
	}
	return PrivilegedModeActivation{}, fmt.Errorf("set cookie-session privileged mode: too much contention")
}

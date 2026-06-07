package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hmans.de/chatto/internal/events"
)

var ErrAuthenticationRevoked = errors.New("authentication revoked")

// RuntimeCredentialRevocationResult reports how many runtime credential records
// were deleted during best-effort cleanup after a generation has been advanced.
type RuntimeCredentialRevocationResult struct {
	CookieSessions int
	AuthTokens     int
}

// RevokeRuntimeCredentialsForUser deletes currently stored runtime credentials
// for a user. The auth generation is the revocation guarantee; this scan is cleanup.
func (c *ChattoCore) RevokeRuntimeCredentialsForUser(ctx context.Context, userID, reason string) (RuntimeCredentialRevocationResult, error) {
	var result RuntimeCredentialRevocationResult

	cookieSessions, err := c.RevokeCookieSessionsForUser(ctx, userID)
	if err != nil {
		return result, err
	}
	result.CookieSessions = cookieSessions

	authTokens, err := c.RevokeAllAuthTokensForUserWithReason(ctx, userID, reason)
	if err != nil {
		return result, err
	}
	result.AuthTokens = authTokens

	return result, nil
}

func (c *ChattoCore) CurrentAuthGeneration(ctx context.Context, userID string) (uint64, error) {
	if userID == "" {
		return 0, nil
	}
	if err := c.waitForUserAuthGenerationCurrent(ctx, userID); err != nil {
		return 0, err
	}
	generation, active := c.Users.AuthGeneration(userID)
	if !active {
		return 0, ErrAuthenticationRevoked
	}
	return generation, nil
}

// RequireAuthenticationAllowed rejects credential flows that authenticated
// against an older user auth generation.
func (c *ChattoCore) RequireAuthenticationAllowed(ctx context.Context, userID string, authGeneration uint64) error {
	currentGeneration, err := c.CurrentAuthGeneration(ctx, userID)
	if err != nil {
		return err
	}
	if authGeneration != currentGeneration {
		return ErrAuthenticationRevoked
	}
	return nil
}

// ResolveCredentialAuthGeneration validates a stored credential generation and
// returns the generation that should be retained on the credential.
//
// Credentials written before auth_generation existed unmarshal as generation 0.
// For compatibility, those records are grandfathered when their CreatedAt is
// not older than the user's current password hash event, then upgraded in
// place by the caller. Legacy imported password hashes only have the legacy
// user record timestamp, so this intentionally preserves upgraded 0.0.x
// credentials until a new 0.1.x password change/reset advances the generation.
func (c *ChattoCore) ResolveCredentialAuthGeneration(ctx context.Context, userID string, authGeneration uint64, credentialCreatedAt time.Time) (uint64, error) {
	currentGeneration, err := c.CurrentAuthGeneration(ctx, userID)
	if err != nil {
		return 0, err
	}
	if authGeneration == currentGeneration {
		return currentGeneration, nil
	}
	if authGeneration != 0 {
		return 0, ErrAuthenticationRevoked
	}
	if currentGeneration == 0 {
		return 0, nil
	}

	_, passwordSetAt, hasPassword := c.Users.PasswordHashWithSetAt(userID)
	if !hasPassword || credentialCreatedAt.IsZero() || credentialCreatedAt.Before(passwordSetAt) {
		return 0, ErrAuthenticationRevoked
	}
	return currentGeneration, nil
}

func (c *ChattoCore) waitForUserAuthGenerationCurrent(ctx context.Context, userID string) error {
	if c.EventPublisher == nil || c.UsersProjector == nil {
		return nil
	}
	agg := events.UserAggregate(userID)
	if err := c.waitForProjectionSubjectsCurrent(ctx, "user auth generation", c.UsersProjector,
		agg.Subject(events.EventUserPasswordHashChanged),
		agg.Subject(events.EventUserAccountDeleted),
	); err != nil {
		return fmt.Errorf("wait for user auth generation: %w", err)
	}
	return nil
}

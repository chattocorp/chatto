package core

import (
	"context"
	"errors"
	"fmt"

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

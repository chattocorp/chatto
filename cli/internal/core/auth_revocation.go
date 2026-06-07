package core

import (
	"context"
)

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

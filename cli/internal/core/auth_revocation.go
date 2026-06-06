package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const authRevokedBeforeKeyPrefix = "auth_revoked_before."

var ErrAuthenticationRevoked = errors.New("authentication revoked")

type authRevocationCutoffSnapshot struct {
	value  []byte
	exists bool
	cutoff time.Time
}

// CredentialRevocation is a pending per-user authentication cutoff. Callers
// begin it before appending a durable credential-change event, Commit after the
// event is durable, and Rollback on failure.
type CredentialRevocation struct {
	core      *ChattoCore
	userID    string
	snapshot  authRevocationCutoffSnapshot
	committed bool
}

// RuntimeCredentialRevocationResult reports how many runtime credential records
// were deleted during best-effort cleanup after a cutoff has been advanced.
type RuntimeCredentialRevocationResult struct {
	CookieSessions int
	AuthTokens     int
}

func authRevokedBeforeKey(userID string) string {
	return authRevokedBeforeKeyPrefix + userID
}

// EstablishCredentialRevocation advances the per-user cutoff immediately. It is
// useful for terminal flows like account deletion where rollback is unnecessary.
func (c *ChattoCore) EstablishCredentialRevocation(ctx context.Context, userID string) error {
	if userID == "" {
		return nil
	}
	cutoff := time.Now().UTC()
	if err := c.storeAuthRevocationCutoff(ctx, userID, cutoff); err != nil {
		return err
	}
	return nil
}

// BeginCredentialRevocation advances the per-user cutoff and returns a handle
// that can roll back to the previous cutoff if the durable credential change
// fails to append.
func (c *ChattoCore) BeginCredentialRevocation(ctx context.Context, userID string) (*CredentialRevocation, error) {
	var snapshot authRevocationCutoffSnapshot
	if userID == "" {
		return &CredentialRevocation{committed: true}, nil
	}
	entry, err := c.storage.runtimeStateKV.Get(ctx, authRevokedBeforeKey(userID))
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil, fmt.Errorf("failed to read previous auth revocation cutoff: %w", err)
	}
	if err == nil {
		snapshot.exists = true
		snapshot.value = append([]byte(nil), entry.Value()...)
	}

	snapshot.cutoff = time.Now().UTC()
	if err := c.storeAuthRevocationCutoff(ctx, userID, snapshot.cutoff); err != nil {
		return nil, err
	}
	return &CredentialRevocation{core: c, userID: userID, snapshot: snapshot}, nil
}

// Commit keeps the advanced cutoff. After Commit, Rollback is a no-op.
func (r *CredentialRevocation) Commit() {
	if r == nil {
		return
	}
	r.committed = true
}

// Rollback restores the previous cutoff if this revocation has not committed.
func (r *CredentialRevocation) Rollback(ctx context.Context) {
	if r == nil || r.committed || r.core == nil {
		return
	}
	r.core.rollbackAuthRevocationCutoff(ctx, r.userID, r.snapshot)
	r.committed = true
}

// RevokeRuntimeCredentialsForUser deletes currently stored runtime credentials
// for a user. The cutoff is the revocation guarantee; this scan is cleanup.
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

func (c *ChattoCore) storeAuthRevocationCutoff(ctx context.Context, userID string, cutoff time.Time) error {
	if _, err := c.storage.runtimeStateKV.Put(ctx, authRevokedBeforeKey(userID), []byte(cutoff.UTC().Format(time.RFC3339Nano))); err != nil {
		return fmt.Errorf("failed to store auth revocation cutoff: %w", err)
	}
	return nil
}

func (c *ChattoCore) rollbackAuthRevocationCutoff(ctx context.Context, userID string, snapshot authRevocationCutoffSnapshot) {
	if userID == "" || snapshot.cutoff.IsZero() {
		return
	}
	key := authRevokedBeforeKey(userID)
	entry, err := c.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		return
	}
	if string(entry.Value()) != snapshot.cutoff.Format(time.RFC3339Nano) {
		return
	}
	if snapshot.exists {
		_, _ = c.storage.runtimeStateKV.Put(ctx, key, snapshot.value)
		return
	}
	_ = c.storage.runtimeStateKV.Delete(ctx, key)
}

func (c *ChattoCore) authRevocationCutoff(ctx context.Context, userID string) (time.Time, bool, error) {
	if userID == "" {
		return time.Time{}, false, nil
	}
	entry, err := c.storage.runtimeStateKV.Get(ctx, authRevokedBeforeKey(userID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return time.Time{}, false, nil
		}
		return time.Time{}, false, fmt.Errorf("failed to read auth revocation cutoff: %w", err)
	}
	cutoff, err := time.Parse(time.RFC3339Nano, string(entry.Value()))
	if err != nil {
		return time.Time{}, false, fmt.Errorf("failed to parse auth revocation cutoff: %w", err)
	}
	return cutoff, true, nil
}

// RequireAuthenticationAllowed rejects credential flows that authenticated
// before the user's current revocation cutoff.
func (c *ChattoCore) RequireAuthenticationAllowed(ctx context.Context, userID string, authenticatedAt time.Time) error {
	cutoff, ok, err := c.authRevocationCutoff(ctx, userID)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}
	if authenticatedAt.IsZero() || authenticatedAt.Before(cutoff) {
		return ErrAuthenticationRevoked
	}
	return nil
}

package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

const authRevokedBeforeKeyPrefix = "auth_revoked_before."
const maxAuthRevocationCutoffUpdateAttempts = 8

var ErrAuthenticationRevoked = errors.New("authentication revoked")

type authRevocationCutoffState struct {
	// Cutoffs is keyed by the revocation operation that introduced each cutoff.
	// Successful operations leave their marker behind; failed operations remove
	// only their own marker. Validation uses the maximum value in the set.
	Cutoffs map[string]string `json:"cutoffs,omitempty"`
}

// CredentialRevocation is a pending per-user authentication cutoff. Callers
// begin it before appending a durable credential-change event, Commit after the
// event is durable, and Rollback on failure.
type CredentialRevocation struct {
	core      *ChattoCore
	userID    string
	id        string
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
	if err := c.storeAuthRevocationCutoff(ctx, userID, "established."+NewAuthToken(), time.Now().UTC()); err != nil {
		return err
	}
	return nil
}

// BeginCredentialRevocation adds a per-user cutoff marker and returns a handle
// that can remove that marker if the durable credential change fails to append.
func (c *ChattoCore) BeginCredentialRevocation(ctx context.Context, userID string) (*CredentialRevocation, error) {
	if userID == "" {
		return &CredentialRevocation{committed: true}, nil
	}
	id := "pending." + NewAuthToken()
	if err := c.storeAuthRevocationCutoff(ctx, userID, id, time.Now().UTC()); err != nil {
		return nil, err
	}
	return &CredentialRevocation{core: c, userID: userID, id: id}, nil
}

// Commit keeps the advanced cutoff. After Commit, Rollback is a no-op.
func (r *CredentialRevocation) Commit() {
	if r == nil {
		return
	}
	r.committed = true
}

// Rollback removes this operation's cutoff marker if it has not committed.
func (r *CredentialRevocation) Rollback(ctx context.Context) {
	if r == nil || r.committed || r.core == nil {
		return
	}
	r.core.rollbackAuthRevocationCutoff(ctx, r.userID, r.id)
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

func (c *ChattoCore) storeAuthRevocationCutoff(ctx context.Context, userID, id string, cutoff time.Time) error {
	if userID == "" || id == "" || cutoff.IsZero() {
		return nil
	}
	return c.updateAuthRevocationCutoffs(ctx, userID, func(state *authRevocationCutoffState) {
		state.set(id, cutoff)
	})
}

func (c *ChattoCore) rollbackAuthRevocationCutoff(ctx context.Context, userID, id string) {
	if userID == "" || id == "" {
		return
	}
	_ = c.updateAuthRevocationCutoffs(ctx, userID, func(state *authRevocationCutoffState) {
		delete(state.Cutoffs, id)
	})
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
	state, err := parseAuthRevocationCutoffState(entry.Value())
	if err != nil {
		return time.Time{}, false, fmt.Errorf("failed to parse auth revocation cutoff: %w", err)
	}
	cutoff, ok, err := state.max()
	if err != nil {
		return time.Time{}, false, fmt.Errorf("failed to parse auth revocation cutoff: %w", err)
	}
	return cutoff, ok, nil
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

func (c *ChattoCore) updateAuthRevocationCutoffs(ctx context.Context, userID string, mutate func(*authRevocationCutoffState)) error {
	key := authRevokedBeforeKey(userID)
	for attempt := 0; attempt < maxAuthRevocationCutoffUpdateAttempts; attempt++ {
		entry, err := c.storage.runtimeStateKV.Get(ctx, key)
		if err != nil {
			if !errors.Is(err, jetstream.ErrKeyNotFound) {
				return fmt.Errorf("failed to read auth revocation cutoff: %w", err)
			}

			state := authRevocationCutoffState{}
			mutate(&state)
			if state.empty() {
				return nil
			}
			data, err := state.marshal()
			if err != nil {
				return fmt.Errorf("failed to marshal auth revocation cutoff: %w", err)
			}
			if _, err := c.storage.runtimeStateKV.Create(ctx, key, data); err != nil {
				if errors.Is(err, jetstream.ErrKeyExists) {
					continue
				}
				return fmt.Errorf("failed to store auth revocation cutoff: %w", err)
			}
			return nil
		}

		state, err := parseAuthRevocationCutoffState(entry.Value())
		if err != nil {
			return fmt.Errorf("failed to parse auth revocation cutoff: %w", err)
		}
		mutate(&state)
		if state.empty() {
			if err := c.storage.runtimeStateKV.Delete(ctx, key, jetstream.LastRevision(entry.Revision())); err != nil {
				if errors.Is(err, jetstream.ErrKeyExists) || errors.Is(err, jetstream.ErrKeyNotFound) {
					continue
				}
				return fmt.Errorf("failed to delete auth revocation cutoff: %w", err)
			}
			return nil
		}

		data, err := state.marshal()
		if err != nil {
			return fmt.Errorf("failed to marshal auth revocation cutoff: %w", err)
		}
		if _, err := c.storage.runtimeStateKV.Update(ctx, key, data, entry.Revision()); err != nil {
			if errors.Is(err, jetstream.ErrKeyExists) || errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return fmt.Errorf("failed to store auth revocation cutoff: %w", err)
		}
		return nil
	}
	return fmt.Errorf("failed to update auth revocation cutoff after %d attempts", maxAuthRevocationCutoffUpdateAttempts)
}

func parseAuthRevocationCutoffState(data []byte) (authRevocationCutoffState, error) {
	var state authRevocationCutoffState
	if len(data) == 0 {
		return state, nil
	}

	if data[0] != '{' {
		// Backward compatibility for the original scalar cutoff value.
		cutoff, ok, err := parseAuthRevocationCutoffString(string(data))
		if err != nil {
			return state, err
		}
		if ok {
			state.set("legacy", cutoff)
		}
		return state, nil
	}

	if err := json.Unmarshal(data, &state); err != nil {
		return state, err
	}
	return state, nil
}

func (s *authRevocationCutoffState) set(id string, cutoff time.Time) {
	if id == "" || cutoff.IsZero() {
		return
	}
	if s.Cutoffs == nil {
		s.Cutoffs = map[string]string{}
	}
	existing, ok, err := parseAuthRevocationCutoffString(s.Cutoffs[id])
	if err == nil && ok && !cutoff.After(existing) {
		return
	}
	s.Cutoffs[id] = cutoff.UTC().Format(time.RFC3339Nano)
}

func (s authRevocationCutoffState) max() (time.Time, bool, error) {
	var max time.Time
	for _, raw := range s.Cutoffs {
		cutoff, ok, err := parseAuthRevocationCutoffString(raw)
		if err != nil {
			return time.Time{}, false, err
		}
		if ok && (max.IsZero() || cutoff.After(max)) {
			max = cutoff
		}
	}
	if max.IsZero() {
		return time.Time{}, false, nil
	}
	return max, true, nil
}

func (s authRevocationCutoffState) empty() bool {
	return len(s.Cutoffs) == 0
}

func (s authRevocationCutoffState) marshal() ([]byte, error) {
	if s.empty() {
		return nil, nil
	}
	return json.Marshal(s)
}

func parseAuthRevocationCutoffString(raw string) (time.Time, bool, error) {
	if raw == "" {
		return time.Time{}, false, nil
	}
	cutoff, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, false, err
	}
	return cutoff, true, nil
}

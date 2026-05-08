package core

import (
	"context"
	"fmt"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ServerSpaceID returns the deployment's user-facing space ID, or "" if
// no user-facing space exists yet (fresh install). Cached after the first
// successful resolve via InitServerSpaceID or implicit lookup.
func (c *ChattoCore) ServerSpaceID() string {
	v, _ := c.serverSpaceID.Load().(string)
	return v
}

// SetServerSpaceID records the deployment's user-facing space ID so the
// storage layer can route reads/writes to the SERVER_* buckets. Used by
// the boot path and by tests that exercise server-routed code paths.
func (c *ChattoCore) SetServerSpaceID(spaceID string) {
	c.serverSpaceID.Store(spaceID)
}

// InitServerSpaceID resolves and caches the deployment's user-facing
// space ID. Called once at boot, after any auto-bootstrap has had a
// chance to create the initial space. Errors only on inconsistent state
// (multiple user-facing spaces); a fresh-install zero-space deployment
// caches "" and is not an error.
func (c *ChattoCore) InitServerSpaceID(ctx context.Context) error {
	id, err := c.resolveServerSpaceID(ctx)
	if err != nil {
		return err
	}
	c.SetServerSpaceID(id)
	return nil
}

// resolveServerSpaceID lists all spaces, excludes the DM system space, and
// returns the single remaining ID. Returns ("", nil) if no user-facing
// spaces exist (fresh install). Returns an error if there are 2+ — that
// only happens in tests that intentionally create multiple spaces; those
// tests should call SetServerSpaceID explicitly.
func (c *ChattoCore) resolveServerSpaceID(ctx context.Context) (string, error) {
	spaces, err := c.ListSpaces(ctx)
	if err != nil {
		return "", fmt.Errorf("list spaces: %w", err)
	}
	userFacing := userFacingSpaces(spaces)
	switch len(userFacing) {
	case 0:
		return "", nil
	case 1:
		return userFacing[0].Id, nil
	default:
		ids := make([]string, 0, len(userFacing))
		for _, s := range userFacing {
			ids = append(ids, s.Id)
		}
		return "", fmt.Errorf("multiple user-facing spaces present (%v); deployment expected exactly one", ids)
	}
}

// JoinServer joins the user to the deployment's user-facing space. Used by
// signup flows so a newly-created user is a server member by default.
//
// Best-effort by design: if no server space resolves yet (fresh install)
// or the resolver errors transiently, we log and continue rather than
// failing the signup. JoinSpace is idempotent so retrying is safe.
func (c *ChattoCore) JoinServer(ctx context.Context, userID string) {
	id := c.ServerSpaceID()
	if id == "" {
		// Try a fresh resolve in case the cache is stale (e.g., the first
		// space was created since boot).
		resolved, err := c.resolveServerSpaceID(ctx)
		if err != nil {
			c.logger.Warn("auto-join server skipped: resolver error", "user_id", userID, "error", err)
			return
		}
		if resolved == "" {
			return
		}
		c.SetServerSpaceID(resolved)
		id = resolved
	}
	if _, err := c.JoinSpace(ctx, userID, id); err != nil {
		c.logger.Warn("auto-join server failed", "user_id", userID, "space_id", id, "error", err)
	}
}

func userFacingSpaces(spaces []*corev1.Space) []*corev1.Space {
	out := make([]*corev1.Space, 0, len(spaces))
	for _, s := range spaces {
		if IsDMSpace(s.Id) {
			continue
		}
		out = append(out, s)
	}
	return out
}

package core

import (
	"context"
	"sort"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// FirstUserFacingSpaceID returns the deployment's user-facing space ID if
// one exists, or "" otherwise. Tolerant of multiple stale space records
// (picks the lex-smallest ID for determinism) — designed for the in-flight
// pivot away from the Space concept, where the API still surfaces a space
// ID for URL construction even though the data layer no longer routes by
// it. Will be removed when the Space type retires from the GraphQL API.
func (c *ChattoCore) FirstUserFacingSpaceID(ctx context.Context) (string, error) {
	spaces, err := c.ListSpaces(ctx)
	if err != nil {
		return "", err
	}
	ids := make([]string, 0, len(spaces))
	for _, s := range spaces {
		if IsDMSpace(s.Id) {
			continue
		}
		ids = append(ids, s.Id)
	}
	if len(ids) == 0 {
		return "", nil
	}
	sort.Strings(ids)
	return ids[0], nil
}

// JoinServer auto-joins the user to any rooms in the deployment's
// user-facing space that have auto_join enabled. Best-effort; logs and
// continues on failure. Server "membership" itself is implicit
// post-#330 — every authenticated user counts as a member.
func (c *ChattoCore) JoinServer(ctx context.Context, userID string) {
	id, err := c.FirstUserFacingSpaceID(ctx)
	if err != nil {
		c.logger.Warn("auto-join server skipped: lookup error", "user_id", userID, "error", err)
		return
	}
	if id == "" {
		return
	}
	c.AutoJoinDefaultRooms(ctx, id, userID)
}

// userFacingSpaces returns the spaces that aren't the DM system space.
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

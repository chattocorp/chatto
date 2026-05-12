package core

import "context"

// JoinServer auto-joins the user to any channel rooms that have auto_join
// enabled. Best-effort; logs and continues on failure. Server "membership"
// itself is implicit post-#330 — every authenticated user counts as a
// member.
func (c *ChattoCore) JoinServer(ctx context.Context, userID string) {
	c.AutoJoinDefaultRooms(ctx, ServerSpaceID, userID)
}

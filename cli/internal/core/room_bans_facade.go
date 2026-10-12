package core

import (
	"context"

	"time"
)

// BanMember delegates to the owning RoomMembershipModel.
func (c *ChattoCore) BanMember(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID, reason string, expiresAt *time.Time) (*RoomBan, error) {
	return c.roomMemberships.BanMember(ctx, actorID, kind, roomID, targetUserID, reason, expiresAt)
}

// UnbanMember delegates to the owning RoomMembershipModel.
func (c *ChattoCore) UnbanMember(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID, reason string) error {
	return c.roomMemberships.UnbanMember(ctx, actorID, kind, roomID, targetUserID, reason)
}

// ListActiveRoomBans delegates to the owning RoomMembershipModel.
func (c *ChattoCore) ListActiveRoomBans(ctx context.Context, roomID *string) ([]RoomBan, error) {
	return c.roomMemberships.ListActiveRoomBans(ctx, roomID)
}

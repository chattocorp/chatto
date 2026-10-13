package core

import (
	"context"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

// ListMemberRooms delegates to the owning RoomMembershipModel.
func (c *ChattoCore) ListMemberRooms(ctx context.Context, kind RoomKind, userID string, opts MemberRoomListOptions) ([]*evtv1.Room, error) {
	return c.roomMemberships.ListMemberRooms(ctx, kind, userID, opts)
}

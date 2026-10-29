package core

import (
	"context"

	"github.com/chattocorp/chatto/cli/internal/readmodel"
)

// ListMemberRooms delegates to the owning RoomMembershipModel.
func (c *ChattoCore) ListMemberRooms(ctx context.Context, kind RoomKind, userID string, opts MemberRoomListOptions) ([]*readmodel.Room, error) {
	return c.roomMemberships.ListMemberRooms(ctx, kind, userID, opts)
}

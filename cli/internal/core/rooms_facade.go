package core

import (
	"context"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// DeleteRoom delegates to the owning RoomCommandModel.
func (c *ChattoCore) DeleteRoom(ctx context.Context, actorID string, kind RoomKind, room_id string) error {
	return c.roomCommands.DeleteRoom(ctx, actorID, kind, room_id)
}

// ListMemberRooms delegates to the owning RoomMembershipModel.
func (c *ChattoCore) ListMemberRooms(ctx context.Context, kind RoomKind, userID string, opts MemberRoomListOptions) ([]*evtv1.Room, error) {
	return c.roomMemberships.ListMemberRooms(ctx, kind, userID, opts)
}

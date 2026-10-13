package core

import (
	"context"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

// GetRoomGroup delegates to the owning RoomLayoutModel.
func (c *ChattoCore) GetRoomGroup(ctx context.Context, groupID string) (*evtv1.RoomGroup, error) {
	return c.roomLayout.GetRoomGroup(ctx, groupID)
}

// ListRoomGroupsOrdered delegates to the owning RoomLayoutModel.
func (c *ChattoCore) ListRoomGroupsOrdered(ctx context.Context, kind RoomKind) ([]*evtv1.RoomGroup, error) {
	return c.roomLayout.ListRoomGroupsOrdered(ctx, kind)
}

// ensureChannelRoomsAreInAGroup delegates to the owning RoomLayoutModel.
func (c *ChattoCore) ensureChannelRoomsAreInAGroup(ctx context.Context) error {
	return c.roomLayout.ensureChannelRoomsAreInAGroup(ctx)
}

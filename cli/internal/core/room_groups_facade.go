package core

import (
	"context"

	"github.com/chattocorp/chatto/cli/internal/readmodel"
)

// GetRoomGroup delegates to the owning RoomLayoutModel.
func (c *ChattoCore) GetRoomGroup(ctx context.Context, groupID string) (*readmodel.RoomGroup, error) {
	return c.roomLayout.GetRoomGroup(ctx, groupID)
}

// ListRoomGroupsOrdered delegates to the owning RoomLayoutModel.
func (c *ChattoCore) ListRoomGroupsOrdered(ctx context.Context, kind RoomKind) ([]*readmodel.RoomGroup, error) {
	return c.roomLayout.ListRoomGroupsOrdered(ctx, kind)
}

// ensureChannelRoomsAreInAGroup delegates to the owning RoomLayoutModel.
func (c *ChattoCore) ensureChannelRoomsAreInAGroup(ctx context.Context) error {
	return c.roomLayout.ensureChannelRoomsAreInAGroup(ctx)
}

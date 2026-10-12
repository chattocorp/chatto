package core

import (
	"context"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

// CreateRoomGroup delegates to the owning RoomLayoutModel.
func (c *ChattoCore) CreateRoomGroup(ctx context.Context, actorID, name, description string) (*evtv1.RoomGroup, error) {
	return c.roomLayout.CreateRoomGroup(ctx, actorID, name, description)
}

// UpdateRoomGroup delegates to the owning RoomLayoutModel.
func (c *ChattoCore) UpdateRoomGroup(ctx context.Context, actorID, groupID, name, description string) (*evtv1.RoomGroup, error) {
	return c.roomLayout.UpdateRoomGroup(ctx, actorID, groupID, name, description)
}

// UpdateRoomGroupFields delegates to the owning RoomLayoutModel.
func (c *ChattoCore) UpdateRoomGroupFields(ctx context.Context, actorID, groupID string, name, description *string) (*evtv1.RoomGroup, error) {
	return c.roomLayout.UpdateRoomGroupFields(ctx, actorID, groupID, name, description)
}

// GetRoomGroup delegates to the owning RoomLayoutModel.
func (c *ChattoCore) GetRoomGroup(ctx context.Context, groupID string) (*evtv1.RoomGroup, error) {
	return c.roomLayout.GetRoomGroup(ctx, groupID)
}

// DeleteRoomGroup delegates to the owning RoomLayoutModel.
func (c *ChattoCore) DeleteRoomGroup(ctx context.Context, actorID, groupID string) error {
	return c.roomLayout.DeleteRoomGroup(ctx, actorID, groupID)
}

// MoveRoomToGroup delegates to the owning RoomLayoutModel.
func (c *ChattoCore) MoveRoomToGroup(ctx context.Context, actorID, roomID, targetGroupID string) error {
	return c.roomLayout.MoveRoomToGroup(ctx, actorID, roomID, targetGroupID)
}

// MoveRoomToGroupFromSource delegates to the owning RoomLayoutModel.
func (c *ChattoCore) MoveRoomToGroupFromSource(ctx context.Context, actorID, roomID, sourceGroupID, targetGroupID string) error {
	return c.roomLayout.MoveRoomToGroupFromSource(ctx, actorID, roomID, sourceGroupID, targetGroupID)
}

// ReorderRoomGroups delegates to the owning RoomLayoutModel.
func (c *ChattoCore) ReorderRoomGroups(ctx context.Context, actorID string, orderedGroupIDs []string) error {
	return c.roomLayout.ReorderRoomGroups(ctx, actorID, orderedGroupIDs)
}

// ReorderRoomsInGroup delegates to the owning RoomLayoutModel.
func (c *ChattoCore) ReorderRoomsInGroup(ctx context.Context, actorID, groupID string, orderedRoomIDs []string) error {
	return c.roomLayout.ReorderRoomsInGroup(ctx, actorID, groupID, orderedRoomIDs)
}

// CreateSidebarLink delegates to the owning RoomLayoutModel.
func (c *ChattoCore) CreateSidebarLink(ctx context.Context, actorID, groupID, label, rawURL string) (*evtv1.SidebarLink, error) {
	return c.roomLayout.CreateSidebarLink(ctx, actorID, groupID, label, rawURL)
}

// UpdateSidebarLink delegates to the owning RoomLayoutModel.
func (c *ChattoCore) UpdateSidebarLink(ctx context.Context, actorID, linkID, label, rawURL string) (*evtv1.SidebarLink, error) {
	return c.roomLayout.UpdateSidebarLink(ctx, actorID, linkID, label, rawURL)
}

// UpdateSidebarLinkInGroup delegates to the owning RoomLayoutModel.
func (c *ChattoCore) UpdateSidebarLinkInGroup(ctx context.Context, actorID, groupID, linkID, label, rawURL string) (*evtv1.SidebarLink, error) {
	return c.roomLayout.UpdateSidebarLinkInGroup(ctx, actorID, groupID, linkID, label, rawURL)
}

// DeleteSidebarLink delegates to the owning RoomLayoutModel.
func (c *ChattoCore) DeleteSidebarLink(ctx context.Context, actorID, linkID string) error {
	return c.roomLayout.DeleteSidebarLink(ctx, actorID, linkID)
}

// DeleteSidebarLinkInGroup delegates to the owning RoomLayoutModel.
func (c *ChattoCore) DeleteSidebarLinkInGroup(ctx context.Context, actorID, groupID, linkID string) error {
	return c.roomLayout.DeleteSidebarLinkInGroup(ctx, actorID, groupID, linkID)
}

// MoveSidebarLinkToGroup delegates to the owning RoomLayoutModel.
func (c *ChattoCore) MoveSidebarLinkToGroup(ctx context.Context, actorID, linkID, targetGroupID string) error {
	return c.roomLayout.MoveSidebarLinkToGroup(ctx, actorID, linkID, targetGroupID)
}

// MoveSidebarLinkBetweenGroups delegates to the owning RoomLayoutModel.
func (c *ChattoCore) MoveSidebarLinkBetweenGroups(ctx context.Context, actorID, linkID, sourceGroupID, targetGroupID string) error {
	return c.roomLayout.MoveSidebarLinkBetweenGroups(ctx, actorID, linkID, sourceGroupID, targetGroupID)
}

// ReorderSidebarItemsInGroup delegates to the owning RoomLayoutModel.
func (c *ChattoCore) ReorderSidebarItemsInGroup(ctx context.Context, actorID, groupID string, orderedEntries []*evtv1.SidebarGroupEntry) error {
	return c.roomLayout.ReorderSidebarItemsInGroup(ctx, actorID, groupID, orderedEntries)
}

// ListRoomGroupsOrdered delegates to the owning RoomLayoutModel.
func (c *ChattoCore) ListRoomGroupsOrdered(ctx context.Context, kind RoomKind) ([]*evtv1.RoomGroup, error) {
	return c.roomLayout.ListRoomGroupsOrdered(ctx, kind)
}

// ensureChannelRoomsAreInAGroup delegates to the owning RoomLayoutModel.
func (c *ChattoCore) ensureChannelRoomsAreInAGroup(ctx context.Context) error {
	return c.roomLayout.ensureChannelRoomsAreInAGroup(ctx)
}

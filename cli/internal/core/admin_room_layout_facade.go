package core

import (
	"context"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func (c *ChattoCore) GetAdminRoom(ctx context.Context, actorID, roomID string) (*AdminRoomManagementDetails, error) {
	return c.roomLayout.GetAdminRoom(ctx, actorID, roomID)
}

func (c *ChattoCore) GetAdminRoomGroup(ctx context.Context, actorID, groupID string) (*AdminRoomGroupManagementDetails, error) {
	return c.roomLayout.GetAdminRoomGroup(ctx, actorID, groupID)
}

func (c *ChattoCore) AdminCreateRoomGroup(ctx context.Context, actorID, name, description string) (*evtv1.RoomGroup, error) {
	return c.roomLayout.AdminCreateRoomGroup(ctx, actorID, name, description)
}

func (c *ChattoCore) AdminUpdateRoomGroup(ctx context.Context, actorID, groupID string, name, description *string) (*evtv1.RoomGroup, error) {
	return c.roomLayout.AdminUpdateRoomGroup(ctx, actorID, groupID, name, description)
}

func (c *ChattoCore) AdminDeleteRoomGroup(ctx context.Context, actorID, groupID string) error {
	return c.roomLayout.AdminDeleteRoomGroup(ctx, actorID, groupID)
}

func (c *ChattoCore) AdminReorderRoomGroups(ctx context.Context, actorID string, orderedGroupIDs []string) error {
	return c.roomLayout.AdminReorderRoomGroups(ctx, actorID, orderedGroupIDs)
}

func (c *ChattoCore) AdminMoveRoomGroup(ctx context.Context, actorID, groupID, beforeGroupID string) error {
	return c.roomLayout.AdminMoveRoomGroup(ctx, actorID, groupID, beforeGroupID)
}

func (c *ChattoCore) AdminMoveRoomToGroup(ctx context.Context, actorID, roomID, targetGroupID string) (*evtv1.Room, error) {
	return c.roomLayout.AdminMoveRoomToGroup(ctx, actorID, roomID, targetGroupID)
}

func (c *ChattoCore) AdminReorderSidebarItemsInGroup(ctx context.Context, actorID, groupID string, orderedEntries []*evtv1.SidebarGroupEntry) (*evtv1.RoomGroup, error) {
	return c.roomLayout.AdminReorderSidebarItemsInGroup(ctx, actorID, groupID, orderedEntries)
}

func (c *ChattoCore) AdminMoveSidebarItem(ctx context.Context, actorID string, item *evtv1.SidebarGroupEntry, targetGroupID string, before *evtv1.SidebarGroupEntry) (*evtv1.RoomGroup, error) {
	return c.roomLayout.AdminMoveSidebarItem(ctx, actorID, item, targetGroupID, before)
}

func (c *ChattoCore) AdminCreateSidebarLink(ctx context.Context, actorID, groupID, label, rawURL string) (*evtv1.SidebarLink, error) {
	return c.roomLayout.AdminCreateSidebarLink(ctx, actorID, groupID, label, rawURL)
}

func (c *ChattoCore) AdminUpdateSidebarLink(ctx context.Context, actorID, linkID string, label, rawURL *string) (*evtv1.SidebarLink, error) {
	return c.roomLayout.AdminUpdateSidebarLink(ctx, actorID, linkID, label, rawURL)
}

func (c *ChattoCore) AdminDeleteSidebarLink(ctx context.Context, actorID, linkID string) error {
	return c.roomLayout.AdminDeleteSidebarLink(ctx, actorID, linkID)
}

func (c *ChattoCore) AdminMoveSidebarLinkToGroup(ctx context.Context, actorID, linkID, targetGroupID string) (*evtv1.SidebarLink, error) {
	return c.roomLayout.AdminMoveSidebarLinkToGroup(ctx, actorID, linkID, targetGroupID)
}

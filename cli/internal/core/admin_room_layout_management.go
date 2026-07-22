package core

import (
	"context"
	"errors"
	"fmt"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type AdminRoomManagementDetails struct {
	Room                 *corev1.Room
	CanManageRoom        bool
	CanManagePermissions bool
}

type AdminRoomGroupManagementDetails struct {
	Group                *corev1.RoomGroup
	CanManageGroup       bool
	CanManagePermissions bool
}

func (c *ChattoCore) GetAdminRoom(ctx context.Context, actorID, roomID string) (*AdminRoomManagementDetails, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	room, err := c.GetRoom(ctx, KindChannel, roomID)
	if err != nil {
		return nil, err
	}
	canManageRoom, err := c.PermResolver().HasRoomPermission(ctx, actorID, KindChannel, roomID, PermRoomManage)
	if err != nil {
		return nil, err
	}
	canManageRoles, err := c.CanManageRoles(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !canManageRoom && !canManageRoles {
		return nil, ErrPermissionDenied
	}
	return &AdminRoomManagementDetails{
		Room:                 room,
		CanManageRoom:        canManageRoom,
		CanManagePermissions: canManageRoom || canManageRoles,
	}, nil
}

func (c *ChattoCore) GetAdminRoomGroup(ctx context.Context, actorID, groupID string) (*AdminRoomGroupManagementDetails, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	group, err := c.GetRoomGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	canManageGroup, err := c.CanManageRoomGroup(ctx, actorID, groupID)
	if err != nil {
		return nil, err
	}
	canManageRoles, err := c.CanManageRoles(ctx, actorID)
	if err != nil {
		return nil, err
	}
	if !canManageGroup && !canManageRoles {
		return nil, ErrPermissionDenied
	}
	return &AdminRoomGroupManagementDetails{
		Group:                group,
		CanManageGroup:       canManageGroup,
		CanManagePermissions: canManageGroup || canManageRoles,
	}, nil
}

func (c *ChattoCore) AdminCreateRoomGroup(ctx context.Context, actorID, name, description string) (*corev1.RoomGroup, error) {
	if err := c.requireCanManageAnyRoom(ctx, actorID); err != nil {
		return nil, err
	}
	return c.createRoomGroup(ctx, actorID, name, description, c.anyRoomAuthorityCheck(ctx, actorID))
}

func (c *ChattoCore) AdminUpdateRoomGroup(ctx context.Context, actorID, groupID string, name, description *string) (*corev1.RoomGroup, error) {
	if err := c.requireCanManageRoomGroup(ctx, actorID, groupID); err != nil {
		return nil, err
	}
	if name == nil && description == nil {
		return nil, fmt.Errorf("%w: provide at least one room group field to update", ErrInvalidArgument)
	}
	return c.updateRoomGroupFields(ctx, actorID, groupID, name, description, c.roomGroupAuthorityCheck(ctx, actorID, groupID))
}

func (c *ChattoCore) AdminDeleteRoomGroup(ctx context.Context, actorID, groupID string) error {
	if err := c.requireCanManageRoomGroup(ctx, actorID, groupID); err != nil {
		return err
	}
	return c.deleteRoomGroup(ctx, actorID, groupID, c.roomGroupAuthorityCheck(ctx, actorID, groupID))
}

func (c *ChattoCore) AdminReorderRoomGroups(ctx context.Context, actorID string, orderedGroupIDs []string) error {
	if err := c.requireCanManageAnyRoom(ctx, actorID); err != nil {
		return err
	}
	return c.reorderRoomGroups(ctx, actorID, orderedGroupIDs, c.anyRoomAuthorityCheck(ctx, actorID))
}

func (c *ChattoCore) AdminMoveRoomToGroup(ctx context.Context, actorID, roomID, targetGroupID string) (*corev1.Room, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	for attempt := 0; attempt < maxMoveRoomToGroupRetries; attempt++ {
		room, err := c.GetRoom(ctx, KindChannel, roomID)
		if err != nil {
			return nil, err
		}
		sourceGroupID := room.GetGroupId()
		if err := c.requireCanManageRoomGroup(ctx, actorID, sourceGroupID); err != nil {
			return nil, err
		}
		if err := c.requireCanManageRoomGroup(ctx, actorID, targetGroupID); err != nil {
			return nil, err
		}
		authorize := func(sourceGroupID, targetGroupID string) error {
			return c.roomGroupAuthorityCheck(ctx, actorID, sourceGroupID, targetGroupID)()
		}
		if err := c.moveRoomToGroup(ctx, actorID, roomID, sourceGroupID, targetGroupID, true, authorize); err != nil {
			if errors.Is(err, ErrRoomMoveSourceChanged) {
				continue
			}
			return nil, err
		}
		return c.GetRoom(ctx, KindChannel, roomID)
	}
	return nil, fmt.Errorf("move room source authorization retry exhausted: %w", ErrRoomMoveSourceChanged)
}

func (c *ChattoCore) AdminReorderSidebarItemsInGroup(ctx context.Context, actorID, groupID string, orderedEntries []*corev1.SidebarGroupEntry) (*corev1.RoomGroup, error) {
	if err := c.requireCanManageRoomGroup(ctx, actorID, groupID); err != nil {
		return nil, err
	}
	if err := c.reorderSidebarItemsInGroup(ctx, actorID, groupID, orderedEntries, c.roomGroupAuthorityCheck(ctx, actorID, groupID)); err != nil {
		return nil, err
	}
	return c.GetRoomGroup(ctx, groupID)
}

func (c *ChattoCore) AdminCreateSidebarLink(ctx context.Context, actorID, groupID, label, rawURL string) (*corev1.SidebarLink, error) {
	if err := c.requireCanManageRoomGroup(ctx, actorID, groupID); err != nil {
		return nil, err
	}
	return c.createSidebarLink(ctx, actorID, groupID, label, rawURL, c.roomGroupAuthorityCheck(ctx, actorID, groupID))
}

func (c *ChattoCore) AdminUpdateSidebarLink(ctx context.Context, actorID, linkID string, label, rawURL *string) (*corev1.SidebarLink, error) {
	groupID, err := c.sidebarLinkGroup(ctx, linkID)
	if err != nil {
		return nil, err
	}
	if err := c.requireCanManageRoomGroup(ctx, actorID, groupID); err != nil {
		return nil, err
	}
	if label == nil && rawURL == nil {
		return nil, fmt.Errorf("%w: provide at least one sidebar link field to update", ErrInvalidArgument)
	}
	group, err := c.GetRoomGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	link := sidebarLinkFromGroup(group, linkID)
	if link == nil {
		return nil, ErrSidebarLinkNotFound
	}
	nextLabel := link.GetLabel()
	if label != nil {
		nextLabel = *label
	}
	nextURL := link.GetUrl()
	if rawURL != nil {
		nextURL = *rawURL
	}
	return c.updateSidebarLinkInGroup(ctx, actorID, groupID, linkID, nextLabel, nextURL, c.roomGroupAuthorityCheck(ctx, actorID, groupID))
}

func (c *ChattoCore) AdminDeleteSidebarLink(ctx context.Context, actorID, linkID string) error {
	groupID, err := c.sidebarLinkGroup(ctx, linkID)
	if err != nil {
		return err
	}
	if err := c.requireCanManageRoomGroup(ctx, actorID, groupID); err != nil {
		return err
	}
	return c.deleteSidebarLinkInGroup(ctx, actorID, groupID, linkID, c.roomGroupAuthorityCheck(ctx, actorID, groupID))
}

func (c *ChattoCore) AdminMoveSidebarLinkToGroup(ctx context.Context, actorID, linkID, targetGroupID string) (*corev1.SidebarLink, error) {
	sourceGroupID, err := c.sidebarLinkGroup(ctx, linkID)
	if err != nil {
		return nil, err
	}
	if err := c.requireCanManageRoomGroup(ctx, actorID, sourceGroupID); err != nil {
		return nil, err
	}
	if err := c.requireCanManageRoomGroup(ctx, actorID, targetGroupID); err != nil {
		return nil, err
	}
	authorize := func(sourceGroupID, targetGroupID string) error {
		return c.roomGroupAuthorityCheck(ctx, actorID, sourceGroupID, targetGroupID)()
	}
	if err := c.moveSidebarLinkBetweenGroups(ctx, actorID, linkID, sourceGroupID, targetGroupID, authorize); err != nil {
		return nil, err
	}
	targetGroup, err := c.GetRoomGroup(ctx, targetGroupID)
	if err != nil {
		return nil, err
	}
	link := sidebarLinkFromGroup(targetGroup, linkID)
	if link == nil {
		return nil, ErrSidebarLinkNotFound
	}
	return link, nil
}

func (c *ChattoCore) requireCanManageAnyRoom(ctx context.Context, actorID string) error {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return err
	}
	ok, err := c.CanManageAnyRoom(ctx, actorID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrPermissionDenied
	}
	return nil
}

func (c *ChattoCore) requireCanManageRoomGroup(ctx context.Context, actorID, groupID string) error {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return err
	}
	ok, err := c.CanManageRoomGroup(ctx, actorID, groupID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrPermissionDenied
	}
	return nil
}

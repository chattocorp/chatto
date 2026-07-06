package core

import (
	"context"
	"errors"
	"fmt"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func (c *ChattoCore) AdminCreateRoomGroup(ctx context.Context, actorID, name, description string) (*corev1.RoomGroup, error) {
	if err := c.requireCanManageRoles(ctx, actorID); err != nil {
		return nil, err
	}
	return c.CreateRoomGroup(ctx, actorID, name, description)
}

func (c *ChattoCore) AdminUpdateRoomGroup(ctx context.Context, actorID, groupID string, name, description *string) (*corev1.RoomGroup, error) {
	if err := c.requireCanManageRoles(ctx, actorID); err != nil {
		return nil, err
	}
	if name == nil && description == nil {
		return nil, fmt.Errorf("%w: provide at least one room group field to update", ErrInvalidArgument)
	}
	group, err := c.GetRoomGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	nextName := group.GetName()
	if name != nil {
		nextName = *name
	}
	nextDescription := group.GetDescription()
	if description != nil {
		nextDescription = *description
	}
	return c.UpdateRoomGroup(ctx, actorID, groupID, nextName, nextDescription)
}

func (c *ChattoCore) AdminDeleteRoomGroup(ctx context.Context, actorID, groupID string) error {
	if err := c.requireCanManageRoles(ctx, actorID); err != nil {
		return err
	}
	return c.DeleteRoomGroup(ctx, actorID, groupID)
}

func (c *ChattoCore) AdminReorderRoomGroups(ctx context.Context, actorID string, orderedGroupIDs []string) error {
	if err := c.requireCanManageRoles(ctx, actorID); err != nil {
		return err
	}
	return c.ReorderRoomGroups(ctx, actorID, orderedGroupIDs)
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
		if err := c.MoveRoomToGroupFromSource(ctx, actorID, roomID, sourceGroupID, targetGroupID); err != nil {
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
	if err := c.ReorderSidebarItemsInGroup(ctx, actorID, groupID, orderedEntries); err != nil {
		return nil, err
	}
	return c.GetRoomGroup(ctx, groupID)
}

func (c *ChattoCore) AdminCreateSidebarLink(ctx context.Context, actorID, groupID, label, rawURL string) (*corev1.SidebarLink, error) {
	if err := c.requireCanManageRoomGroup(ctx, actorID, groupID); err != nil {
		return nil, err
	}
	return c.CreateSidebarLink(ctx, actorID, groupID, label, rawURL)
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
	return c.UpdateSidebarLinkInGroup(ctx, actorID, groupID, linkID, nextLabel, nextURL)
}

func (c *ChattoCore) AdminDeleteSidebarLink(ctx context.Context, actorID, linkID string) error {
	groupID, err := c.sidebarLinkGroup(ctx, linkID)
	if err != nil {
		return err
	}
	if err := c.requireCanManageRoomGroup(ctx, actorID, groupID); err != nil {
		return err
	}
	return c.DeleteSidebarLinkInGroup(ctx, actorID, groupID, linkID)
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
	if err := c.MoveSidebarLinkBetweenGroups(ctx, actorID, linkID, sourceGroupID, targetGroupID); err != nil {
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

func (c *ChattoCore) requireCanManageRoles(ctx context.Context, actorID string) error {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return err
	}
	ok, err := c.CanManageRoles(ctx, actorID)
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

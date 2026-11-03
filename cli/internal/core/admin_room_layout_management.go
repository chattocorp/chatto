package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

type AdminRoomManagementDetails struct {
	Room                 *readmodel.Room
	CanManageRoom        bool
	CanManagePermissions bool
}

type AdminRoomGroupManagementDetails struct {
	Group                *readmodel.RoomGroup
	CanManageGroup       bool
	CanManagePermissions bool
}

func (m *RoomLayoutModel) GetAdminRoom(ctx context.Context, actorID, roomID string) (*AdminRoomManagementDetails, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	room, err := m.rooms.GetRoom(ctx, KindChannel, roomID)
	if err != nil {
		return nil, err
	}
	canManageRoom, err := m.policy.resolver.HasRoomPermission(ctx, actorID, KindChannel, roomID, PermRoomManage)
	if err != nil {
		return nil, err
	}
	canManageRoles, err := m.policy.CanManageRoles(ctx, actorID)
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

func (m *RoomLayoutModel) GetAdminRoomGroup(ctx context.Context, actorID, groupID string) (*AdminRoomGroupManagementDetails, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	group, err := m.rooms.GetRoomGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	canManageGroup, err := m.policy.CanManageRoomGroup(ctx, actorID, groupID)
	if err != nil {
		return nil, err
	}
	canManageRoles, err := m.policy.CanManageRoles(ctx, actorID)
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

func (m *RoomLayoutModel) AdminCreateRoomGroup(ctx context.Context, actorID, name, description string) (*readmodel.RoomGroup, error) {
	if err := m.policy.requireCanManageAnyRoom(ctx, actorID); err != nil {
		return nil, err
	}
	return m.createRoomGroup(ctx, actorID, name, description, m.anyRoomAuthorityCheck(ctx, actorID))
}

func (m *RoomLayoutModel) AdminUpdateRoomGroup(ctx context.Context, actorID, groupID string, name, description *string) (*readmodel.RoomGroup, error) {
	if err := m.policy.requireCanManageRoomGroup(ctx, actorID, groupID); err != nil {
		return nil, err
	}
	if name == nil && description == nil {
		return nil, fmt.Errorf("%w: provide at least one room group field to update", ErrInvalidArgument)
	}
	return m.updateRoomGroupFields(ctx, actorID, groupID, name, description, m.roomGroupAuthorityCheck(ctx, actorID, groupID))
}

func (m *RoomLayoutModel) AdminDeleteRoomGroup(ctx context.Context, actorID, groupID string) error {
	if err := m.policy.requireCanManageRoomGroup(ctx, actorID, groupID); err != nil {
		return err
	}
	return m.deleteRoomGroup(ctx, actorID, groupID, m.roomGroupAuthorityCheck(ctx, actorID, groupID))
}

func (m *RoomLayoutModel) AdminReorderRoomGroups(ctx context.Context, actorID string, orderedGroupIDs []string) error {
	if err := m.policy.requireCanManageAnyRoom(ctx, actorID); err != nil {
		return err
	}
	return m.reorderRoomGroups(ctx, actorID, orderedGroupIDs, m.anyRoomAuthorityCheck(ctx, actorID))
}

// AdminMoveRoomGroup places one group before another group, or last when
// beforeGroupID is empty. The server recomputes the complete order on an OCC
// conflict so callers can submit relative intent from a filtered UI.
func (m *RoomLayoutModel) AdminMoveRoomGroup(ctx context.Context, actorID, groupID, beforeGroupID string) error {
	if err := m.policy.requireCanManageAnyRoom(ctx, actorID); err != nil {
		return err
	}
	return m.placeRoomGroup(ctx, actorID, groupID, beforeGroupID, m.anyRoomAuthorityCheck(ctx, actorID))
}

func (m *RoomLayoutModel) AdminMoveRoomToGroup(ctx context.Context, actorID, roomID, targetGroupID string) (*readmodel.Room, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	for range maxMoveRoomToGroupRetries {
		room, err := m.rooms.GetRoom(ctx, KindChannel, roomID)
		if err != nil {
			return nil, err
		}
		sourceGroupID := room.GetGroupId()
		if err := m.policy.requireCanManageRoomGroup(ctx, actorID, sourceGroupID); err != nil {
			return nil, err
		}
		if err := m.policy.requireCanManageRoomGroup(ctx, actorID, targetGroupID); err != nil {
			return nil, err
		}
		authorize := func(sourceGroupID, targetGroupID string) error {
			return m.roomGroupAuthorityCheck(ctx, actorID, sourceGroupID, targetGroupID)()
		}
		if err := m.moveRoomToGroup(ctx, actorID, roomID, sourceGroupID, targetGroupID, true, authorize); err != nil {
			if errors.Is(err, ErrRoomMoveSourceChanged) {
				continue
			}
			return nil, err
		}
		return m.rooms.GetRoom(ctx, KindChannel, roomID)
	}
	return nil, fmt.Errorf("move room source authorization retry exhausted: %w", ErrRoomMoveSourceChanged)
}

func (m *RoomLayoutModel) AdminReorderSidebarItemsInGroup(ctx context.Context, actorID, groupID string, orderedEntries []*evtv1.SidebarGroupEntry) (*readmodel.RoomGroup, error) {
	if err := m.policy.requireCanManageRoomGroup(ctx, actorID, groupID); err != nil {
		return nil, err
	}
	if err := m.reorderSidebarItemsInGroup(ctx, actorID, groupID, orderedEntries, m.roomGroupAuthorityCheck(ctx, actorID, groupID)); err != nil {
		return nil, err
	}
	return m.rooms.GetRoomGroup(ctx, groupID)
}

// AdminMoveSidebarItem places one room or sidebar link in a destination group.
// Each OCC attempt validates the source and destination authorization inputs
// with a stable request-time read.
func (m *RoomLayoutModel) AdminMoveSidebarItem(ctx context.Context, actorID string, item *evtv1.SidebarGroupEntry, targetGroupID string, before *evtv1.SidebarGroupEntry) (*readmodel.RoomGroup, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	authorize := func(sourceGroupID, destinationGroupID string) error {
		return m.roomGroupAuthorityCheck(ctx, actorID, sourceGroupID, destinationGroupID)()
	}
	if err := m.placeSidebarItem(ctx, actorID, item, targetGroupID, before, authorize); err != nil {
		return nil, err
	}
	return m.rooms.GetRoomGroup(ctx, targetGroupID)
}

func (m *RoomLayoutModel) AdminCreateSidebarLink(ctx context.Context, actorID, groupID, label, rawURL string) (*readmodel.SidebarLink, error) {
	if err := m.policy.requireCanManageRoomGroup(ctx, actorID, groupID); err != nil {
		return nil, err
	}
	return m.createSidebarLink(ctx, actorID, groupID, label, rawURL, m.roomGroupAuthorityCheck(ctx, actorID, groupID))
}

func (m *RoomLayoutModel) AdminUpdateSidebarLink(ctx context.Context, actorID, linkID string, label, rawURL *string) (*readmodel.SidebarLink, error) {
	groupID, err := m.sidebarLinkGroup(ctx, linkID)
	if err != nil {
		return nil, err
	}
	if err := m.policy.requireCanManageRoomGroup(ctx, actorID, groupID); err != nil {
		return nil, err
	}
	if label == nil && rawURL == nil {
		return nil, fmt.Errorf("%w: provide at least one sidebar link field to update", ErrInvalidArgument)
	}
	group, err := m.rooms.GetRoomGroup(ctx, groupID)
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
	return m.updateSidebarLinkInGroup(ctx, actorID, groupID, linkID, nextLabel, nextURL, m.roomGroupAuthorityCheck(ctx, actorID, groupID))
}

func (m *RoomLayoutModel) AdminDeleteSidebarLink(ctx context.Context, actorID, linkID string) error {
	groupID, err := m.sidebarLinkGroup(ctx, linkID)
	if err != nil {
		return err
	}
	if err := m.policy.requireCanManageRoomGroup(ctx, actorID, groupID); err != nil {
		return err
	}
	return m.deleteSidebarLinkInGroup(ctx, actorID, groupID, linkID, m.roomGroupAuthorityCheck(ctx, actorID, groupID))
}

func (m *RoomLayoutModel) AdminMoveSidebarLinkToGroup(ctx context.Context, actorID, linkID, targetGroupID string) (*readmodel.SidebarLink, error) {
	sourceGroupID, err := m.sidebarLinkGroup(ctx, linkID)
	if err != nil {
		return nil, err
	}
	if err := m.policy.requireCanManageRoomGroup(ctx, actorID, sourceGroupID); err != nil {
		return nil, err
	}
	if err := m.policy.requireCanManageRoomGroup(ctx, actorID, targetGroupID); err != nil {
		return nil, err
	}
	authorize := func(sourceGroupID, targetGroupID string) error {
		return m.roomGroupAuthorityCheck(ctx, actorID, sourceGroupID, targetGroupID)()
	}
	if err := m.moveSidebarLinkBetweenGroups(ctx, actorID, linkID, sourceGroupID, targetGroupID, authorize); err != nil {
		return nil, err
	}
	targetGroup, err := m.rooms.GetRoomGroup(ctx, targetGroupID)
	if err != nil {
		return nil, err
	}
	link := sidebarLinkFromGroup(targetGroup, linkID)
	if link == nil {
		return nil, ErrSidebarLinkNotFound
	}
	return link, nil
}

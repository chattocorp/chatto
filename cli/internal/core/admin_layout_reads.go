package core

import (
	"context"

	"github.com/chattocorp/chatto/cli/internal/readmodel"
)

// GetAdminRoomGroupLayout returns complete entries only to a group manager.
// Permission editors without room.manage retain metadata-only access.
func (m *RoomLayoutModel) GetAdminRoomGroupLayout(ctx context.Context, actorID, groupID string) (*readmodel.RoomGroup, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	group, err := m.rooms.GetRoomGroup(ctx, groupID)
	if err != nil {
		return nil, err
	}
	if err := m.policy.requireCanManageRoomGroup(ctx, actorID, groupID); err != nil {
		return nil, err
	}
	return group, nil
}

// AdminSidebarLink identifies a link and its owning group from one snapshot.
type AdminSidebarLink struct {
	Link    *readmodel.SidebarLink
	GroupID string
}

// GetAdminSidebarLink authorizes the group from the same projection snapshot
// that supplies the link. A concurrent move cannot mix a link with another group.
func (m *RoomLayoutModel) GetAdminSidebarLink(ctx context.Context, actorID, linkID string) (*AdminSidebarLink, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, err
	}
	snapshot := m.rooms.sidebarLinkMoveSnapshot(linkID, "")
	if snapshot.Link == nil {
		if err := m.rooms.waitForGroupLayoutCurrent(ctx, m.publisher); err != nil {
			return nil, err
		}
		snapshot = m.rooms.sidebarLinkMoveSnapshot(linkID, "")
	}
	if snapshot.Link == nil {
		return nil, ErrSidebarLinkNotFound
	}
	if err := m.policy.requireCanManageRoomGroup(ctx, actorID, snapshot.SourceGroupID); err != nil {
		return nil, err
	}
	return &AdminSidebarLink{Link: snapshot.Link, GroupID: snapshot.SourceGroupID}, nil
}

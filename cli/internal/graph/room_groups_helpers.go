package graph

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// requireGroupManageAuth gates set-CRUD and set-permission mutations on
// `role.manage` — the same permission required to manage server-wide role
// definitions, applied here because configuring set permissions is the same
// trust level as configuring role permissions.
func (r *Resolver) requireGroupManageAuth(ctx context.Context, userID string) error {
	can, err := r.core.CanManageRoles(ctx, userID)
	if err != nil {
		return fmt.Errorf("check role.manage: %w", err)
	}
	if !can {
		return core.ErrPermissionDenied
	}
	return nil
}

func (r *Resolver) requireRoomGroupRoomManageAuth(ctx context.Context, userID, groupID string) error {
	can, err := r.core.CanManageRoomGroup(ctx, userID, groupID)
	if err != nil {
		return fmt.Errorf("check group room.manage: %w", err)
	}
	if !can {
		return core.ErrPermissionDenied
	}
	return nil
}

// roomGroupToModel converts a proto RoomGroup to its GraphQL model, optionally
// wiring a viewerRooms map for the rooms-sub-resolver. For mutation responses
// we typically don't need to resolve member rooms, so pass nil.
func roomGroupToModel(set *corev1.RoomGroup, viewerRooms map[string]*corev1.Room) *model.RoomGroupModel {
	if set == nil {
		return nil
	}
	return &model.RoomGroupModel{
		ID:           set.Id,
		Name:         set.Name,
		Description:  set.Description,
		RoomIds:      set.RoomIds,
		Entries:      set.Entries,
		SidebarLinks: set.SidebarLinks,
		ViewerRooms:  viewerRooms,
	}
}

func sidebarEntryInputsToProto(inputs []*model.SidebarGroupEntryInput) []*corev1.SidebarGroupEntry {
	entries := make([]*corev1.SidebarGroupEntry, 0, len(inputs))
	for _, input := range inputs {
		if input == nil {
			continue
		}
		kind := corev1.SidebarGroupEntry_KIND_UNSPECIFIED
		switch input.Type {
		case model.RoomGroupItemTypeRoom:
			kind = corev1.SidebarGroupEntry_ROOM
		case model.RoomGroupItemTypeSidebarLink:
			kind = corev1.SidebarGroupEntry_SIDEBAR_LINK
		}
		entries = append(entries, &corev1.SidebarGroupEntry{
			Kind: kind,
			Id:   input.ID,
		})
	}
	return entries
}

func findSidebarLink(group *corev1.RoomGroup, linkID string) (*corev1.SidebarLink, error) {
	for _, link := range group.GetSidebarLinks() {
		if link.GetId() == linkID {
			return link, nil
		}
	}
	return nil, core.ErrSidebarLinkNotFound
}

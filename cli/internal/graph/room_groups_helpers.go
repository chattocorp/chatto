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

// roomGroupToModel converts a proto RoomGroup to its GraphQL model, optionally
// wiring a viewerRooms map for the rooms-sub-resolver. For mutation responses
// we typically don't need to resolve member rooms, so pass nil.
func roomGroupToModel(set *corev1.RoomGroup, viewerRooms map[string]*corev1.Room) *model.RoomGroupModel {
	if set == nil {
		return nil
	}
	return &model.RoomGroupModel{
		ID:          set.Id,
		Name:        set.Name,
		Description: set.Description,
		RoomIds:     set.RoomIds,
		ViewerRooms: viewerRooms,
	}
}

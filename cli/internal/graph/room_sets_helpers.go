package graph

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// requireSetManageAuth gates set-CRUD and set-permission mutations on
// `role.manage` — the same permission required to manage server-wide role
// definitions, applied here because configuring set permissions is the same
// trust level as configuring role permissions.
func (r *Resolver) requireSetManageAuth(ctx context.Context, userID string) error {
	can, err := r.core.CanManageRoles(ctx, userID)
	if err != nil {
		return fmt.Errorf("check role.manage: %w", err)
	}
	if !can {
		return core.ErrPermissionDenied
	}
	return nil
}

// roomSetToModel converts a proto RoomSet to its GraphQL model, optionally
// wiring a viewerRooms map for the rooms-sub-resolver. For mutation responses
// we typically don't need to resolve member rooms, so pass nil.
func roomSetToModel(set *corev1.RoomSet, viewerRooms map[string]*corev1.Room) *model.RoomSetModel {
	if set == nil {
		return nil
	}
	return &model.RoomSetModel{
		ID:          set.Id,
		Name:        set.Name,
		Description: set.Description,
		RoomIds:     set.RoomIds,
		ViewerRooms: viewerRooms,
	}
}

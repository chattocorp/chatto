package graph

import (
	"context"

	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// roomTypeIs reports whether the requested filter (which may be nil)
// matches the given concrete type. nil means "no filter — match
// everything"; non-nil means "match only this type."
func roomTypeIs(filter *model.RoomType, want model.RoomType) bool {
	return filter == nil || *filter == want
}

// resolveServerSpace returns the *corev1.Space for the deployment's
// user-facing space, or (nil, nil) on fresh installs. Vestigial — kept
// alive while the Space API surface is mid-retirement.
func (r *Resolver) resolveServerSpace(ctx context.Context) (*corev1.Space, error) {
	id, err := r.core.FirstUserFacingSpaceID(ctx)
	if err != nil || id == "" {
		return nil, err
	}
	return r.core.GetSpace(ctx, id)
}

// isServerSpace reports whether spaceID matches the deployment's first
// user-facing space (the one PrimarySpaceID surfaces). Vestigial.
func (r *Resolver) isServerSpace(ctx context.Context, spaceID string) bool {
	id, err := r.core.FirstUserFacingSpaceID(ctx)
	return err == nil && id != "" && id == spaceID
}

// appendDMRoomsForServer appends the user's DM conversations to a server-space
// rooms list (issue #330 / ADR-027 phase 3). Storage stays in the hidden DM
// space (ADR-015); only the API surface merges. The caller's dm.view permission
// is checked — without it the original list is returned unchanged.
//
// No-op when:
//   - spaceID isn't the deployment's server space (so resolvers can call this
//     unconditionally on any space without leaking DMs into other spaces); or
//   - the caller asked for channels only (`type: CHANNEL`).
func (r *Resolver) appendDMRoomsForServer(ctx context.Context, spaceID, userID string, rooms []*corev1.Room, roomType *model.RoomType) ([]*corev1.Room, error) {
	if !r.isServerSpace(ctx, spaceID) {
		return rooms, nil
	}
	if !roomTypeIs(roomType, model.RoomTypeDm) {
		return rooms, nil
	}
	canDM, err := r.core.CanDMView(ctx, userID)
	if err != nil || !canDM {
		return rooms, nil
	}
	dms, err := r.core.ListDMConversations(ctx, userID)
	if err != nil {
		return nil, err
	}
	return append(rooms, dms...), nil
}

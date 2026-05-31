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

// appendDMRoomsForServer appends the user's DM conversations to a channel
// rooms list. No-op when the caller asked for channels only (`type: CHANNEL`).
// The DM listing path is membership-filtered; there is no separate DM read
// permission.
func (r *Resolver) appendDMRoomsForServer(ctx context.Context, userID string, rooms []*corev1.Room, roomType *model.RoomType) ([]*corev1.Room, error) {
	if !roomTypeIs(roomType, model.RoomTypeDm) {
		return rooms, nil
	}
	dms, err := r.core.ListDMConversations(ctx, userID)
	if err != nil {
		return nil, err
	}
	return append(rooms, dms...), nil
}

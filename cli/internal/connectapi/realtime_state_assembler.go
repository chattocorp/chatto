// SPDX-FileCopyrightText: 2026-present Chatto contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package connectapi

import (
	"context"

	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

// BuildRealtimeRoom returns the same authorized resource as GetRoom. Callers
// assemble it in delivery order, never at publication time, so delayed hints
// cannot restore the publishing replica's old state.
func (a *API) BuildRealtimeRoom(ctx context.Context, userID, roomID, threadRootEventID string) (*apiv1.RoomWithViewerState, error) {
	if err := a.core.NotificationOccurrences().WaitRoomStateCurrent(ctx, userID, roomID, threadRootEventID); err != nil {
		return nil, err
	}
	room, err := a.core.RoomDirectoryReads().GetCurrentRoom(ctx, userID, roomID)
	if err != nil {
		return nil, err
	}
	return a.apiRoomWithViewerState(ctx, userID, room)
}

// BuildRealtimeNotifications returns a bounded first page and complete attention
// counts, with the same authorization and catch-up rules as the list API.
func (a *API) BuildRealtimeNotifications(ctx context.Context, userID string) (*apiv1.ListNotificationOccurrencesResponse, error) {
	return (&notificationService{api: a}).listOccurrences(ctx, userID, nil)
}

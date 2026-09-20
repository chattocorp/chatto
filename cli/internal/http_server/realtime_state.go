// SPDX-FileCopyrightText: 2026-present Chatto contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package http_server

import (
	"context"
	"time"

	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

// hydrateRealtimeState adds current viewer state only after the event has
// passed delivery authorization. Mapping owns detached payloads. Failed reads
// leave the optional payload absent so clients use their normal read/retry path.
func (s *HTTPServer) hydrateRealtimeState(ctx context.Context, viewerID string, event *realtimev1.RealtimeEvent) {
	switch event.GetEvent().(type) {
	case *realtimev1.RealtimeEvent_MessagePosted, *realtimev1.RealtimeEvent_RoomReadStateChanged,
		*realtimev1.RealtimeEvent_NotificationUnreadStateChanged, *realtimev1.RealtimeEvent_NotificationOccurrencesChanged:
	default:
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	room := func(roomID, threadID string) *apiv1.RoomWithViewerState {
		value, err := s.connectAPI.BuildRealtimeRoom(ctx, viewerID, roomID, threadID)
		if err != nil {
			return nil
		}
		return value
	}
	switch payload := event.GetEvent().(type) {
	case *realtimev1.RealtimeEvent_MessagePosted:
		payload.MessagePosted.Room = room(payload.MessagePosted.GetRoomId(), payload.MessagePosted.GetThreadRootEventId())
	case *realtimev1.RealtimeEvent_RoomReadStateChanged:
		payload.RoomReadStateChanged.Room = room(payload.RoomReadStateChanged.GetRoomId(), "")
	case *realtimev1.RealtimeEvent_NotificationUnreadStateChanged:
		payload.NotificationUnreadStateChanged.Room = room(payload.NotificationUnreadStateChanged.GetRoomId(), payload.NotificationUnreadStateChanged.GetThreadRootEventId())
	case *realtimev1.RealtimeEvent_NotificationOccurrencesChanged:
		payload.NotificationOccurrencesChanged.Notifications = nil
		value, err := s.connectAPI.BuildRealtimeNotifications(ctx, viewerID)
		if err == nil {
			payload.NotificationOccurrencesChanged.Notifications = value
		}
	}
}

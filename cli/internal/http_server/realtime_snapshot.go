// SPDX-FileCopyrightText: 2026-present Chatto contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package http_server

import (
	"context"
	"errors"

	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

// realtimeSnapshotFrames captures detached public resources at one exact
// ServerContentView boundary. Protobuf encoding and WebSocket writes happen
// only after the content-view read barrier has been released.
func (s *HTTPServer) realtimeSnapshotFrames(ctx context.Context, userID string) (uint64, []*realtimev1.RealtimeServerFrame, error) {
	if s.connectAPI == nil {
		return 0, nil, errors.New("Connect API is unavailable")
	}
	resources, err := s.connectAPI.BuildRealtimeSnapshot(ctx, userID)
	if err != nil {
		return 0, nil, err
	}
	snapshots := []*realtimev1.RealtimeSnapshot{
		{Resource: &realtimev1.RealtimeSnapshot_Server{Server: resources.Server}},
		{Resource: &realtimev1.RealtimeSnapshot_Rooms{Rooms: resources.Rooms}},
		{Resource: &realtimev1.RealtimeSnapshot_RoomGroups{RoomGroups: resources.RoomGroups}},
		{Resource: &realtimev1.RealtimeSnapshot_Users{Users: resources.Users}},
		{Resource: &realtimev1.RealtimeSnapshot_ActiveCalls{ActiveCalls: resources.ActiveCalls}},
	}
	frames := make([]*realtimev1.RealtimeServerFrame, 0, len(snapshots))
	for _, snapshot := range snapshots {
		frames = append(frames, &realtimev1.RealtimeServerFrame{
			Frame: &realtimev1.RealtimeServerFrame_Snapshot{Snapshot: snapshot},
		})
	}
	return resources.Sequence, frames, nil
}

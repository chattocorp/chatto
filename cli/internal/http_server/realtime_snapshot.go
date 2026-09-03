// SPDX-FileCopyrightText: 2026-present Chatto contributors
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package http_server

import (
	"context"
	"errors"

	"google.golang.org/protobuf/proto"

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
		{Resource: &realtimev1.RealtimeSnapshot_Rooms{Rooms: &realtimev1.RealtimeRoomsSnapshot{Rooms: resources.Rooms.GetRooms()}}},
		{Resource: &realtimev1.RealtimeSnapshot_RoomGroups{RoomGroups: &realtimev1.RealtimeRoomGroupsSnapshot{RoomGroups: resources.RoomGroups.GetGroups()}}},
		{Resource: &realtimev1.RealtimeSnapshot_Users{Users: &realtimev1.RealtimeUsersSnapshot{Users: resources.Users.GetUsers()}}},
		{Resource: &realtimev1.RealtimeSnapshot_ActiveCalls{ActiveCalls: &realtimev1.RealtimeActiveCallsSnapshot{ActiveCalls: resources.ActiveCalls.GetCalls()}}},
	}
	frames := make([]*realtimev1.RealtimeServerFrame, 0, len(snapshots))
	var snapshotBytes uint64
	for _, snapshot := range snapshots {
		frame := &realtimev1.RealtimeServerFrame{
			Frame: &realtimev1.RealtimeServerFrame_Snapshot{Snapshot: snapshot},
		}
		frames = append(frames, frame)
		snapshotBytes += uint64(proto.Size(frame))
	}
	s.metrics.realtimeSnapshotBytes.Store(snapshotBytes)
	return resources.Sequence, frames, nil
}

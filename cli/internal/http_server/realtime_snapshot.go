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

// realtimeSnapshotFrame captures detached public resources at one exact
// ServerContentView boundary. Protobuf encoding and WebSocket writes happen
// only after the content-view read barrier has been released.
func (s *HTTPServer) realtimeSnapshotFrame(ctx context.Context, userID string) (uint64, *realtimev1.RealtimeServerFrame, error) {
	if s.connectAPI == nil {
		return 0, nil, errors.New("Connect API is unavailable")
	}
	resources, err := s.connectAPI.BuildRealtimeSnapshot(ctx, userID)
	if err != nil {
		return 0, nil, err
	}
	snapshot := &realtimev1.RealtimeSnapshot{
		Server:      resources.Server,
		Rooms:       resources.Rooms.GetRooms(),
		RoomGroups:  resources.RoomGroups.GetGroups(),
		Users:       resources.Users.GetUsers(),
		ActiveCalls: resources.ActiveCalls.GetCalls(),
	}
	frame := &realtimev1.RealtimeServerFrame{
		Frame: &realtimev1.RealtimeServerFrame_Snapshot{Snapshot: snapshot},
	}
	s.metrics.realtimeSnapshotBytes.Store(uint64(proto.Size(frame)))
	return resources.Sequence, frame, nil
}

package http_server

import (
	"context"
	"errors"

	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

// realtimeSnapshotFrames returns the current authorized public resource
// families. Snapshot frames reuse the same protobufs as ConnectRPC reads.
func (s *HTTPServer) realtimeSnapshotFrames(ctx context.Context, userID string) ([]*realtimev1.RealtimeServerFrame, error) {
	if s.connectAPI == nil {
		return nil, errors.New("Connect API is unavailable")
	}
	chunks, err := s.connectAPI.BuildServerSnapshot(ctx, userID)
	if err != nil {
		return nil, err
	}
	frames := make([]*realtimev1.RealtimeServerFrame, 0, len(chunks))
	for _, chunk := range chunks {
		frames = append(frames, &realtimev1.RealtimeServerFrame{
			Frame: &realtimev1.RealtimeServerFrame_Snapshot{Snapshot: chunk},
		})
	}
	return frames, nil
}

func (s *HTTPServer) writeRealtimeSnapshot(ctx context.Context, userID string, writeFrame func(*realtimev1.RealtimeServerFrame) error) error {
	frames, err := s.realtimeSnapshotFrames(ctx, userID)
	if err != nil {
		return err
	}
	for _, frame := range frames {
		if err := writeFrame(frame); err != nil {
			return err
		}
	}
	return nil
}

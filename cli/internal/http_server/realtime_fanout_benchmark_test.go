package http_server

import (
	"fmt"
	"testing"

	"github.com/chattocorp/chatto/cli/internal/core"
	"google.golang.org/protobuf/proto"
)

// BenchmarkRealtimeMessagePostFrames measures the connection-side cost of one
// message post for a room audience: each viewer's public event mapping, body
// hydration, resume cursor, and protobuf encoding. One operation is one event
// for all viewers. The benchmark also reports the cost of one viewer.
//
// The hub fanout before this step and the WebSocket write after it are outside
// the measurement. See BenchmarkMyEventsHubRoomFanout in the core package.
func BenchmarkRealtimeMessagePostFrames(b *testing.B) {
	const viewers = 100
	env := setupWebSocketTestServer(b)

	viewerIDs := make([]string, 0, viewers)
	for i := range viewers {
		user, err := env.core.CreateUser(env.ctx, core.SystemActorID, fmt.Sprintf("frames%d", i), fmt.Sprintf("Frames %d", i), "password123")
		if err != nil {
			b.Fatalf("CreateUser: %v", err)
		}
		viewerIDs = append(viewerIDs, user.Id)
	}
	room, err := env.core.CreateRoom(env.ctx, viewerIDs[0], core.KindChannel, "", "frames", "")
	if err != nil {
		b.Fatalf("CreateRoom: %v", err)
	}
	for _, id := range viewerIDs {
		if _, err := env.core.JoinRoom(env.ctx, id, core.KindChannel, id, room.Id); err != nil {
			b.Fatalf("JoinRoom: %v", err)
		}
	}
	posted, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, viewerIDs[0], "A plain message of ordinary length, without attachments.", nil, "", "", nil, false)
	if err != nil {
		b.Fatalf("PostMessage: %v", err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	frameBytes := 0
	for range b.N {
		// Each delivery gets a new envelope, so per-event shared work counts
		// once in each operation.
		envelope := core.NewEVTEventEnvelopeWithDeliverySeq(posted, 42)
		for _, viewerID := range viewerIDs {
			frame, err := env.httpServer.realtimeServerFrameForEvent(env.ctx, viewerID, envelope)
			if err != nil {
				b.Fatalf("realtimeServerFrameForEvent: %v", err)
			}
			data, err := proto.Marshal(frame)
			if err != nil {
				b.Fatalf("Marshal: %v", err)
			}
			frameBytes = len(data)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*viewers), "ns/viewer")
	b.ReportMetric(float64(frameBytes), "bytes/frame")
}

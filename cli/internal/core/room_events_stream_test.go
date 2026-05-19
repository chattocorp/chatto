package core

import (
	"context"
	"testing"
	"time"
)

func TestChattoCore_StreamRoomEventsLive(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space and room
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")

	// Create users and set up memberships
	user1, _ := core.CreateUser(ctx, "system", "user1", "user1", "password123")
	user2, _ := core.CreateUser(ctx, "system", "user2", "user2", "password123")
	core.JoinRoom(ctx, user1.Id, KindChannel, user1.Id, room.Id)
	core.JoinRoom(ctx, user2.Id, KindChannel, user2.Id, room.Id)

	// Start live streaming (should NOT receive historical events)
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	eventChan, err := core.StreamRoomEventsLive(streamCtx, KindChannel, room.Id)
	if err != nil {
		t.Fatalf("Failed to start streaming: %v", err)
	}

	// Give subscription time to be ready
	time.Sleep(50 * time.Millisecond)

	// Post a new message (should be received in live stream)
	// Note: Must be synchronous to ensure body storage completes before we check it
	_, err = core.PostMessage(ctx, KindChannel, room.Id, user1.Id, "Live message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	// Use select with timeout to prevent indefinite blocking
	select {
	case event := <-eventChan:
		if event.GetMessagePosted() == nil {
			t.Error("Expected MessagePosted event")
		}

		// Body is lazy-loaded, fetch it separately using messageBodyId
		messagePosted := event.GetMessagePosted()
		fetchedBody, err := core.GetMessageBody(ctx, KindForSpace(messagePosted.SpaceId), messagePosted.MessageBodyId)
		if err != nil {
			t.Fatalf("Failed to fetch message body: %v", err)
		}
		if fetchedBody != "Live message" {
			t.Errorf("Expected message 'Live message', got '%s'", fetchedBody)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Timeout waiting for live event")
	}
}

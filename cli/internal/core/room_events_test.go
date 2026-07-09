package core

import (
	"context"
	"fmt"
	"testing"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestChattoCore_GetRoomEvents(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space and room
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")

	// Create users and set up memberships
	user1, _ := core.CreateUser(ctx, "system", "user1", "user1", "password123")
	user2, _ := core.CreateUser(ctx, "system", "user2", "user2", "password123")
	core.JoinRoom(ctx, user1.Id, KindChannel, user1.Id, room.Id)
	core.JoinRoom(ctx, user2.Id, KindChannel, user2.Id, room.Id)

	// Post some messages
	core.PostMessage(ctx, KindChannel, room.Id, user1.Id, "Message 1", nil, "", "", nil, false)
	core.PostMessage(ctx, KindChannel, room.Id, user2.Id, "Message 2", nil, "", "", nil, false)
	core.PostMessage(ctx, KindChannel, room.Id, user1.Id, "Message 3", nil, "", "", nil, false)

	// Get room events (returns visible room events: messages + room membership + room lifecycle)
	eventsResult, err := core.GetRoomEvents(ctx, KindChannel, room.Id, 50, nil)
	if err != nil {
		t.Fatalf("Failed to get room events: %v", err)
	}
	events := eventsResult.Events

	// Should receive 6 RoomEvents total:
	// - 1 RoomCreated event
	// - 2 UserJoinedRoom events (user1 and user2)
	// - 3 MessagePosted events
	if len(events) != 6 {
		t.Errorf("Expected 6 room events (1 created + 2 joins + 3 messages), got %d", len(events))
	}

	// Count message events and verify bodies can be fetched
	messageCount := 0
	expectedBodies := []string{"Message 1", "Message 2", "Message 3"}
	for _, event := range events {
		if msg := event.GetMessagePosted(); msg != nil {
			// Body lookup is keyed by the durable event envelope id.
			fetchedBody, err := core.GetMessageBody(ctx, KindChannel, event.Id)
			if err != nil {
				t.Errorf("Failed to fetch message body: %v", err)
			}
			if fetchedBody != expectedBodies[messageCount] {
				t.Errorf("Expected body '%s', got '%s'", expectedBodies[messageCount], fetchedBody)
			}
			messageCount++
		}
	}

	if messageCount != 3 {
		t.Errorf("Expected 3 message events, got %d", messageCount)
	}
}

// TestChattoCore_GetRoomEvents_JoinAndLeaveEvents verifies that both UserJoinedRoom
// and UserLeftRoom events are stored in the stream and can be retrieved.
func TestChattoCore_GetRoomEvents_JoinAndLeaveEvents(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space and room
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")

	// Create a user
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")

	// User joins the room
	_, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}

	// User leaves the room
	err = core.LeaveRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to leave room: %v", err)
	}

	// Get room events - should include join and leave events.
	eventsResult, err := core.GetRoomEvents(ctx, KindChannel, room.Id, 50, nil)
	if err != nil {
		t.Fatalf("Failed to get room events: %v", err)
	}
	events := eventsResult.Events

	joinCount := 0
	leaveCount := 0
	roomCreatedCount := 0
	for _, event := range events {
		if event.GetUserJoinedRoom() != nil {
			joinCount++
		}
		if event.GetUserLeftRoom() != nil {
			leaveCount++
		}
		if event.GetRoomCreated() != nil {
			roomCreatedCount++
		}
	}

	if len(events) != 3 {
		t.Errorf("Expected 3 events (1 created + 1 join + 1 leave), got %d", len(events))
		for _, e := range events {
			t.Logf("Event type: %T", e.Event)
		}
	}

	if roomCreatedCount != 1 {
		t.Errorf("Expected 1 RoomCreated event, got %d", roomCreatedCount)
	}
	if joinCount != 1 {
		t.Errorf("Expected 1 UserJoinedRoom event, got %d", joinCount)
	}
	if leaveCount != 1 {
		t.Errorf("Expected 1 UserLeftRoom event, got %d", leaveCount)
	}
	if exists, err := core.RoomMembershipExists(ctx, KindChannel, user.Id, room.Id); err != nil || exists {
		t.Fatalf("RoomMembershipExists after leave = %v, %v; want false, nil", exists, err)
	}
}

func TestChattoCore_GetRoomEvents_RoomLifecycleCommandsAreImmediatelyVisible(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "lifecycle-room", "Original description")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}
	assertLatestRoomEvent(t, core, ctx, KindChannel, room.Id, func(event *corev1.Event) bool {
		created := event.GetRoomCreated()
		return created != nil && created.RoomId == room.Id
	})

	if _, err := core.UpdateRoom(ctx, "test-user", KindChannel, room.Id, "renamed-lifecycle-room", "Updated description"); err != nil {
		t.Fatalf("Failed to update room: %v", err)
	}
	assertLatestRoomEvent(t, core, ctx, KindChannel, room.Id, func(event *corev1.Event) bool {
		updated := event.GetRoomUpdated()
		return updated != nil && updated.RoomId == room.Id && updated.Name == "renamed-lifecycle-room"
	})

	if _, err := core.ArchiveRoom(ctx, "test-user", KindChannel, room.Id); err != nil {
		t.Fatalf("Failed to archive room: %v", err)
	}
	assertLatestRoomEvent(t, core, ctx, KindChannel, room.Id, func(event *corev1.Event) bool {
		archived := event.GetRoomArchived()
		return archived != nil && archived.RoomId == room.Id
	})

	if _, err := core.UnarchiveRoom(ctx, "test-user", KindChannel, room.Id); err != nil {
		t.Fatalf("Failed to unarchive room: %v", err)
	}
	assertLatestRoomEvent(t, core, ctx, KindChannel, room.Id, func(event *corev1.Event) bool {
		unarchived := event.GetRoomUnarchived()
		return unarchived != nil && unarchived.RoomId == room.Id
	})

	if err := core.DeleteRoom(ctx, "test-user", KindChannel, room.Id); err != nil {
		t.Fatalf("Failed to delete room: %v", err)
	}
	assertLatestRoomEvent(t, core, ctx, KindChannel, room.Id, func(event *corev1.Event) bool {
		deleted := event.GetRoomDeleted()
		return deleted != nil && deleted.RoomId == room.Id
	})
}

// TestChattoCore_GetRoomEvents_JoinAfterLastMessage verifies that join events
// are visible even when a user joins an inactive room (after the last message).
// This is a regression test for a bug where GetRoomEvents used lastMsgAt as the
// end time, causing join events after that time to be excluded.
func TestChattoCore_GetRoomEvents_JoinAfterLastMessage(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space and room
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")

	// Create user1 and have them post a message
	user1, _ := core.CreateUser(ctx, "system", "user1", "user1", "password123")
	_, err := core.JoinRoom(ctx, user1.Id, KindChannel, user1.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}
	_, err = core.PostMessage(ctx, KindChannel, room.Id, user1.Id, "Last message before new user joins", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	// Simulate time passing (the room becomes "inactive")
	// In production, this could be hours or days

	// Create user2 and have them join the room AFTER the last message
	user2, _ := core.CreateUser(ctx, "system", "user2", "user2", "password123")
	_, err = core.JoinRoom(ctx, user2.Id, KindChannel, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}

	// Get room events - should include user2's join event even though it
	// happened after the last message.
	eventsResult, err := core.GetRoomEvents(ctx, KindChannel, room.Id, 50, nil)
	if err != nil {
		t.Fatalf("Failed to get room events: %v", err)
	}
	events := eventsResult.Events

	foundUser2Join := false
	for _, event := range events {
		joinEvent := event.GetUserJoinedRoom()
		if joinEvent != nil && event.ActorId == user2.Id {
			foundUser2Join = true
			break
		}
	}

	if !foundUser2Join {
		t.Errorf("User2's join event not found - this is a regression!")
		t.Logf("Total events: %d", len(events))
		for _, e := range events {
			t.Logf("Event: actorId=%s type=%T", e.ActorId, e.Event)
		}
	}
}

func TestChattoCore_GetRoomEvents_DeletedMessageBody(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")

	// Join space and room (required for posting messages)
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	// Post a message
	messageBody := "This message will be deleted"
	roomEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, messageBody, nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	// Get the message ID
	postedMessage := roomEvent.GetMessagePosted()
	if postedMessage == nil {
		t.Fatal("Event should be a MessagePosted event")
	}

	// Delete the message body using the event envelope id (author can delete own messages)
	err = core.DeleteMessage(ctx, user.Id, KindChannel, room.Id, roomEvent.Id)
	if err != nil {
		t.Fatalf("Failed to delete message: %v", err)
	}

	// Get room events
	eventsResult, err := core.GetRoomEvents(ctx, KindChannel, room.Id, 50, nil)
	if err != nil {
		t.Fatalf("Failed to get room events: %v", err)
	}
	events := eventsResult.Events

	// Find the MessagePosted event
	var messageEvent *corev1.Event
	for _, event := range events {
		if event.GetMessagePosted() != nil {
			messageEvent = event.Event
			break
		}
	}

	if messageEvent == nil {
		t.Fatal("Expected to find MessagePosted event")
	}

	messagePosted := messageEvent.GetMessagePosted()

	// Verify the body is empty (deleted) when fetched via GetMessageBody
	fetchedBody, err := core.GetMessageBody(ctx, KindChannel, messageEvent.Id)
	if err != nil {
		t.Fatalf("Failed to fetch message body: %v", err)
	}
	if fetchedBody != "" {
		t.Errorf("Expected empty body for deleted message, got '%s'", fetchedBody)
	}

	// Verify other metadata is still present (audit trail)
	if messagePosted.RoomId != room.Id {
		t.Errorf("RoomId = %s, want %s", messagePosted.RoomId, room.Id)
	}
	if messageEvent.ActorId != user.Id {
		t.Errorf("ActorId = %s, want %s", messageEvent.ActorId, user.Id)
	}
}

func TestChattoCore_GetRoomEvents_Pagination(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	t.Run("returns newest messages when more than limit exist", func(t *testing.T) {
		// Post 100 messages
		for i := 1; i <= 100; i++ {
			_, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, fmt.Sprintf("Message %d", i), nil, "", "", nil, false)
			if err != nil {
				t.Fatalf("Failed to post message %d: %v", i, err)
			}
		}

		// Request 50 messages (default limit) - should return the 50 newest
		eventsResult, err := core.GetRoomEvents(ctx, KindChannel, room.Id, 50, nil)
		if err != nil {
			t.Fatalf("Failed to get room events: %v", err)
		}
		events := eventsResult.Events

		// Count MessagePosted events
		var messageEvents []*corev1.Event
		for _, event := range events {
			if event.GetMessagePosted() != nil {
				messageEvents = append(messageEvents, event.Event)
			}
		}

		if len(messageEvents) != 50 {
			t.Errorf("Expected 50 message events, got %d", len(messageEvents))
		}

		// The last message in our result should be "Message 100" (most recent)
		lastMsgBody, err := core.GetMessageBody(ctx, KindChannel, messageEvents[len(messageEvents)-1].Id)
		if err != nil {
			t.Fatalf("Failed to get last message body: %v", err)
		}
		if lastMsgBody != "Message 100" {
			t.Errorf("Expected last message body to be 'Message 100', got '%s'", lastMsgBody)
		}

		// The first message in our result should be "Message 51" (51st newest)
		firstMsgBody, err := core.GetMessageBody(ctx, KindChannel, messageEvents[0].Id)
		if err != nil {
			t.Fatalf("Failed to get first message body: %v", err)
		}
		if firstMsgBody != "Message 51" {
			t.Errorf("Expected first message body to be 'Message 51', got '%s'", firstMsgBody)
		}
	})

	t.Run("sequence-based pagination returns correct range", func(t *testing.T) {
		// Get the 50 newest messages first (to get a cursor)
		eventsResult, err := core.GetRoomEvents(ctx, KindChannel, room.Id, 50, nil)
		if err != nil {
			t.Fatalf("Failed to get room events: %v", err)
		}
		events := eventsResult.Events

		// Use the earliest event's sequence as the pagination cursor.
		// Events come back in chronological order, so events[0] is oldest.
		if len(events) == 0 {
			t.Fatal("expected at least one event in first batch")
		}
		earliestSeq := events[0].Sequence

		// Now fetch older messages using sequence cursor
		olderEventsResult, err := core.GetRoomEvents(ctx, KindChannel, room.Id, 50, &earliestSeq)
		if err != nil {
			t.Fatalf("Failed to get older room events: %v", err)
		}
		olderEvents := olderEventsResult.Events

		// Count MessagePosted events in the older batch
		var olderMessageEvents []*corev1.Event
		for _, event := range olderEvents {
			if event.GetMessagePosted() != nil {
				olderMessageEvents = append(olderMessageEvents, event.Event)
			}
		}

		// Should get at least some messages (1-50 range)
		// Note: Room creation and user join events may also be included
		if len(olderMessageEvents) == 0 {
			t.Error("Expected to get some older message events")
		}

		// The newest message in the older batch should be before our cursor
		if len(olderMessageEvents) > 0 {
			newestOldBody, err := core.GetMessageBody(ctx, KindChannel, olderMessageEvents[len(olderMessageEvents)-1].Id)
			if err != nil {
				t.Fatalf("Failed to get newest old message body: %v", err)
			}
			// Should be Message 50 or earlier
			if newestOldBody == "Message 51" || newestOldBody == "Message 52" {
				t.Errorf("Expected newest old message to be Message 50 or earlier, got '%s'", newestOldBody)
			}
		}
	})
}

// TestChattoCore_GetRoomEvents_NoMessagesYet verifies that GetRoomEvents returns
// events even when no messages have been posted yet (only non-message events like
// room creation or join events exist). This was a regression - the time-based
// pagination would return empty if GetRoomLastMessageAt returned zero.
func TestChattoCore_GetRoomEvents_NoMessagesYet(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space and user

	user, err := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create a new room (this adds the creator as a member and publishes a RoomCreated event)
	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "new-room", "A fresh room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Don't post any messages - room has only the RoomCreated event

	// Get room events - should return events even without messages
	eventsResult, err := core.GetRoomEvents(ctx, KindChannel, room.Id, 50, nil)
	if err != nil {
		t.Fatalf("Failed to get room events: %v", err)
	}
	events := eventsResult.Events

	// Should have at least 1 event (the RoomCreated event)
	if len(events) == 0 {
		t.Fatal("Expected at least 1 event, got 0")
	}

	// Verify we got a RoomCreated event for this room
	var foundRoomCreatedEvent bool
	for _, event := range events {
		if created := event.GetRoomCreated(); created != nil && created.RoomId == room.Id {
			foundRoomCreatedEvent = true
			if event.ActorId != user.Id {
				t.Errorf("Expected room created event actor to be %s, got %s", user.Id, event.ActorId)
			}
			break
		}
	}

	if !foundRoomCreatedEvent {
		t.Error("Expected to find RoomCreated event for the new room")
	}
}

func TestChattoCore_GetRoomEventByEventID(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user

	user, err := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "test-room", "A test room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	t.Run("lookup root message by event ID", func(t *testing.T) {
		// Post a root message
		postedEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Hello, world!", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post message: %v", err)
		}

		messagePosted := postedEvent.GetMessagePosted()
		if messagePosted == nil {
			t.Fatal("Expected MessagePosted event")
		}

		// Look up by event ID (use wrapper's Id, not inner EventId)
		foundEvent, err := core.GetRoomEventByEventID(ctx, KindChannel, room.Id, postedEvent.Id)
		if err != nil {
			t.Fatalf("GetRoomEventByEventID failed: %v", err)
		}

		if foundEvent == nil {
			t.Fatal("Expected to find event, got nil")
		}

		// Verify it's the same message
		foundMessage := foundEvent.GetMessagePosted()
		if foundMessage == nil {
			t.Fatal("Expected found event to be MessagePosted")
		}

		if foundEvent.Id != postedEvent.Id {
			t.Errorf("Event ID mismatch: got %s, want %s", foundEvent.Id, postedEvent.Id)
		}

		if foundMessage.RoomId != messagePosted.RoomId {
			t.Errorf("RoomId mismatch: got %s, want %s", foundMessage.RoomId, messagePosted.RoomId)
		}

		if foundEvent.Id != postedEvent.Id {
			t.Errorf("Event ID mismatch: got %s, want %s", foundEvent.Id, postedEvent.Id)
		}
	})

	t.Run("lookup non-existent event ID returns nil", func(t *testing.T) {
		event, err := core.GetRoomEventByEventID(ctx, KindChannel, room.Id, "nonexistent123")
		if err != nil {
			t.Fatalf("GetRoomEventByEventID should not error for non-existent: %v", err)
		}

		if event != nil {
			t.Error("Expected nil for non-existent event ID")
		}
	})

	t.Run("lookup in wrong room returns nil", func(t *testing.T) {
		// Post a message in the first room
		postedEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Room 1 message", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post message: %v", err)
		}

		// Create a second room
		room2, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "test-room-2", "Another test room")
		if err != nil {
			t.Fatalf("Failed to create room 2: %v", err)
		}

		// Try to look up the message using room2's context - should return nil
		event, err := core.GetRoomEventByEventID(ctx, KindChannel, room2.Id, postedEvent.Id)
		if err != nil {
			t.Fatalf("GetRoomEventByEventID should not error for wrong room: %v", err)
		}

		if event != nil {
			t.Error("Expected nil when looking up event in wrong room")
		}
	})
}

func assertLatestRoomEvent(
	t *testing.T,
	core *ChattoCore,
	ctx context.Context,
	kind RoomKind,
	roomID string,
	matches func(*corev1.Event) bool,
) {
	t.Helper()

	result, err := core.GetRoomEvents(ctx, kind, roomID, 50, nil)
	if err != nil {
		t.Fatalf("GetRoomEvents: %v", err)
	}
	if len(result.Events) == 0 {
		t.Fatal("expected at least one room event")
	}

	latest := result.Events[len(result.Events)-1]
	if !matches(latest.Event) {
		t.Fatalf("latest event = %T, want requested lifecycle event", latest.Event.GetEvent())
	}
}

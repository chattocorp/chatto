package core

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestChattoCore_AddReaction(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user first (required for encryption key)
	user, err := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Create space and room
	room, _ := core.CreateRoom(ctx, user.Id, KindChannel, "", "test-room", "Test room")

	// Join room before posting
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	// Post a message to react to
	event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Hello world", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage failed: %v", err)
	}
	eventID := event.Id

	t.Run("add new reaction", func(t *testing.T) {
		added, err := core.AddReaction(ctx, KindChannel, room.Id, eventID, "thumbsup", user.Id)
		if err != nil {
			t.Fatalf("AddReaction failed: %v", err)
		}
		if !added {
			t.Error("Expected reaction to be added (return true)")
		}
	})

	t.Run("add duplicate reaction returns false", func(t *testing.T) {
		// Try to add the same reaction again
		added, err := core.AddReaction(ctx, KindChannel, room.Id, eventID, "thumbsup", user.Id)
		if err != nil {
			t.Fatalf("AddReaction failed: %v", err)
		}
		if added {
			t.Error("Expected duplicate reaction to return false")
		}
	})

	t.Run("different users can add same emoji", func(t *testing.T) {
		added, err := core.AddReaction(ctx, KindChannel, room.Id, eventID, "thumbsup", "other-user")
		if err != nil {
			t.Fatalf("AddReaction failed: %v", err)
		}
		if !added {
			t.Error("Expected different user's reaction to be added")
		}
	})

	t.Run("same user can add different emoji", func(t *testing.T) {
		added, err := core.AddReaction(ctx, KindChannel, room.Id, eventID, "heart", user.Id)
		if err != nil {
			t.Fatalf("AddReaction failed: %v", err)
		}
		if !added {
			t.Error("Expected different emoji reaction to be added")
		}
	})

	t.Run("add reaction with unicode emoji is rejected", func(t *testing.T) {
		_, err := core.AddReaction(ctx, KindChannel, room.Id, eventID, "🎉", user.Id)
		if err == nil {
			t.Error("Expected error when adding reaction with Unicode emoji")
		}
	})

	t.Run("add reaction with invalid input", func(t *testing.T) {
		_, err := core.AddReaction(ctx, KindChannel, room.Id, eventID, "not_valid", user.Id)
		if err == nil {
			t.Error("Expected error for invalid emoji input")
		}
	})
}

func TestChattoCore_AddReactionConcurrentDuplicate(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, room, eventID := setupReactionTest(t, core, ctx)

	var addedCount atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			added, err := core.AddReaction(ctx, KindChannel, room.Id, eventID, "thumbsup", user.Id)
			if err != nil {
				t.Errorf("AddReaction failed: %v", err)
				return
			}
			if added {
				addedCount.Add(1)
			}
		}()
	}
	wg.Wait()

	if got := addedCount.Load(); got != 1 {
		t.Fatalf("concurrent duplicate adds returned %d successes, want 1", got)
	}
	reactions, err := core.GetReactions(ctx, eventID)
	if err != nil {
		t.Fatalf("GetReactions failed: %v", err)
	}
	if len(reactions) != 1 || reactions[0].Emoji != "thumbsup" || len(reactions[0].UserIDs) != 1 || reactions[0].UserIDs[0] != user.Id {
		t.Fatalf("unexpected reactions after concurrent duplicate add: %+v", reactions)
	}
}

func TestChattoCore_AddReactionRefreshesStaleNoopSnapshot(t *testing.T) {
	harness := newTestEventHarness(t)
	ctx := testContext(t)

	reactions := NewReactionProjection()
	reactionsProjector := harness.projector(reactions)
	core := &ChattoCore{
		logger:             testCoreLogger(),
		EventPublisher:     harness.publisher,
		Reactions:          reactions,
		ReactionsProjector: reactionsProjector,
	}
	core.roomModel = newRoomModel(nil, nil, nil, nil, nil, nil, nil, nil, reactions, reactionsProjector)

	addedOnOtherReplica := newReactionAddedEvent("U1", "R1", "M1", "thumbsup")
	addSubject := events.RoomAggregate("R1").SubjectFor(addedOnOtherReplica)
	addSeq, err := harness.publisher.AppendEventually(ctx, addSubject, addedOnOtherReplica)
	if err != nil {
		t.Fatalf("append existing reaction: %v", err)
	}
	if err := reactions.Apply(addedOnOtherReplica, addSeq); err != nil {
		t.Fatalf("seed stale reaction projection: %v", err)
	}

	removedOnOtherReplica := newReactionRemovedEvent("U1", "R1", "M1", "thumbsup")
	removeSubject := events.RoomAggregate("R1").SubjectFor(removedOnOtherReplica)
	if _, err := harness.publisher.AppendEventually(ctx, removeSubject, removedOnOtherReplica); err != nil {
		t.Fatalf("append remote reaction removal: %v", err)
	}
	if !reactions.ReactionMutationSnapshot("R1", "M1", "thumbsup", "U1").Exists {
		t.Fatal("test setup expected stale projection to still contain reaction")
	}

	type result struct {
		added bool
		err   error
	}
	resultCh := make(chan result, 1)
	go func() {
		added, err := core.publishReactionMutation(
			ctx,
			KindChannel,
			"R1",
			"M1",
			"thumbsup",
			"U1",
			newReactionAddedEvent("U1", "R1", "M1", "thumbsup"),
		)
		resultCh <- result{added: added, err: err}
	}()

	select {
	case got := <-resultCh:
		t.Fatalf("AddReaction returned before stale projection catch-up: added=%v err=%v", got.added, got.err)
	case <-time.After(25 * time.Millisecond):
	}

	startTestProjector(t, reactionsProjector)
	got := <-resultCh
	if got.err != nil {
		t.Fatalf("AddReaction after projection catch-up: %v", got.err)
	}
	if !got.added {
		t.Fatal("AddReaction returned false from stale no-op snapshot after catch-up")
	}
	if snapshot := reactions.ReactionMutationSnapshot("R1", "M1", "thumbsup", "U1"); !snapshot.Exists {
		t.Fatal("reaction projection should contain re-added reaction")
	}
}

func TestChattoCore_RemoveReaction(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user first (required for encryption key)
	user, err := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Create space and room
	room, _ := core.CreateRoom(ctx, user.Id, KindChannel, "", "test-room", "Test room")

	// Join room before posting
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	// Post a message
	event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Hello world", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage failed: %v", err)
	}
	eventID := event.Id

	// Add a reaction first
	_, err = core.AddReaction(ctx, KindChannel, room.Id, eventID, "thumbsup", user.Id)
	if err != nil {
		t.Fatalf("AddReaction failed: %v", err)
	}

	t.Run("remove existing reaction", func(t *testing.T) {
		removed, err := core.RemoveReaction(ctx, KindChannel, room.Id, eventID, "thumbsup", user.Id)
		if err != nil {
			t.Fatalf("RemoveReaction failed: %v", err)
		}
		if !removed {
			t.Error("Expected reaction to be removed (return true)")
		}
	})

	t.Run("remove non-existent reaction returns false", func(t *testing.T) {
		removed, err := core.RemoveReaction(ctx, KindChannel, room.Id, eventID, "thumbsup", user.Id)
		if err != nil {
			t.Fatalf("RemoveReaction failed: %v", err)
		}
		if removed {
			t.Error("Expected removing non-existent reaction to return false")
		}
	})

	t.Run("remove reaction that was never added", func(t *testing.T) {
		removed, err := core.RemoveReaction(ctx, KindChannel, room.Id, eventID, "tada", user.Id)
		if err != nil {
			t.Fatalf("RemoveReaction failed: %v", err)
		}
		if removed {
			t.Error("Expected removing never-added reaction to return false")
		}
	})

	t.Run("remove reaction with unicode emoji is rejected", func(t *testing.T) {
		_, err := core.RemoveReaction(ctx, KindChannel, room.Id, eventID, "🚀", user.Id)
		if err == nil {
			t.Error("Expected error when removing reaction with Unicode emoji")
		}
	})
}

func TestChattoCore_GetReactions(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user first (required for encryption key)
	user, err := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	// Create space and room
	room, _ := core.CreateRoom(ctx, user.Id, KindChannel, "", "test-room", "Test room")

	// Join room before posting
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	// Post a message
	event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Hello world", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage failed: %v", err)
	}
	eventID := event.Id

	t.Run("empty reactions", func(t *testing.T) {
		reactions, err := core.GetReactions(ctx, eventID)
		if err != nil {
			t.Fatalf("GetReactions failed: %v", err)
		}
		if len(reactions) != 0 {
			t.Errorf("Expected 0 reactions, got %d", len(reactions))
		}
	})

	// Add some reactions
	core.AddReaction(ctx, KindChannel, room.Id, eventID, "thumbsup", "user1")
	core.AddReaction(ctx, KindChannel, room.Id, eventID, "thumbsup", "user2")
	core.AddReaction(ctx, KindChannel, room.Id, eventID, "heart", "user1")

	t.Run("get aggregated reactions", func(t *testing.T) {
		reactions, err := core.GetReactions(ctx, eventID)
		if err != nil {
			t.Fatalf("GetReactions failed: %v", err)
		}
		if len(reactions) != 2 {
			t.Errorf("Expected 2 unique emoji, got %d", len(reactions))
		}

		// Find the thumbs up reaction (returned as shortcode names)
		var thumbsUp *ReactionSummary
		var heart *ReactionSummary
		for i := range reactions {
			if reactions[i].Emoji == "thumbsup" {
				thumbsUp = &reactions[i]
			}
			if reactions[i].Emoji == "heart" {
				heart = &reactions[i]
			}
		}

		if thumbsUp == nil {
			t.Fatal("Expected thumbs up reaction")
		}
		if len(thumbsUp.UserIDs) != 2 {
			t.Errorf("Expected 2 users for thumbs up, got %d", len(thumbsUp.UserIDs))
		}

		if heart == nil {
			t.Fatal("Expected heart reaction")
		}
		if len(heart.UserIDs) != 1 {
			t.Errorf("Expected 1 user for heart, got %d", len(heart.UserIDs))
		}
	})

	t.Run("reactions isolated to message", func(t *testing.T) {
		// Post another message
		event2, _ := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Another message", nil, "", "", nil, false)
		eventID2 := event2.Id

		// Check that the new message has no reactions
		reactions, err := core.GetReactions(ctx, eventID2)
		if err != nil {
			t.Fatalf("GetReactions failed: %v", err)
		}
		if len(reactions) != 0 {
			t.Errorf("Expected 0 reactions for new message, got %d", len(reactions))
		}
	})
}

func TestChattoCore_GetReactionsBatch(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	room, _ := core.CreateRoom(ctx, user.Id, KindChannel, "", "test-room", "Test room")
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	// Post two messages
	event1, _ := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Message 1", nil, "", "", nil, false)
	event2, _ := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Message 2", nil, "", "", nil, false)

	// Add reactions to message 1
	core.AddReaction(ctx, KindChannel, room.Id, event1.Id, "thumbsup", user.Id)
	core.AddReaction(ctx, KindChannel, room.Id, event1.Id, "heart", "user2")

	// Add reaction to message 2
	core.AddReaction(ctx, KindChannel, room.Id, event2.Id, "tada", user.Id)

	t.Run("batch fetch returns reactions for multiple messages", func(t *testing.T) {
		result, err := core.GetReactionsBatch(ctx, []string{event1.Id, event2.Id})
		if err != nil {
			t.Fatalf("GetReactionsBatch failed: %v", err)
		}

		// Message 1 should have 2 emoji types
		msg1Reactions := result[event1.Id]
		if len(msg1Reactions) != 2 {
			t.Errorf("Expected 2 reaction types for message 1, got %d", len(msg1Reactions))
		}

		// Message 2 should have 1 emoji type
		msg2Reactions := result[event2.Id]
		if len(msg2Reactions) != 1 {
			t.Errorf("Expected 1 reaction type for message 2, got %d", len(msg2Reactions))
		}
		if len(msg2Reactions) > 0 && msg2Reactions[0].Emoji != "tada" {
			t.Errorf("Expected tada emoji for message 2, got %q", msg2Reactions[0].Emoji)
		}
	})

	t.Run("batch fetch with no reactions returns empty map", func(t *testing.T) {
		event3, _ := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Message 3", nil, "", "", nil, false)
		result, err := core.GetReactionsBatch(ctx, []string{event3.Id})
		if err != nil {
			t.Fatalf("GetReactionsBatch failed: %v", err)
		}

		if len(result[event3.Id]) != 0 {
			t.Errorf("Expected 0 reactions for message 3, got %d", len(result[event3.Id]))
		}
	})

	t.Run("batch fetch with empty event IDs", func(t *testing.T) {
		result, err := core.GetReactionsBatch(ctx, []string{})
		if err != nil {
			t.Fatalf("GetReactionsBatch failed: %v", err)
		}
		if len(result) != 0 {
			t.Errorf("Expected empty result for empty event IDs, got %d entries", len(result))
		}
	})

	t.Run("batch fetch isolates reactions between messages", func(t *testing.T) {
		// Verify that message 2's reactions don't bleed into message 1 and vice versa
		result, err := core.GetReactionsBatch(ctx, []string{event1.Id, event2.Id})
		if err != nil {
			t.Fatalf("GetReactionsBatch failed: %v", err)
		}

		// Check message 1 doesn't have tada
		for _, r := range result[event1.Id] {
			if r.Emoji == "tada" {
				t.Error("Message 1 should not have tada reaction")
			}
		}

		// Check message 2 doesn't have thumbsup or heart
		for _, r := range result[event2.Id] {
			if r.Emoji == "thumbsup" || r.Emoji == "heart" {
				t.Errorf("Message 2 should not have %s reaction", r.Emoji)
			}
		}
	})
}

func TestChattoCore_EchoReactionsCanonicalizeToOriginal(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}

	room, _ := core.CreateRoom(ctx, user.Id, KindChannel, "", "test-room", "Test room")
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	// Post a root message, then a thread reply with "also send to channel" to create an echo
	rootEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Root message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage (root) failed: %v", err)
	}

	replyEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Thread reply", nil, rootEvent.Id, rootEvent.Id, nil, true)
	if err != nil {
		t.Fatalf("PostMessage (reply+echo) failed: %v", err)
	}

	// Find the echo event in the room events
	result, err := core.GetRoomEvents(ctx, KindChannel, room.Id, 50, nil)
	if err != nil {
		t.Fatalf("GetRoomEvents failed: %v", err)
	}

	var echoEventID string
	for _, ev := range result.Events {
		if msg := ev.GetMessagePosted(); msg != nil && msg.EchoOfEventId == replyEvent.Id {
			echoEventID = ev.Id
			break
		}
	}
	if echoEventID == "" {
		t.Fatal("Echo event not found in room events")
	}

	added, err := core.AddReaction(ctx, KindChannel, room.Id, echoEventID, "thumbsup", user.Id)
	if err != nil {
		t.Fatalf("AddReaction on echo failed: %v", err)
	}
	if !added {
		t.Fatal("AddReaction on echo added = false, want true")
	}

	reactions, err := core.GetReactions(ctx, replyEvent.Id)
	if err != nil {
		t.Fatalf("GetReactions on original failed: %v", err)
	}
	if len(reactions) != 1 || reactions[0].Emoji != "thumbsup" || len(reactions[0].UserIDs) != 1 || reactions[0].UserIDs[0] != user.Id {
		t.Fatalf("original reactions = %+v, want thumbsup by user", reactions)
	}

	echoReactions, err := core.GetReactions(ctx, echoEventID)
	if err != nil {
		t.Fatalf("GetReactions on echo failed: %v", err)
	}
	if len(echoReactions) != 1 || echoReactions[0].Emoji != "thumbsup" || len(echoReactions[0].UserIDs) != 1 || echoReactions[0].UserIDs[0] != user.Id {
		t.Fatalf("echo reactions = %+v, want same thumbsup by user", echoReactions)
	}

	batch, err := core.GetReactionsBatch(ctx, []string{replyEvent.Id, echoEventID})
	if err != nil {
		t.Fatalf("GetReactionsBatch failed: %v", err)
	}
	if len(batch[replyEvent.Id]) != 1 || len(batch[echoEventID]) != 1 || batch[replyEvent.Id][0].Emoji != batch[echoEventID][0].Emoji {
		t.Fatalf("batch reactions = %+v, want matching original and echo entries", batch)
	}

	added, err = core.AddReaction(ctx, KindChannel, room.Id, replyEvent.Id, "thumbsup", user.Id)
	if err != nil {
		t.Fatalf("duplicate AddReaction via original failed: %v", err)
	}
	if added {
		t.Fatal("duplicate AddReaction via original added = true, want false")
	}

	removed, err := core.RemoveReaction(ctx, KindChannel, room.Id, echoEventID, "thumbsup", user.Id)
	if err != nil {
		t.Fatalf("RemoveReaction via echo failed: %v", err)
	}
	if !removed {
		t.Fatal("RemoveReaction via echo removed = false, want true")
	}

	reactions, err = core.GetReactions(ctx, replyEvent.Id)
	if err != nil {
		t.Fatalf("GetReactions after remove failed: %v", err)
	}
	if len(reactions) != 0 {
		t.Fatalf("original reactions after remove = %+v, want none", reactions)
	}
}

func TestReactionKey(t *testing.T) {
	t.Run("reactionKey format", func(t *testing.T) {
		key := reactionKey("E1a2b3c4d5e6f7g", "thumbsup", "user1")
		// Key should be: E1a2b3c4d5e6f7g.thumbsup.user1
		expected := "E1a2b3c4d5e6f7g.thumbsup.user1"
		if key != expected {
			t.Errorf("Key format incorrect: got %q, want %q", key, expected)
		}
	})

	t.Run("roundtrip", func(t *testing.T) {
		// Create a key and parse it back
		originalEventID := "E9z8y7x6w5v4u3t"
		originalEmojiName := "heart"
		originalUserID := "user2"

		key := reactionKey(originalEventID, originalEmojiName, originalUserID)
		eventID, emojiName, userID, err := parseReactionKey(key)
		if err != nil {
			t.Fatalf("parseReactionKey failed: %v", err)
		}
		if eventID != originalEventID {
			t.Errorf("Expected eventID %q, got %q", originalEventID, eventID)
		}
		if emojiName != originalEmojiName {
			t.Errorf("Expected emojiName %q, got %q", originalEmojiName, emojiName)
		}
		if userID != originalUserID {
			t.Errorf("Expected userID %q, got %q", originalUserID, userID)
		}
	})

	t.Run("parseReactionKey invalid format", func(t *testing.T) {
		_, _, _, err := parseReactionKey("invalid")
		if err == nil {
			t.Error("Expected error for invalid key format")
		}
	})
}

func TestEmoji(t *testing.T) {
	t.Run("IsValidEmojiName accepts known names", func(t *testing.T) {
		valid := []string{"thumbsup", "+1", "heart", "joy", "tada", "rocket", "fire", "grinning", "t-rex", "melting_face", "saluting_face"}
		for _, name := range valid {
			if !IsValidEmojiName(name) {
				t.Errorf("Expected %q to be valid", name)
			}
		}
	})

	t.Run("IsValidEmojiName rejects unknown names", func(t *testing.T) {
		invalid := []string{"invalid", "foobar", "not_an_emoji", ""}
		for _, name := range invalid {
			if IsValidEmojiName(name) {
				t.Errorf("Expected %q to be invalid", name)
			}
		}
	})

	t.Run("resolveEmojiInput accepts shortcode names", func(t *testing.T) {
		name, err := resolveEmojiInput("thumbsup")
		if err != nil {
			t.Fatalf("resolveEmojiInput failed: %v", err)
		}
		if name != "thumbsup" {
			t.Errorf("Expected 'thumbsup', got %q", name)
		}
	})

	t.Run("resolveEmojiInput rejects unicode emoji", func(t *testing.T) {
		_, err := resolveEmojiInput("👍")
		if err == nil {
			t.Error("Expected error when passing Unicode emoji to resolveEmojiInput")
		}
	})

	t.Run("resolveEmojiInput rejects invalid input", func(t *testing.T) {
		_, err := resolveEmojiInput("totally_bogus")
		if err == nil {
			t.Error("Expected error for invalid input")
		}
	})
}

func setupReactionTest(t *testing.T, core *ChattoCore, ctx context.Context) (*corev1.User, *corev1.Room, string) {
	t.Helper()
	user, err := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "test-room", "Test room")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom failed: %v", err)
	}
	event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Hello world", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage failed: %v", err)
	}
	return user, room, event.Id
}

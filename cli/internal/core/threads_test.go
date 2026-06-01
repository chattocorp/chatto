package core

import (
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestChattoCore_PostMessage_Threading(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	t.Run("root message has empty inReplyTo", func(t *testing.T) {
		event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Root message", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root message: %v", err)
		}

		msg := event.GetMessagePosted()
		if msg == nil {
			t.Fatal("Expected MessagePosted event")
		}
		if msg.InReplyTo != "" {
			t.Errorf("Root message should have empty InReplyTo, got %q", msg.InReplyTo)
		}
	})

	t.Run("thread reply has parent event ID", func(t *testing.T) {
		// Post a root message first
		rootEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Thread root", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root message: %v", err)
		}
		rootEventID := rootEvent.Id

		// Post a reply to the root message
		replyEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Thread reply", nil, rootEventID, "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post reply: %v", err)
		}

		reply := replyEvent.GetMessagePosted()
		if reply == nil {
			t.Fatal("Expected MessagePosted event for reply")
		}
		if reply.InThread != rootEventID {
			t.Errorf("Reply should have InThread=%q, got %q", rootEventID, reply.InThread)
		}
		if reply.InReplyTo != "" {
			t.Errorf("Reply should have empty InReplyTo (no attribution), got %q", reply.InReplyTo)
		}
	})

	t.Run("thread replies are excluded from GetRoomEvents", func(t *testing.T) {
		// GetRoomEvents should only return root messages, not thread replies
		// Thread replies are retrieved separately via GetThreadEvents
		eventsResult, err := core.GetRoomEvents(ctx, KindChannel, room.Id, 50, nil)
		if err != nil {
			t.Fatalf("Failed to get room events: %v", err)
		}
		events := eventsResult.Events

		var rootCount, replyCount int
		for _, event := range events {
			if msg := event.GetMessagePosted(); msg != nil {
				if msg.InReplyTo == "" {
					rootCount++
				} else {
					replyCount++
				}
			}
		}

		if rootCount == 0 {
			t.Error("Expected at least one root message")
		}
		if replyCount != 0 {
			t.Errorf("Expected no thread replies in GetRoomEvents, got %d", replyCount)
		}
	})

	t.Run("GetThreadEvents returns root and replies", func(t *testing.T) {
		// Post a new root message
		rootEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Thread root for GetThreadEvents test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root message: %v", err)
		}
		rootEventID := rootEvent.Id

		// Post multiple replies
		for i := 1; i <= 3; i++ {
			_, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, fmt.Sprintf("Reply %d", i), nil, rootEventID, "", nil, false)
			if err != nil {
				t.Fatalf("Failed to post reply %d: %v", i, err)
			}
		}

		// Fetch thread events
		threadEvents, err := core.GetThreadEvents(ctx, KindChannel, room.Id, rootEventID)
		if err != nil {
			t.Fatalf("Failed to get thread events: %v", err)
		}

		// Should have 4 events: 1 root + 3 replies
		if len(threadEvents) != 4 {
			t.Errorf("Expected 4 thread events, got %d", len(threadEvents))
		}

		// First event should be the root
		if threadEvents[0].Id != rootEventID {
			t.Errorf("First event should be root (id %s), got id %s", rootEventID, threadEvents[0].Id)
		}

		// All subsequent events should be in the thread
		for i := 1; i < len(threadEvents); i++ {
			msg := threadEvents[i].GetMessagePosted()
			if msg == nil {
				t.Errorf("Event %d should be a MessagePosted event", i)
				continue
			}
			if msg.InThread != rootEventID {
				t.Errorf("Event %d should have InThread=%q, got %q", i, rootEventID, msg.InThread)
			}
		}
	})

	t.Run("GetThreadMetadata returns reply count and last reply time", func(t *testing.T) {
		// Post a new root message
		rootEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Thread root for metadata test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root message: %v", err)
		}
		rootEventID := rootEvent.Id

		// Initially, no replies
		metadata, err := core.GetThreadMetadata(ctx, KindChannel, room.Id, rootEventID)
		if err != nil {
			t.Fatalf("Failed to get thread metadata: %v", err)
		}
		if metadata.ReplyCount != 0 {
			t.Errorf("Expected ReplyCount=0 for new thread, got %d", metadata.ReplyCount)
		}
		if metadata.LastReplyAt != nil {
			t.Errorf("Expected LastReplyAt=nil for new thread, got %v", metadata.LastReplyAt)
		}

		// Post a reply
		time.Sleep(10 * time.Millisecond) // Ensure distinct timestamp
		reply1, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "First reply", nil, rootEventID, "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post reply: %v", err)
		}
		reply1Time := reply1.CreatedAt.AsTime()

		// Check metadata after first reply
		metadata, err = core.GetThreadMetadata(ctx, KindChannel, room.Id, rootEventID)
		if err != nil {
			t.Fatalf("Failed to get thread metadata: %v", err)
		}
		if metadata.ReplyCount != 1 {
			t.Errorf("Expected ReplyCount=1 after one reply, got %d", metadata.ReplyCount)
		}
		if metadata.LastReplyAt == nil {
			t.Error("Expected LastReplyAt to be set after reply")
		} else if !metadata.LastReplyAt.Equal(reply1Time) {
			// Allow small time difference due to serialization
			diff := metadata.LastReplyAt.Sub(reply1Time)
			if diff > time.Second || diff < -time.Second {
				t.Errorf("LastReplyAt should be close to reply time, got diff=%v", diff)
			}
		}

		// Post more replies
		time.Sleep(10 * time.Millisecond)
		_, err = core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Second reply", nil, rootEventID, "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post second reply: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
		reply3, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Third reply", nil, rootEventID, "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post third reply: %v", err)
		}
		reply3Time := reply3.CreatedAt.AsTime()

		// Check metadata after three replies
		metadata, err = core.GetThreadMetadata(ctx, KindChannel, room.Id, rootEventID)
		if err != nil {
			t.Fatalf("Failed to get thread metadata: %v", err)
		}
		if metadata.ReplyCount != 3 {
			t.Errorf("Expected ReplyCount=3 after three replies, got %d", metadata.ReplyCount)
		}
		if metadata.LastReplyAt == nil {
			t.Error("Expected LastReplyAt to be set")
		} else {
			diff := metadata.LastReplyAt.Sub(reply3Time)
			if diff > time.Second || diff < -time.Second {
				t.Errorf("LastReplyAt should be close to last reply time, got diff=%v", diff)
			}
		}
	})

	t.Run("GetThreadMetadata tracks participants", func(t *testing.T) {
		// Create a second user
		user2, err := core.CreateUser(ctx, "system", "user2", "User 2", "password123")
		if err != nil {
			t.Fatalf("Failed to create user2: %v", err)
		}
		core.JoinRoom(ctx, user.Id, KindChannel, user2.Id, room.Id)

		// Post a new root message
		rootEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Thread for participants test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root message: %v", err)
		}
		rootEventID := rootEvent.Id

		// User 1 replies
		_, err = core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Reply from user 1", nil, rootEventID, "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post reply 1: %v", err)
		}

		// Check participants (should include user 1)
		metadata, err := core.GetThreadMetadata(ctx, KindChannel, room.Id, rootEventID)
		if err != nil {
			t.Fatalf("Failed to get thread metadata: %v", err)
		}
		if len(metadata.ParticipantIDs) != 1 {
			t.Errorf("Expected 1 participant, got %d", len(metadata.ParticipantIDs))
		}
		if metadata.ParticipantIDs[0] != user.Id {
			t.Errorf("Expected participant to be %s, got %s", user.Id, metadata.ParticipantIDs[0])
		}

		// User 2 replies
		_, err = core.PostMessage(ctx, KindChannel, room.Id, user2.Id, "Reply from user 2", nil, rootEventID, "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post reply 2: %v", err)
		}

		// Check participants (should include both users)
		metadata, err = core.GetThreadMetadata(ctx, KindChannel, room.Id, rootEventID)
		if err != nil {
			t.Fatalf("Failed to get thread metadata: %v", err)
		}
		if len(metadata.ParticipantIDs) != 2 {
			t.Errorf("Expected 2 participants, got %d", len(metadata.ParticipantIDs))
		}

		// User 1 replies again (should not duplicate)
		_, err = core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Another reply from user 1", nil, rootEventID, "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post reply 3: %v", err)
		}

		// Check participants (should still be 2)
		metadata, err = core.GetThreadMetadata(ctx, KindChannel, room.Id, rootEventID)
		if err != nil {
			t.Fatalf("Failed to get thread metadata: %v", err)
		}
		if len(metadata.ParticipantIDs) != 2 {
			t.Errorf("Expected 2 participants after duplicate reply, got %d", len(metadata.ParticipantIDs))
		}
		if metadata.ReplyCount != 3 {
			t.Errorf("Expected ReplyCount=3, got %d", metadata.ReplyCount)
		}
	})

	t.Run("inThread is derived from inReplyTo target when caller omits it", func(t *testing.T) {
		// Post a root message that starts a thread.
		rootEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Inherit-thread root", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root message: %v", err)
		}

		// Post a thread reply (this is what the next message will reply to).
		threadReply, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Reply inside thread", nil, rootEvent.Id, "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post thread reply: %v", err)
		}

		// Post a message with inReplyTo pointing into the thread but inThread empty,
		// simulating a bot/extension/older client that doesn't know about inThread.
		inherited, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Reply attribution-only", nil, "", threadReply.Id, nil, false)
		if err != nil {
			t.Fatalf("Failed to post reply with empty inThread: %v", err)
		}

		msg := inherited.GetMessagePosted()
		if msg == nil {
			t.Fatal("Expected MessagePosted event")
		}
		if msg.InThread != rootEvent.Id {
			t.Errorf("Expected InThread to be derived to %q, got %q", rootEvent.Id, msg.InThread)
		}
		if msg.InReplyTo != threadReply.Id {
			t.Errorf("Expected InReplyTo to be %q, got %q", threadReply.Id, msg.InReplyTo)
		}
	})

	t.Run("inThread stays empty when inReplyTo target is itself a root", func(t *testing.T) {
		// Post a plain root message (not part of any thread).
		root, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Plain root", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root: %v", err)
		}

		// Reply to it with attribution only — no thread should be inferred.
		reply, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Channel reply to root", nil, "", root.Id, nil, false)
		if err != nil {
			t.Fatalf("Failed to post reply: %v", err)
		}

		msg := reply.GetMessagePosted()
		if msg == nil {
			t.Fatal("Expected MessagePosted event")
		}
		if msg.InThread != "" {
			t.Errorf("Expected InThread to remain empty, got %q", msg.InThread)
		}
	})
}

func TestChattoCore_ThreadLastOpened(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	root, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Root message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post root message: %v", err)
	}
	threadRootEventId := root.Id

	// Initially should return zero time (never opened)
	lastOpened, err := core.GetThreadLastOpened(ctx, KindChannel, user.Id, room.Id, threadRootEventId)
	if err != nil {
		t.Fatalf("Failed to get thread last opened: %v", err)
	}
	if !lastOpened.IsZero() {
		t.Errorf("Expected zero time for unopened thread, got %v", lastOpened)
	}

	// Set thread last opened - first time should return zero
	prevTime, err := core.SetThreadLastOpened(ctx, KindChannel, user.Id, room.Id, threadRootEventId)
	if err != nil {
		t.Fatalf("Failed to set thread last opened: %v", err)
	}
	if !prevTime.IsZero() {
		t.Errorf("Expected zero time for first open, got %v", prevTime)
	}

	// Should now return a non-zero time
	lastOpened, err = core.GetThreadLastOpened(ctx, KindChannel, user.Id, room.Id, threadRootEventId)
	if err != nil {
		t.Fatalf("Failed to get thread last opened after set: %v", err)
	}
	if lastOpened.IsZero() {
		t.Error("Expected non-zero time after setting")
	}

	// Set again - should return the previous timestamp
	prevTime2, err := core.SetThreadLastOpened(ctx, KindChannel, user.Id, room.Id, threadRootEventId)
	if err != nil {
		t.Fatalf("Failed to set thread last opened second time: %v", err)
	}
	if prevTime2.IsZero() {
		t.Error("Expected non-zero previous time for second open")
	}
	if !prevTime2.Equal(lastOpened) {
		t.Errorf("Previous time mismatch: got %v, want %v", prevTime2, lastOpened)
	}
}

func TestChattoCore_PostMessage_UpdatesThreadLastOpened(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	// Post a root message to create a thread
	rootMsg, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Root message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post root message: %v", err)
	}
	threadRootEventId := rootMsg.Id

	// Initially thread should not be "opened"
	lastOpened, err := core.GetThreadLastOpened(ctx, KindChannel, user.Id, room.Id, threadRootEventId)
	if err != nil {
		t.Fatalf("Failed to get thread last opened: %v", err)
	}
	if !lastOpened.IsZero() {
		t.Errorf("Expected zero time for unopened thread, got %v", lastOpened)
	}

	// Post a thread reply
	_, err = core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Thread reply", nil, threadRootEventId, "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post thread reply: %v", err)
	}

	// Now the thread should be marked as "opened" for the user who posted
	lastOpened, err = core.GetThreadLastOpened(ctx, KindChannel, user.Id, room.Id, threadRootEventId)
	if err != nil {
		t.Fatalf("Failed to get thread last opened after reply: %v", err)
	}
	if lastOpened.IsZero() {
		t.Error("Expected non-zero time after posting thread reply - poster's thread last opened should be updated")
	}
}

func TestChattoCore_ThreadFollow(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	threadRootEventId := "test-thread-root-123"

	t.Run("IsFollowingThread returns false for unfollowed thread", func(t *testing.T) {
		isFollowing, err := core.IsFollowingThread(ctx, KindChannel, user.Id, room.Id, threadRootEventId)
		if err != nil {
			t.Fatalf("Failed to check thread follow: %v", err)
		}
		if isFollowing {
			t.Error("Expected not following for new thread")
		}
	})

	t.Run("FollowThread then IsFollowingThread returns true", func(t *testing.T) {
		err := core.FollowThread(ctx, KindChannel, user.Id, room.Id, threadRootEventId)
		if err != nil {
			t.Fatalf("Failed to follow thread: %v", err)
		}

		key := threadFollowKey(user.Id, room.Id, threadRootEventId)
		if _, err := core.storage.runtimeStateKV.Get(ctx, key); err != nil {
			t.Fatalf("Expected thread follow in RUNTIME_STATE: %v", err)
		}
		assertLegacyKeyAbsent(t, core.storage.serverRuntimeKV, key, "legacy SERVER_RUNTIME follow")

		isFollowing, err := core.IsFollowingThread(ctx, KindChannel, user.Id, room.Id, threadRootEventId)
		if err != nil {
			t.Fatalf("Failed to check thread follow: %v", err)
		}
		if !isFollowing {
			t.Error("Expected following after FollowThread")
		}
	})

	t.Run("FollowThread is idempotent", func(t *testing.T) {
		err := core.FollowThread(ctx, KindChannel, user.Id, room.Id, threadRootEventId)
		if err != nil {
			t.Fatalf("Failed to follow thread second time: %v", err)
		}
	})

	t.Run("UnfollowThread then IsFollowingThread returns false", func(t *testing.T) {
		err := core.UnfollowThread(ctx, KindChannel, user.Id, room.Id, threadRootEventId)
		if err != nil {
			t.Fatalf("Failed to unfollow thread: %v", err)
		}

		isFollowing, err := core.IsFollowingThread(ctx, KindChannel, user.Id, room.Id, threadRootEventId)
		if err != nil {
			t.Fatalf("Failed to check thread follow: %v", err)
		}
		if isFollowing {
			t.Error("Expected not following after UnfollowThread")
		}
	})

	t.Run("UnfollowThread is idempotent", func(t *testing.T) {
		err := core.UnfollowThread(ctx, KindChannel, user.Id, room.Id, threadRootEventId)
		if err != nil {
			t.Fatalf("Failed to unfollow thread second time: %v", err)
		}
	})
}

func TestChattoCore_GetThreadFollowers(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	userA, _ := core.CreateUser(ctx, "system", "usera", "usera", "password123")
	userB, _ := core.CreateUser(ctx, "system", "userb", "userb", "password123")
	userC, _ := core.CreateUser(ctx, "system", "userc", "userc", "password123")
	threadRootEventId := "test-thread-root-456"

	t.Run("returns empty list for thread with no followers", func(t *testing.T) {
		followers, err := core.GetThreadFollowers(ctx, KindChannel, room.Id, threadRootEventId)
		if err != nil {
			t.Fatalf("Failed to get thread followers: %v", err)
		}
		if len(followers) != 0 {
			t.Errorf("Expected 0 followers, got %d", len(followers))
		}
	})

	t.Run("returns correct follower IDs", func(t *testing.T) {
		// Follow with multiple users
		core.FollowThread(ctx, KindChannel, userA.Id, room.Id, threadRootEventId)
		core.FollowThread(ctx, KindChannel, userB.Id, room.Id, threadRootEventId)
		core.FollowThread(ctx, KindChannel, userC.Id, room.Id, threadRootEventId)

		followers, err := core.GetThreadFollowers(ctx, KindChannel, room.Id, threadRootEventId)
		if err != nil {
			t.Fatalf("Failed to get thread followers: %v", err)
		}
		if len(followers) != 3 {
			t.Fatalf("Expected 3 followers, got %d", len(followers))
		}

		// Check all user IDs are present (order may vary)
		followerSet := map[string]bool{}
		for _, id := range followers {
			followerSet[id] = true
		}
		for _, expected := range []string{userA.Id, userB.Id, userC.Id} {
			if !followerSet[expected] {
				t.Errorf("Expected follower %s not found", expected)
			}
		}
	})

	t.Run("excludes unfollowed users", func(t *testing.T) {
		core.UnfollowThread(ctx, KindChannel, userB.Id, room.Id, threadRootEventId)

		followers, err := core.GetThreadFollowers(ctx, KindChannel, room.Id, threadRootEventId)
		if err != nil {
			t.Fatalf("Failed to get thread followers: %v", err)
		}
		if len(followers) != 2 {
			t.Fatalf("Expected 2 followers after unfollow, got %d", len(followers))
		}

		for _, id := range followers {
			if id == userB.Id {
				t.Error("Unfollowed user should not be in followers list")
			}
		}
	})
}

func TestChattoCore_ListFollowedThreads(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room1, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "room-one", "First room")
	room2, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "room-two", "Second room")
	userA, _ := core.CreateUser(ctx, "system", "usera", "usera", "password123")
	userB, _ := core.CreateUser(ctx, "system", "userb", "userb", "password123")
	core.JoinRoom(ctx, userA.Id, KindChannel, userA.Id, room1.Id)
	core.JoinRoom(ctx, userA.Id, KindChannel, userA.Id, room2.Id)
	core.JoinRoom(ctx, userB.Id, KindChannel, userB.Id, room1.Id)
	core.JoinRoom(ctx, userB.Id, KindChannel, userB.Id, room2.Id)

	t.Run("returns empty list when no threads are followed", func(t *testing.T) {
		threads, err := core.ListFollowedThreads(ctx, userA.Id, []string{LegacyServerSpaceID})
		if err != nil {
			t.Fatalf("Failed to list followed threads: %v", err)
		}
		if len(threads) != 0 {
			t.Errorf("Expected 0 followed threads, got %d", len(threads))
		}
	})

	// Create thread 1 in room1: User A posts root, User B replies
	rootMsg1, _ := core.PostMessage(ctx, KindChannel, room1.Id, userA.Id, "Root message 1", nil, "", "", nil, false)
	time.Sleep(10 * time.Millisecond) // Ensure distinct timestamps
	_, _ = core.PostMessage(ctx, KindChannel, room1.Id, userB.Id, "Reply to thread 1", nil, rootMsg1.Id, "", nil, false)

	// Create thread 2 in room2: User A posts root, User B replies twice (to get a newer lastReplyAt)
	rootMsg2, _ := core.PostMessage(ctx, KindChannel, room2.Id, userA.Id, "Root message 2", nil, "", "", nil, false)
	time.Sleep(10 * time.Millisecond)
	_, _ = core.PostMessage(ctx, KindChannel, room2.Id, userB.Id, "First reply to thread 2", nil, rootMsg2.Id, "", nil, false)
	time.Sleep(10 * time.Millisecond)
	_, _ = core.PostMessage(ctx, KindChannel, room2.Id, userB.Id, "Second reply to thread 2", nil, rootMsg2.Id, "", nil, false)

	t.Run("returns followed threads sorted by last activity", func(t *testing.T) {
		// User A auto-follows both threads (root author auto-follow on first reply)
		threads, err := core.ListFollowedThreads(ctx, userA.Id, []string{LegacyServerSpaceID})
		if err != nil {
			t.Fatalf("Failed to list followed threads: %v", err)
		}
		if len(threads) != 2 {
			t.Fatalf("Expected 2 followed threads, got %d", len(threads))
		}

		// Thread 2 should be first (more recent last reply)
		if threads[0].ThreadRootEventID != rootMsg2.Id {
			t.Errorf("Expected thread 2 first (newest), got %s", threads[0].ThreadRootEventID)
		}
		if threads[1].ThreadRootEventID != rootMsg1.Id {
			t.Errorf("Expected thread 1 second (older), got %s", threads[1].ThreadRootEventID)
		}
	})

	t.Run("includes correct metadata", func(t *testing.T) {
		threads, err := core.ListFollowedThreads(ctx, userA.Id, []string{LegacyServerSpaceID})
		if err != nil {
			t.Fatalf("Failed to list followed threads: %v", err)
		}

		// Thread 2 (first in list) has 2 replies
		thread2 := threads[0]
		if thread2.ReplyCount != 2 {
			t.Errorf("Expected 2 replies for thread 2, got %d", thread2.ReplyCount)
		}
		if thread2.RoomID != room2.Id {
			t.Errorf("Expected room2 ID, got %s", thread2.RoomID)
		}
		if thread2.SpaceID != LegacySpaceIDForRoomKind(KindChannel) {
			t.Errorf("Expected canonical space ID, got %s", thread2.SpaceID)
		}
		if thread2.LastReplyAt == nil {
			t.Error("Expected LastReplyAt to be set for thread 2")
		}

		// Thread 1 has 1 reply
		thread1 := threads[1]
		if thread1.ReplyCount != 1 {
			t.Errorf("Expected 1 reply for thread 1, got %d", thread1.ReplyCount)
		}
	})

	t.Run("hasUnread is true when thread has activity after last opened", func(t *testing.T) {
		threads, err := core.ListFollowedThreads(ctx, userA.Id, []string{LegacyServerSpaceID})
		if err != nil {
			t.Fatalf("Failed to list followed threads: %v", err)
		}

		// Both threads should be unread (user A never opened them via SetThreadLastOpened
		// since auto-follow happened from PostMessage which does call SetThreadLastOpened
		// for the poster but User A is the root author, not the replier)
		// Thread 1: User A is root author - auto-followed on first reply.
		// PostMessage sets thread_last_opened for the replier (User B), not for User A.
		// So User A's last-opened is from when they were auto-followed... but the
		// auto-follow doesn't set last-opened. Let's verify:
		for _, thread := range threads {
			if !thread.HasUnread {
				t.Errorf("Expected hasUnread=true for thread %s (user never opened it)", thread.ThreadRootEventID)
			}
		}
	})

	t.Run("hasUnread is false after opening the thread", func(t *testing.T) {
		// User A opens thread 2
		core.SetThreadLastOpened(ctx, KindChannel, userA.Id, room2.Id, rootMsg2.Id)

		threads, err := core.ListFollowedThreads(ctx, userA.Id, []string{LegacyServerSpaceID})
		if err != nil {
			t.Fatalf("Failed to list followed threads: %v", err)
		}

		for _, thread := range threads {
			if thread.ThreadRootEventID == rootMsg2.Id {
				if thread.HasUnread {
					t.Error("Expected hasUnread=false after opening thread 2")
				}
			} else if thread.ThreadRootEventID == rootMsg1.Id {
				if !thread.HasUnread {
					t.Error("Expected hasUnread=true for thread 1 (not opened)")
				}
			}
		}
	})

	t.Run("excludes unfollowed threads", func(t *testing.T) {
		// User A unfollows thread 1
		core.UnfollowThread(ctx, KindChannel, userA.Id, room1.Id, rootMsg1.Id)

		threads, err := core.ListFollowedThreads(ctx, userA.Id, []string{LegacyServerSpaceID})
		if err != nil {
			t.Fatalf("Failed to list followed threads: %v", err)
		}
		if len(threads) != 1 {
			t.Fatalf("Expected 1 followed thread after unfollow, got %d", len(threads))
		}
		if threads[0].ThreadRootEventID != rootMsg2.Id {
			t.Errorf("Expected thread 2 to remain, got %s", threads[0].ThreadRootEventID)
		}
	})

	t.Run("only returns threads for the specified user", func(t *testing.T) {
		// User B followed both threads (auto-follow from posting replies)
		threadsB, err := core.ListFollowedThreads(ctx, userB.Id, []string{LegacyServerSpaceID})
		if err != nil {
			t.Fatalf("Failed to list followed threads for user B: %v", err)
		}
		if len(threadsB) != 2 {
			t.Errorf("Expected 2 followed threads for user B, got %d", len(threadsB))
		}

		// User A should still only have 1 (unfollowed thread 1 above)
		threadsA, err := core.ListFollowedThreads(ctx, userA.Id, []string{LegacyServerSpaceID})
		if err != nil {
			t.Fatalf("Failed to list followed threads for user A: %v", err)
		}
		if len(threadsA) != 1 {
			t.Errorf("Expected 1 followed thread for user A, got %d", len(threadsA))
		}
	})

	t.Run("orphaned follow key is skipped gracefully", func(t *testing.T) {
		// Manually follow a thread that has no metadata (orphaned)
		core.FollowThread(ctx, KindChannel, userA.Id, room1.Id, "nonexistent-thread-id")

		threads, err := core.ListFollowedThreads(ctx, userA.Id, []string{LegacyServerSpaceID})
		if err != nil {
			t.Fatalf("Failed to list followed threads: %v", err)
		}

		// Should still work, including the orphaned thread (with zero metadata)
		foundOrphan := false
		for _, thread := range threads {
			if thread.ThreadRootEventID == "nonexistent-thread-id" {
				foundOrphan = true
				if thread.ReplyCount != 0 {
					t.Errorf("Expected 0 replies for orphaned thread, got %d", thread.ReplyCount)
				}
			}
		}
		if !foundOrphan {
			t.Error("Expected orphaned thread to still appear in list (with zero metadata)")
		}

		// Clean up
		core.UnfollowThread(ctx, KindChannel, userA.Id, room1.Id, "nonexistent-thread-id")
	})

	t.Run("returns empty list for empty spaceIDs slice", func(t *testing.T) {
		threads, err := core.ListFollowedThreads(ctx, userA.Id, []string{})
		if err != nil {
			t.Fatalf("Failed to list followed threads: %v", err)
		}
		if len(threads) != 0 {
			t.Errorf("Expected 0 followed threads for empty spaceIDs, got %d", len(threads))
		}
	})

}

func TestChattoCore_PostMessage_AutoFollowsThread(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	userA, _ := core.CreateUser(ctx, "system", "usera", "usera", "password123")
	userB, _ := core.CreateUser(ctx, "system", "userb", "userb", "password123")
	core.JoinRoom(ctx, userA.Id, KindChannel, userA.Id, room.Id)
	core.JoinRoom(ctx, userB.Id, KindChannel, userB.Id, room.Id)

	// User A posts root message
	rootMsg, err := core.PostMessage(ctx, KindChannel, room.Id, userA.Id, "Root message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post root message: %v", err)
	}

	// Neither user should be following yet (no thread exists)
	isFollowing, _ := core.IsFollowingThread(ctx, KindChannel, userA.Id, room.Id, rootMsg.Id)
	if isFollowing {
		t.Error("Root author should not be following before any replies")
	}

	// User B replies - both should be auto-followed
	_, err = core.PostMessage(ctx, KindChannel, room.Id, userB.Id, "Reply from B", nil, rootMsg.Id, "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post reply: %v", err)
	}

	isFollowingA, _ := core.IsFollowingThread(ctx, KindChannel, userA.Id, room.Id, rootMsg.Id)
	isFollowingB, _ := core.IsFollowingThread(ctx, KindChannel, userB.Id, room.Id, rootMsg.Id)
	if !isFollowingA {
		t.Error("Root author should be auto-followed after first reply")
	}
	if !isFollowingB {
		t.Error("Reply author should be auto-followed after posting")
	}
}

func TestChattoCore_PostMessage_ReFollowsAfterUnfollow(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	userA, _ := core.CreateUser(ctx, "system", "usera", "usera", "password123")
	userB, _ := core.CreateUser(ctx, "system", "userb", "userb", "password123")
	core.JoinRoom(ctx, userA.Id, KindChannel, userA.Id, room.Id)
	core.JoinRoom(ctx, userB.Id, KindChannel, userB.Id, room.Id)

	// Create thread
	rootMsg, _ := core.PostMessage(ctx, KindChannel, room.Id, userA.Id, "Root", nil, "", "", nil, false)
	core.PostMessage(ctx, KindChannel, room.Id, userB.Id, "Reply 1", nil, rootMsg.Id, "", nil, false)

	// User B explicitly unfollows
	core.UnfollowThread(ctx, KindChannel, userB.Id, room.Id, rootMsg.Id)
	isFollowing, _ := core.IsFollowingThread(ctx, KindChannel, userB.Id, room.Id, rootMsg.Id)
	if isFollowing {
		t.Fatal("User B should not be following after unfollow")
	}

	// User B posts again - should be re-followed (posting always re-follows)
	_, err := core.PostMessage(ctx, KindChannel, room.Id, userB.Id, "Reply 2", nil, rootMsg.Id, "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post second reply: %v", err)
	}

	isFollowing, _ = core.IsFollowingThread(ctx, KindChannel, userB.Id, room.Id, rootMsg.Id)
	if !isFollowing {
		t.Error("User B should be re-followed after posting again")
	}
}

func TestChattoCore_PostMessage_RootAuthorUnfollowRespected(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	userA, _ := core.CreateUser(ctx, "system", "usera", "usera", "password123")
	userB, _ := core.CreateUser(ctx, "system", "userb", "userb", "password123")
	userC, _ := core.CreateUser(ctx, "system", "userc", "userc", "password123")
	core.JoinRoom(ctx, userA.Id, KindChannel, userA.Id, room.Id)
	core.JoinRoom(ctx, userB.Id, KindChannel, userB.Id, room.Id)
	core.JoinRoom(ctx, userC.Id, KindChannel, userC.Id, room.Id)

	// User A posts root message, User B replies (auto-follows both)
	rootMsg, _ := core.PostMessage(ctx, KindChannel, room.Id, userA.Id, "Root", nil, "", "", nil, false)
	core.PostMessage(ctx, KindChannel, room.Id, userB.Id, "Reply 1", nil, rootMsg.Id, "", nil, false)

	// Verify User A was auto-followed on first reply
	isFollowing, _ := core.IsFollowingThread(ctx, KindChannel, userA.Id, room.Id, rootMsg.Id)
	if !isFollowing {
		t.Fatal("Root author should be auto-followed after first reply")
	}

	// Root author explicitly unfollows
	core.UnfollowThread(ctx, KindChannel, userA.Id, room.Id, rootMsg.Id)
	isFollowing, _ = core.IsFollowingThread(ctx, KindChannel, userA.Id, room.Id, rootMsg.Id)
	if isFollowing {
		t.Fatal("Root author should not be following after explicit unfollow")
	}

	// Same user posts another reply — root author should NOT be re-followed (2-user case)
	_, err := core.PostMessage(ctx, KindChannel, room.Id, userB.Id, "Reply 2", nil, rootMsg.Id, "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post reply: %v", err)
	}

	isFollowing, _ = core.IsFollowingThread(ctx, KindChannel, userA.Id, room.Id, rootMsg.Id)
	if isFollowing {
		t.Error("Root author should NOT be re-followed after explicit unfollow (2-user case)")
	}

	// A third user posts a reply — root author should still NOT be re-followed
	_, err = core.PostMessage(ctx, KindChannel, room.Id, userC.Id, "Reply 3", nil, rootMsg.Id, "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post reply: %v", err)
	}

	isFollowing, _ = core.IsFollowingThread(ctx, KindChannel, userA.Id, room.Id, rootMsg.Id)
	if isFollowing {
		t.Error("Root author should NOT be re-followed after explicit unfollow (3-user case)")
	}
}

func TestChattoCore_NotifyThreadFollowers(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	userA, _ := core.CreateUser(ctx, "system", "usera", "usera", "password123")
	userB, _ := core.CreateUser(ctx, "system", "userb", "userb", "password123")
	userC, _ := core.CreateUser(ctx, "system", "userc", "userc", "password123")
	core.JoinRoom(ctx, userA.Id, KindChannel, userA.Id, room.Id)
	core.JoinRoom(ctx, userB.Id, KindChannel, userB.Id, room.Id)
	core.JoinRoom(ctx, userC.Id, KindChannel, userC.Id, room.Id)

	// User A creates thread, User B replies (both auto-followed)
	rootMsg, _ := core.PostMessage(ctx, KindChannel, room.Id, userA.Id, "Root", nil, "", "", nil, false)
	core.PostMessage(ctx, KindChannel, room.Id, userB.Id, "Reply from B", nil, rootMsg.Id, "", nil, false)

	// User C also replies (auto-followed), then unfollows
	core.PostMessage(ctx, KindChannel, room.Id, userC.Id, "Reply from C", nil, rootMsg.Id, "", nil, false)
	core.UnfollowThread(ctx, KindChannel, userC.Id, room.Id, rootMsg.Id)

	// Clear all existing notifications
	core.DismissAllNotifications(ctx, userA.Id)
	core.DismissAllNotifications(ctx, userB.Id)
	core.DismissAllNotifications(ctx, userC.Id)

	// User B posts another reply - should notify A (follower) but NOT C (unfollowed) or B (author)
	core.PostMessage(ctx, KindChannel, room.Id, userB.Id, "Another reply from B", nil, rootMsg.Id, "", nil, false)

	// Check notifications
	notifsA, _ := core.GetNotifications(ctx, userA.Id)
	notifsB, _ := core.GetNotifications(ctx, userB.Id)
	notifsC, _ := core.GetNotifications(ctx, userC.Id)

	if len(notifsA) != 1 {
		t.Errorf("Expected 1 notification for user A (follower), got %d", len(notifsA))
	}
	if len(notifsB) != 0 {
		t.Errorf("Expected 0 notifications for user B (reply author), got %d", len(notifsB))
	}
	if len(notifsC) != 0 {
		t.Errorf("Expected 0 notifications for user C (unfollowed), got %d", len(notifsC))
	}
}

func TestChattoCore_PostMessage_ThreadReplyEcho(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "echo-user", "Echo User", "password123")
	core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)

	t.Run("echo publishes two events", func(t *testing.T) {
		// Post root message
		rootEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Thread root", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root: %v", err)
		}

		// Post thread reply with echo
		replyEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Thread reply echoed", nil, rootEvent.Id, "", nil, true)
		if err != nil {
			t.Fatalf("Failed to post echo reply: %v", err)
		}

		reply := replyEvent.GetMessagePosted()
		if reply == nil {
			t.Fatal("Expected MessagePosted event for reply")
		}

		// The reply should still be a thread reply (inThread contains the thread root event ID)
		if reply.InThread != rootEvent.Id {
			t.Errorf("Reply should have InThread=%q, got %q", rootEvent.Id, reply.InThread)
		}

		// GetRoomEvents should contain the echo event
		roomEventsResult, err := core.GetRoomEvents(ctx, KindChannel, room.Id, 50, nil)
		if err != nil {
			t.Fatalf("Failed to get room events: %v", err)
		}
		roomEvents := roomEventsResult.Events

		var foundEcho bool
		for _, e := range roomEvents {
			if msg := e.GetMessagePosted(); msg != nil && msg.EchoOfEventId != "" {
				foundEcho = true
				// The echo has its own envelope id. EchoOfEventId /
				// EchoFromThreadRootEventId are the shared identifiers.
				if msg.EchoOfEventId != replyEvent.Id {
					t.Errorf("Echo.EchoOfEventId should be %q, got %q", replyEvent.Id, msg.EchoOfEventId)
				}
				if msg.EchoFromThreadRootEventId != rootEvent.Id {
					t.Errorf("Echo.ThreadRootEventId should be %q, got %q", rootEvent.Id, msg.EchoFromThreadRootEventId)
				}
				break
			}
		}
		if !foundEcho {
			t.Error("Expected echo MessagePostedEvent in GetRoomEvents")
		}

		// GetThreadEvents should NOT contain the echo (only original reply)
		threadEvents, err := core.GetThreadEvents(ctx, KindChannel, room.Id, rootEvent.Id)
		if err != nil {
			t.Fatalf("Failed to get thread events: %v", err)
		}
		for _, e := range threadEvents {
			if msg := e.GetMessagePosted(); msg != nil && msg.EchoOfEventId != "" {
				t.Error("Echo MessagePostedEvent should NOT appear in GetThreadEvents")
			}
		}
	})

	t.Run("echo without thread reply is rejected", func(t *testing.T) {
		// alsoSendToChannel=true without inReplyTo doesn't make sense
		// but at the core layer, inReplyTo="" means root message, so echo is silently skipped
		event, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Not a reply", nil, "", "", nil, true)
		if err != nil {
			t.Fatalf("Should not fail: %v", err)
		}
		// Should be a normal message, no echo
		if event.GetMessagePosted() == nil {
			t.Fatal("Expected MessagePosted event")
		}
	})

	t.Run("reply_count only increments once with echo", func(t *testing.T) {
		// Post root
		rootEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Root for count test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root: %v", err)
		}

		// Post reply with echo
		_, err = core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Reply with echo", nil, rootEvent.Id, "", nil, true)
		if err != nil {
			t.Fatalf("Failed to post reply: %v", err)
		}

		metadata, err := core.GetThreadMetadata(ctx, KindChannel, room.Id, rootEvent.Id)
		if err != nil {
			t.Fatalf("Failed to get metadata: %v", err)
		}
		if metadata.ReplyCount != 1 {
			t.Errorf("Expected ReplyCount=1 (echo should not increment), got %d", metadata.ReplyCount)
		}
	})

	t.Run("echo carries the same body content as the reply", func(t *testing.T) {
		// Post root and reply with echo.
		rootEvent, _ := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Root for body test", nil, "", "", nil, false)
		replyEvent, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "Shared body content", nil, rootEvent.Id, "", nil, true)
		if err != nil {
			t.Fatalf("Failed to post reply: %v", err)
		}

		// Echo and reply each have their own envelope id and encryption
		// context, but decrypt to the same visible content.
		reply := replyEvent.GetMessagePosted()
		replyBody := reply.GetBody()
		if replyBody == nil {
			t.Fatal("reply has no embedded body")
		}

		roomEventsResult, _ := core.GetRoomEvents(ctx, KindChannel, room.Id, 50, nil)
		var echoBody *corev1.MessageBody
		var echoID string
		for _, e := range roomEventsResult.Events {
			if msg := e.GetMessagePosted(); msg != nil && msg.EchoOfEventId == replyEvent.Id {
				echoBody = msg.GetBody()
				echoID = e.Id
				break
			}
		}
		if echoBody == nil {
			t.Fatal("Echo not found in room events (or has no embedded body)")
		}
		if echoID == "" {
			t.Fatal("Echo event has no id")
		}
		if string(echoBody.EncryptedBody) == string(replyBody.EncryptedBody) {
			t.Errorf("Echo body ciphertext should be independently encrypted")
		}
		echoText, err := core.GetMessageBody(ctx, KindChannel, echoID)
		if err != nil {
			t.Fatalf("Failed to decrypt echo body: %v", err)
		}
		replyText, err := core.GetMessageBody(ctx, KindChannel, replyEvent.Id)
		if err != nil {
			t.Fatalf("Failed to decrypt reply body: %v", err)
		}
		if echoText != replyText || echoText != "Shared body content" {
			t.Errorf("Echo/reply body = %q/%q, want shared content", echoText, replyText)
		}
	})
}

func TestChattoCore_PostMessage_EchoMentionNotification(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and two users
	room, _ := core.CreateRoom(ctx, "system", KindChannel, "", "General", "General discussion")
	author, _ := core.CreateUser(ctx, "system", "mention-author", "Author", "password123")
	target, _ := core.CreateUser(ctx, "system", "mention-target", "Target", "password123")
	core.JoinRoom(ctx, author.Id, KindChannel, author.Id, room.Id)
	core.JoinRoom(ctx, target.Id, KindChannel, target.Id, room.Id)

	t.Run("echo with mention produces exactly one notification", func(t *testing.T) {
		// Subscribe to live mention events for the target user
		mentionCount := 0
		sub, err := nc.Subscribe(subjects.LiveSyncUserEvent(target.Id, "mentioned"), func(msg *nats.Msg) {
			mentionCount++
		})
		if err != nil {
			t.Fatalf("Failed to subscribe: %v", err)
		}
		defer sub.Unsubscribe()

		// Post root message
		rootEvent, err := core.PostMessage(ctx, KindChannel, room.Id, author.Id, "Thread root", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root: %v", err)
		}

		// Post thread reply with echo, mentioning the target user
		_, err = core.PostMessage(ctx, KindChannel, room.Id, author.Id, "Hey @mention-target check this out", nil, rootEvent.Id, "", nil, true)
		if err != nil {
			t.Fatalf("Failed to post echo reply with mention: %v", err)
		}

		// Wait for async notifications to be delivered
		nc.Flush()
		time.Sleep(500 * time.Millisecond)

		// Should have received exactly 1 live mention event (not 2)
		if mentionCount != 1 {
			t.Errorf("Expected exactly 1 live mention event, got %d", mentionCount)
		}

		// Should have exactly 1 persistent notification
		count, err := core.GetNotificationCount(ctx, target.Id)
		if err != nil {
			t.Fatalf("GetNotificationCount error: %v", err)
		}
		if count != 1 {
			t.Errorf("Expected exactly 1 persistent notification, got %d", count)
		}
	})
}

func TestChattoCore_PostMessage_InReplyToNotification(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup: space, room, two users joined
	room, _ := core.CreateRoom(ctx, "system", KindChannel, "", "general", "")
	alice, _ := core.CreateUser(ctx, "system", "alice", "Alice", "password123")
	bob, _ := core.CreateUser(ctx, "system", "bob", "Bob", "password123")
	core.JoinRoom(ctx, alice.Id, KindChannel, alice.Id, room.Id)
	core.JoinRoom(ctx, bob.Id, KindChannel, bob.Id, room.Id)

	t.Run("creates notification for in-reply-to author", func(t *testing.T) {
		// Alice posts a message
		aliceMsg, err := core.PostMessage(ctx, KindChannel, room.Id, alice.Id, "Hello world", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post message: %v", err)
		}

		// Bob replies with inReplyTo (room-level reply, no thread)
		_, err = core.PostMessage(ctx, KindChannel, room.Id, bob.Id, "Hi back", nil, "", aliceMsg.Id, nil, false)
		if err != nil {
			t.Fatalf("Failed to post reply: %v", err)
		}

		// Alice should have a ReplyNotification
		notifications, err := core.GetNotifications(ctx, alice.Id)
		if err != nil {
			t.Fatalf("GetNotifications error: %v", err)
		}
		if len(notifications) == 0 {
			t.Fatal("Expected at least 1 notification for Alice")
		}

		// Find the reply notification
		var found bool
		for _, n := range notifications {
			replyNotif := n.GetReply()
			if replyNotif != nil {
				found = true
				if replyNotif.RoomId != room.Id {
					t.Errorf("ReplyNotification.RoomId = %s, want %s", replyNotif.RoomId, room.Id)
				}
				if replyNotif.InReplyToId != aliceMsg.Id {
					t.Errorf("ReplyNotification.InReplyToId = %s, want %s", replyNotif.InReplyToId, aliceMsg.Id)
				}
				if replyNotif.InThread != "" {
					t.Errorf("ReplyNotification.InThread should be empty for room-level reply, got %s", replyNotif.InThread)
				}
				if n.ActorId != bob.Id {
					t.Errorf("Notification.ActorId = %s, want %s (bob)", n.ActorId, bob.Id)
				}
				break
			}
		}
		if !found {
			t.Error("Expected a ReplyNotification for Alice, but none found")
		}

		// Bob should NOT have any notifications
		bobNotifs, err := core.GetNotifications(ctx, bob.Id)
		if err != nil {
			t.Fatalf("GetNotifications error: %v", err)
		}
		if len(bobNotifs) != 0 {
			t.Errorf("Expected 0 notifications for Bob, got %d", len(bobNotifs))
		}
	})

	t.Run("self-reply does not create notification", func(t *testing.T) {
		// Clear existing notifications
		core.DismissAllNotifications(ctx, alice.Id)

		// Alice posts a message
		aliceMsg, err := core.PostMessage(ctx, KindChannel, room.Id, alice.Id, "Talking to myself", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post message: %v", err)
		}

		// Alice replies to her own message
		_, err = core.PostMessage(ctx, KindChannel, room.Id, alice.Id, "Replying to myself", nil, "", aliceMsg.Id, nil, false)
		if err != nil {
			t.Fatalf("Failed to post self-reply: %v", err)
		}

		// Alice should have no reply notifications
		notifications, err := core.GetNotifications(ctx, alice.Id)
		if err != nil {
			t.Fatalf("GetNotifications error: %v", err)
		}
		for _, n := range notifications {
			if n.GetReply() != nil {
				t.Error("Expected no ReplyNotification for self-reply")
			}
		}
	})

	t.Run("muted room skips notification", func(t *testing.T) {
		// Clear existing notifications
		core.DismissAllNotifications(ctx, alice.Id)

		// Alice mutes the room
		core.SetRoomNotificationLevel(ctx, alice.Id, room.Id, corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED)
		defer core.SetRoomNotificationLevel(ctx, alice.Id, room.Id, corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT)

		// Alice posts a message
		aliceMsg, err := core.PostMessage(ctx, KindChannel, room.Id, alice.Id, "Muted test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post message: %v", err)
		}

		// Bob replies
		_, err = core.PostMessage(ctx, KindChannel, room.Id, bob.Id, "Reply to muted", nil, "", aliceMsg.Id, nil, false)
		if err != nil {
			t.Fatalf("Failed to post reply: %v", err)
		}

		// Alice should have no notifications (muted)
		notifications, err := core.GetNotifications(ctx, alice.Id)
		if err != nil {
			t.Fatalf("GetNotifications error: %v", err)
		}
		for _, n := range notifications {
			if n.GetReply() != nil {
				t.Error("Expected no ReplyNotification when room is muted")
			}
		}
	})

	t.Run("mention + reply deduplicates to mention only", func(t *testing.T) {
		// Clear existing notifications
		core.DismissAllNotifications(ctx, alice.Id)

		// Alice posts a message
		aliceMsg, err := core.PostMessage(ctx, KindChannel, room.Id, alice.Id, "Dedup test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post message: %v", err)
		}

		// Bob replies AND mentions Alice in the same message
		_, err = core.PostMessage(ctx, KindChannel, room.Id, bob.Id, "Hey @alice check this out", nil, "", aliceMsg.Id, nil, false)
		if err != nil {
			t.Fatalf("Failed to post reply with mention: %v", err)
		}

		// Alice should have exactly 1 notification (the mention, not a duplicate reply)
		notifications, err := core.GetNotifications(ctx, alice.Id)
		if err != nil {
			t.Fatalf("GetNotifications error: %v", err)
		}

		mentionCount := 0
		replyCount := 0
		for _, n := range notifications {
			if n.GetMention() != nil {
				mentionCount++
			}
			if n.GetReply() != nil {
				replyCount++
			}
		}

		if mentionCount != 1 {
			t.Errorf("Expected 1 mention notification, got %d", mentionCount)
		}
		if replyCount != 0 {
			t.Errorf("Expected 0 reply notifications (deduped by mention), got %d", replyCount)
		}
	})

	t.Run("thread reply sets InThread field", func(t *testing.T) {
		// Clear existing notifications
		core.DismissAllNotifications(ctx, alice.Id)

		// Alice posts a root message
		rootMsg, err := core.PostMessage(ctx, KindChannel, room.Id, alice.Id, "Thread root", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root: %v", err)
		}

		// Bob posts a thread reply
		_, err = core.PostMessage(ctx, KindChannel, room.Id, bob.Id, "Thread reply", nil, rootMsg.Id, "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post thread reply: %v", err)
		}

		// Alice should have a ReplyNotification with InThread set
		notifications, err := core.GetNotifications(ctx, alice.Id)
		if err != nil {
			t.Fatalf("GetNotifications error: %v", err)
		}

		var found bool
		for _, n := range notifications {
			replyNotif := n.GetReply()
			if replyNotif != nil {
				found = true
				if replyNotif.InThread != rootMsg.Id {
					t.Errorf("ReplyNotification.InThread = %q, want %q", replyNotif.InThread, rootMsg.Id)
				}
				break
			}
		}
		if !found {
			t.Error("Expected a ReplyNotification for thread reply")
		}
	})

	t.Run("in-thread inReplyTo notifies original author with InThread set", func(t *testing.T) {
		// Clear existing notifications
		core.DismissAllNotifications(ctx, alice.Id)
		core.DismissAllNotifications(ctx, bob.Id)

		// Create a third user for this test
		charlie, _ := core.CreateUser(ctx, "system", "charlie", "Charlie", "password123")
		core.JoinRoom(ctx, charlie.Id, KindChannel, charlie.Id, room.Id)

		// Alice posts a root message (starts the thread)
		rootMsg, err := core.PostMessage(ctx, KindChannel, room.Id, alice.Id, "Thread root for inReplyTo test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root: %v", err)
		}

		// Bob posts a reply in the thread
		bobMsg, err := core.PostMessage(ctx, KindChannel, room.Id, bob.Id, "Bob's thread msg", nil, rootMsg.Id, "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post thread reply: %v", err)
		}

		// Clear notifications from thread participant notifications
		core.DismissAllNotifications(ctx, alice.Id)
		core.DismissAllNotifications(ctx, bob.Id)

		// Charlie replies to Bob's specific message within the thread (inThread + inReplyTo)
		_, err = core.PostMessage(ctx, KindChannel, room.Id, charlie.Id, "Replying to Bob in thread", nil, rootMsg.Id, bobMsg.Id, nil, false)
		if err != nil {
			t.Fatalf("Failed to post in-thread inReplyTo: %v", err)
		}

		// Bob should have a ReplyNotification from notifyInReplyToAuthor with InThread set
		// (He also gets one from notifyThreadParticipants, but we check that at least one has InThread)
		bobNotifs, err := core.GetNotifications(ctx, bob.Id)
		if err != nil {
			t.Fatalf("GetNotifications error: %v", err)
		}

		var foundReply bool
		for _, n := range bobNotifs {
			replyNotif := n.GetReply()
			if replyNotif != nil && replyNotif.InReplyToId == bobMsg.Id {
				foundReply = true
				if replyNotif.InThread != rootMsg.Id {
					t.Errorf("ReplyNotification.InThread = %q, want %q", replyNotif.InThread, rootMsg.Id)
				}
				break
			}
		}
		if !foundReply {
			t.Error("Expected Bob to get a ReplyNotification for in-thread inReplyTo")
		}
	})

	t.Run("in-thread inReplyTo deduplicates with thread participant notification", func(t *testing.T) {
		// Clear existing notifications
		core.DismissAllNotifications(ctx, alice.Id)
		core.DismissAllNotifications(ctx, bob.Id)

		// Alice posts a root message
		rootMsg, err := core.PostMessage(ctx, KindChannel, room.Id, alice.Id, "Dedup thread root", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root: %v", err)
		}

		// Clear Alice's thread participant notification
		core.DismissAllNotifications(ctx, alice.Id)

		// Bob replies to Alice's root message in the thread (both inThread and inReplyTo point to root)
		// Alice is both the thread root author (notifyThreadParticipants) and the inReplyTo author (notifyInReplyToAuthor)
		_, err = core.PostMessage(ctx, KindChannel, room.Id, bob.Id, "Replying to root in thread", nil, rootMsg.Id, rootMsg.Id, nil, false)
		if err != nil {
			t.Fatalf("Failed to post reply: %v", err)
		}

		// Alice should have exactly 1 ReplyNotification, not 2 (dedup between thread + inReplyTo)
		notifications, err := core.GetNotifications(ctx, alice.Id)
		if err != nil {
			t.Fatalf("GetNotifications error: %v", err)
		}

		replyCount := 0
		for _, n := range notifications {
			if n.GetReply() != nil {
				replyCount++
			}
		}
		if replyCount != 1 {
			t.Errorf("Expected exactly 1 ReplyNotification (deduped), got %d", replyCount)
		}
	})
}

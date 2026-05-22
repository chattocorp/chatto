package graph

import (
	"context"
	"errors"
	"testing"
	"time"

	"hmans.de/chatto/internal/core"
)

// ============================================================================
// Subscription Resolver Tests
// ============================================================================

func TestSubscriptionResolver_MyEvents(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("subscribe to space events as member", func(t *testing.T) {
		// Create a context with timeout for subscription
		subCtx, cancel := context.WithTimeout(env.authContext(), 5*time.Second)
		defer cancel()

		// Subscribe to space events
		eventChan, err := env.resolver.Subscription().MyEvents(subCtx)
		if err != nil {
			t.Fatalf("Unexpected error subscribing: %v", err)
		}

		if eventChan == nil {
			t.Fatal("Expected event channel, got nil")
		}

		// Give subscription time to be ready
		time.Sleep(50 * time.Millisecond)

		// Post a message to trigger an event
		go func() {
			_, err = env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "Test subscription message", nil, "", "", nil, false)
			if err != nil {
				t.Logf("Failed to post message: %v", err)
			}
		}()

		// Wait for MessagePosted event (skip non-message events like presence)
		deadline := time.After(3 * time.Second)
		found := false
		for !found {
			select {
			case event := <-eventChan:
				if event == nil {
					t.Error("Received nil event")
					return
				}
				if event.GetMessagePosted() != nil {
					found = true
				}
			case <-deadline:
				t.Error("Timeout waiting for MessagePosted event")
				return
			}
		}
	})

	t.Run("subscribe without authentication", func(t *testing.T) {
		eventChan, err := env.resolver.Subscription().MyEvents(env.unauthContext())
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}

		if eventChan != nil {
			t.Errorf("Expected nil channel, got %v", eventChan)
		}
	})

	t.Run("non-member of any space still subscribes (auth-only gate)", func(t *testing.T) {
		// Post-pivot: the deployment is a single server. Any authenticated user
		// can subscribe; per-room membership filtering happens per-event.
		otherUser, err := env.core.CreateUser(env.ctx, "system", "othersubuser", "Other Sub User", "password456")
		if err != nil {
			t.Fatalf("Failed to create other user: %v", err)
		}

		subCtx, cancel := context.WithTimeout(env.authContextForUser(otherUser), 1*time.Second)
		defer cancel()

		eventChan, err := env.resolver.Subscription().MyEvents(subCtx)
		if err != nil {
			t.Errorf("Expected subscription to succeed for authenticated non-member, got %v", err)
		}

		if eventChan == nil {
			t.Error("Expected event channel, got nil")
		}
	})

	t.Run("only receive events for member rooms", func(t *testing.T) {
		// Create another room in the same space that the user is NOT a member of
		otherRoom, err := env.core.CreateRoom(env.ctx, env.testUser.Id, core.KindChannel, "", "other-room", "Another room")
		if err != nil {
			t.Fatalf("Failed to create other room: %v", err)
		}

		// Create another user to post in that room
		poster, err := env.core.CreateUser(env.ctx, "system", "poster", "Poster User", "password")
		if err != nil {
			t.Fatalf("Failed to create poster user: %v", err)
		}
		_, err = env.core.JoinRoom(env.ctx, poster.Id, core.KindChannel, poster.Id, otherRoom.Id)
		if err != nil {
			t.Fatalf("Failed to join room: %v", err)
		}

		// Leave the test user from the other room (they're not a member)
		// (they were never added, so this is fine)

		subCtx, cancel := context.WithTimeout(env.authContext(), 3*time.Second)
		defer cancel()

		eventChan, err := env.resolver.Subscription().MyEvents(subCtx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Give subscription time to be ready
		time.Sleep(50 * time.Millisecond)

		// Post message in the other room (user should NOT receive this)
		go func() {
			_, err = env.core.PostMessage(env.ctx, core.KindChannel, otherRoom.Id, poster.Id, "Message in other room", nil, "", "", nil, false)
			if err != nil {
				t.Logf("Failed to post message: %v", err)
			}
		}()

		// Should timeout without receiving the event
		select {
		case event := <-eventChan:
			if event != nil && event.GetMessagePosted() != nil {
				t.Error("Should not receive events from rooms user is not a member of")
			}
		case <-time.After(500 * time.Millisecond):
			// Expected - no event received
		}
	})

	t.Run("subscription cleanup on context cancellation", func(t *testing.T) {
		subCtx, cancel := context.WithCancel(env.authContext())

		eventChan, err := env.resolver.Subscription().MyEvents(subCtx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Cancel the context to stop the subscription
		cancel()

		// Give some time for cleanup
		time.Sleep(100 * time.Millisecond)

		// Channel should be closed or no longer receiving
		select {
		case _, ok := <-eventChan:
			if ok {
				// If we received something, that's fine - it might be buffered
				t.Log("Received event after cancellation (may be buffered)")
			}
			// Channel closed is expected behavior
		case <-time.After(200 * time.Millisecond):
			// Timeout is acceptable - subscription stopped
		}
	})

	t.Run("receive multiple message events in order", func(t *testing.T) {
		subCtx, cancel := context.WithTimeout(env.authContext(), 10*time.Second)
		defer cancel()

		eventChan, err := env.resolver.Subscription().MyEvents(subCtx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Give subscription time to be ready
		time.Sleep(50 * time.Millisecond)

		// Post multiple messages serially. Capture each posted event's ID so
		// we can assert the subscription delivers them in the same order —
		// catches JetStream-consumer regressions that would otherwise pass
		// the simpler "count messages" check.
		messages := []string{"First message", "Second message", "Third message"}
		expectedIDs := make([]string, 0, len(messages))
		for _, msg := range messages {
			posted, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, msg, nil, "", "", nil, false)
			if err != nil {
				t.Fatalf("Failed to post message %q: %v", msg, err)
			}
			expectedIDs = append(expectedIDs, posted.Id)
		}

		// Drain the channel, ignoring any non-MessagePosted events that the
		// merged subscription now interleaves (presence, etc.).
		receivedIDs := make([]string, 0, len(messages))
		deadline := time.After(5 * time.Second)
		for len(receivedIDs) < len(messages) {
			select {
			case event := <-eventChan:
				if event == nil {
					t.Error("Received nil event")
					continue
				}
				if event.GetMessagePosted() == nil {
					continue // not a message event — skip
				}
				receivedIDs = append(receivedIDs, event.Id)
			case <-deadline:
				t.Fatalf("Timeout waiting for message events: got %d of %d (ids so far: %v)", len(receivedIDs), len(messages), receivedIDs)
			}
		}

		for i, want := range expectedIDs {
			if receivedIDs[i] != want {
				t.Errorf("Event %d: expected ID %q, got %q (full received order: %v, expected: %v)", i, want, receivedIDs[i], receivedIDs, expectedIDs)
			}
		}
	})

	t.Run("receive thread reply as MessagePosted event", func(t *testing.T) {
		// Thread replies are delivered via JetStream as regular MessagePosted events.
		// Client-side filtering determines what to display:
		// - Room views filter out events where inThread is set
		// - Thread panes filter for events where inThread matches their thread root

		// Create a second user (User B) who will post replies
		userB, err := env.core.CreateUser(env.ctx, "system", "userb", "User B", "password123")
		if err != nil {
			t.Fatalf("Failed to create user B: %v", err)
		}
		_, err = env.core.JoinRoom(env.ctx, userB.Id, core.KindChannel, userB.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("Failed to join room: %v", err)
		}

		// User A posts a root message
		rootEvent, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "Thread root", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root message: %v", err)
		}
		rootEventID := rootEvent.Id

		// User A subscribes to space events
		subCtx, cancel := context.WithTimeout(env.authContext(), 10*time.Second)
		defer cancel()

		eventChan, err := env.resolver.Subscription().MyEvents(subCtx)
		if err != nil {
			t.Fatalf("Unexpected error subscribing: %v", err)
		}

		// Give subscription time to be ready
		time.Sleep(100 * time.Millisecond)

		// User B posts a reply to the root message
		go func() {
			_, err = env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, userB.Id, "Reply from User B", nil, rootEventID, "", nil, false)
			if err != nil {
				t.Logf("Failed to post reply: %v", err)
			}
		}()

		// User A should receive the thread reply as a MessagePosted event with inThread set
		// Loop to skip other events (joins, etc.) until we find the thread reply
		timeout := time.After(10 * time.Second)
		for {
			select {
			case event := <-eventChan:
				if event == nil {
					continue
				}
				if msg := event.GetMessagePosted(); msg != nil && msg.InThread != "" {
					if msg.InThread != rootEventID {
						t.Errorf("Expected InThread=%q, got %q", rootEventID, msg.InThread)
					}
					t.Logf("Received MessagePosted for thread reply in thread=%s", msg.InThread)
					return // Success - thread reply received
				}
			case <-timeout:
				t.Fatal("Timeout waiting for thread reply event")
			}
		}
	})
}

func TestSubscriptionResolver_MyEvents_DeploymentEvents(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("subscribe to deployment events authenticated", func(t *testing.T) {
		subCtx, cancel := context.WithTimeout(env.authContext(), 5*time.Second)
		defer cancel()

		eventChan, err := env.resolver.Subscription().MyEvents(subCtx)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if eventChan == nil {
			t.Fatal("Expected event channel, got nil")
		}
	})

	t.Run("subscribe without authentication", func(t *testing.T) {
		eventChan, err := env.resolver.Subscription().MyEvents(env.unauthContext())
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}

		if eventChan != nil {
			t.Errorf("Expected nil channel, got %v", eventChan)
		}
	})

	t.Run("receive mention notification when mentioned by another user", func(t *testing.T) {
		// Create User B who will mention User A
		userB, err := env.core.CreateUser(env.ctx, "system", "mentioner", "User B", "password123")
		if err != nil {
			t.Fatalf("Failed to create user B: %v", err)
		}
		_, err = env.core.JoinRoom(env.ctx, userB.Id, core.KindChannel, userB.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("Failed to join room: %v", err)
		}

		// User A subscribes to server events
		subCtx, cancel := context.WithTimeout(env.authContext(), 10*time.Second)
		defer cancel()

		eventChan, err := env.resolver.Subscription().MyEvents(subCtx)
		if err != nil {
			t.Fatalf("Unexpected error subscribing: %v", err)
		}

		// Give subscription time to be ready (longer delay for slow CI)
		time.Sleep(200 * time.Millisecond)

		// User B posts a message mentioning User A (env.testUser)
		// The testUser's login is used for the mention
		go func() {
			_, err = env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, userB.Id, "Hey @"+env.testUser.Login+" check this out!", nil, "", "", nil, false)
			if err != nil {
				t.Logf("Failed to post message: %v", err)
			}
		}()

		// Wait for mention notification event (for room indicator) or notification created event (for bell icon)
		// Accept whichever of MentionNotificationEvent or
		// NotificationCreatedEvent arrives first. Skip presence and other
		// chatter the merged subscription now delivers.
		deadline := time.After(5 * time.Second)
		for {
			select {
			case event := <-eventChan:
				if event == nil {
					t.Fatal("Received nil event")
				}
				if mentioned := event.GetMentionNotification(); mentioned != nil {
					if mentioned.RoomId != env.testRoom.Id {
						t.Errorf("Expected room ID %s, got %s", env.testRoom.Id, mentioned.RoomId)
					}
					if mentioned.MentionedByUserId != userB.Id {
						t.Errorf("Expected mentioner ID %s, got %s", userB.Id, mentioned.MentionedByUserId)
					}
					t.Logf("Successfully received mention notification in room %s", mentioned.RoomId)
					return
				}
				if notifCreated := event.GetNotificationCreated(); notifCreated != nil {
					if notifCreated.RoomId != env.testRoom.Id {
						t.Errorf("Expected room ID %s, got %s", env.testRoom.Id, notifCreated.RoomId)
					}
					t.Logf("Successfully received notification created event for mention in room %s", notifCreated.RoomId)
					return
				}
				t.Logf("Ignoring non-mention event: %T", event.Event)
			case <-deadline:
				t.Fatal("Timeout waiting for mention event")
			}
		}
	})
}

// TestSubscriptionResolver_Presence tests that subscribing to myServerEvents
// sets the user's presence and delivers PresenceChangedEvents for other users.
func TestSubscriptionResolver_Presence(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("receive presence changed event when another user comes online", func(t *testing.T) {
		// Create User B who will come online after User A is subscribed
		userB, err := env.core.CreateUser(env.ctx, "system", "userb-presence", "User B", "password123")
		if err != nil {
			t.Fatalf("Failed to create user B: %v", err)
		}

		// User A subscribes to server events (sets their presence to ONLINE)
		instCtxA, instCancelA := context.WithTimeout(env.authContext(), 10*time.Second)
		defer instCancelA()
		_, err = env.resolver.Subscription().MyEvents(instCtxA)
		if err != nil {
			t.Fatalf("Unexpected error subscribing User A to server events: %v", err)
		}

		// User A subscribes to space events (receives presence change events via KV watcher)
		subCtxA, cancelA := context.WithTimeout(env.authContext(), 10*time.Second)
		defer cancelA()
		eventChanA, err := env.resolver.Subscription().MyEvents(subCtxA)
		if err != nil {
			t.Fatalf("Unexpected error subscribing User A to space events: %v", err)
		}

		// Wait for User A's subscription to complete initial sync
		time.Sleep(200 * time.Millisecond)

		// User B subscribes to server events (this sets their presence to ONLINE)
		instCtxB, instCancelB := context.WithTimeout(env.authContextForUser(userB), 5*time.Second)
		defer instCancelB()
		_, err = env.resolver.Subscription().MyEvents(instCtxB)
		if err != nil {
			t.Fatalf("Unexpected error subscribing User B to server events: %v", err)
		}

		// User A should receive a PresenceChangedEvent for User B via space events
		deadline := time.After(5 * time.Second)
		found := false
		for !found {
			select {
			case event := <-eventChanA:
				if event == nil {
					t.Fatal("Received nil event")
				}
				presenceEvent := event.GetPresenceChanged()
				if presenceEvent == nil {
					t.Logf("Received non-presence event: %T, skipping", event.Event)
					continue
				}
				if event.ActorId != userB.Id {
					t.Logf("Received presence event for %s (not User B), skipping", event.ActorId)
					continue
				}
				if presenceEvent.Status != "ONLINE" {
					t.Errorf("Expected status ONLINE, got %s", presenceEvent.Status)
				}
				t.Logf("Successfully received presence event: user %s is now %s", event.ActorId, presenceEvent.Status)
				found = true
			case <-deadline:
				t.Fatal("Timeout waiting for User B's presence event")
			}
		}
	})

	t.Run("presence set by server-events subscription, remains after subscription ends (TTL-based expiry)", func(t *testing.T) {
		// Create a user
		userC, err := env.core.CreateUser(env.ctx, "system", "userc-presence", "User C", "password123")
		if err != nil {
			t.Fatalf("Failed to create user C: %v", err)
		}

		// Subscribe to server events (this sets presence to ONLINE)
		subCtx, cancel := context.WithCancel(env.authContextForUser(userC))

		_, err = env.resolver.Subscription().MyEvents(subCtx)
		if err != nil {
			t.Fatalf("Unexpected error subscribing: %v", err)
		}

		// Wait for presence to be set
		var status string
		for i := 0; i < 20; i++ {
			time.Sleep(50 * time.Millisecond)
			status, err = env.core.GetUserPresence(env.ctx, userC.Id)
			if err != nil {
				t.Fatalf("Failed to get presence status: %v", err)
			}
			if status == "ONLINE" {
				break
			}
		}
		if status != "ONLINE" {
			t.Errorf("Expected status ONLINE while subscribed, got %s", status)
		}

		// Cancel the subscription
		cancel()

		// Wait for cleanup
		time.Sleep(200 * time.Millisecond)

		// Verify user is still ONLINE - we no longer delete presence on disconnect.
		// TTL-based expiry supports multi-device scenarios.
		status, err = env.core.GetUserPresence(env.ctx, userC.Id)
		if err != nil {
			t.Fatalf("Failed to get presence status after cleanup: %v", err)
		}
		if status != "ONLINE" {
			t.Errorf("Expected status ONLINE after subscription ends (TTL handles expiry), got %s", status)
		}
	})
}

package core

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/config"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

// ============================================================================
// Test Setup
// ============================================================================

// testContext returns a context with a reasonable timeout for tests.
// This prevents tests from hanging indefinitely if something goes wrong.
func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

// setupTestCore is a shared test helper that creates a ChattoCore instance
// with an embedded NATS server for testing. Used by all test files in this package.
func setupTestCore(t *testing.T) (*ChattoCore, *nats.Conn) {
	t.Helper()

	_, nc := testutil.StartSharedNATS(t)

	ctx := testContext(t)

	// Create ChattoCore
	cfg := config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	}
	core, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatalf("Failed to create ChattoCore: %v", err)
	}

	startCoreServices(t, core)

	return core, nc
}

// startCoreServices runs ChattoCore's background services (PresenceHub +
// projectors) for the duration of a test. Required because membership
// mutations call WaitFor, and StreamMyEvents depends on PresenceHub —
// neither works without the corresponding loop running.
//
// Once `core.Run` owns the lifecycle of every background service, new
// projectors (ADR-035) get picked up here automatically.
func startCoreServices(t *testing.T, core *ChattoCore) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- core.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("core.Run did not stop within timeout")
		}
	})
	// Block until Run's boot phase is complete — projectors started
	// AND ensureChannelRoomsAreInAGroup has run. Without this the
	// test thread races ahead and issues reads against an empty
	// projection (RoomCatalog returns "not found" for rooms whose
	// RoomCreated hasn't been applied yet), and SeedDefaultRooms
	// calls would seed rooms without a group assignment.
	bootCtx, bootCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer bootCancel()
	if err := core.WaitForBoot(bootCtx); err != nil {
		t.Fatalf("WaitForBoot: %v", err)
	}
}

func TestNewChattoCoreInitializesNotificationPreferencesService(t *testing.T) {
	core, _ := setupTestCore(t)

	if core.NotificationPreferences() == nil {
		t.Fatal("NotificationPreferences() = nil")
	}
}

func eventStreamMsgCount(t *testing.T, core *ChattoCore) uint64 {
	t.Helper()

	ctx := testContext(t)
	stream, err := core.EventStreamForDebug(ctx)
	if err != nil {
		t.Fatalf("event stream: %v", err)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatalf("event stream info: %v", err)
	}
	return info.State.Msgs
}

func TestCreateJetStreamResourceWithRetryRetriesStoreCreateFailure(t *testing.T) {
	ctx := testContext(t)
	attempts := 0

	got, err := createJetStreamResourceWithRetry(ctx, func(context.Context) (string, error) {
		attempts++
		if attempts < 3 {
			return "", &jetstream.APIError{
				Code:        500,
				ErrorCode:   10049,
				Description: "error creating store for stream",
			}
		}
		return "created", nil
	})

	if err != nil {
		t.Fatalf("createJetStreamResourceWithRetry error = %v", err)
	}
	if got != "created" {
		t.Fatalf("resource = %q, want created", got)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestCreateJetStreamResourceWithRetryDoesNotRetryOtherErrors(t *testing.T) {
	ctx := testContext(t)
	attempts := 0
	wantErr := errors.New("not retriable")

	_, err := createJetStreamResourceWithRetry(ctx, func(context.Context) (string, error) {
		attempts++
		return "", wantErr
	})

	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
}

func TestNewChattoCore_DoesNotProvisionLegacyImportResourcesOnFreshBoot(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	ctx := testContext(t)
	cfg := config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	}

	core, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}

	for _, bucket := range []string{
		"INSTANCE",
		"INSTANCE_CONFIG",
		"SERVER_CONFIG",
		"SERVER_RBAC",
		"SERVER_RUNTIME",
		"SERVER_BODIES",
		"SERVER_REACTIONS",
		"USER_PRESENCE",
		"CALL_STATE",
	} {
		if _, err := core.js.KeyValue(ctx, bucket); !errors.Is(err, jetstream.ErrBucketNotFound) {
			t.Fatalf("legacy bucket %s lookup error = %v, want ErrBucketNotFound", bucket, err)
		}
	}
	if _, err := core.js.KeyValue(ctx, "MEMORY_CACHE"); err != nil {
		t.Fatalf("MEMORY_CACHE lookup error = %v", err)
	}
	if _, err := core.js.Stream(ctx, "SERVER_EVENTS"); !errors.Is(err, jetstream.ErrStreamNotFound) {
		t.Fatalf("legacy stream SERVER_EVENTS lookup error = %v, want ErrStreamNotFound", err)
	}
	if _, err := core.js.ObjectStore(ctx, "INSTANCE_ASSETS"); !errors.Is(err, jetstream.ErrBucketNotFound) {
		t.Fatalf("legacy object store INSTANCE_ASSETS lookup error = %v, want ErrBucketNotFound", err)
	}
	if _, err := core.js.ObjectStore(ctx, "SERVER_ASSETS"); err != nil {
		t.Fatalf("SERVER_ASSETS lookup error = %v", err)
	}
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestChattoCore_RunReplaysProjectionsBeforeBootEnsures(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	cfg := config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	}

	start := func(t *testing.T, core *ChattoCore) func() {
		t.Helper()

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- core.Run(ctx) }()

		bootCtx, bootCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer bootCancel()
		if err := core.WaitForBoot(bootCtx); err != nil {
			cancel()
			t.Fatalf("WaitForBoot: %v", err)
		}

		return func() {
			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("core.Run did not stop within timeout")
			}
		}
	}

	ctx := testContext(t)
	first, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatalf("first core: %v", err)
	}
	stopFirst := start(t, first)
	if err := first.SeedDefaultRooms(ctx); err != nil {
		stopFirst()
		t.Fatalf("seed default rooms: %v", err)
	}
	eventsAfterFirstBoot := eventStreamMsgCount(t, first)
	stopFirst()

	second, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatalf("second core: %v", err)
	}
	stopSecond := start(t, second)
	defer stopSecond()
	if err := second.SeedDefaultRooms(ctx); err != nil {
		t.Fatalf("seed default rooms after restart: %v", err)
	}
	eventsAfterSecondBoot := eventStreamMsgCount(t, second)

	if eventsAfterSecondBoot != eventsAfterFirstBoot {
		t.Fatalf("expected restart boot to append no events, got %d -> %d", eventsAfterFirstBoot, eventsAfterSecondBoot)
	}
}

func TestChattoCore_RunAppliesConfigOwnersToExistingVerifiedUsers(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	cfg := config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	}

	start := func(t *testing.T, core *ChattoCore) func() {
		t.Helper()

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { done <- core.Run(ctx) }()

		bootCtx, bootCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer bootCancel()
		if err := core.WaitForBoot(bootCtx); err != nil {
			cancel()
			t.Fatalf("WaitForBoot: %v", err)
		}

		return func() {
			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("core.Run did not stop within timeout")
			}
		}
	}

	ctx := testContext(t)
	first, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatalf("first core: %v", err)
	}
	stopFirst := start(t, first)
	user, err := first.CreateVerifiedUser(ctx, SystemActorID, "retro-owner", "Retro Owner", "password123", "owner@example.com")
	if err != nil {
		stopFirst()
		t.Fatalf("create verified user: %v", err)
	}
	if isOwner, err := first.IsServerOwner(ctx, user.Id); err != nil || isOwner {
		stopFirst()
		t.Fatalf("user should not be owner before owners.emails is configured, owner=%v err=%v", isOwner, err)
	}
	stopFirst()

	cfg.Owners = config.OwnersConfig{Emails: []string{"OWNER@example.com"}}
	second, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatalf("second core: %v", err)
	}
	stopSecond := start(t, second)
	if isOwner, err := second.IsServerOwner(ctx, user.Id); err != nil || !isOwner {
		stopSecond()
		t.Fatalf("user should be owner after owners.emails boot sync, owner=%v err=%v", isOwner, err)
	}
	eventsAfterPromotion := eventStreamMsgCount(t, second)
	stopSecond()

	third, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatalf("third core: %v", err)
	}
	stopThird := start(t, third)
	defer stopThird()
	eventsAfterRestart := eventStreamMsgCount(t, third)
	if eventsAfterRestart != eventsAfterPromotion {
		t.Fatalf("expected owner boot sync to be idempotent, got %d -> %d events", eventsAfterPromotion, eventsAfterRestart)
	}
}

// TestChattoCore_FullWorkflow tests an end-to-end workflow demonstrating
// all core functionality working together.
func TestChattoCore_FullWorkflow(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, err := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Create a space

	// Create multiple rooms
	room1, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "General", "General discussion")
	if err != nil {
		t.Fatalf("Failed to create room 1: %v", err)
	}

	room2, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "Random", "Random chat")
	if err != nil {
		t.Fatalf("Failed to create room 2: %v", err)
	}

	// Verify rooms can be listed
	rooms, err := core.ListRooms(ctx, KindChannel)
	if err != nil {
		t.Fatalf("Failed to list rooms: %v", err)
	}
	if len(rooms) != 2 {
		t.Errorf("Expected 2 rooms, got %d", len(rooms))
	}

	// Join the space first (required for room membership)

	// Join the rooms (required for posting messages)
	_, err = core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room1.Id)
	if err != nil {
		t.Fatalf("Failed to join room 1: %v", err)
	}

	_, err = core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room2.Id)
	if err != nil {
		t.Fatalf("Failed to join room 2: %v", err)
	}

	// Post messages to different rooms
	_, err = core.PostMessage(ctx, KindChannel, room1.Id, user.Id, "Hello in General", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message to room 1: %v", err)
	}

	_, err = core.PostMessage(ctx, KindChannel, room2.Id, user.Id, "Hello in Random", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message to room 2: %v", err)
	}

	// Verify user can authenticate
	authenticated, err := core.VerifyPassword(ctx, user.Login, "password123")
	if err != nil {
		t.Fatalf("Failed to verify password: %v", err)
	}
	if authenticated.Id != user.Id {
		t.Error("Authenticated user doesn't match")
	}
}

// ============================================================================
// Per-Space Bucket Cache Tests
// ============================================================================

// TestPerSpaceBucketCache_ConcurrentGetOrCreate verifies that concurrent calls to getOrCreate
// for the same space result in only one bucket being created (double-checked locking works).

// TestPerSpaceBucketCache_CachingWorks verifies that buckets are actually cached
// and subsequent calls return the same instance without recreating.

// TestPerSpaceBucketCache_DeleteAndRecreate verifies that cache deletion works
// and that a new bucket is created after deletion. Uses a non-server space so
// the lazycache code path is exercised (deployment-wide channel and DM room
// data bypasses the lazycache).

// TestPerSpaceBucketCache_BucketConfigured verifies that storage buckets are correctly configured.

// ============================================================================
// Instance Event Authorization Tests
// ============================================================================

// TestChattoCore_isAuthorizedForLiveEvent verifies the authorization logic
// for server-level events based on subject patterns.
func TestChattoCore_isAuthorizedForLiveEvent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create two users for testing
	userA, _ := core.CreateUser(ctx, "system", "userA", "User A", "")
	userB, _ := core.CreateUser(ctx, "system", "userB", "User B", "")

	tests := []struct {
		name       string
		userID     string
		subject    string
		wantResult bool
	}{
		// User-scoped events: private by default
		{
			name:       "user event - same user receives their own registration_completed",
			userID:     userA.Id,
			subject:    "live.sync.user." + userA.Id + ".registration_completed",
			wantResult: true,
		},
		{
			name:       "user event - other user does NOT receive registration_completed",
			userID:     userB.Id,
			subject:    "live.sync.user." + userA.Id + ".registration_completed",
			wantResult: false,
		},
		{
			name:       "user event - same user receives their own user_deleted",
			userID:     userA.Id,
			subject:    "live.sync.user." + userA.Id + ".user_deleted",
			wantResult: true,
		},
		{
			name:       "user event - other user does NOT receive user_deleted",
			userID:     userB.Id,
			subject:    "live.sync.user." + userA.Id + ".user_deleted",
			wantResult: false,
		},

		// Profile updates: broadcast to everyone (since profiles are public)
		{
			name:       "profile_updated - same user receives it",
			userID:     userA.Id,
			subject:    "live.sync.user." + userA.Id + ".profile_updated",
			wantResult: true,
		},
		{
			name:       "profile_updated - other user ALSO receives it (broadcast)",
			userID:     userB.Id,
			subject:    "live.sync.user." + userA.Id + ".profile_updated",
			wantResult: true,
		},

		// Config-scoped events (incl. server branding + room layout):
		// every authenticated user is implicitly a member, so both receive.
		{
			name:       "config event - user A receives it",
			userID:     userA.Id,
			subject:    "live.sync.config.server_updated",
			wantResult: true,
		},
		{
			name:       "config event - user B also receives it",
			userID:     userB.Id,
			subject:    "live.sync.config.server_updated",
			wantResult: true,
		},

		// Invalid subjects
		{
			name:       "invalid subject format - too few parts",
			userID:     userA.Id,
			subject:    "live.sync.user",
			wantResult: false,
		},
		{
			name:       "invalid subject format - wrong prefix",
			userID:     userA.Id,
			subject:    "wrong.prefix.user." + userA.Id + ".event",
			wantResult: false,
		},
		{
			name:       "unknown scope",
			userID:     userA.Id,
			subject:    "live.sync.unknown.someid.event",
			wantResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := core.isAuthorizedForLiveEvent(ctx, tt.userID, tt.subject)
			if result != tt.wantResult {
				t.Errorf("isAuthorizedForLiveEvent(%s, %s) = %v, want %v",
					tt.userID, tt.subject, result, tt.wantResult)
			}
		})
	}
}

// ============================================================================
// newEvent Tests
// ============================================================================

func TestNewSpaceEvent_PopulatesId(t *testing.T) {
	event := newEvent("test-actor", &corev1.Event{
		Event: &corev1.Event_RoomCreated{
			RoomCreated: &corev1.RoomCreatedEvent{
				RoomId: "test-room",
				Name:   "Test Room",
			},
		},
	})

	if event.Id == "" {
		t.Error("newEvent() should populate Id field")
	}

	if !strings.HasPrefix(event.Id, "E") {
		t.Errorf("newEvent() Id should start with 'E', got %s", event.Id)
	}

	if len(event.Id) != 15 {
		t.Errorf("newEvent() Id should be 15 characters, got %d", len(event.Id))
	}
}

func TestNewSpaceEvent_DoesNotOverwriteExistingId(t *testing.T) {
	existingId := "E12345678901234"
	event := newEvent("test-actor", &corev1.Event{
		Id: existingId,
		Event: &corev1.Event_RoomCreated{
			RoomCreated: &corev1.RoomCreatedEvent{
				RoomId: "test-room",
				Name:   "Test Room",
			},
		},
	})

	if event.Id != existingId {
		t.Errorf("newEvent() should not overwrite existing Id, got %s", event.Id)
	}
}

func TestNewSpaceEvent_PopulatesActorId(t *testing.T) {
	event := newEvent("test-actor", &corev1.Event{
		Event: &corev1.Event_RoomCreated{
			RoomCreated: &corev1.RoomCreatedEvent{},
		},
	})

	if event.ActorId != "test-actor" {
		t.Errorf("newEvent() should populate ActorId, got %s", event.ActorId)
	}
}

func TestNewSpaceEvent_PopulatesCreatedAt(t *testing.T) {
	event := newEvent("test-actor", &corev1.Event{
		Event: &corev1.Event_RoomCreated{
			RoomCreated: &corev1.RoomCreatedEvent{},
		},
	})

	if event.CreatedAt == nil {
		t.Error("newEvent() should populate CreatedAt field")
	}
}

// TestStreamMyEvents_ClosesOnSessionTerminated verifies that
// the server event stream closes after receiving a SessionTerminatedEvent,
// and that the event is delivered to the channel before it closes.
func TestStreamMyEvents_ClosesOnSessionTerminated(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, _ := core.CreateUser(ctx, "system", "sessionterm1", "Session Term User", "")

	// Start streaming server events
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	eventChan, err := core.StreamMyEvents(subCtx, user.Id)
	if err != nil {
		t.Fatalf("StreamMyEvents failed: %v", err)
	}

	// Give subscription time to establish
	time.Sleep(100 * time.Millisecond)

	// Publish session terminated event
	if err := core.PublishSessionTerminated(ctx, user.Id, "logout"); err != nil {
		t.Fatalf("PublishSessionTerminated failed: %v", err)
	}

	// Should receive the SessionTerminatedEvent, then the channel should close
	receivedEvent := false
	timeout := time.After(2 * time.Second)

	for {
		select {
		case event, ok := <-eventChan:
			if !ok {
				// Channel closed — expected after session terminated
				if !receivedEvent {
					t.Fatal("Channel closed before receiving SessionTerminatedEvent")
				}
				return // Success!
			}
			if st := EventSessionTerminated(event); st != nil {
				if st.Reason != "logout" {
					t.Errorf("Expected reason 'logout', got %q", st.Reason)
				}
				receivedEvent = true
				// Channel should close shortly after — keep reading
			}
		case <-timeout:
			if receivedEvent {
				t.Fatal("Received SessionTerminatedEvent but channel was not closed")
			}
			t.Fatal("Timeout waiting for SessionTerminatedEvent")
		}
	}
}

// ============================================================================
// StreamMyEvents Typing Indicator Tests
// ============================================================================

// TestStreamMyEvents_FiltersOwnTypingEvents verifies that typing indicator
// events are NOT delivered back to the user who published them. This is critical
// for multi-server clients where the frontend's currentUserId may differ from
// the remote server user ID, making client-side filtering unreliable.
func TestStreamMyEvents_FiltersOwnTypingEvents(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create two users
	user1, _ := core.CreateUser(ctx, "system", "typing1", "Typing User 1", "")
	user2, _ := core.CreateUser(ctx, "system", "typing2", "Typing User 2", "")

	// Create a space and room

	room, err := core.CreateRoom(ctx, user1.Id, KindChannel, "", "test-room", "")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	// Both users must explicitly join the room (CreateRoom doesn't auto-join)
	_, err = core.JoinRoom(ctx, user1.Id, KindChannel, user1.Id, room.Id)
	if err != nil {
		t.Fatalf("JoinRoom (user1) failed: %v", err)
	}

	_, err = core.JoinRoom(ctx, user2.Id, KindChannel, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("JoinRoom (user2) failed: %v", err)
	}

	// Start streaming space events for user1 (the one who will type)
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	eventChan, err := core.StreamMyEvents(subCtx, user1.Id)
	if err != nil {
		t.Fatalf("StreamMyEvents failed: %v", err)
	}

	// Give subscription time to establish
	time.Sleep(100 * time.Millisecond)

	// user1 publishes a typing indicator (their own typing)
	err = core.PublishTypingIndicator(ctx, user1.Id, KindChannel, room.Id, nil)
	if err != nil {
		t.Fatalf("PublishTypingIndicator failed: %v", err)
	}

	// user1 should NOT receive their own typing event
	select {
	case event := <-eventChan:
		if EventUserTyping(event) != nil {
			t.Error("User received their own typing event — should be filtered server-side")
		}
	case <-time.After(500 * time.Millisecond):
		// Expected: no typing event for self
	}

	// Now user2 publishes a typing indicator
	err = core.PublishTypingIndicator(ctx, user2.Id, KindChannel, room.Id, nil)
	if err != nil {
		t.Fatalf("PublishTypingIndicator failed: %v", err)
	}

	// user1 SHOULD receive user2's typing event
	timeout := time.After(2 * time.Second)
	for {
		select {
		case event := <-eventChan:
			if typing := EventUserTyping(event); typing != nil {
				if event.ActorID() != user2.Id {
					t.Errorf("Expected typing event from user2 (%s), got %s", user2.Id, event.ActorID())
				}
				return // Success!
			}
			// Other events might come through, keep waiting
		case <-timeout:
			t.Fatal("Timeout waiting for user2's typing event")
		}
	}
}

func TestFilterLiveSyncEvent_DropsMissingPayload(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	event, ok := core.filterLiveSyncEvent(ctx, "U1", map[string]struct{}{}, &nats.Msg{
		Subject: "live.sync.config.server_updated",
	}, &corev1.LiveEvent{
		Id:      "LIVE-empty",
		ActorId: "U1",
	})

	if ok {
		t.Fatal("expected empty LiveEvent to be rejected")
	}
	if event != nil {
		t.Fatalf("expected no delivered event, got %+v", event)
	}
}

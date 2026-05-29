package core

import (
	"bytes"
	"errors"
	"fmt"
	"sync"
	"testing"
)

// ============================================================================
// Room Membership Tests
// ============================================================================

func TestRoomMemberships_CreateOrUpdate(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup: Create space, user, and room first
	user, _ := core.CreateUser(ctx, "actor1", "testuser", "Test User", "password")
	room, _ := core.CreateRoom(ctx, "actor1", KindChannel, "", "test-room", "test-room Desc")

	// User must be a space member first

	// Create room membership
	membership, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to create room membership: %v", err)
	}

	if membership == nil {
		t.Fatal("Expected membership to be returned")
	}

	if membership.UserId != user.Id {
		t.Errorf("Expected user ID '%s', got '%s'", user.Id, membership.UserId)
	}

	if membership.RoomId != room.Id {
		t.Errorf("Expected room ID '%s', got '%s'", room.Id, membership.RoomId)
	}

	// Verify we can retrieve the membership
	retrieved, err := core.GetRoomMembership(ctx, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to get room membership: %v", err)
	}

	if retrieved.UserId != user.Id {
		t.Errorf("Expected user ID '%s', got '%s'", user.Id, retrieved.UserId)
	}

	if retrieved.RoomId != room.Id {
		t.Errorf("Expected room ID '%s', got '%s'", room.Id, retrieved.RoomId)
	}
}

func TestRoomMemberships_CreateOrUpdate_Idempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	user, _ := core.CreateUser(ctx, "actor1", "testuser", "Test User", "password")
	room, _ := core.CreateRoom(ctx, "actor1", KindChannel, "", "test-room", "test-room Desc")

	// Create first membership
	first, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to create first membership: %v", err)
	}

	// CreateOrUpdate is idempotent - calling it again should succeed
	second, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Errorf("CreateOrUpdate should be idempotent and succeed on duplicate, got error: %v", err)
	}

	// Both should have the same data
	if first.UserId != second.UserId || first.RoomId != second.RoomId {
		t.Error("Repeated CreateOrUpdate should return same membership data")
	}
}

func TestRoomMemberships_ConcurrentJoinPublishesSingleEvent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, _ := core.CreateUser(ctx, "actor1", "testuser", "Test User", "password")
	room, _ := core.CreateRoom(ctx, "actor1", KindChannel, "", "test-room", "test-room Desc")

	const joiners = 12
	start := make(chan struct{})
	errs := make(chan error, joiners)
	var wg sync.WaitGroup
	wg.Add(joiners)
	for range joiners {
		go func() {
			defer wg.Done()
			<-start
			_, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
			errs <- err
		}()
	}

	close(start)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("JoinRoom: %v", err)
		}
	}

	eventsResult, err := core.GetRoomEvents(ctx, KindChannel, room.Id, 50, nil)
	if err != nil {
		t.Fatalf("GetRoomEvents: %v", err)
	}

	joinCount := 0
	for _, event := range eventsResult.Events {
		if event.GetUserJoinedRoom() != nil && event.ActorId == user.Id {
			joinCount++
		}
	}
	if joinCount != 1 {
		t.Fatalf("expected exactly one UserJoinedRoom event, got %d", joinCount)
	}
}

func TestRoomMemberships_Get_NotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup space (required for per-space bucket)

	_, err := core.GetRoomMembership(ctx, KindChannel, "nonexistent-user", "nonexistent-room")
	if err == nil {
		t.Error("Expected error when getting nonexistent membership")
	}
}

func TestRoomMemberships_Exists(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	user, _ := core.CreateUser(ctx, "actor1", "testuser", "Test User", "password")
	room, _ := core.CreateRoom(ctx, "actor1", KindChannel, "", "test-room", "test-room Desc")

	// Check non-existent membership
	exists, err := core.RoomMembershipExists(ctx, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check membership existence: %v", err)
	}
	if exists {
		t.Error("Expected membership to not exist")
	}

	// Create membership
	_, err = core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to create membership: %v", err)
	}

	// Check existing membership
	exists, err = core.RoomMembershipExists(ctx, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check membership existence: %v", err)
	}
	if !exists {
		t.Error("Expected membership to exist")
	}
}

func TestRoomMemberships_Delete(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	user, _ := core.CreateUser(ctx, "actor1", "testuser", "Test User", "password")
	room, _ := core.CreateRoom(ctx, "actor1", KindChannel, "", "test-room", "test-room Desc")

	// Create membership
	_, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to create membership: %v", err)
	}

	// Verify it exists
	exists, err := core.RoomMembershipExists(ctx, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check membership existence: %v", err)
	}
	if !exists {
		t.Error("Expected membership to exist before deletion")
	}

	// Delete membership
	err = core.LeaveRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to delete membership: %v", err)
	}

	// Verify it no longer exists
	exists, err = core.RoomMembershipExists(ctx, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check membership existence after deletion: %v", err)
	}
	if exists {
		t.Error("Expected membership to not exist after deletion")
	}
}

func TestRoomMemberships_Delete_Idempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup

	// Delete is idempotent - deleting a non-existent membership should succeed
	err := core.LeaveRoom(ctx, "actor1", KindChannel, "nonexistent-user", "nonexistent-room")
	if err != nil {
		t.Errorf("Delete should be idempotent and succeed for non-existent membership, got error: %v", err)
	}
}

func TestRoomMemberships_GetForUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	user, _ := core.CreateUser(ctx, "actor1", "testuser", "Test User", "password")
	room1, _ := core.CreateRoom(ctx, "actor1", KindChannel, "", "room-1", "room-1 Desc")
	room2, _ := core.CreateRoom(ctx, "actor1", KindChannel, "", "room-2", "room-2 Desc")
	room3, _ := core.CreateRoom(ctx, "actor1", KindChannel, "", "room-3", "room-3 Desc")

	// Create memberships for user in multiple rooms
	_, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room1.Id)
	if err != nil {
		t.Fatalf("Failed to create membership for room1: %v", err)
	}

	_, err = core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room2.Id)
	if err != nil {
		t.Fatalf("Failed to create membership for room2: %v", err)
	}

	_, err = core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room3.Id)
	if err != nil {
		t.Fatalf("Failed to create membership for room3: %v", err)
	}

	// Retrieve all rooms for the user
	memberships, err := core.GetUserRoomMemberships(ctx, KindChannel, user.Id)
	if err != nil {
		t.Fatalf("Failed to get rooms for user: %v", err)
	}

	// Verify we got exactly 3 memberships
	if len(memberships) != 3 {
		t.Errorf("Expected 3 memberships, got %d", len(memberships))
	}

	// Verify all returned memberships have the correct userID
	for _, m := range memberships {
		if m.UserId != user.Id {
			t.Errorf("Expected user ID '%s', got '%s'", user.Id, m.UserId)
		}
	}

	// Verify we got all three rooms
	roomIDs := make(map[string]bool)
	for _, m := range memberships {
		roomIDs[m.RoomId] = true
	}

	expectedRooms := []string{room1.Id, room2.Id, room3.Id}
	for _, roomID := range expectedRooms {
		if !roomIDs[roomID] {
			t.Errorf("Expected to find room %s in results", roomID)
		}
	}
}

func TestRoomMemberships_GetForUser_NoRooms(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	user, _ := core.CreateUser(ctx, "actor1", "testuser", "Test User", "password")

	// Get rooms for a user with no memberships
	memberships, err := core.GetUserRoomMemberships(ctx, KindChannel, user.Id)
	if err != nil {
		t.Fatalf("Failed to get rooms for user with no memberships: %v", err)
	}

	// Should return empty result
	if len(memberships) != 0 {
		t.Errorf("Expected 0 memberships, got %d", len(memberships))
	}
}

func TestRoomMemberships_GetForRoom(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	user1, _ := core.CreateUser(ctx, "actor1", "user1", "User 1", "password")
	user2, _ := core.CreateUser(ctx, "actor1", "user2", "User 2", "password")
	user3, _ := core.CreateUser(ctx, "actor1", "user3", "User 3", "password")
	room, _ := core.CreateRoom(ctx, "actor1", KindChannel, "", "test-room", "test-room Desc")

	// All users must be space members first

	// Create memberships for multiple users in the same room
	_, err := core.JoinRoom(ctx, user1.Id, KindChannel, user1.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to create membership for user1: %v", err)
	}

	_, err = core.JoinRoom(ctx, user2.Id, KindChannel, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to create membership for user2: %v", err)
	}

	_, err = core.JoinRoom(ctx, user3.Id, KindChannel, user3.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to create membership for user3: %v", err)
	}

	// Retrieve all users in the room
	memberships, err := core.GetRoomMembersList(ctx, KindChannel, room.Id)
	if err != nil {
		t.Fatalf("Failed to get users for room: %v", err)
	}

	// Verify we got exactly 3 memberships
	if len(memberships) != 3 {
		t.Errorf("Expected 3 memberships, got %d", len(memberships))
	}

	// Verify all returned memberships have the correct roomID
	for _, m := range memberships {
		if m.RoomId != room.Id {
			t.Errorf("Expected room ID '%s', got '%s'", room.Id, m.RoomId)
		}
	}

	// Verify we got all three users
	userIDs := make(map[string]bool)
	for _, m := range memberships {
		userIDs[m.UserId] = true
	}

	expectedUsers := []string{user1.Id, user2.Id, user3.Id}
	for _, userID := range expectedUsers {
		if !userIDs[userID] {
			t.Errorf("Expected to find user %s in results", userID)
		}
	}
}

func TestRoomMemberships_GetForRoom_NoMembers(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	room, _ := core.CreateRoom(ctx, "actor1", KindChannel, "", "test-room", "test-room Desc")

	// Get users for a room with no memberships
	memberships, err := core.GetRoomMembersList(ctx, KindChannel, room.Id)
	if err != nil {
		t.Fatalf("Failed to get users for room with no memberships: %v", err)
	}

	// Should return empty result
	if len(memberships) != 0 {
		t.Errorf("Expected 0 memberships, got %d", len(memberships))
	}
}

func TestRoomMemberships_DeleteAfterRecreate(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	user, _ := core.CreateUser(ctx, "actor1", "testuser", "Test User", "password")
	room, _ := core.CreateRoom(ctx, "actor1", KindChannel, "", "test-room", "test-room Desc")

	// Create membership
	_, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to create initial membership: %v", err)
	}

	// Delete it
	err = core.LeaveRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to delete membership: %v", err)
	}

	// Recreate it (should succeed since it was deleted)
	_, err = core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to recreate membership: %v", err)
	}

	// Verify it exists
	exists, err := core.RoomMembershipExists(ctx, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check recreated membership: %v", err)
	}
	if !exists {
		t.Error("Expected recreated membership to exist")
	}
}

func TestRoomMemberships_Integration_CompleteLifecycle(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	user, _ := core.CreateUser(ctx, "actor1", "integrationuser", "Integration User", "password")
	room, _ := core.CreateRoom(ctx, "actor1", KindChannel, "", "integration-room", "integration-room Desc")

	// 1. Verify doesn't exist
	exists, err := core.RoomMembershipExists(ctx, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Initial existence check failed: %v", err)
	}
	if exists {
		t.Error("Membership should not exist initially")
	}

	// 2. Create
	created, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Creation failed: %v", err)
	}
	if created.UserId != user.Id || created.RoomId != room.Id {
		t.Error("Created membership has incorrect data")
	}

	// 3. Verify exists
	exists, err = core.RoomMembershipExists(ctx, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Existence check after creation failed: %v", err)
	}
	if !exists {
		t.Error("Membership should exist after creation")
	}

	// 4. Get and verify data persisted correctly
	retrieved, err := core.GetRoomMembership(ctx, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Get after creation failed: %v", err)
	}
	if retrieved.UserId != user.Id || retrieved.RoomId != room.Id {
		t.Error("Retrieved membership has incorrect data")
	}

	// 5. Delete
	err = core.LeaveRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Deletion failed: %v", err)
	}

	// 6. Verify deleted
	exists, err = core.RoomMembershipExists(ctx, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Existence check after deletion failed: %v", err)
	}
	if exists {
		t.Error("Membership should not exist after deletion")
	}

	// 7. Get should fail
	_, err = core.GetRoomMembership(ctx, KindChannel, user.Id, room.Id)
	if err == nil {
		t.Error("Get should fail after deletion")
	}

	// 8. Second delete should succeed (idempotent behavior)
	err = core.LeaveRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	if err != nil {
		t.Errorf("Second delete should succeed due to idempotent behavior, got error: %v", err)
	}
}

// ============================================================================
// JoinRoom + Archive Interaction Tests
// ============================================================================

func TestChattoCore_JoinRoom_ArchivedRoom(t *testing.T) {
	t.Run("cannot join archived room", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		room, _ := core.CreateRoom(ctx, "owner", KindChannel, "", "general", "")

		_, err := core.ArchiveRoom(ctx, "owner", KindChannel, room.Id)
		if err != nil {
			t.Fatalf("ArchiveRoom failed: %v", err)
		}

		// New user joins the space
		newUser := "new-user"

		// Try to join the archived room
		_, err = core.JoinRoom(ctx, newUser, KindChannel, newUser, room.Id)
		if err == nil {
			t.Error("Expected error when joining archived room")
		}
		if err != nil && !errors.Is(err, fmt.Errorf("cannot join archived room")) {
			// Just check it contains the expected message
			if !bytes.Contains([]byte(err.Error()), []byte("cannot join archived room")) {
				t.Errorf("Expected 'cannot join archived room' error, got: %v", err)
			}
		}
	})

	t.Run("existing members remain after archive", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		room, _ := core.CreateRoom(ctx, "owner", KindChannel, "", "general", "")

		// User joins the room first
		user := "member"
		_, err := core.JoinRoom(ctx, user, KindChannel, user, room.Id)
		if err != nil {
			t.Fatalf("JoinRoom failed: %v", err)
		}

		// Archive the room
		_, err = core.ArchiveRoom(ctx, "owner", KindChannel, room.Id)
		if err != nil {
			t.Fatalf("ArchiveRoom failed: %v", err)
		}

		// Existing membership should still be there
		exists, err := core.RoomMembershipExists(ctx, KindChannel, user, room.Id)
		if err != nil {
			t.Fatalf("RoomMembershipExists failed: %v", err)
		}
		if !exists {
			t.Error("Expected existing room membership to remain after archiving")
		}
	})
}

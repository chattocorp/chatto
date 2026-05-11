package core

import (
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

func TestChattoCore_CreateSpace(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, err := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	if space == nil {
		t.Fatal("Expected space to be returned")
	}

	if space.Id == "" {
		t.Error("Expected space ID to be set")
	}

	if space.Name != "Test Space" {
		t.Errorf("Expected space name 'Test Space', got '%s'", space.Name)
	}

	if space.Description != "A test space" {
		t.Errorf("Expected description 'A test space', got '%s'", space.Description)
	}

	// Verify we can retrieve the space
	retrieved, err := core.GetSpace(ctx, space.Id)
	if err != nil {
		t.Fatalf("Failed to get space: %v", err)
	}

	if retrieved.Id != space.Id {
		t.Errorf("Expected space ID '%s', got '%s'", space.Id, retrieved.Id)
	}
}

func TestChattoCore_CreateSpace_EagerResourceCreation(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := testContext(t)

	space, err := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	// Create JetStream context to verify resources
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("Failed to create JetStream context: %v", err)
	}

	// The first non-DM space is auto-promoted to be the deployment's
	// server space, so its data lives in the shared SERVER_* buckets
	// (eager-created in newStorage) rather than per-space buckets.
	_ = space
	serverBuckets := []string{
		"SERVER_CONFIG",
		"SERVER_RBAC",
		"SERVER_RUNTIME",
		"SERVER_BODIES",
		"SERVER_REACTIONS",
		"SERVER_THREADS",
	}
	for _, bucketName := range serverBuckets {
		if _, err := js.KeyValue(ctx, bucketName); err != nil {
			t.Errorf("Expected KV bucket %s to exist, got error: %v", bucketName, err)
		}
	}
	if _, err := js.ObjectStore(ctx, "SERVER_ASSETS"); err != nil {
		t.Errorf("Expected SERVER_ASSETS object store to exist, got error: %v", err)
	}
	if _, err := js.Stream(ctx, "SERVER_EVENTS"); err != nil {
		t.Errorf("Expected SERVER_EVENTS stream to exist, got error: %v", err)
	}
}

func TestChattoCore_GetSpace_NotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, err := core.GetSpace(ctx, "nonexistent")
	if err == nil {
		t.Error("Expected error when getting nonexistent space")
	}
}

// TestChattoCore_CreateSpace_DescriptionTooLong tests that oversized descriptions are rejected.
// This is a security test to prevent storage issues and DoS.
func TestChattoCore_CreateSpace_DescriptionTooLong(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("description at max length succeeds", func(t *testing.T) {
		// Create a description at exactly the max length
		maxDesc := make([]byte, MaxDescriptionLength)
		for i := range maxDesc {
			maxDesc[i] = 'a'
		}

		_, err := core.CreateSpace(ctx, "test-user", "MaxDescSpace", string(maxDesc))
		if err != nil {
			t.Errorf("Expected success for description at max length, got: %v", err)
		}
	})

	t.Run("description over max length fails", func(t *testing.T) {
		// Create a description over the max length
		oversizedDesc := make([]byte, MaxDescriptionLength+1)
		for i := range oversizedDesc {
			oversizedDesc[i] = 'a'
		}

		_, err := core.CreateSpace(ctx, "test-user", "OversizedDescSpace", string(oversizedDesc))
		if err == nil {
			t.Error("Expected error for oversized description")
		}
		if err != ErrDescriptionTooLong {
			t.Errorf("Expected ErrDescriptionTooLong, got: %v", err)
		}
	})
}

// TestChattoCore_CreateSpace_NameTooLong tests that oversized space names are rejected.
func TestChattoCore_CreateSpace_NameTooLong(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("name at max length succeeds", func(t *testing.T) {
		// Create a name at exactly the max length
		maxName := make([]byte, MaxSpaceNameLength)
		for i := range maxName {
			maxName[i] = 'a'
		}

		_, err := core.CreateSpace(ctx, "test-user", string(maxName), "Description")
		if err != nil {
			t.Errorf("Expected success for name at max length, got: %v", err)
		}
	})

	t.Run("name over max length fails", func(t *testing.T) {
		// Create a name over the max length
		oversizedName := make([]byte, MaxSpaceNameLength+1)
		for i := range oversizedName {
			oversizedName[i] = 'a'
		}

		_, err := core.CreateSpace(ctx, "test-user", string(oversizedName), "Description")
		if err == nil {
			t.Error("Expected error for oversized name")
		}
		if err != ErrSpaceNameTooLong {
			t.Errorf("Expected ErrSpaceNameTooLong, got: %v", err)
		}
	})
}

// TestChattoCore_UpdateSpace_NameTooLong tests that oversized space names are rejected on update.
func TestChattoCore_UpdateSpace_NameTooLong(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a space
	space, err := core.CreateSpace(ctx, "test-user", "Original Name", "Original description")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	t.Run("update to max length succeeds", func(t *testing.T) {
		maxName := make([]byte, MaxSpaceNameLength)
		for i := range maxName {
			maxName[i] = 'b'
		}

		_, err := core.UpdateSpace(ctx, "test-user", space.Id, string(maxName), "Description")
		if err != nil {
			t.Errorf("Expected success for name at max length, got: %v", err)
		}
	})

	t.Run("update to over max length fails", func(t *testing.T) {
		oversizedName := make([]byte, MaxSpaceNameLength+1)
		for i := range oversizedName {
			oversizedName[i] = 'c'
		}

		_, err := core.UpdateSpace(ctx, "test-user", space.Id, string(oversizedName), "Description")
		if err == nil {
			t.Error("Expected error for oversized name")
		}
		if err != ErrSpaceNameTooLong {
			t.Errorf("Expected ErrSpaceNameTooLong, got: %v", err)
		}
	})
}

// TestChattoCore_UpdateSpace_DescriptionTooLong tests that oversized descriptions are rejected on update.
func TestChattoCore_UpdateSpace_DescriptionTooLong(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a space
	space, err := core.CreateSpace(ctx, "test-user", "Update Desc Space", "Original description")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	t.Run("update to max length succeeds", func(t *testing.T) {
		maxDesc := make([]byte, MaxDescriptionLength)
		for i := range maxDesc {
			maxDesc[i] = 'b'
		}

		_, err := core.UpdateSpace(ctx, "test-user", space.Id, "Update Desc Space", string(maxDesc))
		if err != nil {
			t.Errorf("Expected success for description at max length, got: %v", err)
		}
	})

	t.Run("update to over max length fails", func(t *testing.T) {
		oversizedDesc := make([]byte, MaxDescriptionLength+1)
		for i := range oversizedDesc {
			oversizedDesc[i] = 'c'
		}

		_, err := core.UpdateSpace(ctx, "test-user", space.Id, "Update Desc Space", string(oversizedDesc))
		if err == nil {
			t.Error("Expected error for oversized description")
		}
		if err != ErrDescriptionTooLong {
			t.Errorf("Expected ErrDescriptionTooLong, got: %v", err)
		}
	})
}

func TestChattoCore_CreateMultipleSpaces(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space1, err := core.CreateSpace(ctx, "test-user", "Space 1", "First space")
	if err != nil {
		t.Fatalf("Failed to create space 1: %v", err)
	}

	space2, err := core.CreateSpace(ctx, "test-user", "Space 2", "Second space")
	if err != nil {
		t.Fatalf("Failed to create space 2: %v", err)
	}

	if space1.Id == space2.Id {
		t.Error("Expected different IDs for different spaces")
	}

	// Verify both can be retrieved
	retrieved1, _ := core.GetSpace(ctx, space1.Id)
	retrieved2, _ := core.GetSpace(ctx, space2.Id)

	if retrieved1.Name != "Space 1" {
		t.Errorf("Expected 'Space 1', got '%s'", retrieved1.Name)
	}

	if retrieved2.Name != "Space 2" {
		t.Errorf("Expected 'Space 2', got '%s'", retrieved2.Name)
	}
}

func TestChattoCore_ListSpaces(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Initially should only have the DM system space
	spaces, err := core.ListSpaces(ctx)
	if err != nil {
		t.Fatalf("Failed to list spaces: %v", err)
	}
	if len(spaces) != 1 {
		t.Errorf("Expected 1 space (DM space), got %d", len(spaces))
	}
	if spaces[0].Id != DMSpaceID {
		t.Errorf("Expected DM space, got %s", spaces[0].Id)
	}

	// Create some spaces
	space1, _ := core.CreateSpace(ctx, "test-user", "Space 1", "First")
	space2, _ := core.CreateSpace(ctx, "test-user", "Space 2", "Second")
	space3, _ := core.CreateSpace(ctx, "test-user", "Space 3", "Third")

	// List should return all spaces including DM space
	spaces, err = core.ListSpaces(ctx)
	if err != nil {
		t.Fatalf("Failed to list spaces: %v", err)
	}
	if len(spaces) != 4 {
		t.Errorf("Expected 4 spaces (3 + DM space), got %d", len(spaces))
	}

	// Verify all user-created spaces are present
	ids := make(map[string]bool)
	for _, space := range spaces {
		ids[space.Id] = true
	}
	if !ids[space1.Id] || !ids[space2.Id] || !ids[space3.Id] {
		t.Error("Not all created spaces were returned by ListSpaces")
	}
	if !ids[DMSpaceID] {
		t.Error("DM space should be in ListSpaces")
	}
}

func TestChattoCore_UpdateSpace(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a space
	space, err := core.CreateSpace(ctx, "test-user", "Original Name", "Original Description")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	// Update the space
	updated, err := core.UpdateSpace(ctx, "test-user", space.Id, "Updated Name", "Updated Description")
	if err != nil {
		t.Fatalf("Failed to update space: %v", err)
	}

	if updated.Name != "Updated Name" {
		t.Errorf("Expected name 'Updated Name', got '%s'", updated.Name)
	}
	if updated.Description != "Updated Description" {
		t.Errorf("Expected description 'Updated Description', got '%s'", updated.Description)
	}

	// Verify the update persisted
	retrieved, err := core.GetSpace(ctx, space.Id)
	if err != nil {
		t.Fatalf("Failed to get updated space: %v", err)
	}

	if retrieved.Name != "Updated Name" {
		t.Errorf("Updated name not persisted: got '%s'", retrieved.Name)
	}
	if retrieved.Description != "Updated Description" {
		t.Errorf("Updated description not persisted: got '%s'", retrieved.Description)
	}
}

func TestChattoCore_UpdateSpace_NotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, err := core.UpdateSpace(ctx, "test-user", "nonexistent", "New Name", "New Desc")
	if err == nil {
		t.Error("Expected error when updating nonexistent space")
	}
}

func TestChattoCore_DeleteSpace(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a space
	space, err := core.CreateSpace(ctx, "test-user", "To Delete", "Will be deleted")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	// Verify it exists
	_, err = core.GetSpace(ctx, space.Id)
	if err != nil {
		t.Fatalf("Space should exist: %v", err)
	}

	// Delete the space
	err = core.DeleteSpace(ctx, "test-user", space.Id)
	if err != nil {
		t.Fatalf("Failed to delete space: %v", err)
	}

	// Verify it's gone
	_, err = core.GetSpace(ctx, space.Id)
	if err == nil {
		t.Error("Expected error when getting deleted space")
	}
}

func TestChattoCore_DeleteSpace_NotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	err := core.DeleteSpace(ctx, "test-user", "nonexistent")
	if err == nil {
		t.Error("Expected error when deleting nonexistent space")
	}
}

func TestChattoCore_ConcurrentSpaceUpdate(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a space
	space, err := core.CreateSpace(ctx, "test-user", "Test Space", "Original Description")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	// Try to update the space twice concurrently
	errChan := make(chan error, 2)

	go func() {
		_, err := core.UpdateSpace(ctx, "test-user", space.Id, "Updated by goroutine 1", "Description 1")
		errChan <- err
	}()

	go func() {
		_, err := core.UpdateSpace(ctx, "test-user", space.Id, "Updated by goroutine 2", "Description 2")
		errChan <- err
	}()

	// Collect results
	err1 := <-errChan
	err2 := <-errChan

	// Both should succeed (last writer wins in KV)
	if err1 != nil {
		t.Errorf("First update failed: %v", err1)
	}
	if err2 != nil {
		t.Errorf("Second update failed: %v", err2)
	}

	// Verify the space still exists and has one of the updates
	final, err := core.GetSpace(ctx, space.Id)
	if err != nil {
		t.Fatalf("Failed to get final space state: %v", err)
	}

	// The final state should be one of the two updates
	if final.Name != "Updated by goroutine 1" && final.Name != "Updated by goroutine 2" {
		t.Errorf("Expected space to have one of the concurrent updates, got: %s", final.Name)
	}
}


// ============================================================================
// AutoJoinDefaultRooms Tests
// ============================================================================

func TestAutoJoinDefaultRooms_JoinsAutoJoinRooms(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	creatorID := "creator123"
	space, err := core.CreateSpace(ctx, creatorID, "Test Space", "")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	generalRoom, err := core.CreateRoom(ctx, creatorID, space.Id, "general", "")
	if err != nil {
		t.Fatalf("Failed to create general room: %v", err)
	}
	if _, err := core.SetRoomAutoJoin(ctx, creatorID, space.Id, generalRoom.Id, true); err != nil {
		t.Fatalf("Failed to set auto_join: %v", err)
	}

	secretRoom, err := core.CreateRoom(ctx, creatorID, space.Id, "secret", "")
	if err != nil {
		t.Fatalf("Failed to create secret room: %v", err)
	}

	newUserID := "newuser456"
	core.AutoJoinDefaultRooms(ctx, space.Id, newUserID)

	inGeneral, err := core.RoomMembershipExists(ctx, space.Id, newUserID, generalRoom.Id)
	if err != nil {
		t.Fatalf("RoomMembershipExists: %v", err)
	}
	if !inGeneral {
		t.Error("Expected user to be auto-joined to 'general'")
	}

	inSecret, err := core.RoomMembershipExists(ctx, space.Id, newUserID, secretRoom.Id)
	if err != nil {
		t.Fatalf("RoomMembershipExists: %v", err)
	}
	if inSecret {
		t.Error("Did not expect user to be auto-joined to 'secret'")
	}
}

func TestAutoJoinDefaultRooms_SkipsArchivedRooms(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	creatorID := "creator123"
	space, err := core.CreateSpace(ctx, creatorID, "Test Space", "")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	archivedRoom, err := core.CreateRoom(ctx, creatorID, space.Id, "archived", "")
	if err != nil {
		t.Fatalf("Failed to create archived room: %v", err)
	}
	if _, err := core.SetRoomAutoJoin(ctx, creatorID, space.Id, archivedRoom.Id, true); err != nil {
		t.Fatalf("Failed to set auto_join: %v", err)
	}
	if _, err := core.ArchiveRoom(ctx, creatorID, space.Id, archivedRoom.Id); err != nil {
		t.Fatalf("Failed to archive room: %v", err)
	}

	newUserID := "newuser456"
	core.AutoJoinDefaultRooms(ctx, space.Id, newUserID)

	in, err := core.RoomMembershipExists(ctx, space.Id, newUserID, archivedRoom.Id)
	if err != nil {
		t.Fatalf("RoomMembershipExists: %v", err)
	}
	if in {
		t.Error("Did not expect user to be auto-joined to archived room")
	}
}

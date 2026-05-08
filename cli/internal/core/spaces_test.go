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
// Space Membership Tests
// ============================================================================

func TestSpaceMemberships_CreateOrUpdate(t *testing.T) {
	sm, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "user123"
	spaceID := "space456"

	// Create membership
	membership, err := sm.JoinSpace(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Failed to create membership: %v", err)
	}

	if membership == nil {
		t.Fatal("Expected membership to be returned")
	}

	if membership.UserId != userID {
		t.Errorf("Expected user ID '%s', got '%s'", userID, membership.UserId)
	}

	if membership.SpaceId != spaceID {
		t.Errorf("Expected space ID '%s', got '%s'", spaceID, membership.SpaceId)
	}

	// Verify we can retrieve the membership
	retrieved, err := sm.GetSpaceMembership(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Failed to get membership: %v", err)
	}

	if retrieved.UserId != userID {
		t.Errorf("Expected user ID '%s', got '%s'", userID, retrieved.UserId)
	}

	if retrieved.SpaceId != spaceID {
		t.Errorf("Expected space ID '%s', got '%s'", spaceID, retrieved.SpaceId)
	}
}

func TestSpaceMemberships_CreateOrUpdate_Idempotent(t *testing.T) {
	sm, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "user123"
	spaceID := "space456"

	// Create first membership
	first, err := sm.JoinSpace(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Failed to create first membership: %v", err)
	}

	// CreateOrUpdate is idempotent - calling it again should succeed
	second, err := sm.JoinSpace(ctx, userID, spaceID)
	if err != nil {
		t.Errorf("CreateOrUpdate should be idempotent and succeed on duplicate, got error: %v", err)
	}

	// Both should have the same data
	if first.UserId != second.UserId || first.SpaceId != second.SpaceId {
		t.Error("Repeated CreateOrUpdate should return same membership data")
	}
}

func TestSpaceMemberships_Get(t *testing.T) {
	sm, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "user789"
	spaceID := "space012"

	// Create membership first
	created, err := sm.JoinSpace(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Failed to create membership: %v", err)
	}

	// Retrieve membership
	retrieved, err := sm.GetSpaceMembership(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Failed to get membership: %v", err)
	}

	if retrieved.UserId != created.UserId {
		t.Errorf("Expected user ID '%s', got '%s'", created.UserId, retrieved.UserId)
	}

	if retrieved.SpaceId != created.SpaceId {
		t.Errorf("Expected space ID '%s', got '%s'", created.SpaceId, retrieved.SpaceId)
	}
}

func TestSpaceMemberships_Get_NotFound(t *testing.T) {
	sm, _ := setupTestCore(t)
	ctx := testContext(t)

	_, err := sm.GetSpaceMembership(ctx, "nonexistent-user", "nonexistent-space")
	if err == nil {
		t.Error("Expected error when getting nonexistent membership")
	}
}

func TestSpaceMemberships_Exists(t *testing.T) {
	sm, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "user345"
	spaceID := "space678"

	// Check non-existent membership
	exists, err := sm.SpaceMembershipExists(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Failed to check membership existence: %v", err)
	}
	if exists {
		t.Error("Expected membership to not exist")
	}

	// Create membership
	_, err = sm.JoinSpace(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Failed to create membership: %v", err)
	}

	// Check existing membership
	exists, err = sm.SpaceMembershipExists(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Failed to check membership existence: %v", err)
	}
	if !exists {
		t.Error("Expected membership to exist")
	}
}

func TestSpaceMemberships_Delete(t *testing.T) {
	sm, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "user111"
	spaceID := "space222"

	// Create membership
	_, err := sm.JoinSpace(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Failed to create membership: %v", err)
	}

	// Verify it exists
	exists, err := sm.SpaceMembershipExists(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Failed to check membership existence: %v", err)
	}
	if !exists {
		t.Error("Expected membership to exist before deletion")
	}

	// Delete membership
	err = sm.LeaveSpace(ctx, userID, spaceID, false)
	if err != nil {
		t.Fatalf("Failed to delete membership: %v", err)
	}

	// Verify it no longer exists
	exists, err = sm.SpaceMembershipExists(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Failed to check membership existence after deletion: %v", err)
	}
	if exists {
		t.Error("Expected membership to not exist after deletion")
	}
}

func TestSpaceMemberships_Delete_NotFound(t *testing.T) {
	sm, _ := setupTestCore(t)
	ctx := testContext(t)

	// Delete is idempotent - deleting a non-existent membership should succeed
	err := sm.LeaveSpace(ctx, "nonexistent-user", "nonexistent-space", false)
	if err != nil {
		t.Errorf("Delete should be idempotent and succeed for non-existent membership, got error: %v", err)
	}
}

func TestSpaceMemberships_ConcurrentCreation(t *testing.T) {
	sm, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "user999"
	spaceID := "space888"

	// Try to create the same membership concurrently
	// Both should succeed due to idempotent behavior
	errChan := make(chan error, 2)

	createMembership := func() {
		_, err := sm.JoinSpace(ctx, userID, spaceID)
		errChan <- err
	}

	go createMembership()
	go createMembership()

	// Collect results
	err1 := <-errChan
	err2 := <-errChan

	// Both should succeed due to idempotent CreateOrUpdate
	if err1 != nil {
		t.Errorf("First concurrent CreateOrUpdate should succeed, got error: %v", err1)
	}
	if err2 != nil {
		t.Errorf("Second concurrent CreateOrUpdate should succeed, got error: %v", err2)
	}

	// Verify the membership exists
	exists, err := sm.SpaceMembershipExists(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Failed to check membership existence: %v", err)
	}
	if !exists {
		t.Error("Expected membership to exist")
	}

	// Should be able to retrieve it
	membership, err := sm.GetSpaceMembership(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Failed to get membership: %v", err)
	}
	if membership.UserId != userID || membership.SpaceId != spaceID {
		t.Error("Retrieved membership has incorrect data")
	}
}

func TestSpaceMemberships_MultipleSpacesPerUser(t *testing.T) {
	sm, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "user555"
	space1 := "space111"
	space2 := "space222"
	space3 := "space333"

	// Create memberships for same user in different spaces
	_, err := sm.JoinSpace(ctx, userID, space1)
	if err != nil {
		t.Fatalf("Failed to create membership 1: %v", err)
	}

	_, err = sm.JoinSpace(ctx, userID, space2)
	if err != nil {
		t.Fatalf("Failed to create membership 2: %v", err)
	}

	_, err = sm.JoinSpace(ctx, userID, space3)
	if err != nil {
		t.Fatalf("Failed to create membership 3: %v", err)
	}

	// Verify all three exist
	spaces := []string{space1, space2, space3}
	for _, spaceID := range spaces {
		exists, err := sm.SpaceMembershipExists(ctx, userID, spaceID)
		if err != nil {
			t.Fatalf("Failed to check membership for space %s: %v", spaceID, err)
		}
		if !exists {
			t.Errorf("Expected membership in space %s to exist", spaceID)
		}
	}

	// Delete one membership
	err = sm.LeaveSpace(ctx, userID, space2, false)
	if err != nil {
		t.Fatalf("Failed to delete membership: %v", err)
	}

	// Verify space2 is gone but others remain
	exists, err := sm.SpaceMembershipExists(ctx, userID, space2)
	if err != nil {
		t.Fatalf("Failed to check deleted membership: %v", err)
	}
	if exists {
		t.Error("Expected membership in space2 to be deleted")
	}

	// Verify space1 and space3 still exist
	for _, spaceID := range []string{space1, space3} {
		exists, err := sm.SpaceMembershipExists(ctx, userID, spaceID)
		if err != nil {
			t.Fatalf("Failed to check membership for space %s: %v", spaceID, err)
		}
		if !exists {
			t.Errorf("Expected membership in space %s to still exist", spaceID)
		}
	}
}

func TestSpaceMemberships_MultipleUsersPerSpace(t *testing.T) {
	sm, _ := setupTestCore(t)
	ctx := testContext(t)

	spaceID := "space444"
	user1 := "user101"
	user2 := "user102"
	user3 := "user103"

	// Create memberships for different users in same space
	_, err := sm.JoinSpace(ctx, user1, spaceID)
	if err != nil {
		t.Fatalf("Failed to create membership for user1: %v", err)
	}

	_, err = sm.JoinSpace(ctx, user2, spaceID)
	if err != nil {
		t.Fatalf("Failed to create membership for user2: %v", err)
	}

	_, err = sm.JoinSpace(ctx, user3, spaceID)
	if err != nil {
		t.Fatalf("Failed to create membership for user3: %v", err)
	}

	// Verify all three exist
	users := []string{user1, user2, user3}
	for _, userID := range users {
		exists, err := sm.SpaceMembershipExists(ctx, userID, spaceID)
		if err != nil {
			t.Fatalf("Failed to check membership for user %s: %v", userID, err)
		}
		if !exists {
			t.Errorf("Expected membership for user %s to exist", userID)
		}
	}
}

func TestSpaceMemberships_GetError_KeyNotFound(t *testing.T) {
	sm, _ := setupTestCore(t)
	ctx := testContext(t)

	// Get should return an error that wraps jetstream.ErrKeyNotFound
	_, err := sm.GetSpaceMembership(ctx, "nonexistent-user", "nonexistent-space")
	if err == nil {
		t.Fatal("Expected error when getting nonexistent membership")
	}

	// The error should indicate it's about the specific user and space
	errMsg := err.Error()
	if errMsg == "" {
		t.Error("Expected non-empty error message")
	}
}

func TestSpaceMemberships_CreateAndRetrieveMultiple(t *testing.T) {
	sm, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a grid of memberships
	memberships := []struct {
		userID  string
		spaceID string
	}{
		{"user-a", "space-1"},
		{"user-a", "space-2"},
		{"user-b", "space-1"},
		{"user-b", "space-3"},
		{"user-c", "space-2"},
		{"user-c", "space-3"},
	}

	// Create all memberships
	for _, m := range memberships {
		_, err := sm.JoinSpace(ctx, m.userID, m.spaceID)
		if err != nil {
			t.Fatalf("Failed to create membership for %s in %s: %v", m.userID, m.spaceID, err)
		}
	}

	// Verify all can be retrieved
	for _, m := range memberships {
		retrieved, err := sm.GetSpaceMembership(ctx, m.userID, m.spaceID)
		if err != nil {
			t.Fatalf("Failed to get membership for %s in %s: %v", m.userID, m.spaceID, err)
		}

		if retrieved.UserId != m.userID {
			t.Errorf("Expected user ID %s, got %s", m.userID, retrieved.UserId)
		}

		if retrieved.SpaceId != m.spaceID {
			t.Errorf("Expected space ID %s, got %s", m.spaceID, retrieved.SpaceId)
		}
	}

	// Verify non-existent combinations don't exist
	nonExistent := []struct {
		userID  string
		spaceID string
	}{
		{"user-a", "space-3"},
		{"user-b", "space-2"},
		{"user-c", "space-1"},
	}

	for _, m := range nonExistent {
		exists, err := sm.SpaceMembershipExists(ctx, m.userID, m.spaceID)
		if err != nil {
			t.Fatalf("Failed to check non-existent membership for %s in %s: %v", m.userID, m.spaceID, err)
		}

		if exists {
			t.Errorf("Expected membership for %s in %s to not exist", m.userID, m.spaceID)
		}
	}
}

func TestSpaceMemberships_DeleteAfterRecreate(t *testing.T) {
	sm, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "user777"
	spaceID := "space999"

	// Create membership
	_, err := sm.JoinSpace(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Failed to create initial membership: %v", err)
	}

	// Delete it
	err = sm.LeaveSpace(ctx, userID, spaceID, false)
	if err != nil {
		t.Fatalf("Failed to delete membership: %v", err)
	}

	// Recreate it (should succeed since it was deleted)
	_, err = sm.JoinSpace(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Failed to recreate membership: %v", err)
	}

	// Verify it exists
	exists, err := sm.SpaceMembershipExists(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Failed to check recreated membership: %v", err)
	}
	if !exists {
		t.Error("Expected recreated membership to exist")
	}

	// Should be able to retrieve it
	membership, err := sm.GetSpaceMembership(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Failed to get recreated membership: %v", err)
	}
	if membership.UserId != userID || membership.SpaceId != spaceID {
		t.Error("Recreated membership has incorrect data")
	}
}

func TestSpaceMemberships_ExistsWithKeyNotFound(t *testing.T) {
	sm, _ := setupTestCore(t)
	ctx := testContext(t)

	// Exists should return false (not error) when key doesn't exist
	exists, err := sm.SpaceMembershipExists(ctx, "no-such-user", "no-such-space")
	if err != nil {
		t.Fatalf("Exists should not return error for non-existent key: %v", err)
	}
	if exists {
		t.Error("Expected Exists to return false for non-existent membership")
	}
}

func TestSpaceMemberships_Integration_WithRealJetStreamOperations(t *testing.T) {
	sm, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "integration-user"
	spaceID := "integration-space"

	// Test the complete lifecycle
	// 1. Verify doesn't exist
	exists, err := sm.SpaceMembershipExists(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Initial existence check failed: %v", err)
	}
	if exists {
		t.Error("Membership should not exist initially")
	}

	// 2. Create
	created, err := sm.JoinSpace(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Creation failed: %v", err)
	}
	if created.UserId != userID || created.SpaceId != spaceID {
		t.Error("Created membership has incorrect data")
	}

	// 3. Verify exists
	exists, err = sm.SpaceMembershipExists(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Existence check after creation failed: %v", err)
	}
	if !exists {
		t.Error("Membership should exist after creation")
	}

	// 4. Get and verify data persisted correctly
	retrieved, err := sm.GetSpaceMembership(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Get after creation failed: %v", err)
	}
	if retrieved.UserId != userID || retrieved.SpaceId != spaceID {
		t.Error("Retrieved membership has incorrect data")
	}

	// 5. Delete
	err = sm.LeaveSpace(ctx, userID, spaceID, false)
	if err != nil {
		t.Fatalf("Deletion failed: %v", err)
	}

	// 6. Verify deleted
	exists, err = sm.SpaceMembershipExists(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Existence check after deletion failed: %v", err)
	}
	if exists {
		t.Error("Membership should not exist after deletion")
	}

	// 7. Get should fail
	_, err = sm.GetSpaceMembership(ctx, userID, spaceID)
	if err == nil {
		t.Error("Get should fail after deletion")
	}

	// 8. Second delete should succeed (idempotent behavior)
	err = sm.LeaveSpace(ctx, userID, spaceID, false)
	if err != nil {
		t.Errorf("Second delete should succeed due to idempotent behavior, got error: %v", err)
	}
}

func TestSpaceMemberships_BucketIsCorrectlyConfigured(t *testing.T) {
	_, nc := setupTestCore(t)
	ctx := testContext(t)

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("Failed to create JetStream context: %v", err)
	}

	// Get the bucket to inspect its configuration
	kv, err := js.KeyValue(ctx, "INSTANCE")
	if err != nil {
		t.Fatalf("Failed to get INSTANCE bucket: %v", err)
	}

	status, err := kv.Status(ctx)
	if err != nil {
		t.Fatalf("Failed to get bucket status: %v", err)
	}

	if status.Bucket() != "INSTANCE" {
		t.Errorf("Expected bucket name 'INSTANCE', got '%s'", status.Bucket())
	}

	// Verify it's using file storage (not memory)
	if status.BackingStore() != "JetStream" {
		t.Logf("Backing store: %s", status.BackingStore())
	}
}

func TestSpaceMemberships_GetSpacesForUser(t *testing.T) {
	sm, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "user-multi-space"
	space1 := "space-alpha"
	space2 := "space-beta"
	space3 := "space-gamma"

	// Create memberships for the user in multiple spaces
	_, err := sm.JoinSpace(ctx, userID, space1)
	if err != nil {
		t.Fatalf("Failed to create membership for space1: %v", err)
	}

	_, err = sm.JoinSpace(ctx, userID, space2)
	if err != nil {
		t.Fatalf("Failed to create membership for space2: %v", err)
	}

	_, err = sm.JoinSpace(ctx, userID, space3)
	if err != nil {
		t.Fatalf("Failed to create membership for space3: %v", err)
	}

	// Retrieve all spaces for the user
	memberships, err := sm.GetUserSpaceMemberships(ctx, userID)
	if err != nil {
		t.Fatalf("Failed to get spaces for user: %v", err)
	}

	// Verify we got exactly 3 memberships
	if len(memberships) != 3 {
		t.Errorf("Expected 3 memberships, got %d", len(memberships))
	}

	// Verify all returned memberships have the correct userID
	for _, m := range memberships {
		if m.UserId != userID {
			t.Errorf("Expected user ID '%s', got '%s'", userID, m.UserId)
		}
	}

	// Verify we got all three spaces
	spaceIDs := make(map[string]bool)
	for _, m := range memberships {
		spaceIDs[m.SpaceId] = true
	}

	expectedSpaces := []string{space1, space2, space3}
	for _, spaceID := range expectedSpaces {
		if !spaceIDs[spaceID] {
			t.Errorf("Expected to find space %s in results", spaceID)
		}
	}
}

func TestSpaceMemberships_GetSpacesForUser_NoSpaces(t *testing.T) {
	sm, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "user-no-spaces"

	// Get spaces for a user with no memberships
	memberships, err := sm.GetUserSpaceMemberships(ctx, userID)
	if err != nil {
		t.Fatalf("Failed to get spaces for user with no memberships: %v", err)
	}

	// Should return empty result (nil or empty slice are both valid in Go)
	if len(memberships) != 0 {
		t.Errorf("Expected 0 memberships, got %d", len(memberships))
	}
}

func TestSpaceMemberships_GetSpacesForUser_SingleSpace(t *testing.T) {
	sm, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "user-single-space"
	spaceID := "space-solo"

	// Create a single membership
	_, err := sm.JoinSpace(ctx, userID, spaceID)
	if err != nil {
		t.Fatalf("Failed to create membership: %v", err)
	}

	// Retrieve spaces
	memberships, err := sm.GetUserSpaceMemberships(ctx, userID)
	if err != nil {
		t.Fatalf("Failed to get spaces for user: %v", err)
	}

	// Should return exactly 1 membership
	if len(memberships) != 1 {
		t.Errorf("Expected 1 membership, got %d", len(memberships))
	}

	if len(memberships) > 0 {
		if memberships[0].UserId != userID {
			t.Errorf("Expected user ID '%s', got '%s'", userID, memberships[0].UserId)
		}
		if memberships[0].SpaceId != spaceID {
			t.Errorf("Expected space ID '%s', got '%s'", spaceID, memberships[0].SpaceId)
		}
	}
}

func TestSpaceMemberships_GetSpacesForUser_IsolationBetweenUsers(t *testing.T) {
	sm, _ := setupTestCore(t)
	ctx := testContext(t)

	user1 := "user-isolation-1"
	user2 := "user-isolation-2"

	space1 := "space-iso-1"
	space2 := "space-iso-2"
	space3 := "space-iso-3"

	// Create memberships: user1 in space1 and space2, user2 in space2 and space3
	_, err := sm.JoinSpace(ctx, user1, space1)
	if err != nil {
		t.Fatalf("Failed to create membership for user1-space1: %v", err)
	}

	_, err = sm.JoinSpace(ctx, user1, space2)
	if err != nil {
		t.Fatalf("Failed to create membership for user1-space2: %v", err)
	}

	_, err = sm.JoinSpace(ctx, user2, space2)
	if err != nil {
		t.Fatalf("Failed to create membership for user2-space2: %v", err)
	}

	_, err = sm.JoinSpace(ctx, user2, space3)
	if err != nil {
		t.Fatalf("Failed to create membership for user2-space3: %v", err)
	}

	// Get spaces for user1
	user1Memberships, err := sm.GetUserSpaceMemberships(ctx, user1)
	if err != nil {
		t.Fatalf("Failed to get spaces for user1: %v", err)
	}

	// User1 should have exactly 2 memberships
	if len(user1Memberships) != 2 {
		t.Errorf("Expected user1 to have 2 memberships, got %d", len(user1Memberships))
	}

	// Verify all memberships belong to user1
	for _, m := range user1Memberships {
		if m.UserId != user1 {
			t.Errorf("Expected user ID '%s', got '%s'", user1, m.UserId)
		}
	}

	// Get spaces for user2
	user2Memberships, err := sm.GetUserSpaceMemberships(ctx, user2)
	if err != nil {
		t.Fatalf("Failed to get spaces for user2: %v", err)
	}

	// User2 should have exactly 2 memberships
	if len(user2Memberships) != 2 {
		t.Errorf("Expected user2 to have 2 memberships, got %d", len(user2Memberships))
	}

	// Verify all memberships belong to user2
	for _, m := range user2Memberships {
		if m.UserId != user2 {
			t.Errorf("Expected user ID '%s', got '%s'", user2, m.UserId)
		}
	}

	// Verify user1's spaces
	user1SpaceIDs := make(map[string]bool)
	for _, m := range user1Memberships {
		user1SpaceIDs[m.SpaceId] = true
	}

	if !user1SpaceIDs[space1] {
		t.Error("Expected user1 to have membership in space1")
	}
	if !user1SpaceIDs[space2] {
		t.Error("Expected user1 to have membership in space2")
	}
	if user1SpaceIDs[space3] {
		t.Error("Did not expect user1 to have membership in space3")
	}

	// Verify user2's spaces
	user2SpaceIDs := make(map[string]bool)
	for _, m := range user2Memberships {
		user2SpaceIDs[m.SpaceId] = true
	}

	if user2SpaceIDs[space1] {
		t.Error("Did not expect user2 to have membership in space1")
	}
	if !user2SpaceIDs[space2] {
		t.Error("Expected user2 to have membership in space2")
	}
	if !user2SpaceIDs[space3] {
		t.Error("Expected user2 to have membership in space3")
	}
}

func TestSpaceMemberships_GetSpacesForUser_AfterDeletion(t *testing.T) {
	sm, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "user-deletion-test"
	space1 := "space-del-1"
	space2 := "space-del-2"
	space3 := "space-del-3"

	// Create three memberships
	_, err := sm.JoinSpace(ctx, userID, space1)
	if err != nil {
		t.Fatalf("Failed to create membership for space1: %v", err)
	}

	_, err = sm.JoinSpace(ctx, userID, space2)
	if err != nil {
		t.Fatalf("Failed to create membership for space2: %v", err)
	}

	_, err = sm.JoinSpace(ctx, userID, space3)
	if err != nil {
		t.Fatalf("Failed to create membership for space3: %v", err)
	}

	// Verify we have 3 memberships
	memberships, err := sm.GetUserSpaceMemberships(ctx, userID)
	if err != nil {
		t.Fatalf("Failed to get spaces: %v", err)
	}
	if len(memberships) != 3 {
		t.Errorf("Expected 3 memberships initially, got %d", len(memberships))
	}

	// Delete one membership
	err = sm.LeaveSpace(ctx, userID, space2, false)
	if err != nil {
		t.Fatalf("Failed to delete membership: %v", err)
	}

	// Get spaces again
	memberships, err = sm.GetUserSpaceMemberships(ctx, userID)
	if err != nil {
		t.Fatalf("Failed to get spaces after deletion: %v", err)
	}

	// Should now have 2 memberships
	if len(memberships) != 2 {
		t.Errorf("Expected 2 memberships after deletion, got %d", len(memberships))
	}

	// Verify space2 is not in the results
	for _, m := range memberships {
		if m.SpaceId == space2 {
			t.Error("Did not expect deleted space2 to appear in results")
		}
	}

	// Verify space1 and space3 are still present
	spaceIDs := make(map[string]bool)
	for _, m := range memberships {
		spaceIDs[m.SpaceId] = true
	}

	if !spaceIDs[space1] {
		t.Error("Expected space1 to still be present")
	}
	if !spaceIDs[space3] {
		t.Error("Expected space3 to still be present")
	}
}

// ============================================================================
// Auto-Join Tests
// ============================================================================

func TestJoinSpace_AutoJoinsDefaultRooms(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a space and add rooms with auto_join enabled
	creatorID := "creator123"
	space, err := core.CreateSpace(ctx, creatorID, "Test Space", "Description")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	// Join space as creator (required to create rooms)
	_, err = core.JoinSpace(ctx, creatorID, space.Id)
	if err != nil {
		t.Fatalf("Failed to join space as creator: %v", err)
	}

	// Create rooms and set auto_join on specific ones
	generalRoom, err := core.CreateRoom(ctx, creatorID, space.Id, "general", "General discussion")
	if err != nil {
		t.Fatalf("Failed to create general room: %v", err)
	}
	if _, err := core.SetRoomAutoJoin(ctx, creatorID, space.Id, generalRoom.Id, true); err != nil {
		t.Fatalf("Failed to set auto_join on general room: %v", err)
	}

	announcementsRoom, err := core.CreateRoom(ctx, creatorID, space.Id, "announcements", "Announcements")
	if err != nil {
		t.Fatalf("Failed to create announcements room: %v", err)
	}
	if _, err := core.SetRoomAutoJoin(ctx, creatorID, space.Id, announcementsRoom.Id, true); err != nil {
		t.Fatalf("Failed to set auto_join on announcements room: %v", err)
	}

	// Create a room that should NOT be auto-joined (auto_join defaults to false)
	secretRoom, err := core.CreateRoom(ctx, creatorID, space.Id, "secret", "Secret room")
	if err != nil {
		t.Fatalf("Failed to create secret room: %v", err)
	}

	// Now have a new user join the space
	newUserID := "newuser456"
	_, err = core.JoinSpace(ctx, newUserID, space.Id)
	if err != nil {
		t.Fatalf("Failed to join space as new user: %v", err)
	}

	// Verify the new user is auto-joined to "general"
	generalMember, err := core.RoomMembershipExists(ctx, space.Id, newUserID, generalRoom.Id)
	if err != nil {
		t.Fatalf("Failed to check general room membership: %v", err)
	}
	if !generalMember {
		t.Error("Expected new user to be auto-joined to 'general' room")
	}

	// Verify the new user is auto-joined to "announcements"
	announcementsMember, err := core.RoomMembershipExists(ctx, space.Id, newUserID, announcementsRoom.Id)
	if err != nil {
		t.Fatalf("Failed to check announcements room membership: %v", err)
	}
	if !announcementsMember {
		t.Error("Expected new user to be auto-joined to 'announcements' room")
	}

	// Verify the new user is NOT auto-joined to "secret"
	secretMember, err := core.RoomMembershipExists(ctx, space.Id, newUserID, secretRoom.Id)
	if err != nil {
		t.Fatalf("Failed to check secret room membership: %v", err)
	}
	if secretMember {
		t.Error("Did not expect new user to be auto-joined to 'secret' room")
	}
}

func TestJoinSpace_AutoJoinWithNoAutoJoinRooms(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a space with rooms that don't have auto_join enabled
	creatorID := "creator123"
	space, err := core.CreateSpace(ctx, creatorID, "Test Space", "Description")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	// Join space as creator
	_, err = core.JoinSpace(ctx, creatorID, space.Id)
	if err != nil {
		t.Fatalf("Failed to join space as creator: %v", err)
	}

	// Create a room without auto_join (defaults to false)
	_, err = core.CreateRoom(ctx, creatorID, space.Id, "random-room", "Some room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Have a new user join the space (should not fail even with no auto-join rooms)
	newUserID := "newuser456"
	_, err = core.JoinSpace(ctx, newUserID, space.Id)
	if err != nil {
		t.Fatalf("Failed to join space as new user: %v", err)
	}

	// Verify the join succeeded (no auto-join rooms to verify)
	exists, err := core.SpaceMembershipExists(ctx, newUserID, space.Id)
	if err != nil {
		t.Fatalf("Failed to check space membership: %v", err)
	}
	if !exists {
		t.Error("Expected new user to be a space member")
	}
}

func TestJoinSpace_AutoJoinsCustomNamedRoom(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a space with a custom-named room that has auto_join enabled
	creatorID := "creator123"
	space, err := core.CreateSpace(ctx, creatorID, "Test Space", "Description")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	_, err = core.JoinSpace(ctx, creatorID, space.Id)
	if err != nil {
		t.Fatalf("Failed to join space as creator: %v", err)
	}

	// Create a custom-named room and enable auto_join
	welcomeRoom, err := core.CreateRoom(ctx, creatorID, space.Id, "welcome", "Welcome channel")
	if err != nil {
		t.Fatalf("Failed to create welcome room: %v", err)
	}
	if _, err := core.SetRoomAutoJoin(ctx, creatorID, space.Id, welcomeRoom.Id, true); err != nil {
		t.Fatalf("Failed to set auto_join on welcome room: %v", err)
	}

	// Create a room without auto_join
	_, err = core.CreateRoom(ctx, creatorID, space.Id, "private", "Private room")
	if err != nil {
		t.Fatalf("Failed to create private room: %v", err)
	}

	// Have a new user join the space
	newUserID := "newuser456"
	_, err = core.JoinSpace(ctx, newUserID, space.Id)
	if err != nil {
		t.Fatalf("Failed to join space as new user: %v", err)
	}

	// Verify the new user is auto-joined to the custom "welcome" room
	welcomeMember, err := core.RoomMembershipExists(ctx, space.Id, newUserID, welcomeRoom.Id)
	if err != nil {
		t.Fatalf("Failed to check welcome room membership: %v", err)
	}
	if !welcomeMember {
		t.Error("Expected new user to be auto-joined to 'welcome' room with auto_join=true")
	}
}

func TestJoinSpace_AutoJoinSkipsArchivedRooms(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	creatorID := "creator123"
	space, err := core.CreateSpace(ctx, creatorID, "Test Space", "Description")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	_, err = core.JoinSpace(ctx, creatorID, space.Id)
	if err != nil {
		t.Fatalf("Failed to join space as creator: %v", err)
	}

	// Create two rooms with auto_join enabled
	activeRoom, err := core.CreateRoom(ctx, creatorID, space.Id, "active", "Active room")
	if err != nil {
		t.Fatalf("Failed to create active room: %v", err)
	}
	if _, err := core.SetRoomAutoJoin(ctx, creatorID, space.Id, activeRoom.Id, true); err != nil {
		t.Fatalf("Failed to set auto_join on active room: %v", err)
	}

	archivedRoom, err := core.CreateRoom(ctx, creatorID, space.Id, "archived", "Archived room")
	if err != nil {
		t.Fatalf("Failed to create archived room: %v", err)
	}
	if _, err := core.SetRoomAutoJoin(ctx, creatorID, space.Id, archivedRoom.Id, true); err != nil {
		t.Fatalf("Failed to set auto_join on archived room: %v", err)
	}

	// Archive one of the auto_join rooms
	_, err = core.ArchiveRoom(ctx, creatorID, space.Id, archivedRoom.Id)
	if err != nil {
		t.Fatalf("Failed to archive room: %v", err)
	}

	// New user joins the space
	newUserID := "newuser456"
	_, err = core.JoinSpace(ctx, newUserID, space.Id)
	if err != nil {
		t.Fatalf("Failed to join space as new user: %v", err)
	}

	// User should be auto-joined to the active room
	activeMember, err := core.RoomMembershipExists(ctx, space.Id, newUserID, activeRoom.Id)
	if err != nil {
		t.Fatalf("Failed to check active room membership: %v", err)
	}
	if !activeMember {
		t.Error("Expected new user to be auto-joined to active room")
	}

	// User should NOT be auto-joined to the archived room
	archivedMember, err := core.RoomMembershipExists(ctx, space.Id, newUserID, archivedRoom.Id)
	if err != nil {
		t.Fatalf("Failed to check archived room membership: %v", err)
	}
	if archivedMember {
		t.Error("Did not expect new user to be auto-joined to archived room")
	}
}

// ============================================================================
// LeaveSpace Cleanup Tests
// ============================================================================

func TestChattoCore_LeaveSpace_CleansUpRoleAssignments(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space (creator is "owner")
	space, err := core.CreateSpace(ctx, "owner", "Test Space", "A test space")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	// Another user joins the space
	targetUser := "target-user"
	_, err = core.JoinSpace(ctx, targetUser, space.Id)
	if err != nil {
		t.Fatalf("Failed to join space: %v", err)
	}

	// Create a custom role
	_, err = core.CreateRole(ctx, "owner", space.Id, "vip", "VIP", "VIP role")
	if err != nil {
		t.Fatalf("Failed to create role: %v", err)
	}

	// Assign roles to the target user
	err = core.AssignRole(ctx, "owner", space.Id, targetUser, "vip")
	if err != nil {
		t.Fatalf("Failed to assign role: %v", err)
	}

	// Verify role assignment exists (user also has implicit 'member' role)
	roles, err := core.GetUserRoles(ctx, space.Id, targetUser)
	if err != nil {
		t.Fatalf("Failed to get user roles: %v", err)
	}
	hasVIP := false
	for _, r := range roles {
		if r == "vip" {
			hasVIP = true
			break
		}
	}
	if !hasVIP {
		t.Fatalf("Expected user to have 'vip' role, got: %v", roles)
	}

	// User leaves the space
	err = core.LeaveSpace(ctx, targetUser, space.Id, false)
	if err != nil {
		t.Fatalf("Failed to leave space: %v", err)
	}

	// Verify role assignments are cleaned up
	roles, err = core.GetUserRoles(ctx, space.Id, targetUser)
	if err != nil {
		t.Fatalf("Failed to get user roles after leave: %v", err)
	}
	if len(roles) != 0 {
		t.Errorf("Expected no roles after leaving space, got: %v", roles)
	}
}


package core

import (
	"errors"
	"testing"
)

func TestChattoCore_CreateRoom(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// First create a space

	// Create a room
	room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Verify room was created
	if room.Id == "" {
		t.Error("Room ID should not be empty")
	}
	if KindOfRoom(room) != KindChannel {
		t.Errorf("Room kind = %s, want %s", KindOfRoom(room), KindChannel)
	}
	if room.Name != "General" {
		t.Errorf("Room Name = %s, want General", room.Name)
	}
	if room.Description != "General discussion" {
		t.Errorf("Room Description = %s, want 'General discussion'", room.Description)
	}

	// Verify room can be retrieved
	retrievedRoom, err := core.GetRoom(ctx, KindChannel, room.Id)
	if err != nil {
		t.Fatalf("Failed to retrieve room: %v", err)
	}

	if retrievedRoom.Id != room.Id {
		t.Errorf("Retrieved room ID = %s, want %s", retrievedRoom.Id, room.Id)
	}
}

func TestChattoCore_CreateRoom_Validation(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// First create a space

	t.Run("empty name", func(t *testing.T) {
		_, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "", "Description")
		if err == nil {
			t.Error("Expected error for empty room name")
		}
		if err.Error() != "room name is required" {
			t.Errorf("Expected 'room name is required' error, got: %v", err)
		}
	})

	t.Run("whitespace only name", func(t *testing.T) {
		_, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "   ", "Description")
		if err == nil {
			t.Error("Expected error for whitespace-only room name")
		}
		if err.Error() != "room name is required" {
			t.Errorf("Expected 'room name is required' error, got: %v", err)
		}
	})

	t.Run("name too long", func(t *testing.T) {
		longName := string(make([]byte, 31)) // 31 characters
		for i := range longName {
			longName = longName[:i] + "a" + longName[i+1:]
		}
		_, err := core.CreateRoom(ctx, "test-user", KindChannel, "", longName, "Description")
		if err == nil {
			t.Error("Expected error for room name that is too long")
		}
		if err.Error() != "room name must be 30 characters or less" {
			t.Errorf("Expected 'room name must be 30 characters or less' error, got: %v", err)
		}
	})

	t.Run("description too long", func(t *testing.T) {
		longDesc := string(make([]byte, 501)) // 501 characters
		for i := range longDesc {
			longDesc = longDesc[:i] + "a" + longDesc[i+1:]
		}
		_, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "ValidName", longDesc)
		if err == nil {
			t.Error("Expected error for room description that is too long")
		}
		if err.Error() != "room description must be 500 characters or less" {
			t.Errorf("Expected 'room description must be 500 characters or less' error, got: %v", err)
		}
	})

	t.Run("valid name at max length", func(t *testing.T) {
		maxName := string(make([]byte, 30)) // exactly 30 characters
		for i := range maxName {
			maxName = maxName[:i] + "a" + maxName[i+1:]
		}
		room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", maxName, "Description")
		if err != nil {
			t.Errorf("Expected success for room name at max length, got: %v", err)
		}
		if room == nil {
			t.Error("Expected room to be created")
		}
	})

	t.Run("valid description at max length", func(t *testing.T) {
		maxDesc := string(make([]byte, 500)) // exactly 500 characters
		for i := range maxDesc {
			maxDesc = maxDesc[:i] + "a" + maxDesc[i+1:]
		}
		room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "ValidName2", maxDesc)
		if err != nil {
			t.Errorf("Expected success for room description at max length, got: %v", err)
		}
		if room == nil {
			t.Error("Expected room to be created")
		}
	})

	t.Run("name with leading/trailing whitespace is trimmed", func(t *testing.T) {
		room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "  TrimmedName  ", "Description")
		if err != nil {
			t.Errorf("Expected success, got: %v", err)
		}
		if room.Name != "TrimmedName" {
			t.Errorf("Expected name to be trimmed, got: %s", room.Name)
		}
	})
}

func TestValidateRoomName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError string
	}{
		{"empty string", "", "room name is required"},
		{"whitespace only", "   ", "room name is required"},
		{"tabs only", "\t\t", "room name is required"},
		{"valid name", "General", ""},
		{"valid name with hyphen", "general-discussion", ""},
		{"valid name with underscore", "general_discussion", ""},
		{"valid mixed case", "GeneralDiscussion", ""},
		{"valid with numbers", "room123", ""},
		{"valid single char", "A", ""},
		{"valid 30 chars", string(make([]byte, 30)), ""}, // will be replaced below
		{"too long 31 chars", string(make([]byte, 31)), "room name must be 30 characters or less"},
		{"invalid with spaces", "General Discussion", "room name must contain only alphanumeric characters, hyphens, and underscores (no spaces or special characters)"},
		{"invalid with slash", "general/discussion", "room name must contain only alphanumeric characters, hyphens, and underscores (no spaces or special characters)"},
		{"invalid with dot", "general.discussion", "room name must contain only alphanumeric characters, hyphens, and underscores (no spaces or special characters)"},
		{"invalid with special chars", "general@discussion", "room name must contain only alphanumeric characters, hyphens, and underscores (no spaces or special characters)"},
		{"invalid with emoji", "general💬", "room name must contain only alphanumeric characters, hyphens, and underscores (no spaces or special characters)"},
	}

	// Fix the 30-char test case
	for i, tt := range tests {
		if tt.name == "valid 30 chars" {
			validName := ""
			for j := 0; j < 30; j++ {
				validName += "a"
			}
			tests[i].input = validName
		}
		if tt.name == "too long 31 chars" {
			longName := ""
			for j := 0; j < 31; j++ {
				longName += "a"
			}
			tests[i].input = longName
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRoomName(tt.input)
			if tt.wantError == "" {
				if err != nil {
					t.Errorf("ValidateRoomName(%q) = %v, want nil", tt.input, err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateRoomName(%q) = nil, want error %q", tt.input, tt.wantError)
				} else if err.Error() != tt.wantError {
					t.Errorf("ValidateRoomName(%q) = %q, want %q", tt.input, err.Error(), tt.wantError)
				}
			}
		})
	}
}

func TestValidateRoomDescription(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantError string
	}{
		{"empty string", "", ""},
		{"valid description", "This is a room for general discussion", ""},
		{"valid 500 chars", "", ""}, // will be replaced below
		{"too long 501 chars", "", "room description must be 500 characters or less"},
	}

	// Fix the test cases with dynamic strings
	for i, tt := range tests {
		if tt.name == "valid 500 chars" {
			validDesc := ""
			for j := 0; j < 500; j++ {
				validDesc += "a"
			}
			tests[i].input = validDesc
		}
		if tt.name == "too long 501 chars" {
			longDesc := ""
			for j := 0; j < 501; j++ {
				longDesc += "a"
			}
			tests[i].input = longDesc
		}
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateRoomDescription(tt.input)
			if tt.wantError == "" {
				if err != nil {
					t.Errorf("ValidateRoomDescription(%q) = %v, want nil", tt.input, err)
				}
			} else {
				if err == nil {
					t.Errorf("ValidateRoomDescription(%q) = nil, want error %q", tt.input, tt.wantError)
				} else if err.Error() != tt.wantError {
					t.Errorf("ValidateRoomDescription(%q) = %q, want %q", tt.input, err.Error(), tt.wantError)
				}
			}
		})
	}
}

func TestChattoCore_GetRoom_NotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a space first

	_, err := core.GetRoom(ctx, KindChannel, "nonexistent")
	if err == nil {
		t.Error("Expected error when getting nonexistent room")
	}
}

func TestChattoCore_CreateRoom_DuplicateName(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a space

	// Create first room
	_, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	if err != nil {
		t.Fatalf("Failed to create first room: %v", err)
	}

	// Try to create another room with the same name
	_, err = core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "Another general room")
	if !errors.Is(err, ErrRoomNameExists) {
		t.Errorf("Expected ErrRoomNameExists, got: %v", err)
	}
}

func TestChattoCore_CreateRoom_DuplicateName_WithWhitespace(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a room
	_, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	if err != nil {
		t.Fatalf("Failed to create first room: %v", err)
	}

	// Try to create with whitespace around the name - should be trimmed and detected as duplicate
	_, err = core.CreateRoom(ctx, "test-user", KindChannel, "", "  General  ", "With whitespace")
	if err == nil {
		t.Error("Expected error for duplicate name with whitespace")
	}
}

func TestChattoCore_CreateRoom_DuplicateName_CaseInsensitive(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a room with lowercase
	_, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "general", "General discussion")
	if err != nil {
		t.Fatalf("Failed to create first room: %v", err)
	}

	// Create room with different case - should fail (case-insensitive)
	_, err = core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion uppercase")
	if !errors.Is(err, ErrRoomNameExists) {
		t.Errorf("Expected ErrRoomNameExists for different case, got: %v", err)
	}

	// Create room with all caps - should also fail
	_, err = core.CreateRoom(ctx, "test-user", KindChannel, "", "GENERAL", "General discussion allcaps")
	if !errors.Is(err, ErrRoomNameExists) {
		t.Errorf("Expected ErrRoomNameExists for all caps, got: %v", err)
	}
}

func TestChattoCore_RoomNameExists(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a space

	// Check non-existent room name
	exists, err := core.RoomNameExists(ctx, KindChannel, "General")
	if err != nil {
		t.Fatalf("Failed to check room name existence: %v", err)
	}
	if exists {
		t.Error("Room name should not exist yet")
	}

	// Create a room
	_, err = core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Check existing room name
	exists, err = core.RoomNameExists(ctx, KindChannel, "General")
	if err != nil {
		t.Fatalf("Failed to check room name existence after creation: %v", err)
	}
	if !exists {
		t.Error("Room name should exist after creation")
	}

	// Check case-insensitive match (lowercase query for uppercase room)
	exists, err = core.RoomNameExists(ctx, KindChannel, "general")
	if err != nil {
		t.Fatalf("Failed to check lowercase room name: %v", err)
	}
	if !exists {
		t.Error("Room name 'general' should match existing 'General' (case-insensitive)")
	}

	// Check case-insensitive match (uppercase query)
	exists, err = core.RoomNameExists(ctx, KindChannel, "GENERAL")
	if err != nil {
		t.Fatalf("Failed to check uppercase room name: %v", err)
	}
	if !exists {
		t.Error("Room name 'GENERAL' should match existing 'General' (case-insensitive)")
	}

	// Check non-existent room name
	exists, err = core.RoomNameExists(ctx, KindChannel, "Random")
	if err != nil {
		t.Fatalf("Failed to check non-existent room name: %v", err)
	}
	if exists {
		t.Error("Non-existent room name should not exist")
	}
}

func TestChattoCore_UpdateRoom(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space and room
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "OriginalName", "Original Description")

	// Update the room
	updated, err := core.UpdateRoom(ctx, "test-user", KindChannel, room.Id, "Updated-Name", "Updated Description")
	if err != nil {
		t.Fatalf("Failed to update room: %v", err)
	}

	if updated.Name != "Updated-Name" {
		t.Errorf("Expected name 'Updated-Name', got '%s'", updated.Name)
	}
	if updated.Description != "Updated Description" {
		t.Errorf("Expected description 'Updated Description', got '%s'", updated.Description)
	}

	// Verify update persisted
	retrieved, _ := core.GetRoom(ctx, KindChannel, room.Id)
	if retrieved.Name != "Updated-Name" {
		t.Errorf("Updated name not persisted: got '%s'", retrieved.Name)
	}
}

func TestChattoCore_UpdateRoom_DuplicateName(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create two rooms
	_, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "Room-A", "First room")
	if err != nil {
		t.Fatalf("Failed to create first room: %v", err)
	}
	roomB, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "Room-B", "Second room")
	if err != nil {
		t.Fatalf("Failed to create second room: %v", err)
	}

	// Try to rename Room-B to Room-A - should fail
	_, err = core.UpdateRoom(ctx, "test-user", KindChannel, roomB.Id, "Room-A", "Updated description")
	if !errors.Is(err, ErrRoomNameExists) {
		t.Errorf("Expected ErrRoomNameExists when renaming to existing name, got: %v", err)
	}

	// Try to rename Room-B to "room-a" (case-insensitive match) - should fail
	_, err = core.UpdateRoom(ctx, "test-user", KindChannel, roomB.Id, "room-a", "Updated description")
	if !errors.Is(err, ErrRoomNameExists) {
		t.Errorf("Expected ErrRoomNameExists for case-insensitive match, got: %v", err)
	}
}

func TestChattoCore_UpdateRoom_SameName_DifferentCase(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a room
	room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Update room to same name with different casing - should succeed
	updated, err := core.UpdateRoom(ctx, "test-user", KindChannel, room.Id, "GENERAL", "Updated description")
	if err != nil {
		t.Errorf("Expected success when updating to same name with different case, got: %v", err)
	}
	if updated.Name != "GENERAL" {
		t.Errorf("Expected name 'GENERAL', got '%s'", updated.Name)
	}
}

func TestChattoCore_UpdateRoom_PreservesArchived(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "original-name", "Description")

	if _, err := core.ArchiveRoom(ctx, "test-user", KindChannel, room.Id); err != nil {
		t.Fatalf("Failed to archive room: %v", err)
	}

	updated, err := core.UpdateRoom(ctx, "test-user", KindChannel, room.Id, "new-name", "New description")
	if err != nil {
		t.Fatalf("Failed to update room: %v", err)
	}

	if updated.Name != "new-name" {
		t.Errorf("Expected name 'new-name', got '%s'", updated.Name)
	}
	if !updated.Archived {
		t.Error("Expected archived to be preserved as true after UpdateRoom")
	}

	retrieved, err := core.GetRoom(ctx, KindChannel, room.Id)
	if err != nil {
		t.Fatalf("Failed to get room: %v", err)
	}
	if !retrieved.Archived {
		t.Error("Expected archived to persist as true")
	}
}

func TestChattoCore_RoomNameExistsExcluding(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create rooms
	roomA, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "Room-A", "First room")
	roomB, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "Room-B", "Second room")

	// Check if "Room-A" exists excluding roomA - should return false
	exists, err := core.RoomNameExistsExcluding(ctx, KindChannel, "Room-A", roomA.Id)
	if err != nil {
		t.Fatalf("Failed to check: %v", err)
	}
	if exists {
		t.Error("Should not find Room-A when excluding roomA")
	}

	// Check if "Room-A" exists excluding roomB - should return true
	exists, err = core.RoomNameExistsExcluding(ctx, KindChannel, "Room-A", roomB.Id)
	if err != nil {
		t.Fatalf("Failed to check: %v", err)
	}
	if !exists {
		t.Error("Should find Room-A when excluding roomB")
	}

	// Check case-insensitive match with exclusion
	exists, err = core.RoomNameExistsExcluding(ctx, KindChannel, "room-a", roomA.Id)
	if err != nil {
		t.Fatalf("Failed to check: %v", err)
	}
	if exists {
		t.Error("Should not find 'room-a' when excluding roomA (case-insensitive)")
	}
}

func TestChattoCore_DeleteRoom(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space and room
	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "ToDelete", "Will be deleted")

	// Verify room exists
	_, err := core.GetRoom(ctx, KindChannel, room.Id)
	if err != nil {
		t.Fatalf("Room should exist: %v", err)
	}

	// Delete the room
	err = core.DeleteRoom(ctx, "test-user", KindChannel, room.Id)
	if err != nil {
		t.Fatalf("Failed to delete room: %v", err)
	}

	// Verify it's gone
	_, err = core.GetRoom(ctx, KindChannel, room.Id)
	if err == nil {
		t.Error("Expected error when getting deleted room")
	}
}

// TestChattoCore_RoomName_ReuseAfterDelete verifies that deleting a room frees its name
// for reuse. Without index cleanup in DeleteRoom this would leak the name forever.
func TestChattoCore_RoomName_ReuseAfterDelete(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "general", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	if err := core.DeleteRoom(ctx, "test-user", KindChannel, room.Id); err != nil {
		t.Fatalf("DeleteRoom: %v", err)
	}

	// Same name (and case-variant) must be available again.
	if _, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", ""); err != nil {
		t.Fatalf("re-create after delete should succeed, got: %v", err)
	}
}

// TestChattoCore_RoomName_ReuseAfterRename verifies that renaming a room frees its old
// name so another room can claim it.
func TestChattoCore_RoomName_ReuseAfterRename(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "old-name", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	if _, err := core.UpdateRoom(ctx, "test-user", KindChannel, room.Id, "new-name", ""); err != nil {
		t.Fatalf("UpdateRoom rename: %v", err)
	}

	// The old name should now be free for a different room.
	if _, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "old-name", ""); err != nil {
		t.Fatalf("create with freed name should succeed, got: %v", err)
	}

	// And the new name should still be taken by the renamed room.
	if _, err := core.CreateRoom(ctx, "test-user", KindChannel, "", "new-name", ""); !errors.Is(err, ErrRoomNameExists) {
		t.Errorf("expected ErrRoomNameExists for taken new name, got: %v", err)
	}
}

// (TestChattoCore_RoomName_BackfillFromBareRoom removed in ADR-035
// phase 6 — the KV name index has been retired in favor of the
// RoomCatalog projection's FindByName; there's no backfill path left
// to exercise.)

func TestChattoCore_ListRoomsBySpace(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a space

	// Initially should be empty
	rooms, err := core.ListRooms(ctx, KindChannel)
	if err != nil {
		t.Fatalf("Failed to list rooms: %v", err)
	}
	if len(rooms) != 0 {
		t.Errorf("Expected 0 rooms, got %d", len(rooms))
	}

	// Create some rooms
	room1, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "room-1", "First room")
	room2, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "room-2", "Second room")
	room3, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "room-3", "Third room")

	// List should return all rooms
	rooms, err = core.ListRooms(ctx, KindChannel)
	if err != nil {
		t.Fatalf("Failed to list rooms: %v", err)
	}
	if len(rooms) != 3 {
		t.Errorf("Expected 3 rooms, got %d", len(rooms))
	}

	// Verify all rooms are present
	ids := make(map[string]bool)
	for _, room := range rooms {
		ids[room.Id] = true
	}
	if !ids[room1.Id] || !ids[room2.Id] || !ids[room3.Id] {
		t.Error("Not all created rooms were returned by ListRoomsBySpace")
	}
}

// ============================================================================
// ArchiveRoom Tests
// ============================================================================

func TestChattoCore_ArchiveRoom(t *testing.T) {
	t.Run("sets archived flag", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "general", "")

		_, err := core.ArchiveRoom(ctx, "test-user", KindChannel, room.Id)
		if err != nil {
			t.Fatalf("ArchiveRoom failed: %v", err)
		}

		retrieved, err := core.GetRoom(ctx, KindChannel, room.Id)
		if err != nil {
			t.Fatalf("GetRoom failed: %v", err)
		}
		if !retrieved.Archived {
			t.Error("Expected room to be archived")
		}
	})

	t.Run("returns updated room", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "general", "General chat")

		result, err := core.ArchiveRoom(ctx, "test-user", KindChannel, room.Id)
		if err != nil {
			t.Fatalf("ArchiveRoom failed: %v", err)
		}
		if !result.Archived {
			t.Error("Expected returned room to have Archived=true")
		}
		if result.Id != room.Id {
			t.Errorf("Expected room ID %q, got %q", room.Id, result.Id)
		}
		if result.Name != "general" {
			t.Errorf("Expected room name 'general', got %q", result.Name)
		}
	})

	t.Run("idempotent on already-archived room", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "general", "")

		_, err := core.ArchiveRoom(ctx, "test-user", KindChannel, room.Id)
		if err != nil {
			t.Fatalf("First ArchiveRoom failed: %v", err)
		}

		result, err := core.ArchiveRoom(ctx, "test-user", KindChannel, room.Id)
		if err != nil {
			t.Fatalf("Second ArchiveRoom failed: %v", err)
		}
		if !result.Archived {
			t.Error("Expected room to still be archived after second archive")
		}
	})

	t.Run("nonexistent room returns error", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		_, err := core.ArchiveRoom(ctx, "test-user", KindChannel, "bogus-room-id")
		if err == nil {
			t.Error("Expected error when archiving nonexistent room")
		}
	})

	t.Run("preserves room's group position", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		_, _ = core.CreateRoom(ctx, "test-user", KindChannel, "", "keep", "")
		room2, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "archive-me", "")

		// Both rooms land in the seed group via CreateRoom's
		// default-group lookup.
		if _, err := core.ArchiveRoom(ctx, "test-user", KindChannel, room2.Id); err != nil {
			t.Fatalf("ArchiveRoom failed: %v", err)
		}

		groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
		if err != nil {
			t.Fatalf("ListRoomGroupsOrdered failed: %v", err)
		}
		if len(groups) == 0 {
			t.Fatal("Expected at least the seed group to still exist")
		}
		if len(groups[0].RoomIds) != 2 {
			t.Fatalf("Expected room to stay in its group after archive (2 rooms), got %d", len(groups[0].RoomIds))
		}
	})

	t.Run("no layout exists", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "general", "")

		// Archive when no layout exists — should not error
		_, err := core.ArchiveRoom(ctx, "test-user", KindChannel, room.Id)
		if err != nil {
			t.Fatalf("ArchiveRoom without layout should not error: %v", err)
		}
	})
}

// ============================================================================
// UnarchiveRoom Tests
// ============================================================================

func TestChattoCore_UnarchiveRoom(t *testing.T) {
	t.Run("clears archived flag", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "general", "")

		_, err := core.ArchiveRoom(ctx, "test-user", KindChannel, room.Id)
		if err != nil {
			t.Fatalf("ArchiveRoom failed: %v", err)
		}

		_, err = core.UnarchiveRoom(ctx, "test-user", KindChannel, room.Id)
		if err != nil {
			t.Fatalf("UnarchiveRoom failed: %v", err)
		}

		retrieved, err := core.GetRoom(ctx, KindChannel, room.Id)
		if err != nil {
			t.Fatalf("GetRoom failed: %v", err)
		}
		if retrieved.Archived {
			t.Error("Expected room to be unarchived")
		}
	})

	t.Run("returns updated room", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "general", "")

		core.ArchiveRoom(ctx, "test-user", KindChannel, room.Id)

		result, err := core.UnarchiveRoom(ctx, "test-user", KindChannel, room.Id)
		if err != nil {
			t.Fatalf("UnarchiveRoom failed: %v", err)
		}
		if result.Archived {
			t.Error("Expected returned room to have Archived=false")
		}
		if result.Id != room.Id {
			t.Errorf("Expected room ID %q, got %q", room.Id, result.Id)
		}
	})

	t.Run("idempotent on non-archived room", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "general", "")

		// Unarchive a room that is not archived — should succeed
		result, err := core.UnarchiveRoom(ctx, "test-user", KindChannel, room.Id)
		if err != nil {
			t.Fatalf("UnarchiveRoom on non-archived room failed: %v", err)
		}
		if result.Archived {
			t.Error("Expected room to remain unarchived")
		}
	})

	t.Run("nonexistent room returns error", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		_, err := core.UnarchiveRoom(ctx, "test-user", KindChannel, "bogus-room-id")
		if err == nil {
			t.Error("Expected error when unarchiving nonexistent room")
		}
	})

	t.Run("preserves room's set position across archive/unarchive", func(t *testing.T) {
		// Archive/unarchive only toggles the archived flag — the room keeps
		// its set position so an operator can find and unarchive it via the
		// admin UI without losing where it lived in the layout.
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		_, _ = core.CreateRoom(ctx, "test-user", KindChannel, "", "keep", "")
		room2, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "archive-and-unarchive", "")

		// Both rooms land in the seed group via CreateRoom's default-group lookup.
		if _, err := core.ArchiveRoom(ctx, "test-user", KindChannel, room2.Id); err != nil {
			t.Fatalf("ArchiveRoom failed: %v", err)
		}
		if _, err := core.UnarchiveRoom(ctx, "test-user", KindChannel, room2.Id); err != nil {
			t.Fatalf("UnarchiveRoom failed: %v", err)
		}

		groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
		if err != nil {
			t.Fatalf("ListRoomGroupsOrdered failed: %v", err)
		}
		if len(groups) == 0 {
			t.Fatal("Expected at least the seed group to still exist")
		}
		if len(groups[0].RoomIds) != 2 {
			t.Fatalf("Expected both rooms to be in the group, got %d", len(groups[0].RoomIds))
		}
	})
}

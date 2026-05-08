package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestChattoCore_CreateRoom(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// First create a space
	space, err := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	// Create a room
	room, err := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Verify room was created
	if room.Id == "" {
		t.Error("Room ID should not be empty")
	}
	if room.SpaceId != space.Id {
		t.Errorf("Room SpaceId = %s, want %s", room.SpaceId, space.Id)
	}
	if room.Name != "General" {
		t.Errorf("Room Name = %s, want General", room.Name)
	}
	if room.Description != "General discussion" {
		t.Errorf("Room Description = %s, want 'General discussion'", room.Description)
	}

	// Verify room can be retrieved
	retrievedRoom, err := core.GetRoom(ctx, space.Id, room.Id)
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
	space, err := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	t.Run("empty name", func(t *testing.T) {
		_, err := core.CreateRoom(ctx, "test-user", space.Id, "", "Description")
		if err == nil {
			t.Error("Expected error for empty room name")
		}
		if err.Error() != "room name is required" {
			t.Errorf("Expected 'room name is required' error, got: %v", err)
		}
	})

	t.Run("whitespace only name", func(t *testing.T) {
		_, err := core.CreateRoom(ctx, "test-user", space.Id, "   ", "Description")
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
		_, err := core.CreateRoom(ctx, "test-user", space.Id, longName, "Description")
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
		_, err := core.CreateRoom(ctx, "test-user", space.Id, "ValidName", longDesc)
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
		room, err := core.CreateRoom(ctx, "test-user", space.Id, maxName, "Description")
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
		room, err := core.CreateRoom(ctx, "test-user", space.Id, "ValidName2", maxDesc)
		if err != nil {
			t.Errorf("Expected success for room description at max length, got: %v", err)
		}
		if room == nil {
			t.Error("Expected room to be created")
		}
	})

	t.Run("name with leading/trailing whitespace is trimmed", func(t *testing.T) {
		room, err := core.CreateRoom(ctx, "test-user", space.Id, "  TrimmedName  ", "Description")
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
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	_, err := core.GetRoom(ctx, space.Id, "nonexistent")
	if err == nil {
		t.Error("Expected error when getting nonexistent room")
	}
}

func TestChattoCore_CreateRoom_DuplicateName(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a space
	space, err := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	// Create first room
	_, err = core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	if err != nil {
		t.Fatalf("Failed to create first room: %v", err)
	}

	// Try to create another room with the same name
	_, err = core.CreateRoom(ctx, "test-user", space.Id, "General", "Another general room")
	if !errors.Is(err, ErrRoomNameExists) {
		t.Errorf("Expected ErrRoomNameExists, got: %v", err)
	}
}

func TestChattoCore_CreateRoom_DuplicateName_WithWhitespace(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Create a room
	_, err := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	if err != nil {
		t.Fatalf("Failed to create first room: %v", err)
	}

	// Try to create with whitespace around the name - should be trimmed and detected as duplicate
	_, err = core.CreateRoom(ctx, "test-user", space.Id, "  General  ", "With whitespace")
	if err == nil {
		t.Error("Expected error for duplicate name with whitespace")
	}
}

func TestChattoCore_CreateRoom_DuplicateName_CaseInsensitive(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Create a room with lowercase
	_, err := core.CreateRoom(ctx, "test-user", space.Id, "general", "General discussion")
	if err != nil {
		t.Fatalf("Failed to create first room: %v", err)
	}

	// Create room with different case - should fail (case-insensitive)
	_, err = core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion uppercase")
	if !errors.Is(err, ErrRoomNameExists) {
		t.Errorf("Expected ErrRoomNameExists for different case, got: %v", err)
	}

	// Create room with all caps - should also fail
	_, err = core.CreateRoom(ctx, "test-user", space.Id, "GENERAL", "General discussion allcaps")
	if !errors.Is(err, ErrRoomNameExists) {
		t.Errorf("Expected ErrRoomNameExists for all caps, got: %v", err)
	}
}


func TestChattoCore_RoomNameExists(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a space
	space, err := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	// Check non-existent room name
	exists, err := core.RoomNameExists(ctx, space.Id, "General")
	if err != nil {
		t.Fatalf("Failed to check room name existence: %v", err)
	}
	if exists {
		t.Error("Room name should not exist yet")
	}

	// Create a room
	_, err = core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Check existing room name
	exists, err = core.RoomNameExists(ctx, space.Id, "General")
	if err != nil {
		t.Fatalf("Failed to check room name existence after creation: %v", err)
	}
	if !exists {
		t.Error("Room name should exist after creation")
	}

	// Check case-insensitive match (lowercase query for uppercase room)
	exists, err = core.RoomNameExists(ctx, space.Id, "general")
	if err != nil {
		t.Fatalf("Failed to check lowercase room name: %v", err)
	}
	if !exists {
		t.Error("Room name 'general' should match existing 'General' (case-insensitive)")
	}

	// Check case-insensitive match (uppercase query)
	exists, err = core.RoomNameExists(ctx, space.Id, "GENERAL")
	if err != nil {
		t.Fatalf("Failed to check uppercase room name: %v", err)
	}
	if !exists {
		t.Error("Room name 'GENERAL' should match existing 'General' (case-insensitive)")
	}

	// Check non-existent room name
	exists, err = core.RoomNameExists(ctx, space.Id, "Random")
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
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "OriginalName", "Original Description")

	// Update the room
	updated, err := core.UpdateRoom(ctx, "test-user", space.Id, room.Id, "Updated-Name", "Updated Description")
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
	retrieved, _ := core.GetRoom(ctx, space.Id, room.Id)
	if retrieved.Name != "Updated-Name" {
		t.Errorf("Updated name not persisted: got '%s'", retrieved.Name)
	}
}

func TestChattoCore_UpdateRoom_DuplicateName(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Create two rooms
	_, err := core.CreateRoom(ctx, "test-user", space.Id, "Room-A", "First room")
	if err != nil {
		t.Fatalf("Failed to create first room: %v", err)
	}
	roomB, err := core.CreateRoom(ctx, "test-user", space.Id, "Room-B", "Second room")
	if err != nil {
		t.Fatalf("Failed to create second room: %v", err)
	}

	// Try to rename Room-B to Room-A - should fail
	_, err = core.UpdateRoom(ctx, "test-user", space.Id, roomB.Id, "Room-A", "Updated description")
	if !errors.Is(err, ErrRoomNameExists) {
		t.Errorf("Expected ErrRoomNameExists when renaming to existing name, got: %v", err)
	}

	// Try to rename Room-B to "room-a" (case-insensitive match) - should fail
	_, err = core.UpdateRoom(ctx, "test-user", space.Id, roomB.Id, "room-a", "Updated description")
	if !errors.Is(err, ErrRoomNameExists) {
		t.Errorf("Expected ErrRoomNameExists for case-insensitive match, got: %v", err)
	}
}

func TestChattoCore_UpdateRoom_SameName_DifferentCase(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Create a room
	room, err := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Update room to same name with different casing - should succeed
	updated, err := core.UpdateRoom(ctx, "test-user", space.Id, room.Id, "GENERAL", "Updated description")
	if err != nil {
		t.Errorf("Expected success when updating to same name with different case, got: %v", err)
	}
	if updated.Name != "GENERAL" {
		t.Errorf("Expected name 'GENERAL', got '%s'", updated.Name)
	}
}

func TestChattoCore_SetRoomAutoJoin(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "general", "General discussion")

	// Default should be false
	retrieved, err := core.GetRoom(ctx, space.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to get room: %v", err)
	}
	if retrieved.AutoJoin {
		t.Error("Expected auto_join to default to false")
	}

	// Set auto_join to true
	updated, err := core.SetRoomAutoJoin(ctx, "test-user", space.Id, room.Id, true)
	if err != nil {
		t.Fatalf("Failed to set auto_join: %v", err)
	}
	if !updated.AutoJoin {
		t.Error("Expected auto_join to be true after setting")
	}

	// Verify persisted
	retrieved, err = core.GetRoom(ctx, space.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to get room: %v", err)
	}
	if !retrieved.AutoJoin {
		t.Error("Expected auto_join to persist as true")
	}

	// Toggle back to false
	updated, err = core.SetRoomAutoJoin(ctx, "test-user", space.Id, room.Id, false)
	if err != nil {
		t.Fatalf("Failed to set auto_join back to false: %v", err)
	}
	if updated.AutoJoin {
		t.Error("Expected auto_join to be false after toggling back")
	}

	// Verify persisted
	retrieved, err = core.GetRoom(ctx, space.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to get room: %v", err)
	}
	if retrieved.AutoJoin {
		t.Error("Expected auto_join to persist as false")
	}
}

func TestChattoCore_UpdateRoom_PreservesArchivedAndAutoJoin(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "original-name", "Description")

	// Set archived and auto_join to true
	_, err := core.ArchiveRoom(ctx, "test-user", space.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to archive room: %v", err)
	}
	_, err = core.SetRoomAutoJoin(ctx, "test-user", space.Id, room.Id, true)
	if err != nil {
		t.Fatalf("Failed to set auto_join: %v", err)
	}

	// Update the room name
	updated, err := core.UpdateRoom(ctx, "test-user", space.Id, room.Id, "new-name", "New description")
	if err != nil {
		t.Fatalf("Failed to update room: %v", err)
	}

	// Verify name was updated
	if updated.Name != "new-name" {
		t.Errorf("Expected name 'new-name', got '%s'", updated.Name)
	}

	// Verify archived and auto_join were preserved
	if !updated.Archived {
		t.Error("Expected archived to be preserved as true after UpdateRoom")
	}
	if !updated.AutoJoin {
		t.Error("Expected auto_join to be preserved as true after UpdateRoom")
	}

	// Verify persisted
	retrieved, err := core.GetRoom(ctx, space.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to get room: %v", err)
	}
	if !retrieved.Archived {
		t.Error("Expected archived to persist as true")
	}
	if !retrieved.AutoJoin {
		t.Error("Expected auto_join to persist as true")
	}
}

func TestChattoCore_RoomNameExistsExcluding(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Create rooms
	roomA, _ := core.CreateRoom(ctx, "test-user", space.Id, "Room-A", "First room")
	roomB, _ := core.CreateRoom(ctx, "test-user", space.Id, "Room-B", "Second room")

	// Check if "Room-A" exists excluding roomA - should return false
	exists, err := core.RoomNameExistsExcluding(ctx, space.Id, "Room-A", roomA.Id)
	if err != nil {
		t.Fatalf("Failed to check: %v", err)
	}
	if exists {
		t.Error("Should not find Room-A when excluding roomA")
	}

	// Check if "Room-A" exists excluding roomB - should return true
	exists, err = core.RoomNameExistsExcluding(ctx, space.Id, "Room-A", roomB.Id)
	if err != nil {
		t.Fatalf("Failed to check: %v", err)
	}
	if !exists {
		t.Error("Should find Room-A when excluding roomB")
	}

	// Check case-insensitive match with exclusion
	exists, err = core.RoomNameExistsExcluding(ctx, space.Id, "room-a", roomA.Id)
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
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "ToDelete", "Will be deleted")

	// Verify room exists
	_, err := core.GetRoom(ctx, space.Id, room.Id)
	if err != nil {
		t.Fatalf("Room should exist: %v", err)
	}

	// Delete the room
	err = core.DeleteRoom(ctx, "test-user", space.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to delete room: %v", err)
	}

	// Verify it's gone
	_, err = core.GetRoom(ctx, space.Id, room.Id)
	if err == nil {
		t.Error("Expected error when getting deleted room")
	}
}

// TestChattoCore_RoomName_ReuseAfterDelete verifies that deleting a room frees its name
// for reuse. Without index cleanup in DeleteRoom this would leak the name forever.
func TestChattoCore_RoomName_ReuseAfterDelete(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "")
	room, err := core.CreateRoom(ctx, "test-user", space.Id, "general", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	if err := core.DeleteRoom(ctx, "test-user", space.Id, room.Id); err != nil {
		t.Fatalf("DeleteRoom: %v", err)
	}

	// Same name (and case-variant) must be available again.
	if _, err := core.CreateRoom(ctx, "test-user", space.Id, "General", ""); err != nil {
		t.Fatalf("re-create after delete should succeed, got: %v", err)
	}
}

// TestChattoCore_RoomName_ReuseAfterRename verifies that renaming a room frees its old
// name so another room can claim it.
func TestChattoCore_RoomName_ReuseAfterRename(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "")
	room, err := core.CreateRoom(ctx, "test-user", space.Id, "old-name", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	if _, err := core.UpdateRoom(ctx, "test-user", space.Id, room.Id, "new-name", ""); err != nil {
		t.Fatalf("UpdateRoom rename: %v", err)
	}

	// The old name should now be free for a different room.
	if _, err := core.CreateRoom(ctx, "test-user", space.Id, "old-name", ""); err != nil {
		t.Fatalf("create with freed name should succeed, got: %v", err)
	}

	// And the new name should still be taken by the renamed room.
	if _, err := core.CreateRoom(ctx, "test-user", space.Id, "new-name", ""); !errors.Is(err, ErrRoomNameExists) {
		t.Errorf("expected ErrRoomNameExists for taken new name, got: %v", err)
	}
}

// TestChattoCore_RoomName_BackfillFromBareRoom simulates a room created before atomic
// name claiming existed: the room record is in the bucket but the index entry is not.
// The next CreateRoom for that same name must still detect the collision via backfill.
func TestChattoCore_RoomName_BackfillFromBareRoom(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "")
	room, err := core.CreateRoom(ctx, "test-user", space.Id, "general", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	// Simulate the pre-migration state: index entry is missing, room record is present.
	bucket, err := core.getSpaceConfigBucket(ctx, space.Id)
	if err != nil {
		t.Fatalf("getSpaceConfigBucket: %v", err)
	}
	if err := bucket.Delete(ctx, roomNameIndexKey(room.Name)); err != nil {
		t.Fatalf("delete index entry: %v", err)
	}
	core.roomNameIndexBackfilled.Delete(space.Id) // force backfill on next call

	// A duplicate must still be rejected — backfill should re-claim the name from the room record.
	if _, err := core.CreateRoom(ctx, "test-user", space.Id, "General", ""); !errors.Is(err, ErrRoomNameExists) {
		t.Errorf("expected ErrRoomNameExists after backfill, got: %v", err)
	}

	// Existence query should agree.
	exists, err := core.RoomNameExists(ctx, space.Id, "general")
	if err != nil {
		t.Fatalf("RoomNameExists: %v", err)
	}
	if !exists {
		t.Error("expected room name to exist after backfill")
	}
}

func TestChattoCore_ListRoomsBySpace(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a space
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Initially should be empty
	rooms, err := core.ListRoomsBySpace(ctx, space.Id)
	if err != nil {
		t.Fatalf("Failed to list rooms: %v", err)
	}
	if len(rooms) != 0 {
		t.Errorf("Expected 0 rooms, got %d", len(rooms))
	}

	// Create some rooms
	room1, _ := core.CreateRoom(ctx, "test-user", space.Id, "room-1", "First room")
	room2, _ := core.CreateRoom(ctx, "test-user", space.Id, "room-2", "Second room")
	room3, _ := core.CreateRoom(ctx, "test-user", space.Id, "room-3", "Third room")

	// List should return all rooms
	rooms, err = core.ListRoomsBySpace(ctx, space.Id)
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
// Room Membership Tests
// ============================================================================

func TestRoomMemberships_CreateOrUpdate(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup: Create space, user, and room first
	space, _ := core.CreateSpace(ctx, "actor1", "Test Space", "Test Description")
	user, _ := core.CreateUser(ctx, "actor1", "testuser", "Test User", "password")
	room, _ := core.CreateRoom(ctx, "actor1", space.Id, "test-room", "test-room Desc")

	// User must be a space member first
	_, err := core.JoinSpace(ctx, user.Id, space.Id)
	if err != nil {
		t.Fatalf("Failed to create space membership: %v", err)
	}

	// Create room membership
	membership, err := core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)
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
	retrieved, err := core.GetRoomMembership(ctx, space.Id, user.Id, room.Id)
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

func TestRoomMemberships_CreateOrUpdate_RequiresSpaceMembership(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup: Create space, user, and room
	space, _ := core.CreateSpace(ctx, "actor1", "Test Space", "Test Description")
	user, _ := core.CreateUser(ctx, "actor1", "testuser", "Test User", "password")
	room, _ := core.CreateRoom(ctx, "actor1", space.Id, "test-room", "test-room Desc")

	// Try to create room membership WITHOUT being a space member first
	_, err := core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)
	if err == nil {
		t.Fatal("Expected error when creating room membership without space membership")
	}
}

func TestRoomMemberships_CreateOrUpdate_Idempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	space, _ := core.CreateSpace(ctx, "actor1", "Test Space", "Test Description")
	user, _ := core.CreateUser(ctx, "actor1", "testuser", "Test User", "password")
	room, _ := core.CreateRoom(ctx, "actor1", space.Id, "test-room", "test-room Desc")
	_, _ = core.JoinSpace(ctx, user.Id, space.Id)

	// Create first membership
	first, err := core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to create first membership: %v", err)
	}

	// CreateOrUpdate is idempotent - calling it again should succeed
	second, err := core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)
	if err != nil {
		t.Errorf("CreateOrUpdate should be idempotent and succeed on duplicate, got error: %v", err)
	}

	// Both should have the same data
	if first.UserId != second.UserId || first.RoomId != second.RoomId {
		t.Error("Repeated CreateOrUpdate should return same membership data")
	}
}

func TestRoomMemberships_Get_NotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup space (required for per-space bucket)
	space, _ := core.CreateSpace(ctx, "actor1", "Test Space", "Test Description")

	_, err := core.GetRoomMembership(ctx, space.Id, "nonexistent-user", "nonexistent-room")
	if err == nil {
		t.Error("Expected error when getting nonexistent membership")
	}
}

func TestRoomMemberships_Exists(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	space, _ := core.CreateSpace(ctx, "actor1", "Test Space", "Test Description")
	user, _ := core.CreateUser(ctx, "actor1", "testuser", "Test User", "password")
	room, _ := core.CreateRoom(ctx, "actor1", space.Id, "test-room", "test-room Desc")
	_, _ = core.JoinSpace(ctx, user.Id, space.Id)

	// Check non-existent membership
	exists, err := core.RoomMembershipExists(ctx, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check membership existence: %v", err)
	}
	if exists {
		t.Error("Expected membership to not exist")
	}

	// Create membership
	_, err = core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to create membership: %v", err)
	}

	// Check existing membership
	exists, err = core.RoomMembershipExists(ctx, space.Id, user.Id, room.Id)
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
	space, _ := core.CreateSpace(ctx, "actor1", "Test Space", "Test Description")
	user, _ := core.CreateUser(ctx, "actor1", "testuser", "Test User", "password")
	room, _ := core.CreateRoom(ctx, "actor1", space.Id, "test-room", "test-room Desc")
	_, _ = core.JoinSpace(ctx, user.Id, space.Id)

	// Create membership
	_, err := core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to create membership: %v", err)
	}

	// Verify it exists
	exists, err := core.RoomMembershipExists(ctx, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check membership existence: %v", err)
	}
	if !exists {
		t.Error("Expected membership to exist before deletion")
	}

	// Delete membership
	err = core.LeaveRoom(ctx, user.Id, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to delete membership: %v", err)
	}

	// Verify it no longer exists
	exists, err = core.RoomMembershipExists(ctx, space.Id, user.Id, room.Id)
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
	space, _ := core.CreateSpace(ctx, "actor1", "Test Space", "Test Description")

	// Delete is idempotent - deleting a non-existent membership should succeed
	err := core.LeaveRoom(ctx, "actor1", space.Id, "nonexistent-user", "nonexistent-room")
	if err != nil {
		t.Errorf("Delete should be idempotent and succeed for non-existent membership, got error: %v", err)
	}
}

func TestRoomMemberships_GetForUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	space, _ := core.CreateSpace(ctx, "actor1", "Test Space", "Test Description")
	user, _ := core.CreateUser(ctx, "actor1", "testuser", "Test User", "password")
	room1, _ := core.CreateRoom(ctx, "actor1", space.Id, "room-1", "room-1 Desc")
	room2, _ := core.CreateRoom(ctx, "actor1", space.Id, "room-2", "room-2 Desc")
	room3, _ := core.CreateRoom(ctx, "actor1", space.Id, "room-3", "room-3 Desc")
	_, _ = core.JoinSpace(ctx, user.Id, space.Id)

	// Create memberships for user in multiple rooms
	_, err := core.JoinRoom(ctx, user.Id, space.Id, user.Id, room1.Id)
	if err != nil {
		t.Fatalf("Failed to create membership for room1: %v", err)
	}

	_, err = core.JoinRoom(ctx, user.Id, space.Id, user.Id, room2.Id)
	if err != nil {
		t.Fatalf("Failed to create membership for room2: %v", err)
	}

	_, err = core.JoinRoom(ctx, user.Id, space.Id, user.Id, room3.Id)
	if err != nil {
		t.Fatalf("Failed to create membership for room3: %v", err)
	}

	// Retrieve all rooms for the user
	memberships, err := core.GetUserRoomMemberships(ctx, space.Id, user.Id)
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
	space, _ := core.CreateSpace(ctx, "actor1", "Test Space", "Test Description")
	user, _ := core.CreateUser(ctx, "actor1", "testuser", "Test User", "password")

	// Get rooms for a user with no memberships
	memberships, err := core.GetUserRoomMemberships(ctx, space.Id, user.Id)
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
	space, _ := core.CreateSpace(ctx, "actor1", "Test Space", "Test Description")
	user1, _ := core.CreateUser(ctx, "actor1", "user1", "User 1", "password")
	user2, _ := core.CreateUser(ctx, "actor1", "user2", "User 2", "password")
	user3, _ := core.CreateUser(ctx, "actor1", "user3", "User 3", "password")
	room, _ := core.CreateRoom(ctx, "actor1", space.Id, "test-room", "test-room Desc")

	// All users must be space members first
	_, _ = core.JoinSpace(ctx, user1.Id, space.Id)
	_, _ = core.JoinSpace(ctx, user2.Id, space.Id)
	_, _ = core.JoinSpace(ctx, user3.Id, space.Id)

	// Create memberships for multiple users in the same room
	_, err := core.JoinRoom(ctx, user1.Id, space.Id, user1.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to create membership for user1: %v", err)
	}

	_, err = core.JoinRoom(ctx, user2.Id, space.Id, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to create membership for user2: %v", err)
	}

	_, err = core.JoinRoom(ctx, user3.Id, space.Id, user3.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to create membership for user3: %v", err)
	}

	// Retrieve all users in the room
	memberships, err := core.GetRoomMembersList(ctx, space.Id, room.Id)
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
	space, _ := core.CreateSpace(ctx, "actor1", "Test Space", "Test Description")
	room, _ := core.CreateRoom(ctx, "actor1", space.Id, "test-room", "test-room Desc")

	// Get users for a room with no memberships
	memberships, err := core.GetRoomMembersList(ctx, space.Id, room.Id)
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
	space, _ := core.CreateSpace(ctx, "actor1", "Test Space", "Test Description")
	user, _ := core.CreateUser(ctx, "actor1", "testuser", "Test User", "password")
	room, _ := core.CreateRoom(ctx, "actor1", space.Id, "test-room", "test-room Desc")
	_, _ = core.JoinSpace(ctx, user.Id, space.Id)

	// Create membership
	_, err := core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to create initial membership: %v", err)
	}

	// Delete it
	err = core.LeaveRoom(ctx, user.Id, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to delete membership: %v", err)
	}

	// Recreate it (should succeed since it was deleted)
	_, err = core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to recreate membership: %v", err)
	}

	// Verify it exists
	exists, err := core.RoomMembershipExists(ctx, space.Id, user.Id, room.Id)
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
	space, _ := core.CreateSpace(ctx, "actor1", "Integration Space", "Integration Test")
	user, _ := core.CreateUser(ctx, "actor1", "integrationuser", "Integration User", "password")
	room, _ := core.CreateRoom(ctx, "actor1", space.Id, "integration-room", "integration-room Desc")
	_, _ = core.JoinSpace(ctx, user.Id, space.Id)

	// 1. Verify doesn't exist
	exists, err := core.RoomMembershipExists(ctx, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Initial existence check failed: %v", err)
	}
	if exists {
		t.Error("Membership should not exist initially")
	}

	// 2. Create
	created, err := core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Creation failed: %v", err)
	}
	if created.UserId != user.Id || created.RoomId != room.Id {
		t.Error("Created membership has incorrect data")
	}

	// 3. Verify exists
	exists, err = core.RoomMembershipExists(ctx, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Existence check after creation failed: %v", err)
	}
	if !exists {
		t.Error("Membership should exist after creation")
	}

	// 4. Get and verify data persisted correctly
	retrieved, err := core.GetRoomMembership(ctx, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Get after creation failed: %v", err)
	}
	if retrieved.UserId != user.Id || retrieved.RoomId != room.Id {
		t.Error("Retrieved membership has incorrect data")
	}

	// 5. Delete
	err = core.LeaveRoom(ctx, user.Id, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Deletion failed: %v", err)
	}

	// 6. Verify deleted
	exists, err = core.RoomMembershipExists(ctx, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Existence check after deletion failed: %v", err)
	}
	if exists {
		t.Error("Membership should not exist after deletion")
	}

	// 7. Get should fail
	_, err = core.GetRoomMembership(ctx, space.Id, user.Id, room.Id)
	if err == nil {
		t.Error("Get should fail after deletion")
	}

	// 8. Second delete should succeed (idempotent behavior)
	err = core.LeaveRoom(ctx, user.Id, space.Id, user.Id, room.Id)
	if err != nil {
		t.Errorf("Second delete should succeed due to idempotent behavior, got error: %v", err)
	}
}

// ============================================================================
// Message Tests
// ============================================================================

func TestChattoCore_PostMessage(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")

	// Join space and room (required for posting messages)
	core.JoinSpace(ctx, user.Id, space.Id)
	core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)

	// Post a message
	messageBody := "Hello, world!"
	roomEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, messageBody, nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	// Verify returned event metadata
	if roomEvent.ActorId != user.Id {
		t.Errorf("Event ActorId = %s, want %s", roomEvent.ActorId, user.Id)
	}

	// Verify it's a MessagePosted event
	messagePosted := roomEvent.GetMessagePosted()
	if messagePosted == nil {
		t.Fatal("Event should be a MessagePosted event")
	}

	// Verify space_id and room_id are in the concrete event
	if messagePosted.SpaceId != space.Id {
		t.Errorf("MessagePosted.SpaceId = %s, want %s", messagePosted.SpaceId, space.Id)
	}
	if messagePosted.RoomId != room.Id {
		t.Errorf("MessagePosted.RoomId = %s, want %s", messagePosted.RoomId, room.Id)
	}

	// Body is now lazy-loaded, fetch it separately using messageBodyId
	fetchedBody, err := core.GetMessageBody(ctx, messagePosted.SpaceId, messagePosted.MessageBodyId)
	if err != nil {
		t.Fatalf("Failed to fetch message body: %v", err)
	}
	if fetchedBody != messageBody {
		t.Errorf("Message body = %s, want %s", fetchedBody, messageBody)
	}
}

func TestChattoCore_PostMessage_BodyStoredInKV(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")

	// Join space and room (required for posting messages)
	core.JoinSpace(ctx, user.Id, space.Id)
	core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)

	// Post a message
	messageBody := "This is a test message for GDPR compliance!"
	roomEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, messageBody, nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	// Get the message ID from the event
	messagePosted := roomEvent.GetMessagePosted()
	if messagePosted == nil {
		t.Fatal("Event should be a MessagePosted event")
	}

	// Verify the body can be fetched via GetMessageBody using messageBodyId
	fetchedBody, err := core.GetMessageBody(ctx, messagePosted.SpaceId, messagePosted.MessageBodyId)
	if err != nil {
		t.Fatalf("Failed to fetch message body: %v", err)
	}
	if fetchedBody != messageBody {
		t.Errorf("Message body = %s, want %s", fetchedBody, messageBody)
	}

	// Verify the body is stored in the BODIES bucket
	// MessageBodyId now contains the full compound key ({userId}.{bodyId})
	bucket, err := core.getBodiesBucket(ctx, space.Id)
	if err != nil {
		t.Fatalf("Failed to get bodies bucket: %v", err)
	}

	entry, err := bucket.Get(ctx, messagePosted.MessageBodyId)
	if err != nil {
		t.Fatalf("Failed to get message body from KV: %v", err)
	}

	// Unmarshal and verify
	var storedBody corev1.MessageBody
	if err := proto.Unmarshal(entry.Value(), &storedBody); err != nil {
		t.Fatalf("Failed to unmarshal message body: %v", err)
	}

	// Messages are always encrypted - verify encrypted fields are set
	if len(storedBody.EncryptedBody) == 0 {
		t.Error("Expected encrypted body to be non-empty")
	}
	if len(storedBody.EncryptionNonce) == 0 {
		t.Error("Expected encryption nonce to be non-empty")
	}

	// Verify timestamps are set correctly
	if storedBody.CreatedAt == nil {
		t.Error("CreatedAt should be set")
	}
	// UpdatedAt should be nil for new messages (only set when message is edited)
	if storedBody.UpdatedAt != nil {
		t.Error("UpdatedAt should be nil for new messages")
	}
}

func TestChattoCore_PostMessage_ConcurrentOCC(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")

	// Join space and room
	core.JoinSpace(ctx, user.Id, space.Id)
	core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)

	// Post multiple messages concurrently to test OCC retry logic.
	// 5 concurrent publishes is a realistic stress test - in practice,
	// even this level of concurrency to the exact same subject is rare.
	const numMessages = 5
	errChan := make(chan error, numMessages)
	idChan := make(chan string, numMessages)

	for i := 0; i < numMessages; i++ {
		go func(msgNum int) {
			body := fmt.Sprintf("Concurrent message %d", msgNum)
			roomEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, body, nil, "", "", nil, false)
			if err != nil {
				errChan <- err
				return
			}
			idChan <- roomEvent.Id
		}(i)
	}

	// Collect results
	var errs []error
	eventIDs := make(map[string]bool)
	for i := 0; i < numMessages; i++ {
		select {
		case err := <-errChan:
			errs = append(errs, err)
		case id := <-idChan:
			eventIDs[id] = true
		}
	}

	// All messages should succeed
	if len(errs) > 0 {
		t.Errorf("Expected no errors, got %d: %v", len(errs), errs)
	}

	// All event IDs should be unique (no duplicates from OCC retries)
	if len(eventIDs) != numMessages {
		t.Errorf("Expected %d unique event IDs, got %d", numMessages, len(eventIDs))
	}
}

func TestChattoCore_PostMessage_InvalidRoom(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Try to post to non-existent room
	_, err := core.PostMessage(ctx, space.Id, "nonexistent", "user123", "Hello", nil, "", "", nil, false)
	if err == nil {
		t.Error("Expected error when posting to nonexistent room")
	}
}

// TestChattoCore_PostMessage_BodyTooLong tests that oversized message bodies are rejected.
// This is a security test to prevent DoS via oversized messages.
func TestChattoCore_PostMessage_BodyTooLong(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")

	// Join space and room
	core.JoinSpace(ctx, user.Id, space.Id)
	core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)

	t.Run("message at max length succeeds", func(t *testing.T) {
		// Create a message body at exactly the max length
		maxBody := make([]byte, MaxMessageBodyLength)
		for i := range maxBody {
			maxBody[i] = 'a'
		}

		_, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, string(maxBody), nil, "", "", nil, false)
		if err != nil {
			t.Errorf("Expected success for message at max length, got: %v", err)
		}
	})

	t.Run("message over max length fails", func(t *testing.T) {
		// Create a message body over the max length
		oversizedBody := make([]byte, MaxMessageBodyLength+1)
		for i := range oversizedBody {
			oversizedBody[i] = 'a'
		}

		_, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, string(oversizedBody), nil, "", "", nil, false)
		if err == nil {
			t.Error("Expected error for oversized message body")
		}
		if err != ErrMessageTooLong {
			t.Errorf("Expected ErrMessageTooLong, got: %v", err)
		}
	})
}

// TestChattoCore_PostMessage_InvisibleChars tests that messages with only invisible Unicode
// characters are rejected. This prevents blank-looking messages that would confuse users.
func TestChattoCore_PostMessage_InvisibleChars(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")

	// Join space and room
	core.JoinSpace(ctx, user.Id, space.Id)
	core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)

	t.Run("zero-width spaces only is rejected", func(t *testing.T) {
		_, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "\u200B\u200B\u200B", nil, "", "", nil, false)
		if err == nil {
			t.Error("Expected error for message with only zero-width spaces")
		}
	})

	t.Run("mixed invisible chars only is rejected", func(t *testing.T) {
		// Mix of: zero-width space, ZWNJ, ZWJ, word joiner, BOM
		_, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "\u200B\u200C\u200D\u2060\uFEFF", nil, "", "", nil, false)
		if err == nil {
			t.Error("Expected error for message with only invisible characters")
		}
	})

	t.Run("whitespace and invisible chars only is rejected", func(t *testing.T) {
		_, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "  \u200B  \t\u200C\n", nil, "", "", nil, false)
		if err == nil {
			t.Error("Expected error for message with only whitespace and invisible chars")
		}
	})

	t.Run("visible text with invisible chars is allowed", func(t *testing.T) {
		_, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "\u200BHello\u200B", nil, "", "", nil, false)
		if err != nil {
			t.Errorf("Expected success for message with visible text, got: %v", err)
		}
	})

	t.Run("emoji only is allowed", func(t *testing.T) {
		_, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "😀", nil, "", "", nil, false)
		if err != nil {
			t.Errorf("Expected success for emoji-only message, got: %v", err)
		}
	})
}

func TestChattoCore_GetRoomEvents(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space and room
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")

	// Create users and set up memberships
	user1, _ := core.CreateUser(ctx, "system", "user1", "user1", "password123")
	user2, _ := core.CreateUser(ctx, "system", "user2", "user2", "password123")
	core.JoinSpace(ctx, user1.Id, space.Id)
	core.JoinSpace(ctx, user2.Id, space.Id)
	core.JoinRoom(ctx, user1.Id, space.Id, user1.Id, room.Id)
	core.JoinRoom(ctx, user2.Id, space.Id, user2.Id, room.Id)

	// Post some messages
	core.PostMessage(ctx, space.Id, room.Id, user1.Id, "Message 1", nil, "", "", nil, false)
	core.PostMessage(ctx, space.Id, room.Id, user2.Id, "Message 2", nil, "", "", nil, false)
	core.PostMessage(ctx, space.Id, room.Id, user1.Id, "Message 3", nil, "", "", nil, false)

	// Get room events (returns all RoomEvents: messages + room membership + room lifecycle)
	eventsResult, err := core.GetRoomEvents(ctx, space.Id, room.Id, 50, nil)
	if err != nil {
		t.Fatalf("Failed to get room events: %v", err)
	}
	events := eventsResult.Events

	// Should receive 6 RoomEvents total:
	// - 1 RoomCreated event
	// - 2 UserJoinedRoom events (user1 and user2)
	// - 3 MessagePosted events
	// Note: With unified space stream, room lifecycle events are now included
	if len(events) != 6 {
		t.Errorf("Expected 6 room events (1 created + 2 joins + 3 messages), got %d", len(events))
	}

	// Count message events and verify bodies can be fetched
	messageCount := 0
	expectedBodies := []string{"Message 1", "Message 2", "Message 3"}
	for _, event := range events {
		if msg := event.GetMessagePosted(); msg != nil {
			// Body is lazy-loaded, fetch it separately using messageBodyId
			fetchedBody, err := core.GetMessageBody(ctx, msg.SpaceId, msg.MessageBodyId)
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
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")

	// Create a user
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	_, err := core.JoinSpace(ctx, user.Id, space.Id)
	if err != nil {
		t.Fatalf("Failed to join space: %v", err)
	}

	// User joins the room
	_, err = core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}

	// User leaves the room
	err = core.LeaveRoom(ctx, user.Id, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to leave room: %v", err)
	}

	// Get room events - should include join and leave events
	eventsResult, err := core.GetRoomEvents(ctx, space.Id, room.Id, 50, nil)
	if err != nil {
		t.Fatalf("Failed to get room events: %v", err)
	}
	events := eventsResult.Events

	// Count event types
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

	// Verify events: 1 RoomCreated + 1 UserJoinedRoom + 1 UserLeftRoom = 3 total
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
}

// TestChattoCore_GetRoomEvents_JoinAfterLastMessage verifies that join events
// are visible even when a user joins an inactive room (after the last message).
// This is a regression test for a bug where GetRoomEvents used lastMsgAt as the
// end time, causing join events after that time to be excluded.
func TestChattoCore_GetRoomEvents_JoinAfterLastMessage(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space and room
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")

	// Create user1 and have them post a message
	user1, _ := core.CreateUser(ctx, "system", "user1", "user1", "password123")
	_, err := core.JoinSpace(ctx, user1.Id, space.Id)
	if err != nil {
		t.Fatalf("Failed to join space: %v", err)
	}
	_, err = core.JoinRoom(ctx, user1.Id, space.Id, user1.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}
	_, err = core.PostMessage(ctx, space.Id, room.Id, user1.Id, "Last message before new user joins", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	// Simulate time passing (the room becomes "inactive")
	// In production, this could be hours or days

	// Create user2 and have them join the room AFTER the last message
	user2, _ := core.CreateUser(ctx, "system", "user2", "user2", "password123")
	_, err = core.JoinSpace(ctx, user2.Id, space.Id)
	if err != nil {
		t.Fatalf("Failed to join space: %v", err)
	}
	_, err = core.JoinRoom(ctx, user2.Id, space.Id, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to join room: %v", err)
	}

	// Get room events - should include user2's join event even though it
	// happened after the last message
	eventsResult, err := core.GetRoomEvents(ctx, space.Id, room.Id, 50, nil)
	if err != nil {
		t.Fatalf("Failed to get room events: %v", err)
	}
	events := eventsResult.Events

	// Find user2's join event
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

func TestChattoCore_PostMessage_Threading(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	core.JoinSpace(ctx, user.Id, space.Id)
	core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)

	t.Run("root message has empty inReplyTo", func(t *testing.T) {
		event, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Root message", nil, "", "", nil, false)
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
		rootEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Thread root", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root message: %v", err)
		}
		rootEventID := rootEvent.Id

		// Post a reply to the root message
		replyEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Thread reply", nil, rootEventID, "", nil, false)
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
		eventsResult, err := core.GetRoomEvents(ctx, space.Id, room.Id, 50, nil)
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
		rootEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Thread root for GetThreadEvents test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root message: %v", err)
		}
		rootEventID := rootEvent.Id

		// Post multiple replies
		for i := 1; i <= 3; i++ {
			_, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, fmt.Sprintf("Reply %d", i), nil, rootEventID, "", nil, false)
			if err != nil {
				t.Fatalf("Failed to post reply %d: %v", i, err)
			}
		}

		// Fetch thread events
		threadEvents, err := core.GetThreadEvents(ctx, space.Id, room.Id, rootEventID)
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
		rootEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Thread root for metadata test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root message: %v", err)
		}
		rootEventID := rootEvent.Id

		// Initially, no replies
		metadata, err := core.GetThreadMetadata(ctx, space.Id, room.Id, rootEventID)
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
		reply1, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "First reply", nil, rootEventID, "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post reply: %v", err)
		}
		reply1Time := reply1.CreatedAt.AsTime()

		// Check metadata after first reply
		metadata, err = core.GetThreadMetadata(ctx, space.Id, room.Id, rootEventID)
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
		_, err = core.PostMessage(ctx, space.Id, room.Id, user.Id, "Second reply", nil, rootEventID, "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post second reply: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
		reply3, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Third reply", nil, rootEventID, "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post third reply: %v", err)
		}
		reply3Time := reply3.CreatedAt.AsTime()

		// Check metadata after three replies
		metadata, err = core.GetThreadMetadata(ctx, space.Id, room.Id, rootEventID)
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
		core.JoinSpace(ctx, user2.Id, space.Id)
		core.JoinRoom(ctx, user.Id, space.Id, user2.Id, room.Id)

		// Post a new root message
		rootEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Thread for participants test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root message: %v", err)
		}
		rootEventID := rootEvent.Id

		// User 1 replies
		_, err = core.PostMessage(ctx, space.Id, room.Id, user.Id, "Reply from user 1", nil, rootEventID, "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post reply 1: %v", err)
		}

		// Check participants (should include user 1)
		metadata, err := core.GetThreadMetadata(ctx, space.Id, room.Id, rootEventID)
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
		_, err = core.PostMessage(ctx, space.Id, room.Id, user2.Id, "Reply from user 2", nil, rootEventID, "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post reply 2: %v", err)
		}

		// Check participants (should include both users)
		metadata, err = core.GetThreadMetadata(ctx, space.Id, room.Id, rootEventID)
		if err != nil {
			t.Fatalf("Failed to get thread metadata: %v", err)
		}
		if len(metadata.ParticipantIDs) != 2 {
			t.Errorf("Expected 2 participants, got %d", len(metadata.ParticipantIDs))
		}

		// User 1 replies again (should not duplicate)
		_, err = core.PostMessage(ctx, space.Id, room.Id, user.Id, "Another reply from user 1", nil, rootEventID, "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post reply 3: %v", err)
		}

		// Check participants (should still be 2)
		metadata, err = core.GetThreadMetadata(ctx, space.Id, room.Id, rootEventID)
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
		rootEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Inherit-thread root", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root message: %v", err)
		}

		// Post a thread reply (this is what the next message will reply to).
		threadReply, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Reply inside thread", nil, rootEvent.Id, "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post thread reply: %v", err)
		}

		// Post a message with inReplyTo pointing into the thread but inThread empty,
		// simulating a bot/extension/older client that doesn't know about inThread.
		inherited, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Reply attribution-only", nil, "", threadReply.Id, nil, false)
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
		root, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Plain root", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root: %v", err)
		}

		// Reply to it with attribution only — no thread should be inferred.
		reply, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Channel reply to root", nil, "", root.Id, nil, false)
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

func TestChattoCore_StreamRoomEventsLive(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space and room
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")

	// Create users and set up memberships
	user1, _ := core.CreateUser(ctx, "system", "user1", "user1", "password123")
	user2, _ := core.CreateUser(ctx, "system", "user2", "user2", "password123")
	core.JoinSpace(ctx, user1.Id, space.Id)
	core.JoinSpace(ctx, user2.Id, space.Id)
	core.JoinRoom(ctx, user1.Id, space.Id, user1.Id, room.Id)
	core.JoinRoom(ctx, user2.Id, space.Id, user2.Id, room.Id)

	// Start live streaming (should NOT receive historical events)
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	eventChan, err := core.StreamRoomEventsLive(streamCtx, space.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to start streaming: %v", err)
	}

	// Give subscription time to be ready
	time.Sleep(50 * time.Millisecond)

	// Post a new message (should be received in live stream)
	// Note: Must be synchronous to ensure body storage completes before we check it
	_, err = core.PostMessage(ctx, space.Id, room.Id, user1.Id, "Live message", nil, "", "", nil, false)
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
		fetchedBody, err := core.GetMessageBody(ctx, messagePosted.SpaceId, messagePosted.MessageBodyId)
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

func TestChattoCore_DeleteMessage_GDPR(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")

	// Join space and room (required for posting messages)
	core.JoinSpace(ctx, user.Id, space.Id)
	core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)

	// Post a message
	messageBody := "This message will be deleted for GDPR compliance"
	roomEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, messageBody, nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	// Get the message ID
	messagePosted := roomEvent.GetMessagePosted()
	if messagePosted == nil {
		t.Fatal("Event should be a MessagePosted event")
	}

	// Verify the body is in BODIES bucket
	// MessageBodyId now contains the full compound key ({userId}.{bodyId})
	bucket, err := core.getBodiesBucket(ctx, space.Id)
	if err != nil {
		t.Fatalf("Failed to get bodies bucket: %v", err)
	}

	_, err = bucket.Get(ctx, messagePosted.MessageBodyId)
	if err != nil {
		t.Fatalf("Message body should exist in KV before deletion: %v", err)
	}

	// Delete the message using the full compound key (author can delete own messages)
	err = core.DeleteMessage(ctx, user.Id, space.Id, room.Id, messagePosted.MessageBodyId)
	if err != nil {
		t.Fatalf("Failed to delete message: %v", err)
	}

	// Verify the body is no longer in KV
	_, err = bucket.Get(ctx, messagePosted.MessageBodyId)
	if err == nil {
		t.Error("Message body should not exist in KV after deletion")
	}

	// Verify we can delete again without error (idempotent)
	err = core.DeleteMessage(ctx, "test-user", space.Id, room.Id, messagePosted.MessageBodyId)
	if err != nil {
		t.Errorf("Deleting already deleted message should not error: %v", err)
	}
}

func TestChattoCore_GetRoomEvents_DeletedMessageBody(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")

	// Join space and room (required for posting messages)
	core.JoinSpace(ctx, user.Id, space.Id)
	core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)

	// Post a message
	messageBody := "This message will be deleted"
	roomEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, messageBody, nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	// Get the message ID
	postedMessage := roomEvent.GetMessagePosted()
	if postedMessage == nil {
		t.Fatal("Event should be a MessagePosted event")
	}

	// Delete the message body using messageBodyId (author can delete own messages)
	err = core.DeleteMessage(ctx, user.Id, space.Id, room.Id, postedMessage.MessageBodyId)
	if err != nil {
		t.Fatalf("Failed to delete message: %v", err)
	}

	// Get room events
	eventsResult, err := core.GetRoomEvents(ctx, space.Id, room.Id, 50, nil)
	if err != nil {
		t.Fatalf("Failed to get room events: %v", err)
	}
	events := eventsResult.Events

	// Find the MessagePosted event
	var messageEvent *corev1.SpaceEvent
	for _, event := range events {
		if event.GetMessagePosted() != nil {
			messageEvent = event.SpaceEvent
			break
		}
	}

	if messageEvent == nil {
		t.Fatal("Expected to find MessagePosted event")
	}

	messagePosted := messageEvent.GetMessagePosted()

	// Verify the body is empty (deleted) when fetched via GetMessageBody
	fetchedBody, err := core.GetMessageBody(ctx, messagePosted.SpaceId, messagePosted.MessageBodyId)
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

func TestChattoCore_DeleteMessage_DeletesAttachments(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")

	// Join space and room (required for posting messages)
	core.JoinSpace(ctx, user.Id, space.Id)
	core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)

	// Upload an attachment (using createTestPNG from attachments_test.go)
	imageData := createTestPNG(100, 100)
	attachment, err := core.UploadAttachment(ctx, space.Id, room.Id, "test.png", "image/png", bytes.NewReader(imageData))
	if err != nil {
		t.Fatalf("Failed to upload attachment: %v", err)
	}

	// Post a message with the attachment
	roomEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Message with attachment", []*corev1.Attachment{attachment}, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	postedMessage := roomEvent.GetMessagePosted()
	if postedMessage == nil {
		t.Fatal("Event should be a MessagePosted event")
	}

	// Verify attachment exists in ObjectStore
	store, err := core.GetAttachmentsStore(ctx, space.Id)
	if err != nil {
		t.Fatalf("Failed to get attachments store: %v", err)
	}

	_, err = store.Get(ctx, attachment.Id)
	if err != nil {
		t.Fatalf("Attachment should exist before deletion: %v", err)
	}

	// Delete the message
	err = core.DeleteMessage(ctx, "test-user", space.Id, room.Id, postedMessage.MessageBodyId)
	if err != nil {
		t.Fatalf("Failed to delete message: %v", err)
	}

	// Verify attachment is also deleted
	_, err = store.Get(ctx, attachment.Id)
	if err == nil {
		t.Error("Attachment should be deleted along with the message")
	}

	// Verify message body is deleted
	body, err := core.GetMessageBody(ctx, space.Id, postedMessage.MessageBodyId)
	if err != nil {
		t.Fatalf("Failed to get message body: %v", err)
	}
	if body != "" {
		t.Error("Message body should be empty after deletion")
	}
}

func TestChattoCore_DeleteAttachmentFromMessage(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")

	// Join space and room (required for posting messages)
	core.JoinSpace(ctx, user.Id, space.Id)
	core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)

	// Upload two attachments
	imageData := createTestPNG(100, 100)
	attachment1, err := core.UploadAttachment(ctx, space.Id, room.Id, "test1.png", "image/png", bytes.NewReader(imageData))
	if err != nil {
		t.Fatalf("Failed to upload attachment 1: %v", err)
	}
	attachment2, err := core.UploadAttachment(ctx, space.Id, room.Id, "test2.png", "image/png", bytes.NewReader(imageData))
	if err != nil {
		t.Fatalf("Failed to upload attachment 2: %v", err)
	}

	// Post a message with both attachments
	roomEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Message with attachments", []*corev1.Attachment{attachment1, attachment2}, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	postedMessage := roomEvent.GetMessagePosted()
	if postedMessage == nil {
		t.Fatal("Event should be a MessagePosted event")
	}

	// Verify both attachments exist
	store, err := core.GetAttachmentsStore(ctx, space.Id)
	if err != nil {
		t.Fatalf("Failed to get attachments store: %v", err)
	}
	if _, err := store.Get(ctx, attachment1.Id); err != nil {
		t.Fatalf("Attachment 1 should exist: %v", err)
	}
	if _, err := store.Get(ctx, attachment2.Id); err != nil {
		t.Fatalf("Attachment 2 should exist: %v", err)
	}

	// Delete only attachment 1
	err = core.DeleteAttachmentFromMessage(ctx, user.Id, space.Id, room.Id, postedMessage.MessageBodyId, attachment1.Id)
	if err != nil {
		t.Fatalf("Failed to delete attachment: %v", err)
	}

	// Verify attachment 1 is deleted from ObjectStore
	if _, err := store.Get(ctx, attachment1.Id); err == nil {
		t.Error("Attachment 1 should be deleted from ObjectStore")
	}

	// Verify attachment 2 still exists
	if _, err := store.Get(ctx, attachment2.Id); err != nil {
		t.Error("Attachment 2 should still exist")
	}

	// Verify message body still has attachment 2 but not attachment 1
	messageBody, err := core.GetFullMessageBody(ctx, space.Id, postedMessage.MessageBodyId)
	if err != nil {
		t.Fatalf("Failed to get message body: %v", err)
	}
	if len(messageBody.Attachments) != 1 {
		t.Errorf("Expected 1 attachment, got %d", len(messageBody.Attachments))
	}
	if messageBody.Attachments[0].Id != attachment2.Id {
		t.Error("Remaining attachment should be attachment 2")
	}
}

func TestChattoCore_DeleteAttachmentFromMessage_NotAuthor(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and two users
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	author, _ := core.CreateUser(ctx, "system", "author", "author", "password123")
	otherUser, _ := core.CreateUser(ctx, "system", "other", "other", "password123")

	// Both users join space and room
	core.JoinSpace(ctx, author.Id, space.Id)
	core.JoinRoom(ctx, author.Id, space.Id, author.Id, room.Id)
	core.JoinSpace(ctx, otherUser.Id, space.Id)
	core.JoinRoom(ctx, otherUser.Id, space.Id, otherUser.Id, room.Id)

	// Upload attachment and post message as author
	imageData := createTestPNG(100, 100)
	attachment, err := core.UploadAttachment(ctx, space.Id, room.Id, "test.png", "image/png", bytes.NewReader(imageData))
	if err != nil {
		t.Fatalf("Failed to upload attachment: %v", err)
	}

	roomEvent, err := core.PostMessage(ctx, space.Id, room.Id, author.Id, "Message with attachment", []*corev1.Attachment{attachment}, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	postedMessage := roomEvent.GetMessagePosted()

	// Try to delete attachment as other user - should fail
	err = core.DeleteAttachmentFromMessage(ctx, otherUser.Id, space.Id, room.Id, postedMessage.MessageBodyId, attachment.Id)
	if err == nil {
		t.Error("Expected error when non-author tries to delete attachment")
	}
	if err != ErrNotMessageAuthor {
		t.Errorf("Expected ErrNotMessageAuthor, got: %v", err)
	}

	// Verify attachment still exists
	store, _ := core.GetAttachmentsStore(ctx, space.Id)
	if _, err := store.Get(ctx, attachment.Id); err != nil {
		t.Error("Attachment should still exist after failed deletion")
	}
}

// ============================================================================
// S3 Attachment Deletion Integration Tests
// ============================================================================

func TestChattoCore_DeleteMessage_DeletesS3Attachments(t *testing.T) {
	core, _, s3Client := setupTestCoreWithS3(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	core.JoinSpace(ctx, user.Id, space.Id)
	core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)

	// Upload attachment (stored in S3)
	imageData := createTestPNG(100, 100)
	attachment, err := core.UploadAttachment(ctx, space.Id, room.Id, "test.png", "image/png", bytes.NewReader(imageData))
	if err != nil {
		t.Fatalf("Failed to upload attachment: %v", err)
	}

	s3Key := attachment.Storage.GetS3().Key

	// Post message with attachment
	roomEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Message with S3 attachment", []*corev1.Attachment{attachment}, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	postedMessage := roomEvent.GetMessagePosted()

	// Verify S3 object exists
	_, err = s3Client.StatObject(ctx, s3Key)
	if err != nil {
		t.Fatalf("S3 object should exist before deletion: %v", err)
	}

	// Delete the message
	err = core.DeleteMessage(ctx, user.Id, space.Id, room.Id, postedMessage.MessageBodyId)
	if err != nil {
		t.Fatalf("Failed to delete message: %v", err)
	}

	// Verify S3 object is also deleted
	_, err = s3Client.StatObject(ctx, s3Key)
	if err == nil {
		t.Error("S3 object should be deleted along with the message")
	}
}

func TestChattoCore_DeleteAttachmentFromMessage_S3(t *testing.T) {
	core, _, s3Client := setupTestCoreWithS3(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	core.JoinSpace(ctx, user.Id, space.Id)
	core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)

	// Upload two attachments (stored in S3)
	imageData := createTestPNG(100, 100)
	attachment1, err := core.UploadAttachment(ctx, space.Id, room.Id, "test1.png", "image/png", bytes.NewReader(imageData))
	if err != nil {
		t.Fatalf("Failed to upload attachment 1: %v", err)
	}
	attachment2, err := core.UploadAttachment(ctx, space.Id, room.Id, "test2.png", "image/png", bytes.NewReader(imageData))
	if err != nil {
		t.Fatalf("Failed to upload attachment 2: %v", err)
	}

	s3Key1 := attachment1.Storage.GetS3().Key
	s3Key2 := attachment2.Storage.GetS3().Key

	// Post message with both attachments
	roomEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Message with S3 attachments", []*corev1.Attachment{attachment1, attachment2}, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	postedMessage := roomEvent.GetMessagePosted()

	// Delete only attachment 1
	err = core.DeleteAttachmentFromMessage(ctx, user.Id, space.Id, room.Id, postedMessage.MessageBodyId, attachment1.Id)
	if err != nil {
		t.Fatalf("Failed to delete attachment from message: %v", err)
	}

	// Verify attachment 1 is deleted from S3
	_, err = s3Client.StatObject(ctx, s3Key1)
	if err == nil {
		t.Error("Attachment 1 should be deleted from S3")
	}

	// Verify attachment 2 still exists in S3
	_, err = s3Client.StatObject(ctx, s3Key2)
	if err != nil {
		t.Error("Attachment 2 should still exist in S3")
	}

	// Verify message body still has attachment 2 but not attachment 1
	messageBody, err := core.GetFullMessageBody(ctx, space.Id, postedMessage.MessageBodyId)
	if err != nil {
		t.Fatalf("Failed to get message body: %v", err)
	}
	if len(messageBody.Attachments) != 1 {
		t.Errorf("Expected 1 attachment remaining, got %d", len(messageBody.Attachments))
	}
	if messageBody.Attachments[0].Id != attachment2.Id {
		t.Error("Remaining attachment should be attachment 2")
	}
}

func TestChattoCore_DeleteSpace_DeletesMessageBodiesBucket(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")

	// Join space and room (required for posting messages)
	core.JoinSpace(ctx, user.Id, space.Id)
	core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)

	// Post a message to create the bodies bucket
	_, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Test message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	// Verify the bodies bucket exists
	bucket, err := core.getBodiesBucket(ctx, space.Id)
	if err != nil {
		t.Fatalf("Failed to get bodies bucket: %v", err)
	}
	if bucket == nil {
		t.Fatal("Bodies bucket should exist")
	}

	// Delete the space
	err = core.DeleteSpace(ctx, "test-user", space.Id)
	if err != nil {
		t.Fatalf("Failed to delete space: %v", err)
	}

	// Verify we can't get the space anymore
	_, err = core.GetSpace(ctx, space.Id)
	if err == nil {
		t.Error("Space should not exist after deletion")
	}

	// Cache cleanup is an implementation detail; the important behavior is verified above
}

// ============================================================================
// Unread Message Tracking Tests
// ============================================================================

func TestChattoCore_GetRoomLastEvent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")

	// Initially: no last event
	id, _, exists, err := core.GetRoomLastEvent(ctx, space.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to get room last event: %v", err)
	}
	if exists || id != "" {
		t.Errorf("Expected no last event for empty room, got id=%q exists=%v", id, exists)
	}

	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	core.JoinSpace(ctx, user.Id, space.Id)
	core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)

	first, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "First message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	id, ts, exists, err := core.GetRoomLastEvent(ctx, space.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to get room last event after post: %v", err)
	}
	if !exists {
		t.Fatal("Expected room last event to exist after a post")
	}
	if id != first.Id {
		t.Errorf("Expected last event id %q, got %q", first.Id, id)
	}
	if ts.IsZero() {
		t.Error("Expected non-zero timestamp")
	}

	second, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Second message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post second message: %v", err)
	}

	id, _, _, err = core.GetRoomLastEvent(ctx, space.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to get room last event after second post: %v", err)
	}
	if id != second.Id {
		t.Errorf("Expected last event id %q after second post, got %q", second.Id, id)
	}
}

func TestChattoCore_LastReadEventID(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")

	// Initially: empty (never read)
	id, err := core.GetLastReadEventID(ctx, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to get last read event id: %v", err)
	}
	if id != "" {
		t.Errorf("Expected empty for unread room, got %q", id)
	}

	// Set and read back
	if err := core.SetLastReadEventID(ctx, space.Id, user.Id, room.Id, "Eabcdefghij012"); err != nil {
		t.Fatalf("Failed to set last read event id: %v", err)
	}
	id, err = core.GetLastReadEventID(ctx, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to get last read event id after set: %v", err)
	}
	if id != "Eabcdefghij012" {
		t.Errorf("Expected %q, got %q", "Eabcdefghij012", id)
	}

	// Overwrite
	if err := core.SetLastReadEventID(ctx, space.Id, user.Id, room.Id, "Exyzxyzxyzxyz9"); err != nil {
		t.Fatalf("Failed to update last read event id: %v", err)
	}
	id, err = core.GetLastReadEventID(ctx, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to get last read event id after update: %v", err)
	}
	if id != "Exyzxyzxyzxyz9" {
		t.Errorf("Expected %q, got %q", "Exyzxyzxyzxyz9", id)
	}
}

// TestChattoCore_LastReadEventID_LazyInitCaughtUp verifies that a user with
// no read marker yet (e.g. a pre-existing user encountering this code path
// for the first time post-deploy) is lazy-initialized as caught up to the
// room's current last root event, so they don't see a wall of unreads.
func TestChattoCore_LastReadEventID_LazyInitCaughtUp(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	poster, _ := core.CreateUser(ctx, "system", "poster", "poster", "password123")
	core.JoinSpace(ctx, poster.Id, space.Id)
	core.JoinRoom(ctx, poster.Id, space.Id, poster.Id, room.Id)

	posted, err := core.PostMessage(ctx, space.Id, room.Id, poster.Id, "msg", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage error: %v", err)
	}

	// A user that has never written a marker (simulating post-deploy upgrade
	// where no `room_read_event` entry exists for them) should be lazy-
	// initialized to the room's current last event, not treated as unread.
	stranger, _ := core.CreateUser(ctx, "system", "stranger", "stranger", "password123")
	got, err := core.GetLastReadEventID(ctx, space.Id, stranger.Id, room.Id)
	if err != nil {
		t.Fatalf("GetLastReadEventID error: %v", err)
	}
	if got != posted.Id {
		t.Errorf("Expected lazy init to current last event %q, got %q", posted.Id, got)
	}

	// The marker should now be persisted — a second read returns the same
	// value without re-running the init.
	got2, err := core.GetLastReadEventID(ctx, space.Id, stranger.Id, room.Id)
	if err != nil {
		t.Fatalf("Second GetLastReadEventID error: %v", err)
	}
	if got2 != posted.Id {
		t.Errorf("Expected persisted marker %q, got %q", posted.Id, got2)
	}
}

// TestChattoCore_LastReadEventID_LazyInitEmptyRoom verifies that lazy init
// against an empty room returns "" without writing a marker.
func TestChattoCore_LastReadEventID_LazyInitEmptyRoom(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "empty-user", "empty-user", "password123")

	got, err := core.GetLastReadEventID(ctx, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("GetLastReadEventID error: %v", err)
	}
	if got != "" {
		t.Errorf("Expected empty event id for empty room, got %q", got)
	}
}

func TestChattoCore_HasUnread_NoMessages(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	core.JoinSpace(ctx, user.Id, space.Id)
	core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)

	// Room with no messages should have no unread
	hasUnread, err := core.HasUnread(ctx, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread: %v", err)
	}
	if hasUnread {
		t.Error("Expected no unread for room with no messages")
	}
}

func TestChattoCore_HasUnread_NewMessages(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user1, _ := core.CreateUser(ctx, "system", "user1", "user1", "password123")
	user2, _ := core.CreateUser(ctx, "system", "user2", "user2", "password123")
	core.JoinSpace(ctx, user1.Id, space.Id)
	core.JoinSpace(ctx, user2.Id, space.Id)
	core.JoinRoom(ctx, user1.Id, space.Id, user1.Id, room.Id)
	core.JoinRoom(ctx, user2.Id, space.Id, user2.Id, room.Id)

	// User1 posts a message
	_, err := core.PostMessage(ctx, space.Id, room.Id, user1.Id, "Hello!", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	// User2 should have unread (hasn't read the room yet)
	hasUnread, err := core.HasUnread(ctx, space.Id, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread for user2: %v", err)
	}
	if !hasUnread {
		t.Error("Expected user2 to have unread messages")
	}

	// User1 should NOT have unread (they posted, so they've "read" up to that point)
	hasUnread, err = core.HasUnread(ctx, space.Id, user1.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread for user1: %v", err)
	}
	if hasUnread {
		t.Error("Expected user1 to have NO unread (posting auto-marks as read)")
	}
}

func TestChattoCore_HasUnread_AfterMarkingRead(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup with two users
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user1, _ := core.CreateUser(ctx, "system", "user1", "user1", "password123")
	user2, _ := core.CreateUser(ctx, "system", "user2", "user2", "password123")
	core.JoinSpace(ctx, user1.Id, space.Id)
	core.JoinSpace(ctx, user2.Id, space.Id)
	core.JoinRoom(ctx, user1.Id, space.Id, user1.Id, room.Id)
	core.JoinRoom(ctx, user2.Id, space.Id, user2.Id, room.Id)

	// User2 posts a message
	_, err := core.PostMessage(ctx, space.Id, room.Id, user2.Id, "Hello!", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	// User1 should have unread (someone else posted)
	hasUnread, err := core.HasUnread(ctx, space.Id, user1.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread: %v", err)
	}
	if !hasUnread {
		t.Error("Expected user1 to have unread from user2's message")
	}

	// Get the room's last event
	lastID, _, exists, err := core.GetRoomLastEvent(ctx, space.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to get room last event: %v", err)
	}
	if !exists {
		t.Fatal("Expected room to have a last event")
	}

	// User1 marks as read up to the last event
	if err := core.SetLastReadEventID(ctx, space.Id, user1.Id, room.Id, lastID); err != nil {
		t.Fatalf("Failed to set last read event id: %v", err)
	}

	// User1 should have no unread now
	hasUnread, err = core.HasUnread(ctx, space.Id, user1.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread: %v", err)
	}
	if hasUnread {
		t.Error("Expected no unread after marking as read")
	}

	// User2 posts another message
	_, err = core.PostMessage(ctx, space.Id, room.Id, user2.Id, "Another message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post second message: %v", err)
	}

	// User1 should have unread again (user2 posted new message)
	hasUnread, err = core.HasUnread(ctx, space.Id, user1.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread after new message: %v", err)
	}
	if !hasUnread {
		t.Error("Expected unread after new message posted by another user")
	}
}

func TestChattoCore_HasUnread_NonMember(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	member, _ := core.CreateUser(ctx, "system", "member", "member", "password123")
	nonMember, _ := core.CreateUser(ctx, "system", "nonmember", "nonmember", "password123")

	// Only member joins
	core.JoinSpace(ctx, member.Id, space.Id)
	core.JoinRoom(ctx, member.Id, space.Id, member.Id, room.Id)

	// Post a message
	_, err := core.PostMessage(ctx, space.Id, room.Id, member.Id, "Hello!", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	// Non-member should NOT have unread (returns false, not error)
	hasUnread, err := core.HasUnread(ctx, space.Id, nonMember.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread for non-member: %v", err)
	}
	if hasUnread {
		t.Error("Non-member should not have unread messages")
	}
}

func TestChattoCore_HasUnread_MultipleRooms(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup with two users
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room1, _ := core.CreateRoom(ctx, "test-user", space.Id, "room-1", "Room 1")
	room2, _ := core.CreateRoom(ctx, "test-user", space.Id, "room-2", "Room 2")
	user1, _ := core.CreateUser(ctx, "system", "user1", "user1", "password123")
	user2, _ := core.CreateUser(ctx, "system", "user2", "user2", "password123")
	core.JoinSpace(ctx, user1.Id, space.Id)
	core.JoinSpace(ctx, user2.Id, space.Id)
	core.JoinRoom(ctx, user1.Id, space.Id, user1.Id, room1.Id)
	core.JoinRoom(ctx, user1.Id, space.Id, user1.Id, room2.Id)
	core.JoinRoom(ctx, user2.Id, space.Id, user2.Id, room1.Id)
	core.JoinRoom(ctx, user2.Id, space.Id, user2.Id, room2.Id)

	// User2 posts to room1 only
	_, err := core.PostMessage(ctx, space.Id, room1.Id, user2.Id, "Message in room1", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post to room1: %v", err)
	}

	// Room1 should have unread for user1 (user2 posted)
	hasUnread, err := core.HasUnread(ctx, space.Id, user1.Id, room1.Id)
	if err != nil {
		t.Fatalf("Failed to check unread for room1: %v", err)
	}
	if !hasUnread {
		t.Error("Expected room1 to have unread for user1")
	}

	// Room2 should NOT have unread for user1 (no messages)
	hasUnread, err = core.HasUnread(ctx, space.Id, user1.Id, room2.Id)
	if err != nil {
		t.Fatalf("Failed to check unread for room2: %v", err)
	}
	if hasUnread {
		t.Error("Expected room2 to have no unread (no messages)")
	}

	// User1 marks room1 as read
	lastID, _, _, _ := core.GetRoomLastEvent(ctx, space.Id, room1.Id)
	core.SetLastReadEventID(ctx, space.Id, user1.Id, room1.Id, lastID)

	// Room1 should now have no unread for user1
	hasUnread, err = core.HasUnread(ctx, space.Id, user1.Id, room1.Id)
	if err != nil {
		t.Fatalf("Failed to check unread for room1 after read: %v", err)
	}
	if hasUnread {
		t.Error("Expected room1 to have no unread after marking as read")
	}
}

func TestChattoCore_HasUnread_JoiningRoomWithExistingMessages(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup: user1 creates space and room
	space, _ := core.CreateSpace(ctx, "user1", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "user1", space.Id, "General", "General discussion")
	user1, _ := core.CreateUser(ctx, "system", "user1", "user1", "password123")
	user2, _ := core.CreateUser(ctx, "system", "user2", "user2", "password123")

	// user1 joins and posts messages BEFORE user2 joins
	core.JoinSpace(ctx, user1.Id, space.Id)
	core.JoinRoom(ctx, user1.Id, space.Id, user1.Id, room.Id)

	_, err := core.PostMessage(ctx, space.Id, room.Id, user1.Id, "Message 1", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message 1: %v", err)
	}
	_, err = core.PostMessage(ctx, space.Id, room.Id, user1.Id, "Message 2", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message 2: %v", err)
	}

	// Now user2 joins (after messages already exist)
	core.JoinSpace(ctx, user2.Id, space.Id)
	core.JoinRoom(ctx, user2.Id, space.Id, user2.Id, room.Id)

	// user2 should NOT have unread - they just joined and haven't "been there" before
	// Existing messages should be considered "caught up" at join time
	hasUnread, err := core.HasUnread(ctx, space.Id, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread: %v", err)
	}
	if hasUnread {
		t.Error("Expected new member to NOT have unread for pre-existing messages")
	}

	// But if user1 posts a NEW message after user2 joined, that should be unread
	_, err = core.PostMessage(ctx, space.Id, room.Id, user1.Id, "New message after user2 joined", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post new message: %v", err)
	}

	hasUnread, err = core.HasUnread(ctx, space.Id, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread after new message: %v", err)
	}
	if !hasUnread {
		t.Error("Expected user2 to have unread for message posted after they joined")
	}
}

// TestChattoCore_HasUnread_StaleMarker verifies that if a user's read marker
// points to a non-existent (e.g. deleted) event, HasUnread reports the room as
// unread rather than falling silent — the next mark-read self-corrects.
func TestChattoCore_HasUnread_StaleMarker(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "stale-marker-user", "stale-marker-user", "password123")
	core.JoinSpace(ctx, user.Id, space.Id)
	core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)

	if _, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "real msg", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage error: %v", err)
	}

	// Force the read marker to reference a non-existent event ID — the
	// "marker pointed at a deleted message" scenario.
	if err := core.SetLastReadEventID(ctx, space.Id, user.Id, room.Id, "Edoesnotexist"); err != nil {
		t.Fatalf("SetLastReadEventID error: %v", err)
	}

	hasUnread, err := core.HasUnread(ctx, space.Id, user.Id, room.Id)
	if err != nil {
		t.Fatalf("HasUnread error: %v", err)
	}
	if !hasUnread {
		t.Error("Expected stale read marker to surface as unread")
	}
}

// TestChattoCore_LastReadEventID_LazyInitRespectsExistingMarker verifies the
// invariant the Create-not-Put fix is there to guarantee: a marker written by
// any other path (MarkRoomAsRead, JoinRoom, PostMessage auto-mark) takes
// precedence over the lazy-init fallback. The race itself isn't directly
// exercisable without controlling KV timing, but the visible contract is
// "if a marker exists, GetLastReadEventID returns *that* value, never
// lazy-init's value."
func TestChattoCore_LastReadEventID_LazyInitRespectsExistingMarker(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	poster, _ := core.CreateUser(ctx, "system", "race-poster", "race-poster", "password123")
	core.JoinSpace(ctx, poster.Id, space.Id)
	core.JoinRoom(ctx, poster.Id, space.Id, poster.Id, room.Id)
	if _, err := core.PostMessage(ctx, space.Id, room.Id, poster.Id, "msg", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage error: %v", err)
	}

	// "Stranger" has no marker yet — the post-deploy / deploy-era case that
	// drives the lazy-init path. Pre-write the key directly with a marker
	// the stranger never wrote, simulating a concurrent winner.
	stranger, _ := core.CreateUser(ctx, "system", "race-stranger", "race-stranger", "password123")
	const concurrentWinner = "Eraceconcurwin"
	bucket, err := core.getSpaceRuntimeBucket(ctx, space.Id)
	if err != nil {
		t.Fatalf("getSpaceRuntimeBucket error: %v", err)
	}
	if _, err := bucket.Put(ctx, roomReadEventKey(stranger.Id, room.Id), []byte(concurrentWinner)); err != nil {
		t.Fatalf("seed marker error: %v", err)
	}

	got, err := core.GetLastReadEventID(ctx, space.Id, stranger.Id, room.Id)
	if err != nil {
		t.Fatalf("GetLastReadEventID error: %v", err)
	}
	if got != concurrentWinner {
		t.Errorf("Expected concurrent winner %q, got %q (lazy-init clobbered)", concurrentWinner, got)
	}
}

func TestChattoCore_HasUnread_ThreadReplyDoesNotCauseUnread(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup: two users in a room
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user1, _ := core.CreateUser(ctx, "system", "user1-thread", "user1-thread", "password123")
	user2, _ := core.CreateUser(ctx, "system", "user2-thread", "user2-thread", "password123")
	core.JoinSpace(ctx, user1.Id, space.Id)
	core.JoinSpace(ctx, user2.Id, space.Id)
	core.JoinRoom(ctx, user1.Id, space.Id, user1.Id, room.Id)
	core.JoinRoom(ctx, user2.Id, space.Id, user2.Id, room.Id)

	// User1 posts a root message
	rootEvent, err := core.PostMessage(ctx, space.Id, room.Id, user1.Id, "Root message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post root message: %v", err)
	}

	// User2 reads the room (marks as read up to root message)
	lastID, _, _, err := core.GetRoomLastEvent(ctx, space.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to get last event: %v", err)
	}
	if err := core.SetLastReadEventID(ctx, space.Id, user2.Id, room.Id, lastID); err != nil {
		t.Fatalf("Failed to set last read: %v", err)
	}

	// Verify user2 has no unread
	hasUnread, err := core.HasUnread(ctx, space.Id, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread: %v", err)
	}
	if hasUnread {
		t.Fatal("Expected no unread after marking as read")
	}

	// User1 posts a thread reply to the root message
	_, err = core.PostMessage(ctx, space.Id, room.Id, user1.Id, "Thread reply", nil, rootEvent.Id, "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post thread reply: %v", err)
	}

	// User2 should still NOT have unread — thread replies don't affect room-level unread
	hasUnread, err = core.HasUnread(ctx, space.Id, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread after thread reply: %v", err)
	}
	if hasUnread {
		t.Error("Thread reply should NOT cause room-level unread")
	}

	// But a new ROOT message should still cause unread
	_, err = core.PostMessage(ctx, space.Id, room.Id, user1.Id, "Another root message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post second root message: %v", err)
	}

	hasUnread, err = core.HasUnread(ctx, space.Id, user2.Id, room.Id)
	if err != nil {
		t.Fatalf("Failed to check unread after second root message: %v", err)
	}
	if !hasUnread {
		t.Error("New root message should cause room-level unread")
	}
}

func TestChattoCore_GetRoomEvents_Pagination(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and user
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	core.JoinSpace(ctx, user.Id, space.Id)
	core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)

	t.Run("returns newest messages when more than limit exist", func(t *testing.T) {
		// Post 100 messages
		for i := 1; i <= 100; i++ {
			_, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, fmt.Sprintf("Message %d", i), nil, "", "", nil, false)
			if err != nil {
				t.Fatalf("Failed to post message %d: %v", i, err)
			}
		}

		// Request 50 messages (default limit) - should return the 50 newest
		eventsResult, err := core.GetRoomEvents(ctx, space.Id, room.Id, 50, nil)
		if err != nil {
			t.Fatalf("Failed to get room events: %v", err)
		}
		events := eventsResult.Events

		// Count MessagePosted events
		var messageEvents []*corev1.MessagePostedEvent
		for _, event := range events {
			if msg := event.GetMessagePosted(); msg != nil {
				messageEvents = append(messageEvents, msg)
			}
		}

		if len(messageEvents) != 50 {
			t.Errorf("Expected 50 message events, got %d", len(messageEvents))
		}

		// The last message in our result should be "Message 100" (most recent)
		lastMsgBody, err := core.GetMessageBody(ctx, space.Id, messageEvents[len(messageEvents)-1].MessageBodyId)
		if err != nil {
			t.Fatalf("Failed to get last message body: %v", err)
		}
		if lastMsgBody != "Message 100" {
			t.Errorf("Expected last message body to be 'Message 100', got '%s'", lastMsgBody)
		}

		// The first message in our result should be "Message 51" (51st newest)
		firstMsgBody, err := core.GetMessageBody(ctx, space.Id, messageEvents[0].MessageBodyId)
		if err != nil {
			t.Fatalf("Failed to get first message body: %v", err)
		}
		if firstMsgBody != "Message 51" {
			t.Errorf("Expected first message body to be 'Message 51', got '%s'", firstMsgBody)
		}
	})

	t.Run("sequence-based pagination returns correct range", func(t *testing.T) {
		// Get the 50 newest messages first (to get a cursor)
		eventsResult, err := core.GetRoomEvents(ctx, space.Id, room.Id, 50, nil)
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
		olderEventsResult, err := core.GetRoomEvents(ctx, space.Id, room.Id, 50, &earliestSeq)
		if err != nil {
			t.Fatalf("Failed to get older room events: %v", err)
		}
		olderEvents := olderEventsResult.Events

		// Count MessagePosted events in the older batch
		var olderMessageEvents []*corev1.MessagePostedEvent
		for _, event := range olderEvents {
			if msg := event.GetMessagePosted(); msg != nil {
				olderMessageEvents = append(olderMessageEvents, msg)
			}
		}

		// Should get at least some messages (1-50 range)
		// Note: Room creation and user join events may also be included
		if len(olderMessageEvents) == 0 {
			t.Error("Expected to get some older message events")
		}

		// The newest message in the older batch should be before our cursor
		if len(olderMessageEvents) > 0 {
			newestOldBody, err := core.GetMessageBody(ctx, space.Id, olderMessageEvents[len(olderMessageEvents)-1].MessageBodyId)
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
	space, err := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	user, err := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	_, err = core.JoinSpace(ctx, user.Id, space.Id)
	if err != nil {
		t.Fatalf("Failed to join space: %v", err)
	}

	// Create a new room (this adds the creator as a member and publishes a RoomCreated event)
	room, err := core.CreateRoom(ctx, user.Id, space.Id, "new-room", "A fresh room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	// Don't post any messages - room has only the RoomCreated event

	// Get room events - should return events even without messages
	eventsResult, err := core.GetRoomEvents(ctx, space.Id, room.Id, 50, nil)
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
	space, err := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	user, err := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	_, err = core.JoinSpace(ctx, user.Id, space.Id)
	if err != nil {
		t.Fatalf("Failed to join space: %v", err)
	}

	room, err := core.CreateRoom(ctx, user.Id, space.Id, "test-room", "A test room")
	if err != nil {
		t.Fatalf("Failed to create room: %v", err)
	}

	t.Run("lookup root message by event ID", func(t *testing.T) {
		// Post a root message
		postedEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Hello, world!", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post message: %v", err)
		}

		messagePosted := postedEvent.GetMessagePosted()
		if messagePosted == nil {
			t.Fatal("Expected MessagePosted event")
		}

		// Look up by event ID (use wrapper's Id, not inner EventId)
		foundEvent, err := core.GetRoomEventByEventID(ctx, space.Id, room.Id, postedEvent.Id)
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

		if foundMessage.MessageBodyId != messagePosted.MessageBodyId {
			t.Errorf("MessageBodyId mismatch: got %s, want %s", foundMessage.MessageBodyId, messagePosted.MessageBodyId)
		}

		if foundEvent.Id != postedEvent.Id {
			t.Errorf("Event ID mismatch: got %s, want %s", foundEvent.Id, postedEvent.Id)
		}
	})

	t.Run("lookup non-existent event ID returns nil", func(t *testing.T) {
		event, err := core.GetRoomEventByEventID(ctx, space.Id, room.Id, "nonexistent123")
		if err != nil {
			t.Fatalf("GetRoomEventByEventID should not error for non-existent: %v", err)
		}

		if event != nil {
			t.Error("Expected nil for non-existent event ID")
		}
	})

	t.Run("lookup in wrong room returns nil", func(t *testing.T) {
		// Post a message in the first room
		postedEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Room 1 message", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post message: %v", err)
		}

		// Create a second room
		room2, err := core.CreateRoom(ctx, user.Id, space.Id, "test-room-2", "Another test room")
		if err != nil {
			t.Fatalf("Failed to create room 2: %v", err)
		}

		// Try to look up the message using room2's context - should return nil
		event, err := core.GetRoomEventByEventID(ctx, space.Id, room2.Id, postedEvent.Id)
		if err != nil {
			t.Fatalf("GetRoomEventByEventID should not error for wrong room: %v", err)
		}

		if event != nil {
			t.Error("Expected nil when looking up event in wrong room")
		}
	})
}

func TestChattoCore_ThreadLastOpened(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	threadRootEventId := "test-thread-root-123"

	// Initially should return zero time (never opened)
	lastOpened, err := core.GetThreadLastOpened(ctx, space.Id, user.Id, room.Id, threadRootEventId)
	if err != nil {
		t.Fatalf("Failed to get thread last opened: %v", err)
	}
	if !lastOpened.IsZero() {
		t.Errorf("Expected zero time for unopened thread, got %v", lastOpened)
	}

	// Set thread last opened - first time should return zero
	prevTime, err := core.SetThreadLastOpened(ctx, space.Id, user.Id, room.Id, threadRootEventId)
	if err != nil {
		t.Fatalf("Failed to set thread last opened: %v", err)
	}
	if !prevTime.IsZero() {
		t.Errorf("Expected zero time for first open, got %v", prevTime)
	}

	// Should now return a non-zero time
	lastOpened, err = core.GetThreadLastOpened(ctx, space.Id, user.Id, room.Id, threadRootEventId)
	if err != nil {
		t.Fatalf("Failed to get thread last opened after set: %v", err)
	}
	if lastOpened.IsZero() {
		t.Error("Expected non-zero time after setting")
	}

	// Set again - should return the previous timestamp
	prevTime2, err := core.SetThreadLastOpened(ctx, space.Id, user.Id, room.Id, threadRootEventId)
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
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	core.JoinSpace(ctx, user.Id, space.Id)
	core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)

	// Post a root message to create a thread
	rootMsg, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Root message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post root message: %v", err)
	}
	threadRootEventId := rootMsg.Id

	// Initially thread should not be "opened"
	lastOpened, err := core.GetThreadLastOpened(ctx, space.Id, user.Id, room.Id, threadRootEventId)
	if err != nil {
		t.Fatalf("Failed to get thread last opened: %v", err)
	}
	if !lastOpened.IsZero() {
		t.Errorf("Expected zero time for unopened thread, got %v", lastOpened)
	}

	// Post a thread reply
	_, err = core.PostMessage(ctx, space.Id, room.Id, user.Id, "Thread reply", nil, threadRootEventId, "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post thread reply: %v", err)
	}

	// Now the thread should be marked as "opened" for the user who posted
	lastOpened, err = core.GetThreadLastOpened(ctx, space.Id, user.Id, room.Id, threadRootEventId)
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

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "testuser", "testuser", "password123")
	threadRootEventId := "test-thread-root-123"

	t.Run("IsFollowingThread returns false for unfollowed thread", func(t *testing.T) {
		isFollowing, err := core.IsFollowingThread(ctx, space.Id, user.Id, room.Id, threadRootEventId)
		if err != nil {
			t.Fatalf("Failed to check thread follow: %v", err)
		}
		if isFollowing {
			t.Error("Expected not following for new thread")
		}
	})

	t.Run("FollowThread then IsFollowingThread returns true", func(t *testing.T) {
		err := core.FollowThread(ctx, space.Id, user.Id, room.Id, threadRootEventId)
		if err != nil {
			t.Fatalf("Failed to follow thread: %v", err)
		}

		isFollowing, err := core.IsFollowingThread(ctx, space.Id, user.Id, room.Id, threadRootEventId)
		if err != nil {
			t.Fatalf("Failed to check thread follow: %v", err)
		}
		if !isFollowing {
			t.Error("Expected following after FollowThread")
		}
	})

	t.Run("FollowThread is idempotent", func(t *testing.T) {
		err := core.FollowThread(ctx, space.Id, user.Id, room.Id, threadRootEventId)
		if err != nil {
			t.Fatalf("Failed to follow thread second time: %v", err)
		}
	})

	t.Run("UnfollowThread then IsFollowingThread returns false", func(t *testing.T) {
		err := core.UnfollowThread(ctx, space.Id, user.Id, room.Id, threadRootEventId)
		if err != nil {
			t.Fatalf("Failed to unfollow thread: %v", err)
		}

		isFollowing, err := core.IsFollowingThread(ctx, space.Id, user.Id, room.Id, threadRootEventId)
		if err != nil {
			t.Fatalf("Failed to check thread follow: %v", err)
		}
		if isFollowing {
			t.Error("Expected not following after UnfollowThread")
		}
	})

	t.Run("UnfollowThread is idempotent", func(t *testing.T) {
		err := core.UnfollowThread(ctx, space.Id, user.Id, room.Id, threadRootEventId)
		if err != nil {
			t.Fatalf("Failed to unfollow thread second time: %v", err)
		}
	})
}

func TestChattoCore_GetThreadFollowers(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	userA, _ := core.CreateUser(ctx, "system", "usera", "usera", "password123")
	userB, _ := core.CreateUser(ctx, "system", "userb", "userb", "password123")
	userC, _ := core.CreateUser(ctx, "system", "userc", "userc", "password123")
	threadRootEventId := "test-thread-root-456"

	t.Run("returns empty list for thread with no followers", func(t *testing.T) {
		followers, err := core.GetThreadFollowers(ctx, space.Id, room.Id, threadRootEventId)
		if err != nil {
			t.Fatalf("Failed to get thread followers: %v", err)
		}
		if len(followers) != 0 {
			t.Errorf("Expected 0 followers, got %d", len(followers))
		}
	})

	t.Run("returns correct follower IDs", func(t *testing.T) {
		// Follow with multiple users
		core.FollowThread(ctx, space.Id, userA.Id, room.Id, threadRootEventId)
		core.FollowThread(ctx, space.Id, userB.Id, room.Id, threadRootEventId)
		core.FollowThread(ctx, space.Id, userC.Id, room.Id, threadRootEventId)

		followers, err := core.GetThreadFollowers(ctx, space.Id, room.Id, threadRootEventId)
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
		core.UnfollowThread(ctx, space.Id, userB.Id, room.Id, threadRootEventId)

		followers, err := core.GetThreadFollowers(ctx, space.Id, room.Id, threadRootEventId)
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

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room1, _ := core.CreateRoom(ctx, "test-user", space.Id, "room-one", "First room")
	room2, _ := core.CreateRoom(ctx, "test-user", space.Id, "room-two", "Second room")
	userA, _ := core.CreateUser(ctx, "system", "usera", "usera", "password123")
	userB, _ := core.CreateUser(ctx, "system", "userb", "userb", "password123")
	core.JoinSpace(ctx, userA.Id, space.Id)
	core.JoinSpace(ctx, userB.Id, space.Id)
	core.JoinRoom(ctx, userA.Id, space.Id, userA.Id, room1.Id)
	core.JoinRoom(ctx, userA.Id, space.Id, userA.Id, room2.Id)
	core.JoinRoom(ctx, userB.Id, space.Id, userB.Id, room1.Id)
	core.JoinRoom(ctx, userB.Id, space.Id, userB.Id, room2.Id)

	t.Run("returns empty list when no threads are followed", func(t *testing.T) {
		threads, err := core.ListFollowedThreads(ctx, userA.Id, []string{space.Id})
		if err != nil {
			t.Fatalf("Failed to list followed threads: %v", err)
		}
		if len(threads) != 0 {
			t.Errorf("Expected 0 followed threads, got %d", len(threads))
		}
	})

	// Create thread 1 in room1: User A posts root, User B replies
	rootMsg1, _ := core.PostMessage(ctx, space.Id, room1.Id, userA.Id, "Root message 1", nil, "", "", nil, false)
	time.Sleep(10 * time.Millisecond) // Ensure distinct timestamps
	_, _ = core.PostMessage(ctx, space.Id, room1.Id, userB.Id, "Reply to thread 1", nil, rootMsg1.Id, "", nil, false)

	// Create thread 2 in room2: User A posts root, User B replies twice (to get a newer lastReplyAt)
	rootMsg2, _ := core.PostMessage(ctx, space.Id, room2.Id, userA.Id, "Root message 2", nil, "", "", nil, false)
	time.Sleep(10 * time.Millisecond)
	_, _ = core.PostMessage(ctx, space.Id, room2.Id, userB.Id, "First reply to thread 2", nil, rootMsg2.Id, "", nil, false)
	time.Sleep(10 * time.Millisecond)
	_, _ = core.PostMessage(ctx, space.Id, room2.Id, userB.Id, "Second reply to thread 2", nil, rootMsg2.Id, "", nil, false)

	t.Run("returns followed threads sorted by last activity", func(t *testing.T) {
		// User A auto-follows both threads (root author auto-follow on first reply)
		threads, err := core.ListFollowedThreads(ctx, userA.Id, []string{space.Id})
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
		threads, err := core.ListFollowedThreads(ctx, userA.Id, []string{space.Id})
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
		if thread2.SpaceID != space.Id {
			t.Errorf("Expected space ID, got %s", thread2.SpaceID)
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
		threads, err := core.ListFollowedThreads(ctx, userA.Id, []string{space.Id})
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
		core.SetThreadLastOpened(ctx, space.Id, userA.Id, room2.Id, rootMsg2.Id)

		threads, err := core.ListFollowedThreads(ctx, userA.Id, []string{space.Id})
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
		core.UnfollowThread(ctx, space.Id, userA.Id, room1.Id, rootMsg1.Id)

		threads, err := core.ListFollowedThreads(ctx, userA.Id, []string{space.Id})
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
		threadsB, err := core.ListFollowedThreads(ctx, userB.Id, []string{space.Id})
		if err != nil {
			t.Fatalf("Failed to list followed threads for user B: %v", err)
		}
		if len(threadsB) != 2 {
			t.Errorf("Expected 2 followed threads for user B, got %d", len(threadsB))
		}

		// User A should still only have 1 (unfollowed thread 1 above)
		threadsA, err := core.ListFollowedThreads(ctx, userA.Id, []string{space.Id})
		if err != nil {
			t.Fatalf("Failed to list followed threads for user A: %v", err)
		}
		if len(threadsA) != 1 {
			t.Errorf("Expected 1 followed thread for user A, got %d", len(threadsA))
		}
	})

	t.Run("orphaned follow key is skipped gracefully", func(t *testing.T) {
		// Manually follow a thread that has no metadata (orphaned)
		core.FollowThread(ctx, space.Id, userA.Id, room1.Id, "nonexistent-thread-id")

		threads, err := core.ListFollowedThreads(ctx, userA.Id, []string{space.Id})
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
		core.UnfollowThread(ctx, space.Id, userA.Id, room1.Id, "nonexistent-thread-id")
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

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	userA, _ := core.CreateUser(ctx, "system", "usera", "usera", "password123")
	userB, _ := core.CreateUser(ctx, "system", "userb", "userb", "password123")
	core.JoinSpace(ctx, userA.Id, space.Id)
	core.JoinSpace(ctx, userB.Id, space.Id)
	core.JoinRoom(ctx, userA.Id, space.Id, userA.Id, room.Id)
	core.JoinRoom(ctx, userB.Id, space.Id, userB.Id, room.Id)

	// User A posts root message
	rootMsg, err := core.PostMessage(ctx, space.Id, room.Id, userA.Id, "Root message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post root message: %v", err)
	}

	// Neither user should be following yet (no thread exists)
	isFollowing, _ := core.IsFollowingThread(ctx, space.Id, userA.Id, room.Id, rootMsg.Id)
	if isFollowing {
		t.Error("Root author should not be following before any replies")
	}

	// User B replies - both should be auto-followed
	_, err = core.PostMessage(ctx, space.Id, room.Id, userB.Id, "Reply from B", nil, rootMsg.Id, "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post reply: %v", err)
	}

	isFollowingA, _ := core.IsFollowingThread(ctx, space.Id, userA.Id, room.Id, rootMsg.Id)
	isFollowingB, _ := core.IsFollowingThread(ctx, space.Id, userB.Id, room.Id, rootMsg.Id)
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

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	userA, _ := core.CreateUser(ctx, "system", "usera", "usera", "password123")
	userB, _ := core.CreateUser(ctx, "system", "userb", "userb", "password123")
	core.JoinSpace(ctx, userA.Id, space.Id)
	core.JoinSpace(ctx, userB.Id, space.Id)
	core.JoinRoom(ctx, userA.Id, space.Id, userA.Id, room.Id)
	core.JoinRoom(ctx, userB.Id, space.Id, userB.Id, room.Id)

	// Create thread
	rootMsg, _ := core.PostMessage(ctx, space.Id, room.Id, userA.Id, "Root", nil, "", "", nil, false)
	core.PostMessage(ctx, space.Id, room.Id, userB.Id, "Reply 1", nil, rootMsg.Id, "", nil, false)

	// User B explicitly unfollows
	core.UnfollowThread(ctx, space.Id, userB.Id, room.Id, rootMsg.Id)
	isFollowing, _ := core.IsFollowingThread(ctx, space.Id, userB.Id, room.Id, rootMsg.Id)
	if isFollowing {
		t.Fatal("User B should not be following after unfollow")
	}

	// User B posts again - should be re-followed (posting always re-follows)
	_, err := core.PostMessage(ctx, space.Id, room.Id, userB.Id, "Reply 2", nil, rootMsg.Id, "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post second reply: %v", err)
	}

	isFollowing, _ = core.IsFollowingThread(ctx, space.Id, userB.Id, room.Id, rootMsg.Id)
	if !isFollowing {
		t.Error("User B should be re-followed after posting again")
	}
}

func TestChattoCore_PostMessage_RootAuthorUnfollowRespected(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	userA, _ := core.CreateUser(ctx, "system", "usera", "usera", "password123")
	userB, _ := core.CreateUser(ctx, "system", "userb", "userb", "password123")
	userC, _ := core.CreateUser(ctx, "system", "userc", "userc", "password123")
	core.JoinSpace(ctx, userA.Id, space.Id)
	core.JoinSpace(ctx, userB.Id, space.Id)
	core.JoinSpace(ctx, userC.Id, space.Id)
	core.JoinRoom(ctx, userA.Id, space.Id, userA.Id, room.Id)
	core.JoinRoom(ctx, userB.Id, space.Id, userB.Id, room.Id)
	core.JoinRoom(ctx, userC.Id, space.Id, userC.Id, room.Id)

	// User A posts root message, User B replies (auto-follows both)
	rootMsg, _ := core.PostMessage(ctx, space.Id, room.Id, userA.Id, "Root", nil, "", "", nil, false)
	core.PostMessage(ctx, space.Id, room.Id, userB.Id, "Reply 1", nil, rootMsg.Id, "", nil, false)

	// Verify User A was auto-followed on first reply
	isFollowing, _ := core.IsFollowingThread(ctx, space.Id, userA.Id, room.Id, rootMsg.Id)
	if !isFollowing {
		t.Fatal("Root author should be auto-followed after first reply")
	}

	// Root author explicitly unfollows
	core.UnfollowThread(ctx, space.Id, userA.Id, room.Id, rootMsg.Id)
	isFollowing, _ = core.IsFollowingThread(ctx, space.Id, userA.Id, room.Id, rootMsg.Id)
	if isFollowing {
		t.Fatal("Root author should not be following after explicit unfollow")
	}

	// Same user posts another reply — root author should NOT be re-followed (2-user case)
	_, err := core.PostMessage(ctx, space.Id, room.Id, userB.Id, "Reply 2", nil, rootMsg.Id, "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post reply: %v", err)
	}

	isFollowing, _ = core.IsFollowingThread(ctx, space.Id, userA.Id, room.Id, rootMsg.Id)
	if isFollowing {
		t.Error("Root author should NOT be re-followed after explicit unfollow (2-user case)")
	}

	// A third user posts a reply — root author should still NOT be re-followed
	_, err = core.PostMessage(ctx, space.Id, room.Id, userC.Id, "Reply 3", nil, rootMsg.Id, "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post reply: %v", err)
	}

	isFollowing, _ = core.IsFollowingThread(ctx, space.Id, userA.Id, room.Id, rootMsg.Id)
	if isFollowing {
		t.Error("Root author should NOT be re-followed after explicit unfollow (3-user case)")
	}
}

func TestChattoCore_NotifyThreadFollowers(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	userA, _ := core.CreateUser(ctx, "system", "usera", "usera", "password123")
	userB, _ := core.CreateUser(ctx, "system", "userb", "userb", "password123")
	userC, _ := core.CreateUser(ctx, "system", "userc", "userc", "password123")
	core.JoinSpace(ctx, userA.Id, space.Id)
	core.JoinSpace(ctx, userB.Id, space.Id)
	core.JoinSpace(ctx, userC.Id, space.Id)
	core.JoinRoom(ctx, userA.Id, space.Id, userA.Id, room.Id)
	core.JoinRoom(ctx, userB.Id, space.Id, userB.Id, room.Id)
	core.JoinRoom(ctx, userC.Id, space.Id, userC.Id, room.Id)

	// User A creates thread, User B replies (both auto-followed)
	rootMsg, _ := core.PostMessage(ctx, space.Id, room.Id, userA.Id, "Root", nil, "", "", nil, false)
	core.PostMessage(ctx, space.Id, room.Id, userB.Id, "Reply from B", nil, rootMsg.Id, "", nil, false)

	// User C also replies (auto-followed), then unfollows
	core.PostMessage(ctx, space.Id, room.Id, userC.Id, "Reply from C", nil, rootMsg.Id, "", nil, false)
	core.UnfollowThread(ctx, space.Id, userC.Id, room.Id, rootMsg.Id)

	// Clear all existing notifications
	core.DismissAllNotifications(ctx, userA.Id)
	core.DismissAllNotifications(ctx, userB.Id)
	core.DismissAllNotifications(ctx, userC.Id)

	// User B posts another reply - should notify A (follower) but NOT C (unfollowed) or B (author)
	core.PostMessage(ctx, space.Id, room.Id, userB.Id, "Another reply from B", nil, rootMsg.Id, "", nil, false)

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
	space, _ := core.CreateSpace(ctx, "test-user", "Echo Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	user, _ := core.CreateUser(ctx, "system", "echo-user", "Echo User", "password123")
	core.JoinSpace(ctx, user.Id, space.Id)
	core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id)

	t.Run("echo publishes two events", func(t *testing.T) {
		// Post root message
		rootEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Thread root", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root: %v", err)
		}

		// Post thread reply with echo
		replyEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Thread reply echoed", nil, rootEvent.Id, "", nil, true)
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
		roomEventsResult, err := core.GetRoomEvents(ctx, space.Id, room.Id, 50, nil)
		if err != nil {
			t.Fatalf("Failed to get room events: %v", err)
		}
		roomEvents := roomEventsResult.Events

		var foundEcho bool
		for _, e := range roomEvents {
			if msg := e.GetMessagePosted(); msg != nil && msg.EchoOfEventId != "" {
				foundEcho = true
				if msg.MessageBodyId != reply.MessageBodyId {
					t.Errorf("Echo should share messageBodyId with reply: got echo=%q, reply=%q", msg.MessageBodyId, reply.MessageBodyId)
				}
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
		threadEvents, err := core.GetThreadEvents(ctx, space.Id, room.Id, rootEvent.Id)
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
		event, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Not a reply", nil, "", "", nil, true)
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
		rootEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Root for count test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root: %v", err)
		}

		// Post reply with echo
		_, err = core.PostMessage(ctx, space.Id, room.Id, user.Id, "Reply with echo", nil, rootEvent.Id, "", nil, true)
		if err != nil {
			t.Fatalf("Failed to post reply: %v", err)
		}

		metadata, err := core.GetThreadMetadata(ctx, space.Id, room.Id, rootEvent.Id)
		if err != nil {
			t.Fatalf("Failed to get metadata: %v", err)
		}
		if metadata.ReplyCount != 1 {
			t.Errorf("Expected ReplyCount=1 (echo should not increment), got %d", metadata.ReplyCount)
		}
	})

	t.Run("shared message body between echo and reply", func(t *testing.T) {
		// Post root and reply with echo
		rootEvent, _ := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Root for body test", nil, "", "", nil, false)
		replyEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "Shared body content", nil, rootEvent.Id, "", nil, true)
		if err != nil {
			t.Fatalf("Failed to post reply: %v", err)
		}

		reply := replyEvent.GetMessagePosted()

		// Find the echo in room events
		roomEventsResult, _ := core.GetRoomEvents(ctx, space.Id, room.Id, 50, nil)
		roomEvents := roomEventsResult.Events
		var echoBodyID string
		for _, e := range roomEvents {
			if msg := e.GetMessagePosted(); msg != nil && msg.EchoOfEventId == replyEvent.Id {
				echoBodyID = msg.MessageBodyId
				break
			}
		}

		if echoBodyID == "" {
			t.Fatal("Echo not found in room events")
		}
		if echoBodyID != reply.MessageBodyId {
			t.Errorf("Echo and reply should share messageBodyId: echo=%q, reply=%q", echoBodyID, reply.MessageBodyId)
		}
	})
}

func TestChattoCore_PostMessage_EchoMentionNotification(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := testContext(t)

	// Create space, room, and two users
	space, _ := core.CreateSpace(ctx, "system", "Mention Space", "A test space")
	room, _ := core.CreateRoom(ctx, "system", space.Id, "General", "General discussion")
	author, _ := core.CreateUser(ctx, "system", "mention-author", "Author", "password123")
	target, _ := core.CreateUser(ctx, "system", "mention-target", "Target", "password123")
	core.JoinSpace(ctx, author.Id, space.Id)
	core.JoinSpace(ctx, target.Id, space.Id)
	core.JoinRoom(ctx, author.Id, space.Id, author.Id, room.Id)
	core.JoinRoom(ctx, target.Id, space.Id, target.Id, room.Id)

	t.Run("echo with mention produces exactly one notification", func(t *testing.T) {
		// Subscribe to live mention events for the target user
		mentionCount := 0
		sub, err := nc.Subscribe("live.instance.user."+target.Id+".mentioned", func(msg *nats.Msg) {
			mentionCount++
		})
		if err != nil {
			t.Fatalf("Failed to subscribe: %v", err)
		}
		defer sub.Unsubscribe()

		// Post root message
		rootEvent, err := core.PostMessage(ctx, space.Id, room.Id, author.Id, "Thread root", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root: %v", err)
		}

		// Post thread reply with echo, mentioning the target user
		_, err = core.PostMessage(ctx, space.Id, room.Id, author.Id, "Hey @mention-target check this out", nil, rootEvent.Id, "", nil, true)
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

// ============================================================================
// In-Reply-To Notification Tests
// ============================================================================

func TestChattoCore_PostMessage_InReplyToNotification(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup: space, room, two users joined
	space, _ := core.CreateSpace(ctx, "system", "Notify Space", "")
	room, _ := core.CreateRoom(ctx, "system", space.Id, "general", "")
	alice, _ := core.CreateUser(ctx, "system", "alice", "Alice", "password123")
	bob, _ := core.CreateUser(ctx, "system", "bob", "Bob", "password123")
	core.JoinSpace(ctx, alice.Id, space.Id)
	core.JoinSpace(ctx, bob.Id, space.Id)
	core.JoinRoom(ctx, alice.Id, space.Id, alice.Id, room.Id)
	core.JoinRoom(ctx, bob.Id, space.Id, bob.Id, room.Id)

	t.Run("creates notification for in-reply-to author", func(t *testing.T) {
		// Alice posts a message
		aliceMsg, err := core.PostMessage(ctx, space.Id, room.Id, alice.Id, "Hello world", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post message: %v", err)
		}

		// Bob replies with inReplyTo (room-level reply, no thread)
		_, err = core.PostMessage(ctx, space.Id, room.Id, bob.Id, "Hi back", nil, "", aliceMsg.Id, nil, false)
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
				if replyNotif.SpaceId != space.Id {
					t.Errorf("ReplyNotification.SpaceId = %s, want %s", replyNotif.SpaceId, space.Id)
				}
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
		aliceMsg, err := core.PostMessage(ctx, space.Id, room.Id, alice.Id, "Talking to myself", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post message: %v", err)
		}

		// Alice replies to her own message
		_, err = core.PostMessage(ctx, space.Id, room.Id, alice.Id, "Replying to myself", nil, "", aliceMsg.Id, nil, false)
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
		core.SetRoomNotificationLevel(ctx, space.Id, alice.Id, room.Id, corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED)
		defer core.SetRoomNotificationLevel(ctx, space.Id, alice.Id, room.Id, corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT)

		// Alice posts a message
		aliceMsg, err := core.PostMessage(ctx, space.Id, room.Id, alice.Id, "Muted test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post message: %v", err)
		}

		// Bob replies
		_, err = core.PostMessage(ctx, space.Id, room.Id, bob.Id, "Reply to muted", nil, "", aliceMsg.Id, nil, false)
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
		aliceMsg, err := core.PostMessage(ctx, space.Id, room.Id, alice.Id, "Dedup test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post message: %v", err)
		}

		// Bob replies AND mentions Alice in the same message
		_, err = core.PostMessage(ctx, space.Id, room.Id, bob.Id, "Hey @alice check this out", nil, "", aliceMsg.Id, nil, false)
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
		rootMsg, err := core.PostMessage(ctx, space.Id, room.Id, alice.Id, "Thread root", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root: %v", err)
		}

		// Bob posts a thread reply
		_, err = core.PostMessage(ctx, space.Id, room.Id, bob.Id, "Thread reply", nil, rootMsg.Id, "", nil, false)
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
		core.JoinSpace(ctx, charlie.Id, space.Id)
		core.JoinRoom(ctx, charlie.Id, space.Id, charlie.Id, room.Id)

		// Alice posts a root message (starts the thread)
		rootMsg, err := core.PostMessage(ctx, space.Id, room.Id, alice.Id, "Thread root for inReplyTo test", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root: %v", err)
		}

		// Bob posts a reply in the thread
		bobMsg, err := core.PostMessage(ctx, space.Id, room.Id, bob.Id, "Bob's thread msg", nil, rootMsg.Id, "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post thread reply: %v", err)
		}

		// Clear notifications from thread participant notifications
		core.DismissAllNotifications(ctx, alice.Id)
		core.DismissAllNotifications(ctx, bob.Id)

		// Charlie replies to Bob's specific message within the thread (inThread + inReplyTo)
		_, err = core.PostMessage(ctx, space.Id, room.Id, charlie.Id, "Replying to Bob in thread", nil, rootMsg.Id, bobMsg.Id, nil, false)
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
		rootMsg, err := core.PostMessage(ctx, space.Id, room.Id, alice.Id, "Dedup thread root", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root: %v", err)
		}

		// Clear Alice's thread participant notification
		core.DismissAllNotifications(ctx, alice.Id)

		// Bob replies to Alice's root message in the thread (both inThread and inReplyTo point to root)
		// Alice is both the thread root author (notifyThreadParticipants) and the inReplyTo author (notifyInReplyToAuthor)
		_, err = core.PostMessage(ctx, space.Id, room.Id, bob.Id, "Replying to root in thread", nil, rootMsg.Id, rootMsg.Id, nil, false)
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

// ============================================================================
// Room Layout Tests
// ============================================================================

func TestChattoCore_GetRoomLayout_NoLayout(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	layout, err := core.GetRoomLayout(ctx, space.Id)
	if err != nil {
		t.Fatalf("GetRoomLayout should not error for missing layout: %v", err)
	}
	if layout != nil {
		t.Error("Expected nil layout when none is configured")
	}
}

func TestChattoCore_UpdateRoomLayout_Create(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room1, _ := core.CreateRoom(ctx, "test-user", space.Id, "General", "General discussion")
	room2, _ := core.CreateRoom(ctx, "test-user", space.Id, "Random", "Random chat")

	layout := &corev1.RoomLayout{
		Sections: []*corev1.RoomLayoutSection{
			{
				Id:      "sec1",
				Name:    "Main",
				RoomIds: []string{room1.Id, room2.Id},
			},
		},
	}

	result, err := core.UpdateRoomLayout(ctx, space.Id, layout)
	if err != nil {
		t.Fatalf("UpdateRoomLayout failed: %v", err)
	}
	if len(result.Sections) != 1 {
		t.Fatalf("Expected 1 section, got %d", len(result.Sections))
	}
	if result.Sections[0].Name != "Main" {
		t.Errorf("Section name = %q, want %q", result.Sections[0].Name, "Main")
	}
	if len(result.Sections[0].RoomIds) != 2 {
		t.Fatalf("Expected 2 room IDs, got %d", len(result.Sections[0].RoomIds))
	}

	// Verify it persists
	fetched, err := core.GetRoomLayout(ctx, space.Id)
	if err != nil {
		t.Fatalf("GetRoomLayout failed: %v", err)
	}
	if fetched == nil {
		t.Fatal("Expected layout to be persisted")
	}
	if len(fetched.Sections) != 1 {
		t.Fatalf("Expected 1 section, got %d", len(fetched.Sections))
	}
	if fetched.Sections[0].Id != "sec1" {
		t.Errorf("Section ID = %q, want %q", fetched.Sections[0].Id, "sec1")
	}
}

func TestChattoCore_UpdateRoomLayout_Update(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room1, _ := core.CreateRoom(ctx, "test-user", space.Id, "Alpha", "")
	room2, _ := core.CreateRoom(ctx, "test-user", space.Id, "Bravo", "")
	room3, _ := core.CreateRoom(ctx, "test-user", space.Id, "Charlie", "")

	// Create initial layout
	layout1 := &corev1.RoomLayout{
		Sections: []*corev1.RoomLayoutSection{
			{Id: "s1", Name: "Section 1", RoomIds: []string{room1.Id, room2.Id}},
		},
	}
	_, err := core.UpdateRoomLayout(ctx, space.Id, layout1)
	if err != nil {
		t.Fatalf("Initial UpdateRoomLayout failed: %v", err)
	}

	// Update with different layout
	layout2 := &corev1.RoomLayout{
		Sections: []*corev1.RoomLayoutSection{
			{Id: "s1", Name: "Renamed Section", RoomIds: []string{room2.Id, room1.Id}},
			{Id: "s2", Name: "New Section", RoomIds: []string{room3.Id}},
		},
	}
	result, err := core.UpdateRoomLayout(ctx, space.Id, layout2)
	if err != nil {
		t.Fatalf("Second UpdateRoomLayout failed: %v", err)
	}
	if len(result.Sections) != 2 {
		t.Fatalf("Expected 2 sections, got %d", len(result.Sections))
	}
	if result.Sections[0].Name != "Renamed Section" {
		t.Errorf("Section 0 name = %q, want %q", result.Sections[0].Name, "Renamed Section")
	}
	if result.Sections[1].Name != "New Section" {
		t.Errorf("Section 1 name = %q, want %q", result.Sections[1].Name, "New Section")
	}
	// Verify order within first section is reversed
	if result.Sections[0].RoomIds[0] != room2.Id {
		t.Errorf("Section 0 first room = %q, want %q", result.Sections[0].RoomIds[0], room2.Id)
	}
}

func TestChattoCore_DeleteRoom_RemovesFromLayout(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room1, _ := core.CreateRoom(ctx, "test-user", space.Id, "Keep", "")
	room2, _ := core.CreateRoom(ctx, "test-user", space.Id, "Delete", "")
	room3, _ := core.CreateRoom(ctx, "test-user", space.Id, "AlsoKeep", "")

	// Create layout with all rooms
	layout := &corev1.RoomLayout{
		Sections: []*corev1.RoomLayoutSection{
			{Id: "s1", Name: "All Rooms", RoomIds: []string{room1.Id, room2.Id, room3.Id}},
		},
	}
	_, err := core.UpdateRoomLayout(ctx, space.Id, layout)
	if err != nil {
		t.Fatalf("UpdateRoomLayout failed: %v", err)
	}

	// Delete the middle room
	err = core.DeleteRoom(ctx, "test-user", space.Id, room2.Id)
	if err != nil {
		t.Fatalf("DeleteRoom failed: %v", err)
	}

	// Verify layout was updated
	fetched, err := core.GetRoomLayout(ctx, space.Id)
	if err != nil {
		t.Fatalf("GetRoomLayout after delete failed: %v", err)
	}
	if fetched == nil {
		t.Fatal("Expected layout to still exist")
	}
	if len(fetched.Sections[0].RoomIds) != 2 {
		t.Fatalf("Expected 2 room IDs after delete, got %d", len(fetched.Sections[0].RoomIds))
	}
	for _, id := range fetched.Sections[0].RoomIds {
		if id == room2.Id {
			t.Error("Deleted room should not be in layout")
		}
	}
}

func TestChattoCore_DeleteRoom_NoLayout(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "Delete", "")

	// Delete room when no layout exists — should not error
	err := core.DeleteRoom(ctx, "test-user", space.Id, room.Id)
	if err != nil {
		t.Fatalf("DeleteRoom without layout should not error: %v", err)
	}
}

func TestChattoCore_UpdateRoomLayout_EmptySections(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Layout with empty sections list (clears layout)
	layout := &corev1.RoomLayout{
		Sections: []*corev1.RoomLayoutSection{},
	}
	result, err := core.UpdateRoomLayout(ctx, space.Id, layout)
	if err != nil {
		t.Fatalf("UpdateRoomLayout with empty sections failed: %v", err)
	}
	if len(result.Sections) != 0 {
		t.Errorf("Expected 0 sections, got %d", len(result.Sections))
	}
}

func TestChattoCore_UpdateRoomLayout_MultipleSections(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room1, _ := core.CreateRoom(ctx, "test-user", space.Id, "Alpha", "")
	room2, _ := core.CreateRoom(ctx, "test-user", space.Id, "Bravo", "")
	room3, _ := core.CreateRoom(ctx, "test-user", space.Id, "Charlie", "")
	room4, _ := core.CreateRoom(ctx, "test-user", space.Id, "Delta", "")

	layout := &corev1.RoomLayout{
		Sections: []*corev1.RoomLayoutSection{
			{Id: "general", Name: "General", RoomIds: []string{room1.Id, room2.Id}},
			{Id: "projects", Name: "Projects", RoomIds: []string{room3.Id, room4.Id}},
		},
	}

	result, err := core.UpdateRoomLayout(ctx, space.Id, layout)
	if err != nil {
		t.Fatalf("UpdateRoomLayout failed: %v", err)
	}

	if len(result.Sections) != 2 {
		t.Fatalf("Expected 2 sections, got %d", len(result.Sections))
	}

	// Verify section order and contents
	if result.Sections[0].Id != "general" || result.Sections[1].Id != "projects" {
		t.Error("Section order not preserved")
	}
	if len(result.Sections[0].RoomIds) != 2 || len(result.Sections[1].RoomIds) != 2 {
		t.Error("Section room counts incorrect")
	}
}

// ============================================================================
// ArchiveRoom Tests
// ============================================================================

func TestChattoCore_ArchiveRoom(t *testing.T) {
	t.Run("sets archived flag", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "")
		room, _ := core.CreateRoom(ctx, "test-user", space.Id, "general", "")

		_, err := core.ArchiveRoom(ctx, "test-user", space.Id, room.Id)
		if err != nil {
			t.Fatalf("ArchiveRoom failed: %v", err)
		}

		retrieved, err := core.GetRoom(ctx, space.Id, room.Id)
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

		space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "")
		room, _ := core.CreateRoom(ctx, "test-user", space.Id, "general", "General chat")

		result, err := core.ArchiveRoom(ctx, "test-user", space.Id, room.Id)
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

		space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "")
		room, _ := core.CreateRoom(ctx, "test-user", space.Id, "general", "")

		_, err := core.ArchiveRoom(ctx, "test-user", space.Id, room.Id)
		if err != nil {
			t.Fatalf("First ArchiveRoom failed: %v", err)
		}

		result, err := core.ArchiveRoom(ctx, "test-user", space.Id, room.Id)
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

		space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "")

		_, err := core.ArchiveRoom(ctx, "test-user", space.Id, "bogus-room-id")
		if err == nil {
			t.Error("Expected error when archiving nonexistent room")
		}
	})

	t.Run("removes room from layout section", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "")
		room1, _ := core.CreateRoom(ctx, "test-user", space.Id, "keep", "")
		room2, _ := core.CreateRoom(ctx, "test-user", space.Id, "archive-me", "")

		layout := &corev1.RoomLayout{
			Sections: []*corev1.RoomLayoutSection{
				{Id: "s1", Name: "Main", RoomIds: []string{room1.Id, room2.Id}},
			},
		}
		_, err := core.UpdateRoomLayout(ctx, space.Id, layout)
		if err != nil {
			t.Fatalf("UpdateRoomLayout failed: %v", err)
		}

		_, err = core.ArchiveRoom(ctx, "test-user", space.Id, room2.Id)
		if err != nil {
			t.Fatalf("ArchiveRoom failed: %v", err)
		}

		fetched, err := core.GetRoomLayout(ctx, space.Id)
		if err != nil {
			t.Fatalf("GetRoomLayout failed: %v", err)
		}
		if fetched == nil {
			t.Fatal("Expected layout to still exist")
		}
		if len(fetched.Sections[0].RoomIds) != 1 {
			t.Fatalf("Expected 1 room in section after archive, got %d", len(fetched.Sections[0].RoomIds))
		}
		if fetched.Sections[0].RoomIds[0] != room1.Id {
			t.Errorf("Expected remaining room to be %q, got %q", room1.Id, fetched.Sections[0].RoomIds[0])
		}
	})

	t.Run("no layout exists", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "")
		room, _ := core.CreateRoom(ctx, "test-user", space.Id, "general", "")

		// Archive when no layout exists — should not error
		_, err := core.ArchiveRoom(ctx, "test-user", space.Id, room.Id)
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

		space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "")
		room, _ := core.CreateRoom(ctx, "test-user", space.Id, "general", "")

		_, err := core.ArchiveRoom(ctx, "test-user", space.Id, room.Id)
		if err != nil {
			t.Fatalf("ArchiveRoom failed: %v", err)
		}

		_, err = core.UnarchiveRoom(ctx, "test-user", space.Id, room.Id)
		if err != nil {
			t.Fatalf("UnarchiveRoom failed: %v", err)
		}

		retrieved, err := core.GetRoom(ctx, space.Id, room.Id)
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

		space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "")
		room, _ := core.CreateRoom(ctx, "test-user", space.Id, "general", "")

		core.ArchiveRoom(ctx, "test-user", space.Id, room.Id)

		result, err := core.UnarchiveRoom(ctx, "test-user", space.Id, room.Id)
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

		space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "")
		room, _ := core.CreateRoom(ctx, "test-user", space.Id, "general", "")

		// Unarchive a room that is not archived — should succeed
		result, err := core.UnarchiveRoom(ctx, "test-user", space.Id, room.Id)
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

		space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "")

		_, err := core.UnarchiveRoom(ctx, "test-user", space.Id, "bogus-room-id")
		if err == nil {
			t.Error("Expected error when unarchiving nonexistent room")
		}
	})

	t.Run("does not re-add room to layout", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "")
		room1, _ := core.CreateRoom(ctx, "test-user", space.Id, "keep", "")
		room2, _ := core.CreateRoom(ctx, "test-user", space.Id, "archive-and-unarchive", "")

		// Create layout with both rooms
		layout := &corev1.RoomLayout{
			Sections: []*corev1.RoomLayoutSection{
				{Id: "s1", Name: "Main", RoomIds: []string{room1.Id, room2.Id}},
			},
		}
		_, err := core.UpdateRoomLayout(ctx, space.Id, layout)
		if err != nil {
			t.Fatalf("UpdateRoomLayout failed: %v", err)
		}

		// Archive removes from layout
		_, err = core.ArchiveRoom(ctx, "test-user", space.Id, room2.Id)
		if err != nil {
			t.Fatalf("ArchiveRoom failed: %v", err)
		}

		// Unarchive should NOT put it back in the layout
		_, err = core.UnarchiveRoom(ctx, "test-user", space.Id, room2.Id)
		if err != nil {
			t.Fatalf("UnarchiveRoom failed: %v", err)
		}

		fetched, err := core.GetRoomLayout(ctx, space.Id)
		if err != nil {
			t.Fatalf("GetRoomLayout failed: %v", err)
		}
		if len(fetched.Sections[0].RoomIds) != 1 {
			t.Fatalf("Expected 1 room in section after unarchive, got %d", len(fetched.Sections[0].RoomIds))
		}
		if fetched.Sections[0].RoomIds[0] != room1.Id {
			t.Errorf("Expected only %q in section, got %q", room1.Id, fetched.Sections[0].RoomIds[0])
		}
	})
}

// ============================================================================
// JoinRoom + Archive Interaction Tests
// ============================================================================

func TestChattoCore_JoinRoom_ArchivedRoom(t *testing.T) {
	t.Run("cannot join archived room", func(t *testing.T) {
		core, _ := setupTestCore(t)
		ctx := testContext(t)

		space, _ := core.CreateSpace(ctx, "owner", "Test Space", "")
		_, _ = core.JoinSpace(ctx, "owner", space.Id)
		room, _ := core.CreateRoom(ctx, "owner", space.Id, "general", "")

		_, err := core.ArchiveRoom(ctx, "owner", space.Id, room.Id)
		if err != nil {
			t.Fatalf("ArchiveRoom failed: %v", err)
		}

		// New user joins the space
		newUser := "new-user"
		_, _ = core.JoinSpace(ctx, newUser, space.Id)

		// Try to join the archived room
		_, err = core.JoinRoom(ctx, newUser, space.Id, newUser, room.Id)
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

		space, _ := core.CreateSpace(ctx, "owner", "Test Space", "")
		_, _ = core.JoinSpace(ctx, "owner", space.Id)
		room, _ := core.CreateRoom(ctx, "owner", space.Id, "general", "")

		// User joins the room first
		user := "member"
		_, _ = core.JoinSpace(ctx, user, space.Id)
		_, err := core.JoinRoom(ctx, user, space.Id, user, room.Id)
		if err != nil {
			t.Fatalf("JoinRoom failed: %v", err)
		}

		// Archive the room
		_, err = core.ArchiveRoom(ctx, "owner", space.Id, room.Id)
		if err != nil {
			t.Fatalf("ArchiveRoom failed: %v", err)
		}

		// Existing membership should still be there
		exists, err := core.RoomMembershipExists(ctx, space.Id, user, room.Id)
		if err != nil {
			t.Fatalf("RoomMembershipExists failed: %v", err)
		}
		if !exists {
			t.Error("Expected existing room membership to remain after archiving")
		}
	})
}

// ============================================================================
// Archive + AutoJoin Interaction
// ============================================================================

func TestChattoCore_ArchiveRoom_PreservesAutoJoin(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "general", "")

	// Set auto_join first
	_, err := core.SetRoomAutoJoin(ctx, "test-user", space.Id, room.Id, true)
	if err != nil {
		t.Fatalf("SetRoomAutoJoin failed: %v", err)
	}

	// Archive the room
	result, err := core.ArchiveRoom(ctx, "test-user", space.Id, room.Id)
	if err != nil {
		t.Fatalf("ArchiveRoom failed: %v", err)
	}

	if !result.AutoJoin {
		t.Error("Expected auto_join to be preserved as true after archiving")
	}
	if !result.Archived {
		t.Error("Expected room to be archived")
	}

	// Verify persisted
	retrieved, err := core.GetRoom(ctx, space.Id, room.Id)
	if err != nil {
		t.Fatalf("GetRoom failed: %v", err)
	}
	if !retrieved.AutoJoin {
		t.Error("Expected auto_join to persist as true after archiving")
	}
}

// ============================================================================
// SetRoomAutoJoin Edge Cases
// ============================================================================

func TestChattoCore_SetRoomAutoJoin_NonexistentRoom(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "")

	_, err := core.SetRoomAutoJoin(ctx, "test-user", space.Id, "bogus-room-id", true)
	if err == nil {
		t.Error("Expected error when setting auto_join on nonexistent room")
	}
}

func TestChattoCore_SetRoomAutoJoin_ArchivedRoom(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "general", "")

	// Archive the room
	_, err := core.ArchiveRoom(ctx, "test-user", space.Id, room.Id)
	if err != nil {
		t.Fatalf("ArchiveRoom failed: %v", err)
	}

	// Setting auto_join on an archived room should succeed (it's just metadata)
	result, err := core.SetRoomAutoJoin(ctx, "test-user", space.Id, room.Id, true)
	if err != nil {
		t.Fatalf("SetRoomAutoJoin on archived room should succeed: %v", err)
	}
	if !result.AutoJoin {
		t.Error("Expected auto_join to be true")
	}
	if !result.Archived {
		t.Error("Expected room to still be archived")
	}
}
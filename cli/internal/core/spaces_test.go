package core

import (
	"testing"
)

// ============================================================================
// JoinDefaultRooms Tests
// ============================================================================

func TestJoinDefaultRooms_JoinsAutoJoinRooms(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	creatorID := "creator123"

	generalRoom, err := core.CreateRoom(ctx, creatorID, KindChannel, "general", "")
	if err != nil {
		t.Fatalf("Failed to create general room: %v", err)
	}
	if _, err := core.SetRoomAutoJoin(ctx, creatorID, KindChannel, generalRoom.Id, true); err != nil {
		t.Fatalf("Failed to set auto_join: %v", err)
	}

	secretRoom, err := core.CreateRoom(ctx, creatorID, KindChannel, "secret", "")
	if err != nil {
		t.Fatalf("Failed to create secret room: %v", err)
	}

	newUserID := "newuser456"
	core.JoinDefaultRooms(ctx, newUserID)

	inGeneral, err := core.RoomMembershipExists(ctx, KindChannel, newUserID, generalRoom.Id)
	if err != nil {
		t.Fatalf("RoomMembershipExists: %v", err)
	}
	if !inGeneral {
		t.Error("Expected user to be auto-joined to 'general'")
	}

	inSecret, err := core.RoomMembershipExists(ctx, KindChannel, newUserID, secretRoom.Id)
	if err != nil {
		t.Fatalf("RoomMembershipExists: %v", err)
	}
	if inSecret {
		t.Error("Did not expect user to be auto-joined to 'secret'")
	}
}

func TestJoinDefaultRooms_SkipsArchivedRooms(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	creatorID := "creator123"

	archivedRoom, err := core.CreateRoom(ctx, creatorID, KindChannel, "archived", "")
	if err != nil {
		t.Fatalf("Failed to create archived room: %v", err)
	}
	if _, err := core.SetRoomAutoJoin(ctx, creatorID, KindChannel, archivedRoom.Id, true); err != nil {
		t.Fatalf("Failed to set auto_join: %v", err)
	}
	if _, err := core.ArchiveRoom(ctx, creatorID, KindChannel, archivedRoom.Id); err != nil {
		t.Fatalf("Failed to archive room: %v", err)
	}

	newUserID := "newuser456"
	core.JoinDefaultRooms(ctx, newUserID)

	in, err := core.RoomMembershipExists(ctx, KindChannel, newUserID, archivedRoom.Id)
	if err != nil {
		t.Fatalf("RoomMembershipExists: %v", err)
	}
	if in {
		t.Error("Did not expect user to be auto-joined to archived room")
	}
}

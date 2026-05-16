package core

import (
	"testing"
)

// ============================================================================
// Global Room Membership Tests
// ============================================================================

func TestRoomMembershipExists_GlobalRoom_ImplicitMembership(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	creatorID := "creator123"

	generalRoom, err := core.CreateRoom(ctx, creatorID, KindChannel, "", "general", "")
	if err != nil {
		t.Fatalf("Failed to create general room: %v", err)
	}
	if _, err := core.SetRoomAutoJoin(ctx, creatorID, KindChannel, generalRoom.Id, true); err != nil {
		t.Fatalf("Failed to mark room as global: %v", err)
	}

	secretRoom, err := core.CreateRoom(ctx, creatorID, KindChannel, "", "secret", "")
	if err != nil {
		t.Fatalf("Failed to create secret room: %v", err)
	}

	newUserID := "newuser456"

	inGeneral, err := core.RoomMembershipExists(ctx, KindChannel, newUserID, generalRoom.Id)
	if err != nil {
		t.Fatalf("RoomMembershipExists (global): %v", err)
	}
	if !inGeneral {
		t.Error("Expected implicit membership in a global room (no KV record needed)")
	}

	inSecret, err := core.RoomMembershipExists(ctx, KindChannel, newUserID, secretRoom.Id)
	if err != nil {
		t.Fatalf("RoomMembershipExists (non-global): %v", err)
	}
	if inSecret {
		t.Error("Did not expect implicit membership in a non-global room")
	}
}

func TestLeaveRoom_GlobalRoom_Blocked(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	creatorID := "creator123"

	room, err := core.CreateRoom(ctx, creatorID, KindChannel, "", "lobby", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.SetRoomAutoJoin(ctx, creatorID, KindChannel, room.Id, true); err != nil {
		t.Fatalf("SetRoomAutoJoin: %v", err)
	}

	if err := core.LeaveRoom(ctx, "someone-else", KindChannel, "someone-else", room.Id); err != ErrCannotLeaveAutoJoinRoom {
		t.Errorf("Expected ErrCannotLeaveAutoJoinRoom on a global room, got: %v", err)
	}
}

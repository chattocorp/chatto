package core

import "testing"

// TestCanSeeRoom_VisibilityFollowsJoinability locks in the post-retirement
// contract: a user sees a room iff they're a member OR `room.join` resolves
// to allow at the room. There is no separate room.list gate.
func TestCanSeeRoom_VisibilityFollowsJoinability(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	owner, _ := core.CreateUser(ctx, SystemActorID, "vis-owner", "Owner", "password123")
	if err := core.AssignServerRole(ctx, SystemActorID, owner.Id, RoleOwner); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	room, err := core.CreateRoom(ctx, owner.Id, KindChannel, "", "vis-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	t.Run("default everyone-grant: non-member sees the room", func(t *testing.T) {
		stranger, _ := core.CreateUser(ctx, SystemActorID, "vis-stranger-default", "Stranger", "password123")
		got, err := core.CanSeeRoom(ctx, stranger.Id, KindChannel, room.Id)
		if err != nil {
			t.Fatalf("CanSeeRoom: %v", err)
		}
		if !got {
			t.Error("expected non-member with default room.join grant to see the room")
		}
	})

	t.Run("room-scope deny on everyone: non-member loses visibility", func(t *testing.T) {
		if err := core.DenyRoomPermission(ctx, room.Id, RoleEveryone, PermRoomJoin); err != nil {
			t.Fatalf("DenyRoomPermission: %v", err)
		}
		t.Cleanup(func() {
			_ = core.ClearRoomPermissionState(ctx, room.Id, RoleEveryone, PermRoomJoin)
		})

		stranger, _ := core.CreateUser(ctx, SystemActorID, "vis-stranger-denied", "Stranger", "password123")
		got, err := core.CanSeeRoom(ctx, stranger.Id, KindChannel, room.Id)
		if err != nil {
			t.Fatalf("CanSeeRoom: %v", err)
		}
		if got {
			t.Error("expected non-member to lose visibility after room.join deny")
		}
	})

	t.Run("existing member keeps visibility even when room.join is denied for everyone", func(t *testing.T) {
		// Join while the room is open, then deny join for everyone — the
		// already-member user should still see it. The contract is
		// "member OR canJoin," not "canJoin alone."
		member, _ := core.CreateUser(ctx, SystemActorID, "vis-member", "Member", "password123")
		if _, err := core.JoinRoom(ctx, member.Id, KindChannel, member.Id, room.Id); err != nil {
			t.Fatalf("JoinRoom: %v", err)
		}
		if err := core.DenyRoomPermission(ctx, room.Id, RoleEveryone, PermRoomJoin); err != nil {
			t.Fatalf("DenyRoomPermission: %v", err)
		}
		t.Cleanup(func() {
			_ = core.ClearRoomPermissionState(ctx, room.Id, RoleEveryone, PermRoomJoin)
		})

		got, err := core.CanSeeRoom(ctx, member.Id, KindChannel, room.Id)
		if err != nil {
			t.Fatalf("CanSeeRoom: %v", err)
		}
		if !got {
			t.Error("expected explicit member to retain visibility despite room.join deny")
		}
	})

	t.Run("DM kind: CanSeeRoom is always false (DMs use their own listing)", func(t *testing.T) {
		got, err := core.CanSeeRoom(ctx, owner.Id, KindDM, "R_dm_visibility_probe")
		if err != nil {
			t.Fatalf("CanSeeRoom(DM): %v", err)
		}
		if got {
			t.Error("expected CanSeeRoom to return false for DM rooms")
		}
	})
}

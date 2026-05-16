package core

import "testing"

// TestGlobalRoom_PermissionDerivedMembership pins the contract that a global
// room's implicit membership IS the set of users for whom `room.join`
// resolves to allow at the room. There is no implicit-everyone shortcut:
// RoomMembershipExists asks the resolver, and the resolver decides.
func TestGlobalRoom_PermissionDerivedMembership(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Make the room global.
	owner, _ := core.CreateUser(ctx, SystemActorID, "globalmem-owner", "Owner", "password123")
	if err := core.AssignServerRole(ctx, SystemActorID, owner.Id, RoleOwner); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	room, err := core.CreateRoom(ctx, owner.Id, KindChannel, "", "globalmem", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.SetRoomGlobal(ctx, owner.Id, KindChannel, room.Id, true); err != nil {
		t.Fatalf("SetRoomGlobal: %v", err)
	}

	t.Run("default everyone-grant: every authenticated user is an implicit member", func(t *testing.T) {
		// `room.join` is granted to everyone by default (server scope),
		// so any user resolves allow → implicit membership.
		member, _ := core.CreateUser(ctx, SystemActorID, "globalmem-default", "Default", "password123")
		got, err := core.RoomMembershipExists(ctx, KindChannel, member.Id, room.Id)
		if err != nil {
			t.Fatalf("RoomMembershipExists: %v", err)
		}
		if !got {
			t.Error("expected implicit membership via default everyone-grant on room.join")
		}
	})

	t.Run("user-level deny on room.join suspends membership", func(t *testing.T) {
		// User-level overrides outrank every role grant.
		suspended, _ := core.CreateUser(ctx, SystemActorID, "globalmem-suspended", "Suspended", "password123")
		if err := core.DenyUserPermission(ctx, suspended.Id, PermRoomJoin); err != nil {
			t.Fatalf("DenyUserPermission: %v", err)
		}
		t.Cleanup(func() {
			_ = core.ClearUserPermissionState(ctx, suspended.Id, PermRoomJoin)
		})

		got, err := core.RoomMembershipExists(ctx, KindChannel, suspended.Id, room.Id)
		if err != nil {
			t.Fatalf("RoomMembershipExists: %v", err)
		}
		if got {
			t.Error("expected user-level deny to suspend implicit membership in global room")
		}
	})

	t.Run("group-scope deny gates membership; explicit role grant restores", func(t *testing.T) {
		// Deny room.join on everyone at the room's group; the global
		// room's implicit member set shrinks to roles with an explicit
		// grant.
		groupID := room.GroupId
		if err := core.DenyGroupPermission(ctx, groupID, RoleEveryone, PermRoomJoin); err != nil {
			t.Fatalf("DenyGroupPermission(everyone): %v", err)
		}
		t.Cleanup(func() {
			_ = core.ClearGroupPermissionState(ctx, groupID, RoleEveryone, PermRoomJoin)
		})

		regular, _ := core.CreateUser(ctx, SystemActorID, "globalmem-regular", "Regular", "password123")
		got, err := core.RoomMembershipExists(ctx, KindChannel, regular.Id, room.Id)
		if err != nil {
			t.Fatalf("RoomMembershipExists(regular): %v", err)
		}
		if got {
			t.Error("expected regular to lose implicit membership after group-scope deny on everyone")
		}

		// Moderator role still has room.join via its seeded grants; the
		// hierarchy walker visits the higher-rank role first and finds
		// allow before the everyone-deny applies.
		mod, _ := core.CreateUser(ctx, SystemActorID, "globalmem-mod", "Mod", "password123")
		if err := core.AssignServerRole(ctx, SystemActorID, mod.Id, RoleModerator); err != nil {
			t.Fatalf("AssignServerRole(moderator): %v", err)
		}
		got, err = core.RoomMembershipExists(ctx, KindChannel, mod.Id, room.Id)
		if err != nil {
			t.Fatalf("RoomMembershipExists(mod): %v", err)
		}
		if !got {
			t.Error("expected moderator to retain implicit membership via its own room.join grant")
		}
	})

	t.Run("Room.members matches RoomMembershipExists pointwise", func(t *testing.T) {
		// The contract: for every server user, membership listing and
		// the per-user membership check agree. This is the invariant
		// that lets the rest of the codebase trust either path.
		users, err := core.ListUsers(ctx)
		if err != nil {
			t.Fatalf("ListUsers: %v", err)
		}

		// Compute the "members per the per-user check" set.
		expected := make(map[string]bool, len(users))
		for _, u := range users {
			got, err := core.RoomMembershipExists(ctx, KindChannel, u.Id, room.Id)
			if err != nil {
				t.Fatalf("RoomMembershipExists(%s): %v", u.Id, err)
			}
			expected[u.Id] = got
		}

		// And the "members per Room.members would list" set (same
		// filter logic the resolver uses).
		actual := make(map[string]bool, len(users))
		for _, u := range users {
			canJoin, err := core.CanJoinRoomAt(ctx, u.Id, KindChannel, room.Id)
			if err != nil {
				t.Fatalf("CanJoinRoomAt(%s): %v", u.Id, err)
			}
			actual[u.Id] = canJoin
		}

		for uid, expectedMember := range expected {
			if actual[uid] != expectedMember {
				t.Errorf("user %s: membership-check=%v, listing-check=%v (must match)", uid, expectedMember, actual[uid])
			}
		}
	})
}

package core

import "testing"

// TestCanCreateRoom_GroupTier covers the post-#330 group-tier `room.create`
// behavior. Operators can grant room.create at server scope (acts as a global
// "this role can create rooms anywhere") or at a specific group's scope (only
// in that group). A group-scope deny on a role overrides a server-scope allow
// on the same role.
func TestCanCreateRoom_GroupTier(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Clear the seeded everyone defaults so the test starts from a known
	// state: no role has room.create at any scope.
	if err := core.ClearInstancePermissionState(ctx, RoleEveryone, PermRoomCreate); err != nil {
		t.Fatalf("ClearInstancePermissionState: %v", err)
	}

	groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered: %v", err)
	}
	if len(groups) == 0 {
		t.Fatal("expected at least one seeded room group")
	}
	primaryGroupID := groups[0].Id

	otherGroup, err := core.CreateRoomGroup(ctx, SystemActorID, "Other", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}

	member, err := core.CreateUser(ctx, SystemActorID, "groupcreate-member", "Member", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Baseline: no grants anywhere → denied.
	can, err := core.CanCreateRoom(ctx, member.Id, KindChannel, primaryGroupID)
	if err != nil {
		t.Fatalf("baseline CanCreateRoom: %v", err)
	}
	if can {
		t.Fatal("baseline: expected no room.create with no grants")
	}

	t.Run("server-scope grant allows creating in any group", func(t *testing.T) {
		if err := core.GrantInstancePermission(ctx, RoleEveryone, PermRoomCreate); err != nil {
			t.Fatalf("GrantInstancePermission: %v", err)
		}
		t.Cleanup(func() {
			_ = core.ClearInstancePermissionState(ctx, RoleEveryone, PermRoomCreate)
		})

		for _, gid := range []string{primaryGroupID, otherGroup.Id} {
			can, err := core.CanCreateRoom(ctx, member.Id, KindChannel, gid)
			if err != nil {
				t.Fatalf("CanCreateRoom(group=%s): %v", gid, err)
			}
			if !can {
				t.Errorf("server-scope grant should allow creation in group %s", gid)
			}
		}
		// And with no groupID (server-tier check) should also pass.
		can, err := core.CanCreateRoom(ctx, member.Id, KindChannel, "")
		if err != nil {
			t.Fatalf("CanCreateRoom(no group): %v", err)
		}
		if !can {
			t.Error("server-scope grant should allow no-group (pure server) check")
		}
	})

	t.Run("group-only grant scopes creation to that group", func(t *testing.T) {
		if err := core.GrantGroupPermission(ctx, primaryGroupID, RoleEveryone, PermRoomCreate); err != nil {
			t.Fatalf("GrantGroupPermission: %v", err)
		}
		t.Cleanup(func() {
			_ = core.ClearGroupPermissionState(ctx, primaryGroupID, RoleEveryone, PermRoomCreate)
		})

		can, err := core.CanCreateRoom(ctx, member.Id, KindChannel, primaryGroupID)
		if err != nil {
			t.Fatalf("CanCreateRoom(primary group): %v", err)
		}
		if !can {
			t.Error("group-scope grant should allow creation in that group")
		}

		can, err = core.CanCreateRoom(ctx, member.Id, KindChannel, otherGroup.Id)
		if err != nil {
			t.Fatalf("CanCreateRoom(other group): %v", err)
		}
		if can {
			t.Error("group-scope grant should NOT allow creation in a different group")
		}
	})

	t.Run("group-scope deny overrides server-scope allow", func(t *testing.T) {
		if err := core.GrantInstancePermission(ctx, RoleEveryone, PermRoomCreate); err != nil {
			t.Fatalf("GrantInstancePermission: %v", err)
		}
		t.Cleanup(func() {
			_ = core.ClearInstancePermissionState(ctx, RoleEveryone, PermRoomCreate)
		})
		if err := core.DenyGroupPermission(ctx, primaryGroupID, RoleEveryone, PermRoomCreate); err != nil {
			t.Fatalf("DenyGroupPermission: %v", err)
		}
		t.Cleanup(func() {
			_ = core.ClearGroupPermissionState(ctx, primaryGroupID, RoleEveryone, PermRoomCreate)
		})

		can, err := core.CanCreateRoom(ctx, member.Id, KindChannel, primaryGroupID)
		if err != nil {
			t.Fatalf("CanCreateRoom(primary group): %v", err)
		}
		if can {
			t.Error("group-scope deny should override server-scope allow in that group")
		}

		// Other group has no group-scope entry; server-scope allow still
		// cascades through.
		can, err = core.CanCreateRoom(ctx, member.Id, KindChannel, otherGroup.Id)
		if err != nil {
			t.Fatalf("CanCreateRoom(other group): %v", err)
		}
		if !can {
			t.Error("server-scope allow should still cascade into groups with no override")
		}
	})

	t.Run("empty groupID falls back to pure server-scope check", func(t *testing.T) {
		// Grant only at primary group; no server-scope grant.
		if err := core.GrantGroupPermission(ctx, primaryGroupID, RoleEveryone, PermRoomCreate); err != nil {
			t.Fatalf("GrantGroupPermission: %v", err)
		}
		t.Cleanup(func() {
			_ = core.ClearGroupPermissionState(ctx, primaryGroupID, RoleEveryone, PermRoomCreate)
		})

		can, err := core.CanCreateRoom(ctx, member.Id, KindChannel, "")
		if err != nil {
			t.Fatalf("CanCreateRoom(no group): %v", err)
		}
		if can {
			t.Error("no-group check should not see group-scope grants")
		}
	})
}

// TestServerTierCascadeIntoChannelRooms locks the post-revision behavior of
// ADR-031: server-scope grants are the global default and cascade into channel
// rooms when no group/room override exists. A group-scope decision still wins
// over a server-scope decision (same role).
func TestServerTierCascadeIntoChannelRooms(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	member, err := core.CreateUser(ctx, SystemActorID, "cascade-member", "Member", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "cascade-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	groupID := room.GroupId

	// Pick a permission and start from a clean slate at every tier so the
	// cascade chain is the only mechanism that could allow.
	const perm = PermMessageReact
	if err := core.ClearGroupPermissionState(ctx, groupID, RoleEveryone, perm); err != nil {
		t.Fatalf("ClearGroupPermissionState: %v", err)
	}
	if err := core.ClearInstancePermissionState(ctx, RoleEveryone, perm); err != nil {
		t.Fatalf("ClearInstancePermissionState: %v", err)
	}

	// Baseline: no grants anywhere → no decision → denied.
	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, perm)
	if err != nil {
		t.Fatalf("HasRoomPermission baseline: %v", err)
	}
	if has {
		t.Fatal("baseline: expected deny with no grants at any tier")
	}

	t.Run("server-scope grant cascades into the channel room", func(t *testing.T) {
		if err := core.GrantInstancePermission(ctx, RoleEveryone, perm); err != nil {
			t.Fatalf("GrantInstancePermission: %v", err)
		}
		t.Cleanup(func() {
			_ = core.ClearInstancePermissionState(ctx, RoleEveryone, perm)
		})

		has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, perm)
		if err != nil {
			t.Fatalf("HasRoomPermission: %v", err)
		}
		if !has {
			t.Error("server-scope grant should cascade into the channel room when no group/room override exists")
		}
	})

	t.Run("group-scope deny wins over server-scope allow (same role)", func(t *testing.T) {
		if err := core.GrantInstancePermission(ctx, RoleEveryone, perm); err != nil {
			t.Fatalf("GrantInstancePermission: %v", err)
		}
		t.Cleanup(func() {
			_ = core.ClearInstancePermissionState(ctx, RoleEveryone, perm)
		})
		if err := core.DenyGroupPermission(ctx, groupID, RoleEveryone, perm); err != nil {
			t.Fatalf("DenyGroupPermission: %v", err)
		}
		t.Cleanup(func() {
			_ = core.ClearGroupPermissionState(ctx, groupID, RoleEveryone, perm)
		})

		has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, perm)
		if err != nil {
			t.Fatalf("HasRoomPermission: %v", err)
		}
		if has {
			t.Error("group-scope deny should win over server-scope allow for the same role")
		}
	})
}

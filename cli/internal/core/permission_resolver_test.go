package core

import (
	"testing"
)

// ============================================================================
// HasInstancePermission Tests
// ============================================================================

func TestPermissionResolver_HasInstancePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")

	t.Run("returns true when user has permission via instance-everyone role", func(t *testing.T) {
		// instance-everyone gets space.list by default
		has, err := core.permissionResolver.HasInstancePermission(ctx, user.Id, PermSpaceList)
		if err != nil {
			t.Fatalf("HasInstancePermission() error = %v", err)
		}
		if !has {
			t.Error("Expected user to have space.list via instance-everyone role")
		}
	})

	t.Run("returns true when user has space.create via instance-everyone role", func(t *testing.T) {
		// instance-everyone gets space.create by default
		has, err := core.permissionResolver.HasInstancePermission(ctx, user.Id, PermSpaceCreate)
		if err != nil {
			t.Fatalf("HasInstancePermission() error = %v", err)
		}
		if !has {
			t.Error("Expected user to have space.create via instance-everyone role")
		}
	})

	t.Run("returns false when user lacks permission", func(t *testing.T) {
		// Regular user doesn't have admin.access
		has, err := core.permissionResolver.HasInstancePermission(ctx, user.Id, PermAdminAccess)
		if err != nil {
			t.Fatalf("HasInstancePermission() error = %v", err)
		}
		if has {
			t.Error("Expected user NOT to have admin.access")
		}
	})

	t.Run("returns error for permission that does not apply at instance scope", func(t *testing.T) {
		// space.manage only applies at space scope
		_, err := core.permissionResolver.HasInstancePermission(ctx, user.Id, PermSpaceManage)
		if err == nil {
			t.Error("Expected error for permission that doesn't apply at instance scope")
		}
	})
}

func TestPermissionResolver_HasInstancePermission_DenyWins(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")

	t.Run("same-role denial replaces grant", func(t *testing.T) {
		// Grant permission via instance-everyone role
		err := core.GrantInstanceRolePermission(ctx, InstRoleEveryone, PermSpaceList)
		if err != nil {
			t.Fatalf("Failed to grant permission: %v", err)
		}

		// Deny same permission for the same role (replaces the grant)
		err = core.DenyInstanceRolePermission(ctx, InstRoleEveryone, PermSpaceList)
		if err != nil {
			t.Fatalf("Failed to deny permission: %v", err)
		}

		// User should NOT have the permission (denial replaced grant)
		has, err := core.permissionResolver.HasInstancePermission(ctx, user.Id, PermSpaceList)
		if err != nil {
			t.Fatalf("HasInstancePermission() error = %v", err)
		}
		if has {
			t.Error("Expected denial to replace grant")
		}

		// Restore for other tests
		core.GrantInstanceRolePermission(ctx, InstRoleEveryone, PermSpaceList)
	})
}

func TestPermissionResolver_HasInstancePermission_CustomDenyRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user (has everyone role)
	user, _ := core.CreateUser(ctx, "system", "testuser-denyrole", "Test User", "password123")

	// Verify user initially has space.list (via everyone role default)
	has, err := core.permissionResolver.HasInstancePermission(ctx, user.Id, PermSpaceList)
	if err != nil {
		t.Fatalf("HasInstancePermission() error = %v", err)
	}
	if !has {
		t.Fatal("Expected user to have space.list initially via everyone role")
	}

	// Create a custom deny role (replicates the e2e test scenario)
	denyRole, err := core.CreateInstanceRole(ctx, "instance-denytest", "Deny space.list", "Test deny role")
	if err != nil {
		t.Fatalf("Failed to create deny role: %v", err)
	}
	t.Logf("Created deny role with position: %d", denyRole.Position)

	// Deny space.list on the deny role
	err = core.DenyInstancePermission(ctx, "instance-denytest", PermSpaceList)
	if err != nil {
		t.Fatalf("Failed to deny permission: %v", err)
	}

	// Assign deny role to user
	err = core.AssignInstanceRole(ctx, SystemActorID, user.Id, "instance-denytest")
	if err != nil {
		t.Fatalf("Failed to assign deny role: %v", err)
	}

	// User now has: instance-denytest (deny space.list), everyone (grant space.list)
	// The deny role has the highest rank (lowest position), so its deny should win.
	has, err = core.permissionResolver.HasInstancePermission(ctx, user.Id, PermSpaceList)
	if err != nil {
		t.Fatalf("HasInstancePermission() error = %v", err)
	}
	if has {
		t.Error("Expected custom deny role to block space.list despite everyone granting it")
	}

	// Also verify GetUserInstancePermissions (the old path) agrees
	perms, err := core.GetUserInstancePermissions(ctx, user.Id)
	if err != nil {
		t.Fatalf("GetUserInstancePermissions() error = %v", err)
	}
	for _, p := range perms {
		if p == PermSpaceList {
			t.Error("Expected space.list to NOT be in GetUserInstancePermissions result")
			break
		}
	}
}

func TestPermissionResolver_HasInstancePermission_Hierarchy(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user and assign admin role
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	_ = core.AssignInstanceRole(ctx, SystemActorID, user.Id, InstRoleAdmin)

	t.Run("higher-ranked role grant beats lower-ranked role denial", func(t *testing.T) {
		// Deny space.create for everyone (low rank, position MaxInt32)
		err := core.DenyInstanceRolePermission(ctx, InstRoleEveryone, PermSpaceCreate)
		if err != nil {
			t.Fatalf("Failed to deny permission: %v", err)
		}

		// Grant space.create for admin (high rank, position 1)
		err = core.GrantInstanceRolePermission(ctx, InstRoleAdmin, PermSpaceCreate)
		if err != nil {
			t.Fatalf("Failed to grant permission: %v", err)
		}

		// User has both admin (grant) and everyone (deny) roles.
		// Admin is higher rank (position 1 < MaxInt32), so admin's grant should win.
		has, err := core.permissionResolver.HasInstancePermission(ctx, user.Id, PermSpaceCreate)
		if err != nil {
			t.Fatalf("HasInstancePermission() error = %v", err)
		}
		if !has {
			t.Error("Expected higher-ranked role grant to win over lower-ranked role denial")
		}

		// Cleanup
		core.ClearInstanceRolePermission(ctx, InstRoleEveryone, PermSpaceCreate)
		core.ClearInstanceRolePermission(ctx, InstRoleAdmin, PermSpaceCreate)
	})

	t.Run("higher-ranked role denial beats lower-ranked role grant", func(t *testing.T) {
		// Grant space.create for everyone (low rank)
		err := core.GrantInstanceRolePermission(ctx, InstRoleEveryone, PermSpaceCreate)
		if err != nil {
			t.Fatalf("Failed to grant permission: %v", err)
		}

		// Deny space.create for admin (high rank)
		err = core.DenyInstanceRolePermission(ctx, InstRoleAdmin, PermSpaceCreate)
		if err != nil {
			t.Fatalf("Failed to deny permission: %v", err)
		}

		// Admin denial (position 1) should be checked before everyone grant (position MaxInt32)
		has, err := core.permissionResolver.HasInstancePermission(ctx, user.Id, PermSpaceCreate)
		if err != nil {
			t.Fatalf("HasInstancePermission() error = %v", err)
		}
		if has {
			t.Error("Expected higher-ranked role denial to win over lower-ranked role grant")
		}

		// Cleanup
		core.ClearInstanceRolePermission(ctx, InstRoleEveryone, PermSpaceCreate)
		core.ClearInstanceRolePermission(ctx, InstRoleAdmin, PermSpaceCreate)
	})

}


// ============================================================================
// HasSpacePermission Tests
// ============================================================================

func TestPermissionResolver_HasSpacePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user and space
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	space, _ := core.CreateSpace(ctx, user.Id, "Test Space", "A test space")

	// User joins the space (creator is automatically a member with admin role)

	t.Run("returns true when user has permission via space role", func(t *testing.T) {
		// Space admin gets space.manage
		has, err := core.permissionResolver.HasSpacePermission(ctx, user.Id, space.Id, PermSpaceManage)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if !has {
			t.Error("Expected space admin to have space.manage")
		}
	})

	t.Run("returns false when user lacks permission at space level", func(t *testing.T) {
		// Create another user who is not a member
		otherUser, _ := core.CreateUser(ctx, "system", "otheruser", "Other User", "password123")

		// Non-member should not have space.manage
		has, err := core.permissionResolver.HasSpacePermission(ctx, otherUser.Id, space.Id, PermSpaceManage)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if has {
			t.Error("Expected non-member NOT to have space.manage")
		}
	})

	t.Run("instance-only permission returns false at space level", func(t *testing.T) {
		// admin.access is instance-only, but HasSpacePermission allows it (falls back to instance)
		// It should return false since user doesn't have admin.access at instance level
		has, err := core.permissionResolver.HasSpacePermission(ctx, user.Id, space.Id, PermAdminAccess)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if has {
			t.Error("Expected non-admin to NOT have admin.access")
		}
	})
}

func TestPermissionResolver_HasSpacePermission_InstanceFallback(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user and space
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	space, _ := core.CreateSpace(ctx, user.Id, "Test Space", "A test space")

	t.Run("space member gets space-scoped permissions from space roles", func(t *testing.T) {
		// User is a space member (creator) with owner role
		// room.create is granted via space's everyone role defaults
		has, err := core.permissionResolver.HasSpacePermission(ctx, user.Id, space.Id, PermRoomCreate)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if !has {
			t.Error("Expected space member to have room.create via space role")
		}
	})

	t.Run("non-member does NOT get space-scoped permissions", func(t *testing.T) {
		// Create user who is NOT a space member
		nonMember, _ := core.CreateUser(ctx, "system", "nonmember", "Non Member", "password123")

		// Non-member should NOT get room.create
		has, err := core.permissionResolver.HasSpacePermission(ctx, nonMember.Id, space.Id, PermRoomCreate)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if has {
			t.Error("Expected non-member NOT to have space-scoped permission")
		}
	})

	t.Run("non-member CAN get space.join (special exception)", func(t *testing.T) {
		// Create user who is NOT a space member
		nonMember, _ := core.CreateUser(ctx, "system", "nonmember2", "Non Member 2", "password123")

		// space.join is special - non-members need this to join
		has, err := core.permissionResolver.HasSpacePermission(ctx, nonMember.Id, space.Id, PermSpaceJoin)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if !has {
			t.Error("Expected non-member to have space.join (special exception)")
		}
	})
}

func TestPermissionResolver_HasSpacePermission_DenyWins(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user and space
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	space, _ := core.CreateSpace(ctx, user.Id, "Test Space", "A test space")

	t.Run("deny-wins at space level", func(t *testing.T) {
		// Grant permission to member role
		err := core.GrantSpaceRolePermission(ctx, space.Id, SpaceRoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to grant permission: %v", err)
		}

		// Deny to admin role (user is admin)
		err = core.DenySpaceRolePermission(ctx, space.Id, SpaceRoleOwner, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to deny permission: %v", err)
		}

		// User should NOT have the permission (deny wins across all roles)
		has, err := core.permissionResolver.HasSpacePermission(ctx, user.Id, space.Id, PermMessagePost)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if has {
			t.Error("Expected deny to win at space level")
		}
	})
}


func TestPermissionResolver_HasSpacePermission_InstanceRoleOverride(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user and space
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	space, _ := core.CreateSpace(ctx, user.Id, "Test Space", "A test space")

	t.Run("space can override instance role permissions", func(t *testing.T) {
		// Grant permission to instance-everyone at space level (override)
		err := core.GrantSpaceRolePermission(ctx, space.Id, InstRoleEveryone, PermRoomManage)
		if err != nil {
			t.Fatalf("Failed to grant permission: %v", err)
		}

		// User should have the permission via the space-level override
		has, err := core.permissionResolver.HasSpacePermission(ctx, user.Id, space.Id, PermRoomManage)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if !has {
			t.Error("Expected space-level override for instance role to work")
		}
	})
}

func TestPermissionResolver_HasSpacePermission_DMSpace(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")

	// Use the DMSpaceID constant (which is "DM")

	t.Run("DM space allows message.post", func(t *testing.T) {
		has, err := core.permissionResolver.HasSpacePermission(ctx, user.Id, DMSpaceID, PermMessagePost)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if !has {
			t.Error("Expected DM space to allow message.post")
		}
	})

	t.Run("DM space denies space.manage", func(t *testing.T) {
		has, err := core.permissionResolver.HasSpacePermission(ctx, user.Id, DMSpaceID, PermSpaceManage)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if has {
			t.Error("Expected DM space NOT to allow space.manage")
		}
	})

	t.Run("DM space allows room.join", func(t *testing.T) {
		has, err := core.permissionResolver.HasSpacePermission(ctx, user.Id, DMSpaceID, PermRoomJoin)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if !has {
			t.Error("Expected DM space to allow room.join")
		}
	})
}

// ============================================================================
// HasRoomPermission Tests
// ============================================================================

func TestPermissionResolver_HasRoomPermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user and space with room
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	space, _ := core.CreateSpace(ctx, user.Id, "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, user.Id, space.Id, "General", "General chat")

	t.Run("returns true when user has permission at room level", func(t *testing.T) {
		// Grant permission at room level
		err := core.grantRoomRolePermissionInternal(ctx, space.Id, room.Id, SpaceRoleOwner, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to grant room permission: %v", err)
		}

		has, err := core.permissionResolver.HasRoomPermission(ctx, user.Id, space.Id, room.Id, PermMessagePost)
		if err != nil {
			t.Fatalf("HasRoomPermission() error = %v", err)
		}
		if !has {
			t.Error("Expected user to have permission at room level")
		}
	})

	t.Run("falls back to space level", func(t *testing.T) {
		// User is space admin, should have space.manage which doesn't apply at room level
		// but room.manage does apply at space and room levels
		err := core.GrantSpaceRolePermission(ctx, space.Id, SpaceRoleOwner, PermRoomManage)
		if err != nil {
			t.Fatalf("Failed to grant space permission: %v", err)
		}

		has, err := core.permissionResolver.HasRoomPermission(ctx, user.Id, space.Id, room.Id, PermRoomManage)
		if err != nil {
			t.Fatalf("HasRoomPermission() error = %v", err)
		}
		if !has {
			t.Error("Expected fallback to space level to work")
		}
	})
}

func TestPermissionResolver_HasRoomPermission_AdminRoleDenials(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user and space with room
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	space, _ := core.CreateSpace(ctx, user.Id, "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, user.Id, space.Id, "General", "General chat")

	t.Run("admin role is subject to room-level denials like any other role", func(t *testing.T) {
		// Deny permission at room level for admin role
		err := core.denyRoomRolePermissionInternal(ctx, space.Id, room.Id, SpaceRoleOwner, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to deny room permission: %v", err)
		}

		// User is space admin - should NOT have permission because admin role has no immunity
		has, err := core.permissionResolver.HasRoomPermission(ctx, user.Id, space.Id, room.Id, PermMessagePost)
		if err != nil {
			t.Fatalf("HasRoomPermission() error = %v", err)
		}
		if has {
			t.Error("Expected admin role denial to be enforced (admin has no special immunity)")
		}
	})

}

func TestPermissionResolver_HasRoomPermission_DenyWins(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create regular member (not admin) and space with room
	spaceAdmin, _ := core.CreateUser(ctx, "system", "spaceadmindenywins", "Admin User", "password123")
	space, _ := core.CreateSpace(ctx, spaceAdmin.Id, "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, spaceAdmin.Id, space.Id, "General", "General chat")

	// Create regular member
	member, _ := core.CreateUser(ctx, "system", "memberdenywins", "Member User", "password123")
	core.JoinSpace(ctx, member.Id, space.Id)

	t.Run("higher-ranked role denial wins at room level", func(t *testing.T) {
		// Grant permission to everyone at room level
		err := core.grantRoomRolePermissionInternal(ctx, space.Id, room.Id, SpaceRoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to grant room permission: %v", err)
		}

		// Create a "muted" role with explicit position LOWER than everyone (higher rank).
		// Position 100 is between moderator (2) and everyone (MaxInt32), so muted's
		// denial will be checked before everyone's grant in hierarchy order.
		_, err = core.CreateRoleWithPosition(ctx, spaceAdmin.Id, space.Id, "muted", "Muted", "Cannot post", 100)
		if err != nil {
			t.Fatalf("Failed to create muted role: %v", err)
		}
		err = core.denyRoomRolePermissionInternal(ctx, space.Id, room.Id, "muted", PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to deny room permission: %v", err)
		}

		// Assign muted role to member
		core.AssignRole(ctx, spaceAdmin.Id, space.Id, member.Id, "muted")

		// Member should NOT have permission (higher-ranked muted denial wins over everyone grant)
		has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, space.Id, room.Id, PermMessagePost)
		if err != nil {
			t.Fatalf("HasRoomPermission() error = %v", err)
		}
		if has {
			t.Error("Expected higher-ranked muted role denial to win over everyone grant")
		}
	})
}

// ============================================================================
// Room Override Scenario Tests
// ============================================================================

func TestPermissionResolver_HasRoomPermission_RoomGrantOverridesAbsentSpaceGrant(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "roomoverride1admin", "Admin", "password123")
	space, _ := core.CreateSpace(ctx, admin.Id, "Test Space", "")
	room, _ := core.CreateRoom(ctx, admin.Id, space.Id, "general", "General")

	member, _ := core.CreateUser(ctx, "system", "roomoverride1member", "Member", "password123")
	core.JoinSpace(ctx, member.Id, space.Id)

	// Revoke message.react from everyone at space level (no grant, no deny — just absent)
	core.ClearSpaceRolePermission(ctx, space.Id, SpaceRoleEveryone, PermMessageReact)

	// Verify member doesn't have permission at space level
	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, space.Id, room.Id, PermMessageReact)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("Expected member NOT to have message.react before room grant")
	}

	// Grant at room level
	err = core.grantRoomRolePermissionInternal(ctx, space.Id, room.Id, SpaceRoleEveryone, PermMessageReact)
	if err != nil {
		t.Fatalf("Failed to grant room permission: %v", err)
	}

	has, err = core.permissionResolver.HasRoomPermission(ctx, member.Id, space.Id, room.Id, PermMessageReact)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("Expected room grant to give member message.react")
	}
}

func TestPermissionResolver_HasRoomPermission_RoomDenialOverridesSpaceGrant(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "roomdeny1admin", "Admin", "password123")
	space, _ := core.CreateSpace(ctx, admin.Id, "Test Space", "")
	room, _ := core.CreateRoom(ctx, admin.Id, space.Id, "general", "General")

	member, _ := core.CreateUser(ctx, "system", "roomdeny1member", "Member", "password123")
	core.JoinSpace(ctx, member.Id, space.Id)

	// Ensure message.post is granted at space level
	core.GrantSpaceRolePermission(ctx, space.Id, SpaceRoleEveryone, PermMessagePost)

	// Deny at room level
	core.denyRoomRolePermissionInternal(ctx, space.Id, room.Id, SpaceRoleEveryone, PermMessagePost)

	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, space.Id, room.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("Expected room denial to block space grant")
	}
}

func TestPermissionResolver_HasRoomPermission_RoomGrantCannotOverrideSpaceDenial(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "roomcantoverride1admin", "Admin", "password123")
	space, _ := core.CreateSpace(ctx, admin.Id, "Test Space", "")
	room, _ := core.CreateRoom(ctx, admin.Id, space.Id, "general", "General")

	member, _ := core.CreateUser(ctx, "system", "roomcantoverride1member", "Member", "password123")
	core.JoinSpace(ctx, member.Id, space.Id)

	// Deny at space level
	core.DenySpaceRolePermission(ctx, space.Id, SpaceRoleEveryone, PermMessagePost)

	// Grant at room level
	core.grantRoomRolePermissionInternal(ctx, space.Id, room.Id, SpaceRoleEveryone, PermMessagePost)

	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, space.Id, room.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("Expected space denial to block room grant (deny always wins)")
	}
}

func TestPermissionResolver_HasRoomPermission_ConflictingRoles(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "conflictroleadmin", "Admin", "password123")
	space, _ := core.CreateSpace(ctx, admin.Id, "Test Space", "")
	room, _ := core.CreateRoom(ctx, admin.Id, space.Id, "general", "General")

	member, _ := core.CreateUser(ctx, "system", "conflictrolemember", "Member", "password123")
	core.JoinSpace(ctx, member.Id, space.Id)

	// Create a custom role (gets position 3, higher rank than everyone at MaxInt32)
	core.CreateRole(ctx, admin.Id, space.Id, "poster", "Poster", "Can post")

	// Grant message.post to poster role at room level
	core.grantRoomRolePermissionInternal(ctx, space.Id, room.Id, "poster", PermMessagePost)

	// Deny message.post for everyone role at room level
	core.denyRoomRolePermissionInternal(ctx, space.Id, room.Id, SpaceRoleEveryone, PermMessagePost)

	// Assign poster role to member (member now has: everyone + poster)
	core.AssignRole(ctx, admin.Id, space.Id, member.Id, "poster")

	// Room-level uses hierarchy-wins: poster (position 3, higher rank) grant beats
	// everyone (position MaxInt32, lower rank) deny. This enables patterns like
	// #announcements where higher-ranked roles can override lower-ranked denials.
	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, space.Id, room.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("Expected higher-ranked poster role grant to win over lower-ranked everyone denial at room level")
	}
}

func TestPermissionResolver_HasRoomPermission_IsolationBetweenRooms(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "roomisoadmin", "Admin", "password123")
	space, _ := core.CreateSpace(ctx, admin.Id, "Test Space", "")
	roomA, _ := core.CreateRoom(ctx, admin.Id, space.Id, "rooma", "Room A")
	roomB, _ := core.CreateRoom(ctx, admin.Id, space.Id, "roomb", "Room B")

	member, _ := core.CreateUser(ctx, "system", "roomisomember", "Member", "password123")
	core.JoinSpace(ctx, member.Id, space.Id)

	// Ensure message.post is granted at space level for everyone
	core.GrantSpaceRolePermission(ctx, space.Id, SpaceRoleEveryone, PermMessagePost)

	// Deny message.post only in room A
	core.denyRoomRolePermissionInternal(ctx, space.Id, roomA.Id, SpaceRoleEveryone, PermMessagePost)

	// Room A: denied
	hasA, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, space.Id, roomA.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasA {
		t.Error("Expected member to be denied in room A")
	}

	// Room B: allowed (no room override, falls back to space grant)
	hasB, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, space.Id, roomB.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasB {
		t.Error("Expected member to have permission in room B (no override)")
	}
}

func TestPermissionResolver_HasRoomPermission_InstanceRoleRoomDenial(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "instroomdeny1admin", "Admin", "password123")
	space, _ := core.CreateSpace(ctx, admin.Id, "Test Space", "")
	room, _ := core.CreateRoom(ctx, admin.Id, space.Id, "general", "General")

	member, _ := core.CreateUser(ctx, "system", "instroomdeny1member", "Member", "password123")
	core.JoinSpace(ctx, member.Id, space.Id)

	// Ensure message.post is granted at space level
	core.GrantSpaceRolePermission(ctx, space.Id, SpaceRoleEveryone, PermMessagePost)

	// Deny message.post for instance-everyone at room level
	core.denyRoomRolePermissionInternal(ctx, space.Id, room.Id, InstRoleEveryone, PermMessagePost)

	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, space.Id, room.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("Expected instance role room denial to block permission")
	}
}

func TestPermissionResolver_HasRoomPermission_InstanceRoleRoomGrant(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "instroomgrant1admin", "Admin", "password123")
	space, _ := core.CreateSpace(ctx, admin.Id, "Test Space", "")
	room, _ := core.CreateRoom(ctx, admin.Id, space.Id, "general", "General")

	member, _ := core.CreateUser(ctx, "system", "instroomgrant1member", "Member", "password123")
	core.JoinSpace(ctx, member.Id, space.Id)

	// Clear message.react from everyone at space level (no grant)
	core.ClearSpaceRolePermission(ctx, space.Id, SpaceRoleEveryone, PermMessageReact)

	// Grant message.react to instance-everyone at room level
	core.grantRoomRolePermissionInternal(ctx, space.Id, room.Id, InstRoleEveryone, PermMessageReact)

	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, space.Id, room.Id, PermMessageReact)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("Expected instance role room grant to give permission")
	}
}

func TestPermissionResolver_HasRoomPermission_ClearFallsBackToSpace(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "clearfallbackadmin", "Admin", "password123")
	space, _ := core.CreateSpace(ctx, admin.Id, "Test Space", "")
	room, _ := core.CreateRoom(ctx, admin.Id, space.Id, "general", "General")

	member, _ := core.CreateUser(ctx, "system", "clearfallbackmember", "Member", "password123")
	core.JoinSpace(ctx, member.Id, space.Id)

	// Grant at space level
	core.GrantSpaceRolePermission(ctx, space.Id, SpaceRoleEveryone, PermMessagePost)

	// Deny at room level
	core.denyRoomRolePermissionInternal(ctx, space.Id, room.Id, SpaceRoleEveryone, PermMessagePost)

	// Verify denied
	has, _ := core.permissionResolver.HasRoomPermission(ctx, member.Id, space.Id, room.Id, PermMessagePost)
	if has {
		t.Fatal("Setup error: expected room denial to block")
	}

	// Clear room override
	core.clearRoomRolePermissionInternal(ctx, space.Id, room.Id, SpaceRoleEveryone, PermMessagePost)

	// Should fall back to space grant
	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, space.Id, room.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("Expected clearing room override to fall back to space grant")
	}
}

func TestPermissionResolver_HasRoomPermission_MultiplePermissionsPerRoom(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "multipermadmin", "Admin", "password123")
	space, _ := core.CreateSpace(ctx, admin.Id, "Test Space", "")
	room, _ := core.CreateRoom(ctx, admin.Id, space.Id, "general", "General")

	member, _ := core.CreateUser(ctx, "system", "multipermmember", "Member", "password123")
	core.JoinSpace(ctx, member.Id, space.Id)

	// Grant message.post at room level, deny message.react at room level
	core.grantRoomRolePermissionInternal(ctx, space.Id, room.Id, SpaceRoleEveryone, PermMessagePost)
	core.denyRoomRolePermissionInternal(ctx, space.Id, room.Id, SpaceRoleEveryone, PermMessageReact)

	hasPost, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, space.Id, room.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasPost {
		t.Error("Expected message.post to be granted at room level")
	}

	hasReact, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, space.Id, room.Id, PermMessageReact)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasReact {
		t.Error("Expected message.react to be denied at room level")
	}
}

// ============================================================================
// Deny-Always-Wins Tests
// ============================================================================

func TestPermissionResolver_DenyAlwaysWins(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create admin and regular member
	admin, _ := core.CreateUser(ctx, "system", "hieradmin", "Admin User", "password123")
	space, _ := core.CreateSpace(ctx, admin.Id, "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, admin.Id, space.Id, "General", "General chat")

	member, _ := core.CreateUser(ctx, "system", "hiermember", "Member User", "password123")
	core.JoinSpace(ctx, member.Id, space.Id)

	t.Run("space deny blocks room grant", func(t *testing.T) {
		// Deny at space level
		err := core.DenySpaceRolePermission(ctx, space.Id, SpaceRoleEveryone, PermMessageReact)
		if err != nil {
			t.Fatalf("Failed to deny space permission: %v", err)
		}

		// Grant at room level
		err = core.grantRoomRolePermissionInternal(ctx, space.Id, room.Id, SpaceRoleEveryone, PermMessageReact)
		if err != nil {
			t.Fatalf("Failed to grant room permission: %v", err)
		}

		// Deny always wins: space deny blocks room grant
		has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, space.Id, room.Id, PermMessageReact)
		if err != nil {
			t.Fatalf("HasRoomPermission() error = %v", err)
		}
		if has {
			t.Error("Expected space deny to block room grant (deny always wins)")
		}
	})

	t.Run("instance deny blocks space grant", func(t *testing.T) {
		// Deny at instance level for instance-everyone
		err := core.DenyInstanceRolePermission(ctx, InstRoleEveryone, PermMessageEditOwn)
		if err != nil {
			t.Fatalf("Failed to deny instance permission: %v", err)
		}

		// Grant at space level
		err = core.GrantSpaceRolePermission(ctx, space.Id, SpaceRoleEveryone, PermMessageEditOwn)
		if err != nil {
			t.Fatalf("Failed to grant space permission: %v", err)
		}

		// Deny always wins: instance deny blocks space grant
		has, err := core.permissionResolver.HasSpacePermission(ctx, member.Id, space.Id, PermMessageEditOwn)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if has {
			t.Error("Expected instance deny to block space grant (deny always wins)")
		}
	})

	t.Run("instance deny blocks room grant", func(t *testing.T) {
		// Deny at instance level for instance-everyone
		err := core.DenyInstanceRolePermission(ctx, InstRoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to deny instance permission: %v", err)
		}

		// Grant at room level
		err = core.grantRoomRolePermissionInternal(ctx, space.Id, room.Id, SpaceRoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to grant room permission: %v", err)
		}

		// Deny always wins: instance deny blocks room grant
		has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, space.Id, room.Id, PermMessagePost)
		if err != nil {
			t.Fatalf("HasRoomPermission() error = %v", err)
		}
		if has {
			t.Error("Expected instance deny to block room grant (deny always wins)")
		}
	})

	t.Run("space deny blocks instance grant", func(t *testing.T) {
		// Grant at instance level for instance-everyone
		err := core.GrantInstanceRolePermission(ctx, InstRoleEveryone, PermSpaceJoin)
		if err != nil {
			t.Fatalf("Failed to grant instance permission: %v", err)
		}

		// Deny at space level via space config for instance-everyone
		err = core.DenySpaceRolePermission(ctx, space.Id, InstRoleEveryone, PermSpaceJoin)
		if err != nil {
			t.Fatalf("Failed to deny space permission: %v", err)
		}

		// Create a non-member user to test space.join
		newUser, _ := core.CreateUser(ctx, "system", "newuser", "New User", "password123")

		// Deny always wins: space deny blocks instance grant
		has, err := core.permissionResolver.HasSpacePermission(ctx, newUser.Id, space.Id, PermSpaceJoin)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if has {
			t.Error("Expected space deny to block instance grant (deny always wins)")
		}
	})
}

// ============================================================================
// Instance Authority Tests
// ============================================================================

func TestPermissionResolver_InstanceAuthority(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "authadmin", "Admin User", "password123")
	space, _ := core.CreateSpace(ctx, admin.Id, "Test Space", "A test space")

	member, _ := core.CreateUser(ctx, "system", "authmember", "Member User", "password123")
	core.JoinSpace(ctx, member.Id, space.Id)

	t.Run("instance grant applies for space member", func(t *testing.T) {
		// Grant at instance level for instance-everyone
		err := core.GrantInstanceRolePermission(ctx, InstRoleEveryone, PermSpaceJoin)
		if err != nil {
			t.Fatalf("Failed to grant instance permission: %v", err)
		}

		// Instance grant should apply (no space-level decision)
		has, err := core.permissionResolver.HasSpacePermission(ctx, member.Id, space.Id, PermSpaceJoin)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if !has {
			t.Error("Expected instance grant to apply for space member")
		}
	})
}

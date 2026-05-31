package core

import (
	"testing"
)

// ============================================================================
// HasServerPermission Tests
// ============================================================================

func TestPermissionResolver_HasServerPermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")

	t.Run("returns true when user has user.delete-self via everyone role", func(t *testing.T) {
		has, err := core.permissionResolver.HasServerPermission(ctx, user.Id, PermUserDeleteSelf)
		if err != nil {
			t.Fatalf("HasServerPermission() error = %v", err)
		}
		if !has {
			t.Error("Expected user to have user.delete-self via everyone role")
		}
	})

	t.Run("returns true when user has message.post via everyone role", func(t *testing.T) {
		has, err := core.permissionResolver.HasServerPermission(ctx, user.Id, PermMessagePost)
		if err != nil {
			t.Fatalf("HasServerPermission() error = %v", err)
		}
		if !has {
			t.Error("Expected user to have message.post via everyone role")
		}
	})

	t.Run("returns false when user lacks permission", func(t *testing.T) {
		// Regular user doesn't have admin.access
		has, err := core.permissionResolver.HasServerPermission(ctx, user.Id, PermAdminAccess)
		if err != nil {
			t.Fatalf("HasServerPermission() error = %v", err)
		}
		if has {
			t.Error("Expected user NOT to have admin.access")
		}
	})

}

func TestPermissionResolver_HasServerPermission_DenyWins(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")

	t.Run("same-role denial replaces grant", func(t *testing.T) {
		// Grant permission via instance-everyone role
		err := core.GrantServerPermission(ctx, RoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to grant permission: %v", err)
		}

		// Deny same permission for the same role (replaces the grant)
		err = core.DenyServerPermission(ctx, RoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to deny permission: %v", err)
		}

		// User should NOT have the permission (denial replaced grant)
		has, err := core.permissionResolver.HasServerPermission(ctx, user.Id, PermMessagePost)
		if err != nil {
			t.Fatalf("HasServerPermission() error = %v", err)
		}
		if has {
			t.Error("Expected denial to replace grant")
		}

		// Restore for other tests
		core.GrantServerPermission(ctx, RoleEveryone, PermMessagePost)
	})
}

func TestPermissionResolver_HasServerPermission_CustomDenyRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user (has everyone role)
	user, _ := core.CreateUser(ctx, "system", "testuser-denyrole", "Test User", "password123")

	// Verify user initially has message.post (via everyone role default)
	has, err := core.permissionResolver.HasServerPermission(ctx, user.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("HasServerPermission() error = %v", err)
	}
	if !has {
		t.Fatal("Expected user to have message.post initially via everyone role")
	}

	// Create a custom deny role (replicates the e2e test scenario)
	denyRole, err := core.CreateServerRole(ctx, "denytest", "Deny message.post", "Test deny role")
	if err != nil {
		t.Fatalf("Failed to create deny role: %v", err)
	}
	t.Logf("Created deny role with position: %d", denyRole.Position)

	// Deny message.post on the deny role
	err = core.DenyServerPermission(ctx, "denytest", PermMessagePost)
	if err != nil {
		t.Fatalf("Failed to deny permission: %v", err)
	}

	// Assign deny role to user
	err = core.AssignServerRole(ctx, SystemActorID, user.Id, "denytest")
	if err != nil {
		t.Fatalf("Failed to assign deny role: %v", err)
	}

	// User now has: denytest (deny message.post), everyone (grant message.post).
	// The custom deny role outranks everyone, so its deny should win.
	has, err = core.permissionResolver.HasServerPermission(ctx, user.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("HasServerPermission() error = %v", err)
	}
	if has {
		t.Error("Expected custom deny role to block message.post despite everyone granting it")
	}

	// Also verify GetUserServerPermissions (the old path) agrees
	perms, err := core.GetUserServerPermissions(ctx, user.Id)
	if err != nil {
		t.Fatalf("GetUserServerPermissions() error = %v", err)
	}
	for _, p := range perms {
		if p == PermMessagePost {
			t.Error("Expected message.post to NOT be in GetUserServerPermissions result")
			break
		}
	}
}

func TestPermissionResolver_HasServerPermission_Hierarchy(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user and assign admin role
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	_ = core.AssignServerRole(ctx, SystemActorID, user.Id, RoleAdmin)

	t.Run("higher-ranked role grant beats lower-ranked role denial", func(t *testing.T) {
		// Deny message.post for everyone (low rank, position 0)
		err := core.DenyServerPermission(ctx, RoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to deny permission: %v", err)
		}

		// Grant message.post for admin (high rank, position 900)
		err = core.GrantServerPermission(ctx, RoleAdmin, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to grant permission: %v", err)
		}

		// User has both admin (grant) and everyone (deny) roles.
		// Admin is higher rank (position 900 > 0), so admin's grant should win.
		has, err := core.permissionResolver.HasServerPermission(ctx, user.Id, PermMessagePost)
		if err != nil {
			t.Fatalf("HasServerPermission() error = %v", err)
		}
		if !has {
			t.Error("Expected higher-ranked role grant to win over lower-ranked role denial")
		}

		// Cleanup
		core.ClearServerPermissionState(ctx, RoleEveryone, PermMessagePost)
		core.ClearServerPermissionState(ctx, RoleAdmin, PermMessagePost)
	})

	t.Run("higher-ranked role denial beats lower-ranked role grant", func(t *testing.T) {
		// Grant message.post for everyone (low rank)
		err := core.GrantServerPermission(ctx, RoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to grant permission: %v", err)
		}

		// Deny message.post for admin (high rank)
		err = core.DenyServerPermission(ctx, RoleAdmin, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to deny permission: %v", err)
		}

		// Admin denial (position 900) should be checked before everyone grant (position 0).
		has, err := core.permissionResolver.HasServerPermission(ctx, user.Id, PermMessagePost)
		if err != nil {
			t.Fatalf("HasServerPermission() error = %v", err)
		}
		if has {
			t.Error("Expected higher-ranked role denial to win over lower-ranked role grant")
		}

		// Cleanup
		core.ClearServerPermissionState(ctx, RoleEveryone, PermMessagePost)
		core.ClearServerPermissionState(ctx, RoleAdmin, PermMessagePost)
	})

}

// ============================================================================
// HasSpacePermission Tests
// ============================================================================

func TestPermissionResolver_HasSpacePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user and assign owner role (formerly via CreateSpace).
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	if err := core.AssignServerRole(ctx, SystemActorID, user.Id, RoleOwner); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}

	t.Run("returns true when user has permission via space role", func(t *testing.T) {
		// Space admin gets space.manage
		has, err := core.permissionResolver.HasSpacePermission(ctx, user.Id, KindChannel, PermServerManage)
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
		has, err := core.permissionResolver.HasSpacePermission(ctx, otherUser.Id, KindChannel, PermServerManage)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if has {
			t.Error("Expected non-member NOT to have space.manage")
		}
	})

	// "instance-only permission returns false at space level" was a dual-tier
	// assertion that no longer applies: post-Phase-5 there's only one tier, so
	// an admin-permission grant on a server role propagates everywhere.
}

func TestPermissionResolver_HasSpacePermission_ServerFallback(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user and assign owner role (formerly via CreateSpace).
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	if err := core.AssignServerRole(ctx, SystemActorID, user.Id, RoleOwner); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}

	t.Run("space member gets space-scoped permissions from space roles", func(t *testing.T) {
		// User is a space member (creator) with owner role
		// room.create is granted via space's everyone role defaults
		has, err := core.permissionResolver.HasSpacePermission(ctx, user.Id, KindChannel, PermRoomCreate)
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
		has, err := core.permissionResolver.HasSpacePermission(ctx, nonMember.Id, KindChannel, PermRoomCreate)
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
		has, err := core.permissionResolver.HasSpacePermission(ctx, nonMember.Id, KindChannel, PermMessagePost)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if !has {
			t.Error("Expected non-member to have space.join (special exception)")
		}
	})
}

func TestPermissionResolver_ExplicitDenyOnHighestRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Use an admin user. An owner deny would be just as effective now
	// that the bypass short-circuit is gone, but admin makes the test
	// less dependent on owner-role role-management quirks.
	user, _ := core.CreateUser(ctx, "system", "deny-on-highest", "Test User", "password123")
	if err := core.AssignServerRole(ctx, SystemActorID, user.Id, RoleAdmin); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}

	t.Run("explicit deny on highest-rank role wins over lower-rank grant", func(t *testing.T) {
		// `everyone` grants the perm; `admin` (the user's highest role) denies it.
		// Walker visits admin first → deny → stop. Result: denied.
		if err := core.GrantServerPermission(ctx, RoleEveryone, PermMessagePost); err != nil {
			t.Fatalf("Failed to grant permission: %v", err)
		}
		if err := core.DenyServerPermission(ctx, RoleAdmin, PermMessagePost); err != nil {
			t.Fatalf("Failed to deny permission: %v", err)
		}

		has, err := core.permissionResolver.HasSpacePermission(ctx, user.Id, KindChannel, PermMessagePost)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if has {
			t.Error("Expected admin deny to beat everyone grant under hierarchy-wins")
		}
	})
}

func TestPermissionResolver_HasSpacePermission_ServerRoleOverride(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user and space
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")

	t.Run("space can override role permissions", func(t *testing.T) {
		// Grant permission to instance-everyone at space level (override)
		err := core.GrantServerPermission(ctx, RoleEveryone, PermRoomManage)
		if err != nil {
			t.Fatalf("Failed to grant permission: %v", err)
		}

		// User should have the permission via the space-level override
		has, err := core.permissionResolver.HasSpacePermission(ctx, user.Id, KindChannel, PermRoomManage)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if !has {
			t.Error("Expected space-level override for role to work")
		}
	})
}

func TestPermissionResolver_HasSpacePermission_DMKind(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")

	t.Run("DM rooms allow message.post", func(t *testing.T) {
		has, err := core.permissionResolver.HasSpacePermission(ctx, user.Id, KindDM, PermMessagePost)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if !has {
			t.Error("Expected DM rooms to allow message.post")
		}
	})

	t.Run("DM rooms deny server.manage", func(t *testing.T) {
		has, err := core.permissionResolver.HasSpacePermission(ctx, user.Id, KindDM, PermServerManage)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if has {
			t.Error("Expected DM rooms NOT to allow server.manage")
		}
	})

	t.Run("DM rooms allow room.join", func(t *testing.T) {
		has, err := core.permissionResolver.HasSpacePermission(ctx, user.Id, KindDM, PermRoomJoin)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if !has {
			t.Error("Expected DM rooms to allow room.join")
		}
	})
}

// ============================================================================
// HasRoomPermission Tests
// ============================================================================

func TestPermissionResolver_HasRoomPermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user and assign owner role (formerly via CreateSpace).
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	if err := core.AssignServerRole(ctx, SystemActorID, user.Id, RoleOwner); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	room, _ := core.CreateRoom(ctx, user.Id, KindChannel, "", "General", "General chat")

	t.Run("returns true when user has permission at room level", func(t *testing.T) {
		// Grant permission at room level
		err := core.GrantRoomPermission(ctx, room.Id, RoleOwner, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to grant room permission: %v", err)
		}

		has, err := core.permissionResolver.HasRoomPermission(ctx, user.Id, KindChannel, room.Id, PermMessagePost)
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
		err := core.GrantServerPermission(ctx, RoleOwner, PermRoomManage)
		if err != nil {
			t.Fatalf("Failed to grant space permission: %v", err)
		}

		has, err := core.permissionResolver.HasRoomPermission(ctx, user.Id, KindChannel, room.Id, PermRoomManage)
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

	// Admin role here; the test's point is "no role has structural
	// immunity to a room-level deny." After the bypass primitive was
	// removed, the same claim holds for owner too.
	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	if err := core.AssignServerRole(ctx, SystemActorID, user.Id, RoleAdmin); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	room, _ := core.CreateRoom(ctx, user.Id, KindChannel, "", "General", "General chat")

	t.Run("admin role is subject to room-level denials like any other role", func(t *testing.T) {
		if err := core.DenyRoomPermission(ctx, room.Id, RoleAdmin, PermMessagePost); err != nil {
			t.Fatalf("Failed to deny room permission: %v", err)
		}

		has, err := core.permissionResolver.HasRoomPermission(ctx, user.Id, KindChannel, room.Id, PermMessagePost)
		if err != nil {
			t.Fatalf("HasRoomPermission() error = %v", err)
		}
		if has {
			t.Error("Expected admin role denial to be enforced (admin has no special immunity without bypass)")
		}
	})

}

func TestPermissionResolver_HasRoomPermission_DenyWins(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create space owner and room
	spaceAdmin, _ := core.CreateUser(ctx, "system", "spaceadmindenywins", "Admin User", "password123")
	if err := core.AssignServerRole(ctx, SystemActorID, spaceAdmin.Id, RoleOwner); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	room, _ := core.CreateRoom(ctx, spaceAdmin.Id, KindChannel, "", "General", "General chat")

	// Create regular member
	member, _ := core.CreateUser(ctx, "system", "memberdenywins", "Member User", "password123")
	t.Run("higher-ranked role denial wins at room level", func(t *testing.T) {
		// Grant permission to everyone at room level
		err := core.GrantRoomPermission(ctx, room.Id, RoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to grant room permission: %v", err)
		}

		// Create a "muted" role, which ranks above the implicit everyone role.
		_, err = core.CreateServerRole(ctx, "muted", "Muted", "Cannot post")
		if err != nil {
			t.Fatalf("Failed to create muted role: %v", err)
		}
		err = core.DenyRoomPermission(ctx, room.Id, "muted", PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to deny room permission: %v", err)
		}

		// Assign muted role to member
		core.AssignServerRole(ctx, spaceAdmin.Id, member.Id, "muted")

		// Member should NOT have permission (higher-ranked muted denial wins over everyone grant)
		has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost)
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

func TestPermissionResolver_HasRoomPermission_RoomGrantOverridesAbsentSetGrant(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "roomoverride1admin", "Admin", "password123")
	room, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "general", "General")

	member, _ := core.CreateUser(ctx, "system", "roomoverride1member", "Member", "password123")

	// Clear the group-scope AND server-scope grants for message.react so
	// member starts with no permission at any scope, then verify a per-room
	// override grants it.
	groups, _ := core.ListRoomGroupsOrdered(ctx, KindChannel)
	groupID := groups[0].Id
	if err := core.ClearGroupPermissionState(ctx, groupID, RoleEveryone, PermMessageReact); err != nil {
		t.Fatalf("ClearGroupPermissionState: %v", err)
	}
	if err := core.ClearServerPermissionState(ctx, RoleEveryone, PermMessageReact); err != nil {
		t.Fatalf("ClearServerPermissionState: %v", err)
	}

	// Verify member doesn't have permission with no set grant
	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessageReact)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("Expected member NOT to have message.react before room grant")
	}

	// Grant at room level
	err = core.GrantRoomPermission(ctx, room.Id, RoleEveryone, PermMessageReact)
	if err != nil {
		t.Fatalf("Failed to grant room permission: %v", err)
	}

	has, err = core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessageReact)
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
	room, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "general", "General")

	member, _ := core.CreateUser(ctx, "system", "roomdeny1member", "Member", "password123")
	// Ensure message.post is granted at space level
	core.GrantServerPermission(ctx, RoleEveryone, PermMessagePost)

	// Deny at room level
	core.DenyRoomPermission(ctx, room.Id, RoleEveryone, PermMessagePost)

	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("Expected room denial to block space grant")
	}
}

func TestPermissionResolver_HasRoomPermission_RoomGrantOverridesServerDenialForSameRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "roomoverrideadmin", "Admin", "password123")
	room, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "general", "General")

	member, _ := core.CreateUser(ctx, "system", "roomoverridemember", "Member", "password123")
	core.DenyServerPermission(ctx, RoleEveryone, PermMessagePost)
	core.GrantRoomPermission(ctx, room.Id, RoleEveryone, PermMessagePost)

	// Under the unified hierarchy-wins algorithm, the room-level decision
	// for a role takes precedence over that same role's server-level decision.
	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("expected room grant to override server deny for the same role")
	}
}

func TestPermissionResolver_HasRoomPermission_ConflictingRoles(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "conflictroleadmin", "Admin", "password123")
	if err := core.AssignServerRole(ctx, SystemActorID, admin.Id, RoleOwner); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	room, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "general", "General")

	member, _ := core.CreateUser(ctx, "system", "conflictrolemember", "Member", "password123")
	// Create a custom role (gets a positive custom position, higher rank than everyone).
	core.CreateServerRole(ctx, "poster", "Poster", "Can post")

	// Grant message.post to poster role at room level
	core.GrantRoomPermission(ctx, room.Id, "poster", PermMessagePost)

	// Deny message.post for everyone role at room level
	core.DenyRoomPermission(ctx, room.Id, RoleEveryone, PermMessagePost)

	// Assign poster role to member (member now has: everyone + poster)
	core.AssignServerRole(ctx, admin.Id, member.Id, "poster")

	// Room-level uses hierarchy-wins: poster (higher rank) grant beats
	// everyone (position 0, lower rank) deny. This enables patterns like
	// #announcements where higher-ranked roles can override lower-ranked denials.
	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost)
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
	roomA, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "rooma", "Room A")
	roomB, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "roomb", "Room B")

	member, _ := core.CreateUser(ctx, "system", "roomisomember", "Member", "password123")
	// Ensure message.post is granted at space level for everyone
	core.GrantServerPermission(ctx, RoleEveryone, PermMessagePost)

	// Deny message.post only in room A
	core.DenyRoomPermission(ctx, roomA.Id, RoleEveryone, PermMessagePost)

	// Room A: denied
	hasA, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, roomA.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasA {
		t.Error("Expected member to be denied in room A")
	}

	// Room B: allowed (no room override, falls back to space grant)
	hasB, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, roomB.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasB {
		t.Error("Expected member to have permission in room B (no override)")
	}
}

func TestPermissionResolver_HasRoomPermission_ServerRoleRoomDenial(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "instroomdeny1admin", "Admin", "password123")
	room, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "general", "General")

	member, _ := core.CreateUser(ctx, "system", "instroomdeny1member", "Member", "password123")
	// Ensure message.post is granted at space level
	core.GrantServerPermission(ctx, RoleEveryone, PermMessagePost)

	// Deny message.post for instance-everyone at room level
	core.DenyRoomPermission(ctx, room.Id, RoleEveryone, PermMessagePost)

	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if has {
		t.Error("Expected role room denial to block permission")
	}
}

func TestPermissionResolver_HasRoomPermission_ServerRoleRoomGrant(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "instroomgrant1admin", "Admin", "password123")
	room, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "general", "General")

	member, _ := core.CreateUser(ctx, "system", "instroomgrant1member", "Member", "password123")
	// Clear message.react from everyone at space level (no grant)
	core.ClearServerPermissionState(ctx, RoleEveryone, PermMessageReact)

	// Grant message.react to instance-everyone at room level
	core.GrantRoomPermission(ctx, room.Id, RoleEveryone, PermMessageReact)

	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessageReact)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !has {
		t.Error("Expected role room grant to give permission")
	}
}

func TestPermissionResolver_HasRoomPermission_ClearFallsBackToSpace(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "clearfallbackadmin", "Admin", "password123")
	room, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "general", "General")

	member, _ := core.CreateUser(ctx, "system", "clearfallbackmember", "Member", "password123")
	// Grant at space level
	core.GrantServerPermission(ctx, RoleEveryone, PermMessagePost)

	// Deny at room level
	core.DenyRoomPermission(ctx, room.Id, RoleEveryone, PermMessagePost)

	// Verify denied
	has, _ := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost)
	if has {
		t.Fatal("Setup error: expected room denial to block")
	}

	// Clear room override
	core.ClearRoomPermissionState(ctx, room.Id, RoleEveryone, PermMessagePost)

	// Should fall back to space grant
	has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost)
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
	room, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "general", "General")

	member, _ := core.CreateUser(ctx, "system", "multipermmember", "Member", "password123")
	// Grant message.post at room level, deny message.react at room level
	core.GrantRoomPermission(ctx, room.Id, RoleEveryone, PermMessagePost)
	core.DenyRoomPermission(ctx, room.Id, RoleEveryone, PermMessageReact)

	hasPost, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !hasPost {
		t.Error("Expected message.post to be granted at room level")
	}

	hasReact, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessageReact)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hasReact {
		t.Error("Expected message.react to be denied at room level")
	}
}

// ============================================================================
// Per-User Override Contract — user-level grants/denies beat role decisions
// ============================================================================

func TestPermissionResolver_UserLevelOverrides(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("user-level deny suspends a user despite role grants", func(t *testing.T) {
		// The classic suspension use case: deny a perm directly to this
		// user, and no role they have can re-grant it.
		mod, _ := core.CreateUser(ctx, SystemActorID, "user-deny-mod", "Mod", "password123")
		if err := core.AssignServerRole(ctx, SystemActorID, mod.Id, RoleModerator); err != nil {
			t.Fatalf("AssignServerRole: %v", err)
		}
		// Moderator has message.post via default everyone-grants; verify baseline.
		has, _ := core.HasServerPermission(ctx, mod.Id, PermMessagePost)
		if !has {
			t.Fatal("baseline: moderator should have message.post")
		}
		// Suspend posting by user-deny.
		if err := core.DenyUserPermission(ctx, mod.Id, PermMessagePost); err != nil {
			t.Fatalf("DenyUserPermission: %v", err)
		}
		has, _ = core.HasServerPermission(ctx, mod.Id, PermMessagePost)
		if has {
			t.Error("expected user-deny to suspend the moderator's message.post")
		}
	})

	t.Run("user-level deny beats role grants on owner", func(t *testing.T) {
		// Owner is just a role with every server-scope permission granted;
		// a user-level deny for a specific permission still suspends it.
		// This is the "ban one owner from posting" extreme case.
		owner, _ := core.CreateUser(ctx, SystemActorID, "user-deny-owner", "Owner", "password123")
		if err := core.AssignOwnerRole(ctx, owner.Id); err != nil {
			t.Fatalf("AssignOwnerRole: %v", err)
		}
		if err := core.DenyUserPermission(ctx, owner.Id, PermMessagePost); err != nil {
			t.Fatalf("DenyUserPermission: %v", err)
		}
		has, _ := core.HasServerPermission(ctx, owner.Id, PermMessagePost)
		if has {
			t.Error("expected user-deny to override owner-role grant for message.post")
		}
	})

	t.Run("user-level grant gives a single user a permission no role grants them", func(t *testing.T) {
		// The classic "give this one user admin powers on room X without
		// inventing a role" use case.
		user, _ := core.CreateUser(ctx, SystemActorID, "user-grant-bob", "Bob", "password123")
		room, _ := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "general", "General")

		// Without a grant, bob can't delete-any in this room.
		has, _ := core.permissionResolver.HasRoomPermission(ctx, user.Id, KindChannel, room.Id, PermMessageManage)
		if has {
			t.Fatal("baseline: bob should not have delete-any")
		}

		// Grant directly on the user, at room scope.
		if err := core.GrantUserRoomPermission(ctx, room.Id, user.Id, PermMessageManage); err != nil {
			t.Fatalf("GrantUserRoomPermission: %v", err)
		}
		has, _ = core.permissionResolver.HasRoomPermission(ctx, user.Id, KindChannel, room.Id, PermMessageManage)
		if !has {
			t.Error("expected user-level room grant to give bob delete-any in this room")
		}

		// Other rooms unaffected.
		other, _ := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "other", "Other")
		has, _ = core.permissionResolver.HasRoomPermission(ctx, user.Id, KindChannel, other.Id, PermMessageManage)
		if has {
			t.Error("user-level room grant should not leak to other rooms")
		}
	})

	t.Run("user-level room grant beats role set-level deny for the same user", func(t *testing.T) {
		// Operator denies posting on the room's set via the everyone role,
		// then re-enables it for one specific user in one specific room.
		user, _ := core.CreateUser(ctx, SystemActorID, "user-room-grant", "User", "password123")
		room, _ := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "private", "Private")
		groupID := room.GroupId
		if err := core.DenyGroupPermission(ctx, groupID, RoleEveryone, PermMessagePost); err != nil {
			t.Fatalf("DenyGroupPermission: %v", err)
		}
		// Without the user-grant, user can't post.
		has, _ := core.permissionResolver.HasRoomPermission(ctx, user.Id, KindChannel, room.Id, PermMessagePost)
		if has {
			t.Fatal("baseline: user should be denied by everyone-role set deny")
		}
		// User-level room grant.
		if err := core.GrantUserRoomPermission(ctx, room.Id, user.Id, PermMessagePost); err != nil {
			t.Fatalf("GrantUserRoomPermission: %v", err)
		}
		has, _ = core.permissionResolver.HasRoomPermission(ctx, user.Id, KindChannel, room.Id, PermMessagePost)
		if !has {
			t.Error("expected user-level room grant to override everyone-role set deny")
		}
	})

	t.Run("DM boundary deny beats user-level room grant", func(t *testing.T) {
		// Security invariant: the DM boundary deny-list is checked BEFORE
		// user-level overrides. Even an explicit user-level grant of
		// message.delete-any in a DM room must not allow it — DM privacy
		// is non-negotiable.
		c, _ := setupTestCore(t)
		ctx2 := testContext(t)
		user, _ := c.CreateUser(ctx2, SystemActorID, "dm-boundary-user", "User", "password123")
		dmRoomID := "R_dm_boundary_user_test"
		if err := c.GrantUserRoomPermission(ctx2, dmRoomID, user.Id, PermMessageManage); err != nil {
			t.Fatalf("GrantUserRoomPermission: %v", err)
		}
		has, _ := c.permissionResolver.HasRoomPermission(ctx2, user.Id, KindDM, dmRoomID, PermMessageManage)
		if has {
			t.Error("expected DM boundary deny to override user-level grant for message.delete-any")
		}
	})

	t.Run("DM boundary deny applies to owner too", func(t *testing.T) {
		// Owner has every server-scope permission via enumerated grants —
		// the boundary deny-list must still block DM moderation. The
		// boundary check runs before Phase 1 (user-level) and Phase 2
		// (role walk), so no role can sidestep it.
		c, _ := setupTestCore(t)
		ctx2 := testContext(t)
		owner, _ := c.CreateUser(ctx2, SystemActorID, "dm-boundary-owner", "Owner", "password123")
		if err := c.AssignOwnerRole(ctx2, owner.Id); err != nil {
			t.Fatalf("AssignOwnerRole: %v", err)
		}
		// Sanity: owner has the perms via the owner-role grants.
		has, _ := c.HasServerPermission(ctx2, owner.Id, PermMessagePost)
		if !has {
			t.Fatal("baseline: owner should resolve allow for message.post via owner-role grant")
		}
		// In DM context, the boundary deny-list still blocks.
		dmRoomID := "R_dm_boundary_owner_test"
		for _, perm := range []Permission{PermMessageManage, PermRoomManage} {
			has, _ := c.permissionResolver.HasRoomPermission(ctx2, owner.Id, KindDM, dmRoomID, perm)
			if has {
				t.Errorf("expected DM boundary to block %s for owner, got allow", perm)
			}
		}
	})

	t.Run("clear restores normal role-based resolution", func(t *testing.T) {
		// Use a fresh core so prior subtests' state can't contaminate this one.
		c, _ := setupTestCore(t)
		c2ctx := testContext(t)
		user, _ := c.CreateUser(c2ctx, SystemActorID, "clear-user", "User", "password123")
		has, _ := c.HasServerPermission(c2ctx, user.Id, PermMessagePost)
		if !has {
			t.Fatal("baseline: user should have message.post via everyone")
		}
		_ = c.DenyUserPermission(c2ctx, user.Id, PermMessagePost)
		has, _ = c.HasServerPermission(c2ctx, user.Id, PermMessagePost)
		if has {
			t.Fatal("expected user-deny to take effect")
		}
		if err := c.ClearUserPermissionState(c2ctx, user.Id, PermMessagePost); err != nil {
			t.Fatalf("ClearUserPermissionState: %v", err)
		}
		has, _ = c.HasServerPermission(c2ctx, user.Id, PermMessagePost)
		if !has {
			t.Error("expected clear to restore default-allow")
		}
	})
}

// ============================================================================
// DM Permission Contract — locks down what the unified walker resolves
// in a DM room for a regular participant and for elevated roles. The DM
// boundary deny-list is the security boundary; everything else flows
// through normal RBAC.
// ============================================================================

func TestPermissionResolver_DMContract(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	regular, _ := core.CreateUser(ctx, "system", "dmcontract-regular", "Regular", "password123")
	moderator, _ := core.CreateUser(ctx, "system", "dmcontract-mod", "Moderator", "password123")
	if err := core.AssignServerRole(ctx, SystemActorID, moderator.Id, RoleModerator); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}

	// Synthetic DM room ID — the walker doesn't care about room existence,
	// only about whether room-scope KV entries exist for it (we set none).
	dmRoomID := "R_dm_contract_test"

	// Each row encodes the expected resolution for the given persona at
	// room scope in a DM. Asserts the new contract — change requires a
	// deliberate review.
	type expected struct {
		regular   bool
		moderator bool
	}
	cases := []struct {
		perm Permission
		want expected
		why  string
	}{
		// === Boundary-denied (privacy + category mismatch) ===
		{PermRoomManage, expected{false, false}, "DM rooms can't be managed channel-style"},
		{PermMessageManage, expected{false, false}, "DM privacy: no cross-user moderation"},
		{PermMessageEcho, expected{false, false}, "echo channel-only"},
		{PermRoomCreate, expected{false, false}, "DMs use FindOrCreateDM"},

		// === Resolvable, default-granted to everyone === (so regular passes)
		{PermRoomJoin, expected{true, true}, "auto-join on DM creation; perm resolves"},
		{PermMessagePost, expected{true, true}, "core DM capability"},
		{PermMessagePostInThread, expected{true, true}, "core DM capability"},
		{PermMessageReact, expected{true, true}, "core DM capability"},
	}

	for _, tc := range cases {
		t.Run(string(tc.perm), func(t *testing.T) {
			gotRegular, err := core.permissionResolver.HasRoomPermission(ctx, regular.Id, KindDM, dmRoomID, tc.perm)
			if err != nil {
				t.Fatalf("regular HasRoomPermission: %v", err)
			}
			if gotRegular != tc.want.regular {
				t.Errorf("regular: HasRoomPermission(%s) = %v, want %v (%s)", tc.perm, gotRegular, tc.want.regular, tc.why)
			}
			gotMod, err := core.permissionResolver.HasRoomPermission(ctx, moderator.Id, KindDM, dmRoomID, tc.perm)
			if err != nil {
				t.Fatalf("moderator HasRoomPermission: %v", err)
			}
			if gotMod != tc.want.moderator {
				t.Errorf("moderator: HasRoomPermission(%s) = %v, want %v (%s)", tc.perm, gotMod, tc.want.moderator, tc.why)
			}
		})
	}
}

// ============================================================================
// Hierarchy-Wins Tests — room overrides take precedence over server defaults
// within the same role; higher-ranked roles beat lower-ranked ones across
// roles. See PermissionResolver's doc comment.
// ============================================================================

func TestPermissionResolver_RoomOverridesServerForSameRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	admin, _ := core.CreateUser(ctx, "system", "hieradmin", "Admin User", "password123")
	room, _ := core.CreateRoom(ctx, admin.Id, KindChannel, "", "General", "General chat")

	member, _ := core.CreateUser(ctx, "system", "hiermember", "Member User", "password123")

	t.Run("room grant overrides server deny on the same role", func(t *testing.T) {
		// Server-wide deny on everyone, room-level grant on everyone.
		// Under hierarchy-wins, the room-level decision wins for the same role.
		if err := core.DenyServerPermission(ctx, RoleEveryone, PermMessageReact); err != nil {
			t.Fatalf("DenyServerPermission: %v", err)
		}
		if err := core.GrantRoomPermission(ctx, room.Id, RoleEveryone, PermMessageReact); err != nil {
			t.Fatalf("GrantRoomPermission: %v", err)
		}

		has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessageReact)
		if err != nil {
			t.Fatalf("HasRoomPermission: %v", err)
		}
		if !has {
			t.Error("expected room grant to override server deny for the same role")
		}
	})

	t.Run("room deny overrides server grant on the same role", func(t *testing.T) {
		if err := core.GrantServerPermission(ctx, RoleEveryone, PermMessagePost); err != nil {
			t.Fatalf("GrantServerPermission: %v", err)
		}
		if err := core.DenyRoomPermission(ctx, room.Id, RoleEveryone, PermMessagePost); err != nil {
			t.Fatalf("DenyRoomPermission: %v", err)
		}

		has, err := core.permissionResolver.HasRoomPermission(ctx, member.Id, KindChannel, room.Id, PermMessagePost)
		if err != nil {
			t.Fatalf("HasRoomPermission: %v", err)
		}
		if has {
			t.Error("expected room deny to override server grant for the same role")
		}
	})

	t.Run("server grant + server deny on the same role: deny wins (grant probed first, but only one was set)", func(t *testing.T) {
		// Sanity check on the within-role probe order: Grant and Deny shouldn't
		// coexist on the same role/scope in practice (GrantServerPermission
		// clears any matching deny and vice versa), but cover the rare race.
		newUser, _ := core.CreateUser(ctx, "system", "graceuser", "Grace", "password123")

		if err := core.GrantServerPermission(ctx, RoleEveryone, PermMessagePost); err != nil {
			t.Fatalf("GrantServerPermission: %v", err)
		}
		if err := core.DenyServerPermission(ctx, RoleEveryone, PermMessagePost); err != nil {
			t.Fatalf("DenyServerPermission: %v", err)
		}

		// Deny operation clears the prior grant, so only the deny remains in KV.
		has, err := core.permissionResolver.HasSpacePermission(ctx, newUser.Id, KindChannel, PermMessagePost)
		if err != nil {
			t.Fatalf("HasSpacePermission: %v", err)
		}
		if has {
			t.Error("expected deny on everyone to block message.post for a user with no higher-ranked role")
		}
	})
}

// ============================================================================
// Instance Authority Tests
// ============================================================================

func TestPermissionResolver_ServerAuthority(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, _ = core.CreateUser(ctx, "system", "authadmin", "Admin User", "password123")

	member, _ := core.CreateUser(ctx, "system", "authmember", "Member User", "password123")
	t.Run("instance grant applies for space member", func(t *testing.T) {
		// Grant at instance level for instance-everyone
		err := core.GrantServerPermission(ctx, RoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to grant server permission: %v", err)
		}

		// Instance grant should apply (no space-level decision)
		has, err := core.permissionResolver.HasSpacePermission(ctx, member.Id, KindChannel, PermMessagePost)
		if err != nil {
			t.Fatalf("HasSpacePermission() error = %v", err)
		}
		if !has {
			t.Error("Expected instance grant to apply for space member")
		}
	})
}

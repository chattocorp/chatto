package core

import (
	"testing"
)

// ============================================================================
// Instance Permission Can* Helper Tests
// ============================================================================

// TestServerCanHelpers verifies that the semantic Can* helper functions
// for server-level permissions correctly wrap HasPermission.
func TestServerCanHelpers(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a regular user (not admin, not owner)
	regularUser, err := core.CreateUser(ctx, SystemActorID, "regular", "Regular User", "password123")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Create an admin user using AssignAdminRole
	adminUser, err := core.CreateUser(ctx, SystemActorID, "adminuser", "Admin User", "password123")
	if err != nil {
		t.Fatalf("failed to create admin user: %v", err)
	}
	if err := core.AssignAdminRole(ctx, adminUser.Id); err != nil {
		t.Fatalf("failed to assign admin role: %v", err)
	}

	// Test everyone permissions (available to all authenticated users)
	t.Run("regular user has everyone permissions", func(t *testing.T) {
		tests := []struct {
			name  string
			check func() (bool, error)
		}{
			{"CanStartDM", func() (bool, error) { return core.CanStartDM(ctx, regularUser.Id) }},
			{"CanDeleteUserSelf", func() (bool, error) { return core.CanDeleteUser(ctx, regularUser.Id, regularUser.Id) }},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				can, err := tc.check()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !can {
					t.Errorf("regular user should have %s permission", tc.name)
				}
			})
		}
	})

	t.Run("regular user does NOT have admin permissions", func(t *testing.T) {
		can, err := core.HasAnyAdminPermission(ctx, regularUser.Id)
		if err != nil {
			t.Fatalf("HasAnyAdminPermission error: %v", err)
		}
		if can {
			t.Error("regular user should NOT have any admin capability")
		}

		can, err = core.CanAdminUsersView(ctx, regularUser.Id)
		if err != nil {
			t.Fatalf("CanAdminUsersView error: %v", err)
		}
		if can {
			t.Error("regular user should NOT have CanAdminUsersView permission")
		}
	})

	t.Run("admin user has admin permissions", func(t *testing.T) {
		adminTests := []struct {
			name  string
			check func() (bool, error)
		}{
			{"HasAnyAdminPermission", func() (bool, error) { return core.HasAnyAdminPermission(ctx, adminUser.Id) }},
			{"CanAdminUsersView", func() (bool, error) { return core.CanAdminUsersView(ctx, adminUser.Id) }},
			{"CanAssignRoles", func() (bool, error) { return core.CanAssignRoles(ctx, adminUser.Id) }},
			{"CanManageRoles", func() (bool, error) { return core.CanManageRoles(ctx, adminUser.Id) }},
			{"CanAdminSystemView", func() (bool, error) { return core.CanAdminSystemView(ctx, adminUser.Id) }},
		}

		for _, tc := range adminTests {
			t.Run(tc.name, func(t *testing.T) {
				can, err := tc.check()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !can {
					t.Errorf("admin user should have %s permission", tc.name)
				}
			})
		}
	})

}

// TestCanDeleteUser tests the special logic for user deletion permissions.
func TestCanDeleteUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create test users
	user1, err := core.CreateUser(ctx, SystemActorID, "user1", "User One", "password123")
	if err != nil {
		t.Fatalf("failed to create user1: %v", err)
	}

	user2, err := core.CreateUser(ctx, SystemActorID, "user2", "User Two", "password123")
	if err != nil {
		t.Fatalf("failed to create user2: %v", err)
	}

	adminUser, err := core.CreateUser(ctx, SystemActorID, "adminfordelete", "Admin User", "password123")
	if err != nil {
		t.Fatalf("failed to create admin: %v", err)
	}
	if err := core.AssignAdminRole(ctx, adminUser.Id); err != nil {
		t.Fatalf("failed to assign admin role: %v", err)
	}

	t.Run("user can delete their own account (self-deletion)", func(t *testing.T) {
		can, err := core.CanDeleteUser(ctx, user1.Id, user1.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("user should be able to delete their own account")
		}
	})

	t.Run("user cannot delete another user's account", func(t *testing.T) {
		can, err := core.CanDeleteUser(ctx, user1.Id, user2.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("regular user should NOT be able to delete another user's account")
		}
	})

	t.Run("admin can delete any user's account", func(t *testing.T) {
		can, err := core.CanDeleteUser(ctx, adminUser.Id, user1.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("admin should be able to delete any user's account")
		}

		can, err = core.CanDeleteUser(ctx, adminUser.Id, user2.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("admin should be able to delete any user's account")
		}
	})

	t.Run("admin can delete their own account", func(t *testing.T) {
		can, err := core.CanDeleteUser(ctx, adminUser.Id, adminUser.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("admin should be able to delete their own account")
		}
	})

	t.Run("self-deletion denied when user.delete.self permission is revoked", func(t *testing.T) {
		// Create a custom role that denies self-deletion
		if _, err := core.CreateServerRole(ctx, SystemActorID, "selfdelete-denied", "No Self Delete", ""); err != nil {
			t.Fatalf("failed to create role: %v", err)
		}
		if err := core.DenyServerPermission(ctx, SystemActorID, "selfdelete-denied", PermUserDeleteSelf); err != nil {
			t.Fatalf("failed to deny permission: %v", err)
		}

		// Create a user and assign the deny role
		blockedUser, err := core.CreateUser(ctx, SystemActorID, "noselfdelete", "No Self Delete User", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		if err := core.AssignServerRole(ctx, SystemActorID, blockedUser.Id, "selfdelete-denied"); err != nil {
			t.Fatalf("failed to assign role: %v", err)
		}

		can, err := core.CanDeleteUser(ctx, blockedUser.Id, blockedUser.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("user with denied user.delete.self should NOT be able to self-delete")
		}
	})

	t.Run("admin can still delete others when self-delete is denied on everyone", func(t *testing.T) {
		// Even if self-delete is restricted via custom role, admin user.delete still works
		can, err := core.CanDeleteUser(ctx, adminUser.Id, user1.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("admin should still be able to delete other users via user.delete permission")
		}
	})
}

// TestPermissionsWithCustomRoles tests that custom roles
// with specific permissions work correctly with the Can* helpers.
func TestPermissionsWithCustomRoles(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a custom role with limited admin permissions
	customRole, err := core.CreateServerRole(ctx, SystemActorID, "viewer", "Viewer Admin", "Can only view admin pages")
	if err != nil {
		t.Fatalf("failed to create custom role: %v", err)
	}

	// Grant only a concrete admin view permission.
	err = core.GrantServerPermission(ctx, SystemActorID, customRole.Name, PermAdminUsersView)
	if err != nil {
		t.Fatalf("failed to grant users view permission: %v", err)
	}

	// Create user with custom role
	customUser, err := core.CreateUser(ctx, SystemActorID, "customroleuser", "Custom Role User", "password123")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, customUser.Id, customRole.Name); err != nil {
		t.Fatalf("failed to assign role: %v", err)
	}

	t.Run("custom role user has granted permissions", func(t *testing.T) {
		can, err := core.HasAnyAdminPermission(ctx, customUser.Id)
		if err != nil {
			t.Fatalf("HasAnyAdminPermission error: %v", err)
		}
		if !can {
			t.Error("custom role user should have an admin capability")
		}

		can, err = core.CanAdminUsersView(ctx, customUser.Id)
		if err != nil {
			t.Fatalf("CanAdminUsersView error: %v", err)
		}
		if !can {
			t.Error("custom role user should have CanAdminUsersView permission")
		}
	})

	t.Run("custom role user does NOT have ungranted permissions", func(t *testing.T) {
		can, err := core.CanAssignRoles(ctx, customUser.Id)
		if err != nil {
			t.Fatalf("CanAssignRoles error: %v", err)
		}
		if can {
			t.Error("custom role user should NOT have CanAssignRoles permission")
		}

		can, err = core.CanManageRoles(ctx, customUser.Id)
		if err != nil {
			t.Fatalf("CanManageRoles error: %v", err)
		}
		if can {
			t.Error("custom role user should NOT have CanManageRoles permission")
		}
	})
}

// TestCanHelpers verifies that the semantic Can* helper functions correctly
// wrap the underlying HasPermission checks.
func TestCanHelpers(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a space and user
	creator, err := core.CreateUser(ctx, SystemActorID, "creator", "Creator", "password123")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	// CreateSpace used to assign the owner role to its actor. Post-ADR-030
	// that's a separate step, so we mint it explicitly.
	if err := core.AssignServerRole(ctx, SystemActorID, creator.Id, RoleOwner); err != nil {
		t.Fatalf("failed to assign owner role: %v", err)
	}

	// Create a regular member (non-admin)
	member, err := core.CreateUser(ctx, SystemActorID, "member", "Member", "password123")
	if err != nil {
		t.Fatalf("failed to create member user: %v", err)
	}

	// Test cases for admin (creator) - should have all permissions
	adminTests := []struct {
		name   string
		check  func() (bool, error)
		expect bool
	}{
		{"CanManageServer", func() (bool, error) { return core.CanManageServer(ctx, creator.Id) }, true},
		{"CanManageRoles", func() (bool, error) { return core.CanManageRoles(ctx, creator.Id) }, true},
		{"CanAssignRoles", func() (bool, error) { return core.CanAssignRoles(ctx, creator.Id) }, true},
		{"CanCreateRoom", func() (bool, error) { return core.CanCreateRoom(ctx, creator.Id, KindChannel, "") }, true},
		{"CanManageAnyRoom", func() (bool, error) { return core.CanManageAnyRoom(ctx, creator.Id) }, true},
		{"CanJoinRoom", func() (bool, error) { return core.CanJoinRoom(ctx, creator.Id, KindChannel) }, true},
	}

	t.Run("admin has all permissions", func(t *testing.T) {
		for _, tc := range adminTests {
			t.Run(tc.name, func(t *testing.T) {
				can, err := tc.check()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if can != tc.expect {
					t.Errorf("expected %v, got %v", tc.expect, can)
				}
			})
		}
	})

	// Test cases for regular member - should have default member permissions only
	memberTests := []struct {
		name   string
		check  func() (bool, error)
		expect bool
	}{
		// Default member permissions (should be true)
		{"CanJoinRoom", func() (bool, error) { return core.CanJoinRoom(ctx, member.Id, KindChannel) }, true},

		// Admin/elevated permissions (should be false) - room.create is opt-in
		{"CanCreateRoom", func() (bool, error) { return core.CanCreateRoom(ctx, member.Id, KindChannel, "") }, false},
		{"CanManageServer", func() (bool, error) { return core.CanManageServer(ctx, member.Id) }, false},
		{"CanManageRoles", func() (bool, error) { return core.CanManageRoles(ctx, member.Id) }, false},
		{"CanAssignRoles", func() (bool, error) { return core.CanAssignRoles(ctx, member.Id) }, false},
		{"CanManageAnyRoom", func() (bool, error) { return core.CanManageAnyRoom(ctx, member.Id) }, false},
	}

	t.Run("member has default permissions only", func(t *testing.T) {
		for _, tc := range memberTests {
			t.Run(tc.name, func(t *testing.T) {
				can, err := tc.check()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if can != tc.expect {
					t.Errorf("expected %v, got %v", tc.expect, can)
				}
			})
		}
	})
}

// TestCanHelpers_RevokedMemberPermission verifies that revoking a permission
// from the member role actually prevents members from using that permission.
// This tests the fix for the fast path that was bypassing RBAC resolution.
func TestCanHelpers_RevokedMemberPermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a creator and assign them the owner role (formerly granted by
	// CreateSpace).
	creator, err := core.CreateUser(ctx, SystemActorID, "creator", "Creator", "password123")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, creator.Id, RoleOwner); err != nil {
		t.Fatalf("failed to assign owner role: %v", err)
	}
	_ = creator

	// Create a regular member (non-admin)
	member, err := core.CreateUser(ctx, SystemActorID, "member", "Member", "password123")
	if err != nil {
		t.Fatalf("failed to create member user: %v", err)
	}

	t.Run("member does NOT have rooms.create by default", func(t *testing.T) {
		can, err := core.CanCreateRoom(ctx, member.Id, KindChannel, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("member should NOT have CanCreateRoom permission by default (opt-in only)")
		}
	})

	// Grant and then revoke rooms.create from the everyone role
	t.Run("grant then revoke rooms.create from everyone role", func(t *testing.T) {
		// First grant room.create to everyone role (since it's not granted by default)
		err := core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermRoomCreate)
		if err != nil {
			t.Fatalf("failed to grant permission: %v", err)
		}

		// Verify member now has the permission
		can, err := core.CanCreateRoom(ctx, member.Id, KindChannel, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("member should have CanCreateRoom after grant")
		}

		// Now revoke it
		err = core.RevokeServerPermission(ctx, SystemActorID, RoleEveryone, PermRoomCreate)
		if err != nil {
			t.Fatalf("failed to revoke permission: %v", err)
		}

		// Member should no longer have CanCreateRoom
		can, err = core.CanCreateRoom(ctx, member.Id, KindChannel, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("member should NOT have CanCreateRoom after revocation")
		}

		// Admin should still have it
		can, err = core.CanCreateRoom(ctx, creator.Id, KindChannel, "")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("admin should still have CanCreateRoom")
		}
	})

	// Revoke rooms.join from the everyone role
	t.Run("revoke rooms.join from everyone role", func(t *testing.T) {
		err := core.RevokeServerPermission(ctx, SystemActorID, RoleEveryone, PermRoomJoin)
		if err != nil {
			t.Fatalf("failed to revoke permission: %v", err)
		}

		// Member should no longer have CanJoinRoom
		can, err := core.CanJoinRoom(ctx, member.Id, KindChannel)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("member should NOT have CanJoinRoom after revocation")
		}

		// Admin should still have it
		can, err = core.CanJoinRoom(ctx, creator.Id, KindChannel)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("admin should still have CanJoinRoom")
		}
	})
}

// TestCanHelpers_RoomOverrides verifies that room-scoped Can* helpers
// respect room-level permission overrides from the permission resolver.
func TestCanHelpers_RoomOverrides(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	creator, err := core.CreateUser(ctx, SystemActorID, "roomoverrideadmin", "Creator", "password123")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	room, err := core.CreateRoom(ctx, creator.Id, KindChannel, "", "general", "General")
	if err != nil {
		t.Fatalf("failed to create room: %v", err)
	}

	member, err := core.CreateUser(ctx, SystemActorID, "roomoverridemember", "Member", "password123")
	if err != nil {
		t.Fatalf("failed to create member: %v", err)
	}
	t.Run("CanPostMessage respects room-level denial", func(t *testing.T) {
		// Ensure space grants message.post
		core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost)

		// Deny at room level
		core.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePost)

		can, err := core.CanPostMessage(ctx, member.Id, KindChannel, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("CanPostMessage should return false when room denies message.post")
		}

		// Cleanup
		core.ClearRoomPermissionState(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePost)
	})

	t.Run("CanPostInThread respects room-level denial", func(t *testing.T) {
		// Ensure space grants message.post-in-thread
		core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePostInThread)

		// Deny at room level
		core.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePostInThread)

		can, err := core.CanPostInThread(ctx, member.Id, KindChannel, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("CanPostInThread should return false when room denies message.post-in-thread")
		}

		// Cleanup
		core.ClearRoomPermissionState(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePostInThread)
	})

	t.Run("CanReactToMessage respects room-level grant", func(t *testing.T) {
		// Clear message.react from everyone at space level
		core.ClearServerPermissionState(ctx, SystemActorID, RoleEveryone, PermMessageReact)

		// Grant at room level
		core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageReact)

		can, err := core.CanReactToMessage(ctx, member.Id, KindChannel, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("CanReactToMessage should return true when room grants message.react")
		}

		// Cleanup
		core.ClearRoomPermissionState(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageReact)
	})

	t.Run("CanManageOthersMessage respects room-level grant", func(t *testing.T) {
		// Grant message.manage at room level.
		core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageManage)

		can, err := core.CanManageOthersMessage(ctx, member.Id, KindChannel, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("CanManageOthersMessage should return true when room grants message.manage")
		}

		// Cleanup
		core.ClearRoomPermissionState(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageManage)
	})
}

// TestCanHelpers_NonMember verifies that non-members get denied.

package core

import (
	"testing"
)

// ============================================================================
// Instance Permission Can* Helper Tests
// ============================================================================

// TestInstanceCanHelpers verifies that the semantic Can* helper functions
// for instance-level permissions correctly wrap HasPermission.
func TestInstanceCanHelpers(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a regular user (not admin, not owner)
	regularUser, err := core.CreateUser(ctx, SystemActorID, "regular", "Regular User", "password123")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Create an admin user using AssignInstanceAdminRole
	adminUser, err := core.CreateUser(ctx, SystemActorID, "adminuser", "Admin User", "password123")
	if err != nil {
		t.Fatalf("failed to create admin user: %v", err)
	}
	if err := core.AssignInstanceAdminRole(ctx, adminUser.Id); err != nil {
		t.Fatalf("failed to assign admin role: %v", err)
	}

	// Test everyone permissions (available to all authenticated users)
	t.Run("regular user has everyone permissions", func(t *testing.T) {
		tests := []struct {
			name  string
			check func() (bool, error)
		}{
			{"CanDMView", func() (bool, error) { return core.CanDMView(ctx, regularUser.Id) }},
			{"CanDMWrite", func() (bool, error) { return core.CanDMWrite(ctx, regularUser.Id) }},
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
		can, err := core.CanAdminAccess(ctx, regularUser.Id)
		if err != nil {
			t.Fatalf("CanAdminAccess error: %v", err)
		}
		if can {
			t.Error("regular user should NOT have CanAdminAccess permission")
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
			{"CanAdminAccess", func() (bool, error) { return core.CanAdminAccess(ctx, adminUser.Id) }},
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
	if err := core.AssignInstanceAdminRole(ctx, adminUser.Id); err != nil {
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
		if _, err := core.CreateServerRole(ctx, "noselfdelete", "No Self Delete", ""); err != nil {
			t.Fatalf("failed to create role: %v", err)
		}
		if err := core.DenyInstancePermission(ctx, "noselfdelete", PermUserDeleteSelf); err != nil {
			t.Fatalf("failed to deny permission: %v", err)
		}

		// Create a user and assign the deny role
		blockedUser, err := core.CreateUser(ctx, SystemActorID, "noselfdelete", "No Self Delete User", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		if err := core.AssignServerRole(ctx, SystemActorID, blockedUser.Id, "noselfdelete"); err != nil {
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

// TestPermissionsWithCustomRoles tests that custom instance roles
// with specific permissions work correctly with the Can* helpers.
func TestPermissionsWithCustomRoles(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a custom role with limited admin permissions
	customRole, err := core.CreateServerRole(ctx, "viewer", "Viewer Admin", "Can only view admin pages")
	if err != nil {
		t.Fatalf("failed to create custom role: %v", err)
	}

	// Grant only view permissions using GrantPermission
	err = core.GrantInstancePermission(ctx, customRole.Name, PermAdminAccess)
	if err != nil {
		t.Fatalf("failed to grant admin permission: %v", err)
	}
	err = core.GrantInstancePermission(ctx, customRole.Name, PermAdminUsersView)
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
		can, err := core.CanAdminAccess(ctx, customUser.Id)
		if err != nil {
			t.Fatalf("CanAdminAccess error: %v", err)
		}
		if !can {
			t.Error("custom role user should have CanAdminAccess permission")
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
		{"CanBrowseRooms", func() (bool, error) { return core.CanBrowseRooms(ctx, creator.Id, KindChannel) }, true},
		{"CanCreateRoom", func() (bool, error) { return core.CanCreateRoom(ctx, creator.Id, KindChannel) }, true},
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
		{"CanBrowseRooms", func() (bool, error) { return core.CanBrowseRooms(ctx, member.Id, KindChannel) }, true},
		{"CanJoinRoom", func() (bool, error) { return core.CanJoinRoom(ctx, member.Id, KindChannel) }, true},

		// Admin/elevated permissions (should be false) - room.create is opt-in
		{"CanCreateRoom", func() (bool, error) { return core.CanCreateRoom(ctx, member.Id, KindChannel) }, false},
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
// This tests the fix for the fast path that was bypassing the RBAC engine.
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

	// Verify member has default permissions before revocation
	t.Run("member has rooms.browse by default", func(t *testing.T) {
		can, err := core.CanBrowseRooms(ctx, member.Id, KindChannel)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("member should have CanBrowseRooms permission by default")
		}
	})

	t.Run("member does NOT have rooms.create by default", func(t *testing.T) {
		can, err := core.CanCreateRoom(ctx, member.Id, KindChannel)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("member should NOT have CanCreateRoom permission by default (opt-in only)")
		}
	})

	// Revoke rooms.browse from the everyone role
	t.Run("revoke rooms.browse from everyone role", func(t *testing.T) {
		err := core.RevokeInstancePermission(ctx, RoleEveryone, PermRoomList)
		if err != nil {
			t.Fatalf("failed to revoke permission: %v", err)
		}

		// Member should no longer have CanBrowseRooms
		can, err := core.CanBrowseRooms(ctx, member.Id, KindChannel)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("member should NOT have CanBrowseRooms after revocation")
		}

		// Admin should still have it
		can, err = core.CanBrowseRooms(ctx, creator.Id, KindChannel)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("admin should still have CanBrowseRooms")
		}
	})

	// Grant and then revoke rooms.create from the everyone role
	t.Run("grant then revoke rooms.create from everyone role", func(t *testing.T) {
		// First grant room.create to everyone role (since it's not granted by default)
		err := core.GrantInstancePermission(ctx, RoleEveryone, PermRoomCreate)
		if err != nil {
			t.Fatalf("failed to grant permission: %v", err)
		}

		// Verify member now has the permission
		can, err := core.CanCreateRoom(ctx, member.Id, KindChannel)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("member should have CanCreateRoom after grant")
		}

		// Now revoke it
		err = core.RevokeInstancePermission(ctx, RoleEveryone, PermRoomCreate)
		if err != nil {
			t.Fatalf("failed to revoke permission: %v", err)
		}

		// Member should no longer have CanCreateRoom
		can, err = core.CanCreateRoom(ctx, member.Id, KindChannel)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("member should NOT have CanCreateRoom after revocation")
		}

		// Admin should still have it
		can, err = core.CanCreateRoom(ctx, creator.Id, KindChannel)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("admin should still have CanCreateRoom")
		}
	})

	// Revoke rooms.join from the everyone role
	t.Run("revoke rooms.join from everyone role", func(t *testing.T) {
		err := core.RevokeInstancePermission(ctx, RoleEveryone, PermRoomJoin)
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
	room, err := core.CreateRoom(ctx, creator.Id, KindChannel, "general", "General")
	if err != nil {
		t.Fatalf("failed to create room: %v", err)
	}

	member, err := core.CreateUser(ctx, SystemActorID, "roomoverridemember", "Member", "password123")
	if err != nil {
		t.Fatalf("failed to create member: %v", err)
	}
	t.Run("CanPostMessage respects room-level denial", func(t *testing.T) {
		// Ensure space grants message.post
		core.GrantInstancePermission(ctx, RoleEveryone, PermMessagePost)

		// Deny at room level
		core.DenyRoomPermission(ctx, room.Id, RoleEveryone, PermMessagePost)

		can, err := core.CanPostMessage(ctx, member.Id, KindChannel, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("CanPostMessage should return false when room denies message.post")
		}

		// Cleanup
		core.ClearRoomPermissionState(ctx, room.Id, RoleEveryone, PermMessagePost)
	})

	t.Run("CanPostInThread respects room-level denial", func(t *testing.T) {
		// Ensure space grants message.post-in-thread
		core.GrantInstancePermission(ctx, RoleEveryone, PermMessagePostInThread)

		// Deny at room level
		core.DenyRoomPermission(ctx, room.Id, RoleEveryone, PermMessagePostInThread)

		can, err := core.CanPostInThread(ctx, member.Id, KindChannel, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("CanPostInThread should return false when room denies message.post-in-thread")
		}

		// Cleanup
		core.ClearRoomPermissionState(ctx, room.Id, RoleEveryone, PermMessagePostInThread)
	})

	t.Run("CanReply respects room-level denial", func(t *testing.T) {
		core.GrantInstancePermission(ctx, RoleEveryone, PermMessageReply)

		core.DenyRoomPermission(ctx, room.Id, RoleEveryone, PermMessageReply)

		can, err := core.CanReply(ctx, member.Id, KindChannel, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("CanReply should return false when room denies message.reply")
		}

		// Cleanup
		core.ClearRoomPermissionState(ctx, room.Id, RoleEveryone, PermMessageReply)
	})

	t.Run("CanReplyInThread respects room-level denial", func(t *testing.T) {
		core.GrantInstancePermission(ctx, RoleEveryone, PermMessageReplyInThread)

		core.DenyRoomPermission(ctx, room.Id, RoleEveryone, PermMessageReplyInThread)

		can, err := core.CanReplyInThread(ctx, member.Id, KindChannel, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("CanReplyInThread should return false when room denies message.reply-in-thread")
		}

		// Cleanup
		core.ClearRoomPermissionState(ctx, room.Id, RoleEveryone, PermMessageReplyInThread)
	})

	t.Run("CanReply is independent of CanPostMessage", func(t *testing.T) {
		core.DenyRoomPermission(ctx, room.Id, RoleEveryone, PermMessagePost)

		canPost, err := core.CanPostMessage(ctx, member.Id, KindChannel, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if canPost {
			t.Error("CanPostMessage should return false when denied")
		}

		canReply, err := core.CanReply(ctx, member.Id, KindChannel, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !canReply {
			t.Error("CanReply should return true when message.reply is granted (independent of message.post)")
		}

		// Cleanup
		core.ClearRoomPermissionState(ctx, room.Id, RoleEveryone, PermMessagePost)
	})

	t.Run("CanReactToMessage respects room-level grant", func(t *testing.T) {
		// Clear message.react from everyone at space level
		core.ClearInstancePermissionState(ctx, RoleEveryone, PermMessageReact)

		// Grant at room level
		core.GrantRoomPermission(ctx, room.Id, RoleEveryone, PermMessageReact)

		can, err := core.CanReactToMessage(ctx, member.Id, KindChannel, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("CanReactToMessage should return true when room grants message.react")
		}

		// Cleanup
		core.ClearRoomPermissionState(ctx, room.Id, RoleEveryone, PermMessageReact)
	})

	t.Run("CanEditOwnMessage respects room-level denial", func(t *testing.T) {
		// Ensure space grants message.edit-own
		core.GrantInstancePermission(ctx, RoleEveryone, PermMessageEditOwn)

		// Deny at room level
		core.DenyRoomPermission(ctx, room.Id, RoleEveryone, PermMessageEditOwn)

		can, err := core.CanEditOwnMessage(ctx, member.Id, KindChannel, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("CanEditOwnMessage should return false when room denies message.edit-own")
		}

		// Cleanup
		core.ClearRoomPermissionState(ctx, room.Id, RoleEveryone, PermMessageEditOwn)
	})

	t.Run("CanDeleteAnyMessage respects room-level grant", func(t *testing.T) {
		// Ensure no space-level grant for message.delete-any (it's not default)
		// Grant at room level
		core.GrantRoomPermission(ctx, room.Id, RoleEveryone, PermMessageDeleteAny)

		can, err := core.CanDeleteAnyMessage(ctx, member.Id, KindChannel, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("CanDeleteAnyMessage should return true when room grants message.delete-any")
		}

		// Cleanup
		core.ClearRoomPermissionState(ctx, room.Id, RoleEveryone, PermMessageDeleteAny)
	})
}

// TestCanHelpers_NonMember verifies that non-members get denied.

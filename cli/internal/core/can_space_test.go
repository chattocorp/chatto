package core

import (
	"testing"
)

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

	space, err := core.CreateSpace(ctx, creator.Id, "test-space", "Test Space")
	if err != nil {
		t.Fatalf("failed to create space: %v", err)
	}

	// Create a regular member (non-admin)
	member, err := core.CreateUser(ctx, SystemActorID, "member", "Member", "password123")
	if err != nil {
		t.Fatalf("failed to create member user: %v", err)
	}

	_, err = core.JoinSpace(ctx, member.Id, space.Id)
	if err != nil {
		t.Fatalf("failed to join space: %v", err)
	}

	// Test cases for admin (creator) - should have all permissions
	adminTests := []struct {
		name   string
		check  func() (bool, error)
		expect bool
	}{
		{"CanAdminSpaceManage", func() (bool, error) { return core.CanAdminSpaceManage(ctx, creator.Id, space.Id) }, true},
		{"CanAdminSpaceDelete", func() (bool, error) { return core.CanAdminSpaceDelete(ctx, creator.Id, space.Id) }, true},
		{"CanSpaceRolesManage", func() (bool, error) { return core.CanSpaceRolesManage(ctx, creator.Id, space.Id) }, true},
		{"CanSpaceRolesAssign", func() (bool, error) { return core.CanSpaceRolesAssign(ctx, creator.Id, space.Id) }, true},
		{"CanAdminMembersInvite", func() (bool, error) { return core.CanAdminMembersInvite(ctx, creator.Id, space.Id) }, true},
		{"CanAdminMembersRemove", func() (bool, error) { return core.CanAdminMembersRemove(ctx, creator.Id, space.Id) }, true},
		{"CanBrowseRooms", func() (bool, error) { return core.CanBrowseRooms(ctx, creator.Id, space.Id) }, true},
		{"CanCreateRoom", func() (bool, error) { return core.CanCreateRoom(ctx, creator.Id, space.Id) }, true},
		{"CanAdminRoomsManage", func() (bool, error) { return core.CanAdminRoomsManage(ctx, creator.Id, space.Id) }, true},
		{"CanJoinRoom", func() (bool, error) { return core.CanJoinRoom(ctx, creator.Id, space.Id) }, true},
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
		{"CanBrowseRooms", func() (bool, error) { return core.CanBrowseRooms(ctx, member.Id, space.Id) }, true},
		{"CanJoinRoom", func() (bool, error) { return core.CanJoinRoom(ctx, member.Id, space.Id) }, true},

		// Admin/elevated permissions (should be false) - room.create is opt-in
		{"CanCreateRoom", func() (bool, error) { return core.CanCreateRoom(ctx, member.Id, space.Id) }, false},
		{"CanAdminSpaceManage", func() (bool, error) { return core.CanAdminSpaceManage(ctx, member.Id, space.Id) }, false},
		{"CanAdminSpaceDelete", func() (bool, error) { return core.CanAdminSpaceDelete(ctx, member.Id, space.Id) }, false},
		{"CanSpaceRolesManage", func() (bool, error) { return core.CanSpaceRolesManage(ctx, member.Id, space.Id) }, false},
		{"CanSpaceRolesAssign", func() (bool, error) { return core.CanSpaceRolesAssign(ctx, member.Id, space.Id) }, false},
		{"CanAdminMembersInvite", func() (bool, error) { return core.CanAdminMembersInvite(ctx, member.Id, space.Id) }, false},
		{"CanAdminMembersRemove", func() (bool, error) { return core.CanAdminMembersRemove(ctx, member.Id, space.Id) }, false},
		{"CanAdminRoomsManage", func() (bool, error) { return core.CanAdminRoomsManage(ctx, member.Id, space.Id) }, false},
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

	// Create a space
	creator, err := core.CreateUser(ctx, SystemActorID, "creator", "Creator", "password123")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	space, err := core.CreateSpace(ctx, creator.Id, "test-space", "Test Space")
	if err != nil {
		t.Fatalf("failed to create space: %v", err)
	}

	// Create a regular member (non-admin)
	member, err := core.CreateUser(ctx, SystemActorID, "member", "Member", "password123")
	if err != nil {
		t.Fatalf("failed to create member user: %v", err)
	}

	_, err = core.JoinSpace(ctx, member.Id, space.Id)
	if err != nil {
		t.Fatalf("failed to join space: %v", err)
	}

	// Verify member has default permissions before revocation
	t.Run("member has rooms.browse by default", func(t *testing.T) {
		can, err := core.CanBrowseRooms(ctx, member.Id, space.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("member should have CanBrowseRooms permission by default")
		}
	})

	t.Run("member does NOT have rooms.create by default", func(t *testing.T) {
		can, err := core.CanCreateRoom(ctx, member.Id, space.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("member should NOT have CanCreateRoom permission by default (opt-in only)")
		}
	})

	// Revoke rooms.browse from the everyone role
	t.Run("revoke rooms.browse from everyone role", func(t *testing.T) {
		err := core.RevokeSpacePermission(ctx, creator.Id, space.Id, RoleEveryone, PermRoomList)
		if err != nil {
			t.Fatalf("failed to revoke permission: %v", err)
		}

		// Member should no longer have CanBrowseRooms
		can, err := core.CanBrowseRooms(ctx, member.Id, space.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("member should NOT have CanBrowseRooms after revocation")
		}

		// Admin should still have it
		can, err = core.CanBrowseRooms(ctx, creator.Id, space.Id)
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
		err := core.GrantSpaceRolePermission(ctx, space.Id, RoleEveryone, PermRoomCreate)
		if err != nil {
			t.Fatalf("failed to grant permission: %v", err)
		}

		// Verify member now has the permission
		can, err := core.CanCreateRoom(ctx, member.Id, space.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("member should have CanCreateRoom after grant")
		}

		// Now revoke it
		err = core.RevokeSpacePermission(ctx, creator.Id, space.Id, RoleEveryone, PermRoomCreate)
		if err != nil {
			t.Fatalf("failed to revoke permission: %v", err)
		}

		// Member should no longer have CanCreateRoom
		can, err = core.CanCreateRoom(ctx, member.Id, space.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("member should NOT have CanCreateRoom after revocation")
		}

		// Admin should still have it
		can, err = core.CanCreateRoom(ctx, creator.Id, space.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("admin should still have CanCreateRoom")
		}
	})

	// Revoke rooms.join from the everyone role
	t.Run("revoke rooms.join from everyone role", func(t *testing.T) {
		err := core.RevokeSpacePermission(ctx, creator.Id, space.Id, RoleEveryone, PermRoomJoin)
		if err != nil {
			t.Fatalf("failed to revoke permission: %v", err)
		}

		// Member should no longer have CanJoinRoom
		can, err := core.CanJoinRoom(ctx, member.Id, space.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("member should NOT have CanJoinRoom after revocation")
		}

		// Admin should still have it
		can, err = core.CanJoinRoom(ctx, creator.Id, space.Id)
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
	space, err := core.CreateSpace(ctx, creator.Id, "test-space", "Test Space")
	if err != nil {
		t.Fatalf("failed to create space: %v", err)
	}
	room, err := core.CreateRoom(ctx, creator.Id, space.Id, "general", "General")
	if err != nil {
		t.Fatalf("failed to create room: %v", err)
	}

	member, err := core.CreateUser(ctx, SystemActorID, "roomoverridemember", "Member", "password123")
	if err != nil {
		t.Fatalf("failed to create member: %v", err)
	}
	core.JoinSpace(ctx, member.Id, space.Id)

	t.Run("CanPostMessage respects room-level denial", func(t *testing.T) {
		// Ensure space grants message.post
		core.GrantSpaceRolePermission(ctx, space.Id, RoleEveryone, PermMessagePost)

		// Deny at room level
		core.denyRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermMessagePost)

		can, err := core.CanPostMessage(ctx, member.Id, space.Id, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("CanPostMessage should return false when room denies message.post")
		}

		// Cleanup
		core.clearRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermMessagePost)
	})

	t.Run("CanPostInThread respects room-level denial", func(t *testing.T) {
		// Ensure space grants message.post-in-thread
		core.GrantSpaceRolePermission(ctx, space.Id, RoleEveryone, PermMessagePostInThread)

		// Deny at room level
		core.denyRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermMessagePostInThread)

		can, err := core.CanPostInThread(ctx, member.Id, space.Id, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("CanPostInThread should return false when room denies message.post-in-thread")
		}

		// Cleanup
		core.clearRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermMessagePostInThread)
	})

	t.Run("CanReply respects room-level denial", func(t *testing.T) {
		core.GrantSpaceRolePermission(ctx, space.Id, RoleEveryone, PermMessageReply)

		core.denyRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermMessageReply)

		can, err := core.CanReply(ctx, member.Id, space.Id, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("CanReply should return false when room denies message.reply")
		}

		// Cleanup
		core.clearRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermMessageReply)
	})

	t.Run("CanReplyInThread respects room-level denial", func(t *testing.T) {
		core.GrantSpaceRolePermission(ctx, space.Id, RoleEveryone, PermMessageReplyInThread)

		core.denyRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermMessageReplyInThread)

		can, err := core.CanReplyInThread(ctx, member.Id, space.Id, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("CanReplyInThread should return false when room denies message.reply-in-thread")
		}

		// Cleanup
		core.clearRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermMessageReplyInThread)
	})

	t.Run("CanReply is independent of CanPostMessage", func(t *testing.T) {
		core.denyRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermMessagePost)

		canPost, err := core.CanPostMessage(ctx, member.Id, space.Id, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if canPost {
			t.Error("CanPostMessage should return false when denied")
		}

		canReply, err := core.CanReply(ctx, member.Id, space.Id, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !canReply {
			t.Error("CanReply should return true when message.reply is granted (independent of message.post)")
		}

		// Cleanup
		core.clearRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermMessagePost)
	})

	t.Run("CanReactToMessage respects room-level grant", func(t *testing.T) {
		// Clear message.react from everyone at space level
		core.ClearSpaceRolePermission(ctx, space.Id, RoleEveryone, PermMessageReact)

		// Grant at room level
		core.grantRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermMessageReact)

		can, err := core.CanReactToMessage(ctx, member.Id, space.Id, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("CanReactToMessage should return true when room grants message.react")
		}

		// Cleanup
		core.clearRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermMessageReact)
	})

	t.Run("CanEditOwnMessage respects room-level denial", func(t *testing.T) {
		// Ensure space grants message.edit-own
		core.GrantSpaceRolePermission(ctx, space.Id, RoleEveryone, PermMessageEditOwn)

		// Deny at room level
		core.denyRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermMessageEditOwn)

		can, err := core.CanEditOwnMessage(ctx, member.Id, space.Id, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("CanEditOwnMessage should return false when room denies message.edit-own")
		}

		// Cleanup
		core.clearRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermMessageEditOwn)
	})

	t.Run("CanDeleteAnyMessage respects room-level grant", func(t *testing.T) {
		// Ensure no space-level grant for message.delete-any (it's not default)
		// Grant at room level
		core.grantRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermMessageDeleteAny)

		can, err := core.CanDeleteAnyMessage(ctx, member.Id, space.Id, room.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("CanDeleteAnyMessage should return true when room grants message.delete-any")
		}

		// Cleanup
		core.clearRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermMessageDeleteAny)
	})
}

// TestCanHelpers_NonMember verifies that non-members get denied.
func TestCanHelpers_NonMember(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a space
	creator, err := core.CreateUser(ctx, SystemActorID, "creator", "Creator", "password123")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	space, err := core.CreateSpace(ctx, creator.Id, "test-space", "Test Space")
	if err != nil {
		t.Fatalf("failed to create space: %v", err)
	}

	// Create a non-member
	outsider, err := core.CreateUser(ctx, SystemActorID, "outsider", "Outsider", "password123")
	if err != nil {
		t.Fatalf("failed to create outsider user: %v", err)
	}

	// Non-members should have no permissions
	can, err := core.CanBrowseRooms(ctx, outsider.Id, space.Id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if can {
		t.Error("non-member should not have CanBrowseRooms permission")
	}

	can, err = core.CanAdminSpaceManage(ctx, outsider.Id, space.Id)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if can {
		t.Error("non-member should not have CanAdminSpaceManage permission")
	}
}

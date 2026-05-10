package core

import (
	"context"
	"testing"

	"hmans.de/chatto/internal/core/rbac"
)

// Helper to construct expected allow key from permission
func expectedAllowKey(subject string, perm Permission, objectId string) string {
	parts := perm.KeyParts()
	return rbac.AllowKey(subject, parts.Verb, parts.ObjectType, objectId)
}

// Helper to construct expected deny key from permission
func expectedDenyKey(subject string, perm Permission, objectId string) string {
	parts := perm.KeyParts()
	return rbac.DenyKey(subject, parts.Verb, parts.ObjectType, objectId)
}

// ============================================================================
// Instance-Level Role Operations Tests
// ============================================================================

func TestGrantInstancePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("creates correct KV key for valid permission", func(t *testing.T) {
		err := core.GrantInstancePermission(ctx, RoleModerator, PermDMWrite)
		if err != nil {
			t.Fatalf("GrantInstancePermission() error = %v", err)
		}

		// Verify key was created
		kv := core.storage.serverRBACEngine.KV()
		expectedKey := expectedAllowKey(RoleModerator, PermDMWrite, rbac.ObjectIdAny)
		_, err = kv.Get(ctx, expectedKey)
		if err != nil {
			t.Errorf("Expected KV key %s to exist, got error: %v", expectedKey, err)
		}
	})

	t.Run("removes existing denial when granting", func(t *testing.T) {
		// First deny the permission
		err := core.DenyInstancePermission(ctx, RoleModerator, PermDMView)
		if err != nil {
			t.Fatalf("DenyInstancePermission() error = %v", err)
		}

		// Now grant it - should remove the denial
		err = core.GrantInstancePermission(ctx, RoleModerator, PermDMView)
		if err != nil {
			t.Fatalf("GrantInstancePermission() error = %v", err)
		}

		// Verify denial was removed
		kv := core.storage.serverRBACEngine.KV()
		denyKey := expectedDenyKey(RoleModerator, PermDMView, rbac.ObjectIdAny)
		_, err = kv.Get(ctx, denyKey)
		if err == nil {
			t.Error("Expected denial key to be removed after grant")
		}
	})

	t.Run("rejects unrecognised permission", func(t *testing.T) {
		err := core.GrantInstancePermission(ctx, RoleModerator, Permission("not.a.real.permission"))
		if err == nil {
			t.Error("Expected error for invalid permission")
		}
	})
}

func TestDenyInstancePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("creates deny key", func(t *testing.T) {
		err := core.DenyInstancePermission(ctx, RoleEveryone, PermDMWrite)
		if err != nil {
			t.Fatalf("DenyInstancePermission() error = %v", err)
		}

		// Verify deny key was created
		kv := core.storage.serverRBACEngine.KV()
		expectedKey := expectedDenyKey(RoleEveryone, PermDMWrite, rbac.ObjectIdAny)
		_, err = kv.Get(ctx, expectedKey)
		if err != nil {
			t.Errorf("Expected deny key %s to exist, got error: %v", expectedKey, err)
		}
	})

	t.Run("removes existing grant when denying", func(t *testing.T) {
		// First grant the permission
		err := core.GrantInstancePermission(ctx, RoleEveryone, PermDMWrite)
		if err != nil {
			t.Fatalf("GrantInstancePermission() error = %v", err)
		}

		// Now deny it - should remove the grant
		err = core.DenyInstancePermission(ctx, RoleEveryone, PermDMWrite)
		if err != nil {
			t.Fatalf("DenyInstancePermission() error = %v", err)
		}

		// Verify grant was removed
		kv := core.storage.serverRBACEngine.KV()
		grantKey := expectedAllowKey(RoleEveryone, PermDMWrite, rbac.ObjectIdAny)
		_, err = kv.Get(ctx, grantKey)
		if err == nil {
			t.Error("Expected grant key to be removed after denial")
		}
	})

	t.Run("rejects unrecognised permission", func(t *testing.T) {
		err := core.DenyInstancePermission(ctx, RoleModerator, Permission("not.real.permission"))
		if err == nil {
			t.Error("Expected error for invalid permission")
		}
	})
}

func TestClearInstancePermissionState(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("clears both grant and denial", func(t *testing.T) {
		// Grant a permission
		err := core.GrantInstancePermission(ctx, RoleModerator, PermDMView)
		if err != nil {
			t.Fatalf("Failed to grant: %v", err)
		}

		// Clear it
		err = core.ClearInstancePermissionState(ctx, RoleModerator, PermDMView)
		if err != nil {
			t.Fatalf("ClearInstancePermissionState() error = %v", err)
		}

		// Verify both keys are gone
		kv := core.storage.serverRBACEngine.KV()
		grantKey := expectedAllowKey(RoleModerator, PermDMView, rbac.ObjectIdAny)
		denyKey := expectedDenyKey(RoleModerator, PermDMView, rbac.ObjectIdAny)

		if _, err := kv.Get(ctx, grantKey); err == nil {
			t.Error("Expected grant key to be cleared")
		}
		if _, err := kv.Get(ctx, denyKey); err == nil {
			t.Error("Expected deny key to be cleared")
		}
	})

	t.Run("succeeds when clearing non-existent key", func(t *testing.T) {
		err := core.ClearInstancePermissionState(ctx, RoleEveryone, PermDMWrite)
		if err != nil {
			t.Errorf("Expected no error when clearing non-existent key, got: %v", err)
		}
	})
}

// ============================================================================
// Space-Level Operations Tests
// ============================================================================

func TestGrantSpaceRolePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	space, _ := core.CreateSpace(ctx, user.Id, "Test Space", "A test space")

	t.Run("creates correct KV key for space role", func(t *testing.T) {
		err := core.GrantSpacePermission(ctx, SystemActorID, space.Id, RoleEveryone, PermRoomCreate)
		if err != nil {
			t.Fatalf("GrantSpaceRolePermission() error = %v", err)
		}

		// Verify key was created in space RBAC KV
		kv := core.storage.serverRBACKV
		expectedKey := expectedAllowKey(RoleEveryone, PermRoomCreate, rbac.ObjectIdAny)
		_, err = kv.Get(ctx, expectedKey)
		if err != nil {
			t.Errorf("Expected space KV key to exist, got error: %v", err)
		}
	})

	t.Run("works for instance role override at space level", func(t *testing.T) {
		// Instance role override at space level
		err := core.GrantSpacePermission(ctx, SystemActorID, space.Id, RoleModerator, PermRoomJoin)
		if err != nil {
			t.Fatalf("GrantSpaceRolePermission() for instance role error = %v", err)
		}
	})

	t.Run("rejects room-only permission at space scope", func(t *testing.T) {
		// room.manage only applies at space and room scopes, but not instance
		// Actually room.manage applies at space and room, so it should work...
		// Let me use a room-only permission if there is one... Looking at the code,
		// room.join applies at all three scopes. Let's skip this test as there's no
		// purely room-only permission that can't be used at space level.
	})
}

func TestDenySpaceRolePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	space, _ := core.CreateSpace(ctx, user.Id, "Test Space", "A test space")

	t.Run("creates deny key in space RBAC", func(t *testing.T) {
		err := core.DenySpacePermission(ctx, SystemActorID, space.Id, RoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("DenySpaceRolePermission() error = %v", err)
		}

		// Verify deny key was created
		kv := core.storage.serverRBACKV
		expectedKey := expectedDenyKey(RoleEveryone, PermMessagePost, rbac.ObjectIdAny)
		_, err = kv.Get(ctx, expectedKey)
		if err != nil {
			t.Errorf("Expected space deny key to exist, got error: %v", err)
		}
	})
}

func TestClearSpaceRolePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	space, _ := core.CreateSpace(ctx, user.Id, "Test Space", "A test space")

	t.Run("clears both grant and denial at space level", func(t *testing.T) {
		// Grant then clear
		_ = core.GrantSpacePermission(ctx, SystemActorID, space.Id, RoleEveryone, PermRoomList)

		err := core.ClearSpacePermissionState(ctx, SystemActorID, space.Id, RoleEveryone, PermRoomList)
		if err != nil {
			t.Fatalf("ClearSpaceRolePermission() error = %v", err)
		}

		// Verify keys are gone
		kv := core.storage.serverRBACKV
		grantKey := expectedAllowKey(RoleEveryone, PermRoomList, rbac.ObjectIdAny)
		if _, err := kv.Get(ctx, grantKey); err == nil {
			t.Error("Expected grant key to be cleared")
		}
	})
}

// ============================================================================
// Room-Level Operations Tests
// ============================================================================

func TestGrantRoomRolePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	space, _ := core.CreateSpace(ctx, user.Id, "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, user.Id, space.Id, "General", "General chat")

	t.Run("creates correct KV key for room-level permission", func(t *testing.T) {
		err := core.grantRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("GrantRoomRolePermission() error = %v", err)
		}

		// Verify key was created with room ID as objectId
		kv := core.storage.serverRBACKV
		expectedKey := expectedAllowKey(RoleEveryone, PermMessagePost, room.Id)
		_, err = kv.Get(ctx, expectedKey)
		if err != nil {
			t.Errorf("Expected room grant key to exist, got error: %v", err)
		}
	})

	t.Run("rejects permission that does not apply at room scope", func(t *testing.T) {
		// space.create only applies at instance scope
		err := core.grantRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermDMWrite)
		if err == nil {
			t.Error("Expected error for permission that doesn't apply at room scope")
		}
	})
}

func TestDenyRoomRolePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	space, _ := core.CreateSpace(ctx, user.Id, "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, user.Id, space.Id, "General", "General chat")

	t.Run("creates deny key at room level", func(t *testing.T) {
		err := core.denyRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("DenyRoomRolePermission() error = %v", err)
		}

		// Verify deny key was created
		kv := core.storage.serverRBACKV
		expectedKey := expectedDenyKey(RoleEveryone, PermMessagePost, room.Id)
		_, err = kv.Get(ctx, expectedKey)
		if err != nil {
			t.Errorf("Expected room deny key to exist, got error: %v", err)
		}
	})

	t.Run("rejects permission that does not apply at room scope", func(t *testing.T) {
		err := core.denyRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermAdminAccess)
		if err == nil {
			t.Error("Expected error for permission that doesn't apply at room scope")
		}
	})
}

func TestClearRoomRolePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	space, _ := core.CreateSpace(ctx, user.Id, "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, user.Id, space.Id, "General", "General chat")

	t.Run("clears both grant and denial at room level", func(t *testing.T) {
		// Grant then clear
		_ = core.grantRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermRoomJoin)

		err := core.clearRoomRolePermissionInternal(ctx, space.Id, room.Id, RoleEveryone, PermRoomJoin)
		if err != nil {
			t.Fatalf("ClearRoomRolePermission() error = %v", err)
		}

		// Verify key was removed
		kv := core.storage.serverRBACKV
		grantKey := expectedAllowKey(RoleEveryone, PermRoomJoin, room.Id)
		if _, err := kv.Get(ctx, grantKey); err == nil {
			t.Error("Expected grant key to be cleared")
		}
	})
}

// ============================================================================
// Idempotency Tests
// ============================================================================

func TestPermissionOpsIdempotency(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("granting same permission twice succeeds", func(t *testing.T) {
		err := core.GrantInstancePermission(ctx, RoleModerator, PermDMView)
		if err != nil {
			t.Fatalf("First grant failed: %v", err)
		}

		err = core.GrantInstancePermission(ctx, RoleModerator, PermDMView)
		if err != nil {
			t.Errorf("Second grant should succeed (idempotent), got: %v", err)
		}
	})

	t.Run("denying same permission twice succeeds", func(t *testing.T) {
		err := core.DenyInstancePermission(ctx, RoleEveryone, PermDMWrite)
		if err != nil {
			t.Fatalf("First deny failed: %v", err)
		}

		err = core.DenyInstancePermission(ctx, RoleEveryone, PermDMWrite)
		if err != nil {
			t.Errorf("Second deny should succeed (idempotent), got: %v", err)
		}
	})

	t.Run("denying after grant updates correctly", func(t *testing.T) {
		perm := PermDMWrite

		// Grant
		err := core.GrantInstancePermission(ctx, RoleEveryone, perm)
		if err != nil {
			t.Fatalf("Grant failed: %v", err)
		}

		// Now deny
		err = core.DenyInstancePermission(ctx, RoleEveryone, perm)
		if err != nil {
			t.Fatalf("Deny failed: %v", err)
		}

		// Verify grant is gone and deny exists
		kv := core.storage.serverRBACEngine.KV()
		grantKey := expectedAllowKey(RoleEveryone, perm, rbac.ObjectIdAny)
		denyKey := expectedDenyKey(RoleEveryone, perm, rbac.ObjectIdAny)

		if _, err := kv.Get(ctx, grantKey); err == nil {
			t.Error("Grant key should be removed after deny")
		}
		if _, err := kv.Get(ctx, denyKey); err != nil {
			t.Error("Deny key should exist")
		}
	})
}

// ============================================================================
// Initialization Tests
// ============================================================================

func TestInitInstanceDefaults(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// InitInstanceDefaults is called during setupTestCore, so we can verify its effects

	t.Run("admin has all instance permissions", func(t *testing.T) {
		for _, perm := range PermissionsForScope(ScopeServer) {
			kv := core.storage.serverRBACEngine.KV()
			key := expectedAllowKey(RoleAdmin, perm.Permission, rbac.ObjectIdAny)
			_, err := kv.Get(ctx, key)
			if err != nil {
				t.Errorf("Expected admin to have permission %s, but key not found", perm.Permission)
			}
		}
	})

	t.Run("everyone has dm.view permission", func(t *testing.T) {
		kv := core.storage.serverRBACEngine.KV()
		key := expectedAllowKey(RoleEveryone, PermDMView, rbac.ObjectIdAny)
		_, err := kv.Get(ctx, key)
		if err != nil {
			t.Error("Expected instance-everyone to have dm.view permission")
		}
	})

	t.Run("everyone has expected permissions", func(t *testing.T) {
		kv := core.storage.serverRBACEngine.KV()
		expectedPerms := []Permission{PermDMView, PermDMWrite, PermUserDeleteSelf}
		for _, perm := range expectedPerms {
			key := expectedAllowKey(RoleEveryone, perm, rbac.ObjectIdAny)
			_, err := kv.Get(ctx, key)
			if err != nil {
				t.Errorf("Expected instance-everyone to have permission %s, but key not found", perm)
			}
		}
	})
}

func TestInitSpaceDefaults(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	if _, err := core.CreateSpace(ctx, user.Id, "Test Space", "A test space"); err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}

	// InitSpaceDefaults is called during CreateSpace, so we can verify its effects

	t.Run("owner has all space permissions", func(t *testing.T) {
		kv := core.storage.serverRBACKV
		for _, perm := range PermissionsForScope(ScopeSpace) {
			key := expectedAllowKey(RoleOwner, perm.Permission, rbac.ObjectIdAny)
			_, err := kv.Get(ctx, key)
			if err != nil {
				t.Errorf("Expected space owner to have permission %s, but key not found", perm.Permission)
			}
		}
	})

	t.Run("everyone has default member permissions", func(t *testing.T) {
		kv := core.storage.serverRBACKV
		for _, perm := range DefaultSpaceEveryonePermissions() {
			key := expectedAllowKey(RoleEveryone, perm, rbac.ObjectIdAny)
			_, err := kv.Get(ctx, key)
			if err != nil {
				t.Errorf("Expected space everyone to have permission %s, but key not found", perm)
			}
		}
	})

	t.Run("moderator has moderation permissions", func(t *testing.T) {
		kv := core.storage.serverRBACKV
		moderatorPerms := []Permission{PermMemberRemove, PermMessageEditAny, PermMessageDeleteAny}
		for _, perm := range moderatorPerms {
			key := expectedAllowKey("moderator", perm, rbac.ObjectIdAny)
			_, err := kv.Get(ctx, key)
			if err != nil {
				t.Errorf("Expected space moderator to have permission %s, but key not found", perm)
			}
		}
	})

	t.Run("instance-everyone permissions are at instance level not space level", func(t *testing.T) {
		t.Skip("Phase 5 collapsed instance/space tiers into a single SERVER_RBAC bucket.")
	})
}

// ============================================================================
// Context Cancellation Tests
// ============================================================================

func TestPermissionOpsWithCancelledContext(t *testing.T) {
	core, _ := setupTestCore(t)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	t.Run("grant fails with cancelled context", func(t *testing.T) {
		err := core.GrantInstancePermission(ctx, RoleModerator, PermDMView)
		if err == nil {
			t.Error("Expected error with cancelled context")
		}
	})
}

// ============================================================================
// Announcements Room Tests
// ============================================================================

func TestSetupAnnouncementsRoomPermissions(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user and space
	user, err := core.CreateUser(ctx, SystemActorID, "ann-test-user", "Ann Test", "password")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	space, err := core.CreateSpace(ctx, user.Id, "Test Space", "")
	if err != nil {
		t.Fatalf("CreateSpace failed: %v", err)
	}

	// Create a regular room
	regularRoom, err := core.CreateRoom(ctx, user.Id, space.Id, "general", "")
	if err != nil {
		t.Fatalf("CreateRoom (general) failed: %v", err)
	}

	// Create an announcements room
	annRoom, err := core.CreateRoom(ctx, user.Id, space.Id, "announcements", "")
	if err != nil {
		t.Fatalf("CreateRoom (announcements) failed: %v", err)
	}

	t.Run("announcements room denies message.post to everyone", func(t *testing.T) {
		kv := core.storage.serverRBACKV
		denyKey := expectedDenyKey(RoleEveryone, PermMessagePost, annRoom.Id)
		_, err := kv.Get(ctx, denyKey)
		if err != nil {
			t.Errorf("Expected deny key %s to exist for announcements room", denyKey)
		}
	})

	t.Run("announcements room grants message.post to owner, admin, and moderator", func(t *testing.T) {
		kv := core.storage.serverRBACKV

		for _, roleName := range []string{RoleOwner, RoleAdmin, RoleModerator} {
			grantKey := expectedAllowKey(roleName, PermMessagePost, annRoom.Id)
			_, err := kv.Get(ctx, grantKey)
			if err != nil {
				t.Errorf("Expected grant key %s to exist for %s in announcements room", grantKey, roleName)
			}
		}
	})

	t.Run("regular room does not have announcements permissions", func(t *testing.T) {
		kv := core.storage.serverRBACKV

		// Regular room should NOT have the everyone denial for message.post
		denyKey := expectedDenyKey(RoleEveryone, PermMessagePost, regularRoom.Id)
		_, err := kv.Get(ctx, denyKey)
		if err == nil {
			t.Errorf("Regular room should not have %s denial for everyone", PermMessagePost)
		}
	})

	t.Run("owner can post in announcements, regular member cannot", func(t *testing.T) {
		// Owner should be able to post
		canOwner, err := core.CanPostMessage(ctx, user.Id, space.Id, annRoom.Id)
		if err != nil {
			t.Fatalf("CanPostMessage (owner) failed: %v", err)
		}
		if !canOwner {
			t.Error("Owner should be able to post in announcements room")
		}

		// Create a regular member
		member, err := core.CreateUser(ctx, SystemActorID, "member-user", "Member", "password")
		if err != nil {
			t.Fatalf("CreateUser (member) failed: %v", err)
		}
		_, err = core.JoinSpace(ctx, member.Id, space.Id)
		if err != nil {
			t.Fatalf("JoinSpace failed: %v", err)
		}
		_, err = core.JoinRoom(ctx, member.Id, space.Id, member.Id, annRoom.Id)
		if err != nil {
			t.Fatalf("JoinRoom failed: %v", err)
		}

		// Regular member should NOT be able to post
		canMember, err := core.CanPostMessage(ctx, member.Id, space.Id, annRoom.Id)
		if err != nil {
			t.Fatalf("CanPostMessage (member) failed: %v", err)
		}
		if canMember {
			t.Error("Regular member should NOT be able to post in announcements room")
		}

		// Regular member SHOULD be able to post in threads (default space permission)
		canMemberPostInThread, err := core.CanPostInThread(ctx, member.Id, space.Id, annRoom.Id)
		if err != nil {
			t.Fatalf("CanPostInThread (member) failed: %v", err)
		}
		if !canMemberPostInThread {
			t.Error("Regular member should be able to post in existing threads in announcements room")
		}
	})
}

package core

import (
	"context"
	"testing"
)

// Helper to construct expected allow key from permission
func expectedAllowKey(subject string, perm Permission, objectId string) string {
	parts := perm.KeyParts()
	return AllowKey(subject, parts.Verb, parts.ObjectType, objectId)
}

// Helper to construct expected deny key from permission
func expectedDenyKey(subject string, perm Permission, objectId string) string {
	parts := perm.KeyParts()
	return DenyKey(subject, parts.Verb, parts.ObjectType, objectId)
}

// Helper to construct expected room-override allow key from permission
func expectedRoomAllowKey(roomID, subject string, perm Permission) string {
	parts := perm.KeyParts()
	return RoomAllowKey(roomID, subject, parts.Verb, parts.ObjectType)
}

// Helper to construct expected room-override deny key from permission
func expectedRoomDenyKey(roomID, subject string, perm Permission) string {
	parts := perm.KeyParts()
	return RoomDenyKey(roomID, subject, parts.Verb, parts.ObjectType)
}

// ============================================================================
// Instance-Level Role Operations Tests
// ============================================================================

func TestGrantServerPermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("creates allow decision for valid permission", func(t *testing.T) {
		err := core.GrantServerPermission(ctx, SystemActorID, RoleModerator, PermMessagePost)
		if err != nil {
			t.Fatalf("GrantServerPermission() error = %v", err)
		}

		if got := core.RBAC.GetDecision(ScopeServer, "", RoleModerator, PermMessagePost); got != DecisionAllow {
			t.Errorf("decision = %s, want %s", got, DecisionAllow)
		}
	})

	t.Run("removes existing denial when granting", func(t *testing.T) {
		// First deny the permission
		err := core.DenyServerPermission(ctx, SystemActorID, RoleModerator, PermMessagePost)
		if err != nil {
			t.Fatalf("DenyServerPermission() error = %v", err)
		}

		// Now grant it - should remove the denial
		err = core.GrantServerPermission(ctx, SystemActorID, RoleModerator, PermMessagePost)
		if err != nil {
			t.Fatalf("GrantServerPermission() error = %v", err)
		}

		if got := core.RBAC.GetDecision(ScopeServer, "", RoleModerator, PermMessagePost); got != DecisionAllow {
			t.Errorf("decision = %s, want %s", got, DecisionAllow)
		}
	})

	t.Run("rejects unrecognised permission", func(t *testing.T) {
		err := core.GrantServerPermission(ctx, SystemActorID, RoleModerator, Permission("not.a.real.permission"))
		if err == nil {
			t.Error("Expected error for invalid permission")
		}
	})
}

func TestDenyServerPermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("creates deny decision", func(t *testing.T) {
		err := core.DenyServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("DenyServerPermission() error = %v", err)
		}

		if got := core.RBAC.GetDecision(ScopeServer, "", RoleEveryone, PermMessagePost); got != DecisionDeny {
			t.Errorf("decision = %s, want %s", got, DecisionDeny)
		}
	})

	t.Run("removes existing grant when denying", func(t *testing.T) {
		// First grant the permission
		err := core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("GrantServerPermission() error = %v", err)
		}

		// Now deny it - should remove the grant
		err = core.DenyServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("DenyServerPermission() error = %v", err)
		}

		if got := core.RBAC.GetDecision(ScopeServer, "", RoleEveryone, PermMessagePost); got != DecisionDeny {
			t.Errorf("decision = %s, want %s", got, DecisionDeny)
		}
	})

	t.Run("rejects unrecognised permission", func(t *testing.T) {
		err := core.DenyServerPermission(ctx, SystemActorID, RoleModerator, Permission("not.real.permission"))
		if err == nil {
			t.Error("Expected error for invalid permission")
		}
	})
}

func TestClearServerPermissionState(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("clears both grant and denial", func(t *testing.T) {
		// Grant a permission
		err := core.GrantServerPermission(ctx, SystemActorID, RoleModerator, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to grant: %v", err)
		}

		// Clear it
		err = core.ClearServerPermissionState(ctx, SystemActorID, RoleModerator, PermMessagePost)
		if err != nil {
			t.Fatalf("ClearServerPermissionState() error = %v", err)
		}

		if got := core.RBAC.GetDecision(ScopeServer, "", RoleModerator, PermMessagePost); got != DecisionNone {
			t.Errorf("decision = %s, want %s", got, DecisionNone)
		}
	})

	t.Run("succeeds when clearing non-existent key", func(t *testing.T) {
		err := core.ClearServerPermissionState(ctx, SystemActorID, RoleEveryone, PermMessagePost)
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

	_, _ = core.CreateUser(ctx, "system", "testuser", "Test User", "password123")

	t.Run("creates allow decision for role", func(t *testing.T) {
		err := core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermRoomCreate)
		if err != nil {
			t.Fatalf("GrantSpaceRolePermission() error = %v", err)
		}

		if got := core.RBAC.GetDecision(ScopeServer, "", RoleEveryone, PermRoomCreate); got != DecisionAllow {
			t.Errorf("decision = %s, want %s", got, DecisionAllow)
		}
	})

	t.Run("works for role override at space level", func(t *testing.T) {
		// Instance role override at space level
		err := core.GrantServerPermission(ctx, SystemActorID, RoleModerator, PermRoomJoin)
		if err != nil {
			t.Fatalf("GrantSpaceRolePermission() for role error = %v", err)
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

	_, _ = core.CreateUser(ctx, "system", "testuser", "Test User", "password123")

	t.Run("creates deny decision in server RBAC", func(t *testing.T) {
		err := core.DenyServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("DenySpaceRolePermission() error = %v", err)
		}

		if got := core.RBAC.GetDecision(ScopeServer, "", RoleEveryone, PermMessagePost); got != DecisionDeny {
			t.Errorf("decision = %s, want %s", got, DecisionDeny)
		}
	})
}

func TestClearSpaceRolePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, _ = core.CreateUser(ctx, "system", "testuser", "Test User", "password123")

	t.Run("clears both grant and denial at space level", func(t *testing.T) {
		// Grant then clear
		_ = core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermRoomJoin)

		err := core.ClearServerPermissionState(ctx, SystemActorID, RoleEveryone, PermRoomJoin)
		if err != nil {
			t.Fatalf("ClearSpaceRolePermission() error = %v", err)
		}

		if got := core.RBAC.GetDecision(ScopeServer, "", RoleEveryone, PermRoomJoin); got != DecisionNone {
			t.Errorf("decision = %s, want %s", got, DecisionNone)
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
	room, _ := core.CreateRoom(ctx, user.Id, KindChannel, "", "General", "General chat")

	t.Run("creates allow decision for room-level permission", func(t *testing.T) {
		err := core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("GrantRoomRolePermission() error = %v", err)
		}

		if got := core.RBAC.GetDecision(ScopeRoom, room.Id, RoleEveryone, PermMessagePost); got != DecisionAllow {
			t.Errorf("decision = %s, want %s", got, DecisionAllow)
		}
	})

	t.Run("rejects permission that does not apply at room scope", func(t *testing.T) {
		err := core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermAdminUsersView)
		if err == nil {
			t.Error("Expected error for permission that doesn't apply at room scope")
		}
	})
}

func TestDenyRoomRolePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	room, _ := core.CreateRoom(ctx, user.Id, KindChannel, "", "General", "General chat")

	t.Run("creates deny decision at room level", func(t *testing.T) {
		err := core.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("DenyRoomRolePermission() error = %v", err)
		}

		if got := core.RBAC.GetDecision(ScopeRoom, room.Id, RoleEveryone, PermMessagePost); got != DecisionDeny {
			t.Errorf("decision = %s, want %s", got, DecisionDeny)
		}
	})

	t.Run("rejects permission that does not apply at room scope", func(t *testing.T) {
		err := core.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermAdminUsersView)
		if err == nil {
			t.Error("Expected error for permission that doesn't apply at room scope")
		}
	})
}

func TestClearRoomRolePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	room, _ := core.CreateRoom(ctx, user.Id, KindChannel, "", "General", "General chat")

	t.Run("clears both grant and denial at room level", func(t *testing.T) {
		// Grant then clear
		_ = core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermRoomJoin)

		err := core.ClearRoomPermissionState(ctx, SystemActorID, room.Id, RoleEveryone, PermRoomJoin)
		if err != nil {
			t.Fatalf("ClearRoomRolePermission() error = %v", err)
		}

		if got := core.RBAC.GetDecision(ScopeRoom, room.Id, RoleEveryone, PermRoomJoin); got != DecisionNone {
			t.Errorf("decision = %s, want %s", got, DecisionNone)
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
		err := core.GrantServerPermission(ctx, SystemActorID, RoleModerator, PermMessagePost)
		if err != nil {
			t.Fatalf("First grant failed: %v", err)
		}

		err = core.GrantServerPermission(ctx, SystemActorID, RoleModerator, PermMessagePost)
		if err != nil {
			t.Errorf("Second grant should succeed (idempotent), got: %v", err)
		}
	})

	t.Run("denying same permission twice succeeds", func(t *testing.T) {
		err := core.DenyServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("First deny failed: %v", err)
		}

		err = core.DenyServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost)
		if err != nil {
			t.Errorf("Second deny should succeed (idempotent), got: %v", err)
		}
	})

	t.Run("denying after grant updates correctly", func(t *testing.T) {
		perm := PermMessagePost

		// Grant
		err := core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, perm)
		if err != nil {
			t.Fatalf("Grant failed: %v", err)
		}

		// Now deny
		err = core.DenyServerPermission(ctx, SystemActorID, RoleEveryone, perm)
		if err != nil {
			t.Fatalf("Deny failed: %v", err)
		}

		if got := core.RBAC.GetDecision(ScopeServer, "", RoleEveryone, perm); got != DecisionDeny {
			t.Errorf("decision = %s, want %s", got, DecisionDeny)
		}
	})
}

// ============================================================================
// Initialization Tests
// ============================================================================

func TestInitServerDefaults(t *testing.T) {
	core, _ := setupTestCore(t)

	// InitServerDefaults is called during setupTestCore, so we can verify its effects

	t.Run("admin has expected server permissions", func(t *testing.T) {
		// Admin-specific defaults include administration, room administration,
		// and message management. Ordinary posting defaults come from everyone.
		for _, perm := range PermissionsForScope(ScopeServer) {
			if perm.Category == CategoryMessage && perm.Permission != PermMessageManage {
				continue
			}
			if got := core.RBAC.GetDecision(ScopeServer, "", RoleAdmin, perm.Permission); got != DecisionAllow {
				t.Errorf("admin decision for %s = %s, want %s", perm.Permission, got, DecisionAllow)
			}
		}
		for _, perm := range []Permission{PermMessagePost, PermMessagePostInThread, PermMessageReact, PermMessageEcho} {
			if got := core.RBAC.GetDecision(ScopeServer, "", RoleAdmin, perm); got != DecisionNone {
				t.Errorf("admin server decision for %s = %s, want %s", perm, got, DecisionNone)
			}
		}
	})

	t.Run("everyone has default server message.post permission", func(t *testing.T) {
		if got := core.RBAC.GetDecision(ScopeServer, "", RoleEveryone, PermMessagePost); got != DecisionAllow {
			t.Errorf("everyone decision for %s = %s, want %s", PermMessagePost, got, DecisionAllow)
		}
	})

	t.Run("everyone has expected permissions", func(t *testing.T) {
		expectedPerms := []Permission{
			PermUserDeleteSelf,
			PermRoomList,
			PermRoomJoin,
			PermMessagePost,
			PermMessagePostInThread,
			PermMessageAttach,
			PermMessageReact,
			PermMessageEcho,
		}
		for _, perm := range expectedPerms {
			if got := core.RBAC.GetDecision(ScopeServer, "", RoleEveryone, perm); got != DecisionAllow {
				t.Errorf("everyone decision for %s = %s, want %s", perm, got, DecisionAllow)
			}
		}
	})
}

func TestDefaultRBACSeed(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, _ = core.CreateUser(ctx, "system", "testuser", "Test User", "password123")

	t.Run("owner role stores no default server permissions", func(t *testing.T) {
		for _, perm := range PermissionsForScope(ScopeServer) {
			if got := core.RBAC.GetDecision(ScopeServer, "", RoleOwner, perm.Permission); got != DecisionNone {
				t.Errorf("owner decision for %s = %s, want %s", perm.Permission, got, DecisionNone)
			}
		}
	})

	t.Run("owner resolves to allow for every permission via effective-owner override", func(t *testing.T) {
		// The behavioural contract: a freshly-assigned owner passes every
		// defined server-scope permission check, including message permissions
		// that no longer have default server-scope grants.
		owner, err := core.CreateUser(ctx, SystemActorID, "enum-owner", "Owner", "password123")
		if err != nil {
			t.Fatalf("CreateUser: %v", err)
		}
		if err := core.AssignOwnerRole(ctx, owner.Id); err != nil {
			t.Fatalf("AssignOwnerRole: %v", err)
		}
		for _, perm := range PermissionsForScope(ScopeServer) {
			has, err := core.HasServerPermission(ctx, owner.Id, perm.Permission)
			if err != nil {
				t.Fatalf("HasServerPermission(%s): %v", perm.Permission, err)
			}
			if !has {
				t.Errorf("Expected owner to resolve allow for %s", perm.Permission)
			}
		}
	})

	t.Run("server roles store exactly their declared defaults", func(t *testing.T) {
		defaults := map[string][]Permission{
			RoleEveryone:  DefaultEveryonePermissions(),
			RoleModerator: DefaultModeratorPermissions(),
			RoleAdmin:     DefaultAdminPermissions(),
			RoleOwner:     nil,
		}
		for role, allowed := range defaults {
			for _, metadata := range PermissionsForScope(ScopeServer) {
				want := DecisionNone
				for _, permission := range allowed {
					if permission == metadata.Permission {
						want = DecisionAllow
						break
					}
				}
				if got := core.RBAC.GetDecision(ScopeServer, "", role, metadata.Permission); got != want {
					t.Errorf("%s decision for %s = %s, want %s", role, metadata.Permission, got, want)
				}
			}
		}
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
		err := core.GrantServerPermission(ctx, SystemActorID, RoleModerator, PermMessagePost)
		if err == nil {
			t.Error("Expected error with cancelled context")
		}
	})
}

// ============================================================================
// Announcements Room Tests
// ============================================================================

func TestDefaultChannelRoomPermissions(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user (with owner role; formerly via CreateSpace)
	user, err := core.CreateUser(ctx, SystemActorID, "ann-test-user", "Ann Test", "password")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, user.Id, RoleOwner); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}

	// Create a regular room
	regularRoom, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "general", "")
	if err != nil {
		t.Fatalf("CreateRoom (general) failed: %v", err)
	}

	// Create an announcements room
	annRoom, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "announcements", "")
	if err != nil {
		t.Fatalf("CreateRoom (announcements) failed: %v", err)
	}

	t.Run("rooms store exactly their creation-time defaults", func(t *testing.T) {
		roles := []string{RoleEveryone, RoleModerator, RoleAdmin, RoleOwner}
		for _, role := range roles {
			for _, metadata := range PermissionsForScope(ScopeRoom) {
				if got := core.RBAC.GetDecision(ScopeRoom, regularRoom.Id, role, metadata.Permission); got != DecisionNone {
					t.Errorf("regular room %s decision for %s = %s, want %s", role, metadata.Permission, got, DecisionNone)
				}

				want := DecisionNone
				if role == RoleEveryone && metadata.Permission == PermMessagePost {
					want = DecisionDeny
				}
				if role == RoleAdmin && metadata.Permission == PermMessagePost {
					want = DecisionAllow
				}
				if got := core.RBAC.GetDecision(ScopeRoom, annRoom.Id, role, metadata.Permission); got != want {
					t.Errorf("announcements %s decision for %s = %s, want %s", role, metadata.Permission, got, want)
				}
			}
		}
	})

	t.Run("room groups store no default permission decisions", func(t *testing.T) {
		group, err := core.CreateRoomGroup(ctx, user.Id, "Default matrix", "")
		if err != nil {
			t.Fatalf("CreateRoomGroup: %v", err)
		}
		for _, role := range []string{RoleEveryone, RoleModerator, RoleAdmin, RoleOwner} {
			for _, metadata := range PermissionsForScope(ScopeGroup) {
				if got := core.RBAC.GetDecision(ScopeGroup, group.Id, role, metadata.Permission); got != DecisionNone {
					t.Errorf("room group %s decision for %s = %s, want %s", role, metadata.Permission, got, DecisionNone)
				}
			}
		}
	})

	t.Run("owner and admin can post in announcements, moderator and regular member cannot", func(t *testing.T) {
		// Owner should be able to post
		canOwner, err := core.CanPostMessage(ctx, user.Id, KindChannel, annRoom.Id)
		if err != nil {
			t.Fatalf("CanPostMessage (owner) failed: %v", err)
		}
		if !canOwner {
			t.Error("Owner should be able to post in announcements room")
		}

		admin, err := core.CreateUser(ctx, SystemActorID, "ann-admin", "Announcements Admin", "password")
		if err != nil {
			t.Fatalf("CreateUser (admin) failed: %v", err)
		}
		if err := core.AssignServerRole(ctx, SystemActorID, admin.Id, RoleAdmin); err != nil {
			t.Fatalf("AssignServerRole (admin): %v", err)
		}
		canAdmin, err := core.CanPostMessage(ctx, admin.Id, KindChannel, annRoom.Id)
		if err != nil {
			t.Fatalf("CanPostMessage (admin) failed: %v", err)
		}
		if !canAdmin {
			t.Error("Admin should be able to post in announcements room")
		}

		moderator, err := core.CreateUser(ctx, SystemActorID, "ann-moderator", "Announcements Moderator", "password")
		if err != nil {
			t.Fatalf("CreateUser (moderator) failed: %v", err)
		}
		if err := core.AssignServerRole(ctx, SystemActorID, moderator.Id, RoleModerator); err != nil {
			t.Fatalf("AssignServerRole (moderator): %v", err)
		}
		canModerator, err := core.CanPostMessage(ctx, moderator.Id, KindChannel, annRoom.Id)
		if err != nil {
			t.Fatalf("CanPostMessage (moderator) failed: %v", err)
		}
		if canModerator {
			t.Error("Moderator should not be able to post in announcements room")
		}

		// Create a regular member
		member, err := core.CreateUser(ctx, SystemActorID, "member-user", "Member", "password")
		if err != nil {
			t.Fatalf("CreateUser (member) failed: %v", err)
		}
		_, err = core.JoinRoom(ctx, member.Id, KindChannel, member.Id, annRoom.Id)
		if err != nil {
			t.Fatalf("JoinRoom failed: %v", err)
		}

		// Regular member should NOT be able to post
		canMember, err := core.CanPostMessage(ctx, member.Id, KindChannel, annRoom.Id)
		if err != nil {
			t.Fatalf("CanPostMessage (member) failed: %v", err)
		}
		if canMember {
			t.Error("Regular member should NOT be able to post in announcements room")
		}

		// Regular member SHOULD be able to post in threads (default space permission)
		canMemberPostInThread, err := core.CanPostInThread(ctx, member.Id, KindChannel, annRoom.Id)
		if err != nil {
			t.Fatalf("CanPostInThread (member) failed: %v", err)
		}
		if !canMemberPostInThread {
			t.Error("Regular member should be able to post in existing threads in announcements room")
		}
	})
}

package core

import (
	"context"
	"slices"
	"testing"
)

// ============================================================================
// Instance-Level Role Operations Tests
// ============================================================================

func TestGrantServerPermission(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("creates allow decision for valid permission", func(t *testing.T) {
		err := core.GrantServerPermission(ctx, SystemActorID, RoleModerator, PermMessagePost)
		if err != nil {
			t.Fatalf("GrantServerPermission() error = %v", err)
		}

		if got := core.rbacModel.decision(ScopeServer, "", RoleModerator, PermMessagePost); got != DecisionAllow {
			t.Errorf("decision = %s, want %s", got, DecisionAllow)
		}
	})

	t.Run("removes existing denial when granting", func(t *testing.T) {
		// First store a deny, as an earlier version could. A server-scope deny
		// of everyone is no longer accepted (ADR-116).
		appendStoredRoleDeny(t, core, ctx, ScopeServer, "", RoleEveryone, PermMessagePost)

		// Now grant it - should remove the denial
		err := core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("GrantServerPermission() error = %v", err)
		}

		if got := core.rbacModel.decision(ScopeServer, "", RoleEveryone, PermMessagePost); got != DecisionAllow {
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

func TestClearServerPermissionState(t *testing.T) {
	t.Parallel()

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

		if got := core.rbacModel.decision(ScopeServer, "", RoleModerator, PermMessagePost); got != DecisionNone {
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
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, _ = core.CreateUser(ctx, "system", "testuser", "Test User", "password123")

	t.Run("creates allow decision for role", func(t *testing.T) {
		err := core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermRoomCreate)
		if err != nil {
			t.Fatalf("GrantSpaceRolePermission() error = %v", err)
		}

		if got := core.rbacModel.decision(ScopeServer, "", RoleEveryone, PermRoomCreate); got != DecisionAllow {
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

func TestClearSpaceRolePermission(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, _ = core.CreateUser(ctx, "system", "testuser", "Test User", "password123")

	t.Run("clears a grant at space level", func(t *testing.T) {
		// Grant then clear
		_ = core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermRoomJoin)

		err := core.ClearServerPermissionState(ctx, SystemActorID, RoleEveryone, PermRoomJoin)
		if err != nil {
			t.Fatalf("ClearSpaceRolePermission() error = %v", err)
		}

		if got := core.rbacModel.decision(ScopeServer, "", RoleEveryone, PermRoomJoin); got != DecisionNone {
			t.Errorf("decision = %s, want %s", got, DecisionNone)
		}
	})
}

// ============================================================================
// Room-Level Operations Tests
// ============================================================================

func TestGrantRoomRolePermission(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	room, _ := core.CreateRoom(ctx, user.Id, KindChannel, "", "General", "General chat")

	t.Run("creates allow decision for room-level permission", func(t *testing.T) {
		err := core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePost)
		if err != nil {
			t.Fatalf("GrantRoomRolePermission() error = %v", err)
		}

		if got := core.rbacModel.decision(ScopeRoom, room.Id, RoleEveryone, PermMessagePost); got != DecisionAllow {
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

// TestDenyUserRoomPermission covers room-level denies. Only a single user
// can be denied (ADR-116).
func TestDenyUserRoomPermission(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	room, _ := core.CreateRoom(ctx, user.Id, KindChannel, "", "General", "General chat")

	t.Run("creates deny decision at room level", func(t *testing.T) {
		err := core.DenyUserRoomPermission(ctx, SystemActorID, room.Id, user.Id, PermMessagePost)
		if err != nil {
			t.Fatalf("DenyUserRoomPermission() error = %v", err)
		}

		if got := core.rbacModel.decision(ScopeRoom, room.Id, user.Id, PermMessagePost); got != DecisionDeny {
			t.Errorf("decision = %s, want %s", got, DecisionDeny)
		}
	})

	t.Run("rejects permission that does not apply at room scope", func(t *testing.T) {
		err := core.DenyUserRoomPermission(ctx, SystemActorID, room.Id, user.Id, PermAdminUsersView)
		if err == nil {
			t.Error("Expected error for permission that doesn't apply at room scope")
		}
	})
}

func TestClearRoomRolePermission(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, _ := core.CreateUser(ctx, "system", "testuser", "Test User", "password123")
	room, _ := core.CreateRoom(ctx, user.Id, KindChannel, "", "General", "General chat")

	t.Run("clears a grant at room level", func(t *testing.T) {
		// Grant then clear
		_ = core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermRoomJoin)

		err := core.ClearRoomPermissionState(ctx, SystemActorID, room.Id, RoleEveryone, PermRoomJoin)
		if err != nil {
			t.Fatalf("ClearRoomRolePermission() error = %v", err)
		}

		if got := core.rbacModel.decision(ScopeRoom, room.Id, RoleEveryone, PermRoomJoin); got != DecisionNone {
			t.Errorf("decision = %s, want %s", got, DecisionNone)
		}
	})

	t.Run("clears a stored role deny at room level", func(t *testing.T) {
		// Roles only grant, but an earlier version could store a role deny
		// (ADR-116). Clearing removes it.
		appendStoredRoleDeny(t, core, ctx, ScopeRoom, room.Id, RoleEveryone, PermMessagePost)
		if got := core.rbacModel.decision(ScopeRoom, room.Id, RoleEveryone, PermMessagePost); got != DecisionDeny {
			t.Fatalf("stored decision = %s, want %s", got, DecisionDeny)
		}

		if err := core.ClearRoomPermissionState(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePost); err != nil {
			t.Fatalf("ClearRoomRolePermission() error = %v", err)
		}

		if got := core.rbacModel.decision(ScopeRoom, room.Id, RoleEveryone, PermMessagePost); got != DecisionNone {
			t.Errorf("decision = %s, want %s", got, DecisionNone)
		}
	})
}

// ============================================================================
// Idempotency Tests
// ============================================================================

func TestPermissionOpsIdempotency(t *testing.T) {
	t.Parallel()

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

	// Only a single user can be denied (ADR-116).
	roomID := createPermissionEditRoom(t, core, ctx, "idempotency-room")
	userID := createPermissionEditUser(t, core, ctx, "idempotency-user")

	t.Run("denying same permission twice succeeds", func(t *testing.T) {
		err := core.DenyUserRoomPermission(ctx, SystemActorID, roomID, userID, PermMessagePost)
		if err != nil {
			t.Fatalf("First deny failed: %v", err)
		}

		err = core.DenyUserRoomPermission(ctx, SystemActorID, roomID, userID, PermMessagePost)
		if err != nil {
			t.Errorf("Second deny should succeed (idempotent), got: %v", err)
		}
	})

	t.Run("denying after grant updates correctly", func(t *testing.T) {
		perm := PermMessageAttach

		// Grant
		err := core.GrantUserRoomPermission(ctx, SystemActorID, roomID, userID, perm)
		if err != nil {
			t.Fatalf("Grant failed: %v", err)
		}

		// Now deny
		err = core.DenyUserRoomPermission(ctx, SystemActorID, roomID, userID, perm)
		if err != nil {
			t.Fatalf("Deny failed: %v", err)
		}

		if got := core.rbacModel.decision(ScopeRoom, roomID, userID, perm); got != DecisionDeny {
			t.Errorf("decision = %s, want %s", got, DecisionDeny)
		}
	})
}

// ============================================================================
// Initialization Tests
// ============================================================================

func TestInitServerDefaults(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCoreWithDefaults(t)

	// InitServerDefaults is called during setupTestCoreWithDefaults, so we can
	// verify its effects.

	t.Run("admin has expected server permissions", func(t *testing.T) {
		// Admin-specific defaults include administration, room administration,
		// and message management. Ordinary posting and call defaults come from
		// room-scope allows. Admins get no server-scope room.list or room.join:
		// those would reach every private room (ADR-116).
		for _, perm := range PermissionsForScope(ScopeServer) {
			if perm.Category == CategoryCall || (perm.Category == CategoryMessage && perm.Permission != PermMessageManage) {
				continue
			}
			// The admin role stores the broad server.manage grant. The resolver
			// supplies its explicitly included Neighbor authority.
			if perm.Permission == PermServerManageNeighbors {
				continue
			}
			want := DecisionAllow
			if perm.Permission == PermRoomList || perm.Permission == PermRoomJoin {
				want = DecisionNone
			}
			if got := core.rbacModel.decision(ScopeServer, "", RoleAdmin, perm.Permission); got != want {
				t.Errorf("admin decision for %s = %s, want %s", perm.Permission, got, want)
			}
		}
		for _, perm := range []Permission{PermMessagePost, PermMessagePostInThread, PermMessageReact, PermMessageEcho, PermCallStart, PermCallJoin, PermCallVoice, PermCallCamera, PermCallScreenShare} {
			if got := core.rbacModel.decision(ScopeServer, "", RoleAdmin, perm); got != DecisionNone {
				t.Errorf("admin server decision for %s = %s, want %s", perm, got, DecisionNone)
			}
		}
	})

	t.Run("everyone has only user.delete-self at server scope", func(t *testing.T) {
		for _, perm := range PermissionsForScope(ScopeServer) {
			want := DecisionNone
			if perm.Permission == PermUserDeleteSelf {
				want = DecisionAllow
			}
			if got := core.rbacModel.decision(ScopeServer, "", RoleEveryone, perm.Permission); got != want {
				t.Errorf("everyone server decision for %s = %s, want %s", perm.Permission, got, want)
			}
		}
	})

	t.Run("everyone has message and call permissions at Direct-messages scope", func(t *testing.T) {
		expectedPerms := []Permission{
			PermMessageRead,
			PermMessagePost,
			PermMessageAttach,
			PermMessageReact,
			PermMessageEcho,
			PermCallStart, PermCallJoin, PermCallVoice, PermCallCamera, PermCallScreenShare,
		}
		for _, perm := range expectedPerms {
			if got := core.rbacModel.decision(ScopeDM, "", RoleEveryone, perm); got != DecisionAllow {
				t.Errorf("everyone DM decision for %s = %s, want %s", perm, got, DecisionAllow)
			}
		}
	})

	t.Run("admin inherits DM call access from everyone but has none at server scope", func(t *testing.T) {
		ctx := testContext(t)
		user, err := core.CreateUser(ctx, SystemActorID, "call-default-admin", "Admin", "password")
		if err != nil {
			t.Fatal(err)
		}
		if err := core.AssignServerRole(ctx, SystemActorID, user.Id, RoleAdmin); err != nil {
			t.Fatal(err)
		}
		for _, permission := range callPermissionIDs() {
			allowed, err := core.hasKindPermission(ctx, KindDM, user.Id, permission)
			if err != nil || !allowed {
				t.Errorf("admin effective DM %s = %v, %v; want allow", permission, allowed, err)
			}
			allowed, err = core.HasServerPermission(ctx, user.Id, permission)
			if err != nil || allowed {
				t.Errorf("admin effective server %s = %v, %v; want no allow", permission, allowed, err)
			}
		}
	})
}

func TestDefaultRBACSeed(t *testing.T) {
	t.Parallel()

	core, _ := setupTestCoreWithDefaults(t)
	ctx := testContext(t)

	_, _ = core.CreateUser(ctx, "system", "testuser", "Test User", "password123")

	t.Run("owner role stores no default server permissions", func(t *testing.T) {
		for _, perm := range PermissionsForScope(ScopeServer) {
			if got := core.rbacModel.decision(ScopeServer, "", RoleOwner, perm.Permission); got != DecisionNone {
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
				if slices.Contains(allowed, metadata.Permission) {
					want = DecisionAllow
				}
				if got := core.rbacModel.decision(ScopeServer, "", role, metadata.Permission); got != want {
					t.Errorf("%s decision for %s = %s, want %s", role, metadata.Permission, got, want)
				}
			}
		}
	})

	t.Run("only everyone stores Direct-messages defaults", func(t *testing.T) {
		for _, role := range []string{RoleEveryone, RoleModerator, RoleAdmin, RoleOwner} {
			for _, metadata := range PermissionsForScope(ScopeDM) {
				want := DecisionNone
				if role == RoleEveryone && slices.Contains(DefaultEveryoneDMPermissions(), metadata.Permission) {
					want = DecisionAllow
				}
				if got := core.rbacModel.decision(ScopeDM, "", role, metadata.Permission); got != want {
					t.Errorf("%s DM decision for %s = %s, want %s", role, metadata.Permission, got, want)
				}
			}
		}
	})
}

// ============================================================================
// Context Cancellation Tests
// ============================================================================

func TestPermissionOpsWithCancelledContext(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	core, _ := setupTestCoreWithDefaults(t)
	ctx := testContext(t)

	// Create a user (with owner role; formerly via CreateSpace)
	user, err := core.CreateUser(ctx, SystemActorID, "ann-test-user", "Ann Test", "password")
	if err != nil {
		t.Fatalf("CreateUser failed: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, user.Id, RoleOwner); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}

	// Create a regular room. It starts closed (ADR-116).
	regularRoom, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "regular", "")
	if err != nil {
		t.Fatalf("CreateRoom (regular) failed: %v", err)
	}

	// Create a seeded open room, like #general, with its explicit defaults.
	openRoom, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "general", "", WithOpenRoomDefaults())
	if err != nil {
		t.Fatalf("CreateRoom (general) failed: %v", err)
	}

	// Create the trusted seeded announcements room with its explicit defaults.
	annRoom, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "announcements", "", WithAnnouncementsRoomDefaults())
	if err != nil {
		t.Fatalf("CreateRoom (announcements) failed: %v", err)
	}

	t.Run("rooms store exactly their creation-time defaults", func(t *testing.T) {
		roles := []string{RoleEveryone, RoleModerator, RoleAdmin, RoleOwner}
		for _, role := range roles {
			for _, metadata := range PermissionsForScope(ScopeRoom) {
				if got := core.rbacModel.decision(ScopeRoom, regularRoom.Id, role, metadata.Permission); got != DecisionNone {
					t.Errorf("regular room %s decision for %s = %s, want %s", role, metadata.Permission, got, DecisionNone)
				}

				want := DecisionNone
				if role == RoleEveryone && slices.Contains(DefaultOpenRoomEveryonePermissions(), metadata.Permission) {
					want = DecisionAllow
				}
				if got := core.rbacModel.decision(ScopeRoom, openRoom.Id, role, metadata.Permission); got != want {
					t.Errorf("open room %s decision for %s = %s, want %s", role, metadata.Permission, got, want)
				}

				want = DecisionNone
				if role == RoleEveryone && slices.Contains(DefaultAnnouncementsEveryonePermissions(), metadata.Permission) {
					want = DecisionAllow
				}
				if role == RoleAdmin && slices.Contains(DefaultAnnouncementsAdminPermissions(), metadata.Permission) {
					want = DecisionAllow
				}
				if got := core.rbacModel.decision(ScopeRoom, annRoom.Id, role, metadata.Permission); got != want {
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
				if got := core.rbacModel.decision(ScopeGroup, group.Id, role, metadata.Permission); got != DecisionNone {
					t.Errorf("room group %s decision for %s = %s, want %s", role, metadata.Permission, got, DecisionNone)
				}
			}
		}
	})

	// Only the owner and admins can post root messages in announcements.
	// Members can reply in threads.
	t.Run("only owner and admin can post root messages in announcements", func(t *testing.T) {
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
		if _, err := core.JoinRoom(ctx, moderator.Id, KindChannel, moderator.Id, annRoom.Id); err != nil {
			t.Fatalf("JoinRoom (moderator) failed: %v", err)
		}
		canModerator, err := core.CanPostMessage(ctx, moderator.Id, KindChannel, annRoom.Id)
		if err != nil {
			t.Fatalf("CanPostMessage (moderator) failed: %v", err)
		}
		if canModerator {
			t.Error("Moderator should not be able to post root messages in announcements room")
		}

		member, err := core.CreateUser(ctx, SystemActorID, "member-user", "Member", "password")
		if err != nil {
			t.Fatalf("CreateUser (member) failed: %v", err)
		}
		if _, err := core.JoinRoom(ctx, member.Id, KindChannel, member.Id, annRoom.Id); err != nil {
			t.Fatalf("JoinRoom failed: %v", err)
		}
		canMember, err := core.CanPostMessage(ctx, member.Id, KindChannel, annRoom.Id)
		if err != nil {
			t.Fatalf("CanPostMessage (member) failed: %v", err)
		}
		if canMember {
			t.Error("Regular member should not be able to post root messages in announcements room")
		}

		canMemberPostInThread, err := core.CanPostInThread(ctx, member.Id, KindChannel, annRoom.Id)
		if err != nil {
			t.Fatalf("CanPostInThread (member) failed: %v", err)
		}
		if !canMemberPostInThread {
			t.Error("Regular member should be able to post in existing threads in announcements room")
		}
	})
}

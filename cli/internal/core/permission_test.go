package core

import (
	"slices"
	"testing"
)

// ============================================================================
// GetPermissionMetadata Tests
// ============================================================================

func TestGetPermissionMetadata(t *testing.T) {
	t.Run("returns correct metadata for known permission", func(t *testing.T) {
		meta, ok := GetPermissionMetadata(PermAdminUsersView)
		if !ok {
			t.Fatal("Expected to find metadata for admin.view-users")
		}
		if meta.Permission != PermAdminUsersView {
			t.Errorf("Permission = %v, want %v", meta.Permission, PermAdminUsersView)
		}
		if meta.Category != CategoryAdmin {
			t.Errorf("Category = %v, want %v", meta.Category, CategoryAdmin)
		}
		if len(meta.Scopes) != 1 || meta.Scopes[0] != ScopeServer {
			t.Errorf("Scopes = %v, want [server]", meta.Scopes)
		}
	})

	t.Run("returns false for unknown permission", func(t *testing.T) {
		_, ok := GetPermissionMetadata("nonexistent.permission")
		if ok {
			t.Error("Expected false for unknown permission")
		}
	})

	t.Run("returns correct metadata for room-overridable permission", func(t *testing.T) {
		meta, ok := GetPermissionMetadata(PermMessagePost)
		if !ok {
			t.Fatal("Expected to find metadata for message.post")
		}
		if !slices.Contains(meta.Scopes, ScopeServer) {
			t.Error("Expected message.post to apply at server scope")
		}
		if !slices.Contains(meta.Scopes, ScopeRoom) {
			t.Error("Expected message.post to apply at room scope (overridable)")
		}
	})
}

// ============================================================================
// ValidatePermission Tests
// ============================================================================

func TestValidatePermission(t *testing.T) {
	t.Run("accepts valid permissions", func(t *testing.T) {
		validPerms := []Permission{
			PermMessagePost,
			PermAdminUsersView,
			PermUserDeleteSelf,
		}

		for _, perm := range validPerms {
			if err := ValidatePermission(perm); err != nil {
				t.Errorf("ValidatePermission(%v) returned error: %v", perm, err)
			}
		}
	})

	t.Run("rejects invalid permissions", func(t *testing.T) {
		invalidPerms := []Permission{
			"invalid.permission",
			"server",
			"",
			"server.nonexistent",
		}

		for _, perm := range invalidPerms {
			if err := ValidatePermission(perm); err == nil {
				t.Errorf("ValidatePermission(%v) should have returned error", perm)
			}
		}
	})
}

func TestValidatePermissionString(t *testing.T) {
	t.Run("accepts valid permission string", func(t *testing.T) {
		if err := ValidatePermissionString("message.post"); err != nil {
			t.Errorf("ValidatePermissionString returned error: %v", err)
		}
	})

	t.Run("rejects invalid permission string", func(t *testing.T) {
		if err := ValidatePermissionString("invalid.perm"); err == nil {
			t.Error("ValidatePermissionString should have returned error for invalid permission")
		}
	})
}

// ============================================================================
// PermissionAppliesAtScope Tests
// ============================================================================

func TestPermissionAppliesAtScope(t *testing.T) {
	testCases := []struct {
		name       string
		permission Permission
		scope      PermissionScope
		expected   bool
	}{
		// Server-only permissions
		{"admin.view-users at server", PermAdminUsersView, ScopeServer, true},
		{"admin.view-users at room", PermAdminUsersView, ScopeRoom, false},
		{"server.manage at server", PermServerManage, ScopeServer, true},
		{"server.manage at room", PermServerManage, ScopeRoom, false},
		{"role.manage at server", PermRoleManage, ScopeServer, true},
		{"role.manage at room", PermRoleManage, ScopeRoom, false},

		// Room-overridable permissions
		{"message.post at server", PermMessagePost, ScopeServer, true},
		{"message.post at group", PermMessagePost, ScopeGroup, true},
		{"message.post at room", PermMessagePost, ScopeRoom, true},
		{"message.attach at server", PermMessageAttach, ScopeServer, true},
		{"message.attach at group", PermMessageAttach, ScopeGroup, true},
		{"message.attach at room", PermMessageAttach, ScopeRoom, true},
		{"room.join at server", PermRoomJoin, ScopeServer, true},
		{"room.join at room", PermRoomJoin, ScopeRoom, true},
		{"room.manage at server", PermRoomManage, ScopeServer, true},
		{"room.manage at room", PermRoomManage, ScopeRoom, true},
		{"room.ban-member at server", PermRoomMemberBan, ScopeServer, true},
		{"room.ban-member at room", PermRoomMemberBan, ScopeRoom, true},
		{"message.manage at room", PermMessageManage, ScopeRoom, true},
		{"room.create at server", PermRoomCreate, ScopeServer, true},
		{"room.create at group", PermRoomCreate, ScopeGroup, true},

		// Unknown permission
		{"unknown at server", "unknown.permission", ScopeServer, false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := PermissionAppliesAtScope(tc.permission, tc.scope)
			if result != tc.expected {
				t.Errorf("PermissionAppliesAtScope(%v, %v) = %v, want %v",
					tc.permission, tc.scope, result, tc.expected)
			}
		})
	}
}

// ============================================================================
// PermissionsForScope Tests
// ============================================================================

func TestPermissionsForScope(t *testing.T) {
	t.Run("server scope returns every defined permission", func(t *testing.T) {
		perms := PermissionsForScope(ScopeServer)
		if len(perms) != len(AllPermissions()) {
			t.Errorf("server scope = %d perms, want all %d", len(perms), len(AllPermissions()))
		}
	})

	t.Run("room scope returns only room-overridable permissions", func(t *testing.T) {
		perms := PermissionsForScope(ScopeRoom)

		found := func(target Permission) bool {
			for _, p := range perms {
				if p.Permission == target {
					return true
				}
			}
			return false
		}

		if !found(PermMessagePost) {
			t.Error("Expected message.post in room permissions")
		}
		if !found(PermRoomManage) {
			t.Error("Expected room.manage in room permissions")
		}
		if !found(PermRoomMemberBan) {
			t.Error("Expected room.ban-member in room permissions")
		}
		if found(PermAdminUsersView) {
			t.Error("admin.view-users should NOT be in room permissions")
		}
		if found(PermServerManage) {
			t.Error("server.manage should NOT be in room permissions")
		}
	})
}

// ============================================================================
// PermissionsForCategory Tests
// ============================================================================

func TestPermissionsForCategory(t *testing.T) {
	t.Run("returns server category permissions", func(t *testing.T) {
		perms := PermissionsForCategory(CategoryServer)
		if len(perms) == 0 {
			t.Fatal("Expected at least one server-category permission")
		}
		found := false
		for _, p := range perms {
			if p.Permission == PermServerManage {
				found = true
			}
			if p.Category != CategoryServer {
				t.Errorf("Permission %v has category %v, expected %v",
					p.Permission, p.Category, CategoryServer)
			}
		}
		if !found {
			t.Error("Expected server.manage in server category")
		}
	})

	t.Run("returns admin category permissions", func(t *testing.T) {
		perms := PermissionsForCategory(CategoryAdmin)
		if len(perms) == 0 {
			t.Fatal("Expected at least one admin permission")
		}

		for _, p := range perms {
			if p.Category != CategoryAdmin {
				t.Errorf("Permission %v has category %v, expected %v",
					p.Permission, p.Category, CategoryAdmin)
			}
		}

		foundAdminUsersView := false
		for _, p := range perms {
			if p.Permission == PermAdminUsersView {
				foundAdminUsersView = true
			}
		}
		if !foundAdminUsersView {
			t.Error("Expected admin.view-users in admin category")
		}
	})

	t.Run("returns empty for nonexistent category", func(t *testing.T) {
		perms := PermissionsForCategory("nonexistent")
		if len(perms) != 0 {
			t.Errorf("Expected empty result for nonexistent category, got %d permissions", len(perms))
		}
	})
}

// ============================================================================
// Default Permissions Tests
// ============================================================================

func TestDefaultEveryonePermissions(t *testing.T) {
	perms := DefaultEveryonePermissions()

	mustInclude := []Permission{
		PermUserDeleteSelf,
		PermRoomList,
		PermRoomJoin,
		PermMessagePost,
		PermMessagePostInThread,
		PermMessageReact,
		PermMessageEcho,
	}
	for _, want := range mustInclude {
		if !slices.Contains(perms, want) {
			t.Errorf("Expected %v in everyone defaults", want)
		}
	}

	// Admin-level and seed-only permissions must not leak into the boot backfill list.
	for _, mustNotInclude := range []Permission{PermServerManage, PermRoleManage, PermRoomCreate, PermAdminUsersView, PermMessageAttach} {
		if slices.Contains(perms, mustNotInclude) {
			t.Errorf("everyone defaults must not include %v", mustNotInclude)
		}
	}
}

func TestDefaultSeedEveryonePermissions(t *testing.T) {
	perms := DefaultSeedEveryonePermissions()

	for _, want := range append(DefaultEveryonePermissions(), PermMessageAttach) {
		if !slices.Contains(perms, want) {
			t.Errorf("Expected %v in fresh seed everyone defaults", want)
		}
	}
}

func TestDefaultModeratorPermissions(t *testing.T) {
	perms := DefaultModeratorPermissions()

	mustInclude := []Permission{
		PermMessageManage,
		PermRoomMemberBan,
	}
	for _, want := range mustInclude {
		if !slices.Contains(perms, want) {
			t.Errorf("Expected %v in moderator defaults", want)
		}
	}

	// Moderators can moderate content and bans, but do not get admin visibility
	// or general server administration by default.
	for _, mustNotInclude := range []Permission{PermAdminUsersView, PermRoomCreate, PermRoomManage, PermServerManage, PermRoleManage} {
		if slices.Contains(perms, mustNotInclude) {
			t.Errorf("moderator defaults must not include %v", mustNotInclude)
		}
	}
}

func TestDefaultRoomEveryonePermissions(t *testing.T) {
	perms := DefaultRoomEveryonePermissions()

	if len(perms) != 0 {
		t.Errorf("Expected room everyone defaults to be empty, got %v", perms)
	}
}

// ============================================================================
// AllPermissions Tests
// ============================================================================

func TestAllPermissions(t *testing.T) {
	perms := AllPermissions()

	if len(perms) == 0 {
		t.Fatal("AllPermissions returned empty list")
	}

	for _, p := range perms {
		if p.Permission == "" {
			t.Error("Found permission with empty Permission field")
		}
		if p.DisplayName == "" {
			t.Errorf("Permission %v has empty DisplayName", p.Permission)
		}
		if p.Description == "" {
			t.Errorf("Permission %v has empty Description", p.Permission)
		}
		if p.Category == "" {
			t.Errorf("Permission %v has empty Category", p.Permission)
		}
		if len(p.Scopes) == 0 {
			t.Errorf("Permission %v has no scopes defined", p.Permission)
		}
	}
}

// ============================================================================
// Consistency Tests
// ============================================================================

func TestPermissionConsistency(t *testing.T) {
	t.Run("everyone defaults are valid", func(t *testing.T) {
		for _, perm := range DefaultEveryonePermissions() {
			if err := ValidatePermission(perm); err != nil {
				t.Errorf("Invalid permission in everyone defaults: %v", perm)
			}
		}
	})

	t.Run("everyone seed defaults are valid", func(t *testing.T) {
		for _, perm := range DefaultSeedEveryonePermissions() {
			if err := ValidatePermission(perm); err != nil {
				t.Errorf("Invalid permission in everyone seed defaults: %v", perm)
			}
		}
	})

	t.Run("moderator defaults are valid", func(t *testing.T) {
		for _, perm := range DefaultModeratorPermissions() {
			if err := ValidatePermission(perm); err != nil {
				t.Errorf("Invalid permission in moderator defaults: %v", perm)
			}
		}
	})

	t.Run("admin defaults are valid", func(t *testing.T) {
		for _, perm := range DefaultAdminPermissions() {
			if err := ValidatePermission(perm); err != nil {
				t.Errorf("Invalid permission in admin defaults: %v", perm)
			}
		}
	})

	t.Run("admin defaults grant room administration and message management", func(t *testing.T) {
		for _, want := range []Permission{
			PermRoomCreate,
			PermRoomJoin,
			PermRoomList,
			PermRoomManage,
			PermRoomMemberBan,
			PermMessageManage,
			PermUserManageAccounts,
		} {
			if !slices.Contains(DefaultAdminPermissions(), want) {
				t.Errorf("admin server defaults should include %v", want)
			}
		}
		for _, mustNotInclude := range []Permission{
			PermMessagePost,
			PermMessagePostInThread,
			PermMessageAttach,
			PermMessageReact,
			PermMessageEcho,
		} {
			if slices.Contains(DefaultAdminPermissions(), mustNotInclude) {
				t.Errorf("admin-specific defaults should rely on everyone for %v", mustNotInclude)
			}
		}
	})
}

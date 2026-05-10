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
		meta, ok := GetPermissionMetadata(PermAdminAccess)
		if !ok {
			t.Fatal("Expected to find metadata for admin.access")
		}
		if meta.Permission != PermAdminAccess {
			t.Errorf("Permission = %v, want %v", meta.Permission, PermAdminAccess)
		}
		if meta.Category != CategoryAdmin {
			t.Errorf("Category = %v, want %v", meta.Category, CategoryAdmin)
		}
		if len(meta.Scopes) != 1 || meta.Scopes[0] != ScopeInstance {
			t.Errorf("Scopes = %v, want [instance]", meta.Scopes)
		}
	})

	t.Run("returns false for unknown permission", func(t *testing.T) {
		_, ok := GetPermissionMetadata("nonexistent.permission")
		if ok {
			t.Error("Expected false for unknown permission")
		}
	})

	t.Run("returns correct metadata for admin permission", func(t *testing.T) {
		meta, ok := GetPermissionMetadata(PermAdminAccess)
		if !ok {
			t.Fatal("Expected to find metadata for admin.access")
		}
		if meta.Category != CategoryAdmin {
			t.Errorf("Category = %v, want %v", meta.Category, CategoryAdmin)
		}
		if !slices.Contains(meta.Scopes, ScopeInstance) {
			t.Error("Expected admin.access to apply at instance scope")
		}
	})

	t.Run("returns correct metadata for multi-scope permission", func(t *testing.T) {
		meta, ok := GetPermissionMetadata(PermMessagePost)
		if !ok {
			t.Fatal("Expected to find metadata for message.post")
		}
		// message.post should apply at instance, space, and room scopes
		if len(meta.Scopes) != 3 {
			t.Errorf("Expected 3 scopes, got %d", len(meta.Scopes))
		}
		if !slices.Contains(meta.Scopes, ScopeInstance) {
			t.Error("Expected message.post to apply at instance scope")
		}
		if !slices.Contains(meta.Scopes, ScopeSpace) {
			t.Error("Expected message.post to apply at space scope")
		}
		if !slices.Contains(meta.Scopes, ScopeRoom) {
			t.Error("Expected message.post to apply at room scope")
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
			PermAdminAccess,
			PermDMView,
			PermDMWrite,
		}

		for _, perm := range validPerms {
			err := ValidatePermission(perm)
			if err != nil {
				t.Errorf("ValidatePermission(%v) returned error: %v", perm, err)
			}
		}
	})

	t.Run("rejects invalid permissions", func(t *testing.T) {
		invalidPerms := []Permission{
			"invalid.permission",
			"space",
			"",
			"space.nonexistent",
		}

		for _, perm := range invalidPerms {
			err := ValidatePermission(perm)
			if err == nil {
				t.Errorf("ValidatePermission(%v) should have returned error", perm)
			}
		}
	})
}

func TestValidatePermissionString(t *testing.T) {
	t.Run("accepts valid permission string", func(t *testing.T) {
		err := ValidatePermissionString("dm.view")
		if err != nil {
			t.Errorf("ValidatePermissionString returned error: %v", err)
		}
	})

	t.Run("rejects invalid permission string", func(t *testing.T) {
		err := ValidatePermissionString("invalid.perm")
		if err == nil {
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
		// Instance-only permissions
		{"admin.access at instance", PermAdminAccess, ScopeInstance, true},
		{"admin.access at space", PermAdminAccess, ScopeSpace, false},
		{"dm.view at instance", PermDMView, ScopeInstance, true},
		{"dm.view at space", PermDMView, ScopeSpace, false},

		// Space-only permissions
		{"space.manage at instance", PermSpaceManage, ScopeInstance, false},
		{"space.manage at space", PermSpaceManage, ScopeSpace, true},
		{"space.manage at room", PermSpaceManage, ScopeRoom, false},
		{"role.manage at space", PermRoleManage, ScopeSpace, true},
		{"role.manage at instance", PermRoleManage, ScopeInstance, false},

		// Multi-scope permissions
		{"message.post at instance", PermMessagePost, ScopeInstance, true},
		{"message.post at space", PermMessagePost, ScopeSpace, true},
		{"message.post at room", PermMessagePost, ScopeRoom, true},
		{"room.join at instance", PermRoomJoin, ScopeInstance, true},
		{"room.join at space", PermRoomJoin, ScopeSpace, true},
		{"room.join at room", PermRoomJoin, ScopeRoom, true},

		// Moderation permissions (instance, space, room)
		{"room.manage at instance", PermRoomManage, ScopeInstance, true},
		{"room.manage at space", PermRoomManage, ScopeSpace, true},
		{"room.manage at room", PermRoomManage, ScopeRoom, true},
		{"message.edit-any at instance", PermMessageEditAny, ScopeInstance, true},
		{"message.edit-any at space", PermMessageEditAny, ScopeSpace, true},
		{"message.delete-any at instance", PermMessageDeleteAny, ScopeInstance, true},
		{"message.delete-any at space", PermMessageDeleteAny, ScopeSpace, true},
		{"message.delete-any at room", PermMessageDeleteAny, ScopeRoom, true},

		// Unknown permission
		{"unknown at instance", "unknown.permission", ScopeInstance, false},
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
	t.Run("returns instance-applicable permissions", func(t *testing.T) {
		perms := PermissionsForScope(ScopeInstance)

		foundDMView := false
		foundAdminAccess := false
		for _, p := range perms {
			if p.Permission == PermDMView {
				foundDMView = true
			}
			if p.Permission == PermAdminAccess {
				foundAdminAccess = true
			}
		}
		if !foundDMView {
			t.Error("Expected dm.view in instance permissions")
		}
		if !foundAdminAccess {
			t.Error("Expected admin.access in instance permissions")
		}

		// Should NOT include space-only permissions
		for _, p := range perms {
			if p.Permission == PermSpaceManage {
				t.Error("space.manage should NOT be in instance permissions")
			}
			if p.Permission == PermRoleManage {
				t.Error("role.manage should NOT be in instance permissions")
			}
		}
	})

	t.Run("returns space-applicable permissions", func(t *testing.T) {
		perms := PermissionsForScope(ScopeSpace)

		// Should include space-only permissions
		foundSpaceManage := false
		foundRoleManage := false
		foundMessagePost := false
		for _, p := range perms {
			if p.Permission == PermSpaceManage {
				foundSpaceManage = true
			}
			if p.Permission == PermRoleManage {
				foundRoleManage = true
			}
			if p.Permission == PermMessagePost {
				foundMessagePost = true
			}
		}
		if !foundSpaceManage {
			t.Error("Expected space.manage in space permissions")
		}
		if !foundRoleManage {
			t.Error("Expected role.manage in space permissions")
		}
		if !foundMessagePost {
			t.Error("Expected message.post in space permissions (multi-scope)")
		}

		// Should NOT include instance-only permissions
		for _, p := range perms {
			if p.Permission == PermAdminAccess {
				t.Error("admin.access should NOT be in space permissions")
			}
		}
	})

	t.Run("returns room-applicable permissions", func(t *testing.T) {
		perms := PermissionsForScope(ScopeRoom)

		// Should include room-level permissions
		foundMessagePost := false
		foundRoomJoin := false
		foundRoomManage := false
		for _, p := range perms {
			if p.Permission == PermMessagePost {
				foundMessagePost = true
			}
			if p.Permission == PermRoomJoin {
				foundRoomJoin = true
			}
			if p.Permission == PermRoomManage {
				foundRoomManage = true
			}
		}
		if !foundMessagePost {
			t.Error("Expected message.post in room permissions")
		}
		if !foundRoomJoin {
			t.Error("Expected room.join in room permissions")
		}
		if !foundRoomManage {
			t.Error("Expected room.manage in room permissions")
		}

		// Should NOT include space-only or instance-only permissions
		for _, p := range perms {
			if p.Permission == PermSpaceManage {
				t.Error("space.manage should NOT be in room permissions")
			}
			if p.Permission == PermAdminAccess {
				t.Error("admin.access should NOT be in room permissions")
			}
		}
	})
}

// ============================================================================
// PermissionsForCategory Tests
// ============================================================================

func TestPermissionsForCategory(t *testing.T) {
	t.Run("returns space category permissions", func(t *testing.T) {
		perms := PermissionsForCategory(CategorySpace)

		// Should include all space permissions
		expectedPerms := []Permission{
			PermSpaceManage, PermSpaceDelete,
		}
		for _, expected := range expectedPerms {
			found := false
			for _, p := range perms {
				if p.Permission == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Expected %v in space category permissions", expected)
			}
		}

		// All returned permissions should be in space category
		for _, p := range perms {
			if p.Category != CategorySpace {
				t.Errorf("Permission %v has category %v, expected %v",
					p.Permission, p.Category, CategorySpace)
			}
		}
	})

	t.Run("returns admin category permissions", func(t *testing.T) {
		perms := PermissionsForCategory(CategoryAdmin)

		if len(perms) == 0 {
			t.Fatal("Expected at least one admin permission")
		}

		// All returned permissions should be in admin category
		for _, p := range perms {
			if p.Category != CategoryAdmin {
				t.Errorf("Permission %v has category %v, expected %v",
					p.Permission, p.Category, CategoryAdmin)
			}
		}

		// Should include specific admin permissions
		foundAdminAccess := false
		foundAdminUsersView := false
		for _, p := range perms {
			if p.Permission == PermAdminAccess {
				foundAdminAccess = true
			}
			if p.Permission == PermAdminUsersView {
				foundAdminUsersView = true
			}
		}
		if !foundAdminAccess {
			t.Error("Expected admin.access in admin category")
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

func TestDefaultInstanceEveryonePermissions_DetailedChecks(t *testing.T) {
	perms := DefaultInstanceEveryonePermissions()

	expectedPerms := []Permission{
		PermUserDeleteSelf,
		PermDMView,
		PermDMWrite,
	}
	for _, expected := range expectedPerms {
		if !slices.Contains(perms, expected) {
			t.Errorf("Expected %v in instance-everyone defaults", expected)
		}
	}

	// Should NOT include admin permissions
	for _, p := range perms {
		meta, _ := GetPermissionMetadata(p)
		if meta.Category == CategoryAdmin {
			t.Errorf("instance-everyone should not have admin permission: %v", p)
		}
	}
}

func TestDefaultSpaceEveryonePermissions(t *testing.T) {
	perms := DefaultSpaceEveryonePermissions()

	// Should include basic member permissions
	expectedPerms := []Permission{
		PermRoomList,
		PermRoomJoin,
		PermRoomLeave,
		PermMessagePost,
		PermMessagePostInThread,
		PermMessageReply,
		PermMessageReplyInThread,
	}
	for _, expected := range expectedPerms {
		if !slices.Contains(perms, expected) {
			t.Errorf("Expected %v in space-everyone defaults", expected)
		}
	}

	// Should NOT include admin-level or opt-in permissions
	if slices.Contains(perms, PermSpaceManage) {
		t.Error("space-everyone should not have space.manage")
	}
	if slices.Contains(perms, PermRoleManage) {
		t.Error("space-everyone should not have role.manage")
	}
	if slices.Contains(perms, PermRoomCreate) {
		t.Error("space-everyone should not have room.create (opt-in only)")
	}
}

func TestDefaultSpaceModeratorPermissions(t *testing.T) {
	perms := DefaultSpaceModeratorPermissions()

	// Should include moderator powers
	expectedPerms := []Permission{
		PermRoomManage,
		PermMemberRemove,
		PermMessageDeleteAny,
	}
	for _, expected := range expectedPerms {
		if !slices.Contains(perms, expected) {
			t.Errorf("Expected %v in space-moderator defaults", expected)
		}
	}
}

// ============================================================================
// Role Naming Tests
// ============================================================================

func TestScopedRoleName(t *testing.T) {
	testCases := []struct {
		scope    PermissionScope
		roleName string
		expected string
	}{
		{ScopeInstance, "admin", "instance.admin"},
		{ScopeInstance, "verified", "instance.verified"},
		{ScopeInstance, "everyone", "instance.everyone"},
		{ScopeSpace, "admin", "space.admin"},
		{ScopeSpace, "everyone", "space.everyone"},
		{ScopeSpace, "moderator", "space.moderator"},
		{ScopeRoom, "custom-role", "room.custom-role"},
	}

	for _, tc := range testCases {
		t.Run(tc.expected, func(t *testing.T) {
			result := ScopedRoleName(tc.scope, tc.roleName)
			if result != tc.expected {
				t.Errorf("ScopedRoleName(%v, %v) = %v, want %v",
					tc.scope, tc.roleName, result, tc.expected)
			}
		})
	}
}

func TestParseScopedRoleName(t *testing.T) {
	testCases := []struct {
		input        string
		expectedScope PermissionScope
		expectedRole  string
	}{
		{"instance.admin", ScopeInstance, "admin"},
		{"instance.verified", ScopeInstance, "verified"},
		{"instance.everyone", ScopeInstance, "everyone"},
		{"space.admin", ScopeSpace, "admin"},
		{"space.everyone", ScopeSpace, "everyone"},
		{"space.moderator", ScopeSpace, "moderator"},
		{"room.custom-role", ScopeRoom, "custom-role"},
		// Edge cases
		{"invalid", "", ""},                // No separator
		{"", "", ""},                       // Empty string
		{".admin", "", "admin"},            // Empty scope
		{"instance.", ScopeInstance, ""},   // Empty role name
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			scope, roleName := ParseScopedRoleName(tc.input)
			if scope != tc.expectedScope {
				t.Errorf("ParseScopedRoleName(%v) scope = %v, want %v",
					tc.input, scope, tc.expectedScope)
			}
			if roleName != tc.expectedRole {
				t.Errorf("ParseScopedRoleName(%v) roleName = %v, want %v",
					tc.input, roleName, tc.expectedRole)
			}
		})
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

	// Verify all permissions have required fields
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

	// Check for expected count (should be around 32 permissions)
	if len(perms) < 25 {
		t.Errorf("Expected at least 25 permissions, got %d", len(perms))
	}
}

// ============================================================================
// Consistency Tests
// ============================================================================

func TestPermissionConsistency(t *testing.T) {
	// Verify that all permissions in default lists are valid
	t.Run("instance-everyone defaults are valid", func(t *testing.T) {
		for _, perm := range DefaultInstanceEveryonePermissions() {
			if err := ValidatePermission(perm); err != nil {
				t.Errorf("Invalid permission in instance-everyone defaults: %v", perm)
			}
		}
	})

	t.Run("space-everyone defaults are valid", func(t *testing.T) {
		for _, perm := range DefaultSpaceEveryonePermissions() {
			if err := ValidatePermission(perm); err != nil {
				t.Errorf("Invalid permission in space-everyone defaults: %v", perm)
			}
		}
	})

	t.Run("space-moderator defaults are valid", func(t *testing.T) {
		for _, perm := range DefaultSpaceModeratorPermissions() {
			if err := ValidatePermission(perm); err != nil {
				t.Errorf("Invalid permission in space-moderator defaults: %v", perm)
			}
		}
	})
}

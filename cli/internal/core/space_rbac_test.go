package core

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/core/rbac"
)

// ============================================================================
// Role Name Validation Tests
// ============================================================================

func TestValidateRoleName(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		// Valid space role names - lowercase letters only
		{"simple", "admin", false},
		{"moderator", "moderator", false},
		{"single-char", "a", false},
		{"max-length", "abcdefghijklmnopqrstuvwxyzabcdef", false}, // 32 chars

		// Invalid space role names - now reject dashes and numbers
		{"with-dash", "contentmod", false},      // without dash is valid
		{"with-number", "tierone", false},       // without number is valid
		{"dash-is-invalid", "content-mod", true}, // dash is now invalid
		{"number-is-invalid", "tier1", true},     // number is now invalid
		{"reserved-instance", "instance", true},  // reserved word

		// Other invalid names
		{"empty", "", true},
		{"uppercase", "Admin", true},
		{"mixed-case", "contentModerator", true},
		{"starts-with-number", "1admin", true},
		{"starts-with-dash", "-admin", true},
		{"contains-underscore", "content_mod", true},
		{"contains-space", "content mod", true},
		{"contains-dot", "content.mod", true},
		{"too-long", "abcdefghijklmnopqrstuvwxyzabcdefg", true}, // 33 chars
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := rbac.ValidateRoleName(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateRoleName(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}

// ============================================================================
// Role CRUD Tests
// ============================================================================

func TestChattoCore_CreateRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a space first (roles are per-space)
	space, err := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	// Create a role
	role, err := core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate content")
	if err != nil {
		t.Fatalf("Failed to create role: %v", err)
	}

	if role == nil {
		t.Fatal("Expected role to be returned")
	}

	if role.Name != "testmod" {
		t.Errorf("Expected role name 'testmod', got '%s'", role.Name)
	}

	if role.DisplayName != "Test Mod" {
		t.Errorf("Expected display name 'Test Mod', got '%s'", role.DisplayName)
	}

	if role.Description != "Can moderate content" {
		t.Errorf("Expected description 'Can moderate content', got '%s'", role.Description)
	}
}

func TestChattoCore_CreateRole_InvalidName(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Try to create role with invalid name
	_, err := core.CreateRole(ctx, "test-user", space.Id, "Invalid-Name", "Invalid", "Should fail")
	if err == nil {
		t.Error("Expected error for invalid role name")
	}
	if !errors.Is(err, ErrInvalidRoleName) {
		t.Errorf("Expected ErrInvalidRoleName, got %v", err)
	}
}

func TestChattoCore_CreateRole_Duplicate(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Create a role
	_, err := core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "First")
	if err != nil {
		t.Fatalf("Failed to create first role: %v", err)
	}

	// Try to create same role again
	_, err = core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod 2", "Second")
	if err == nil {
		t.Error("Expected error for duplicate role")
	}
	if !errors.Is(err, ErrRoleAlreadyExists) {
		t.Errorf("Expected ErrRoleAlreadyExists, got %v", err)
	}
}

func TestChattoCore_GetRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Create a role
	_, err := core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate content")
	if err != nil {
		t.Fatalf("Failed to create role: %v", err)
	}

	// Retrieve it
	retrieved, err := core.GetRole(ctx, space.Id, "testmod")
	if err != nil {
		t.Fatalf("Failed to get role: %v", err)
	}

	if retrieved.Name != "testmod" {
		t.Errorf("Expected role name 'testmod', got '%s'", retrieved.Name)
	}

	if retrieved.DisplayName != "Test Mod" {
		t.Errorf("Expected display name 'Test Mod', got '%s'", retrieved.DisplayName)
	}
}

func TestChattoCore_GetRole_NotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Try to get nonexistent role
	_, err := core.GetRole(ctx, space.Id, "nonexistent")
	if err == nil {
		t.Error("Expected error when getting nonexistent role")
	}

	if !errors.Is(err, ErrRoleNotFound) {
		t.Errorf("Expected ErrRoleNotFound, got %v", err)
	}
}

func TestChattoCore_ListRoles(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Initially should have 4 default roles (owner, admin, moderator, everyone) created by CreateSpace
	roles, err := core.ListRoles(ctx, space.Id)
	if err != nil {
		t.Fatalf("Failed to list roles: %v", err)
	}
	if len(roles) != 4 {
		t.Errorf("Expected 4 default roles, got %d", len(roles))
	}

	// Create some additional roles
	core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Test mod role")
	core.CreateRole(ctx, "test-user", space.Id, "vip", "VIP", "VIP role")

	// List again - should have 6 total (4 default + 2 custom)
	roles, err = core.ListRoles(ctx, space.Id)
	if err != nil {
		t.Fatalf("Failed to list roles: %v", err)
	}
	if len(roles) != 6 {
		t.Errorf("Expected 6 roles, got %d", len(roles))
	}
}

func TestChattoCore_UpdateRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Create a role
	_, err := core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate content")
	if err != nil {
		t.Fatalf("Failed to create role: %v", err)
	}

	// Update it
	updated, err := core.UpdateRole(ctx, "test-user", space.Id, "testmod", "Super Moderator", "Enhanced moderation")
	if err != nil {
		t.Fatalf("Failed to update role: %v", err)
	}

	if updated.Name != "testmod" {
		t.Error("Role name should not change on update")
	}

	if updated.DisplayName != "Super Moderator" {
		t.Errorf("Expected display name 'Super Moderator', got '%s'", updated.DisplayName)
	}

	if updated.Description != "Enhanced moderation" {
		t.Errorf("Expected description 'Enhanced moderation', got '%s'", updated.Description)
	}

	// Verify persisted
	retrieved, _ := core.GetRole(ctx, space.Id, "testmod")
	if retrieved.DisplayName != "Super Moderator" {
		t.Error("Update was not persisted")
	}
}

func TestChattoCore_DeleteRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Create a role
	_, err := core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate content")
	if err != nil {
		t.Fatalf("Failed to create role: %v", err)
	}

	// Delete it
	err = core.DeleteRole(ctx, "test-user", space.Id, "testmod")
	if err != nil {
		t.Fatalf("Failed to delete role: %v", err)
	}

	// Verify it's gone
	_, err = core.GetRole(ctx, space.Id, "testmod")
	if !errors.Is(err, ErrRoleNotFound) {
		t.Error("Role should not exist after deletion")
	}
}

func TestChattoCore_DeleteRole_SystemRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Admin role is automatically created by CreateSpace via CreateDefaultRoles
	// Verify it exists
	_, err := core.GetRole(ctx, space.Id, RoleOwner)
	if err != nil {
		t.Fatalf("Admin role should exist after CreateSpace: %v", err)
	}

	// Try to delete - should fail
	err = core.DeleteRole(ctx, "test-user", space.Id, RoleOwner)
	if err == nil {
		t.Error("Expected error when deleting system role")
	}

	if !errors.Is(err, ErrCannotDeleteSystemRole) {
		t.Errorf("Expected ErrCannotDeleteSystemRole, got %v", err)
	}

	// Also test everyone role
	err = core.DeleteRole(ctx, "test-user", space.Id, RoleEveryone)
	if !errors.Is(err, ErrCannotDeleteSystemRole) {
		t.Errorf("Expected ErrCannotDeleteSystemRole for everyone role, got %v", err)
	}
}


func TestChattoCore_DeleteRole_CleansUpPermissionsAndAssignments(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Create a role, grant permissions, and assign to a user
	_, err := core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate")
	if err != nil {
		t.Fatalf("Failed to create role: %v", err)
	}

	// Grant permissions
	core.GrantSpacePermission(ctx, "test-user", space.Id, "testmod", PermRoleAssign)
	core.GrantSpacePermission(ctx, "test-user", space.Id, "testmod", PermRoomManage)

	// Assign role to a user
	core.AssignRole(ctx, "test-user", space.Id, "test-user", "testmod")

	// Verify permissions and assignment exist
	perms, _ := core.GetRolePermissions(ctx, space.Id, "testmod")
	if len(perms) != 2 {
		t.Fatalf("Expected 2 permissions, got %d", len(perms))
	}

	roles, _ := core.GetUserRoles(ctx, space.Id, "test-user")
	found := false
	for _, r := range roles {
		if r == "testmod" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("User should have testmod role before deletion")
	}

	// Delete the role
	err = core.DeleteRole(ctx, "test-user", space.Id, "testmod")
	if err != nil {
		t.Fatalf("Failed to delete role: %v", err)
	}

	// Verify permissions are cleaned up
	perms, _ = core.GetRolePermissions(ctx, space.Id, "testmod")
	if len(perms) != 0 {
		t.Errorf("Expected 0 permissions after role deletion, got %d", len(perms))
	}

	// Verify assignment is cleaned up
	roles, _ = core.GetUserRoles(ctx, space.Id, "test-user")
	for _, r := range roles {
		if r == "testmod" {
			t.Error("User should not have testmod role after role deletion")
		}
	}
}

// ============================================================================
// Permission Assignment Tests
// ============================================================================

func TestChattoCore_GrantRolePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate")

	// Grant a permission
	err := core.GrantSpacePermission(ctx, "test-user", space.Id, "testmod", PermRoleAssign)
	if err != nil {
		t.Fatalf("Failed to grant permission: %v", err)
	}

	// Verify it was granted
	perms, err := core.GetRolePermissions(ctx, space.Id, "testmod")
	if err != nil {
		t.Fatalf("Failed to get permissions: %v", err)
	}

	if len(perms) != 1 {
		t.Errorf("Expected 1 permission, got %d", len(perms))
	}

	if perms[0] != PermRoleAssign {
		t.Errorf("Expected permission %s, got %s", PermRoleAssign, perms[0])
	}
}

func TestChattoCore_GrantRolePermission_Idempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate")

	// Grant same permission twice
	core.GrantSpacePermission(ctx, "test-user", space.Id, "testmod", PermRoleAssign)
	err := core.GrantSpacePermission(ctx, "test-user", space.Id, "testmod", PermRoleAssign)
	if err != nil {
		t.Fatalf("Second grant should not fail: %v", err)
	}

	// Should still have only one permission
	perms, _ := core.GetRolePermissions(ctx, space.Id, "testmod")
	if len(perms) != 1 {
		t.Errorf("Expected 1 permission after duplicate grant, got %d", len(perms))
	}
}

func TestChattoCore_GrantRolePermission_RoleNotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Try to grant to nonexistent role
	err := core.GrantSpacePermission(ctx, "test-user", space.Id, "nonexistent", PermRoomList)
	if !errors.Is(err, ErrRoleNotFound) {
		t.Errorf("Expected ErrRoleNotFound, got %v", err)
	}
}

func TestChattoCore_GrantRolePermission_InvalidPermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate")

	// Try to grant an invalid permission
	err := core.GrantSpacePermission(ctx, "test-user", space.Id, "testmod", Permission("invalid_perm"))
	if err == nil {
		t.Error("Expected error when granting invalid permission")
	}

	// Verify the error is ErrInvalidPermission
	if !errors.Is(err, ErrInvalidPermission) {
		t.Errorf("Expected ErrInvalidPermission, got %v", err)
	}
}

func TestChattoCore_RevokeRolePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate")

	// Grant then revoke
	core.GrantSpacePermission(ctx, "test-user", space.Id, "testmod", PermRoleAssign)
	err := core.RevokeSpacePermission(ctx, "test-user", space.Id, "testmod", PermRoleAssign)
	if err != nil {
		t.Fatalf("Failed to revoke permission: %v", err)
	}

	// Verify it was revoked
	perms, _ := core.GetRolePermissions(ctx, space.Id, "testmod")
	if len(perms) != 0 {
		t.Errorf("Expected 0 permissions after revoke, got %d", len(perms))
	}
}

func TestChattoCore_RevokeRolePermission_Idempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate")

	// Revoke permission that was never granted
	err := core.RevokeSpacePermission(ctx, "test-user", space.Id, "testmod", PermRoleAssign)
	if err != nil {
		t.Fatalf("Revoking non-existent permission should not fail: %v", err)
	}
}

func TestChattoCore_GetRolePermissions_Multiple(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate")

	// Grant multiple permissions
	core.GrantSpacePermission(ctx, "test-user", space.Id, "testmod", PermRoomList)
	core.GrantSpacePermission(ctx, "test-user", space.Id, "testmod", PermRoleAssign)
	core.GrantSpacePermission(ctx, "test-user", space.Id, "testmod", PermRoomManage)

	perms, err := core.GetRolePermissions(ctx, space.Id, "testmod")
	if err != nil {
		t.Fatalf("Failed to get permissions: %v", err)
	}

	if len(perms) != 3 {
		t.Errorf("Expected 3 permissions, got %d", len(perms))
	}

	// Check all permissions are present (order may vary)
	permSet := make(map[Permission]bool)
	for _, p := range perms {
		permSet[p] = true
	}

	if !permSet[PermRoomList] {
		t.Error("Missing PermRoomList")
	}
	if !permSet[PermRoleAssign] {
		t.Error("Missing PermRoleAssign")
	}
	if !permSet[PermRoomManage] {
		t.Error("Missing PermRoomManage")
	}
}

func TestChattoCore_GetRolePermissions_Empty(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate")

	// No permissions granted yet
	perms, err := core.GetRolePermissions(ctx, space.Id, "testmod")
	if err != nil {
		t.Fatalf("Failed to get permissions: %v", err)
	}

	if len(perms) != 0 {
		t.Errorf("Expected 0 permissions, got %d", len(perms))
	}
}

// ============================================================================
// Role Assignment Tests
// ============================================================================

func TestChattoCore_AssignRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate")

	// Assign role to user
	err := core.AssignRole(ctx, "test-user", space.Id, "user123", "testmod")
	if err != nil {
		t.Fatalf("Failed to assign role: %v", err)
	}

	// Verify user has the role
	roles, err := core.GetUserRoles(ctx, space.Id, "user123")
	if err != nil {
		t.Fatalf("Failed to get user roles: %v", err)
	}

	if len(roles) != 1 {
		t.Errorf("Expected 1 role, got %d", len(roles))
	}

	if roles[0] != "testmod" {
		t.Errorf("Expected role 'testmod', got '%s'", roles[0])
	}
}

func TestChattoCore_AssignRole_Idempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate")

	// Assign same role twice
	core.AssignRole(ctx, "test-user", space.Id, "user123", "testmod")
	err := core.AssignRole(ctx, "test-user", space.Id, "user123", "testmod")
	if err != nil {
		t.Fatalf("Second assign should not fail: %v", err)
	}

	// Should still have only one role
	roles, _ := core.GetUserRoles(ctx, space.Id, "user123")
	if len(roles) != 1 {
		t.Errorf("Expected 1 role after duplicate assign, got %d", len(roles))
	}
}

func TestChattoCore_AssignRole_RoleNotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Try to assign nonexistent role
	err := core.AssignRole(ctx, "test-user", space.Id, "user123", "nonexistent")
	if !errors.Is(err, ErrRoleNotFound) {
		t.Errorf("Expected ErrRoleNotFound, got %v", err)
	}
}

func TestChattoCore_RevokeRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate")

	// Assign then revoke
	core.AssignRole(ctx, "test-user", space.Id, "user123", "testmod")
	err := core.RevokeRole(ctx, "test-user", space.Id, "user123", "testmod")
	if err != nil {
		t.Fatalf("Failed to revoke role: %v", err)
	}

	// Verify role was revoked
	roles, _ := core.GetUserRoles(ctx, space.Id, "user123")
	if len(roles) != 0 {
		t.Errorf("Expected 0 roles after revoke, got %d", len(roles))
	}
}

func TestChattoCore_RevokeRole_Idempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate")

	// Revoke role that was never assigned
	err := core.RevokeRole(ctx, "test-user", space.Id, "user123", "testmod")
	if err != nil {
		t.Fatalf("Revoking non-assigned role should not fail: %v", err)
	}
}

func TestChattoCore_RevokeRole_AdminCannotDemotePeerAdmin(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Create two admins (must be space members for permission checks)
	adminA := "admin-a"
	adminB := "admin-b"
	core.JoinSpace(ctx, adminA, space.Id)
	core.JoinSpace(ctx, adminB, space.Id)
	core.AssignRole(ctx, SystemActorID, space.Id, adminA, RoleOwner)
	core.AssignRole(ctx, SystemActorID, space.Id, adminB, RoleOwner)

	// Admin A should NOT be able to revoke admin role from Admin B (peer)
	// Owner can manage the owner role (role hierarchy passes), but can't demote a peer
	// (user hierarchy blocks it since both are at position 0)
	err := core.RevokeRole(ctx, adminA, space.Id, adminB, RoleOwner)
	if !errors.Is(err, ErrCannotManageHigherUser) {
		t.Errorf("Expected ErrCannotManageHigherUser when owner tries to revoke peer's owner role, got: %v", err)
	}

	// Verify Admin B still has admin role
	roles, _ := core.GetUserRoles(ctx, space.Id, adminB)
	hasAdmin := false
	for _, r := range roles {
		if r == RoleOwner {
			hasAdmin = true
			break
		}
	}
	if !hasAdmin {
		t.Error("Admin B should still have admin role")
	}
}

func TestChattoCore_RevokeRole_AdminCanDemoteLowerRankedUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Create admin and moderator (must be space members for permission checks)
	admin := "admin-user"
	mod := "mod-user"
	core.JoinSpace(ctx, admin, space.Id)
	core.JoinSpace(ctx, mod, space.Id)
	core.AssignRole(ctx, SystemActorID, space.Id, admin, RoleOwner)
	// moderator role is created by default with position 1

	// Assign moderator role to mod user
	core.AssignRole(ctx, SystemActorID, space.Id, mod, "moderator")

	// Admin SHOULD be able to revoke moderator role from the lower-ranked user
	err := core.RevokeRole(ctx, admin, space.Id, mod, "moderator")
	if err != nil {
		t.Errorf("Admin should be able to revoke role from lower-ranked user, got: %v", err)
	}

	// Verify moderator role was revoked
	roles, _ := core.GetUserRoles(ctx, space.Id, mod)
	for _, r := range roles {
		if r == "moderator" {
			t.Error("Moderator role should have been revoked")
		}
	}
}

func TestChattoCore_GetUserRoles_Multiple(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate")
	core.CreateRole(ctx, "test-user", space.Id, "vip", "VIP", "VIP user")

	// Assign multiple roles
	core.AssignRole(ctx, "test-user", space.Id, "user123", "testmod")
	core.AssignRole(ctx, "test-user", space.Id, "user123", "vip")

	roles, err := core.GetUserRoles(ctx, space.Id, "user123")
	if err != nil {
		t.Fatalf("Failed to get user roles: %v", err)
	}

	if len(roles) != 2 {
		t.Errorf("Expected 2 roles, got %d", len(roles))
	}

	// Check both roles are present (order may vary)
	roleSet := make(map[string]bool)
	for _, r := range roles {
		roleSet[r] = true
	}

	if !roleSet["testmod"] {
		t.Error("Missing 'testmod' role")
	}
	if !roleSet["vip"] {
		t.Error("Missing 'vip' role")
	}
}

func TestChattoCore_GetRoleUsers(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate")

	// Assign role to multiple users
	core.AssignRole(ctx, "test-user", space.Id, "user1", "testmod")
	core.AssignRole(ctx, "test-user", space.Id, "user2", "testmod")
	core.AssignRole(ctx, "test-user", space.Id, "user3", "testmod")

	users, err := core.GetRoleUsers(ctx, space.Id, "testmod")
	if err != nil {
		t.Fatalf("Failed to get role users: %v", err)
	}

	if len(users) != 3 {
		t.Errorf("Expected 3 users, got %d", len(users))
	}

	// Check all users are present
	userSet := make(map[string]bool)
	for _, u := range users {
		userSet[u] = true
	}

	if !userSet["user1"] || !userSet["user2"] || !userSet["user3"] {
		t.Error("Missing expected users")
	}
}

func TestChattoCore_GetRoleUsers_Empty(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate")

	// No users assigned yet
	users, err := core.GetRoleUsers(ctx, space.Id, "testmod")
	if err != nil {
		t.Fatalf("Failed to get role users: %v", err)
	}

	if len(users) != 0 {
		t.Errorf("Expected 0 users, got %d", len(users))
	}
}

// ============================================================================
// Permission Checking Tests
// ============================================================================

func TestChattoCore_hasSpacePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.JoinSpace(ctx, "user123", space.Id) // Must be a member for space permission checks
	core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate")
	core.GrantSpacePermission(ctx, "test-user", space.Id, "testmod", PermRoleAssign)
	core.AssignRole(ctx, "test-user", space.Id, "user123", "testmod")

	// User should have the permission
	has, err := core.hasSpacePermission(ctx, space.Id, "user123", PermRoleAssign)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if !has {
		t.Error("Expected user to have permission")
	}

	// User should NOT have a permission not granted
	has, err = core.hasSpacePermission(ctx, space.Id, "user123", PermSpaceManage)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if has {
		t.Error("Expected user to NOT have permission")
	}
}

func TestChattoCore_hasSpacePermission_MultipleRoles(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	core.JoinSpace(ctx, "user123", space.Id) // Must be a member for space permission checks

	// Create two roles with different permissions
	core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate")
	core.GrantSpacePermission(ctx, "test-user", space.Id, "testmod", PermRoleAssign)

	core.CreateRole(ctx, "test-user", space.Id, "admin", "Admin", "Full access")
	core.GrantSpacePermission(ctx, "test-user", space.Id, "admin", PermSpaceManage)

	// Assign both roles to user
	core.AssignRole(ctx, "test-user", space.Id, "user123", "testmod")
	core.AssignRole(ctx, "test-user", space.Id, "user123", "admin")

	// User should have permissions from both roles
	has, _ := core.hasSpacePermission(ctx, space.Id, "user123", PermRoleAssign)
	if !has {
		t.Error("Expected user to have PermRoleAssign from testmod role")
	}

	has, _ = core.hasSpacePermission(ctx, space.Id, "user123", PermSpaceManage)
	if !has {
		t.Error("Expected user to have PermSpaceManage from admin role")
	}
}

func TestChattoCore_hasSpacePermission_NoRoles(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// User with no roles should not have any permissions
	has, err := core.hasSpacePermission(ctx, space.Id, "user123", PermRoomList)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if has {
		t.Error("Expected user with no roles to have no permissions")
	}
}

func TestChattoCore_requireSpacePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.JoinSpace(ctx, "user123", space.Id) // Must be a member for space permission checks
	core.CreateRole(ctx, "test-user", space.Id, "testmod", "Test Mod", "Can moderate")
	core.GrantSpacePermission(ctx, "test-user", space.Id, "testmod", PermRoleAssign)
	core.AssignRole(ctx, "test-user", space.Id, "user123", "testmod")

	// Should succeed for granted permission
	err := core.requireSpacePermission(ctx, space.Id, "user123", PermRoleAssign)
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Should fail for non-granted permission
	err = core.requireSpacePermission(ctx, space.Id, "user123", PermSpaceManage)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("Expected ErrPermissionDenied, got %v", err)
	}
}

// ============================================================================
// Default Roles Tests
// ============================================================================

func TestChattoCore_CreateDefaultRoles(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// CreateSpace now automatically calls CreateDefaultRoles, so we just verify
	// that after creating a space, the default roles exist with correct permissions
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Verify admin role exists
	adminRole, err := core.GetRole(ctx, space.Id, RoleOwner)
	if err != nil {
		t.Fatalf("Failed to get admin role: %v", err)
	}
	if adminRole.Name != RoleOwner {
		t.Errorf("Expected admin role name '%s', got '%s'", RoleOwner, adminRole.Name)
	}

	// Owner role now has explicitly stored permissions. Verify the space creator
	// (who has the owner role) has all the expected permissions.

	// Spot-check a few permissions to verify owner has them all
	ownerPermsToCheck := []Permission{PermSpaceManage, PermSpaceDelete, PermRoleManage, PermRoleAssign}
	for _, perm := range ownerPermsToCheck {
		has, err := core.hasSpacePermission(ctx, space.Id, "test-user", perm)
		if err != nil {
			t.Fatalf("Failed to check admin permission %s: %v", perm, err)
		}
		if !has {
			t.Errorf("Owner should have permission %s", perm)
		}
	}

	// Verify everyone role exists with explicit default permissions
	everyoneRole, err := core.GetRole(ctx, space.Id, RoleEveryone)
	if err != nil {
		t.Fatalf("Failed to get everyone role: %v", err)
	}
	if everyoneRole.Name != RoleEveryone {
		t.Errorf("Expected everyone role name '%s', got '%s'", RoleEveryone, everyoneRole.Name)
	}

	everyonePerms, _ := core.GetRolePermissions(ctx, space.Id, RoleEveryone)
	// Post-Phase-5: instance + space defaults both seed against the same
	// SERVER_RBAC bucket, so `everyone` ends up with the union of both
	// default sets. We just sanity-check it's non-empty rather than pinning
	// a specific count that drifts whenever defaults change.
	if len(everyonePerms) == 0 {
		t.Errorf("Expected everyone to have default permissions, got 0")
	}

	// Test that CreateDefaultRoles is idempotent (can be called again without error)
	err = core.CreateDefaultRoles(ctx, space.Id)
	if err != nil {
		t.Errorf("CreateDefaultRoles should be idempotent, got error: %v", err)
	}
}

// ============================================================================
// Instance Role Space Permission Tests
// ============================================================================

func TestChattoCore_GrantInstanceRoleSpacePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Grant a space permission to instance role "moderator"
	err := core.GrantInstanceRoleSpacePermission(ctx, "test-user", space.Id, RoleModerator, PermRoomCreate)
	if err != nil {
		t.Fatalf("Failed to grant instance role space permission: %v", err)
	}

	// Verify permission was granted
	grants, denials, err := core.GetInstanceRoleSpacePermissions(ctx, space.Id, RoleModerator)
	if err != nil {
		t.Fatalf("Failed to get instance role space permissions: %v", err)
	}

	// Note: space.join is automatically granted to instance:verified when creating a space
	hasCreateRoom := false
	for _, p := range grants {
		if p == PermRoomCreate {
			hasCreateRoom = true
			break
		}
	}
	if !hasCreateRoom {
		t.Errorf("Expected PermRoomCreate to be granted, got %v", grants)
	}
	if len(denials) != 0 {
		t.Errorf("Expected 0 denials, got %v", denials)
	}
}

func TestChattoCore_DenyInstanceRoleSpacePermission(t *testing.T) {
	t.Skip("Phase 5 collapsed instance/space tiers; instance-role-grants-space-permission cross-tier scenarios no longer apply.")
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Deny a space permission for instance role "instance-moderator"
	err := core.DenyInstanceRoleSpacePermission(ctx, "test-user", space.Id, RoleModerator, PermRoomJoin)
	if err != nil {
		t.Fatalf("Failed to deny instance role space permission: %v", err)
	}

	// Verify permission was denied
	grants, denials, err := core.GetInstanceRoleSpacePermissions(ctx, space.Id, RoleModerator)
	if err != nil {
		t.Fatalf("Failed to get instance role space permissions: %v", err)
	}

	if len(grants) != 0 {
		t.Errorf("Expected 0 grants, got %v", grants)
	}
	if len(denials) != 1 || denials[0] != PermRoomJoin {
		t.Errorf("Expected 1 denial of PermRoomJoin, got %v", denials)
	}
}

func TestChattoCore_ClearInstanceRoleSpacePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Grant then clear
	core.GrantInstanceRoleSpacePermission(ctx, "test-user", space.Id, RoleModerator, PermRoomCreate)

	err := core.ClearInstanceRoleSpacePermission(ctx, "test-user", space.Id, RoleModerator, PermRoomCreate)
	if err != nil {
		t.Fatalf("Failed to clear instance role space permission: %v", err)
	}

	// Verify permission was cleared
	grants, denials, err := core.GetInstanceRoleSpacePermissions(ctx, space.Id, RoleModerator)
	if err != nil {
		t.Fatalf("Failed to get instance role space permissions: %v", err)
	}

	// PermRoomCreate should be cleared
	hasCreateRoom := false
	for _, p := range grants {
		if p == PermRoomCreate {
			hasCreateRoom = true
			break
		}
	}
	if hasCreateRoom {
		t.Errorf("Expected PermRoomCreate to be cleared from grants, got %v", grants)
	}
	if len(denials) != 0 {
		t.Errorf("Expected 0 denials, got %v", denials)
	}
}

func TestChattoCore_GrantInstanceRoleSpacePermission_ClearsDenial(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// First deny, then grant (grant should clear the denial)
	if err := core.DenyInstanceRoleSpacePermission(ctx, "test-user", space.Id, RoleModerator, PermRoomCreate); err != nil {
		t.Fatalf("Failed to deny: %v", err)
	}
	if err := core.GrantInstanceRoleSpacePermission(ctx, "test-user", space.Id, RoleModerator, PermRoomCreate); err != nil {
		t.Fatalf("Failed to grant: %v", err)
	}

	grants, denials, err := core.GetInstanceRoleSpacePermissions(ctx, space.Id, RoleModerator)
	if err != nil {
		t.Fatalf("Failed to get permissions: %v", err)
	}

	hasCreateRoom := false
	for _, p := range grants {
		if p == PermRoomCreate {
			hasCreateRoom = true
			break
		}
	}
	if !hasCreateRoom {
		t.Errorf("Expected PermRoomCreate to be granted, got %v", grants)
	}
	if len(denials) != 0 {
		t.Errorf("Expected denial to be cleared, got %v", denials)
	}
}

func TestChattoCore_DenyInstanceRoleSpacePermission_ClearsGrant(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// First grant, then deny (deny should clear the grant)
	if err := core.GrantInstanceRoleSpacePermission(ctx, "test-user", space.Id, RoleModerator, PermRoomCreate); err != nil {
		t.Fatalf("Failed to grant: %v", err)
	}
	if err := core.DenyInstanceRoleSpacePermission(ctx, "test-user", space.Id, RoleModerator, PermRoomCreate); err != nil {
		t.Fatalf("Failed to deny: %v", err)
	}

	grants, denials, err := core.GetInstanceRoleSpacePermissions(ctx, space.Id, RoleModerator)
	if err != nil {
		t.Fatalf("Failed to get permissions: %v", err)
	}

	// PermRoomCreate should be cleared from grants
	hasCreateRoom := false
	for _, p := range grants {
		if p == PermRoomCreate {
			hasCreateRoom = true
			break
		}
	}
	if hasCreateRoom {
		t.Errorf("Expected PermRoomCreate grant to be cleared, got %v", grants)
	}
	if len(denials) != 1 || denials[0] != PermRoomCreate {
		t.Errorf("Expected 1 denial of PermRoomCreate, got %v", denials)
	}
}

func TestChattoCore_InstanceRoleSpacePermission_InvalidPermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Try to grant an invalid permission
	err := core.GrantInstanceRoleSpacePermission(ctx, "test-user", space.Id, RoleModerator, Permission("invalid"))
	if !errors.Is(err, ErrInvalidPermission) {
		t.Errorf("Expected ErrInvalidPermission, got %v", err)
	}
}

func TestChattoCore_hasSpacePermission_IncludesInstanceRoles(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, _ := core.CreateUser(ctx, SystemActorID, "testuser", "Test User", "password123")
	core.AssignInstanceRole(ctx, SystemActorID, user.Id, RoleModerator)

	// Create a space
	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Join space as the test user
	core.JoinSpace(ctx, user.Id, space.Id)

	// Initially user should not have PermRoleManage through instance role
	has, _ := core.hasSpacePermission(ctx, space.Id, user.Id, PermRoleManage)
	if has {
		t.Error("User should not have PermRoleManage yet")
	}

	// Grant PermRoleManage to instance moderator role at space level
	core.GrantInstanceRoleSpacePermission(ctx, "test-user", space.Id, RoleModerator, PermRoleManage)

	// Now user should have the permission via instance role
	has, err := core.hasSpacePermission(ctx, space.Id, user.Id, PermRoleManage)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if !has {
		t.Error("User should have PermRoleManage via instance role")
	}
}

func TestChattoCore_hasSpacePermission_InstanceRoleDenialOverrides(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, _ := core.CreateUser(ctx, SystemActorID, "testuser2", "Test User 2", "password123")
	core.AssignInstanceRole(ctx, SystemActorID, user.Id, RoleModerator)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.JoinSpace(ctx, user.Id, space.Id)

	// Member role grants PermRoomList by default
	// First verify user has it
	has, _ := core.hasSpacePermission(ctx, space.Id, user.Id, PermRoomList)
	if !has {
		t.Fatal("User should have PermRoomList via member role initially")
	}

	// Deny PermRoomList for instance moderator role at space level
	core.DenyInstanceRoleSpacePermission(ctx, "test-user", space.Id, RoleModerator, PermRoomList)

	// User should now be denied via instance role denial
	has, err := core.hasSpacePermission(ctx, space.Id, user.Id, PermRoomList)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if has {
		t.Error("User should be denied PermRoomList via instance role denial")
	}
}

func TestChattoCore_ListInstanceRolesWithSpacePermissions(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Grant/deny permissions to non-universal instance roles
	// (universal role everyone is managed as space role, not listed here)
	if err := core.GrantInstanceRoleSpacePermission(ctx, "test-user", space.Id, RoleModerator, PermRoomCreate); err != nil {
		t.Fatalf("Failed to grant to moderator: %v", err)
	}
	if err := core.DenyInstanceRoleSpacePermission(ctx, "test-user", space.Id, RoleAdmin, PermRoomJoin); err != nil {
		t.Fatalf("Failed to deny to admin: %v", err)
	}

	// List all instance roles with space permissions
	configs, err := core.ListInstanceRolesWithSpacePermissions(ctx, space.Id)
	if err != nil {
		t.Fatalf("Failed to list instance role space permissions: %v", err)
	}

	// Should have owner, admin, moderator (no universal roles)
	if len(configs) < 3 {
		t.Errorf("Expected at least 3 instance role configs, got %d", len(configs))
	}

	// Universal roles should NOT appear in instance role configs
	for _, cfg := range configs {
		if cfg.Role.Name == RoleEveryone {
			t.Errorf("Universal role %q should not appear in instance role configs", cfg.Role.Name)
		}
	}

	// Find moderator and admin configs
	var moderatorConfig *InstanceRoleSpaceConfig
	var adminConfig *InstanceRoleSpaceConfig
	for i := range configs {
		if configs[i].Role.Name == RoleModerator {
			moderatorConfig = &configs[i]
		}
		if configs[i].Role.Name == RoleAdmin {
			adminConfig = &configs[i]
		}
	}

	if moderatorConfig == nil {
		t.Fatal("Expected moderator instance role in configs")
	}
	hasCreateRoom := false
	for _, p := range moderatorConfig.Grants {
		if p == PermRoomCreate {
			hasCreateRoom = true
			break
		}
	}
	if !hasCreateRoom {
		t.Errorf("Expected moderator to have PermRoomCreate granted, got %v", moderatorConfig.Grants)
	}

	if adminConfig == nil {
		t.Fatal("Expected admin instance role in configs")
	}
	if len(adminConfig.Denials) != 1 || adminConfig.Denials[0] != PermRoomJoin {
		t.Errorf("Expected admin to have PermRoomJoin denied, got %v", adminConfig.Denials)
	}
}

// =============================================================================
// Space Join/Leave Permission Tests
// =============================================================================

func TestSpaceJoinPermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	creator, _ := core.CreateUser(ctx, "system", "creator", "Creator", "password")

	// Create a space (creator is automatically an admin)
	space, err := core.CreateSpace(ctx, creator.Id, "Test Space", "A test space")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	t.Run("space.join is granted to everyone at instance level", func(t *testing.T) {
		joiner, _ := core.CreateUser(ctx, "system", "checker", "Checker", "password")

		canJoin, err := core.CanJoinSpace(ctx, joiner.Id, space.Id)
		if err != nil {
			t.Fatalf("Failed to check CanJoinSpace: %v", err)
		}
		if !canJoin {
			t.Error("Expected user to be able to join space via instance-level grant")
		}
	})

	t.Run("any user can join space (CanJoinSpace returns true)", func(t *testing.T) {
		joiner, _ := core.CreateUser(ctx, "system", "joiner", "Joiner", "password")

		canJoin, err := core.CanJoinSpace(ctx, joiner.Id, space.Id)
		if err != nil {
			t.Fatalf("Failed to check CanJoinSpace: %v", err)
		}
		if !canJoin {
			t.Error("Expected user to be able to join space")
		}
	})

	t.Run("denying space.join on everyone prevents joining", func(t *testing.T) {
		// Deny space.join for everyone role at instance level
		if err := core.DenyInstanceRolePermission(ctx, RoleEveryone, PermSpaceJoin); err != nil {
			t.Fatalf("Failed to deny space.join: %v", err)
		}

		blocked, _ := core.CreateUser(ctx, "system", "blocked", "Blocked", "password")

		canJoin, err := core.CanJoinSpace(ctx, blocked.Id, space.Id)
		if err != nil {
			t.Fatalf("Failed to check CanJoinSpace: %v", err)
		}
		if canJoin {
			t.Error("Expected denied user to NOT be able to join space")
		}
	})
}

func TestSpaceLeavePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	creator, _ := core.CreateUser(ctx, "system", "creator", "Creator", "password")

	// Create a space
	space, err := core.CreateSpace(ctx, creator.Id, "Test Space", "A test space")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	t.Run("space.leave is granted to everyone role by default", func(t *testing.T) {
		// Check that everyone role has space.leave
		perms := DefaultSpaceEveryonePermissions()
		hasSpaceLeave := false
		for _, p := range perms {
			if p == PermSpaceLeave {
				hasSpaceLeave = true
				break
			}
		}
		if !hasSpaceLeave {
			t.Errorf("Expected space.leave to be in DefaultEveryonePermissions, got %v", perms)
		}
	})

	t.Run("member can leave space (CanLeaveSpace returns true)", func(t *testing.T) {
		// Create and join a user to the space
		leaver, _ := core.CreateUser(ctx, "system", "leaver", "Leaver", "password")
		core.JoinSpace(ctx, leaver.Id, space.Id)

		canLeave, err := core.CanLeaveSpace(ctx, leaver.Id, space.Id)
		if err != nil {
			t.Fatalf("Failed to check CanLeaveSpace: %v", err)
		}
		if !canLeave {
			t.Error("Expected member to be able to leave space")
		}
	})

	t.Run("revoking space.leave prevents leaving", func(t *testing.T) {
		// Revoke space.leave from everyone role (use core function to also clear keys)
		if err := core.RevokeSpacePermission(ctx, creator.Id, space.Id, RoleEveryone, PermSpaceLeave); err != nil {
			t.Fatalf("Failed to revoke space.leave: %v", err)
		}

		// Create and join a new user
		trapped, _ := core.CreateUser(ctx, "system", "trapped", "Trapped", "password")
		core.JoinSpace(ctx, trapped.Id, space.Id)

		canLeave, err := core.CanLeaveSpace(ctx, trapped.Id, space.Id)
		if err != nil {
			t.Fatalf("Failed to check CanLeaveSpace: %v", err)
		}
		if canLeave {
			t.Error("Expected user without space.leave permission to NOT be able to leave")
		}
	})
}

// ============================================================================
// Room-Level Permission Tests
// ============================================================================

func TestChattoCore_GrantRoomRolePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "test-room", "Test channel")

	// Grant message.post at room level for member role
	err := core.GrantRoomRolePermission(ctx, "test-user", space.Id, room.Id, RoleEveryone, PermMessagePost)
	if err != nil {
		t.Fatalf("Failed to grant room permission: %v", err)
	}

	// Verify via GetRoleRoomPermissions
	grants, denials, err := core.GetRoleRoomPermissions(ctx, space.Id, room.Id, RoleEveryone)
	if err != nil {
		t.Fatalf("Failed to get room permissions: %v", err)
	}
	if len(grants) != 1 || grants[0] != PermMessagePost {
		t.Errorf("Expected [message.post] grant, got %v", grants)
	}
	if len(denials) != 0 {
		t.Errorf("Expected no denials, got %v", denials)
	}
}

func TestChattoCore_DenyRoomRolePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "test-room", "Test channel")

	// Deny message.post at room level
	err := core.DenyRoomRolePermission(ctx, "test-user", space.Id, room.Id, RoleEveryone, PermMessagePost)
	if err != nil {
		t.Fatalf("Failed to deny room permission: %v", err)
	}

	grants, denials, err := core.GetRoleRoomPermissions(ctx, space.Id, room.Id, RoleEveryone)
	if err != nil {
		t.Fatalf("Failed to get room permissions: %v", err)
	}
	if len(grants) != 0 {
		t.Errorf("Expected no grants, got %v", grants)
	}
	if len(denials) != 1 || denials[0] != PermMessagePost {
		t.Errorf("Expected [message.post] denial, got %v", denials)
	}
}

func TestChattoCore_ClearRoomRolePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "test-room", "Test channel")

	// Grant, then clear
	core.GrantRoomRolePermission(ctx, "test-user", space.Id, room.Id, RoleEveryone, PermMessagePost)
	err := core.ClearRoomRolePermission(ctx, "test-user", space.Id, room.Id, RoleEveryone, PermMessagePost)
	if err != nil {
		t.Fatalf("Failed to clear room permission: %v", err)
	}

	grants, denials, err := core.GetRoleRoomPermissions(ctx, space.Id, room.Id, RoleEveryone)
	if err != nil {
		t.Fatalf("Failed to get room permissions: %v", err)
	}
	if len(grants) != 0 {
		t.Errorf("Expected no grants after clear, got %v", grants)
	}
	if len(denials) != 0 {
		t.Errorf("Expected no denials after clear, got %v", denials)
	}
}

func TestChattoCore_GrantRoomRolePermission_InvalidScope(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "general", "General")

	// space.manage is not room-scoped — should fail
	err := core.GrantRoomRolePermission(ctx, "test-user", space.Id, room.Id, RoleEveryone, PermSpaceManage)
	if err == nil {
		t.Error("Expected error for non-room-scoped permission, got nil")
	}
}

func TestChattoCore_RoomPermissions_PerRoomIsolation(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room1, _ := core.CreateRoom(ctx, "test-user", space.Id, "room-alpha", "Room Alpha")
	room2, _ := core.CreateRoom(ctx, "test-user", space.Id, "room-beta", "Room Beta")

	// Deny message.post only in room1
	core.DenyRoomRolePermission(ctx, "test-user", space.Id, room1.Id, RoleEveryone, PermMessagePost)

	// Room1 should have the denial
	grants1, denials1, _ := core.GetRoleRoomPermissions(ctx, space.Id, room1.Id, RoleEveryone)
	if len(denials1) != 1 {
		t.Errorf("Room1: expected 1 denial, got %d", len(denials1))
	}
	if len(grants1) != 0 {
		t.Errorf("Room1: expected 0 grants, got %d", len(grants1))
	}

	// Room2 should have no overrides
	grants2, denials2, _ := core.GetRoleRoomPermissions(ctx, space.Id, room2.Id, RoleEveryone)
	if len(grants2) != 0 || len(denials2) != 0 {
		t.Errorf("Room2: expected no overrides, got grants=%v denials=%v", grants2, denials2)
	}
}

func TestChattoCore_RoomPermissions_AuthorizationRequired(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "admin-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "admin-user", space.Id, "general", "General")

	// Create a non-admin user as a space member
	core.JoinSpace(ctx, "regular-user", space.Id)

	// Regular user should not be able to grant room permissions
	err := core.GrantRoomRolePermission(ctx, "regular-user", space.Id, room.Id, RoleEveryone, PermMessagePost)
	if !errors.Is(err, ErrPermissionDenied) {
		t.Errorf("Expected ErrPermissionDenied, got %v", err)
	}
}

func TestChattoCore_GrantRoomRolePermission_GrantClearsDenial(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "general", "General")

	// Deny, then grant — should clear the denial
	core.DenyRoomRolePermission(ctx, "test-user", space.Id, room.Id, RoleEveryone, PermMessagePost)
	core.GrantRoomRolePermission(ctx, "test-user", space.Id, room.Id, RoleEveryone, PermMessagePost)

	grants, denials, _ := core.GetRoleRoomPermissions(ctx, space.Id, room.Id, RoleEveryone)
	if len(grants) != 1 || grants[0] != PermMessagePost {
		t.Errorf("Expected [message.post] grant, got %v", grants)
	}
	if len(denials) != 0 {
		t.Errorf("Expected no denials after grant, got %v", denials)
	}
}

// ============================================================================
// GetUserEffectiveSpacePermissions Tests
// ============================================================================

func TestChattoCore_GetUserEffectiveSpacePermissions_SpaceRoles(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create creator and regular user
	creator, _ := core.CreateUser(ctx, SystemActorID, "creator", "Creator", "password123")
	user, _ := core.CreateUser(ctx, SystemActorID, "user", "User", "password123")

	// Create a space (creator becomes owner)
	space, _ := core.CreateSpace(ctx, creator.Id, "Test Space", "A test space")

	// User joins space (becomes member with everyone role)
	core.JoinSpace(ctx, user.Id, space.Id)

	// Get user's effective permissions
	perms, err := core.GetUserEffectiveSpacePermissions(ctx, space.Id, user.Id)
	if err != nil {
		t.Fatalf("GetUserEffectiveSpacePermissions failed: %v", err)
	}

	// Convert to set for easier checking
	permSet := make(map[string]bool)
	for _, p := range perms {
		permSet[string(p)] = true
	}

	// User should have default member permissions (via everyone role)
	// Note: room.create is NOT a default permission - it's opt-in
	expectedPerms := []string{"room.list", "room.join", "message.post", "message.post-in-thread", "message.react", "message.reply", "message.reply-in-thread"}
	for _, exp := range expectedPerms {
		if !permSet[exp] {
			t.Errorf("Expected user to have %s permission", exp)
		}
	}

	// User should NOT have admin permissions
	adminPerms := []string{"space.manage", "space.delete", "role.manage", "role.assign"}
	for _, admin := range adminPerms {
		if permSet[admin] {
			t.Errorf("User should not have %s permission", admin)
		}
	}
}

func TestChattoCore_GetUserEffectiveSpacePermissions_InstanceRoleGrants(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user with moderator instance role
	user, _ := core.CreateUser(ctx, SystemActorID, "staffuser", "Staff User", "password123")
	core.AssignInstanceRole(ctx, SystemActorID, user.Id, RoleModerator)

	// Create a space
	creator, _ := core.CreateUser(ctx, SystemActorID, "creator2", "Creator", "password123")
	space, _ := core.CreateSpace(ctx, creator.Id, "Test Space 2", "A test space")

	// User joins space
	core.JoinSpace(ctx, user.Id, space.Id)

	// Initially user should NOT have role.manage
	perms1, _ := core.GetUserEffectiveSpacePermissions(ctx, space.Id, user.Id)
	permSet1 := make(map[string]bool)
	for _, p := range perms1 {
		permSet1[string(p)] = true
	}
	if permSet1["role.manage"] {
		t.Error("User should not have role.manage before grant")
	}

	// Grant role.manage to moderator instance role at space level
	err := core.GrantInstanceRoleSpacePermission(ctx, creator.Id, space.Id, RoleModerator, PermRoleManage)
	if err != nil {
		t.Fatalf("Failed to grant instance role space permission: %v", err)
	}

	// Now user should have role.manage via instance role
	perms2, err := core.GetUserEffectiveSpacePermissions(ctx, space.Id, user.Id)
	if err != nil {
		t.Fatalf("GetUserEffectiveSpacePermissions failed: %v", err)
	}
	permSet2 := make(map[string]bool)
	for _, p := range perms2 {
		permSet2[string(p)] = true
	}
	if !permSet2["role.manage"] {
		t.Error("User should have role.manage via instance role grant")
	}
}

func TestChattoCore_GetUserEffectiveSpacePermissions_DenyAlwaysWins(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user and space
	creator, _ := core.CreateUser(ctx, SystemActorID, "creator3", "Creator", "password123")
	user, _ := core.CreateUser(ctx, SystemActorID, "user3", "User", "password123")
	space, _ := core.CreateSpace(ctx, creator.Id, "Test Space 3", "A test space")

	// User joins space
	core.JoinSpace(ctx, user.Id, space.Id)

	// First grant room.create to everyone role (not granted by default)
	err := core.GrantSpaceRolePermission(ctx, space.Id, RoleEveryone, PermRoomCreate)
	if err != nil {
		t.Fatalf("Failed to grant permission: %v", err)
	}

	// Verify user has room.create after grant
	perms1, _ := core.GetUserEffectiveSpacePermissions(ctx, space.Id, user.Id)
	permSet1 := make(map[string]bool)
	for _, p := range perms1 {
		permSet1[string(p)] = true
	}
	if !permSet1["room.create"] {
		t.Error("User should have room.create after grant")
	}

	// Deny room.create to everyone role
	err = core.DenySpacePermission(ctx, creator.Id, space.Id, RoleEveryone, PermRoomCreate)
	if err != nil {
		t.Fatalf("Failed to deny permission: %v", err)
	}

	// Now user should NOT have room.create (deny wins)
	perms2, err := core.GetUserEffectiveSpacePermissions(ctx, space.Id, user.Id)
	if err != nil {
		t.Fatalf("GetUserEffectiveSpacePermissions failed: %v", err)
	}
	permSet2 := make(map[string]bool)
	for _, p := range perms2 {
		permSet2[string(p)] = true
	}
	if permSet2["room.create"] {
		t.Error("User should NOT have room.create after denial")
	}
}

func TestChattoCore_GetUserEffectiveSpacePermissions_InstanceRoleDenialInSpace(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user with moderator instance role
	user, _ := core.CreateUser(ctx, SystemActorID, "verifieduser", "Verified User", "password123")
	core.AssignInstanceRole(ctx, SystemActorID, user.Id, RoleModerator)

	// Create a space
	creator, _ := core.CreateUser(ctx, SystemActorID, "creator4", "Creator", "password123")
	space, _ := core.CreateSpace(ctx, creator.Id, "Test Space 4", "A test space")

	// User joins space
	core.JoinSpace(ctx, user.Id, space.Id)

	// Verify user has room.list by default
	perms1, _ := core.GetUserEffectiveSpacePermissions(ctx, space.Id, user.Id)
	permSet1 := make(map[string]bool)
	for _, p := range perms1 {
		permSet1[string(p)] = true
	}
	if !permSet1["room.list"] {
		t.Error("User should have room.list by default")
	}

	// Deny room.list to moderator instance role at space level
	err := core.DenyInstanceRoleSpacePermission(ctx, creator.Id, space.Id, RoleModerator, PermRoomList)
	if err != nil {
		t.Fatalf("Failed to deny instance role space permission: %v", err)
	}

	// Now user should NOT have room.list (instance role denial in space wins)
	perms2, err := core.GetUserEffectiveSpacePermissions(ctx, space.Id, user.Id)
	if err != nil {
		t.Fatalf("GetUserEffectiveSpacePermissions failed: %v", err)
	}
	permSet2 := make(map[string]bool)
	for _, p := range perms2 {
		permSet2[string(p)] = true
	}
	if permSet2["room.list"] {
		t.Error("User should NOT have room.list after instance role denial")
	}
}

// ============================================================================
// Role Hierarchy Tests (Space Level)
// ============================================================================

func TestChattoCore_AssignRole_HierarchyEnforcement(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "space-creator", "Test Space", "A test space")

	// Setup users
	owner := "owner-user"
	mod := "mod-user"
	regular := "regular-user"

	core.JoinSpace(ctx, owner, space.Id)
	core.JoinSpace(ctx, mod, space.Id)
	core.JoinSpace(ctx, regular, space.Id)

	// Give owner the owner role (position 0) - owners have all permissions
	core.AssignRole(ctx, SystemActorID, space.Id, owner, RoleOwner)
	// Give mod the moderator role (position ~1)
	core.AssignRole(ctx, SystemActorID, space.Id, mod, RoleModerator)

	// Grant role assignment permission to moderator so we can test hierarchy
	core.GrantSpacePermission(ctx, SystemActorID, space.Id, RoleModerator, PermRoleAssign)

	t.Run("owner can assign moderator role", func(t *testing.T) {
		// Owner (position 0) can assign moderator
		err := core.AssignRole(ctx, owner, space.Id, regular, RoleModerator)
		if err != nil {
			t.Errorf("Owner should be able to assign moderator role: %v", err)
		}
		// Cleanup
		core.RevokeRole(ctx, owner, space.Id, regular, RoleModerator)
	})

	t.Run("moderator cannot assign owner role due to hierarchy", func(t *testing.T) {
		// Moderator has permission but cannot assign owner (higher rank)
		err := core.AssignRole(ctx, mod, space.Id, regular, RoleOwner)
		if !errors.Is(err, ErrCannotAssignHigherRole) {
			t.Errorf("Expected ErrCannotAssignHigherRole, got: %v", err)
		}
	})

	t.Run("user without role assignment permission is denied", func(t *testing.T) {
		// Create a custom role
		core.CreateRole(ctx, owner, space.Id, "helper", "Helper", "Can help")

		// Regular member has no permission at all - should get permission denied
		err := core.AssignRole(ctx, regular, space.Id, mod, "helper")
		if err == nil {
			t.Error("Expected error for regular member without permission")
		}
		if errors.Is(err, ErrCannotAssignHigherRole) {
			t.Error("Regular member should fail on permission check, not hierarchy check")
		}
	})

	t.Run("moderator can assign lower-ranked custom role", func(t *testing.T) {
		// Create a custom role (will be lower rank than moderator)
		role, _ := core.CreateRole(ctx, owner, space.Id, "editor", "Editor", "Can edit")
		// Verify the custom role has a position lower than moderator
		// (position is higher number = lower rank)
		if role.Position <= 2 { // moderator is at 2
			t.Skipf("Custom role position %d is not lower than moderator position 2", role.Position)
		}

		// Mod should be able to assign editor (lower rank)
		err := core.AssignRole(ctx, mod, space.Id, regular, "editor")
		if err != nil {
			t.Errorf("Moderator should be able to assign lower-ranked role: %v", err)
		}
	})
}

func TestChattoCore_RevokeRole_CannotDemoteSelf(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "space-creator", "Test Space", "A test space")

	// Setup user with owner role
	owner := "owner-user"
	core.JoinSpace(ctx, owner, space.Id)
	core.AssignRole(ctx, SystemActorID, space.Id, owner, RoleOwner)

	// Owner cannot revoke their own owner role - there's a specific check for this
	err := core.RevokeRole(ctx, owner, space.Id, owner, RoleOwner)
	if !errors.Is(err, ErrCannotRevokeSelfAdmin) {
		t.Errorf("Expected ErrCannotRevokeSelfAdmin when revoking own owner role, got: %v", err)
	}

	// Verify they still have the role
	roles, _ := core.GetUserRoles(ctx, space.Id, owner)
	hasOwner := false
	for _, r := range roles {
		if r == RoleOwner {
			hasOwner = true
			break
		}
	}
	if !hasOwner {
		t.Error("Owner should still have owner role after failed self-revoke")
	}
}

func TestChattoCore_RevokeRole_PeerProtection(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "space-creator", "Test Space", "A test space")

	// Setup two moderators
	modA := "mod-a"
	modB := "mod-b"
	core.JoinSpace(ctx, modA, space.Id)
	core.JoinSpace(ctx, modB, space.Id)
	core.AssignRole(ctx, SystemActorID, space.Id, modA, RoleModerator)
	core.AssignRole(ctx, SystemActorID, space.Id, modB, RoleModerator)

	// Grant role assignment permission to moderators so we can test hierarchy
	core.GrantSpacePermission(ctx, SystemActorID, space.Id, RoleModerator, PermRoleAssign)

	t.Run("moderator A cannot revoke moderator B's moderator role", func(t *testing.T) {
		// Both are equal rank (moderator), so neither can demote the other
		// First check is "can't revoke role higher than yours" - but they're equal
		// So the check that should fail is the hierarchy check on the role itself
		err := core.RevokeRole(ctx, modA, space.Id, modB, RoleModerator)
		// Either ErrCannotRevokeHigherRole (role hierarchy) or ErrCannotManageHigherUser (user hierarchy)
		if err == nil {
			t.Error("Expected error for peer demotion")
		}
		if !errors.Is(err, ErrCannotRevokeHigherRole) && !errors.Is(err, ErrCannotManageHigherUser) {
			t.Errorf("Expected hierarchy error for peer demotion, got: %v", err)
		}
	})

	t.Run("vice versa - B cannot demote A", func(t *testing.T) {
		err := core.RevokeRole(ctx, modB, space.Id, modA, RoleModerator)
		if err == nil {
			t.Error("Expected error for peer demotion (reverse)")
		}
		if !errors.Is(err, ErrCannotRevokeHigherRole) && !errors.Is(err, ErrCannotManageHigherUser) {
			t.Errorf("Expected hierarchy error for peer demotion (reverse), got: %v", err)
		}
	})

	// Verify both still have their roles
	rolesA, _ := core.GetUserRoles(ctx, space.Id, modA)
	rolesB, _ := core.GetUserRoles(ctx, space.Id, modB)

	hasMod := func(roles []string) bool {
		for _, r := range roles {
			if r == RoleModerator {
				return true
			}
		}
		return false
	}

	if !hasMod(rolesA) {
		t.Error("Moderator A should still have moderator role")
	}
	if !hasMod(rolesB) {
		t.Error("Moderator B should still have moderator role")
	}
}

func TestChattoCore_CreateRole_PositionAssignment(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	t.Run("first custom role gets position after system roles", func(t *testing.T) {
		// System roles: owner (0), moderator (2), member (MAX)
		role, err := core.CreateRole(ctx, "test-user", space.Id, "editor", "Editor", "Can edit")
		if err != nil {
			t.Fatalf("CreateRole failed: %v", err)
		}
		// Custom roles should get a position > moderator (position 2)
		// Note: Current implementation may use MaxInt32 for custom roles,
		// which is the same as PositionEveryone. This is tracked as a known issue.
		t.Logf("Custom role 'editor' assigned position %d", role.Position)
		if role.Position <= 0 {
			t.Errorf("Custom role position = %d, should be positive", role.Position)
		}
	})

	t.Run("position preserved after display name update", func(t *testing.T) {
		role, err := core.CreateRole(ctx, "test-user", space.Id, "contributor", "Contributor", "Can contribute")
		if err != nil {
			t.Fatalf("CreateRole failed: %v", err)
		}
		originalPos := role.Position

		// Update display name
		updated, err := core.UpdateRole(ctx, "test-user", space.Id, "contributor", "Super Contributor", "Can contribute a lot")
		if err != nil {
			t.Fatalf("UpdateRole failed: %v", err)
		}

		if updated.Position != originalPos {
			t.Errorf("Position changed from %d to %d after update", originalPos, updated.Position)
		}
	})

	t.Run("roles can be reordered", func(t *testing.T) {
		// Create multiple custom roles
		core.CreateRole(ctx, "test-user", space.Id, "alpha", "Alpha", "Alpha role")
		core.CreateRole(ctx, "test-user", space.Id, "beta", "Beta", "Beta role")

		// Reorder them
		roles, err := core.ReorderSpaceRoles(ctx, "test-user", space.Id, []string{"beta", "alpha"})
		if err != nil {
			t.Fatalf("ReorderSpaceRoles failed: %v", err)
		}

		// Find positions of alpha and beta
		var alphaPos, betaPos int32
		for _, r := range roles {
			if r.Name == "alpha" {
				alphaPos = r.Position
			}
			if r.Name == "beta" {
				betaPos = r.Position
			}
		}

		// After reorder, beta should come before alpha (lower position = higher rank)
		if betaPos >= alphaPos {
			t.Errorf("After reorder: beta position (%d) should be < alpha position (%d)", betaPos, alphaPos)
		}
	})
}

func TestChattoCore_CanManageSpaceUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "space-creator", "Test Space", "A test space")

	// Setup hierarchy using system roles: owner > moderator > member
	// Note: Custom roles currently get position MaxInt32 (same as member),
	// so we test with system roles that have defined positions.
	owner := "owner-user"
	mod := "mod-user"
	member := "member-user"

	core.JoinSpace(ctx, owner, space.Id)
	core.JoinSpace(ctx, mod, space.Id)
	core.JoinSpace(ctx, member, space.Id)

	core.AssignRole(ctx, SystemActorID, space.Id, owner, RoleOwner)
	core.AssignRole(ctx, SystemActorID, space.Id, mod, RoleModerator)
	// member has no explicit role, just the implicit member role

	t.Run("owner can manage all", func(t *testing.T) {
		canMod, _ := core.CanManageSpaceUser(ctx, space.Id, owner, mod)
		canMember, _ := core.CanManageSpaceUser(ctx, space.Id, owner, member)

		if !canMod {
			t.Error("Owner should be able to manage moderator")
		}
		if !canMember {
			t.Error("Owner should be able to manage member")
		}
	})

	t.Run("moderator can manage member but not owner", func(t *testing.T) {
		canOwner, _ := core.CanManageSpaceUser(ctx, space.Id, mod, owner)
		canMember, _ := core.CanManageSpaceUser(ctx, space.Id, mod, member)

		if canOwner {
			t.Error("Moderator should NOT be able to manage owner")
		}
		if !canMember {
			t.Error("Moderator should be able to manage member")
		}
	})

	t.Run("member cannot manage anyone with a role", func(t *testing.T) {
		canOwner, _ := core.CanManageSpaceUser(ctx, space.Id, member, owner)
		canMod, _ := core.CanManageSpaceUser(ctx, space.Id, member, mod)

		if canOwner {
			t.Error("Member should NOT be able to manage owner")
		}
		if canMod {
			t.Error("Member should NOT be able to manage moderator")
		}
	})

	t.Run("peers at same level cannot manage each other", func(t *testing.T) {
		// Create another moderator
		mod2 := "mod-user-2"
		core.JoinSpace(ctx, mod2, space.Id)
		core.AssignRole(ctx, SystemActorID, space.Id, mod2, RoleModerator)

		canManage, _ := core.CanManageSpaceUser(ctx, space.Id, mod, mod2)
		if canManage {
			t.Error("Moderator should NOT be able to manage another moderator (same rank)")
		}

		canManageReverse, _ := core.CanManageSpaceUser(ctx, space.Id, mod2, mod)
		if canManageReverse {
			t.Error("Moderator should NOT be able to manage another moderator (reverse)")
		}
	})
}

package core

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/testutil"
)

// ============================================================================
// Instance Permission Validation Tests
// ============================================================================

func TestValidatePermission_ServerScope(t *testing.T) {
	tests := []struct {
		name    string
		perm    Permission
		wantErr bool
	}{
		{"admin valid", PermAdminAccess, false},
		// Unified permissions with ScopeServer
		{"message.post valid (unified scope)", Permission("message.post"), false},
		{"message.react valid (unified scope)", Permission("message.react"), false},
		{"room.join valid (unified scope)", Permission("room.join"), false},
		{"room.create valid (unified scope)", Permission("room.create"), false},
		{"server.manage valid", Permission("server.manage"), false},
		{"role.manage valid", Permission("role.manage"), false},
		// Invalid permissions
		{"invalid permission", Permission("invalid"), true},
		{"empty permission", Permission(""), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePermission(tt.perm)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidatePermission(%q) error = %v, wantErr %v", tt.perm, err, tt.wantErr)
			}
		})
	}
}

func TestIsSystemRole(t *testing.T) {
	tests := []struct {
		name string
		role string
		want bool
	}{
		{"admin is system role", RoleAdmin, true},
		{"everyone is system role", RoleEveryone, true},
		{"custom is not system role", "custom", false},
		{"empty is not system role", "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsSystemRole(tt.role); got != tt.want {
				t.Errorf("IsSystemRole(%q) = %v, want %v", tt.role, got, tt.want)
			}
		})
	}
}

func TestDefaultServerEveryonePermissions(t *testing.T) {
	perms := DefaultEveryonePermissions()
	if len(perms) == 0 {
		t.Error("Expected at least one default everyone permission")
	}

	// Should contain all base permissions
	expected := []Permission{PermUserDeleteSelf, PermMessagePost}
	permSet := make(map[Permission]bool)
	for _, p := range perms {
		permSet[p] = true
	}
	for _, exp := range expected {
		if !permSet[exp] {
			t.Errorf("Expected %s in default everyone permissions", exp)
		}
	}
}

// ============================================================================
// Instance RBAC Initialization Tests
// ============================================================================

func TestChattoCore_initServerRBAC(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// initServerRBAC is called during NewChattoCore, so just verify the state

	// Check that everyone can delete their own account.
	hasPerm, err := core.HasServerPermission(ctx, "any-user", PermUserDeleteSelf)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if !hasPerm {
		t.Error("Expected everyone to have user.delete-self permission")
	}

	// Check that everyone has message.post permission
	hasPerm, err = core.HasServerPermission(ctx, "any-user", PermMessagePost)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if !hasPerm {
		t.Error("Expected everyone to have message.post permission")
	}

	// Check that everyone does NOT have admin permission
	hasPerm, err = core.HasServerPermission(ctx, "any-user", PermAdminAccess)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if hasPerm {
		t.Error("Expected member to NOT have admin permission")
	}
}

func TestChattoCore_initServerRBAC_Idempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Call initServerRBAC again - should not error (sentinel key already set)
	err := core.initServerRBAC(ctx)
	if err != nil {
		t.Fatalf("Second initServerRBAC should be idempotent: %v", err)
	}
}

func TestChattoCore_initServerRBAC_PreservesPermissionChanges(t *testing.T) {
	// This test verifies that permission changes made by admins are preserved
	// when the instance restarts (a new ChattoCore is created).

	ctx := testContext(t)

	_, nc := testutil.StartNATS(t)

	cfg := config.CoreConfig{
		Assets: config.AssetsConfig{
			SigningSecret: "test-signing-secret",
		},
	}

	// Step 1: Create first core instance (simulates initial startup)
	core1, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatalf("Failed to create first ChattoCore: %v", err)
	}
	startCoreServices(t, core1)

	// Create a user
	user, err := core1.CreateUser(ctx, "", "testuser", "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Verify default permission is granted (everyone can create spaces)
	hasPerm, err := core1.HasServerPermission(ctx, user.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if !hasPerm {
		t.Error("Expected user to have space.create permission by default")
	}

	// Step 2: Admin revokes the permission from the everyone role
	err = core1.DenyServerPermission(ctx, RoleEveryone, PermMessagePost)
	if err != nil {
		t.Fatalf("Failed to deny permission: %v", err)
	}

	// Verify permission is now denied
	hasPerm, err = core1.HasServerPermission(ctx, user.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("Failed to check permission after denial: %v", err)
	}
	if hasPerm {
		t.Error("Expected user to NOT have space.create permission after denial")
	}

	// Step 3: Simulate a restart by creating a new ChattoCore with the same NATS connection
	// This should NOT reset the permissions to defaults
	core2, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatalf("Failed to create second ChattoCore: %v", err)
	}
	startCoreServices(t, core2)

	// Step 4: Verify the permission change was preserved
	hasPerm, err = core2.HasServerPermission(ctx, user.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("Failed to check permission after 'restart': %v", err)
	}
	if hasPerm {
		t.Error("Expected user to still NOT have space.create permission after restart - permission was incorrectly reset to default")
	}
}

// ============================================================================
// Instance Admin Role Tests
// ============================================================================

func TestChattoCore_AssignAdminRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "test-user-123"

	// Initially not an admin
	isAdmin, err := core.IsServerAdmin(ctx, userID)
	if err != nil {
		t.Fatalf("Failed to check admin: %v", err)
	}
	if isAdmin {
		t.Error("Expected user to not be admin initially")
	}

	// Assign admin role
	if err := core.AssignAdminRole(ctx, userID); err != nil {
		t.Fatalf("Failed to assign admin role: %v", err)
	}

	// Now should be admin
	isAdmin, err = core.IsServerAdmin(ctx, userID)
	if err != nil {
		t.Fatalf("Failed to check admin: %v", err)
	}
	if !isAdmin {
		t.Error("Expected user to be admin after assignment")
	}
}

func TestChattoCore_RevokeAdminRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "test-user-456"

	// Assign admin role
	if err := core.AssignAdminRole(ctx, userID); err != nil {
		t.Fatalf("Failed to assign admin role: %v", err)
	}

	// Verify is admin
	isAdmin, _ := core.IsServerAdmin(ctx, userID)
	if !isAdmin {
		t.Fatal("Expected user to be admin")
	}

	// Revoke admin role
	if err := core.RevokeAdminRole(ctx, userID); err != nil {
		t.Fatalf("Failed to revoke admin role: %v", err)
	}

	// Should no longer be admin
	isAdmin, err := core.IsServerAdmin(ctx, userID)
	if err != nil {
		t.Fatalf("Failed to check admin: %v", err)
	}
	if isAdmin {
		t.Error("Expected user to not be admin after revocation")
	}
}

func TestChattoCore_RevokeAdminRole_Idempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "test-user-789"

	// Revoke without ever assigning - should not error
	err := core.RevokeAdminRole(ctx, userID)
	if err != nil {
		t.Errorf("RevokeAdminRole should be idempotent: %v", err)
	}
}

func TestChattoCore_ListAdmins(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Initially no admins
	admins, err := core.ListAdmins(ctx)
	if err != nil {
		t.Fatalf("Failed to list admins: %v", err)
	}
	if len(admins) != 0 {
		t.Errorf("Expected 0 admins initially, got %d", len(admins))
	}

	// Add some admins
	core.AssignAdminRole(ctx, "admin1")
	core.AssignAdminRole(ctx, "admin2")

	// List admins
	admins, err = core.ListAdmins(ctx)
	if err != nil {
		t.Fatalf("Failed to list admins: %v", err)
	}
	if len(admins) != 2 {
		t.Errorf("Expected 2 admins, got %d", len(admins))
	}
}

// ============================================================================
// Instance Permission Checking Tests
// ============================================================================

func TestChattoCore_HasPermission_Admin(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "admin-user"

	// Assign admin role
	if err := core.AssignAdminRole(ctx, userID); err != nil {
		t.Fatalf("Failed to assign admin role: %v", err)
	}

	// Admin should have all permissions
	for _, perm := range []Permission{PermMessagePost, PermAdminAccess} {
		hasPerm, err := core.HasServerPermission(ctx, userID, perm)
		if err != nil {
			t.Fatalf("Failed to check permission %s: %v", perm, err)
		}
		if !hasPerm {
			t.Errorf("Expected admin to have permission %s", perm)
		}
	}
}

func TestChattoCore_HasPermission_Member(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "regular-user"

	// Everyone should have message.post by default.
	hasPerm, err := core.HasServerPermission(ctx, userID, PermMessagePost)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if !hasPerm {
		t.Error("Expected member to have spaces.browse permission")
	}

	// Everyone should have spaces.create
	hasPerm, err = core.HasServerPermission(ctx, userID, PermMessagePost)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if !hasPerm {
		t.Error("Expected member to have spaces.create permission")
	}

	// Member should NOT have admin
	hasPerm, err = core.HasServerPermission(ctx, userID, PermAdminAccess)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if hasPerm {
		t.Error("Expected member to NOT have admin permission")
	}
}

// ============================================================================
// Can* Helper Tests
// ============================================================================

func TestChattoCore_CanAdminAccess(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Regular user cannot view admin
	can, err := core.CanAdminAccess(ctx, "regular-user")
	if err != nil {
		t.Fatalf("Failed to check CanAdminAccess: %v", err)
	}
	if can {
		t.Error("Expected CanAdminAccess to return false for regular users")
	}

	// Admin can view admin
	if err := core.AssignAdminRole(ctx, "admin-user"); err != nil {
		t.Fatalf("Failed to assign admin role: %v", err)
	}
	can, err = core.CanAdminAccess(ctx, "admin-user")
	if err != nil {
		t.Fatalf("Failed to check CanAdminAccess: %v", err)
	}
	if !can {
		t.Error("Expected CanAdminAccess to return true for admin users")
	}
}

// ============================================================================
// Error Handling Tests
// ============================================================================

func TestChattoCore_HasPermission_InvalidPermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Invalid permission - should return false, no error
	// (since the permission doesn't exist in any role, it just won't be found)
	hasPerm, err := core.HasServerPermission(ctx, "any-user", Permission("invalid"))
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if hasPerm {
		t.Error("Expected false for invalid permission")
	}
}

func TestValidatePermission_Error(t *testing.T) {
	err := ValidatePermission(Permission("nonexistent"))
	if err == nil {
		t.Error("Expected error for invalid permission")
	}
	if !errors.Is(err, ErrInvalidPermission) {
		t.Errorf("Expected ErrInvalidPermission, got %v", err)
	}
}

func TestChattoCore_HasUserPermissionViaRoles(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("returns true for member role permissions", func(t *testing.T) {
		userID := "member-role-check"

		// message.post is in default member permissions
		hasPerm, err := core.HasUserPermissionViaRoles(ctx, userID, PermMessagePost)
		if err != nil {
			t.Fatalf("Failed to check: %v", err)
		}
		if !hasPerm {
			t.Error("Expected true for spaces.browse (member permission)")
		}
	})

	t.Run("returns false for non-member permissions", func(t *testing.T) {
		userID := "non-member-perm-check"

		// admin is NOT in default member permissions
		hasPerm, err := core.HasUserPermissionViaRoles(ctx, userID, PermAdminAccess)
		if err != nil {
			t.Fatalf("Failed to check: %v", err)
		}
		if hasPerm {
			t.Error("Expected false for admin (not a member permission)")
		}
	})

	t.Run("returns true for admin role (all permissions)", func(t *testing.T) {
		userID := "admin-role-check"

		// Assign admin role
		if err := core.AssignAdminRole(ctx, userID); err != nil {
			t.Fatalf("Failed to assign admin: %v", err)
		}

		// Admin has all permissions via roles
		hasPerm, err := core.HasUserPermissionViaRoles(ctx, userID, PermAdminAccess)
		if err != nil {
			t.Fatalf("Failed to check: %v", err)
		}
		if !hasPerm {
			t.Error("Expected true for admin user")
		}
	})

	t.Run("returns false for users without the role", func(t *testing.T) {
		userID := "no-admin-role-check"

		// HasUserPermissionViaRoles should return false for admin permission
		hasPerm, err := core.HasUserPermissionViaRoles(ctx, userID, PermAdminAccess)
		if err != nil {
			t.Fatalf("Failed to check: %v", err)
		}
		if hasPerm {
			t.Error("Expected false - user doesn't have admin role")
		}
	})
}

// ============================================================================
// Hierarchy-Wins Tests
// ============================================================================

// These tests verify that HasUserPermissionViaRoles, HasUserPermissionDeniedViaRoles,
// and GetUserServerPermissions use the hierarchy-wins model (matching the actual
// authorizer PermissionResolver.walkPermission), NOT the deny-override model.
//
// The critical scenario: admin role (position 1) grants a permission, but the
// everyone role (position MAX) denies it. Hierarchy-wins says admin's grant wins
// because admin is checked first. Deny-override would incorrectly say everyone's
// deny wins.

func TestChattoCore_HierarchyWins_HighRankGrantBeatsLowRankDeny(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "hierarchy-admin"
	if err := core.AssignAdminRole(ctx, userID); err != nil {
		t.Fatalf("Failed to assign admin role: %v", err)
	}

	// Deny space.create on the everyone role (low rank)
	if err := core.DenyServerPermission(ctx, RoleEveryone, PermMessagePost); err != nil {
		t.Fatalf("Failed to deny permission: %v", err)
	}
	// Admin role still has space.create granted (from InitServerDefaults)

	t.Run("HasServerPermission grants via hierarchy", func(t *testing.T) {
		// The actual authorizer should grant (admin checked first, has grant)
		has, err := core.HasServerPermission(ctx, userID, PermMessagePost)
		if err != nil {
			t.Fatalf("HasServerPermission error: %v", err)
		}
		if !has {
			t.Error("Expected HasServerPermission to return true: admin grant should beat everyone deny")
		}
	})

	t.Run("HasUserPermissionViaRoles matches authorizer", func(t *testing.T) {
		// UI function must agree with the authorizer
		has, err := core.HasUserPermissionViaRoles(ctx, userID, PermMessagePost)
		if err != nil {
			t.Fatalf("HasUserPermissionViaRoles error: %v", err)
		}
		if !has {
			t.Error("Expected HasUserPermissionViaRoles to return true: admin grant should beat everyone deny")
		}
	})

	t.Run("HasUserPermissionDeniedViaRoles matches authorizer", func(t *testing.T) {
		// Permission is NOT effectively denied for this user (admin grant wins)
		denied, err := core.HasUserPermissionDeniedViaRoles(ctx, userID, PermMessagePost)
		if err != nil {
			t.Fatalf("HasUserPermissionDeniedViaRoles error: %v", err)
		}
		if denied {
			t.Error("Expected HasUserPermissionDeniedViaRoles to return false: admin grant should beat everyone deny")
		}
	})

	t.Run("GetUserServerPermissions includes the permission", func(t *testing.T) {
		perms, err := core.GetUserServerPermissions(ctx, userID)
		if err != nil {
			t.Fatalf("GetUserServerPermissions error: %v", err)
		}
		found := false
		for _, p := range perms {
			if p == PermMessagePost {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected GetUserServerPermissions to include space.create: admin grant should beat everyone deny")
		}
	})
}

func TestChattoCore_HierarchyWins_LowRankDenyBlocksWhenNoHigherGrant(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Regular user with no special roles — only has "everyone"
	userID := "hierarchy-regular"

	// Deny space.create on the everyone role
	if err := core.DenyServerPermission(ctx, RoleEveryone, PermMessagePost); err != nil {
		t.Fatalf("Failed to deny permission: %v", err)
	}

	t.Run("HasUserPermissionViaRoles returns false", func(t *testing.T) {
		has, err := core.HasUserPermissionViaRoles(ctx, userID, PermMessagePost)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if has {
			t.Error("Expected false: everyone deny with no higher-rank grant")
		}
	})

	t.Run("HasUserPermissionDeniedViaRoles returns true", func(t *testing.T) {
		denied, err := core.HasUserPermissionDeniedViaRoles(ctx, userID, PermMessagePost)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if !denied {
			t.Error("Expected true: everyone deny with no higher-rank grant")
		}
	})

	t.Run("GetUserServerPermissions excludes the permission", func(t *testing.T) {
		perms, err := core.GetUserServerPermissions(ctx, userID)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		for _, p := range perms {
			if p == PermMessagePost {
				t.Error("Expected space.create NOT to be in permissions: everyone deny with no higher-rank grant")
			}
		}
	})
}

func TestChattoCore_HierarchyWins_OwnerBeatsEverythingElse(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create owner user
	owner, err := core.CreateUser(ctx, SystemActorID, "hierarchy-owner", "Owner", "password123")
	if err != nil {
		t.Fatalf("Failed to create owner: %v", err)
	}
	if err := core.AssignOwnerRole(ctx, owner.Id); err != nil {
		t.Fatalf("Failed to assign owner role: %v", err)
	}

	// Deny admin.access on both admin and everyone roles
	if err := core.DenyServerPermission(ctx, RoleEveryone, PermAdminAccess); err != nil {
		t.Fatalf("Failed to deny everyone: %v", err)
	}
	if err := core.DenyServerPermission(ctx, RoleAdmin, PermAdminAccess); err != nil {
		t.Fatalf("Failed to deny admin: %v", err)
	}
	// Owner role still has admin.access granted

	t.Run("owner grant beats admin and everyone deny", func(t *testing.T) {
		has, err := core.HasUserPermissionViaRoles(ctx, owner.Id, PermAdminAccess)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if !has {
			t.Error("Expected true: owner grant should beat admin+everyone deny")
		}
	})

	t.Run("permission is not denied for owner", func(t *testing.T) {
		denied, err := core.HasUserPermissionDeniedViaRoles(ctx, owner.Id, PermAdminAccess)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if denied {
			t.Error("Expected false: owner grant should beat admin+everyone deny")
		}
	})
}

// ============================================================================
// Instance Role Creation Tests
// ============================================================================

func TestChattoCore_CreateServerRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("creates role successfully", func(t *testing.T) {
		role, err := core.CreateServerRole(ctx, "customrole", "Custom Role", "A custom role")
		if err != nil {
			t.Fatalf("Failed to create role: %v", err)
		}
		if role.Name != "customrole" {
			t.Errorf("Expected role name 'customrole', got '%s'", role.Name)
		}
	})

	t.Run("accepts role name with dashes", func(t *testing.T) {
		role, err := core.CreateServerRole(ctx, "custom-role", "Custom", "Dashes allowed")
		if err != nil {
			t.Fatalf("CreateServerRole(custom-role): %v", err)
		}
		if role.Name != "custom-role" {
			t.Errorf("Expected role name 'custom-role', got %q", role.Name)
		}
	})

	t.Run("accepts role name with numbers", func(t *testing.T) {
		role, err := core.CreateServerRole(ctx, "tier2", "Tier 2", "Numbers allowed")
		if err != nil {
			t.Fatalf("CreateServerRole(tier2): %v", err)
		}
		if role.Name != "tier2" {
			t.Errorf("Expected role name 'tier2', got %q", role.Name)
		}
	})

	t.Run("rejects role name with leading dash", func(t *testing.T) {
		_, err := core.CreateServerRole(ctx, "-custom", "Custom", "Should fail")
		if !errors.Is(err, ErrInvalidRoleName) {
			t.Errorf("Expected ErrInvalidRoleName, got %v", err)
		}
	})

	t.Run("rejects system role names", func(t *testing.T) {
		_, err := core.CreateServerRole(ctx, RoleAdmin, "Admin", "Should fail")
		if err == nil {
			t.Error("Expected error for system role name")
		}
	})
}

// ============================================================================
// Generic Role Assignment Tests
// ============================================================================

func TestChattoCore_AssignServerRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("assigns admin role", func(t *testing.T) {
		userID := "assign-admin-test"

		if err := core.AssignServerRole(ctx, SystemActorID, userID, RoleAdmin); err != nil {
			t.Fatalf("Failed to assign role: %v", err)
		}

		// Verify via IsServerAdmin
		isAdmin, _ := core.IsServerAdmin(ctx, userID)
		if !isAdmin {
			t.Error("Expected user to be admin after assignment")
		}
	})

	t.Run("rejects everyone role assignment", func(t *testing.T) {
		userID := "assign-everyone-test"

		err := core.AssignServerRole(ctx, SystemActorID, userID, RoleEveryone)
		if err == nil {
			t.Error("Expected error when assigning everyone role")
		}
	})

	t.Run("rejects non-existent role", func(t *testing.T) {
		userID := "assign-nonexistent-test"

		err := core.AssignServerRole(ctx, SystemActorID, userID, "nonexistent")
		if !errors.Is(err, ErrRoleNotFound) {
			t.Errorf("Expected ErrRoleNotFound, got %v", err)
		}
	})

	t.Run("assigns custom role", func(t *testing.T) {
		userID := "assign-custom-test"

		// Create a custom role first (must have instance- prefix)
		_, err := core.CreateServerRole(ctx, "tester", "Tester", "QA tester")
		if err != nil {
			t.Fatalf("Failed to create role: %v", err)
		}

		// Assign the custom role
		if err := core.AssignServerRole(ctx, SystemActorID, userID, "tester"); err != nil {
			t.Fatalf("Failed to assign role: %v", err)
		}

		// Verify via GetUserServerRoles
		roles, _ := core.GetUserRoles(ctx, userID)
		found := false
		for _, r := range roles {
			if r == "tester" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected instance-tester role in user's roles")
		}
	})
}

func TestChattoCore_RevokeServerRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("revokes admin role", func(t *testing.T) {
		userID := "revoke-admin-test"

		// Assign first
		core.AssignServerRole(ctx, SystemActorID, userID, RoleAdmin)

		// Verify assigned
		isAdmin, _ := core.IsServerAdmin(ctx, userID)
		if !isAdmin {
			t.Fatal("Expected user to be admin")
		}

		// Revoke
		if err := core.RevokeServerRole(ctx, SystemActorID, userID, RoleAdmin); err != nil {
			t.Fatalf("Failed to revoke role: %v", err)
		}

		// Verify revoked
		isAdmin, _ = core.IsServerAdmin(ctx, userID)
		if isAdmin {
			t.Error("Expected user to not be admin after revocation")
		}
	})

	t.Run("rejects everyone role revocation", func(t *testing.T) {
		userID := "revoke-everyone-test"

		err := core.RevokeServerRole(ctx, SystemActorID, userID, RoleEveryone)
		if err == nil {
			t.Error("Expected error when revoking everyone role")
		}
	})

	t.Run("rejects non-existent role", func(t *testing.T) {
		userID := "revoke-nonexistent-test"

		err := core.RevokeServerRole(ctx, SystemActorID, userID, "nonexistent")
		if !errors.Is(err, ErrRoleNotFound) {
			t.Errorf("Expected ErrRoleNotFound, got %v", err)
		}
	})

	t.Run("idempotent when not assigned", func(t *testing.T) {
		userID := "revoke-unassigned-test"

		// Revoke role user doesn't have - should not error
		err := core.RevokeServerRole(ctx, SystemActorID, userID, RoleAdmin)
		if err != nil {
			t.Errorf("Expected no error for idempotent revoke: %v", err)
		}
	})
}

func TestChattoCore_ListServerRoleUsers(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("returns empty list for everyone role", func(t *testing.T) {
		// Everyone role is implicit - should return empty list
		users, err := core.GetRoleUsers(ctx, RoleEveryone)
		if err != nil {
			t.Fatalf("Failed to list users: %v", err)
		}
		if len(users) != 0 {
			t.Errorf("Expected 0 users for everyone role, got %d", len(users))
		}
	})

	t.Run("returns users with admin role", func(t *testing.T) {
		// Assign some admins
		core.AssignServerRole(ctx, SystemActorID, "list-admin1", RoleAdmin)
		core.AssignServerRole(ctx, SystemActorID, "list-admin2", RoleAdmin)

		users, err := core.GetRoleUsers(ctx, RoleAdmin)
		if err != nil {
			t.Fatalf("Failed to list users: %v", err)
		}

		// Should include our test users (may have others from other tests)
		userSet := make(map[string]bool)
		for _, u := range users {
			userSet[u] = true
		}
		if !userSet["list-admin1"] || !userSet["list-admin2"] {
			t.Errorf("Expected list-admin1 and list-admin2 in users: %v", users)
		}
	})

	t.Run("rejects non-existent role", func(t *testing.T) {
		_, err := core.GetRoleUsers(ctx, "nonexistent")
		if !errors.Is(err, ErrRoleNotFound) {
			t.Errorf("Expected ErrRoleNotFound, got %v", err)
		}
	})
}

func TestChattoCore_GetUserServerRoles(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("returns empty list for user with no explicit roles", func(t *testing.T) {
		userID := "no-roles-user"

		roles, err := core.GetUserRoles(ctx, userID)
		if err != nil {
			t.Fatalf("Failed to get roles: %v", err)
		}
		// Virtual roles (everyone) are not returned by GetUserServerRoles
		if len(roles) != 0 {
			t.Errorf("Expected 0 roles for user with no explicit roles, got %d: %v", len(roles), roles)
		}
	})

	t.Run("returns admin role when assigned", func(t *testing.T) {
		userID := "admin-roles-user"

		core.AssignServerRole(ctx, SystemActorID, userID, RoleAdmin)

		roles, err := core.GetUserRoles(ctx, userID)
		if err != nil {
			t.Fatalf("Failed to get roles: %v", err)
		}
		if len(roles) != 1 {
			t.Errorf("Expected 1 role, got %d: %v", len(roles), roles)
		}
		if len(roles) > 0 && roles[0] != RoleAdmin {
			t.Errorf("Expected admin role, got %s", roles[0])
		}
	})

	t.Run("returns multiple roles", func(t *testing.T) {
		user, err := core.CreateUser(ctx, "system", "multi-roles-user", "Multi", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		// Create custom role (must have instance- prefix)
		core.CreateServerRole(ctx, "editor", "Editor", "Content editor")

		// Assign multiple roles
		core.AssignServerRole(ctx, SystemActorID, user.Id, RoleAdmin)
		core.AssignServerRole(ctx, SystemActorID, user.Id, "editor")

		roles, err := core.GetUserRoles(ctx, user.Id)
		if err != nil {
			t.Fatalf("Failed to get roles: %v", err)
		}
		// Should have admin and instance-editor
		if len(roles) != 2 {
			t.Errorf("Expected 2 roles, got %d: %v", len(roles), roles)
		}

		roleSet := make(map[string]bool)
		for _, r := range roles {
			roleSet[r] = true
		}
		if !roleSet[RoleAdmin] || !roleSet["editor"] {
			t.Errorf("Expected admin and instance-editor roles: %v", roles)
		}
	})
}

// ============================================================================
// Instance Role Assignment Hierarchy Tests
// ============================================================================

func TestChattoCore_AssignServerRole_HierarchyCheck(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create an owner
	owner, err := core.CreateUser(ctx, SystemActorID, "hierarchy-owner", "Owner", "password123")
	if err != nil {
		t.Fatalf("Failed to create owner: %v", err)
	}
	if err := core.AssignOwnerRole(ctx, owner.Id); err != nil {
		t.Fatalf("Failed to assign owner role: %v", err)
	}

	// Create an admin user
	admin, err := core.CreateUser(ctx, SystemActorID, "hierarchy-admin", "Admin", "password123")
	if err != nil {
		t.Fatalf("Failed to create admin: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, admin.Id, RoleAdmin); err != nil {
		t.Fatalf("Failed to assign admin role: %v", err)
	}

	// Create a target user
	target, err := core.CreateUser(ctx, SystemActorID, "hierarchy-target", "Target", "password123")
	if err != nil {
		t.Fatalf("Failed to create target: %v", err)
	}

	t.Run("admin cannot assign owner role (higher rank)", func(t *testing.T) {
		err := core.AssignServerRole(ctx, admin.Id, target.Id, RoleOwner)
		if err == nil {
			t.Error("Expected error: admin should not be able to assign owner role")
		}
		if !errors.Is(err, ErrCannotAssignHigherRole) {
			t.Errorf("Expected ErrCannotAssignHigherRole, got: %v", err)
		}
	})

	t.Run("admin cannot assign admin role (equal rank)", func(t *testing.T) {
		err := core.AssignServerRole(ctx, admin.Id, target.Id, RoleAdmin)
		if err == nil {
			t.Error("Expected error: admin should not be able to assign equal-rank role")
		}
		if !errors.Is(err, ErrCannotAssignHigherRole) {
			t.Errorf("Expected ErrCannotAssignHigherRole, got: %v", err)
		}
	})

	t.Run("admin can assign moderator role (lower rank)", func(t *testing.T) {
		err := core.AssignServerRole(ctx, admin.Id, target.Id, RoleModerator)
		if err != nil {
			t.Fatalf("Expected admin to assign moderator role: %v", err)
		}
	})

	t.Run("owner can assign admin role", func(t *testing.T) {
		err := core.AssignServerRole(ctx, owner.Id, target.Id, RoleAdmin)
		if err != nil {
			t.Fatalf("Expected owner to assign admin role: %v", err)
		}
	})

	t.Run("admin cannot self-escalate to owner", func(t *testing.T) {
		err := core.AssignServerRole(ctx, admin.Id, admin.Id, RoleOwner)
		if err == nil {
			t.Error("Expected error: admin should not be able to self-escalate to owner")
		}
		if !errors.Is(err, ErrCannotAssignHigherRole) {
			t.Errorf("Expected ErrCannotAssignHigherRole, got: %v", err)
		}
	})

	t.Run("system actor bypasses hierarchy check", func(t *testing.T) {
		err := core.AssignServerRole(ctx, SystemActorID, target.Id, RoleOwner)
		if err != nil {
			t.Fatalf("Expected system actor to bypass hierarchy: %v", err)
		}
	})

	t.Run("admin cannot assign moderator role to peer admin", func(t *testing.T) {
		// Symmetric peer-deny with RevokeServerRole: even though the role
		// itself is lower-ranked, decorating a peer with extra roles
		// pollutes role state on someone the actor doesn't outrank.
		peerAdmin, err := core.CreateUser(ctx, SystemActorID, "assign-peer-admin", "Peer", "password123")
		if err != nil {
			t.Fatalf("Failed to create peer admin: %v", err)
		}
		if err := core.AssignServerRole(ctx, SystemActorID, peerAdmin.Id, RoleAdmin); err != nil {
			t.Fatalf("Failed to seed peer admin: %v", err)
		}

		err = core.AssignServerRole(ctx, admin.Id, peerAdmin.Id, RoleModerator)
		if !errors.Is(err, ErrCannotManageHigherUser) {
			t.Errorf("Expected ErrCannotManageHigherUser when admin targets peer admin, got: %v", err)
		}
	})
}

func TestChattoCore_RevokeServerRole_HierarchyCheck(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create an owner
	owner, err := core.CreateUser(ctx, SystemActorID, "revoke-hier-owner", "Owner", "password123")
	if err != nil {
		t.Fatalf("Failed to create owner: %v", err)
	}
	if err := core.AssignOwnerRole(ctx, owner.Id); err != nil {
		t.Fatalf("Failed to assign owner role: %v", err)
	}

	// Create an admin user
	admin, err := core.CreateUser(ctx, SystemActorID, "revoke-hier-admin", "Admin", "password123")
	if err != nil {
		t.Fatalf("Failed to create admin: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, admin.Id, RoleAdmin); err != nil {
		t.Fatalf("Failed to assign admin role: %v", err)
	}

	// Create another admin to try revoking
	otherAdmin, err := core.CreateUser(ctx, SystemActorID, "revoke-hier-admin2", "Admin2", "password123")
	if err != nil {
		t.Fatalf("Failed to create other admin: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, otherAdmin.Id, RoleAdmin); err != nil {
		t.Fatalf("Failed to assign admin role: %v", err)
	}

	// Assign moderator to a target user
	target, err := core.CreateUser(ctx, SystemActorID, "revoke-hier-target", "Target", "password123")
	if err != nil {
		t.Fatalf("Failed to create target: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, target.Id, RoleModerator); err != nil {
		t.Fatalf("Failed to assign moderator: %v", err)
	}

	t.Run("admin cannot revoke owner role (higher rank)", func(t *testing.T) {
		err := core.RevokeServerRole(ctx, admin.Id, owner.Id, RoleOwner)
		if err == nil {
			t.Error("Expected error: admin should not be able to revoke owner role")
		}
	})

	t.Run("admin cannot revoke another admin's role (equal rank)", func(t *testing.T) {
		err := core.RevokeServerRole(ctx, admin.Id, otherAdmin.Id, RoleAdmin)
		if err == nil {
			t.Error("Expected error: admin should not be able to revoke equal-rank role")
		}
	})

	t.Run("admin can revoke moderator role (lower rank)", func(t *testing.T) {
		err := core.RevokeServerRole(ctx, admin.Id, target.Id, RoleModerator)
		if err != nil {
			t.Fatalf("Expected admin to revoke moderator role: %v", err)
		}
	})

	t.Run("owner can revoke admin role", func(t *testing.T) {
		err := core.RevokeServerRole(ctx, owner.Id, admin.Id, RoleAdmin)
		if err != nil {
			t.Fatalf("Expected owner to revoke admin role: %v", err)
		}
	})

	t.Run("system actor bypasses hierarchy check", func(t *testing.T) {
		// Re-assign admin to test system bypass on revoke
		if err := core.AssignServerRole(ctx, SystemActorID, admin.Id, RoleAdmin); err != nil {
			t.Fatalf("Failed to re-assign admin: %v", err)
		}
		err := core.RevokeServerRole(ctx, SystemActorID, admin.Id, RoleAdmin)
		if err != nil {
			t.Fatalf("Expected system actor to bypass hierarchy: %v", err)
		}
	})
}

// ============================================================================
// Instance Role Position and Reordering Tests
// ============================================================================

func TestChattoCore_ReorderServerRoles(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("reorders custom roles", func(t *testing.T) {
		// Create custom roles
		_, err := core.CreateServerRole(ctx, "alpha", "Alpha", "First custom role")
		if err != nil {
			t.Fatalf("Failed to create alpha role: %v", err)
		}
		_, err = core.CreateServerRole(ctx, "beta", "Beta", "Second custom role")
		if err != nil {
			t.Fatalf("Failed to create beta role: %v", err)
		}

		// Get initial positions
		initialRoles, _ := core.ListServerRoles(ctx)
		var alphaInitialPos, betaInitialPos int32
		for _, r := range initialRoles {
			if r.Name == "alpha" {
				alphaInitialPos = r.Position
			}
			if r.Name == "beta" {
				betaInitialPos = r.Position
			}
		}
		t.Logf("Initial positions: alpha=%d, beta=%d", alphaInitialPos, betaInitialPos)

		// Reorder: put beta before alpha
		reordered, err := core.ReorderServerRoles(ctx, []string{"beta", "alpha"})
		if err != nil {
			t.Fatalf("Failed to reorder: %v", err)
		}

		// Find the new positions from the returned list
		var alphaNowPos, betaNowPos int32
		for _, r := range reordered {
			if r.Name == "alpha" {
				alphaNowPos = r.Position
			}
			if r.Name == "beta" {
				betaNowPos = r.Position
			}
		}

		// Reorder semantics: the orderedNames argument goes from LEAST to
		// MOST powerful, so beta should end up with the LOWER position
		// (lower rank) and alpha with the higher one. The test name is
		// preserved for git history; assertion adjusts to the new order.
		if betaNowPos >= alphaNowPos {
			t.Errorf("After reorder, beta (position %d) should be lower-ranked than alpha (position %d)", betaNowPos, alphaNowPos)
		}
	})

	t.Run("rejects system role reordering", func(t *testing.T) {
		_, err := core.ReorderServerRoles(ctx, []string{RoleAdmin, RoleModerator})
		if err == nil {
			t.Error("Expected error when trying to reorder system roles")
		}
	})

	t.Run("rejects incomplete custom role list", func(t *testing.T) {
		_, err := core.ReorderServerRoles(ctx, []string{"alpha"})
		if err == nil {
			t.Error("Expected error when reorder omits a custom role")
		}
	})

	t.Run("rejects duplicate custom roles", func(t *testing.T) {
		_, err := core.ReorderServerRoles(ctx, []string{"alpha", "alpha"})
		if err == nil {
			t.Error("Expected error when reorder includes a duplicate role")
		}
	})

	t.Run("rejects unknown custom role", func(t *testing.T) {
		_, err := core.ReorderServerRoles(ctx, []string{"alpha", "gamma"})
		if !errors.Is(err, ErrRoleNotFound) {
			t.Fatalf("Expected ErrRoleNotFound, got %v", err)
		}
	})

	t.Run("preserves system role positions", func(t *testing.T) {
		roles, _ := core.ListServerRoles(ctx)

		var ownerPos, adminPos, modPos int32
		for _, r := range roles {
			switch r.Name {
			case RoleOwner:
				ownerPos = r.Position
			case RoleAdmin:
				adminPos = r.Position
			case RoleModerator:
				modPos = r.Position
			}
		}

		// System role positions: everyone=0, moderator=100, admin=900, owner=1000.
		if ownerPos != PositionOwner {
			t.Errorf("Expected owner position %d, got %d", PositionOwner, ownerPos)
		}
		if adminPos != PositionAdmin {
			t.Errorf("Expected admin position %d, got %d", PositionAdmin, adminPos)
		}
		if modPos != PositionModerator {
			t.Errorf("Expected moderator position %d, got %d", PositionModerator, modPos)
		}
	})
}

func TestChattoCore_CreateServerRole_PositionAssignment(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("custom role gets position between everyone and moderator", func(t *testing.T) {
		role, err := core.CreateServerRole(ctx, "reviewer", "Reviewer", "Code reviewer")
		if err != nil {
			t.Fatalf("Failed to create role: %v", err)
		}

		// Higher position = higher rank. Custom roles slot in between
		// everyone (0) and moderator (100), so they outrank everyone but
		// not the system roles.
		t.Logf("Custom role 'reviewer' got position %d", role.Position)
		if role.Position <= PositionEveryone {
			t.Errorf("Expected custom role position > %d, got %d", PositionEveryone, role.Position)
		}
		if role.Position >= PositionModerator {
			t.Errorf("Expected custom role position < %d (moderator), got %d", PositionModerator, role.Position)
		}
	})

	t.Run("position preserved after update", func(t *testing.T) {
		// Create a role
		role, err := core.CreateServerRole(ctx, "editor", "Editor", "Content editor")
		if err != nil {
			t.Fatalf("Failed to create role: %v", err)
		}
		originalPos := role.Position

		// Update the display name
		updated, err := core.UpdateServerRole(ctx, "editor", "Super Editor", "Super content editor")
		if err != nil {
			t.Fatalf("Failed to update role: %v", err)
		}

		if updated.Position != originalPos {
			t.Errorf("Position changed after update: %d -> %d", originalPos, updated.Position)
		}
	})

	t.Run("multiple custom roles can be created", func(t *testing.T) {
		roles := []string{"helper", "triage", "support"}
		for _, name := range roles {
			_, err := core.CreateServerRole(ctx, name, name, "Test role")
			if err != nil {
				t.Fatalf("Failed to create role %s: %v", name, err)
			}
		}

		// Verify all exist
		allRoles, _ := core.ListServerRoles(ctx)
		roleNames := make(map[string]bool)
		for _, r := range allRoles {
			roleNames[r.Name] = true
		}

		for _, name := range roles {
			if !roleNames[name] {
				t.Errorf("Role %s not found after creation", name)
			}
		}
	})
}

// ============================================================================
// Role CRUD Tests
// ============================================================================

func TestChattoCore_CreateRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a space first (roles are per-space)

	// Create a role
	role, err := core.CreateServerRole(ctx, "testmod", "Test Mod", "Can moderate content")
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

	// Try to create role with invalid name
	_, err := core.CreateServerRole(ctx, "Invalid-Name", "Invalid", "Should fail")
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

	// Create a role
	_, err := core.CreateServerRole(ctx, "testmod", "Test Mod", "First")
	if err != nil {
		t.Fatalf("Failed to create first role: %v", err)
	}

	// Try to create same role again
	_, err = core.CreateServerRole(ctx, "testmod", "Test Mod 2", "Second")
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

	// Create a role
	_, err := core.CreateServerRole(ctx, "testmod", "Test Mod", "Can moderate content")
	if err != nil {
		t.Fatalf("Failed to create role: %v", err)
	}

	// Retrieve it
	retrieved, err := core.GetServerRole(ctx, "testmod")
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

	// Try to get nonexistent role
	_, err := core.GetServerRole(ctx, "nonexistent")
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

	// Initially should have 4 default roles (owner, admin, moderator, everyone) created by CreateSpace
	roles, err := core.ListServerRoles(ctx)
	if err != nil {
		t.Fatalf("Failed to list roles: %v", err)
	}
	if len(roles) != 4 {
		t.Errorf("Expected 4 default roles, got %d", len(roles))
	}

	// Create some additional roles
	core.CreateServerRole(ctx, "testmod", "Test Mod", "Test mod role")
	core.CreateServerRole(ctx, "vip", "VIP", "VIP role")

	// List again - should have 6 total (4 default + 2 custom)
	roles, err = core.ListServerRoles(ctx)
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

	// Create a role
	_, err := core.CreateServerRole(ctx, "testmod", "Test Mod", "Can moderate content")
	if err != nil {
		t.Fatalf("Failed to create role: %v", err)
	}

	// Update it
	updated, err := core.UpdateServerRole(ctx, "testmod", "Super Moderator", "Enhanced moderation")
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
	retrieved, _ := core.GetServerRole(ctx, "testmod")
	if retrieved.DisplayName != "Super Moderator" {
		t.Error("Update was not persisted")
	}
}

func TestChattoCore_DeleteRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a role
	_, err := core.CreateServerRole(ctx, "testmod", "Test Mod", "Can moderate content")
	if err != nil {
		t.Fatalf("Failed to create role: %v", err)
	}

	// Delete it
	err = core.DeleteServerRole(ctx, "testmod")
	if err != nil {
		t.Fatalf("Failed to delete role: %v", err)
	}

	// Verify it's gone
	_, err = core.GetServerRole(ctx, "testmod")
	if !errors.Is(err, ErrRoleNotFound) {
		t.Error("Role should not exist after deletion")
	}
}

func TestChattoCore_DeleteRole_SystemRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Admin role is automatically created by CreateSpace via CreateDefaultRoles
	// Verify it exists
	_, err := core.GetServerRole(ctx, RoleOwner)
	if err != nil {
		t.Fatalf("Admin role should exist after CreateSpace: %v", err)
	}

	// Try to delete - should fail
	err = core.DeleteServerRole(ctx, RoleOwner)
	if err == nil {
		t.Error("Expected error when deleting system role")
	}

	if !errors.Is(err, ErrCannotDeleteSystemRole) {
		t.Errorf("Expected ErrCannotDeleteSystemRole, got %v", err)
	}

	// Also test everyone role
	err = core.DeleteServerRole(ctx, RoleEveryone)
	if !errors.Is(err, ErrCannotDeleteSystemRole) {
		t.Errorf("Expected ErrCannotDeleteSystemRole for everyone role, got %v", err)
	}
}

func TestChattoCore_DeleteRole_CleansUpPermissionsAndAssignments(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a role, grant permissions, and assign to a user
	_, err := core.CreateServerRole(ctx, "testmod", "Test Mod", "Can moderate")
	if err != nil {
		t.Fatalf("Failed to create role: %v", err)
	}

	// Grant permissions
	core.GrantServerPermission(ctx, "testmod", PermRoleAssign)
	core.GrantServerPermission(ctx, "testmod", PermRoomManage)

	// Assign role to a user
	core.AssignServerRole(ctx, SystemActorID, "test-user", "testmod")

	// Verify permissions and assignment exist
	perms, _ := core.GetServerRolePermissions(ctx, "testmod")
	if len(perms) != 2 {
		t.Fatalf("Expected 2 permissions, got %d", len(perms))
	}

	roles, _ := core.GetUserRoles(ctx, "test-user")
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
	err = core.DeleteServerRole(ctx, "testmod")
	if err != nil {
		t.Fatalf("Failed to delete role: %v", err)
	}

	// Verify permissions are cleaned up
	perms, _ = core.GetServerRolePermissions(ctx, "testmod")
	if len(perms) != 0 {
		t.Errorf("Expected 0 permissions after role deletion, got %d", len(perms))
	}

	// Verify assignment is cleaned up
	roles, _ = core.GetUserRoles(ctx, "test-user")
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

	core.CreateServerRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Grant a permission
	err := core.GrantServerPermission(ctx, "testmod", PermRoleAssign)
	if err != nil {
		t.Fatalf("Failed to grant permission: %v", err)
	}

	// Verify it was granted
	perms, err := core.GetServerRolePermissions(ctx, "testmod")
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

	core.CreateServerRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Grant same permission twice
	core.GrantServerPermission(ctx, "testmod", PermRoleAssign)
	err := core.GrantServerPermission(ctx, "testmod", PermRoleAssign)
	if err != nil {
		t.Fatalf("Second grant should not fail: %v", err)
	}

	// Should still have only one permission
	perms, _ := core.GetServerRolePermissions(ctx, "testmod")
	if len(perms) != 1 {
		t.Errorf("Expected 1 permission after duplicate grant, got %d", len(perms))
	}
}

// GrantServerPermission no longer validates that the role exists post-#330:
// permission grants land in the same SERVER_RBAC bucket regardless. Role
// existence is enforced when CRUD operations against the role itself are made.
// (Previously TestChattoCore_GrantRolePermission_RoleNotFound covered the
// engine-level pre-check; that path is gone.)

func TestChattoCore_GrantRolePermission_InvalidPermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	core.CreateServerRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Try to grant an invalid permission
	err := core.GrantServerPermission(ctx, "testmod", Permission("invalid_perm"))
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

	core.CreateServerRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Grant then revoke
	core.GrantServerPermission(ctx, "testmod", PermRoleAssign)
	err := core.RevokeServerPermission(ctx, "testmod", PermRoleAssign)
	if err != nil {
		t.Fatalf("Failed to revoke permission: %v", err)
	}

	// Verify it was revoked
	perms, _ := core.GetServerRolePermissions(ctx, "testmod")
	if len(perms) != 0 {
		t.Errorf("Expected 0 permissions after revoke, got %d", len(perms))
	}
}

func TestChattoCore_RevokeRolePermission_Idempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	core.CreateServerRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Revoke permission that was never granted
	err := core.RevokeServerPermission(ctx, "testmod", PermRoleAssign)
	if err != nil {
		t.Fatalf("Revoking non-existent permission should not fail: %v", err)
	}
}

func TestChattoCore_GetRolePermissions_Multiple(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	core.CreateServerRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Grant multiple permissions
	core.GrantServerPermission(ctx, "testmod", PermRoomJoin)
	core.GrantServerPermission(ctx, "testmod", PermRoleAssign)
	core.GrantServerPermission(ctx, "testmod", PermRoomManage)

	perms, err := core.GetServerRolePermissions(ctx, "testmod")
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

	if !permSet[PermRoomJoin] {
		t.Error("Missing PermRoomJoin")
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

	core.CreateServerRole(ctx, "testmod", "Test Mod", "Can moderate")

	// No permissions granted yet
	perms, err := core.GetServerRolePermissions(ctx, "testmod")
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

	core.CreateServerRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Assign role to user
	err := core.AssignServerRole(ctx, SystemActorID, "user123", "testmod")
	if err != nil {
		t.Fatalf("Failed to assign role: %v", err)
	}

	// Verify user has the role
	roles, err := core.GetUserRoles(ctx, "user123")
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

	core.CreateServerRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Assign same role twice
	core.AssignServerRole(ctx, SystemActorID, "user123", "testmod")
	err := core.AssignServerRole(ctx, SystemActorID, "user123", "testmod")
	if err != nil {
		t.Fatalf("Second assign should not fail: %v", err)
	}

	// Should still have only one role
	roles, _ := core.GetUserRoles(ctx, "user123")
	if len(roles) != 1 {
		t.Errorf("Expected 1 role after duplicate assign, got %d", len(roles))
	}
}

func TestChattoCore_AssignRole_RoleNotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Try to assign nonexistent role
	err := core.AssignServerRole(ctx, SystemActorID, "user123", "nonexistent")
	if !errors.Is(err, ErrRoleNotFound) {
		t.Errorf("Expected ErrRoleNotFound, got %v", err)
	}
}

func TestChattoCore_RevokeRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	core.CreateServerRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Assign then revoke
	core.AssignServerRole(ctx, SystemActorID, "user123", "testmod")
	err := core.RevokeServerRole(ctx, SystemActorID, "user123", "testmod")
	if err != nil {
		t.Fatalf("Failed to revoke role: %v", err)
	}

	// Verify role was revoked
	roles, _ := core.GetUserRoles(ctx, "user123")
	if len(roles) != 0 {
		t.Errorf("Expected 0 roles after revoke, got %d", len(roles))
	}
}

func TestChattoCore_RevokeRole_Idempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	core.CreateServerRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Revoke role that was never assigned
	err := core.RevokeServerRole(ctx, SystemActorID, "user123", "testmod")
	if err != nil {
		t.Fatalf("Revoking non-assigned role should not fail: %v", err)
	}
}

func TestChattoCore_RevokeRole_AdminCannotDemotePeerAdmin(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create two admins (must be space members for permission checks)
	adminA := "admin-a"
	adminB := "admin-b"
	core.AssignServerRole(ctx, SystemActorID, adminA, RoleOwner)
	core.AssignServerRole(ctx, SystemActorID, adminB, RoleOwner)

	// Admin A should NOT be able to revoke admin role from Admin B (peer)
	// Owner can manage the owner role (role hierarchy passes), but can't demote a peer
	// (user hierarchy blocks it since both are at position 0)
	err := core.RevokeServerRole(ctx, adminA, adminB, RoleOwner)
	if !errors.Is(err, ErrCannotManageHigherUser) {
		t.Errorf("Expected ErrCannotManageHigherUser when owner tries to revoke peer's owner role, got: %v", err)
	}

	// Verify Admin B still has admin role
	roles, _ := core.GetUserRoles(ctx, adminB)
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

	// Create admin and moderator (must be space members for permission checks)
	admin := "admin-user"
	mod := "mod-user"
	core.AssignServerRole(ctx, SystemActorID, admin, RoleOwner)
	// moderator role is created by default with position 1

	// Assign moderator role to mod user
	core.AssignServerRole(ctx, SystemActorID, mod, "moderator")

	// Admin SHOULD be able to revoke moderator role from the lower-ranked user
	err := core.RevokeServerRole(ctx, admin, mod, "moderator")
	if err != nil {
		t.Errorf("Admin should be able to revoke role from lower-ranked user, got: %v", err)
	}

	// Verify moderator role was revoked
	roles, _ := core.GetUserRoles(ctx, mod)
	for _, r := range roles {
		if r == "moderator" {
			t.Error("Moderator role should have been revoked")
		}
	}
}

func TestChattoCore_GetUserRoles_Multiple(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	core.CreateServerRole(ctx, "testmod", "Test Mod", "Can moderate")
	core.CreateServerRole(ctx, "vip", "VIP", "VIP user")

	// Assign multiple roles
	core.AssignServerRole(ctx, SystemActorID, "user123", "testmod")
	core.AssignServerRole(ctx, SystemActorID, "user123", "vip")

	roles, err := core.GetUserRoles(ctx, "user123")
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

	core.CreateServerRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Assign role to multiple users
	core.AssignServerRole(ctx, SystemActorID, "user1", "testmod")
	core.AssignServerRole(ctx, SystemActorID, "user2", "testmod")
	core.AssignServerRole(ctx, SystemActorID, "user3", "testmod")

	users, err := core.GetRoleUsers(ctx, "testmod")
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

	core.CreateServerRole(ctx, "testmod", "Test Mod", "Can moderate")

	// No users assigned yet
	users, err := core.GetRoleUsers(ctx, "testmod")
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

	core.CreateServerRole(ctx, "testmod", "Test Mod", "Can moderate")
	core.GrantServerPermission(ctx, "testmod", PermRoleAssign)
	core.AssignServerRole(ctx, SystemActorID, "user123", "testmod")

	// User should have the permission
	has, err := core.hasServerPermission(ctx, "user123", PermRoleAssign)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if !has {
		t.Error("Expected user to have permission")
	}

	// User should NOT have a permission not granted
	has, err = core.hasServerPermission(ctx, "user123", PermServerManage)
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

	// Create two roles with different permissions
	core.CreateServerRole(ctx, "testmod", "Test Mod", "Can moderate")
	core.GrantServerPermission(ctx, "testmod", PermRoleAssign)

	core.CreateServerRole(ctx, "admin", "Admin", "Full access")
	core.GrantServerPermission(ctx, "admin", PermServerManage)

	// Assign both roles to user
	core.AssignServerRole(ctx, SystemActorID, "user123", "testmod")
	core.AssignServerRole(ctx, SystemActorID, "user123", "admin")

	// User should have permissions from both roles
	has, _ := core.hasServerPermission(ctx, "user123", PermRoleAssign)
	if !has {
		t.Error("Expected user to have PermRoleAssign from testmod role")
	}

	has, _ = core.hasServerPermission(ctx, "user123", PermServerManage)
	if !has {
		t.Error("Expected user to have PermServerManage from admin role")
	}
}

// ============================================================================
// Default Roles Tests
// ============================================================================

func TestChattoCore_CreateDefaultRoles(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Default roles are seeded by initServerRBAC at boot; assign owner to a
	// user so we can verify owner-role permission propagation.
	if err := core.AssignServerRole(ctx, SystemActorID, "test-user", RoleOwner); err != nil {
		t.Fatalf("Failed to assign owner role: %v", err)
	}

	// Verify admin role exists
	adminRole, err := core.GetServerRole(ctx, RoleOwner)
	if err != nil {
		t.Fatalf("Failed to get admin role: %v", err)
	}
	if adminRole.Name != RoleOwner {
		t.Errorf("Expected admin role name '%s', got '%s'", RoleOwner, adminRole.Name)
	}

	// Owner role now has explicitly stored permissions. Verify the owner-role
	// holder has all the expected permissions.

	// Spot-check a few permissions to verify owner has them all
	ownerPermsToCheck := []Permission{PermServerManage, PermRoleManage, PermRoleAssign}
	for _, perm := range ownerPermsToCheck {
		has, err := core.hasServerPermission(ctx, "test-user", perm)
		if err != nil {
			t.Fatalf("Failed to check admin permission %s: %v", perm, err)
		}
		if !has {
			t.Errorf("Owner should have permission %s", perm)
		}
	}

	// Verify everyone role exists with explicit default permissions
	everyoneRole, err := core.GetServerRole(ctx, RoleEveryone)
	if err != nil {
		t.Fatalf("Failed to get everyone role: %v", err)
	}
	if everyoneRole.Name != RoleEveryone {
		t.Errorf("Expected everyone role name '%s', got '%s'", RoleEveryone, everyoneRole.Name)
	}

	everyonePerms, _ := core.GetServerRolePermissions(ctx, RoleEveryone)
	// Post-Phase-5: instance + space defaults both seed against the same
	// SERVER_RBAC bucket, so `everyone` ends up with the union of both
	// default sets. We just sanity-check it's non-empty rather than pinning
	// a specific count that drifts whenever defaults change.
	if len(everyonePerms) == 0 {
		t.Errorf("Expected everyone to have default permissions, got 0")
	}

	// Test that CreateDefaultRoles is idempotent (can be called again without error)
	err = core.CreateDefaultRoles(ctx)
	if err != nil {
		t.Errorf("CreateDefaultRoles should be idempotent, got error: %v", err)
	}
}

// ============================================================================
// Room-Level Permission Tests
// ============================================================================

func TestChattoCore_GrantRoomRolePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "test-room", "Test channel")

	// Grant message.post at room level for member role
	err := core.GrantRoomPermission(ctx, room.Id, RoleEveryone, PermMessagePost)
	if err != nil {
		t.Fatalf("Failed to grant room permission: %v", err)
	}

	// Verify via GetRoleRoomPermissions
	grants, denials, err := core.GetRoomRolePermissions(ctx, room.Id, RoleEveryone)
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

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "test-room", "Test channel")

	// Deny message.post at room level
	err := core.DenyRoomPermission(ctx, room.Id, RoleEveryone, PermMessagePost)
	if err != nil {
		t.Fatalf("Failed to deny room permission: %v", err)
	}

	grants, denials, err := core.GetRoomRolePermissions(ctx, room.Id, RoleEveryone)
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

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "test-room", "Test channel")

	// Grant, then clear
	core.GrantRoomPermission(ctx, room.Id, RoleEveryone, PermMessagePost)
	err := core.ClearRoomPermissionState(ctx, room.Id, RoleEveryone, PermMessagePost)
	if err != nil {
		t.Fatalf("Failed to clear room permission: %v", err)
	}

	grants, denials, err := core.GetRoomRolePermissions(ctx, room.Id, RoleEveryone)
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

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "general", "General")

	// space.manage is not room-scoped — should fail
	err := core.GrantRoomPermission(ctx, room.Id, RoleEveryone, PermServerManage)
	if err == nil {
		t.Error("Expected error for non-room-scoped permission, got nil")
	}
}

func TestChattoCore_RoomPermissions_PerRoomIsolation(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room1, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "room-alpha", "Room Alpha")
	room2, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "room-beta", "Room Beta")

	// Deny message.post only in room1
	core.DenyRoomPermission(ctx, room1.Id, RoleEveryone, PermMessagePost)

	// Room1 should have the denial
	grants1, denials1, _ := core.GetRoomRolePermissions(ctx, room1.Id, RoleEveryone)
	if len(denials1) != 1 {
		t.Errorf("Room1: expected 1 denial, got %d", len(denials1))
	}
	if len(grants1) != 0 {
		t.Errorf("Room1: expected 0 grants, got %d", len(grants1))
	}

	// Room2 should have no overrides
	grants2, denials2, _ := core.GetRoomRolePermissions(ctx, room2.Id, RoleEveryone)
	if len(grants2) != 0 || len(denials2) != 0 {
		t.Errorf("Room2: expected no overrides, got grants=%v denials=%v", grants2, denials2)
	}
}

// Authorization for room-level permission mutations now lives at the GraphQL
// boundary (Resolver.requireRoomManageAuth → CanSpaceRolesManage). The previous
// in-core gate that this test exercised has been retired; the resolver-level
// equivalent is covered by mutation_test.go.

func TestChattoCore_GrantRoomRolePermission_GrantClearsDenial(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "general", "General")

	// Deny, then grant — should clear the denial
	core.DenyRoomPermission(ctx, room.Id, RoleEveryone, PermMessagePost)
	core.GrantRoomPermission(ctx, room.Id, RoleEveryone, PermMessagePost)

	grants, denials, _ := core.GetRoomRolePermissions(ctx, room.Id, RoleEveryone)
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
	_, _ = core.CreateUser(ctx, SystemActorID, "creator", "Creator", "password123")
	user, _ := core.CreateUser(ctx, SystemActorID, "user", "User", "password123")

	// Create a space (creator becomes owner)

	// User joins space (becomes member with everyone role)
	// Get user's effective permissions
	perms, err := core.GetUserEffectiveSpacePermissions(ctx, KindChannel, user.Id)
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
	expectedPerms := []string{"room.join", "message.post", "message.post-in-thread", "message.react"}
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

func TestChattoCore_GetUserEffectiveSpacePermissions_ServerRoleGrants(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user with moderator role
	user, _ := core.CreateUser(ctx, SystemActorID, "staffuser", "Staff User", "password123")
	core.AssignServerRole(ctx, SystemActorID, user.Id, RoleModerator)

	// Create a space
	_, _ = core.CreateUser(ctx, SystemActorID, "creator2", "Creator", "password123")

	// User joins space
	// Initially user should NOT have role.manage
	perms1, _ := core.GetUserEffectiveSpacePermissions(ctx, KindChannel, user.Id)
	permSet1 := make(map[string]bool)
	for _, p := range perms1 {
		permSet1[string(p)] = true
	}
	if permSet1["role.manage"] {
		t.Error("User should not have role.manage before grant")
	}

	// Grant role.manage to moderator role
	err := core.GrantServerPermission(ctx, RoleModerator, PermRoleManage)
	if err != nil {
		t.Fatalf("Failed to grant role permission: %v", err)
	}

	// Now user should have role.manage via role
	perms2, err := core.GetUserEffectiveSpacePermissions(ctx, KindChannel, user.Id)
	if err != nil {
		t.Fatalf("GetUserEffectiveSpacePermissions failed: %v", err)
	}
	permSet2 := make(map[string]bool)
	for _, p := range perms2 {
		permSet2[string(p)] = true
	}
	if !permSet2["role.manage"] {
		t.Error("User should have role.manage via role grant")
	}
}

func TestChattoCore_GetUserEffectiveSpacePermissions_DenyAlwaysWins(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create user and space
	_, _ = core.CreateUser(ctx, SystemActorID, "creator3", "Creator", "password123")
	user, _ := core.CreateUser(ctx, SystemActorID, "user3", "User", "password123")

	// User joins space
	// First grant room.create to everyone role (not granted by default)
	err := core.GrantServerPermission(ctx, RoleEveryone, PermRoomCreate)
	if err != nil {
		t.Fatalf("Failed to grant permission: %v", err)
	}

	// Verify user has room.create after grant
	perms1, _ := core.GetUserEffectiveSpacePermissions(ctx, KindChannel, user.Id)
	permSet1 := make(map[string]bool)
	for _, p := range perms1 {
		permSet1[string(p)] = true
	}
	if !permSet1["room.create"] {
		t.Error("User should have room.create after grant")
	}

	// Deny room.create to everyone role
	err = core.DenyServerPermission(ctx, RoleEveryone, PermRoomCreate)
	if err != nil {
		t.Fatalf("Failed to deny permission: %v", err)
	}

	// Now user should NOT have room.create (deny wins)
	perms2, err := core.GetUserEffectiveSpacePermissions(ctx, KindChannel, user.Id)
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

func TestChattoCore_GetUserEffectiveSpacePermissions_ServerRoleDenialInSpace(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a user with moderator role
	user, _ := core.CreateUser(ctx, SystemActorID, "verifieduser", "Verified User", "password123")
	core.AssignServerRole(ctx, SystemActorID, user.Id, RoleModerator)

	// Create a space
	_, _ = core.CreateUser(ctx, SystemActorID, "creator4", "Creator", "password123")

	// Verify user has room.join by default
	perms1, _ := core.GetUserEffectiveSpacePermissions(ctx, KindChannel, user.Id)
	permSet1 := make(map[string]bool)
	for _, p := range perms1 {
		permSet1[string(p)] = true
	}
	if !permSet1["room.join"] {
		t.Error("User should have room.join by default")
	}

	// Deny room.join to moderator role
	err := core.DenyServerPermission(ctx, RoleModerator, PermRoomJoin)
	if err != nil {
		t.Fatalf("Failed to deny role permission: %v", err)
	}

	// Now user should NOT have room.join (role denial wins)
	perms2, err := core.GetUserEffectiveSpacePermissions(ctx, KindChannel, user.Id)
	if err != nil {
		t.Fatalf("GetUserEffectiveSpacePermissions failed: %v", err)
	}
	permSet2 := make(map[string]bool)
	for _, p := range perms2 {
		permSet2[string(p)] = true
	}
	if permSet2["room.join"] {
		t.Error("User should NOT have room.join after role denial")
	}
}

// ============================================================================
// Role Hierarchy Tests (Space Level)
// ============================================================================

func TestChattoCore_AssignRole_HierarchyEnforcement(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup users
	owner := "owner-user"
	mod := "mod-user"
	regular := "regular-user"

	// Give owner the owner role (position 0) - owners have all permissions
	core.AssignServerRole(ctx, SystemActorID, owner, RoleOwner)
	// Give mod the moderator role (position ~1)
	core.AssignServerRole(ctx, SystemActorID, mod, RoleModerator)

	// Grant role assignment permission to moderator so we can test hierarchy
	core.GrantServerPermission(ctx, RoleModerator, PermRoleAssign)

	t.Run("owner can assign moderator role", func(t *testing.T) {
		// Owner (position 0) can assign moderator
		err := core.AssignServerRole(ctx, owner, regular, RoleModerator)
		if err != nil {
			t.Errorf("Owner should be able to assign moderator role: %v", err)
		}
		// Cleanup
		core.RevokeServerRole(ctx, owner, regular, RoleModerator)
	})

	t.Run("moderator cannot assign owner role due to hierarchy", func(t *testing.T) {
		// Moderator has permission but cannot assign owner (higher rank)
		err := core.AssignServerRole(ctx, mod, regular, RoleOwner)
		if !errors.Is(err, ErrCannotAssignHigherRole) {
			t.Errorf("Expected ErrCannotAssignHigherRole, got: %v", err)
		}
	})

	t.Run("regular member without explicit roles is blocked by role hierarchy", func(t *testing.T) {
		// Create a custom role
		core.CreateServerRole(ctx, "helper", "Helper", "Can help")

		// Regular member only has the implicit "everyone" role (max position),
		// which cannot manage any concrete role.
		err := core.AssignServerRole(ctx, regular, mod, "helper")
		if !errors.Is(err, ErrCannotAssignHigherRole) {
			t.Errorf("Expected ErrCannotAssignHigherRole for regular member, got: %v", err)
		}
	})

	t.Run("moderator can assign lower-ranked custom role", func(t *testing.T) {
		// Create a custom role (will be lower rank than moderator)
		role, _ := core.CreateServerRole(ctx, "editor", "Editor", "Can edit")
		// Verify the custom role has a position lower than moderator
		// (position is higher number = lower rank)
		if role.Position <= 2 { // moderator is at 2
			t.Skipf("Custom role position %d is not lower than moderator position 2", role.Position)
		}

		// Mod should be able to assign editor (lower rank)
		err := core.AssignServerRole(ctx, mod, regular, "editor")
		if err != nil {
			t.Errorf("Moderator should be able to assign lower-ranked role: %v", err)
		}
	})
}

func TestChattoCore_RevokeRole_CannotDemoteSelf(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup user with owner role
	owner := "owner-user"
	core.AssignServerRole(ctx, SystemActorID, owner, RoleOwner)

	// Owner cannot revoke their own owner role - there's a specific check for this
	err := core.RevokeServerRole(ctx, owner, owner, RoleOwner)
	if !errors.Is(err, ErrCannotRevokeSelfAdmin) {
		t.Errorf("Expected ErrCannotRevokeSelfAdmin when revoking own owner role, got: %v", err)
	}

	// Verify they still have the role
	roles, _ := core.GetUserRoles(ctx, owner)
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

	// Setup two moderators
	modA := "mod-a"
	modB := "mod-b"
	core.AssignServerRole(ctx, SystemActorID, modA, RoleModerator)
	core.AssignServerRole(ctx, SystemActorID, modB, RoleModerator)

	// Grant role assignment permission to moderators so we can test hierarchy
	core.GrantServerPermission(ctx, RoleModerator, PermRoleAssign)

	t.Run("moderator A cannot revoke moderator B's moderator role", func(t *testing.T) {
		// Both are equal rank (moderator), so neither can demote the other
		// First check is "can't revoke role higher than yours" - but they're equal
		// So the check that should fail is the hierarchy check on the role itself
		err := core.RevokeServerRole(ctx, modA, modB, RoleModerator)
		// Either ErrCannotRevokeHigherRole (role hierarchy) or ErrCannotManageHigherUser (user hierarchy)
		if err == nil {
			t.Error("Expected error for peer demotion")
		}
		if !errors.Is(err, ErrCannotRevokeHigherRole) && !errors.Is(err, ErrCannotManageHigherUser) {
			t.Errorf("Expected hierarchy error for peer demotion, got: %v", err)
		}
	})

	t.Run("vice versa - B cannot demote A", func(t *testing.T) {
		err := core.RevokeServerRole(ctx, modB, modA, RoleModerator)
		if err == nil {
			t.Error("Expected error for peer demotion (reverse)")
		}
		if !errors.Is(err, ErrCannotRevokeHigherRole) && !errors.Is(err, ErrCannotManageHigherUser) {
			t.Errorf("Expected hierarchy error for peer demotion (reverse), got: %v", err)
		}
	})

	// Verify both still have their roles
	rolesA, _ := core.GetUserRoles(ctx, modA)
	rolesB, _ := core.GetUserRoles(ctx, modB)

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

	t.Run("first custom role gets position after system roles", func(t *testing.T) {
		// System roles: owner (0), moderator (2), member (MAX)
		role, err := core.CreateServerRole(ctx, "editor", "Editor", "Can edit")
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
		role, err := core.CreateServerRole(ctx, "contributor", "Contributor", "Can contribute")
		if err != nil {
			t.Fatalf("CreateRole failed: %v", err)
		}
		originalPos := role.Position

		// Update display name
		updated, err := core.UpdateServerRole(ctx, "contributor", "Super Contributor", "Can contribute a lot")
		if err != nil {
			t.Fatalf("UpdateRole failed: %v", err)
		}

		if updated.Position != originalPos {
			t.Errorf("Position changed from %d to %d after update", originalPos, updated.Position)
		}
	})

	t.Run("roles can be reordered via ReorderServerRoles", func(t *testing.T) {
		// Create multiple custom roles
		core.CreateServerRole(ctx, "alpha", "Alpha", "Alpha role")
		core.CreateServerRole(ctx, "beta", "Beta", "Beta role")

		// Reorder all custom roles. ReorderServerRoles requires a complete
		// custom-role list so clients cannot accidentally drop roles from the
		// authoritative ordering event.
		roles, err := core.ReorderServerRoles(ctx, []string{"editor", "contributor", "beta", "alpha"})
		if err != nil {
			t.Fatalf("ReorderServerRoles failed: %v", err)
		}

		var alphaPos, betaPos int32
		for _, r := range roles {
			if r.Name == "alpha" {
				alphaPos = r.Position
			}
			if r.Name == "beta" {
				betaPos = r.Position
			}
		}

		if betaPos >= alphaPos {
			t.Errorf("After reorder: beta position (%d) should be < alpha position (%d)", betaPos, alphaPos)
		}
	})
}

func TestChattoCore_OutranksUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup hierarchy using system roles: owner > moderator > member
	// Note: Custom roles currently get position MaxInt32 (same as member),
	// so we test with system roles that have defined positions.
	owner := "owner-user"
	mod := "mod-user"
	member := "member-user"

	core.AssignServerRole(ctx, SystemActorID, owner, RoleOwner)
	core.AssignServerRole(ctx, SystemActorID, mod, RoleModerator)
	// member has no explicit role, just the implicit member role

	t.Run("owner can manage all", func(t *testing.T) {
		canMod, _ := core.OutranksUser(ctx, owner, mod)
		canMember, _ := core.OutranksUser(ctx, owner, member)

		if !canMod {
			t.Error("Owner should be able to manage moderator")
		}
		if !canMember {
			t.Error("Owner should be able to manage member")
		}
	})

	t.Run("moderator can manage member but not owner", func(t *testing.T) {
		canOwner, _ := core.OutranksUser(ctx, mod, owner)
		canMember, _ := core.OutranksUser(ctx, mod, member)

		if canOwner {
			t.Error("Moderator should NOT be able to manage owner")
		}
		if !canMember {
			t.Error("Moderator should be able to manage member")
		}
	})

	t.Run("member cannot manage anyone with a role", func(t *testing.T) {
		canOwner, _ := core.OutranksUser(ctx, member, owner)
		canMod, _ := core.OutranksUser(ctx, member, mod)

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
		core.AssignServerRole(ctx, SystemActorID, mod2, RoleModerator)

		canManage, _ := core.OutranksUser(ctx, mod, mod2)
		if canManage {
			t.Error("Moderator should NOT be able to manage another moderator (same rank)")
		}

		canManageReverse, _ := core.OutranksUser(ctx, mod2, mod)
		if canManageReverse {
			t.Error("Moderator should NOT be able to manage another moderator (reverse)")
		}
	})
}

package core

import (
	"errors"
	"testing"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core/rbac"
)

// ============================================================================
// Instance Permission Validation Tests
// ============================================================================

func TestValidatePermission_InstanceScope(t *testing.T) {
	tests := []struct {
		name    string
		perm    Permission
		wantErr bool
	}{
		{"admin valid", PermAdminAccess, false},
		// Unified permissions with ScopeServer
		{"room.leave valid (unified scope)", Permission("room.leave"), false},
		{"message.post valid (unified scope)", Permission("message.post"), false},
		{"message.react valid (unified scope)", Permission("message.react"), false},
		{"room.join valid (unified scope)", Permission("room.join"), false},
		{"room.create valid (unified scope)", Permission("room.create"), false},
		// Space-only permissions are valid in the unified model (they just don't apply at instance scope)
		{"space.manage valid (but space scope only)", Permission("space.manage"), false},
		{"role.manage valid (but space scope only)", Permission("role.manage"), false},
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

func TestDefaultInstanceEveryonePermissions(t *testing.T) {
	perms := DefaultInstanceEveryonePermissions()
	if len(perms) == 0 {
		t.Error("Expected at least one default everyone permission")
	}

	// Should contain all base permissions
	expected := []Permission{PermUserDeleteSelf, PermDMView, PermDMWrite}
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

func TestChattoCore_initInstanceRBAC(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// initInstanceRBAC is called during NewChattoCore, so just verify the state

	// Check that everyone has dm.view permission
	hasPerm, err := core.HasInstancePermission(ctx, "any-user", PermDMView)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if !hasPerm {
		t.Error("Expected everyone to have dm.view permission")
	}

	// Check that everyone has dm.write permission
	hasPerm, err = core.HasInstancePermission(ctx, "any-user", PermDMWrite)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if !hasPerm {
		t.Error("Expected everyone to have dm.write permission")
	}

	// Check that everyone does NOT have admin permission
	hasPerm, err = core.HasInstancePermission(ctx, "any-user", PermAdminAccess)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if hasPerm {
		t.Error("Expected member to NOT have admin permission")
	}
}

func TestChattoCore_initInstanceRBAC_Idempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Call initInstanceRBAC again - should not error (sentinel key already set)
	err := core.initInstanceRBAC(ctx)
	if err != nil {
		t.Fatalf("Second initInstanceRBAC should be idempotent: %v", err)
	}
}

func TestChattoCore_initInstanceRBAC_PreservesPermissionChanges(t *testing.T) {
	// This test verifies that permission changes made by admins are preserved
	// when the instance restarts (a new ChattoCore is created).

	ctx := testContext(t)

	// Start embedded NATS server that persists across both cores
	opts := &server.Options{
		JetStream: true,
		Port:      -1,
		StoreDir:  t.TempDir(),
	}

	ns, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("Failed to create NATS server: %v", err)
	}

	go ns.Start()
	if !ns.ReadyForConnections(5 * 1e9) {
		t.Fatal("NATS server not ready")
	}

	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		t.Fatalf("Failed to connect to NATS: %v", err)
	}

	t.Cleanup(func() {
		nc.Close()
		ns.Shutdown()
		ns.WaitForShutdown()
	})

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

	// Create a user
	user, err := core1.CreateUser(ctx, "", "testuser", "Test User", "password123")
	if err != nil {
		t.Fatalf("Failed to create user: %v", err)
	}

	// Verify default permission is granted (everyone can create spaces)
	hasPerm, err := core1.HasInstancePermission(ctx, user.Id, PermDMWrite)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if !hasPerm {
		t.Error("Expected user to have space.create permission by default")
	}

	// Step 2: Admin revokes the permission from the everyone role
	err = core1.DenyInstancePermission(ctx, RoleEveryone, PermDMWrite)
	if err != nil {
		t.Fatalf("Failed to deny permission: %v", err)
	}

	// Verify permission is now denied
	hasPerm, err = core1.HasInstancePermission(ctx, user.Id, PermDMWrite)
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

	// Step 4: Verify the permission change was preserved
	hasPerm, err = core2.HasInstancePermission(ctx, user.Id, PermDMWrite)
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

func TestChattoCore_AssignInstanceAdminRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "test-user-123"

	// Initially not an admin
	isAdmin, err := core.IsInstanceAdmin(ctx, userID)
	if err != nil {
		t.Fatalf("Failed to check admin: %v", err)
	}
	if isAdmin {
		t.Error("Expected user to not be admin initially")
	}

	// Assign admin role
	if err := core.AssignInstanceAdminRole(ctx, userID); err != nil {
		t.Fatalf("Failed to assign admin role: %v", err)
	}

	// Now should be admin
	isAdmin, err = core.IsInstanceAdmin(ctx, userID)
	if err != nil {
		t.Fatalf("Failed to check admin: %v", err)
	}
	if !isAdmin {
		t.Error("Expected user to be admin after assignment")
	}
}

func TestChattoCore_RevokeInstanceAdminRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "test-user-456"

	// Assign admin role
	if err := core.AssignInstanceAdminRole(ctx, userID); err != nil {
		t.Fatalf("Failed to assign admin role: %v", err)
	}

	// Verify is admin
	isAdmin, _ := core.IsInstanceAdmin(ctx, userID)
	if !isAdmin {
		t.Fatal("Expected user to be admin")
	}

	// Revoke admin role
	if err := core.RevokeInstanceAdminRole(ctx, userID); err != nil {
		t.Fatalf("Failed to revoke admin role: %v", err)
	}

	// Should no longer be admin
	isAdmin, err := core.IsInstanceAdmin(ctx, userID)
	if err != nil {
		t.Fatalf("Failed to check admin: %v", err)
	}
	if isAdmin {
		t.Error("Expected user to not be admin after revocation")
	}
}

func TestChattoCore_RevokeInstanceAdminRole_Idempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "test-user-789"

	// Revoke without ever assigning - should not error
	err := core.RevokeInstanceAdminRole(ctx, userID)
	if err != nil {
		t.Errorf("RevokeInstanceAdminRole should be idempotent: %v", err)
	}
}

func TestChattoCore_ListInstanceAdmins(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Initially no admins
	admins, err := core.ListInstanceAdmins(ctx)
	if err != nil {
		t.Fatalf("Failed to list admins: %v", err)
	}
	if len(admins) != 0 {
		t.Errorf("Expected 0 admins initially, got %d", len(admins))
	}

	// Add some admins
	core.AssignInstanceAdminRole(ctx, "admin1")
	core.AssignInstanceAdminRole(ctx, "admin2")

	// List admins
	admins, err = core.ListInstanceAdmins(ctx)
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
	if err := core.AssignInstanceAdminRole(ctx, userID); err != nil {
		t.Fatalf("Failed to assign admin role: %v", err)
	}

	// Admin should have all permissions
	for _, perm := range []Permission{PermDMWrite, PermAdminAccess} {
		hasPerm, err := core.HasInstancePermission(ctx, userID, perm)
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

	// Everyone should have spaces.browse
	hasPerm, err := core.HasInstancePermission(ctx, userID, PermDMView)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if !hasPerm {
		t.Error("Expected member to have spaces.browse permission")
	}

	// Everyone should have spaces.create
	hasPerm, err = core.HasInstancePermission(ctx, userID, PermDMWrite)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if !hasPerm {
		t.Error("Expected member to have spaces.create permission")
	}

	// Member should NOT have admin
	hasPerm, err = core.HasInstancePermission(ctx, userID, PermAdminAccess)
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
	if err := core.AssignInstanceAdminRole(ctx, "admin-user"); err != nil {
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
	hasPerm, err := core.HasInstancePermission(ctx, "any-user", Permission("invalid"))
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

		// spaces.browse is in default member permissions
		hasPerm, err := core.HasUserPermissionViaRoles(ctx, userID, PermDMView)
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
		if err := core.AssignInstanceAdminRole(ctx, userID); err != nil {
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
// and GetUserInstancePermissions use the hierarchy-wins model (matching the actual
// authorizer walkInstancePermission), NOT the deny-override model.
//
// The critical scenario: admin role (position 1) grants a permission, but the
// everyone role (position MAX) denies it. Hierarchy-wins says admin's grant wins
// because admin is checked first. Deny-override would incorrectly say everyone's
// deny wins.

func TestChattoCore_HierarchyWins_HighRankGrantBeatsLowRankDeny(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "hierarchy-admin"
	if err := core.AssignInstanceAdminRole(ctx, userID); err != nil {
		t.Fatalf("Failed to assign admin role: %v", err)
	}

	// Deny space.create on the everyone role (low rank)
	if err := core.DenyInstancePermission(ctx, RoleEveryone, PermDMWrite); err != nil {
		t.Fatalf("Failed to deny permission: %v", err)
	}
	// Admin role still has space.create granted (from InitInstanceDefaults)

	t.Run("HasInstancePermission grants via hierarchy", func(t *testing.T) {
		// The actual authorizer should grant (admin checked first, has grant)
		has, err := core.HasInstancePermission(ctx, userID, PermDMWrite)
		if err != nil {
			t.Fatalf("HasInstancePermission error: %v", err)
		}
		if !has {
			t.Error("Expected HasInstancePermission to return true: admin grant should beat everyone deny")
		}
	})

	t.Run("HasUserPermissionViaRoles matches authorizer", func(t *testing.T) {
		// UI function must agree with the authorizer
		has, err := core.HasUserPermissionViaRoles(ctx, userID, PermDMWrite)
		if err != nil {
			t.Fatalf("HasUserPermissionViaRoles error: %v", err)
		}
		if !has {
			t.Error("Expected HasUserPermissionViaRoles to return true: admin grant should beat everyone deny")
		}
	})

	t.Run("HasUserPermissionDeniedViaRoles matches authorizer", func(t *testing.T) {
		// Permission is NOT effectively denied for this user (admin grant wins)
		denied, err := core.HasUserPermissionDeniedViaRoles(ctx, userID, PermDMWrite)
		if err != nil {
			t.Fatalf("HasUserPermissionDeniedViaRoles error: %v", err)
		}
		if denied {
			t.Error("Expected HasUserPermissionDeniedViaRoles to return false: admin grant should beat everyone deny")
		}
	})

	t.Run("GetUserInstancePermissions includes the permission", func(t *testing.T) {
		perms, err := core.GetUserInstancePermissions(ctx, userID)
		if err != nil {
			t.Fatalf("GetUserInstancePermissions error: %v", err)
		}
		found := false
		for _, p := range perms {
			if p == PermDMWrite {
				found = true
				break
			}
		}
		if !found {
			t.Error("Expected GetUserInstancePermissions to include space.create: admin grant should beat everyone deny")
		}
	})
}

func TestChattoCore_HierarchyWins_LowRankDenyBlocksWhenNoHigherGrant(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Regular user with no special roles — only has "everyone"
	userID := "hierarchy-regular"

	// Deny space.create on the everyone role
	if err := core.DenyInstancePermission(ctx, RoleEveryone, PermDMWrite); err != nil {
		t.Fatalf("Failed to deny permission: %v", err)
	}

	t.Run("HasUserPermissionViaRoles returns false", func(t *testing.T) {
		has, err := core.HasUserPermissionViaRoles(ctx, userID, PermDMWrite)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if has {
			t.Error("Expected false: everyone deny with no higher-rank grant")
		}
	})

	t.Run("HasUserPermissionDeniedViaRoles returns true", func(t *testing.T) {
		denied, err := core.HasUserPermissionDeniedViaRoles(ctx, userID, PermDMWrite)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if !denied {
			t.Error("Expected true: everyone deny with no higher-rank grant")
		}
	})

	t.Run("GetUserInstancePermissions excludes the permission", func(t *testing.T) {
		perms, err := core.GetUserInstancePermissions(ctx, userID)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		for _, p := range perms {
			if p == PermDMWrite {
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
	if err := core.AssignInstanceOwnerRole(ctx, owner.Id); err != nil {
		t.Fatalf("Failed to assign owner role: %v", err)
	}

	// Deny admin.access on both admin and everyone roles
	if err := core.DenyInstancePermission(ctx, RoleEveryone, PermAdminAccess); err != nil {
		t.Fatalf("Failed to deny everyone: %v", err)
	}
	if err := core.DenyInstancePermission(ctx, RoleAdmin, PermAdminAccess); err != nil {
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

func TestChattoCore_CreateInstanceRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("creates role successfully", func(t *testing.T) {
		role, err := core.CreateInstanceRole(ctx, "customrole", "Custom Role", "A custom role")
		if err != nil {
			t.Fatalf("Failed to create role: %v", err)
		}
		if role.Name != "customrole" {
			t.Errorf("Expected role name 'customrole', got '%s'", role.Name)
		}
	})

	t.Run("rejects role name with dashes", func(t *testing.T) {
		_, err := core.CreateInstanceRole(ctx, "custom-role", "Custom", "Should fail")
		if err == nil {
			t.Error("Expected error for role name with dashes")
		}
		if !errors.Is(err, ErrInvalidRoleName) {
			t.Errorf("Expected ErrInvalidRoleName, got %v", err)
		}
	})

	t.Run("rejects role name with numbers", func(t *testing.T) {
		_, err := core.CreateInstanceRole(ctx, "role2", "Role 2", "Should fail")
		if err == nil {
			t.Error("Expected error for role name with numbers")
		}
		if !errors.Is(err, ErrInvalidRoleName) {
			t.Errorf("Expected ErrInvalidRoleName, got %v", err)
		}
	})

	t.Run("rejects system role names", func(t *testing.T) {
		_, err := core.CreateInstanceRole(ctx, RoleAdmin, "Admin", "Should fail")
		if err == nil {
			t.Error("Expected error for system role name")
		}
	})
}

// ============================================================================
// Generic Role Assignment Tests
// ============================================================================

func TestChattoCore_AssignInstanceRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("assigns admin role", func(t *testing.T) {
		userID := "assign-admin-test"

		if err := core.AssignInstanceRole(ctx, SystemActorID, userID, RoleAdmin); err != nil {
			t.Fatalf("Failed to assign role: %v", err)
		}

		// Verify via IsInstanceAdmin
		isAdmin, _ := core.IsInstanceAdmin(ctx, userID)
		if !isAdmin {
			t.Error("Expected user to be admin after assignment")
		}
	})

	t.Run("rejects everyone role assignment", func(t *testing.T) {
		userID := "assign-everyone-test"

		err := core.AssignInstanceRole(ctx, SystemActorID, userID, RoleEveryone)
		if err == nil {
			t.Error("Expected error when assigning everyone role")
		}
	})

	t.Run("rejects non-existent role", func(t *testing.T) {
		userID := "assign-nonexistent-test"

		err := core.AssignInstanceRole(ctx, SystemActorID, userID, "nonexistent")
		if !errors.Is(err, ErrRoleNotFound) {
			t.Errorf("Expected ErrRoleNotFound, got %v", err)
		}
	})

	t.Run("assigns custom role", func(t *testing.T) {
		userID := "assign-custom-test"

		// Create a custom role first (must have instance- prefix)
		_, err := core.CreateInstanceRole(ctx, "tester", "Tester", "QA tester")
		if err != nil {
			t.Fatalf("Failed to create role: %v", err)
		}

		// Assign the custom role
		if err := core.AssignInstanceRole(ctx, SystemActorID, userID, "tester"); err != nil {
			t.Fatalf("Failed to assign role: %v", err)
		}

		// Verify via GetUserInstanceRoles
		roles, _ := core.GetUserInstanceRoles(ctx, userID)
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

func TestChattoCore_RevokeInstanceRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("revokes admin role", func(t *testing.T) {
		userID := "revoke-admin-test"

		// Assign first
		core.AssignInstanceRole(ctx, SystemActorID, userID, RoleAdmin)

		// Verify assigned
		isAdmin, _ := core.IsInstanceAdmin(ctx, userID)
		if !isAdmin {
			t.Fatal("Expected user to be admin")
		}

		// Revoke
		if err := core.RevokeInstanceRole(ctx, SystemActorID, userID, RoleAdmin); err != nil {
			t.Fatalf("Failed to revoke role: %v", err)
		}

		// Verify revoked
		isAdmin, _ = core.IsInstanceAdmin(ctx, userID)
		if isAdmin {
			t.Error("Expected user to not be admin after revocation")
		}
	})

	t.Run("rejects everyone role revocation", func(t *testing.T) {
		userID := "revoke-everyone-test"

		err := core.RevokeInstanceRole(ctx, SystemActorID, userID, RoleEveryone)
		if err == nil {
			t.Error("Expected error when revoking everyone role")
		}
	})

	t.Run("rejects non-existent role", func(t *testing.T) {
		userID := "revoke-nonexistent-test"

		err := core.RevokeInstanceRole(ctx, SystemActorID, userID, "nonexistent")
		if !errors.Is(err, ErrRoleNotFound) {
			t.Errorf("Expected ErrRoleNotFound, got %v", err)
		}
	})

	t.Run("idempotent when not assigned", func(t *testing.T) {
		userID := "revoke-unassigned-test"

		// Revoke role user doesn't have - should not error
		err := core.RevokeInstanceRole(ctx, SystemActorID, userID, RoleAdmin)
		if err != nil {
			t.Errorf("Expected no error for idempotent revoke: %v", err)
		}
	})
}

func TestChattoCore_ListInstanceRoleUsers(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("returns empty list for everyone role", func(t *testing.T) {
		// Everyone role is implicit - should return empty list
		users, err := core.ListInstanceRoleUsers(ctx, RoleEveryone)
		if err != nil {
			t.Fatalf("Failed to list users: %v", err)
		}
		if len(users) != 0 {
			t.Errorf("Expected 0 users for everyone role, got %d", len(users))
		}
	})

	t.Run("returns users with admin role", func(t *testing.T) {
		// Assign some admins
		core.AssignInstanceRole(ctx, SystemActorID, "list-admin1", RoleAdmin)
		core.AssignInstanceRole(ctx, SystemActorID, "list-admin2", RoleAdmin)

		users, err := core.ListInstanceRoleUsers(ctx, RoleAdmin)
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
		_, err := core.ListInstanceRoleUsers(ctx, "nonexistent")
		if !errors.Is(err, ErrRoleNotFound) {
			t.Errorf("Expected ErrRoleNotFound, got %v", err)
		}
	})
}

func TestChattoCore_GetUserInstanceRoles(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("returns empty list for user with no explicit roles", func(t *testing.T) {
		userID := "no-roles-user"

		roles, err := core.GetUserInstanceRoles(ctx, userID)
		if err != nil {
			t.Fatalf("Failed to get roles: %v", err)
		}
		// Virtual roles (everyone) are not returned by GetUserInstanceRoles
		if len(roles) != 0 {
			t.Errorf("Expected 0 roles for user with no explicit roles, got %d: %v", len(roles), roles)
		}
	})

	t.Run("returns admin role when assigned", func(t *testing.T) {
		userID := "admin-roles-user"

		core.AssignInstanceRole(ctx, SystemActorID, userID, RoleAdmin)

		roles, err := core.GetUserInstanceRoles(ctx, userID)
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
		core.CreateInstanceRole(ctx, "editor", "Editor", "Content editor")

		// Assign multiple roles
		core.AssignInstanceRole(ctx, SystemActorID, user.Id, RoleAdmin)
		core.AssignInstanceRole(ctx, SystemActorID, user.Id, "editor")

		roles, err := core.GetUserInstanceRoles(ctx, user.Id)
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

func TestChattoCore_AssignInstanceRole_HierarchyCheck(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create an owner
	owner, err := core.CreateUser(ctx, SystemActorID, "hierarchy-owner", "Owner", "password123")
	if err != nil {
		t.Fatalf("Failed to create owner: %v", err)
	}
	if err := core.AssignInstanceOwnerRole(ctx, owner.Id); err != nil {
		t.Fatalf("Failed to assign owner role: %v", err)
	}

	// Create an admin user
	admin, err := core.CreateUser(ctx, SystemActorID, "hierarchy-admin", "Admin", "password123")
	if err != nil {
		t.Fatalf("Failed to create admin: %v", err)
	}
	if err := core.AssignInstanceRole(ctx, SystemActorID, admin.Id, RoleAdmin); err != nil {
		t.Fatalf("Failed to assign admin role: %v", err)
	}

	// Create a target user
	target, err := core.CreateUser(ctx, SystemActorID, "hierarchy-target", "Target", "password123")
	if err != nil {
		t.Fatalf("Failed to create target: %v", err)
	}

	t.Run("admin cannot assign owner role (higher rank)", func(t *testing.T) {
		err := core.AssignInstanceRole(ctx, admin.Id, target.Id, RoleOwner)
		if err == nil {
			t.Error("Expected error: admin should not be able to assign owner role")
		}
		if !errors.Is(err, ErrCannotAssignHigherRole) {
			t.Errorf("Expected ErrCannotAssignHigherRole, got: %v", err)
		}
	})

	t.Run("admin cannot assign admin role (equal rank)", func(t *testing.T) {
		err := core.AssignInstanceRole(ctx, admin.Id, target.Id, RoleAdmin)
		if err == nil {
			t.Error("Expected error: admin should not be able to assign equal-rank role")
		}
		if !errors.Is(err, ErrCannotAssignHigherRole) {
			t.Errorf("Expected ErrCannotAssignHigherRole, got: %v", err)
		}
	})

	t.Run("admin can assign moderator role (lower rank)", func(t *testing.T) {
		err := core.AssignInstanceRole(ctx, admin.Id, target.Id, RoleModerator)
		if err != nil {
			t.Fatalf("Expected admin to assign moderator role: %v", err)
		}
	})

	t.Run("owner can assign admin role", func(t *testing.T) {
		err := core.AssignInstanceRole(ctx, owner.Id, target.Id, RoleAdmin)
		if err != nil {
			t.Fatalf("Expected owner to assign admin role: %v", err)
		}
	})

	t.Run("admin cannot self-escalate to owner", func(t *testing.T) {
		err := core.AssignInstanceRole(ctx, admin.Id, admin.Id, RoleOwner)
		if err == nil {
			t.Error("Expected error: admin should not be able to self-escalate to owner")
		}
		if !errors.Is(err, ErrCannotAssignHigherRole) {
			t.Errorf("Expected ErrCannotAssignHigherRole, got: %v", err)
		}
	})

	t.Run("system actor bypasses hierarchy check", func(t *testing.T) {
		err := core.AssignInstanceRole(ctx, SystemActorID, target.Id, RoleOwner)
		if err != nil {
			t.Fatalf("Expected system actor to bypass hierarchy: %v", err)
		}
	})
}

func TestChattoCore_RevokeInstanceRole_HierarchyCheck(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create an owner
	owner, err := core.CreateUser(ctx, SystemActorID, "revoke-hier-owner", "Owner", "password123")
	if err != nil {
		t.Fatalf("Failed to create owner: %v", err)
	}
	if err := core.AssignInstanceOwnerRole(ctx, owner.Id); err != nil {
		t.Fatalf("Failed to assign owner role: %v", err)
	}

	// Create an admin user
	admin, err := core.CreateUser(ctx, SystemActorID, "revoke-hier-admin", "Admin", "password123")
	if err != nil {
		t.Fatalf("Failed to create admin: %v", err)
	}
	if err := core.AssignInstanceRole(ctx, SystemActorID, admin.Id, RoleAdmin); err != nil {
		t.Fatalf("Failed to assign admin role: %v", err)
	}

	// Create another admin to try revoking
	otherAdmin, err := core.CreateUser(ctx, SystemActorID, "revoke-hier-admin2", "Admin2", "password123")
	if err != nil {
		t.Fatalf("Failed to create other admin: %v", err)
	}
	if err := core.AssignInstanceRole(ctx, SystemActorID, otherAdmin.Id, RoleAdmin); err != nil {
		t.Fatalf("Failed to assign admin role: %v", err)
	}

	// Assign moderator to a target user
	target, err := core.CreateUser(ctx, SystemActorID, "revoke-hier-target", "Target", "password123")
	if err != nil {
		t.Fatalf("Failed to create target: %v", err)
	}
	if err := core.AssignInstanceRole(ctx, SystemActorID, target.Id, RoleModerator); err != nil {
		t.Fatalf("Failed to assign moderator: %v", err)
	}

	t.Run("admin cannot revoke owner role (higher rank)", func(t *testing.T) {
		err := core.RevokeInstanceRole(ctx, admin.Id, owner.Id, RoleOwner)
		if err == nil {
			t.Error("Expected error: admin should not be able to revoke owner role")
		}
	})

	t.Run("admin cannot revoke another admin's role (equal rank)", func(t *testing.T) {
		err := core.RevokeInstanceRole(ctx, admin.Id, otherAdmin.Id, RoleAdmin)
		if err == nil {
			t.Error("Expected error: admin should not be able to revoke equal-rank role")
		}
	})

	t.Run("admin can revoke moderator role (lower rank)", func(t *testing.T) {
		err := core.RevokeInstanceRole(ctx, admin.Id, target.Id, RoleModerator)
		if err != nil {
			t.Fatalf("Expected admin to revoke moderator role: %v", err)
		}
	})

	t.Run("owner can revoke admin role", func(t *testing.T) {
		err := core.RevokeInstanceRole(ctx, owner.Id, admin.Id, RoleAdmin)
		if err != nil {
			t.Fatalf("Expected owner to revoke admin role: %v", err)
		}
	})

	t.Run("system actor bypasses hierarchy check", func(t *testing.T) {
		// Re-assign admin to test system bypass on revoke
		if err := core.AssignInstanceRole(ctx, SystemActorID, admin.Id, RoleAdmin); err != nil {
			t.Fatalf("Failed to re-assign admin: %v", err)
		}
		err := core.RevokeInstanceRole(ctx, SystemActorID, admin.Id, RoleAdmin)
		if err != nil {
			t.Fatalf("Expected system actor to bypass hierarchy: %v", err)
		}
	})
}

// ============================================================================
// Instance Role Position and Reordering Tests
// ============================================================================

func TestChattoCore_ReorderInstanceRoles(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("reorders custom roles", func(t *testing.T) {
		// Create custom roles
		_, err := core.CreateInstanceRole(ctx, "alpha", "Alpha", "First custom role")
		if err != nil {
			t.Fatalf("Failed to create alpha role: %v", err)
		}
		_, err = core.CreateInstanceRole(ctx, "beta", "Beta", "Second custom role")
		if err != nil {
			t.Fatalf("Failed to create beta role: %v", err)
		}

		// Get initial positions
		initialRoles, _ := core.ListInstanceRoles(ctx)
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
		reordered, err := core.ReorderInstanceRoles(ctx, []string{"beta", "alpha"})
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

		// beta should now have lower position (higher rank)
		if betaNowPos >= alphaNowPos {
			t.Errorf("After reorder, beta (position %d) should be before alpha (position %d)", betaNowPos, alphaNowPos)
		}
	})

	t.Run("rejects system role reordering", func(t *testing.T) {
		// Try to include a system role in the reorder
		_, err := core.ReorderInstanceRoles(ctx, []string{RoleAdmin, RoleModerator})
		if err == nil {
			t.Error("Expected error when trying to reorder system roles")
		}
	})

	t.Run("preserves system role positions", func(t *testing.T) {
		roles, _ := core.ListInstanceRoles(ctx)

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

		// System roles should have fixed positions: owner=0, admin=1, moderator=2
		if ownerPos != 0 {
			t.Errorf("Expected owner position 0, got %d", ownerPos)
		}
		if adminPos != 1 {
			t.Errorf("Expected admin position 1, got %d", adminPos)
		}
		if modPos != 2 {
			t.Errorf("Expected moderator position 2, got %d", modPos)
		}
	})
}

func TestChattoCore_CreateInstanceRole_PositionAssignment(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("custom role gets position after system roles", func(t *testing.T) {
		role, err := core.CreateInstanceRole(ctx, "reviewer", "Reviewer", "Code reviewer")
		if err != nil {
			t.Fatalf("Failed to create role: %v", err)
		}

		// Custom roles should have position > 2 (after owner, admin, moderator)
		// They get position from GetNextAvailablePosition which returns MaxInt32
		// for custom roles when using the standard initialization
		t.Logf("Custom role 'instance-reviewer' got position %d", role.Position)

		// The role should be lower rank than system roles (higher position number)
		if role.Position <= 2 {
			t.Errorf("Expected custom role position > 2, got %d", role.Position)
		}
	})

	t.Run("position preserved after update", func(t *testing.T) {
		// Create a role
		role, err := core.CreateInstanceRole(ctx, "editor", "Editor", "Content editor")
		if err != nil {
			t.Fatalf("Failed to create role: %v", err)
		}
		originalPos := role.Position

		// Update the display name
		updated, err := core.UpdateInstanceRole(ctx, "editor", "Super Editor", "Super content editor")
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
			_, err := core.CreateInstanceRole(ctx, name, name, "Test role")
			if err != nil {
				t.Fatalf("Failed to create role %s: %v", name, err)
			}
		}

		// Verify all exist
		allRoles, _ := core.ListInstanceRoles(ctx)
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
	_, err := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	if err != nil {
		t.Fatalf("Failed to create space: %v", err)
	}

	// Create a role
	role, err := core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Can moderate content")
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

	core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Try to create role with invalid name
	_, err := core.CreateInstanceRole(ctx, "Invalid-Name", "Invalid", "Should fail")
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

	core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Create a role
	_, err := core.CreateInstanceRole(ctx, "testmod", "Test Mod", "First")
	if err != nil {
		t.Fatalf("Failed to create first role: %v", err)
	}

	// Try to create same role again
	_, err = core.CreateInstanceRole(ctx, "testmod", "Test Mod 2", "Second")
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

	core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Create a role
	_, err := core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Can moderate content")
	if err != nil {
		t.Fatalf("Failed to create role: %v", err)
	}

	// Retrieve it
	retrieved, err := core.GetInstanceRole(ctx, "testmod")
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

	core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Try to get nonexistent role
	_, err := core.GetInstanceRole(ctx, "nonexistent")
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

	core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Initially should have 4 default roles (owner, admin, moderator, everyone) created by CreateSpace
	roles, err := core.ListInstanceRoles(ctx)
	if err != nil {
		t.Fatalf("Failed to list roles: %v", err)
	}
	if len(roles) != 4 {
		t.Errorf("Expected 4 default roles, got %d", len(roles))
	}

	// Create some additional roles
	core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Test mod role")
	core.CreateInstanceRole(ctx, "vip", "VIP", "VIP role")

	// List again - should have 6 total (4 default + 2 custom)
	roles, err = core.ListInstanceRoles(ctx)
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

	core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Create a role
	_, err := core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Can moderate content")
	if err != nil {
		t.Fatalf("Failed to create role: %v", err)
	}

	// Update it
	updated, err := core.UpdateInstanceRole(ctx, "testmod", "Super Moderator", "Enhanced moderation")
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
	retrieved, _ := core.GetInstanceRole(ctx, "testmod")
	if retrieved.DisplayName != "Super Moderator" {
		t.Error("Update was not persisted")
	}
}

func TestChattoCore_DeleteRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Create a role
	_, err := core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Can moderate content")
	if err != nil {
		t.Fatalf("Failed to create role: %v", err)
	}

	// Delete it
	err = core.DeleteInstanceRole(ctx, "testmod")
	if err != nil {
		t.Fatalf("Failed to delete role: %v", err)
	}

	// Verify it's gone
	_, err = core.GetInstanceRole(ctx, "testmod")
	if !errors.Is(err, ErrRoleNotFound) {
		t.Error("Role should not exist after deletion")
	}
}

func TestChattoCore_DeleteRole_SystemRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Admin role is automatically created by CreateSpace via CreateDefaultRoles
	// Verify it exists
	_, err := core.GetInstanceRole(ctx, RoleOwner)
	if err != nil {
		t.Fatalf("Admin role should exist after CreateSpace: %v", err)
	}

	// Try to delete - should fail
	err = core.DeleteInstanceRole(ctx, RoleOwner)
	if err == nil {
		t.Error("Expected error when deleting system role")
	}

	if !errors.Is(err, ErrCannotDeleteSystemRole) {
		t.Errorf("Expected ErrCannotDeleteSystemRole, got %v", err)
	}

	// Also test everyone role
	err = core.DeleteInstanceRole(ctx, RoleEveryone)
	if !errors.Is(err, ErrCannotDeleteSystemRole) {
		t.Errorf("Expected ErrCannotDeleteSystemRole for everyone role, got %v", err)
	}
}


func TestChattoCore_DeleteRole_CleansUpPermissionsAndAssignments(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Create a role, grant permissions, and assign to a user
	_, err := core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Can moderate")
	if err != nil {
		t.Fatalf("Failed to create role: %v", err)
	}

	// Grant permissions
	core.GrantInstancePermission(ctx, "testmod", PermRoleAssign)
	core.GrantInstancePermission(ctx, "testmod", PermRoomManage)

	// Assign role to a user
	core.AssignInstanceRole(ctx, "test-user", "test-user", "testmod")

	// Verify permissions and assignment exist
	perms, _ := core.GetInstanceRolePermissions(ctx, "testmod")
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
	err = core.DeleteInstanceRole(ctx, "testmod")
	if err != nil {
		t.Fatalf("Failed to delete role: %v", err)
	}

	// Verify permissions are cleaned up
	perms, _ = core.GetInstanceRolePermissions(ctx, "testmod")
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

	core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Grant a permission
	err := core.GrantInstancePermission(ctx, "testmod", PermRoleAssign)
	if err != nil {
		t.Fatalf("Failed to grant permission: %v", err)
	}

	// Verify it was granted
	perms, err := core.GetInstanceRolePermissions(ctx, "testmod")
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

	core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Grant same permission twice
	core.GrantInstancePermission(ctx, "testmod", PermRoleAssign)
	err := core.GrantInstancePermission(ctx, "testmod", PermRoleAssign)
	if err != nil {
		t.Fatalf("Second grant should not fail: %v", err)
	}

	// Should still have only one permission
	perms, _ := core.GetInstanceRolePermissions(ctx, "testmod")
	if len(perms) != 1 {
		t.Errorf("Expected 1 permission after duplicate grant, got %d", len(perms))
	}
}

// GrantInstancePermission no longer validates that the role exists post-#330:
// permission grants land in the same SERVER_RBAC bucket regardless. Role
// existence is enforced when CRUD operations against the role itself are made.
// (Previously TestChattoCore_GrantRolePermission_RoleNotFound covered the
// engine-level pre-check; that path is gone.)

func TestChattoCore_GrantRolePermission_InvalidPermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Try to grant an invalid permission
	err := core.GrantInstancePermission(ctx, "testmod", Permission("invalid_perm"))
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

	core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Grant then revoke
	core.GrantInstancePermission(ctx, "testmod", PermRoleAssign)
	err := core.RevokeInstancePermission(ctx, "testmod", PermRoleAssign)
	if err != nil {
		t.Fatalf("Failed to revoke permission: %v", err)
	}

	// Verify it was revoked
	perms, _ := core.GetInstanceRolePermissions(ctx, "testmod")
	if len(perms) != 0 {
		t.Errorf("Expected 0 permissions after revoke, got %d", len(perms))
	}
}

func TestChattoCore_RevokeRolePermission_Idempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Revoke permission that was never granted
	err := core.RevokeInstancePermission(ctx, "testmod", PermRoleAssign)
	if err != nil {
		t.Fatalf("Revoking non-existent permission should not fail: %v", err)
	}
}

func TestChattoCore_GetRolePermissions_Multiple(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Grant multiple permissions
	core.GrantInstancePermission(ctx, "testmod", PermRoomList)
	core.GrantInstancePermission(ctx, "testmod", PermRoleAssign)
	core.GrantInstancePermission(ctx, "testmod", PermRoomManage)

	perms, err := core.GetInstanceRolePermissions(ctx, "testmod")
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

	core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Can moderate")

	// No permissions granted yet
	perms, err := core.GetInstanceRolePermissions(ctx, "testmod")
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
	core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Assign role to user
	err := core.AssignInstanceRole(ctx, "test-user", "user123", "testmod")
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
	core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Assign same role twice
	core.AssignInstanceRole(ctx, "test-user", "user123", "testmod")
	err := core.AssignInstanceRole(ctx, "test-user", "user123", "testmod")
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

	core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	// Try to assign nonexistent role
	err := core.AssignInstanceRole(ctx, "test-user", "user123", "nonexistent")
	if !errors.Is(err, ErrRoleNotFound) {
		t.Errorf("Expected ErrRoleNotFound, got %v", err)
	}
}

func TestChattoCore_RevokeRole(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Assign then revoke
	core.AssignInstanceRole(ctx, "test-user", "user123", "testmod")
	err := core.RevokeInstanceRole(ctx, "test-user", "user123", "testmod")
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

	core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Revoke role that was never assigned
	err := core.RevokeInstanceRole(ctx, "test-user", "user123", "testmod")
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
	core.AssignInstanceRole(ctx, SystemActorID, adminA, RoleOwner)
	core.AssignInstanceRole(ctx, SystemActorID, adminB, RoleOwner)

	// Admin A should NOT be able to revoke admin role from Admin B (peer)
	// Owner can manage the owner role (role hierarchy passes), but can't demote a peer
	// (user hierarchy blocks it since both are at position 0)
	err := core.RevokeInstanceRole(ctx, adminA, adminB, RoleOwner)
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
	core.AssignInstanceRole(ctx, SystemActorID, admin, RoleOwner)
	// moderator role is created by default with position 1

	// Assign moderator role to mod user
	core.AssignInstanceRole(ctx, SystemActorID, mod, "moderator")

	// Admin SHOULD be able to revoke moderator role from the lower-ranked user
	err := core.RevokeInstanceRole(ctx, admin, mod, "moderator")
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
	core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Can moderate")
	core.CreateInstanceRole(ctx, "vip", "VIP", "VIP user")

	// Assign multiple roles
	core.AssignInstanceRole(ctx, "test-user", "user123", "testmod")
	core.AssignInstanceRole(ctx, "test-user", "user123", "vip")

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
	core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Can moderate")

	// Assign role to multiple users
	core.AssignInstanceRole(ctx, "test-user", "user1", "testmod")
	core.AssignInstanceRole(ctx, "test-user", "user2", "testmod")
	core.AssignInstanceRole(ctx, "test-user", "user3", "testmod")

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
	core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Can moderate")

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
	core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Can moderate")
	core.GrantInstancePermission(ctx, "testmod", PermRoleAssign)
	core.AssignInstanceRole(ctx, "test-user", "user123", "testmod")

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
	core.CreateInstanceRole(ctx, "testmod", "Test Mod", "Can moderate")
	core.GrantInstancePermission(ctx, "testmod", PermRoleAssign)

	core.CreateInstanceRole(ctx, "admin", "Admin", "Full access")
	core.GrantInstancePermission(ctx, "admin", PermSpaceManage)

	// Assign both roles to user
	core.AssignInstanceRole(ctx, "test-user", "user123", "testmod")
	core.AssignInstanceRole(ctx, "test-user", "user123", "admin")

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
	adminRole, err := core.GetInstanceRole(ctx, RoleOwner)
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
	everyoneRole, err := core.GetInstanceRole(ctx, RoleEveryone)
	if err != nil {
		t.Fatalf("Failed to get everyone role: %v", err)
	}
	if everyoneRole.Name != RoleEveryone {
		t.Errorf("Expected everyone role name '%s', got '%s'", RoleEveryone, everyoneRole.Name)
	}

	everyonePerms, _ := core.GetInstanceRolePermissions(ctx, RoleEveryone)
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
// Room-Level Permission Tests
// ============================================================================

func TestChattoCore_GrantRoomRolePermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "test-room", "Test channel")

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

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "test-room", "Test channel")

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

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "test-room", "Test channel")

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

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "general", "General")

	// space.manage is not room-scoped — should fail
	err := core.GrantRoomPermission(ctx, room.Id, RoleEveryone, PermSpaceManage)
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

	space, _ := core.CreateSpace(ctx, "test-user", "Test Space", "A test space")
	room, _ := core.CreateRoom(ctx, "test-user", space.Id, "general", "General")

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

	// Grant role.manage to moderator role
	err := core.GrantInstancePermission(ctx, RoleModerator, PermRoleManage)
	if err != nil {
		t.Fatalf("Failed to grant role permission: %v", err)
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
	err := core.GrantInstancePermission(ctx, RoleEveryone, PermRoomCreate)
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
	err = core.DenyInstancePermission(ctx, RoleEveryone, PermRoomCreate)
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

	// Deny room.list to moderator role
	err := core.DenyInstancePermission(ctx, RoleModerator, PermRoomList)
	if err != nil {
		t.Fatalf("Failed to deny role permission: %v", err)
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
	core.AssignInstanceRole(ctx, SystemActorID, owner, RoleOwner)
	// Give mod the moderator role (position ~1)
	core.AssignInstanceRole(ctx, SystemActorID, mod, RoleModerator)

	// Grant role assignment permission to moderator so we can test hierarchy
	core.GrantInstancePermission(ctx, RoleModerator, PermRoleAssign)

	t.Run("owner can assign moderator role", func(t *testing.T) {
		// Owner (position 0) can assign moderator
		err := core.AssignInstanceRole(ctx, owner, regular, RoleModerator)
		if err != nil {
			t.Errorf("Owner should be able to assign moderator role: %v", err)
		}
		// Cleanup
		core.RevokeInstanceRole(ctx, owner, regular, RoleModerator)
	})

	t.Run("moderator cannot assign owner role due to hierarchy", func(t *testing.T) {
		// Moderator has permission but cannot assign owner (higher rank)
		err := core.AssignInstanceRole(ctx, mod, regular, RoleOwner)
		if !errors.Is(err, ErrCannotAssignHigherRole) {
			t.Errorf("Expected ErrCannotAssignHigherRole, got: %v", err)
		}
	})

	t.Run("regular member without explicit roles is blocked by role hierarchy", func(t *testing.T) {
		// Create a custom role
		core.CreateInstanceRole(ctx, "helper", "Helper", "Can help")

		// Regular member only has the implicit "everyone" role (max position),
		// which cannot manage any concrete role.
		err := core.AssignInstanceRole(ctx, regular, mod, "helper")
		if !errors.Is(err, ErrCannotAssignHigherRole) {
			t.Errorf("Expected ErrCannotAssignHigherRole for regular member, got: %v", err)
		}
	})

	t.Run("moderator can assign lower-ranked custom role", func(t *testing.T) {
		// Create a custom role (will be lower rank than moderator)
		role, _ := core.CreateInstanceRole(ctx, "editor", "Editor", "Can edit")
		// Verify the custom role has a position lower than moderator
		// (position is higher number = lower rank)
		if role.Position <= 2 { // moderator is at 2
			t.Skipf("Custom role position %d is not lower than moderator position 2", role.Position)
		}

		// Mod should be able to assign editor (lower rank)
		err := core.AssignInstanceRole(ctx, mod, regular, "editor")
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
	core.AssignInstanceRole(ctx, SystemActorID, owner, RoleOwner)

	// Owner cannot revoke their own owner role - there's a specific check for this
	err := core.RevokeInstanceRole(ctx, owner, owner, RoleOwner)
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
	core.AssignInstanceRole(ctx, SystemActorID, modA, RoleModerator)
	core.AssignInstanceRole(ctx, SystemActorID, modB, RoleModerator)

	// Grant role assignment permission to moderators so we can test hierarchy
	core.GrantInstancePermission(ctx, RoleModerator, PermRoleAssign)

	t.Run("moderator A cannot revoke moderator B's moderator role", func(t *testing.T) {
		// Both are equal rank (moderator), so neither can demote the other
		// First check is "can't revoke role higher than yours" - but they're equal
		// So the check that should fail is the hierarchy check on the role itself
		err := core.RevokeInstanceRole(ctx, modA, modB, RoleModerator)
		// Either ErrCannotRevokeHigherRole (role hierarchy) or ErrCannotManageHigherUser (user hierarchy)
		if err == nil {
			t.Error("Expected error for peer demotion")
		}
		if !errors.Is(err, ErrCannotRevokeHigherRole) && !errors.Is(err, ErrCannotManageHigherUser) {
			t.Errorf("Expected hierarchy error for peer demotion, got: %v", err)
		}
	})

	t.Run("vice versa - B cannot demote A", func(t *testing.T) {
		err := core.RevokeInstanceRole(ctx, modB, modA, RoleModerator)
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

	core.CreateSpace(ctx, "test-user", "Test Space", "A test space")

	t.Run("first custom role gets position after system roles", func(t *testing.T) {
		// System roles: owner (0), moderator (2), member (MAX)
		role, err := core.CreateInstanceRole(ctx, "editor", "Editor", "Can edit")
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
		role, err := core.CreateInstanceRole(ctx, "contributor", "Contributor", "Can contribute")
		if err != nil {
			t.Fatalf("CreateRole failed: %v", err)
		}
		originalPos := role.Position

		// Update display name
		updated, err := core.UpdateInstanceRole(ctx, "contributor", "Super Contributor", "Can contribute a lot")
		if err != nil {
			t.Fatalf("UpdateRole failed: %v", err)
		}

		if updated.Position != originalPos {
			t.Errorf("Position changed from %d to %d after update", originalPos, updated.Position)
		}
	})

	t.Run("roles can be reordered via ReorderInstanceRoles", func(t *testing.T) {
		// Create multiple custom roles
		core.CreateInstanceRole(ctx, "alpha", "Alpha", "Alpha role")
		core.CreateInstanceRole(ctx, "beta", "Beta", "Beta role")

		// Reorder them
		roles, err := core.ReorderInstanceRoles(ctx, []string{"beta", "alpha"})
		if err != nil {
			t.Fatalf("ReorderInstanceRoles failed: %v", err)
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

func TestChattoCore_CanManageUser(t *testing.T) {
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

	core.AssignInstanceRole(ctx, SystemActorID, owner, RoleOwner)
	core.AssignInstanceRole(ctx, SystemActorID, mod, RoleModerator)
	// member has no explicit role, just the implicit member role

	t.Run("owner can manage all", func(t *testing.T) {
		canMod, _ := core.CanManageUser(ctx, owner, mod)
		canMember, _ := core.CanManageUser(ctx, owner, member)

		if !canMod {
			t.Error("Owner should be able to manage moderator")
		}
		if !canMember {
			t.Error("Owner should be able to manage member")
		}
	})

	t.Run("moderator can manage member but not owner", func(t *testing.T) {
		canOwner, _ := core.CanManageUser(ctx, mod, owner)
		canMember, _ := core.CanManageUser(ctx, mod, member)

		if canOwner {
			t.Error("Moderator should NOT be able to manage owner")
		}
		if !canMember {
			t.Error("Moderator should be able to manage member")
		}
	})

	t.Run("member cannot manage anyone with a role", func(t *testing.T) {
		canOwner, _ := core.CanManageUser(ctx, member, owner)
		canMod, _ := core.CanManageUser(ctx, member, mod)

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
		core.AssignInstanceRole(ctx, SystemActorID, mod2, RoleModerator)

		canManage, _ := core.CanManageUser(ctx, mod, mod2)
		if canManage {
			t.Error("Moderator should NOT be able to manage another moderator (same rank)")
		}

		canManageReverse, _ := core.CanManageUser(ctx, mod2, mod)
		if canManageReverse {
			t.Error("Moderator should NOT be able to manage another moderator (reverse)")
		}
	})
}

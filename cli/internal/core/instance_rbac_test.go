package core

import (
	"errors"
	"testing"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"hmans.de/chatto/internal/config"
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
		// Unified permissions with ScopeInstance
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
	err = core1.DenyInstanceRolePermission(ctx, RoleEveryone, PermDMWrite)
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
	if err := core.DenyInstanceRolePermission(ctx, RoleEveryone, PermDMWrite); err != nil {
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
	if err := core.DenyInstanceRolePermission(ctx, RoleEveryone, PermDMWrite); err != nil {
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
	if err := core.DenyInstanceRolePermission(ctx, RoleEveryone, PermAdminAccess); err != nil {
		t.Fatalf("Failed to deny everyone: %v", err)
	}
	if err := core.DenyInstanceRolePermission(ctx, RoleAdmin, PermAdminAccess); err != nil {
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

	t.Run("creates role with instance- prefix successfully", func(t *testing.T) {
		role, err := core.CreateInstanceRole(ctx, "instance-customrole", "Custom Role", "A custom instance role")
		if err != nil {
			t.Fatalf("Failed to create instance role: %v", err)
		}
		if role.Name != "instance-customrole" {
			t.Errorf("Expected role name 'instance-customrole', got '%s'", role.Name)
		}
	})

	t.Run("rejects role name without instance- prefix", func(t *testing.T) {
		_, err := core.CreateInstanceRole(ctx, "customrole", "Custom Role", "Should fail")
		if err == nil {
			t.Error("Expected error for role name without instance- prefix")
		}
		if !errors.Is(err, ErrInvalidRoleName) {
			t.Errorf("Expected ErrInvalidRoleName, got %v", err)
		}
	})

	t.Run("rejects role name with numbers in suffix", func(t *testing.T) {
		_, err := core.CreateInstanceRole(ctx, "instance-role2", "Role 2", "Should fail")
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
		_, err := core.CreateInstanceRole(ctx, "instance-tester", "Tester", "QA tester")
		if err != nil {
			t.Fatalf("Failed to create role: %v", err)
		}

		// Assign the custom role
		if err := core.AssignInstanceRole(ctx, SystemActorID, userID, "instance-tester"); err != nil {
			t.Fatalf("Failed to assign role: %v", err)
		}

		// Verify via GetUserInstanceRoles
		roles, _ := core.GetUserInstanceRoles(ctx, userID)
		found := false
		for _, r := range roles {
			if r == "instance-tester" {
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
		core.CreateInstanceRole(ctx, "instance-editor", "Editor", "Content editor")

		// Assign multiple roles
		core.AssignInstanceRole(ctx, SystemActorID, user.Id, RoleAdmin)
		core.AssignInstanceRole(ctx, SystemActorID, user.Id, "instance-editor")

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
		if !roleSet[RoleAdmin] || !roleSet["instance-editor"] {
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
		_, err := core.CreateInstanceRole(ctx, "instance-alpha", "Alpha", "First custom role")
		if err != nil {
			t.Fatalf("Failed to create alpha role: %v", err)
		}
		_, err = core.CreateInstanceRole(ctx, "instance-beta", "Beta", "Second custom role")
		if err != nil {
			t.Fatalf("Failed to create beta role: %v", err)
		}

		// Get initial positions
		initialRoles, _ := core.ListInstanceRoles(ctx)
		var alphaInitialPos, betaInitialPos int32
		for _, r := range initialRoles {
			if r.Name == "instance-alpha" {
				alphaInitialPos = r.Position
			}
			if r.Name == "instance-beta" {
				betaInitialPos = r.Position
			}
		}
		t.Logf("Initial positions: alpha=%d, beta=%d", alphaInitialPos, betaInitialPos)

		// Reorder: put beta before alpha
		reordered, err := core.ReorderInstanceRoles(ctx, []string{"instance-beta", "instance-alpha"})
		if err != nil {
			t.Fatalf("Failed to reorder: %v", err)
		}

		// Find the new positions from the returned list
		var alphaNowPos, betaNowPos int32
		for _, r := range reordered {
			if r.Name == "instance-alpha" {
				alphaNowPos = r.Position
			}
			if r.Name == "instance-beta" {
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
		role, err := core.CreateInstanceRole(ctx, "instance-reviewer", "Reviewer", "Code reviewer")
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
		role, err := core.CreateInstanceRole(ctx, "instance-editor", "Editor", "Content editor")
		if err != nil {
			t.Fatalf("Failed to create role: %v", err)
		}
		originalPos := role.Position

		// Update the display name
		updated, err := core.UpdateInstanceRole(ctx, "instance-editor", "Super Editor", "Super content editor")
		if err != nil {
			t.Fatalf("Failed to update role: %v", err)
		}

		if updated.Position != originalPos {
			t.Errorf("Position changed after update: %d -> %d", originalPos, updated.Position)
		}
	})

	t.Run("multiple custom roles can be created", func(t *testing.T) {
		roles := []string{"instance-helper", "instance-triage", "instance-support"}
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

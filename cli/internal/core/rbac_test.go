package core

import (
	"context"
	"errors"
	"strings"
	"sync"
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
		{"admin valid", PermAdminUsersView, false},
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

	// Server defaults provide ordinary member capabilities globally.
	expected := []Permission{
		PermUserDeleteSelf,
		PermRoomList,
		PermRoomJoin,
		PermMessagePost,
		PermMessagePostInThread,
		PermMessageReact,
		PermMessageEcho,
	}
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

	// Check that everyone has message.post at server scope by default.
	hasPerm, err = core.HasServerPermission(ctx, "any-user", PermMessagePost)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if !hasPerm {
		t.Error("Expected everyone to have server-scope message.post permission")
	}

	// Check that everyone does NOT have admin view permission
	hasPerm, err = core.HasServerPermission(ctx, "any-user", PermAdminUsersView)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if hasPerm {
		t.Error("Expected member to NOT have admin view permission")
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
		SecretKey: "test-core-secret",
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

	// Verify default permission is granted.
	hasPerm, err := core1.HasServerPermission(ctx, user.Id, PermUserDeleteSelf)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if !hasPerm {
		t.Error("Expected user to have user.delete-self permission by default")
	}

	// Step 2: Admin revokes the permission from the everyone role
	err = core1.DenyServerPermission(ctx, SystemActorID, RoleEveryone, PermUserDeleteSelf)
	if err != nil {
		t.Fatalf("Failed to deny permission: %v", err)
	}

	// Verify permission is now denied
	hasPerm, err = core1.HasServerPermission(ctx, user.Id, PermUserDeleteSelf)
	if err != nil {
		t.Fatalf("Failed to check permission after denial: %v", err)
	}
	if hasPerm {
		t.Error("Expected user to NOT have user.delete-self permission after denial")
	}

	// Step 3: Simulate a restart by creating a new ChattoCore with the same NATS connection
	// This should NOT reset the permissions to defaults
	core2, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatalf("Failed to create second ChattoCore: %v", err)
	}
	startCoreServices(t, core2)

	// Step 4: Verify the permission change was preserved
	hasPerm, err = core2.HasServerPermission(ctx, user.Id, PermUserDeleteSelf)
	if err != nil {
		t.Fatalf("Failed to check permission after 'restart': %v", err)
	}
	if hasPerm {
		t.Error("Expected user to still NOT have user.delete-self permission after restart - permission was incorrectly reset to default")
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

	// Admin should have server-admin permissions, but message permissions are
	// room-tier defaults unless explicitly configured at server scope.
	for _, perm := range []Permission{PermAdminUsersView} {
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

	// Everyone should have user.delete-self by default.
	hasPerm, err := core.HasServerPermission(ctx, userID, PermUserDeleteSelf)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if !hasPerm {
		t.Error("Expected member to have user.delete-self permission")
	}

	// Everyone should have server-scope message.post by default.
	hasPerm, err = core.HasServerPermission(ctx, userID, PermMessagePost)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if !hasPerm {
		t.Error("Expected member to have server-scope message.post permission")
	}

	// Member should NOT have admin view permission
	hasPerm, err = core.HasServerPermission(ctx, userID, PermAdminUsersView)
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if hasPerm {
		t.Error("Expected member to NOT have admin view permission")
	}
}

// ============================================================================
// Can* Helper Tests
// ============================================================================

func TestChattoCore_HasAnyAdminPermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Regular user cannot view admin
	can, err := core.HasAnyAdminPermission(ctx, "regular-user")
	if err != nil {
		t.Fatalf("Failed to check HasAnyAdminPermission: %v", err)
	}
	if can {
		t.Error("Expected HasAnyAdminPermission to return false for regular users")
	}

	// Admin can view admin
	if err := core.AssignAdminRole(ctx, "admin-user"); err != nil {
		t.Fatalf("Failed to assign admin role: %v", err)
	}
	can, err = core.HasAnyAdminPermission(ctx, "admin-user")
	if err != nil {
		t.Fatalf("Failed to check HasAnyAdminPermission: %v", err)
	}
	if !can {
		t.Error("Expected HasAnyAdminPermission to return true for admin users")
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

		// user.delete-self is in default member permissions.
		hasPerm, err := core.HasUserPermissionViaRoles(ctx, userID, PermUserDeleteSelf)
		if err != nil {
			t.Fatalf("Failed to check: %v", err)
		}
		if !hasPerm {
			t.Error("Expected true for user.delete-self (member permission)")
		}
	})

	t.Run("returns false for non-member permissions", func(t *testing.T) {
		userID := "non-member-perm-check"

		// admin.view-users is NOT in default member permissions
		hasPerm, err := core.HasUserPermissionViaRoles(ctx, userID, PermAdminUsersView)
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
		hasPerm, err := core.HasUserPermissionViaRoles(ctx, userID, PermAdminUsersView)
		if err != nil {
			t.Fatalf("Failed to check: %v", err)
		}
		if !hasPerm {
			t.Error("Expected true for admin user")
		}
	})

	t.Run("returns false for users without the role", func(t *testing.T) {
		userID := "no-admin-role-check"

		// HasUserPermissionViaRoles should return false for admin view permission
		hasPerm, err := core.HasUserPermissionViaRoles(ctx, userID, PermAdminUsersView)
		if err != nil {
			t.Fatalf("Failed to check: %v", err)
		}
		if hasPerm {
			t.Error("Expected false - user doesn't have admin role")
		}
	})
}

// ============================================================================
// Deny-Wins Tests
// ============================================================================

func TestChattoCore_DenyWins_EveryoneDenyBeatsAdminGrant(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	userID := "denywins-admin"
	if err := core.AssignAdminRole(ctx, userID); err != nil {
		t.Fatalf("Failed to assign admin role: %v", err)
	}

	if err := core.DenyServerPermission(ctx, SystemActorID, RoleEveryone, PermAdminUsersView); err != nil {
		t.Fatalf("Failed to deny permission: %v", err)
	}

	t.Run("HasServerPermission denies despite admin grant", func(t *testing.T) {
		has, err := core.HasServerPermission(ctx, userID, PermAdminUsersView)
		if err != nil {
			t.Fatalf("HasServerPermission error: %v", err)
		}
		if has {
			t.Error("Expected HasServerPermission to return false: everyone deny should beat admin grant")
		}
	})

	t.Run("HasUserPermissionViaRoles matches authorizer", func(t *testing.T) {
		has, err := core.HasUserPermissionViaRoles(ctx, userID, PermAdminUsersView)
		if err != nil {
			t.Fatalf("HasUserPermissionViaRoles error: %v", err)
		}
		if has {
			t.Error("Expected HasUserPermissionViaRoles to return false: everyone deny should beat admin grant")
		}
	})

	t.Run("HasUserPermissionDeniedViaRoles matches authorizer", func(t *testing.T) {
		denied, err := core.HasUserPermissionDeniedViaRoles(ctx, userID, PermAdminUsersView)
		if err != nil {
			t.Fatalf("HasUserPermissionDeniedViaRoles error: %v", err)
		}
		if !denied {
			t.Error("Expected HasUserPermissionDeniedViaRoles to return true: everyone deny should beat admin grant")
		}
	})

	t.Run("GetUserServerPermissions excludes the permission", func(t *testing.T) {
		perms, err := core.GetUserServerPermissions(ctx, userID)
		if err != nil {
			t.Fatalf("GetUserServerPermissions error: %v", err)
		}
		found := false
		for _, p := range perms {
			if p == PermAdminUsersView {
				found = true
				break
			}
		}
		if found {
			t.Error("Expected GetUserServerPermissions to exclude admin.view-users: everyone deny should beat admin grant")
		}
	})
}

func TestChattoCore_DenyWins_EveryoneDenyBlocksMember(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Regular user with no special roles — only has "everyone"
	userID := "denywins-regular"

	// Deny space.create on the everyone role
	if err := core.DenyServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost); err != nil {
		t.Fatalf("Failed to deny permission: %v", err)
	}

	t.Run("HasUserPermissionViaRoles returns false", func(t *testing.T) {
		has, err := core.HasUserPermissionViaRoles(ctx, userID, PermMessagePost)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if has {
			t.Error("Expected false: everyone deny should block member")
		}
	})

	t.Run("HasUserPermissionDeniedViaRoles returns true", func(t *testing.T) {
		denied, err := core.HasUserPermissionDeniedViaRoles(ctx, userID, PermMessagePost)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if !denied {
			t.Error("Expected true: everyone deny should block member")
		}
	})

	t.Run("GetUserServerPermissions excludes the permission", func(t *testing.T) {
		perms, err := core.GetUserServerPermissions(ctx, userID)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		for _, p := range perms {
			if p == PermMessagePost {
				t.Error("Expected message.post NOT to be in permissions: everyone deny should block member")
			}
		}
	})
}

func TestChattoCore_OwnerOverride_BeatsEverythingElse(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create owner user
	owner, err := core.CreateUser(ctx, SystemActorID, "owner-override-owner", "Owner", "password123")
	if err != nil {
		t.Fatalf("Failed to create owner: %v", err)
	}
	if err := core.AssignOwnerRole(ctx, owner.Id); err != nil {
		t.Fatalf("Failed to assign owner role: %v", err)
	}

	// Deny admin.view-users on both admin and everyone roles
	if err := core.DenyServerPermission(ctx, SystemActorID, RoleEveryone, PermAdminUsersView); err != nil {
		t.Fatalf("Failed to deny everyone: %v", err)
	}
	if err := core.DenyServerPermission(ctx, SystemActorID, RoleAdmin, PermAdminUsersView); err != nil {
		t.Fatalf("Failed to deny admin: %v", err)
	}
	// Owner role still has admin.view-users granted

	t.Run("owner grant beats admin and everyone deny", func(t *testing.T) {
		has, err := core.HasUserPermissionViaRoles(ctx, owner.Id, PermAdminUsersView)
		if err != nil {
			t.Fatalf("error: %v", err)
		}
		if !has {
			t.Error("Expected true: owner grant should beat admin+everyone deny")
		}
	})

	t.Run("permission is not denied for owner", func(t *testing.T) {
		denied, err := core.HasUserPermissionDeniedViaRoles(ctx, owner.Id, PermAdminUsersView)
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
		role, err := core.CreateServerRole(ctx, SystemActorID, "customrole", "Custom Role", "A custom role")
		if err != nil {
			t.Fatalf("Failed to create role: %v", err)
		}
		if role.Name != "customrole" {
			t.Errorf("Expected role name 'customrole', got '%s'", role.Name)
		}
	})

	t.Run("accepts role name with dashes", func(t *testing.T) {
		role, err := core.CreateServerRole(ctx, SystemActorID, "custom-role", "Custom", "Dashes allowed")
		if err != nil {
			t.Fatalf("CreateServerRole(custom-role): %v", err)
		}
		if role.Name != "custom-role" {
			t.Errorf("Expected role name 'custom-role', got %q", role.Name)
		}
	})

	t.Run("accepts role name with numbers", func(t *testing.T) {
		role, err := core.CreateServerRole(ctx, SystemActorID, "tier2", "Tier 2", "Numbers allowed")
		if err != nil {
			t.Fatalf("CreateServerRole(tier2): %v", err)
		}
		if role.Name != "tier2" {
			t.Errorf("Expected role name 'tier2', got %q", role.Name)
		}
	})

	t.Run("rejects role name with leading dash", func(t *testing.T) {
		_, err := core.CreateServerRole(ctx, SystemActorID, "-custom", "Custom", "Should fail")
		if !errors.Is(err, ErrInvalidRoleName) {
			t.Errorf("Expected ErrInvalidRoleName, got %v", err)
		}
	})

	t.Run("rejects system role names", func(t *testing.T) {
		_, err := core.CreateServerRole(ctx, SystemActorID, RoleAdmin, "Admin", "Should fail")
		if err == nil {
			t.Error("Expected error for system role name")
		}
	})

	t.Run("rejects virtual mention handles", func(t *testing.T) {
		for _, name := range []string{"all", "here"} {
			_, err := core.CreateServerRole(ctx, SystemActorID, name, name, "Should fail")
			if !errors.Is(err, ErrRoleAlreadyExists) {
				t.Errorf("CreateServerRole(%q) error = %v, want ErrRoleAlreadyExists", name, err)
			}
		}
	})

	t.Run("rejects existing user logins", func(t *testing.T) {
		if _, err := core.CreateUser(ctx, SystemActorID, "role-collision-user", "Role Collision", "password123"); err != nil {
			t.Fatalf("CreateUser role-collision-user: %v", err)
		}
		_, err := core.CreateServerRole(ctx, SystemActorID, "role-collision-user", "Role Collision", "Should fail")
		if !errors.Is(err, ErrRoleAlreadyExists) {
			t.Errorf("CreateServerRole existing login error = %v, want ErrRoleAlreadyExists", err)
		}
	})
}

func TestChattoCore_MentionablesModelSerializesUserAndRoleCreation(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	const handle = "raceclaim"
	var wg sync.WaitGroup
	wg.Add(2)

	var userErr error
	var roleErr error
	go func() {
		defer wg.Done()
		_, userErr = core.CreateUser(ctx, SystemActorID, handle, "Race Claim", "password123")
	}()
	go func() {
		defer wg.Done()
		_, roleErr = core.CreateServerRole(ctx, SystemActorID, handle, "Race Claim", "")
	}()
	wg.Wait()

	if (userErr == nil) == (roleErr == nil) {
		t.Fatalf("CreateUser err = %v, CreateServerRole err = %v; want exactly one success", userErr, roleErr)
	}

	if userErr == nil {
		if !errors.Is(roleErr, ErrRoleAlreadyExists) {
			t.Fatalf("role err = %v, want ErrRoleAlreadyExists", roleErr)
		}
		if _, err := core.CreateServerRole(ctx, SystemActorID, handle, "Race Claim", ""); !errors.Is(err, ErrRoleAlreadyExists) {
			t.Fatalf("CreateServerRole after user claim err = %v, want ErrRoleAlreadyExists", err)
		}
		return
	}

	if !errors.Is(userErr, ErrUsernameBlocked) && !errors.Is(userErr, ErrLoginAlreadyTaken) {
		t.Fatalf("user err = %v, want ErrUsernameBlocked or ErrLoginAlreadyTaken", userErr)
	}
	if _, err := core.CreateUser(ctx, SystemActorID, handle, "Race Claim", "password123"); !errors.Is(err, ErrUsernameBlocked) {
		t.Fatalf("CreateUser after role claim err = %v, want ErrUsernameBlocked", err)
	}
}

func TestChattoCore_DeleteServerRoleReleasesMentionHandle(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	if _, err := core.CreateServerRole(ctx, SystemActorID, "released-role", "Released", ""); err != nil {
		t.Fatalf("CreateServerRole: %v", err)
	}
	if err := core.DeleteServerRole(ctx, SystemActorID, "released-role"); err != nil {
		t.Fatalf("DeleteServerRole: %v", err)
	}
	if _, err := core.CreateUser(ctx, SystemActorID, "released-role", "Released", "password123"); err != nil {
		t.Fatalf("CreateUser after role deletion: %v", err)
	}
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
		_, err := core.CreateServerRole(ctx, SystemActorID, "tester", "Tester", "QA tester")
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
		core.CreateServerRole(ctx, SystemActorID, "editor", "Editor", "Content editor")

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
// Instance Role Assignment Tests
// ============================================================================

func TestChattoCore_AssignServerRole_PermissionOnly(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create an owner
	owner, err := core.CreateUser(ctx, SystemActorID, "assign-owner", "Owner", "password123")
	if err != nil {
		t.Fatalf("Failed to create owner: %v", err)
	}
	if err := core.AssignOwnerRole(ctx, owner.Id); err != nil {
		t.Fatalf("Failed to assign owner role: %v", err)
	}

	// Create an admin user
	admin, err := core.CreateUser(ctx, SystemActorID, "assign-admin", "Admin", "password123")
	if err != nil {
		t.Fatalf("Failed to create admin: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, admin.Id, RoleAdmin); err != nil {
		t.Fatalf("Failed to assign admin role: %v", err)
	}

	// Create a target user
	target, err := core.CreateUser(ctx, SystemActorID, "assign-target", "Target", "password123")
	if err != nil {
		t.Fatalf("Failed to create target: %v", err)
	}

	t.Run("admin can assign owner role when API gate permits the call", func(t *testing.T) {
		err := core.AssignServerRole(ctx, admin.Id, target.Id, RoleOwner)
		if err != nil {
			t.Fatalf("AssignServerRole: %v", err)
		}
	})

	t.Run("admin can assign admin role when API gate permits the call", func(t *testing.T) {
		err := core.AssignServerRole(ctx, admin.Id, target.Id, RoleAdmin)
		if err != nil {
			t.Fatalf("AssignServerRole: %v", err)
		}
	})

	t.Run("admin can assign moderator role", func(t *testing.T) {
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

	t.Run("admin can self-assign owner when API gate permits the call", func(t *testing.T) {
		err := core.AssignServerRole(ctx, admin.Id, admin.Id, RoleOwner)
		if err != nil {
			t.Fatalf("AssignServerRole: %v", err)
		}
	})

	t.Run("system actor can assign owner role", func(t *testing.T) {
		err := core.AssignServerRole(ctx, SystemActorID, target.Id, RoleOwner)
		if err != nil {
			t.Fatalf("Expected system actor to assign owner role: %v", err)
		}
	})

	t.Run("admin can assign moderator role to peer admin when API gate permits the call", func(t *testing.T) {
		peerAdmin, err := core.CreateUser(ctx, SystemActorID, "assign-peer-admin", "Peer", "password123")
		if err != nil {
			t.Fatalf("Failed to create peer admin: %v", err)
		}
		if err := core.AssignServerRole(ctx, SystemActorID, peerAdmin.Id, RoleAdmin); err != nil {
			t.Fatalf("Failed to seed peer admin: %v", err)
		}

		err = core.AssignServerRole(ctx, admin.Id, peerAdmin.Id, RoleModerator)
		if err != nil {
			t.Fatalf("AssignServerRole: %v", err)
		}
	})
}

func TestChattoCore_RevokeServerRole_PermissionOnly(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create an owner
	owner, err := core.CreateUser(ctx, SystemActorID, "revoke-owner", "Owner", "password123")
	if err != nil {
		t.Fatalf("Failed to create owner: %v", err)
	}
	if err := core.AssignOwnerRole(ctx, owner.Id); err != nil {
		t.Fatalf("Failed to assign owner role: %v", err)
	}

	// Create an admin user
	admin, err := core.CreateUser(ctx, SystemActorID, "revoke-admin", "Admin", "password123")
	if err != nil {
		t.Fatalf("Failed to create admin: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, admin.Id, RoleAdmin); err != nil {
		t.Fatalf("Failed to assign admin role: %v", err)
	}

	// Create another admin to try revoking
	otherAdmin, err := core.CreateUser(ctx, SystemActorID, "revoke-admin2", "Admin2", "password123")
	if err != nil {
		t.Fatalf("Failed to create other admin: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, otherAdmin.Id, RoleAdmin); err != nil {
		t.Fatalf("Failed to assign admin role: %v", err)
	}

	// Assign moderator to a target user
	target, err := core.CreateUser(ctx, SystemActorID, "revoke-target", "Target", "password123")
	if err != nil {
		t.Fatalf("Failed to create target: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, target.Id, RoleModerator); err != nil {
		t.Fatalf("Failed to assign moderator: %v", err)
	}

	t.Run("admin can revoke owner role when API gate permits the call", func(t *testing.T) {
		err := core.RevokeServerRole(ctx, admin.Id, owner.Id, RoleOwner)
		if err != nil {
			t.Fatalf("RevokeServerRole: %v", err)
		}
	})

	t.Run("admin can revoke another admin's role when API gate permits the call", func(t *testing.T) {
		err := core.RevokeServerRole(ctx, admin.Id, otherAdmin.Id, RoleAdmin)
		if err != nil {
			t.Fatalf("RevokeServerRole: %v", err)
		}
	})

	t.Run("admin can revoke moderator role", func(t *testing.T) {
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

	t.Run("system actor can revoke admin role", func(t *testing.T) {
		// Re-assign admin to test system revoke.
		if err := core.AssignServerRole(ctx, SystemActorID, admin.Id, RoleAdmin); err != nil {
			t.Fatalf("Failed to re-assign admin: %v", err)
		}
		err := core.RevokeServerRole(ctx, SystemActorID, admin.Id, RoleAdmin)
		if err != nil {
			t.Fatalf("Expected system actor to revoke admin role: %v", err)
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
		_, err := core.CreateServerRole(ctx, SystemActorID, "alpha", "Alpha", "First custom role")
		if err != nil {
			t.Fatalf("Failed to create alpha role: %v", err)
		}
		_, err = core.CreateServerRole(ctx, SystemActorID, "beta", "Beta", "Second custom role")
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
		reordered, err := core.ReorderServerRoles(ctx, SystemActorID, []string{"beta", "alpha"})
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

		// Reorder semantics: the orderedNames argument goes from lower to
		// higher display position, so beta should end up below alpha.
		if betaNowPos >= alphaNowPos {
			t.Errorf("After reorder, beta (position %d) should sort below alpha (position %d)", betaNowPos, alphaNowPos)
		}
	})

	t.Run("rejects system role reordering", func(t *testing.T) {
		_, err := core.ReorderServerRoles(ctx, SystemActorID, []string{RoleAdmin, RoleModerator})
		if err == nil {
			t.Error("Expected error when trying to reorder system roles")
		}
	})

	t.Run("rejects incomplete custom role list", func(t *testing.T) {
		_, err := core.ReorderServerRoles(ctx, SystemActorID, []string{"alpha"})
		if err == nil {
			t.Error("Expected error when reorder omits a custom role")
		}
	})

	t.Run("rejects duplicate custom roles", func(t *testing.T) {
		_, err := core.ReorderServerRoles(ctx, SystemActorID, []string{"alpha", "alpha"})
		if err == nil {
			t.Error("Expected error when reorder includes a duplicate role")
		}
	})

	t.Run("rejects unknown custom role", func(t *testing.T) {
		_, err := core.ReorderServerRoles(ctx, SystemActorID, []string{"alpha", "gamma"})
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
		role, err := core.CreateServerRole(ctx, SystemActorID, "reviewer", "Reviewer", "Code reviewer")
		if err != nil {
			t.Fatalf("Failed to create role: %v", err)
		}

		// Custom roles slot in between everyone (0) and moderator (100).
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
		role, err := core.CreateServerRole(ctx, SystemActorID, "editor", "Editor", "Content editor")
		if err != nil {
			t.Fatalf("Failed to create role: %v", err)
		}
		originalPos := role.Position

		// Update the display name
		updated, err := core.UpdateServerRole(ctx, SystemActorID, "editor", "Super Editor", "Super content editor")
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
			_, err := core.CreateServerRole(ctx, SystemActorID, name, name, "Test role")
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
	role, err := core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Can moderate content")
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
	_, err := core.CreateServerRole(ctx, SystemActorID, "Invalid-Name", "Invalid", "Should fail")
	if err == nil {
		t.Error("Expected error for invalid role name")
	}
	if !errors.Is(err, ErrInvalidRoleName) {
		t.Errorf("Expected ErrInvalidRoleName, got %v", err)
	}
}

func TestChattoCore_RoleMetadataLengthLimits(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("create accepts values at max length", func(t *testing.T) {
		role, err := core.CreateServerRole(
			ctx,
			SystemActorID, "maxrole",
			strings.Repeat("d", MaxRoleDisplayNameLength),
			strings.Repeat("x", MaxRoleDescriptionLength),
		)
		if err != nil {
			t.Fatalf("CreateServerRole at max lengths: %v", err)
		}
		if len(role.DisplayName) != MaxRoleDisplayNameLength || len(role.Description) != MaxRoleDescriptionLength {
			t.Fatalf("role lengths = displayName:%d description:%d", len(role.DisplayName), len(role.Description))
		}
	})

	t.Run("create rejects over-limit display name", func(t *testing.T) {
		_, err := core.CreateServerRole(ctx, SystemActorID, "longdisplay", strings.Repeat("d", MaxRoleDisplayNameLength+1), "")
		assertStringLengthError(t, err, "role display name", MaxRoleDisplayNameLength)
	})

	t.Run("create rejects over-limit description", func(t *testing.T) {
		_, err := core.CreateServerRole(ctx, SystemActorID, "longdescription", "Role", strings.Repeat("d", MaxRoleDescriptionLength+1))
		assertStringLengthError(t, err, "role description", MaxRoleDescriptionLength)
	})

	t.Run("update rejects over-limit metadata", func(t *testing.T) {
		if _, err := core.CreateServerRole(ctx, SystemActorID, "editable", "Editable", ""); err != nil {
			t.Fatalf("CreateServerRole: %v", err)
		}
		_, err := core.UpdateServerRole(ctx, SystemActorID, "editable", strings.Repeat("d", MaxRoleDisplayNameLength+1), "")
		assertStringLengthError(t, err, "role display name", MaxRoleDisplayNameLength)
	})
}

func TestChattoCore_CreateRole_Duplicate(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a role
	_, err := core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "First")
	if err != nil {
		t.Fatalf("Failed to create first role: %v", err)
	}

	// Try to create same role again
	_, err = core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod 2", "Second")
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
	_, err := core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Can moderate content")
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
	core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Test mod role")
	core.CreateServerRole(ctx, SystemActorID, "vip", "VIP", "VIP role")

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
	_, err := core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Can moderate content")
	if err != nil {
		t.Fatalf("Failed to create role: %v", err)
	}

	// Update it
	updated, err := core.UpdateServerRole(ctx, SystemActorID, "testmod", "Super Moderator", "Enhanced moderation")
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
	_, err := core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Can moderate content")
	if err != nil {
		t.Fatalf("Failed to create role: %v", err)
	}

	// Delete it
	err = core.DeleteServerRole(ctx, SystemActorID, "testmod")
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
	err = core.DeleteServerRole(ctx, SystemActorID, RoleOwner)
	if err == nil {
		t.Error("Expected error when deleting system role")
	}

	if !errors.Is(err, ErrCannotDeleteSystemRole) {
		t.Errorf("Expected ErrCannotDeleteSystemRole, got %v", err)
	}

	// Also test everyone role
	err = core.DeleteServerRole(ctx, SystemActorID, RoleEveryone)
	if !errors.Is(err, ErrCannotDeleteSystemRole) {
		t.Errorf("Expected ErrCannotDeleteSystemRole for everyone role, got %v", err)
	}
}

func TestChattoCore_DeleteRole_CleansUpPermissionsAndAssignments(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a role, grant permissions, and assign to a user
	_, err := core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Can moderate")
	if err != nil {
		t.Fatalf("Failed to create role: %v", err)
	}

	// Grant permissions
	core.GrantServerPermission(ctx, SystemActorID, "testmod", PermRoleAssign)
	core.GrantServerPermission(ctx, SystemActorID, "testmod", PermRoomManage)

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
	err = core.DeleteServerRole(ctx, SystemActorID, "testmod")
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

	core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Can moderate")

	// Grant a permission
	err := core.GrantServerPermission(ctx, SystemActorID, "testmod", PermRoleAssign)
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

	core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Can moderate")

	// Grant same permission twice
	core.GrantServerPermission(ctx, SystemActorID, "testmod", PermRoleAssign)
	err := core.GrantServerPermission(ctx, SystemActorID, "testmod", PermRoleAssign)
	if err != nil {
		t.Fatalf("Second grant should not fail: %v", err)
	}

	// Should still have only one permission
	perms, _ := core.GetServerRolePermissions(ctx, "testmod")
	if len(perms) != 1 {
		t.Errorf("Expected 1 permission after duplicate grant, got %d", len(perms))
	}
}

// GrantServerPermission does not validate that the role exists. Role
// existence is enforced when CRUD operations against the role itself are made;
// standalone permission decisions can be appended before the subject exists.

func TestChattoCore_GrantRolePermission_InvalidPermission(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Can moderate")

	// Try to grant an invalid permission
	err := core.GrantServerPermission(ctx, SystemActorID, "testmod", Permission("invalid_perm"))
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

	core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Can moderate")

	// Grant then revoke
	core.GrantServerPermission(ctx, SystemActorID, "testmod", PermRoleAssign)
	err := core.RevokeServerPermission(ctx, SystemActorID, "testmod", PermRoleAssign)
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

	core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Can moderate")

	// Revoke permission that was never granted
	err := core.RevokeServerPermission(ctx, SystemActorID, "testmod", PermRoleAssign)
	if err != nil {
		t.Fatalf("Revoking non-existent permission should not fail: %v", err)
	}
}

func TestChattoCore_GetRolePermissions_Multiple(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Can moderate")

	// Grant multiple permissions
	core.GrantServerPermission(ctx, SystemActorID, "testmod", PermRoomJoin)
	core.GrantServerPermission(ctx, SystemActorID, "testmod", PermRoleAssign)
	core.GrantServerPermission(ctx, SystemActorID, "testmod", PermRoomManage)

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

	core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Can moderate")

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

	core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Can moderate")

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

	core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Can moderate")

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

	core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Can moderate")

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

	core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Can moderate")

	// Revoke role that was never assigned
	err := core.RevokeServerRole(ctx, SystemActorID, "user123", "testmod")
	if err != nil {
		t.Fatalf("Revoking non-assigned role should not fail: %v", err)
	}
}

func TestChattoCore_RevokeRole_CanDemotePeerWhenAPIGatePermits(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create two admins (must be space members for permission checks)
	adminA := "admin-a"
	adminB := "admin-b"
	core.AssignServerRole(ctx, SystemActorID, adminA, RoleOwner)
	core.AssignServerRole(ctx, SystemActorID, adminB, RoleOwner)

	err := core.RevokeServerRole(ctx, adminA, adminB, RoleOwner)
	if err != nil {
		t.Fatalf("RevokeServerRole: %v", err)
	}

	// Verify Admin B no longer has owner role.
	roles, _ := core.GetUserRoles(ctx, adminB)
	hasAdmin := false
	for _, r := range roles {
		if r == RoleOwner {
			hasAdmin = true
			break
		}
	}
	if hasAdmin {
		t.Error("Admin B should no longer have owner role")
	}
}

func TestChattoCore_RevokeRole_AdminCanDemoteLowerRankedUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create admin and moderator (must be space members for permission checks)
	admin := "admin-user"
	mod := "mod-user"
	core.AssignServerRole(ctx, SystemActorID, admin, RoleOwner)
	// moderator role is created by default with position 100

	// Assign moderator role to mod user
	core.AssignServerRole(ctx, SystemActorID, mod, "moderator")

	// Admin should be able to revoke moderator role when authorized by caller context.
	err := core.RevokeServerRole(ctx, admin, mod, "moderator")
	if err != nil {
		t.Errorf("Admin should be able to revoke role, got: %v", err)
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

	core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Can moderate")
	core.CreateServerRole(ctx, SystemActorID, "vip", "VIP", "VIP user")

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

	core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Can moderate")

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

	core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Can moderate")

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

	core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Can moderate")
	core.GrantServerPermission(ctx, SystemActorID, "testmod", PermRoleAssign)
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
	core.CreateServerRole(ctx, SystemActorID, "testmod", "Test Mod", "Can moderate")
	core.GrantServerPermission(ctx, SystemActorID, "testmod", PermRoleAssign)

	core.CreateServerRole(ctx, SystemActorID, "admin", "Admin", "Full access")
	core.GrantServerPermission(ctx, SystemActorID, "admin", PermServerManage)

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
	// Sanity-check that the event-sourced defaults are present without pinning
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
	clearDefaultEveryoneRoomPermissions(t, core, ctx, room.Id)

	// Grant message.post at room level for member role
	err := core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePost)
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
	clearDefaultEveryoneRoomPermissions(t, core, ctx, room.Id)

	// Deny message.post at room level
	err := core.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePost)
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
	clearDefaultEveryoneRoomPermissions(t, core, ctx, room.Id)

	// Grant, then clear
	core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePost)
	err := core.ClearRoomPermissionState(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePost)
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
	err := core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermServerManage)
	if err == nil {
		t.Error("Expected error for non-room-scoped permission, got nil")
	}
}

func TestChattoCore_RoomPermissions_PerRoomIsolation(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room1, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "room-alpha", "Room Alpha")
	room2, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "room-beta", "Room Beta")
	clearDefaultEveryoneRoomPermissions(t, core, ctx, room1.Id)
	clearDefaultEveryoneRoomPermissions(t, core, ctx, room2.Id)

	// Deny message.post only in room1
	core.DenyRoomPermission(ctx, SystemActorID, room1.Id, RoleEveryone, PermMessagePost)

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

// Authorization for room-level permission mutations now lives in public
// operation models. The previous in-core gate that this test exercised has
// been retired; API-level coverage exercises the replacement path.

func clearDefaultEveryoneRoomPermissions(t *testing.T, core *ChattoCore, ctx context.Context, roomID string) {
	t.Helper()
	for _, perm := range DefaultRoomEveryonePermissions() {
		if err := core.ClearRoomPermissionState(ctx, SystemActorID, roomID, RoleEveryone, perm); err != nil {
			t.Fatalf("ClearRoomPermissionState(%s): %v", perm, err)
		}
	}
}

func TestChattoCore_GrantRoomRolePermission_GrantClearsDenial(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "general", "General")
	clearDefaultEveryoneRoomPermissions(t, core, ctx, room.Id)

	// Deny, then grant — should clear the denial
	core.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePost)
	core.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessagePost)

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

	// User should have default server member permissions (via everyone role).
	expectedPerms := []string{"user.delete-self"}
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
	err := core.GrantServerPermission(ctx, SystemActorID, RoleModerator, PermRoleManage)
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
	err := core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermRoomCreate)
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
	err = core.DenyServerPermission(ctx, SystemActorID, RoleEveryone, PermRoomCreate)
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

	// Verify moderator has admin.view-users by default.
	perms1, _ := core.GetUserEffectiveSpacePermissions(ctx, KindChannel, user.Id)
	permSet1 := make(map[string]bool)
	for _, p := range perms1 {
		permSet1[string(p)] = true
	}
	if !permSet1["admin.view-users"] {
		t.Error("User should have admin.view-users by default")
	}

	// Deny admin.view-users to moderator role.
	err := core.DenyServerPermission(ctx, SystemActorID, RoleModerator, PermAdminUsersView)
	if err != nil {
		t.Fatalf("Failed to deny role permission: %v", err)
	}

	// Now user should NOT have admin.view-users (role denial wins).
	perms2, err := core.GetUserEffectiveSpacePermissions(ctx, KindChannel, user.Id)
	if err != nil {
		t.Fatalf("GetUserEffectiveSpacePermissions failed: %v", err)
	}
	permSet2 := make(map[string]bool)
	for _, p := range perms2 {
		permSet2[string(p)] = true
	}
	if permSet2["admin.view-users"] {
		t.Error("User should NOT have admin.view-users after role denial")
	}
}

// ============================================================================
// Role Assignment Tests
// ============================================================================

func TestChattoCore_AssignRole_PermissionOnly(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup users
	owner := "owner-user"
	mod := "mod-user"
	regular := "regular-user"

	core.AssignServerRole(ctx, SystemActorID, owner, RoleOwner)
	core.AssignServerRole(ctx, SystemActorID, mod, RoleModerator)

	// Grant role assignment permission to moderator so the core API call can be
	// exercised as permission-only behavior.
	core.GrantServerPermission(ctx, SystemActorID, RoleModerator, PermRoleAssign)

	t.Run("owner can assign moderator role", func(t *testing.T) {
		// Owner (position 1000) can assign moderator
		err := core.AssignServerRole(ctx, owner, regular, RoleModerator)
		if err != nil {
			t.Errorf("Owner should be able to assign moderator role: %v", err)
		}
		// Cleanup
		core.RevokeServerRole(ctx, owner, regular, RoleModerator)
	})

	t.Run("moderator can assign owner role when caller gate permits the call", func(t *testing.T) {
		err := core.AssignServerRole(ctx, mod, regular, RoleOwner)
		if err != nil {
			t.Fatalf("AssignServerRole: %v", err)
		}
	})

	t.Run("regular member can assign custom role when caller gate permits the call", func(t *testing.T) {
		core.CreateServerRole(ctx, SystemActorID, "helper", "Helper", "Can help")

		err := core.AssignServerRole(ctx, regular, mod, "helper")
		if err != nil {
			t.Fatalf("AssignServerRole: %v", err)
		}
	})

	t.Run("moderator can assign custom role", func(t *testing.T) {
		// Create a custom role.
		role, _ := core.CreateServerRole(ctx, SystemActorID, "editor", "Editor", "Can edit")
		// Verify the custom role is in the custom position band.
		if role.Position >= PositionModerator {
			t.Skipf("Custom role position %d is not below moderator position %d", role.Position, PositionModerator)
		}

		err := core.AssignServerRole(ctx, mod, regular, "editor")
		if err != nil {
			t.Errorf("Moderator should be able to assign custom role: %v", err)
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

func TestChattoCore_RevokeRole_PeersCanRevokeWhenAPIGatePermits(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Setup two moderators
	modA := "mod-a"
	modB := "mod-b"
	core.AssignServerRole(ctx, SystemActorID, modA, RoleModerator)
	core.AssignServerRole(ctx, SystemActorID, modB, RoleModerator)

	// Grant role assignment permission to moderators so the core API calls can
	// be exercised as permission-only behavior.
	core.GrantServerPermission(ctx, SystemActorID, RoleModerator, PermRoleAssign)

	t.Run("moderator A can revoke moderator B's moderator role", func(t *testing.T) {
		err := core.RevokeServerRole(ctx, modA, modB, RoleModerator)
		if err != nil {
			t.Fatalf("RevokeServerRole: %v", err)
		}
	})

	t.Run("vice versa - B can demote A", func(t *testing.T) {
		err := core.RevokeServerRole(ctx, modB, modA, RoleModerator)
		if err != nil {
			t.Fatalf("RevokeServerRole: %v", err)
		}
	})

	// Verify both roles were revoked.
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

	if hasMod(rolesA) {
		t.Error("Moderator A should no longer have moderator role")
	}
	if hasMod(rolesB) {
		t.Error("Moderator B should no longer have moderator role")
	}
}

func TestChattoCore_AdminRoleManagementAuthorization(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	regular, err := core.CreateUser(ctx, SystemActorID, "role-regular", "Role Regular", "password")
	if err != nil {
		t.Fatalf("CreateUser regular: %v", err)
	}
	admin, err := core.CreateUser(ctx, SystemActorID, "role-admin", "Role Admin", "password")
	if err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, admin.Id, RoleAdmin); err != nil {
		t.Fatalf("AssignServerRole admin: %v", err)
	}

	pingable := true
	if _, err := core.AdminCreateServerRole(ctx, "", AdminRoleInput{Name: "helpdesk", DisplayName: "Helpdesk"}); !errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("unauth create err = %v, want ErrNotAuthenticated", err)
	}
	if _, err := core.AdminCreateServerRole(ctx, regular.Id, AdminRoleInput{Name: "helpdesk", DisplayName: "Helpdesk"}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("regular create err = %v, want ErrPermissionDenied", err)
	}
	role, err := core.AdminCreateServerRole(ctx, admin.Id, AdminRoleInput{
		Name:        "helpdesk",
		DisplayName: "Helpdesk",
		Description: "Support team",
		Pingable:    &pingable,
	})
	if err != nil {
		t.Fatalf("AdminCreateServerRole: %v", err)
	}
	if !role.Pingable {
		t.Fatal("Pingable = false, want true")
	}

	pingable = false
	updated, err := core.AdminUpdateServerRole(ctx, admin.Id, AdminRoleInput{
		Name:        "helpdesk",
		DisplayName: "Support",
		Description: "Support queue",
		Pingable:    &pingable,
	})
	if err != nil {
		t.Fatalf("AdminUpdateServerRole: %v", err)
	}
	if updated.DisplayName != "Support" || updated.Pingable {
		t.Fatalf("updated role = %+v, want display Support and pingable false", updated)
	}

	if err := core.AdminDeleteServerRole(ctx, regular.Id, "helpdesk"); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("regular delete err = %v, want ErrPermissionDenied", err)
	}
	if err := core.AdminDeleteServerRole(ctx, admin.Id, RoleOwner); !errors.Is(err, ErrCannotDeleteSystemRole) {
		t.Fatalf("delete owner err = %v, want ErrCannotDeleteSystemRole", err)
	}
	if err := core.AdminDeleteServerRole(ctx, admin.Id, "helpdesk"); err != nil {
		t.Fatalf("AdminDeleteServerRole: %v", err)
	}
}

func TestChattoCore_ServerRoleDetailsRostersRequireAssign(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	regular, err := core.CreateUser(ctx, SystemActorID, "roster-regular", "Roster Regular", "password")
	if err != nil {
		t.Fatalf("CreateUser regular: %v", err)
	}
	admin, err := core.CreateUser(ctx, SystemActorID, "roster-admin", "Roster Admin", "password")
	if err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	member, err := core.CreateUser(ctx, SystemActorID, "roster-member", "Roster Member", "password")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	if _, err := core.CreateServerRole(ctx, SystemActorID, "support", "Support", ""); err != nil {
		t.Fatalf("CreateServerRole support: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, member.Id, "support"); err != nil {
		t.Fatalf("AssignServerRole support: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, admin.Id, RoleAdmin); err != nil {
		t.Fatalf("AssignServerRole admin: %v", err)
	}

	regularDetails, err := core.GetServerRoleDetails(ctx, regular.Id, "support")
	if err != nil {
		t.Fatalf("GetServerRoleDetails regular: %v", err)
	}
	if regularDetails.ViewerCanAssignRoles || len(regularDetails.Users) != 0 {
		t.Fatalf("regular details canAssign=%v users=%d, want false/0", regularDetails.ViewerCanAssignRoles, len(regularDetails.Users))
	}

	adminDetails, err := core.GetServerRoleDetails(ctx, admin.Id, "support")
	if err != nil {
		t.Fatalf("GetServerRoleDetails admin: %v", err)
	}
	if !adminDetails.ViewerCanAssignRoles {
		t.Fatal("admin ViewerCanAssignRoles = false, want true")
	}
	if len(adminDetails.Users) != 1 || adminDetails.Users[0].ID != member.Id {
		t.Fatalf("admin role users = %+v, want member %s", adminDetails.Users, member.Id)
	}
}

func TestChattoCore_CreateRole_PositionAssignment(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("first custom role gets position after system roles", func(t *testing.T) {
		// System roles: everyone (0), moderator (100), admin (900), owner (1000)
		role, err := core.CreateServerRole(ctx, SystemActorID, "editor", "Editor", "Can edit")
		if err != nil {
			t.Fatalf("CreateRole failed: %v", err)
		}
		// Custom roles should slot between everyone and moderator by default.
		t.Logf("Custom role 'editor' assigned position %d", role.Position)
		if role.Position <= 0 {
			t.Errorf("Custom role position = %d, should be positive", role.Position)
		}
	})

	t.Run("position preserved after display name update", func(t *testing.T) {
		role, err := core.CreateServerRole(ctx, SystemActorID, "contributor", "Contributor", "Can contribute")
		if err != nil {
			t.Fatalf("CreateRole failed: %v", err)
		}
		originalPos := role.Position

		// Update display name
		updated, err := core.UpdateServerRole(ctx, SystemActorID, "contributor", "Super Contributor", "Can contribute a lot")
		if err != nil {
			t.Fatalf("UpdateRole failed: %v", err)
		}

		if updated.Position != originalPos {
			t.Errorf("Position changed from %d to %d after update", originalPos, updated.Position)
		}
	})

	t.Run("roles can be reordered via ReorderServerRoles", func(t *testing.T) {
		// Create multiple custom roles
		core.CreateServerRole(ctx, SystemActorID, "alpha", "Alpha", "Alpha role")
		core.CreateServerRole(ctx, SystemActorID, "beta", "Beta", "Beta role")

		// Reorder all custom roles. ReorderServerRoles requires a complete
		// custom-role list so clients cannot accidentally drop roles from the
		// authoritative ordering event.
		roles, err := core.ReorderServerRoles(ctx, SystemActorID, []string{"editor", "contributor", "beta", "alpha"})
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

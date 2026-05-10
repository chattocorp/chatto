package core

import (
	"testing"
)

// ============================================================================
// Instance Permission Can* Helper Tests
// ============================================================================

// TestInstanceCanHelpers verifies that the semantic Can* helper functions
// for instance-level permissions correctly wrap HasPermission.
func TestInstanceCanHelpers(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a regular user (not admin, not owner)
	regularUser, err := core.CreateUser(ctx, SystemActorID, "regular", "Regular User", "password123")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}

	// Create an admin user using AssignInstanceAdminRole
	adminUser, err := core.CreateUser(ctx, SystemActorID, "adminuser", "Admin User", "password123")
	if err != nil {
		t.Fatalf("failed to create admin user: %v", err)
	}
	if err := core.AssignInstanceAdminRole(ctx, adminUser.Id); err != nil {
		t.Fatalf("failed to assign admin role: %v", err)
	}

	// Test everyone permissions (available to all authenticated users)
	t.Run("regular user has everyone permissions", func(t *testing.T) {
		tests := []struct {
			name  string
			check func() (bool, error)
		}{
			{"CanDMView", func() (bool, error) { return core.CanDMView(ctx, regularUser.Id) }},
			{"CanDMWrite", func() (bool, error) { return core.CanDMWrite(ctx, regularUser.Id) }},
		}
		for _, tc := range tests {
			t.Run(tc.name, func(t *testing.T) {
				can, err := tc.check()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !can {
					t.Errorf("regular user should have %s permission", tc.name)
				}
			})
		}
	})

	t.Run("regular user does NOT have admin permissions", func(t *testing.T) {
		can, err := core.CanAdminAccess(ctx, regularUser.Id)
		if err != nil {
			t.Fatalf("CanAdminAccess error: %v", err)
		}
		if can {
			t.Error("regular user should NOT have CanAdminAccess permission")
		}

		can, err = core.CanAdminUsersView(ctx, regularUser.Id)
		if err != nil {
			t.Fatalf("CanAdminUsersView error: %v", err)
		}
		if can {
			t.Error("regular user should NOT have CanAdminUsersView permission")
		}
	})

	t.Run("admin user has admin permissions", func(t *testing.T) {
		adminTests := []struct {
			name  string
			check func() (bool, error)
		}{
			{"CanAdminAccess", func() (bool, error) { return core.CanAdminAccess(ctx, adminUser.Id) }},
			{"CanAdminUsersView", func() (bool, error) { return core.CanAdminUsersView(ctx, adminUser.Id) }},
			{"CanAdminUsersManage", func() (bool, error) { return core.CanAdminUsersManage(ctx, adminUser.Id) }},
			{"CanAdminRolesView", func() (bool, error) { return core.CanAdminRolesView(ctx, adminUser.Id) }},
			{"CanAdminRolesManage", func() (bool, error) { return core.CanAdminRolesManage(ctx, adminUser.Id) }},
			{"CanAdminSystemView", func() (bool, error) { return core.CanAdminSystemView(ctx, adminUser.Id) }},
		}

		for _, tc := range adminTests {
			t.Run(tc.name, func(t *testing.T) {
				can, err := tc.check()
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if !can {
					t.Errorf("admin user should have %s permission", tc.name)
				}
			})
		}
	})

}

// TestCanAdminManageUser verifies the role-hierarchy check used by admin
// user-management actions (identity edits, cooldown clearing).
func TestCanAdminManageUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	owner, err := core.CreateUser(ctx, SystemActorID, "owner", "Owner", "password123")
	if err != nil {
		t.Fatalf("failed to create owner: %v", err)
	}
	if err := core.AssignInstanceOwnerRole(ctx, owner.Id); err != nil {
		t.Fatalf("failed to assign owner role: %v", err)
	}

	admin1, err := core.CreateUser(ctx, SystemActorID, "admin1", "Admin One", "password123")
	if err != nil {
		t.Fatalf("failed to create admin1: %v", err)
	}
	if err := core.AssignInstanceAdminRole(ctx, admin1.Id); err != nil {
		t.Fatalf("failed to assign admin role to admin1: %v", err)
	}

	admin2, err := core.CreateUser(ctx, SystemActorID, "admin2", "Admin Two", "password123")
	if err != nil {
		t.Fatalf("failed to create admin2: %v", err)
	}
	if err := core.AssignInstanceAdminRole(ctx, admin2.Id); err != nil {
		t.Fatalf("failed to assign admin role to admin2: %v", err)
	}

	regular, err := core.CreateUser(ctx, SystemActorID, "regular", "Regular", "password123")
	if err != nil {
		t.Fatalf("failed to create regular: %v", err)
	}

	cases := []struct {
		name       string
		actor      string
		target     string
		wantCan    bool
	}{
		{"self (admin)", admin1.Id, admin1.Id, true},
		{"self (regular)", regular.Id, regular.Id, true},
		{"admin can manage regular", admin1.Id, regular.Id, true},
		{"regular cannot manage admin", regular.Id, admin1.Id, false},
		{"admin cannot manage owner", admin1.Id, owner.Id, false},
		{"owner can manage admin", owner.Id, admin1.Id, true},
		{"peer admins cannot manage each other", admin1.Id, admin2.Id, false},
		{"regular cannot manage regular peer", regular.Id, owner.Id, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			can, err := core.CanAdminManageUser(ctx, tc.actor, tc.target)
			if err != nil {
				t.Fatalf("CanAdminManageUser error: %v", err)
			}
			if can != tc.wantCan {
				t.Errorf("CanAdminManageUser(%s, %s) = %v, want %v",
					tc.actor, tc.target, can, tc.wantCan)
			}
		})
	}
}

// TestCanDeleteUser tests the special logic for user deletion permissions.
func TestCanDeleteUser(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create test users
	user1, err := core.CreateUser(ctx, SystemActorID, "user1", "User One", "password123")
	if err != nil {
		t.Fatalf("failed to create user1: %v", err)
	}

	user2, err := core.CreateUser(ctx, SystemActorID, "user2", "User Two", "password123")
	if err != nil {
		t.Fatalf("failed to create user2: %v", err)
	}

	adminUser, err := core.CreateUser(ctx, SystemActorID, "adminfordelete", "Admin User", "password123")
	if err != nil {
		t.Fatalf("failed to create admin: %v", err)
	}
	if err := core.AssignInstanceAdminRole(ctx, adminUser.Id); err != nil {
		t.Fatalf("failed to assign admin role: %v", err)
	}

	t.Run("user can delete their own account (self-deletion)", func(t *testing.T) {
		can, err := core.CanDeleteUser(ctx, user1.Id, user1.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("user should be able to delete their own account")
		}
	})

	t.Run("user cannot delete another user's account", func(t *testing.T) {
		can, err := core.CanDeleteUser(ctx, user1.Id, user2.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("regular user should NOT be able to delete another user's account")
		}
	})

	t.Run("admin can delete any user's account", func(t *testing.T) {
		can, err := core.CanDeleteUser(ctx, adminUser.Id, user1.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("admin should be able to delete any user's account")
		}

		can, err = core.CanDeleteUser(ctx, adminUser.Id, user2.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("admin should be able to delete any user's account")
		}
	})

	t.Run("admin can delete their own account", func(t *testing.T) {
		can, err := core.CanDeleteUser(ctx, adminUser.Id, adminUser.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("admin should be able to delete their own account")
		}
	})

	t.Run("self-deletion denied when user.delete.self permission is revoked", func(t *testing.T) {
		// Create a custom role that denies self-deletion
		if _, err := core.CreateInstanceRole(ctx, "instance-noselfdelete", "No Self Delete", ""); err != nil {
			t.Fatalf("failed to create role: %v", err)
		}
		if err := core.DenyInstancePermission(ctx, "instance-noselfdelete", PermUserDeleteSelf); err != nil {
			t.Fatalf("failed to deny permission: %v", err)
		}

		// Create a user and assign the deny role
		blockedUser, err := core.CreateUser(ctx, SystemActorID, "noselfdelete", "No Self Delete User", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}
		if err := core.AssignInstanceRole(ctx, SystemActorID, blockedUser.Id, "instance-noselfdelete"); err != nil {
			t.Fatalf("failed to assign role: %v", err)
		}

		can, err := core.CanDeleteUser(ctx, blockedUser.Id, blockedUser.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if can {
			t.Error("user with denied user.delete.self should NOT be able to self-delete")
		}
	})

	t.Run("admin can still delete others when self-delete is denied on everyone", func(t *testing.T) {
		// Even if self-delete is restricted via custom role, admin user.delete still works
		can, err := core.CanDeleteUser(ctx, adminUser.Id, user1.Id)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !can {
			t.Error("admin should still be able to delete other users via user.delete permission")
		}
	})
}

// TestPermissionsWithCustomRoles tests that custom instance roles
// with specific permissions work correctly with the Can* helpers.
func TestPermissionsWithCustomRoles(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a custom role with limited admin permissions
	customRole, err := core.CreateInstanceRole(ctx, "instance-viewer", "Viewer Admin", "Can only view admin pages")
	if err != nil {
		t.Fatalf("failed to create custom role: %v", err)
	}

	// Grant only view permissions using GrantPermission
	err = core.GrantInstancePermission(ctx, customRole.Name, PermAdminAccess)
	if err != nil {
		t.Fatalf("failed to grant admin permission: %v", err)
	}
	err = core.GrantInstancePermission(ctx, customRole.Name, PermAdminUsersView)
	if err != nil {
		t.Fatalf("failed to grant users view permission: %v", err)
	}

	// Create user with custom role
	customUser, err := core.CreateUser(ctx, SystemActorID, "customroleuser", "Custom Role User", "password123")
	if err != nil {
		t.Fatalf("failed to create user: %v", err)
	}
	if err := core.AssignInstanceRole(ctx, SystemActorID, customUser.Id, customRole.Name); err != nil {
		t.Fatalf("failed to assign role: %v", err)
	}

	t.Run("custom role user has granted permissions", func(t *testing.T) {
		can, err := core.CanAdminAccess(ctx, customUser.Id)
		if err != nil {
			t.Fatalf("CanAdminAccess error: %v", err)
		}
		if !can {
			t.Error("custom role user should have CanAdminAccess permission")
		}

		can, err = core.CanAdminUsersView(ctx, customUser.Id)
		if err != nil {
			t.Fatalf("CanAdminUsersView error: %v", err)
		}
		if !can {
			t.Error("custom role user should have CanAdminUsersView permission")
		}
	})

	t.Run("custom role user does NOT have ungranted permissions", func(t *testing.T) {
		can, err := core.CanAdminUsersManage(ctx, customUser.Id)
		if err != nil {
			t.Fatalf("CanAdminUsersManage error: %v", err)
		}
		if can {
			t.Error("custom role user should NOT have CanAdminUsersManage permission")
		}

		can, err = core.CanAdminRolesManage(ctx, customUser.Id)
		if err != nil {
			t.Fatalf("CanAdminRolesManage error: %v", err)
		}
		if can {
			t.Error("custom role user should NOT have CanAdminRolesManage permission")
		}
	})
}

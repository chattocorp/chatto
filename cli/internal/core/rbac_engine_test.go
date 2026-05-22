package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

)

// setupTestEngine creates an embedded NATS server, KV bucket, and RBAC engine for testing.
func setupTestEngine(t *testing.T, config Config) (*Engine, func()) {
	t.Helper()

	// Start embedded NATS server with JetStream
	opts := &server.Options{
		Port:      -1, // Random available port
		JetStream: true,
		StoreDir:  t.TempDir(),
	}
	ns, err := server.NewServer(opts)
	if err != nil {
		t.Fatalf("Failed to create NATS server: %v", err)
	}
	ns.Start()

	if !ns.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server not ready")
	}

	// Connect to the server
	nc, err := nats.Connect(ns.ClientURL())
	if err != nil {
		ns.Shutdown()
		t.Fatalf("Failed to connect to NATS: %v", err)
	}

	// Create JetStream context
	js, err := jetstream.New(nc)
	if err != nil {
		nc.Close()
		ns.Shutdown()
		t.Fatalf("Failed to create JetStream context: %v", err)
	}

	// Create KV bucket
	ctx := context.Background()
	kv, err := js.CreateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket: "TEST_RBAC",
	})
	if err != nil {
		nc.Close()
		ns.Shutdown()
		t.Fatalf("Failed to create KV bucket: %v", err)
	}

	engine := NewEngine(kv, config)

	cleanup := func() {
		nc.Close()
		ns.Shutdown()
	}

	return engine, cleanup
}

// Default test config without user overrides
func defaultTestConfig() Config {
	return Config{
		SystemRoles: []string{"admin", "everyone"},
		AdminRole:   "admin",
		ValidateVerbObjectType: func(verb, objectType string) error {
			// Valid verb+objectType combinations for tests
			validCombos := map[string]bool{
				"create.room":     true,
				"post.message":    true,
				"delete-any.message": true,
				"manage.space":    true,
				"create.space":    true,
				"access.admin":    true,
				"delete.room":     true,
			}
			key := verb + "." + objectType
			if !validCombos[key] {
				return ErrInvalidPermission
			}
			return nil
		},
	}
}

func TestEngine_CreateRole(t *testing.T) {
	engine, cleanup := setupTestEngine(t, defaultTestConfig())
	defer cleanup()

	ctx := context.Background()

	t.Run("creates role successfully", func(t *testing.T) {
		role, err := engine.CreateRole(ctx, "moderator", "Moderator", "Can moderate content")
		if err != nil {
			t.Fatalf("CreateRole() error = %v", err)
		}
		if role.Name != "moderator" {
			t.Errorf("Role.Name = %v, want moderator", role.Name)
		}
		if role.DisplayName != "Moderator" {
			t.Errorf("Role.DisplayName = %v, want Moderator", role.DisplayName)
		}
	})

	t.Run("returns ErrRoleAlreadyExists for duplicate", func(t *testing.T) {
		_, err := engine.CreateRole(ctx, "moderator", "Mod2", "Another mod")
		if !errors.Is(err, ErrRoleAlreadyExists) {
			t.Errorf("CreateRole() error = %v, want ErrRoleAlreadyExists", err)
		}
	})

	t.Run("returns ErrInvalidRoleName for invalid name", func(t *testing.T) {
		_, err := engine.CreateRole(ctx, "Invalid Name", "Invalid", "Bad name")
		if !errors.Is(err, ErrInvalidRoleName) {
			t.Errorf("CreateRole() error = %v, want ErrInvalidRoleName", err)
		}
	})
}

func TestEngine_GetRole(t *testing.T) {
	engine, cleanup := setupTestEngine(t, defaultTestConfig())
	defer cleanup()

	ctx := context.Background()

	// Create a role first
	_, err := engine.CreateRole(ctx, "testrole", "Test Role", "For testing")
	if err != nil {
		t.Fatalf("Setup CreateRole() error = %v", err)
	}

	t.Run("returns role successfully", func(t *testing.T) {
		role, err := engine.GetRole(ctx, "testrole")
		if err != nil {
			t.Fatalf("GetRole() error = %v", err)
		}
		if role.Name != "testrole" {
			t.Errorf("Role.Name = %v, want testrole", role.Name)
		}
	})

	t.Run("returns ErrRoleNotFound for nonexistent", func(t *testing.T) {
		_, err := engine.GetRole(ctx, "nonexistent")
		if !errors.Is(err, ErrRoleNotFound) {
			t.Errorf("GetRole() error = %v, want ErrRoleNotFound", err)
		}
	})
}

func TestEngine_ListRoles(t *testing.T) {
	engine, cleanup := setupTestEngine(t, defaultTestConfig())
	defer cleanup()

	ctx := context.Background()

	t.Run("returns empty list when no roles", func(t *testing.T) {
		roles, err := engine.ListRoles(ctx)
		if err != nil {
			t.Fatalf("ListRoles() error = %v", err)
		}
		if len(roles) != 0 {
			t.Errorf("ListRoles() returned %d roles, want 0", len(roles))
		}
	})

	// Create some roles
	engine.CreateRole(ctx, "rolea", "Role A", "First")
	engine.CreateRole(ctx, "roleb", "Role B", "Second")

	t.Run("returns all roles", func(t *testing.T) {
		roles, err := engine.ListRoles(ctx)
		if err != nil {
			t.Fatalf("ListRoles() error = %v", err)
		}
		if len(roles) != 2 {
			t.Errorf("ListRoles() returned %d roles, want 2", len(roles))
		}
	})
}

func TestEngine_UpdateRole(t *testing.T) {
	engine, cleanup := setupTestEngine(t, defaultTestConfig())
	defer cleanup()

	ctx := context.Background()

	// Create a role first
	_, err := engine.CreateRole(ctx, "updatetest", "Original", "Original desc")
	if err != nil {
		t.Fatalf("Setup CreateRole() error = %v", err)
	}

	t.Run("updates role successfully", func(t *testing.T) {
		role, err := engine.UpdateRole(ctx, "updatetest", "Updated", "New desc")
		if err != nil {
			t.Fatalf("UpdateRole() error = %v", err)
		}
		if role.DisplayName != "Updated" {
			t.Errorf("Role.DisplayName = %v, want Updated", role.DisplayName)
		}
		if role.Description != "New desc" {
			t.Errorf("Role.Description = %v, want New desc", role.Description)
		}
	})

	t.Run("returns ErrRoleNotFound for nonexistent", func(t *testing.T) {
		_, err := engine.UpdateRole(ctx, "nonexistent", "Name", "Desc")
		if !errors.Is(err, ErrRoleNotFound) {
			t.Errorf("UpdateRole() error = %v, want ErrRoleNotFound", err)
		}
	})
}

func TestEngine_DeleteRole(t *testing.T) {
	engine, cleanup := setupTestEngine(t, defaultTestConfig())
	defer cleanup()

	ctx := context.Background()

	t.Run("returns ErrCannotDeleteSystemRole for system roles", func(t *testing.T) {
		// First create the system role
		engine.CreateRole(ctx, "admin", "Admin", "System admin")

		err := engine.DeleteRole(ctx, "admin")
		if !errors.Is(err, ErrCannotDeleteSystemRole) {
			t.Errorf("DeleteRole() error = %v, want ErrCannotDeleteSystemRole", err)
		}
	})

	t.Run("deletes role and cleans up associated data", func(t *testing.T) {
		// Create a role with permissions and assignments
		_, err := engine.CreateRole(ctx, "deletable", "Deletable", "To be deleted")
		if err != nil {
			t.Fatalf("CreateRole() error = %v", err)
		}
		engine.GrantRolePermission(ctx, "deletable", "create", "room", ObjectIdAny)
		engine.AssignRole(ctx, "user1", "deletable")

		// Delete it
		err = engine.DeleteRole(ctx, "deletable")
		if err != nil {
			t.Fatalf("DeleteRole() error = %v", err)
		}

		// Verify it's gone
		_, err = engine.GetRole(ctx, "deletable")
		if !errors.Is(err, ErrRoleNotFound) {
			t.Errorf("GetRole() after delete error = %v, want ErrRoleNotFound", err)
		}
	})

	t.Run("returns ErrRoleNotFound for nonexistent", func(t *testing.T) {
		err := engine.DeleteRole(ctx, "nonexistent")
		if !errors.Is(err, ErrRoleNotFound) {
			t.Errorf("DeleteRole() error = %v, want ErrRoleNotFound", err)
		}
	})
}

func TestEngine_GrantRolePermission(t *testing.T) {
	engine, cleanup := setupTestEngine(t, defaultTestConfig())
	defer cleanup()

	ctx := context.Background()

	// Create a role first
	engine.CreateRole(ctx, "permtest", "Perm Test", "For testing perms")

	t.Run("grants permission successfully", func(t *testing.T) {
		err := engine.GrantRolePermission(ctx, "permtest", "create", "room", ObjectIdAny)
		if err != nil {
			t.Fatalf("GrantPermission() error = %v", err)
		}

		perms, _ := engine.GetRolePermissions(ctx, "permtest")
		if len(perms) != 1 {
			t.Errorf("GetRolePermissions() returned %d perms, want 1", len(perms))
		} else if perms[0].Verb != "create" || perms[0].ObjectType != "room" {
			t.Errorf("GetRolePermissions() = %+v, want verb=create, objectType=room", perms[0])
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		err := engine.GrantRolePermission(ctx, "permtest", "create", "room", ObjectIdAny)
		if err != nil {
			t.Errorf("GrantPermission() second call error = %v", err)
		}
	})

	t.Run("returns error for invalid permission", func(t *testing.T) {
		err := engine.GrantRolePermission(ctx, "permtest", "invalid", "perm", ObjectIdAny)
		if !errors.Is(err, ErrInvalidPermission) {
			t.Errorf("GrantPermission() error = %v, want ErrInvalidPermission", err)
		}
	})

	t.Run("returns ErrRoleNotFound for nonexistent role", func(t *testing.T) {
		err := engine.GrantRolePermission(ctx, "nonexistent", "create", "room", ObjectIdAny)
		if !errors.Is(err, ErrRoleNotFound) {
			t.Errorf("GrantPermission() error = %v, want ErrRoleNotFound", err)
		}
	})
}

func TestEngine_RevokeRolePermission(t *testing.T) {
	engine, cleanup := setupTestEngine(t, defaultTestConfig())
	defer cleanup()

	ctx := context.Background()

	// Setup: create role with permission
	engine.CreateRole(ctx, "revoketest", "Revoke Test", "For testing")
	engine.GrantRolePermission(ctx, "revoketest", "create", "room", ObjectIdAny)

	t.Run("revokes permission successfully", func(t *testing.T) {
		err := engine.RevokeRolePermission(ctx, "revoketest", "create", "room", ObjectIdAny)
		if err != nil {
			t.Fatalf("RevokePermission() error = %v", err)
		}

		perms, _ := engine.GetRolePermissions(ctx, "revoketest")
		if len(perms) != 0 {
			t.Errorf("GetRolePermissions() = %v, want []", perms)
		}
	})

	t.Run("is idempotent for non-granted permission", func(t *testing.T) {
		err := engine.RevokeRolePermission(ctx, "revoketest", "post", "message", ObjectIdAny)
		if err != nil {
			t.Errorf("RevokePermission() for non-granted error = %v", err)
		}
	})
}

func TestEngine_AssignRole(t *testing.T) {
	engine, cleanup := setupTestEngine(t, defaultTestConfig())
	defer cleanup()

	ctx := context.Background()

	// Create a role first
	engine.CreateRole(ctx, "assigntest", "Assign Test", "For testing")

	t.Run("assigns role successfully", func(t *testing.T) {
		err := engine.AssignRole(ctx, "user123", "assigntest")
		if err != nil {
			t.Fatalf("AssignRole() error = %v", err)
		}

		roles, _ := engine.GetUserRoles(ctx, "user123")
		if len(roles) != 1 || roles[0] != "assigntest" {
			t.Errorf("GetUserRoles() = %v, want [assigntest]", roles)
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		err := engine.AssignRole(ctx, "user123", "assigntest")
		if err != nil {
			t.Errorf("AssignRole() second call error = %v", err)
		}
	})

	t.Run("returns ErrRoleNotFound for nonexistent role", func(t *testing.T) {
		err := engine.AssignRole(ctx, "user456", "nonexistent")
		if !errors.Is(err, ErrRoleNotFound) {
			t.Errorf("AssignRole() error = %v, want ErrRoleNotFound", err)
		}
	})
}

func TestEngine_RevokeRole(t *testing.T) {
	engine, cleanup := setupTestEngine(t, defaultTestConfig())
	defer cleanup()

	ctx := context.Background()

	// Setup
	engine.CreateRole(ctx, "revokeroletest", "Revoke Role", "For testing")
	engine.AssignRole(ctx, "user789", "revokeroletest")

	t.Run("revokes role successfully", func(t *testing.T) {
		err := engine.RevokeRole(ctx, "user789", "revokeroletest")
		if err != nil {
			t.Fatalf("RevokeRole() error = %v", err)
		}

		roles, _ := engine.GetUserRoles(ctx, "user789")
		if len(roles) != 0 {
			t.Errorf("GetUserRoles() = %v, want []", roles)
		}
	})

	t.Run("is idempotent for non-assigned role", func(t *testing.T) {
		err := engine.RevokeRole(ctx, "user789", "revokeroletest")
		if err != nil {
			t.Errorf("RevokeRole() for non-assigned error = %v", err)
		}
	})
}

func TestEngine_GetRoleUsers(t *testing.T) {
	engine, cleanup := setupTestEngine(t, defaultTestConfig())
	defer cleanup()

	ctx := context.Background()

	// Setup
	engine.CreateRole(ctx, "userstest", "Users Test", "For testing")
	engine.AssignRole(ctx, "alice", "userstest")
	engine.AssignRole(ctx, "bob", "userstest")

	t.Run("returns all users with role", func(t *testing.T) {
		users, err := engine.GetRoleUsers(ctx, "userstest")
		if err != nil {
			t.Fatalf("GetRoleUsers() error = %v", err)
		}
		if len(users) != 2 {
			t.Errorf("GetRoleUsers() returned %d users, want 2", len(users))
		}
	})

	t.Run("returns ErrRoleNotFound for nonexistent", func(t *testing.T) {
		_, err := engine.GetRoleUsers(ctx, "nonexistent")
		if !errors.Is(err, ErrRoleNotFound) {
			t.Errorf("GetRoleUsers() error = %v, want ErrRoleNotFound", err)
		}
	})
}

func TestEngine_HasPermissionViaRoles(t *testing.T) {
	engine, cleanup := setupTestEngine(t, defaultTestConfig())
	defer cleanup()

	ctx := context.Background()

	// Setup: create admin and regular roles
	engine.CreateRole(ctx, "admin", "Admin", "Full access")
	engine.CreateRole(ctx, "editor", "Editor", "Can edit")
	engine.GrantRolePermission(ctx, "editor", "post", "message", ObjectIdAny)

	t.Run("admin needs explicit permissions like any other role", func(t *testing.T) {
		engine.AssignRole(ctx, "admin-user", "admin")

		// Admin does NOT have implicit all permissions - must be granted explicitly
		has, err := engine.HasPermissionViaRoles(ctx, "admin-user", "post", "message", ObjectIdAny)
		if err != nil {
			t.Fatalf("HasPermissionViaRoles() error = %v", err)
		}
		if has {
			t.Error("HasPermissionViaRoles() = true, want false for admin without explicit grant")
		}

		// Grant the permission explicitly
		engine.GrantRolePermission(ctx, "admin", "post", "message", ObjectIdAny)
		has, err = engine.HasPermissionViaRoles(ctx, "admin-user", "post", "message", ObjectIdAny)
		if err != nil {
			t.Fatalf("HasPermissionViaRoles() error = %v", err)
		}
		if !has {
			t.Error("HasPermissionViaRoles() = false for admin with explicit grant, want true")
		}

		// Admin should NOT have non-granted permissions
		has, err = engine.HasPermissionViaRoles(ctx, "admin-user", "manage", "space", ObjectIdAny)
		if err != nil {
			t.Fatalf("HasPermissionViaRoles() error = %v", err)
		}
		if has {
			t.Error("HasPermissionViaRoles() = true for admin with non-granted perm, want false")
		}
	})

	t.Run("user with role has granted permission", func(t *testing.T) {
		engine.AssignRole(ctx, "editor-user", "editor")

		has, err := engine.HasPermissionViaRoles(ctx, "editor-user", "post", "message", ObjectIdAny)
		if err != nil {
			t.Fatalf("HasPermissionViaRoles() error = %v", err)
		}
		if !has {
			t.Error("HasPermissionViaRoles() = false, want true")
		}
	})

	t.Run("user without role has no permission", func(t *testing.T) {
		has, err := engine.HasPermissionViaRoles(ctx, "random-user", "post", "message", ObjectIdAny)
		if err != nil {
			t.Fatalf("HasPermissionViaRoles() error = %v", err)
		}
		if has {
			t.Error("HasPermissionViaRoles() = true, want false")
		}
	})

	t.Run("user with role doesn't have non-granted permission", func(t *testing.T) {
		has, err := engine.HasPermissionViaRoles(ctx, "editor-user", "manage", "space", ObjectIdAny)
		if err != nil {
			t.Fatalf("HasPermissionViaRoles() error = %v", err)
		}
		if has {
			t.Error("HasPermissionViaRoles() = true for non-granted perm, want false")
		}
	})
}

func TestEngine_RolePermissionDenials(t *testing.T) {
	engine, cleanup := setupTestEngine(t, defaultTestConfig())
	defer cleanup()

	ctx := context.Background()

	// Setup: create roles
	engine.CreateRole(ctx, "editor", "Editor", "Can edit content")
	engine.CreateRole(ctx, "suspended", "Suspended", "Suspended user")

	t.Run("DenyRolePermission creates denial", func(t *testing.T) {
		err := engine.DenyRolePermission(ctx, "suspended", "post", "message", ObjectIdAny)
		if err != nil {
			t.Fatalf("DenyRolePermission() error = %v", err)
		}

		denials, err := engine.GetRolePermissionDenials(ctx, "suspended")
		if err != nil {
			t.Fatalf("GetRolePermissionDenials() error = %v", err)
		}
		if len(denials) != 1 {
			t.Errorf("GetRolePermissionDenials() returned %d denials, want 1", len(denials))
		} else if denials[0].Verb != "post" || denials[0].ObjectType != "message" {
			t.Errorf("GetRolePermissionDenials() = %+v, want verb=post, objectType=message", denials[0])
		}
	})

	t.Run("DenyRolePermission removes existing grant", func(t *testing.T) {
		// First grant the permission
		engine.GrantRolePermission(ctx, "editor", "post", "message", ObjectIdAny)

		// Verify it's granted
		perms, _ := engine.GetRolePermissions(ctx, "editor")
		if len(perms) == 0 || perms[0].Verb != "post" {
			t.Fatal("Setup failed: permission not granted")
		}

		// Now deny it
		err := engine.DenyRolePermission(ctx, "editor", "post", "message", ObjectIdAny)
		if err != nil {
			t.Fatalf("DenyRolePermission() error = %v", err)
		}

		// Verify grant is removed
		perms, _ = engine.GetRolePermissions(ctx, "editor")
		if len(perms) != 0 {
			t.Errorf("Grant should be removed, got %v", perms)
		}

		// Verify denial is set
		denials, _ := engine.GetRolePermissionDenials(ctx, "editor")
		if len(denials) != 1 || denials[0].Verb != "post" {
			t.Errorf("Denial should be set, got %v", denials)
		}
	})

	t.Run("GrantRolePermission removes existing denial", func(t *testing.T) {
		// First deny the permission
		engine.DenyRolePermission(ctx, "editor", "manage", "space", ObjectIdAny)

		// Verify it's denied
		denials, _ := engine.GetRolePermissionDenials(ctx, "editor")
		found := false
		for _, d := range denials {
			if d.Verb == "manage" && d.ObjectType == "space" {
				found = true
				break
			}
		}
		if !found {
			t.Fatal("Setup failed: permission not denied")
		}

		// Now grant it
		err := engine.GrantRolePermission(ctx, "editor", "manage", "space", ObjectIdAny)
		if err != nil {
			t.Fatalf("GrantRolePermission() error = %v", err)
		}

		// Verify denial is removed
		denials, _ = engine.GetRolePermissionDenials(ctx, "editor")
		for _, d := range denials {
			if d.Verb == "manage" && d.ObjectType == "space" {
				t.Error("Denial should be removed")
			}
		}

		// Verify grant is set
		perms, _ := engine.GetRolePermissions(ctx, "editor")
		found = false
		for _, p := range perms {
			if p.Verb == "manage" && p.ObjectType == "space" {
				found = true
				break
			}
		}
		if !found {
			t.Error("Grant should be set")
		}
	})

	t.Run("ClearRolePermissionState removes both grant and denial", func(t *testing.T) {
		// Set a grant
		engine.GrantRolePermission(ctx, "editor", "create", "room", ObjectIdAny)
		// Set a denial on different permission
		engine.DenyRolePermission(ctx, "editor", "delete", "room", ObjectIdAny)

		// Clear both
		err := engine.ClearRolePermissionState(ctx, "editor", "create", "room", ObjectIdAny)
		if err != nil {
			t.Fatalf("ClearRolePermissionState() error = %v", err)
		}
		err = engine.ClearRolePermissionState(ctx, "editor", "delete", "room", ObjectIdAny)
		if err != nil {
			t.Fatalf("ClearRolePermissionState() error = %v", err)
		}

		// Verify grant is removed
		perms, _ := engine.GetRolePermissions(ctx, "editor")
		for _, p := range perms {
			if p.Verb == "create" && p.ObjectType == "room" {
				t.Error("Grant should be cleared")
			}
		}

		// Verify denial is removed
		denials, _ := engine.GetRolePermissionDenials(ctx, "editor")
		for _, d := range denials {
			if d.Verb == "delete" && d.ObjectType == "room" {
				t.Error("Denial should be cleared")
			}
		}
	})

	t.Run("role denial overrides grant from another role", func(t *testing.T) {
		// Create a new user and assign them to two roles
		engine.GrantRolePermission(ctx, "editor", "post", "message", ObjectIdAny)

		// Create a fresh user
		engine.AssignRole(ctx, "test-user", "editor")
		engine.AssignRole(ctx, "test-user", "suspended")

		// Verify user has editor permission
		engine.DenyRolePermission(ctx, "suspended", "post", "message", ObjectIdAny)

		// Now check: suspended role should deny what editor grants
		has, err := engine.HasPermissionViaRoles(ctx, "test-user", "post", "message", ObjectIdAny)
		if err != nil {
			t.Fatalf("HasPermissionViaRoles() error = %v", err)
		}
		if has {
			t.Error("HasPermissionViaRoles() = true, want false (denial should override grant)")
		}
	})

	t.Run("admin role is subject to role denials like any other role", func(t *testing.T) {
		// Create admin role and assign it
		engine.CreateRole(ctx, "admin", "Admin", "Admin role")
		engine.GrantRolePermission(ctx, "admin", "post", "message", ObjectIdAny) // Admin must have explicit grants
		engine.AssignRole(ctx, "admin-user", "admin")
		engine.AssignRole(ctx, "admin-user", "suspended")

		// Admin should NOT have the permission due to suspended role denial
		// (deny-wins pattern applies to admin like any other role)
		has, err := engine.HasPermissionViaRoles(ctx, "admin-user", "post", "message", ObjectIdAny)
		if err != nil {
			t.Fatalf("HasPermissionViaRoles() error = %v", err)
		}
		if has {
			t.Error("HasPermissionViaRoles() = true for admin, want false (deny-wins applies to admin)")
		}
	})
}

func TestEngine_HasUserAdmin(t *testing.T) {
	engine, cleanup := setupTestEngine(t, defaultTestConfig())
	defer cleanup()

	ctx := context.Background()

	// Create admin role
	engine.CreateRole(ctx, "admin", "Admin", "Full access")

	t.Run("returns false for non-admin", func(t *testing.T) {
		isAdmin, err := engine.HasUserAdmin(ctx, "regular-user")
		if err != nil {
			t.Fatalf("HasUserAdmin() error = %v", err)
		}
		if isAdmin {
			t.Error("HasUserAdmin() = true, want false")
		}
	})

	t.Run("returns true for admin", func(t *testing.T) {
		engine.AssignRole(ctx, "admin-user", "admin")

		isAdmin, err := engine.HasUserAdmin(ctx, "admin-user")
		if err != nil {
			t.Fatalf("HasUserAdmin() error = %v", err)
		}
		if !isAdmin {
			t.Error("HasUserAdmin() = false, want true")
		}
	})
}

func TestEngine_RevokeAllUserRoles(t *testing.T) {
	engine, cleanup := setupTestEngine(t, defaultTestConfig())
	defer cleanup()

	ctx := context.Background()

	// Setup: create roles and assign to users
	engine.CreateRole(ctx, "rolea", "Role A", "First role")
	engine.CreateRole(ctx, "roleb", "Role B", "Second role")
	engine.AssignRole(ctx, "user1", "rolea")
	engine.AssignRole(ctx, "user1", "roleb")
	engine.AssignRole(ctx, "user2", "rolea") // Different user - should not be affected

	t.Run("removes all roles for user", func(t *testing.T) {
		err := engine.RevokeAllUserRoles(ctx, "user1")
		if err != nil {
			t.Fatalf("RevokeAllUserRoles() error = %v", err)
		}

		roles, _ := engine.GetUserRoles(ctx, "user1")
		if len(roles) != 0 {
			t.Errorf("Expected 0 roles after revoke, got %d: %v", len(roles), roles)
		}
	})

	t.Run("does not affect other users", func(t *testing.T) {
		roles, _ := engine.GetUserRoles(ctx, "user2")
		if len(roles) != 1 {
			t.Errorf("Expected 1 role for user2, got %d: %v", len(roles), roles)
		}
		if len(roles) > 0 && roles[0] != "rolea" {
			t.Errorf("Expected user2 to have rolea, got %v", roles)
		}
	})

	t.Run("is idempotent", func(t *testing.T) {
		// Call again on user with no roles - should not error
		err := engine.RevokeAllUserRoles(ctx, "user1")
		if err != nil {
			t.Errorf("Second RevokeAllUserRoles() error = %v", err)
		}
	})

	t.Run("succeeds for user with no roles", func(t *testing.T) {
		// Call on a user that never had roles
		err := engine.RevokeAllUserRoles(ctx, "nonexistent-user")
		if err != nil {
			t.Errorf("RevokeAllUserRoles() for user with no roles error = %v", err)
		}
	})
}

func TestEngine_UtilityFunctions(t *testing.T) {
	engine, cleanup := setupTestEngine(t, defaultTestConfig())
	defer cleanup()

	ctx := context.Background()

	engine.CreateRole(ctx, "utiltest", "Util Test", "For testing")
	engine.GrantRolePermission(ctx, "utiltest", "create", "room", ObjectIdAny)

	t.Run("RoleExists", func(t *testing.T) {
		exists, err := engine.RoleExists(ctx, "utiltest")
		if err != nil {
			t.Fatalf("RoleExists() error = %v", err)
		}
		if !exists {
			t.Error("RoleExists() = false, want true")
		}

		exists, err = engine.RoleExists(ctx, "nonexistent")
		if err != nil {
			t.Fatalf("RoleExists() error = %v", err)
		}
		if exists {
			t.Error("RoleExists() = true for nonexistent, want false")
		}
	})

	t.Run("HasRole", func(t *testing.T) {
		engine.AssignRole(ctx, "has-role-user", "utiltest")

		has, err := engine.HasRole(ctx, "has-role-user", "utiltest")
		if err != nil {
			t.Fatalf("HasRole() error = %v", err)
		}
		if !has {
			t.Error("HasRole() = false, want true")
		}

		has, err = engine.HasRole(ctx, "other-user", "utiltest")
		if err != nil {
			t.Fatalf("HasRole() error = %v", err)
		}
		if has {
			t.Error("HasRole() = true for non-assigned, want false")
		}
	})

	t.Run("RoleHasPermission", func(t *testing.T) {
		has, err := engine.RoleHasPermission(ctx, "utiltest", "create", "room", ObjectIdAny)
		if err != nil {
			t.Fatalf("RoleHasPermission() error = %v", err)
		}
		if !has {
			t.Error("RoleHasPermission() = false, want true")
		}

		has, err = engine.RoleHasPermission(ctx, "utiltest", "post", "message", ObjectIdAny)
		if err != nil {
			t.Fatalf("RoleHasPermission() error = %v", err)
		}
		if has {
			t.Error("RoleHasPermission() = true for non-granted, want false")
		}
	})
}

func TestParseKeyFunctions(t *testing.T) {
	t.Run("ParseMemberKey", func(t *testing.T) {
		tests := []struct {
			key      string
			wantRole string
			wantUser string
		}{
			{"member.admin.U9mP2qR5tYz3wK", "admin", "U9mP2qR5tYz3wK"},
			{"member.owner.U9mP2qR5tYz3wK", "owner", "U9mP2qR5tYz3wK"},
			{"member.moderator.Uabc123def456x", "moderator", "Uabc123def456x"},
			{"role.admin", "", ""},   // Wrong prefix
			{"member.admin", "", ""}, // Missing user
			{"invalid", "", ""},      // Completely invalid
		}

		for _, tt := range tests {
			role, user := ParseMemberKey(tt.key)
			if role != tt.wantRole || user != tt.wantUser {
				t.Errorf("ParseMemberKey(%q) = (%q, %q), want (%q, %q)",
					tt.key, role, user, tt.wantRole, tt.wantUser)
			}
		}
	})

	t.Run("ParseAllowKey", func(t *testing.T) {
		tests := []struct {
			key            string
			wantSubject    string
			wantVerb       string
			wantObjectType string
			wantObjectId   string
		}{
			{"allow.admin.create.room.any", "admin", "create", "room", "any"},
			{"allow.moderator.access.admin.any", "moderator", "access", "admin", "any"},
			{"allow.U9mP2qR5tYz3wK.create.room.any", "U9mP2qR5tYz3wK", "create", "room", "any"},
			{"allow.everyone.post.message.Rabc456", "everyone", "post", "message", "Rabc456"},
			{"deny.admin.create.room.any", "", "", "", ""},  // Wrong prefix
			{"allow.admin.create", "", "", "", ""},          // Missing parts
			{"invalid", "", "", "", ""},                     // Completely invalid
		}

		for _, tt := range tests {
			parts := ParseAllowKey(tt.key)
			if parts.Subject != tt.wantSubject || parts.Verb != tt.wantVerb ||
				parts.ObjectType != tt.wantObjectType || parts.ObjectId != tt.wantObjectId {
				t.Errorf("ParseAllowKey(%q) = {Subject:%q, Verb:%q, ObjectType:%q, ObjectId:%q}, want {Subject:%q, Verb:%q, ObjectType:%q, ObjectId:%q}",
					tt.key, parts.Subject, parts.Verb, parts.ObjectType, parts.ObjectId,
					tt.wantSubject, tt.wantVerb, tt.wantObjectType, tt.wantObjectId)
			}
		}
	})

	t.Run("ParseDenyKey", func(t *testing.T) {
		tests := []struct {
			key            string
			wantSubject    string
			wantVerb       string
			wantObjectType string
			wantObjectId   string
		}{
			{"deny.everyone.create.room.any", "everyone", "create", "room", "any"},
			{"deny.everyone.create.space.any", "everyone", "create", "space", "any"},
			{"deny.U9mP2qR5tYz3wK.create.room.any", "U9mP2qR5tYz3wK", "create", "room", "any"},
			{"allow.admin.create.room.any", "", "", "", ""}, // Wrong prefix
			{"deny.admin", "", "", "", ""},                  // Missing parts
			{"invalid", "", "", "", ""},                     // Completely invalid
		}

		for _, tt := range tests {
			parts := ParseDenyKey(tt.key)
			if parts.Subject != tt.wantSubject || parts.Verb != tt.wantVerb ||
				parts.ObjectType != tt.wantObjectType || parts.ObjectId != tt.wantObjectId {
				t.Errorf("ParseDenyKey(%q) = {Subject:%q, Verb:%q, ObjectType:%q, ObjectId:%q}, want {Subject:%q, Verb:%q, ObjectType:%q, ObjectId:%q}",
					tt.key, parts.Subject, parts.Verb, parts.ObjectType, parts.ObjectId,
					tt.wantSubject, tt.wantVerb, tt.wantObjectType, tt.wantObjectId)
			}
		}
	})
}

// ============================================================================
// Role Hierarchy Tests
// ============================================================================

func TestEngine_GetUserHighestPosition(t *testing.T) {
	engine, cleanup := setupTestEngine(t, defaultTestConfig())
	defer cleanup()

	ctx := context.Background()

	// Create roles with different positions. Higher position = higher rank.
	engine.CreateRoleWithPosition(ctx, "owner", "Owner", "Top rank", 300)
	engine.CreateRoleWithPosition(ctx, "moderator", "Moderator", "Mid rank", 200)
	engine.CreateRoleWithPosition(ctx, "editor", "Editor", "Lower rank", 100)

	t.Run("returns PositionEveryone for user with no roles", func(t *testing.T) {
		pos, err := engine.GetUserHighestPosition(ctx, "no-roles-user")
		if err != nil {
			t.Fatalf("GetUserHighestPosition() error = %v", err)
		}
		if pos != PositionEveryone {
			t.Errorf("GetUserHighestPosition() = %d, want %d (PositionEveryone)", pos, PositionEveryone)
		}
	})

	t.Run("returns role position for user with single role", func(t *testing.T) {
		engine.AssignRole(ctx, "single-role-user", "moderator")

		pos, err := engine.GetUserHighestPosition(ctx, "single-role-user")
		if err != nil {
			t.Fatalf("GetUserHighestPosition() error = %v", err)
		}
		if pos != 200 {
			t.Errorf("GetUserHighestPosition() = %d, want 200", pos)
		}
	})

	t.Run("returns highest position (highest rank) for user with multiple roles", func(t *testing.T) {
		engine.AssignRole(ctx, "multi-role-user", "editor")    // position 100
		engine.AssignRole(ctx, "multi-role-user", "moderator") // position 200

		pos, err := engine.GetUserHighestPosition(ctx, "multi-role-user")
		if err != nil {
			t.Fatalf("GetUserHighestPosition() error = %v", err)
		}
		if pos != 200 {
			t.Errorf("GetUserHighestPosition() = %d, want 200 (highest of assigned roles)", pos)
		}
	})

	t.Run("updates correctly after role revocation", func(t *testing.T) {
		engine.AssignRole(ctx, "revoke-test-user", "owner")     // position 300
		engine.AssignRole(ctx, "revoke-test-user", "moderator") // position 200

		pos, _ := engine.GetUserHighestPosition(ctx, "revoke-test-user")
		if pos != 300 {
			t.Errorf("Before revoke: position = %d, want 300", pos)
		}

		engine.RevokeRole(ctx, "revoke-test-user", "owner")

		pos, err := engine.GetUserHighestPosition(ctx, "revoke-test-user")
		if err != nil {
			t.Fatalf("GetUserHighestPosition() after revoke error = %v", err)
		}
		if pos != 200 {
			t.Errorf("After revoke: position = %d, want 200", pos)
		}
	})
}

func TestEngine_CanUserManageRole(t *testing.T) {
	engine, cleanup := setupTestEngine(t, defaultTestConfig())
	defer cleanup()

	ctx := context.Background()

	// Higher position = higher rank.
	engine.CreateRoleWithPosition(ctx, "owner", "Owner", "Owner role", PositionOwner)
	engine.CreateRoleWithPosition(ctx, "admin", "Admin", "Admin role", PositionAdmin)
	engine.CreateRoleWithPosition(ctx, "moderator", "Moderator", "Moderator role", PositionModerator)
	engine.CreateRoleWithPosition(ctx, "editor", "Editor", "Editor role", 50)

	t.Run("owner can manage any role", func(t *testing.T) {
		engine.AssignRole(ctx, "owner-user", "owner")

		for _, p := range []int32{PositionOwner, PositionAdmin, PositionModerator, 50} {
			can, err := engine.CanUserManageRole(ctx, "owner-user", p)
			if err != nil {
				t.Fatalf("CanUserManageRole(%d): %v", p, err)
			}
			if !can {
				t.Errorf("Owner should be able to manage role at position %d", p)
			}
		}
	})

	t.Run("admin can manage lower-ranked roles only", func(t *testing.T) {
		engine.AssignRole(ctx, "admin-user", "admin")

		// Admin cannot manage admin (same position) or owner (higher rank).
		for _, p := range []int32{PositionAdmin, PositionOwner} {
			can, _ := engine.CanUserManageRole(ctx, "admin-user", p)
			if can {
				t.Errorf("Admin should NOT be able to manage role at position %d", p)
			}
		}

		// Admin can manage moderator (lower rank).
		can, _ := engine.CanUserManageRole(ctx, "admin-user", PositionModerator)
		if !can {
			t.Error("Admin should be able to manage moderator role")
		}
	})

	t.Run("moderator can manage roles below their position", func(t *testing.T) {
		engine.AssignRole(ctx, "mod-user", "moderator")

		can, _ := engine.CanUserManageRole(ctx, "mod-user", 50)
		if !can {
			t.Error("Moderator should be able to manage editor role")
		}
	})

	t.Run("user cannot manage roles at same position", func(t *testing.T) {
		engine.AssignRole(ctx, "mod-user-2", "moderator")

		can, _ := engine.CanUserManageRole(ctx, "mod-user-2", PositionModerator)
		if can {
			t.Error("Moderator should NOT be able to manage roles at same position")
		}
	})

	t.Run("user cannot manage roles at higher position (higher rank)", func(t *testing.T) {
		engine.AssignRole(ctx, "editor-user", "editor")

		for _, p := range []int32{PositionModerator, PositionAdmin} {
			can, _ := engine.CanUserManageRole(ctx, "editor-user", p)
			if can {
				t.Errorf("Editor should NOT be able to manage role at position %d", p)
			}
		}
	})

	t.Run("user with no roles cannot manage any custom role", func(t *testing.T) {
		can, _ := engine.CanUserManageRole(ctx, "no-roles-user", 50)
		if can {
			t.Error("User with no roles should NOT be able to manage any role")
		}
	})
}

func TestEngine_OutranksUser_Hierarchy(t *testing.T) {
	engine, cleanup := setupTestEngine(t, defaultTestConfig())
	defer cleanup()

	ctx := context.Background()

	// Higher position = higher rank.
	engine.CreateRoleWithPosition(ctx, "admin", "Admin", "Admin role", PositionAdmin)
	engine.CreateRoleWithPosition(ctx, "moderator", "Moderator", "Moderator role", PositionModerator)
	engine.CreateRoleWithPosition(ctx, "editor", "Editor", "Editor role", 50)

	// Assign roles
	engine.AssignRole(ctx, "admin-user", "admin")
	engine.AssignRole(ctx, "mod-user", "moderator")
	engine.AssignRole(ctx, "editor-user", "editor")

	t.Run("higher-ranked user can manage lower-ranked user", func(t *testing.T) {
		// Admin can manage moderator
		can, err := engine.OutranksUser(ctx, "admin-user", "mod-user")
		if err != nil {
			t.Fatalf("OutranksUser error = %v", err)
		}
		if !can {
			t.Error("Admin should be able to manage moderator")
		}

		// Moderator can manage editor
		can, err = engine.OutranksUser(ctx, "mod-user", "editor-user")
		if err != nil {
			t.Fatalf("OutranksUser error = %v", err)
		}
		if !can {
			t.Error("Moderator should be able to manage editor")
		}
	})

	t.Run("equal-ranked users cannot manage each other", func(t *testing.T) {
		engine.AssignRole(ctx, "mod-user-2", "moderator")

		// Moderator cannot manage another moderator
		can, err := engine.OutranksUser(ctx, "mod-user", "mod-user-2")
		if err != nil {
			t.Fatalf("OutranksUser error = %v", err)
		}
		if can {
			t.Error("Moderator should NOT be able to manage another moderator")
		}

		// And vice versa
		can, err = engine.OutranksUser(ctx, "mod-user-2", "mod-user")
		if err != nil {
			t.Fatalf("OutranksUser error = %v", err)
		}
		if can {
			t.Error("Moderator should NOT be able to manage another moderator (reverse)")
		}
	})

	t.Run("lower-ranked user cannot manage higher-ranked user", func(t *testing.T) {
		// Editor cannot manage moderator
		can, err := engine.OutranksUser(ctx, "editor-user", "mod-user")
		if err != nil {
			t.Fatalf("OutranksUser error = %v", err)
		}
		if can {
			t.Error("Editor should NOT be able to manage moderator")
		}

		// Editor cannot manage admin
		can, err = engine.OutranksUser(ctx, "editor-user", "admin-user")
		if err != nil {
			t.Fatalf("OutranksUser error = %v", err)
		}
		if can {
			t.Error("Editor should NOT be able to manage admin")
		}
	})

	t.Run("user with no roles cannot manage user with any role", func(t *testing.T) {
		can, err := engine.OutranksUser(ctx, "no-roles-user", "editor-user")
		if err != nil {
			t.Fatalf("OutranksUser error = %v", err)
		}
		if can {
			t.Error("User with no roles should NOT be able to manage user with role")
		}
	})

	t.Run("user can manage user with no roles", func(t *testing.T) {
		// Any user with a role can manage users with no roles (PositionEveryone)
		can, err := engine.OutranksUser(ctx, "editor-user", "no-roles-user")
		if err != nil {
			t.Fatalf("OutranksUser error = %v", err)
		}
		if !can {
			t.Error("User with role should be able to manage user with no roles")
		}
	})
}

func TestEngine_GetNextAvailablePosition(t *testing.T) {
	ctx := context.Background()

	t.Run("returns PositionCustomFirst when no custom roles exist", func(t *testing.T) {
		engine, cleanup := setupTestEngine(t, defaultTestConfig())
		defer cleanup()

		pos, err := engine.GetNextAvailablePosition(ctx)
		if err != nil {
			t.Fatalf("GetNextAvailablePosition() error = %v", err)
		}
		if pos != PositionCustomFirst {
			t.Errorf("GetNextAvailablePosition() = %d, want %d (PositionCustomFirst)", pos, PositionCustomFirst)
		}
	})

	t.Run("returns one past highest existing custom role", func(t *testing.T) {
		engine, cleanup := setupTestEngine(t, defaultTestConfig())
		defer cleanup()

		_, err := engine.CreateRoleWithPosition(ctx, "firstrole", "First Role", "First", PositionCustomFirst)
		if err != nil {
			t.Fatalf("CreateRoleWithPosition() error = %v", err)
		}

		pos, err := engine.GetNextAvailablePosition(ctx)
		if err != nil {
			t.Fatalf("GetNextAvailablePosition() error = %v", err)
		}
		if pos != PositionCustomFirst+1 {
			t.Errorf("GetNextAvailablePosition() = %d, want %d", pos, PositionCustomFirst+1)
		}

		_, err = engine.CreateRoleWithPosition(ctx, "secondrole", "Second Role", "Second", PositionCustomFirst+1)
		if err != nil {
			t.Fatalf("CreateRoleWithPosition() error = %v", err)
		}

		pos, err = engine.GetNextAvailablePosition(ctx)
		if err != nil {
			t.Fatalf("GetNextAvailablePosition() error = %v", err)
		}
		if pos != PositionCustomFirst+2 {
			t.Errorf("GetNextAvailablePosition() = %d, want %d", pos, PositionCustomFirst+2)
		}
	})

	t.Run("handles gaps in positions", func(t *testing.T) {
		engine, cleanup := setupTestEngine(t, defaultTestConfig())
		defer cleanup()

		// Create roles at positions 1 and 5, skipping 2-4
		_, err := engine.CreateRoleWithPosition(ctx, "firstrole", "First Role", "First", 1)
		if err != nil {
			t.Fatalf("CreateRoleWithPosition() error = %v", err)
		}
		_, err = engine.CreateRoleWithPosition(ctx, "fifthrole", "Fifth Role", "Fifth", 5)
		if err != nil {
			t.Fatalf("CreateRoleWithPosition() error = %v", err)
		}

		pos, err := engine.GetNextAvailablePosition(ctx)
		if err != nil {
			t.Fatalf("GetNextAvailablePosition() error = %v", err)
		}
		// Should return 6 (after highest position), not fill gaps
		if pos != 6 {
			t.Errorf("GetNextAvailablePosition() = %d, want 6", pos)
		}
	})
}

func TestEngine_UpdateRolePosition(t *testing.T) {
	engine, cleanup := setupTestEngine(t, defaultTestConfig())
	defer cleanup()

	ctx := context.Background()

	// Create a role
	engine.CreateRoleWithPosition(ctx, "testpos", "Test Position", "For testing", 5)

	t.Run("updates position successfully", func(t *testing.T) {
		role, err := engine.UpdateRolePosition(ctx, "testpos", 10)
		if err != nil {
			t.Fatalf("UpdateRolePosition() error = %v", err)
		}
		if role.Position != 10 {
			t.Errorf("UpdateRolePosition() position = %d, want 10", role.Position)
		}

		// Verify persistence
		fetched, _ := engine.GetRole(ctx, "testpos")
		if fetched.Position != 10 {
			t.Errorf("Persisted position = %d, want 10", fetched.Position)
		}
	})

	t.Run("preserves other role fields", func(t *testing.T) {
		role, err := engine.UpdateRolePosition(ctx, "testpos", 15)
		if err != nil {
			t.Fatalf("UpdateRolePosition() error = %v", err)
		}
		if role.DisplayName != "Test Position" {
			t.Errorf("DisplayName = %q, want %q", role.DisplayName, "Test Position")
		}
		if role.Description != "For testing" {
			t.Errorf("Description = %q, want %q", role.Description, "For testing")
		}
	})

	t.Run("returns ErrRoleNotFound for nonexistent role", func(t *testing.T) {
		_, err := engine.UpdateRolePosition(ctx, "nonexistent", 1)
		if !errors.Is(err, ErrRoleNotFound) {
			t.Errorf("UpdateRolePosition() error = %v, want ErrRoleNotFound", err)
		}
	})
}

func TestEngine_ReorderRoles(t *testing.T) {
	engine, cleanup := setupTestEngine(t, defaultTestConfig())
	defer cleanup()

	ctx := context.Background()

	// Create custom roles with initial positions starting at PositionCustomFirst
	// to avoid collisions with system roles (admin=1, moderator=2).
	engine.CreateRoleWithPosition(ctx, "alpha", "Alpha", "First", PositionCustomFirst)
	engine.CreateRoleWithPosition(ctx, "beta", "Beta", "Second", PositionCustomFirst+1)
	engine.CreateRoleWithPosition(ctx, "gamma", "Gamma", "Third", PositionCustomFirst+2)

	t.Run("reorders roles correctly", func(t *testing.T) {
		// Reorder: gamma, alpha, beta (reversed order with gamma first).
		// New positions start at PositionCustomFirst so they never collide
		// with admin (1) or moderator (2).
		roles, err := engine.ReorderRoles(ctx, []string{"gamma", "alpha", "beta"})
		if err != nil {
			t.Fatalf("ReorderRoles() error = %v", err)
		}

		posMap := make(map[string]int32)
		for _, r := range roles {
			posMap[r.Name] = r.Position
		}

		want := map[string]int32{
			"gamma": PositionCustomFirst,
			"alpha": PositionCustomFirst + 1,
			"beta":  PositionCustomFirst + 2,
		}
		for name, w := range want {
			if posMap[name] != w {
				t.Errorf("%s position = %d, want %d", name, posMap[name], w)
			}
		}
	})

	t.Run("returns error for system roles in list", func(t *testing.T) {
		_, err := engine.ReorderRoles(ctx, []string{"admin", "gamma"})
		if err == nil {
			t.Error("ReorderRoles() should error for system role")
		}
	})

	t.Run("returns error for nonexistent role in list", func(t *testing.T) {
		_, err := engine.ReorderRoles(ctx, []string{"alpha", "nonexistent"})
		if err == nil {
			t.Error("ReorderRoles() should error for nonexistent role")
		}
	})

	t.Run("returns all roles sorted by position", func(t *testing.T) {
		roles, err := engine.ReorderRoles(ctx, []string{"beta", "gamma", "alpha"})
		if err != nil {
			t.Fatalf("ReorderRoles() error = %v", err)
		}

		// Verify sorted order
		for i := 1; i < len(roles); i++ {
			if roles[i].Position < roles[i-1].Position {
				t.Errorf("Roles not sorted by position: %s(%d) < %s(%d)",
					roles[i].Name, roles[i].Position, roles[i-1].Name, roles[i-1].Position)
			}
		}
	})

	t.Run("empty list is valid no-op", func(t *testing.T) {
		roles, err := engine.ReorderRoles(ctx, []string{})
		if err != nil {
			t.Fatalf("ReorderRoles() with empty list error = %v", err)
		}
		// Should return existing roles
		if len(roles) == 0 {
			t.Error("ReorderRoles() with empty list should return existing roles")
		}
	})
}

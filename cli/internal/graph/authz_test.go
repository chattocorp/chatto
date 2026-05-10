package graph

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/core"
)

func TestRequireAuth(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("authenticated user succeeds", func(t *testing.T) {
		user, err := requireAuth(env.authContext())
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if user.Id != env.testUser.Id {
			t.Errorf("Expected user ID %s, got %s", env.testUser.Id, user.Id)
		}
	})

	t.Run("unauthenticated returns error", func(t *testing.T) {
		_, err := requireAuth(env.unauthContext())
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}
	})
}

func TestRequireSelf(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("accessing own data succeeds", func(t *testing.T) {
		user, err := requireSelf(env.authContext(), env.testUser.Id)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if user.Id != env.testUser.Id {
			t.Errorf("Expected user ID %s, got %s", env.testUser.Id, user.Id)
		}
	})

	t.Run("accessing other user's data fails", func(t *testing.T) {
		otherUser, err := env.core.CreateUser(env.ctx, "system", "other", "Other", "password123")
		if err != nil {
			t.Fatalf("Failed to create other user: %v", err)
		}

		_, err = requireSelf(env.authContext(), otherUser.Id)
		if !errors.Is(err, ErrNotSelf) {
			t.Errorf("Expected ErrNotSelf, got %v", err)
		}
	})

	t.Run("unauthenticated returns auth error", func(t *testing.T) {
		_, err := requireSelf(env.unauthContext(), env.testUser.Id)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}
	})
}

func TestRequireSpaceMember(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("space member succeeds", func(t *testing.T) {
		user, err := requireSpaceMember(env.authContext(), env.core, env.testSpace.Id)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if user.Id != env.testUser.Id {
			t.Errorf("Expected user ID %s, got %s", env.testUser.Id, user.Id)
		}
	})

	t.Run("non-member fails", func(t *testing.T) {
		// Create a user who is not a member of the space
		nonMember, err := env.core.CreateUser(env.ctx, "system", "nonmember", "Non Member", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		_, err = requireSpaceMember(env.authContextForUser(nonMember), env.core, env.testSpace.Id)
		if !errors.Is(err, ErrNotSpaceMember) {
			t.Errorf("Expected ErrNotSpaceMember, got %v", err)
		}
	})

	t.Run("unauthenticated returns auth error", func(t *testing.T) {
		_, err := requireSpaceMember(env.unauthContext(), env.core, env.testSpace.Id)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}
	})
}

func TestRequireRoomMember(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("room member succeeds", func(t *testing.T) {
		user, err := requireRoomMember(env.authContext(), env.core, env.testSpace.Id, env.testRoom.Id)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if user.Id != env.testUser.Id {
			t.Errorf("Expected user ID %s, got %s", env.testUser.Id, user.Id)
		}
	})

	t.Run("space member but not room member fails", func(t *testing.T) {
		// Create a user who is a space member but not room member
		spaceMember, err := env.core.CreateUser(env.ctx, "system", "spacemember", "Space Member", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}
		_, err = env.core.JoinSpace(env.ctx, spaceMember.Id, env.testSpace.Id)
		if err != nil {
			t.Fatalf("Failed to join space: %v", err)
		}

		_, err = requireRoomMember(env.authContextForUser(spaceMember), env.core, env.testSpace.Id, env.testRoom.Id)
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("Expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("not even a space member fails", func(t *testing.T) {
		// Create a user who is not a member of the space at all
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider", "Outsider", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		_, err = requireRoomMember(env.authContextForUser(outsider), env.core, env.testSpace.Id, env.testRoom.Id)
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("Expected ErrNotRoomMember, got %v", err)
		}
	})

	t.Run("unauthenticated returns auth error", func(t *testing.T) {
		_, err := requireRoomMember(env.unauthContext(), env.core, env.testSpace.Id, env.testRoom.Id)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}
	})
}

func TestRequireInstanceAdmin(t *testing.T) {
	adminEmail := "admin@example.com"
	env := setupTestResolverWithAdmin(t, []string{adminEmail})

	t.Run("config admin with verified email succeeds", func(t *testing.T) {
		// Note: Username "admin" is blocked by default, so we use "adminuser"
		admin, err := env.core.CreateUser(env.ctx, "system", "adminuser", "Admin", "password123")
		if err != nil {
			t.Fatalf("Failed to create admin: %v", err)
		}
		// Verify the admin email so they are recognized as config admin
		if err := env.core.AddVerifiedEmailDirect(env.ctx, admin.Id, adminEmail); err != nil {
			t.Fatalf("Failed to verify admin email: %v", err)
		}

		user, err := requireInstanceAdmin(env.authContextForUser(admin), env.core)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if user.Id != admin.Id {
			t.Errorf("Expected user ID %s, got %s", admin.Id, user.Id)
		}
	})

	t.Run("RBAC admin succeeds", func(t *testing.T) {
		rbacAdmin, err := env.core.CreateUser(env.ctx, "system", "rbacadmin", "RBACAdmin", "password123")
		if err != nil {
			t.Fatalf("Failed to create RBAC admin: %v", err)
		}

		// Assign admin role via RBAC
		if err := env.core.AssignInstanceAdminRole(env.ctx, rbacAdmin.Id); err != nil {
			t.Fatalf("Failed to assign admin role: %v", err)
		}

		user, err := requireInstanceAdmin(env.authContextForUser(rbacAdmin), env.core)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if user.Id != rbacAdmin.Id {
			t.Errorf("Expected user ID %s, got %s", rbacAdmin.Id, user.Id)
		}
	})

	t.Run("non-admin fails", func(t *testing.T) {
		// Create a second user who is NOT an admin (the first user from setupTestResolver
		// is auto-promoted to instance owner, so we need a fresh user)
		regularUser, err := env.core.CreateUser(env.ctx, "system", "regularuser", "Regular User", "password123")
		if err != nil {
			t.Fatalf("Failed to create regular user: %v", err)
		}

		_, err = requireInstanceAdmin(env.authContextForUser(regularUser), env.core)
		if !errors.Is(err, ErrNotInstanceAdmin) {
			t.Errorf("Expected ErrNotInstanceAdmin, got %v", err)
		}
	})

	t.Run("unauthenticated returns auth error", func(t *testing.T) {
		_, err := requireInstanceAdmin(env.unauthContext(), env.core)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}
	})
}

func TestRequireInstancePermission(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("everyone has default permissions", func(t *testing.T) {
		user, err := env.core.CreateUser(env.ctx, "system", "member", "Member", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		// Everyone should have dm.view by default
		_, err = requireInstancePermission(env.authContextForUser(user), env.core, core.PermDMView)
		if err != nil {
			t.Errorf("Expected user to have dm.view, got error: %v", err)
		}

		// Everyone should have dm.write by default
		_, err = requireInstancePermission(env.authContextForUser(user), env.core, core.PermDMWrite)
		if err != nil {
			t.Errorf("Expected user to have dm.write, got error: %v", err)
		}
	})

	t.Run("everyone denial blocks permission", func(t *testing.T) {
		user, err := env.core.CreateUser(env.ctx, "system", "denied", "Denied", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		// Deny dm.write for everyone role
		if err := env.core.DenyInstancePermission(env.ctx, core.RoleEveryone, core.PermDMWrite); err != nil {
			t.Fatalf("Failed to deny permission: %v", err)
		}

		// Permission should be denied
		_, err = requireInstancePermission(env.authContextForUser(user), env.core, core.PermDMWrite)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("Expected ErrPermissionDenied (everyone role denial), got %v", err)
		}
	})

	t.Run("admin role grants admin permission", func(t *testing.T) {
		user, err := env.core.CreateUser(env.ctx, "system", "granted", "Granted", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		// Members don't have admin by default
		_, err = requireInstancePermission(env.authContextForUser(user), env.core, core.PermAdminAccess)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("Expected ErrPermissionDenied for admin, got %v", err)
		}

		// Assign admin role
		if err := env.core.AssignInstanceRole(env.ctx, core.SystemActorID, user.Id, core.RoleAdmin); err != nil {
			t.Fatalf("Failed to assign admin role: %v", err)
		}

		// Should now have access
		_, err = requireInstancePermission(env.authContextForUser(user), env.core, core.PermAdminAccess)
		if err != nil {
			t.Errorf("Expected admin to work after role assignment, got error: %v", err)
		}
	})

	t.Run("unauthenticated returns auth error", func(t *testing.T) {
		_, err := requireInstancePermission(env.unauthContext(), env.core, core.PermDMView)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}
	})
}

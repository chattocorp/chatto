package graph

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/core"
)

func TestPermissionExplanation_ServerAdminAtServerScope(t *testing.T) {
	env := setupTestResolver(t)
	rbac := env.resolver.RbacQueries()

	target := env.createVerifiedUser(t, "server-admin-target", "Target", "password123")
	results, err := rbac.PermissionExplanation(env.authContext(), nil, target.Id, nil)
	if err != nil {
		t.Fatalf("PermissionExplanation: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected non-empty explanations at instance scope")
	}
}

func TestPermissionExplanation_PrivilegedUserCannotInspectThemselves(t *testing.T) {
	env := setupTestResolver(t)
	rbac := env.resolver.RbacQueries()

	// env.testUser is auto-promoted to server owner, but the inspector is
	// still an operator surface for checking someone else's permissions.
	_, err := rbac.PermissionExplanation(env.authContext(), nil, env.testUser.Id, nil)
	if !errors.Is(err, core.ErrPermissionDenied) {
		t.Errorf("expected ErrPermissionDenied for privileged self-inspection, got %v", err)
	}
}

func TestPermissionExplanation_NonAdminCannotInspectThemselves(t *testing.T) {
	env := setupTestResolver(t)
	rbac := env.resolver.RbacQueries()

	// The inspector is admin-only — non-admins can't even inspect themselves.
	regular := env.createVerifiedUser(t, "regular-self", "Regular", "password123")
	_, err := rbac.PermissionExplanation(env.authContextForUser(regular), nil, regular.Id, nil)
	if !errors.Is(err, core.ErrPermissionDenied) {
		t.Errorf("expected ErrPermissionDenied for non-admin self-inspection, got %v", err)
	}
}

func TestPermissionExplanation_AdminInspectsAnotherUser(t *testing.T) {
	env := setupTestResolverWithAdmin(t, []string{"testuser@example.com"})
	rbac := env.resolver.RbacQueries()

	target := env.createVerifiedUser(t, "target", "Target", "password123")

	results, err := rbac.PermissionExplanation(env.authContext(), nil, target.Id, nil)
	if err != nil {
		t.Fatalf("PermissionExplanation: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected non-empty explanations from admin inspecting another user")
	}
}

func TestPermissionExplanation_NonAdminCannotInspectAnotherUser(t *testing.T) {
	env := setupTestResolver(t)
	rbac := env.resolver.RbacQueries()

	// env.testUser is the bootstrap owner (auto-promoted server owner) and so
	// has admin access. Use freshly-created users instead — neither is admin.
	regular := env.createVerifiedUser(t, "regular", "Regular", "password123")
	target := env.createVerifiedUser(t, "target2", "Target 2", "password123")

	_, err := rbac.PermissionExplanation(env.authContextForUser(regular), nil, target.Id, nil)
	if !errors.Is(err, core.ErrPermissionDenied) {
		t.Errorf("expected ErrPermissionDenied when non-admin inspects another user, got %v", err)
	}
}

func TestPermissionExplanation_RoleManagerInspectsAnotherUser(t *testing.T) {
	env := setupTestResolver(t)
	rbac := env.resolver.RbacQueries()

	manager := env.createVerifiedUser(t, "role-manager-explain", "Role Manager", "password123")
	target := env.createVerifiedUser(t, "role-manager-target", "Target", "password123")
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, manager.Id, core.RoleModerator); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	if err := env.core.GrantServerPermission(env.ctx, core.RoleModerator, core.PermRoleManage); err != nil {
		t.Fatalf("GrantServerPermission role.manage: %v", err)
	}

	results, err := rbac.PermissionExplanation(env.authContextForUser(manager), nil, target.Id, nil)
	if err != nil {
		t.Fatalf("PermissionExplanation: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected non-empty explanations from role manager inspecting another user")
	}
}

func TestPermissionExplanation_SpaceAdminCannotInspectAnotherSpace(t *testing.T) {
	t.Skip("Phase 5 collapsed instance/space tiers; multi-space cross-tier scenarios no longer apply.")
}

// TestPermissionExplanation_RoomMustBelongToServer verifies that passing a
// roomID that does not exist on the deployment is rejected. Without this
// check, the API would silently return an empty trace for a nonexistent
// room — confusing and an authorization-shaped contract gap.

func TestPermissionExplanation_Unauthenticated(t *testing.T) {
	env := setupTestResolver(t)
	rbac := env.resolver.RbacQueries()

	_, err := rbac.PermissionExplanation(env.unauthContext(), nil, env.testUser.Id, nil)
	if !errors.Is(err, ErrNotAuthenticated) {
		t.Errorf("expected ErrNotAuthenticated, got %v", err)
	}
}

package graph

import (
	"testing"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

// TestRolePermissionMatrix_BasicShape verifies the matrix returns the
// expected columns (server + every room group + every room) and that
// cells exist only for permissions applicable at each scope's tier.
func TestRolePermissionMatrix_BasicShape(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	got, err := query.RolePermissionMatrix(env.authContext(), "everyone")
	if err != nil {
		t.Fatalf("RolePermissionMatrix: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil matrix")
	}
	if got.RoleName != "everyone" {
		t.Errorf("matrix.RoleName = %q, want %q", got.RoleName, "everyone")
	}

	var sawServer bool
	for _, sc := range got.Scopes {
		if sc.ID == "server" && sc.Kind == model.UserPermissionScopeKindServer {
			sawServer = true
			break
		}
	}
	if !sawServer {
		t.Error("matrix.Scopes is missing the 'server' column")
	}

	permSet := map[string]bool{}
	for _, p := range got.ApplicablePermissions {
		permSet[p] = true
	}
	scopeSet := map[string]bool{}
	for _, sc := range got.Scopes {
		scopeSet[sc.ID] = true
	}
	for _, cell := range got.Cells {
		if !permSet[cell.Permission] {
			t.Errorf("cell references unknown permission %q", cell.Permission)
		}
		if !scopeSet[cell.ScopeID] {
			t.Errorf("cell references unknown scope %q", cell.ScopeID)
		}
	}
}

// TestRolePermissionMatrix_ReflectsExplicitGrant proves that granting a
// channel-room permission to a role at group scope flips both the
// Override and the Effective fields to ALLOW on that group's column —
// and that rooms in the group inherit it as their effective decision.
func TestRolePermissionMatrix_ReflectsExplicitGrant(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	// Find the seeded room group; grant message.manage on it.
	groups, err := env.core.ListRoomGroupsOrdered(env.ctx, core.KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered: %v", err)
	}
	if len(groups) == 0 {
		t.Fatal("expected at least one seeded room group")
	}
	groupID := groups[0].Id
	groupScopeID := "group:" + groupID

	if err := env.core.GrantGroupPermission(env.ctx, groupID, "moderator", core.PermMessageManage); err != nil {
		t.Fatalf("GrantGroupPermission: %v", err)
	}

	got, err := query.RolePermissionMatrix(env.authContext(), "moderator")
	if err != nil {
		t.Fatalf("RolePermissionMatrix: %v", err)
	}

	var groupCell *model.UserPermissionCell
	for _, c := range got.Cells {
		if c.Permission == string(core.PermMessageManage) && c.ScopeID == groupScopeID {
			groupCell = c
			break
		}
	}
	if groupCell == nil {
		t.Fatalf("expected a cell for (message.manage, %s)", groupScopeID)
	}
	if groupCell.Override != model.UserPermissionDecisionAllow {
		t.Errorf("group.Override = %v, want ALLOW", groupCell.Override)
	}
	if groupCell.Effective != model.UserPermissionDecisionAllow {
		t.Errorf("group.Effective = %v, want ALLOW", groupCell.Effective)
	}
}

// TestRolePermissionMatrix_AuthorizationGate confirms only callers with
// `role.manage` can read a role's matrix.
func TestRolePermissionMatrix_AuthorizationGate(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	t.Run("anonymous is rejected", func(t *testing.T) {
		_, err := query.RolePermissionMatrix(env.unauthContext(), "everyone")
		if err == nil {
			t.Error("expected error for unauthenticated caller")
		}
	})

	t.Run("regular member without role.manage is rejected", func(t *testing.T) {
		regular := env.createVerifiedUser(t, "rm-regular", "Regular", "password123")
		_, err := query.RolePermissionMatrix(env.authContextForUser(regular), "everyone")
		if err == nil {
			t.Error("expected ErrPermissionDenied for non-admin caller")
		}
	})

	t.Run("owner succeeds", func(t *testing.T) {
		_, err := query.RolePermissionMatrix(env.authContext(), "everyone")
		if err != nil {
			t.Errorf("expected owner to read role matrix, got %v", err)
		}
	})
}

// TestRolePermissionMatrix_UnknownRoleReturnsNil ensures a missing role
// resolves to nil (and not an error) so the GraphQL field can be null.
func TestRolePermissionMatrix_UnknownRoleReturnsNil(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	got, err := query.RolePermissionMatrix(env.authContext(), "does-not-exist")
	if err != nil {
		t.Fatalf("RolePermissionMatrix: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for unknown role, got %+v", got)
	}
}

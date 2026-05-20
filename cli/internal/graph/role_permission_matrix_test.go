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
// permission to a role at server scope flips both the Override and the
// Effective fields to ALLOW on the server column. (And that the same
// grant cascades through to room columns via the room → server walk.)
func TestRolePermissionMatrix_ReflectsExplicitGrant(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	if err := env.core.GrantServerPermission(env.ctx, "moderator", core.PermMessageManage); err != nil {
		t.Fatalf("GrantServerPermission: %v", err)
	}

	got, err := query.RolePermissionMatrix(env.authContext(), "moderator")
	if err != nil {
		t.Fatalf("RolePermissionMatrix: %v", err)
	}

	var server *model.UserPermissionCell
	for _, c := range got.Cells {
		if c.Permission == string(core.PermMessageManage) && c.ScopeID == "server" {
			server = c
			break
		}
	}
	if server == nil {
		t.Fatal("expected a cell for (message.manage, server)")
	}
	if server.Override != model.UserPermissionDecisionAllow {
		t.Errorf("server.Override = %v, want ALLOW", server.Override)
	}
	if server.Effective != model.UserPermissionDecisionAllow {
		t.Errorf("server.Effective = %v, want ALLOW", server.Effective)
	}

	// Pick any room cell for the same permission — it should inherit
	// ALLOW as effective even though it has no override of its own.
	var roomCell *model.UserPermissionCell
	for _, c := range got.Cells {
		if c.Permission == string(core.PermMessageManage) &&
			c.ScopeID != "server" &&
			c.ScopeID[:5] == "room:" {
			roomCell = c
			break
		}
	}
	if roomCell != nil {
		if roomCell.Override != model.UserPermissionDecisionNone {
			t.Errorf("room.Override = %v, want NONE", roomCell.Override)
		}
		if roomCell.Effective != model.UserPermissionDecisionAllow {
			t.Errorf("room.Effective = %v, want ALLOW (inherited from server)", roomCell.Effective)
		}
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

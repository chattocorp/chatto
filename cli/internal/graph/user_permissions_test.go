package graph

import (
	"testing"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

// TestUserPermissionMatrix_BasicShape verifies the matrix returns the
// expected columns (server + every room group + every room) and that
// cells exist only for permissions applicable at each scope's tier.
func TestUserPermissionMatrix_BasicShape(t *testing.T) {
	env := setupTestResolver(t)
	rbac := env.resolver.RbacQueries()

	target := env.createVerifiedUser(t, "matrix-target", "Target", "password123")

	got, err := rbac.UserPermissionMatrix(env.authContext(), nil, target.Id)
	if err != nil {
		t.Fatalf("UserPermissionMatrix: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil matrix")
	}
	if got.UserID != target.Id {
		t.Errorf("matrix.UserID = %q, want %q", got.UserID, target.Id)
	}

	// Should contain at least the server column.
	var sawServer bool
	for _, sc := range got.Scopes {
		if sc.ID == "server" && sc.Kind == model.PermissionMatrixScopeKindServer {
			sawServer = true
			break
		}
	}
	if !sawServer {
		t.Error("matrix.Scopes is missing the 'server' column")
	}

	// Cells must reference valid permissions and scopes.
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

// TestUserPermissionMatrix_ReflectsExplicitOverride proves that writing
// a user-level override (server, group, or room scope) flips the cell's
// Override field from NONE to ALLOW/DENY, and updates the Effective
// state to match.
func TestUserPermissionMatrix_ReflectsExplicitOverride(t *testing.T) {
	env := setupTestResolver(t)
	rbac := env.resolver.RbacQueries()

	target := env.createVerifiedUser(t, "matrix-override", "Override", "password123")

	// Deny message.post on the user at server scope. Should show up as a
	// solid DENY override on the server column.
	if err := env.core.DenyUserPermission(env.ctx, core.SystemActorID, target.Id, core.PermMessagePost); err != nil {
		t.Fatalf("DenyUserPermission: %v", err)
	}

	got, err := rbac.UserPermissionMatrix(env.authContext(), nil, target.Id)
	if err != nil {
		t.Fatalf("UserPermissionMatrix: %v", err)
	}

	var cell *model.PermissionMatrixCell
	for _, c := range got.Cells {
		if c.Permission == string(core.PermMessagePost) && c.ScopeID == "server" {
			cell = c
			break
		}
	}
	if cell == nil {
		t.Fatal("expected a cell for (message.post, server)")
	}
	if cell.Override != model.PermissionMatrixDecisionDeny {
		t.Errorf("Override = %v, want DENY", cell.Override)
	}
	if cell.Effective != model.PermissionMatrixDecisionDeny {
		t.Errorf("Effective = %v, want DENY (any applicable deny wins over grants)", cell.Effective)
	}
}

// TestUserPermissionMatrix_AuthorizationGate confirms only authorized
// callers can read another user's matrix. Self-call is rejected (no
// self-bypass on the user-permission target gate), peers are rejected,
// and admins with user.manage-permissions succeed.
func TestUserPermissionMatrix_AuthorizationGate(t *testing.T) {
	env := setupTestResolver(t)
	rbac := env.resolver.RbacQueries()

	target := env.createVerifiedUser(t, "matrix-perm-target", "Target", "password123")

	t.Run("anonymous is rejected", func(t *testing.T) {
		_, err := rbac.UserPermissionMatrix(env.unauthContext(), nil, target.Id)
		if err == nil {
			t.Error("expected error for unauthenticated caller")
		}
	})

	t.Run("regular member is rejected", func(t *testing.T) {
		regular := env.createVerifiedUser(t, "matrix-perm-regular", "Regular", "password123")
		_, err := rbac.UserPermissionMatrix(env.authContextForUser(regular), nil, target.Id)
		if err == nil {
			t.Error("expected ErrPermissionDenied for non-admin caller")
		}
	})

	t.Run("self-call is rejected (no self-bypass)", func(t *testing.T) {
		_, err := rbac.UserPermissionMatrix(env.authContextForUser(target), nil, target.Id)
		if err == nil {
			t.Error("expected error for self-call — the user-permission gate has no self-bypass")
		}
	})

	t.Run("owner succeeds", func(t *testing.T) {
		_, err := rbac.UserPermissionMatrix(env.authContext(), nil, target.Id)
		if err != nil {
			t.Errorf("expected owner to read target's matrix, got %v", err)
		}
	})
}

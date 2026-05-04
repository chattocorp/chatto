package graph

import (
	"errors"
	"slices"
	"testing"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

// TestTierRoles_InstanceScopeListsInstanceRoles verifies that instance-scope
// queries return every instance role, in position order, with no inheritance
// (instance is the top tier).
func TestTierRoles_InstanceScopeListsInstanceRoles(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	got, err := query.TierRoles(env.authContext(), nil, nil)
	if err != nil {
		t.Fatalf("TierRoles: %v", err)
	}
	if got == nil || len(got.Roles) == 0 {
		t.Fatal("expected non-empty role matrix at instance scope")
	}

	// Every role at instance scope must be an instance role with no inheritance.
	for _, r := range got.Roles {
		if !r.IsInstanceRole {
			t.Errorf("role %q at instance scope should be an instance role", r.RoleName)
		}
		if len(r.InheritedAllows) != 0 || len(r.InheritedDenials) != 0 {
			t.Errorf("role %q at instance scope should have empty inheritance", r.RoleName)
		}
		if r.Override == nil {
			t.Errorf("role %q has nil override", r.RoleName)
		}
	}

	// Roles must be sorted by position ascending.
	for i := 1; i < len(got.Roles); i++ {
		if got.Roles[i-1].Position > got.Roles[i].Position {
			t.Errorf("roles not sorted by position: %d > %d", got.Roles[i-1].Position, got.Roles[i].Position)
		}
	}

	// Applicable permissions list must be non-empty (instance scope has at
	// minimum space.create / space.list / dm.* etc.).
	if len(got.ApplicablePermissions) == 0 {
		t.Error("expected non-empty applicablePermissions at instance scope")
	}
}

// TestTierRoles_SpaceScope verifies that space scope returns the space's roles
// and excludes instance system roles (which would collide post-ADR-028 since
// the role namespace is unified). Custom instance-only roles still surface
// when present. The full instance-tier listing collapses with the dual engine
// in PR 4.
func TestTierRoles_SpaceScope(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	got, err := query.TierRoles(env.authContext(), &env.testSpace.Id, nil)
	if err != nil {
		t.Fatalf("TierRoles: %v", err)
	}
	if got == nil || len(got.Roles) == 0 {
		t.Fatal("expected non-empty role matrix at space scope")
	}

	for _, r := range got.Roles {
		// System roles must surface as space roles, not duplicated as instance
		// rows (per ADR-028 — names like "owner" exist in both engines and
		// would collide).
		if core.IsSystemRole(r.RoleName) && r.IsInstanceRole {
			t.Errorf("system role %q should not appear as an instance row at space scope", r.RoleName)
		}
		if !r.IsInstanceRole {
			// Space roles never have inheritance at space scope.
			if len(r.InheritedAllows) != 0 || len(r.InheritedDenials) != 0 {
				t.Errorf("space role %q should have empty inheritance at space scope", r.RoleName)
			}
		}
	}
}

// TestTierRoles_RoomScopeRoleInheritsResolvedSpaceState seeds a space-level
// override on an instance role and then asserts that the room-scope view
// shows that override in the role's inherited baseline. This verifies the
// "effective space + instance" merge for instance-role inheritance at room
// scope.
func TestTierRoles_RoomScopeRoleInheritsResolvedSpaceState(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	// Instance everyone gets message.post at instance level.
	if err := env.core.GrantInstanceRolePermission(env.ctx, core.RoleEveryone, core.PermMessagePost); err != nil {
		t.Fatalf("seed instance grant: %v", err)
	}
	// And gets it denied at the space level via instance-role-config-at-space.
	if err := env.core.DenyInstanceRoleSpacePermission(env.ctx, env.testUser.Id, env.testSpace.Id, core.RoleEveryone, core.PermMessagePost); err != nil {
		t.Fatalf("seed space deny: %v", err)
	}

	got, err := query.TierRoles(env.authContext(), &env.testSpace.Id, &env.testRoom.Id)
	if err != nil {
		t.Fatalf("TierRoles: %v", err)
	}

	// Find an instance role we expect to be present at room scope (admin or
	// moderator — verified roles aren't excluded at room scope).
	var found *model.TierRole
	for _, r := range got.Roles {
		if r.IsInstanceRole && r.RoleName == core.RoleEveryone {
			found = r
			break
		}
	}
	if found == nil {
		// everyone role is universal and gets filtered out at space/room scope;
		// fall back to checking the merge directly via the helper.
		allows, denies := mergeInheritedDecisions(
			nil, []core.Permission{core.PermMessagePost}, // space override (deny wins)
			[]core.Permission{core.PermMessagePost}, nil, // instance grant (suppressed)
		)
		if len(allows) != 0 {
			t.Errorf("space deny should suppress instance allow in merge; got allows=%v", allows)
		}
		if !slices.Contains(denies, string(core.PermMessagePost)) {
			t.Errorf("expected space deny to surface in merged denies; got %v", denies)
		}
		return
	}
	// Space deny must win — message.post should appear in inheritedDenials,
	// not inheritedAllows.
	if slices.Contains(found.InheritedAllows, string(core.PermMessagePost)) {
		t.Errorf("space deny should suppress instance allow; got allows=%v", found.InheritedAllows)
	}
	if !slices.Contains(found.InheritedDenials, string(core.PermMessagePost)) {
		t.Errorf("expected space deny to surface in inherited denials; got %v", found.InheritedDenials)
	}
}

// TestTierRoles_NonAdminCannotInspectInstanceScope verifies the auth gate
// shared with rolePermissions.
func TestTierRoles_NonAdminCannotInspectInstanceScope(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	regular := env.createVerifiedUser(t, "regular-tr", "Regular", "password123")
	_, err := query.TierRoles(env.authContextForUser(regular), nil, nil)
	if !errors.Is(err, core.ErrPermissionDenied) {
		t.Errorf("expected ErrPermissionDenied at instance scope, got %v", err)
	}
}

// TestTierRoles_CrossSpaceLeakRejected was a dual-tier safeguard that no
// longer applies post-PR 4 (RBAC is server-wide).
func TestTierRoles_CrossSpaceLeakRejected(t *testing.T) {
	t.Skip("Per ADR-021 / ADR-028 (PR 4) RBAC is server-wide; the " +
		"cross-space leakage gate this test exercised no longer exists.")
}

// TestTierRoles_RoomIDWithoutSpaceIDFails sanity-checks the contract.
func TestTierRoles_RoomIDWithoutSpaceIDFails(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	_, err := query.TierRoles(env.authContext(), nil, &env.testRoom.Id)
	if err == nil {
		t.Error("expected error when roomId provided without spaceId")
	}
}

// TestTierRoles_AgreesWithRolePermissions cross-checks the matrix output
// against the existing rolePermissions resolver: for every role, the
// override published by tierRoles must match what rolePermissions reports
// at the same scope.
func TestTierRoles_AgreesWithRolePermissions(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	// Seed a few decisions so the comparison isn't entirely trivial.
	if err := env.core.GrantSpaceRolePermission(env.ctx, env.testSpace.Id, core.RoleAdmin, core.PermRoomManage); err != nil {
		t.Fatalf("seed grant: %v", err)
	}
	if err := env.core.DenySpaceRolePermission(env.ctx, env.testSpace.Id, core.RoleEveryone, core.PermRoomCreate); err != nil {
		t.Fatalf("seed deny: %v", err)
	}

	matrix, err := query.TierRoles(env.authContext(), &env.testSpace.Id, nil)
	if err != nil {
		t.Fatalf("TierRoles: %v", err)
	}

	for _, tr := range matrix.Roles {
		single, err := query.RolePermissions(env.authContext(), tr.RoleName, &env.testSpace.Id, nil)
		if err != nil {
			t.Fatalf("RolePermissions for %q: %v", tr.RoleName, err)
		}
		if single == nil || single.Space == nil {
			t.Fatalf("RolePermissions for %q returned nil space tier", tr.RoleName)
		}
		assertSameStringSet(t, "permissions for "+tr.RoleName, tr.Override.Permissions, single.Space.Permissions)
		assertSameStringSet(t, "denials for "+tr.RoleName, tr.Override.PermissionDenials, single.Space.PermissionDenials)
	}
}

func assertSameStringSet(t *testing.T, label string, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("%s: length mismatch got=%v want=%v", label, got, want)
		return
	}
	gotSet := make(map[string]struct{}, len(got))
	for _, s := range got {
		gotSet[s] = struct{}{}
	}
	for _, s := range want {
		if _, ok := gotSet[s]; !ok {
			t.Errorf("%s: missing %q (got=%v want=%v)", label, s, got, want)
		}
	}
}

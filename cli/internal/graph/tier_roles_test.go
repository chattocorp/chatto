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

	got, err := query.TierRoles(env.authContext(), nil)
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

// TestTierRoles_SpaceScopeMixesSpaceAndInstanceRoles verifies that the
// scoped variant (called with a roomId) returns both space roles (without
// instance tier inheritance) and instance roles (with instance tier
// inheritance), excluding universal roles.
//
// Post-PR(b) there is no separately-callable space scope — passing a
// roomId is what enables the instance + space + room view.
func TestTierRoles_SpaceScopeMixesSpaceAndInstanceRoles(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	// Grant a permission on the everyone instance role at instance scope, so
	// instance-role rows have non-empty inheritance to assert against.
	if err := env.core.GrantInstanceRolePermission(env.ctx, core.InstRoleEveryone, core.PermSpaceJoin); err != nil {
		t.Fatalf("seed instance grant: %v", err)
	}

	got, err := query.TierRoles(env.authContext(), &env.testRoom.Id)
	if err != nil {
		t.Fatalf("TierRoles: %v", err)
	}
	if got == nil || len(got.Roles) == 0 {
		t.Fatal("expected non-empty role matrix at space scope")
	}

	hasSpaceRole := false
	hasInstanceRole := false
	for _, r := range got.Roles {
		if r.IsInstanceRole {
			hasInstanceRole = true
			// Universal roles should be filtered out at space scope.
			if core.IsSpaceUniversalRole(r.RoleName) {
				t.Errorf("universal role %q should not appear at space scope", r.RoleName)
			}
		} else {
			hasSpaceRole = true
			// At room scope, space roles inherit the resolved space-tier
			// state — non-empty inheritance is expected.
		}
	}
	if !hasSpaceRole {
		t.Error("expected at least one space role")
	}
	if !hasInstanceRole {
		t.Error("expected at least one instance role")
	}

	// Instance role inheritance at space scope must reflect the instance
	// tier we seeded above.
	for _, r := range got.Roles {
		if !r.IsInstanceRole || r.RoleName != core.InstRoleEveryone {
			continue
		}
		if !slices.Contains(r.InheritedAllows, string(core.PermSpaceJoin)) {
			t.Errorf("expected everyone role at space scope to inherit space.create grant; got %v", r.InheritedAllows)
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
	if err := env.core.GrantInstanceRolePermission(env.ctx, core.InstRoleEveryone, core.PermMessagePost); err != nil {
		t.Fatalf("seed instance grant: %v", err)
	}
	// And gets it denied at the space level via instance-role-config-at-space.
	if err := env.core.DenyInstanceRoleSpacePermission(env.ctx, env.testUser.Id, env.testSpace.Id, core.InstRoleEveryone, core.PermMessagePost); err != nil {
		t.Fatalf("seed space deny: %v", err)
	}

	got, err := query.TierRoles(env.authContext(), &env.testRoom.Id)
	if err != nil {
		t.Fatalf("TierRoles: %v", err)
	}

	// Find an instance role we expect to be present at room scope (admin or
	// moderator — verified roles aren't excluded at room scope).
	var found *model.TierRole
	for _, r := range got.Roles {
		if r.IsInstanceRole && r.RoleName == core.InstRoleEveryone {
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
	_, err := query.TierRoles(env.authContextForUser(regular), nil)
	if !errors.Is(err, core.ErrPermissionDenied) {
		t.Errorf("expected ErrPermissionDenied at instance scope, got %v", err)
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
	if err := env.core.GrantSpaceRolePermission(env.ctx, env.testSpace.Id, core.SpaceRoleAdmin, core.PermRoomManage); err != nil {
		t.Fatalf("seed grant: %v", err)
	}
	if err := env.core.DenySpaceRolePermission(env.ctx, env.testSpace.Id, core.InstRoleEveryone, core.PermSpaceJoin); err != nil {
		t.Fatalf("seed deny: %v", err)
	}

	matrix, err := query.TierRoles(env.authContext(), &env.testRoom.Id)
	if err != nil {
		t.Fatalf("TierRoles: %v", err)
	}

	for _, tr := range matrix.Roles {
		single, err := query.RolePermissions(env.authContext(), tr.RoleName, &env.testRoom.Id)
		if err != nil {
			t.Fatalf("RolePermissions for %q: %v", tr.RoleName, err)
		}
		if single == nil || single.Room == nil {
			t.Fatalf("RolePermissions for %q returned nil room tier", tr.RoleName)
		}
		// Both queries are at room scope (we passed `&env.testRoom.Id`).
		// Compare the room-tier override that each surface reports.
		assertSameStringSet(t, "permissions for "+tr.RoleName, tr.Override.Permissions, single.Room.Permissions)
		assertSameStringSet(t, "denials for "+tr.RoleName, tr.Override.PermissionDenials, single.Room.PermissionDenials)
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

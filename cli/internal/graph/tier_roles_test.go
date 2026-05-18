package graph

import (
	"errors"
	"slices"
	"testing"

	"hmans.de/chatto/internal/core"
)

// TestTierRoles_ServerScopeListsAllRoles verifies that server-scope queries
// return every role, in position order, with no inheritance (server is the
// top tier).
func TestTierRoles_ServerScopeListsAllRoles(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	got, err := query.TierRoles(env.authContext(), nil, nil)
	if err != nil {
		t.Fatalf("TierRoles: %v", err)
	}
	if got == nil || len(got.Roles) == 0 {
		t.Fatal("expected non-empty role matrix at server scope")
	}

	for _, r := range got.Roles {
		if len(r.InheritedAllows) != 0 || len(r.InheritedDenials) != 0 {
			t.Errorf("role %q at server scope should have empty inheritance", r.RoleName)
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

	if len(got.ApplicablePermissions) == 0 {
		t.Error("expected non-empty applicablePermissions at server scope")
	}
}

// TestTierRoles_RoomScopeShowsServerInheritance seeds a server-level grant
// on the everyone role and asserts that other roles' room-scope view shows
// that grant in their inherited baseline.
func TestTierRoles_RoomScopeShowsServerInheritance(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	if err := env.core.GrantInstancePermission(env.ctx, core.RoleAdmin, core.PermMessagePost); err != nil {
		t.Fatalf("seed server grant: %v", err)
	}

	got, err := query.TierRoles(env.authContext(), &env.testRoom.Id, nil)
	if err != nil {
		t.Fatalf("TierRoles: %v", err)
	}
	if got == nil || len(got.Roles) == 0 {
		t.Fatal("expected non-empty role matrix at room scope")
	}

	// `everyone` is a real column at room scope — operators need to edit
	// per-room overrides on it (e.g. the announcements pattern: deny
	// message.post on everyone in this one room).
	var everyone *struct{ allows, denies []string }
	for _, r := range got.Roles {
		if r.RoleName == core.RoleEveryone {
			everyone = &struct{ allows, denies []string }{r.InheritedAllows, r.InheritedDenials}
			break
		}
	}
	if everyone == nil {
		t.Error("expected everyone role to appear at room scope")
	}

	var admin *struct{ allows, denies []string }
	for _, r := range got.Roles {
		if r.RoleName == core.RoleAdmin {
			admin = &struct{ allows, denies []string }{r.InheritedAllows, r.InheritedDenials}
			break
		}
	}
	if admin == nil {
		t.Fatal("expected admin role in room-scope tier matrix")
	}
	if !slices.Contains(admin.allows, string(core.PermMessagePost)) {
		t.Errorf("expected admin role at room scope to inherit message.post grant; got %v", admin.allows)
	}
}

// TestTierRoles_NonAdminCannotInspectServerScope verifies the auth gate
// shared with rolePermissions.
func TestTierRoles_NonAdminCannotInspectServerScope(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	regular := env.createVerifiedUser(t, "regular-tr", "Regular", "password123")
	_, err := query.TierRoles(env.authContextForUser(regular), nil, nil)
	if !errors.Is(err, core.ErrPermissionDenied) {
		t.Errorf("expected ErrPermissionDenied at server scope, got %v", err)
	}
}

// TestTierRoles_AgreesWithRolePermissions cross-checks the matrix output
// against the existing rolePermissions resolver: for every role, the
// override published by tierRoles must match what rolePermissions reports
// at the same scope.
func TestTierRoles_AgreesWithRolePermissions(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	if err := env.core.GrantInstancePermission(env.ctx, core.RoleAdmin, core.PermRoomManage); err != nil {
		t.Fatalf("seed grant: %v", err)
	}
	if err := env.core.DenyInstancePermission(env.ctx, core.RoleEveryone, core.PermMessagePost); err != nil {
		t.Fatalf("seed deny: %v", err)
	}

	matrix, err := query.TierRoles(env.authContext(), &env.testRoom.Id, nil)
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
		assertSameStringSet(t, "permissions for "+tr.RoleName, tr.Override.Permissions, single.Room.Permissions)
		assertSameStringSet(t, "denials for "+tr.RoleName, tr.Override.PermissionDenials, single.Room.PermissionDenials)
	}
}

// TestTierRoles_RoomScopeGroupDenyShadowsServerAllow pins down the fix for a
// matrix-display bug where a permission denied at group scope but allowed at
// server scope appeared in BOTH inheritedAllows AND inheritedDenials at room
// scope, so the editor rendered a faded "allow" baseline on top of the real
// deny. The effective inheritance baseline must mirror what the walker would
// resolve without a per-room override: group decisions suppress server
// decisions for the same role+perm.
func TestTierRoles_RoomScopeGroupDenyShadowsServerAllow(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	// message.post is granted at server scope by default (everyone). Add an
	// explicit deny at the room's group on everyone — the matrix's room-scope
	// inheritance baseline should now report DENY, with no parallel ALLOW.
	groupID := env.testRoom.GroupId
	if err := env.core.DenyGroupPermission(env.ctx, groupID, core.RoleEveryone, core.PermMessagePost); err != nil {
		t.Fatalf("DenyGroupPermission: %v", err)
	}

	got, err := query.TierRoles(env.authContext(), &env.testRoom.Id, nil)
	if err != nil {
		t.Fatalf("TierRoles: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil TierRoles result")
	}

	var everyone *struct{ allows, denies []string }
	for _, r := range got.Roles {
		if r.RoleName == core.RoleEveryone {
			everyone = &struct{ allows, denies []string }{r.InheritedAllows, r.InheritedDenials}
			break
		}
	}
	if everyone == nil {
		t.Fatal("expected everyone role at room scope")
	}

	if !slices.Contains(everyone.denies, string(core.PermMessagePost)) {
		t.Errorf("expected message.post in inheritedDenials at room scope; got %v", everyone.denies)
	}
	if slices.Contains(everyone.allows, string(core.PermMessagePost)) {
		t.Errorf("did NOT expect message.post in inheritedAllows once denied at group scope; got %v", everyone.allows)
	}
}

// TestTierRoles_GroupScopeShowsServerInheritance verifies the matrix-display
// fix: at the group tier, a role's server-scope state shows through as
// inherited state for every perm configurable at both server and group scope.
// Without this, the group editor showed empty inheritance and the user
// couldn't tell what defaults were already in effect from the server tier.
func TestTierRoles_GroupScopeShowsServerInheritance(t *testing.T) {
	env := setupTestResolver(t)
	query := env.resolver.Query()

	// Seed a deny on admin at server scope for room.create — pinning the
	// inheritedDenials path. Also rely on the default everyone allow for
	// message.post (seeded at server scope) for the inheritedAllows path.
	if err := env.core.DenyInstancePermission(env.ctx, core.RoleAdmin, core.PermRoomCreate); err != nil {
		t.Fatalf("DenyInstancePermission: %v", err)
	}

	groups, err := env.core.ListRoomGroupsOrdered(env.ctx, core.KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered: %v", err)
	}
	if len(groups) == 0 {
		t.Fatal("expected at least one seeded group")
	}
	groupID := groups[0].Id

	got, err := query.TierRoles(env.authContext(), nil, &groupID)
	if err != nil {
		t.Fatalf("TierRoles: %v", err)
	}
	if got == nil || len(got.Roles) == 0 {
		t.Fatal("expected non-empty role matrix at group scope")
	}

	findRole := func(name string) *struct{ allows, denies []string } {
		for _, r := range got.Roles {
			if r.RoleName == name {
				return &struct{ allows, denies []string }{r.InheritedAllows, r.InheritedDenials}
			}
		}
		return nil
	}

	admin := findRole(core.RoleAdmin)
	if admin == nil {
		t.Fatal("expected admin role in group-scope matrix")
	}
	if !slices.Contains(admin.denies, string(core.PermRoomCreate)) {
		t.Errorf("expected room.create in admin's inheritedDenials at group scope; got %v", admin.denies)
	}

	everyone := findRole(core.RoleEveryone)
	if everyone == nil {
		t.Fatal("expected everyone role in group-scope matrix")
	}
	if !slices.Contains(everyone.allows, string(core.PermMessagePost)) {
		t.Errorf("expected message.post (default everyone allow at server) in inheritedAllows at group scope; got %v", everyone.allows)
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

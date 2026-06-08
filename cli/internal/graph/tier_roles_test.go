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
	rbac := env.resolver.RbacQueries()

	got, err := rbac.RolePermissionTierMatrix(env.authContext(), nil, nil, nil)
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
	rbac := env.resolver.RbacQueries()

	if err := env.core.GrantServerPermission(env.ctx, core.RoleAdmin, core.PermMessagePost); err != nil {
		t.Fatalf("seed server grant: %v", err)
	}

	got, err := rbac.RolePermissionTierMatrix(env.authContext(), nil, &env.testRoom.Id, nil)
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

// TestTierRoles_ServerScopeAuthorization verifies the server-scope auth gate.
func TestTierRoles_ServerScopeAuthorization(t *testing.T) {
	env := setupTestResolver(t)
	rbac := env.resolver.RbacQueries()

	t.Run("regular member without role.manage is rejected", func(t *testing.T) {
		regular := env.createVerifiedUser(t, "regular-tr", "Regular", "password123")
		_, err := rbac.RolePermissionTierMatrix(env.authContextForUser(regular), nil, nil, nil)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied at server scope, got %v", err)
		}
	})

	t.Run("delegated role manager can inspect server scope", func(t *testing.T) {
		manager := env.createVerifiedUser(t, "role-manager-tr", "Role Manager", "password123")
		if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, manager.Id, core.RoleModerator); err != nil {
			t.Fatalf("AssignServerRole: %v", err)
		}
		if err := env.core.GrantServerPermission(env.ctx, core.RoleModerator, core.PermRoleManage); err != nil {
			t.Fatalf("GrantServerPermission role.manage: %v", err)
		}
		got, err := rbac.RolePermissionTierMatrix(env.authContextForUser(manager), nil, nil, nil)
		if err != nil {
			t.Fatalf("expected delegated role manager to inspect server scope, got %v", err)
		}
		if got == nil || len(got.Roles) == 0 {
			t.Fatal("expected non-empty server-scope role matrix")
		}
	})
}

func TestTierRoles_RoomManagerCanInspectTheirRoom(t *testing.T) {
	env := setupTestResolver(t)
	rbac := env.resolver.RbacQueries()

	manager := env.createVerifiedUser(t, "room-manager-tr", "Room Manager", "password123")
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, manager.Id, core.RoleModerator); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	if err := env.core.GrantRoomPermission(env.ctx, env.testRoom.Id, core.RoleModerator, core.PermRoomManage); err != nil {
		t.Fatalf("GrantRoomPermission room.manage: %v", err)
	}

	got, err := rbac.RolePermissionTierMatrix(env.authContextForUser(manager), nil, &env.testRoom.Id, nil)
	if err != nil {
		t.Fatalf("expected room manager to inspect their room matrix, got %v", err)
	}
	if got == nil || len(got.Roles) == 0 {
		t.Fatal("expected non-empty room-scope role matrix")
	}
}

// TestTierRoles_RoomOverridesMatchCoreState verifies the matrix override
// column reflects the per-room grants and denials persisted in core.
func TestTierRoles_RoomOverridesMatchCoreState(t *testing.T) {
	env := setupTestResolver(t)
	rbac := env.resolver.RbacQueries()

	if err := env.core.GrantRoomPermission(env.ctx, env.testRoom.Id, core.RoleAdmin, core.PermRoomManage); err != nil {
		t.Fatalf("seed room grant: %v", err)
	}
	if err := env.core.DenyRoomPermission(env.ctx, env.testRoom.Id, core.RoleAdmin, core.PermMessagePost); err != nil {
		t.Fatalf("seed room deny: %v", err)
	}

	matrix, err := rbac.RolePermissionTierMatrix(env.authContext(), nil, &env.testRoom.Id, nil)
	if err != nil {
		t.Fatalf("TierRoles: %v", err)
	}

	var adminRole *struct {
		permissions       []string
		permissionDenials []string
	}
	for _, tr := range matrix.Roles {
		if tr.RoleName == core.RoleAdmin {
			adminRole = &struct {
				permissions       []string
				permissionDenials []string
			}{tr.Override.Permissions, tr.Override.PermissionDenials}
			break
		}
	}
	if adminRole == nil {
		t.Fatal("expected admin role in room-scope matrix")
	}
	assertSameStringSet(t, "admin room permissions", adminRole.permissions, []string{string(core.PermRoomManage)})
	assertSameStringSet(t, "admin room denials", adminRole.permissionDenials, []string{string(core.PermMessagePost)})
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
	rbac := env.resolver.RbacQueries()

	// message.post is granted at server scope by default (everyone). Add an
	// explicit deny at the room's group on everyone — the matrix's room-scope
	// inheritance baseline should now report DENY, with no parallel ALLOW.
	groupID := env.testRoom.GroupId
	if err := env.core.DenyGroupPermission(env.ctx, groupID, core.RoleEveryone, core.PermMessagePost); err != nil {
		t.Fatalf("DenyGroupPermission: %v", err)
	}

	got, err := rbac.RolePermissionTierMatrix(env.authContext(), nil, &env.testRoom.Id, nil)
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
	rbac := env.resolver.RbacQueries()

	// Seed a deny on admin at server scope for room.create — pinning the
	// inheritedDenials path. Also rely on the default everyone allow for
	// message.post (seeded at server scope) for the inheritedAllows path.
	if err := env.core.DenyServerPermission(env.ctx, core.RoleAdmin, core.PermRoomCreate); err != nil {
		t.Fatalf("DenyServerPermission: %v", err)
	}

	groups, err := env.core.ListRoomGroupsOrdered(env.ctx, core.KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered: %v", err)
	}
	if len(groups) == 0 {
		t.Fatal("expected at least one seeded group")
	}
	groupID := groups[0].Id

	got, err := rbac.RolePermissionTierMatrix(env.authContext(), nil, nil, &groupID)
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

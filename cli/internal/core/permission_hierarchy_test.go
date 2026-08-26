package core

import "testing"

func TestPermissionCatalogRegistersAncestors(t *testing.T) {
	for _, meta := range AllPermissions() {
		for _, ancestor := range PermissionAncestors(meta.Permission) {
			if _, registered := GetPermissionMetadata(ancestor); !registered {
				t.Errorf("permission %q has unregistered ancestor %q", meta.Permission, ancestor)
			}
		}
	}
	if err := ValidatePermission(PermMessage); err != nil {
		t.Fatalf("ValidatePermission(message): %v", err)
	}
}

func TestPermissionHierarchy_AncestorGrantAndExactDeny(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, _ := core.CreateUser(ctx, SystemActorID, "hierarchy-user", "Hierarchy User", "password123")

	if err := core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermAdmin); err != nil {
		t.Fatalf("grant admin ancestor: %v", err)
	}
	for _, perm := range []Permission{PermAdminUsersView, PermAdminAuditView} {
		has, err := core.HasServerPermission(ctx, user.Id, perm)
		if err != nil || !has {
			t.Fatalf("ancestor grant has %s = %v, %v; want true, nil", perm, has, err)
		}
	}
	if err := core.DenyServerPermission(ctx, SystemActorID, RoleEveryone, PermAdminUsersView); err != nil {
		t.Fatalf("deny exact descendant: %v", err)
	}
	has, err := core.HasServerPermission(ctx, user.Id, PermAdminUsersView)
	if err != nil || has {
		t.Fatalf("exact deny has admin.view-users = %v, %v; want false, nil", has, err)
	}
	has, err = core.HasServerPermission(ctx, user.Id, PermAdminAuditView)
	if err != nil || !has {
		t.Fatalf("exact deny propagated to sibling: has admin.view-audit = %v, %v", has, err)
	}
}

func TestPermissionHierarchy_ExactPathResolvesBeforeAncestorSubjects(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, _ := core.CreateUser(ctx, SystemActorID, "hierarchy-exact", "Hierarchy Exact", "password123")
	if _, err := core.CreateServerRole(ctx, SystemActorID, "hierarchyparent", "Hierarchy parent", ""); err != nil {
		t.Fatalf("CreateServerRole: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, user.Id, "hierarchyparent"); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	if err := core.GrantServerPermission(ctx, SystemActorID, "hierarchyparent", PermMessage); err != nil {
		t.Fatalf("grant named ancestor: %v", err)
	}
	if err := core.DenyServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost); err != nil {
		t.Fatalf("deny everyone descendant: %v", err)
	}
	has, err := core.HasServerPermission(ctx, user.Id, PermMessagePost)
	if err != nil || has {
		t.Fatalf("exact everyone denial has message.post = %v, %v; want false, nil", has, err)
	}
}

func TestPermissionHierarchy_ScopeAndNamedRoleResolution(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, _ := core.CreateUser(ctx, SystemActorID, "hierarchy-scoped", "Hierarchy Scoped", "password123")
	room, _ := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "hierarchy-scope", "Hierarchy scope")
	group, err := core.CreateRoomGroup(ctx, SystemActorID, "Hierarchy group", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	if err := core.MoveRoomToGroup(ctx, SystemActorID, room.Id, group.Id); err != nil {
		t.Fatalf("MoveRoomToGroup: %v", err)
	}
	if err := core.GrantGroupPermission(ctx, SystemActorID, group.Id, RoleEveryone, PermMessage); err != nil {
		t.Fatalf("grant group ancestor: %v", err)
	}
	if err := core.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessage); err != nil {
		t.Fatalf("deny room ancestor: %v", err)
	}
	has, err := core.PermResolver().HasRoomPermission(ctx, user.Id, KindChannel, room.Id, PermMessagePost)
	if err != nil || !has {
		t.Fatalf("ancestor denial propagated: has message.post = %v, %v", has, err)
	}

	if _, err := core.CreateServerRole(ctx, SystemActorID, "hierarchydeny", "Hierarchy deny", ""); err != nil {
		t.Fatalf("CreateServerRole: %v", err)
	}
	if err := core.AssignServerRole(ctx, SystemActorID, user.Id, "hierarchydeny"); err != nil {
		t.Fatalf("AssignServerRole: %v", err)
	}
	if err := core.DenyRoomPermission(ctx, SystemActorID, room.Id, "hierarchydeny", PermMessagePost); err != nil {
		t.Fatalf("named exact deny: %v", err)
	}
	has, err = core.PermResolver().HasRoomPermission(ctx, user.Id, KindChannel, room.Id, PermMessagePost)
	if err != nil || has {
		t.Fatalf("named exact denial has message.post = %v, %v; want false, nil", has, err)
	}
}

func TestPermissionHierarchy_BotAndDelegatedRoleCeilings(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, _ := core.CreateUser(ctx, SystemActorID, "hierarchy-owner", "Hierarchy Owner", "password123")
	if err := core.GrantUserPermission(ctx, SystemActorID, owner.Id, PermMessage); err != nil {
		t.Fatalf("grant owner ancestor: %v", err)
	}
	bot, err := core.CreateBot(ctx, owner.Id, "hierarchy_bot", "Hierarchy bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	if err := core.GrantUserPermission(ctx, SystemActorID, bot.User.Id, PermMessage); err != nil {
		t.Fatalf("grant bot ancestor: %v", err)
	}
	has, err := core.HasServerPermission(ctx, bot.User.Id, PermMessagePost)
	if err != nil || !has {
		t.Fatalf("bot ancestor attenuation has message.post = %v, %v", has, err)
	}
	if err := core.DenyUserPermission(ctx, SystemActorID, bot.User.Id, PermMessagePost); err != nil {
		t.Fatalf("deny bot descendant: %v", err)
	}
	has, err = core.HasServerPermission(ctx, bot.User.Id, PermMessagePost)
	if err != nil || has {
		t.Fatalf("bot exact deny has message.post = %v, %v; want false, nil", has, err)
	}

	manager, _ := core.CreateUser(ctx, SystemActorID, "hierarchy-manager", "Hierarchy Manager", "password123")
	if err := core.GrantUserPermission(ctx, SystemActorID, manager.Id, PermRoleAssign); err != nil {
		t.Fatalf("grant role.assign: %v", err)
	}
	if err := core.GrantUserPermission(ctx, SystemActorID, manager.Id, PermMessage); err != nil {
		t.Fatalf("grant manager ancestor: %v", err)
	}
	if _, err := core.CreateServerRole(ctx, SystemActorID, "hierarchytarget", "Hierarchy target", ""); err != nil {
		t.Fatalf("CreateServerRole: %v", err)
	}
	if err := core.GrantServerPermission(ctx, SystemActorID, "hierarchytarget", PermMessagePost); err != nil {
		t.Fatalf("grant target leaf: %v", err)
	}
	canAssign, err := core.CanAssignRole(ctx, manager.Id, "hierarchytarget")
	if err != nil || !canAssign {
		t.Fatalf("ancestor ceiling CanAssignRole = %v, %v; want true, nil", canAssign, err)
	}
}

func TestPermissionHierarchy_ExplanationAndRoleMatrix(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, _ := core.CreateUser(ctx, SystemActorID, "hierarchy-explain", "Hierarchy Explain", "password123")
	if err := core.GrantUserPermission(ctx, SystemActorID, user.Id, PermRoleManage); err != nil {
		t.Fatalf("grant role.manage: %v", err)
	}
	if err := core.GrantServerPermission(ctx, SystemActorID, RoleEveryone, PermAdmin); err != nil {
		t.Fatalf("grant ancestor: %v", err)
	}
	explanation, err := core.PermResolver().ExplainServerPermission(ctx, user.Id, PermAdminUsersView)
	if err != nil {
		t.Fatalf("ExplainServerPermission: %v", err)
	}
	if len(explanation.Trace) != 1 || explanation.Trace[0].Permission != PermAdmin {
		t.Fatalf("trace = %+v, want one admin ancestor entry", explanation.Trace)
	}
	matrix, err := core.GetRolePermissionMatrix(ctx, user.Id, RoleEveryone)
	if err != nil {
		t.Fatalf("GetRolePermissionMatrix: %v", err)
	}
	for _, cell := range matrix.Cells {
		if cell.ScopeID == "server" && cell.Permission == string(PermAdminUsersView) {
			if cell.Effective != MatrixDecisionAllow {
				t.Fatalf("matrix effective = %s, want allow", cell.Effective)
			}
			return
		}
	}
	t.Fatal("admin.view-users server matrix cell not found")
}

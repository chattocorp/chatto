package connectapi

import (
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestPermissionScopePagesAndInheritance(t *testing.T) {
	env := newConnectAPITestEnv(t)
	for _, perm := range []core.Permission{core.PermRoleManage, core.PermUserManagePermissions} {
		if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, perm); err != nil {
			t.Fatal(err)
		}
	}
	ctx := withCaller(env.ctx, env.viewer)
	groupID := env.defaultRoomGroupID(t)
	var roomID string
	for i := 0; i < 24; i++ {
		room, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, groupID, fmt.Sprintf("scope-page-%d", i), "Scope page")
		if err != nil {
			t.Fatal(err)
		}
		roomID = room.Id
	}
	_, err := env.permissions.SetRolePermission(ctx, connect.NewRequest(&adminv1.SetRolePermissionRequest{
		RoleName: core.RoleModerator, Permission: string(core.PermMessagePost), Decision: adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW,
		Scope: &adminv1.PermissionScope{Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_GROUP, Id: groupID},
	}))
	if err != nil {
		t.Fatal(err)
	}
	first, err := env.permissions.GetRolePermissionMatrix(ctx, connect.NewRequest(&adminv1.GetRolePermissionMatrixRequest{RoleName: core.RoleModerator}))
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Msg.Matrix.Scopes) != 20 || !first.Msg.Page.HasMore {
		t.Fatalf("first scope count=%d page=%v", len(first.Msg.Matrix.Scopes), first.Msg.Page)
	}
	second, err := env.permissions.ListRolePermissionDecisions(ctx, connect.NewRequest(&adminv1.ListRolePermissionDecisionsRequest{RoleName: core.RoleModerator, Page: &apiv1.PageRequest{Offset: 20}}))
	if err != nil {
		t.Fatal(err)
	}
	if int(first.Msg.Page.TotalCount) != 20+len(second.Msg.Scopes) || second.Msg.Page.HasMore {
		t.Fatal("scope page counts disagree")
	}
	target := &adminv1.PermissionScope{Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_ROOM, Id: roomID}
	exact, err := env.permissions.GetRolePermissionMatrix(ctx, connect.NewRequest(&adminv1.GetRolePermissionMatrixRequest{RoleName: core.RoleModerator, Scope: target}))
	if err != nil {
		t.Fatal(err)
	}
	if len(exact.Msg.Matrix.Scopes) != 1 || exact.Msg.Page.TotalCount != 1 {
		t.Fatal("exact filter did not restrict scopes")
	}
	cell := findAPIPermissionCell(exact.Msg.Matrix.Cells, "room:"+roomID, string(core.PermMessagePost))
	if cell == nil || cell.Effective != adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW || cell.Override != adminv1.PermissionDecision_PERMISSION_DECISION_NONE {
		t.Fatalf("parent outside page lost: %v", cell)
	}
	users, err := env.permissions.ListUserPermissionDecisions(ctx, connect.NewRequest(&adminv1.ListUserPermissionDecisionsRequest{UserId: env.viewer.Id, Scope: target}))
	if err != nil {
		t.Fatal(err)
	}
	if len(users.Msg.Scopes) != 1 || users.Msg.Page.TotalCount != 1 {
		t.Fatal("user decisions did not filter scopes")
	}
	matrix, err := env.permissions.GetUserPermissionMatrix(ctx, connect.NewRequest(&adminv1.GetUserPermissionMatrixRequest{UserId: env.viewer.Id, Page: &apiv1.PageRequest{Limit: 1, Offset: 1}}))
	if err != nil {
		t.Fatal(err)
	}
	if len(matrix.Msg.Matrix.Scopes) != 1 || !matrix.Msg.Page.HasMore {
		t.Fatal("user matrix did not page")
	}
	dm, err := env.permissions.ListRolePermissionDecisions(ctx, connect.NewRequest(&adminv1.ListRolePermissionDecisionsRequest{RoleName: core.RoleModerator, Scope: &adminv1.PermissionScope{Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_DM}}))
	if err != nil || len(dm.Msg.Scopes) != 1 || dm.Msg.Scopes[0].Kind != adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_DM {
		t.Fatalf("explicit DM: %v", err)
	}
	for _, scope := range []*adminv1.PermissionScope{{}, {Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_ROOM}, {Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER, Id: "bad"}} {
		_, err := env.permissions.GetRolePermissionMatrix(ctx, connect.NewRequest(&adminv1.GetRolePermissionMatrixRequest{RoleName: core.RoleModerator, Scope: scope}))
		if connect.CodeOf(err) != connect.CodeInvalidArgument {
			t.Fatalf("invalid scope code=%v", connect.CodeOf(err))
		}
	}
	_, err = env.permissions.ListRolePermissionDecisions(env.ctx, connect.NewRequest(&adminv1.ListRolePermissionDecisionsRequest{RoleName: "missing"}))
	if connect.CodeOf(err) != connect.CodeUnauthenticated {
		t.Fatal("anonymous read accepted")
	}
	if err := env.core.ClearUserPermissionState(env.ctx, core.SystemActorID, env.viewer.Id, core.PermRoleManage); err != nil {
		t.Fatal(err)
	}
	_, err = env.permissions.ListRolePermissionDecisions(ctx, connect.NewRequest(&adminv1.ListRolePermissionDecisionsRequest{RoleName: "missing", Page: &apiv1.PageRequest{Offset: 20}}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("revoked reader code=%v", connect.CodeOf(err))
	}
}

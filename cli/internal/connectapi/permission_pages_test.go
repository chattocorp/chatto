package connectapi

import (
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestPermissionMatricesExcludeArchivedChannels(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	for _, permission := range []core.Permission{core.PermRoleManage, core.PermUserManagePermissions} {
		require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, permission))
	}
	service := &botService{api: env.api}
	bot, err := service.CreateBot(ctx, connect.NewRequest(&apiv1.CreateBotRequest{Login: "archive_bot", DisplayName: "Archive Bot"}))
	require.NoError(t, err)
	group, err := env.core.CreateRoomGroup(env.ctx, core.SystemActorID, "Archive matrix", "")
	require.NoError(t, err)
	archived, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, group.Id, "archive-matrix-room", "")
	require.NoError(t, err)
	active, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, env.defaultRoomGroupID(t), "active-matrix-room", "")
	require.NoError(t, err)
	target := &adminv1.PermissionScope{Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_ROOM, Id: archived.Id}

	// Both response matrix types expose the same scope and cell collections.
	type matrixView interface {
		GetScopes() []*adminv1.PermissionMatrixScope
		GetCells() []*adminv1.PermissionMatrixCell
	}
	for _, subject := range []struct{ name, userID string }{
		{"role", ""}, {"human", env.viewer.Id}, {"bot", bot.Msg.Bot.User.Id},
	} {
		t.Run(subject.name, func(t *testing.T) {
			fetch := func(page *apiv1.PageRequest, scope *adminv1.PermissionScope) (matrixView, *apiv1.PageInfo) {
				t.Helper()
				if subject.name == "role" {
					res, err := env.permissions.GetRolePermissionMatrix(ctx, connect.NewRequest(&adminv1.GetRolePermissionMatrixRequest{
						RoleName: core.RoleModerator, IncludeDirectMessageScope: true, Page: page, Scope: scope,
					}))
					require.NoError(t, err)
					return res.Msg.Matrix, res.Msg.Page
				}
				res, err := env.permissions.GetUserPermissionMatrix(ctx, connect.NewRequest(&adminv1.GetUserPermissionMatrixRequest{
					UserId: subject.userID, IncludeDirectMessageScope: true, Page: page, Scope: scope,
				}))
				require.NoError(t, err)
				return res.Msg.Matrix, res.Msg.Page
			}
			if subject.name == "role" {
				_, err := env.permissions.SetRolePermission(ctx, connect.NewRequest(&adminv1.SetRolePermissionRequest{
					RoleName: core.RoleModerator, Scope: target, Permission: string(core.PermMessagePost),
					Decision: adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW,
				}))
				require.NoError(t, err)
			} else {
				_, err := env.permissions.SetUserPermission(ctx, connect.NewRequest(&adminv1.SetUserPermissionRequest{
					UserId: subject.userID, Scope: target, Permission: string(core.PermMessagePost),
					Decision: adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW,
				}))
				require.NoError(t, err)
			}
			before, beforePage := fetch(&apiv1.PageRequest{Limit: 100}, nil)
			scopeIDs := func(matrix matrixView) []string {
				ids := make([]string, 0, len(matrix.GetScopes()))
				for _, scope := range matrix.GetScopes() {
					ids = append(ids, scope.Id)
				}
				return ids
			}
			require.Contains(t, scopeIDs(before), "room:"+archived.Id)
			_, err := env.core.ArchiveRoom(env.ctx, core.SystemActorID, core.KindChannel, archived.Id)
			require.NoError(t, err)
			after, afterPage := fetch(&apiv1.PageRequest{Limit: 100}, nil)
			ids := scopeIDs(after)
			require.NotContains(t, ids, "room:"+archived.Id)
			for _, id := range []string{"server", "dm", "group:" + group.Id, "room:" + active.Id} {
				require.Contains(t, ids, id)
			}
			for _, cell := range after.GetCells() {
				require.NotEqual(t, "room:"+archived.Id, cell.ScopeId)
			}
			require.Equal(t, beforePage.TotalCount-1, afterPage.TotalCount)
			require.False(t, afterPage.HasMore)
			for offset, id := range ids {
				matrix, page := fetch(&apiv1.PageRequest{Limit: 1, Offset: int32(offset)}, nil)
				require.Equal(t, []string{id}, scopeIDs(matrix))
				require.Equal(t, afterPage.TotalCount, page.TotalCount)
				require.Equal(t, offset < len(ids)-1, page.HasMore)
			}
			end, endPage := fetch(&apiv1.PageRequest{Limit: 1, Offset: int32(len(ids))}, nil)
			require.Empty(t, end.GetScopes())
			require.False(t, endPage.HasMore)
			exact, exactPage := fetch(nil, target)
			require.Empty(t, exact.GetScopes())
			require.Empty(t, exact.GetCells())
			require.Zero(t, exactPage.TotalCount)
			require.False(t, exactPage.HasMore)

			_, err = env.core.UnarchiveRoom(env.ctx, core.SystemActorID, core.KindChannel, archived.Id)
			require.NoError(t, err)
			restored, restoredPage := fetch(nil, target)
			require.Equal(t, []string{"room:" + archived.Id}, scopeIDs(restored))
			require.EqualValues(t, 1, restoredPage.TotalCount)
			cell := findAPIPermissionCell(restored.GetCells(), "room:"+archived.Id, string(core.PermMessagePost))
			require.NotNil(t, cell)
			require.Equal(t, adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW, cell.Override)
		})
	}
}

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

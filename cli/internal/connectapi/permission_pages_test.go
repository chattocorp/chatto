package connectapi

import (
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/authctx"
	"github.com/chattocorp/chatto/cli/internal/core"
	adminv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/admin/v1"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"github.com/stretchr/testify/require"
)

func TestPermissionExplanationReportsPrivilegedModeGate(t *testing.T) {
	t.Parallel()
	env := newConnectAPITestEnv(t)
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermRoleManage))
	target, err := env.core.CreateUser(env.ctx, core.SystemActorID, "explanation-target", "Explanation Target", "password123")
	require.NoError(t, err)
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, target.Id, core.PermServerManage))
	ctx := authctx.WithCredential(withCaller(env.ctx, env.viewer), authctx.RuntimeCredential{
		Kind: authctx.RuntimeCredentialKindBearerToken, UserID: env.viewer.Id,
		Handle: "inspector-session", PrivilegedModeExpiresAt: time.Now().Add(time.Hour),
	})
	response, err := env.permissions.ExplainPermissions(ctx, connect.NewRequest(&adminv1.ExplainPermissionsRequest{UserId: target.Id}))
	require.NoError(t, err)
	found := false
	for _, explanation := range response.Msg.Explanations {
		if explanation.Permission != string(core.PermServerManage) {
			continue
		}
		found = true
		require.Equal(t, adminv1.PermissionDecision_PERMISSION_DECISION_DENY, explanation.State)
		require.Equal(t, "@privileged-mode", explanation.DecidedByRole)
		require.Len(t, explanation.Trace, 2)
		require.True(t, explanation.Trace[0].Applied)
		require.Equal(t, "@privileged-mode", explanation.Trace[0].RoleName)
		require.False(t, explanation.Trace[1].Applied)
		require.Equal(t, adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW, explanation.Trace[1].Decision)
	}
	require.True(t, found, "server.manage explanation missing")
}

func TestPermissionMatricesExcludeArchivedChannels(t *testing.T) {
	t.Parallel()

	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	for _, permission := range []core.Permission{core.PermRoleManage, core.PermUserManagePermissions} {
		require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, permission))
	}
	service := &botService{api: env.api}
	allowBotCreation(t, env.ctx, env.core, env.viewer.Id)
	bot, err := service.CreateBot(ctx, connect.NewRequest(&apiv1.CreateBotRequest{Login: "archive_bot", DisplayName: "Archive Bot"}))
	require.NoError(t, err)
	group, err := env.core.RoomLayout().CreateRoomGroup(env.ctx, core.SystemActorID, "Archive matrix", "")
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
			fetch := func(scope *adminv1.PermissionScope) matrixView {
				t.Helper()
				if subject.name == "role" {
					res, err := env.permissions.GetRolePermissionMatrix(ctx, connect.NewRequest(&adminv1.GetRolePermissionMatrixRequest{
						RoleName: core.RoleModerator, IncludeDirectMessageScope: true, Scope: scope,
					}))
					require.NoError(t, err)
					return res.Msg.Matrix
				}
				res, err := env.permissions.GetUserPermissionMatrix(ctx, connect.NewRequest(&adminv1.GetUserPermissionMatrixRequest{
					UserId: subject.userID, IncludeDirectMessageScope: true, Scope: scope,
				}))
				require.NoError(t, err)
				return res.Msg.Matrix
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
			before := fetch(nil)
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
			after := fetch(nil)
			ids := scopeIDs(after)
			require.NotContains(t, ids, "room:"+archived.Id)
			for _, id := range []string{"server", "dm", "group:" + group.Id, "room:" + active.Id} {
				require.Contains(t, ids, id)
			}
			for _, cell := range after.GetCells() {
				require.NotEqual(t, "room:"+archived.Id, cell.ScopeId)
			}
			require.Len(t, after.GetScopes(), len(before.GetScopes())-1)
			exact := fetch(target)
			require.Empty(t, exact.GetScopes())
			require.Empty(t, exact.GetCells())

			_, err = env.core.UnarchiveRoom(env.ctx, core.SystemActorID, core.KindChannel, archived.Id)
			require.NoError(t, err)
			restored := fetch(target)
			require.Equal(t, []string{"room:" + archived.Id}, scopeIDs(restored))
			cell := findAPIPermissionCell(restored.GetCells(), "room:"+archived.Id, string(core.PermMessagePost))
			require.NotNil(t, cell)
			require.Equal(t, adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW, cell.Override)
		})
	}
}

func TestPermissionScopePagesAndInheritance(t *testing.T) {
	t.Parallel()

	env := newConnectAPITestEnv(t)
	for _, perm := range []core.Permission{core.PermRoleManage, core.PermUserManagePermissions} {
		if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, perm); err != nil {
			t.Fatal(err)
		}
	}
	ctx := withCaller(env.ctx, env.viewer)
	groupID := env.defaultRoomGroupID(t)
	var roomID string
	for i := range 104 {
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
	require.GreaterOrEqual(t, len(first.Msg.Matrix.Scopes), 104)
	second, err := env.permissions.ListRolePermissionDecisions(ctx, connect.NewRequest(&adminv1.ListRolePermissionDecisionsRequest{RoleName: core.RoleModerator, Page: &apiv1.PageRequest{Offset: 100}}))
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Msg.Matrix.Scopes) != 100+len(second.Msg.Scopes) || second.Msg.Page.HasMore {
		t.Fatal("scope page counts disagree")
	}
	require.EqualValues(t, len(first.Msg.Matrix.Scopes), second.Msg.Page.TotalCount)
	target := &adminv1.PermissionScope{Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_ROOM, Id: roomID}
	exact, err := env.permissions.GetRolePermissionMatrix(ctx, connect.NewRequest(&adminv1.GetRolePermissionMatrixRequest{RoleName: core.RoleModerator, Scope: target}))
	if err != nil {
		t.Fatal(err)
	}
	if len(exact.Msg.Matrix.Scopes) != 1 {
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
	matrix, err := env.permissions.GetUserPermissionMatrix(ctx, connect.NewRequest(&adminv1.GetUserPermissionMatrixRequest{UserId: env.viewer.Id}))
	if err != nil {
		t.Fatal(err)
	}
	require.Len(t, matrix.Msg.Matrix.Scopes, len(first.Msg.Matrix.Scopes))
	dm, err := env.permissions.ListRolePermissionDecisions(ctx, connect.NewRequest(&adminv1.ListRolePermissionDecisionsRequest{RoleName: core.RoleModerator, Scope: &adminv1.PermissionScope{Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_DM}}))
	if err != nil || len(dm.Msg.Scopes) != 1 || dm.Msg.Scopes[0].Kind != adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_DM {
		t.Fatalf("explicit DM: %v", err)
	}
	for _, scope := range []*adminv1.PermissionScope{{}, {Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_ROOM}, {Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER, Id: "bad"}} {
		_, err := env.permissions.GetRolePermissionMatrix(ctx, connect.NewRequest(&adminv1.GetRolePermissionMatrixRequest{RoleName: core.RoleModerator, Scope: scope}))
		if errorCode(err) != connect.CodeInvalidArgument {
			t.Fatalf("invalid scope code=%v", errorCode(err))
		}
	}
	_, err = env.permissions.ListRolePermissionDecisions(env.ctx, connect.NewRequest(&adminv1.ListRolePermissionDecisionsRequest{RoleName: "missing"}))
	if errorCode(err) != connect.CodeUnauthenticated {
		t.Fatal("anonymous read accepted")
	}
	if err := env.core.ClearUserPermissionState(env.ctx, core.SystemActorID, env.viewer.Id, core.PermRoleManage); err != nil {
		t.Fatal(err)
	}
	_, err = env.permissions.ListRolePermissionDecisions(ctx, connect.NewRequest(&adminv1.ListRolePermissionDecisionsRequest{RoleName: "missing", Page: &apiv1.PageRequest{Offset: 20}}))
	if errorCode(err) != connect.CodePermissionDenied {
		t.Fatalf("revoked reader code=%v", errorCode(err))
	}
}

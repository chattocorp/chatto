package connectapi

import (
	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	"testing"
)

func TestBotPermissionSummaryUsesExistingMatrixBoundary(t *testing.T) {
	env := newConnectAPITestEnv(t)
	service := env.permissions
	bot, err := env.core.CreateBot(env.ctx, env.viewer.Id, "public_bot", "Public Bot")
	require.NoError(t, err)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "reader", "Reader", "password123")
	require.NoError(t, err)
	require.NoError(t, env.core.GrantUserPermission(env.ctx, env.viewer.Id, bot.User.Id, core.PermMessageRead))
	require.NoError(t, env.core.GrantUserPermission(env.ctx, env.viewer.Id, bot.User.Id, core.PermMessagePost))
	request := &adminv1.GetUserPermissionMatrixRequest{UserId: bot.User.Id, Summary: true, IncludeDirectMessageScope: true, Page: &apiv1.PageRequest{Limit: 1}}
	_, err = service.GetUserPermissionMatrix(env.ctx, connect.NewRequest(request))
	require.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	ctx := withCaller(env.ctx, viewer)
	response, err := service.GetUserPermissionMatrix(ctx, connect.NewRequest(request))
	require.NoError(t, err)
	require.Len(t, response.Msg.Matrix.Scopes, 1)
	require.Greater(t, len(response.Msg.Matrix.Cells), 1) // Offsets count scopes, not cells.
	require.Equal(t, int64(2), response.Msg.Page.TotalCount)
	require.True(t, response.Msg.Page.HasMore)
	require.Equal(t, adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER, response.Msg.Matrix.Scopes[0].Kind)
	for _, cell := range response.Msg.Matrix.Cells {
		require.Equal(t, adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW, cell.Effective)
		require.Equal(t, adminv1.PermissionDecision_PERMISSION_DECISION_UNSPECIFIED, cell.Override)
		require.Nil(t, cell.AllowPermitted)
	}
	request.Page.Offset = 1
	response, err = service.GetUserPermissionMatrix(ctx, connect.NewRequest(request))
	require.NoError(t, err)
	require.False(t, response.Msg.Page.HasMore)
	require.Equal(t, adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_DM, response.Msg.Matrix.Scopes[0].Kind)
	request.Summary = false
	_, err = service.GetUserPermissionMatrix(ctx, connect.NewRequest(request))
	require.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	_, err = service.ListUserPermissionDecisions(ctx, connect.NewRequest(&adminv1.ListUserPermissionDecisionsRequest{UserId: bot.User.Id}))
	require.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	// Public summary access never grants editing rights.
	_, err = service.SetUserPermission(ctx, connect.NewRequest(&adminv1.SetUserPermissionRequest{UserId: bot.User.Id, Permission: string(core.PermMessagePost), Decision: adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW}))
	require.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	for _, summary := range []bool{false, true} {
		request.UserId = env.viewer.Id
		request.Summary = summary
		_, err = service.GetUserPermissionMatrix(ctx, connect.NewRequest(request))
		require.Error(t, err)
		require.Contains(t, []connect.Code{connect.CodePermissionDenied, connect.CodeNotFound}, connect.CodeOf(err))
	}
	// A manager can still use the existing full editing matrix.
	request.UserId = bot.User.Id
	request.Summary = false
	request.Page = nil
	response, err = service.GetUserPermissionMatrix(withCaller(env.ctx, env.viewer), connect.NewRequest(request))
	require.NoError(t, err)
	require.NotNil(t, response.Msg.Matrix.Cells[0].AllowPermitted)
}

func TestBotPermissionSummaryFiltersRoomsAndInactiveGrants(t *testing.T) {
	env := newConnectAPITestEnv(t)
	bot, err := env.core.CreateBot(env.ctx, env.viewer.Id, "scoped_bot", "Scoped Bot")
	require.NoError(t, err)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "summary_reader", "Reader", "password123")
	require.NoError(t, err)
	room, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, "", "summary-secret", "")
	require.NoError(t, err)
	require.NoError(t, env.core.GrantUserPermission(env.ctx, env.viewer.Id, bot.User.Id, core.PermMessageRead))
	require.NoError(t, env.core.DenyUserRoomPermission(env.ctx, core.SystemActorID, room.Id, env.viewer.Id, core.PermMessageRead))
	require.NoError(t, env.core.DenyUserRoomPermission(env.ctx, core.SystemActorID, room.Id, viewer.Id, core.PermRoomList))
	request := &adminv1.GetUserPermissionMatrixRequest{UserId: bot.User.Id, Summary: true, IncludeDirectMessageScope: true, Page: &apiv1.PageRequest{Limit: 100}}
	response, err := env.permissions.GetUserPermissionMatrix(withCaller(env.ctx, viewer), connect.NewRequest(request))
	require.NoError(t, err)
	require.True(t, response.Msg.GetSummary())
	for _, scope := range response.Msg.Matrix.Scopes {
		require.NotEqual(t, "room:"+room.Id, scope.Id)
	}
	for _, cell := range response.Msg.Matrix.Cells {
		require.Equal(t, adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW, cell.Effective)
		require.NotEqual(t, "server", cell.ScopeId) // A hidden restriction prevents a broad claim.
	}
	request.Scope = &adminv1.PermissionScope{Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_ROOM, Id: room.Id}
	response, err = env.permissions.GetUserPermissionMatrix(withCaller(env.ctx, viewer), connect.NewRequest(request))
	require.NoError(t, err)
	require.Empty(t, response.Msg.Matrix.Scopes)
	require.Zero(t, response.Msg.Page.TotalCount)
	response, err = env.permissions.GetUserPermissionMatrix(withCaller(env.ctx, env.viewer), connect.NewRequest(request))
	require.NoError(t, err)
	require.Len(t, response.Msg.Matrix.Cells, 1)
	require.Equal(t, adminv1.PermissionDecision_PERMISSION_DECISION_NONE, response.Msg.Matrix.Cells[0].Effective)
	require.Equal(t, adminv1.PermissionDecision_PERMISSION_DECISION_UNSPECIFIED, response.Msg.Matrix.Cells[0].Override)
}

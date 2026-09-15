package connectapi

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestEffectivePermissionServiceBoundaryAndPaging(t *testing.T) {
	env := newConnectAPITestEnv(t)
	service := &effectivePermissionService{api: env.api}
	bot, err := env.core.CreateBot(env.ctx, env.viewer.Id, "public_bot", "Bot")
	require.NoError(t, err)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "reader", "Reader", "password123")
	require.NoError(t, err)
	require.NoError(t, env.core.GrantUserPermission(env.ctx, env.viewer.Id, bot.User.Id, core.PermMessageRead))
	req := &apiv1.ListEffectivePermissionsRequest{UserId: bot.User.Id, Page: &apiv1.PageRequest{Limit: 1}}
	_, err = service.ListEffectivePermissions(env.ctx, connect.NewRequest(req))
	require.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	ctx := withCaller(env.ctx, viewer)
	response, err := service.ListEffectivePermissions(ctx, connect.NewRequest(req))
	require.NoError(t, err)
	require.Len(t, response.Msg.Permissions, 1)
	require.True(t, response.Msg.Page.HasMore)
	first := response.Msg.Permissions[0]
	req.Page.Offset = 1
	response, err = service.ListEffectivePermissions(ctx, connect.NewRequest(req))
	require.NoError(t, err)
	require.Len(t, response.Msg.Permissions, 1)
	require.NotEqual(t, first, response.Msg.Permissions[0])
	_, err = env.permissions.GetUserPermissionMatrix(ctx, connect.NewRequest(&adminv1.GetUserPermissionMatrixRequest{UserId: bot.User.Id}))
	require.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	req.UserId = env.viewer.Id
	_, err = service.ListEffectivePermissions(ctx, connect.NewRequest(req))
	require.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, viewer.Id, core.PermUserManagePermissions))
	response, err = service.ListEffectivePermissions(ctx, connect.NewRequest(req))
	require.NoError(t, err)
	require.NotEmpty(t, response.Msg.Permissions)
	req.UserId = "missing"
	_, err = service.ListEffectivePermissions(ctx, connect.NewRequest(req))
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

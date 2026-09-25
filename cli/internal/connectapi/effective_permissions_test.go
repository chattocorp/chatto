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

func TestEffectivePermissionServiceBoundaryAndCompleteResult(t *testing.T) {
	env := newConnectAPITestEnv(t)
	service := &effectivePermissionService{api: env.api}
	bot, err := env.core.CreateBot(env.ctx, env.viewer.Id, "public_bot", "Bot")
	require.NoError(t, err)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "reader", "Reader", "password123")
	require.NoError(t, err)
	require.NoError(t, env.core.GrantUserPermission(env.ctx, env.viewer.Id, bot.User.Id, core.PermMessageRead))
	// Exceed the former maximum page size to catch silent truncation.
	for i := 0; i < 55; i++ {
		_, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, "", fmt.Sprintf("effective-%d", i), "")
		require.NoError(t, err)
	}
	req := &apiv1.ListEffectivePermissionsRequest{UserId: bot.User.Id}
	_, err = service.ListEffectivePermissions(env.ctx, connect.NewRequest(req))
	require.Equal(t, connect.CodeUnauthenticated, errorCode(err))
	ctx := withCaller(env.ctx, viewer)
	response, err := service.ListEffectivePermissions(ctx, connect.NewRequest(req))
	require.NoError(t, err)
	expected, err := env.core.ListEffectivePermissions(env.ctx, viewer.Id, bot.User.Id)
	require.NoError(t, err)
	require.Len(t, response.Msg.Permissions, len(expected))
	require.Greater(t, len(response.Msg.Permissions), 100)
	_, err = env.permissions.GetUserPermissionMatrix(ctx, connect.NewRequest(&adminv1.GetUserPermissionMatrixRequest{UserId: bot.User.Id}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	req.UserId = env.viewer.Id
	_, err = service.ListEffectivePermissions(ctx, connect.NewRequest(req))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, viewer.Id, core.PermUserManagePermissions))
	response, err = service.ListEffectivePermissions(ctx, connect.NewRequest(req))
	require.NoError(t, err)
	require.NotEmpty(t, response.Msg.Permissions)
	req.UserId = "missing"
	_, err = service.ListEffectivePermissions(ctx, connect.NewRequest(req))
	require.Equal(t, connect.CodeNotFound, errorCode(err))
}

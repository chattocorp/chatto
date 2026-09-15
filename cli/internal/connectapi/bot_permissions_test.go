package connectapi

import (
	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	"testing"
)

func TestBotPermissionListingPublicBoundaryAndPaging(t *testing.T) {
	env := newConnectAPITestEnv(t)
	service := &botService{api: env.api}
	bot, err := env.core.CreateBot(env.ctx, env.viewer.Id, "public_bot", "Public Bot")
	require.NoError(t, err)
	viewer, err := env.core.CreateUser(env.ctx, core.SystemActorID, "reader", "Reader", "password123")
	require.NoError(t, err)
	require.NoError(t, env.core.GrantUserPermission(env.ctx, env.viewer.Id, bot.User.Id, core.PermMessageRead))
	request := &apiv1.ListBotPermissionsRequest{BotUserId: bot.User.Id, Page: &apiv1.PageRequest{Limit: 1}}
	_, err = service.ListBotPermissions(env.ctx, connect.NewRequest(request))
	require.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
	ctx := withCaller(env.ctx, viewer)
	page, err := service.ListBotPermissions(ctx, connect.NewRequest(request))
	require.NoError(t, err)
	require.Len(t, page.Msg.Permissions, 1)
	require.Equal(t, int64(2), page.Msg.Page.TotalCount)
	require.True(t, page.Msg.Page.HasMore)
	require.True(t, page.Msg.Permissions[0].Active)
	require.Equal(t, apiv1.BotPermissionScope_BOT_PERMISSION_SCOPE_DM, page.Msg.Permissions[0].Scope)
	request.Page.Offset = 1
	page, err = service.ListBotPermissions(ctx, connect.NewRequest(request))
	require.NoError(t, err)
	require.False(t, page.Msg.Page.HasMore)
	require.Equal(t, apiv1.BotPermissionScope_BOT_PERMISSION_SCOPE_SERVER, page.Msg.Permissions[0].Scope)
	request.BotUserId = viewer.Id
	_, err = service.ListBotPermissions(ctx, connect.NewRequest(request))
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
}

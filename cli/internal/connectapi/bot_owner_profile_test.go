package connectapi

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestPublicBotOwnerProfile(t *testing.T) {
	env := newConnectAPITestEnv(t)
	bot, err := env.core.CreateBot(env.ctx, env.viewer.Id, "owned_bot", "Owned Bot")
	require.NoError(t, err)
	reader, err := env.core.CreateUser(env.ctx, core.SystemActorID, "owner_reader", "Reader", "password123")
	require.NoError(t, err)
	service := &userService{api: env.api}
	request := connect.NewRequest(&apiv1.BatchGetUsersRequest{UserIds: []string{bot.User.Id, reader.Id}})
	_, err = service.BatchGetUsers(env.ctx, request)
	require.Equal(t, connect.CodeUnauthenticated, errorCode(err))
	response, err := service.BatchGetUsers(withCaller(env.ctx, reader), request)
	require.NoError(t, err)
	require.Len(t, response.Msg.Users, 2)
	require.Equal(t, env.viewer.Id, response.Msg.Users[0].User.GetBot().GetOwnerUserId())
	require.Nil(t, response.Msg.Users[1].User.Bot)

	// Snapshot hydration and ordinary reads must expose the same public identity.
	snapshot, err := env.api.realtimeSnapshotUser(env.ctx, &core.HydratedUserContent{User: bot.User})
	require.NoError(t, err)
	require.Equal(t, env.viewer.Id, snapshot.GetBot().GetOwnerUserId())
	adminSummary := (&adminUserManagementService{api: env.api}).adminMember(env.ctx, core.AdminMember{ID: bot.User.Id, IsBot: true, BotOwnerUserID: env.viewer.Id}).GetUser()
	require.Equal(t, env.viewer.Id, adminSummary.GetBot().GetOwnerUserId())
	humanSnapshot, err := env.api.realtimeSnapshotUser(env.ctx, &core.HydratedUserContent{User: reader})
	require.NoError(t, err)
	require.Nil(t, humanSnapshot.Bot)

	require.NoError(t, env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.Id, core.PermBotManage))
	_, err = env.core.ReassignBotOwner(env.ctx, env.viewer.Id, bot.User.Id, reader.Id)
	require.NoError(t, err)
	response, err = service.BatchGetUsers(withCaller(env.ctx, reader), request)
	require.NoError(t, err)
	require.Equal(t, reader.Id, response.Msg.Users[0].User.GetBot().GetOwnerUserId())
	deleted, err := userSummary(env.ctx, env.api, core.DeletedUserReference(bot.User.Id), nil)
	require.NoError(t, err)
	require.Nil(t, deleted.Bot)
}

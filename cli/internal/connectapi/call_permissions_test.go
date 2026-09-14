package connectapi

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestCallPermissionsAcrossAPIEntryPoints(t *testing.T) {
	env := newConnectAPITestEnv(t)
	env.api.config.LiveKit = config.LiveKitConfig{Enabled: true, URL: "ws://livekit.test", APIKey: "key", APISecret: "secret"}
	ctx := withCaller(env.ctx, env.viewer)
	room, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, "", "call-access-api", "")
	require.NoError(t, err)
	_, err = env.core.JoinRoom(env.ctx, env.viewer.Id, core.KindChannel, env.viewer.Id, room.Id)
	require.NoError(t, err)
	require.NoError(t, env.core.DenyUserRoomPermission(env.ctx, core.SystemActorID, room.Id, env.viewer.Id, core.PermCallStart))
	_, err = env.voice.JoinCall(ctx, connect.NewRequest(&apiv1.JoinCallRequest{RoomId: room.Id}))
	require.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	require.NoError(t, env.core.GrantUserRoomPermission(env.ctx, core.SystemActorID, room.Id, env.viewer.Id, core.PermCallStart))
	_, err = env.voice.JoinCall(ctx, connect.NewRequest(&apiv1.JoinCallRequest{RoomId: room.Id}))
	require.NoError(t, err)
	for _, permission := range []core.Permission{core.PermCallVoice, core.PermCallCamera, core.PermCallScreenShare} {
		require.NoError(t, env.core.DenyUserRoomPermission(env.ctx, core.SystemActorID, room.Id, env.viewer.Id, permission))
	}
	token, err := env.voice.CreateCallToken(ctx, connect.NewRequest(&apiv1.CreateCallTokenRequest{RoomId: room.Id}))
	require.NoError(t, err)
	require.NotEmpty(t, token.Msg.Token)
	_, err = env.voice.CreateCallMediaPublisherToken(ctx, connect.NewRequest(&apiv1.CreateCallMediaPublisherTokenRequest{RoomId: room.Id, Kind: apiv1.CallMediaPublisherKind_CALL_MEDIA_PUBLISHER_KIND_GAME_SHARE}))
	require.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	require.NoError(t, env.core.GrantUserRoomPermission(env.ctx, core.SystemActorID, room.Id, env.viewer.Id, core.PermCallScreenShare))
	_, err = env.voice.CreateCallMediaPublisherToken(ctx, connect.NewRequest(&apiv1.CreateCallMediaPublisherTokenRequest{RoomId: room.Id, Kind: apiv1.CallMediaPublisherKind_CALL_MEDIA_PUBLISHER_KIND_GAME_SHARE}))
	require.NoError(t, err, "native captured audio does not need call.voice")
	require.NoError(t, env.core.DenyUserRoomPermission(env.ctx, core.SystemActorID, room.Id, env.viewer.Id, core.PermCallJoin))
	_, err = env.voice.CreateCallToken(ctx, connect.NewRequest(&apiv1.CreateCallTokenRequest{RoomId: room.Id}))
	require.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	_, err = env.voice.JoinCall(ctx, connect.NewRequest(&apiv1.JoinCallRequest{RoomId: room.Id}))
	require.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	_, err = env.voice.ListCallParticipants(ctx, connect.NewRequest(&apiv1.ListCallParticipantsRequest{RoomId: room.Id}))
	require.NoError(t, err)
	_, err = env.voice.LeaveCall(ctx, connect.NewRequest(&apiv1.LeaveCallRequest{RoomId: room.Id}))
	require.NoError(t, err)
}

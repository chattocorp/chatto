package connectapi

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestMessageServiceModeratorEchoRemoval(t *testing.T) {
	for _, tc := range []struct {
		name       string
		owner      bool
		powerMode  bool
		manage     bool
		wantRemove bool
	}{
		{name: "owner with power mode", owner: true, powerMode: true, wantRemove: true},
		{name: "owner without power mode", owner: true},
		{name: "delegated moderator", manage: true, powerMode: true, wantRemove: true},
		{name: "delegated moderator without power mode", manage: true},
		{name: "member without manage"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			env := newConnectAPITestEnv(t)
			room := env.createJoinedRoom("echo-moderation")
			if tc.owner {
				require.NoError(t, env.core.AssignOwnerRole(env.ctx, env.viewer.Id))
			}
			if tc.manage {
				require.NoError(t, env.core.GrantUserRoomPermission(env.ctx, core.SystemActorID, room.Id, env.viewer.Id, core.PermMessageManage))
			}
			bot, err := env.core.CreateBot(env.ctx, env.viewer.Id, "echo_bot", "Echo Bot")
			require.NoError(t, err)
			_, err = env.core.AddMember(env.ctx, core.SystemActorID, core.KindChannel, room.Id, bot.User.Id)
			require.NoError(t, err)
			root := env.post(room.Id, env.viewer.Id, "root", "")
			reply, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, bot.User.Id, "bot reply", nil, root.Id, "", nil, true)
			require.NoError(t, err)
			echoID, ok := env.core.ChannelEchoEventID(reply.Id)
			require.True(t, ok)
			ctx := withBearerCredential(env.ctx, env.viewer, "echo-moderation-session")
			if tc.powerMode {
				ctx = withArmedBearerCredential(env.ctx, env.viewer, "echo-moderation-session")
			}

			// Removing an artifact needs manage authority, not permission to post
			// or create a new echo.
			if tc.manage {
				require.NoError(t, env.core.DenyUserRoomPermission(env.ctx, core.SystemActorID, room.Id, env.viewer.Id, core.PermMessageEcho))
				require.NoError(t, env.core.DenyUserRoomPermission(env.ctx, core.SystemActorID, room.Id, env.viewer.Id, core.PermMessagePost))
			}
			response, err := env.messages.UpdateMessage(ctx, connect.NewRequest(&apiv1.UpdateMessageRequest{
				RoomId: room.Id, EventId: reply.Id, AlsoSendToChannel: boolPtr(false),
			}))
			if tc.wantRemove {
				require.NoError(t, err)
				require.Equal(t, "bot reply", response.Msg.GetMessage().GetBody())
				require.Empty(t, response.Msg.GetMessage().GetChannelEchoEventId())
			} else {
				requireConnectCode(t, err, connect.CodePermissionDenied)
			}
			require.Equal(t, tc.wantRemove, env.core.IsHiddenChannelEcho(echoID))
			body, err := env.core.GetMessageBody(env.ctx, reply.Id)
			require.NoError(t, err)
			require.Equal(t, "bot reply", body)

			// Even a moderator must not add another author's echo. The body
			// change in the same rejected request must not be committed.
			_, err = env.messages.UpdateMessage(ctx, connect.NewRequest(&apiv1.UpdateMessageRequest{
				RoomId: room.Id, EventId: reply.Id,
				Body: stringPtr("must not land"), AlsoSendToChannel: boolPtr(true),
			}))
			requireConnectCode(t, err, connect.CodePermissionDenied)
			body, err = env.core.GetMessageBody(env.ctx, reply.Id)
			require.NoError(t, err)
			require.Equal(t, "bot reply", body)
			require.Equal(t, tc.wantRemove, env.core.IsHiddenChannelEcho(echoID))
		})
	}
}

func TestMessageServiceModeratorEchoRemovalInDM(t *testing.T) {
	env := newConnectAPITestEnv(t)
	require.NoError(t, env.core.AssignOwnerRole(env.ctx, env.viewer.Id))
	participant, err := env.core.CreateUser(env.ctx, core.SystemActorID, "echo-participant", "Participant", "password")
	require.NoError(t, err)
	dm, _, err := env.core.FindOrCreateDM(env.ctx, env.viewer.Id, []string{participant.Id})
	require.NoError(t, err)
	root, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, env.viewer.Id, "root", nil, "", "", nil, false)
	require.NoError(t, err)
	reply, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, env.viewer.Id, "reply", nil, root.Id, "", nil, true)
	require.NoError(t, err)
	echoID, ok := env.core.ChannelEchoEventID(reply.Id)
	require.True(t, ok)
	request := connect.NewRequest(&apiv1.UpdateMessageRequest{
		RoomId: dm.Id, EventId: reply.Id, AlsoSendToChannel: boolPtr(false),
	})
	ctx := withArmedBearerCredential(env.ctx, participant, "echo-participant-session")
	_, err = env.messages.UpdateMessage(ctx, request)
	requireConnectCode(t, err, connect.CodePermissionDenied)
	require.False(t, env.core.IsHiddenChannelEcho(echoID))
	require.NoError(t, env.core.SetUserPermissionState(env.ctx, env.viewer.Id, participant.Id,
		core.PermissionTargetScope{Kind: core.MatrixScopeDM}, core.PermMessageManage, core.PermissionStateAllow))

	// Even an owner in power mode needs DM membership.
	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "echo-outsider", "Outsider", "password")
	require.NoError(t, err)
	require.NoError(t, env.core.AssignOwnerRole(env.ctx, outsider.Id))
	_, err = env.messages.UpdateMessage(withArmedBearerCredential(env.ctx, outsider, "echo-outsider-session"), request)
	requireConnectCode(t, err, connect.CodePermissionDenied)
	require.False(t, env.core.IsHiddenChannelEcho(echoID))

	response, err := env.messages.UpdateMessage(ctx, request)
	require.NoError(t, err)
	require.Equal(t, "reply", response.Msg.GetMessage().GetBody())
	require.Empty(t, response.Msg.GetMessage().GetChannelEchoEventId())
	require.True(t, env.core.IsHiddenChannelEcho(echoID))
}

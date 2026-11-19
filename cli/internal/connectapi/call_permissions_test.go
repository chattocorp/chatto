package connectapi

import (
	"fmt"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/authctx"
	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/require"
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
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
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
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	require.NoError(t, env.core.GrantUserRoomPermission(env.ctx, core.SystemActorID, room.Id, env.viewer.Id, core.PermCallScreenShare))
	_, err = env.voice.CreateCallMediaPublisherToken(ctx, connect.NewRequest(&apiv1.CreateCallMediaPublisherTokenRequest{RoomId: room.Id, Kind: apiv1.CallMediaPublisherKind_CALL_MEDIA_PUBLISHER_KIND_GAME_SHARE}))
	require.NoError(t, err, "native captured audio does not need call.voice")
	require.NoError(t, env.core.DenyUserRoomPermission(env.ctx, core.SystemActorID, room.Id, env.viewer.Id, core.PermCallJoin))
	_, err = env.voice.CreateCallMediaPublisherToken(ctx, connect.NewRequest(&apiv1.CreateCallMediaPublisherTokenRequest{RoomId: room.Id, Kind: apiv1.CallMediaPublisherKind_CALL_MEDIA_PUBLISHER_KIND_GAME_SHARE}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err), "screenshare cannot bypass revoked call.join")
	_, err = env.voice.CreateCallToken(ctx, connect.NewRequest(&apiv1.CreateCallTokenRequest{RoomId: room.Id}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	_, err = env.voice.JoinCall(ctx, connect.NewRequest(&apiv1.JoinCallRequest{RoomId: room.Id}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	_, err = env.voice.ListCallParticipants(ctx, connect.NewRequest(&apiv1.ListCallParticipantsRequest{RoomId: room.Id}))
	require.NoError(t, err)
	_, err = env.voice.LeaveCall(ctx, connect.NewRequest(&apiv1.LeaveCallRequest{RoomId: room.Id}))
	require.NoError(t, err)
}

func TestCallPermissionsTokenSourcesThroughAPI(t *testing.T) {
	env := newConnectAPITestEnv(t)
	env.api.config.LiveKit = config.LiveKitConfig{Enabled: true, URL: "ws://livekit.test", APIKey: "key", APISecret: "secret"}
	ctx := withCaller(env.ctx, env.viewer)
	room, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, "", "call-token-matrix", "")
	require.NoError(t, err)
	_, err = env.core.JoinRoom(env.ctx, env.viewer.Id, core.KindChannel, env.viewer.Id, room.Id)
	require.NoError(t, err)
	_, err = env.voice.JoinCall(ctx, connect.NewRequest(&apiv1.JoinCallRequest{RoomId: room.Id}))
	require.NoError(t, err)
	expected := [][]string{
		{},
		{"microphone"},
		{"camera"},
		{"microphone", "camera"},
		{"screen_share", "screen_share_audio"},
		{"microphone", "screen_share", "screen_share_audio"},
		{"camera", "screen_share", "screen_share_audio"},
		{"microphone", "camera", "screen_share", "screen_share_audio"},
	}
	for mask, sources := range expected {
		t.Run(fmt.Sprintf("media_%03b", mask), func(t *testing.T) {
			for bit, permission := range []core.Permission{core.PermCallVoice, core.PermCallCamera, core.PermCallScreenShare} {
				if mask&(1<<bit) != 0 {
					require.NoError(t, env.core.GrantUserRoomPermission(env.ctx, core.SystemActorID, room.Id, env.viewer.Id, permission))
				} else {
					require.NoError(t, env.core.DenyUserRoomPermission(env.ctx, core.SystemActorID, room.Id, env.viewer.Id, permission))
				}
			}
			token, err := env.voice.CreateCallToken(ctx, connect.NewRequest(&apiv1.CreateCallTokenRequest{RoomId: room.Id}))
			require.NoError(t, err)
			// Verify the signed credential returned by the handler, including the
			// resolver-to-token handoff, rather than calling the token helper directly.
			parsed, err := jwt.Parse(token.Msg.Token, func(*jwt.Token) (any, error) { return []byte("secret"), nil }, jwt.WithValidMethods([]string{"HS256"}))
			require.NoError(t, err)
			grant, ok := parsed.Claims.(jwt.MapClaims)["video"].(map[string]any)
			require.True(t, ok)
			require.Equal(t, mask != 0, grant["canPublish"])
			require.Equal(t, true, grant["canSubscribe"])
			require.Equal(t, false, grant["canPublishData"])
			var actual []string
			if raw, ok := grant["canPublishSources"].([]any); ok {
				for _, source := range raw {
					actual = append(actual, source.(string))
				}
			}
			require.ElementsMatch(t, sources, actual)
		})
	}
}

func TestCallTokenRPCsDuringFinalSecondOfShieldMode(t *testing.T) {
	env := newConnectAPITestEnv(t)
	env.api.config.LiveKit = config.LiveKitConfig{Enabled: true, URL: "ws://livekit.test", APIKey: "key", APISecret: "secret"}
	user := env.viewer
	room, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, "", "shield-call", "")
	require.NoError(t, err)
	_, err = env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id)
	require.NoError(t, err)
	require.NoError(t, env.core.AssignServerRole(env.ctx, core.SystemActorID, user.Id, core.RoleModerator))
	deadline := time.Now().Add(time.Hour)
	ctx := authctx.WithCredential(withCaller(env.ctx, user), authctx.RuntimeCredential{
		Handle: "session", UserID: user.Id, Kind: authctx.RuntimeCredentialKindCookieSession,
		PrivilegedRoles: []string{core.RoleModerator}, PrivilegedModeExpiresAt: deadline,
	})
	_, err = env.voice.JoinCall(ctx, connect.NewRequest(&apiv1.JoinCallRequest{RoomId: room.Id}))
	require.NoError(t, err)
	check := func(t *testing.T, issue func() string) {
		t.Helper()
		for _, remaining := range []time.Duration{time.Millisecond, 0} {
			env.voice.now = func() time.Time { return deadline.Add(-remaining) }
			parsed, err := jwt.Parse(issue(), func(*jwt.Token) (any, error) { return []byte("secret"), nil }, jwt.WithValidMethods([]string{"HS256"}))
			require.NoError(t, err)
			metadata := parsed.Claims.(jwt.MapClaims)["metadata"].(string)
			ref := core.ParseParticipantMetadata(metadata).SelectionRef
			require.Equal(t, remaining > 0, ref != "", "selection reference with %s remaining", remaining)
		}
	}
	t.Run("CreateCallToken", func(t *testing.T) {
		check(t, func() string {
			res, err := env.voice.CreateCallToken(ctx, connect.NewRequest(&apiv1.CreateCallTokenRequest{RoomId: room.Id}))
			require.NoError(t, err)
			return res.Msg.Token
		})
	})
	t.Run("CreateCallMediaPublisherToken", func(t *testing.T) {
		check(t, func() string {
			res, err := env.voice.CreateCallMediaPublisherToken(ctx, connect.NewRequest(&apiv1.CreateCallMediaPublisherTokenRequest{
				RoomId: room.Id, Kind: apiv1.CallMediaPublisherKind_CALL_MEDIA_PUBLISHER_KIND_GAME_SHARE,
			}))
			require.NoError(t, err)
			return res.Msg.Token
		})
	})
}

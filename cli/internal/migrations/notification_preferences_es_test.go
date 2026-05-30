package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestMigrateNotificationPreferencesToES_NoLegacyState(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

	require.NoError(t, MigrateNotificationPreferencesToES(ctx, kv, publisher, testLogger()))

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, info.State.Msgs)
}

func TestMigrateNotificationPreferencesToES_SeedsPerUserConfig(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

	putProtoKV(t, ctx, kv, "user_preferences.U1", &corev1.UserPreferences{
		NotificationLevel: corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED,
	})
	putProtoKV(t, ctx, kv, "room_user_preferences.U1.R1", &corev1.RoomUserPreferences{
		NotificationLevel: corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES,
	})
	putProtoKV(t, ctx, kv, "room_user_preferences.U2.R2", &corev1.RoomUserPreferences{
		NotificationLevel: corev1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT,
	})

	require.NoError(t, MigrateNotificationPreferencesToES(ctx, kv, publisher, testLogger()))

	subject := events.ConfigSubjectAggregate("U1").Subject(events.EventUserServerNotificationLevelSet)
	require.Equal(t, "evt.config.U1.user_server_notification_level_set", subject)
	last, err := stream.GetLastMsgForSubject(ctx, subject)
	require.NoError(t, err)
	require.NotZero(t, last.Sequence)

	gotValues := map[string]corev1.NotificationLevel{}
	for seq := uint64(1); seq <= 2; seq++ {
		msg, err := stream.GetMsg(ctx, seq)
		require.NoError(t, err)
		var got corev1.Event
		require.NoError(t, proto.Unmarshal(msg.Data, &got))
		require.Equal(t, "system:migration", got.GetActorId())
		switch change := got.GetEvent().(type) {
		case *corev1.Event_UserServerNotificationLevelSet:
			require.Equal(t, "U1", change.UserServerNotificationLevelSet.GetUserId())
			gotValues["notifications.server.level"] = change.UserServerNotificationLevelSet.GetLevel()
		case *corev1.Event_UserRoomNotificationLevelSet:
			require.Equal(t, "U1", change.UserRoomNotificationLevelSet.GetUserId())
			gotValues["notifications.rooms."+change.UserRoomNotificationLevelSet.GetRoomId()+".level"] = change.UserRoomNotificationLevelSet.GetLevel()
		default:
			t.Fatalf("unexpected event variant %T", change)
		}
	}
	require.Equal(t, corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED, gotValues["notifications.server.level"])
	require.Equal(t, corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES, gotValues["notifications.rooms.R1.level"])

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 2, info.State.Msgs)

	require.NoError(t, MigrateNotificationPreferencesToES(ctx, kv, publisher, testLogger()))
	infoReplay, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 2, infoReplay.State.Msgs)
}

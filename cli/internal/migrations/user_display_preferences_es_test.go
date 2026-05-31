package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestMigrateUserDisplayPreferencesToES_SeedsAndReplays(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

	tz := "Europe/Berlin"
	putProtoKV(t, ctx, kv, "user_preferences.U1", &corev1.ServerUserPreferences{
		Timezone:   proto.String(tz),
		TimeFormat: corev1.TimeFormat_TIME_FORMAT_24H,
	})
	putProtoKV(t, ctx, kv, "user_preferences.U2", &corev1.ServerUserPreferences{})

	// Existing notification config on the same user subject must not block
	// importing display preference paths.
	agg := events.ConfigSubjectAggregate("U1")
	_, err := publisher.AppendAt(ctx, agg.Subject(events.EventUserServerNotificationLevelSet), &corev1.Event{
		Id:      newMigrationEventID(),
		ActorId: "system:test",
		Event: &corev1.Event_UserServerNotificationLevelSet{UserServerNotificationLevelSet: &corev1.UserServerNotificationLevelSetEvent{
			UserId: "U1",
			Level:  corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED,
		}},
	}, 0)
	require.NoError(t, err)

	require.NoError(t, MigrateUserDisplayPreferencesToES(ctx, kv, publisher, testLogger()))

	gotValues := map[string]any{}
	gotStrings := map[string]string{}
	eventsForU1, _, err := publisher.SubjectEvents(ctx, agg.AllEventsFilter())
	require.NoError(t, err)
	for _, got := range eventsForU1 {
		switch change := got.GetEvent().(type) {
		case *corev1.Event_UserTimezoneChanged:
			gotStrings["preferences.timezone"] = change.UserTimezoneChanged.GetTimezone()
		case *corev1.Event_UserTimeFormatChanged:
			gotValues["preferences.time_format"] = change.UserTimeFormatChanged.GetTimeFormat()
		case *corev1.Event_UserServerNotificationLevelSet:
			gotValues["notifications.server.level"] = change.UserServerNotificationLevelSet.GetLevel()
		}
	}
	require.Equal(t, "Europe/Berlin", gotStrings["preferences.timezone"])
	require.Equal(t, corev1.TimeFormat_TIME_FORMAT_24H, gotValues["preferences.time_format"])
	require.Equal(t, corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED, gotValues["notifications.server.level"])

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	msgsAfterFirstRun := info.State.Msgs

	require.NoError(t, MigrateUserDisplayPreferencesToES(ctx, kv, publisher, testLogger()))
	infoReplay, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, msgsAfterFirstRun, infoReplay.State.Msgs)
}

func TestMigrateUserDisplayPreferencesToES_SkipsStaleKVWhenUserPreferenceEventExists(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

	staleTZ := "Europe/Berlin"
	putProtoKV(t, ctx, kv, "user_preferences.U1", &corev1.ServerUserPreferences{
		Timezone:   proto.String(staleTZ),
		TimeFormat: corev1.TimeFormat_TIME_FORMAT_24H,
	})

	latestTZ := "America/New_York"
	userAgg := events.UserAggregate("U1")
	_, err := publisher.AppendAt(ctx, userAgg.Subject(events.EventUserServerPreferencesChanged), &corev1.Event{
		Id:      newMigrationEventID(),
		ActorId: "U1",
		Event: &corev1.Event_UserServerPreferencesChanged{
			UserServerPreferencesChanged: &corev1.UserServerPreferencesChangedEvent{
				UserId: "U1",
				Preferences: &corev1.ServerUserPreferences{
					Timezone:   proto.String(latestTZ),
					TimeFormat: corev1.TimeFormat_TIME_FORMAT_12H,
				},
			},
		},
	}, 0)
	require.NoError(t, err)

	require.NoError(t, MigrateUserDisplayPreferencesToES(ctx, kv, publisher, testLogger()))

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, info.State.Msgs)

	_, _, err = configSubjectEvents(ctx, publisher, "U1")
	require.NoError(t, err)
	_, err = stream.GetLastMsgForSubject(ctx, events.ConfigSubjectAggregate("U1").Subject(events.EventUserTimezoneChanged))
	require.Error(t, err)
}

func TestMigrateUserDisplayPreferencesToES_NoLegacyState(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

	require.NoError(t, MigrateUserDisplayPreferencesToES(ctx, kv, publisher, testLogger()))

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, info.State.Msgs)
}

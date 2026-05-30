package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
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
	_, err := publisher.AppendAt(ctx, agg.Subject(events.EventConfigValueSet), &corev1.Event{
		Id:      newMigrationEventID(),
		ActorId: "system:test",
		Event: &corev1.Event_ConfigValueSet{ConfigValueSet: &corev1.ConfigValueSetEvent{
			Subject: "U1",
			Path:    "notifications.server.level",
			Value:   &configv1.ConfigValue{Value: &configv1.ConfigValue_IntValue{IntValue: 2}},
		}},
	}, 0)
	require.NoError(t, err)

	require.NoError(t, MigrateUserDisplayPreferencesToES(ctx, kv, publisher, testLogger()))

	gotValues := map[string]int64{}
	gotStrings := map[string]string{}
	eventsForU1, _, err := publisher.SubjectEvents(ctx, agg.AllEventsFilter())
	require.NoError(t, err)
	for _, got := range eventsForU1 {
		change, ok := got.GetEvent().(*corev1.Event_ConfigValueSet)
		if !ok {
			continue
		}
		switch value := change.ConfigValueSet.GetValue().GetValue().(type) {
		case *configv1.ConfigValue_StringValue:
			gotStrings[change.ConfigValueSet.GetPath()] = value.StringValue
		case *configv1.ConfigValue_IntValue:
			gotValues[change.ConfigValueSet.GetPath()] = value.IntValue
		}
	}
	require.Equal(t, "Europe/Berlin", gotStrings["preferences.timezone"])
	require.EqualValues(t, corev1.TimeFormat_TIME_FORMAT_24H, gotValues["preferences.time_format"])
	require.EqualValues(t, 2, gotValues["notifications.server.level"])

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	msgsAfterFirstRun := info.State.Msgs

	require.NoError(t, MigrateUserDisplayPreferencesToES(ctx, kv, publisher, testLogger()))
	infoReplay, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, msgsAfterFirstRun, infoReplay.State.Msgs)
}

func TestMigrateUserDisplayPreferencesToES_NoLegacyState(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

	require.NoError(t, MigrateUserDisplayPreferencesToES(ctx, kv, publisher, testLogger()))

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, info.State.Msgs)
}

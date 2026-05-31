package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestMigrateServerConfigToES_NoLegacyState(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

	// No "config.instance" key in KV — migration is a no-op.
	require.NoError(t, MigrateServerConfigToES(ctx, kv, publisher, testLogger()))

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, info.State.Msgs)
}

func TestMigrateServerConfigToES_SeedsAndReplays(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

	desired := &configv1.ServerConfig{
		ServerName:       "Legacy Server",
		WelcomeMessage:   "old welcome",
		Motd:             "old MOTD",
		BlockedUsernames: "foo\nbar",
		Description:      "old description",
	}
	data, err := proto.Marshal(desired)
	require.NoError(t, err)
	_, err = kv.Put(ctx, "config.instance", data)
	require.NoError(t, err)

	// First run: one semantic event per legacy config field lands on evt.config.server.
	require.NoError(t, MigrateServerConfigToES(ctx, kv, publisher, testLogger()))

	subject := events.ConfigAggregate().Subject(events.EventServerNameChanged)
	require.Equal(t, "evt.config.server.server_name_changed", subject)
	msg, err := stream.GetLastMsgForSubject(ctx, subject)
	require.NoError(t, err)
	require.NotZero(t, msg.Sequence)

	gotValues := map[string]string{}
	for seq := uint64(1); seq <= 5; seq++ {
		msg, err := stream.GetMsg(ctx, seq)
		require.NoError(t, err)
		var got corev1.Event
		require.NoError(t, proto.Unmarshal(msg.Data, &got))
		require.Equal(t, "system:migration", got.GetActorId())
		switch change := got.GetEvent().(type) {
		case *corev1.Event_ServerNameChanged:
			gotValues["server.name"] = change.ServerNameChanged.GetName()
		case *corev1.Event_ServerWelcomeMessageChanged:
			gotValues["server.welcome_message"] = change.ServerWelcomeMessageChanged.GetWelcomeMessage()
		case *corev1.Event_ServerMotdChanged:
			gotValues["server.motd"] = change.ServerMotdChanged.GetMotd()
		case *corev1.Event_ServerBlockedUsernamesChanged:
			gotValues["auth.blocked_usernames"] = change.ServerBlockedUsernamesChanged.GetBlockedUsernames()
		case *corev1.Event_ServerDescriptionChanged:
			gotValues["server.description"] = change.ServerDescriptionChanged.GetDescription()
		default:
			t.Fatalf("unexpected event variant %T", change)
		}
	}
	require.Equal(t, "Legacy Server", gotValues["server.name"])
	require.Equal(t, "old welcome", gotValues["server.welcome_message"])
	require.Equal(t, "old MOTD", gotValues["server.motd"])
	require.Equal(t, "foo\nbar", gotValues["auth.blocked_usernames"])
	require.Equal(t, "old description", gotValues["server.description"])

	// Stream has exactly one batch worth of messages.
	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 5, info.State.Msgs)

	// Replay: OCC skips the second batch; no new messages land.
	require.NoError(t, MigrateServerConfigToES(ctx, kv, publisher, testLogger()))
	infoReplay, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 5, infoReplay.State.Msgs)
}

func TestMigrateServerConfigToES_PrefersLegacyEVTSnapshotOverStaleKV(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

	putProtoKV(t, ctx, kv, "config.instance", &configv1.ServerConfig{
		ServerName: "Stale KV",
		Motd:       "old motd",
	})

	agg := events.ConfigAggregate()
	_, err := publisher.AppendAt(ctx, agg.Subject(events.EventServerConfigChanged), &corev1.Event{
		Id:      newMigrationEventID(),
		ActorId: "system:test",
		Event: &corev1.Event_ServerConfigChanged{
			ServerConfigChanged: &corev1.ServerConfigChangedEvent{
				Config: &configv1.ServerConfig{
					ServerName:     "Latest EVT",
					Motd:           "new motd",
					WelcomeMessage: "new welcome",
				},
			},
		},
	}, 0)
	require.NoError(t, err)

	require.NoError(t, MigrateServerConfigToES(ctx, kv, publisher, testLogger()))

	msg, err := stream.GetLastMsgForSubject(ctx, agg.Subject(events.EventServerNameChanged))
	require.NoError(t, err)
	var got corev1.Event
	require.NoError(t, proto.Unmarshal(msg.Data, &got))
	require.Equal(t, "Latest EVT", got.GetServerNameChanged().GetName())

	msg, err = stream.GetLastMsgForSubject(ctx, agg.Subject(events.EventServerMotdChanged))
	require.NoError(t, err)
	require.NoError(t, proto.Unmarshal(msg.Data, &got))
	require.Equal(t, "new motd", got.GetServerMotdChanged().GetMotd())
}

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

	// First run: one event per generic config path lands on evt.config.server.
	require.NoError(t, MigrateServerConfigToES(ctx, kv, publisher, testLogger()))

	subject := events.ConfigAggregate().Subject(events.EventConfigValueSet)
	require.Equal(t, "evt.config.server.value_set", subject)
	msg, err := stream.GetLastMsgForSubject(ctx, subject)
	require.NoError(t, err)
	require.NotZero(t, msg.Sequence)

	gotValues := map[string]string{}
	for seq := uint64(1); seq <= 5; seq++ {
		msg, err := stream.GetMsg(ctx, seq)
		require.NoError(t, err)
		var got corev1.Event
		require.NoError(t, proto.Unmarshal(msg.Data, &got))
		change, ok := got.GetEvent().(*corev1.Event_ConfigValueSet)
		require.True(t, ok, "expected ConfigValueSet variant")
		require.Equal(t, "server", change.ConfigValueSet.GetSubject())
		require.Equal(t, "system:migration", got.GetActorId())
		gotValues[change.ConfigValueSet.GetPath()] = change.ConfigValueSet.GetValue().GetStringValue()
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

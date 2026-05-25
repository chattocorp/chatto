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

	// First run: one event lands on evt.config.server.
	require.NoError(t, MigrateServerConfigToES(ctx, kv, publisher, testLogger()))

	subject := events.ConfigAggregate().Subject(events.EventServerConfigChanged)
	require.Equal(t, "evt.config.server.config_changed", subject)
	msg, err := stream.GetLastMsgForSubject(ctx, subject)
	require.NoError(t, err)
	require.NotZero(t, msg.Sequence)

	// Decode it and confirm the snapshot is what we seeded.
	var got corev1.Event
	require.NoError(t, proto.Unmarshal(msg.Data, &got))
	change, ok := got.GetEvent().(*corev1.Event_ServerConfigChanged)
	require.True(t, ok, "expected ServerConfigChanged variant")
	require.Equal(t, "Legacy Server", change.ServerConfigChanged.GetConfig().GetServerName())
	require.Equal(t, "system:migration", got.GetActorId())

	// Stream has exactly one message.
	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, info.State.Msgs)

	// Replay: OCC skips the second AppendAt; no new message lands.
	require.NoError(t, MigrateServerConfigToES(ctx, kv, publisher, testLogger()))
	infoReplay, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, infoReplay.State.Msgs)
}

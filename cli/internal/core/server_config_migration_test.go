package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
)

func TestMigrateServerConfig_NoLegacyState(t *testing.T) {
	// setupTestCore runs MigrateServerConfig once during NewChattoCore;
	// at that point the legacy INSTANCE_CONFIG KV is empty, so the
	// migration should report SkippedNoLegacyState. We re-run it
	// explicitly to assert that.
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	stats, err := core.MigrateServerConfig(ctx)
	require.NoError(t, err)
	require.True(t, stats.SkippedNoLegacyState, "expected SkippedNoLegacyState=true on empty KV")
	require.False(t, stats.Emitted)
	require.False(t, stats.SkippedAlreadyMigrated)
}

func TestMigrateServerConfig_SeedsFromKVAndIsReplayable(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Seed the legacy KV directly. Going through ConfigManager would
	// already publish a ServerConfigChangedEvent (dual-write), which
	// would defeat the test's purpose: we want to simulate "an existing
	// pre-ES deployment whose KV holds the config and whose SERVER_EVT
	// has nothing on the config aggregate yet."
	desired := &configv1.ServerConfig{
		ServerName:       "Legacy Server",
		WelcomeMessage:   "old welcome",
		Motd:             "old MOTD",
		BlockedUsernames: "foo\nbar",
		Description:      "old description",
	}
	data, err := proto.Marshal(desired)
	require.NoError(t, err)
	_, err = core.storage.runtimeConfigKV.Put(ctx, "config.instance", data)
	require.NoError(t, err)

	// Migration #1: KV-seeded event lands on evt.config.server.
	stats, err := core.MigrateServerConfig(ctx)
	require.NoError(t, err)
	require.True(t, stats.Emitted, "first run should emit")
	require.False(t, stats.SkippedAlreadyMigrated)
	require.False(t, stats.SkippedNoLegacyState)

	// Verify the event landed on the right subject.
	subject := events.ConfigAggregate().Subject()
	require.Equal(t, "evt.config.server", subject)
	msg, err := core.storage.serverEvtStream.GetLastMsgForSubject(ctx, subject)
	require.NoError(t, err)
	require.NotZero(t, msg.Sequence)

	// Projector picks it up; the in-memory snapshot reflects the seed.
	waitForCondition(t, 3*time.Second, func() bool {
		cfg, configured := core.ServerConfig.Get()
		return configured && cfg != nil && cfg.ServerName == "Legacy Server"
	})

	// Migration #2: OCC-skipped — no new events written.
	replay, err := core.MigrateServerConfig(ctx)
	require.NoError(t, err)
	require.False(t, replay.Emitted)
	require.True(t, replay.SkippedAlreadyMigrated, "replay should be SkippedAlreadyMigrated")
	require.False(t, replay.SkippedNoLegacyState)

	// Stream still has exactly the one event we emitted.
	msg2, err := core.storage.serverEvtStream.GetLastMsgForSubject(ctx, subject)
	require.NoError(t, err)
	require.Equal(t, msg.Sequence, msg2.Sequence)
}

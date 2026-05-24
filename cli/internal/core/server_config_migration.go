package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ServerConfigMigrationStats summarises what MigrateServerConfig did
// on a single boot.
type ServerConfigMigrationStats struct {
	// Emitted is true if a ServerConfigChangedEvent was newly appended
	// to SERVER_EVT for the singleton config aggregate. False on
	// already-migrated boots and when there's no KV state to migrate.
	Emitted bool
	// SkippedAlreadyMigrated is true if SERVER_EVT already had at
	// least one event on evt.config.server (re-run after the first
	// successful migration; OCC short-circuits).
	SkippedAlreadyMigrated bool
	// SkippedNoLegacyState is true if INSTANCE_CONFIG has no
	// "config.instance" key (fresh deployment that was never
	// configured); nothing to seed.
	SkippedNoLegacyState bool
}

// MigrateServerConfig is the one-shot ES migration for server config
// (ADR-035 phase 3 for the config aggregate).
//
// It reads the current config from INSTANCE_CONFIG KV and emits one
// ServerConfigChangedEvent on evt.config.server with expectedSeq=0.
// Behaviour:
//
//   - Fresh deployment, KV has a config (e.g. bootstrap created one):
//     one event emitted with the KV entry's Created() timestamp as
//     created_at. Emitted=true.
//   - Fresh deployment, no KV config: nothing to seed.
//     SkippedNoLegacyState=true.
//   - Replay after a successful first run: AppendAt(seq=0) fails with
//     ErrConflict because evt.config.server already has events.
//     SkippedAlreadyMigrated=true.
//
// Replayability via OCC matches MigrateRoomMembership: re-running the
// migration on any boot is a deliberate no-op rather than an error.
func (c *ChattoCore) MigrateServerConfig(ctx context.Context) (ServerConfigMigrationStats, error) {
	var stats ServerConfigMigrationStats

	// Read directly from the legacy INSTANCE_CONFIG KV bucket. The
	// projection isn't populated yet at boot — migration runs before
	// the projector consumer starts (see NewChattoCore) — so we can't
	// route through ConfigManager.GetServerConfig here.
	entry, err := c.storage.runtimeConfigKV.Get(ctx, "config.instance")
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			stats.SkippedNoLegacyState = true
			return stats, nil
		}
		return stats, fmt.Errorf("read legacy server config: %w", err)
	}

	cfg := &configv1.ServerConfig{}
	if err := proto.Unmarshal(entry.Value(), cfg); err != nil {
		return stats, fmt.Errorf("unmarshal legacy server config: %w", err)
	}

	createdAt := timestamppb.New(entry.Created())

	event := &corev1.Event{
		Id:        NewEventID(),
		ActorId:   "system:migration",
		CreatedAt: createdAt,
		Event: &corev1.Event_ServerConfigChanged{
			ServerConfigChanged: &corev1.ServerConfigChangedEvent{
				Config: cfg,
			},
		},
	}

	subject := events.ConfigAggregate().Subject()
	_, err = c.EventPublisher.AppendAt(ctx, subject, event, 0)
	if err == nil {
		stats.Emitted = true
		return stats, nil
	}
	if errors.Is(err, events.ErrConflict) {
		// SERVER_EVT already has events on this aggregate — a previous
		// migration run (or runtime publish) populated it. Treat as
		// no-op.
		stats.SkippedAlreadyMigrated = true
		return stats, nil
	}
	return stats, fmt.Errorf("seed ServerConfigChangedEvent: %w", err)
}

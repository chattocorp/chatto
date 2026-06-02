// Package migrations holds one-shot data migrations that run at boot.
//
// # Why this package exists
//
// Chatto's storage layer evolves over time, and occasionally the shape of
// data already persisted in NATS KV needs to change (e.g. JSON → proto,
// one-blob-per-user → one-entry-per-thing). Each such transition has a
// migration function in this package that:
//
//  1. Runs idempotently at boot from ChattoCore initialisation.
//  2. Scans for legacy storage keys, converts them to the new shape, and
//     deletes the legacy entries.
//  3. Short-circuits when no legacy keys remain, so it costs essentially
//     nothing on subsequent boots.
//
// Migrations live here, not next to the feature code, so there is a
// single place to audit "what code do we keep around purely to convert
// legacy data?". When a migration's legacy data is known to have been
// drained from all live deployments, the migration can be deleted in
// one tightly scoped PR.
//
// # Adding a migration
//
//  1. Add a file `xxx.go` in this package with a function taking
//     whatever it needs (typically a KV bucket and a logger). Document
//     the legacy shape, the new shape, and the reason for the change in
//     a doc comment.
//  2. Call it from [RunAll].
//
// # Removing a migration
//
// Once the legacy data has been confirmed drained on every live
// deployment that will ever boot this code, delete the file and the
// corresponding call in [RunAll].
package migrations

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/events"
)

// RunAll runs every active migration in order, stopping at the first
// error.
//
// Legacy source handles may be nil on fresh ES-only deployments; RunAll treats
// nil as an empty import source. Current target handles are required.
//
// KV buckets:
//   - `serverKV`: INSTANCE bucket (legacy user data and related indexes).
//   - `serverConfigKV`: SERVER_CONFIG (rooms, room memberships,
//     notification levels, …).
//   - `serverBodiesKV`: SERVER_BODIES (message content + standalone
//     attachment metadata records).
//   - `serverRuntimeKV`: SERVER_RUNTIME (sequences, sentinels,
//     recomputable state).
//   - `runtimeConfigKV`: INSTANCE_CONFIG (operator-editable server
//     settings — name, MOTD, blocked usernames, etc.).
//   - `runtimeStateKV`: RUNTIME_STATE (persisted latest-value runtime/user
//     state such as read markers, thread follows, push subscriptions, and
//     HMAC-keyed auth/workflow tokens).
//   - `serverReactionsKV`: SERVER_REACTIONS (legacy current reaction state).
//   - `serverEventsStream`: SERVER_EVENTS (legacy room/member event log).
//
// `publisher` writes to the EVT event-sourcing stream and is
// used by the ES migrations (ADR-035 phase 3 per aggregate) that seed
// the stream from pre-ES KV state on first boot.
func RunAll(
	ctx context.Context,
	serverKV, serverConfigKV, serverBodiesKV, serverRuntimeKV, runtimeConfigKV jetstream.KeyValue,
	runtimeStateKV jetstream.KeyValue,
	serverEventsStream jetstream.Stream,
	serverReactionsKV jetstream.KeyValue,
	publisher *events.Publisher,
	logger *log.Logger,
) error {
	run := func(name string, legacySourcePresent bool, fn func() error) error {
		return runMigrationStep(ctx, publisher, logger, name, legacySourcePresent, fn)
	}

	if err := run("verified_emails", serverKV != nil, func() error {
		return MigrateVerifiedEmailsToProto(ctx, serverKV, logger)
	}); err != nil {
		return err
	}
	if err := run("push_subscriptions_runtime_state", serverKV != nil, func() error {
		return MigratePushSubscriptionsToRuntimeState(ctx, serverKV, runtimeStateKV, logger)
	}); err != nil {
		return err
	}
	if err := run("server_branding_es", serverKV != nil, func() error {
		return MigrateServerBrandingToES(ctx, serverKV, publisher, logger)
	}); err != nil {
		return err
	}
	if err := run("users_es", serverKV != nil, func() error {
		return MigrateUsersToES(ctx, serverKV, publisher, logger)
	}); err != nil {
		return err
	}
	if err := run("user_display_preferences_es", serverKV != nil, func() error {
		return MigrateUserDisplayPreferencesToES(ctx, serverKV, publisher, logger)
	}); err != nil {
		return err
	}

	if err := run("room_kind", serverConfigKV != nil, func() error {
		return BackfillRoomKind(ctx, serverConfigKV, logger)
	}); err != nil {
		return err
	}
	// Room metadata + memberships share the evt.room.{R} subject and must
	// seed together: a RoomCreatedEvent first, then the chronologically
	// ordered UserJoinedRoomEvents. Atomic AppendBatch keeps that invariant.
	if err := run("room_aggregate_es", serverConfigKV != nil, func() error {
		return MigrateRoomAggregateToES(ctx, serverConfigKV, publisher, logger)
	}); err != nil {
		return err
	}
	if err := run("room_groups_es", serverConfigKV != nil, func() error {
		return MigrateRoomGroupsToES(ctx, serverConfigKV, publisher, logger)
	}); err != nil {
		return err
	}
	if err := run("room_layout_es", serverConfigKV != nil, func() error {
		return MigrateRoomLayoutToES(ctx, serverConfigKV, publisher, logger)
	}); err != nil {
		return err
	}
	if err := run("notification_preferences_es", serverConfigKV != nil, func() error {
		return MigrateNotificationPreferencesToES(ctx, serverConfigKV, publisher, logger)
	}); err != nil {
		return err
	}

	bodyRuntimeSourcePresent := serverBodiesKV != nil && serverRuntimeKV != nil
	if err := run("attachment_locator_data", bodyRuntimeSourcePresent, func() error {
		return BackfillAttachmentLocatorData(ctx, serverBodiesKV, serverRuntimeKV, logger)
	}); err != nil {
		return err
	}
	if err := run("legacy_attachment_records", bodyRuntimeSourcePresent, func() error {
		return DropLegacyAttachmentRecords(ctx, serverBodiesKV, serverRuntimeKV, logger)
	}); err != nil {
		return err
	}

	if err := run("read_markers_runtime_state", serverRuntimeKV != nil, func() error {
		return MigrateReadMarkersToRuntimeState(ctx, serverRuntimeKV, runtimeStateKV, logger)
	}); err != nil {
		return err
	}
	if err := run("thread_follows_runtime_state", serverRuntimeKV != nil, func() error {
		return MigrateThreadFollowsToRuntimeState(ctx, serverRuntimeKV, runtimeStateKV, logger)
	}); err != nil {
		return err
	}

	if err := run("server_config_es", runtimeConfigKV != nil, func() error {
		return MigrateServerConfigToES(ctx, runtimeConfigKV, publisher, logger)
	}); err != nil {
		return err
	}
	if err := run("messages_es", serverEventsStream != nil, func() error {
		return MigrateMessagesToES(ctx, serverEventsStream, serverBodiesKV, publisher, logger)
	}); err != nil {
		return err
	}
	if err := run("reactions_es", serverEventsStream != nil && serverReactionsKV != nil, func() error {
		return MigrateReactionsToES(ctx, serverEventsStream, serverReactionsKV, publisher, logger)
	}); err != nil {
		return err
	}
	return nil
}

func runMigrationStep(
	ctx context.Context,
	publisher *events.Publisher,
	logger *log.Logger,
	name string,
	legacySourcePresent bool,
	run func() error,
) error {
	if !legacySourcePresent {
		logger.Info(
			"ES boot migration step skipped",
			"step", name,
			"legacy_source_present", false,
			"evt_messages_appended", 0,
			"evt_bytes_appended", 0,
			"duration_ms", 0,
		)
		return nil
	}

	startedAt := time.Now()
	before := captureMigrationEVTUsage(ctx, publisher, logger, name, "before")
	logger.Info("ES boot migration step started", "step", name, "legacy_source_present", true)

	err := run()
	durationMS := time.Since(startedAt).Milliseconds()
	appended := before.delta(captureMigrationEVTUsage(ctx, publisher, logger, name, "after"))

	if err != nil {
		logger.Warn(
			"ES boot migration step failed",
			"step", name,
			"legacy_source_present", true,
			"evt_messages_appended", appended.messages,
			"evt_bytes_appended", appended.bytes,
			"duration_ms", durationMS,
			"error", err,
		)
		return fmt.Errorf("%s: %w", name, err)
	}

	logger.Info(
		"ES boot migration step completed",
		"step", name,
		"legacy_source_present", true,
		"evt_messages_appended", appended.messages,
		"evt_bytes_appended", appended.bytes,
		"duration_ms", durationMS,
	)
	return nil
}

type migrationEVTUsage struct {
	messages uint64
	bytes    uint64
	ok       bool
}

func captureMigrationEVTUsage(ctx context.Context, publisher *events.Publisher, logger *log.Logger, step, phase string) migrationEVTUsage {
	if publisher == nil {
		return migrationEVTUsage{}
	}
	messages, bytes, err := publisher.StreamUsage(ctx)
	if err != nil {
		logger.Warn("ES boot migration stream metrics unavailable", "step", step, "phase", phase, "error", err)
		return migrationEVTUsage{}
	}
	return migrationEVTUsage{messages: messages, bytes: bytes, ok: true}
}

func (before migrationEVTUsage) delta(after migrationEVTUsage) migrationEVTUsage {
	if !before.ok || !after.ok {
		return migrationEVTUsage{}
	}
	var delta migrationEVTUsage
	if after.messages >= before.messages {
		delta.messages = after.messages - before.messages
	}
	if after.bytes >= before.bytes {
		delta.bytes = after.bytes - before.bytes
	}
	delta.ok = true
	return delta
}

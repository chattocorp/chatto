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

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/events"
)

// RunAll runs every active migration in order, stopping at the first
// error.
//
// KV buckets:
//   - `serverKV`: INSTANCE bucket (user data, auth tokens, etc.).
//   - `serverConfigKV`: SERVER_CONFIG (rooms, room memberships,
//     notification levels, …).
//   - `serverBodiesKV`: SERVER_BODIES (message content + standalone
//     attachment metadata records).
//   - `serverRuntimeKV`: SERVER_RUNTIME (sequences, sentinels,
//     recomputable state).
//   - `runtimeConfigKV`: INSTANCE_CONFIG (operator-editable server
//     settings — name, MOTD, blocked usernames, etc.).
//   - `runtimeStateKV`: RUNTIME_STATE (persisted latest-value runtime/user
//     state such as read markers, thread follows, and push subscriptions).
//   - `serverReactionsKV`: SERVER_REACTIONS (legacy current reaction
//     state; retained until reaction ES migration cleanup).
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
	if err := MigrateVerifiedEmailsToProto(ctx, serverKV, logger); err != nil {
		return fmt.Errorf("verified_emails: %w", err)
	}
	if err := BackfillRoomKind(ctx, serverConfigKV, logger); err != nil {
		return fmt.Errorf("room_kind: %w", err)
	}
	if err := BackfillAttachmentLocatorData(ctx, serverBodiesKV, serverRuntimeKV, logger); err != nil {
		return fmt.Errorf("attachment_locator_data: %w", err)
	}
	if err := DropLegacyAttachmentRecords(ctx, serverBodiesKV, serverRuntimeKV, logger); err != nil {
		return fmt.Errorf("legacy_attachment_records: %w", err)
	}
	if err := MigrateReadMarkersToRuntimeState(ctx, serverRuntimeKV, runtimeStateKV, logger); err != nil {
		return fmt.Errorf("read_markers_runtime_state: %w", err)
	}
	if err := MigrateThreadFollowsToRuntimeState(ctx, serverRuntimeKV, runtimeStateKV, logger); err != nil {
		return fmt.Errorf("thread_follows_runtime_state: %w", err)
	}
	if err := MigratePushSubscriptionsToRuntimeState(ctx, serverKV, runtimeStateKV, logger); err != nil {
		return fmt.Errorf("push_subscriptions_runtime_state: %w", err)
	}
	// Room metadata + memberships share the evt.room.{R} subject and
	// must seed together — a RoomCreatedEvent first, then the
	// chronologically-ordered UserJoinedRoomEvents. Atomic AppendBatch
	// keeps that invariant on every observable sequence.
	if err := MigrateRoomAggregateToES(ctx, serverConfigKV, publisher, logger); err != nil {
		return fmt.Errorf("room_aggregate_es: %w", err)
	}
	if err := MigrateRoomGroupsToES(ctx, serverConfigKV, publisher, logger); err != nil {
		return fmt.Errorf("room_groups_es: %w", err)
	}
	if err := MigrateRoomLayoutToES(ctx, serverConfigKV, publisher, logger); err != nil {
		return fmt.Errorf("room_layout_es: %w", err)
	}
	if err := MigrateServerConfigToES(ctx, runtimeConfigKV, publisher, logger); err != nil {
		return fmt.Errorf("server_config_es: %w", err)
	}
	if err := MigrateServerBrandingToES(ctx, serverKV, publisher, logger); err != nil {
		return fmt.Errorf("server_branding_es: %w", err)
	}
	if err := MigrateNotificationPreferencesToES(ctx, serverConfigKV, publisher, logger); err != nil {
		return fmt.Errorf("notification_preferences_es: %w", err)
	}
	if err := MigrateMessagesToES(ctx, serverEventsStream, serverBodiesKV, publisher, logger); err != nil {
		return fmt.Errorf("messages_es: %w", err)
	}
	if err := MigrateReactionsToES(ctx, serverEventsStream, serverReactionsKV, publisher, logger); err != nil {
		return fmt.Errorf("reactions_es: %w", err)
	}
	if err := MigrateUsersToES(ctx, serverKV, publisher, logger); err != nil {
		return fmt.Errorf("users_es: %w", err)
	}
	if err := MigrateUserDisplayPreferencesToES(ctx, serverKV, publisher, logger); err != nil {
		return fmt.Errorf("user_display_preferences_es: %w", err)
	}
	return nil
}

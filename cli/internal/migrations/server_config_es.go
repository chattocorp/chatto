package migrations

import (
	"context"
	"errors"
	"fmt"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	configv1 "hmans.de/chatto/internal/pb/chatto/config/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// MigrateServerConfigToES seeds the EVT stream from the existing
// config.instance entry in INSTANCE_CONFIG (ADR-035 phase 3 for the
// server-config aggregate).
//
// On a deployment that has at least one operator-saved config, this emits
// generic ConfigValueSetEvent entries on evt.config.server.value_set. The KV
// entry's Created() timestamp is preserved as each event's created_at so the
// audit log dates the seed events correctly.
//
// On a fresh deployment with no INSTANCE_CONFIG entry, this is a
// no-op (returns nil without emitting anything).
//
// # Idempotency
//
// Replay-safe via wildcard OCC: the batch expects evt.config.server.> to be
// empty. If the aggregate already has events, events.ErrConflict is treated as
// a deliberate skip.
//
// # When this can be removed
//
// Once every live deployment has booted at least once on a version
// that includes this migration AND ADR-035 phase 7 (decommission
// the legacy INSTANCE_CONFIG KV entry) has shipped.
func MigrateServerConfigToES(
	ctx context.Context,
	runtimeConfigKV jetstream.KeyValue,
	publisher *events.Publisher,
	logger *log.Logger,
) error {
	entry, err := runtimeConfigKV.Get(ctx, "config.instance")
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil
		}
		return fmt.Errorf("read legacy server config: %w", err)
	}

	cfg := &configv1.ServerConfig{}
	if err := proto.Unmarshal(entry.Value(), cfg); err != nil {
		return fmt.Errorf("unmarshal legacy server config: %w", err)
	}

	agg := events.ConfigAggregate()
	createdAt := timestamppb.New(entry.Created())
	writes := []struct {
		path  string
		value string
	}{
		{path: "server.name", value: cfg.GetServerName()},
		{path: "server.description", value: cfg.GetDescription()},
		{path: "server.welcome_message", value: cfg.GetWelcomeMessage()},
		{path: "server.motd", value: cfg.GetMotd()},
		{path: "auth.blocked_usernames", value: cfg.GetBlockedUsernames()},
	}
	batch := make([]events.BatchEntry, 0, len(writes))
	for i, write := range writes {
		event := &corev1.Event{
			Id:        newMigrationEventID(),
			ActorId:   "system:migration",
			CreatedAt: createdAt,
			Event: &corev1.Event_ConfigValueSet{
				ConfigValueSet: &corev1.ConfigValueSetEvent{
					Subject: events.ConfigSingletonID,
					Path:    write.path,
					Value: &configv1.ConfigValue{
						Value: &configv1.ConfigValue_StringValue{StringValue: write.value},
					},
				},
			},
		}
		batchEntry := events.BatchEntry{
			Subject: agg.Subject(events.EventConfigValueSet),
			Event:   event,
		}
		if i == 0 {
			batchEntry.ExpectedSeq = 0
			batchEntry.FilterSubject = agg.AllEventsFilter()
			batchEntry.HasOCC = true
		}
		batch = append(batch, batchEntry)
	}

	_, err = publisher.AppendBatch(ctx, batch)
	if err == nil {
		logger.Info("server_config ES migration: seeded generic config events from legacy KV", "subject", agg.Subject(events.EventConfigValueSet), "values", len(batch))
		return nil
	}
	if errors.Is(err, events.ErrConflict) {
		// EVT already has events on this aggregate — a previous
		// migration run (or a runtime publish) populated it. Skip.
		return nil
	}
	return fmt.Errorf("seed generic server config events: %w", err)
}

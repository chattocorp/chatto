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
// semantic server config events. The KV entry's Created() timestamp is
// preserved as each event's created_at so the audit log dates the seed events
// correctly.
//
// On a fresh deployment with no INSTANCE_CONFIG entry, this is a
// no-op (returns nil without emitting anything).
//
// # Idempotency
//
// Replay-safe by event type: already-seen semantic fields are skipped, while
// missing fields are appended with wildcard OCC against evt.config.server.>.
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
	existingEvents, lastSeq, err := configSubjectEvents(ctx, publisher, events.ConfigSingletonID)
	if err != nil {
		return fmt.Errorf("read existing server config events: %w", err)
	}
	seen := make(map[string]struct{})
	for _, event := range existingEvents {
		if typ := events.EventTypeOf(event); typ != "" {
			seen[typ] = struct{}{}
		}
	}

	cfg, createdAt := latestLegacyServerConfigSnapshot(existingEvents)
	if cfg == nil {
		entry, err := runtimeConfigKV.Get(ctx, "config.instance")
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				return nil
			}
			return fmt.Errorf("read legacy server config: %w", err)
		}

		cfg = &configv1.ServerConfig{}
		if err := proto.Unmarshal(entry.Value(), cfg); err != nil {
			return fmt.Errorf("unmarshal legacy server config: %w", err)
		}
		createdAt = timestamppb.New(entry.Created())
	}

	agg := events.ConfigAggregate()
	legacyEvents := []*corev1.Event{
		{
			Id:        newMigrationEventID(),
			ActorId:   "system:migration",
			CreatedAt: createdAt,
			Event: &corev1.Event_ServerNameChanged{
				ServerNameChanged: &corev1.ServerNameChangedEvent{Name: cfg.GetServerName()},
			},
		},
		{
			Id:        newMigrationEventID(),
			ActorId:   "system:migration",
			CreatedAt: createdAt,
			Event: &corev1.Event_ServerDescriptionChanged{
				ServerDescriptionChanged: &corev1.ServerDescriptionChangedEvent{Description: cfg.GetDescription()},
			},
		},
		{
			Id:        newMigrationEventID(),
			ActorId:   "system:migration",
			CreatedAt: createdAt,
			Event: &corev1.Event_ServerWelcomeMessageChanged{
				ServerWelcomeMessageChanged: &corev1.ServerWelcomeMessageChangedEvent{WelcomeMessage: cfg.GetWelcomeMessage()},
			},
		},
		{
			Id:        newMigrationEventID(),
			ActorId:   "system:migration",
			CreatedAt: createdAt,
			Event: &corev1.Event_ServerMotdChanged{
				ServerMotdChanged: &corev1.ServerMotdChangedEvent{Motd: cfg.GetMotd()},
			},
		},
		{
			Id:        newMigrationEventID(),
			ActorId:   "system:migration",
			CreatedAt: createdAt,
			Event: &corev1.Event_ServerBlockedUsernamesChanged{
				ServerBlockedUsernamesChanged: &corev1.ServerBlockedUsernamesChangedEvent{BlockedUsernames: cfg.GetBlockedUsernames()},
			},
		},
	}
	batch := make([]events.BatchEntry, 0, len(legacyEvents))
	for _, event := range legacyEvents {
		if _, ok := seen[events.EventTypeOf(event)]; ok {
			continue
		}
		batchEntry := events.BatchEntry{
			Subject: agg.SubjectFor(event),
			Event:   event,
		}
		if len(batch) == 0 {
			batchEntry.ExpectedSeq = lastSeq
			batchEntry.FilterSubject = agg.AllEventsFilter()
			batchEntry.HasOCC = true
		}
		batch = append(batch, batchEntry)
	}
	if len(batch) == 0 {
		return nil
	}

	_, err = publisher.AppendBatch(ctx, batch)
	if err == nil {
		logger.Info("server_config ES migration: seeded semantic config events from legacy KV", "values", len(batch))
		return nil
	}
	if errors.Is(err, events.ErrConflict) {
		// EVT already has events on this aggregate — a previous
		// migration run (or a runtime publish) populated it. Skip.
		return nil
	}
	return fmt.Errorf("seed semantic server config events: %w", err)
}

func latestLegacyServerConfigSnapshot(existingEvents []*corev1.Event) (*configv1.ServerConfig, *timestamppb.Timestamp) {
	for i := len(existingEvents) - 1; i >= 0; i-- {
		event := existingEvents[i]
		change := event.GetServerConfigChanged()
		if change == nil {
			continue
		}
		createdAt := event.GetCreatedAt()
		if createdAt == nil {
			createdAt = timestamppb.Now()
		}
		cfg := change.GetConfig()
		if cfg == nil {
			cfg = &configv1.ServerConfig{}
		}
		return proto.Clone(cfg).(*configv1.ServerConfig), createdAt
	}
	return nil, nil
}

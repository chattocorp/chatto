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

// MigrateServerConfigToES seeds the EVT stream from the
// existing config.instance entry in INSTANCE_CONFIG (ADR-035 phase 3
// for the server-config aggregate).
//
// On a deployment that has at least one operator-saved config, this
// emits exactly one ServerConfigChangedEvent on evt.config.server
// carrying the current snapshot. The KV entry's Created() timestamp
// is preserved as the event's created_at so the audit log dates the
// seed event correctly.
//
// On a fresh deployment with no INSTANCE_CONFIG entry, this is a
// no-op (returns nil without emitting anything).
//
// # Idempotency
//
// Replay-safe via OCC: AppendAt(seq=0) on evt.config.server hits
// events.ErrConflict if the aggregate already has events, and we
// treat that as a deliberate skip.
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

	event := &corev1.Event{
		Id:        newMigrationEventID(),
		ActorId:   "system:migration",
		CreatedAt: timestamppb.New(entry.Created()),
		Event: &corev1.Event_ServerConfigChanged{
			ServerConfigChanged: &corev1.ServerConfigChangedEvent{
				Config: cfg,
			},
		},
	}

	// Wildcard OCC against the aggregate's full filter — "aggregate
	// must be empty" (idempotent replay: any prior config event →
	// ErrConflict → no-op).
	agg := events.ConfigAggregate()
	subject := agg.SubjectFor(event)
	_, err = publisher.AppendAtFilter(ctx, subject, event, agg.AllEventsFilter(), 0)
	if err == nil {
		logger.Info("server_config ES migration: seeded event from legacy KV", "subject", subject)
		return nil
	}
	if errors.Is(err, events.ErrConflict) {
		// EVT already has events on this aggregate — a previous
		// migration run (or a runtime publish) populated it. Skip.
		return nil
	}
	return fmt.Errorf("seed ServerConfigChangedEvent: %w", err)
}

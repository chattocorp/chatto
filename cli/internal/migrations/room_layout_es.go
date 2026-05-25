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
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// MigrateRoomLayoutToES seeds the EVT stream with one
// RoomGroupsReorderedEvent on evt.layout.default carrying the
// operator-defined inter-group ordering that used to live in the
// `room_layout` KV doc (ADR-035 phase 3 for the layout aggregate).
//
// # Idempotency
//
// AppendAt(seq=0) on the singleton subject is the OCC checkpoint.
// On second and subsequent boots the event is already present, the
// publish returns ErrConflict, and this function no-ops.
//
// # Empty / missing layout
//
// On a brand-new deployment there's no room_layout KV record; this
// function returns nil. The RoomLayoutProjection stays empty and the
// reconciler in ListRoomGroupsOrdered appends groups by NanoID order
// until an operator does an explicit reorder.
//
// Legacy `legacy_sections` / `legacy_unsorted_room_ids` fields on the
// layout doc are intentionally NOT drained here — they were a
// pre-ADR-031 shape that has long since been migrated away on every
// live deployment, and ADR-031 itself didn't ship rollback support
// for that shape. If a truly ancient deployment somehow still carries
// those fields, the operator will need to re-create their groups
// manually post-upgrade.
//
// # When this can be removed
//
// Once every live deployment has booted at least once on a version
// that includes this migration AND ADR-035 phase 7 (decommission
// the legacy room_layout KV) has shipped.
func MigrateRoomLayoutToES(
	ctx context.Context,
	serverConfigKV jetstream.KeyValue,
	publisher *events.Publisher,
	logger *log.Logger,
) error {
	entry, err := serverConfigKV.Get(ctx, "room_layout")
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil
		}
		return fmt.Errorf("get room_layout: %w", err)
	}

	var layout corev1.RoomLayout
	if err := proto.Unmarshal(entry.Value(), &layout); err != nil {
		logger.Warn("room_layout ES migration: skipping unmarshalable record", "error", err)
		return nil
	}

	if len(layout.GetGroupIds()) == 0 {
		return nil
	}

	event := &corev1.Event{
		Id:        newMigrationEventID(),
		ActorId:   "system:migration",
		CreatedAt: timestamppb.New(entry.Created()),
		Event: &corev1.Event_RoomGroupsReordered{
			RoomGroupsReordered: &corev1.RoomGroupsReorderedEvent{
				GroupIds: layout.GetGroupIds(),
			},
		},
	}

	// Wildcard OCC against the aggregate's full filter — "aggregate
	// must be empty" (idempotent replay: any prior layout event →
	// ErrConflict → no-op).
	agg := events.LayoutAggregate()
	if _, err := publisher.AppendAtFilter(ctx, agg.SubjectFor(event), event, agg.AllEventsFilter(), 0); err != nil {
		if errors.Is(err, events.ErrConflict) {
			return nil
		}
		return fmt.Errorf("seed RoomGroupsReorderedEvent: %w", err)
	}

	logger.Info("room_layout ES migration: seeded layout ordering",
		"group_count", len(layout.GetGroupIds()))
	return nil
}

package migrations

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// MigrateRoomGroupsToES seeds the EVT stream from the existing
// room_group.{groupID} keys in SERVER_CONFIG (ADR-035 phase 3 for
// the group aggregate).
//
// For each group: emits one RoomGroupCreatedEvent on evt.group.{G}
// carrying id/name/description, followed by one
// RoomAddedToGroupEvent per room in the group's ordered room_ids
// list. The Append-not-AppendAt path is used for the room-add
// events so the publisher computes per-aggregate expected-seq
// itself.
//
// # Idempotency
//
// First AppendAt(seq=0) for RoomGroupCreated on each aggregate is
// the OCC checkpoint. If that conflicts (the group is already on
// the stream), the entire aggregate's seed is skipped — we don't
// attempt to "catch up" partial state.
//
// # When this can be removed
//
// Once every live deployment has booted at least once on a version
// that includes this migration AND ADR-035 phase 7 (decommission
// the legacy room_group KV) has shipped.
func MigrateRoomGroupsToES(
	ctx context.Context,
	serverConfigKV jetstream.KeyValue,
	publisher *events.Publisher,
	logger *log.Logger,
) error {
	kl, err := serverConfigKV.ListKeysFiltered(ctx, "room_group.*")
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil
		}
		return fmt.Errorf("list room_group keys: %w", err)
	}

	var allKeys []string
	for key := range kl.Keys() {
		allKeys = append(allKeys, key)
	}
	sort.Strings(allKeys)

	var migrated, skipped, memberEvents int
	for _, key := range allKeys {
		entry, err := serverConfigKV.Get(ctx, key)
		if err != nil {
			logger.Warn("room_groups ES migration: skipping unfetchable entry", "key", key, "error", err)
			continue
		}

		var group corev1.RoomGroup
		if err := proto.Unmarshal(entry.Value(), &group); err != nil {
			logger.Warn("room_groups ES migration: skipping unmarshalable entry", "key", key, "error", err)
			continue
		}

		agg := events.GroupAggregate(group.GetId())
		createdAt := timestamppb.New(entry.Created())

		// Atomic batch: RoomGroupCreated first (wildcard OCC ensures
		// the aggregate is empty — preserves the per-aggregate
		// uniqueness guarantee under the per-(agg, event-type) subject
		// shape), then one RoomAddedToGroupEvent per room.
		created := &corev1.Event{
			Id:        newMigrationEventID(),
			ActorId:   "system:migration",
			CreatedAt: createdAt,
			Event: &corev1.Event_RoomGroupCreated{
				RoomGroupCreated: &corev1.RoomGroupCreatedEvent{
					GroupId:     group.GetId(),
					Name:        group.GetName(),
					Description: group.GetDescription(),
				},
			},
		}
		batch := []events.BatchEntry{{
			Subject:       agg.SubjectFor(created),
			Event:         created,
			HasOCC:        true,
			FilterSubject: agg.AllEventsFilter(),
		}}
		for _, roomID := range group.GetRoomIds() {
			add := &corev1.Event{
				Id:        newMigrationEventID(),
				ActorId:   "system:migration",
				CreatedAt: createdAt,
				Event: &corev1.Event_RoomAddedToGroup{
					RoomAddedToGroup: &corev1.RoomAddedToGroupEvent{
						GroupId: group.GetId(),
						RoomId:  roomID,
					},
				},
			}
			batch = append(batch, events.BatchEntry{
				Subject: agg.SubjectFor(add),
				Event:   add,
			})
		}

		if _, err := publisher.AppendBatch(ctx, batch); err != nil {
			if errors.Is(err, events.ErrConflict) {
				skipped++
				continue
			}
			return fmt.Errorf("seed room group %s: %w", group.GetId(), err)
		}
		migrated++
		memberEvents += len(group.GetRoomIds())
	}

	if migrated > 0 || memberEvents > 0 {
		logger.Info(
			"room_groups ES migration: seeded events from legacy KV",
			"groups_migrated", migrated,
			"groups_skipped", skipped,
			"room_memberships_emitted", memberEvents,
		)
	}
	return nil
}

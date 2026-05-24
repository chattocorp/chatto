package migrations

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// MigrateRoomMetadataToES seeds the EVT stream from the existing
// room.{kind}.{roomID} keys in SERVER_CONFIG (ADR-035 phase 3 for
// the room-metadata side of the room aggregate).
//
// For each room: emits one RoomCreatedEvent on evt.room.{R} carrying
// the room's id, name, description, and kind. Archived rooms also
// emit a RoomArchivedEvent so the projection's archived state
// matches the KV value.
//
// # Idempotency
//
// Each aggregate is checked OCC-style via AppendAt(seq=0). Already-
// migrated rooms (those with any prior event on evt.room.{R} —
// including a UserJoinedRoom from the membership migration that
// ran earlier) hit ErrConflict; we treat that as "this aggregate is
// already on the stream, skip the metadata seed."
//
// IMPORTANT: this migration must run AFTER MigrateRoomMembershipToES
// because the membership migration may have populated evt.room.{R}
// with UserJoinedRoom events. In that case the OCC scope of the
// room aggregate is already non-empty, so AppendAt(seq=0) for
// RoomCreated would conflict — that's the expected "skip" path.
// Future deployments that migrate fresh will see RoomCreated first.
//
// # When this can be removed
//
// Once every live deployment has booted at least once on a version
// that includes this migration AND ADR-035 phase 7 (decommission
// the legacy room KV) has shipped.
func MigrateRoomMetadataToES(
	ctx context.Context,
	serverConfigKV jetstream.KeyValue,
	publisher *events.Publisher,
	logger *log.Logger,
) error {
	kl, err := serverConfigKV.ListKeysFiltered(ctx, "room.channel.*", "room.dm.*")
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil
		}
		return fmt.Errorf("list room keys: %w", err)
	}

	var allKeys []string
	for key := range kl.Keys() {
		allKeys = append(allKeys, key)
	}
	sort.Strings(allKeys)

	var migrated, skipped, archivedEmitted int
	for _, key := range allKeys {
		parts := strings.Split(key, ".")
		if len(parts) != 3 {
			logger.Warn("room_metadata ES migration: skipping malformed key", "key", key)
			continue
		}
		// parts[0]="room", parts[1]=kind, parts[2]=roomID

		entry, err := serverConfigKV.Get(ctx, key)
		if err != nil {
			logger.Warn("room_metadata ES migration: skipping unfetchable entry", "key", key, "error", err)
			continue
		}

		var room corev1.Room
		if err := proto.Unmarshal(entry.Value(), &room); err != nil {
			logger.Warn("room_metadata ES migration: skipping unmarshalable entry", "key", key, "error", err)
			continue
		}

		subject := events.RoomAggregate(room.GetId()).Subject()
		createdAt := timestamppb.New(entry.Created())

		created := &corev1.Event{
			Id:        newMigrationEventID(),
			ActorId:   "system:migration",
			CreatedAt: createdAt,
			Event: &corev1.Event_RoomCreated{
				RoomCreated: &corev1.RoomCreatedEvent{
					RoomId:      room.GetId(),
					Name:        room.GetName(),
					Description: room.GetDescription(),
					Kind:        room.GetKind(),
				},
			},
		}

		// AppendAt(seq=0) — if any prior event already exists on this
		// room's subject (e.g. the membership migration emitted a
		// UserJoinedRoom), this conflicts and we skip the rest of
		// this room's metadata seed.
		_, err = publisher.AppendAt(ctx, subject, created, 0)
		if err != nil {
			if errors.Is(err, events.ErrConflict) {
				skipped++
				continue
			}
			return fmt.Errorf("seed RoomCreatedEvent for %s: %w", room.GetId(), err)
		}
		migrated++

		// If the room was archived, follow up with a RoomArchivedEvent
		// so the projection's archived bit matches KV. expectedSeq=1
		// because we just placed one event on the aggregate.
		if room.GetArchived() {
			archived := &corev1.Event{
				Id:        newMigrationEventID(),
				ActorId:   "system:migration",
				CreatedAt: createdAt,
				Event: &corev1.Event_RoomArchived{
					RoomArchived: &corev1.RoomArchivedEvent{RoomId: room.GetId()},
				},
			}
			// Use the seq returned from the RoomCreated publish — that's
			// the canonical expected last seq for the next AppendAt on
			// this subject.
			// (We can't capture it from above since AppendAt only
			// returns it on success — that's fine here; the next call
			// computes it.)
			_, err := publisher.Append(ctx, subject, archived)
			if err != nil {
				return fmt.Errorf("seed RoomArchivedEvent for %s: %w", room.GetId(), err)
			}
			archivedEmitted++
		}
	}

	if migrated > 0 || archivedEmitted > 0 {
		logger.Info(
			"room_metadata ES migration: seeded events from legacy KV",
			"rooms_migrated", migrated,
			"rooms_skipped", skipped,
			"archived_events", archivedEmitted,
		)
	}
	return nil
}

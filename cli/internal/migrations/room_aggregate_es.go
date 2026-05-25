package migrations

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	gonanoid "github.com/matoous/go-nanoid/v2"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// MigrateRoomAggregateToES seeds the EVT stream from the existing
// `room.{kind}.{roomID}` and `room_membership.{kind}.{roomID}.{userID}`
// keys in SERVER_CONFIG (ADR-035 phase 3 for the room aggregate).
//
// Room metadata and membership share one event subject — `evt.room.{R}` —
// so they must seed together: a `RoomCreatedEvent` must always be the
// first event on the subject, with optional `RoomArchivedEvent` and the
// chronologically-ordered `UserJoinedRoomEvent`s following. This is
// emitted as a single atomic AppendBatch so the projection never observes
// a partial seed (and so a crash mid-batch can't leave a room whose
// `RoomCreated` is missing).
//
// # Idempotency
//
// Each batch's first entry uses `HasOCC: true` + `ExpectedSeq: 0`. On
// re-run, the publish fails with events.ErrConflict and the room is
// skipped wholesale — we don't try to "catch up" partial state.
//
// # When this can be removed
//
// Once every live deployment has booted at least once on a version that
// includes this migration AND ADR-035 phase 7 (decommission the legacy
// room + room_membership KV keys) has shipped.
func MigrateRoomAggregateToES(
	ctx context.Context,
	serverConfigKV jetstream.KeyValue,
	publisher *events.Publisher,
	logger *log.Logger,
) error {
	roomKeys, err := listSortedKeys(ctx, serverConfigKV, "room.channel.*", "room.dm.*")
	if err != nil {
		return fmt.Errorf("list room keys: %w", err)
	}

	memberships, err := loadMembershipsByRoom(ctx, serverConfigKV, logger)
	if err != nil {
		return fmt.Errorf("load memberships: %w", err)
	}

	var migrated, skipped, archivedEvents, memberEvents int
	for _, key := range roomKeys {
		entry, err := serverConfigKV.Get(ctx, key)
		if err != nil {
			logger.Warn("room_aggregate ES migration: skipping unfetchable entry", "key", key, "error", err)
			continue
		}

		var room corev1.Room
		if err := proto.Unmarshal(entry.Value(), &room); err != nil {
			logger.Warn("room_aggregate ES migration: skipping unmarshalable entry", "key", key, "error", err)
			continue
		}

		agg := events.RoomAggregate(room.GetId())
		roomCreatedAt := timestamppb.New(entry.Created())

		// systemEvent stamps Id/ActorId/CreatedAt onto a caller-built
		// shell so the per-event boilerplate stays out of the batch
		// construction below. Closures over `roomCreatedAt` so each
		// room's migration events share the room's creation time.
		systemEvent := func(body *corev1.Event) *corev1.Event {
			return stamp(body, "system:migration", roomCreatedAt)
		}

		// First batch entry uses wildcard OCC on the aggregate's full
		// filter — "the aggregate must be empty," not just "no prior
		// RoomCreated event." Preserves the per-aggregate uniqueness
		// guarantee under the per-(agg, event-type) subject shape and
		// keeps replay idempotency intact (any prior event on the
		// aggregate → ErrConflict → skip).
		createdEvent := systemEvent(&corev1.Event{Event: &corev1.Event_RoomCreated{
			RoomCreated: &corev1.RoomCreatedEvent{
				RoomId:      room.GetId(),
				Name:        room.GetName(),
				Description: room.GetDescription(),
				Kind:        room.GetKind(),
			},
		}})
		batch := []events.BatchEntry{{
			Subject:       agg.SubjectFor(createdEvent),
			Event:         createdEvent,
			HasOCC:        true,
			FilterSubject: agg.AllEventsFilter(),
		}}

		if room.GetArchived() {
			archivedEvent := systemEvent(&corev1.Event{Event: &corev1.Event_RoomArchived{
				RoomArchived: &corev1.RoomArchivedEvent{RoomId: room.GetId()},
			}})
			batch = append(batch, events.BatchEntry{
				Subject: agg.SubjectFor(archivedEvent),
				Event:   archivedEvent,
			})
		}

		for _, m := range memberships[room.GetId()] {
			joinedEvent := stamp(&corev1.Event{Event: &corev1.Event_UserJoinedRoom{
				UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: room.GetId()},
			}}, m.userID, timestamppb.New(m.createdAt))
			batch = append(batch, events.BatchEntry{
				Subject: agg.SubjectFor(joinedEvent),
				Event:   joinedEvent,
			})
		}

		if _, err := publisher.AppendBatch(ctx, batch); err != nil {
			if errors.Is(err, events.ErrConflict) {
				skipped++
				continue
			}
			return fmt.Errorf("seed room aggregate for %s: %w", room.GetId(), err)
		}

		migrated++
		if room.GetArchived() {
			archivedEvents++
		}
		memberEvents += len(memberships[room.GetId()])
	}

	if migrated > 0 || skipped > 0 {
		logger.Info(
			"room_aggregate ES migration: seeded events from legacy KV",
			"rooms_migrated", migrated,
			"rooms_skipped", skipped,
			"archived_events", archivedEvents,
			"member_events", memberEvents,
		)
	}
	return nil
}

// membershipEntry pairs a userID with the KV-recorded creation time of
// its room_membership entry. Used to order UserJoinedRoom events
// chronologically within each room's seed batch.
type membershipEntry struct {
	userID    string
	createdAt time.Time
}

// loadMembershipsByRoom reads every `room_membership.>` key and groups
// the entries by roomID, sorted chronologically (with userID as a
// deterministic tiebreaker). Orphan memberships whose key is malformed
// are logged and skipped.
func loadMembershipsByRoom(
	ctx context.Context,
	serverConfigKV jetstream.KeyValue,
	logger *log.Logger,
) (map[string][]membershipEntry, error) {
	keys, err := listSortedKeys(ctx, serverConfigKV, "room_membership.>")
	if err != nil {
		return nil, err
	}

	byRoom := make(map[string][]membershipEntry)
	for _, key := range keys {
		// Key shape: room_membership.{kind}.{roomID}.{userID}.
		parts := strings.Split(key, ".")
		if len(parts) != 4 {
			logger.Warn("room_aggregate ES migration: skipping malformed membership key", "key", key)
			continue
		}
		roomID, userID := parts[2], parts[3]

		entry, err := serverConfigKV.Get(ctx, key)
		if err != nil {
			logger.Warn("room_aggregate ES migration: skipping unfetchable membership", "key", key, "error", err)
			continue
		}
		byRoom[roomID] = append(byRoom[roomID], membershipEntry{userID: userID, createdAt: entry.Created()})
	}

	for roomID, ms := range byRoom {
		sort.Slice(ms, func(i, j int) bool {
			if !ms[i].createdAt.Equal(ms[j].createdAt) {
				return ms[i].createdAt.Before(ms[j].createdAt)
			}
			return ms[i].userID < ms[j].userID
		})
		byRoom[roomID] = ms
	}
	return byRoom, nil
}

// listSortedKeys returns the union of keys matching the given filters,
// sorted lexicographically. Treats jetstream.ErrNoKeysFound as an empty
// result so callers don't have to.
func listSortedKeys(ctx context.Context, kv jetstream.KeyValue, filters ...string) ([]string, error) {
	kl, err := kv.ListKeysFiltered(ctx, filters...)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for key := range kl.Keys() {
		out = append(out, key)
	}
	sort.Strings(out)
	return out, nil
}

// stamp populates Id/ActorId/CreatedAt on a caller-built event shell
// and returns it. Lets call sites build a one-field `&corev1.Event{Event: ...}`
// without restating the boilerplate three times.
func stamp(e *corev1.Event, actorID string, createdAt *timestamppb.Timestamp) *corev1.Event {
	e.Id = newMigrationEventID()
	e.ActorId = actorID
	e.CreatedAt = createdAt
	return e
}

// newMigrationEventID generates an event ID with the standard "E"
// prefix used by core.NewEventID, kept inline here to avoid pulling
// the migrations package into a dependency on core.
func newMigrationEventID() string {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	id, err := gonanoid.Generate(alphabet, 14)
	if err != nil {
		// Generation only fails on RNG failure, which never happens
		// in practice. Same fatal posture as core.newID.
		panic("migrations: failed to generate event ID: " + err.Error())
	}
	return "E" + id
}

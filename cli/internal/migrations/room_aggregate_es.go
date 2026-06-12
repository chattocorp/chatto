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

type legacyRoomMembershipEvents map[string]map[string]*corev1.Event

// MigrateRoomAggregateToES seeds the EVT stream from the existing
// `room.{kind}.{roomID}` keys and both known room membership key shapes
// in SERVER_CONFIG (ADR-035 phase 3 for the room aggregate):
// `room_membership.{kind}.{roomID}.{userID}` and the older
// `room_membership.{userID}.{roomID}`.
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
// re-run, the publish fails with events.ErrConflict. The room metadata is
// then treated as already seeded, but missing membership events are
// backfilled so older membership key shapes discovered after a failed boot
// are not stranded.
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
	legacyServerEvents ...jetstream.Stream,
) error {
	roomKeys, err := listSortedKeys(ctx, serverConfigKV, "room.channel.*", "room.dm.*")
	if err != nil {
		return fmt.Errorf("list room keys: %w", err)
	}

	memberships, err := loadMembershipsByRoom(ctx, serverConfigKV, logger)
	if err != nil {
		return fmt.Errorf("load memberships: %w", err)
	}

	legacyJoins, err := loadLegacyRoomMembershipEvents(ctx, firstLegacyStream(legacyServerEvents), logger)
	if err != nil {
		return fmt.Errorf("load legacy room membership events: %w", err)
	}
	applyLegacyMembershipEvents(memberships, legacyJoins)

	var migrated, skipped, archivedEvents, memberEvents, memberBackfillEvents int
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
			joinedEvent := m.joinEvent(room.GetId())
			batch = append(batch, events.BatchEntry{
				Subject: agg.SubjectFor(joinedEvent),
				Event:   joinedEvent,
			})
		}

		if _, err := publisher.AppendBatch(ctx, batch); err != nil {
			if errors.Is(err, events.ErrConflict) {
				backfilled, backfillErr := backfillMissingRoomMemberships(ctx, publisher, room.GetId(), memberships[room.GetId()])
				if backfillErr != nil {
					return fmt.Errorf("backfill room memberships for %s: %w", room.GetId(), backfillErr)
				}
				memberBackfillEvents += backfilled
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
			"member_backfill_events", memberBackfillEvents,
		)
	}
	return nil
}

func backfillMissingRoomMemberships(
	ctx context.Context,
	publisher *events.Publisher,
	roomID string,
	memberships []membershipEntry,
) (int, error) {
	if len(memberships) == 0 {
		return 0, nil
	}

	agg := events.RoomAggregate(roomID)
	subject := agg.Subject(events.EventUserJoinedRoom)
	existingEvents, expectedSeq, err := publisher.SubjectEvents(ctx, subject)
	if err != nil {
		return 0, fmt.Errorf("read existing membership events: %w", err)
	}

	existing := make(map[string]bool, len(existingEvents))
	for _, event := range existingEvents {
		if event.GetUserJoinedRoom() == nil {
			continue
		}
		existing[event.GetActorId()] = true
	}

	var imported int
	for _, membership := range memberships {
		if existing[membership.userID] {
			continue
		}

		event := membership.joinEvent(roomID)

		seq, err := publisher.AppendAt(ctx, subject, event, expectedSeq)
		if err != nil {
			return imported, err
		}
		expectedSeq = seq
		imported++
	}
	return imported, nil
}

// membershipEntry pairs a userID with the KV-recorded creation time of
// its room_membership entry. Used to order UserJoinedRoom events
// chronologically within each room's seed batch.
type membershipEntry struct {
	userID      string
	createdAt   time.Time
	legacyEvent *corev1.Event
}

func (m membershipEntry) joinEvent(roomID string) *corev1.Event {
	if m.legacyEvent != nil {
		return proto.Clone(m.legacyEvent).(*corev1.Event)
	}
	return stamp(&corev1.Event{Event: &corev1.Event_UserJoinedRoom{
		UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: roomID},
	}}, m.userID, timestamppb.New(m.createdAt))
}

// loadMembershipsByRoom reads every `room_membership.>` key and groups
// the entries by roomID, sorted chronologically (with userID as a
// deterministic tiebreaker). It accepts both the current
// `room_membership.{kind}.{roomID}.{userID}` shape and the old
// `room_membership.{userID}.{roomID}` shape.
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
		roomID, userID, ok := parseRoomMembershipKey(key)
		if !ok {
			logger.Warn("room_aggregate ES migration: skipping malformed membership key", "key", key)
			continue
		}

		entry, err := serverConfigKV.Get(ctx, key)
		if err != nil {
			logger.Warn("room_aggregate ES migration: skipping unfetchable membership", "key", key, "error", err)
			continue
		}
		membership := membershipEntry{userID: userID, createdAt: entry.Created()}
		members := byRoom[roomID]
		duplicate := false
		for i, existing := range members {
			if existing.userID != userID {
				continue
			}
			duplicate = true
			if membership.createdAt.Before(existing.createdAt) {
				members[i] = membership
			}
			break
		}
		if duplicate {
			byRoom[roomID] = members
			continue
		}
		byRoom[roomID] = append(members, membership)
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

func parseRoomMembershipKey(key string) (roomID string, userID string, ok bool) {
	parts := strings.Split(key, ".")
	switch len(parts) {
	case 4:
		if parts[0] != "room_membership" {
			return "", "", false
		}
		return parts[2], parts[3], true
	case 3:
		if parts[0] != "room_membership" {
			return "", "", false
		}
		return parts[2], parts[1], true
	default:
		return "", "", false
	}
}

func firstLegacyStream(streams []jetstream.Stream) jetstream.Stream {
	if len(streams) == 0 {
		return nil
	}
	return streams[0]
}

func loadLegacyRoomMembershipEvents(
	ctx context.Context,
	stream jetstream.Stream,
	logger *log.Logger,
) (legacyRoomMembershipEvents, error) {
	if stream == nil {
		return nil, nil
	}

	consumer, err := stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		FilterSubjects:    []string{"server.room.*.*.meta"},
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		AckPolicy:         jetstream.AckNonePolicy,
		MemoryStorage:     true,
		InactiveThreshold: 30 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("create legacy room membership consumer on SERVER_EVENTS: %w", err)
	}
	defer stream.DeleteConsumer(context.Background(), consumer.CachedInfo().Name)

	info, err := consumer.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("get legacy room membership consumer info: %w", err)
	}
	if info.NumPending == 0 {
		return nil, nil
	}

	msgs, err := consumer.Fetch(int(info.NumPending), jetstream.FetchMaxWait(60*time.Second))
	if err != nil && !errors.Is(err, jetstream.ErrNoMessages) {
		return nil, fmt.Errorf("fetch legacy room membership events: %w", err)
	}
	if msgs == nil {
		return nil, nil
	}

	byRoom := make(legacyRoomMembershipEvents)
	for msg := range msgs.Messages() {
		var legacyEvent corev1.Event
		if err := proto.Unmarshal(msg.Data(), &legacyEvent); err != nil {
			logger.Warn("room_aggregate ES migration: skipping unmarshalable legacy room meta event", "subject", msg.Subject(), "error", err)
			continue
		}

		join := legacyEvent.GetUserJoinedRoom()
		if join == nil {
			continue
		}
		roomID := join.GetRoomId()
		userID := legacyEvent.GetActorId()
		if roomID == "" || userID == "" {
			logger.Warn("room_aggregate ES migration: skipping legacy room join with missing room or actor", "subject", msg.Subject(), "room_id", roomID, "actor_id", userID)
			continue
		}

		legacyEvent.CreatedAt = preserveTimestamp(legacyEvent.GetCreatedAt())
		if legacyEvent.GetId() == "" {
			legacyEvent.Id = newMigrationEventID()
		}

		roomJoins := byRoom[roomID]
		if roomJoins == nil {
			roomJoins = make(map[string]*corev1.Event)
			byRoom[roomID] = roomJoins
		}
		if existing := roomJoins[userID]; existing != nil && legacyEvent.GetCreatedAt().AsTime().Before(existing.GetCreatedAt().AsTime()) {
			continue
		}
		roomJoins[userID] = proto.Clone(&legacyEvent).(*corev1.Event)
	}
	return byRoom, nil
}

func applyLegacyMembershipEvents(
	memberships map[string][]membershipEntry,
	legacyJoins legacyRoomMembershipEvents,
) {
	for roomID, joins := range legacyJoins {
		members := memberships[roomID]
		seen := make(map[string]int, len(members))
		for i, member := range members {
			seen[member.userID] = i
		}

		for userID, event := range joins {
			if idx, ok := seen[userID]; ok {
				members[idx].createdAt = event.GetCreatedAt().AsTime()
				members[idx].legacyEvent = event
			}
		}

		sort.Slice(members, func(i, j int) bool {
			if !members[i].createdAt.Equal(members[j].createdAt) {
				return members[i].createdAt.Before(members[j].createdAt)
			}
			return members[i].userID < members[j].userID
		})
		memberships[roomID] = members
	}
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

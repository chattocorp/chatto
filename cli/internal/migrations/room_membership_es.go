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
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// MigrateRoomMembershipToES seeds the SERVER_EVT stream from the
// existing room_membership.{kind}.{roomID}.{userID} keys in
// SERVER_CONFIG (ADR-035 phase 3 for the room-membership aggregate).
//
// For each (kind, roomID) aggregate it emits one UserJoinedRoomEvent
// per current member into evt.room.{roomID}, using sequential
// expected-last-subject-sequence values starting at 0. KV entry
// creation timestamps are preserved as the event's created_at so the
// audit log doesn't lie about when the join happened.
//
// # Idempotency
//
// Re-running this on a populated SERVER_EVT is a no-op: the first
// AppendAt(seq=0) on each subject hits events.ErrConflict, the rest
// of that subject is skipped, and the function moves on. A crash
// midway through a subject is the only path that returns an error
// instead of skipping (defensive — refuse to silently misorder).
//
// # When this can be removed
//
// Once every live deployment has booted at least once on a version
// that includes this migration AND ADR-035 phase 7 (decommission
// the legacy room_membership KV keys) has shipped. At that point
// there's no more legacy state to migrate from.
func MigrateRoomMembershipToES(
	ctx context.Context,
	serverConfigKV jetstream.KeyValue,
	publisher *events.Publisher,
	logger *log.Logger,
) error {
	kl, err := serverConfigKV.ListKeysFiltered(ctx, "room_membership.>")
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil
		}
		return fmt.Errorf("list room_membership keys: %w", err)
	}

	bySubject := make(map[string][]membershipEntry)

	var allKeys []string
	for key := range kl.Keys() {
		allKeys = append(allKeys, key)
	}
	sort.Strings(allKeys)

	subjectOrder := make([]string, 0)
	seenSubject := make(map[string]struct{})

	for _, key := range allKeys {
		parts := strings.Split(key, ".")
		if len(parts) != 4 {
			logger.Warn("room_membership ES migration: skipping malformed key", "key", key)
			continue
		}
		// parts[0]="room_membership", parts[1]=kind, parts[2]=roomID, parts[3]=userID
		roomID := parts[2]
		userID := parts[3]
		subject := events.RoomAggregate(roomID).Subject()

		entry, err := serverConfigKV.Get(ctx, key)
		if err != nil {
			logger.Warn("room_membership ES migration: skipping unfetchable entry", "key", key, "error", err)
			continue
		}

		if _, ok := seenSubject[subject]; !ok {
			subjectOrder = append(subjectOrder, subject)
			seenSubject[subject] = struct{}{}
		}
		bySubject[subject] = append(bySubject[subject], membershipEntry{
			userID:    userID,
			createdAt: entry.Created(),
		})
	}

	var emitted, migrated, skipped int
	for _, subject := range subjectOrder {
		entries := bySubject[subject]
		// Sort chronologically so the resulting event stream reflects
		// the order joins actually happened. userID is the deterministic
		// tiebreaker for memberships created in the same instant.
		sort.Slice(entries, func(i, j int) bool {
			if !entries[i].createdAt.Equal(entries[j].createdAt) {
				return entries[i].createdAt.Before(entries[j].createdAt)
			}
			return entries[i].userID < entries[j].userID
		})

		n, wasSkipped, err := migrateOneRoomSubject(ctx, publisher, subject, entries)
		if err != nil {
			return fmt.Errorf("migrate %s: %w", subject, err)
		}
		if wasSkipped {
			skipped++
		} else if n > 0 {
			migrated++
			emitted += n
		}
	}

	if migrated > 0 {
		logger.Info(
			"room_membership ES migration: seeded events from legacy KV",
			"aggregates_migrated", migrated,
			"aggregates_skipped", skipped,
			"events_emitted", emitted,
		)
	}
	return nil
}

// membershipEntry pairs a userID with the KV-recorded creation time of
// its room_membership entry.
type membershipEntry struct {
	userID    string
	createdAt time.Time
}

// migrateOneRoomSubject emits Join events for one aggregate. Returns
// (eventsEmitted, skippedBecauseAlreadyMigrated, error).
//
// On a fresh aggregate every event is published with sequential
// expected seqs. On an already-migrated aggregate, the first AppendAt
// hits events.ErrConflict (the subject already has events from a
// previous run); we treat that as a deliberate full-aggregate skip.
// A non-conflict error, or a mid-aggregate conflict (which would mean
// a previous run crashed AND something appended since), is returned
// so the caller can surface it.
func migrateOneRoomSubject(
	ctx context.Context,
	publisher *events.Publisher,
	subject string,
	entries []membershipEntry,
) (int, bool, error) {
	emitted := 0
	roomID := parseRoomIDFromEvtSubject(subject)

	// expectedSeq is the *stream* sequence of the most recent message
	// on this subject, threaded forward across calls. 0 = "no prior
	// message exists." Each AppendAt returns the new stream seq, which
	// becomes the next expectedSeq.
	var expectedSeq uint64
	for i, m := range entries {
		event := &corev1.Event{
			Id:        newMigrationEventID(),
			ActorId:   m.userID,
			CreatedAt: timestamppb.New(m.createdAt),
			Event: &corev1.Event_UserJoinedRoom{
				UserJoinedRoom: &corev1.UserJoinedRoomEvent{
					RoomId: roomID,
				},
			},
		}

		seq, err := publisher.AppendAt(ctx, subject, event, expectedSeq)
		if err == nil {
			emitted++
			expectedSeq = seq
			continue
		}
		if errors.Is(err, events.ErrConflict) {
			if i == 0 {
				return 0, true, nil
			}
			return emitted, false, fmt.Errorf("unexpected mid-aggregate conflict at index %d: %w", i, err)
		}
		return emitted, false, err
	}
	return emitted, false, nil
}

// parseRoomIDFromEvtSubject is a local helper: events.ParseRoomSubject
// is the canonical parser, but we constructed the subject from the
// KV key segment so the prefix check is redundant here.
func parseRoomIDFromEvtSubject(subject string) string {
	roomID, _ := events.ParseRoomSubject(subject)
	return roomID
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

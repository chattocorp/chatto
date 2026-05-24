package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// MigrationStats summarizes what a one-shot ES migration did.
type MigrationStats struct {
	// SubjectsScanned is the number of distinct (kind, roomID) aggregates
	// seen across all KV keys.
	SubjectsScanned int
	// SubjectsMigrated is the number of subjects where at least one event
	// was newly appended to SERVER_EVT.
	SubjectsMigrated int
	// SubjectsSkipped is the number of subjects that already had events
	// in SERVER_EVT (re-run; replayability path).
	SubjectsSkipped int
	// EventsEmitted is the total number of events written to SERVER_EVT.
	EventsEmitted int
}

// MigrateRoomMembership is the one-shot ES migration for room membership
// (ADR-035 phase 3 for this aggregate).
//
// It scans every `room_membership.{kind}.{roomID}.{userID}` key in
// SERVER_CONFIG, groups by (kind, roomID), and for each group emits one
// UserJoinedRoomEvent per member into SERVER_EVT using sequential
// expected-last-subject-sequence values starting at 0.
//
// Replayability: the first event on each subject is published with
// expected seq 0. If that conflicts (events.ErrConflict), the subject is
// already in SERVER_EVT — we skip the rest of the subject and continue.
// Re-running this command on an already-migrated bucket is a no-op.
//
// Determinism: keys are sorted lexicographically before iteration. The
// key shape (kind, roomID, userID in that order) means sort order groups
// by kind, then by room, then by user — which is the order events get
// per-subject sequence numbers. As long as KV state doesn't change
// between runs, the migration emits events in the same order each time.
//
// Timestamps: the source RoomMembership proto doesn't carry a creation
// timestamp, but NATS KV records one on the entry's latest revision.
// We use `entry.Created()` as the event's created_at. For room
// memberships this is effectively "time of join" — JoinRoom is
// idempotent, so a member's KV entry is only Put once in practice.
// Re-joins that did update the entry would shift the timestamp to
// their most recent re-join (rare; acceptable).
//
// Actor: each event's actor_id is set to the user who joined (extracted
// from the KV key's trailing segment). This matches what a live JoinRoom
// would record.
func (c *ChattoCore) MigrateRoomMembership(ctx context.Context) (MigrationStats, error) {
	var stats MigrationStats

	kv := c.storage.serverConfigKV
	kl, err := kv.ListKeysFiltered(ctx, "room_membership.>")
	if err != nil {
		return stats, fmt.Errorf("list room_membership keys: %w", err)
	}

	bySubject := make(map[string][]membershipEntry) // subject → entries

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
			c.logger.Warn("Skipping malformed membership key", "key", key)
			continue
		}
		// parts[0] = "room_membership", parts[1] = kind, parts[2] = roomID, parts[3] = userID
		roomID := parts[2]
		userID := parts[3]
		subject := events.RoomAggregate(roomID).Subject()

		entry, err := kv.Get(ctx, key)
		if err != nil {
			c.logger.Warn("Skipping unfetchable membership entry", "key", key, "error", err)
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

	stats.SubjectsScanned = len(subjectOrder)

	for _, subject := range subjectOrder {
		entries := bySubject[subject]
		// Sort chronologically so the resulting event stream reflects
		// the order joins actually happened. userID is the deterministic
		// tiebreaker for memberships created in the same instant
		// (negligible in practice; matters only for replay determinism).
		sort.Slice(entries, func(i, j int) bool {
			if !entries[i].createdAt.Equal(entries[j].createdAt) {
				return entries[i].createdAt.Before(entries[j].createdAt)
			}
			return entries[i].userID < entries[j].userID
		})

		emitted, skipped, err := c.migrateOneRoomSubject(ctx, subject, entries)
		if err != nil {
			return stats, fmt.Errorf("migrate %s: %w", subject, err)
		}
		if skipped {
			stats.SubjectsSkipped++
		} else if emitted > 0 {
			stats.SubjectsMigrated++
			stats.EventsEmitted += emitted
		}
	}

	return stats, nil
}

// membershipEntry pairs a userID with the KV-recorded creation time of
// its room_membership entry. Used by MigrateRoomMembership to sort
// memberships chronologically before emission.
type membershipEntry struct {
	userID    string
	createdAt time.Time
}

// migrateOneRoomSubject emits Join events for one subject. Returns
// (eventsEmitted, skippedBecauseAlreadyMigrated, error).
//
// On a fresh subject: all events are published with sequential expected
// seqs and emitted == len(members).
//
// On an already-migrated subject: the first AppendAt fails with
// events.ErrConflict (expected seq 0, but the subject already has
// events). We treat that as "skip the rest of this subject" — replay of
// the migration on a populated SERVER_EVT is a deliberate no-op rather
// than an error.
//
// A non-conflict error short-circuits the loop and is returned to the
// caller — the migration command will surface it. Partial progress
// within a subject is fine: the next replay resumes by either (a) all
// events for this subject existing (full skip) or (b) the OCC check
// passing at the right offset (extremely unlikely but harmless).
func (c *ChattoCore) migrateOneRoomSubject(ctx context.Context, subject string, entries []membershipEntry) (int, bool, error) {
	emitted := 0
	// expectedSeq is the *stream* sequence of the most recent message on
	// this subject, threaded forward across calls. 0 = "no prior message
	// exists." Each AppendAt returns the new stream seq, which becomes
	// the next expectedSeq.
	var expectedSeq uint64
	for i, m := range entries {
		event := &corev1.Event{
			Id:        NewEventID(),
			ActorId:   m.userID,
			CreatedAt: timestamppb.New(m.createdAt),
			Event: &corev1.Event_UserJoinedRoom{
				UserJoinedRoom: &corev1.UserJoinedRoomEvent{
					RoomId: parseRoomIDFromEvtSubject(subject),
				},
			},
		}

		seq, err := c.EventPublisher.AppendAt(ctx, subject, event, expectedSeq)
		if err == nil {
			emitted++
			expectedSeq = seq
			continue
		}
		if errors.Is(err, events.ErrConflict) {
			if i == 0 {
				// Subject is already migrated. Treat as a no-op for the
				// whole subject and move on.
				return 0, true, nil
			}
			// Conflict midway: a previous run crashed partway through
			// this subject AND something else (this run or a concurrent
			// writer) has appended events since. Bail rather than
			// silently misorder.
			return emitted, false, fmt.Errorf("unexpected mid-subject conflict at index %d: %w", i, err)
		}
		return emitted, false, err
	}
	return emitted, false, nil
}

// parseRoomIDFromEvtSubject is a local helper for migration; the events
// package's ParseRoomSubject is the canonical parser, but we have the
// subject and want the roomID without re-validating the prefix we just
// constructed.
func parseRoomIDFromEvtSubject(subject string) string {
	roomID, _ := events.ParseRoomSubject(subject)
	return roomID
}

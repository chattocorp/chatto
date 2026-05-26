package migrations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// MigrateMessagesToES seeds the EVT stream with the message history
// currently stored in SERVER_EVENTS + SERVER_BODIES (issue #597
// phase 3).
//
// # Source of truth before
//
//   - SERVER_EVENTS stream holds MessagePostedEvent envelopes under
//     `server.room.{kind}.{R}.msg.{eventID}` (root posts) and
//     `server.room.{kind}.{R}.msg.{rootID}.replies.{eventID}` (thread
//     replies). Encrypted message bodies live in the SERVER_BODIES KV
//     bucket, addressable by the MessagePostedEvent's
//     message_body_id field ({userID}.{bodyID} compound key).
//
//   - Historical edits and deletes are NOT durable in SERVER_EVENTS —
//     they only fired as live-only events. Edits mutated SERVER_BODIES
//     in place; deletes removed the body. So the legacy data we can
//     migrate is: every MessagePostedEvent that ever happened, plus
//     whichever body content survives in SERVER_BODIES at migration
//     time. Edit history is lost (we get current state only).
//     Deleted-post bodies are missing.
//
// # Source of truth after
//
//   - Each MessagePostedEvent is re-emitted on EVT at
//     `evt.room.{R}.message_posted` with the body content embedded in
//     the event payload (the MessagePostedEvent.body field added in
//     #597 phase 1). The legacy message_body_id field is left empty
//     on imported events — the embedded body is the only source of
//     truth post-migration.
//
//   - Posts whose body has been deleted from SERVER_BODIES (legacy
//     hard-delete) are imported with body=nil. The projection holds
//     the event; resolvers render "[message deleted]" or similar
//     based on the absent body. This matches the framing in the
//     #597 design: the projection holds the audit-trail event,
//     bodies are crypto-shredded territory.
//
// # Idempotency and crash-safety
//
// Per-room: migrated message events are emitted in bounded atomic
// chunks. Each chunk's first entry carries subject-level OCC against
// `evt.room.{R}.message_posted`; subsequent entries in the same chunk do
// not carry dependent OCC because JetStream evaluates every batch entry
// against the committed pre-batch state. On re-run, the importer reads
// existing event IDs for the subject, verifies they match the legacy
// prefix, and resumes at the first missing event.
//
// # When this can be removed
//
// Once every live deployment has booted at least once on a version
// that includes this migration AND ADR-035 phase 7 (decommission the
// legacy SERVER_EVENTS message subjects + SERVER_BODIES bucket) has
// shipped.
func MigrateMessagesToES(
	ctx context.Context,
	serverEventsStream jetstream.Stream,
	serverBodiesKV jetstream.KeyValue,
	publisher *events.Publisher,
	logger *log.Logger,
) error {
	// Walk every message envelope on SERVER_EVENTS via a temporary
	// consumer. Filter scope: server.room.*.*.msg.> covers both root
	// posts (msg.{eventID}) and thread replies
	// (msg.{rootID}.replies.{eventID}).
	//
	// We use a regular consumer (not OrderedConsumer) so we can call
	// Info() upfront and learn the exact NumPending — this lets the
	// Fetch be bounded by the known count, avoiding the
	// minutes-long-FetchMaxWait pitfall on a small or empty stream.
	// Same pattern as core.fetchRoomEventsWithConsumer.
	filterSubjects := []string{"server.room.*.*.msg.>"}
	consumer, err := serverEventsStream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		FilterSubjects:    filterSubjects,
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		AckPolicy:         jetstream.AckNonePolicy,
		MemoryStorage:     true,
		InactiveThreshold: 30 * time.Second,
	})
	if err != nil {
		return fmt.Errorf("create migration consumer on SERVER_EVENTS: %w", err)
	}
	defer serverEventsStream.DeleteConsumer(context.Background(), consumer.CachedInfo().Name)

	info, err := consumer.Info(ctx)
	if err != nil {
		return fmt.Errorf("get consumer info: %w", err)
	}
	numPending := info.NumPending
	if numPending == 0 {
		// Nothing to migrate. Don't log — first boot on a fresh
		// deployment hits this path.
		return nil
	}

	var imported, skipped, bodyMissing int
	startedAt := time.Now()

	msgs, err := consumer.Fetch(int(numPending), jetstream.FetchMaxWait(60*time.Second))
	if err != nil && !errors.Is(err, jetstream.ErrNoMessages) {
		return fmt.Errorf("fetch migration messages: %w", err)
	}
	if msgs == nil {
		return nil
	}

	type roomBatch struct {
		entries []events.BatchEntry
	}
	roomBatches := make(map[string]*roomBatch)
	var roomOrder []string

	for msg := range msgs.Messages() {
		// Decode the legacy event envelope.
		var legacyEvent corev1.Event
		if err := proto.Unmarshal(msg.Data(), &legacyEvent); err != nil {
			logger.Warn("messages ES migration: skipping unmarshalable event", "subject", msg.Subject(), "error", err)
			continue
		}

		posted := legacyEvent.GetMessagePosted()
		if posted == nil {
			// Subject matched the message filter but payload isn't a
			// MessagePostedEvent. Shouldn't happen (the filter scope
			// is message subjects only) but defensive.
			continue
		}
		roomID := posted.GetRoomId()
		if roomID == "" {
			// Older posts may have room_id reserved; nothing we can do.
			logger.Warn("messages ES migration: skipping post without room_id", "subject", msg.Subject())
			continue
		}

		// Look up the body from SERVER_BODIES via the legacy
		// message_body_id pointer. Missing bodies are not fatal —
		// historic deletes wiped the body but left the post event.
		// Import with body=nil.
		var body *corev1.MessageBody
		if bodyKey := posted.GetMessageBodyId(); bodyKey != "" {
			entry, getErr := serverBodiesKV.Get(ctx, bodyKey)
			switch {
			case getErr == nil:
				var mb corev1.MessageBody
				if err := proto.Unmarshal(entry.Value(), &mb); err != nil {
					logger.Warn("messages ES migration: skipping unparseable body", "body_key", bodyKey, "error", err)
				} else {
					body = &mb
				}
			case errors.Is(getErr, jetstream.ErrKeyNotFound):
				bodyMissing++
			default:
				logger.Warn("messages ES migration: failed to fetch body", "body_key", bodyKey, "error", getErr)
			}
		}

		// Build the migrated event. Preserve the original envelope
		// metadata (id, actor, created_at) so the timeline order and
		// audit trail are preserved. Body is embedded; message_body_id
		// is repurposed post-cutover as an alias for event_id (same
		// value as the EventId field), so legacy resolver code paths
		// that pass MessageBodyId around continue to resolve through
		// eventIDFromBodyKey.
		eventID := posted.GetEventId()
		newEvent := &corev1.Event{
			Id:        legacyEvent.GetId(),
			ActorId:   legacyEvent.GetActorId(),
			CreatedAt: preserveTimestamp(legacyEvent.GetCreatedAt()),
			Event: &corev1.Event_MessagePosted{
				MessagePosted: &corev1.MessagePostedEvent{
					RoomId:                    roomID,
					EventId:                   eventID,
					MessageBodyId:             eventID,
					InReplyTo:                 posted.GetInReplyTo(),
					InThread:                  posted.GetInThread(),
					MentionedUserIds:          posted.GetMentionedUserIds(),
					EchoOfEventId:             posted.GetEchoOfEventId(),
					EchoFromThreadRootEventId: posted.GetEchoFromThreadRootEventId(),
					Body:                      body,
				},
			},
		}

		agg := events.RoomAggregate(roomID)
		subject := agg.Subject(events.EventMessagePosted)
		batch := roomBatches[roomID]
		if batch == nil {
			batch = &roomBatch{}
			roomBatches[roomID] = batch
			roomOrder = append(roomOrder, roomID)
		}
		batch.entries = append(batch.entries, events.BatchEntry{
			Subject: subject,
			Event:   newEvent,
		})
	}

	for _, roomID := range roomOrder {
		batch := roomBatches[roomID]
		if len(batch.entries) == 0 {
			continue
		}

		roomImported, roomSkipped, err := publishMessageMigrationRoom(ctx, publisher, roomID, batch.entries, logger)
		if err != nil {
			return fmt.Errorf("publish migrated messages (room=%s): %w", roomID, err)
		}
		imported += roomImported
		skipped += roomSkipped
	}

	if imported > 0 || skipped > 0 {
		logger.Info(
			"messages ES migration: seeded events from legacy SERVER_EVENTS + SERVER_BODIES",
			"messages_imported", imported,
			"messages_skipped", skipped,
			"rooms_processed", len(roomBatches),
			"bodies_missing", bodyMissing,
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
	}
	return nil
}

const messageMigrationBatchSize = 500

func publishMessageMigrationRoom(
	ctx context.Context,
	publisher *events.Publisher,
	roomID string,
	entries []events.BatchEntry,
	logger *log.Logger,
) (imported int, skipped int, err error) {
	if len(entries) == 0 {
		return 0, 0, nil
	}

	subject := entries[0].Subject
	existingIDs, expectedSeq, err := publisher.SubjectEventIDs(ctx, subject)
	if err != nil {
		return 0, 0, fmt.Errorf("read existing message events: %w", err)
	}
	if len(existingIDs) > len(entries) {
		logger.Warn(
			"messages ES migration: skipping room with more projected messages than legacy messages",
			"room_id", roomID,
			"existing_messages", len(existingIDs),
			"legacy_messages", len(entries),
		)
		return 0, len(entries), nil
	}
	for i, existingID := range existingIDs {
		if entries[i].Event.GetId() != existingID {
			logger.Warn(
				"messages ES migration: skipping room with non-matching existing message prefix",
				"room_id", roomID,
				"index", i,
				"existing_event_id", existingID,
				"legacy_event_id", entries[i].Event.GetId(),
			)
			return 0, len(entries), nil
		}
	}
	if len(existingIDs) == len(entries) {
		return 0, len(entries), nil
	}

	pending := entries[len(existingIDs):]
	for start := 0; start < len(pending); start += messageMigrationBatchSize {
		end := start + messageMigrationBatchSize
		if end > len(pending) {
			end = len(pending)
		}

		chunk := append([]events.BatchEntry(nil), pending[start:end]...)
		chunk[0].HasOCC = true
		chunk[0].ExpectedSeq = expectedSeq

		seqs, err := publisher.AppendBatch(ctx, chunk)
		if err != nil {
			if errors.Is(err, events.ErrConflict) {
				return imported, skipped, fmt.Errorf("message chunk OCC conflict after resume point %d: %w", len(existingIDs)+imported, err)
			}
			return imported, skipped, err
		}
		expectedSeq = seqs[len(seqs)-1]
		imported += len(chunk)
	}
	return imported, len(existingIDs), nil
}

// preserveTimestamp returns the original timestamp if non-nil, or a
// fresh "now" if missing. Imported events should keep their original
// created_at so chronological ordering in the projection matches the
// original SERVER_EVENTS sequence.
func preserveTimestamp(ts *timestamppb.Timestamp) *timestamppb.Timestamp {
	if ts != nil {
		return ts
	}
	return timestamppb.Now()
}

// SubjectPrefixServerRoomMsg is the subject prefix this migration
// scans on SERVER_EVENTS. Exported as a constant so tests don't have
// to repeat the string and can assert on it.
const SubjectPrefixServerRoomMsg = "server.room."

// IsMessageSubject reports whether the given subject is one this
// migration would consume. Used by tests; not used in the migration
// itself.
func IsMessageSubject(subject string) bool {
	if !strings.HasPrefix(subject, SubjectPrefixServerRoomMsg) {
		return false
	}
	// server.room.{kind}.{roomID}.msg.…
	parts := strings.SplitN(subject, ".", 5)
	if len(parts) < 5 {
		return false
	}
	rest := parts[4]
	return strings.HasPrefix(rest, "msg.")
}

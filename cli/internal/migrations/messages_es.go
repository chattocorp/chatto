package migrations

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/encoding/protowire"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type migratedMessageEntry struct {
	threadCreated *events.BatchEntry
	bodyEvent     *events.BatchEntry
	message       events.BatchEntry
}

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
//     bucket, addressable by the legacy MessagePostedEvent.message_body_id
//     field ({userID}.{bodyID} compound key). The current proto reserves
//     that field, so the importer reads it from protobuf unknown fields.
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
//   - Each surviving body is re-emitted on EVT at
//     `evt.room.{R}.message_body` immediately before its bodyless
//     `evt.room.{R}.message_posted` fact. The message Event envelope ID is
//     the stable message identity; body payloads live on private body event
//     events so they can be securely deleted independently.
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
// Per-room: the importer treats `message_posted` as the commit point. Optional
// `thread_created` and `message_body` side events are published before the
// bodyless `message_posted` event for each legacy message, preserving the
// body-before-event consistency rule from ADR-011 without relying on large
// atomic batches. On re-run, the importer reads existing message IDs for the
// subject, verifies they match the legacy prefix, and resumes at the first
// missing message. Side events use deterministic event IDs and are skipped if a
// previous boot already emitted them before crashing.
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
		entries     []migratedMessageEntry
		seenThreads map[string]struct{}
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

		// Look up the body from SERVER_BODIES via the legacy message_body_id
		// pointer. The field is now reserved on MessagePostedEvent, so it
		// decodes into protobuf unknown fields. Missing bodies are not fatal:
		// historic deletes wiped the body but left the post event. Import with
		// body=nil.
		var body *corev1.MessageBody
		if bodyKey := legacyMessageBodyID(posted); bodyKey != "" {
			if serverBodiesKV == nil {
				bodyMissing++
			} else {
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
		}
		if body == nil {
			body = legacyEmbeddedMessageBody(posted)
		}

		// Build the migrated event. Preserve the original envelope
		// metadata (id, actor, created_at) so the timeline order and
		// audit trail are preserved. Body content is carried by a preceding
		// MessageBodyEvent; the message's durable identity remains the
		// Event envelope ID.
		//
		// Message identity lives on the envelope. Recover it from the
		// envelope id or, as a last resort for old payloads, the legacy
		// subject.
		eventID := legacyEvent.GetId()
		if eventID == "" {
			eventID = subjects.ParseEventIDFromSubject(msg.Subject())
		}
		if eventID == "" {
			logger.Warn("messages ES migration: skipping post without event id", "subject", msg.Subject())
			continue
		}

		envelopeID := legacyEvent.GetId()
		if envelopeID == "" {
			envelopeID = eventID
		}

		inThread := posted.GetInThread()
		if inThread == "" {
			if rootID, ok := subjects.ParseThreadRootEventIDFromSubject(msg.Subject()); ok {
				inThread = rootID
			}
		}

		newEvent := &corev1.Event{
			Id:        envelopeID,
			ActorId:   legacyEvent.GetActorId(),
			CreatedAt: preserveTimestamp(legacyEvent.GetCreatedAt()),
			Event: &corev1.Event_MessagePosted{
				MessagePosted: &corev1.MessagePostedEvent{
					RoomId:                    roomID,
					InReplyTo:                 posted.GetInReplyTo(),
					InThread:                  inThread,
					MentionedUserIds:          posted.GetMentionedUserIds(),
					EchoOfEventId:             posted.GetEchoOfEventId(),
					EchoFromThreadRootEventId: posted.GetEchoFromThreadRootEventId(),
				},
			},
		}

		agg := events.RoomAggregate(roomID)
		subject := agg.Subject(events.EventMessagePosted)
		batch := roomBatches[roomID]
		if batch == nil {
			batch = &roomBatch{seenThreads: make(map[string]struct{})}
			roomBatches[roomID] = batch
			roomOrder = append(roomOrder, roomID)
		}

		var threadCreated *events.BatchEntry
		if inThread != "" {
			if _, seen := batch.seenThreads[inThread]; !seen {
				batch.seenThreads[inThread] = struct{}{}
				entry := events.BatchEntry{
					Subject: agg.Subject(events.EventThreadCreated),
					Event: &corev1.Event{
						Id:        migratedThreadCreatedEventID(roomID, inThread),
						ActorId:   legacyEvent.GetActorId(),
						CreatedAt: preserveTimestamp(legacyEvent.GetCreatedAt()),
						Event: &corev1.Event_ThreadCreated{
							ThreadCreated: &corev1.ThreadCreatedEvent{
								RoomId:            roomID,
								ThreadRootEventId: inThread,
							},
						},
					},
				}
				threadCreated = &entry
			}
		}
		var bodyEvent *events.BatchEntry
		if body != nil {
			bodyEventID := migratedMessageBodyEventID(envelopeID)
			if body.GetBodyEventId() == "" {
				body.BodyEventId = bodyEventID
			}
			entry := events.BatchEntry{
				Subject: agg.Subject(events.EventMessageBody),
				Event: &corev1.Event{
					Id:        bodyEventID,
					ActorId:   legacyEvent.GetActorId(),
					CreatedAt: preserveTimestamp(legacyEvent.GetCreatedAt()),
					Event: &corev1.Event_MessageBody{
						MessageBody: &corev1.MessageBodyEvent{
							RoomId:  roomID,
							EventId: envelopeID,
							Body:    body,
						},
					},
				},
			}
			bodyEvent = &entry
		}

		batch.entries = append(batch.entries, migratedMessageEntry{
			threadCreated: threadCreated,
			bodyEvent:     bodyEvent,
			message: events.BatchEntry{
				Subject: subject,
				Event:   newEvent,
			},
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
	entries []migratedMessageEntry,
	logger *log.Logger,
) (imported int, skipped int, err error) {
	if len(entries) == 0 {
		return 0, 0, nil
	}

	subject := entries[0].message.Subject
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
		if entries[i].message.Event.GetId() != existingID {
			logger.Warn(
				"messages ES migration: skipping room with non-matching existing message prefix",
				"room_id", roomID,
				"index", i,
				"existing_event_id", existingID,
				"legacy_event_id", entries[i].message.Event.GetId(),
			)
			return 0, len(entries), nil
		}
	}
	if len(existingIDs) == len(entries) {
		return 0, len(entries), nil
	}

	agg := events.RoomAggregate(roomID)
	threadCreatedSubject := agg.Subject(events.EventThreadCreated)
	bodySubject := agg.Subject(events.EventMessageBody)
	existingThreadIDs, threadExpectedSeq, err := publisher.SubjectEventIDs(ctx, threadCreatedSubject)
	if err != nil {
		return 0, 0, fmt.Errorf("read existing thread-created events: %w", err)
	}
	existingBodyIDs, bodyExpectedSeq, err := publisher.SubjectEventIDs(ctx, bodySubject)
	if err != nil {
		return 0, 0, fmt.Errorf("read existing message-body events: %w", err)
	}
	seenThreadIDs := stringSet(existingThreadIDs)
	seenBodyIDs := stringSet(existingBodyIDs)

	pending := entries[len(existingIDs):]
	for _, entry := range pending {
		if entry.threadCreated != nil {
			threadExpectedSeq, err = appendMigrationSideEventIfMissing(ctx, publisher, *entry.threadCreated, threadExpectedSeq, seenThreadIDs)
			if err != nil {
				return imported, skipped, fmt.Errorf("append migrated thread event for message %s: %w", entry.message.Event.GetId(), err)
			}
		}
		if entry.bodyEvent != nil {
			bodyExpectedSeq, err = appendMigrationSideEventIfMissing(ctx, publisher, *entry.bodyEvent, bodyExpectedSeq, seenBodyIDs)
			if err != nil {
				return imported, skipped, fmt.Errorf("append migrated body event for message %s: %w", entry.message.Event.GetId(), err)
			}
		}

		seq, err := publisher.AppendAt(ctx, entry.message.Subject, entry.message.Event, expectedSeq)
		if err != nil {
			if errors.Is(err, events.ErrConflict) {
				return imported, skipped, fmt.Errorf("message event OCC conflict after resume point %d: %w", len(existingIDs)+imported, err)
			}
			return imported, skipped, err
		}
		expectedSeq = seq
		imported++
	}
	return imported, len(existingIDs), nil
}

func appendMigrationSideEventIfMissing(
	ctx context.Context,
	publisher *events.Publisher,
	entry events.BatchEntry,
	expectedSeq uint64,
	seenIDs map[string]struct{},
) (uint64, error) {
	eventID := entry.Event.GetId()
	if _, seen := seenIDs[eventID]; seen {
		return expectedSeq, nil
	}
	seq, err := publisher.AppendAt(ctx, entry.Subject, entry.Event, expectedSeq)
	if err == nil {
		seenIDs[eventID] = struct{}{}
		return seq, nil
	}
	if !errors.Is(err, events.ErrConflict) {
		return expectedSeq, err
	}

	ids, lastSeq, readErr := publisher.SubjectEventIDs(ctx, entry.Subject)
	if readErr != nil {
		return expectedSeq, fmt.Errorf("refresh side event subject after OCC conflict: %w", readErr)
	}
	for _, id := range ids {
		seenIDs[id] = struct{}{}
	}
	if _, seen := seenIDs[eventID]; seen {
		return lastSeq, nil
	}
	return expectedSeq, err
}

func stringSet(values []string) map[string]struct{} {
	out := make(map[string]struct{}, len(values))
	for _, value := range values {
		out[value] = struct{}{}
	}
	return out
}

func migratedThreadCreatedEventID(roomID, threadRootEventID string) string {
	return "thread_created." + roomID + "." + threadRootEventID
}

func migratedMessageBodyEventID(messageEventID string) string {
	return "message_body." + messageEventID
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

func legacyMessageBodyID(posted *corev1.MessagePostedEvent) string {
	if posted == nil {
		return ""
	}
	unknown := posted.ProtoReflect().GetUnknown()
	for len(unknown) > 0 {
		num, typ, n := protowire.ConsumeTag(unknown)
		if n < 0 {
			return ""
		}
		unknown = unknown[n:]
		if num == 3 && typ == protowire.BytesType {
			value, m := protowire.ConsumeString(unknown)
			if m < 0 {
				return ""
			}
			return value
		}
		m := protowire.ConsumeFieldValue(num, typ, unknown)
		if m < 0 {
			return ""
		}
		unknown = unknown[m:]
	}
	return ""
}

func legacyEmbeddedMessageBody(posted *corev1.MessagePostedEvent) *corev1.MessageBody {
	if posted == nil {
		return nil
	}
	unknown := posted.ProtoReflect().GetUnknown()
	for len(unknown) > 0 {
		num, typ, n := protowire.ConsumeTag(unknown)
		if n < 0 {
			return nil
		}
		unknown = unknown[n:]
		if num == 9 && typ == protowire.BytesType {
			value, m := protowire.ConsumeBytes(unknown)
			if m < 0 {
				return nil
			}
			var body corev1.MessageBody
			if err := proto.Unmarshal(value, &body); err != nil {
				return nil
			}
			return &body
		}
		m := protowire.ConsumeFieldValue(num, typ, unknown)
		if m < 0 {
			return nil
		}
		unknown = unknown[m:]
	}
	return nil
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

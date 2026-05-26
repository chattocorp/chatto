package migrations

import (
	"context"
	"encoding/binary"
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

// MigrateReactionsToES seeds EVT with the current reaction state from
// the legacy SERVER_REACTIONS KV bucket.
//
// # Source of truth before
//
// SERVER_REACTIONS stores one key per active reaction:
// `{messageEventId}.{emojiName}.{userId}`. The value is an 8-byte
// big-endian Unix-nano timestamp used to order reaction groups. Add/remove
// history was not durable; only the current set survives.
//
// # Source of truth after
//
// Each active legacy reaction becomes a durable ReactionAddedEvent on
// `evt.room.{roomId}.reaction_added`. The reacting user is the event
// actor. The message-to-room mapping is recovered from legacy
// SERVER_EVENTS message envelopes.
//
// # Idempotency and crash-safety
//
// Reactions are grouped by room and emitted in bounded atomic chunks.
// The first entry in each chunk carries subject-level OCC against
// `evt.room.{roomId}.reaction_added`. On replay, the importer reads the
// existing reaction identities, verifies they match the legacy prefix, and
// resumes at the first missing reaction.
func MigrateReactionsToES(
	ctx context.Context,
	serverEventsStream jetstream.Stream,
	serverReactionsKV jetstream.KeyValue,
	publisher *events.Publisher,
	logger *log.Logger,
) error {
	reactionKeys, err := listLegacyReactionKeys(ctx, serverReactionsKV)
	if err != nil {
		return err
	}
	if len(reactionKeys) == 0 {
		return nil
	}

	messageRooms, err := legacyMessageRooms(ctx, serverEventsStream, logger)
	if err != nil {
		return err
	}

	type roomBatch struct {
		entries []events.BatchEntry
	}
	roomBatches := make(map[string]*roomBatch)
	var roomOrder []string
	var skippedMissingRoom int

	for _, key := range reactionKeys {
		messageEventID, emoji, userID, err := parseLegacyReactionKey(key)
		if err != nil {
			logger.Warn("reactions ES migration: skipping invalid key", "key", key, "error", err)
			continue
		}
		roomID := messageRooms[messageEventID]
		if roomID == "" {
			skippedMissingRoom++
			logger.Warn("reactions ES migration: skipping reaction for unknown message", "key", key, "message_event_id", messageEventID)
			continue
		}

		entry, err := serverReactionsKV.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return fmt.Errorf("get reaction key %s: %w", key, err)
		}

		event := stamp(&corev1.Event{
			Event: &corev1.Event_ReactionAdded{
				ReactionAdded: &corev1.ReactionAddedEvent{
					RoomId:         roomID,
					MessageEventId: messageEventID,
					Emoji:          emoji,
				},
			},
		}, userID, reactionTimestamp(entry.Value()))

		agg := events.RoomAggregate(roomID)
		batch := roomBatches[roomID]
		if batch == nil {
			batch = &roomBatch{}
			roomBatches[roomID] = batch
			roomOrder = append(roomOrder, roomID)
		}
		batch.entries = append(batch.entries, events.BatchEntry{
			Subject: agg.Subject(events.EventReactionAdded),
			Event:   event,
		})
	}

	var imported, skipped int
	startedAt := time.Now()
	for _, roomID := range roomOrder {
		batch := roomBatches[roomID]
		if len(batch.entries) == 0 {
			continue
		}

		roomImported, roomSkipped, err := publishReactionMigrationRoom(ctx, publisher, roomID, batch.entries, logger)
		if err != nil {
			return fmt.Errorf("publish migrated reactions (room=%s): %w", roomID, err)
		}
		imported += roomImported
		skipped += roomSkipped
	}

	if imported > 0 || skipped > 0 || skippedMissingRoom > 0 {
		logger.Info(
			"reactions ES migration: seeded events from legacy SERVER_REACTIONS",
			"reactions_imported", imported,
			"reactions_skipped", skipped,
			"missing_room_skipped", skippedMissingRoom,
			"rooms_processed", len(roomBatches),
			"duration_ms", time.Since(startedAt).Milliseconds(),
		)
	}
	return nil
}

func publishReactionMigrationRoom(
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
	existingEvents, expectedSeq, err := publisher.SubjectEvents(ctx, subject)
	if err != nil {
		return 0, 0, fmt.Errorf("read existing reaction events: %w", err)
	}
	if len(existingEvents) > len(entries) {
		logger.Warn(
			"reactions ES migration: skipping room with more projected reactions than legacy reactions",
			"room_id", roomID,
			"existing_reactions", len(existingEvents),
			"legacy_reactions", len(entries),
		)
		return 0, len(entries), nil
	}
	for i, existing := range existingEvents {
		if reactionIdentity(existing) != reactionIdentity(entries[i].Event) {
			logger.Warn(
				"reactions ES migration: skipping room with non-matching existing reaction prefix",
				"room_id", roomID,
				"index", i,
				"existing_reaction", reactionIdentity(existing),
				"legacy_reaction", reactionIdentity(entries[i].Event),
			)
			return 0, len(entries), nil
		}
	}
	if len(existingEvents) == len(entries) {
		return 0, len(entries), nil
	}

	pending := entries[len(existingEvents):]
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
				return imported, skipped, fmt.Errorf("reaction chunk OCC conflict after resume point %d: %w", len(existingEvents)+imported, err)
			}
			return imported, skipped, err
		}
		expectedSeq = seqs[len(seqs)-1]
		imported += len(chunk)
	}
	return imported, len(existingEvents), nil
}

func reactionIdentity(event *corev1.Event) string {
	added := event.GetReactionAdded()
	if added == nil {
		return ""
	}
	return added.GetMessageEventId() + "\x00" + added.GetEmoji() + "\x00" + event.GetActorId()
}

func listLegacyReactionKeys(ctx context.Context, kv jetstream.KeyValue) ([]string, error) {
	lister, err := kv.ListKeys(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("list reaction keys: %w", err)
	}
	var keys []string
	for key := range lister.Keys() {
		keys = append(keys, key)
	}
	return keys, nil
}

func parseLegacyReactionKey(key string) (messageEventID, emoji, userID string, err error) {
	parts := strings.SplitN(key, ".", 3)
	if len(parts) != 3 {
		return "", "", "", fmt.Errorf("invalid reaction key format")
	}
	return parts[0], parts[1], parts[2], nil
}

func reactionTimestamp(value []byte) *timestamppb.Timestamp {
	if len(value) >= 8 {
		return timestamppb.New(time.Unix(0, int64(binary.BigEndian.Uint64(value))))
	}
	return timestamppb.Now()
}

func legacyMessageRooms(ctx context.Context, stream jetstream.Stream, logger *log.Logger) (map[string]string, error) {
	consumer, err := stream.CreateConsumer(ctx, jetstream.ConsumerConfig{
		FilterSubjects:    []string{"server.room.*.*.msg.>"},
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		AckPolicy:         jetstream.AckNonePolicy,
		MemoryStorage:     true,
		InactiveThreshold: 30 * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("create reaction migration consumer on SERVER_EVENTS: %w", err)
	}
	defer stream.DeleteConsumer(context.Background(), consumer.CachedInfo().Name)

	info, err := consumer.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("get reaction migration consumer info: %w", err)
	}
	messageRooms := make(map[string]string, int(info.NumPending))
	if info.NumPending == 0 {
		return messageRooms, nil
	}

	msgs, err := consumer.Fetch(int(info.NumPending), jetstream.FetchMaxWait(60*time.Second))
	if err != nil && !errors.Is(err, jetstream.ErrNoMessages) {
		return nil, fmt.Errorf("fetch legacy message rooms: %w", err)
	}
	if msgs == nil {
		return messageRooms, nil
	}
	for msg := range msgs.Messages() {
		var event corev1.Event
		if err := proto.Unmarshal(msg.Data(), &event); err != nil {
			logger.Warn("reactions ES migration: skipping unmarshalable message event", "subject", msg.Subject(), "error", err)
			continue
		}
		posted := event.GetMessagePosted()
		if posted == nil || posted.GetRoomId() == "" {
			continue
		}
		eventID := posted.GetEventId()
		if eventID == "" {
			eventID = event.GetId()
		}
		if eventID != "" {
			messageRooms[eventID] = posted.GetRoomId()
		}
	}
	return messageRooms, nil
}

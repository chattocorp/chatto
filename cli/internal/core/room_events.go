package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// RoomEvent pairs a SpaceEvent with its JetStream stream sequence so the
// pagination layer can build opaque cursors without re-deriving the
// sequence per event. SpaceEvent is embedded so callers can access event
// fields directly (`event.Id`, `event.GetMessagePosted()`, etc.).
type RoomEvent struct {
	*corev1.Event
	Sequence uint64
}

// RoomEventsResult is the return type for paginated room event queries.
// HasOlder/HasNewer indicate whether more events exist beyond the
// returned page. StartCursorSeq/EndCursorSeq are the JetStream sequences
// of the first and last event in the page; the GraphQL layer renders
// them as opaque cursor strings. Both are zero when Events is empty.
type RoomEventsResult struct {
	Events         []*RoomEvent
	HasOlder       bool
	HasNewer       bool
	StartCursorSeq uint64
	EndCursorSeq   uint64
}

// RoomEventsAroundResult contains the result of fetching events around a target event.
type RoomEventsAroundResult struct {
	Events      []*RoomEvent
	TargetIndex int
	HasOlder    bool
	HasNewer    bool
}

// GetRoomEvents fetches historical events for a specific room from the SPACE stream.
// Returns up to 'limit' most recent events. If 'beforeSeq' is provided, fetches events
// strictly older than that JetStream sequence. Uses sequence-based lookups for both
// initial load and pagination, with a small-room fast path when the total event count
// fits in one fetch. Message bodies are lazy-loaded via GraphQL resolvers.
func (c *ChattoCore) GetRoomEvents(ctx context.Context, kind RoomKind, room_id string, limit int, beforeSeq *uint64) (*RoomEventsResult, error) {
	if limit <= 0 {
		limit = defaultHistoricalMessageLimit
	}

	stream := c.storage.serverEventsStream

	// Filter for root messages and meta events only (excludes thread replies).
	// "msg.*" matches root messages; "meta" matches room lifecycle events (joins, leaves, etc.)
	filterSubjects := subjects.RoomRootEventsFilters(string(kind), room_id)

	// --- Small room fast path ---
	// Check total room event count (uses "room.>" which includes thread replies,
	// so the count may slightly overestimate — that's fine for this decision).
	roomAllSubject := subjects.RoomAllEvents(string(kind), room_id)
	streamInfo, err := stream.Info(ctx, jetstream.WithSubjectFilter(roomAllSubject))
	if err != nil {
		return nil, fmt.Errorf("failed to get stream info: %w", err)
	}

	// Sum the per-subject counts to get the room-specific event count.
	// State.Msgs is the total stream count (all rooms); State.Subjects
	// contains only subjects matching our WithSubjectFilter.
	var roomMsgCount uint64
	for _, count := range streamInfo.State.Subjects {
		roomMsgCount += count
	}

	if roomMsgCount == 0 {
		return &RoomEventsResult{}, nil
	}

	if int(roomMsgCount) <= limit {
		// Room has very few events — fetch everything in one shot
		events, err := c.fetchRoomEventsWithConsumer(ctx, stream, filterSubjects, jetstream.ConsumerConfig{
			FilterSubjects:    filterSubjects,
			DeliverPolicy:     jetstream.DeliverAllPolicy,
			AckPolicy:         jetstream.AckNonePolicy,
			MemoryStorage:     true,
			InactiveThreshold: 10 * time.Second,
		}, beforeSeq)
		if err != nil {
			return nil, err
		}
		hasOlder := len(events) > limit
		if hasOlder {
			events = events[len(events)-limit:]
		}
		c.logger.Debug("Fetched room events (small room fast path)", "kind", kind, "room_id", room_id, "count", len(events))
		return roomEventsResult(events, hasOlder, beforeSeq != nil), nil
	}

	// --- Large room paths ---
	if beforeSeq == nil {
		return c.getRoomEventsInitialLoad(ctx, stream, kind, room_id, limit, filterSubjects, streamInfo)
	}
	return c.getRoomEventsPagination(ctx, stream, kind, room_id, limit, *beforeSeq, filterSubjects, streamInfo)
}

// getRoomEventsInitialLoad fetches the most recent events using sequence-based start.
// Uses GetLastMsgForSubject to find the room's last event, then starts a consumer
// close to the end to avoid scanning the entire stream.
func (c *ChattoCore) getRoomEventsInitialLoad(
	ctx context.Context,
	stream jetstream.Stream, kind RoomKind, room_id string,
	limit int,
	filterSubjects []string,
	streamInfo *jetstream.StreamInfo,
) (*RoomEventsResult, error) {
	// Find the last sequence for this room's root messages and meta events.
	// Both are O(1) lookups in JetStream's subject index.
	msgSubject := subjects.RoomRootMessages(string(kind), room_id)
	metaSubject := subjects.RoomMeta(string(kind), room_id)

	var lastSeq uint64

	lastMsg, err := stream.GetLastMsgForSubject(ctx, msgSubject)
	if err != nil && !errors.Is(err, jetstream.ErrMsgNotFound) {
		return nil, fmt.Errorf("failed to get last message: %w", err)
	}
	if lastMsg != nil {
		lastSeq = lastMsg.Sequence
	}

	lastMeta, err := stream.GetLastMsgForSubject(ctx, metaSubject)
	if err != nil && !errors.Is(err, jetstream.ErrMsgNotFound) {
		return nil, fmt.Errorf("failed to get last meta event: %w", err)
	}
	if lastMeta != nil && lastMeta.Sequence > lastSeq {
		lastSeq = lastMeta.Sequence
	}

	if lastSeq == 0 {
		return &RoomEventsResult{}, nil
	}

	firstSeq := streamInfo.State.FirstSeq

	// Start the consumer at an estimated position near the end.
	// The stream contains events from ALL rooms in the space, so there may be
	// non-matching events between our target events. The multiplier accounts for this.
	multipliers := []uint64{3, 10, 50}
	for _, mult := range multipliers {
		startSeq := lastSeq - uint64(limit)*mult + 1
		if startSeq < firstSeq || startSeq > lastSeq {
			startSeq = firstSeq
		}

		events, err := c.fetchRoomEventsWithConsumer(ctx, stream, filterSubjects, jetstream.ConsumerConfig{
			FilterSubjects:    filterSubjects,
			DeliverPolicy:     jetstream.DeliverByStartSequencePolicy,
			OptStartSeq:       startSeq,
			AckPolicy:         jetstream.AckNonePolicy,
			MemoryStorage:     true,
			InactiveThreshold: 10 * time.Second,
		}, nil)
		if err != nil {
			return nil, err
		}

		if len(events) >= limit || startSeq == firstSeq {
			// Got enough events, or we've already scanned from the beginning
			hasOlder := len(events) > limit || startSeq > firstSeq
			if len(events) > limit {
				events = events[len(events)-limit:]
			}
			c.logger.Debug("Fetched room events (initial load)", "kind", kind, "room_id", room_id, "count", len(events), "multiplier", mult)
			return roomEventsResult(events, hasOlder, false), nil
		}
		// Not enough events — widen the range and retry
	}

	// Shouldn't reach here, but fall back to DeliverAllPolicy
	events, err := c.fetchRoomEventsWithConsumer(ctx, stream, filterSubjects, jetstream.ConsumerConfig{
		FilterSubjects:    filterSubjects,
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		AckPolicy:         jetstream.AckNonePolicy,
		MemoryStorage:     true,
		InactiveThreshold: 10 * time.Second,
	}, nil)
	if err != nil {
		return nil, err
	}
	hasOlder := len(events) > limit
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	c.logger.Debug("Fetched room events (initial load fallback)", "kind", kind, "room_id", room_id, "count", len(events))
	return roomEventsResult(events, hasOlder, false), nil
}

// getRoomEventsPagination fetches older events before a sequence cursor.
// Uses the same multiplier-based seeking as the initial-load path: start the
// consumer at `beforeSeq - limit*mult` and widen if we don't get enough
// matching events. The post-filter inside fetchRoomEventsWithConsumer ensures
// only events with sequence < beforeSeq are returned.
func (c *ChattoCore) getRoomEventsPagination(
	ctx context.Context,
	stream jetstream.Stream, kind RoomKind, room_id string,
	limit int,
	beforeSeq uint64,
	filterSubjects []string,
	streamInfo *jetstream.StreamInfo,
) (*RoomEventsResult, error) {
	firstSeq := streamInfo.State.FirstSeq

	if beforeSeq <= firstSeq {
		// Cursor points to or past the start of the stream — nothing older.
		return &RoomEventsResult{HasNewer: true}, nil
	}

	multipliers := []uint64{3, 10, 50}
	for _, mult := range multipliers {
		startSeq := beforeSeq - uint64(limit)*mult
		if startSeq < firstSeq || startSeq >= beforeSeq {
			startSeq = firstSeq
		}

		events, err := c.fetchRoomEventsWithConsumer(ctx, stream, filterSubjects, jetstream.ConsumerConfig{
			FilterSubjects:    filterSubjects,
			DeliverPolicy:     jetstream.DeliverByStartSequencePolicy,
			OptStartSeq:       startSeq,
			AckPolicy:         jetstream.AckNonePolicy,
			MemoryStorage:     true,
			InactiveThreshold: 10 * time.Second,
		}, &beforeSeq)
		if err != nil {
			return nil, err
		}

		if len(events) >= limit || startSeq == firstSeq {
			hasOlder := len(events) > limit || startSeq > firstSeq
			if len(events) > limit {
				events = events[len(events)-limit:]
			}
			c.logger.Debug("Fetched room events (pagination)", "kind", kind, "room_id", room_id, "count", len(events), "multiplier", mult)
			result := roomEventsResult(events, hasOlder, true)
			return result, nil
		}
	}

	// Fallback: full scan with cursor filter
	events, err := c.fetchRoomEventsWithConsumer(ctx, stream, filterSubjects, jetstream.ConsumerConfig{
		FilterSubjects:    filterSubjects,
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		AckPolicy:         jetstream.AckNonePolicy,
		MemoryStorage:     true,
		InactiveThreshold: 10 * time.Second,
	}, &beforeSeq)
	if err != nil {
		return nil, err
	}
	hasOlder := len(events) > limit
	if len(events) > limit {
		events = events[len(events)-limit:]
	}
	c.logger.Debug("Fetched room events (pagination fallback)", "kind", kind, "room_id", room_id, "count", len(events))
	return roomEventsResult(events, hasOlder, true), nil
}

// roomEventsResult assembles a RoomEventsResult and computes the start/end
// cursor sequences from the events slice. Returns a result with zero
// cursors if events is empty.
func roomEventsResult(events []*RoomEvent, hasOlder, hasNewer bool) *RoomEventsResult {
	r := &RoomEventsResult{
		Events:   events,
		HasOlder: hasOlder,
		HasNewer: hasNewer,
	}
	if len(events) > 0 {
		r.StartCursorSeq = events[0].Sequence
		r.EndCursorSeq = events[len(events)-1].Sequence
	}
	return r
}

// fetchRoomEventsWithConsumer creates an ephemeral consumer, fetches all matching events,
// filters them by sequence cursor, and cleans up the consumer. This is the shared fetch
// logic used by all GetRoomEvents code paths. If beforeSeq is non-nil, only events with
// JetStream stream sequence strictly less than *beforeSeq are returned.
func (c *ChattoCore) fetchRoomEventsWithConsumer(
	ctx context.Context,
	stream jetstream.Stream,
	filterSubjects []string,
	config jetstream.ConsumerConfig,
	beforeSeq *uint64,
) ([]*RoomEvent, error) {
	consumer, err := stream.CreateConsumer(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}
	defer stream.DeleteConsumer(context.Background(), consumer.CachedInfo().Name)

	info, err := consumer.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get consumer info: %w", err)
	}

	numPending := info.NumPending
	if numPending == 0 {
		return nil, nil
	}

	msgs, err := consumer.Fetch(int(numPending), jetstream.FetchMaxWait(5*time.Second))
	if err != nil && !errors.Is(err, jetstream.ErrNoMessages) {
		return nil, fmt.Errorf("failed to fetch messages: %w", err)
	}

	var events []*RoomEvent
	if msgs != nil {
		for msg := range msgs.Messages() {
			meta, err := msg.Metadata()
			if err != nil {
				continue
			}
			seq := meta.Sequence.Stream

			// Filter: only include events strictly before the cursor sequence.
			if beforeSeq != nil && seq >= *beforeSeq {
				continue
			}

			var event corev1.Event
			if err := proto.Unmarshal(msg.Data(), &event); err != nil {
				continue
			}

			// Skip events with unknown/removed inner types (e.g., old ThreadReplyEchoEvent)
			if event.Event == nil {
				continue
			}

			events = append(events, &RoomEvent{Event: &event, Sequence: seq})
		}
	}

	return events, nil
}

// GetRoomEventsAround fetches room events centered around a specific event.
// Returns a window of events with the target event roughly in the middle.
// Authorization: Caller must verify room membership before calling.
func (c *ChattoCore) GetRoomEventsAround(ctx context.Context, kind RoomKind, roomID, eventID string, limit int) (*RoomEventsAroundResult, error) {
	if limit <= 0 {
		limit = defaultHistoricalMessageLimit
	}

	// 1. Look up the target event's JetStream sequence (O(1) subject lookup)
	targetSeq, err := c.GetEventSequence(ctx, kind, roomID, eventID)
	if err != nil {
		return nil, fmt.Errorf("failed to get target event sequence: %w", err)
	}
	if targetSeq == 0 {
		return nil, fmt.Errorf("event not found: %s", eventID)
	}

	// 2. Get the stream and filter subjects
	stream := c.storage.serverEventsStream

	filterSubjects := subjects.RoomRootEventsFilters(string(kind), roomID)

	streamInfo, err := stream.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream info: %w", err)
	}
	firstSeq := streamInfo.State.FirstSeq

	// 3. Use progressive multiplier pattern to fetch events around the target.
	// We want limit/2 events before the target and limit/2 after.
	// The stream is shared across all rooms, so we need to over-fetch.
	halfLimit := limit / 2

	multipliers := []uint64{3, 10, 50}
	for _, mult := range multipliers {
		// Start well before the target to ensure we get enough events before it
		startSeq := targetSeq - uint64(halfLimit)*mult
		if startSeq < firstSeq || startSeq > targetSeq {
			startSeq = firstSeq
		}

		events, err := c.fetchRoomEventsWithConsumer(ctx, stream, filterSubjects, jetstream.ConsumerConfig{
			FilterSubjects:    filterSubjects,
			DeliverPolicy:     jetstream.DeliverByStartSequencePolicy,
			OptStartSeq:       startSeq,
			AckPolicy:         jetstream.AckNonePolicy,
			MemoryStorage:     true,
			InactiveThreshold: 10 * time.Second,
		}, nil) // No end time filter — we want events after the target too
		if err != nil {
			return nil, err
		}

		// Find the target event in the fetched results
		targetIdx := -1
		for i, e := range events {
			if e.Id == eventID {
				targetIdx = i
				break
			}
		}

		if targetIdx == -1 {
			// Target not found in this fetch window — widen and retry
			if startSeq == firstSeq {
				// Already scanning from the beginning — target must not match filters
				return nil, fmt.Errorf("event %s not found in room root events", eventID)
			}
			continue
		}

		// We have enough events before the target (or started from the beginning)
		beforeCount := targetIdx
		if beforeCount >= halfLimit || startSeq == firstSeq {
			// Slice the window: halfLimit before + target + halfLimit after
			windowStart := targetIdx - halfLimit
			if windowStart < 0 {
				windowStart = 0
			}
			windowEnd := targetIdx + halfLimit + 1
			if windowEnd > len(events) {
				windowEnd = len(events)
			}

			windowEvents := events[windowStart:windowEnd]
			newTargetIdx := targetIdx - windowStart

			return &RoomEventsAroundResult{
				Events:      windowEvents,
				TargetIndex: newTargetIdx,
				HasOlder:    windowStart > 0 || startSeq > firstSeq,
				HasNewer:    windowEnd < len(events),
			}, nil
		}
		// Not enough events before target — widen and retry
	}

	// Fallback: scan from beginning
	events, err := c.fetchRoomEventsWithConsumer(ctx, stream, filterSubjects, jetstream.ConsumerConfig{
		FilterSubjects:    filterSubjects,
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		AckPolicy:         jetstream.AckNonePolicy,
		MemoryStorage:     true,
		InactiveThreshold: 10 * time.Second,
	}, nil)
	if err != nil {
		return nil, err
	}

	targetIdx := -1
	for i, e := range events {
		if e.Id == eventID {
			targetIdx = i
			break
		}
	}
	if targetIdx == -1 {
		return nil, fmt.Errorf("event %s not found in room root events", eventID)
	}

	windowStart := targetIdx - halfLimit
	if windowStart < 0 {
		windowStart = 0
	}
	windowEnd := targetIdx + halfLimit + 1
	if windowEnd > len(events) {
		windowEnd = len(events)
	}

	windowEvents := events[windowStart:windowEnd]
	newTargetIdx := targetIdx - windowStart

	c.logger.Debug("Fetched room events around target (fallback)", "kind", kind, "room_id", roomID, "count", len(windowEvents))
	return &RoomEventsAroundResult{
		Events:      windowEvents,
		TargetIndex: newTargetIdx,
		HasOlder:    windowStart > 0,
		HasNewer:    windowEnd < len(events),
	}, nil
}

// GetRoomEventsAfter fetches room events after a given sequence cursor.
// Used for forward pagination in "jump to message" mode.
// Authorization: Caller must verify room membership before calling.
func (c *ChattoCore) GetRoomEventsAfter(ctx context.Context, kind RoomKind, roomID string, afterSeq uint64, limit int) (*RoomEventsResult, error) {
	if limit <= 0 {
		limit = defaultHistoricalMessageLimit
	}

	stream := c.storage.serverEventsStream

	filterSubjects := subjects.RoomRootEventsFilters(string(kind), roomID)

	// Start the consumer at the sequence immediately after the cursor.
	// JetStream returns messages with stream sequence >= OptStartSeq, so
	// `afterSeq + 1` excludes the cursor event itself.
	startSeq := afterSeq + 1
	events, err := c.fetchRoomEventsWithConsumer(ctx, stream, filterSubjects, jetstream.ConsumerConfig{
		FilterSubjects:    filterSubjects,
		DeliverPolicy:     jetstream.DeliverByStartSequencePolicy,
		OptStartSeq:       startSeq,
		AckPolicy:         jetstream.AckNonePolicy,
		MemoryStorage:     true,
		InactiveThreshold: 10 * time.Second,
	}, nil)
	if err != nil {
		return nil, err
	}

	// Take the first `limit` events (forward pagination)
	hasNewer := len(events) > limit
	if hasNewer {
		events = events[:limit]
	}

	c.logger.Debug("Fetched room events after cursor", "kind", kind, "room_id", roomID, "count", len(events))
	r := &RoomEventsResult{
		Events:   events,
		HasOlder: true, // Forward pagination always has older events (those before the cursor)
		HasNewer: hasNewer,
	}
	if len(events) > 0 {
		r.StartCursorSeq = events[0].Sequence
		r.EndCursorSeq = events[len(events)-1].Sequence
	}
	return r, nil
}

// getRoomEventMsg fetches the raw JetStream message for an event by its event ID.
// Supports both root messages and thread replies via O(1) subject lookup.
// Returns nil if the event doesn't exist.
func (c *ChattoCore) getRoomEventMsg(ctx context.Context, kind RoomKind, roomID, eventID string) (*jetstream.RawStreamMsg, error) {
	stream := c.storage.serverEventsStream

	// First, try root message subject pattern: space.{s}.room.{r}.msg.{eventId}
	subject := subjects.RoomMessage(string(kind), roomID, eventID)
	msg, err := stream.GetLastMsgForSubject(ctx, subject)
	if err != nil && !errors.Is(err, jetstream.ErrMsgNotFound) {
		return nil, fmt.Errorf("failed to get message by subject: %w", err)
	}

	// If not found as root message, try thread reply pattern: space.{s}.room.{r}.thread.*.{eventId}
	if msg == nil {
		threadSubject := subjects.RoomThreadLookup(string(kind), roomID, eventID)
		msg, err = stream.GetLastMsgForSubject(ctx, threadSubject)
		if err != nil {
			if errors.Is(err, jetstream.ErrMsgNotFound) {
				return nil, nil
			}
			return nil, fmt.Errorf("failed to get thread message by subject: %w", err)
		}
	}

	return msg, nil
}

// GetRoomEventByEventID fetches a room event by its event ID using O(1) subject lookup.
// Supports both root messages and thread replies.
// Returns nil if the event doesn't exist.
// Authorization: Caller must verify room membership before calling.
func (c *ChattoCore) GetRoomEventByEventID(ctx context.Context, kind RoomKind, roomID, eventID string) (*corev1.Event, error) {
	msg, err := c.getRoomEventMsg(ctx, kind, roomID, eventID)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, nil
	}

	var event corev1.Event
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		return nil, fmt.Errorf("failed to unmarshal event: %w", err)
	}

	// Return nil for events with unknown/removed inner types (e.g., old ThreadReplyEchoEvent)
	if event.Event == nil {
		return nil, nil
	}

	return &event, nil
}

// GetEventSequence returns the JetStream stream sequence number for an event by its event ID.
// Returns 0 if the event doesn't exist.
func (c *ChattoCore) GetEventSequence(ctx context.Context, kind RoomKind, roomID, eventID string) (uint64, error) {
	msg, err := c.getRoomEventMsg(ctx, kind, roomID, eventID)
	if err != nil {
		return 0, err
	}
	if msg == nil {
		return 0, nil
	}
	return msg.Sequence, nil
}

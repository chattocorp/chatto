package core

import (
	"context"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// RoomEvent pairs a *corev1.Event with its stream sequence so the
// pagination layer can build opaque cursors without re-deriving the
// sequence per event. Event is embedded so callers can access event
// fields directly (`event.Id`, `event.GetMessagePosted()`, etc.).
type RoomEvent struct {
	*corev1.Event
	Sequence uint64
}

// RoomEventsResult is the return type for paginated room event queries.
// HasOlder/HasNewer indicate whether more events exist beyond the
// returned page. StartCursorSeq/EndCursorSeq are stream sequences for
// the first and last event in the page; API layers render them as opaque
// cursor strings. Both are zero when Events is empty.
type RoomEventsResult struct {
	Events         []*RoomEvent
	HasOlder       bool
	HasNewer       bool
	StartCursorSeq uint64
	EndCursorSeq   uint64
}

// RoomEventsAroundResult contains the result of fetching events around
// a target event.
type RoomEventsAroundResult struct {
	Events      []*RoomEvent
	TargetIndex int
	HasOlder    bool
	HasNewer    bool
}

// GetRoomEvents returns up to `limit` most recent room timeline entries
// in oldest-first order (chronological — matches the legacy
// SERVER_EVENTS-backed shape). If `beforeSeq` is non-nil, only entries
// with stream sequence strictly less than `*beforeSeq` are returned.
//
// Reads from the in-memory RoomTimelineProjection (ADR-033 / #597). The
// projection stores the room-visible event log; folded facts such as thread
// replies, edits, retractions, reactions, and asset processing are handled by
// sibling projections or derived indexes.
//
// Authorization: caller must verify room membership before calling.
// The `kind` parameter is retained on the signature for API stability
// with the legacy SERVER_EVENTS-backed implementation; the projection
// is kind-agnostic (the room aggregate's subject is
// evt.room.{R}.{eventType} — kind is a property of the room, not the
// event).
func (c *ChattoCore) GetRoomEvents(ctx context.Context, kind RoomKind, room_id string, limit int, beforeSeq *uint64) (*RoomEventsResult, error) {
	limit = clampHistoricalMessageLimit(limit)
	var before uint64
	if beforeSeq != nil {
		before = *beforeSeq
	}

	// Bounded newest-first walk via the visible-room timeline. Fetch
	// limit+1 to detect HasOlder without a second call.
	raw := c.roomModel.visibleRoomTimeline(room_id, limit+1, before, nil)
	hasOlder := len(raw) > limit
	if hasOlder {
		raw = raw[:limit]
	}
	visible := make([]*RoomEvent, len(raw))
	for i, e := range raw {
		visible[i] = &RoomEvent{Event: e.Event, Sequence: e.StreamSeq}
	}

	// Reverse newest-first → oldest-first to match legacy callers +
	// frontend expectations.
	for i, j := 0, len(visible)-1; i < j; i, j = i+1, j-1 {
		visible[i], visible[j] = visible[j], visible[i]
	}

	r := &RoomEventsResult{
		Events:   visible,
		HasOlder: hasOlder,
		HasNewer: beforeSeq != nil,
	}
	if len(visible) > 0 {
		r.StartCursorSeq = visible[0].Sequence
		r.EndCursorSeq = visible[len(visible)-1].Sequence
	}
	return r, nil
}

// GetRoomEventByEventID returns a room event by its envelope id, or
// nil if not found. Supports posted messages, including thread replies and
// channel echoes, plus visible lifecycle/meta events.
//
// Authorization: caller must verify room membership before calling.
func (c *ChattoCore) GetRoomEventByEventID(ctx context.Context, kind RoomKind, roomID, eventID string) (*corev1.Event, error) {
	entry, ok := c.roomModel.timelineEntry(eventID)
	if !ok {
		return nil, nil
	}
	if entry.Event.GetEvent() == nil {
		return nil, nil
	}
	if c.roomModel.isHiddenEcho(eventID) {
		return nil, nil
	}
	// Honour the roomID scope — looking up an event in the wrong
	// room should be a clean miss, not a leak.
	if roomIDOfEvent(entry.Event) != roomID {
		return nil, nil
	}
	return entry.Event, nil
}

// GetRoomEventsAround returns a window of events centered on a target
// event ID. The result includes `limit/2` events before and after the
// target, with the target at TargetIndex.
//
// Authorization: caller must verify room membership before calling.
func (c *ChattoCore) GetRoomEventsAround(ctx context.Context, kind RoomKind, roomID, eventID string, limit int) (*RoomEventsAroundResult, error) {
	limit = clampHistoricalMessageLimit(limit)

	target, ok := c.roomModel.timelineEntry(eventID)
	if !ok {
		return nil, ErrMessageNotFound
	}
	if c.roomModel.isHiddenEcho(eventID) {
		return nil, ErrMessageNotFound
	}
	if !isVisibleRoomTimelineEntry(target.Event) {
		// Target is a thread reply or otherwise filtered from the
		// room timeline. The legacy implementation returned an
		// "event not found in room root events" error here; preserve
		// that posture.
		return nil, ErrMessageNotFound
	}

	raw, targetIdx, hasOlder, hasNewer, ok := c.roomModel.visibleRoomTimelineAround(roomID, eventID, limit)
	if !ok {
		return nil, ErrMessageNotFound
	}
	window := make([]*RoomEvent, len(raw))
	for i, e := range raw {
		window[i] = &RoomEvent{Event: e.Event, Sequence: e.StreamSeq}
	}
	return &RoomEventsAroundResult{
		Events:      window,
		TargetIndex: targetIdx,
		HasOlder:    hasOlder,
		HasNewer:    hasNewer,
	}, nil
}

// GetRoomEventsAfter returns up to `limit` events with stream sequence
// strictly greater than afterSeq, oldest-first (i.e. forward
// pagination order).
//
// Authorization: caller must verify room membership before calling.
func (c *ChattoCore) GetRoomEventsAfter(ctx context.Context, kind RoomKind, roomID string, afterSeq uint64, limit int) (*RoomEventsResult, error) {
	limit = clampHistoricalMessageLimit(limit)

	// Walk visible entries oldest-first from the cursor so forward
	// pagination returns the nearest newer events first. Fetch limit+1
	// to detect whether another forward page exists.
	raw := c.roomModel.visibleRoomTimelineAfter(roomID, limit+1, afterSeq, nil)
	hasNewer := len(raw) > limit
	if hasNewer {
		raw = raw[:limit]
	}
	newer := make([]*RoomEvent, 0, len(raw))
	for _, e := range raw {
		newer = append(newer, &RoomEvent{Event: e.Event, Sequence: e.StreamSeq})
	}

	r := &RoomEventsResult{
		Events:   newer,
		HasOlder: true, // forward pagination always has older content (everything <= afterSeq).
		HasNewer: hasNewer,
	}
	if len(newer) > 0 {
		r.StartCursorSeq = newer[0].Sequence
		r.EndCursorSeq = newer[len(newer)-1].Sequence
	}
	return r, nil
}

func clampHistoricalMessageLimit(limit int) int {
	if limit <= 0 {
		return defaultHistoricalMessageLimit
	}
	if limit > maxHistoricalMessageLimit {
		return maxHistoricalMessageLimit
	}
	return limit
}

// GetEventSequence returns the stream sequence number for an event by
// its envelope id, or 0 if not found.
func (c *ChattoCore) GetEventSequence(ctx context.Context, kind RoomKind, roomID, eventID string) (uint64, error) {
	entry, ok := c.roomModel.timelineEntry(eventID)
	if !ok {
		return 0, nil
	}
	return entry.StreamSeq, nil
}

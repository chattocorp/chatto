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
// the first and last event in the page; the GraphQL layer renders them
// as opaque cursor strings. Both are zero when Events is empty.
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
// projection holds every event under the room aggregate; this method
// filters thread replies out of the channel view (thread replies are
// served via GetThreadEvents).
//
// Authorization: caller must verify room membership before calling.
// The `kind` parameter is retained on the signature for API stability
// with the legacy SERVER_EVENTS-backed implementation; the projection
// is kind-agnostic (the room aggregate's subject is
// evt.room.{R}.{eventType} — kind is a property of the room, not the
// event).
func (c *ChattoCore) GetRoomEvents(ctx context.Context, kind RoomKind, room_id string, limit int, beforeSeq *uint64) (*RoomEventsResult, error) {
	if limit <= 0 {
		limit = defaultHistoricalMessageLimit
	}
	var before uint64
	if beforeSeq != nil {
		before = *beforeSeq
	}

	// Bounded newest-first walk via VisibleRoomTimeline. Fetch
	// limit+1 to detect HasOlder without a second call.
	raw := c.RoomTimeline.VisibleRoomTimeline(room_id, limit+1, before, isVisibleRoomTimelineEntry)
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
// nil if not found. Supports root messages, thread replies, and
// lifecycle/meta events alike — all live in the same RoomTimeline
// projection.
//
// Authorization: caller must verify room membership before calling.
func (c *ChattoCore) GetRoomEventByEventID(ctx context.Context, kind RoomKind, roomID, eventID string) (*corev1.Event, error) {
	entry, ok := c.RoomTimeline.Get(eventID)
	if !ok {
		return nil, nil
	}
	if entry.Event.GetEvent() == nil {
		return nil, nil
	}
	if c.RoomTimeline.IsHiddenEcho(eventID) {
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
	if limit <= 0 {
		limit = defaultHistoricalMessageLimit
	}

	target, ok := c.RoomTimeline.Get(eventID)
	if !ok {
		return nil, ErrMessageNotFound
	}
	if c.RoomTimeline.IsHiddenEcho(eventID) {
		return nil, ErrMessageNotFound
	}
	if !isVisibleRoomTimelineEntry(target.Event) {
		// Target is a thread reply or otherwise filtered from the
		// room timeline. The legacy implementation returned an
		// "event not found in room root events" error here; preserve
		// that posture.
		return nil, ErrMessageNotFound
	}

	// Walk the room's visible timeline newest-first, then reverse it
	// to the API's chronological oldest-first contract. RoomEventCount
	// gives an upper bound so we don't ask for an unbounded slice.
	roomLen := c.RoomTimeline.RoomEventCount(roomID)
	raw := c.RoomTimeline.VisibleRoomTimeline(roomID, roomLen, 0, isVisibleRoomTimelineEntry)
	visible := make([]*RoomEvent, len(raw))
	for i, e := range raw {
		visible[i] = &RoomEvent{Event: e.Event, Sequence: e.StreamSeq}
	}
	for i, j := 0, len(visible)-1; i < j; i, j = i+1, j-1 {
		visible[i], visible[j] = visible[j], visible[i]
	}

	targetIdx := -1
	for i, e := range visible {
		if e.Id == eventID {
			targetIdx = i
			break
		}
	}
	if targetIdx == -1 {
		return nil, ErrMessageNotFound
	}

	// visible[] is chronological oldest-first. Return the around-window
	// in the same order so start/end cursors mean oldest/newest just like
	// Room.events.
	half := limit / 2
	start := targetIdx - half
	if start < 0 {
		start = 0
	}
	end := targetIdx + half + 1
	if end > len(visible) {
		end = len(visible)
	}

	window := make([]*RoomEvent, end-start)
	copy(window, visible[start:end])
	return &RoomEventsAroundResult{
		Events:      window,
		TargetIndex: targetIdx - start,
		HasOlder:    start > 0,
		HasNewer:    end < len(visible),
	}, nil
}

// GetRoomEventsAfter returns up to `limit` events with stream sequence
// strictly greater than afterSeq, oldest-first (i.e. forward
// pagination order).
//
// Authorization: caller must verify room membership before calling.
func (c *ChattoCore) GetRoomEventsAfter(ctx context.Context, kind RoomKind, roomID string, afterSeq uint64, limit int) (*RoomEventsResult, error) {
	if limit <= 0 {
		limit = defaultHistoricalMessageLimit
	}

	// Walk visible entries oldest-first from the cursor so forward
	// pagination returns the nearest newer events first. Fetch limit+1
	// to detect whether another forward page exists.
	raw := c.RoomTimeline.VisibleRoomTimelineAfter(roomID, limit+1, afterSeq, isVisibleRoomTimelineEntry)
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

// GetEventSequence returns the stream sequence number for an event by
// its envelope id, or 0 if not found.
func (c *ChattoCore) GetEventSequence(ctx context.Context, kind RoomKind, roomID, eventID string) (uint64, error) {
	entry, ok := c.RoomTimeline.Get(eventID)
	if !ok {
		return 0, nil
	}
	return entry.StreamSeq, nil
}

// isVisibleRoomTimelineEntry reports whether a timeline entry should
// surface in the room-level view (GetRoomEvents and friends).
//
// Hidden:
//   - Thread replies (MessagePostedEvent with in_thread != "") —
//     served via GetThreadEvents.
//   - MessageEditedEvent / MessageRetractedEvent — folded onto the
//     original post via projection.LatestBody; not surfaced as
//     separate timeline entries.
//   - ReactionAddedEvent / ReactionRemovedEvent — folded into the
//     reaction projection.
//
// Visible: root messages, room lifecycle (created/updated/archived/
// unarchived/deleted), memberships (user_joined / user_left).
func isVisibleRoomTimelineEntry(event *corev1.Event) bool {
	if event == nil {
		return false
	}
	switch e := event.GetEvent().(type) {
	case *corev1.Event_MessagePosted:
		return e.MessagePosted.GetInThread() == ""
	case *corev1.Event_MessageEdited, *corev1.Event_MessageRetracted,
		*corev1.Event_ThreadCreated,
		*corev1.Event_AssetCreated, *corev1.Event_AssetDeleted,
		*corev1.Event_AssetProcessingStarted,
		*corev1.Event_AssetProcessingSucceeded, *corev1.Event_AssetProcessingFailed,
		*corev1.Event_ReactionAdded, *corev1.Event_ReactionRemoved:
		return false
	}
	return true
}

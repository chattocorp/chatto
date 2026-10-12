package core

import (
	"context"
	"fmt"
	"time"
)

// ============================================================================
// Unread Message Tracking
// ============================================================================
//
// Per-user, per-room read state lives in RUNTIME_STATE and is keyed on the
// last-read root message's stable event ID (14-char NanoID, see ADR-026), not
// on the (volatile) JetStream sequence number. Event IDs are embedded in NATS
// subjects, so they survive stream renumbering and rebuilds. See docs/adr/ADR-028
// for rationale.
//
// The legacy `room_read_status.*` keys (uint64 sequence numbers) are orphaned
// and ignored. Users with no `read.room.*` key are lazy-initialized to the
// room's current last root event on first read — the "caught up at deploy
// time" semantic.

const maxReadMarkerUpdateRetries = 5

// LastReadEventIDAdvance describes the result of an advance-only room read
// marker update.
type LastReadEventIDAdvance struct {
	PreviousEventID string
	PreviousTime    time.Time
	CurrentEventID  string
	CurrentTime     time.Time
	Updated         bool
}

// NotifyRoomMarkedAsRead publishes a live event to notify the user that they marked
// a room as read. This enables real-time updates to space unread indicators.
// This is best-effort - failures are logged but don't affect the mark-as-read operation.
func (c *ChattoCore) NotifyRoomMarkedAsRead(ctx context.Context, userID string, kind RoomKind, roomID string) {
	c.readStateModel.NotifyRoomMarkedAsRead(ctx, userID, kind, roomID)
}

// GetRoomLastEvent returns the last root message's event ID and proto-level
// `created_at` timestamp for a room. Excludes thread replies — only root
// messages affect room-level unread tracking. exists is false if the room
// has no ordinary root messages. Historical imports do not advance this marker.
//
// Uses the proto's `created_at` rather than JetStream's stored time so the
// value stays correct after #354 phase 4d (which re-publishes messages
// with fresh JetStream timestamps but leaves the proto payloads intact).
func (c *ChattoCore) GetRoomLastEvent(ctx context.Context, kind RoomKind, roomID string) (eventID string, ts time.Time, exists bool, err error) {
	return c.readStateModel.GetRoomLastEvent(ctx, kind, roomID)
}

// GetRoomLastReadableEvent returns the most recent room-visible message that
// userID can currently read. Channel echoes use the interaction relationship
// of their canonical thread root.
func (c *ChattoCore) GetRoomLastReadableEvent(ctx context.Context, kind RoomKind, userID, roomID string) (eventID string, ts time.Time, exists bool, err error) {
	return c.readStateModel.GetRoomLastReadableEvent(ctx, kind, userID, roomID)
}

// roomReadEventKey returns the RUNTIME_STATE key for tracking the user's
// last-read root event ID in a room.
func roomReadEventKey(userID, roomID string) string {
	return fmt.Sprintf("read.room.%s.%s", userID, roomID)
}

// GetLastReadEventID returns the user's last-read root-message event ID for a
// room. If no marker exists yet, it lazy-initializes the marker to the room's
// current last root event ("caught up at deploy time"); if the room has no
// messages, it returns "".
func (c *ChattoCore) GetLastReadEventID(ctx context.Context, kind RoomKind, userID, roomID string) (string, error) {
	return c.readStateModel.GetLastReadEventID(ctx, kind, userID, roomID)
}

// PeekLastReadEventID returns the stored read marker for a room without lazy
// initialization. exists is false when no marker has been written yet.
func (c *ChattoCore) PeekLastReadEventID(ctx context.Context, userID, roomID string) (eventID string, exists bool, err error) {
	return c.readStateModel.PeekLastReadEventID(ctx, userID, roomID)
}

// SetLastReadEventID stores the user's last-read root-message event ID.
//
// Callers MUST pass either a root message event ID or the empty string. Thread
// reply event IDs are tracked separately by the thread read marker; using one
// here would make room-level unread comparisons point at the wrong timeline.
func (c *ChattoCore) SetLastReadEventID(ctx context.Context, kind RoomKind, userID, roomID, eventID string) error {
	return c.readStateModel.SetLastReadEventID(ctx, kind, userID, roomID, eventID)
}

// initializeLastReadEventID creates the member's initial room marker without
// replacing a marker already written by another replica or a concurrent
// user-facing read operation.
func (c *ChattoCore) initializeLastReadEventID(ctx context.Context, userID, roomID, eventID string) error {
	return c.readStateModel.initializeLastReadEventID(ctx, userID, roomID, eventID)
}

// AdvanceLastReadEventID stores eventID as the user's room read marker only if
// it is newer than the marker already in RUNTIME_STATE. The compare-and-write
// loop uses KV revisions so concurrent replicas cannot move the marker
// backwards after seeing stale state.
func (c *ChattoCore) AdvanceLastReadEventID(ctx context.Context, kind RoomKind, userID, roomID, eventID string) (*LastReadEventIDAdvance, error) {
	return c.readStateModel.AdvanceLastReadEventID(ctx, kind, userID, roomID, eventID)
}

func shouldAdvanceReadMarker(currentEventID string, currentTime time.Time, nextEventID string, nextTime time.Time) bool {
	if currentEventID == nextEventID {
		return false
	}
	if nextEventID == "" || nextTime.IsZero() {
		return currentEventID == ""
	}
	if currentEventID == "" || currentTime.IsZero() {
		return true
	}
	return nextTime.After(currentTime)
}

// GetEventTimestamp returns the proto-level `created_at` timestamp for a
// root message event ID in a room. Returns zero time if the event ID is
// empty or the message doesn't exist (e.g. deleted or never published).
//
// Reads from the proto payload rather than JetStream's stored time so the
// value stays correct after #354 phase 4d (which re-publishes with fresh
// JetStream timestamps but preserves the proto payload).
func (c *ChattoCore) GetEventTimestamp(ctx context.Context, kind RoomKind, roomID, eventID string) (time.Time, error) {
	return c.readStateModel.GetEventTimestamp(ctx, kind, roomID, eventID)
}

// HasUnread reports whether a room has active Badge attention for a user.
// Thread Badge attention rolls up into the parent room. The result is independent
// of the user's last-read cursor and false when the user cannot see the room.
func (c *ChattoCore) HasUnread(ctx context.Context, kind RoomKind, userID, roomID string) (bool, error) {
	return c.readStateModel.HasUnread(ctx, kind, userID, roomID)
}

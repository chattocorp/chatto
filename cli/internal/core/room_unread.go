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

// roomReadEventKey returns the RUNTIME_STATE key for tracking the user's
// last-read root event ID in a room.
func roomReadEventKey(userID, roomID string) string {
	return fmt.Sprintf("read.room.%s.%s", userID, roomID)
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

// HasUnread reports whether a room has active Badge attention for a user.
// Thread Badge attention rolls up into the parent room. The result is independent
// of the user's last-read cursor and false when the user cannot see the room.
func (c *ChattoCore) HasUnread(ctx context.Context, kind RoomKind, userID, roomID string) (bool, error) {
	return c.readStateModel.HasUnread(ctx, kind, userID, roomID)
}

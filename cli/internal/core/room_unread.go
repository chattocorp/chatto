package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
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

// NotifyRoomMarkedAsRead publishes a live event to notify the user that they marked
// a room as read. This enables real-time updates to space unread indicators.
// This is best-effort - failures are logged but don't affect the mark-as-read operation.
func (c *ChattoCore) NotifyRoomMarkedAsRead(ctx context.Context, userID string, kind RoomKind, roomID string) {
	event := newLiveEvent(userID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_RoomMarkedAsRead{
			RoomMarkedAsRead: &corev1.RoomMarkedAsReadEvent{
				RoomId: roomID,
			},
		},
	})

	// Publish to user's server event stream (only they need to know)
	subject := subjects.LiveSyncUserEvent(userID, "room_read")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Warn("Failed to publish room marked as read event",
			"user_id", userID,
			"kind", kind,
			"room_id", roomID,
			"error", err)
	}
}

// GetRoomLastEvent returns the last root message's event ID and proto-level
// `created_at` timestamp for a room. Excludes thread replies — only root
// messages affect room-level unread tracking. exists is false if the room
// has no root messages.
//
// Uses the proto's `created_at` rather than JetStream's stored time so the
// value stays correct after #354 phase 4d (which re-publishes messages
// with fresh JetStream timestamps but leaves the proto payloads intact).
func (c *ChattoCore) GetRoomLastEvent(ctx context.Context, kind RoomKind, roomID string) (eventID string, ts time.Time, exists bool, err error) {
	ev := c.getRoomLastRootEvent(roomID)
	if ev == nil {
		return "", time.Time{}, false, nil
	}
	var createdAt time.Time
	if ts := ev.GetCreatedAt(); ts != nil {
		createdAt = ts.AsTime()
	}
	return ev.GetId(), createdAt, true, nil
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
	bucket := c.storage.runtimeStateKV

	key := roomReadEventKey(userID, roomID)
	entry, err := bucket.Get(ctx, key)
	if err == nil {
		return string(entry.Value()), nil
	}
	if !errors.Is(err, jetstream.ErrKeyNotFound) {
		return "", fmt.Errorf("failed to get read marker: %w", err)
	}

	// No marker yet — initialize to the room's current last root event so the
	// user starts caught up rather than seeing a wall of unreads.
	lastID, _, exists, err := c.GetRoomLastEvent(ctx, kind, roomID)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", nil
	}
	// Use Create (atomic insert) rather than Put: a concurrent writer
	// (PostMessage auto-mark, MarkRoomAsRead) may have set a real marker
	// between our Get and our write, and we must not clobber it.
	if _, err := bucket.Create(ctx, key, []byte(lastID)); err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			entry, getErr := bucket.Get(ctx, key)
			if getErr != nil {
				return "", fmt.Errorf("failed to re-read read marker after concurrent init: %w", getErr)
			}
			return string(entry.Value()), nil
		}
		c.logger.Warn("Failed to lazy-initialize read marker", "user_id", userID, "room_id", roomID, "error", err)
	}
	return lastID, nil
}

// SetLastReadEventID stores the user's last-read root-message event ID.
//
// Callers MUST pass either a root message event ID or the empty string. Thread
// reply event IDs are tracked separately by the thread read marker; using one
// here would make room-level unread comparisons point at the wrong timeline.
func (c *ChattoCore) SetLastReadEventID(ctx context.Context, kind RoomKind, userID, roomID, eventID string) error {
	bucket := c.storage.runtimeStateKV
	if _, err := bucket.Put(ctx, roomReadEventKey(userID, roomID), []byte(eventID)); err != nil {
		return fmt.Errorf("failed to set read marker: %w", err)
	}
	c.logger.Debug("Set last read event", "user_id", userID, "room_id", roomID, "event_id", eventID)
	return nil
}

// GetEventTimestamp returns the proto-level `created_at` timestamp for a
// root message event ID in a room. Returns zero time if the event ID is
// empty or the message doesn't exist (e.g. deleted or never published).
//
// Reads from the proto payload rather than JetStream's stored time so the
// value stays correct after #354 phase 4d (which re-publishes with fresh
// JetStream timestamps but preserves the proto payload).
func (c *ChattoCore) GetEventTimestamp(ctx context.Context, kind RoomKind, roomID, eventID string) (time.Time, error) {
	if eventID == "" {
		return time.Time{}, nil
	}
	entry, ok := c.RoomTimeline.Get(eventID)
	if !ok {
		return time.Time{}, nil
	}
	// Honour roomID scope — same as GetRoomEventByEventID.
	if roomIDOfEvent(entry.Event) != roomID {
		return time.Time{}, nil
	}
	if ts := entry.Event.GetCreatedAt(); ts != nil {
		return ts.AsTime(), nil
	}
	return time.Time{}, nil
}

// HasUnread reports whether a room has unread messages for a user. Returns
// false if the user is not a member, the room is muted, or there are no
// messages. Compares the user's stored read marker (event ID) against the
// room's current last root message.
func (c *ChattoCore) HasUnread(ctx context.Context, kind RoomKind, userID, roomID string) (bool, error) {
	isMember, err := c.RoomMembershipExists(ctx, kind, userID, roomID)
	if err != nil {
		return false, fmt.Errorf("failed to check room membership: %w", err)
	}
	if !isMember {
		return false, nil
	}

	level, err := c.GetEffectiveNotificationLevel(ctx, userID, roomID)
	if err != nil {
		c.logger.Warn("Failed to get notification level for unread check, continuing with default",
			"user_id", userID, "kind", kind, "room_id", roomID, "error", err)
	} else if level == corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
		return false, nil
	}

	lastID, lastTime, exists, err := c.GetRoomLastEvent(ctx, kind, roomID)
	if err != nil {
		return false, err
	}
	if !exists {
		return false, nil
	}

	readID, err := c.GetLastReadEventID(ctx, kind, userID, roomID)
	if err != nil {
		return false, err
	}
	if readID == "" {
		// Member has a marker but no specific event read yet (joined an
		// empty room, then messages arrived). Anything counts as unread.
		return true, nil
	}
	if readID == lastID {
		return false, nil // Caught up — fast path
	}

	// Read marker points to an older (or deleted) message. Resolve its
	// timestamp and compare. A missing message means the marker is stale —
	// treat as unread; the user re-marks and state self-corrects.
	readTime, err := c.GetEventTimestamp(ctx, kind, roomID, readID)
	if err != nil {
		return false, err
	}
	if readTime.IsZero() {
		return true, nil
	}
	return lastTime.After(readTime), nil
}

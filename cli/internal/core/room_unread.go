package core

import (
	"context"
	"fmt"
	"hmans.de/chatto/internal/pb/chatto/core/live/v1"
	"time"

	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/jetstreamutil"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
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
	event := newLiveEvent(userID, &livev1.LiveEvent{
		Event: &livev1.LiveEvent_RoomMarkedAsRead{
			RoomMarkedAsRead: &livev1.RoomMarkedAsReadEvent{
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

// GetRoomLastReadableEvent returns the most recent room-visible message that
// userID can currently read. Channel echoes use the interaction relationship
// of their canonical thread root.
func (c *ChattoCore) GetRoomLastReadableEvent(ctx context.Context, kind RoomKind, userID, roomID string) (eventID string, ts time.Time, exists bool, err error) {
	broad, err := c.CanReadMessages(ctx, userID, kind, roomID)
	if err != nil {
		return "", time.Time{}, false, err
	}
	interactions := false
	if !broad && kind != KindDM {
		interactions, err = c.CanReadMessageInteractions(ctx, userID, kind, roomID)
		if err != nil {
			return "", time.Time{}, false, err
		}
	}
	visible := func(event *evtv1.Event) bool {
		message := event.GetMessagePosted()
		if message == nil || message.GetInThread() != "" {
			return false
		}
		if broad || kind == KindDM {
			return true
		}
		rootID, ok := c.roomModel.threadRootForMessage(roomID, event.GetId())
		return ok && interactions && c.roomModel.hasThreadInteraction(userID, roomID, rootID)
	}
	entry, ok := c.roomModel.lastVisibleRoomEntry(roomID, visible)
	if !ok || entry == nil || entry.Event == nil {
		return "", time.Time{}, false, nil
	}
	createdAt := time.Time{}
	if entry.Event.GetCreatedAt() != nil {
		createdAt = entry.Event.GetCreatedAt().AsTime()
	}
	return entry.Event.GetId(), createdAt, true, nil
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
	entry, exists, err := c.readStateModel.index.roomMarker(ctx, userID, roomID)
	if err != nil {
		return "", fmt.Errorf("read room marker index: %w", err)
	}
	if exists {
		return string(entry.value), nil
	}

	// No marker yet — initialize to the room's current last root event so the
	// user starts caught up rather than seeing a wall of unreads.
	lastID, _, exists, err := c.GetRoomLastReadableEvent(ctx, kind, userID, roomID)
	if err != nil {
		return "", err
	}
	if !exists {
		return "", nil
	}
	// Use Create (atomic insert) rather than Put: a concurrent writer
	// (PostMessage auto-mark, MarkRoomAsRead) may have set a real marker
	// between our Get and our write, and we must not clobber it.
	revision, err := bucket.Create(ctx, key, []byte(lastID))
	if err != nil {
		if jetstreamutil.IsSequenceConflict(err) {
			current, getErr := bucket.Get(ctx, key)
			if getErr != nil {
				return "", fmt.Errorf("failed to re-read read marker after concurrent init: %w", getErr)
			}
			if waitErr := c.readStateModel.index.waitForRevision(ctx, key, current.Revision()); waitErr != nil {
				return "", fmt.Errorf("wait for concurrent read marker: %w", waitErr)
			}
			return string(current.Value()), nil
		}
		c.logger.Warn("Failed to lazy-initialize read marker", "user_id", userID, "room_id", roomID, "error", err)
		return lastID, nil
	}
	if err := c.readStateModel.index.waitForRevision(ctx, key, revision); err != nil {
		return "", fmt.Errorf("wait for initialized read marker: %w", err)
	}
	return lastID, nil
}

// PeekLastReadEventID returns the stored read marker for a room without lazy
// initialization. exists is false when no marker has been written yet.
func (c *ChattoCore) PeekLastReadEventID(ctx context.Context, userID, roomID string) (eventID string, exists bool, err error) {
	entry, exists, err := c.readStateModel.index.roomMarker(ctx, userID, roomID)
	if err != nil {
		return "", false, fmt.Errorf("read room marker index: %w", err)
	}
	if !exists {
		return "", false, nil
	}
	return string(entry.value), true, nil
}

// SetLastReadEventID stores the user's last-read root-message event ID.
//
// Callers MUST pass either a root message event ID or the empty string. Thread
// reply event IDs are tracked separately by the thread read marker; using one
// here would make room-level unread comparisons point at the wrong timeline.
func (c *ChattoCore) SetLastReadEventID(ctx context.Context, kind RoomKind, userID, roomID, eventID string) error {
	bucket := c.storage.runtimeStateKV
	key := roomReadEventKey(userID, roomID)

	for attempt := 0; attempt < maxReadMarkerUpdateRetries; attempt++ {
		entry, exists, err := c.readStateModel.index.roomMarker(ctx, userID, roomID)
		if err != nil {
			return fmt.Errorf("read room marker index: %w", err)
		}

		var revision uint64
		if exists {
			revision, err = bucket.Update(ctx, key, []byte(eventID), entry.revision)
		} else {
			revision, err = bucket.Create(ctx, key, []byte(eventID))
		}
		if err != nil {
			if jetstreamutil.IsSequenceConflict(err) {
				if waitErr := c.readStateModel.index.waitForRevisionAfter(ctx, key, entry.revision); waitErr != nil {
					return fmt.Errorf("wait for conflicting read marker: %w", waitErr)
				}
				continue
			}
			return fmt.Errorf("failed to set read marker: %w", err)
		}
		if err := c.readStateModel.index.waitForRevision(ctx, key, revision); err != nil {
			return fmt.Errorf("wait for read marker: %w", err)
		}
		c.logger.Debug("Set last read event", "user_id", userID, "room_id", roomID, "event_id", eventID)
		return nil
	}

	return fmt.Errorf("read marker update failed after %d retries", maxReadMarkerUpdateRetries)
}

// initializeLastReadEventID creates the member's initial room marker without
// replacing a marker already written by another replica or a concurrent
// user-facing read operation.
func (c *ChattoCore) initializeLastReadEventID(ctx context.Context, userID, roomID, eventID string) error {
	bucket := c.storage.runtimeStateKV
	key := roomReadEventKey(userID, roomID)
	revision, err := bucket.Create(ctx, key, []byte(eventID))
	if err != nil {
		if !jetstreamutil.IsSequenceConflict(err) {
			return fmt.Errorf("failed to initialize read marker: %w", err)
		}
		current, getErr := bucket.Get(ctx, key)
		if getErr != nil {
			return fmt.Errorf("re-read concurrently initialized read marker: %w", getErr)
		}
		revision = current.Revision()
	}
	if err := c.readStateModel.index.waitForRevision(ctx, key, revision); err != nil {
		return fmt.Errorf("wait for initialized read marker: %w", err)
	}
	return nil
}

// AdvanceLastReadEventID stores eventID as the user's room read marker only if
// it is newer than the marker already in RUNTIME_STATE. The compare-and-write
// loop uses KV revisions so concurrent replicas cannot move the marker
// backwards after seeing stale state.
func (c *ChattoCore) AdvanceLastReadEventID(ctx context.Context, kind RoomKind, userID, roomID, eventID string) (*LastReadEventIDAdvance, error) {
	nextTime, err := c.GetEventTimestamp(ctx, kind, roomID, eventID)
	if err != nil {
		return nil, err
	}

	bucket := c.storage.runtimeStateKV
	key := roomReadEventKey(userID, roomID)

	for attempt := 0; attempt < maxReadMarkerUpdateRetries; attempt++ {
		entry, exists, err := c.readStateModel.index.roomMarker(ctx, userID, roomID)
		if err != nil {
			return nil, fmt.Errorf("read room marker index: %w", err)
		}
		if !exists {
			if eventID != "" && nextTime.IsZero() {
				return &LastReadEventIDAdvance{}, nil
			}
			revision, err := bucket.Create(ctx, key, []byte(eventID))
			if err != nil {
				if jetstreamutil.IsSequenceConflict(err) {
					if waitErr := c.readStateModel.index.waitForRevisionAfter(ctx, key, entry.revision); waitErr != nil {
						return nil, fmt.Errorf("wait for conflicting read marker: %w", waitErr)
					}
					continue
				}
				return nil, fmt.Errorf("failed to create read marker: %w", err)
			}
			if err := c.readStateModel.index.waitForRevision(ctx, key, revision); err != nil {
				return nil, fmt.Errorf("wait for created read marker: %w", err)
			}
			c.logger.Debug("Advanced last read event", "user_id", userID, "room_id", roomID, "event_id", eventID)
			return &LastReadEventIDAdvance{
				CurrentEventID: eventID,
				CurrentTime:    nextTime,
				Updated:        true,
			}, nil
		}

		previousEventID := string(entry.value)
		previousTime, err := c.GetEventTimestamp(ctx, kind, roomID, previousEventID)
		if err != nil {
			return nil, err
		}
		result := &LastReadEventIDAdvance{
			PreviousEventID: previousEventID,
			PreviousTime:    previousTime,
			CurrentEventID:  previousEventID,
			CurrentTime:     previousTime,
		}

		if !shouldAdvanceReadMarker(previousEventID, previousTime, eventID, nextTime) {
			return result, nil
		}

		revision, err := bucket.Update(ctx, key, []byte(eventID), entry.revision)
		if err != nil {
			if jetstreamutil.IsSequenceConflict(err) {
				if waitErr := c.readStateModel.index.waitForRevisionAfter(ctx, key, entry.revision); waitErr != nil {
					return nil, fmt.Errorf("wait for conflicting read marker: %w", waitErr)
				}
				continue
			}
			return nil, fmt.Errorf("failed to advance read marker: %w", err)
		}
		if err := c.readStateModel.index.waitForRevision(ctx, key, revision); err != nil {
			return nil, fmt.Errorf("wait for advanced read marker: %w", err)
		}
		result.CurrentEventID = eventID
		result.CurrentTime = nextTime
		result.Updated = true
		c.logger.Debug("Advanced last read event", "user_id", userID, "room_id", roomID, "previous_event_id", previousEventID, "event_id", eventID)
		return result, nil
	}

	return nil, fmt.Errorf("read marker update failed after %d retries", maxReadMarkerUpdateRetries)
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
	if eventID == "" {
		return time.Time{}, nil
	}
	entry, ok := c.roomModel.timelineEntry(eventID)
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

// HasUnread reports whether a room has active Badge attention for a user.
// Thread Badge markers roll up into the parent room. The result is independent
// of the user's last-read cursor and false when the user cannot see the room.
func (c *ChattoCore) HasUnread(ctx context.Context, kind RoomKind, userID, roomID string) (bool, error) {
	isMember, err := c.RoomMembershipExists(ctx, kind, userID, roomID)
	if err != nil {
		return false, fmt.Errorf("failed to check room membership: %w", err)
	}
	if !isMember {
		return false, nil
	}
	badgeUnread, err := c.notificationOccurrences.HasNotificationUnread(ctx, userID, roomID, "")
	if err != nil {
		return false, fmt.Errorf("read notification Badge state: %w", err)
	}
	return badgeUnread, nil
}

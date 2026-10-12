package core

import (
	"context"
	"fmt"
	"time"

	pubsubv1 "hmans.de/chatto/internal/pb/chatto/core/pubsub/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
	"hmans.de/chatto/pkg/jetstreamutil"
)

// NotifyRoomMarkedAsRead publishes a live event to notify the user that they marked
// a room as read. This enables real-time updates to space unread indicators.
// This is best-effort - failures are logged but don't affect the mark-as-read operation.
func (s *ReadStateModel) NotifyRoomMarkedAsRead(ctx context.Context, userID string, kind RoomKind, roomID string) {
	event := newPubSubEvent(userID, &pubsubv1.PubSubEvent{
		Event: &pubsubv1.PubSubEvent_RoomReadStateChanged{
			RoomReadStateChanged: &realtimev1.RoomReadStateChangedEvent{
				RoomId: roomID,
			},
		},
	})

	if err := s.effects.publishUserPubSubEvent(ctx, userID, event); err != nil {
		s.logger.Warn("Failed to publish room marked as read event",
			"user_id", userID,
			"kind", kind,
			"room_id", roomID,
			"error", err)
	}
}

// GetRoomLastEvent returns the last root message's event ID and proto-level
// `created_at` timestamp for a room. Excludes thread replies — only root
// messages affect room-level unread tracking. exists is false if the room
// has no ordinary root messages. Historical imports do not advance this marker.
//
// Uses the proto's `created_at` rather than JetStream's stored time so the
// value stays correct after #354 phase 4d (which re-publishes messages
// with fresh JetStream timestamps but leaves the proto payloads intact).
func (s *ReadStateModel) GetRoomLastEvent(ctx context.Context, kind RoomKind, roomID string) (eventID string, ts time.Time, exists bool, err error) {
	entry := s.rooms.getRoomLastRootEntry(roomID)
	if entry == nil {
		return "", time.Time{}, false, nil
	}
	return entry.EventID, entry.CreatedAt, true, nil
}

// GetRoomLastReadableEvent returns the most recent room-visible message that
// userID can currently read. Channel echoes use the interaction relationship
// of their canonical thread root.
func (s *ReadStateModel) GetRoomLastReadableEvent(ctx context.Context, kind RoomKind, userID, roomID string) (eventID string, ts time.Time, exists bool, err error) {
	broad, err := s.policy.CanReadMessages(ctx, userID, kind, roomID)
	if err != nil {
		return "", time.Time{}, false, err
	}
	interactions := false
	if !broad {
		interactions, err = s.policy.CanReadMessageInteractions(ctx, userID, kind, roomID)
		if err != nil {
			return "", time.Time{}, false, err
		}
	}
	visible := func(entry *TimelineEntry) bool {
		if entry == nil || !entry.IsMessagePost() || entry.InThreadEventID != "" || entry.HistoricalImport {
			return false
		}
		if broad {
			return true
		}
		return interactions && s.rooms.hasThreadInteraction(userID, roomID, entry.ThreadRootEventID)
	}
	entry, ok := s.rooms.lastVisibleRoomEntry(roomID, visible)
	if !ok || entry == nil {
		return "", time.Time{}, false, nil
	}
	return entry.EventID, entry.CreatedAt, true, nil
}

// GetLastReadEventID returns the user's last-read root-message event ID for a
// room. If no marker exists yet, it lazy-initializes the marker to the room's
// current last root event ("caught up at deploy time"); if the room has no
// messages, it returns "".
func (s *ReadStateModel) GetLastReadEventID(ctx context.Context, kind RoomKind, userID, roomID string) (string, error) {
	bucket := s.index.kv

	key := roomReadEventKey(userID, roomID)
	entry, exists, err := s.index.roomMarker(ctx, userID, roomID)
	if err != nil {
		return "", fmt.Errorf("read room marker index: %w", err)
	}
	if exists {
		return string(entry.value), nil
	}

	// No marker yet — initialize to the room's current last root event so the
	// user starts caught up rather than seeing a wall of unreads.
	lastID, _, exists, err := s.GetRoomLastReadableEvent(ctx, kind, userID, roomID)
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
			if waitErr := s.index.waitForRevision(ctx, key, current.Revision()); waitErr != nil {
				return "", fmt.Errorf("wait for concurrent read marker: %w", waitErr)
			}
			return string(current.Value()), nil
		}
		s.logger.Warn("Failed to lazy-initialize read marker", "user_id", userID, "room_id", roomID, "error", err)
		return lastID, nil
	}
	if err := s.index.waitForRevision(ctx, key, revision); err != nil {
		return "", fmt.Errorf("wait for initialized read marker: %w", err)
	}
	return lastID, nil
}

// PeekLastReadEventID returns the stored read marker for a room without lazy
// initialization. exists is false when no marker has been written yet.
func (s *ReadStateModel) PeekLastReadEventID(ctx context.Context, userID, roomID string) (eventID string, exists bool, err error) {
	entry, exists, err := s.index.roomMarker(ctx, userID, roomID)
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
func (s *ReadStateModel) SetLastReadEventID(ctx context.Context, kind RoomKind, userID, roomID, eventID string) error {
	bucket := s.index.kv
	key := roomReadEventKey(userID, roomID)

	for range maxReadMarkerUpdateRetries {
		entry, exists, err := s.index.roomMarker(ctx, userID, roomID)
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
				if waitErr := s.index.waitForRevisionAfter(ctx, key, entry.revision); waitErr != nil {
					return fmt.Errorf("wait for conflicting read marker: %w", waitErr)
				}
				continue
			}
			return fmt.Errorf("failed to set read marker: %w", err)
		}
		if err := s.index.waitForRevision(ctx, key, revision); err != nil {
			return fmt.Errorf("wait for read marker: %w", err)
		}
		s.logger.Debug("Set last read event", "user_id", userID, "room_id", roomID, "event_id", eventID)
		return nil
	}

	return fmt.Errorf("read marker update failed after %d retries", maxReadMarkerUpdateRetries)
}

// AdvanceLastReadEventID stores eventID as the user's room read marker only if
// it is newer than the marker already in RUNTIME_STATE. The compare-and-write
// loop uses KV revisions so concurrent replicas cannot move the marker
// backwards after seeing stale state.
func (s *ReadStateModel) AdvanceLastReadEventID(ctx context.Context, kind RoomKind, userID, roomID, eventID string) (*LastReadEventIDAdvance, error) {
	nextTime, err := s.GetEventTimestamp(ctx, kind, roomID, eventID)
	if err != nil {
		return nil, err
	}

	bucket := s.index.kv
	key := roomReadEventKey(userID, roomID)

	for range maxReadMarkerUpdateRetries {
		entry, exists, err := s.index.roomMarker(ctx, userID, roomID)
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
					if waitErr := s.index.waitForRevisionAfter(ctx, key, entry.revision); waitErr != nil {
						return nil, fmt.Errorf("wait for conflicting read marker: %w", waitErr)
					}
					continue
				}
				return nil, fmt.Errorf("failed to create read marker: %w", err)
			}
			if err := s.index.waitForRevision(ctx, key, revision); err != nil {
				return nil, fmt.Errorf("wait for created read marker: %w", err)
			}
			s.logger.Debug("Advanced last read event", "user_id", userID, "room_id", roomID, "event_id", eventID)
			return &LastReadEventIDAdvance{
				CurrentEventID: eventID,
				CurrentTime:    nextTime,
				Updated:        true,
			}, nil
		}

		previousEventID := string(entry.value)
		previousTime, err := s.GetEventTimestamp(ctx, kind, roomID, previousEventID)
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
				if waitErr := s.index.waitForRevisionAfter(ctx, key, entry.revision); waitErr != nil {
					return nil, fmt.Errorf("wait for conflicting read marker: %w", waitErr)
				}
				continue
			}
			return nil, fmt.Errorf("failed to advance read marker: %w", err)
		}
		if err := s.index.waitForRevision(ctx, key, revision); err != nil {
			return nil, fmt.Errorf("wait for advanced read marker: %w", err)
		}
		result.CurrentEventID = eventID
		result.CurrentTime = nextTime
		result.Updated = true
		s.logger.Debug("Advanced last read event", "user_id", userID, "room_id", roomID, "previous_event_id", previousEventID, "event_id", eventID)
		return result, nil
	}

	return nil, fmt.Errorf("read marker update failed after %d retries", maxReadMarkerUpdateRetries)
}

// GetEventTimestamp returns the proto-level `created_at` timestamp for a
// root message event ID in a room. Returns zero time if the event ID is
// empty or the message doesn't exist (e.g. deleted or never published).
//
// Reads from the proto payload rather than JetStream's stored time so the
// value stays correct after #354 phase 4d (which re-publishes with fresh
// JetStream timestamps but preserves the proto payload).
func (s *ReadStateModel) GetEventTimestamp(ctx context.Context, kind RoomKind, roomID, eventID string) (time.Time, error) {
	if eventID == "" {
		return time.Time{}, nil
	}
	entry, ok := s.rooms.timelineEntry(eventID)
	if !ok {
		return time.Time{}, nil
	}
	// Honour roomID scope — same as GetRoomEventByEventID.
	if entry.RoomID != roomID {
		return time.Time{}, nil
	}
	return entry.CreatedAt, nil
}

// HasUnread reports whether a room has active Badge attention for a user.
// Thread Badge attention rolls up into the parent room. The result is independent
// of the user's last-read cursor and false when the user cannot see the room.
func (s *ReadStateModel) HasUnread(ctx context.Context, kind RoomKind, userID, roomID string) (bool, error) {
	isMember, err := s.memberships.RoomMembershipExists(ctx, kind, userID, roomID)
	if err != nil {
		return false, fmt.Errorf("failed to check room membership: %w", err)
	}
	if !isMember {
		return false, nil
	}
	badgeUnread, err := s.notifications.HasNotificationUnread(ctx, userID, roomID, "")
	if err != nil {
		return false, fmt.Errorf("read notification Badge state: %w", err)
	}
	return badgeUnread, nil
}

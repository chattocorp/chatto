package core

import (
	"context"
	"encoding/binary"
	"fmt"
	"slices"
	"time"

	pubsubv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/pubsub/v1"
	realtimev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/realtime/v1"
	"github.com/chattocorp/chatto/pkg/jetstreamutil"
)

// GetThreadLastOpened retrieves the timestamp of the latest thread message the
// user has seen. Returns zero time if the thread has never been opened.
//
// New RUNTIME_STATE markers store the seen message event ID. Values migrated
// from SERVER_RUNTIME may still be the legacy 8-byte UnixNano timestamp; those
// are decoded here so existing read state survives the rollout.
func (s *ReadStateModel) GetThreadLastOpened(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string) (time.Time, error) {
	entry, exists, err := s.index.threadMarker(ctx, userID, roomID, threadRootEventID)
	if err != nil {
		return time.Time{}, fmt.Errorf("read thread marker index: %w", err)
	}
	if !exists {
		return time.Time{}, nil
	}

	return s.threadReadMarkerTime(ctx, kind, roomID, entry.value)
}

// SetThreadLastReadEventID stores eventID as the latest thread message the user
// has seen, but only if it is newer than the existing marker (advance-only).
// Returns the previous marker time (zero if never opened before).
func (s *ReadStateModel) SetThreadLastReadEventID(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID, eventID string) (time.Time, error) {
	result, err := s.advanceThreadLastReadEventID(ctx, kind, userID, roomID, threadRootEventID, eventID)
	if err != nil {
		return time.Time{}, err
	}
	return result.PreviousLastReadAt, nil
}

// advanceThreadLastReadEventID returns the timestamps from the successful CAS
// decision, including the retained marker when the request does not advance it.
func (s *ReadStateModel) advanceThreadLastReadEventID(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID, eventID string) (*MarkThreadAsReadResult, error) {
	bucket := s.index.kv
	key := threadLastOpenedKey(userID, roomID, threadRootEventID)

	nextTime, err := s.GetEventTimestamp(ctx, kind, roomID, eventID)
	if err != nil {
		return nil, err
	}

	for range maxReadMarkerUpdateRetries {
		var previousTime time.Time
		entry, exists, err := s.index.threadMarker(ctx, userID, roomID, threadRootEventID)
		if err != nil {
			return nil, fmt.Errorf("read thread marker index: %w", err)
		}
		if !exists {
			if nextTime.IsZero() {
				return &MarkThreadAsReadResult{}, nil
			}
			revision, err := bucket.Create(ctx, key, []byte(eventID))
			if err != nil {
				if jetstreamutil.IsSequenceConflict(err) {
					if waitErr := s.index.waitForRevisionAfter(ctx, key, entry.revision); waitErr != nil {
						return nil, fmt.Errorf("wait for conflicting thread marker: %w", waitErr)
					}
					continue
				}
				return nil, fmt.Errorf("failed to create thread last opened: %w", err)
			}
			if err := s.index.waitForRevision(ctx, key, revision); err != nil {
				return nil, fmt.Errorf("wait for created thread marker: %w", err)
			}
			s.logger.Debug("Set thread last read event", "user_id", userID, "room_id", roomID, "thread_root_event_id", threadRootEventID, "previous", previousTime, "event_id", eventID)
			s.publishThreadViewerStateChangedEvent(ctx, userID, kind, roomID, threadRootEventID, s.rooms.threadFollowState(userID, roomID, threadRootEventID) == ThreadFollowStateFollowing)
			return &MarkThreadAsReadResult{PreviousLastReadAt: previousTime, LastReadAt: nextTime}, nil
		}

		previousTime, err = s.threadReadMarkerTime(ctx, kind, roomID, entry.value)
		if err != nil {
			return nil, err
		}
		if nextTime.IsZero() || !nextTime.After(previousTime) {
			return &MarkThreadAsReadResult{PreviousLastReadAt: previousTime, LastReadAt: previousTime}, nil
		}

		revision, err := bucket.Update(ctx, key, []byte(eventID), entry.revision)
		if err != nil {
			if jetstreamutil.IsSequenceConflict(err) {
				if waitErr := s.index.waitForRevisionAfter(ctx, key, entry.revision); waitErr != nil {
					return nil, fmt.Errorf("wait for conflicting thread marker: %w", waitErr)
				}
				continue
			}
			return nil, fmt.Errorf("failed to set thread last opened: %w", err)
		}
		if err := s.index.waitForRevision(ctx, key, revision); err != nil {
			return nil, fmt.Errorf("wait for updated thread marker: %w", err)
		}

		s.logger.Debug("Set thread last read event", "user_id", userID, "room_id", roomID, "thread_root_event_id", threadRootEventID, "previous", previousTime, "event_id", eventID)
		s.publishThreadViewerStateChangedEvent(ctx, userID, kind, roomID, threadRootEventID, s.rooms.threadFollowState(userID, roomID, threadRootEventID) == ThreadFollowStateFollowing)
		return &MarkThreadAsReadResult{PreviousLastReadAt: previousTime, LastReadAt: nextTime}, nil
	}

	return nil, fmt.Errorf("thread read marker update failed after %d retries", maxReadMarkerUpdateRetries)
}

// SetThreadLastOpenedAt is retained for timestamp-based callers/tests. It
// stores a legacy timestamp marker in RUNTIME_STATE and should not be used for
// new code when a concrete event ID is available.
func (s *ReadStateModel) SetThreadLastOpenedAt(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string, ts time.Time) (time.Time, error) {
	bucket := s.index.kv
	key := threadLastOpenedKey(userID, roomID, threadRootEventID)

	for range maxReadMarkerUpdateRetries {
		var previousTime time.Time
		entry, exists, err := s.index.threadMarker(ctx, userID, roomID, threadRootEventID)
		if err != nil {
			return time.Time{}, fmt.Errorf("read thread marker index: %w", err)
		}
		if !exists {
			if ts.IsZero() {
				return time.Time{}, nil
			}
			buf := make([]byte, 8)
			binary.BigEndian.PutUint64(buf, uint64(ts.UnixNano()))
			revision, err := bucket.Create(ctx, key, buf)
			if err != nil {
				if jetstreamutil.IsSequenceConflict(err) {
					if waitErr := s.index.waitForRevisionAfter(ctx, key, entry.revision); waitErr != nil {
						return time.Time{}, fmt.Errorf("wait for conflicting thread marker: %w", waitErr)
					}
					continue
				}
				return time.Time{}, fmt.Errorf("failed to create thread last opened: %w", err)
			}
			if err := s.index.waitForRevision(ctx, key, revision); err != nil {
				return time.Time{}, fmt.Errorf("wait for created thread marker: %w", err)
			}
			s.logger.Debug("Set legacy thread last opened timestamp", "user_id", userID, "room_id", roomID, "thread_root_event_id", threadRootEventID, "previous", previousTime, "at", ts)
			s.publishThreadViewerStateChangedEvent(ctx, userID, kind, roomID, threadRootEventID, s.rooms.threadFollowState(userID, roomID, threadRootEventID) == ThreadFollowStateFollowing)
			return previousTime, nil
		}

		previousTime, err = s.threadReadMarkerTime(ctx, kind, roomID, entry.value)
		if err != nil {
			return time.Time{}, err
		}
		if !ts.After(previousTime) {
			return previousTime, nil
		}

		buf := make([]byte, 8)
		binary.BigEndian.PutUint64(buf, uint64(ts.UnixNano()))
		revision, err := bucket.Update(ctx, key, buf, entry.revision)
		if err != nil {
			if jetstreamutil.IsSequenceConflict(err) {
				if waitErr := s.index.waitForRevisionAfter(ctx, key, entry.revision); waitErr != nil {
					return time.Time{}, fmt.Errorf("wait for conflicting thread marker: %w", waitErr)
				}
				continue
			}
			return time.Time{}, fmt.Errorf("failed to set thread last opened: %w", err)
		}
		if err := s.index.waitForRevision(ctx, key, revision); err != nil {
			return time.Time{}, fmt.Errorf("wait for updated thread marker: %w", err)
		}
		s.logger.Debug("Set legacy thread last opened timestamp", "user_id", userID, "room_id", roomID, "thread_root_event_id", threadRootEventID, "previous", previousTime, "at", ts)
		s.publishThreadViewerStateChangedEvent(ctx, userID, kind, roomID, threadRootEventID, s.rooms.threadFollowState(userID, roomID, threadRootEventID) == ThreadFollowStateFollowing)
		return previousTime, nil
	}

	return time.Time{}, fmt.Errorf("thread read marker update failed after %d retries", maxReadMarkerUpdateRetries)
}

// SetThreadLastOpened records the latest current reply in the thread as read.
// Returns the previous marker time (zero if never opened before).
func (s *ReadStateModel) SetThreadLastOpened(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string) (time.Time, error) {
	latestID := s.latestThreadMessageEventID(threadRootEventID)
	if latestID == "" {
		return time.Time{}, nil
	}
	return s.SetThreadLastReadEventID(ctx, kind, userID, roomID, threadRootEventID, latestID)
}

func (s *ReadStateModel) threadReadMarkerTime(ctx context.Context, kind RoomKind, roomID string, value []byte) (time.Time, error) {
	if len(value) == 8 {
		nanos := int64(binary.BigEndian.Uint64(value))
		return time.Unix(0, nanos), nil
	}
	eventID := string(value)
	if eventID == "" {
		return time.Time{}, nil
	}
	return s.GetEventTimestamp(ctx, kind, roomID, eventID)
}

func (s *ReadStateModel) latestThreadMessageEventID(threadRootEventID string) string {
	entries := s.rooms.threadEvents(threadRootEventID)
	for _, entry := range slices.Backward(entries) {
		if entry == nil || !entry.IsMessagePost() {
			continue
		}
		if id := entry.EventID; id != "" {
			return id
		}
	}
	return threadRootEventID
}

// publishThreadViewerStateChangedEvent publishes a user-scoped thread state
// invalidation. It fires for follow changes and read-marker advances; projection
// transports hydrate the complete current root row from its identifiers.
func (s *ReadStateModel) publishThreadViewerStateChangedEvent(ctx context.Context, userID string, kind RoomKind, roomID, threadRootEventID string, isFollowing bool) {
	event := newPubSubEvent(userID, &pubsubv1.PubSubEvent{
		Event: &pubsubv1.PubSubEvent_ThreadViewerStateChanged{
			ThreadViewerStateChanged: &realtimev1.ThreadViewerStateChangedEvent{
				RoomId:            roomID,
				ThreadRootEventId: threadRootEventID,
				IsFollowing:       isFollowing,
			},
		},
	})

	if err := s.integration.publishUserPubSubEvent(ctx, userID, event); err != nil {
		s.logger.Warn("Failed to publish thread follow changed event", "error", err, "user_id", userID, "thread_root_event_id", threadRootEventID)
	}
}

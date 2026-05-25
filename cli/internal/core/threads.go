package core

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ThreadMetadata contains reply count, last reply timestamp, and participants for a thread.
type ThreadMetadata struct {
	ReplyCount     int
	LastReplyAt    *time.Time
	ParticipantIDs []string
}

// FollowedThread represents a thread the user is following, enriched with metadata for display.
type FollowedThread struct {
	SpaceID           string
	RoomID            string
	ThreadRootEventID string
	ReplyCount        int
	LastReplyAt       *time.Time
	ParticipantIDs    []string
	HasUnread         bool
}

// maxThreadParticipants is the maximum number of participant IDs tracked per thread.
const maxThreadParticipants = 50

// GetThreadEvents returns the root message followed by every reply
// (in stream-arrival order) for the given thread root.
//
// Source: RoomTimelineProjection for the root, ThreadProjection for
// the replies. The ThreadProjection holds replies plus edit/retract
// events targeting them — currently we surface only MessagePostedEvent
// replies here so legacy callers see the same shape as the
// SERVER_EVENTS-backed implementation. Edits / retracts are folded
// onto the original via LatestBody at body-resolve time.
//
// Authorization: caller must verify room membership before calling.
func (c *ChattoCore) GetThreadEvents(ctx context.Context, kind RoomKind, room_id string, threadRootEventId string) ([]*corev1.Event, error) {
	rootEntry, ok := c.RoomTimeline.Get(threadRootEventId)
	if !ok {
		return nil, fmt.Errorf("thread root message not found: event ID %s", threadRootEventId)
	}
	if rootEntry.Event.GetMessagePosted() == nil {
		return nil, fmt.Errorf("event ID %s is not a message event", threadRootEventId)
	}

	replies := c.Threads.ThreadEvents(threadRootEventId)
	events := make([]*corev1.Event, 0, 1+len(replies))
	events = append(events, rootEntry.Event)
	for _, r := range replies {
		// Skip edit/retract entries — the body resolver folds them via
		// LatestBody. The thread pane only wants the post events.
		if r.Event.GetMessagePosted() == nil {
			continue
		}
		events = append(events, r.Event)
	}
	return events, nil
}

// threadMetadataKey returns the KV key for thread metadata: {roomId}.{rootEventId}
func threadMetadataKey(roomID string, rootEventId string) string {
	return fmt.Sprintf("%s.%s", roomID, rootEventId)
}

// updateThreadMetadata updates the thread metadata in KV with optimistic locking.
// Called when a reply is posted to a thread. Tracks reply count, last reply time, and participants.
// The rootAuthorID is the author of the thread root message - they're included as the first participant.
func (c *ChattoCore) updateThreadMetadata(ctx context.Context, kind RoomKind, roomID string, rootEventId string, rootAuthorID, replyAuthorID string, replyTime time.Time) error {
	const maxRetries = 5

	bucket := c.storage.serverThreadsKV

	key := threadMetadataKey(roomID, rootEventId)

	for attempt := 0; attempt < maxRetries; attempt++ {
		// Get current entry (if any) with its revision
		var revision uint64
		var metadata *corev1.ThreadMetadata

		entry, err := bucket.Get(ctx, key)
		if err == nil {
			// Key exists - unmarshal and get revision
			revision = entry.Revision()
			metadata = &corev1.ThreadMetadata{}
			if unmarshalErr := proto.Unmarshal(entry.Value(), metadata); unmarshalErr != nil {
				c.logger.Warn("Failed to unmarshal thread metadata, creating new", "error", unmarshalErr)
				metadata = nil
			}
		} else if !errors.Is(err, jetstream.ErrKeyNotFound) {
			return fmt.Errorf("failed to get thread metadata: %w", err)
		}

		// Create or update metadata
		if metadata == nil {
			// First reply to this thread - initialize participant list with root author first
			participants := []string{}
			if rootAuthorID != "" {
				participants = append(participants, rootAuthorID)
			}
			// Add reply author if different from root author
			if replyAuthorID != rootAuthorID {
				participants = append(participants, replyAuthorID)
			}
			metadata = &corev1.ThreadMetadata{
				RootEventId:    rootEventId,
				ReplyCount:     1,
				LastReplyAt:    timestamppb.New(replyTime),
				ParticipantIds: participants,
			}
		} else {
			metadata.ReplyCount++
			metadata.LastReplyAt = timestamppb.New(replyTime)

			// Ensure root author is in participants (for backward compatibility with existing threads)
			if rootAuthorID != "" {
				rootAuthorExists := false
				for _, pid := range metadata.ParticipantIds {
					if pid == rootAuthorID {
						rootAuthorExists = true
						break
					}
				}
				if !rootAuthorExists && len(metadata.ParticipantIds) < maxThreadParticipants {
					// Insert root author at the beginning
					metadata.ParticipantIds = append([]string{rootAuthorID}, metadata.ParticipantIds...)
				}
			}

			// Add reply author if not already present and under cap
			replyAuthorExists := false
			for _, pid := range metadata.ParticipantIds {
				if pid == replyAuthorID {
					replyAuthorExists = true
					break
				}
			}
			if !replyAuthorExists && len(metadata.ParticipantIds) < maxThreadParticipants {
				metadata.ParticipantIds = append(metadata.ParticipantIds, replyAuthorID)
			}
		}

		// Marshal and store
		data, err := proto.Marshal(metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal thread metadata: %w", err)
		}

		// Try atomic update
		var updateErr error
		if revision == 0 {
			// No existing key - use Create for atomic insert
			_, updateErr = bucket.Create(ctx, key, data)
		} else {
			// Existing key - use Update with revision check
			_, updateErr = bucket.Update(ctx, key, data, revision)
		}

		if updateErr == nil {
			c.logger.Debug("Updated thread metadata",
				"kind", kind,
				"room_id", roomID,
				"root_event_id", rootEventId,
				"reply_count", metadata.ReplyCount,
				"participants", len(metadata.ParticipantIds))
			return nil
		}

		// Check if it's a revision conflict (concurrent update)
		if errors.Is(updateErr, jetstream.ErrKeyExists) {
			c.logger.Debug("Thread metadata revision conflict, retrying",
				"room_id", roomID,
				"root_event_id", rootEventId,
				"attempt", attempt+1)
			continue
		}

		// Some other error
		return fmt.Errorf("failed to store thread metadata: %w", updateErr)
	}

	return fmt.Errorf("failed to update thread metadata after %d retries due to concurrent modifications", maxRetries)
}

// notifyThreadFollowers creates persistent notifications for all thread followers when someone replies.
// Followers are users who have explicitly or automatically followed the thread (stored in RUNTIME KV).
// Users in skipIDs are excluded (e.g., already notified via inReplyTo).
// This is best-effort - failures are logged but don't affect message posting.
func (c *ChattoCore) notifyThreadFollowers(ctx context.Context, kind RoomKind, roomID, replyAuthorID, replyEventID, threadRootID string, skipIDs []string) {
	// Get all users following this thread
	followerIDs, err := c.GetThreadFollowers(ctx, kind, roomID, threadRootID)
	if err != nil {
		c.logger.Warn("Failed to get thread followers for notification",
			"thread_root_id", threadRootID,
			"error", err)
		return
	}

	// Build skip set for O(1) lookups
	skipSet := make(map[string]bool, len(skipIDs))
	for _, id := range skipIDs {
		skipSet[id] = true
	}

	// Notify each follower except the reply author and skipped users
	notifiedCount := 0
	for _, followerID := range followerIDs {
		// Don't notify the person who posted the reply
		if followerID == replyAuthorID {
			continue
		}

		// Skip users already notified via other means (e.g., inReplyTo)
		if skipSet[followerID] {
			continue
		}

		// Skip if user has muted this room
		level, err := c.GetEffectiveNotificationLevel(ctx, followerID, roomID)
		if err != nil {
			c.logger.Warn("Failed to get notification level for thread follower, continuing",
				"user_id", followerID, "error", err)
		} else if level == corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
			continue
		}

		// Create persistent notification (for bell icon and notification center)
		// This also publishes NotificationCreatedEvent for real-time updates
		_, err = c.CreateNotification(ctx, followerID, replyAuthorID, &corev1.Notification{
			Notification: &corev1.Notification_Reply{
				Reply: &corev1.ReplyNotification{
					RoomId:      roomID,
					EventId:     replyEventID,
					InReplyToId: threadRootID,
					InThread:    threadRootID,
				},
			},
		})
		if err != nil {
			c.logger.Warn("Failed to create reply notification",
				"recipient_id", followerID,
				"reply_author_id", replyAuthorID,
				"kind", kind,
				"room_id", roomID,
				"error", err)
		} else {
			notifiedCount++
		}
	}

	if notifiedCount > 0 {
		c.logger.Debug("Created reply notifications for thread followers",
			"thread_root_id", threadRootID,
			"reply_author_id", replyAuthorID,
			"notified_count", notifiedCount,
			"kind", kind,
			"room_id", roomID)
	}
}

// notifyInReplyToAuthor creates a persistent notification for the author of a message
// that received a reply (via inReplyTo). Works for both room-level and in-thread replies.
// Returns the notified user ID so the caller can add it to the already-notified set,
// or empty string if no notification was sent.
// This is best-effort - failures are logged but don't affect message posting.
func (c *ChattoCore) notifyInReplyToAuthor(ctx context.Context, kind RoomKind, roomID, replyAuthorID, replyEventID, inReplyToEventID, inThread string, alreadyNotifiedIDs []string) string {
	// Look up the original message to find its author
	originalEvent, err := c.GetRoomEventByEventID(ctx, kind, roomID, inReplyToEventID)
	if err != nil || originalEvent == nil {
		c.logger.Warn("Failed to get in-reply-to message for notification",
			"in_reply_to_id", inReplyToEventID,
			"error", err)
		return ""
	}

	originalAuthorID := originalEvent.ActorId
	if originalAuthorID == "" {
		return ""
	}

	// Don't notify yourself
	if originalAuthorID == replyAuthorID {
		return ""
	}

	// Don't notify if the user was already notified (e.g., via @mention)
	for _, notifiedID := range alreadyNotifiedIDs {
		if notifiedID == originalAuthorID {
			return ""
		}
	}

	// Skip if user has muted this room
	level, err := c.GetEffectiveNotificationLevel(ctx, originalAuthorID, roomID)
	if err != nil {
		c.logger.Warn("Failed to get notification level for in-reply-to author, continuing",
			"user_id", originalAuthorID, "error", err)
	} else if level == corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED {
		return ""
	}

	// Create persistent notification (for bell icon and notification center)
	_, err = c.CreateNotification(ctx, originalAuthorID, replyAuthorID, &corev1.Notification{
		Notification: &corev1.Notification_Reply{
			Reply: &corev1.ReplyNotification{
				RoomId:      roomID,
				EventId:     replyEventID,
				InReplyToId: inReplyToEventID,
				InThread:    inThread,
			},
		},
	})
	if err != nil {
		c.logger.Warn("Failed to create in-reply-to notification",
			"recipient_id", originalAuthorID,
			"reply_author_id", replyAuthorID,
			"kind", kind,
			"room_id", roomID,
			"error", err)
		return ""
	}

	c.logger.Debug("Created in-reply-to notification",
		"recipient_id", originalAuthorID,
		"reply_author_id", replyAuthorID,
		"kind", kind,
		"room_id", roomID)

	return originalAuthorID
}

// GetThreadMetadata returns reply count, last reply timestamp, and
// participants for a thread root message. Returns zero values if the
// thread has no replies. Derived live from the ThreadProjection.
func (c *ChattoCore) GetThreadMetadata(ctx context.Context, kind RoomKind, roomID string, rootEventId string) (*ThreadMetadata, error) {
	replies := c.Threads.ThreadEvents(rootEventId)

	metadata := &ThreadMetadata{}
	participants := make(map[string]struct{})
	var latestReplyAt *time.Time
	for _, r := range replies {
		// Only MessagePostedEvent entries count as replies — edit /
		// retract entries land in the thread's bucket but mustn't
		// inflate the metadata.
		if r.Event.GetMessagePosted() == nil {
			continue
		}
		metadata.ReplyCount++
		if actor := r.Event.GetActorId(); actor != "" {
			if len(participants) < maxThreadParticipants {
				participants[actor] = struct{}{}
			}
		}
		if t := r.Event.GetCreatedAt(); t != nil {
			ts := t.AsTime()
			if latestReplyAt == nil || ts.After(*latestReplyAt) {
				latestReplyAt = &ts
			}
		}
	}
	if len(participants) > 0 {
		metadata.ParticipantIDs = make([]string, 0, len(participants))
		for id := range participants {
			metadata.ParticipantIDs = append(metadata.ParticipantIDs, id)
		}
	}
	metadata.LastReplyAt = latestReplyAt
	return metadata, nil
}

// threadLastOpenedKey returns the KV key for tracking when a user last opened a thread.
func threadLastOpenedKey(userID, roomID, threadRootEventID string) string {
	return fmt.Sprintf("thread_last_opened.%s.%s.%s", userID, roomID, threadRootEventID)
}

// GetThreadLastOpened retrieves the timestamp when a user last opened a thread.
// Returns zero time if the thread has never been opened.
func (c *ChattoCore) GetThreadLastOpened(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string) (time.Time, error) {
	bucket := c.storage.serverRuntimeKV

	entry, err := bucket.Get(ctx, threadLastOpenedKey(userID, roomID, threadRootEventID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return time.Time{}, nil // Never opened
		}
		return time.Time{}, fmt.Errorf("failed to get thread last opened: %w", err)
	}

	// Decode int64 (Unix nano) from bytes using binary.BigEndian
	if len(entry.Value()) != 8 {
		return time.Time{}, fmt.Errorf("invalid thread last opened value")
	}
	nanos := int64(binary.BigEndian.Uint64(entry.Value()))
	return time.Unix(0, nanos), nil
}

// SetThreadLastOpenedAt stores ts as the user's last-opened time for a
// thread, but only if ts is newer than the existing marker (advance-only).
// Returns the previous last-opened time (zero if never opened before).
func (c *ChattoCore) SetThreadLastOpenedAt(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string, ts time.Time) (time.Time, error) {
	bucket := c.storage.serverRuntimeKV
	key := threadLastOpenedKey(userID, roomID, threadRootEventID)

	var previousTime time.Time
	entry, err := bucket.Get(ctx, key)
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return time.Time{}, fmt.Errorf("failed to get previous thread last opened: %w", err)
	}
	if err == nil && len(entry.Value()) == 8 {
		nanos := int64(binary.BigEndian.Uint64(entry.Value()))
		previousTime = time.Unix(0, nanos)
	}

	if !ts.After(previousTime) {
		return previousTime, nil
	}

	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(ts.UnixNano()))

	if _, err = bucket.Put(ctx, key, buf); err != nil {
		return time.Time{}, fmt.Errorf("failed to set thread last opened: %w", err)
	}

	c.logger.Debug("Set thread last opened", "user_id", userID, "room_id", roomID, "thread_root_event_id", threadRootEventID, "previous", previousTime, "at", ts)
	return previousTime, nil
}

// SetThreadLastOpened records the current wall-clock time as the user's
// last-opened time for a thread. Returns the previous last-opened time
// (zero if never opened before).
func (c *ChattoCore) SetThreadLastOpened(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string) (time.Time, error) {
	return c.SetThreadLastOpenedAt(ctx, kind, userID, roomID, threadRootEventID, time.Now())
}

// threadFollowKey returns the KV key for tracking whether a user is following a thread.
func threadFollowKey(userID, roomID, threadRootEventID string) string {
	return fmt.Sprintf("thread_follow.%s.%s.%s", userID, roomID, threadRootEventID)
}

// FollowThread marks a user as following a thread so they receive reply notifications.
// Stores a single byte in the RUNTIME KV bucket. Idempotent.
// Publishes a ThreadFollowChangedEvent for multi-tab sync.
func (c *ChattoCore) FollowThread(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string) error {
	bucket := c.storage.serverRuntimeKV

	if _, err := bucket.Put(ctx, threadFollowKey(userID, roomID, threadRootEventID), []byte{0x01}); err != nil {
		return fmt.Errorf("failed to follow thread: %w", err)
	}

	c.publishThreadFollowChangedEvent(ctx, userID, kind, roomID, threadRootEventID, true)
	return nil
}

// UnfollowThread removes a user's follow on a thread so they stop receiving reply notifications.
// Idempotent - calling when not following is a no-op.
// Publishes a ThreadFollowChangedEvent for multi-tab sync.
func (c *ChattoCore) UnfollowThread(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string) error {
	bucket := c.storage.serverRuntimeKV

	if err := bucket.Delete(ctx, threadFollowKey(userID, roomID, threadRootEventID)); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to unfollow thread: %w", err)
	}

	c.publishThreadFollowChangedEvent(ctx, userID, kind, roomID, threadRootEventID, false)
	return nil
}

// publishThreadFollowChangedEvent publishes a live event when a user's thread follow state changes.
// User-scoped: only delivered to the user who changed their follow state.
func (c *ChattoCore) publishThreadFollowChangedEvent(ctx context.Context, userID string, kind RoomKind, roomID, threadRootEventID string, isFollowing bool) {
	event := &corev1.Event{
		Id:        NewEventID(),
		ActorId:   userID,
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_ThreadFollowChanged{
			ThreadFollowChanged: &corev1.ThreadFollowChangedEvent{
				RoomId:            roomID,
				ThreadRootEventId: threadRootEventID,
				IsFollowing:       isFollowing,
			},
		},
	}

	subject := subjects.LiveUserEvent(userID, "thread_follow_changed")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Warn("Failed to publish thread follow changed event", "error", err, "user_id", userID, "thread_root_event_id", threadRootEventID)
	}
}

// IsFollowingThread checks if a user is following a thread.
func (c *ChattoCore) IsFollowingThread(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string) (bool, error) {
	bucket := c.storage.serverRuntimeKV

	if _, err := bucket.Get(ctx, threadFollowKey(userID, roomID, threadRootEventID)); err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return false, nil
		}
		return false, fmt.Errorf("failed to check thread follow: %w", err)
	}
	return true, nil
}

// GetThreadFollowers returns all user IDs following a specific thread.
// Uses ListKeysFiltered to scan for thread_follow.*.{roomID}.{threadRootEventID} keys.
func (c *ChattoCore) GetThreadFollowers(ctx context.Context, kind RoomKind, roomID, threadRootEventID string) ([]string, error) {
	bucket := c.storage.serverRuntimeKV

	pattern := fmt.Sprintf("thread_follow.*.%s.%s", roomID, threadRootEventID)
	lister, err := bucket.ListKeysFiltered(ctx, pattern)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list thread followers: %w", err)
	}

	var userIDs []string
	for key := range lister.Keys() {
		// Key format: thread_follow.{userID}.{roomID}.{threadRootEventID}
		parts := strings.Split(key, ".")
		if len(parts) >= 4 {
			userIDs = append(userIDs, parts[1])
		}
	}
	return userIDs, nil
}

// ListFollowedThreads returns all threads followed by the user in the given spaces,
// sorted by last activity (newest first). Threads with no metadata are skipped.
// Authorization: Caller must verify space membership before calling.
func (c *ChattoCore) ListFollowedThreads(ctx context.Context, userID string, spaceIDs []string) ([]*FollowedThread, error) {
	var allThreads []*FollowedThread

	for _, spaceID := range spaceIDs {
		threads, err := c.listFollowedThreadsInSpace(ctx, userID, KindForSpace(spaceID))
		if err != nil {
			c.logger.Warn("Failed to list followed threads for space", "space_id", spaceID, "error", err)
			continue
		}
		allThreads = append(allThreads, threads...)
	}

	// Sort by LastReplyAt descending (newest first), nil values last
	sort.Slice(allThreads, func(i, j int) bool {
		if allThreads[i].LastReplyAt == nil {
			return false
		}
		if allThreads[j].LastReplyAt == nil {
			return true
		}
		return allThreads[i].LastReplyAt.After(*allThreads[j].LastReplyAt)
	})

	return allThreads, nil
}

// listFollowedThreadsInSpace returns all threads followed by the user in a single space.
func (c *ChattoCore) listFollowedThreadsInSpace(ctx context.Context, userID string, kind RoomKind) ([]*FollowedThread, error) {
	bucket := c.storage.serverRuntimeKV

	// List all thread_follow keys for this user across all rooms
	// Use ">" to match remaining parts: thread_follow.{userId}.{roomId}.{threadRootEventId}
	pattern := fmt.Sprintf("thread_follow.%s.>", userID)
	lister, err := bucket.ListKeysFiltered(ctx, pattern)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to list followed threads: %w", err)
	}

	var result []*FollowedThread
	for key := range lister.Keys() {
		// Key format: thread_follow.{userID}.{roomID}.{threadRootEventID}
		parts := strings.Split(key, ".")
		if len(parts) < 4 {
			continue
		}
		roomID := parts[2]
		threadRootEventID := parts[3]

		// Get thread metadata (reply count, last reply, participants)
		metadata, err := c.GetThreadMetadata(ctx, kind, roomID, threadRootEventID)
		if err != nil {
			c.logger.Warn("Failed to get thread metadata for followed thread", "error", err, "room_id", roomID, "thread_root_event_id", threadRootEventID)
			continue
		}

		// Determine unread status: thread has unread if lastReplyAt > threadLastOpened
		hasUnread := false
		if metadata.LastReplyAt != nil {
			lastOpened, err := c.GetThreadLastOpened(ctx, kind, userID, roomID, threadRootEventID)
			if err != nil {
				hasUnread = true // Can't determine, assume unread
			} else {
				hasUnread = lastOpened.IsZero() || metadata.LastReplyAt.After(lastOpened)
			}
		}

		result = append(result, &FollowedThread{
			SpaceID:           SpaceIDForKind(kind),
			RoomID:            roomID,
			ThreadRootEventID: threadRootEventID,
			ReplyCount:        metadata.ReplyCount,
			LastReplyAt:       metadata.LastReplyAt,
			ParticipantIDs:    metadata.ParticipantIDs,
			HasUnread:         hasUnread,
		})
	}

	return result, nil
}

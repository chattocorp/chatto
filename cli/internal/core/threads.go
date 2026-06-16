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

// FollowedThreadsPage is a paginated set of followed threads with the total
// count before pagination.
type FollowedThreadsPage struct {
	Threads    []*FollowedThread
	TotalCount int
	HasMore    bool
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
	rootEntry, ok := c.rooms().timelineEntry(threadRootEventId)
	if !ok {
		return nil, fmt.Errorf("thread root message not found: event ID %s", threadRootEventId)
	}
	if rootEntry.Event.GetMessagePosted() == nil {
		return nil, fmt.Errorf("event ID %s is not a message event", threadRootEventId)
	}

	replies := c.rooms().threadEvents(threadRootEventId)
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

// GetThreadReplyEvents returns a paginated page of message-posted replies for a
// thread root. The root event itself is not included. Results are chronological
// and use the same stream-sequence cursor convention as room events.
//
// Authorization: caller must verify room membership before calling.
func (c *ChattoCore) GetThreadReplyEvents(ctx context.Context, kind RoomKind, roomID, threadRootEventID string, limit int, beforeSeq *uint64, afterSeq *uint64) (*RoomEventsResult, error) {
	limit = clampHistoricalMessageLimit(limit)

	rootEntry, ok := c.rooms().timelineEntry(threadRootEventID)
	if !ok {
		return nil, fmt.Errorf("thread root message not found: event ID %s", threadRootEventID)
	}
	if rootEntry.Event.GetMessagePosted() == nil {
		return nil, fmt.Errorf("event ID %s is not a message event", threadRootEventID)
	}
	if roomIDOfEvent(rootEntry.Event) != roomID {
		return nil, fmt.Errorf("thread root message not found in room %s: event ID %s", roomID, threadRootEventID)
	}

	entries := c.rooms().threadEvents(threadRootEventID)
	if afterSeq != nil && *afterSeq > 0 {
		return threadReplyEventsAfter(entries, *afterSeq, limit), nil
	}

	var before uint64
	if beforeSeq != nil {
		before = *beforeSeq
	}

	raw := make([]*RoomEvent, 0, limit+1)
	for i := len(entries) - 1; i >= 0 && len(raw) < limit+1; i-- {
		entry := entries[i]
		if !isThreadReplyEventForPage(entry) {
			continue
		}
		if before > 0 && entry.StreamSeq >= before {
			continue
		}
		raw = append(raw, &RoomEvent{Event: entry.Event, Sequence: entry.StreamSeq})
	}

	hasOlder := len(raw) > limit
	if hasOlder {
		raw = raw[:limit]
	}

	for i, j := 0, len(raw)-1; i < j; i, j = i+1, j-1 {
		raw[i], raw[j] = raw[j], raw[i]
	}

	result := &RoomEventsResult{
		Events:   raw,
		HasOlder: hasOlder,
		HasNewer: beforeSeq != nil,
	}
	setRoomEventsResultCursors(result)
	return result, nil
}

// GetThreadReplyEventsAround returns a chronological page of replies centered
// on anchorEventID. The root event itself is not included.
//
// Authorization: caller must verify room membership before calling.
func (c *ChattoCore) GetThreadReplyEventsAround(ctx context.Context, kind RoomKind, roomID, threadRootEventID, anchorEventID string, limit int) (*RoomEventsResult, error) {
	limit = clampHistoricalMessageLimit(limit)

	rootEntry, ok := c.rooms().timelineEntry(threadRootEventID)
	if !ok {
		return nil, fmt.Errorf("thread root message not found: event ID %s", threadRootEventID)
	}
	if rootEntry.Event.GetMessagePosted() == nil {
		return nil, fmt.Errorf("event ID %s is not a message event", threadRootEventID)
	}
	if roomIDOfEvent(rootEntry.Event) != roomID {
		return nil, fmt.Errorf("thread root message not found in room %s: event ID %s", roomID, threadRootEventID)
	}

	entries := c.rooms().threadEvents(threadRootEventID)
	replies := make([]*TimelineEntry, 0, len(entries))
	targetIndex := 0
	foundAnchor := anchorEventID == threadRootEventID
	for _, entry := range entries {
		if !isThreadReplyEventForPage(entry) {
			continue
		}
		if entry.Event.GetId() == anchorEventID {
			targetIndex = len(replies)
			foundAnchor = true
		}
		replies = append(replies, entry)
	}
	if !foundAnchor {
		return nil, ErrMessageNotFound
	}

	start := 0
	end := len(replies)
	if anchorEventID == threadRootEventID {
		if end > limit {
			end = limit
		}
	} else {
		beforeCount := (limit - 1) / 2
		start = targetIndex - beforeCount
		if start < 0 {
			start = 0
		}
		end = start + limit
		if end > len(replies) {
			end = len(replies)
			start = end - limit
			if start < 0 {
				start = 0
			}
		}
	}

	raw := make([]*RoomEvent, 0, end-start)
	for _, entry := range replies[start:end] {
		raw = append(raw, &RoomEvent{Event: entry.Event, Sequence: entry.StreamSeq})
	}

	result := &RoomEventsResult{
		Events:   raw,
		HasOlder: start > 0,
		HasNewer: end < len(replies),
	}
	setRoomEventsResultCursors(result)
	return result, nil
}

func threadReplyEventsAfter(entries []*TimelineEntry, afterSeq uint64, limit int) *RoomEventsResult {
	raw := make([]*RoomEvent, 0, limit+1)
	for _, entry := range entries {
		if !isThreadReplyEventForPage(entry) {
			continue
		}
		if entry.StreamSeq <= afterSeq {
			continue
		}
		raw = append(raw, &RoomEvent{Event: entry.Event, Sequence: entry.StreamSeq})
		if len(raw) >= limit+1 {
			break
		}
	}

	hasNewer := len(raw) > limit
	if hasNewer {
		raw = raw[:limit]
	}

	result := &RoomEventsResult{
		Events:   raw,
		HasOlder: true,
		HasNewer: hasNewer,
	}
	setRoomEventsResultCursors(result)
	return result
}

func isThreadReplyEventForPage(entry *TimelineEntry) bool {
	return entry != nil && entry.Event != nil && entry.Event.GetMessagePosted() != nil
}

func setRoomEventsResultCursors(result *RoomEventsResult) {
	if result == nil || len(result.Events) == 0 {
		return
	}
	result.StartCursorSeq = result.Events[0].Sequence
	result.EndCursorSeq = result.Events[len(result.Events)-1].Sequence
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
// thread has no replies. Derived from the ThreadProjection's cached summary.
func (c *ChattoCore) GetThreadMetadata(ctx context.Context, kind RoomKind, roomID string, rootEventId string) (*ThreadMetadata, error) {
	return c.rooms().threadMetadata(rootEventId), nil
}

// threadLastOpenedKey returns the RUNTIME_STATE key for tracking the latest
// thread message the user has seen.
func threadLastOpenedKey(userID, roomID, threadRootEventID string) string {
	return fmt.Sprintf("read.thread.%s.%s.%s", userID, roomID, threadRootEventID)
}

// GetThreadLastOpened retrieves the timestamp of the latest thread message the
// user has seen. Returns zero time if the thread has never been opened.
//
// New RUNTIME_STATE markers store the seen message event ID. Values migrated
// from SERVER_RUNTIME may still be the legacy 8-byte UnixNano timestamp; those
// are decoded here so existing read state survives the rollout.
func (c *ChattoCore) GetThreadLastOpened(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string) (time.Time, error) {
	entry, err := c.storage.runtimeStateKV.Get(ctx, threadLastOpenedKey(userID, roomID, threadRootEventID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return time.Time{}, nil // Never opened
		}
		return time.Time{}, fmt.Errorf("failed to get thread last opened: %w", err)
	}

	return c.threadReadMarkerTime(ctx, kind, roomID, entry.Value())
}

// SetThreadLastReadEventID stores eventID as the latest thread message the user
// has seen, but only if it is newer than the existing marker (advance-only).
// Returns the previous marker time (zero if never opened before).
func (c *ChattoCore) SetThreadLastReadEventID(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID, eventID string) (time.Time, error) {
	bucket := c.storage.runtimeStateKV
	key := threadLastOpenedKey(userID, roomID, threadRootEventID)

	var previousTime time.Time
	entry, err := bucket.Get(ctx, key)
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return time.Time{}, fmt.Errorf("failed to get previous thread last opened: %w", err)
	}
	if err == nil {
		previousTime, err = c.threadReadMarkerTime(ctx, kind, roomID, entry.Value())
		if err != nil {
			return time.Time{}, err
		}
	}

	nextTime, err := c.GetEventTimestamp(ctx, kind, roomID, eventID)
	if err != nil {
		return time.Time{}, err
	}
	if nextTime.IsZero() {
		return previousTime, nil
	}

	if !nextTime.After(previousTime) {
		return previousTime, nil
	}

	if _, err = bucket.Put(ctx, key, []byte(eventID)); err != nil {
		return time.Time{}, fmt.Errorf("failed to set thread last opened: %w", err)
	}

	c.logger.Debug("Set thread last read event", "user_id", userID, "room_id", roomID, "thread_root_event_id", threadRootEventID, "previous", previousTime, "event_id", eventID)
	return previousTime, nil
}

// SetThreadLastOpenedAt is retained for timestamp-based callers/tests. It
// stores a legacy timestamp marker in RUNTIME_STATE and should not be used for
// new code when a concrete event ID is available.
func (c *ChattoCore) SetThreadLastOpenedAt(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string, ts time.Time) (time.Time, error) {
	bucket := c.storage.runtimeStateKV
	key := threadLastOpenedKey(userID, roomID, threadRootEventID)

	var previousTime time.Time
	entry, err := bucket.Get(ctx, key)
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return time.Time{}, fmt.Errorf("failed to get previous thread last opened: %w", err)
	}
	if err == nil {
		previousTime, err = c.threadReadMarkerTime(ctx, kind, roomID, entry.Value())
		if err != nil {
			return time.Time{}, err
		}
	}

	if !ts.After(previousTime) {
		return previousTime, nil
	}

	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(ts.UnixNano()))
	if _, err = bucket.Put(ctx, key, buf); err != nil {
		return time.Time{}, fmt.Errorf("failed to set thread last opened: %w", err)
	}
	c.logger.Debug("Set legacy thread last opened timestamp", "user_id", userID, "room_id", roomID, "thread_root_event_id", threadRootEventID, "previous", previousTime, "at", ts)
	return previousTime, nil
}

// SetThreadLastOpened records the latest current reply in the thread as read.
// Returns the previous marker time (zero if never opened before).
func (c *ChattoCore) SetThreadLastOpened(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string) (time.Time, error) {
	latestID := c.latestThreadMessageEventID(threadRootEventID)
	if latestID == "" {
		return time.Time{}, nil
	}
	return c.SetThreadLastReadEventID(ctx, kind, userID, roomID, threadRootEventID, latestID)
}

func (c *ChattoCore) threadReadMarkerTime(ctx context.Context, kind RoomKind, roomID string, value []byte) (time.Time, error) {
	if len(value) == 8 {
		nanos := int64(binary.BigEndian.Uint64(value))
		return time.Unix(0, nanos), nil
	}
	eventID := string(value)
	if eventID == "" {
		return time.Time{}, nil
	}
	return c.GetEventTimestamp(ctx, kind, roomID, eventID)
}

func (c *ChattoCore) latestThreadMessageEventID(threadRootEventID string) string {
	entries := c.rooms().threadEvents(threadRootEventID)
	for i := len(entries) - 1; i >= 0; i-- {
		event := entries[i].Event
		if event == nil || event.GetMessagePosted() == nil {
			continue
		}
		if id := event.GetId(); id != "" {
			return id
		}
	}
	return threadRootEventID
}

// threadFollowKey returns the KV key for tracking whether a user is following a thread.
func threadFollowKey(userID, roomID, threadRootEventID string) string {
	return fmt.Sprintf("thread_follow.%s.%s.%s", userID, roomID, threadRootEventID)
}

// FollowThread marks a user as following a thread so they receive reply notifications.
// Stores a single byte in RUNTIME_STATE. Idempotent.
// Publishes a ThreadFollowChangedEvent for multi-tab sync.
func (c *ChattoCore) FollowThread(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string) error {
	bucket := c.storage.runtimeStateKV

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
	bucket := c.storage.runtimeStateKV

	if err := bucket.Delete(ctx, threadFollowKey(userID, roomID, threadRootEventID)); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to unfollow thread: %w", err)
	}

	c.publishThreadFollowChangedEvent(ctx, userID, kind, roomID, threadRootEventID, false)
	return nil
}

// publishThreadFollowChangedEvent publishes a live event when a user's thread follow state changes.
// User-scoped: only delivered to the user who changed their follow state.
func (c *ChattoCore) publishThreadFollowChangedEvent(ctx context.Context, userID string, kind RoomKind, roomID, threadRootEventID string, isFollowing bool) {
	event := newLiveEvent(userID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_ThreadFollowChanged{
			ThreadFollowChanged: &corev1.ThreadFollowChangedEvent{
				RoomId:            roomID,
				ThreadRootEventId: threadRootEventID,
				IsFollowing:       isFollowing,
			},
		},
	})

	subject := subjects.LiveSyncUserEvent(userID, "thread_follow_changed")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Warn("Failed to publish thread follow changed event", "error", err, "user_id", userID, "thread_root_event_id", threadRootEventID)
	}
}

// IsFollowingThread checks if a user is following a thread.
func (c *ChattoCore) IsFollowingThread(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string) (bool, error) {
	bucket := c.storage.runtimeStateKV

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
	bucket := c.storage.runtimeStateKV

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

// ListFollowedThreads returns all threads followed by the user in the given
// spaces, sorted by last activity (newest first).
// Authorization: Caller must verify space membership before calling.
func (c *ChattoCore) ListFollowedThreads(ctx context.Context, userID string, spaceIDs []string) ([]*FollowedThread, error) {
	page, err := c.ListFollowedThreadsPage(ctx, userID, spaceIDs, 0, 0)
	if err != nil {
		return nil, err
	}
	return page.Threads, nil
}

// ListFollowedThreadsPage returns followed threads for the user in the given
// spaces, sorted by last activity (newest first), with pagination applied before
// per-thread read-marker lookups.
//
// Authorization: Caller must verify space membership before calling.
func (c *ChattoCore) ListFollowedThreadsPage(ctx context.Context, userID string, spaceIDs []string, limit, offset int) (*FollowedThreadsPage, error) {
	var allThreads []*FollowedThread

	for _, spaceID := range spaceIDs {
		threads, err := c.listFollowedThreadsInSpace(ctx, userID, RoomKindFromLegacySpaceID(spaceID))
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

	totalCount := len(allThreads)
	if offset < 0 {
		offset = 0
	}
	if limit < 0 {
		limit = 0
	}

	pageThreads := allThreads
	if offset >= totalCount {
		pageThreads = nil
	} else if limit > 0 {
		end := offset + limit
		if end > totalCount {
			end = totalCount
		}
		pageThreads = allThreads[offset:end]
	} else if offset > 0 {
		pageThreads = allThreads[offset:]
	}

	for _, thread := range pageThreads {
		thread.HasUnread = c.followedThreadHasUnread(ctx, userID, thread)
	}

	return &FollowedThreadsPage{
		Threads:    pageThreads,
		TotalCount: totalCount,
		HasMore:    offset+len(pageThreads) < totalCount,
	}, nil
}

// listFollowedThreadsInSpace returns all threads followed by the user in a single space.
func (c *ChattoCore) listFollowedThreadsInSpace(ctx context.Context, userID string, kind RoomKind) ([]*FollowedThread, error) {
	bucket := c.storage.runtimeStateKV

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

		result = append(result, &FollowedThread{
			SpaceID:           LegacySpaceIDForRoomKind(kind),
			RoomID:            roomID,
			ThreadRootEventID: threadRootEventID,
			ReplyCount:        metadata.ReplyCount,
			LastReplyAt:       metadata.LastReplyAt,
			ParticipantIDs:    metadata.ParticipantIDs,
		})
	}

	return result, nil
}

func (c *ChattoCore) followedThreadHasUnread(ctx context.Context, userID string, thread *FollowedThread) bool {
	if thread == nil || thread.LastReplyAt == nil {
		return false
	}
	kind := RoomKindFromLegacySpaceID(thread.SpaceID)
	lastOpened, err := c.GetThreadLastOpened(ctx, kind, userID, thread.RoomID, thread.ThreadRootEventID)
	if err != nil {
		return true
	}
	return lastOpened.IsZero() || thread.LastReplyAt.After(lastOpened)
}

// HasUnreadFollowedThreads reports whether any followed thread has unread
// replies without materializing the full followed-thread result.
func (c *ChattoCore) HasUnreadFollowedThreads(ctx context.Context, userID string, spaceIDs []string) (bool, error) {
	for _, spaceID := range spaceIDs {
		threads, err := c.listFollowedThreadsInSpace(ctx, userID, RoomKindFromLegacySpaceID(spaceID))
		if err != nil {
			c.logger.Warn("Failed to list followed threads for space", "space_id", spaceID, "error", err)
			continue
		}
		for _, thread := range threads {
			if c.followedThreadHasUnread(ctx, userID, thread) {
				return true, nil
			}
		}
	}
	return false, nil
}

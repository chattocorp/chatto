package core

import (
	"context"

	"fmt"

	"time"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

// ThreadMetadata contains existence and display metadata for a thread.
type ThreadMetadata struct {
	Exists             bool
	ReplyCount         int
	LastReplyAt        *time.Time
	LatestReplyEventID string
	ParticipantIDs     []string
	// ParticipantCount counts the root author and current reply authors once each,
	// excluding erased users, beyond the preview limit.
	ParticipantCount int
}

// FollowedThread represents a thread the user is following, enriched with metadata for display.
type FollowedThread struct {
	SpaceID           string
	RoomID            string
	ThreadRootEventID string
	Exists            bool
	ReplyCount        int
	LastReplyAt       *time.Time
	ActivityAt        *time.Time
	// RootCreatedAt is the root message creation time, independent of replies.
	RootCreatedAt  *time.Time
	ParticipantIDs []string
	// ParticipantCount counts the root author and current reply authors once each,
	// excluding erased users, beyond the preview limit.
	ParticipantCount   int
	LatestReplyEventID string
	HasUnreadReplies   bool
}

// newFollowedThread builds a thread row from projected metadata. ActivityAt is
// the last reply time. For a thread without replies, it is rootCreatedAt,
// which can be nil when the root time is unknown.
func newFollowedThread(kind RoomKind, roomID, rootEventID string, metadata *ThreadMetadata, rootCreatedAt *time.Time) *FollowedThread {
	activityAt := metadata.LastReplyAt
	if activityAt == nil {
		activityAt = rootCreatedAt
	}
	return &FollowedThread{
		SpaceID:            LegacySpaceIDForRoomKind(kind),
		RoomID:             roomID,
		ThreadRootEventID:  rootEventID,
		Exists:             metadata.Exists,
		ReplyCount:         metadata.ReplyCount,
		LastReplyAt:        metadata.LastReplyAt,
		ActivityAt:         activityAt,
		RootCreatedAt:      rootCreatedAt,
		LatestReplyEventID: metadata.LatestReplyEventID,
		ParticipantIDs:     metadata.ParticipantIDs,
		ParticipantCount:   metadata.ParticipantCount,
	}
}

// ThreadOrder selects the date used to order followed threads before pagination.
type ThreadOrder int32

const (
	// ThreadOrderDefault preserves the caller's default ordering.
	ThreadOrderDefault ThreadOrder = iota
	// ThreadOrderActivity puts the latest reply or unreplied root first.
	ThreadOrderActivity
	// ThreadOrderCreated puts the newest root message first.
	ThreadOrderCreated
)

// rootCreatedAt reads the detached timeline metadata without hydrating message
// bodies. Callers must check thread access before using this sort date.
func (s *ThreadFollowModel) rootCreatedAt(roomID, rootID string) *time.Time {
	entry, ok := s.rooms.timelineEntry(rootID)
	if !ok || entry.RoomID != roomID || !entry.IsMessagePost() || entry.InThreadEventID != "" || s.rooms.isHiddenEcho(rootID) || entry.CreatedAt.IsZero() {
		return nil
	}
	return &entry.CreatedAt
}

// Validate rejects unsupported order values at the domain boundary.
func (o ThreadOrder) Validate() error {
	if o < ThreadOrderDefault || o > ThreadOrderCreated {
		return fmt.Errorf("%w: unsupported thread order", ErrInvalidArgument)
	}
	return nil
}

// compareThreadDates orders newest dates first, unknown dates last, then identity.
func compareThreadDates(a, b *FollowedThread, order ThreadOrder) int {
	at, bt := a.ActivityAt, b.ActivityAt
	if order == ThreadOrderCreated {
		at, bt = a.RootCreatedAt, b.RootCreatedAt
	}
	if at != nil && bt == nil {
		return -1
	}
	if at == nil && bt != nil {
		return 1
	}
	if at != nil && bt != nil {
		if cmp := bt.Compare(*at); cmp != 0 {
			return cmp
		}
	}
	ak, bk := followedThreadSortKey(a), followedThreadSortKey(b)
	if ak < bk {
		return -1
	}
	if ak > bk {
		return 1
	}
	return 0
}

// FollowedThreadsPage is a paginated set of followed threads with the total
// count before pagination.
type FollowedThreadsPage struct {
	Threads    []*FollowedThread
	TotalCount int
	HasMore    bool
}

// maxThreadParticipants bounds the display preview; the full author set is retained.
const maxThreadParticipants = 50

// GetThreadEvents returns the root message followed by every reply
// (in stream-arrival order) for the given thread root.
//
// Source: RoomTimelineProjection for the root, ThreadProjection for
// the replies. The ThreadProjection holds replies plus edit/retract
// events targeting them — currently we surface only MessagePostedEvent
// replies here so legacy callers see the same shape as the
// SERVER_EVENTS-backed implementation. Edits / retracts are folded
// onto the original through current-body EVT hydration at resolve time.
//
// Authorization: caller must verify room membership before calling.
func (c *ChattoCore) GetThreadEvents(ctx context.Context, kind RoomKind, room_id string, threadRootEventId string) ([]*evtv1.Event, error) {
	rootEntry, ok := c.roomModel.timelineEntry(threadRootEventId)
	if !ok {
		return nil, fmt.Errorf("thread root message not found: event ID %s", threadRootEventId)
	}
	if !rootEntry.IsMessagePost() {
		return nil, fmt.Errorf("event ID %s is not a message event", threadRootEventId)
	}

	replies := c.roomModel.threadEvents(threadRootEventId)
	refs := make([]*TimelineEntry, 0, 1+len(replies))
	refs = append(refs, rootEntry)
	for _, r := range replies {
		if !r.IsMessagePost() {
			continue
		}
		refs = append(refs, r)
	}
	hydrated, err := c.hydrateTimelineEntries(ctx, refs)
	if err != nil {
		return nil, err
	}
	events := make([]*evtv1.Event, len(hydrated))
	for i, event := range hydrated {
		events[i] = event.Event
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

	rootEntry, ok := c.roomModel.timelineEntry(threadRootEventID)
	if !ok {
		return nil, fmt.Errorf("thread root message not found: event ID %s", threadRootEventID)
	}
	if !rootEntry.IsMessagePost() {
		return nil, fmt.Errorf("event ID %s is not a message event", threadRootEventID)
	}
	if rootEntry.RoomID != roomID {
		return nil, fmt.Errorf("thread root message not found in room %s: event ID %s", roomID, threadRootEventID)
	}

	entries := c.roomModel.threadEvents(threadRootEventID)
	if afterSeq != nil && *afterSeq > 0 {
		return c.threadReplyEventsAfter(ctx, entries, *afterSeq, limit)
	}

	var before uint64
	if beforeSeq != nil {
		before = *beforeSeq
	}

	refs := make([]*TimelineEntry, 0, limit+1)
	for i := len(entries) - 1; i >= 0 && len(refs) < limit+1; i-- {
		entry := entries[i]
		if !isThreadReplyEventForPage(entry) {
			continue
		}
		if before > 0 && entry.StreamSeq >= before {
			continue
		}
		refs = append(refs, entry)
	}

	hasOlder := len(refs) > limit
	if hasOlder {
		refs = refs[:limit]
	}

	for i, j := 0, len(refs)-1; i < j; i, j = i+1, j-1 {
		refs[i], refs[j] = refs[j], refs[i]
	}

	hydrated, err := c.hydrateTimelineEntries(ctx, refs)
	if err != nil {
		return nil, err
	}
	result := &RoomEventsResult{
		Events:   hydrated,
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

	rootEntry, ok := c.roomModel.timelineEntry(threadRootEventID)
	if !ok {
		return nil, fmt.Errorf("thread root message not found: event ID %s", threadRootEventID)
	}
	if !rootEntry.IsMessagePost() {
		return nil, fmt.Errorf("event ID %s is not a message event", threadRootEventID)
	}
	if rootEntry.RoomID != roomID {
		return nil, fmt.Errorf("thread root message not found in room %s: event ID %s", roomID, threadRootEventID)
	}

	entries := c.roomModel.threadEvents(threadRootEventID)
	replies := make([]*TimelineEntry, 0, len(entries))
	targetIndex := 0
	foundAnchor := anchorEventID == threadRootEventID
	for _, entry := range entries {
		if !isThreadReplyEventForPage(entry) {
			continue
		}
		if entry.EventID == anchorEventID {
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
		start = max(targetIndex-beforeCount, 0)
		end = start + limit
		if end > len(replies) {
			end = len(replies)
			start = max(end-limit, 0)
		}
	}

	raw, err := c.hydrateTimelineEntries(ctx, replies[start:end])
	if err != nil {
		return nil, err
	}

	result := &RoomEventsResult{
		Events:   raw,
		HasOlder: start > 0,
		HasNewer: end < len(replies),
	}
	setRoomEventsResultCursors(result)
	return result, nil
}

func (c *ChattoCore) threadReplyEventsAfter(ctx context.Context, entries []*TimelineEntry, afterSeq uint64, limit int) (*RoomEventsResult, error) {
	refs := make([]*TimelineEntry, 0, limit+1)
	for _, entry := range entries {
		if !isThreadReplyEventForPage(entry) {
			continue
		}
		if entry.StreamSeq <= afterSeq {
			continue
		}
		refs = append(refs, entry)
		if len(refs) >= limit+1 {
			break
		}
	}

	hasNewer := len(refs) > limit
	if hasNewer {
		refs = refs[:limit]
	}
	raw, err := c.hydrateTimelineEntries(ctx, refs)
	if err != nil {
		return nil, err
	}

	result := &RoomEventsResult{
		Events:   raw,
		HasOlder: true,
		HasNewer: hasNewer,
	}
	setRoomEventsResultCursors(result)
	return result, nil
}

func isThreadReplyEventForPage(entry *TimelineEntry) bool {
	return entry != nil && entry.IsMessagePost()
}

func setRoomEventsResultCursors(result *RoomEventsResult) {
	if result == nil || len(result.Events) == 0 {
		return
	}
	result.StartCursorSeq = result.Events[0].Sequence
	result.EndCursorSeq = result.Events[len(result.Events)-1].Sequence
}

// GetThreadMetadata returns existence, reply count, last reply timestamp, and
// participants for a thread root message. Derived from Threads and Room Timeline. Root
// event IDs are globally unique, so no room scope is needed. The result is
// never nil; Exists is false for an unknown root.
func (c *ChattoCore) GetThreadMetadata(rootEventID string) *ThreadMetadata {
	return c.roomModel.threadMetadata(rootEventID)
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
	return c.readStateModel.GetThreadLastOpened(ctx, kind, userID, roomID, threadRootEventID)
}

// FollowThread marks a user as following a thread so they receive reply notifications.
// Stores durable follow state in EVT. Idempotent.
// Publishes a ThreadViewerStateChangedEvent for multi-tab sync when state changes.
func (c *ChattoCore) FollowThread(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string) error {
	return c.threadFollows.followThread(ctx, kind, userID, roomID, threadRootEventID)
}

func (c *ChattoCore) FollowThreadWithSource(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string, source evtv1.ThreadFollowSource) error {
	return c.threadFollows.FollowThreadWithSource(ctx, kind, userID, roomID, threadRootEventID, source)
}

// UnfollowThread removes a user's follow on a thread so they stop receiving reply notifications.
// Idempotent - calling when not following is a no-op.
// Publishes a ThreadViewerStateChangedEvent for multi-tab sync when state changes.
func (c *ChattoCore) UnfollowThread(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string) error {
	return c.threadFollows.unfollowThread(ctx, kind, userID, roomID, threadRootEventID)
}

// FollowThreadIfNeverSet follows a thread only when the user has no prior
// follow state. Explicit unfollows and existing follows are left untouched.
func (c *ChattoCore) FollowThreadIfNeverSet(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string, source evtv1.ThreadFollowSource) (bool, error) {
	return c.threadFollows.FollowThreadIfNeverSet(ctx, kind, userID, roomID, threadRootEventID, source)
}

// IsFollowingThread checks if a user is following a thread.
func (c *ChattoCore) IsFollowingThread(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string) (bool, error) {
	return c.threadFollows.IsFollowingThread(ctx, kind, userID, roomID, threadRootEventID)
}

func followedThreadSortKey(thread *FollowedThread) string {
	if thread == nil {
		return ""
	}
	return thread.RoomID + "\x00" + thread.ThreadRootEventID
}

// HasUnreadFollowedThreads reports whether any followed thread has unread
// replies without materializing the public result.
func (c *ChattoCore) HasUnreadFollowedThreads(ctx context.Context, userID string, spaceIDs []string) (bool, error) {
	return c.threadFollows.hasUnreadFollowedThreads(ctx, userID, spaceIDs)
}

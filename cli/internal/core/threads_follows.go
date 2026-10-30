package core

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
)

func (s *ThreadFollowModel) threadFollowState(ctx context.Context, userID, roomID, threadRootEventID string) (ThreadFollowState, error) {
	return s.rooms.threadFollowState(userID, roomID, threadRootEventID), nil
}

func (s *ThreadFollowModel) appendThreadFollowStateEvent(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string, target ThreadFollowState, source evtv1.ThreadFollowSource, onlyIfNeverSet bool) (bool, error) {
	agg := evtstream.RoomAggregate(roomID)
	filter := agg.AllEventsFilter()
	var lastErr error

	for attempt := 1; attempt <= maxThreadCreateAppendAttempts; attempt++ {
		pos, err := s.publisher.LastSubjectPosition(ctx, filter)
		if err != nil {
			return false, fmt.Errorf("read thread follow OCC tail: %w", err)
		}
		if err := s.waitForThreadFollowStateCurrent(ctx, agg); err != nil {
			return false, err
		}

		current := s.rooms.threadFollowState(userID, roomID, threadRootEventID)
		if onlyIfNeverSet && current != ThreadFollowStateNone {
			return false, nil
		}
		if !onlyIfNeverSet && current == target {
			return false, nil
		}

		event := newEvent(userID, &evtv1.Event{})
		switch target {
		case ThreadFollowStateFollowing:
			event.Event = &evtv1.Event_ThreadFollowed{
				ThreadFollowed: &evtv1.ThreadFollowedEvent{
					RoomId:            roomID,
					ThreadRootEventId: threadRootEventID,
					UserId:            userID,
					Source:            source,
				},
			}
		case ThreadFollowStateUnfollowed:
			event.Event = &evtv1.Event_ThreadUnfollowed{
				ThreadUnfollowed: &evtv1.ThreadUnfollowedEvent{
					RoomId:            roomID,
					ThreadRootEventId: threadRootEventID,
					UserId:            userID,
				},
			}
		default:
			return false, fmt.Errorf("unsupported thread follow state %q", target)
		}

		seq, err := s.publisher.AppendAt(ctx, agg.SubjectFor(event), event, events.ExpectFilterSeq(filter, pos.Seq))
		if err == nil {
			if err := s.rooms.waitForThreads(ctx, events.SubjectPosition(agg.SubjectFor(event), seq)); err != nil {
				return true, err
			}
			return true, nil
		}
		if !errors.Is(err, events.ErrConflict) {
			return false, err
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		case <-time.After(time.Duration(1<<attempt) * time.Millisecond):
		}
	}

	return false, fmt.Errorf("append thread follow state after %d attempts: %w", maxThreadCreateAppendAttempts, lastErr)
}

func (s *ThreadFollowModel) waitForThreadFollowStateCurrent(ctx context.Context, agg evtstream.Aggregate) error {
	for _, eventType := range []string{evtstream.EventThreadFollowed, evtstream.EventThreadUnfollowed} {
		pos, err := s.publisher.LastSubjectPosition(ctx, agg.Subject(eventType))
		if err != nil {
			return fmt.Errorf("read thread follow state tail: %w", err)
		}
		if pos.IsZero() {
			continue
		}
		if err := s.rooms.waitForThreads(ctx, pos); err != nil {
			return err
		}
	}
	return nil
}

// followThread marks a user as following a thread so they receive reply notifications.
// Stores durable follow state in EVT. Idempotent.
// Publishes a ThreadViewerStateChangedEvent for multi-tab sync when state changes.
func (s *ThreadFollowModel) followThread(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string) error {
	changed, err := s.appendThreadFollowStateEvent(ctx, kind, userID, roomID, threadRootEventID, ThreadFollowStateFollowing, evtv1.ThreadFollowSource_THREAD_FOLLOW_SOURCE_MANUAL, false)
	if err == nil && changed {
		s.hintBadgeThread(ctx, userID, roomID, threadRootEventID)
	}
	return err
}

// FollowThreadWithSource records a follow with an explicit source for trusted
// internal callers, such as the automatic follow when a user posts a reply. It
// replaces any earlier follow state, including an explicit unfollow, and is a
// no-op when the user already follows the thread. Unlike a manual follow, it
// does not send a Badge hint to the user's clients. It does not check
// membership or read access. Use FollowThreadIfNeverSet to keep an earlier
// user choice.
func (s *ThreadFollowModel) FollowThreadWithSource(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string, source evtv1.ThreadFollowSource) error {
	_, err := s.appendThreadFollowStateEvent(ctx, kind, userID, roomID, threadRootEventID, ThreadFollowStateFollowing, source, false)
	return err
}

// unfollowThread removes a user's follow on a thread so they stop receiving reply notifications.
// Idempotent - calling when not following is a no-op.
// Publishes a ThreadViewerStateChangedEvent for multi-tab sync when state changes.
func (s *ThreadFollowModel) unfollowThread(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string) error {
	changed, err := s.appendThreadFollowStateEvent(ctx, kind, userID, roomID, threadRootEventID, ThreadFollowStateUnfollowed, evtv1.ThreadFollowSource_THREAD_FOLLOW_SOURCE_UNSPECIFIED, false)
	if err == nil && changed {
		s.hintBadgeThread(ctx, userID, roomID, threadRootEventID)
	}
	return err
}

// hintBadgeThread tells the user's clients to re-read a thread's Badge state
// after a manual follow change, which changes followed-thread attention.
func (s *ThreadFollowModel) hintBadgeThread(ctx context.Context, userID, roomID, threadRootEventID string) {
	if err := s.materializer.waitForDecisions(ctx); err != nil {
		s.logger.Warn("Failed to wait for notification decisions after a follow change", "error", err)
		return
	}
	s.integration.NotifyNotificationUnreadStateChanged(ctx, userID, userID, roomID, threadRootEventID)
}

// FollowThreadIfNeverSet follows a thread only when the user has no prior
// follow state. Explicit unfollows and existing follows are left untouched.
func (s *ThreadFollowModel) FollowThreadIfNeverSet(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string, source evtv1.ThreadFollowSource) (bool, error) {
	return s.appendThreadFollowStateEvent(ctx, kind, userID, roomID, threadRootEventID, ThreadFollowStateFollowing, source, true)
}

// IsFollowingThread checks if a user is following a thread.
func (s *ThreadFollowModel) IsFollowingThread(ctx context.Context, kind RoomKind, userID, roomID, threadRootEventID string) (bool, error) {
	state, err := s.threadFollowState(ctx, userID, roomID, threadRootEventID)
	if err != nil {
		return false, fmt.Errorf("failed to check thread follow: %w", err)
	}
	return state == ThreadFollowStateFollowing, nil
}

// GetThreadFollowers returns all user IDs following a specific thread.
func (s *ThreadFollowModel) GetThreadFollowers(ctx context.Context, kind RoomKind, roomID, threadRootEventID string) ([]string, error) {
	var userIDs []string
	for _, userID := range s.rooms.threadFollowers(roomID, threadRootEventID) {
		following, err := s.IsFollowingThread(ctx, kind, userID, roomID, threadRootEventID)
		if err != nil {
			s.logger.Warn("Failed to check thread follower state", "error", err, "room_id", roomID, "thread_root_event_id", threadRootEventID)
			continue
		}
		if following {
			userIDs = append(userIDs, userID)
		}
	}
	return userIDs, nil
}

// listFollowedThreads returns all threads followed by the user in the given
// spaces, sorted by last activity (newest first).
// The result includes only rooms where the user is still a member and has
// applicable message-read authority. Threads with a deleted root and no visible
// replies are omitted before pagination and total counts.
func (s *ThreadFollowModel) listFollowedThreads(ctx context.Context, userID string, spaceIDs []string) ([]*FollowedThread, error) {
	page, err := s.ListFollowedThreadsPage(ctx, userID, spaceIDs, false, 0, 0, ThreadRootAuthorAll, ThreadOrderDefault)
	if err != nil {
		return nil, err
	}
	return page.Threads, nil
}

// ListFollowedThreadsPage returns followed threads for the user in the given
// spaces, sorted by last activity (newest first), with pagination applied before
// per-thread read-marker lookups.
//
// When unreadOnly is set, the function reads the markers of all followed
// threads, keeps only threads with unread replies, and then applies
// pagination. TotalCount then counts only unread threads.
//
// The result includes only rooms where the user is still a member and has
// applicable message-read authority.
func (s *ThreadFollowModel) ListFollowedThreadsPage(ctx context.Context, userID string, spaceIDs []string, unreadOnly bool, limit, offset int, rootAuthor ThreadRootAuthorFilter, order ThreadOrder) (*FollowedThreadsPage, error) {
	if err := order.Validate(); err != nil {
		return nil, err
	}
	if err := rootAuthor.Validate(); err != nil {
		return nil, err
	}
	var allThreads []*FollowedThread

	for _, spaceID := range spaceIDs {
		threads, err := s.listFollowedThreadsInSpace(ctx, userID, RoomKindFromLegacySpaceID(spaceID), order == ThreadOrderCreated)
		if err != nil {
			s.logger.Warn("Failed to list followed threads for space", "space_id", spaceID, "error", err)
			continue
		}
		allThreads = append(allThreads, threads...)
	}

	if rootAuthor != ThreadRootAuthorAll {
		filtered := allThreads[:0]
		for _, thread := range allThreads {
			matches, err := s.matchesRootAuthor(ctx, userID, thread.RoomID, thread.ThreadRootEventID, rootAuthor)
			if err != nil {
				return nil, err
			}
			if matches {
				filtered = append(filtered, thread)
			}
		}
		allThreads = filtered
	}

	// Sort the complete authorized set before pagination, with a stable identity tie-break.
	sort.Slice(allThreads, func(i, j int) bool {
		return compareThreadDates(allThreads[i], allThreads[j], order) < 0
	})

	if unreadOnly {
		if err := s.hydrateFollowedThreadViewerStates(ctx, userID, allThreads); err != nil {
			return nil, err
		}
		unread := allThreads[:0]
		for _, thread := range allThreads {
			if thread.HasUnreadReplies {
				unread = append(unread, thread)
			}
		}
		allThreads = unread
	}

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
		end := min(offset+limit, totalCount)
		pageThreads = allThreads[offset:end]
	} else if offset > 0 {
		pageThreads = allThreads[offset:]
	}

	if !unreadOnly {
		if err := s.hydrateFollowedThreadViewerStates(ctx, userID, pageThreads); err != nil {
			return nil, err
		}
	}

	return &FollowedThreadsPage{
		Threads:    pageThreads,
		TotalCount: totalCount,
		HasMore:    offset+len(pageThreads) < totalCount,
	}, nil
}

// listFollowedThreadsInSpace returns all threads followed by the user in a single space.
func (s *ThreadFollowModel) listFollowedThreadsInSpace(ctx context.Context, userID string, kind RoomKind, includeRootDate bool) ([]*FollowedThread, error) {
	var result []*FollowedThread
	for _, ref := range s.rooms.followedThreadsForUser(userID) {
		roomID := ref.roomID
		threadRootEventID := ref.threadRootEventID

		room, err := s.rooms.FindRoomByID(ctx, roomID)
		if err != nil {
			s.logger.Warn("Skipping followed thread with unavailable room", "error", err, "room_id", roomID, "thread_root_event_id", threadRootEventID)
			continue
		}
		if KindOfRoom(room) != kind {
			continue
		}

		following, err := s.IsFollowingThread(ctx, kind, userID, roomID, threadRootEventID)
		if err != nil {
			s.logger.Warn("Failed to check thread follow state while listing", "error", err, "room_id", roomID, "thread_root_event_id", threadRootEventID)
			continue
		}
		if !following {
			continue
		}

		isMember, err := s.policy.RoomMembershipExists(ctx, kind, userID, roomID)
		if err != nil {
			s.logger.Warn("Failed to check room membership for followed thread", "error", err, "room_id", roomID, "thread_root_event_id", threadRootEventID)
			continue
		}
		if !isMember {
			continue
		}
		canRead, err := s.policy.CanReadThreadMessages(ctx, userID, kind, roomID, threadRootEventID)
		if err != nil {
			s.logger.Warn("Failed to check message-read authority for followed thread", "error", err, "room_id", roomID, "thread_root_event_id", threadRootEventID)
			continue
		}
		if !canRead {
			continue
		}

		metadata := s.rooms.threadMetadata(threadRootEventID)
		// Keep the follow state, but hide threads that have no content left.
		if metadata.ReplyCount == 0 && s.rooms.messageTombstoned(threadRootEventID) {
			continue
		}
		var rootCreatedAt *time.Time
		if includeRootDate {
			rootCreatedAt = s.rootCreatedAt(roomID, threadRootEventID)
		} else if metadata.LastReplyAt == nil {
			root, err := s.integration.GetRoomEventByEventID(ctx, kind, roomID, threadRootEventID)
			if err != nil {
				s.logger.Warn("Failed to get root activity for followed thread", "error", err, "room_id", roomID, "thread_root_event_id", threadRootEventID)
				continue
			}
			if root != nil && root.GetCreatedAt() != nil {
				at := root.GetCreatedAt().AsTime()
				rootCreatedAt = &at
			}
		}
		result = append(result, newFollowedThread(kind, roomID, threadRootEventID, metadata, rootCreatedAt))
	}

	return result, nil
}

func (s *ThreadFollowModel) hydrateFollowedThreadViewerStates(ctx context.Context, userID string, threads []*FollowedThread) error {
	for _, thread := range threads {
		if thread == nil || thread.LastReplyAt == nil {
			continue
		}
		kind := RoomKindFromLegacySpaceID(thread.SpaceID)
		lastOpened, err := s.readState.GetThreadLastOpened(ctx, kind, userID, thread.RoomID, thread.ThreadRootEventID)
		if err != nil {
			return fmt.Errorf("read followed thread cursor %s: %w", thread.ThreadRootEventID, err)
		}
		thread.HasUnreadReplies = lastOpened.IsZero() || thread.LastReplyAt.After(lastOpened)
	}
	return nil
}

// hasUnreadFollowedThreads reports whether any followed thread has unread
// replies without materializing the public result.
func (s *ThreadFollowModel) hasUnreadFollowedThreads(ctx context.Context, userID string, spaceIDs []string) (bool, error) {
	for _, spaceID := range spaceIDs {
		threads, err := s.listFollowedThreadsInSpace(ctx, userID, RoomKindFromLegacySpaceID(spaceID), false)
		if err != nil {
			s.logger.Warn("Failed to list followed threads for space", "space_id", spaceID, "error", err)
			continue
		}
		if err := s.hydrateFollowedThreadViewerStates(ctx, userID, threads); err != nil {
			return false, err
		}
		for _, thread := range threads {
			if thread.HasUnreadReplies {
				return true, nil
			}
		}
	}
	return false, nil
}

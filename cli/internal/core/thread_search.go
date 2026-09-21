package core

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"time"

	"google.golang.org/protobuf/proto"
	searchv1 "hmans.de/chatto/internal/pb/chatto/search/v1"
	searchsvc "hmans.de/chatto/internal/search"
)

// ThreadSearchQuery queries the optional provider. Hits are untrusted until hydration.
type ThreadSearchQuery func(context.Context, *searchv1.QueryRequest) (*searchv1.QueryResponse, error)

// ThreadSearchResult joins current display state with one authorized matching message.
type ThreadSearchResult struct {
	Metadata  *FollowedThread
	Match     MessageSearchResult
	Following bool
}

// ThreadSearchPage counts distinct authorized groups before pagination.
type ThreadSearchPage struct {
	Threads    []ThreadSearchResult
	TotalCount int
	HasMore    bool
}

// FollowedSearchRoots returns the complete followed-root scope in authorized rooms.
// Empty must be handled by the caller as no matches, never unrestricted search.
func (s *MessageSearchReadModel) FollowedSearchRoots(scope *MessageSearchScope, actorID string) []string {
	var roots []string
	for _, ref := range s.core.roomModel.followedThreadsForUser(actorID) {
		if scope.rooms[ref.roomID] != nil {
			roots = append(roots, ref.threadRootEventID)
		}
	}
	slices.Sort(roots)
	return roots
}

// FilterFollowedSearchResults rechecks follow state after provider work.
func (s *MessageSearchReadModel) FilterFollowedSearchResults(ctx context.Context, actorID string, results []MessageSearchResult) ([]MessageSearchResult, error) {
	filtered := make([]MessageSearchResult, 0, len(results))
	for _, result := range results {
		following, err := s.core.IsFollowingThread(ctx, result.Kind, actorID, result.Event.GetMessagePosted().GetRoomId(), searchResultRoot(result))
		if err != nil {
			return nil, err
		}
		if following {
			filtered = append(filtered, result)
		}
	}
	return filtered, nil
}

func searchResultRoot(result MessageSearchResult) string {
	if root := result.Event.GetMessagePosted().GetInThread(); root != "" {
		return root
	}
	return result.Event.GetId()
}

// SearchThreadGroups resolves groups before pagination. The first current hit
// in provider order represents its thread; resolved roots are excluded on the
// next request so a large thread does not require reading all matching replies.
// Totals and activity order require visiting all matching groups. Work has one
// deadline, including stale-hit walks. Cursors do not pin an index snapshot.
func (s *MessageSearchReadModel) SearchThreadGroups(ctx context.Context, actorID string, limit, offset int, scope *MessageSearchScope, normalized *searchv1.QueryRequest, followedOnly, activityOrder bool, query ThreadSearchQuery) (*ThreadSearchPage, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	request := proto.Clone(normalized).(*searchv1.QueryRequest)
	request.PageSize, request.Cursor = 100, nil
	if scope.NoMatches || len(scope.RoomIDs) == 0 {
		return &ThreadSearchPage{}, nil
	}
	if err := searchsvc.ValidateQueryRequest(request); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidArgument, err)
	}
	matched := make(map[string]MessageSearchResult)
	matchedHits := make(map[string]MessageSearchHit)
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		response, err := query(ctx, request)
		if err != nil {
			return nil, err
		}
		if response == nil || (followedOnly && !response.GetThreadScopeApplied()) || (len(request.ExcludedThreadRootIds) > 0 && !response.GetThreadExclusionsApplied()) {
			return nil, fmt.Errorf("%w: provider does not support thread filters", searchsvc.ErrUnavailable)
		}
		hits := make([]MessageSearchHit, 0, len(response.GetHits()))
		byMessage := make(map[string]MessageSearchHit, len(response.GetHits()))
		for _, hit := range response.GetHits() {
			candidate := MessageSearchHit{MessageID: hit.GetMessageId(), RoomID: hit.GetRoomId(), BodyEventID: hit.GetBodyEventId(), Score: hit.GetRelevanceScore()}
			hits = append(hits, candidate)
			byMessage[searchHitKey(candidate.RoomID, candidate.MessageID)] = candidate
		}
		current, err := s.HydrateHits(ctx, actorID, scope, hits)
		if err != nil {
			return nil, err
		}
		added := false
		for _, result := range current {
			if result.Event.GetMessagePosted().GetEchoOfEventId() != "" {
				continue
			}
			root := searchResultRoot(result)
			if followedOnly && !slices.Contains(request.ThreadRootIds, root) {
				continue
			}
			if _, exists := matched[root]; exists {
				continue
			}
			matched[root] = result
			matchedHits[root] = byMessage[searchHitKey(result.Event.GetMessagePosted().GetRoomId(), result.Event.GetId())]
			request.ExcludedThreadRootIds = append(request.ExcludedThreadRootIds, root)
			added = true
		}
		if len(response.GetNextCursor()) == 0 {
			break
		}
		if added {
			slices.Sort(request.ExcludedThreadRootIds)
			request.Cursor = nil
		} else {
			if bytes.Equal(request.Cursor, response.GetNextCursor()) {
				return nil, fmt.Errorf("%w: search cursor did not advance", searchsvc.ErrInvalidResponse)
			}
			request.Cursor = response.GetNextCursor()
		}
	}
	// Enumeration can span many provider calls. Revalidate accepted body
	// revisions and access before counting or returning the final groups.
	accepted := make([]MessageSearchHit, 0, len(matchedHits))
	for _, hit := range matchedHits {
		accepted = append(accepted, hit)
	}
	clear(matched)
	for start := 0; start < len(accepted); start += 100 {
		current, err := s.HydrateHits(ctx, actorID, scope, accepted[start:min(start+100, len(accepted))])
		if err != nil {
			return nil, err
		}
		for _, result := range current {
			matched[searchResultRoot(result)] = result
		}
	}
	threads := make([]ThreadSearchResult, 0, len(matched))
	for root, match := range matched {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		room := match.Event.GetMessagePosted().GetRoomId()
		allowed, err := s.core.CanReadThreadMessages(ctx, actorID, match.Kind, room, root)
		if err != nil {
			return nil, err
		}
		member, err := s.core.RoomMembershipExists(ctx, match.Kind, actorID, room)
		if err != nil {
			return nil, err
		}
		if !allowed || !member {
			continue
		}
		following, err := s.core.IsFollowingThread(ctx, match.Kind, actorID, room, root)
		if err != nil {
			return nil, err
		}
		if followedOnly && !following {
			continue
		}
		metadata, err := s.core.GetThreadMetadata(ctx, match.Kind, room, root)
		if err != nil {
			return nil, err
		}
		activity := metadata.LastReplyAt
		if activity == nil {
			entry, ok := s.core.roomModel.timelineEntry(root)
			if !ok || entry.RoomID != room {
				continue
			}
			rootEvent, err := s.core.GetRoomEventByEventID(ctx, match.Kind, room, root)
			if err != nil {
				return nil, err
			}
			if rootEvent == nil {
				continue
			}
			at := rootEvent.GetCreatedAt().AsTime()
			activity = &at
		}
		row := &FollowedThread{SpaceID: LegacySpaceIDForRoomKind(match.Kind), RoomID: room, ThreadRootEventID: root,
			Exists: metadata.Exists, ReplyCount: metadata.ReplyCount, LastReplyAt: metadata.LastReplyAt, ActivityAt: activity,
			LatestReplyEventID: metadata.LatestReplyEventID, ParticipantIDs: metadata.ParticipantIDs, ParticipantCount: metadata.ParticipantCount}
		threads = append(threads, ThreadSearchResult{Metadata: row, Match: match, Following: following})
	}
	slices.SortFunc(threads, func(a, b ThreadSearchResult) int {
		if activityOrder {
			if cmp := b.Metadata.ActivityAt.Compare(*a.Metadata.ActivityAt); cmp != 0 {
				return cmp
			}
		} else {
			if normalized.Order == searchv1.SearchOrder_SEARCH_ORDER_RELEVANCE {
				if a.Match.Score > b.Match.Score {
					return -1
				}
				if a.Match.Score < b.Match.Score {
					return 1
				}
			}
			if cmp := b.Match.Event.GetCreatedAt().AsTime().Compare(a.Match.Event.GetCreatedAt().AsTime()); cmp != 0 {
				return cmp
			}
		}
		return bytes.Compare([]byte(followedThreadSortKey(a.Metadata)), []byte(followedThreadSortKey(b.Metadata)))
	})
	total := len(threads)
	start := min(max(offset, 0), total)
	end := min(start+limit, total)
	page := threads[start:end]
	for _, result := range page {
		if err := s.core.hydrateFollowedThreadViewerStates(ctx, actorID, []*FollowedThread{result.Metadata}); err != nil {
			return nil, err
		}
	}
	return &ThreadSearchPage{Threads: page, TotalCount: total, HasMore: end < total}, nil
}

package core

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"time"

	searchv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/search/v1"
	searchsvc "github.com/chattocorp/chatto/cli/internal/search"
	"google.golang.org/protobuf/proto"
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

// FollowedSearchRoots returns the followed-root scope in authorized rooms,
// restricted by root author when requested.
// Empty must be handled by the caller as no matches, never unrestricted search.
func (s *MessageSearchReadModel) FollowedSearchRoots(ctx context.Context, scope *MessageSearchScope, actorID string, rootAuthor ThreadRootAuthorFilter) ([]string, error) {
	if err := rootAuthor.Validate(); err != nil {
		return nil, err
	}
	var roots []string
	for _, ref := range s.rooms.followedThreadsForUser(actorID) {
		if scope.rooms[ref.roomID] != nil {
			matches, err := s.follows.matchesRootAuthor(ctx, actorID, ref.roomID, ref.threadRootEventID, rootAuthor)
			if err != nil {
				return nil, err
			}
			if matches {
				roots = append(roots, ref.threadRootEventID)
			}
		}
	}
	slices.Sort(roots)
	return roots, nil
}

// FilterFollowedSearchResults rechecks follow state after provider work.
func (s *MessageSearchReadModel) FilterFollowedSearchResults(ctx context.Context, actorID string, results []MessageSearchResult) ([]MessageSearchResult, error) {
	filtered := make([]MessageSearchResult, 0, len(results))
	for _, result := range results {
		following, err := s.follows.IsFollowingThread(ctx, result.Kind, actorID, result.Event.GetMessagePosted().GetRoomId(), searchResultRoot(result))
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
// Totals and thread-date order require visiting all matching groups. Work has one
// deadline, including stale-hit walks. Cursors do not pin an index snapshot.
func (s *MessageSearchReadModel) SearchThreadGroups(ctx context.Context, actorID string, limit, offset int, scope *MessageSearchScope, normalized *searchv1.QueryRequest, followedOnly bool, threadOrder ThreadOrder, query ThreadSearchQuery) (*ThreadSearchPage, error) {
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
		allowed, err := s.authorization.CanReadThreadMessages(ctx, actorID, match.Kind, room, root)
		if err != nil {
			return nil, err
		}
		member, err := s.memberships.RoomMembershipExists(ctx, match.Kind, actorID, room)
		if err != nil {
			return nil, err
		}
		if !allowed || !member {
			continue
		}
		following, err := s.follows.IsFollowingThread(ctx, match.Kind, actorID, room, root)
		if err != nil {
			return nil, err
		}
		if followedOnly && !following {
			continue
		}
		metadata := s.rooms.threadMetadata(root)
		var rootCreatedAt *time.Time
		if threadOrder == ThreadOrderCreated {
			rootCreatedAt = s.follows.rootCreatedAt(room, root)
			if rootCreatedAt == nil && metadata.LastReplyAt == nil {
				continue
			}
		} else if metadata.LastReplyAt == nil {
			// A thread without replies or a visible root is not a result.
			rootEvent, err := s.timeline.roomEvent(ctx, match.Kind, room, root)
			if err != nil {
				return nil, err
			}
			if rootEvent == nil {
				continue
			}
			if rootEvent != nil && rootEvent.GetCreatedAt() != nil {
				at := rootEvent.GetCreatedAt().AsTime()
				rootCreatedAt = &at
			}
		}
		row := newFollowedThread(match.Kind, room, root, metadata, rootCreatedAt)
		threads = append(threads, ThreadSearchResult{Metadata: row, Match: match, Following: following})
	}
	slices.SortFunc(threads, func(a, b ThreadSearchResult) int {
		if threadOrder != ThreadOrderDefault {
			return compareThreadDates(a.Metadata, b.Metadata, threadOrder)
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
		if err := s.follows.hydrateFollowedThreadViewerStates(ctx, actorID, []*FollowedThread{result.Metadata}); err != nil {
			return nil, err
		}
	}
	return &ThreadSearchPage{Threads: page, TotalCount: total, HasMore: end < total}, nil
}

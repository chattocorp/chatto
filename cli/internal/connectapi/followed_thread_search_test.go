package connectapi

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	searchv1 "hmans.de/chatto/internal/pb/chatto/search/v1"
)

func TestSearchFollowedThreadsRequiresFromFilterForAuthors(t *testing.T) {
	env := newConnectAPITestEnv(t)
	env.api.config.Search.Enabled = true
	ctx := withCaller(env.ctx, env.viewer)
	room := env.createJoinedRoom("name-search")
	alice, err := env.core.CreateUser(ctx, core.SystemActorID, "alice", "Wonderland Explorer", "password")
	require.NoError(t, err)
	_, err = env.core.JoinRoom(ctx, alice.Id, core.KindChannel, alice.Id, room.Id)
	require.NoError(t, err)
	root := env.post(room.Id, alice.Id, "Release planning", "")
	other := env.post(room.Id, env.viewer.Id, "Another conversation", "")
	reply := env.post(room.Id, alice.Id, "An older reply", other.Id)
	env.post(room.Id, env.viewer.Id, "Latest preview", other.Id)
	for _, id := range []string{root.Id, other.Id} {
		require.NoError(t, env.core.FollowThread(ctx, core.KindChannel, env.viewer.Id, room.Id, id))
	}
	hits := []*searchv1.QueryHit{
		{MessageId: root.Id, RoomId: room.Id, BodyEventId: currentMessageBodyEventID(t, env, room.Id, root.Id)},
		{MessageId: reply.Id, RoomId: room.Id, BodyEventId: currentMessageBodyEventID(t, env, room.Id, reply.Id)},
	}
	for _, tc := range []struct {
		query string
		count int
	}{
		{"ALIce", 0},
		{"explorer", 0},
		{`"WONDERLAND EXPLORER"`, 0},
		{"alice from:alice", 0},
		{"from:alice", 2},
		{"alice from:" + env.viewer.Login, 0},
		{"alice missing", 0},
		{"alice has:attachment after:2020-01-01", 0},
	} {
		t.Run(tc.query, func(t *testing.T) {
			env.api.searchProvider = &fakeMessageSearchProvider{query: func(req *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
				response := &searchv1.QueryResponse{ThreadScopeApplied: true}
				if len(req.RequiredTerms)+len(req.RequiredPhrases) != 0 {
					return response, nil // None of the message bodies contain the query.
				}
				require.Equal(t, []string{alice.Id}, req.AuthorIds)
				require.ElementsMatch(t, []string{root.Id, other.Id}, req.ThreadRootIds)
				response.Hits = hits
				return response, nil
			}}
			response, err := (&messageSearchService{api: env.api}).SearchMessages(ctx, connect.NewRequest(&apiv1.SearchMessagesRequest{Scope: apiv1.MessageSearchScope_MESSAGE_SEARCH_SCOPE_FOLLOWED_THREADS, GroupBy: apiv1.MessageSearchGroupBy_MESSAGE_SEARCH_GROUP_BY_THREAD, Query: tc.query}))
			require.NoError(t, err)
			require.Len(t, response.Msg.Results, tc.count)
		})
	}
}

func TestSearchFollowedThreadsFindsMatchesBeyondFirstCandidateBatch(t *testing.T) {
	env := newConnectAPITestEnv(t)
	env.api.config.Search.Enabled = true
	ctx := withCaller(env.ctx, env.viewer)
	room := env.createJoinedRoom("thread-search-batches")
	var oldest string
	for i := range 101 {
		root := env.post(room.Id, env.viewer.Id, fmt.Sprintf("root %d", i), "")
		require.NoError(t, env.core.FollowThread(ctx, core.KindChannel, env.viewer.Id, room.Id, root.Id))
		if i == 0 {
			oldest = root.Id
		}
	}
	body := currentMessageBodyEventID(t, env, room.Id, oldest)
	calls := 0
	env.api.searchProvider = &fakeMessageSearchProvider{query: func(req *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
		calls++
		require.Len(t, req.ThreadRootIds, 101)
		response := &searchv1.QueryResponse{ThreadScopeApplied: true}
		if slices.Contains(req.ThreadRootIds, oldest) {
			response.Hits = []*searchv1.QueryHit{{MessageId: oldest, RoomId: room.Id, BodyEventId: body}}
		}
		return response, nil
	}}
	response, err := (&messageSearchService{api: env.api}).SearchMessages(ctx, connect.NewRequest(&apiv1.SearchMessagesRequest{Scope: apiv1.MessageSearchScope_MESSAGE_SEARCH_SCOPE_FOLLOWED_THREADS, GroupBy: apiv1.MessageSearchGroupBy_MESSAGE_SEARCH_GROUP_BY_THREAD, Query: "root"}))
	require.NoError(t, err)
	require.Len(t, response.Msg.Results, 1)
	require.Equal(t, oldest, response.Msg.Results[0].ThreadContext.Thread.ThreadRootEventId)
	require.EqualValues(t, 1, response.Msg.GetThreadTotalCount())
	require.Equal(t, 1, calls)
	response, err = (&messageSearchService{api: env.api}).SearchMessages(ctx, connect.NewRequest(&apiv1.SearchMessagesRequest{
		Scope: apiv1.MessageSearchScope_MESSAGE_SEARCH_SCOPE_FOLLOWED_THREADS, Query: "root",
	}))
	require.NoError(t, err)
	require.Len(t, response.Msg.Results, 1)
	require.Equal(t, oldest, response.Msg.Results[0].Message.Id)
	require.Nil(t, response.Msg.ThreadTotalCount)
}

func TestThreadSearchIndependentScopeGroupingAndOrder(t *testing.T) {
	env := newConnectAPITestEnv(t)
	env.api.config.Search.Enabled = true
	ctx := withCaller(env.ctx, env.viewer)
	room := env.createJoinedRoom("independent-search")
	first := env.post(room.Id, env.viewer.Id, "needle first", "")
	second := env.post(room.Id, env.viewer.Id, "needle second", "")
	match := env.post(room.Id, env.viewer.Id, "needle reply", first.Id)
	latest := env.post(room.Id, env.viewer.Id, "not a match", second.Id)
	require.NoError(t, env.core.UnfollowThread(ctx, core.KindChannel, env.viewer.Id, room.Id, second.Id))
	hit := func(id string, score float64) *searchv1.QueryHit {
		return &searchv1.QueryHit{MessageId: id, RoomId: room.Id, BodyEventId: currentMessageBodyEventID(t, env, room.Id, id), RelevanceScore: score}
	}
	firstHit, secondHit := hit(match.Id, 2), hit(second.Id, 10)
	env.api.searchProvider = &fakeMessageSearchProvider{query: func(req *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
		hits := []*searchv1.QueryHit{secondHit, firstHit}
		if req.Order == searchv1.SearchOrder_SEARCH_ORDER_NEWEST {
			hits = []*searchv1.QueryHit{firstHit, secondHit}
		}
		if len(req.ThreadRootIds) > 0 {
			require.Equal(t, []string{first.Id}, req.ThreadRootIds)
			hits = []*searchv1.QueryHit{firstHit}
		}
		return &searchv1.QueryResponse{ThreadScopeApplied: true, Hits: hits}, nil
	}}
	for _, scope := range []apiv1.MessageSearchScope{0, apiv1.MessageSearchScope_MESSAGE_SEARCH_SCOPE_FOLLOWED_THREADS} {
		for _, grouping := range []apiv1.MessageSearchGroupBy{0, apiv1.MessageSearchGroupBy_MESSAGE_SEARCH_GROUP_BY_THREAD} {
			for _, order := range []apiv1.MessageSearchOrder{0, apiv1.MessageSearchOrder_MESSAGE_SEARCH_ORDER_RELEVANCE, apiv1.MessageSearchOrder_MESSAGE_SEARCH_ORDER_NEWEST, apiv1.MessageSearchOrder_MESSAGE_SEARCH_ORDER_THREAD_ACTIVITY} {
				t.Run(fmt.Sprintf("scope=%v/group=%v/order=%v", scope, grouping, order), func(t *testing.T) {
					response, err := (&messageSearchService{api: env.api}).SearchMessages(ctx, connect.NewRequest(&apiv1.SearchMessagesRequest{Query: "needle", Scope: scope, GroupBy: grouping, Order: order}))
					if grouping == 0 && order == apiv1.MessageSearchOrder_MESSAGE_SEARCH_ORDER_THREAD_ACTIVITY {
						require.Equal(t, connect.CodeInvalidArgument, errorCode(err))
						return
					}
					require.NoError(t, err)
					count := 2
					if scope != 0 {
						count = 1
					}
					if grouping == 0 {
						require.Len(t, response.Msg.Results, count)
						for _, result := range response.Msg.Results {
							require.Nil(t, result.ThreadContext)
						}
						return
					}
					require.Len(t, response.Msg.Results, count)
					require.EqualValues(t, count, response.Msg.GetThreadTotalCount())
					expected := second.Id
					if scope != 0 || order == apiv1.MessageSearchOrder_MESSAGE_SEARCH_ORDER_NEWEST {
						expected = first.Id
					}
					require.Equal(t, expected, response.Msg.Results[0].ThreadContext.Thread.ThreadRootEventId)
					for _, result := range response.Msg.Results {
						if result.ThreadContext.Thread.ThreadRootEventId == first.Id {
							require.Equal(t, match.Id, result.Message.Id)
							require.True(t, result.ThreadContext.Thread.ViewerState.GetIsFollowing())
						} else {
							require.Equal(t, second.Id, result.Message.Id)
							require.Equal(t, latest.Id, result.ThreadContext.LatestReply.Id)
							require.False(t, result.ThreadContext.Thread.ViewerState.GetIsFollowing())
						}
					}
				})
			}
		}
	}
}

func TestThreadSearchExcludesResolvedGroupsAndRejectsUnsupportedProviders(t *testing.T) {
	for _, supportsExclusions := range []bool{true, false} {
		t.Run(fmt.Sprint(supportsExclusions), func(t *testing.T) {
			env := newConnectAPITestEnv(t)
			env.api.config.Search.Enabled = true
			ctx := withCaller(env.ctx, env.viewer)
			room := env.createJoinedRoom("group-exclusions")
			root := env.post(room.Id, env.viewer.Id, "needle singleton", "")
			body := currentMessageBodyEventID(t, env, room.Id, root.Id)
			calls := 0
			env.api.searchProvider = &fakeMessageSearchProvider{query: func(req *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
				calls++
				require.LessOrEqual(t, calls, 2, "resolved threads must not require a full reply scan")
				response := &searchv1.QueryResponse{ThreadExclusionsApplied: supportsExclusions}
				if calls == 1 {
					for range 100 {
						response.Hits = append(response.Hits, &searchv1.QueryHit{MessageId: root.Id, RoomId: room.Id, BodyEventId: body})
					}
					response.NextCursor = []byte("thousands-of-other-matches-in-this-thread")
				} else {
					require.Empty(t, req.Cursor)
					require.Equal(t, []string{root.Id}, req.ExcludedThreadRootIds)
				}
				return response, nil
			}}
			response, err := (&messageSearchService{api: env.api}).SearchMessages(ctx, connect.NewRequest(&apiv1.SearchMessagesRequest{Query: "needle", GroupBy: apiv1.MessageSearchGroupBy_MESSAGE_SEARCH_GROUP_BY_THREAD}))
			if !supportsExclusions {
				require.Equal(t, connect.CodeUnavailable, errorCode(err))
				return
			}
			require.NoError(t, err)
			require.Len(t, response.Msg.Results, 1)
			require.Zero(t, response.Msg.Results[0].ThreadContext.Thread.ReplyCount)
			require.Equal(t, root.Id, response.Msg.Results[0].Message.Id)
		})
	}
}

func TestThreadSearchRechecksAcceptedBodyAfterEnumeration(t *testing.T) {
	env := newConnectAPITestEnv(t)
	env.api.config.Search.Enabled = true
	ctx := withCaller(env.ctx, env.viewer)
	room := env.createJoinedRoom("group-revision")
	root := env.post(room.Id, env.viewer.Id, "needle", "")
	body := currentMessageBodyEventID(t, env, room.Id, root.Id)
	calls := 0
	env.api.searchProvider = &fakeMessageSearchProvider{query: func(req *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
		calls++
		if calls == 1 {
			return &searchv1.QueryResponse{Hits: []*searchv1.QueryHit{{MessageId: root.Id, RoomId: room.Id, BodyEventId: body}}, NextCursor: []byte("more")}, nil
		}
		require.Equal(t, 2, calls)
		require.NoError(t, env.core.EditMessage(ctx, env.viewer.Id, core.KindChannel, room.Id, root.Id, "changed"))
		return &searchv1.QueryResponse{ThreadExclusionsApplied: true}, nil
	}}
	response, err := (&messageSearchService{api: env.api}).SearchMessages(ctx, connect.NewRequest(&apiv1.SearchMessagesRequest{Query: "needle", GroupBy: apiv1.MessageSearchGroupBy_MESSAGE_SEARCH_GROUP_BY_THREAD}))
	require.NoError(t, err)
	require.Empty(t, response.Msg.Results)
	require.Zero(t, response.Msg.GetThreadTotalCount())
}

func TestSearchFollowedThreadsRejectsRoomAccessLostDuringSearch(t *testing.T) {
	env := newConnectAPITestEnv(t)
	env.api.config.Search.Enabled = true
	ctx := withCaller(env.ctx, env.viewer)
	room := env.createJoinedRoom("thread-search-access")
	root := env.post(room.Id, env.viewer.Id, "needle", "")
	require.NoError(t, env.core.FollowThread(ctx, core.KindChannel, env.viewer.Id, room.Id, root.Id))
	body := currentMessageBodyEventID(t, env, room.Id, root.Id)
	provider := &fakeMessageSearchProvider{query: func(req *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
		require.NoError(t, env.core.LeaveRoom(ctx, env.viewer.Id, core.KindChannel, env.viewer.Id, room.Id))
		return &searchv1.QueryResponse{ThreadScopeApplied: true, Hits: []*searchv1.QueryHit{{MessageId: root.Id, RoomId: room.Id, BodyEventId: body}}}, nil
	}}
	env.api.searchProvider = provider
	for range 2 {
		response, err := (&messageSearchService{api: env.api}).SearchMessages(ctx, connect.NewRequest(&apiv1.SearchMessagesRequest{Scope: apiv1.MessageSearchScope_MESSAGE_SEARCH_SCOPE_FOLLOWED_THREADS, GroupBy: apiv1.MessageSearchGroupBy_MESSAGE_SEARCH_GROUP_BY_THREAD, Query: "needle"}))
		require.NoError(t, err)
		require.Empty(t, response.Msg.Results)
	}
	require.Len(t, provider.capturedQueries(), 1, "lost rooms must be excluded before subsequent provider queries")
}

func TestSearchFollowedThreadsScopesDeduplicatesAndPagesByActivity(t *testing.T) {
	env := newConnectAPITestEnv(t)
	env.api.config.Search.Enabled = true
	ctx := withCaller(env.ctx, env.viewer)
	room := env.createJoinedRoom("thread-search")
	oldRoot := env.post(room.Id, env.viewer.Id, "old root", "")
	oldReply := env.post(room.Id, env.viewer.Id, "needle buried reply", oldRoot.Id)
	newRoot := env.post(room.Id, env.viewer.Id, "new root", "")
	newReply := env.post(room.Id, env.viewer.Id, "needle second thread", newRoot.Id)
	// Latest activity is unrelated to the matching reply's creation time.
	env.post(room.Id, env.viewer.Id, "latest nonmatching reply", oldRoot.Id)
	unfollowed := env.post(room.Id, env.viewer.Id, "needle unfollowed", "")
	require.NoError(t, env.core.UnfollowThread(ctx, core.KindChannel, env.viewer.Id, room.Id, unfollowed.Id))
	hit := func(id string) *searchv1.QueryHit {
		return &searchv1.QueryHit{MessageId: id, RoomId: room.Id, BodyEventId: currentMessageBodyEventID(t, env, room.Id, id)}
	}
	oldHit, newHit, excludedHit := hit(oldReply.Id), hit(newReply.Id), hit(unfollowed.Id)
	provider := &fakeMessageSearchProvider{query: func(req *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
		require.ElementsMatch(t, []string{oldRoot.Id, newRoot.Id}, req.ThreadRootIds)
		require.Equal(t, []string{"needle"}, req.RequiredTerms)
		// Out-of-scope and duplicate hits must not become rows even if a provider
		// sends them. Message ordering must not determine thread ordering.
		return &searchv1.QueryResponse{ThreadScopeApplied: true, Hits: []*searchv1.QueryHit{newHit, oldHit, oldHit, excludedHit}}, nil
	}}
	env.api.searchProvider = provider
	cursor := ""
	for offset, expected := range []string{oldRoot.Id, newRoot.Id} {
		request := &apiv1.SearchMessagesRequest{Scope: apiv1.MessageSearchScope_MESSAGE_SEARCH_SCOPE_FOLLOWED_THREADS, GroupBy: apiv1.MessageSearchGroupBy_MESSAGE_SEARCH_GROUP_BY_THREAD,
			Query: "needle", PageSize: 1, Cursor: cursor, Order: apiv1.MessageSearchOrder_MESSAGE_SEARCH_ORDER_THREAD_ACTIVITY,
		}
		response, err := (&messageSearchService{api: env.api}).SearchMessages(ctx, connect.NewRequest(request))
		require.NoError(t, err)
		require.Len(t, response.Msg.Results, 1)
		require.Equal(t, expected, response.Msg.Results[0].ThreadContext.Thread.ThreadRootEventId)
		require.EqualValues(t, 2, response.Msg.GetThreadTotalCount())
		require.Equal(t, offset == 0, response.Msg.NextCursor != "")
		require.NotNil(t, response.Msg.Results[0].ThreadContext)
		cursor = response.Msg.NextCursor
		if offset == 0 {
			for _, mutate := range []func(*apiv1.SearchMessagesRequest){
				func(r *apiv1.SearchMessagesRequest) { r.Query = "changed" },
				func(r *apiv1.SearchMessagesRequest) { r.PageSize = 2 },
				func(r *apiv1.SearchMessagesRequest) { r.RoomId = proto.String(room.Id) },
				func(r *apiv1.SearchMessagesRequest) { r.AuthorId = proto.String(env.viewer.Id) },
				func(r *apiv1.SearchMessagesRequest) { r.HasAttachments = true },
				func(r *apiv1.SearchMessagesRequest) { r.Scope = 0; r.GroupBy = 0 },
			} {
				changed := proto.Clone(request).(*apiv1.SearchMessagesRequest)
				changed.Cursor = cursor
				mutate(changed)
				_, err := (&messageSearchService{api: env.api}).SearchMessages(ctx, connect.NewRequest(changed))
				require.Equal(t, connect.CodeInvalidArgument, errorCode(err))
			}
		}
	}
}

func TestThreadSearchUsesSharedStructuredFiltersAndValidation(t *testing.T) {
	env := newConnectAPITestEnv(t)
	env.api.config.Search.Enabled = true
	ctx := withCaller(env.ctx, env.viewer)
	room := env.createJoinedRoom("unified-search")
	root := env.post(room.Id, env.viewer.Id, "needle", "")
	require.NoError(t, env.core.FollowThread(ctx, core.KindChannel, env.viewer.Id, room.Id, root.Id))
	after := timestamppb.New(time.Date(2021, 1, 1, 0, 0, 0, 0, time.UTC))
	before := timestamppb.New(time.Date(2029, 1, 1, 0, 0, 0, 0, time.UTC))
	provider := &fakeMessageSearchProvider{query: func(req *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
		require.Equal(t, []string{room.Id}, req.RoomIds)
		require.Equal(t, []string{env.viewer.Id}, req.AuthorIds)
		require.Equal(t, after.AsTime(), req.CreatedAfter.AsTime())
		require.Equal(t, before.AsTime(), req.CreatedBefore.AsTime())
		require.True(t, req.HasAttachments)
		return &searchv1.QueryResponse{ThreadScopeApplied: true}, nil
	}}
	env.api.searchProvider = provider
	request := &apiv1.SearchMessagesRequest{
		Query:   "needle after:2020-01-01 before:2030-01-01 from:" + env.viewer.Login,
		Scope:   apiv1.MessageSearchScope_MESSAGE_SEARCH_SCOPE_FOLLOWED_THREADS,
		GroupBy: apiv1.MessageSearchGroupBy_MESSAGE_SEARCH_GROUP_BY_THREAD,
		Order:   apiv1.MessageSearchOrder_MESSAGE_SEARCH_ORDER_THREAD_ACTIVITY,
		RoomId:  proto.String(room.Id), AuthorId: proto.String(env.viewer.Id),
		CreatedAfter: after, CreatedBefore: before, HasAttachments: true,
	}
	service := &messageSearchService{api: env.api}
	response, err := service.SearchMessages(ctx, connect.NewRequest(request))
	require.NoError(t, err)
	require.NotNil(t, response.Msg.ThreadTotalCount)
	require.Empty(t, response.Msg.Results)
	for _, mutate := range []func(*apiv1.SearchMessagesRequest){
		func(r *apiv1.SearchMessagesRequest) { r.GroupBy = 0 },
		func(r *apiv1.SearchMessagesRequest) { r.Scope = 99 },
		func(r *apiv1.SearchMessagesRequest) { r.GroupBy = 99 },
		func(r *apiv1.SearchMessagesRequest) { r.PageSize = 101 },
		func(r *apiv1.SearchMessagesRequest) { r.CreatedBefore = after },
	} {
		invalid := proto.Clone(request).(*apiv1.SearchMessagesRequest)
		mutate(invalid)
		_, err := service.SearchMessages(ctx, connect.NewRequest(invalid))
		require.Equal(t, connect.CodeInvalidArgument, errorCode(err))
	}
	request.Query = "from:nonexistent-user"
	response, err = service.SearchMessages(ctx, connect.NewRequest(request))
	require.NoError(t, err)
	require.NotNil(t, response.Msg.ThreadTotalCount)
	require.Zero(t, response.Msg.GetThreadTotalCount())
	require.Len(t, provider.capturedQueries(), 1)
}

func TestSearchFollowedThreadsSkipsStalePagesAndStopsSearchingMatchedThreads(t *testing.T) {
	env := newConnectAPITestEnv(t)
	env.api.config.Search.Enabled = true
	ctx := withCaller(env.ctx, env.viewer)
	room := env.createJoinedRoom("thread-search-stale")
	first := env.post(room.Id, env.viewer.Id, "needle first", "")
	second := env.post(room.Id, env.viewer.Id, "needle second", "")
	require.NoError(t, env.core.FollowThread(ctx, core.KindChannel, env.viewer.Id, room.Id, first.Id))
	require.NoError(t, env.core.FollowThread(ctx, core.KindChannel, env.viewer.Id, room.Id, second.Id))
	firstBody := currentMessageBodyEventID(t, env, room.Id, first.Id)
	secondBody := currentMessageBodyEventID(t, env, room.Id, second.Id)
	calls := 0
	env.api.searchProvider = &fakeMessageSearchProvider{query: func(req *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
		calls++
		response := &searchv1.QueryResponse{ThreadScopeApplied: true, ThreadExclusionsApplied: true}
		switch calls {
		case 1:
			response.Hits = []*searchv1.QueryHit{{MessageId: first.Id, RoomId: room.Id, BodyEventId: "old-body"}}
			response.NextCursor = []byte("next")
		case 2:
			require.Equal(t, []byte("next"), req.Cursor)
			response.Hits = []*searchv1.QueryHit{{MessageId: first.Id, RoomId: room.Id, BodyEventId: firstBody}}
			response.NextCursor = []byte("more")
		case 3:
			require.Empty(t, req.Cursor)
			require.ElementsMatch(t, []string{first.Id, second.Id}, req.ThreadRootIds)
			require.Equal(t, []string{first.Id}, req.ExcludedThreadRootIds)
			response.Hits = []*searchv1.QueryHit{{MessageId: second.Id, RoomId: room.Id, BodyEventId: secondBody}}
		default:
			t.Fatal("unbounded query loop")
		}
		return response, nil
	}}
	response, err := (&messageSearchService{api: env.api}).SearchMessages(ctx, connect.NewRequest(&apiv1.SearchMessagesRequest{Scope: apiv1.MessageSearchScope_MESSAGE_SEARCH_SCOPE_FOLLOWED_THREADS, GroupBy: apiv1.MessageSearchGroupBy_MESSAGE_SEARCH_GROUP_BY_THREAD, Query: "needle"}))
	require.NoError(t, err)
	require.Len(t, response.Msg.Results, 2)
	require.Equal(t, 3, calls)
}

func TestSearchFollowedThreadsIncludesDMAndRechecksUnfollow(t *testing.T) {
	env := newConnectAPITestEnv(t)
	env.api.config.Search.Enabled = true
	ctx := withCaller(env.ctx, env.viewer)
	participant, err := env.core.CreateUser(ctx, core.SystemActorID, "thread-search-dm", "Participant", "password")
	require.NoError(t, err)
	dm, _, err := env.core.FindOrCreateDM(ctx, env.viewer.Id, []string{participant.Id})
	require.NoError(t, err)
	root, err := env.core.PostMessage(ctx, core.KindDM, dm.Id, env.viewer.Id, "needle", nil, "", "", nil, false)
	require.NoError(t, err)
	body := currentMessageBodyEventID(t, env, dm.Id, root.Id)
	require.NoError(t, env.core.FollowThread(ctx, core.KindDM, env.viewer.Id, dm.Id, root.Id))
	unfollow := false
	provider := &fakeMessageSearchProvider{query: func(req *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
		require.True(t, slices.Contains(req.ThreadRootIds, root.Id))
		if unfollow {
			require.NoError(t, env.core.UnfollowThread(ctx, core.KindDM, env.viewer.Id, dm.Id, root.Id))
		}
		return &searchv1.QueryResponse{ThreadScopeApplied: true, Hits: []*searchv1.QueryHit{{MessageId: root.Id, RoomId: dm.Id, BodyEventId: body}}}, nil
	}}
	env.api.searchProvider = provider
	request := &apiv1.SearchMessagesRequest{Scope: apiv1.MessageSearchScope_MESSAGE_SEARCH_SCOPE_FOLLOWED_THREADS, GroupBy: apiv1.MessageSearchGroupBy_MESSAGE_SEARCH_GROUP_BY_THREAD, Query: "needle"}
	response, err := (&messageSearchService{api: env.api}).SearchMessages(ctx, connect.NewRequest(request))
	require.NoError(t, err)
	require.Len(t, response.Msg.Results, 1)
	unfollow = true
	response, err = (&messageSearchService{api: env.api}).SearchMessages(ctx, connect.NewRequest(request))
	require.NoError(t, err)
	require.Empty(t, response.Msg.Results)
}

func TestSearchFollowedThreadsAvailabilityAndAuthorFilter(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	request := connect.NewRequest(&apiv1.SearchMessagesRequest{Scope: apiv1.MessageSearchScope_MESSAGE_SEARCH_SCOPE_FOLLOWED_THREADS, GroupBy: apiv1.MessageSearchGroupBy_MESSAGE_SEARCH_GROUP_BY_THREAD, Query: "needle"})
	_, err := (&messageSearchService{api: env.api}).SearchMessages(env.ctx, request)
	require.Equal(t, connect.CodeUnauthenticated, errorCode(err))
	_, err = (&messageSearchService{api: env.api}).SearchMessages(ctx, request)
	require.Equal(t, connect.CodeFailedPrecondition, errorCode(err))
	env.api.config.Search.Enabled = true
	_, err = (&messageSearchService{api: env.api}).SearchMessages(ctx, request)
	require.Equal(t, connect.CodeUnavailable, errorCode(err))
	room := env.createJoinedRoom("thread-search-author")
	root := env.post(room.Id, env.viewer.Id, "needle", "")
	require.NoError(t, env.core.FollowThread(ctx, core.KindChannel, env.viewer.Id, room.Id, root.Id))
	provider := &fakeMessageSearchProvider{}
	env.api.searchProvider = provider
	_, err = (&messageSearchService{api: env.api}).SearchMessages(ctx, request)
	require.Equal(t, connect.CodeUnavailable, errorCode(err), "older providers cannot silently ignore scope")
	provider.query = func(req *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
		require.Equal(t, []string{env.viewer.Id}, req.AuthorIds)
		require.Equal(t, []string{root.Id}, req.ThreadRootIds)
		return &searchv1.QueryResponse{ThreadScopeApplied: true}, nil
	}
	_, err = (&messageSearchService{api: env.api}).SearchMessages(ctx, connect.NewRequest(&apiv1.SearchMessagesRequest{Scope: apiv1.MessageSearchScope_MESSAGE_SEARCH_SCOPE_FOLLOWED_THREADS, GroupBy: apiv1.MessageSearchGroupBy_MESSAGE_SEARCH_GROUP_BY_THREAD, Query: "from:" + env.viewer.Login}))
	require.NoError(t, err)
	_, err = (&messageSearchService{api: env.api}).SearchMessages(ctx, connect.NewRequest(&apiv1.SearchMessagesRequest{Scope: apiv1.MessageSearchScope_MESSAGE_SEARCH_SCOPE_FOLLOWED_THREADS, GroupBy: apiv1.MessageSearchGroupBy_MESSAGE_SEARCH_GROUP_BY_THREAD, Query: "from:"}))
	require.Equal(t, connect.CodeInvalidArgument, errorCode(err))
}

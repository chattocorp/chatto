package connectapi

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	searchv1 "hmans.de/chatto/internal/pb/chatto/search/v1"
	searchsvc "hmans.de/chatto/internal/search"
)

type fakeMessageSearchProvider struct {
	mu        sync.Mutex
	queries   []*searchv1.QueryRequest
	query     func(*searchv1.QueryRequest) (*searchv1.QueryResponse, error)
	status    *searchv1.GetStatusResponse
	statusErr error
}

func (p *fakeMessageSearchProvider) Query(_ context.Context, request *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
	p.mu.Lock()
	p.queries = append(p.queries, proto.Clone(request).(*searchv1.QueryRequest))
	query := p.query
	p.mu.Unlock()
	if query == nil {
		return &searchv1.QueryResponse{}, nil
	}
	return query(request)
}

func (p *fakeMessageSearchProvider) GetStatus(context.Context) (*searchv1.GetStatusResponse, error) {
	return p.status, p.statusErr
}

func (p *fakeMessageSearchProvider) capturedQueries() []*searchv1.QueryRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]*searchv1.QueryRequest(nil), p.queries...)
}

func TestMessageSearchStatusSeparatesConfigurationAndProviderReadiness(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	provider := &fakeMessageSearchProvider{status: &searchv1.GetStatusResponse{
		State: searchv1.ProviderState_PROVIDER_STATE_READY, IndexedEventCount: proto.Uint64(12),
		TargetEventCount: proto.Uint64(12), RetryAfter: durationpb.New(0),
	}}
	service := &messageSearchService{api: env.api}

	response, err := service.GetStatus(ctx, connect.NewRequest(&apiv1.GetStatusRequest{}))
	require.NoError(t, err)
	require.Equal(t, apiv1.MessageSearchState_MESSAGE_SEARCH_STATE_DISABLED, response.Msg.GetState())

	env.api.config.Search.Enabled = true
	response, err = service.GetStatus(ctx, connect.NewRequest(&apiv1.GetStatusRequest{}))
	require.NoError(t, err)
	require.Equal(t, apiv1.MessageSearchState_MESSAGE_SEARCH_STATE_UNAVAILABLE, response.Msg.GetState())

	env.api.searchProvider = provider
	response, err = service.GetStatus(ctx, connect.NewRequest(&apiv1.GetStatusRequest{}))
	require.NoError(t, err)
	require.Equal(t, apiv1.MessageSearchState_MESSAGE_SEARCH_STATE_READY, response.Msg.GetState())

	provider.statusErr = errors.New("provider offline")
	response, err = service.GetStatus(ctx, connect.NewRequest(&apiv1.GetStatusRequest{}))
	require.NoError(t, err)
	require.Equal(t, apiv1.MessageSearchState_MESSAGE_SEARCH_STATE_UNAVAILABLE, response.Msg.GetState())

	_, err = service.GetStatus(env.ctx, connect.NewRequest(&apiv1.GetStatusRequest{}))
	require.Equal(t, connect.CodeUnauthenticated, connect.CodeOf(err))
}

func TestPublicMessageSearchStatusPreservesProviderState(t *testing.T) {
	tests := []struct {
		name string
		in   searchv1.ProviderState
		want apiv1.MessageSearchState
	}{
		{"unspecified", searchv1.ProviderState_PROVIDER_STATE_UNSPECIFIED, apiv1.MessageSearchState_MESSAGE_SEARCH_STATE_UNAVAILABLE},
		{"starting", searchv1.ProviderState_PROVIDER_STATE_STARTING, apiv1.MessageSearchState_MESSAGE_SEARCH_STATE_STARTING},
		{"indexing", searchv1.ProviderState_PROVIDER_STATE_INDEXING, apiv1.MessageSearchState_MESSAGE_SEARCH_STATE_INDEXING},
		{"ready", searchv1.ProviderState_PROVIDER_STATE_READY, apiv1.MessageSearchState_MESSAGE_SEARCH_STATE_READY},
		{"degraded", searchv1.ProviderState_PROVIDER_STATE_DEGRADED, apiv1.MessageSearchState_MESSAGE_SEARCH_STATE_DEGRADED},
		{"unavailable", searchv1.ProviderState_PROVIDER_STATE_UNAVAILABLE, apiv1.MessageSearchState_MESSAGE_SEARCH_STATE_UNAVAILABLE},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status := publicMessageSearchStatus(&searchv1.GetStatusResponse{State: test.in})
			require.Equal(t, test.want, status.GetState())
		})
	}
}

func TestProviderSearchRequestIncludesCompleteAuthorizedRoomScope(t *testing.T) {
	roomIDs := make([]string, 1001)
	for index := range roomIDs {
		roomIDs[index] = fmt.Sprintf("room-%03d", index)
	}

	request, err := providerSearchRequest(
		&apiv1.SearchMessagesRequest{Query: "search"},
		searchsvc.ParsedQuery{RequiredTerms: []string{"search"}},
		&core.MessageSearchScope{RoomIDs: roomIDs},
	)
	require.NoError(t, err)
	require.Equal(t, roomIDs, request.GetRoomIds())
	require.Equal(t, uint32(50), request.GetPageSize())
	require.NoError(t, searchsvc.ValidateQueryRequest(request))
}

func TestMessageSearchAuthorizesHydratesAndSealsProviderCursor(t *testing.T) {
	env := newConnectAPITestEnv(t)
	env.api.config.Search.Enabled = true
	ctx := withCaller(env.ctx, env.viewer)
	visible, err := env.core.CreateRoom(ctx, core.SystemActorID, core.KindChannel, "", "search-api-room", "")
	require.NoError(t, err)
	_, err = env.core.JoinRoom(ctx, env.viewer.Id, core.KindChannel, env.viewer.Id, visible.Id)
	require.NoError(t, err)
	hidden, err := env.core.CreateRoom(ctx, core.SystemActorID, core.KindChannel, "", "search-api-hidden", "")
	require.NoError(t, err)
	message, err := env.core.PostMessage(ctx, core.KindChannel, visible.Id, env.viewer.Id, "current searchable body", nil, "", "", nil, false)
	require.NoError(t, err)
	messageBody, retracted, ok := env.core.RoomTimeline.LatestBody(message.Id)
	require.True(t, ok)
	require.False(t, retracted)
	stale, err := env.core.PostMessage(ctx, core.KindChannel, visible.Id, env.viewer.Id, "removed searchable body", nil, "", "", nil, false)
	require.NoError(t, err)
	require.NoError(t, env.core.DeleteMessage(ctx, env.viewer.Id, core.KindChannel, visible.Id, stale.Id))

	providerCursor := []byte("provider-page-two")
	provider := &fakeMessageSearchProvider{}
	provider.query = func(request *searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
		response := &searchv1.QueryResponse{Hits: []*searchv1.QueryHit{
			{MessageId: stale.Id, RoomId: visible.Id, BodyEventId: "stale-body"},
			{MessageId: "hidden-message", RoomId: hidden.Id, BodyEventId: "hidden-body"},
			{MessageId: message.Id, RoomId: visible.Id, BodyEventId: messageBody.GetBodyEventId()},
		}}
		if len(request.GetCursor()) == 0 {
			response.NextCursor = providerCursor
		}
		return response, nil
	}
	env.api.searchProvider = provider
	service := &messageSearchService{api: env.api}
	request := &apiv1.SearchMessagesRequest{
		Query:  `current AND "searchable body" in:search-api-room from:timeline-viewer after:2025-01-01 has:attachment`,
		RoomId: proto.String(visible.Id), AuthorId: proto.String(env.viewer.Id),
		Order: apiv1.MessageSearchOrder_MESSAGE_SEARCH_ORDER_NEWEST, PageSize: 5,
	}

	response, err := service.SearchMessages(ctx, connect.NewRequest(request))
	require.NoError(t, err)
	require.Len(t, response.Msg.GetMessages(), 1)
	require.Equal(t, message.Id, response.Msg.GetMessages()[0].GetId())
	require.Equal(t, "current searchable body", response.Msg.GetMessages()[0].GetBody())
	require.NotEmpty(t, response.Msg.GetNextCursor())
	require.NotContains(t, response.Msg.GetNextCursor(), string(providerCursor))

	queries := provider.capturedQueries()
	require.Len(t, queries, 1)
	require.Equal(t, []string{"current"}, queries[0].GetRequiredTerms())
	require.Equal(t, []string{"searchable body"}, queries[0].GetRequiredPhrases())
	require.Equal(t, []string{visible.Id}, queries[0].GetRoomIds())
	require.Equal(t, []string{env.viewer.Id}, queries[0].GetAuthorIds())
	require.True(t, queries[0].GetHasAttachments())
	require.Equal(t, searchv1.SearchOrder_SEARCH_ORDER_NEWEST, queries[0].GetOrder())

	secondRequest := proto.Clone(request).(*apiv1.SearchMessagesRequest)
	secondRequest.Cursor = response.Msg.GetNextCursor()
	second, err := service.SearchMessages(ctx, connect.NewRequest(secondRequest))
	require.NoError(t, err)
	require.Empty(t, second.Msg.GetNextCursor())
	queries = provider.capturedQueries()
	require.Len(t, queries, 2)
	require.Equal(t, providerCursor, queries[1].GetCursor())

	changedRequest := proto.Clone(secondRequest).(*apiv1.SearchMessagesRequest)
	changedRequest.Query = "different"
	_, err = service.SearchMessages(ctx, connect.NewRequest(changedRequest))
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	require.Len(t, provider.capturedQueries(), 2)

	otherViewer, err := env.core.CreateUser(ctx, core.SystemActorID, "search-other", "Search Other", "password")
	require.NoError(t, err)
	_, err = service.SearchMessages(withCaller(env.ctx, otherViewer), connect.NewRequest(secondRequest))
	require.Equal(t, connect.CodeInvalidArgument, connect.CodeOf(err))
	require.Len(t, provider.capturedQueries(), 2)
}

func TestMessageSearchMapsFeatureAndProviderFailures(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	service := &messageSearchService{api: env.api}
	request := connect.NewRequest(&apiv1.SearchMessagesRequest{Query: "search"})

	_, err := service.SearchMessages(ctx, request)
	require.Equal(t, connect.CodeFailedPrecondition, connect.CodeOf(err))

	env.api.config = config.ChattoConfig{Search: config.SearchConfig{Enabled: true}}
	_, err = service.SearchMessages(ctx, request)
	require.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
	room, err := env.core.CreateRoom(ctx, core.SystemActorID, core.KindChannel, "", "search-failure-room", "")
	require.NoError(t, err)
	_, err = env.core.JoinRoom(ctx, env.viewer.Id, core.KindChannel, env.viewer.Id, room.Id)
	require.NoError(t, err)

	provider := &fakeMessageSearchProvider{query: func(*searchv1.QueryRequest) (*searchv1.QueryResponse, error) {
		return nil, searchsvc.ErrUnavailable
	}}
	env.api.searchProvider = provider
	_, err = service.SearchMessages(ctx, request)
	require.Equal(t, connect.CodeUnavailable, connect.CodeOf(err))
}

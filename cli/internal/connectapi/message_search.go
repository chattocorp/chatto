package connectapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	searchv1 "hmans.de/chatto/internal/pb/chatto/search/v1"
	searchsvc "hmans.de/chatto/internal/search"
)

const (
	messageSearchDefaultPageSize = 50
	messageSearchCursorPrefix    = "search:"
	messageSearchCursorPurpose   = "message-search-v1"
)

type messageSearchService struct {
	api *API
}

func (s *messageSearchService) GetStatus(ctx context.Context, _ *connect.Request[apiv1.GetStatusRequest]) (*connect.Response[apiv1.GetStatusResponse], error) {
	if _, err := requireCaller(ctx); err != nil {
		return nil, err
	}
	if !s.api.config.Search.Enabled {
		return connect.NewResponse(&apiv1.GetStatusResponse{State: apiv1.MessageSearchState_MESSAGE_SEARCH_STATE_DISABLED}), nil
	}
	if s.api.searchProvider == nil {
		return connect.NewResponse(&apiv1.GetStatusResponse{State: apiv1.MessageSearchState_MESSAGE_SEARCH_STATE_UNAVAILABLE}), nil
	}
	status, err := s.api.searchProvider.GetStatus(ctx)
	if err != nil || status == nil {
		return connect.NewResponse(&apiv1.GetStatusResponse{State: apiv1.MessageSearchState_MESSAGE_SEARCH_STATE_UNAVAILABLE}), nil
	}
	return connect.NewResponse(publicMessageSearchStatus(status)), nil
}

func (s *messageSearchService) SearchMessages(ctx context.Context, req *connect.Request[apiv1.SearchMessagesRequest]) (*connect.Response[apiv1.SearchMessagesResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if !s.api.config.Search.Enabled {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("message search is disabled"))
	}
	if s.api.searchProvider == nil || s.api.core == nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("message search provider unavailable"))
	}
	parsed, err := searchsvc.ParseQuery(req.Msg.GetQuery())
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	var providerCursor []byte
	if req.Msg.GetCursor() != "" {
		providerCursor, err = s.api.openMessageSearchCursor(caller.UserID, req.Msg)
		if err != nil {
			return nil, err
		}
	}

	scope, err := s.api.core.MessageSearchReads().ResolveScope(ctx, core.MessageSearchScopeInput{
		ActorID: caller.UserID, RoomID: req.Msg.GetRoomId(), RoomSelectors: parsed.RoomSelectors,
		AuthorID: req.Msg.GetAuthorId(), AuthorSelectors: parsed.AuthorSelectors,
	})
	if err != nil {
		return nil, connectError(err)
	}
	if scope.NoMatches || len(scope.RoomIDs) == 0 {
		return connect.NewResponse(&apiv1.SearchMessagesResponse{}), nil
	}

	providerRequest, err := providerSearchRequest(req.Msg, parsed, scope)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	providerRequest.Cursor = providerCursor
	if err := searchsvc.ValidateQueryRequest(providerRequest); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	providerResponse, err := s.api.searchProvider.Query(ctx, providerRequest)
	if err != nil {
		return nil, messageSearchProviderError(err)
	}

	hits := make([]core.MessageSearchHit, 0, len(providerResponse.GetHits()))
	for _, hit := range providerResponse.GetHits() {
		if hit != nil {
			hits = append(hits, core.MessageSearchHit{MessageID: hit.GetMessageId(), RoomID: hit.GetRoomId(), BodyEventID: hit.GetBodyEventId()})
		}
	}
	current, err := s.api.core.MessageSearchReads().HydrateHits(ctx, caller.UserID, scope, hits)
	if err != nil {
		return nil, connectError(err)
	}
	messages, err := s.api.hydrateMessageSearchResults(ctx, caller.UserID, current)
	if err != nil {
		return nil, connectError(err)
	}

	response := &apiv1.SearchMessagesResponse{Messages: messages}
	if len(providerResponse.GetNextCursor()) > 0 {
		response.NextCursor, err = s.api.sealMessageSearchCursor(caller.UserID, req.Msg, providerResponse.GetNextCursor())
		if err != nil {
			return nil, connectInternalError(err)
		}
	}
	return connect.NewResponse(response), nil
}

func providerSearchRequest(request *apiv1.SearchMessagesRequest, parsed searchsvc.ParsedQuery, scope *core.MessageSearchScope) (*searchv1.QueryRequest, error) {
	if request.GetPageSize() > 100 {
		return nil, fmt.Errorf("page_size must not exceed 100")
	}
	if timestamp := request.GetCreatedAfter(); timestamp != nil {
		if err := timestamp.CheckValid(); err != nil {
			return nil, fmt.Errorf("created_after is invalid: %w", err)
		}
	}
	if timestamp := request.GetCreatedBefore(); timestamp != nil {
		if err := timestamp.CheckValid(); err != nil {
			return nil, fmt.Errorf("created_before is invalid: %w", err)
		}
	}
	createdAfter := stricterAfter(timestampTime(request.GetCreatedAfter()), parsed.CreatedAfter)
	createdBefore := stricterBefore(timestampTime(request.GetCreatedBefore()), parsed.CreatedBefore)
	if createdAfter != nil && createdBefore != nil && !createdAfter.Before(*createdBefore) {
		return nil, fmt.Errorf("created_after must precede created_before")
	}
	pageSize := request.GetPageSize()
	if pageSize == 0 {
		pageSize = messageSearchDefaultPageSize
	}
	order := searchv1.SearchOrder_SEARCH_ORDER_RELEVANCE
	switch request.GetOrder() {
	case apiv1.MessageSearchOrder_MESSAGE_SEARCH_ORDER_UNSPECIFIED, apiv1.MessageSearchOrder_MESSAGE_SEARCH_ORDER_RELEVANCE:
	case apiv1.MessageSearchOrder_MESSAGE_SEARCH_ORDER_NEWEST:
		order = searchv1.SearchOrder_SEARCH_ORDER_NEWEST
	default:
		return nil, fmt.Errorf("unsupported message search order")
	}
	provider := &searchv1.QueryRequest{
		RequiredTerms:   append([]string(nil), parsed.RequiredTerms...),
		RequiredPhrases: append([]string(nil), parsed.RequiredPhrases...),
		RoomIds:         append([]string(nil), scope.RoomIDs...),
		AuthorIds:       append([]string(nil), scope.AuthorIDs...),
		HasAttachments:  request.GetHasAttachments() || parsed.HasAttachments,
		Order:           order,
		PageSize:        pageSize,
	}
	if createdAfter != nil {
		provider.CreatedAfter = timestamppb.New(*createdAfter)
	}
	if createdBefore != nil {
		provider.CreatedBefore = timestamppb.New(*createdBefore)
	}
	return provider, nil
}

func publicMessageSearchStatus(status *searchv1.GetStatusResponse) *apiv1.GetStatusResponse {
	response := &apiv1.GetStatusResponse{
		RetryAfter: status.GetRetryAfter(),
	}
	switch status.GetState() {
	case searchv1.ProviderState_PROVIDER_STATE_STARTING:
		response.State = apiv1.MessageSearchState_MESSAGE_SEARCH_STATE_STARTING
	case searchv1.ProviderState_PROVIDER_STATE_INDEXING:
		response.State = apiv1.MessageSearchState_MESSAGE_SEARCH_STATE_INDEXING
	case searchv1.ProviderState_PROVIDER_STATE_READY:
		response.State = apiv1.MessageSearchState_MESSAGE_SEARCH_STATE_READY
	case searchv1.ProviderState_PROVIDER_STATE_DEGRADED:
		response.State = apiv1.MessageSearchState_MESSAGE_SEARCH_STATE_DEGRADED
	case searchv1.ProviderState_PROVIDER_STATE_UNAVAILABLE:
		response.State = apiv1.MessageSearchState_MESSAGE_SEARCH_STATE_UNAVAILABLE
	default:
		response.State = apiv1.MessageSearchState_MESSAGE_SEARCH_STATE_UNAVAILABLE
	}
	return response
}

func messageSearchProviderError(err error) error {
	if errors.Is(err, searchsvc.ErrUnavailable) || errors.Is(err, searchsvc.ErrProviderNotReady) {
		return connect.NewError(connect.CodeUnavailable, errors.New("message search provider unavailable"))
	}
	var serviceError *searchsvc.ServiceError
	if errors.As(err, &serviceError) && serviceError.Code == searchsvc.ErrorCodeInvalidArgument {
		return connect.NewError(connect.CodeInvalidArgument, errors.New("invalid message search cursor"))
	}
	return connectInternalError(fmt.Errorf("query message search provider: %w", err))
}

func (a *API) hydrateMessageSearchResults(ctx context.Context, viewerID string, results []core.MessageSearchResult) ([]*apiv1.Message, error) {
	byKind := make(map[core.RoomKind][]*core.RoomEvent)
	for _, result := range results {
		if result.Event != nil {
			byKind[result.Kind] = append(byKind[result.Kind], &core.RoomEvent{Event: result.Event})
		}
	}
	hydrated := make(map[string]*apiv1.Message, len(results))
	for kind, events := range byKind {
		apiEvents, _, err := newRoomTimelineAssembler(a).hydrateEvents(ctx, viewerID, kind, events)
		if err != nil {
			return nil, err
		}
		for _, event := range apiEvents {
			if message := messageFromTimelineEvent(event); message != nil {
				hydrated[messageSearchResultKey(message.GetRoomId(), message.GetId())] = message
			}
		}
	}
	messages := make([]*apiv1.Message, 0, len(results))
	for _, result := range results {
		posted := result.Event.GetMessagePosted()
		if posted == nil {
			continue
		}
		if message := hydrated[messageSearchResultKey(posted.GetRoomId(), result.Event.GetId())]; message != nil {
			messages = append(messages, message)
		}
	}
	return messages, nil
}

func (a *API) sealMessageSearchCursor(viewerID string, request *apiv1.SearchMessagesRequest, providerCursor []byte) (string, error) {
	scope, err := messageSearchCursorScope(viewerID, request)
	if err != nil {
		return "", err
	}
	token, err := a.core.SealPublicCursor(messageSearchCursorPurpose, scope, providerCursor)
	if err != nil {
		return "", fmt.Errorf("seal message search cursor: %w", err)
	}
	return messageSearchCursorPrefix + token, nil
}

func (a *API) openMessageSearchCursor(viewerID string, request *apiv1.SearchMessagesRequest) ([]byte, error) {
	encoded, ok := strings.CutPrefix(request.GetCursor(), messageSearchCursorPrefix)
	if !ok {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid message search cursor"))
	}
	scope, err := messageSearchCursorScope(viewerID, request)
	if err != nil {
		return nil, connectInternalError(err)
	}
	providerCursor, err := a.core.OpenPublicCursor(messageSearchCursorPurpose, scope, encoded)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid message search cursor"))
	}
	return providerCursor, nil
}

func messageSearchCursorScope(viewerID string, request *apiv1.SearchMessagesRequest) (string, error) {
	clone := proto.Clone(request).(*apiv1.SearchMessagesRequest)
	clone.Cursor = ""
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(clone)
	if err != nil {
		return "", fmt.Errorf("marshal message search cursor scope: %w", err)
	}
	hash := sha256.Sum256(data)
	return viewerID + "\x00" + hex.EncodeToString(hash[:]), nil
}

func timestampTime(timestamp *timestamppb.Timestamp) *time.Time {
	if timestamp == nil {
		return nil
	}
	value := timestamp.AsTime()
	return &value
}

func stricterAfter(left, right *time.Time) *time.Time {
	if left == nil {
		return right
	}
	if right == nil || left.After(*right) {
		return left
	}
	return right
}

func stricterBefore(left, right *time.Time) *time.Time {
	if left == nil {
		return right
	}
	if right == nil || left.Before(*right) {
		return left
	}
	return right
}

func messageSearchResultKey(roomID, messageID string) string {
	return roomID + "\x00" + messageID
}

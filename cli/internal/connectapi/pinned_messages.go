package connectapi

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	defaultPinnedMessageListLimit = 50
	maxPinnedMessageListLimit     = 100
)

func (s *roomService) ListPinnedMessages(ctx context.Context, req *connect.Request[apiv1.ListPinnedMessagesRequest]) (*connect.Response[apiv1.ListPinnedMessagesResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := apiPagination(req.Msg.GetPage(), defaultPinnedMessageListLimit, maxPinnedMessageListLimit)
	result, err := s.api.core.RoomTimelineReads().ListPinnedMessages(ctx, core.PinnedMessageListInput{ActorID: caller.UserID, RoomID: req.Msg.GetRoomId(), Limit: limit, Offset: offset})
	if err != nil {
		return nil, connectError(err)
	}
	items := make([]*apiv1.PinnedMessage, 0, len(result.Items))
	for _, item := range result.Items {
		pinned, err := s.apiPinnedMessage(ctx, caller.UserID, item.Pin, item.Event)
		if err != nil {
			return nil, err
		}
		items = append(items, pinned)
	}
	return connect.NewResponse(&apiv1.ListPinnedMessagesResponse{
		PinnedMessages: items,
		Page:           apiPageInfo(result.TotalCount, result.HasMore),
	}), nil
}

func (s *roomService) BatchGetPinnedMessages(ctx context.Context, req *connect.Request[apiv1.BatchGetPinnedMessagesRequest]) (*connect.Response[apiv1.BatchGetPinnedMessagesResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	result, err := s.api.core.RoomTimelineReads().BatchGetPinnedMessages(ctx, core.PinnedMessageBatchGetInput{
		ActorID: caller.UserID, RoomID: req.Msg.GetRoomId(), MessageEventIDs: req.Msg.GetMessageEventIds(),
	})
	if err != nil {
		return nil, connectError(err)
	}
	items := make([]*apiv1.PinnedMessage, 0, len(result))
	for _, item := range result {
		pinned, err := s.apiPinnedMessage(ctx, caller.UserID, item.Pin, item.Event)
		if err != nil {
			return nil, err
		}
		items = append(items, pinned)
	}
	return connect.NewResponse(&apiv1.BatchGetPinnedMessagesResponse{PinnedMessages: items}), nil
}

func (s *roomService) CreatePinnedMessage(ctx context.Context, req *connect.Request[apiv1.CreatePinnedMessageRequest]) (*connect.Response[apiv1.CreatePinnedMessageResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	pin, err := s.api.core.RoomCommands().CreatePinnedMessage(ctx, core.PinnedMessageMutationInput{ActorID: caller.UserID, RoomID: req.Msg.GetRoomId(), MessageEventID: req.Msg.GetMessageEventId()})
	if err != nil {
		return nil, connectError(err)
	}
	result, err := s.api.core.RoomTimelineReads().GetMessage(ctx, caller.UserID, req.Msg.GetRoomId(), pin.MessageEventID)
	if err != nil {
		return nil, connectError(err)
	}
	pinned, err := s.apiPinnedMessage(ctx, caller.UserID, pin, result.Event)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.CreatePinnedMessageResponse{PinnedMessage: pinned}), nil
}

func (s *roomService) DeletePinnedMessage(ctx context.Context, req *connect.Request[apiv1.DeletePinnedMessageRequest]) (*connect.Response[apiv1.DeletePinnedMessageResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	deleted, err := s.api.core.RoomCommands().DeletePinnedMessage(ctx, core.PinnedMessageMutationInput{ActorID: caller.UserID, RoomID: req.Msg.GetRoomId(), MessageEventID: req.Msg.GetMessageEventId()})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.DeletePinnedMessageResponse{Deleted: deleted}), nil
}

func (s *roomService) apiPinnedMessage(ctx context.Context, viewerID string, pin core.PinnedMessageState, event *corev1.Event) (*apiv1.PinnedMessage, error) {
	apiEvent, err := (&messageService{api: s.api}).hydratePostedEvent(ctx, viewerID, core.KindChannel, event)
	if err != nil {
		return nil, connectError(err)
	}
	actor, err := s.optionalUserSummary(ctx, event.GetActorId())
	if err != nil {
		return nil, err
	}
	pinnedBy, err := s.optionalUserSummary(ctx, pin.ActorID)
	if err != nil {
		return nil, err
	}
	return &apiv1.PinnedMessage{Id: pin.PinEventID, Message: messageFromTimelineEvent(apiEvent), Actor: actor, PinnedBy: pinnedBy, PinnedAt: timestamppb.New(pin.PinnedAt)}, nil
}

func (s *roomService) optionalUserSummary(ctx context.Context, userID string) (*apiv1.User, error) {
	user, err := s.api.core.GetUser(ctx, userID)
	if errors.Is(err, core.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, connectError(err)
	}
	return userSummary(ctx, s.api, user, nil)
}

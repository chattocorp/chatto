package connectapi

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
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
	items, err := newPinnedMessageAssembler(s.api).assemble(ctx, caller.UserID, result.Items)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.ListPinnedMessagesResponse{
		PinnedMessages:  items,
		Page:            apiPageInfo(result.TotalCount, result.HasMore),
		LatestPinMarker: result.LatestPinEventID,
	}), nil
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
	pinned, err := newPinnedMessageAssembler(s.api).assemble(ctx, caller.UserID, []core.PinnedMessageItem{{Pin: pin, Event: result.Event}})
	if err != nil {
		return nil, connectError(err)
	}
	if len(pinned) != 1 {
		return nil, connectError(errors.New("pinned message hydration returned no result"))
	}
	return connect.NewResponse(&apiv1.CreatePinnedMessageResponse{PinnedMessage: pinned[0]}), nil
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

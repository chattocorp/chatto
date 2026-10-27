package connectapi

import (
	"context"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
)

func apiRoomNotice(notice *readmodel.RoomNotice) *apiv1.RoomNotice {
	return &apiv1.RoomNotice{Body: notice.GetBody(), Version: notice.GetVersion(), PublicationId: notice.GetPublicationId(), PublishedAt: notice.GetPublishedAt()}
}

func (s *roomService) GetNotice(ctx context.Context, req *connect.Request[apiv1.GetNoticeRequest]) (*connect.Response[apiv1.GetNoticeResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	notice, err := s.api.core.RoomCommands().GetNotice(ctx, core.RoomIDInput{ActorID: caller.UserID, RoomID: req.Msg.RoomId})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.GetNoticeResponse{Notice: apiRoomNotice(notice)}), nil
}

func (s *roomService) PublishNotice(ctx context.Context, req *connect.Request[apiv1.PublishNoticeRequest]) (*connect.Response[apiv1.PublishNoticeResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	notice, err := s.api.core.RoomCommands().PublishNotice(ctx, core.RoomNoticeInput{ActorID: caller.UserID, RoomID: req.Msg.RoomId, ExpectedVersion: req.Msg.ExpectedVersion, Body: req.Msg.Body, Republish: req.Msg.Republish})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.PublishNoticeResponse{Notice: apiRoomNotice(notice)}), nil
}

func (s *roomService) ClearNotice(ctx context.Context, req *connect.Request[apiv1.ClearNoticeRequest]) (*connect.Response[apiv1.ClearNoticeResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	notice, err := s.api.core.RoomCommands().ClearNotice(ctx, core.RoomNoticeInput{ActorID: caller.UserID, RoomID: req.Msg.RoomId, ExpectedVersion: req.Msg.ExpectedVersion})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.ClearNoticeResponse{Notice: apiRoomNotice(notice)}), nil
}

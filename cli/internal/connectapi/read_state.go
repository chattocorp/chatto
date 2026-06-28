package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	appv1 "hmans.de/chatto/internal/pb/chatto/app/v1"
)

type readStateService struct {
	api *API
}

func (s *readStateService) MarkRoomAsRead(ctx context.Context, req *connect.Request[appv1.MarkRoomAsReadRequest]) (*connect.Response[appv1.MarkRoomAsReadResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	result, err := s.api.core.ReadState().MarkRoomAsRead(ctx, caller.UserID, req.Msg.RoomId, req.Msg.UpToEventId)
	if err != nil {
		return nil, connectError(err)
	}

	resp := &appv1.MarkRoomAsReadResponse{}
	if !result.LastReadAt.IsZero() {
		resp.LastReadAt = timestamppb.New(result.LastReadAt)
	}
	if !result.PreviousLastReadAt.IsZero() {
		resp.PreviousLastReadAt = timestamppb.New(result.PreviousLastReadAt)
	}
	return connect.NewResponse(resp), nil
}

func (s *readStateService) MarkThreadAsRead(ctx context.Context, req *connect.Request[appv1.MarkThreadAsReadRequest]) (*connect.Response[appv1.MarkThreadAsReadResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	result, err := s.api.core.ReadState().MarkThreadAsRead(ctx, caller.UserID, req.Msg.RoomId, req.Msg.ThreadRootEventId, req.Msg.UpToEventId)
	if err != nil {
		return nil, connectError(err)
	}

	resp := &appv1.MarkThreadAsReadResponse{}
	if !result.PreviousReadAt.IsZero() {
		resp.PreviousReadAt = timestamppb.New(result.PreviousReadAt)
	}
	return connect.NewResponse(resp), nil
}

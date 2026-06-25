package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

type readStateService struct {
	api *API
}

func (s *readStateService) MarkRoomAsRead(ctx context.Context, req *connect.Request[apiv1.MarkRoomAsReadRequest]) (*connect.Response[apiv1.MarkRoomAsReadResponse], error) {
	return handleAuthedUnary(ctx, func(ctx context.Context, user authenticatedUser) (*apiv1.MarkRoomAsReadResponse, error) {
		result, err := s.api.core.ReadState().MarkRoomAsRead(ctx, user.Id, req.Msg.RoomId, req.Msg.UpToEventId)
		if err != nil {
			return nil, err
		}

		resp := &apiv1.MarkRoomAsReadResponse{}
		if !result.LastReadAt.IsZero() {
			resp.LastReadAt = timestamppb.New(result.LastReadAt)
		}
		if !result.PreviousLastReadAt.IsZero() {
			resp.PreviousLastReadAt = timestamppb.New(result.PreviousLastReadAt)
		}
		return resp, nil
	})
}

func (s *readStateService) MarkThreadAsRead(ctx context.Context, req *connect.Request[apiv1.MarkThreadAsReadRequest]) (*connect.Response[apiv1.MarkThreadAsReadResponse], error) {
	return handleAuthedUnary(ctx, func(ctx context.Context, user authenticatedUser) (*apiv1.MarkThreadAsReadResponse, error) {
		result, err := s.api.core.ReadState().MarkThreadAsRead(ctx, user.Id, req.Msg.RoomId, req.Msg.ThreadRootEventId, req.Msg.UpToEventId)
		if err != nil {
			return nil, err
		}

		resp := &apiv1.MarkThreadAsReadResponse{}
		if !result.PreviousReadAt.IsZero() {
			resp.PreviousReadAt = timestamppb.New(result.PreviousReadAt)
		}
		return resp, nil
	})
}

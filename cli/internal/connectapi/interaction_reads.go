package connectapi

import (
	"connectrpc.com/connect"
	"context"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func (s *messageService) ListReactionUsers(ctx context.Context, req *connect.Request[apiv1.ListReactionUsersRequest]) (*connect.Response[apiv1.ListReactionUsersResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := apiPagination(req.Msg.GetPage(), 50, 100)
	page, err := s.api.core.RoomTimelineReads().ListReactionUsers(ctx, caller.UserID, req.Msg.GetRoomId(), req.Msg.GetMessageEventId(), req.Msg.GetEmoji(), limit, offset)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.ListReactionUsersResponse{UserIds: page.UserIDs, Page: apiPageInfo(page.TotalCount, page.HasMore)}), nil
}

func (s *threadService) ListThreadParticipants(ctx context.Context, req *connect.Request[apiv1.ListThreadParticipantsRequest]) (*connect.Response[apiv1.ListThreadParticipantsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := apiPagination(req.Msg.GetPage(), 50, 100)
	page, err := s.api.core.RoomTimelineReads().ListThreadParticipants(ctx, caller.UserID, req.Msg.GetRoomId(), req.Msg.GetThreadRootEventId(), limit, offset)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.ListThreadParticipantsResponse{UserIds: page.UserIDs, Page: apiPageInfo(page.TotalCount, page.HasMore)}), nil
}

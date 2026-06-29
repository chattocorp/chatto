package connectapi

import (
	"context"

	"connectrpc.com/connect"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

const (
	defaultFollowedThreadLimit = 20
	maxFollowedThreadLimit     = 100
)

type threadService struct {
	api *API
}

func (s *threadService) ListFollowedThreads(ctx context.Context, req *connect.Request[apiv1.ListFollowedThreadsRequest]) (*connect.Response[apiv1.ListFollowedThreadsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	limit, offset := apiPagination(req.Msg.GetPage(), defaultFollowedThreadLimit, maxFollowedThreadLimit)
	page, err := s.api.core.ThreadFollows().ListFollowedThreads(ctx, caller.UserID, limit, offset)
	if err != nil {
		return nil, connectError(err)
	}

	resp, err := newThreadAssembler(s.api).followedThreadsResponse(ctx, caller.UserID, page)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(resp), nil
}

func (s *threadService) FollowThread(ctx context.Context, req *connect.Request[apiv1.FollowThreadRequest]) (*connect.Response[apiv1.FollowThreadResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.api.core.ThreadFollows().FollowThread(ctx, caller.UserID, req.Msg.RoomId, req.Msg.ThreadRootEventId); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.FollowThreadResponse{Following: true}), nil
}

func (s *threadService) UnfollowThread(ctx context.Context, req *connect.Request[apiv1.UnfollowThreadRequest]) (*connect.Response[apiv1.UnfollowThreadResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.api.core.ThreadFollows().UnfollowThread(ctx, caller.UserID, req.Msg.RoomId, req.Msg.ThreadRootEventId); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.UnfollowThreadResponse{Following: false}), nil
}

package connectapi

import (
	"context"

	"connectrpc.com/connect"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

type threadService struct {
	api *API
}

func (s *threadService) FollowThread(ctx context.Context, req *connect.Request[apiv1.FollowThreadRequest]) (*connect.Response[apiv1.FollowThreadResponse], error) {
	return handleAuthedUnary(ctx, func(ctx context.Context, user authenticatedUser) (*apiv1.FollowThreadResponse, error) {
		if err := s.api.core.ThreadFollows().FollowThread(ctx, user.Id, req.Msg.RoomId, req.Msg.ThreadRootEventId); err != nil {
			return nil, err
		}
		return &apiv1.FollowThreadResponse{Following: true}, nil
	})
}

func (s *threadService) UnfollowThread(ctx context.Context, req *connect.Request[apiv1.UnfollowThreadRequest]) (*connect.Response[apiv1.UnfollowThreadResponse], error) {
	return handleAuthedUnary(ctx, func(ctx context.Context, user authenticatedUser) (*apiv1.UnfollowThreadResponse, error) {
		if err := s.api.core.ThreadFollows().UnfollowThread(ctx, user.Id, req.Msg.RoomId, req.Msg.ThreadRootEventId); err != nil {
			return nil, err
		}
		return &apiv1.UnfollowThreadResponse{Following: false}, nil
	})
}

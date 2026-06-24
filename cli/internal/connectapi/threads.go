package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

type threadService struct {
	api *API
}

func (s *threadService) FollowThread(ctx context.Context, req *connect.Request[apiv1.FollowThreadRequest]) (*connect.Response[apiv1.FollowThreadResponse], error) {
	user, room, err := s.api.requireRoomMember(ctx, req.Msg.RoomId)
	if err != nil {
		return nil, err
	}
	if _, err := s.api.requireThreadRoot(ctx, room, req.Msg.ThreadRootEventId); err != nil {
		return nil, err
	}
	kind := core.KindOfRoom(room)

	if err := s.api.core.FollowThread(ctx, kind, user.Id, room.Id, req.Msg.ThreadRootEventId); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.FollowThreadResponse{Following: true}), nil
}

func (s *threadService) UnfollowThread(ctx context.Context, req *connect.Request[apiv1.UnfollowThreadRequest]) (*connect.Response[apiv1.UnfollowThreadResponse], error) {
	user, room, err := s.api.requireRoomMember(ctx, req.Msg.RoomId)
	if err != nil {
		return nil, err
	}
	if _, err := s.api.requireThreadRoot(ctx, room, req.Msg.ThreadRootEventId); err != nil {
		return nil, err
	}
	kind := core.KindOfRoom(room)

	if err := s.api.core.UnfollowThread(ctx, kind, user.Id, room.Id, req.Msg.ThreadRootEventId); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.UnfollowThreadResponse{Following: false}), nil
}

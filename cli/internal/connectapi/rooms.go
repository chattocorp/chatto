package connectapi

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type roomService struct {
	api *API
}

func (s *roomService) CreateRoom(ctx context.Context, req *connect.Request[apiv1.CreateRoomRequest]) (*connect.Response[apiv1.CreateRoomResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	room, err := s.api.core.RoomCommands().CreateRoom(ctx, core.RoomCreateInput{
		ActorID:     caller.UserID,
		GroupID:     req.Msg.GroupId,
		Name:        req.Msg.Name,
		Description: req.Msg.Description,
		Universal:   req.Msg.Universal,
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.CreateRoomResponse{Room: apiRoom(room)}), nil
}

func (s *roomService) UpdateRoom(ctx context.Context, req *connect.Request[apiv1.UpdateRoomRequest]) (*connect.Response[apiv1.UpdateRoomResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	room, err := s.api.core.RoomCommands().UpdateRoom(ctx, core.RoomUpdateInput{
		ActorID:     caller.UserID,
		RoomID:      req.Msg.RoomId,
		Name:        req.Msg.Name,
		Description: req.Msg.Description,
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.UpdateRoomResponse{Room: apiRoom(room)}), nil
}

func (s *roomService) ArchiveRoom(ctx context.Context, req *connect.Request[apiv1.ArchiveRoomRequest]) (*connect.Response[apiv1.ArchiveRoomResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	room, err := s.api.core.RoomCommands().ArchiveRoom(ctx, core.RoomIDInput{
		ActorID: caller.UserID,
		RoomID:  req.Msg.RoomId,
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.ArchiveRoomResponse{Room: apiRoom(room)}), nil
}

func (s *roomService) UnarchiveRoom(ctx context.Context, req *connect.Request[apiv1.UnarchiveRoomRequest]) (*connect.Response[apiv1.UnarchiveRoomResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	room, err := s.api.core.RoomCommands().UnarchiveRoom(ctx, core.RoomIDInput{
		ActorID: caller.UserID,
		RoomID:  req.Msg.RoomId,
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.UnarchiveRoomResponse{Room: apiRoom(room)}), nil
}

func (s *roomService) SetRoomUniversal(ctx context.Context, req *connect.Request[apiv1.SetRoomUniversalRequest]) (*connect.Response[apiv1.SetRoomUniversalResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	room, err := s.api.core.RoomCommands().SetRoomUniversal(ctx, core.RoomUniversalInput{
		ActorID:   caller.UserID,
		RoomID:    req.Msg.RoomId,
		Universal: req.Msg.Universal,
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.SetRoomUniversalResponse{Room: apiRoom(room)}), nil
}

func (s *roomService) JoinRoom(ctx context.Context, req *connect.Request[apiv1.JoinRoomRequest]) (*connect.Response[apiv1.JoinRoomResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	room, err := s.api.core.RoomCommands().JoinRoom(ctx, core.RoomIDInput{
		ActorID: caller.UserID,
		RoomID:  req.Msg.RoomId,
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.JoinRoomResponse{Room: apiRoom(room)}), nil
}

func (s *roomService) StartDM(ctx context.Context, req *connect.Request[apiv1.StartDMRequest]) (*connect.Response[apiv1.StartDMResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if len(req.Msg.ParticipantIds) > core.MaxDMParticipants-1 {
		return nil, invalidArgument("DM conversations are limited to 10 participants")
	}
	can, err := s.api.core.CanStartDM(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	if !can {
		return nil, connectError(core.ErrPermissionDenied)
	}

	room, _, err := s.api.core.FindOrCreateDM(ctx, caller.UserID, req.Msg.ParticipantIds)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.StartDMResponse{Room: apiRoom(room)}), nil
}

func (s *roomService) LeaveRoom(ctx context.Context, req *connect.Request[apiv1.LeaveRoomRequest]) (*connect.Response[apiv1.LeaveRoomResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.api.core.RoomCommands().LeaveRoom(ctx, core.RoomIDInput{
		ActorID: caller.UserID,
		RoomID:  req.Msg.RoomId,
	}); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.LeaveRoomResponse{Left: true}), nil
}

func (s *roomService) BanRoomMember(ctx context.Context, req *connect.Request[apiv1.BanRoomMemberRequest]) (*connect.Response[apiv1.BanRoomMemberResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	var expiresAt *time.Time
	if req.Msg.ExpiresAt != nil {
		t := req.Msg.ExpiresAt.AsTime()
		expiresAt = &t
	}

	if _, err := s.api.core.RoomCommands().BanRoomMember(ctx, core.RoomBanInput{
		ActorID:   caller.UserID,
		RoomID:    req.Msg.RoomId,
		UserID:    req.Msg.UserId,
		Reason:    req.Msg.Reason,
		ExpiresAt: expiresAt,
	}); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.BanRoomMemberResponse{Banned: true}), nil
}

func (s *roomService) UnbanRoomMember(ctx context.Context, req *connect.Request[apiv1.UnbanRoomMemberRequest]) (*connect.Response[apiv1.UnbanRoomMemberResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.api.core.RoomCommands().UnbanRoomMember(ctx, core.RoomUnbanInput{
		ActorID: caller.UserID,
		RoomID:  req.Msg.RoomId,
		UserID:  req.Msg.UserId,
		Reason:  req.Msg.Reason,
	}); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.UnbanRoomMemberResponse{Unbanned: true}), nil
}

func apiRoom(room *corev1.Room) *apiv1.Room {
	if room == nil {
		return nil
	}
	return &apiv1.Room{
		Id:          room.Id,
		Kind:        apiRoomKind(room.Kind),
		Name:        room.Name,
		Description: room.Description,
		Archived:    room.Archived,
		GroupId:     room.GroupId,
		Universal:   room.Universal,
	}
}

func apiRoomKind(kind corev1.RoomKind) apiv1.RoomKind {
	switch kind {
	case corev1.RoomKind_ROOM_KIND_CHANNEL:
		return apiv1.RoomKind_ROOM_KIND_CHANNEL
	case corev1.RoomKind_ROOM_KIND_DM:
		return apiv1.RoomKind_ROOM_KIND_DM
	default:
		return apiv1.RoomKind_ROOM_KIND_UNSPECIFIED
	}
}

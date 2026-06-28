package connectapi

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/types/known/timestamppb"
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

	room, _, err := s.api.core.RoomCommands().StartDM(ctx, core.RoomStartDMInput{
		ActorID:        caller.UserID,
		ParticipantIDs: req.Msg.ParticipantIds,
	})
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

func (s *roomService) ListRoomBans(ctx context.Context, req *connect.Request[apiv1.ListRoomBansRequest]) (*connect.Response[apiv1.ListRoomBansResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	var roomID *string
	if req.Msg.GetRoomId() != "" {
		value := req.Msg.GetRoomId()
		roomID = &value
	}
	bans, err := s.api.core.RoomCommands().ListActiveRoomBans(ctx, core.RoomBanListInput{
		ActorID: caller.UserID,
		RoomID:  roomID,
	})
	if err != nil {
		return nil, connectError(err)
	}

	directory := memberDirectoryService{api: s.api}
	out := make([]*apiv1.RoomBan, 0, len(bans))
	for _, ban := range bans {
		apiBan, err := s.apiRoomBan(ctx, directory, ban)
		if err != nil {
			return nil, err
		}
		out = append(out, apiBan)
	}
	return connect.NewResponse(&apiv1.ListRoomBansResponse{Bans: out}), nil
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

func (s *roomService) apiRoomBan(ctx context.Context, directory memberDirectoryService, ban core.RoomBan) (*apiv1.RoomBan, error) {
	var expiresAt *timestamppb.Timestamp
	if ban.ExpiresAt != nil {
		expiresAt = timestamppb.New(*ban.ExpiresAt)
	}
	out := &apiv1.RoomBan{
		Id:          ban.EventID,
		RoomId:      ban.RoomID,
		UserId:      ban.UserID,
		ModeratorId: ban.ModeratorID,
		Reason:      ban.Reason,
		CreatedAt:   timestamppb.New(ban.CreatedAt),
		ExpiresAt:   expiresAt,
	}

	room, err := s.api.core.GetRoom(ctx, core.KindChannel, ban.RoomID)
	if err != nil {
		if !errors.Is(err, core.ErrNotFound) && !errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, connectError(err)
		}
	} else {
		out.Room = apiRoom(room)
	}

	user, err := s.api.core.GetUser(ctx, ban.UserID)
	if err != nil {
		if !errors.Is(err, core.ErrNotFound) && !errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, connectError(err)
		}
	} else {
		apiUser, err := directory.directoryMember(ctx, user, nil)
		if err != nil {
			return nil, err
		}
		out.User = apiUser
	}

	moderator, err := s.api.core.GetUser(ctx, ban.ModeratorID)
	if err != nil {
		if !errors.Is(err, core.ErrNotFound) && !errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, connectError(err)
		}
	} else {
		apiModerator, err := directory.directoryMember(ctx, moderator, nil)
		if err != nil {
			return nil, err
		}
		out.Moderator = apiModerator
	}

	return out, nil
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

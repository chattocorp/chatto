package connectapi

import (
	"context"
	"fmt"
	"strings"
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
	if err := validateRoomNameAndDescription(req.Msg.Name, req.Msg.Description); err != nil {
		return nil, err
	}

	can, err := s.api.core.CanCreateRoom(ctx, caller.UserID, core.KindChannel, req.Msg.GroupId)
	if err != nil {
		return nil, connectError(err)
	}
	if !can {
		return nil, connectError(core.ErrPermissionDenied)
	}

	room, err := s.api.core.CreateRoom(ctx, caller.UserID, core.KindChannel, req.Msg.GroupId, req.Msg.Name, req.Msg.Description, core.WithUniversalRoom(req.Msg.Universal))
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
	if err := validateRoomNameAndDescription(req.Msg.Name, req.Msg.Description); err != nil {
		return nil, err
	}
	kind, err := s.resolveRoomKind(ctx, req.Msg.RoomId)
	if err != nil {
		return nil, connectError(err)
	}
	if err := rejectDMRoom(kind, "DM rooms cannot be managed through RoomService"); err != nil {
		return nil, err
	}
	if err := s.requireRoomManager(ctx, caller.UserID); err != nil {
		return nil, err
	}

	room, err := s.api.core.UpdateRoom(ctx, caller.UserID, kind, req.Msg.RoomId, req.Msg.Name, req.Msg.Description)
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
	kind, err := s.resolveRoomKind(ctx, req.Msg.RoomId)
	if err != nil {
		return nil, connectError(err)
	}
	if err := rejectDMRoom(kind, "DM rooms cannot be managed through RoomService"); err != nil {
		return nil, err
	}
	if err := s.requireRoomManager(ctx, caller.UserID); err != nil {
		return nil, err
	}

	room, err := s.api.core.ArchiveRoom(ctx, caller.UserID, kind, req.Msg.RoomId)
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
	kind, err := s.resolveRoomKind(ctx, req.Msg.RoomId)
	if err != nil {
		return nil, connectError(err)
	}
	if err := rejectDMRoom(kind, "DM rooms cannot be managed through RoomService"); err != nil {
		return nil, err
	}
	if err := s.requireRoomManager(ctx, caller.UserID); err != nil {
		return nil, err
	}

	room, err := s.api.core.UnarchiveRoom(ctx, caller.UserID, kind, req.Msg.RoomId)
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
	kind, err := s.resolveRoomKind(ctx, req.Msg.RoomId)
	if err != nil {
		return nil, connectError(err)
	}
	if kind == core.KindDM {
		return nil, invalidArgument("DM rooms cannot be universal")
	}
	if err := s.requireRoomManager(ctx, caller.UserID); err != nil {
		return nil, err
	}

	room, err := s.api.core.SetRoomUniversal(ctx, caller.UserID, kind, req.Msg.RoomId, req.Msg.Universal)
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
	kind, err := s.resolveRoomKind(ctx, req.Msg.RoomId)
	if err != nil {
		return nil, connectError(err)
	}
	if err := rejectDMRoom(kind, "DM rooms cannot be joined through RoomService"); err != nil {
		return nil, err
	}

	can, err := s.api.core.CanJoinRoomAt(ctx, caller.UserID, kind, req.Msg.RoomId)
	if err != nil {
		return nil, connectError(err)
	}
	if !can {
		return nil, connectError(core.ErrPermissionDenied)
	}
	if _, err := s.api.core.JoinRoom(ctx, caller.UserID, kind, caller.UserID, req.Msg.RoomId); err != nil {
		return nil, connectError(err)
	}
	room, err := s.api.core.GetRoom(ctx, kind, req.Msg.RoomId)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.JoinRoomResponse{Room: apiRoom(room)}), nil
}

func (s *roomService) LeaveRoom(ctx context.Context, req *connect.Request[apiv1.LeaveRoomRequest]) (*connect.Response[apiv1.LeaveRoomResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	kind, err := s.resolveRoomKind(ctx, req.Msg.RoomId)
	if err != nil {
		return nil, connectError(err)
	}
	if err := s.api.core.LeaveRoom(ctx, caller.UserID, kind, caller.UserID, req.Msg.RoomId); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.LeaveRoomResponse{Left: true}), nil
}

func (s *roomService) BanRoomMember(ctx context.Context, req *connect.Request[apiv1.BanRoomMemberRequest]) (*connect.Response[apiv1.BanRoomMemberResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	kind, err := s.resolveRoomKind(ctx, req.Msg.RoomId)
	if err != nil {
		return nil, connectError(err)
	}
	if kind == core.KindDM {
		return nil, connectError(core.ErrCannotBanDMRoomMember)
	}
	if err := validateRoomBanReason(req.Msg.Reason); err != nil {
		return nil, err
	}
	var expiresAt *time.Time
	if req.Msg.ExpiresAt != nil {
		t := req.Msg.ExpiresAt.AsTime()
		if !t.After(time.Now()) {
			return nil, invalidArgument("ban expiry must be in the future")
		}
		expiresAt = &t
	}

	can, err := s.api.core.PermResolver().HasRoomPermission(ctx, caller.UserID, kind, req.Msg.RoomId, core.PermRoomMemberBan)
	if err != nil {
		return nil, connectError(err)
	}
	if !can {
		return nil, connectError(core.ErrPermissionDenied)
	}
	if _, err := s.api.core.BanRoomMember(ctx, caller.UserID, kind, req.Msg.RoomId, req.Msg.UserId, req.Msg.Reason, expiresAt); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.BanRoomMemberResponse{Banned: true}), nil
}

func (s *roomService) UnbanRoomMember(ctx context.Context, req *connect.Request[apiv1.UnbanRoomMemberRequest]) (*connect.Response[apiv1.UnbanRoomMemberResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	kind, err := s.resolveRoomKind(ctx, req.Msg.RoomId)
	if err != nil {
		return nil, connectError(err)
	}
	if kind == core.KindDM {
		return nil, connectError(core.ErrCannotBanDMRoomMember)
	}
	if err := validateRoomBanReason(req.Msg.Reason); err != nil {
		return nil, err
	}

	can, err := s.api.core.PermResolver().HasRoomPermission(ctx, caller.UserID, kind, req.Msg.RoomId, core.PermRoomMemberBan)
	if err != nil {
		return nil, connectError(err)
	}
	if !can {
		return nil, connectError(core.ErrPermissionDenied)
	}
	if err := s.api.core.UnbanRoomMember(ctx, caller.UserID, kind, req.Msg.RoomId, req.Msg.UserId, req.Msg.Reason); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.UnbanRoomMemberResponse{Unbanned: true}), nil
}

func (s *roomService) resolveRoomKind(ctx context.Context, roomID string) (core.RoomKind, error) {
	kind, err := s.api.core.FindRoomKind(ctx, roomID)
	if err != nil {
		return "", err
	}
	return kind, nil
}

func (s *roomService) requireRoomManager(ctx context.Context, userID string) error {
	can, err := s.api.core.CanManageAnyRoom(ctx, userID)
	if err != nil {
		return connectError(err)
	}
	if !can {
		return connectError(core.ErrPermissionDenied)
	}
	return nil
}

func validateRoomNameAndDescription(name, description string) error {
	if err := core.ValidateRoomName(name); err != nil {
		return invalidArgument(err.Error())
	}
	if err := core.ValidateRoomDescription(description); err != nil {
		return invalidArgument(err.Error())
	}
	return nil
}

func validateRoomBanReason(reason string) error {
	trimmed := strings.TrimSpace(reason)
	if trimmed == "" {
		return invalidArgument("ban reason is required")
	}
	if len([]rune(trimmed)) > core.MaxRoomBanReasonLength {
		return invalidArgument(fmt.Sprintf("ban reason exceeds %d characters", core.MaxRoomBanReasonLength))
	}
	return nil
}

func rejectDMRoom(kind core.RoomKind, message string) error {
	if kind == core.KindDM {
		return invalidArgument(message)
	}
	return nil
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

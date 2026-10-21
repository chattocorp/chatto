package connectapi

import (
	"context"
	"errors"
	"time"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type roomInvitationService struct{ api *API }

func apiRoomInvitation(v core.RoomInvitationState, canAccept bool) *apiv1.RoomInvitation {
	status := apiv1.RoomInvitationStatus_ROOM_INVITATION_STATUS_PENDING
	switch v.Status {
	case "accepted":
		status = apiv1.RoomInvitationStatus_ROOM_INVITATION_STATUS_ACCEPTED
	case "declined":
		status = apiv1.RoomInvitationStatus_ROOM_INVITATION_STATUS_DECLINED
	default:
		if !time.Now().Before(v.ExpiresAt) {
			status = apiv1.RoomInvitationStatus_ROOM_INVITATION_STATUS_EXPIRED
		}
	}
	return &apiv1.RoomInvitation{Id: v.ID, RoomId: v.RoomID, SenderId: v.SenderID, RecipientId: v.RecipientID, CreatedAt: timestamppb.New(v.CreatedAt), ExpiresAt: timestamppb.New(v.ExpiresAt), Status: status, CanAccept: canAccept}
}
func (s *roomInvitationService) assemble(ctx context.Context, actor string, v core.RoomInvitationState) (*apiv1.RoomInvitation, error) {
	can, err := s.api.core.RoomInvitations().CanAccept(ctx, actor, v)
	if err != nil {
		return nil, err
	}
	return apiRoomInvitation(v, can), nil
}
func (s *roomInvitationService) CreateRoomInvitation(ctx context.Context, req *connect.Request[apiv1.CreateRoomInvitationRequest]) (*connect.Response[apiv1.CreateRoomInvitationResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	v, err := s.api.core.RoomInvitations().Create(ctx, caller.UserID, req.Msg.RoomId, req.Msg.RecipientId)
	if err != nil {
		return nil, err
	}
	result, err := s.assemble(ctx, caller.UserID, v)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.CreateRoomInvitationResponse{Invitation: result}), nil
}
func (s *roomInvitationService) GetRoomInvitation(ctx context.Context, req *connect.Request[apiv1.GetRoomInvitationRequest]) (*connect.Response[apiv1.GetRoomInvitationResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	v, err := s.api.core.RoomInvitations().Get(ctx, caller.UserID, req.Msg.InvitationId)
	if err != nil {
		return nil, err
	}
	result, err := s.assemble(ctx, caller.UserID, v)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.GetRoomInvitationResponse{Invitation: result}), nil
}
func (s *roomInvitationService) BatchGetRoomInvitations(ctx context.Context, req *connect.Request[apiv1.BatchGetRoomInvitationsRequest]) (*connect.Response[apiv1.BatchGetRoomInvitationsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	result := &apiv1.BatchGetRoomInvitationsResponse{}
	for _, id := range req.Msg.InvitationIds {
		v, err := s.api.core.RoomInvitations().Get(ctx, caller.UserID, id)
		if errors.Is(err, core.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		item, err := s.assemble(ctx, caller.UserID, v)
		if err != nil {
			return nil, err
		}
		result.Invitations = append(result.Invitations, item)
	}
	return connect.NewResponse(result), nil
}
func (s *roomInvitationService) ListRoomInvitations(ctx context.Context, req *connect.Request[apiv1.ListRoomInvitationsRequest]) (*connect.Response[apiv1.ListRoomInvitationsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.api.core.RoomInvitations().List(ctx, caller.UserID, req.Msg.Sent, req.Msg.PendingOnly)
	if err != nil {
		return nil, err
	}
	limit, offset := apiPagination(req.Msg.Page, 50, 100)
	total := len(rows)
	start := min(offset, total)
	end := min(start+limit, total)
	result := &apiv1.ListRoomInvitationsResponse{Page: apiPageInfo(total, end < total)}
	for _, v := range rows[start:end] {
		item, err := s.assemble(ctx, caller.UserID, v)
		if err != nil {
			return nil, err
		}
		result.Invitations = append(result.Invitations, item)
	}
	return connect.NewResponse(result), nil
}
func (s *roomInvitationService) AcceptRoomInvitation(ctx context.Context, req *connect.Request[apiv1.AcceptRoomInvitationRequest]) (*connect.Response[apiv1.AcceptRoomInvitationResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	v, err := s.api.core.RoomInvitations().Respond(ctx, caller.UserID, req.Msg.InvitationId, true)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.AcceptRoomInvitationResponse{Invitation: apiRoomInvitation(v, false)}), nil
}
func (s *roomInvitationService) DeclineRoomInvitation(ctx context.Context, req *connect.Request[apiv1.DeclineRoomInvitationRequest]) (*connect.Response[apiv1.DeclineRoomInvitationResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	v, err := s.api.core.RoomInvitations().Respond(ctx, caller.UserID, req.Msg.InvitationId, false)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.DeclineRoomInvitationResponse{Invitation: apiRoomInvitation(v, false)}), nil
}

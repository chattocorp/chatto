package connectapi

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
)

const (
	defaultInvitationLimit = 20
	maxInvitationLimit     = 100
)

type adminInvitationService struct{ api *API }

func (s *adminInvitationService) ListInvitations(ctx context.Context, req *connect.Request[adminv1.ListInvitationsRequest]) (*connect.Response[adminv1.ListInvitationsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	states, err := s.api.core.ListInvitations(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	limit, offset := apiPagination(req.Msg.GetPage(), defaultInvitationLimit, maxInvitationLimit)
	total := len(states)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	result := make([]*adminv1.Invitation, 0, end-offset)
	for _, state := range states[offset:end] {
		result = append(result, s.apiInvitation(state))
	}
	return connect.NewResponse(&adminv1.ListInvitationsResponse{
		Invitations: result,
		Page:        apiPageInfo(total, end < total),
	}), nil
}

func (s *adminInvitationService) GetInvitation(ctx context.Context, req *connect.Request[adminv1.GetInvitationRequest]) (*connect.Response[adminv1.GetInvitationResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	state, err := s.api.core.GetInvitation(ctx, caller.UserID, req.Msg.GetId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.GetInvitationResponse{Invitation: s.apiInvitation(state)}), nil
}

func (s *adminInvitationService) CreateInvitation(ctx context.Context, req *connect.Request[adminv1.CreateInvitationRequest]) (*connect.Response[adminv1.CreateInvitationResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	var maxUses *uint32
	if req.Msg.MaxUses != nil {
		value := req.Msg.GetMaxUses()
		maxUses = &value
	}
	expiresAt, err := apiTimestampToTime(req.Msg.GetExpiresAt())
	if err != nil {
		return nil, err
	}
	state, err := s.api.core.CreateInvitation(ctx, caller.UserID, maxUses, expiresAt)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.CreateInvitationResponse{Invitation: s.apiInvitation(state)}), nil
}

func (s *adminInvitationService) RevokeInvitation(ctx context.Context, req *connect.Request[adminv1.RevokeInvitationRequest]) (*connect.Response[adminv1.RevokeInvitationResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	state, err := s.api.core.RevokeInvitation(ctx, caller.UserID, req.Msg.GetId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.RevokeInvitationResponse{Invitation: s.apiInvitation(state)}), nil
}

func (s *adminInvitationService) apiInvitation(state core.InvitationState) *adminv1.Invitation {
	invitation := &adminv1.Invitation{
		Id:        state.ID,
		Code:      s.api.core.InvitationCode(state.ID),
		CreatedBy: state.CreatedBy,
		CreatedAt: timestamppb.New(state.CreatedAt),
		MaxUses:   state.MaxUses,
		UseCount:  state.UseCount,
		Status:    apiInvitationStatus(core.InvitationStatusAt(state, time.Now())),
	}
	if state.ExpiresAt != nil {
		invitation.ExpiresAt = timestamppb.New(*state.ExpiresAt)
	}
	if state.RevokedAt != nil {
		invitation.RevokedAt = timestamppb.New(*state.RevokedAt)
	}
	return invitation
}

func apiInvitationStatus(status core.InvitationStatus) adminv1.InvitationStatus {
	switch status {
	case core.InvitationStatusActive:
		return adminv1.InvitationStatus_INVITATION_STATUS_ACTIVE
	case core.InvitationStatusExpired:
		return adminv1.InvitationStatus_INVITATION_STATUS_EXPIRED
	case core.InvitationStatusExhausted:
		return adminv1.InvitationStatus_INVITATION_STATUS_EXHAUSTED
	case core.InvitationStatusRevoked:
		return adminv1.InvitationStatus_INVITATION_STATUS_REVOKED
	default:
		return adminv1.InvitationStatus_INVITATION_STATUS_UNSPECIFIED
	}
}

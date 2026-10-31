package connectapi

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	adminv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/admin/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

type adminInvitationCodeService struct{ api *API }

func (s *adminInvitationCodeService) ListInvitationCodes(ctx context.Context, req *connect.Request[adminv1.ListInvitationCodesRequest]) (*connect.Response[adminv1.ListInvitationCodesResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	states, err := s.api.core.ListInvitations(ctx, caller.UserID)
	if err != nil {
		return nil, err
	}
	limit, offset := apiPagination(req.Msg.GetPage(), 20, 100)
	offset = min(offset, len(states))
	end := min(offset+limit, len(states))
	result := make([]*adminv1.InvitationCode, 0, end-offset)
	for _, state := range states[offset:end] {
		result = append(result, s.apiInvitationCode(state))
	}
	response := connect.NewResponse(&adminv1.ListInvitationCodesResponse{InvitationCodes: result, Page: apiPageInfo(len(states), end < len(states))})
	response.Header().Set("Cache-Control", "no-store")
	return response, nil
}

func (s *adminInvitationCodeService) GetInvitationCode(ctx context.Context, req *connect.Request[adminv1.GetInvitationCodeRequest]) (*connect.Response[adminv1.GetInvitationCodeResponse], error) {
	state, err := s.get(ctx, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	response := connect.NewResponse(&adminv1.GetInvitationCodeResponse{InvitationCode: s.apiInvitationCode(state)})
	response.Header().Set("Cache-Control", "no-store")
	return response, nil
}

func (s *adminInvitationCodeService) CreateInvitationCode(ctx context.Context, req *connect.Request[adminv1.CreateInvitationCodeRequest]) (*connect.Response[adminv1.CreateInvitationCodeResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	state, err := s.api.core.CreateInvitationCode(ctx, caller.UserID, req.Msg.GetNote())
	if err != nil {
		return nil, err
	}
	response := connect.NewResponse(&adminv1.CreateInvitationCodeResponse{InvitationCode: s.apiInvitationCode(state)})
	response.Header().Set("Cache-Control", "no-store")
	return response, nil
}

func (s *adminInvitationCodeService) RevokeInvitationCode(ctx context.Context, req *connect.Request[adminv1.RevokeInvitationCodeRequest]) (*connect.Response[adminv1.RevokeInvitationCodeResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	state, err := s.api.core.RevokeInvitation(ctx, caller.UserID, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	response := connect.NewResponse(&adminv1.RevokeInvitationCodeResponse{InvitationCode: s.apiInvitationCode(state)})
	response.Header().Set("Cache-Control", "no-store")
	return response, nil
}

func (s *adminInvitationCodeService) get(ctx context.Context, id string) (core.InvitationState, error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return core.InvitationState{}, err
	}
	state, err := s.api.core.GetInvitation(ctx, caller.UserID, id)
	if err != nil {
		return core.InvitationState{}, err
	}
	return state, nil
}

func (s *adminInvitationCodeService) apiInvitationCode(state core.InvitationState) *adminv1.InvitationCode {
	code := &adminv1.InvitationCode{Id: state.ID, Code: s.api.core.InvitationCode(state.ID), CreatedBy: state.CreatedBy, CreatedAt: timestamppb.New(state.CreatedAt)}
	code.Note = state.Note
	switch core.InvitationStatusAt(state, time.Now()) {
	case core.InvitationStatusActive:
		code.Status = adminv1.InvitationCodeStatus_INVITATION_CODE_STATUS_ACTIVE
	case core.InvitationStatusExpired:
		code.Status = adminv1.InvitationCodeStatus_INVITATION_CODE_STATUS_EXPIRED
	case core.InvitationStatusUsed:
		code.Status = adminv1.InvitationCodeStatus_INVITATION_CODE_STATUS_USED
	case core.InvitationStatusRevoked:
		code.Status = adminv1.InvitationCodeStatus_INVITATION_CODE_STATUS_REVOKED
	}
	if state.ExpiresAt != nil {
		code.ExpiresAt = timestamppb.New(*state.ExpiresAt)
	}
	if state.RevokedAt != nil {
		code.RevokedAt = timestamppb.New(*state.RevokedAt)
	}
	return code
}

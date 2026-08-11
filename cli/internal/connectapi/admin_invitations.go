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
	defaultInviteLinkLimit = 20
	maxInviteLinkLimit     = 100
)

type adminInviteLinkService struct{ api *API }

func (s *adminInviteLinkService) ListInviteLinks(ctx context.Context, req *connect.Request[adminv1.ListInviteLinksRequest]) (*connect.Response[adminv1.ListInviteLinksResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	states, err := s.api.core.ListInvitations(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	limit, offset := apiPagination(req.Msg.GetPage(), defaultInviteLinkLimit, maxInviteLinkLimit)
	total := len(states)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	result := make([]*adminv1.InviteLink, 0, end-offset)
	for _, state := range states[offset:end] {
		result = append(result, s.apiInviteLink(ctx, state))
	}
	return connect.NewResponse(&adminv1.ListInviteLinksResponse{
		InviteLinks: result,
		Page:        apiPageInfo(total, end < total),
	}), nil
}

func (s *adminInviteLinkService) GetInviteLink(ctx context.Context, req *connect.Request[adminv1.GetInviteLinkRequest]) (*connect.Response[adminv1.GetInviteLinkResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	state, err := s.api.core.GetInvitation(ctx, caller.UserID, req.Msg.GetId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.GetInviteLinkResponse{InviteLink: s.apiInviteLink(ctx, state)}), nil
}

func (s *adminInviteLinkService) CreateInviteLink(ctx context.Context, req *connect.Request[adminv1.CreateInviteLinkRequest]) (*connect.Response[adminv1.CreateInviteLinkResponse], error) {
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
	return connect.NewResponse(&adminv1.CreateInviteLinkResponse{InviteLink: s.apiInviteLink(ctx, state)}), nil
}

func (s *adminInviteLinkService) RevokeInviteLink(ctx context.Context, req *connect.Request[adminv1.RevokeInviteLinkRequest]) (*connect.Response[adminv1.RevokeInviteLinkResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	state, err := s.api.core.RevokeInvitation(ctx, caller.UserID, req.Msg.GetId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.RevokeInviteLinkResponse{InviteLink: s.apiInviteLink(ctx, state)}), nil
}

func (s *adminInviteLinkService) apiInviteLink(ctx context.Context, state core.InvitationState) *adminv1.InviteLink {
	inviteLink := &adminv1.InviteLink{
		Id:        state.ID,
		Link:      s.api.absolutizeServerURL(ctx, s.api.core.InvitationLinkPath(state.ID)),
		CreatedBy: state.CreatedBy,
		CreatedAt: timestamppb.New(state.CreatedAt),
		MaxUses:   state.MaxUses,
		UseCount:  state.UseCount,
		Status:    apiInviteLinkStatus(core.InvitationStatusAt(state, time.Now())),
	}
	if state.ExpiresAt != nil {
		inviteLink.ExpiresAt = timestamppb.New(*state.ExpiresAt)
	}
	if state.RevokedAt != nil {
		inviteLink.RevokedAt = timestamppb.New(*state.RevokedAt)
	}
	return inviteLink
}

func apiInviteLinkStatus(status core.InvitationStatus) adminv1.InviteLinkStatus {
	switch status {
	case core.InvitationStatusActive:
		return adminv1.InviteLinkStatus_INVITE_LINK_STATUS_ACTIVE
	case core.InvitationStatusExpired:
		return adminv1.InviteLinkStatus_INVITE_LINK_STATUS_EXPIRED
	case core.InvitationStatusExhausted:
		return adminv1.InviteLinkStatus_INVITE_LINK_STATUS_EXHAUSTED
	case core.InvitationStatusRevoked:
		return adminv1.InviteLinkStatus_INVITE_LINK_STATUS_REVOKED
	default:
		return adminv1.InviteLinkStatus_INVITE_LINK_STATUS_UNSPECIFIED
	}
}

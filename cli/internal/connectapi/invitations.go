package connectapi

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	defaultInviteLinkLimit = 20
	maxInviteLinkLimit     = 100
)

type inviteLinkService struct{ api *API }

func (s *inviteLinkService) ListInviteLinks(ctx context.Context, req *connect.Request[apiv1.ListInviteLinksRequest]) (*connect.Response[apiv1.ListInviteLinksResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	states, err := s.api.core.ListInvitations(ctx, caller.UserID)
	if err != nil {
		return nil, err
	}
	limit, offset := apiPagination(req.Msg.GetPage(), defaultInviteLinkLimit, maxInviteLinkLimit)
	total := len(states)
	if offset > total {
		offset = total
	}
	end := min(offset+limit, total)
	result := make([]*apiv1.InviteLink, 0, end-offset)
	for _, state := range states[offset:end] {
		result = append(result, s.apiInviteLink(ctx, state))
	}
	return connect.NewResponse(&apiv1.ListInviteLinksResponse{
		InviteLinks: result,
		Page:        apiPageInfo(total, end < total),
	}), nil
}

func (s *inviteLinkService) GetInviteLink(ctx context.Context, req *connect.Request[apiv1.GetInviteLinkRequest]) (*connect.Response[apiv1.GetInviteLinkResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	state, err := s.api.core.GetInvitation(ctx, caller.UserID, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.GetInviteLinkResponse{InviteLink: s.apiInviteLink(ctx, state)}), nil
}

func (s *inviteLinkService) CreateInviteLink(ctx context.Context, req *connect.Request[apiv1.CreateInviteLinkRequest]) (*connect.Response[apiv1.CreateInviteLinkResponse], error) {
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
		return nil, err
	}
	return connect.NewResponse(&apiv1.CreateInviteLinkResponse{InviteLink: s.apiInviteLink(ctx, state)}), nil
}

func (s *inviteLinkService) RevokeInviteLink(ctx context.Context, req *connect.Request[apiv1.RevokeInviteLinkRequest]) (*connect.Response[apiv1.RevokeInviteLinkResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	state, err := s.api.core.RevokeInvitation(ctx, caller.UserID, req.Msg.GetId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.RevokeInviteLinkResponse{InviteLink: s.apiInviteLink(ctx, state)}), nil
}

func (s *inviteLinkService) apiInviteLink(ctx context.Context, state core.InvitationState) *apiv1.InviteLink {
	inviteLink := &apiv1.InviteLink{
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

func apiInviteLinkStatus(status core.InvitationStatus) apiv1.InviteLinkStatus {
	switch status {
	case core.InvitationStatusActive:
		return apiv1.InviteLinkStatus_INVITE_LINK_STATUS_ACTIVE
	case core.InvitationStatusExpired:
		return apiv1.InviteLinkStatus_INVITE_LINK_STATUS_EXPIRED
	case core.InvitationStatusExhausted:
		return apiv1.InviteLinkStatus_INVITE_LINK_STATUS_EXHAUSTED
	case core.InvitationStatusRevoked:
		return apiv1.InviteLinkStatus_INVITE_LINK_STATUS_REVOKED
	default:
		return apiv1.InviteLinkStatus_INVITE_LINK_STATUS_UNSPECIFIED
	}
}

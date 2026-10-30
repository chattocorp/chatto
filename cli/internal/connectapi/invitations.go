package connectapi

import (
	"context"

	"connectrpc.com/connect"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// invitationCodeService exposes issuance without any administrative reads.
type invitationCodeService struct{ api *API }

func (s *invitationCodeService) CreateInvitationCode(ctx context.Context, _ *connect.Request[apiv1.CreateInvitationCodeRequest]) (*connect.Response[apiv1.CreateInvitationCodeResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	state, err := s.api.core.IssueInvitationCode(ctx, caller.UserID)
	if err != nil {
		return nil, err
	}
	response := connect.NewResponse(&apiv1.CreateInvitationCodeResponse{InvitationCode: &apiv1.IssuedInvitationCode{
		Id: state.ID, Code: s.api.core.InvitationCode(state.ID), ExpiresAt: timestamppb.New(*state.ExpiresAt),
	}})
	response.Header().Set("Cache-Control", "no-store")
	return response, nil
}

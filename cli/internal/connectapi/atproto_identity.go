package connectapi

import (
	"context"

	"connectrpc.com/connect"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
)

// SetAtprotoIdentity publishes an AT Protocol DID on the caller's profile.
func (s *accountService) SetAtprotoIdentity(ctx context.Context, req *connect.Request[apiv1.SetAtprotoIdentityRequest]) (*connect.Response[apiv1.SetAtprotoIdentityResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	did, err := s.api.core.SetUserAtprotoDID(ctx, caller.UserID, req.Msg.GetDid())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.SetAtprotoIdentityResponse{Did: did}), nil
}

// ClearAtprotoIdentity removes the AT Protocol DID from the caller's profile.
func (s *accountService) ClearAtprotoIdentity(ctx context.Context, _ *connect.Request[apiv1.ClearAtprotoIdentityRequest]) (*connect.Response[apiv1.ClearAtprotoIdentityResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.api.core.ClearUserAtprotoDID(ctx, caller.UserID); err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.ClearAtprotoIdentityResponse{}), nil
}

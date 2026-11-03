package connectapi

import (
	"context"

	"connectrpc.com/connect"
	adminv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/admin/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *adminUserManagementService) ListVerifiedEmails(ctx context.Context, req *connect.Request[adminv1.ListVerifiedEmailsRequest]) (*connect.Response[adminv1.ListVerifiedEmailsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	emails, err := s.api.core.ListAdminVerifiedEmails(ctx, caller.UserID, req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.ListVerifiedEmailsResponse{VerifiedEmails: verifiedEmailsToAPI(emails)}), nil
}

func (s *adminUserManagementService) GetUsernameCooldown(ctx context.Context, req *connect.Request[adminv1.GetUsernameCooldownRequest]) (*connect.Response[adminv1.GetUsernameCooldownResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	changedAt, err := s.api.core.GetAdminUsernameCooldown(ctx, caller.UserID, req.Msg.GetUserId())
	if err != nil {
		return nil, err
	}
	cooldown := &adminv1.UsernameCooldown{UserId: req.Msg.GetUserId()}
	if changedAt != nil {
		cooldown.LastLoginChange = timestamppb.New(*changedAt)
	}
	return connect.NewResponse(&adminv1.GetUsernameCooldownResponse{Cooldown: cooldown}), nil
}

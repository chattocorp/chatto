package connectapi

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	adminv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/admin/v1"
)

func (s *adminUserManagementService) ListUsers(ctx context.Context, req *connect.Request[adminv1.ListUsersRequest]) (*connect.Response[adminv1.ListUsersResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := apiPagination(req.Msg.GetPage(), defaultAdminMemberLimit, maxAdminMemberLimit)
	page, err := s.api.core.ListAdminMembers(ctx, caller.UserID, core.AdminMemberListInput{Search: req.Msg.GetSearch(), Limit: limit, Offset: offset})
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.ListUsersResponse{UserIds: page.UserIDs, Page: apiPageInfo(page.TotalCount, page.HasMore)}), nil
}

func (s *adminUserManagementService) GetUser(ctx context.Context, req *connect.Request[adminv1.GetUserRequest]) (*connect.Response[adminv1.GetUserResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	var user *core.AdminMember
	switch target := req.Msg.GetTarget().(type) {
	case *adminv1.GetUserRequest_UserId:
		user, err = s.api.core.ReadAdminUser(ctx, caller.UserID, strings.TrimSpace(target.UserId))
	case *adminv1.GetUserRequest_Login:
		user, err = s.api.core.ReadAdminUserByLogin(ctx, caller.UserID, strings.TrimSpace(target.Login))
	default:
		return nil, invalidArgument("provide exactly one of user_id or login")
	}
	if err != nil {
		return nil, err
	}
	resource, err := s.assembleAdminUser(ctx, *user)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.GetUserResponse{User: resource}), nil
}

func (s *adminUserManagementService) BatchGetUsers(ctx context.Context, req *connect.Request[adminv1.BatchGetUsersRequest]) (*connect.Response[adminv1.BatchGetUsersResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	users, err := s.api.core.ReadAdminUsers(ctx, caller.UserID, req.Msg.GetUserIds())
	if err != nil {
		return nil, err
	}
	response, err := s.assembleAdminUsers(ctx, users)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(response), nil
}

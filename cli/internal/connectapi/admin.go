package connectapi

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

type adminService struct {
	api *API
}

const (
	defaultAdminUserLimit = 20
	maxAdminUserLimit     = 100
)

func (s *adminService) ListUsers(ctx context.Context, req *connect.Request[apiv1.ListAdminUsersRequest]) (*connect.Response[apiv1.ListAdminUsersResponse], error) {
	if _, err := requireAdminCaller(ctx); err != nil {
		return nil, err
	}

	limit, offset := apiPagination(req.Msg.GetPage(), defaultAdminUserLimit, maxAdminUserLimit)
	if req.Msg.GetPage() == nil && (req.Msg.GetLimit() != 0 || req.Msg.GetOffset() != 0) {
		limit = int(req.Msg.GetLimit())
		if limit <= 0 {
			limit = defaultAdminUserLimit
		}
		if limit > maxAdminUserLimit {
			limit = maxAdminUserLimit
		}
		offset = int(req.Msg.GetOffset())
		if offset < 0 {
			return nil, connectError(core.ErrInvalidArgument)
		}
	}
	users, err := s.api.core.AdminListUsers(ctx, req.Msg.GetSearch(), limit, offset)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.ListAdminUsersResponse{
		Users:      apiAdminUsers(users.Users),
		TotalCount: int32(users.TotalCount),
		HasMore:    users.HasMore,
		Page:       apiPageInfo(users.TotalCount, users.HasMore),
	}), nil
}

func (s *adminService) GetUser(ctx context.Context, req *connect.Request[apiv1.GetAdminUserRequest]) (*connect.Response[apiv1.GetAdminUserResponse], error) {
	if _, err := requireAdminCaller(ctx); err != nil {
		return nil, err
	}
	userID := strings.TrimSpace(req.Msg.GetUserId())
	login := strings.TrimSpace(req.Msg.GetLogin())
	if (userID == "") == (login == "") {
		return nil, invalidArgument("provide exactly one of user_id or login")
	}

	var (
		user *core.AdminUserView
		err  error
	)
	if login != "" {
		user, err = s.api.core.AdminGetUserByLogin(ctx, login)
	} else {
		user, err = s.api.core.AdminGetUser(ctx, userID)
	}
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.GetAdminUserResponse{User: apiAdminUser(user)}), nil
}

func (s *adminService) CreateUser(ctx context.Context, req *connect.Request[apiv1.CreateAdminUserRequest]) (*connect.Response[apiv1.CreateAdminUserResponse], error) {
	if _, err := requireAdminCaller(ctx); err != nil {
		return nil, err
	}
	user, err := s.api.core.AdminCreateUser(ctx, core.AdminCreateUserRequest{
		Login:         req.Msg.GetLogin(),
		DisplayName:   req.Msg.GetDisplayName(),
		Password:      req.Msg.GetPassword(),
		VerifiedEmail: req.Msg.GetVerifiedEmail(),
		RoleNames:     req.Msg.GetRoleNames(),
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.CreateAdminUserResponse{User: apiAdminUser(user)}), nil
}

func (s *adminService) UpdateUser(ctx context.Context, req *connect.Request[apiv1.UpdateAdminUserRequest]) (*connect.Response[apiv1.UpdateAdminUserResponse], error) {
	if _, err := requireAdminCaller(ctx); err != nil {
		return nil, err
	}
	if req.Msg.Login == nil && req.Msg.DisplayName == nil {
		return nil, invalidArgument("at least one of login or display_name must be provided")
	}
	user, err := s.api.core.AdminUpdateOperatorUser(ctx, core.AdminUpdateOperatorUserRequest{
		UserID:      req.Msg.GetUserId(),
		Login:       req.Msg.Login,
		DisplayName: req.Msg.DisplayName,
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.UpdateAdminUserResponse{User: apiAdminUser(user)}), nil
}

func (s *adminService) SetUserPassword(ctx context.Context, req *connect.Request[apiv1.SetAdminUserPasswordRequest]) (*connect.Response[apiv1.SetAdminUserPasswordResponse], error) {
	if _, err := requireAdminCaller(ctx); err != nil {
		return nil, err
	}
	user, err := s.api.core.AdminSetUserPassword(ctx, req.Msg.GetUserId(), req.Msg.GetPassword())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.SetAdminUserPasswordResponse{User: apiAdminUser(user)}), nil
}

func (s *adminService) DeleteUser(ctx context.Context, req *connect.Request[apiv1.DeleteAdminUserRequest]) (*connect.Response[apiv1.DeleteAdminUserResponse], error) {
	if _, err := requireAdminCaller(ctx); err != nil {
		return nil, err
	}
	if err := s.api.core.AdminDeleteUser(ctx, req.Msg.GetUserId()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.DeleteAdminUserResponse{}), nil
}

func (s *adminService) AddUserVerifiedEmail(ctx context.Context, req *connect.Request[apiv1.AddAdminUserVerifiedEmailRequest]) (*connect.Response[apiv1.AddAdminUserVerifiedEmailResponse], error) {
	if _, err := requireAdminCaller(ctx); err != nil {
		return nil, err
	}
	user, err := s.api.core.AdminAddUserVerifiedEmail(ctx, req.Msg.GetUserId(), req.Msg.GetEmail())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.AddAdminUserVerifiedEmailResponse{User: apiAdminUser(user)}), nil
}

func (s *adminService) AssignUserRole(ctx context.Context, req *connect.Request[apiv1.AssignAdminUserRoleRequest]) (*connect.Response[apiv1.AssignAdminUserRoleResponse], error) {
	if _, err := requireAdminCaller(ctx); err != nil {
		return nil, err
	}
	user, err := s.api.core.AdminAssignUserRole(ctx, req.Msg.GetUserId(), req.Msg.GetRoleName())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.AssignAdminUserRoleResponse{User: apiAdminUser(user)}), nil
}

func (s *adminService) RevokeUserRole(ctx context.Context, req *connect.Request[apiv1.RevokeAdminUserRoleRequest]) (*connect.Response[apiv1.RevokeAdminUserRoleResponse], error) {
	if _, err := requireAdminCaller(ctx); err != nil {
		return nil, err
	}
	user, err := s.api.core.AdminRevokeUserRole(ctx, req.Msg.GetUserId(), req.Msg.GetRoleName())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.RevokeAdminUserRoleResponse{User: apiAdminUser(user)}), nil
}

func apiAdminUsers(users []*core.AdminUserView) []*apiv1.AdminUser {
	apiUsers := make([]*apiv1.AdminUser, 0, len(users))
	for _, user := range users {
		apiUsers = append(apiUsers, apiAdminUser(user))
	}
	return apiUsers
}

func apiAdminUser(user *core.AdminUserView) *apiv1.AdminUser {
	if user == nil || user.User == nil {
		return nil
	}
	apiEmails := make([]*apiv1.AdminVerifiedEmail, 0, len(user.VerifiedEmails))
	for _, email := range user.VerifiedEmails {
		apiEmails = append(apiEmails, &apiv1.AdminVerifiedEmail{
			Email:      email.Email,
			VerifiedAt: timestamppb.New(email.VerifiedAt),
		})
	}
	return &apiv1.AdminUser{
		UserId:         user.User.GetId(),
		Login:          user.User.GetLogin(),
		DisplayName:    user.User.GetDisplayName(),
		CreatedAt:      user.User.GetCreatedAt(),
		RoleNames:      append([]string(nil), user.RoleNames...),
		VerifiedEmails: apiEmails,
	}
}

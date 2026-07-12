package connectapi

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	operatorv1 "hmans.de/chatto/internal/pb/chatto/operator/v1"
)

type operatorUserService struct {
	api *API
}

func (s *operatorUserService) CreateUser(ctx context.Context, req *connect.Request[operatorv1.CreateUserRequest]) (*connect.Response[operatorv1.CreateUserResponse], error) {
	if strings.TrimSpace(req.Msg.GetLogin()) == "" {
		return nil, invalidArgument("login is required")
	}
	created, err := s.api.core.AdminCreateUserAs(ctx, core.SystemActorID, core.AdminCreateUserRequest{
		Login:         req.Msg.GetLogin(),
		DisplayName:   req.Msg.GetDisplayName(),
		Password:      req.Msg.GetPassword(),
		VerifiedEmail: req.Msg.GetVerifiedEmail(),
		RoleNames:     req.Msg.GetRoleNames(),
	})
	if err != nil {
		return nil, connectError(err)
	}
	member, err := s.operatorMember(ctx, created)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&operatorv1.CreateUserResponse{Member: member}), nil
}

func (s *operatorUserService) ListUsers(ctx context.Context, req *connect.Request[operatorv1.ListUsersRequest]) (*connect.Response[operatorv1.ListUsersResponse], error) {
	limit, offset := apiPagination(req.Msg.GetPage(), defaultAdminMemberLimit, maxAdminMemberLimit)
	users, err := s.api.core.AdminListUsers(ctx, req.Msg.GetSearch(), limit, offset)
	if err != nil {
		return nil, connectError(err)
	}
	response := &operatorv1.ListUsersResponse{
		Users: make([]*adminv1.AdminMember, 0, len(users.Users)),
		Roles: []*apiv1.Role{},
		Page:  apiPageInfo(users.TotalCount, users.HasMore),
	}
	for _, user := range users.Users {
		member, err := s.operatorMember(ctx, user)
		if err != nil {
			return nil, err
		}
		response.Users = append(response.Users, member)
	}
	roles, err := s.api.core.ListServerRoles(ctx)
	if err != nil {
		return nil, connectError(err)
	}
	response.Roles = make([]*apiv1.Role, 0, len(roles))
	for i := range roles {
		response.Roles = append(response.Roles, publicAPIRole(&roles[i]))
	}
	return connect.NewResponse(response), nil
}

func (s *operatorUserService) GetUser(ctx context.Context, req *connect.Request[operatorv1.GetUserRequest]) (*connect.Response[operatorv1.GetUserResponse], error) {
	userID := strings.TrimSpace(req.Msg.GetUserId())
	login := strings.TrimSpace(req.Msg.GetLogin())
	email := strings.TrimSpace(req.Msg.GetEmail())
	if nonEmptyCount(userID, login, email) != 1 {
		return nil, invalidArgument("provide exactly one of user_id, login, or email")
	}
	if login != "" {
		user, err := s.api.core.GetUserByLogin(ctx, login)
		if err != nil {
			return nil, connectError(err)
		}
		userID = user.GetId()
	}
	if email != "" {
		user, err := s.api.core.GetUserByVerifiedEmail(ctx, email)
		if err != nil {
			return nil, connectError(err)
		}
		userID = user.GetId()
	}
	user, err := s.api.core.AdminGetUser(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	member, err := s.operatorMember(ctx, user)
	if err != nil {
		return nil, err
	}
	roles, err := s.api.core.ListServerRoles(ctx)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&operatorv1.GetUserResponse{
		Member:               member,
		Roles:                operatorAdminMemberRoles(roles),
		AvailablePermissions: corePermissionsToStrings(s.api.core.AllServerPermissions()),
	}), nil
}

func (s *operatorUserService) AssignRole(ctx context.Context, req *connect.Request[operatorv1.AssignRoleRequest]) (*connect.Response[operatorv1.AssignRoleResponse], error) {
	if req.Msg.GetUserId() == "" {
		return nil, invalidArgument("user_id is required")
	}
	if req.Msg.GetRoleName() == "" {
		return nil, invalidArgument("role_name is required")
	}
	user, err := s.api.core.AdminAssignUserRole(ctx, req.Msg.GetUserId(), req.Msg.GetRoleName())
	if err != nil {
		return nil, connectError(err)
	}
	member, err := s.operatorMember(ctx, user)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&operatorv1.AssignRoleResponse{Member: member}), nil
}

func (s *operatorUserService) RevokeRole(ctx context.Context, req *connect.Request[operatorv1.RevokeRoleRequest]) (*connect.Response[operatorv1.RevokeRoleResponse], error) {
	if req.Msg.GetUserId() == "" {
		return nil, invalidArgument("user_id is required")
	}
	if req.Msg.GetRoleName() == "" {
		return nil, invalidArgument("role_name is required")
	}
	user, err := s.api.core.AdminRevokeUserRole(ctx, req.Msg.GetUserId(), req.Msg.GetRoleName())
	if err != nil {
		return nil, connectError(err)
	}
	member, err := s.operatorMember(ctx, user)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&operatorv1.RevokeRoleResponse{Member: member}), nil
}

func (s *operatorUserService) UpdateUser(ctx context.Context, req *connect.Request[operatorv1.UpdateUserRequest]) (*connect.Response[operatorv1.UpdateUserResponse], error) {
	if req.Msg.GetUserId() == "" {
		return nil, invalidArgument("user_id is required")
	}
	updated, err := s.api.core.AdminUpdateOperatorUser(ctx, core.AdminUpdateOperatorUserRequest{
		UserID:      req.Msg.GetUserId(),
		Login:       req.Msg.Login,
		DisplayName: req.Msg.DisplayName,
	})
	if err != nil {
		return nil, connectError(err)
	}
	member, err := s.operatorMember(ctx, updated)
	if err != nil {
		return nil, err
	}
	user, err := (&accountService{api: s.api}).accountUser(ctx, updated.User)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&operatorv1.UpdateUserResponse{User: user, Member: member}), nil
}

func (s *operatorUserService) SetUserPassword(ctx context.Context, req *connect.Request[operatorv1.SetUserPasswordRequest]) (*connect.Response[operatorv1.SetUserPasswordResponse], error) {
	updated, err := s.api.core.AdminSetUserPasswordAs(ctx, core.SystemActorID, req.Msg.GetUserId(), req.Msg.GetPassword())
	if err != nil {
		return nil, connectError(err)
	}
	member, err := s.operatorMember(ctx, updated)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&operatorv1.SetUserPasswordResponse{Member: member}), nil
}

func (s *operatorUserService) DeleteUser(ctx context.Context, req *connect.Request[operatorv1.DeleteUserRequest]) (*connect.Response[operatorv1.DeleteUserResponse], error) {
	if err := s.api.core.AdminDeleteUserAs(ctx, core.SystemActorID, req.Msg.GetUserId()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&operatorv1.DeleteUserResponse{Deleted: true}), nil
}

func (s *operatorUserService) AddVerifiedEmail(ctx context.Context, req *connect.Request[operatorv1.AddVerifiedEmailRequest]) (*connect.Response[operatorv1.AddVerifiedEmailResponse], error) {
	updated, err := s.api.core.AdminAddUserVerifiedEmailAs(ctx, core.SystemActorID, req.Msg.GetUserId(), req.Msg.GetEmail())
	if err != nil {
		return nil, connectError(err)
	}
	member, err := s.operatorMember(ctx, updated)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&operatorv1.AddVerifiedEmailResponse{Member: member}), nil
}

func (s *operatorUserService) ClearUsernameCooldown(ctx context.Context, req *connect.Request[operatorv1.ClearUsernameCooldownRequest]) (*connect.Response[operatorv1.ClearUsernameCooldownResponse], error) {
	if req.Msg.GetUserId() == "" {
		return nil, invalidArgument("user_id is required")
	}
	if err := s.api.core.AdminClearUserLoginChangeCooldown(ctx, req.Msg.GetUserId()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&operatorv1.ClearUsernameCooldownResponse{Cleared: true}), nil
}

func (s *operatorUserService) operatorMember(ctx context.Context, user *core.AdminUserView) (*adminv1.AdminMember, error) {
	return (&adminUserManagementService{api: s.api}).adminMemberForOperator(ctx, user)
}

func operatorAdminMemberRoles(roles []core.RoleWithPermissions) []*adminv1.AdminRole {
	out := make([]*adminv1.AdminRole, 0, len(roles))
	for i := range roles {
		out = append(out, adminAPIRole(&roles[i]))
	}
	return out
}

func nonEmptyCount(values ...string) int {
	count := 0
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			count++
		}
	}
	return count
}

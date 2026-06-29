package connectapi

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

const (
	defaultAdminMemberLimit = 20
	maxAdminMemberLimit     = 100
)

type adminUserManagementService struct {
	api *API
}

func (s *adminUserManagementService) CreateUser(ctx context.Context, req *connect.Request[adminv1.CreateUserRequest]) (*connect.Response[adminv1.CreateUserResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Msg.GetLogin()) == "" {
		return nil, invalidArgument("login is required")
	}
	if !caller.IsSystem {
		canAssign, err := s.api.core.CanAssignRoles(ctx, caller.UserID)
		if err != nil {
			return nil, connectError(err)
		}
		if !canAssign {
			return nil, connectError(core.ErrPermissionDenied)
		}
	}
	created, err := s.api.core.AdminCreateUserAs(ctx, caller.UserID, core.AdminCreateUserRequest{
		Login:         req.Msg.GetLogin(),
		DisplayName:   req.Msg.GetDisplayName(),
		Password:      req.Msg.GetPassword(),
		VerifiedEmail: req.Msg.GetVerifiedEmail(),
		RoleNames:     req.Msg.GetRoleNames(),
	})
	if err != nil {
		return nil, connectError(err)
	}
	member, err := s.adminMemberForCaller(ctx, caller, created)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.CreateUserResponse{Member: member}), nil
}

func (s *adminUserManagementService) ListMembers(ctx context.Context, req *connect.Request[adminv1.ListMembersRequest]) (*connect.Response[adminv1.ListMembersResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := apiPagination(req.Msg.GetPage(), defaultAdminMemberLimit, maxAdminMemberLimit)
	if caller.IsSystem {
		users, err := s.api.core.AdminListUsers(ctx, req.Msg.GetSearch(), limit, offset)
		if err != nil {
			return nil, connectError(err)
		}
		response := &adminv1.ListMembersResponse{
			Users: make([]*adminv1.AdminMember, 0, len(users.Users)),
			Roles: []*adminv1.AdminRoleReference{},
			Page:  apiPageInfo(users.TotalCount, users.HasMore),
		}
		for _, user := range users.Users {
			member, err := s.adminMemberForOperator(ctx, user)
			if err != nil {
				return nil, err
			}
			response.Users = append(response.Users, member)
		}
		roles, err := s.api.core.ListServerRoles(ctx)
		if err != nil {
			return nil, connectError(err)
		}
		response.Roles = make([]*adminv1.AdminRoleReference, 0, len(roles))
		for _, role := range roles {
			response.Roles = append(response.Roles, &adminv1.AdminRoleReference{
				Name:        role.Name,
				DisplayName: role.DisplayName,
			})
		}
		return connect.NewResponse(response), nil
	}
	members, err := s.api.core.ListAdminMembers(ctx, caller.UserID, core.AdminMemberListInput{
		Search: req.Msg.GetSearch(),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, connectError(err)
	}
	response := &adminv1.ListMembersResponse{
		Users: make([]*adminv1.AdminMember, 0, len(members.Users)),
		Roles: make([]*adminv1.AdminRoleReference, 0, len(members.Roles)),
		Page:  apiPageInfo(members.TotalCount, members.HasMore),
	}
	for _, user := range members.Users {
		response.Users = append(response.Users, s.adminMember(ctx, user))
	}
	for _, role := range members.Roles {
		response.Roles = append(response.Roles, &adminv1.AdminRoleReference{
			Name:        role.Name,
			DisplayName: role.DisplayName,
		})
	}
	return connect.NewResponse(response), nil
}

func (s *adminUserManagementService) GetMember(ctx context.Context, req *connect.Request[adminv1.GetMemberRequest]) (*connect.Response[adminv1.GetMemberResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	userID := strings.TrimSpace(req.Msg.GetUserId())
	login := strings.TrimSpace(req.Msg.GetLogin())
	if (userID == "") == (login == "") {
		return nil, invalidArgument("provide exactly one of user_id or login")
	}
	if login != "" {
		user, err := s.api.core.GetUserByLogin(ctx, login)
		if err != nil {
			return nil, connectError(err)
		}
		userID = user.GetId()
	}
	if caller.IsSystem {
		user, err := s.api.core.AdminGetUser(ctx, userID)
		if err != nil {
			return nil, connectError(err)
		}
		member, err := s.adminMemberForOperator(ctx, user)
		if err != nil {
			return nil, err
		}
		roles, err := s.api.core.ListServerRoles(ctx)
		if err != nil {
			return nil, connectError(err)
		}
		return connect.NewResponse(&adminv1.GetMemberResponse{
			Member:                         member,
			Roles:                          adminMemberRolesFromCore(roles),
			AvailablePermissions:           corePermissionsToStrings(s.api.core.AllServerPermissions()),
			ViewerCanAssignRoles:           true,
			ViewerCanManageRoles:           true,
			ViewerCanManageUserPermissions: true,
		}), nil
	}
	details, err := s.api.core.GetAdminMemberDetails(ctx, caller.UserID, userID)
	if err != nil {
		return nil, connectError(err)
	}
	response := &adminv1.GetMemberResponse{
		Member:                         s.adminMember(ctx, *details.Member),
		Roles:                          make([]*adminv1.AdminMemberRole, 0, len(details.Roles)),
		AvailablePermissions:           corePermissionsToStrings(details.AvailablePermissions),
		ViewerCanAssignRoles:           details.ViewerCanAssignRoles,
		ViewerCanManageRoles:           details.ViewerCanManageRoles,
		ViewerCanManageUserPermissions: details.ViewerCanManageUserPermissions,
	}
	for _, role := range details.Roles {
		response.Roles = append(response.Roles, &adminv1.AdminMemberRole{
			Name:              role.Name,
			DisplayName:       role.DisplayName,
			Position:          role.Position,
			Permissions:       corePermissionsToStrings(role.Permissions),
			PermissionDenials: corePermissionsToStrings(role.PermissionDenials),
		})
	}
	return connect.NewResponse(response), nil
}

func (s *adminUserManagementService) AssignRole(ctx context.Context, req *connect.Request[adminv1.AssignRoleRequest]) (*connect.Response[adminv1.AssignRoleResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.GetUserId() == "" {
		return nil, invalidArgument("user_id is required")
	}
	if req.Msg.GetRoleName() == "" {
		return nil, invalidArgument("role_name is required")
	}
	var member *adminv1.AdminMember
	if caller.IsSystem {
		user, err := s.api.core.AdminAssignUserRole(ctx, req.Msg.GetUserId(), req.Msg.GetRoleName())
		if err != nil {
			return nil, connectError(err)
		}
		member, err = s.adminMemberForOperator(ctx, user)
		if err != nil {
			return nil, err
		}
	} else {
		if err := s.api.core.AdminAssignServerRole(ctx, caller.UserID, req.Msg.GetUserId(), req.Msg.GetRoleName()); err != nil {
			return nil, connectError(err)
		}
		member, err = s.adminMemberForUserCaller(ctx, caller.UserID, req.Msg.GetUserId())
		if err != nil {
			return nil, err
		}
	}
	return connect.NewResponse(&adminv1.AssignRoleResponse{Assigned: true, Member: member}), nil
}

func (s *adminUserManagementService) RevokeRole(ctx context.Context, req *connect.Request[adminv1.RevokeRoleRequest]) (*connect.Response[adminv1.RevokeRoleResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.GetUserId() == "" {
		return nil, invalidArgument("user_id is required")
	}
	if req.Msg.GetRoleName() == "" {
		return nil, invalidArgument("role_name is required")
	}
	var member *adminv1.AdminMember
	if caller.IsSystem {
		user, err := s.api.core.AdminRevokeUserRole(ctx, req.Msg.GetUserId(), req.Msg.GetRoleName())
		if err != nil {
			return nil, connectError(err)
		}
		member, err = s.adminMemberForOperator(ctx, user)
		if err != nil {
			return nil, err
		}
	} else {
		if err := s.api.core.AdminRevokeServerRole(ctx, caller.UserID, req.Msg.GetUserId(), req.Msg.GetRoleName()); err != nil {
			return nil, connectError(err)
		}
		member, err = s.adminMemberForUserCaller(ctx, caller.UserID, req.Msg.GetUserId())
		if err != nil {
			return nil, err
		}
	}
	return connect.NewResponse(&adminv1.RevokeRoleResponse{Revoked: true, Member: member}), nil
}

func (s *adminUserManagementService) UpdateUser(ctx context.Context, req *connect.Request[adminv1.UpdateUserRequest]) (*connect.Response[adminv1.UpdateUserResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.GetUserId() == "" {
		return nil, invalidArgument("user_id is required")
	}
	var (
		updatedMember *adminv1.AdminMember
		updatedUser   *apiv1.User
	)
	if caller.IsSystem {
		updated, err := s.api.core.AdminUpdateOperatorUser(ctx, core.AdminUpdateOperatorUserRequest{
			UserID:      req.Msg.GetUserId(),
			Login:       req.Msg.Login,
			DisplayName: req.Msg.DisplayName,
		})
		if err != nil {
			return nil, connectError(err)
		}
		updatedMember, err = s.adminMemberForOperator(ctx, updated)
		if err != nil {
			return nil, err
		}
		updatedUser, err = (&accountService{api: s.api}).accountUser(ctx, updated.User)
		if err != nil {
			return nil, err
		}
	} else {
		updated, err := s.api.core.AdminUpdateUser(ctx, caller.UserID, req.Msg.GetUserId(), core.AdminUpdateUserInput{
			Login:       req.Msg.Login,
			DisplayName: req.Msg.DisplayName,
		})
		if err != nil {
			return nil, connectError(err)
		}
		updatedMember, err = s.adminMemberForUserCaller(ctx, caller.UserID, updated.GetId())
		if err != nil {
			return nil, err
		}
		updatedUser, err = (&accountService{api: s.api}).accountUser(ctx, updated)
		if err != nil {
			return nil, err
		}
	}
	return connect.NewResponse(&adminv1.UpdateUserResponse{User: updatedUser, Member: updatedMember}), nil
}

func (s *adminUserManagementService) SetUserPassword(ctx context.Context, req *connect.Request[adminv1.SetUserPasswordRequest]) (*connect.Response[adminv1.SetUserPasswordResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if !caller.IsSystem {
		canAssign, err := s.api.core.CanAssignRoles(ctx, caller.UserID)
		if err != nil {
			return nil, connectError(err)
		}
		if !canAssign {
			return nil, connectError(core.ErrPermissionDenied)
		}
	}
	updated, err := s.api.core.AdminSetUserPasswordAs(ctx, caller.UserID, req.Msg.GetUserId(), req.Msg.GetPassword())
	if err != nil {
		return nil, connectError(err)
	}
	member, err := s.adminMemberForCaller(ctx, caller, updated)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.SetUserPasswordResponse{Member: member}), nil
}

func (s *adminUserManagementService) DeleteUser(ctx context.Context, req *connect.Request[adminv1.DeleteUserRequest]) (*connect.Response[adminv1.DeleteUserResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if !caller.IsSystem {
		if caller.UserID == req.Msg.GetUserId() {
			return nil, connectError(core.ErrPermissionDenied)
		}
		canDelete, err := s.api.core.HasServerPermission(ctx, caller.UserID, core.PermUserDeleteAny)
		if err != nil {
			return nil, connectError(err)
		}
		if !canDelete {
			return nil, connectError(core.ErrPermissionDenied)
		}
	}
	if err := s.api.core.AdminDeleteUserAs(ctx, caller.UserID, req.Msg.GetUserId()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.DeleteUserResponse{Deleted: true}), nil
}

func (s *adminUserManagementService) AddVerifiedEmail(ctx context.Context, req *connect.Request[adminv1.AddVerifiedEmailRequest]) (*connect.Response[adminv1.AddVerifiedEmailResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if !caller.IsSystem {
		canAssign, err := s.api.core.CanAssignRoles(ctx, caller.UserID)
		if err != nil {
			return nil, connectError(err)
		}
		if !canAssign {
			return nil, connectError(core.ErrPermissionDenied)
		}
	}
	updated, err := s.api.core.AdminAddUserVerifiedEmailAs(ctx, caller.UserID, req.Msg.GetUserId(), req.Msg.GetEmail())
	if err != nil {
		return nil, connectError(err)
	}
	member, err := s.adminMemberForCaller(ctx, caller, updated)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&adminv1.AddVerifiedEmailResponse{Member: member}), nil
}

func (s *adminUserManagementService) ClearUsernameCooldown(ctx context.Context, req *connect.Request[adminv1.ClearUsernameCooldownRequest]) (*connect.Response[adminv1.ClearUsernameCooldownResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.GetUserId() == "" {
		return nil, invalidArgument("user_id is required")
	}
	if caller.IsSystem {
		if err := s.api.core.AdminClearUserLoginChangeCooldown(ctx, req.Msg.GetUserId()); err != nil {
			return nil, connectError(err)
		}
	} else {
		if err := s.api.core.AdminClearLoginChangeCooldown(ctx, caller.UserID, req.Msg.GetUserId()); err != nil {
			return nil, connectError(err)
		}
	}
	return connect.NewResponse(&adminv1.ClearUsernameCooldownResponse{Cleared: true}), nil
}

func (s *adminUserManagementService) adminMember(ctx context.Context, member core.AdminMember) *adminv1.AdminMember {
	response := &adminv1.AdminMember{
		Roles:                  append([]string{}, member.Roles...),
		CreatedAt:              member.CreatedAt,
		HasVerifiedEmail:       member.HasVerifiedEmail,
		VerifiedEmails:         append([]string{}, member.VerifiedEmails...),
		ViewerCanDeleteAccount: member.ViewerCanDeleteAccount,
		User: &apiv1.User{
			Id:          member.ID,
			Login:       member.Login,
			DisplayName: member.DisplayName,
			Deleted:     member.Deleted,
		},
	}
	if member.AvatarURL != "" {
		response.User.AvatarUrl = stringPtr(s.api.absolutizeAssetURL(ctx, member.AvatarURL))
	}
	if member.LastLoginChange != nil {
		response.LastLoginChange = timestamppb.New(*member.LastLoginChange)
	}
	return response
}

func (s *adminUserManagementService) adminMemberForCaller(ctx context.Context, caller Caller, user *core.AdminUserView) (*adminv1.AdminMember, error) {
	if caller.IsSystem {
		return s.adminMemberForOperator(ctx, user)
	}
	return s.adminMemberForUserCaller(ctx, caller.UserID, user.User.GetId())
}

func (s *adminUserManagementService) adminMemberForUserCaller(ctx context.Context, actorID, userID string) (*adminv1.AdminMember, error) {
	details, err := s.api.core.GetAdminMemberDetails(ctx, actorID, userID)
	if err != nil {
		return nil, connectError(err)
	}
	return s.adminMember(ctx, *details.Member), nil
}

func (s *adminUserManagementService) adminMemberForOperator(ctx context.Context, user *core.AdminUserView) (*adminv1.AdminMember, error) {
	if user == nil || user.User == nil {
		return nil, connectError(core.ErrNotFound)
	}
	avatarURL, err := s.api.core.GetUserAvatarURL(ctx, user.User.GetId(), nil, nil, "")
	if err != nil {
		return nil, connectError(err)
	}
	verifiedEmails := make([]string, 0, len(user.VerifiedEmails))
	for _, email := range user.VerifiedEmails {
		verifiedEmails = append(verifiedEmails, email.Email)
	}
	response := &adminv1.AdminMember{
		Roles:                  append([]string(nil), user.RoleNames...),
		CreatedAt:              user.User.GetCreatedAt(),
		HasVerifiedEmail:       len(verifiedEmails) > 0,
		VerifiedEmails:         verifiedEmails,
		ViewerCanDeleteAccount: true,
		User: &apiv1.User{
			Id:          user.User.GetId(),
			Login:       user.User.GetLogin(),
			DisplayName: user.User.GetDisplayName(),
			Deleted:     user.User.GetDeleted(),
		},
	}
	if avatarURL != "" {
		response.User.AvatarUrl = stringPtr(s.api.absolutizeAssetURL(ctx, avatarURL))
	}
	return response, nil
}

func adminMemberRolesFromCore(roles []core.RoleWithPermissions) []*adminv1.AdminMemberRole {
	result := make([]*adminv1.AdminMemberRole, 0, len(roles))
	for _, role := range roles {
		result = append(result, &adminv1.AdminMemberRole{
			Name:              role.Name,
			DisplayName:       role.DisplayName,
			Position:          role.Position,
			Permissions:       corePermissionsToStrings(role.Permissions),
			PermissionDenials: corePermissionsToStrings(role.PermissionDenials),
		})
	}
	return result
}

func corePermissionsToStrings(perms []core.Permission) []string {
	out := make([]string, 0, len(perms))
	for _, perm := range perms {
		out = append(out, string(perm))
	}
	return out
}

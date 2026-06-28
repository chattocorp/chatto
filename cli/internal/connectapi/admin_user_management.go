package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

const (
	defaultAdminMemberLimit = 20
	maxAdminMemberLimit     = 100
)

type adminUserManagementService struct {
	api *API
}

func (s *adminUserManagementService) ListMembers(ctx context.Context, req *connect.Request[apiv1.ListMembersRequest]) (*connect.Response[apiv1.ListMembersResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	limit, offset := apiPagination(req.Msg.GetPage(), defaultAdminMemberLimit, maxAdminMemberLimit)
	members, err := s.api.core.ListAdminMembers(ctx, caller.UserID, core.AdminMemberListInput{
		Search: req.Msg.GetSearch(),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, connectError(err)
	}
	response := &apiv1.ListMembersResponse{
		Users: make([]*apiv1.AdminMember, 0, len(members.Users)),
		Roles: make([]*apiv1.AdminMemberRoleSummary, 0, len(members.Roles)),
		Page:  apiPageInfo(members.TotalCount, members.HasMore),
	}
	for _, user := range members.Users {
		response.Users = append(response.Users, s.adminMember(ctx, user))
	}
	for _, role := range members.Roles {
		response.Roles = append(response.Roles, &apiv1.AdminMemberRoleSummary{
			Name:        role.Name,
			DisplayName: role.DisplayName,
		})
	}
	return connect.NewResponse(response), nil
}

func (s *adminUserManagementService) GetMember(ctx context.Context, req *connect.Request[apiv1.GetMemberRequest]) (*connect.Response[apiv1.GetMemberResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.GetUserId() == "" {
		return nil, invalidArgument("user_id is required")
	}
	details, err := s.api.core.GetAdminMemberDetails(ctx, caller.UserID, req.Msg.GetUserId())
	if err != nil {
		return nil, connectError(err)
	}
	response := &apiv1.GetMemberResponse{
		Member:                         s.adminMember(ctx, *details.Member),
		Roles:                          make([]*apiv1.AdminMemberRole, 0, len(details.Roles)),
		AvailablePermissions:           corePermissionsToStrings(details.AvailablePermissions),
		ViewerCanAssignRoles:           details.ViewerCanAssignRoles,
		ViewerCanManageRoles:           details.ViewerCanManageRoles,
		ViewerCanManageUserPermissions: details.ViewerCanManageUserPermissions,
	}
	for _, role := range details.Roles {
		response.Roles = append(response.Roles, &apiv1.AdminMemberRole{
			Name:              role.Name,
			DisplayName:       role.DisplayName,
			Position:          role.Position,
			Permissions:       corePermissionsToStrings(role.Permissions),
			PermissionDenials: corePermissionsToStrings(role.PermissionDenials),
		})
	}
	return connect.NewResponse(response), nil
}

func (s *adminUserManagementService) AssignRole(ctx context.Context, req *connect.Request[apiv1.AssignRoleRequest]) (*connect.Response[apiv1.AssignRoleResponse], error) {
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
	if err := s.api.core.AdminAssignServerRole(ctx, caller.UserID, req.Msg.GetUserId(), req.Msg.GetRoleName()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.AssignRoleResponse{Assigned: true}), nil
}

func (s *adminUserManagementService) RevokeRole(ctx context.Context, req *connect.Request[apiv1.RevokeRoleRequest]) (*connect.Response[apiv1.RevokeRoleResponse], error) {
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
	if err := s.api.core.AdminRevokeServerRole(ctx, caller.UserID, req.Msg.GetUserId(), req.Msg.GetRoleName()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.RevokeRoleResponse{Revoked: true}), nil
}

func (s *adminUserManagementService) UpdateUser(ctx context.Context, req *connect.Request[apiv1.UpdateUserRequest]) (*connect.Response[apiv1.UpdateUserResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.GetUserId() == "" {
		return nil, invalidArgument("user_id is required")
	}
	updated, err := s.api.core.AdminUpdateUser(ctx, caller.UserID, req.Msg.GetUserId(), core.AdminUpdateUserInput{
		Login:       req.Msg.Login,
		DisplayName: req.Msg.DisplayName,
	})
	if err != nil {
		return nil, connectError(err)
	}
	user, err := (&accountService{api: s.api}).accountUser(ctx, updated)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.UpdateUserResponse{User: user}), nil
}

func (s *adminUserManagementService) ClearUsernameCooldown(ctx context.Context, req *connect.Request[apiv1.ClearUsernameCooldownRequest]) (*connect.Response[apiv1.ClearUsernameCooldownResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.GetUserId() == "" {
		return nil, invalidArgument("user_id is required")
	}
	if err := s.api.core.AdminClearLoginChangeCooldown(ctx, caller.UserID, req.Msg.GetUserId()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.ClearUsernameCooldownResponse{Cleared: true}), nil
}

func (s *adminUserManagementService) adminMember(ctx context.Context, member core.AdminMember) *apiv1.AdminMember {
	response := &apiv1.AdminMember{
		Roles:                  append([]string{}, member.Roles...),
		CreatedAt:              member.CreatedAt,
		HasVerifiedEmail:       member.HasVerifiedEmail,
		VerifiedEmails:         append([]string{}, member.VerifiedEmails...),
		ViewerCanDeleteAccount: member.ViewerCanDeleteAccount,
		User: &apiv1.UserSummary{
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

func corePermissionsToStrings(perms []core.Permission) []string {
	out := make([]string, 0, len(perms))
	for _, perm := range perms {
		out = append(out, string(perm))
	}
	return out
}

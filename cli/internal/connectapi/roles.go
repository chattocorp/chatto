package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

type roleService struct {
	api *API
}

func (s *roleService) ListRoles(ctx context.Context, _ *connect.Request[adminv1.ListRolesRequest]) (*connect.Response[adminv1.ListRolesResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	catalog, err := s.api.core.ListServerRolesForUser(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.ListRolesResponse{
		Roles:                apiRoles(catalog.Roles),
		ViewerCanManageRoles: catalog.ViewerCanManageRoles,
		ViewerCanAssignRoles: catalog.ViewerCanAssignRoles,
	}), nil
}

func (s *roleService) GetRole(ctx context.Context, req *connect.Request[adminv1.GetRoleRequest]) (*connect.Response[adminv1.GetRoleResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.GetName() == "" {
		return nil, invalidArgument("name is required")
	}
	details, err := s.api.core.GetServerRoleDetails(ctx, caller.UserID, req.Msg.GetName())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.GetRoleResponse{
		Role:                 apiRole(details.Role),
		Users:                apiRoleUsers(details.Users),
		ViewerCanManageRoles: details.ViewerCanManageRoles,
		ViewerCanAssignRoles: details.ViewerCanAssignRoles,
	}), nil
}

func (s *roleService) CreateRole(ctx context.Context, req *connect.Request[adminv1.CreateRoleRequest]) (*connect.Response[adminv1.CreateRoleResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	pingable := req.Msg.GetPingable()
	role, err := s.api.core.AdminCreateServerRole(ctx, caller.UserID, core.AdminRoleInput{
		Name:        req.Msg.GetName(),
		DisplayName: req.Msg.GetDisplayName(),
		Description: req.Msg.GetDescription(),
		Pingable:    &pingable,
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.CreateRoleResponse{Role: apiRole(role)}), nil
}

func (s *roleService) UpdateRole(ctx context.Context, req *connect.Request[adminv1.UpdateRoleRequest]) (*connect.Response[adminv1.UpdateRoleResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	role, err := s.api.core.AdminUpdateServerRole(ctx, caller.UserID, core.AdminRoleInput{
		Name:        req.Msg.GetName(),
		DisplayName: req.Msg.GetDisplayName(),
		Description: req.Msg.GetDescription(),
		Pingable:    req.Msg.Pingable,
	})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.UpdateRoleResponse{Role: apiRole(role)}), nil
}

func (s *roleService) DeleteRole(ctx context.Context, req *connect.Request[adminv1.DeleteRoleRequest]) (*connect.Response[adminv1.DeleteRoleResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.api.core.AdminDeleteServerRole(ctx, caller.UserID, req.Msg.GetName()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.DeleteRoleResponse{Deleted: true}), nil
}

func (s *roleService) ReorderRoles(ctx context.Context, req *connect.Request[adminv1.ReorderRolesRequest]) (*connect.Response[adminv1.ReorderRolesResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	roles, err := s.api.core.AdminReorderServerRoles(ctx, caller.UserID, req.Msg.GetRoleNames())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&adminv1.ReorderRolesResponse{Roles: apiRoles(roles)}), nil
}

func apiRoles(roles []core.RoleWithPermissions) []*adminv1.Role {
	out := make([]*adminv1.Role, 0, len(roles))
	for i := range roles {
		out = append(out, apiRole(&roles[i]))
	}
	return out
}

func apiRole(role *core.RoleWithPermissions) *adminv1.Role {
	if role == nil {
		return nil
	}
	return &adminv1.Role{
		Name:              role.Name,
		DisplayName:       role.DisplayName,
		Description:       role.Description,
		Permissions:       corePermissionsToStrings(role.Permissions),
		PermissionDenials: corePermissionsToStrings(role.PermissionDenials),
		IsSystem:          role.IsSystem,
		Position:          role.Position,
		Pingable:          role.Pingable,
	}
}

func apiRoleUsers(users []core.RoleUserSummary) []*apiv1.User {
	out := make([]*apiv1.User, 0, len(users))
	for _, user := range users {
		out = append(out, &apiv1.User{
			Id:          user.ID,
			Login:       user.Login,
			DisplayName: user.DisplayName,
			Deleted:     user.Deleted,
		})
	}
	return out
}

package http_server

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	wirev1 "hmans.de/chatto/internal/pb/chatto/wire/v1"
)

func (c *wireConn) handleWireListAdminMembers(ctx context.Context, userID, requestID string, body *apiv1.ListAdminMembersRequest) (*apiv1.ListAdminMembersResponse, *wirev1.WireError) {
	limit, offset := wirePaginationArgs(int(body.GetLimit()), int(body.GetOffset()), 20, 100)
	members, totalCount, err := c.server.core.GetServerMembers(ctx, body.GetSearch(), limit, offset)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	views := make([]*apiv1.AdminMemberView, 0, len(members))
	for _, member := range members {
		view, err := c.adminMemberView(ctx, userID, member.UserID)
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		if view != nil {
			views = append(views, view)
		}
	}

	roles, err := c.adminRoleViews(ctx)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	return &apiv1.ListAdminMembersResponse{
		Members:    views,
		Roles:      roles,
		TotalCount: int32(totalCount),
		HasMore:    offset+len(views) < totalCount,
	}, nil
}

func (c *wireConn) handleWireGetAdminMember(ctx context.Context, userID, requestID string, body *apiv1.GetAdminMemberRequest) (*apiv1.GetAdminMemberResponse, *wirev1.WireError) {
	if strings.TrimSpace(body.GetUserId()) == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: user_id is required", errWireInvalidArgument))
	}

	member, err := c.adminMemberView(ctx, userID, body.GetUserId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	roles, err := c.adminRoleViews(ctx)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	permissions := core.PermissionsForScope(core.ScopeServer)
	availablePermissions := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		availablePermissions = append(availablePermissions, string(permission.Permission))
	}

	canAssignRoles, err := c.server.core.CanAssignRoles(ctx, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	canManageRoles, err := c.server.core.CanManageRoles(ctx, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	canManageUserPermissions, err := c.server.core.CanManageUserPermissions(ctx, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	return &apiv1.GetAdminMemberResponse{
		Member:                         member,
		Roles:                          roles,
		AvailablePermissions:           availablePermissions,
		ViewerCanAssignRoles:           canAssignRoles,
		ViewerCanManageRoles:           canManageRoles,
		ViewerCanManageUserPermissions: canManageUserPermissions,
	}, nil
}

func (c *wireConn) handleWireAdminUpdateUser(ctx context.Context, userID, requestID string, body *apiv1.AdminUpdateUserRequest) (*apiv1.AdminUpdateUserResponse, *wirev1.WireError) {
	if body == nil || strings.TrimSpace(body.GetUserId()) == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: user_id is required", errWireInvalidArgument))
	}
	if body.Login == nil && body.DisplayName == nil {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: at least one of login or display_name must be provided", errWireInvalidArgument))
	}
	if err := c.requireWireUserAdminTarget(ctx, userID, body.GetUserId()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	if body.DisplayName != nil {
		if _, err := c.server.core.AdminUpdateUserDisplayName(ctx, body.GetUserId(), body.GetDisplayName()); err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
	}
	if body.Login != nil {
		if _, err := c.server.core.AdminUpdateUserLogin(ctx, body.GetUserId(), body.GetLogin()); err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
	}

	member, err := c.adminMemberView(ctx, userID, body.GetUserId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.AdminUpdateUserResponse{Member: member}, nil
}

func (c *wireConn) handleWireAdminClearUsernameCooldown(ctx context.Context, userID, requestID string, body *apiv1.AdminClearUsernameCooldownRequest) (*apiv1.AdminClearUsernameCooldownResponse, *wirev1.WireError) {
	if strings.TrimSpace(body.GetUserId()) == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: user_id is required", errWireInvalidArgument))
	}
	if err := c.requireWireUserAdminTarget(ctx, userID, body.GetUserId()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if err := c.server.core.ClearLoginChangeCooldown(ctx, body.GetUserId()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	member, err := c.adminMemberView(ctx, userID, body.GetUserId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.AdminClearUsernameCooldownResponse{Member: member}, nil
}

func (c *wireConn) handleWireAssignMemberRole(ctx context.Context, userID, requestID string, body *apiv1.AssignMemberRoleRequest) (*apiv1.AssignMemberRoleResponse, *wirev1.WireError) {
	if strings.TrimSpace(body.GetUserId()) == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: user_id is required", errWireInvalidArgument))
	}
	if strings.TrimSpace(body.GetRoleName()) == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: role_name is required", errWireInvalidArgument))
	}
	if err := c.requireWireCanManageServerUsers(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	if err := c.server.core.AssignServerRole(ctx, userID, body.GetUserId(), body.GetRoleName()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	member, err := c.adminMemberView(ctx, userID, body.GetUserId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.AssignMemberRoleResponse{Member: member}, nil
}

func (c *wireConn) handleWireRevokeMemberRole(ctx context.Context, userID, requestID string, body *apiv1.RevokeMemberRoleRequest) (*apiv1.RevokeMemberRoleResponse, *wirev1.WireError) {
	if strings.TrimSpace(body.GetUserId()) == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: user_id is required", errWireInvalidArgument))
	}
	if strings.TrimSpace(body.GetRoleName()) == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: role_name is required", errWireInvalidArgument))
	}
	if err := c.requireWireCanManageServerUsers(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if userID == body.GetUserId() {
		switch body.GetRoleName() {
		case core.RoleOwner:
			return nil, c.errorFromRequestErr(requestID, fmt.Errorf("cannot revoke your own owner role"))
		case core.RoleAdmin:
			return nil, c.errorFromRequestErr(requestID, fmt.Errorf("cannot revoke your own admin role"))
		}
	}

	if err := c.server.core.RevokeServerRole(ctx, userID, body.GetUserId(), body.GetRoleName()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	member, err := c.adminMemberView(ctx, userID, body.GetUserId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.RevokeMemberRoleResponse{Member: member}, nil
}

func (c *wireConn) adminMemberView(ctx context.Context, viewerID, userID string) (*apiv1.AdminMemberView, error) {
	user, err := c.server.core.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	avatarURL, err := c.server.core.GetUserAvatarURL(ctx, userID, nil, nil, "")
	if err != nil {
		return nil, err
	}
	roles, err := c.server.core.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}

	view := &apiv1.AdminMemberView{
		User:      cloneUser(user),
		AvatarUrl: avatarURL,
		Roles:     append([]string(nil), roles...),
	}
	canViewCooldown, err := c.canWireViewMemberCooldown(ctx, viewerID, userID)
	if err != nil {
		return nil, err
	}
	if canViewCooldown {
		lastLoginChange, err := c.server.core.GetLastLoginChange(ctx, userID)
		if err != nil {
			return nil, err
		}
		if !lastLoginChange.IsZero() {
			view.LastLoginChange = timestamppb.New(lastLoginChange)
		}
	}
	return view, nil
}

func (c *wireConn) adminRoleViews(ctx context.Context) ([]*apiv1.AdminRoleView, error) {
	roles, err := c.server.core.ListServerRoles(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]*apiv1.AdminRoleView, 0, len(roles))
	for _, role := range roles {
		out = append(out, adminRoleView(role))
	}
	return out, nil
}

func adminRoleView(role core.RoleWithPermissions) *apiv1.AdminRoleView {
	return &apiv1.AdminRoleView{
		Name:              role.Name,
		DisplayName:       role.DisplayName,
		Description:       role.Description,
		Permissions:       permissionsToStrings(role.Permissions),
		PermissionDenials: permissionsToStrings(role.PermissionDenials),
		IsSystem:          role.IsSystem,
		Position:          role.Position,
		Pingable:          role.Pingable,
	}
}

func permissionsToStrings(permissions []core.Permission) []string {
	out := make([]string, 0, len(permissions))
	for _, permission := range permissions {
		out = append(out, string(permission))
	}
	return out
}

func (c *wireConn) canWireViewMemberCooldown(ctx context.Context, viewerID, targetID string) (bool, error) {
	if viewerID == targetID {
		return true, nil
	}
	return c.isWireServerAdmin(ctx, viewerID)
}

func (c *wireConn) requireWireUserAdminTarget(ctx context.Context, callerID, targetID string) error {
	if callerID == targetID {
		return nil
	}
	return c.requireWireCanManageServerUsers(ctx, callerID)
}

func (c *wireConn) requireWireCanManageServerUsers(ctx context.Context, userID string) error {
	canManage, err := c.server.core.HasServerPermission(ctx, userID, core.PermRoleAssign)
	if err != nil {
		return fmt.Errorf("failed to check admin permission: %w", err)
	}
	if !canManage {
		return core.ErrPermissionDenied
	}
	return nil
}

func (c *wireConn) isWireServerAdmin(ctx context.Context, userID string) (bool, error) {
	isOwner, err := c.server.core.IsServerOwner(ctx, userID)
	if err != nil {
		return false, err
	}
	if isOwner {
		return true, nil
	}
	return c.server.core.IsServerAdmin(ctx, userID)
}

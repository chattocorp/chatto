package http_server

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	wirev1 "hmans.de/chatto/internal/pb/chatto/wire/v1"
)

func (c *wireConn) handleWireGetAdminRoleCapabilities(ctx context.Context, userID, requestID string) (*apiv1.GetAdminRoleCapabilitiesResponse, *wirev1.WireError) {
	canManage, canAssign, err := c.adminRoleCapabilities(ctx, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.GetAdminRoleCapabilitiesResponse{
		ViewerCanManageRoles: canManage,
		ViewerCanAssignRoles: canAssign,
	}, nil
}

func (c *wireConn) handleWireGetAdminRole(ctx context.Context, userID, requestID string, body *apiv1.GetAdminRoleRequest) (*apiv1.GetAdminRoleResponse, *wirev1.WireError) {
	roleName := strings.TrimSpace(body.GetName())
	if roleName == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: name is required", errWireInvalidArgument))
	}

	canManage, canAssign, err := c.adminRoleCapabilities(ctx, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	resp := &apiv1.GetAdminRoleResponse{
		ViewerCanManageRoles: canManage,
		ViewerCanAssignRoles: canAssign,
	}

	role, err := c.server.core.GetServerRole(ctx, roleName)
	if err != nil {
		if errors.Is(err, core.ErrRoleNotFound) {
			return resp, nil
		}
		return nil, c.errorFromRequestErr(requestID, err)
	}
	resp.Role = adminRoleView(*role)

	if canAssign {
		users, err := c.adminRoleUsers(ctx, roleName)
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		resp.Users = users
	}

	return resp, nil
}

func (c *wireConn) handleWireCreateAdminRole(ctx context.Context, userID, requestID string, body *apiv1.CreateAdminRoleRequest) (*apiv1.CreateAdminRoleResponse, *wirev1.WireError) {
	if err := c.requireWireCanManageServerRoles(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	role, err := c.server.core.CreateServerRole(ctx, userID, body.GetName(), body.GetDisplayName(), body.GetDescription(), body.GetPingable())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.CreateAdminRoleResponse{Role: adminRoleView(*role)}, nil
}

func (c *wireConn) handleWireUpdateAdminRole(ctx context.Context, userID, requestID string, body *apiv1.UpdateAdminRoleRequest) (*apiv1.UpdateAdminRoleResponse, *wirev1.WireError) {
	if strings.TrimSpace(body.GetName()) == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: name is required", errWireInvalidArgument))
	}
	if err := c.requireWireCanManageServerRoles(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	var role *core.RoleWithPermissions
	var err error
	if body.Pingable != nil {
		role, err = c.server.core.UpdateServerRole(ctx, userID, body.GetName(), body.GetDisplayName(), body.GetDescription(), body.GetPingable())
	} else {
		role, err = c.server.core.UpdateServerRole(ctx, userID, body.GetName(), body.GetDisplayName(), body.GetDescription())
	}
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.UpdateAdminRoleResponse{Role: adminRoleView(*role)}, nil
}

func (c *wireConn) handleWireDeleteAdminRole(ctx context.Context, userID, requestID string, body *apiv1.DeleteAdminRoleRequest) (*apiv1.DeleteAdminRoleResponse, *wirev1.WireError) {
	if strings.TrimSpace(body.GetName()) == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: name is required", errWireInvalidArgument))
	}
	if err := c.requireWireCanManageServerRoles(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if err := c.server.core.DeleteServerRole(ctx, userID, body.GetName()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.DeleteAdminRoleResponse{Deleted: true}, nil
}

func (c *wireConn) adminRoleCapabilities(ctx context.Context, userID string) (canManage bool, canAssign bool, err error) {
	canManage, err = c.server.core.CanManageRoles(ctx, userID)
	if err != nil {
		return false, false, err
	}
	canAssign, err = c.server.core.CanAssignRoles(ctx, userID)
	if err != nil {
		return false, false, err
	}
	return canManage, canAssign, nil
}

func (c *wireConn) adminRoleUsers(ctx context.Context, roleName string) ([]*corev1.User, error) {
	userIDs, err := c.server.core.GetRoleUsers(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("failed to get role users: %w", err)
	}
	users := make([]*corev1.User, 0, len(userIDs))
	for _, userID := range userIDs {
		user, err := c.server.core.GetUser(ctx, userID)
		if err != nil {
			continue
		}
		users = append(users, cloneUser(user))
	}
	return users, nil
}

func (c *wireConn) requireWireCanManageServerRoles(ctx context.Context, userID string) error {
	canManage, err := c.server.core.CanManageRoles(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to check role.manage permission: %w", err)
	}
	if !canManage {
		return core.ErrPermissionDenied
	}
	return nil
}

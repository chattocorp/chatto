package http_server

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	wirev1 "hmans.de/chatto/internal/pb/chatto/wire/v1"
)

func (c *wireConn) handleWireGetRolePermissionTierMatrix(ctx context.Context, userID, requestID string, body *apiv1.GetRolePermissionTierMatrixRequest) (*apiv1.GetRolePermissionTierMatrixResponse, *wirev1.WireError) {
	roomID := strings.TrimSpace(body.GetRoomId())
	groupID := strings.TrimSpace(body.GetGroupId())
	if roomID != "" && groupID != "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: pass room_id OR group_id, not both", errWireInvalidArgument))
	}
	if err := c.requireWireRoleTierMatrixAccess(ctx, userID, roomID, groupID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	matrix, err := c.rolePermissionTierMatrix(ctx, roomID, groupID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.GetRolePermissionTierMatrixResponse{Matrix: matrix}, nil
}

func (c *wireConn) handleWireGetRolePermissionMatrix(ctx context.Context, userID, requestID string, body *apiv1.GetRolePermissionMatrixRequest) (*apiv1.GetRolePermissionMatrixResponse, *wirev1.WireError) {
	roleName := strings.TrimSpace(body.GetRoleName())
	if roleName == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: role_name is required", errWireInvalidArgument))
	}
	if err := c.requireWireCanManageServerRoles(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	matrix, err := c.rolePermissionMatrix(ctx, roleName)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.GetRolePermissionMatrixResponse{Matrix: matrix}, nil
}

func (c *wireConn) handleWireGetUserPermissionMatrix(ctx context.Context, userID, requestID string, body *apiv1.GetUserPermissionMatrixRequest) (*apiv1.GetUserPermissionMatrixResponse, *wirev1.WireError) {
	targetID := strings.TrimSpace(body.GetUserId())
	if targetID == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: user_id is required", errWireInvalidArgument))
	}
	if err := c.requireWireCanManageUserPermissions(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	matrix, err := c.userPermissionMatrix(ctx, targetID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.GetUserPermissionMatrixResponse{Matrix: matrix}, nil
}

func (c *wireConn) handleWireSetRolePermissionState(ctx context.Context, userID, requestID string, body *apiv1.SetRolePermissionStateRequest) (*apiv1.SetPermissionStateResponse, *wirev1.WireError) {
	roleName := strings.TrimSpace(body.GetRoleName())
	perm := core.Permission(strings.TrimSpace(body.GetPermission()))
	roomID := strings.TrimSpace(body.GetRoomId())
	groupID := strings.TrimSpace(body.GetGroupId())
	if roleName == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: role_name is required", errWireInvalidArgument))
	}
	if perm == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: permission is required", errWireInvalidArgument))
	}
	if roomID != "" && groupID != "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: pass room_id OR group_id, not both", errWireInvalidArgument))
	}
	if err := rejectWireOwnerRolePermissionEdit(roleName); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	if roomID != "" {
		if err := c.requireWireRoomPermissionManage(ctx, userID, roomID); err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		return c.setRoleRoomPermissionState(ctx, userID, roomID, roleName, perm, body.GetState(), requestID)
	}
	if groupID != "" {
		if err := c.requireWireRoleManage(ctx, userID); err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		return c.setRoleGroupPermissionState(ctx, userID, groupID, roleName, perm, body.GetState(), requestID)
	}
	if err := c.requireWireRoleManage(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return c.setRoleServerPermissionState(ctx, userID, roleName, perm, body.GetState(), requestID)
}

func (c *wireConn) handleWireSetUserPermissionState(ctx context.Context, userID, requestID string, body *apiv1.SetUserPermissionStateRequest) (*apiv1.SetPermissionStateResponse, *wirev1.WireError) {
	targetID := strings.TrimSpace(body.GetUserId())
	perm := core.Permission(strings.TrimSpace(body.GetPermission()))
	roomID := strings.TrimSpace(body.GetRoomId())
	groupID := strings.TrimSpace(body.GetGroupId())
	if targetID == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: user_id is required", errWireInvalidArgument))
	}
	if perm == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: permission is required", errWireInvalidArgument))
	}
	if roomID != "" && groupID != "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: pass room_id OR group_id, not both", errWireInvalidArgument))
	}
	if err := c.requireWireCanManageUserPermissions(ctx, userID); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	var err error
	switch body.GetState() {
	case apiv1.PermissionEditState_PERMISSION_EDIT_STATE_ALLOW:
		switch {
		case roomID != "":
			err = c.server.core.GrantUserRoomPermission(ctx, userID, roomID, targetID, perm)
		case groupID != "":
			err = c.server.core.GrantUserGroupPermission(ctx, userID, groupID, targetID, perm)
		default:
			err = c.server.core.GrantUserPermission(ctx, userID, targetID, perm)
		}
	case apiv1.PermissionEditState_PERMISSION_EDIT_STATE_DENY:
		switch {
		case roomID != "":
			err = c.server.core.DenyUserRoomPermission(ctx, userID, roomID, targetID, perm)
		case groupID != "":
			err = c.server.core.DenyUserGroupPermission(ctx, userID, groupID, targetID, perm)
		default:
			err = c.server.core.DenyUserPermission(ctx, userID, targetID, perm)
		}
	case apiv1.PermissionEditState_PERMISSION_EDIT_STATE_NEUTRAL:
		switch {
		case roomID != "":
			err = c.server.core.ClearUserRoomPermissionState(ctx, userID, roomID, targetID, perm)
		case groupID != "":
			err = c.server.core.ClearUserGroupPermissionState(ctx, userID, groupID, targetID, perm)
		default:
			err = c.server.core.ClearUserPermissionState(ctx, userID, targetID, perm)
		}
	default:
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: state is required", errWireInvalidArgument))
	}
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.SetPermissionStateResponse{Changed: true}, nil
}

func (c *wireConn) setRoleServerPermissionState(ctx context.Context, userID, roleName string, perm core.Permission, state apiv1.PermissionEditState, requestID string) (*apiv1.SetPermissionStateResponse, *wirev1.WireError) {
	var err error
	switch state {
	case apiv1.PermissionEditState_PERMISSION_EDIT_STATE_ALLOW:
		err = c.server.core.GrantServerPermission(ctx, userID, roleName, perm)
	case apiv1.PermissionEditState_PERMISSION_EDIT_STATE_DENY:
		err = c.server.core.DenyServerPermission(ctx, userID, roleName, perm)
	case apiv1.PermissionEditState_PERMISSION_EDIT_STATE_NEUTRAL:
		err = c.server.core.ClearServerPermissionState(ctx, userID, roleName, perm)
	default:
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: state is required", errWireInvalidArgument))
	}
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.SetPermissionStateResponse{Changed: true}, nil
}

func (c *wireConn) setRoleGroupPermissionState(ctx context.Context, userID, groupID, roleName string, perm core.Permission, state apiv1.PermissionEditState, requestID string) (*apiv1.SetPermissionStateResponse, *wirev1.WireError) {
	var err error
	switch state {
	case apiv1.PermissionEditState_PERMISSION_EDIT_STATE_ALLOW:
		err = c.server.core.GrantGroupPermission(ctx, userID, groupID, roleName, perm)
	case apiv1.PermissionEditState_PERMISSION_EDIT_STATE_DENY:
		err = c.server.core.DenyGroupPermission(ctx, userID, groupID, roleName, perm)
	case apiv1.PermissionEditState_PERMISSION_EDIT_STATE_NEUTRAL:
		err = c.server.core.ClearGroupPermissionState(ctx, userID, groupID, roleName, perm)
	default:
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: state is required", errWireInvalidArgument))
	}
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.SetPermissionStateResponse{Changed: true}, nil
}

func (c *wireConn) setRoleRoomPermissionState(ctx context.Context, userID, roomID, roleName string, perm core.Permission, state apiv1.PermissionEditState, requestID string) (*apiv1.SetPermissionStateResponse, *wirev1.WireError) {
	var err error
	switch state {
	case apiv1.PermissionEditState_PERMISSION_EDIT_STATE_ALLOW:
		err = c.server.core.GrantRoomPermission(ctx, userID, roomID, roleName, perm)
	case apiv1.PermissionEditState_PERMISSION_EDIT_STATE_DENY:
		err = c.server.core.DenyRoomPermission(ctx, userID, roomID, roleName, perm)
	case apiv1.PermissionEditState_PERMISSION_EDIT_STATE_NEUTRAL:
		err = c.server.core.ClearRoomPermissionState(ctx, userID, roomID, roleName, perm)
	default:
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: state is required", errWireInvalidArgument))
	}
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.SetPermissionStateResponse{Changed: true}, nil
}

func (c *wireConn) rolePermissionTierMatrix(ctx context.Context, roomID, groupID string) (*apiv1.RolePermissionTierMatrixView, error) {
	scope := core.ScopeServer
	if roomID != "" {
		scope = core.ScopeRoom
	}
	if groupID != "" {
		scope = core.ScopeGroup
	}

	out := &apiv1.RolePermissionTierMatrixView{}
	for _, meta := range core.PermissionsForScope(scope) {
		out.ApplicablePermissions = append(out.ApplicablePermissions, string(meta.Permission))
	}

	roles, err := c.server.core.ListServerRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list roles: %w", err)
	}
	sort.SliceStable(roles, func(i, j int) bool {
		return roles[i].Position < roles[j].Position
	})
	for _, role := range roles {
		view, err := c.tierRoleView(ctx, role, scope, roomID, groupID)
		if err != nil {
			return nil, err
		}
		out.Roles = append(out.Roles, view)
	}
	return out, nil
}

func (c *wireConn) tierRoleView(ctx context.Context, role core.RoleWithPermissions, scope core.PermissionScope, roomID, groupID string) (*apiv1.TierRoleView, error) {
	out := &apiv1.TierRoleView{
		RoleName:    role.Name,
		DisplayName: role.DisplayName,
		Description: role.Description,
		IsSystem:    role.IsSystem,
		Position:    int32(role.Position),
	}

	serverGrants, err := c.server.core.GetServerRolePermissions(ctx, role.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to load server grants: %w", err)
	}
	serverDenials, err := c.server.core.GetServerRolePermissionDenials(ctx, role.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to load server denials: %w", err)
	}

	if groupID != "" {
		grants, denials, err := c.server.core.GetGroupRolePermissions(ctx, groupID, role.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to load group overrides: %w", err)
		}
		out.Override = tierPermissionsView(grants, denials)
		out.InheritedAllows = wireFilterByScope(serverGrants, core.ScopeGroup)
		out.InheritedDenials = wireFilterByScope(serverDenials, core.ScopeGroup)
		return out, nil
	}

	switch scope {
	case core.ScopeServer:
		out.Override = tierPermissionsView(serverGrants, serverDenials)
	case core.ScopeRoom:
		grants, denials, err := c.server.core.GetRoomRolePermissions(ctx, roomID, role.Name)
		if err != nil {
			return nil, fmt.Errorf("failed to load room overrides: %w", err)
		}
		out.Override = tierPermissionsView(grants, denials)

		roomGroupID, err := c.lookupWireRoomGroupID(ctx, roomID)
		if err != nil {
			return nil, err
		}
		var groupGrants, groupDenials []core.Permission
		if roomGroupID != "" {
			groupGrants, groupDenials, err = c.server.core.GetGroupRolePermissions(ctx, roomGroupID, role.Name)
			if err != nil {
				return nil, fmt.Errorf("failed to load group overrides for inheritance: %w", err)
			}
		}
		out.InheritedAllows, out.InheritedDenials = mergeWireInheritedDecisions(
			groupGrants,
			groupDenials,
			wireScopedPerms(serverGrants, core.ScopeRoom),
			wireScopedPerms(serverDenials, core.ScopeRoom),
		)
	}

	if out.Override == nil {
		out.Override = &apiv1.TierPermissionsView{}
	}
	return out, nil
}

func (c *wireConn) rolePermissionMatrix(ctx context.Context, roleName string) (*apiv1.RolePermissionMatrixView, error) {
	role, err := c.server.core.GetServerRole(ctx, roleName)
	if err != nil {
		if errors.Is(err, core.ErrRoleNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("load role: %w", err)
	}
	if role == nil {
		return nil, nil
	}

	applicable := wireMatrixApplicablePermissions()
	scopes, err := c.wireMatrixScopes(ctx)
	if err != nil {
		return nil, err
	}

	serverGrants, err := c.server.core.GetServerRolePermissions(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("load server grants: %w", err)
	}
	serverDenials, err := c.server.core.GetServerRolePermissionDenials(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("load server denials: %w", err)
	}

	groupGrants := make(map[string][]core.Permission)
	groupDenials := make(map[string][]core.Permission)
	roomGrants := make(map[string][]core.Permission)
	roomDenials := make(map[string][]core.Permission)
	roomToGroup := make(map[string]string)

	for _, scope := range scopes {
		switch scope.GetKind() {
		case apiv1.PermissionMatrixScopeKind_PERMISSION_MATRIX_SCOPE_KIND_GROUP:
			groupID := wireScopeRefID(scope.GetId(), "group:")
			g, d, err := c.server.core.GetGroupRolePermissions(ctx, groupID, roleName)
			if err != nil {
				return nil, fmt.Errorf("load group %s perms: %w", groupID, err)
			}
			groupGrants[groupID] = g
			groupDenials[groupID] = d
		case apiv1.PermissionMatrixScopeKind_PERMISSION_MATRIX_SCOPE_KIND_ROOM:
			roomID := wireScopeRefID(scope.GetId(), "room:")
			g, d, err := c.server.core.GetRoomRolePermissions(ctx, roomID, roleName)
			if err != nil {
				return nil, fmt.Errorf("load room %s perms: %w", roomID, err)
			}
			roomGrants[roomID] = g
			roomDenials[roomID] = d
			roomToGroup[roomID] = scope.GetParentGroupId()
		}
	}

	cells := make([]*apiv1.PermissionMatrixCellView, 0, len(applicable)*len(scopes))
	for _, permStr := range applicable {
		perm := core.Permission(permStr)
		for _, scope := range scopes {
			cell, ok := buildWireRolePermissionCell(
				perm,
				scope,
				serverGrants,
				serverDenials,
				groupGrants,
				groupDenials,
				roomGrants,
				roomDenials,
				roomToGroup,
			)
			if ok {
				cells = append(cells, cell)
			}
		}
	}

	return &apiv1.RolePermissionMatrixView{
		RoleName:              roleName,
		ApplicablePermissions: applicable,
		Scopes:                scopes,
		Cells:                 cells,
	}, nil
}

func buildWireRolePermissionCell(
	perm core.Permission,
	scope *apiv1.PermissionMatrixScopeView,
	serverGrants, serverDenials []core.Permission,
	groupGrants, groupDenials map[string][]core.Permission,
	roomGrants, roomDenials map[string][]core.Permission,
	roomToGroup map[string]string,
) (*apiv1.PermissionMatrixCellView, bool) {
	switch scope.GetKind() {
	case apiv1.PermissionMatrixScopeKind_PERMISSION_MATRIX_SCOPE_KIND_SERVER:
		if !core.PermissionAppliesAtScope(perm, core.ScopeServer) {
			return nil, false
		}
		override := wireDecisionFromLists(perm, serverGrants, serverDenials)
		return &apiv1.PermissionMatrixCellView{
			Permission: string(perm),
			ScopeId:    scope.GetId(),
			Override:   override,
			Effective:  override,
		}, true
	case apiv1.PermissionMatrixScopeKind_PERMISSION_MATRIX_SCOPE_KIND_GROUP:
		if !core.PermissionAppliesAtScope(perm, core.ScopeGroup) {
			return nil, false
		}
		groupID := wireScopeRefID(scope.GetId(), "group:")
		override := wireDecisionFromLists(perm, groupGrants[groupID], groupDenials[groupID])
		effective := override
		if effective == apiv1.PermissionMatrixDecision_PERMISSION_MATRIX_DECISION_NONE && core.PermissionAppliesAtScope(perm, core.ScopeServer) {
			effective = wireDecisionFromLists(perm, serverGrants, serverDenials)
		}
		return &apiv1.PermissionMatrixCellView{
			Permission: string(perm),
			ScopeId:    scope.GetId(),
			Override:   override,
			Effective:  effective,
		}, true
	case apiv1.PermissionMatrixScopeKind_PERMISSION_MATRIX_SCOPE_KIND_ROOM:
		if !core.PermissionAppliesAtScope(perm, core.ScopeRoom) {
			return nil, false
		}
		roomID := wireScopeRefID(scope.GetId(), "room:")
		override := wireDecisionFromLists(perm, roomGrants[roomID], roomDenials[roomID])
		effective := override
		if effective == apiv1.PermissionMatrixDecision_PERMISSION_MATRIX_DECISION_NONE {
			if groupID := roomToGroup[roomID]; groupID != "" && core.PermissionAppliesAtScope(perm, core.ScopeGroup) {
				effective = wireDecisionFromLists(perm, groupGrants[groupID], groupDenials[groupID])
			}
			if effective == apiv1.PermissionMatrixDecision_PERMISSION_MATRIX_DECISION_NONE && core.PermissionAppliesAtScope(perm, core.ScopeServer) {
				effective = wireDecisionFromLists(perm, serverGrants, serverDenials)
			}
		}
		return &apiv1.PermissionMatrixCellView{
			Permission: string(perm),
			ScopeId:    scope.GetId(),
			Override:   override,
			Effective:  effective,
		}, true
	}
	return nil, false
}

func (c *wireConn) userPermissionMatrix(ctx context.Context, userID string) (*apiv1.UserPermissionMatrixView, error) {
	applicable := wireMatrixApplicablePermissions()
	scopes, err := c.wireMatrixScopes(ctx)
	if err != nil {
		return nil, err
	}

	cells := make([]*apiv1.PermissionMatrixCellView, 0, len(applicable)*len(scopes))
	for _, permStr := range applicable {
		perm := core.Permission(permStr)
		for _, scope := range scopes {
			cell, ok, err := c.buildWireUserPermissionCell(ctx, userID, perm, scope)
			if err != nil {
				return nil, err
			}
			if ok {
				cells = append(cells, cell)
			}
		}
	}

	return &apiv1.UserPermissionMatrixView{
		UserId:                userID,
		ApplicablePermissions: applicable,
		Scopes:                scopes,
		Cells:                 cells,
	}, nil
}

func (c *wireConn) buildWireUserPermissionCell(ctx context.Context, userID string, perm core.Permission, scope *apiv1.PermissionMatrixScopeView) (*apiv1.PermissionMatrixCellView, bool, error) {
	var (
		override  core.DecisionKind
		effective core.DecisionKind
		err       error
	)

	switch scope.GetKind() {
	case apiv1.PermissionMatrixScopeKind_PERMISSION_MATRIX_SCOPE_KIND_SERVER:
		if !core.PermissionAppliesAtScope(perm, core.ScopeServer) {
			return nil, false, nil
		}
		override, err = c.server.core.GetUserExplicitServerOverride(ctx, userID, perm)
		if err != nil {
			return nil, false, err
		}
		effective, err = c.server.core.PermResolver().Resolve(ctx, userID, core.KindChannel, "", perm)
		if err != nil {
			return nil, false, err
		}
	case apiv1.PermissionMatrixScopeKind_PERMISSION_MATRIX_SCOPE_KIND_GROUP:
		if !core.PermissionAppliesAtScope(perm, core.ScopeGroup) {
			return nil, false, nil
		}
		groupID := wireScopeRefID(scope.GetId(), "group:")
		override, err = c.server.core.GetUserExplicitGroupOverride(ctx, groupID, userID, perm)
		if err != nil {
			return nil, false, err
		}
		effective, err = c.server.core.PermResolver().ResolveGroup(ctx, userID, core.KindChannel, groupID, perm)
		if err != nil {
			return nil, false, err
		}
	case apiv1.PermissionMatrixScopeKind_PERMISSION_MATRIX_SCOPE_KIND_ROOM:
		if !core.PermissionAppliesAtScope(perm, core.ScopeRoom) {
			return nil, false, nil
		}
		roomID := wireScopeRefID(scope.GetId(), "room:")
		override, err = c.server.core.GetUserExplicitRoomOverride(ctx, roomID, userID, perm)
		if err != nil {
			return nil, false, err
		}
		effective, err = c.server.core.PermResolver().Resolve(ctx, userID, core.KindChannel, roomID, perm)
		if err != nil {
			return nil, false, err
		}
	default:
		return nil, false, fmt.Errorf("unknown scope kind: %v", scope.GetKind())
	}

	return &apiv1.PermissionMatrixCellView{
		Permission: string(perm),
		ScopeId:    scope.GetId(),
		Override:   wireDecisionToProto(override),
		Effective:  wireDecisionToProto(effective),
	}, true, nil
}

type wireMatrixRoomLite struct {
	ID   string
	Name string
}

func (c *wireConn) wireMatrixScopes(ctx context.Context) ([]*apiv1.PermissionMatrixScopeView, error) {
	scopes := []*apiv1.PermissionMatrixScopeView{
		{
			Id:            "server",
			Label:         "Server",
			Kind:          apiv1.PermissionMatrixScopeKind_PERMISSION_MATRIX_SCOPE_KIND_SERVER,
			ParentGroupId: "",
		},
	}

	groups, err := c.server.core.ListRoomGroupsOrdered(ctx, core.KindChannel)
	if err != nil {
		return nil, fmt.Errorf("load room groups: %w", err)
	}
	roomsByGroup := make(map[string][]*wireMatrixRoomLite, len(groups))
	for _, group := range groups {
		scopes = append(scopes, &apiv1.PermissionMatrixScopeView{
			Id:            "group:" + group.GetId(),
			Label:         group.GetName(),
			Kind:          apiv1.PermissionMatrixScopeKind_PERMISSION_MATRIX_SCOPE_KIND_GROUP,
			ParentGroupId: "",
		})
		for _, roomID := range group.GetRoomIds() {
			room, err := c.server.core.GetRoom(ctx, core.KindChannel, roomID)
			if err != nil || room == nil {
				continue
			}
			roomsByGroup[group.GetId()] = append(roomsByGroup[group.GetId()], &wireMatrixRoomLite{
				ID:   room.GetId(),
				Name: room.GetName(),
			})
		}
	}
	for _, group := range groups {
		for _, room := range roomsByGroup[group.GetId()] {
			scopes = append(scopes, &apiv1.PermissionMatrixScopeView{
				Id:            "room:" + room.ID,
				Label:         room.Name,
				Kind:          apiv1.PermissionMatrixScopeKind_PERMISSION_MATRIX_SCOPE_KIND_ROOM,
				ParentGroupId: group.GetId(),
			})
		}
	}
	return scopes, nil
}

func (c *wireConn) requireWireRoleTierMatrixAccess(ctx context.Context, userID, roomID, groupID string) error {
	canManageRoles, err := c.server.core.CanManageRoles(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to check role.manage: %w", err)
	}
	if roomID == "" {
		if !canManageRoles {
			return core.ErrPermissionDenied
		}
		return nil
	}
	if !canManageRoles {
		hasRoomManage, err := c.server.core.PermResolver().HasRoomPermission(ctx, userID, core.KindChannel, roomID, core.PermRoomManage)
		if err != nil {
			return fmt.Errorf("failed to check room.manage: %w", err)
		}
		if !hasRoomManage {
			return core.ErrPermissionDenied
		}
	}
	return c.requireWireRoomExists(ctx, core.KindChannel, roomID)
}

func (c *wireConn) requireWireRoomPermissionManage(ctx context.Context, userID, roomID string) error {
	canManageRoles, err := c.server.core.CanManageRoles(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to check role.manage: %w", err)
	}
	if canManageRoles {
		return nil
	}
	if roomID != "" {
		hasRoomManage, err := c.server.core.PermResolver().HasRoomPermission(ctx, userID, core.KindChannel, roomID, core.PermRoomManage)
		if err != nil {
			return fmt.Errorf("failed to check room.manage: %w", err)
		}
		if hasRoomManage {
			return nil
		}
	}
	return core.ErrPermissionDenied
}

func (c *wireConn) requireWireRoomExists(ctx context.Context, kind core.RoomKind, roomID string) error {
	if roomID == "" {
		return nil
	}
	room, err := c.server.core.GetRoom(ctx, kind, roomID)
	if err != nil || room == nil {
		return core.ErrPermissionDenied
	}
	return nil
}

func (c *wireConn) requireWireCanManageUserPermissions(ctx context.Context, userID string) error {
	canManage, err := c.server.core.CanManageUserPermissions(ctx, userID)
	if err != nil {
		return fmt.Errorf("failed to check user.manage-permissions permission: %w", err)
	}
	if !canManage {
		return core.ErrPermissionDenied
	}
	return nil
}

func (c *wireConn) lookupWireRoomGroupID(ctx context.Context, roomID string) (string, error) {
	if roomID == "" {
		return "", nil
	}
	room, err := c.server.core.GetRoom(ctx, core.KindChannel, roomID)
	if err != nil {
		return "", fmt.Errorf("failed to load room for inheritance lookup: %w", err)
	}
	if room == nil {
		return "", nil
	}
	return room.GetGroupId(), nil
}

func tierPermissionsView(grants, denials []core.Permission) *apiv1.TierPermissionsView {
	return &apiv1.TierPermissionsView{
		Permissions:       permissionsToStrings(grants),
		PermissionDenials: permissionsToStrings(denials),
	}
}

func wireMatrixApplicablePermissions() []string {
	allPerms := core.AllPermissions()
	out := make([]string, 0, len(allPerms))
	for _, meta := range allPerms {
		if core.PermissionAppliesAtScope(meta.Permission, core.ScopeServer) ||
			core.PermissionAppliesAtScope(meta.Permission, core.ScopeGroup) ||
			core.PermissionAppliesAtScope(meta.Permission, core.ScopeRoom) {
			out = append(out, string(meta.Permission))
		}
	}
	return out
}

func wireFilterByScope(perms []core.Permission, scope core.PermissionScope) []string {
	out := make([]string, 0, len(perms))
	for _, perm := range perms {
		if core.PermissionAppliesAtScope(perm, scope) {
			out = append(out, string(perm))
		}
	}
	return out
}

func wireScopedPerms(perms []core.Permission, scope core.PermissionScope) []core.Permission {
	out := make([]core.Permission, 0, len(perms))
	for _, perm := range perms {
		if core.PermissionAppliesAtScope(perm, scope) {
			out = append(out, perm)
		}
	}
	return out
}

func mergeWireInheritedDecisions(overrideAllow, overrideDeny, parentAllow, parentDeny []core.Permission) ([]string, []string) {
	overridden := make(map[core.Permission]struct{}, len(overrideAllow)+len(overrideDeny))
	for _, p := range overrideAllow {
		overridden[p] = struct{}{}
	}
	for _, p := range overrideDeny {
		overridden[p] = struct{}{}
	}

	allow := make([]string, 0, len(overrideAllow)+len(parentAllow))
	for _, p := range overrideAllow {
		allow = append(allow, string(p))
	}
	for _, p := range parentAllow {
		if _, blocked := overridden[p]; blocked {
			continue
		}
		allow = append(allow, string(p))
	}

	deny := make([]string, 0, len(overrideDeny)+len(parentDeny))
	for _, p := range overrideDeny {
		deny = append(deny, string(p))
	}
	for _, p := range parentDeny {
		if _, blocked := overridden[p]; blocked {
			continue
		}
		deny = append(deny, string(p))
	}
	return allow, deny
}

func wireDecisionFromLists(perm core.Permission, grants, denials []core.Permission) apiv1.PermissionMatrixDecision {
	for _, grant := range grants {
		if grant == perm {
			return apiv1.PermissionMatrixDecision_PERMISSION_MATRIX_DECISION_ALLOW
		}
	}
	for _, denial := range denials {
		if denial == perm {
			return apiv1.PermissionMatrixDecision_PERMISSION_MATRIX_DECISION_DENY
		}
	}
	return apiv1.PermissionMatrixDecision_PERMISSION_MATRIX_DECISION_NONE
}

func wireDecisionToProto(decision core.DecisionKind) apiv1.PermissionMatrixDecision {
	switch decision {
	case core.DecisionAllow:
		return apiv1.PermissionMatrixDecision_PERMISSION_MATRIX_DECISION_ALLOW
	case core.DecisionDeny:
		return apiv1.PermissionMatrixDecision_PERMISSION_MATRIX_DECISION_DENY
	default:
		return apiv1.PermissionMatrixDecision_PERMISSION_MATRIX_DECISION_NONE
	}
}

func wireScopeRefID(scopeID, prefix string) string {
	if len(scopeID) <= len(prefix) {
		return ""
	}
	return scopeID[len(prefix):]
}

func rejectWireOwnerRolePermissionEdit(roleName string) error {
	if roleName == core.RoleOwner {
		return fmt.Errorf("owner permissions are granted virtually and cannot be edited")
	}
	return nil
}

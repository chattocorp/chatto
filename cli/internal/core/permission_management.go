package core

import (
	"context"
	"fmt"
	"sort"
)

type MatrixDecision string

const (
	MatrixDecisionAllow MatrixDecision = "ALLOW"
	MatrixDecisionDeny  MatrixDecision = "DENY"
	MatrixDecisionNone  MatrixDecision = "NONE"
)

type MatrixScopeKind string

const (
	MatrixScopeServer MatrixScopeKind = "SERVER"
	MatrixScopeGroup  MatrixScopeKind = "GROUP"
	MatrixScopeRoom   MatrixScopeKind = "ROOM"
)

type PermissionState string

const (
	PermissionStateAllow PermissionState = "allow"
	PermissionStateDeny  PermissionState = "deny"
	PermissionStateNone  PermissionState = "none"
)

type PermissionTargetScope struct {
	Kind MatrixScopeKind
	ID   string
}

type TierPermissions struct {
	Permissions       []string
	PermissionDenials []string
}

type TierRole struct {
	RoleName         string
	DisplayName      string
	Description      string
	IsSystem         bool
	Position         int32
	Override         TierPermissions
	InheritedAllows  []string
	InheritedDenials []string
}

type TierRoles struct {
	ApplicablePermissions []string
	Roles                 []TierRole
}

type PermissionMatrixScope struct {
	ID            string
	Label         string
	Kind          MatrixScopeKind
	ParentGroupID string
}

type PermissionMatrixCell struct {
	Permission string
	ScopeID    string
	Override   MatrixDecision
	Effective  MatrixDecision
}

type RolePermissionMatrix struct {
	RoleName              string
	ApplicablePermissions []string
	Scopes                []PermissionMatrixScope
	Cells                 []PermissionMatrixCell
}

type UserPermissionMatrix struct {
	UserID                string
	ApplicablePermissions []string
	Scopes                []PermissionMatrixScope
	Cells                 []PermissionMatrixCell
}

func (c *ChattoCore) ExplainPermissions(ctx context.Context, actorID, targetUserID, roomID string) ([]PermissionExplanation, error) {
	if actorID == "" {
		return nil, ErrNotAuthenticated
	}
	if targetUserID == "" {
		return nil, fmt.Errorf("%w: user id is required", ErrInvalidArgument)
	}
	if actorID == targetUserID {
		return nil, ErrPermissionDenied
	}
	canManage, err := c.CanManageRoles(ctx, actorID)
	if err != nil {
		return nil, fmt.Errorf("check role.manage: %w", err)
	}
	if !canManage {
		return nil, ErrPermissionDenied
	}
	if roomID != "" {
		if err := c.requirePermissionExplanationRoom(ctx, roomID); err != nil {
			return nil, err
		}
		return c.PermResolver().ExplainAllPermissions(ctx, targetUserID, KindChannel, roomID)
	}
	return c.PermResolver().ExplainAllPermissions(ctx, targetUserID, "", "")
}

type matrixRoomLite struct {
	ID   string
	Name string
}

func (c *ChattoCore) GetRolePermissionTierMatrix(ctx context.Context, actorID, roomID, groupID string) (*TierRoles, error) {
	if roomID != "" && groupID != "" {
		return nil, fmt.Errorf("%w: pass room id OR group id, not both", ErrInvalidArgument)
	}
	if roomID != "" {
		if err := c.requireCanManageRolePermissionsForRoom(ctx, actorID, roomID); err != nil {
			return nil, err
		}
		return c.buildTierRoles(ctx, ScopeRoom, roomID, "")
	}
	if err := c.requireCanManageAdminRoles(ctx, actorID); err != nil {
		return nil, err
	}
	if groupID != "" {
		return c.buildTierRoles(ctx, ScopeGroup, "", groupID)
	}
	return c.buildTierRoles(ctx, ScopeServer, "", "")
}

func (c *ChattoCore) GetRolePermissionMatrix(ctx context.Context, actorID, roleName string) (*RolePermissionMatrix, error) {
	if err := c.requireCanManageAdminRoles(ctx, actorID); err != nil {
		return nil, err
	}
	return c.buildRolePermissionMatrix(ctx, roleName)
}

func (c *ChattoCore) GetUserPermissionMatrix(ctx context.Context, actorID, userID string) (*UserPermissionMatrix, error) {
	if err := c.requireCanManageUserPermissionTarget(ctx, actorID); err != nil {
		return nil, err
	}
	return c.buildUserPermissionMatrix(ctx, userID)
}

func (c *ChattoCore) SetRolePermissionState(ctx context.Context, actorID, roleName string, scope PermissionTargetScope, perm Permission, state PermissionState) error {
	if roleName == RoleOwner {
		return fmt.Errorf("%w: owner permissions are granted virtually and cannot be edited", ErrInvalidArgument)
	}
	if roleName == "" {
		return fmt.Errorf("%w: role name is required", ErrInvalidArgument)
	}
	switch normalizePermissionScope(scope).Kind {
	case MatrixScopeGroup:
		if err := c.requireCanManageAdminRoles(ctx, actorID); err != nil {
			return err
		}
		if scope.ID == "" {
			return fmt.Errorf("%w: group id is required", ErrInvalidArgument)
		}
		return c.applyRolePermissionState(ctx, actorID, ScopeGroup, scope.ID, roleName, perm, state)
	case MatrixScopeRoom:
		if scope.ID == "" {
			return fmt.Errorf("%w: room id is required", ErrInvalidArgument)
		}
		if err := c.requireCanManageRolePermissionsForRoom(ctx, actorID, scope.ID); err != nil {
			return err
		}
		return c.applyRolePermissionState(ctx, actorID, ScopeRoom, scope.ID, roleName, perm, state)
	default:
		if err := c.requireCanManageAdminRoles(ctx, actorID); err != nil {
			return err
		}
		return c.applyRolePermissionState(ctx, actorID, ScopeServer, "", roleName, perm, state)
	}
}

func (c *ChattoCore) SetUserPermissionState(ctx context.Context, actorID, userID string, scope PermissionTargetScope, perm Permission, state PermissionState) error {
	if userID == "" {
		return fmt.Errorf("%w: user id is required", ErrInvalidArgument)
	}
	if err := c.requireCanManageUserPermissionTarget(ctx, actorID); err != nil {
		return err
	}
	switch normalizePermissionScope(scope).Kind {
	case MatrixScopeGroup:
		if scope.ID == "" {
			return fmt.Errorf("%w: group id is required", ErrInvalidArgument)
		}
		return c.applyUserPermissionState(ctx, actorID, ScopeGroup, scope.ID, userID, perm, state)
	case MatrixScopeRoom:
		if scope.ID == "" {
			return fmt.Errorf("%w: room id is required", ErrInvalidArgument)
		}
		return c.applyUserPermissionState(ctx, actorID, ScopeRoom, scope.ID, userID, perm, state)
	default:
		return c.applyUserPermissionState(ctx, actorID, ScopeServer, "", userID, perm, state)
	}
}

func (c *ChattoCore) requireCanManageRolePermissionsForRoom(ctx context.Context, actorID, roomID string) error {
	if actorID == "" {
		return ErrNotAuthenticated
	}
	canManage, err := c.CanManageRoles(ctx, actorID)
	if err != nil {
		return fmt.Errorf("check role.manage: %w", err)
	}
	if canManage {
		return c.requireChannelRoomExists(ctx, roomID)
	}
	if roomID != "" {
		hasRoomManage, err := c.PermResolver().HasRoomPermission(ctx, actorID, KindChannel, roomID, PermRoomManage)
		if err != nil {
			return fmt.Errorf("check room.manage: %w", err)
		}
		if hasRoomManage {
			return c.requireChannelRoomExists(ctx, roomID)
		}
	}
	return ErrPermissionDenied
}

func (c *ChattoCore) requireCanManageUserPermissionTarget(ctx context.Context, actorID string) error {
	if actorID == "" {
		return ErrNotAuthenticated
	}
	canManage, err := c.CanManageUserPermissions(ctx, actorID)
	if err != nil {
		return fmt.Errorf("check user.manage-permissions: %w", err)
	}
	if !canManage {
		return ErrPermissionDenied
	}
	return nil
}

func (c *ChattoCore) requireChannelRoomExists(ctx context.Context, roomID string) error {
	if roomID == "" {
		return nil
	}
	room, err := c.GetRoom(ctx, KindChannel, roomID)
	if err != nil {
		return err
	}
	if room == nil {
		return ErrNotFound
	}
	return nil
}

func (c *ChattoCore) requirePermissionExplanationRoom(ctx context.Context, roomID string) error {
	room, err := c.GetRoom(ctx, KindChannel, roomID)
	if err != nil || room == nil {
		return ErrPermissionDenied
	}
	return nil
}

func normalizePermissionScope(scope PermissionTargetScope) PermissionTargetScope {
	if scope.Kind == "" {
		scope.Kind = MatrixScopeServer
	}
	return scope
}

func (c *ChattoCore) applyRolePermissionState(ctx context.Context, actorID string, scope PermissionScope, scopeID, roleName string, perm Permission, state PermissionState) error {
	switch scope {
	case ScopeGroup:
		switch state {
		case PermissionStateAllow:
			return c.GrantGroupPermission(ctx, actorID, scopeID, roleName, perm)
		case PermissionStateDeny:
			return c.DenyGroupPermission(ctx, actorID, scopeID, roleName, perm)
		case PermissionStateNone:
			return c.ClearGroupPermissionState(ctx, actorID, scopeID, roleName, perm)
		}
	case ScopeRoom:
		switch state {
		case PermissionStateAllow:
			return c.GrantRoomPermission(ctx, actorID, scopeID, roleName, perm)
		case PermissionStateDeny:
			return c.DenyRoomPermission(ctx, actorID, scopeID, roleName, perm)
		case PermissionStateNone:
			return c.ClearRoomPermissionState(ctx, actorID, scopeID, roleName, perm)
		}
	default:
		switch state {
		case PermissionStateAllow:
			return c.GrantServerPermission(ctx, actorID, roleName, perm)
		case PermissionStateDeny:
			return c.DenyServerPermission(ctx, actorID, roleName, perm)
		case PermissionStateNone:
			return c.ClearServerPermissionState(ctx, actorID, roleName, perm)
		}
	}
	return fmt.Errorf("%w: unknown permission state %q", ErrInvalidArgument, state)
}

func (c *ChattoCore) applyUserPermissionState(ctx context.Context, actorID string, scope PermissionScope, scopeID, userID string, perm Permission, state PermissionState) error {
	switch scope {
	case ScopeGroup:
		switch state {
		case PermissionStateAllow:
			return c.GrantUserGroupPermission(ctx, actorID, scopeID, userID, perm)
		case PermissionStateDeny:
			return c.DenyUserGroupPermission(ctx, actorID, scopeID, userID, perm)
		case PermissionStateNone:
			return c.ClearUserGroupPermissionState(ctx, actorID, scopeID, userID, perm)
		}
	case ScopeRoom:
		switch state {
		case PermissionStateAllow:
			return c.GrantUserRoomPermission(ctx, actorID, scopeID, userID, perm)
		case PermissionStateDeny:
			return c.DenyUserRoomPermission(ctx, actorID, scopeID, userID, perm)
		case PermissionStateNone:
			return c.ClearUserRoomPermissionState(ctx, actorID, scopeID, userID, perm)
		}
	default:
		switch state {
		case PermissionStateAllow:
			return c.GrantUserPermission(ctx, actorID, userID, perm)
		case PermissionStateDeny:
			return c.DenyUserPermission(ctx, actorID, userID, perm)
		case PermissionStateNone:
			return c.ClearUserPermissionState(ctx, actorID, userID, perm)
		}
	}
	return fmt.Errorf("%w: unknown permission state %q", ErrInvalidArgument, state)
}

func (c *ChattoCore) buildTierRoles(ctx context.Context, scope PermissionScope, roomID, groupID string) (*TierRoles, error) {
	out := &TierRoles{}
	if groupID != "" {
		for _, meta := range PermissionsForScope(ScopeGroup) {
			out.ApplicablePermissions = append(out.ApplicablePermissions, string(meta.Permission))
		}
	} else {
		for _, meta := range PermissionsForScope(scope) {
			out.ApplicablePermissions = append(out.ApplicablePermissions, string(meta.Permission))
		}
	}

	roles, err := c.ListServerRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	sort.SliceStable(roles, func(i, j int) bool {
		return roles[i].Position < roles[j].Position
	})
	for _, role := range roles {
		tierRole, err := c.buildTierRole(ctx, role, scope, roomID, groupID)
		if err != nil {
			return nil, err
		}
		out.Roles = append(out.Roles, *tierRole)
	}
	return out, nil
}

func (c *ChattoCore) buildTierRole(ctx context.Context, role RoleWithPermissions, scope PermissionScope, roomID, groupID string) (*TierRole, error) {
	out := &TierRole{
		RoleName:    role.Name,
		DisplayName: role.DisplayName,
		Description: role.Description,
		IsSystem:    role.IsSystem,
		Position:    role.Position,
	}

	serverGrants, err := c.GetServerRolePermissions(ctx, role.Name)
	if err != nil {
		return nil, fmt.Errorf("load server grants: %w", err)
	}
	serverDenials, err := c.GetServerRolePermissionDenials(ctx, role.Name)
	if err != nil {
		return nil, fmt.Errorf("load server denials: %w", err)
	}

	if groupID != "" {
		grants, denials, err := c.GetGroupRolePermissions(ctx, groupID, role.Name)
		if err != nil {
			return nil, fmt.Errorf("load group overrides: %w", err)
		}
		out.Override = newCoreTierPermissions(grants, denials)
		out.InheritedAllows = filterCorePermsByScope(serverGrants, ScopeGroup)
		out.InheritedDenials = filterCorePermsByScope(serverDenials, ScopeGroup)
		return out, nil
	}

	switch scope {
	case ScopeServer:
		out.Override = newCoreTierPermissions(serverGrants, serverDenials)
	case ScopeRoom:
		grants, denials, err := c.GetRoomRolePermissions(ctx, roomID, role.Name)
		if err != nil {
			return nil, fmt.Errorf("load room overrides: %w", err)
		}
		out.Override = newCoreTierPermissions(grants, denials)

		groupID, err := c.lookupRoomGroupID(ctx, roomID)
		if err != nil {
			return nil, err
		}
		var groupGrants, groupDenials []Permission
		if groupID != "" {
			groupGrants, groupDenials, err = c.GetGroupRolePermissions(ctx, groupID, role.Name)
			if err != nil {
				return nil, fmt.Errorf("load group inheritance: %w", err)
			}
		}
		out.InheritedAllows, out.InheritedDenials = mergeInheritedPermissionDecisions(
			groupGrants, groupDenials,
			scopedCorePerms(serverGrants, ScopeRoom),
			scopedCorePerms(serverDenials, ScopeRoom),
		)
	}
	return out, nil
}

func (c *ChattoCore) buildRolePermissionMatrix(ctx context.Context, roleName string) (*RolePermissionMatrix, error) {
	role, err := c.GetServerRole(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("load role: %w", err)
	}
	if role == nil {
		return nil, ErrRoleNotFound
	}

	applicable := matrixApplicablePermissions()
	scopes, err := c.buildMatrixScopes(ctx)
	if err != nil {
		return nil, err
	}

	serverGrants, err := c.GetServerRolePermissions(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("load server grants: %w", err)
	}
	serverDenials, err := c.GetServerRolePermissionDenials(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("load server denials: %w", err)
	}

	groupGrants := make(map[string][]Permission)
	groupDenials := make(map[string][]Permission)
	roomGrants := make(map[string][]Permission)
	roomDenials := make(map[string][]Permission)
	roomToGroup := make(map[string]string)

	for _, scope := range scopes {
		switch scope.Kind {
		case MatrixScopeGroup:
			groupID := scopeRefID(scope.ID, "group:")
			g, d, err := c.GetGroupRolePermissions(ctx, groupID, roleName)
			if err != nil {
				return nil, fmt.Errorf("load group %s permissions: %w", groupID, err)
			}
			groupGrants[groupID] = g
			groupDenials[groupID] = d
		case MatrixScopeRoom:
			roomID := scopeRefID(scope.ID, "room:")
			g, d, err := c.GetRoomRolePermissions(ctx, roomID, roleName)
			if err != nil {
				return nil, fmt.Errorf("load room %s permissions: %w", roomID, err)
			}
			roomGrants[roomID] = g
			roomDenials[roomID] = d
			roomToGroup[roomID] = scope.ParentGroupID
		}
	}

	cells := make([]PermissionMatrixCell, 0, len(applicable)*len(scopes))
	for _, permStr := range applicable {
		perm := Permission(permStr)
		for _, scope := range scopes {
			cell, ok := buildRolePermissionCell(
				perm, scope,
				serverGrants, serverDenials,
				groupGrants, groupDenials,
				roomGrants, roomDenials,
				roomToGroup,
			)
			if ok {
				cells = append(cells, cell)
			}
		}
	}

	return &RolePermissionMatrix{
		RoleName:              roleName,
		ApplicablePermissions: applicable,
		Scopes:                scopes,
		Cells:                 cells,
	}, nil
}

func (c *ChattoCore) buildUserPermissionMatrix(ctx context.Context, userID string) (*UserPermissionMatrix, error) {
	if _, err := c.GetUser(ctx, userID); err != nil {
		return nil, err
	}
	applicable := matrixApplicablePermissions()
	scopes, err := c.buildMatrixScopes(ctx)
	if err != nil {
		return nil, err
	}
	cells := make([]PermissionMatrixCell, 0, len(applicable)*len(scopes))
	for _, permStr := range applicable {
		perm := Permission(permStr)
		for _, scope := range scopes {
			cell, ok, err := c.buildUserPermissionMatrixCell(ctx, userID, perm, scope)
			if err != nil {
				return nil, err
			}
			if ok {
				cells = append(cells, cell)
			}
		}
	}
	return &UserPermissionMatrix{
		UserID:                userID,
		ApplicablePermissions: applicable,
		Scopes:                scopes,
		Cells:                 cells,
	}, nil
}

func matrixApplicablePermissions() []string {
	allPerms := AllPermissions()
	applicable := make([]string, 0, len(allPerms))
	for _, meta := range allPerms {
		if PermissionAppliesAtScope(meta.Permission, ScopeServer) ||
			PermissionAppliesAtScope(meta.Permission, ScopeGroup) ||
			PermissionAppliesAtScope(meta.Permission, ScopeRoom) {
			applicable = append(applicable, string(meta.Permission))
		}
	}
	return applicable
}

func (c *ChattoCore) buildMatrixScopes(ctx context.Context) ([]PermissionMatrixScope, error) {
	scopes := []PermissionMatrixScope{{
		ID:    "server",
		Label: "Server",
		Kind:  MatrixScopeServer,
	}}
	groups, err := c.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		return nil, fmt.Errorf("load room groups: %w", err)
	}

	roomsByGroup := make(map[string][]matrixRoomLite, len(groups))
	for _, group := range groups {
		scopes = append(scopes, PermissionMatrixScope{
			ID:    "group:" + group.Id,
			Label: group.Name,
			Kind:  MatrixScopeGroup,
		})
		for _, roomID := range group.RoomIds {
			room, err := c.GetRoom(ctx, KindChannel, roomID)
			if err != nil || room == nil {
				continue
			}
			roomsByGroup[group.Id] = append(roomsByGroup[group.Id], matrixRoomLite{
				ID:   room.Id,
				Name: room.Name,
			})
		}
	}
	for _, group := range groups {
		for _, room := range roomsByGroup[group.Id] {
			scopes = append(scopes, PermissionMatrixScope{
				ID:            "room:" + room.ID,
				Label:         room.Name,
				Kind:          MatrixScopeRoom,
				ParentGroupID: group.Id,
			})
		}
	}
	return scopes, nil
}

func buildRolePermissionCell(
	perm Permission,
	scope PermissionMatrixScope,
	serverGrants, serverDenials []Permission,
	groupGrants, groupDenials map[string][]Permission,
	roomGrants, roomDenials map[string][]Permission,
	roomToGroup map[string]string,
) (PermissionMatrixCell, bool) {
	switch scope.Kind {
	case MatrixScopeServer:
		if !PermissionAppliesAtScope(perm, ScopeServer) {
			return PermissionMatrixCell{}, false
		}
		override := matrixDecisionFromLists(perm, serverGrants, serverDenials)
		return PermissionMatrixCell{
			Permission: string(perm),
			ScopeID:    scope.ID,
			Override:   override,
			Effective:  override,
		}, true
	case MatrixScopeGroup:
		if !PermissionAppliesAtScope(perm, ScopeGroup) {
			return PermissionMatrixCell{}, false
		}
		groupID := scopeRefID(scope.ID, "group:")
		override := matrixDecisionFromLists(perm, groupGrants[groupID], groupDenials[groupID])
		effective := override
		if effective == MatrixDecisionNone && PermissionAppliesAtScope(perm, ScopeServer) {
			effective = matrixDecisionFromLists(perm, serverGrants, serverDenials)
		}
		return PermissionMatrixCell{
			Permission: string(perm),
			ScopeID:    scope.ID,
			Override:   override,
			Effective:  effective,
		}, true
	case MatrixScopeRoom:
		if !PermissionAppliesAtScope(perm, ScopeRoom) {
			return PermissionMatrixCell{}, false
		}
		roomID := scopeRefID(scope.ID, "room:")
		override := matrixDecisionFromLists(perm, roomGrants[roomID], roomDenials[roomID])
		effective := override
		if effective == MatrixDecisionNone {
			if groupID := roomToGroup[roomID]; groupID != "" && PermissionAppliesAtScope(perm, ScopeGroup) {
				effective = matrixDecisionFromLists(perm, groupGrants[groupID], groupDenials[groupID])
			}
			if effective == MatrixDecisionNone && PermissionAppliesAtScope(perm, ScopeServer) {
				effective = matrixDecisionFromLists(perm, serverGrants, serverDenials)
			}
		}
		return PermissionMatrixCell{
			Permission: string(perm),
			ScopeID:    scope.ID,
			Override:   override,
			Effective:  effective,
		}, true
	default:
		return PermissionMatrixCell{}, false
	}
}

func (c *ChattoCore) buildUserPermissionMatrixCell(ctx context.Context, userID string, perm Permission, scope PermissionMatrixScope) (PermissionMatrixCell, bool, error) {
	var (
		override  DecisionKind
		effective DecisionKind
		err       error
	)

	switch scope.Kind {
	case MatrixScopeServer:
		if !PermissionAppliesAtScope(perm, ScopeServer) {
			return PermissionMatrixCell{}, false, nil
		}
		override, err = c.GetUserExplicitServerOverride(ctx, userID, perm)
		if err != nil {
			return PermissionMatrixCell{}, false, err
		}
		effective, err = c.PermResolver().Resolve(ctx, userID, KindChannel, "", perm)
		if err != nil {
			return PermissionMatrixCell{}, false, err
		}
	case MatrixScopeGroup:
		if !PermissionAppliesAtScope(perm, ScopeGroup) {
			return PermissionMatrixCell{}, false, nil
		}
		groupID := scopeRefID(scope.ID, "group:")
		override, err = c.GetUserExplicitGroupOverride(ctx, groupID, userID, perm)
		if err != nil {
			return PermissionMatrixCell{}, false, err
		}
		effective, err = c.PermResolver().ResolveGroup(ctx, userID, KindChannel, groupID, perm)
		if err != nil {
			return PermissionMatrixCell{}, false, err
		}
	case MatrixScopeRoom:
		if !PermissionAppliesAtScope(perm, ScopeRoom) {
			return PermissionMatrixCell{}, false, nil
		}
		roomID := scopeRefID(scope.ID, "room:")
		override, err = c.GetUserExplicitRoomOverride(ctx, roomID, userID, perm)
		if err != nil {
			return PermissionMatrixCell{}, false, err
		}
		effective, err = c.PermResolver().Resolve(ctx, userID, KindChannel, roomID, perm)
		if err != nil {
			return PermissionMatrixCell{}, false, err
		}
	default:
		return PermissionMatrixCell{}, false, fmt.Errorf("%w: unknown scope kind %q", ErrInvalidArgument, scope.Kind)
	}

	return PermissionMatrixCell{
		Permission: string(perm),
		ScopeID:    scope.ID,
		Override:   matrixDecisionFromCoreDecision(override),
		Effective:  matrixDecisionFromCoreDecision(effective),
	}, true, nil
}

func (c *ChattoCore) lookupRoomGroupID(ctx context.Context, roomID string) (string, error) {
	if roomID == "" {
		return "", nil
	}
	room, err := c.GetRoom(ctx, KindChannel, roomID)
	if err != nil {
		return "", fmt.Errorf("load room for inheritance lookup: %w", err)
	}
	if room == nil {
		return "", nil
	}
	return room.GroupId, nil
}

func newCoreTierPermissions(grants, denials []Permission) TierPermissions {
	return TierPermissions{
		Permissions:       corePermsToStrings(grants),
		PermissionDenials: corePermsToStrings(denials),
	}
}

func filterCorePermsByScope(perms []Permission, scope PermissionScope) []string {
	out := make([]string, 0, len(perms))
	for _, perm := range perms {
		if PermissionAppliesAtScope(perm, scope) {
			out = append(out, string(perm))
		}
	}
	return out
}

func scopedCorePerms(perms []Permission, scope PermissionScope) []Permission {
	out := make([]Permission, 0, len(perms))
	for _, perm := range perms {
		if PermissionAppliesAtScope(perm, scope) {
			out = append(out, perm)
		}
	}
	return out
}

func mergeInheritedPermissionDecisions(overrideAllow, overrideDeny, parentAllow, parentDeny []Permission) ([]string, []string) {
	overridden := make(map[Permission]struct{}, len(overrideAllow)+len(overrideDeny))
	for _, perm := range overrideAllow {
		overridden[perm] = struct{}{}
	}
	for _, perm := range overrideDeny {
		overridden[perm] = struct{}{}
	}

	allow := make([]string, 0, len(overrideAllow)+len(parentAllow))
	for _, perm := range overrideAllow {
		allow = append(allow, string(perm))
	}
	for _, perm := range parentAllow {
		if _, blocked := overridden[perm]; !blocked {
			allow = append(allow, string(perm))
		}
	}

	deny := make([]string, 0, len(overrideDeny)+len(parentDeny))
	for _, perm := range overrideDeny {
		deny = append(deny, string(perm))
	}
	for _, perm := range parentDeny {
		if _, blocked := overridden[perm]; !blocked {
			deny = append(deny, string(perm))
		}
	}
	return allow, deny
}

func matrixDecisionFromLists(perm Permission, grants, denials []Permission) MatrixDecision {
	for _, grant := range grants {
		if grant == perm {
			return MatrixDecisionAllow
		}
	}
	for _, denial := range denials {
		if denial == perm {
			return MatrixDecisionDeny
		}
	}
	return MatrixDecisionNone
}

func matrixDecisionFromCoreDecision(decision DecisionKind) MatrixDecision {
	switch decision {
	case DecisionAllow:
		return MatrixDecisionAllow
	case DecisionDeny:
		return MatrixDecisionDeny
	default:
		return MatrixDecisionNone
	}
}

func scopeRefID(scopeID, prefix string) string {
	if len(scopeID) <= len(prefix) {
		return ""
	}
	return scopeID[len(prefix):]
}

func corePermsToStrings(perms []Permission) []string {
	out := make([]string, len(perms))
	for i, perm := range perms {
		out[i] = string(perm)
	}
	return out
}

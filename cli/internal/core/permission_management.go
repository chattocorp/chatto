package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
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
	MatrixScopeDM     MatrixScopeKind = "DM"
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

// TierPermissions lists the explicit grants of a role at one tier. Roles
// only grant, so there are no role denies (ADR-116).
type TierPermissions struct {
	Permissions []string
}

type TierRole struct {
	RoleName        string
	DisplayName     string
	Description     string
	IsSystem        bool
	Pingable        bool
	Override        TierPermissions
	InheritedAllows []string
	// EffectiveAllows lists what a member with only this role is allowed at
	// the tier, from the resolver (resolveRoleHolder).
	EffectiveAllows []string
}

type TierRoles struct {
	ApplicablePermissions []string
	// ViewerChangeablePermissions lists the applicable permissions that the
	// viewer holds at this tier. The grant limit (ADR-115) requires that for
	// every change of a setting.
	ViewerChangeablePermissions []string
	// Roles in role order, highest first.
	Roles []TierRole
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
	// Effective is the result for the subject. For a human account it is the
	// result without privileged mode.
	Effective MatrixDecision
	// EffectiveWithPrivilegedMode is the result for a human account with
	// privileged mode active. It is empty for roles and bots.
	EffectiveWithPrivilegedMode MatrixDecision
	// AllowPermitted reports the bot owner's RBAC entitlement at this scope.
	// It does not include the acting human's session activation or edit authority.
	// Nil means that the cell does not use a bot owner ceiling.
	AllowPermitted *bool
	// ViewerCanChange reports whether the viewer holds the permission at this
	// scope, which the grant limit requires for every change (ADR-115). Nil
	// for bot cells, which have their own limits.
	ViewerCanChange *bool
}

type RolePermissionMatrix struct {
	Page                  PermissionScopePage
	RoleName              string
	ApplicablePermissions []string
	Scopes                []PermissionMatrixScope
	Cells                 []PermissionMatrixCell
}

type UserPermissionMatrix struct {
	Page                  PermissionScopePage
	UserID                string
	ApplicablePermissions []string
	Scopes                []PermissionMatrixScope
	Cells                 []PermissionMatrixCell
}

func (c *ChattoCore) ExplainPermissions(ctx context.Context, actorID, targetUserID, roomID string) ([]PermissionExplanation, error) {
	return c.ExplainPermissionsAtScope(ctx, actorID, targetUserID, PermissionTargetScope{Kind: MatrixScopeRoom, ID: roomID})
}

// ExplainPermissionsAtScope returns permission explanations for a server,
// direct-message, or channel-room target.
func (c *ChattoCore) ExplainPermissionsAtScope(ctx context.Context, actorID, targetUserID string, target PermissionTargetScope) ([]PermissionExplanation, error) {
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
	target = normalizePermissionScope(target)
	if target.Kind == MatrixScopeDM {
		if target.ID != "" {
			return nil, fmt.Errorf("%w: direct-message scope id must be empty", ErrInvalidArgument)
		}
		return c.PermResolver().ExplainAllPermissions(ctx, targetUserID, KindDM, "")
	}
	if target.Kind == MatrixScopeRoom && target.ID != "" {
		if err := c.requirePermissionExplanationRoom(ctx, target.ID); err != nil {
			return nil, err
		}
		return c.PermResolver().ExplainAllPermissions(ctx, targetUserID, KindChannel, target.ID)
	}
	if target.Kind != MatrixScopeServer && !(target.Kind == MatrixScopeRoom && target.ID == "") {
		return nil, fmt.Errorf("%w: unsupported explanation scope %q", ErrInvalidArgument, target.Kind)
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
		return c.buildTierRolesForViewer(ctx, actorID, ScopeRoom, roomID, "")
	}
	if groupID != "" {
		if err := c.requireCanManageRolePermissionsForGroup(ctx, actorID, groupID); err != nil {
			return nil, err
		}
		return c.buildTierRolesForViewer(ctx, actorID, ScopeGroup, "", groupID)
	}
	if err := c.requireCanManageAdminRoles(ctx, actorID); err != nil {
		return nil, err
	}
	return c.buildTierRolesForViewer(ctx, actorID, ScopeServer, "", "")
}

// buildTierRolesForViewer builds a tier matrix and lists the permissions that
// the viewer may change at the tier.
func (c *ChattoCore) buildTierRolesForViewer(ctx context.Context, actorID string, scope PermissionScope, roomID, groupID string) (*TierRoles, error) {
	out, err := c.buildTierRoles(ctx, scope, roomID, groupID)
	if err != nil {
		return nil, err
	}
	scopeID := roomID
	if scope == ScopeGroup {
		scopeID = groupID
	}
	for _, permission := range out.ApplicablePermissions {
		holds, err := c.actorCanSetDecision(ctx, actorID, ScopedRolePermissionDecision{Scope: scope, ScopeID: scopeID, Permission: Permission(permission)})
		if err != nil {
			return nil, err
		}
		if holds {
			out.ViewerChangeablePermissions = append(out.ViewerChangeablePermissions, permission)
		}
	}
	return out, nil
}

// viewerCanChangeAtMatrixScope reports whether the viewer holds perm at the
// matrix scope, which the grant limit requires to change a setting there.
func (c *ChattoCore) viewerCanChangeAtMatrixScope(ctx context.Context, actorID string, perm Permission, scope PermissionMatrixScope) (bool, error) {
	decision := ScopedRolePermissionDecision{Permission: perm}
	switch scope.Kind {
	case MatrixScopeServer:
		decision.Scope = ScopeServer
	case MatrixScopeDM:
		decision.Scope = ScopeDM
	case MatrixScopeGroup:
		decision.Scope, decision.ScopeID = ScopeGroup, scopeRefID(scope.ID, "group:")
	case MatrixScopeRoom:
		decision.Scope, decision.ScopeID = ScopeRoom, scopeRefID(scope.ID, "room:")
	default:
		return false, nil
	}
	return c.actorCanSetDecision(ctx, actorID, decision)
}

// GetRolePermissionDMTierMatrix returns the role matrix for the singleton
// direct-message scope.
func (c *ChattoCore) GetRolePermissionDMTierMatrix(ctx context.Context, actorID string) (*TierRoles, error) {
	if err := c.requireCanManageAdminRoles(ctx, actorID); err != nil {
		return nil, err
	}
	return c.buildTierRolesForViewer(ctx, actorID, ScopeDM, "", "")
}

func (c *ChattoCore) GetRolePermissionMatrix(ctx context.Context, actorID, roleName string) (*RolePermissionMatrix, error) {
	return c.GetRolePermissionMatrixIncludingDM(ctx, actorID, roleName, false)
}

// GetRolePermissionMatrixIncludingDM returns the first scope page, optionally including DMs.
func (c *ChattoCore) GetRolePermissionMatrixIncludingDM(ctx context.Context, actorID, roleName string, includeDM bool) (*RolePermissionMatrix, error) {
	return c.GetRolePermissionMatrixPage(ctx, actorID, roleName, includeDM, PermissionScopeQuery{})
}

// GetRolePermissionMatrixPage authorizes and evaluates one bounded scope page.
func (c *ChattoCore) GetRolePermissionMatrixPage(ctx context.Context, actorID, roleName string, includeDM bool, query PermissionScopeQuery) (*RolePermissionMatrix, error) {
	if err := c.requireCanManageAdminRoles(ctx, actorID); err != nil {
		return nil, err
	}
	return c.buildRolePermissionMatrix(ctx, actorID, roleName, includeDM, query)
}

func (c *ChattoCore) GetUserPermissionMatrix(ctx context.Context, actorID, userID string) (*UserPermissionMatrix, error) {
	return c.GetUserPermissionMatrixIncludingDM(ctx, actorID, userID, false)
}

// GetUserPermissionMatrixIncludingDM returns the first scope page, optionally including DMs.
func (c *ChattoCore) GetUserPermissionMatrixIncludingDM(ctx context.Context, actorID, userID string, includeDM bool) (*UserPermissionMatrix, error) {
	return c.GetUserPermissionMatrixPage(ctx, actorID, userID, includeDM, PermissionScopeQuery{})
}

// GetUserPermissionMatrixPage authorizes and evaluates one bounded scope page.
func (c *ChattoCore) GetUserPermissionMatrixPage(ctx context.Context, actorID, userID string, includeDM bool, query PermissionScopeQuery) (*UserPermissionMatrix, error) {
	if actorID == "" {
		return nil, ErrNotAuthenticated
	}
	user, err := c.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	if user.GetIsBot() {
		user, err = c.requireBotManagementPermission(ctx, actorID, userID)
		if err != nil {
			return nil, err
		}
	} else if err := c.requireCanManageUserPermissionTarget(ctx, actorID); err != nil {
		return nil, err
	}
	return c.buildUserPermissionMatrix(ctx, actorID, user, includeDM, query)
}

// SetRolePermissionState changes one role decision. Roles, everyone included,
// only grant, so a deny returns ErrInvalidArgument (ADR-116). Role managers may
// edit every scope; room managers may edit the groups and rooms that they
// manage.
// Holders of role.manage may change every role except owner; other editors
// need the role to rank below them (requireRoleManageable). A non-owner may
// change only decisions for permissions that they effectively hold at the
// target scope.
func (c *ChattoCore) SetRolePermissionState(ctx context.Context, actorID, roleName string, scope PermissionTargetScope, perm Permission, state PermissionState) error {
	if roleName == RoleOwner {
		return fmt.Errorf("%w: owner permissions are granted virtually and cannot be edited", ErrInvalidArgument)
	}
	if roleName == "" {
		return fmt.Errorf("%w: role name is required", ErrInvalidArgument)
	}
	// Reject a role deny before authorization, so callers learn that the
	// request itself is invalid.
	if state == PermissionStateDeny {
		return fmt.Errorf("%w: roles only grant permissions; set the deny on a user", ErrInvalidArgument)
	}
	var (
		coreScope     PermissionScope
		requireEditor func() error
	)
	switch normalizePermissionScope(scope).Kind {
	case MatrixScopeDM:
		if scope.ID != "" {
			return fmt.Errorf("%w: direct-message scope id must be empty", ErrInvalidArgument)
		}
		coreScope = ScopeDM
		requireEditor = func() error { return c.requireCanManageAdminRoles(ctx, actorID) }
	case MatrixScopeGroup:
		if scope.ID == "" {
			return fmt.Errorf("%w: group id is required", ErrInvalidArgument)
		}
		coreScope = ScopeGroup
		requireEditor = func() error {
			if err := c.roomModel.waitForGroupLayoutCurrent(ctx, c.EventPublisher); err != nil {
				return fmt.Errorf("wait for room-group projection: %w", err)
			}
			return c.requireCanManageRolePermissionsForGroup(ctx, actorID, scope.ID)
		}
	case MatrixScopeRoom:
		if scope.ID == "" {
			return fmt.Errorf("%w: room id is required", ErrInvalidArgument)
		}
		coreScope = ScopeRoom
		requireEditor = func() error {
			if err := c.roomModel.waitForGroupLayoutCurrent(ctx, c.EventPublisher); err != nil {
				return fmt.Errorf("wait for room-group projection: %w", err)
			}
			if err := c.roomModel.waitForDirectoryCurrent(ctx, c.EventPublisher); err != nil {
				return fmt.Errorf("wait for room directory projection: %w", err)
			}
			return c.requireCanManageRolePermissionsForRoom(ctx, actorID, scope.ID)
		}
	default:
		coreScope = ScopeServer
		scope.ID = ""
		requireEditor = func() error { return c.requireCanManageAdminRoles(ctx, actorID) }
	}
	check := func() error {
		if err := requireEditor(); err != nil {
			return err
		}
		if err := validatePermissionDecisionScope(coreScope, perm); err != nil {
			return err
		}
		if !c.rbacModel.roleExists(roleName) {
			return ErrRoleNotFound
		}
		if err := c.requireRoleManageable(ctx, actorID, roleName); err != nil {
			return err
		}
		return c.requirePermissionDecisionWithinAuthority(ctx, actorID, coreScope, scope.ID, perm)
	}
	if err := check(); err != nil {
		return err
	}
	return c.applyRolePermissionState(ctx, actorID, coreScope, scope.ID, roleName, perm, state, check)
}

func (c *ChattoCore) requireCanManageRolePermissionsForGroup(ctx context.Context, actorID, groupID string) error {
	if actorID == "" {
		return ErrNotAuthenticated
	}
	canManage, err := c.CanManageRoles(ctx, actorID)
	if err != nil {
		return fmt.Errorf("check role.manage: %w", err)
	}
	if canManage {
		_, err := c.GetRoomGroup(ctx, groupID)
		return err
	}
	hasRoomManage, err := c.CanManageRoomGroup(ctx, actorID, groupID)
	if err != nil {
		return fmt.Errorf("check room.manage: %w", err)
	}
	if !hasRoomManage {
		return ErrPermissionDenied
	}
	_, err = c.GetRoomGroup(ctx, groupID)
	return err
}

// SetUserPermissionState changes one direct permission decision of a user.
// Bot decisions follow the bot allowlist rules. For humans, the actor needs
// user.manage-permissions, must outrank the user unless it is their own
// account, and as a non-owner may change only decisions for permissions that
// they effectively hold at the target scope.
func (c *ChattoCore) SetUserPermissionState(ctx context.Context, actorID, userID string, scope PermissionTargetScope, perm Permission, state PermissionState) error {
	if userID == "" {
		return fmt.Errorf("%w: user id is required", ErrInvalidArgument)
	}
	if actorID == "" {
		return ErrNotAuthenticated
	}
	user, err := c.GetUser(ctx, userID)
	if err != nil {
		return err
	}
	if user.GetIsBot() {
		return c.setBotUserPermissionState(ctx, actorID, userID, scope, perm, state)
	}
	var coreScope PermissionScope
	switch normalizePermissionScope(scope).Kind {
	case MatrixScopeDM:
		if scope.ID != "" {
			return fmt.Errorf("%w: direct-message scope id must be empty", ErrInvalidArgument)
		}
		coreScope = ScopeDM
	case MatrixScopeGroup:
		if scope.ID == "" {
			return fmt.Errorf("%w: group id is required", ErrInvalidArgument)
		}
		coreScope = ScopeGroup
	case MatrixScopeRoom:
		if scope.ID == "" {
			return fmt.Errorf("%w: room id is required", ErrInvalidArgument)
		}
		coreScope = ScopeRoom
	default:
		coreScope = ScopeServer
		scope.ID = ""
	}
	check := func() error {
		if err := c.requireCanManageUserPermissionTarget(ctx, actorID); err != nil {
			return err
		}
		if coreScope == ScopeRoom {
			// Room-to-group placement is covered by the stable authorization
			// inputs; the room itself must also be current on this replica.
			if err := c.roomModel.waitForDirectoryCurrent(ctx, c.EventPublisher); err != nil {
				return fmt.Errorf("wait for room directory projection: %w", err)
			}
		}
		if err := c.requireOutranksOtherAccount(actorID, userID); err != nil {
			return err
		}
		if err := validatePermissionDecisionScope(coreScope, perm); err != nil {
			return err
		}
		return c.requirePermissionDecisionWithinAuthority(ctx, actorID, coreScope, scope.ID, perm)
	}
	if err := check(); err != nil {
		return err
	}
	return c.applyUserPermissionState(ctx, actorID, coreScope, scope.ID, userID, perm, state, check)
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

// validatePermissionDecisionScope rejects unknown permissions and decisions
// at a scope where the permission cannot be configured.
func validatePermissionDecisionScope(scope PermissionScope, perm Permission) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}
	if scope == ScopeRoom && !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("%w: permission %s does not apply at room scope", ErrInvalidArgument, perm)
	}
	if scope == ScopeGroup && !PermissionAppliesAtScope(perm, ScopeGroup) && !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("%w: permission %s does not apply at group scope", ErrInvalidArgument, perm)
	}
	if scope == ScopeDM && !PermissionAppliesAtScope(perm, ScopeDM) {
		return fmt.Errorf("%w: permission %s does not apply at direct-message scope", ErrInvalidArgument, perm)
	}
	return nil
}

// applyRolePermissionState appends an allow or a clear for a role. Roles have
// no deny state (ADR-116).
func (c *ChattoCore) applyRolePermissionState(ctx context.Context, actorID string, scope PermissionScope, scopeID, roleName string, perm Permission, state PermissionState, authorize func() error) error {
	if err := validatePermissionDecisionScope(scope, perm); err != nil {
		return err
	}

	var event *evtv1.Event
	switch state {
	case PermissionStateAllow:
		event = newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_RbacPermissionGranted{
			RbacPermissionGranted: rbacRolePermissionGrantedEvent(scope, scopeID, roleName, perm),
		}})
	case PermissionStateNone:
		event = newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_RbacPermissionCleared{
			RbacPermissionCleared: rbacRolePermissionClearedEvent(scope, scopeID, roleName, perm),
		}})
	default:
		return fmt.Errorf("%w: unknown permission state %q", ErrInvalidArgument, state)
	}

	_, err := c.appendRBACEvent(ctx, event, func() error {
		if authorize != nil {
			if err := authorize(); err != nil {
				return err
			}
		}
		current := c.rbacModel.decision(scope, scopeID, roleName, perm)
		if (state == PermissionStateAllow && current == DecisionAllow) ||
			(state == PermissionStateNone && current == DecisionNone) {
			return errRBACNoop
		}
		return nil
	})
	if errors.Is(err, errRBACNoop) {
		return nil
	}
	return err
}

func (c *ChattoCore) applyUserPermissionState(ctx context.Context, actorID string, scope PermissionScope, scopeID, userID string, perm Permission, state PermissionState, authorize func() error) error {
	if err := validatePermissionDecisionScope(scope, perm); err != nil {
		return err
	}

	var event *evtv1.Event
	switch state {
	case PermissionStateAllow:
		event = newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_RbacPermissionGranted{
			RbacPermissionGranted: rbacUserPermissionGrantedEvent(scope, scopeID, userID, perm),
		}})
	case PermissionStateDeny:
		event = newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_RbacPermissionDenied{
			RbacPermissionDenied: rbacUserPermissionDeniedEvent(scope, scopeID, userID, perm),
		}})
	case PermissionStateNone:
		event = newEvent(actorID, &evtv1.Event{Event: &evtv1.Event_RbacPermissionCleared{
			RbacPermissionCleared: rbacUserPermissionClearedEvent(scope, scopeID, userID, perm),
		}})
	default:
		return fmt.Errorf("%w: unknown permission state %q", ErrInvalidArgument, state)
	}

	_, err := c.appendRBACEvent(ctx, event, func() error {
		if authorize != nil {
			if err := authorize(); err != nil {
				return err
			}
		}
		current := c.rbacModel.decision(scope, scopeID, userID, perm)
		if (state == PermissionStateAllow && current == DecisionAllow) ||
			(state == PermissionStateDeny && current == DecisionDeny) ||
			(state == PermissionStateNone && current == DecisionNone) {
			return errRBACNoop
		}
		return nil
	})
	if errors.Is(err, errRBACNoop) {
		return nil
	}
	return err
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
	kind := KindChannel
	if scope == ScopeDM {
		kind = KindDM
	}
	tierGroupID := groupID
	if roomID != "" && tierGroupID == "" {
		if tierGroupID, err = c.lookupRoomGroupID(ctx, roomID); err != nil {
			return nil, err
		}
	}
	for _, role := range roles {
		tierRole, err := c.buildTierRole(ctx, role, scope, roomID, groupID)
		if err != nil {
			return nil, err
		}
		for _, permission := range out.ApplicablePermissions {
			if c.PermResolver().resolveRoleHolder(role.Name, kind, roomID, tierGroupID, Permission(permission)) == DecisionAllow {
				tierRole.EffectiveAllows = append(tierRole.EffectiveAllows, permission)
			}
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
		Pingable:    role.Pingable,
	}

	serverGrants, err := c.GetServerRolePermissions(ctx, role.Name)
	if err != nil {
		return nil, fmt.Errorf("load server grants: %w", err)
	}

	if groupID != "" {
		grants, err := c.GetGroupRolePermissions(ctx, groupID, role.Name)
		if err != nil {
			return nil, fmt.Errorf("load group overrides: %w", err)
		}
		out.Override = TierPermissions{Permissions: corePermsToStrings(grants)}
		out.InheritedAllows = filterCorePermsByScope(serverGrants, ScopeGroup)
		return out, nil
	}

	switch scope {
	case ScopeServer:
		out.Override = TierPermissions{Permissions: corePermsToStrings(serverGrants)}
	case ScopeDM:
		grants, err := c.GetDMRolePermissions(ctx, role.Name)
		if err != nil {
			return nil, fmt.Errorf("load direct-message overrides: %w", err)
		}
		out.Override = TierPermissions{Permissions: corePermsToStrings(grants)}
		out.InheritedAllows = filterCorePermsByScope(serverGrants, ScopeDM)
	case ScopeRoom:
		grants, err := c.GetRoomRolePermissions(ctx, roomID, role.Name)
		if err != nil {
			return nil, fmt.Errorf("load room overrides: %w", err)
		}
		out.Override = TierPermissions{Permissions: corePermsToStrings(grants)}

		groupID, err := c.lookupRoomGroupID(ctx, roomID)
		if err != nil {
			return nil, err
		}
		var groupGrants []Permission
		if groupID != "" {
			groupGrants, err = c.GetGroupRolePermissions(ctx, groupID, role.Name)
			if err != nil {
				return nil, fmt.Errorf("load group inheritance: %w", err)
			}
		}
		out.InheritedAllows = mergeInheritedAllows(groupGrants, scopedCorePerms(serverGrants, ScopeRoom))
	}
	return out, nil
}

func (c *ChattoCore) buildRolePermissionMatrix(ctx context.Context, actorID, roleName string, includeDM bool, query PermissionScopeQuery) (*RolePermissionMatrix, error) {
	role, err := c.GetServerRole(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("load role: %w", err)
	}
	if role == nil {
		return nil, ErrRoleNotFound
	}

	applicable := matrixApplicablePermissions()
	scopes, err := c.buildMatrixScopes(ctx, includeDM || query.includesDM())
	if err != nil {
		return nil, err
	}

	scopes, page, err := selectPermissionScopes(scopes, query)
	if err != nil {
		return nil, err
	}

	serverGrants, err := c.GetServerRolePermissions(ctx, roleName)
	if err != nil {
		return nil, fmt.Errorf("load server grants: %w", err)
	}

	groupGrants := make(map[string][]Permission)
	roomGrants := make(map[string][]Permission)
	var dmGrants []Permission

	for _, scope := range scopes {
		switch scope.Kind {
		case MatrixScopeDM:
			dmGrants, err = c.GetDMRolePermissions(ctx, roleName)
			if err != nil {
				return nil, fmt.Errorf("load direct-message permissions: %w", err)
			}
		case MatrixScopeGroup:
			groupID := scopeRefID(scope.ID, "group:")
			if groupGrants[groupID], err = c.GetGroupRolePermissions(ctx, groupID, roleName); err != nil {
				return nil, fmt.Errorf("load group %s permissions: %w", groupID, err)
			}
		case MatrixScopeRoom:
			roomID := scopeRefID(scope.ID, "room:")
			if roomGrants[roomID], err = c.GetRoomRolePermissions(ctx, roomID, roleName); err != nil {
				return nil, fmt.Errorf("load room %s permissions: %w", roomID, err)
			}
		}
	}

	cells := make([]PermissionMatrixCell, 0, len(applicable)*len(scopes))
	for _, permStr := range applicable {
		perm := Permission(permStr)
		for _, scope := range scopes {
			cell, ok := buildExactRolePermissionCell(perm, scope, serverGrants, dmGrants, groupGrants, roomGrants)
			if !ok {
				continue
			}
			// Show what a member with only this role gets, including the
			// allows of everyone and inclusion, from the resolver itself.
			kind, roomID, groupID := matrixScopeTarget(scope)
			cell.Effective = matrixDecisionFromCoreDecision(c.PermResolver().resolveRoleHolder(roleName, kind, roomID, groupID, perm))
			canChange, err := c.viewerCanChangeAtMatrixScope(ctx, actorID, perm, scope)
			if err != nil {
				return nil, err
			}
			cell.ViewerCanChange = &canChange
			cells = append(cells, cell)
		}
	}

	return &RolePermissionMatrix{
		RoleName:              roleName,
		Page:                  page,
		ApplicablePermissions: applicable,
		Scopes:                scopes,
		Cells:                 cells,
	}, nil
}

func (c *ChattoCore) buildUserPermissionMatrix(ctx context.Context, actorID string, user *evtv1.User, includeDM bool, query PermissionScopeQuery) (*UserPermissionMatrix, error) {
	userID := user.GetId()
	applicable := matrixApplicablePermissions()
	bot := user.GetIsBot()
	if bot {
		filtered := applicable[:0]
		for _, permission := range applicable {
			if botPermissionDelegable(Permission(permission)) {
				filtered = append(filtered, permission)
			}
		}
		applicable = filtered
	}
	var (
		scopes []PermissionMatrixScope
		err    error
	)
	if bot {
		scopes, err = c.buildBotMatrixScopes(ctx, user.GetBotOwnerUserId(), actorID, includeDM || query.includesDM())
	} else {
		scopes, err = c.buildMatrixScopes(ctx, includeDM || query.includesDM())
	}
	if err != nil {
		return nil, err
	}
	scopes, page, err := selectPermissionScopes(scopes, query)
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
				if bot {
					allowed, err := c.botOwnerAllowsAtMatrixScope(ctx, user.GetBotOwnerUserId(), perm, scope)
					if err != nil {
						return nil, err
					}
					cell.AllowPermitted = &allowed
				} else {
					canChange, err := c.viewerCanChangeAtMatrixScope(ctx, actorID, perm, scope)
					if err != nil {
						return nil, err
					}
					cell.ViewerCanChange = &canChange
				}
				cells = append(cells, cell)
			}
		}
	}
	return &UserPermissionMatrix{
		UserID:                userID,
		Page:                  page,
		ApplicablePermissions: applicable,
		Scopes:                scopes,
		Cells:                 cells,
	}, nil
}

func (c *ChattoCore) botOwnerAllowsAtMatrixScope(ctx context.Context, ownerID string, perm Permission, scope PermissionMatrixScope) (bool, error) {
	var (
		decision DecisionKind
		err      error
	)
	switch scope.Kind {
	case MatrixScopeServer:
		decision, err = c.PermResolver().resolveEntitlement(ctx, ownerID, KindChannel, "", "", perm)
	case MatrixScopeDM:
		decision, err = c.PermResolver().resolveEntitlement(ctx, ownerID, KindDM, "", "", perm)
	case MatrixScopeGroup:
		decision, err = c.PermResolver().resolveEntitlement(ctx, ownerID, KindChannel, "", scopeRefID(scope.ID, "group:"), perm)
	case MatrixScopeRoom:
		decision, err = c.PermResolver().resolveEntitlement(ctx, ownerID, KindChannel, scopeRefID(scope.ID, "room:"), "", perm)
	default:
		return false, fmt.Errorf("%w: unknown scope kind %q", ErrInvalidArgument, scope.Kind)
	}
	return decision == DecisionAllow, err
}

func matrixApplicablePermissions() []string {
	allPerms := AllPermissions()
	applicable := make([]string, 0, len(allPerms))
	for _, meta := range allPerms {
		if PermissionAppliesAtScope(meta.Permission, ScopeServer) ||
			PermissionAppliesAtScope(meta.Permission, ScopeDM) ||
			PermissionAppliesAtScope(meta.Permission, ScopeGroup) ||
			PermissionAppliesAtScope(meta.Permission, ScopeRoom) {
			applicable = append(applicable, string(meta.Permission))
		}
	}
	return applicable
}

func (c *ChattoCore) buildMatrixScopes(ctx context.Context, includeDM bool) ([]PermissionMatrixScope, error) {
	return c.buildMatrixScopesVisibleTo(ctx, includeDM)
}

// buildBotMatrixScopes limits bot configuration room metadata to rooms visible
// through the normal directory policy to both the owner and the managing
// caller. Group metadata follows that policy's complete group layout so empty
// groups remain configurable, including for group-scoped room.create grants.
func (c *ChattoCore) buildBotMatrixScopes(ctx context.Context, ownerID, actorID string, includeDM bool) ([]PermissionMatrixScope, error) {
	viewerIDs := []string{ownerID}
	if actorID != ownerID {
		viewerIDs = append(viewerIDs, actorID)
	}
	return c.buildMatrixScopesVisibleTo(ctx, includeDM, viewerIDs...)
}

func (c *ChattoCore) buildMatrixScopesVisibleTo(ctx context.Context, includeDM bool, viewerIDs ...string) ([]PermissionMatrixScope, error) {
	scopes := []PermissionMatrixScope{{
		ID:    "server",
		Label: "Server",
		Kind:  MatrixScopeServer,
	}}
	if includeDM {
		scopes = append(scopes, PermissionMatrixScope{ID: "dm", Label: "Direct messages", Kind: MatrixScopeDM})
	}
	groups, err := c.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		return nil, fmt.Errorf("load room groups: %w", err)
	}

	roomsByGroup := make(map[string][]matrixRoomLite, len(groups))
	for _, group := range groups {
		for _, roomID := range group.RoomIds {
			room, err := c.GetRoom(ctx, KindChannel, roomID)
			if err != nil || room == nil {
				continue
			}
			// Filter before pagination so archived rooms contribute neither columns nor counts.
			if room.GetArchived() {
				continue
			}
			visible := true
			for _, viewerID := range viewerIDs {
				canSee, err := c.CanSeeRoom(ctx, viewerID, KindChannel, room.Id)
				if err != nil {
					return nil, fmt.Errorf("check room visibility: %w", err)
				}
				if !canSee {
					visible = false
					break
				}
			}
			if !visible {
				continue
			}
			roomsByGroup[group.Id] = append(roomsByGroup[group.Id], matrixRoomLite{
				ID:   room.Id,
				Name: room.Name,
			})
		}
	}
	for _, group := range groups {
		scopes = append(scopes, PermissionMatrixScope{
			ID:    "group:" + group.Id,
			Label: group.Name,
			Kind:  MatrixScopeGroup,
		})
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

// matrixScopeTarget maps a matrix scope to the resolver's location.
func matrixScopeTarget(scope PermissionMatrixScope) (kind RoomKind, roomID, groupID string) {
	switch scope.Kind {
	case MatrixScopeDM:
		return KindDM, "", ""
	case MatrixScopeGroup:
		return KindChannel, "", scopeRefID(scope.ID, "group:")
	case MatrixScopeRoom:
		return KindChannel, scopeRefID(scope.ID, "room:"), scope.ParentGroupID
	default:
		return KindChannel, "", ""
	}
}

// buildExactRolePermissionCell builds one role matrix cell from the role's
// stored grants. Roles only grant, so the override is allow or none
// (ADR-116). The caller sets Effective from the resolver.
func buildExactRolePermissionCell(
	perm Permission,
	scope PermissionMatrixScope,
	serverGrants, dmGrants []Permission,
	groupGrants, roomGrants map[string][]Permission,
) (PermissionMatrixCell, bool) {
	var (
		permissionScope PermissionScope
		grants          []Permission
	)
	switch scope.Kind {
	case MatrixScopeServer:
		permissionScope, grants = ScopeServer, serverGrants
	case MatrixScopeDM:
		permissionScope, grants = ScopeDM, dmGrants
	case MatrixScopeGroup:
		permissionScope, grants = ScopeGroup, groupGrants[scopeRefID(scope.ID, "group:")]
	case MatrixScopeRoom:
		permissionScope, grants = ScopeRoom, roomGrants[scopeRefID(scope.ID, "room:")]
	default:
		return PermissionMatrixCell{}, false
	}
	if !PermissionAppliesAtScope(perm, permissionScope) {
		return PermissionMatrixCell{}, false
	}
	override := MatrixDecisionNone
	if slices.Contains(grants, perm) {
		override = MatrixDecisionAllow
	}
	return PermissionMatrixCell{
		Permission: string(perm),
		ScopeID:    scope.ID,
		Override:   override,
	}, true
}

func (c *ChattoCore) buildUserPermissionMatrixCell(ctx context.Context, userID string, perm Permission, scope PermissionMatrixScope) (PermissionMatrixCell, bool, error) {
	var (
		override       DecisionKind
		err            error
		kind           = KindChannel
		roomID         string
		groupID        string
		bannedFromRoom bool
	)

	switch scope.Kind {
	case MatrixScopeServer:
		if !PermissionAppliesAtScope(perm, ScopeServer) {
			return PermissionMatrixCell{}, false, nil
		}
		override, err = c.GetUserExplicitServerOverride(ctx, userID, perm)
	case MatrixScopeDM:
		if !PermissionAppliesAtScope(perm, ScopeDM) {
			return PermissionMatrixCell{}, false, nil
		}
		kind = KindDM
		override, err = c.GetUserExplicitDMOverride(ctx, userID, perm)
	case MatrixScopeGroup:
		if !PermissionAppliesAtScope(perm, ScopeGroup) {
			return PermissionMatrixCell{}, false, nil
		}
		groupID = scopeRefID(scope.ID, "group:")
		override, err = c.GetUserExplicitGroupOverride(ctx, groupID, userID, perm)
	case MatrixScopeRoom:
		if !PermissionAppliesAtScope(perm, ScopeRoom) {
			return PermissionMatrixCell{}, false, nil
		}
		roomID = scopeRefID(scope.ID, "room:")
		override, err = c.GetUserExplicitRoomOverride(ctx, roomID, userID, perm)
		// An active room ban blocks joining whatever the permissions say.
		bannedFromRoom = perm == PermRoomJoin && c.roomModel.isRoomBanActive(roomID, userID, time.Now())
	default:
		return PermissionMatrixCell{}, false, fmt.Errorf("%w: unknown scope kind %q", ErrInvalidArgument, scope.Kind)
	}
	if err != nil {
		return PermissionMatrixCell{}, false, err
	}

	// Show the account's own access, not the viewer's: once without and, for
	// humans, once with privileged mode (ADR-105). Bots have no privileged mode.
	effective, err := c.PermResolver().resolveForAccount(ctx, userID, kind, roomID, groupID, perm, false)
	if err != nil {
		return PermissionMatrixCell{}, false, err
	}
	cell := PermissionMatrixCell{
		Permission: string(perm),
		ScopeID:    scope.ID,
		Override:   matrixDecisionFromCoreDecision(override),
	}
	if isBot, _, _ := c.userModel.isBotAndOwner(userID); !isBot {
		privileged, err := c.PermResolver().resolveForAccount(ctx, userID, kind, roomID, groupID, perm, true)
		if err != nil {
			return PermissionMatrixCell{}, false, err
		}
		if bannedFromRoom {
			privileged = DecisionDeny
		}
		cell.EffectiveWithPrivilegedMode = matrixDecisionFromCoreDecision(privileged)
	}
	if bannedFromRoom {
		effective = DecisionDeny
	}
	cell.Effective = matrixDecisionFromCoreDecision(effective)
	return cell, true, nil
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

// mergeInheritedAllows combines the group allows of a role with its server
// allows into the allows that a room inherits.
func mergeInheritedAllows(groupAllow, serverAllow []Permission) []string {
	allow := corePermsToStrings(groupAllow)
	for _, perm := range serverAllow {
		if !slices.Contains(groupAllow, perm) {
			allow = append(allow, string(perm))
		}
	}
	return allow
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

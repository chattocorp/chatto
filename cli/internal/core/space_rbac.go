package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/core/rbac"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// listKeysWithPattern returns all keys matching a pattern from a KV bucket.
func listKeysWithPattern(ctx context.Context, kv jetstream.KeyValue, pattern string) ([]string, error) {
	lister, err := kv.ListKeysFiltered(ctx, pattern)
	if err != nil {
		return nil, err
	}

	var keys []string
	for key := range lister.Keys() {
		keys = append(keys, key)
	}
	return keys, nil
}

// ============================================================================
// Role CRUD Operations
// ============================================================================

// SystemActorID is used for internal/bootstrap operations that bypass permission checks.
// SECURITY: This value cannot be forged by external users because user IDs are always
// generated with a "U" prefix (via NewUserID), e.g., "U1234567890abcd". The string "system"
// can never match a valid user ID.
const SystemActorID = "system"

// CreateRole creates a new role in a space with auto-assigned position.
// The role name must be lowercase letters/numbers and dashes only.
// Pass SystemActorID to bypass permission check (for internal/bootstrap use).
func (c *ChattoCore) CreateRole(ctx context.Context, actorID, spaceID, name, displayName, description string) (*corev1.Role, error) {
	// Check permission (skip if actorID is system - internal/bootstrap use)
	if actorID != SystemActorID {
		if err := c.requireSpacePermission(ctx, spaceID, actorID, PermRoleManage); err != nil {
			return nil, err
		}
	}

	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return nil, err
	}

	role, err := engine.CreateRole(ctx, name, displayName, description)
	if err != nil {
		if errors.Is(err, rbac.ErrRoleAlreadyExists) {
			return nil, ErrRoleAlreadyExists
		}
		if errors.Is(err, rbac.ErrInvalidRoleName) {
			return nil, ErrInvalidRoleName
		}
		return nil, err
	}

	c.logger.Info("Created role", "name", name, "display_name", displayName, "space_id", spaceID, "position", role.Position)

	return role, nil
}

// CreateRoleWithPosition creates a new role in a space with an explicit position.
// The role name must be lowercase letters/numbers and dashes only.
// Pass SystemActorID to bypass permission check (for internal/bootstrap use).
func (c *ChattoCore) CreateRoleWithPosition(ctx context.Context, actorID, spaceID, name, displayName, description string, position int32) (*corev1.Role, error) {
	// Check permission (skip if actorID is system - internal/bootstrap use)
	if actorID != SystemActorID {
		if err := c.requireSpacePermission(ctx, spaceID, actorID, PermRoleManage); err != nil {
			return nil, err
		}
	}

	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return nil, err
	}

	role, err := engine.CreateRoleWithPosition(ctx, name, displayName, description, position)
	if err != nil {
		if errors.Is(err, rbac.ErrRoleAlreadyExists) {
			return nil, ErrRoleAlreadyExists
		}
		if errors.Is(err, rbac.ErrInvalidRoleName) {
			return nil, ErrInvalidRoleName
		}
		return nil, err
	}

	c.logger.Info("Created role", "name", name, "display_name", displayName, "space_id", spaceID, "position", position)

	return role, nil
}

// GetRole retrieves a role by name from a space.
func (c *ChattoCore) GetRole(ctx context.Context, spaceID, name string) (*corev1.Role, error) {
	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return nil, err
	}

	role, err := engine.GetRole(ctx, name)
	if err != nil {
		if errors.Is(err, rbac.ErrRoleNotFound) {
			return nil, ErrRoleNotFound
		}
		return nil, err
	}

	return &corev1.Role{
		Name:        role.Name,
		DisplayName: role.DisplayName,
		Description: role.Description,
		Position:    role.Position,
	}, nil
}

// ListRoles retrieves all roles in a space.
func (c *ChattoCore) ListRoles(ctx context.Context, spaceID string) ([]*corev1.Role, error) {
	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return nil, err
	}

	roles, err := engine.ListRoles(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]*corev1.Role, len(roles))
	for i, role := range roles {
		result[i] = &corev1.Role{
			Name:        role.Name,
			DisplayName: role.DisplayName,
			Description: role.Description,
			Position:    role.Position,
		}
	}

	return result, nil
}

// UpdateRole updates an existing role's display name and description.
// The role name (identifier) cannot be changed.
func (c *ChattoCore) UpdateRole(ctx context.Context, actorID, spaceID, name, displayName, description string) (*corev1.Role, error) {
	// Check permission
	if err := c.requireSpacePermission(ctx, spaceID, actorID, PermRoleManage); err != nil {
		return nil, err
	}

	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return nil, err
	}

	role, err := engine.UpdateRole(ctx, name, displayName, description)
	if err != nil {
		if errors.Is(err, rbac.ErrRoleNotFound) {
			return nil, ErrRoleNotFound
		}
		return nil, err
	}

	c.logger.Info("Updated role", "name", name, "display_name", displayName, "space_id", spaceID)

	return &corev1.Role{
		Name:        role.Name,
		DisplayName: role.DisplayName,
		Description: role.Description,
		Position:    role.Position,
	}, nil
}

// DeleteRole deletes a role from a space.
// Returns ErrCannotDeleteSystemRole if attempting to delete a system role (owner, moderator, everyone).
func (c *ChattoCore) DeleteRole(ctx context.Context, actorID, spaceID, name string) error {
	// Check permission
	if err := c.requireSpacePermission(ctx, spaceID, actorID, PermRoleManage); err != nil {
		return err
	}

	// Check if it's a system role (before calling engine, to keep same behavior)
	if IsSystemRole(name) {
		return ErrCannotDeleteSystemRole
	}

	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return err
	}

	if err := engine.DeleteRole(ctx, name); err != nil {
		if errors.Is(err, rbac.ErrRoleNotFound) {
			return ErrRoleNotFound
		}
		if errors.Is(err, rbac.ErrCannotDeleteSystemRole) {
			return ErrCannotDeleteSystemRole
		}
		return err
	}

	c.logger.Info("Deleted role", "name", name, "space_id", spaceID)

	return nil
}

// ============================================================================
// Permission Assignment Operations
// ============================================================================

// GrantSpacePermission grants a permission to a role.
// Pass SystemActorID to bypass permission check (for internal/bootstrap use).
func (c *ChattoCore) GrantSpacePermission(ctx context.Context, actorID, spaceID, roleName string, perm Permission) error {
	// Check permission (skip if actorID is system - internal/bootstrap use)
	if actorID != SystemActorID {
		if err := c.requireSpacePermission(ctx, spaceID, actorID, PermRoleManage); err != nil {
			return err
		}
	}

	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return err
	}

	// Convert permission to key parts
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("%w: %s", ErrInvalidPermission, perm)
	}

	if err := engine.GrantRolePermission(ctx, roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny); err != nil {
		if errors.Is(err, rbac.ErrRoleNotFound) {
			return ErrRoleNotFound
		}
		if errors.Is(err, rbac.ErrInvalidPermission) {
			return ErrInvalidPermission
		}
		return err
	}

	c.logger.Debug("Granted permission", "role", roleName, "permission", perm, "space_id", spaceID)

	return nil
}

// RevokeSpacePermission revokes a permission grant from a role.
// This only removes grants, not denials. Use ClearSpacePermissionState to remove both.
func (c *ChattoCore) RevokeSpacePermission(ctx context.Context, actorID, spaceID, roleName string, perm Permission) error {
	// Check permission
	if err := c.requireSpacePermission(ctx, spaceID, actorID, PermRoleManage); err != nil {
		return err
	}

	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return err
	}

	// Convert permission to key parts
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("%w: %s", ErrInvalidPermission, perm)
	}

	if err := engine.RevokeRolePermission(ctx, roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny); err != nil {
		if errors.Is(err, rbac.ErrRoleNotFound) {
			return ErrRoleNotFound
		}
		return err
	}

	c.logger.Debug("Revoked permission", "role", roleName, "permission", perm, "space_id", spaceID)

	return nil
}

// DenySpacePermission denies a permission for a role.
// Users with this role will be blocked from this permission regardless of what other roles grant it.
func (c *ChattoCore) DenySpacePermission(ctx context.Context, actorID, spaceID, roleName string, perm Permission) error {
	// Check permission
	if err := c.requireSpacePermission(ctx, spaceID, actorID, PermRoleManage); err != nil {
		return err
	}

	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return err
	}

	// Convert permission to key parts
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("%w: %s", ErrInvalidPermission, perm)
	}

	if err := engine.DenyRolePermission(ctx, roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny); err != nil {
		if errors.Is(err, rbac.ErrRoleNotFound) {
			return ErrRoleNotFound
		}
		if errors.Is(err, rbac.ErrInvalidPermission) {
			return ErrInvalidPermission
		}
		return err
	}

	c.logger.Debug("Denied permission", "role", roleName, "permission", perm, "space_id", spaceID)

	return nil
}

// ClearSpacePermissionState clears both grant and denial for a permission on a role.
// This returns the permission to a neutral state.
func (c *ChattoCore) ClearSpacePermissionState(ctx context.Context, actorID, spaceID, roleName string, perm Permission) error {
	// Check permission
	if err := c.requireSpacePermission(ctx, spaceID, actorID, PermRoleManage); err != nil {
		return err
	}

	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return err
	}

	// Convert permission to key parts
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("%w: %s", ErrInvalidPermission, perm)
	}

	if err := engine.ClearRolePermissionState(ctx, roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny); err != nil {
		if errors.Is(err, rbac.ErrRoleNotFound) {
			return ErrRoleNotFound
		}
		return err
	}

	c.logger.Debug("Cleared permission state", "role", roleName, "permission", perm, "space_id", spaceID)

	return nil
}

// GetRolePermissions returns all permissions granted to a role.
func (c *ChattoCore) GetRolePermissions(ctx context.Context, spaceID, roleName string) ([]Permission, error) {
	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return nil, err
	}

	perms, err := engine.GetRolePermissions(ctx, roleName)
	if err != nil {
		if errors.Is(err, rbac.ErrRoleNotFound) {
			return nil, ErrRoleNotFound
		}
		return nil, err
	}

	var result []Permission
	for _, p := range perms {
		perm := ReconstructPermission(p.Verb, p.ObjectType)
		if perm != "" {
			result = append(result, Permission(perm))
		}
	}

	return result, nil
}

// GetRolePermissionDenials returns all permissions denied by a role.
func (c *ChattoCore) GetRolePermissionDenials(ctx context.Context, spaceID, roleName string) ([]Permission, error) {
	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return nil, err
	}

	denials, err := engine.GetRolePermissionDenials(ctx, roleName)
	if err != nil {
		if errors.Is(err, rbac.ErrRoleNotFound) {
			return nil, ErrRoleNotFound
		}
		return nil, err
	}

	var result []Permission
	for _, p := range denials {
		perm := ReconstructPermission(p.Verb, p.ObjectType)
		if perm != "" {
			result = append(result, Permission(perm))
		}
	}

	return result, nil
}

// ============================================================================
// Room-Level Permission Wrappers (with authorization)
// ============================================================================

// GrantRoomRolePermission grants a permission to a role at the room level.
// Authorization: Caller must have PermRoleManage in the space.
func (c *ChattoCore) GrantRoomRolePermission(ctx context.Context, actorID, spaceID, roomID, roleName string, perm Permission) error {
	if err := c.requireSpacePermission(ctx, spaceID, actorID, PermRoleManage); err != nil {
		return err
	}

	return c.grantRoomRolePermissionInternal(ctx, spaceID, roomID, roleName, perm)
}

// DenyRoomRolePermission denies a permission for a role at the room level.
// Authorization: Caller must have PermRoleManage in the space.
func (c *ChattoCore) DenyRoomRolePermission(ctx context.Context, actorID, spaceID, roomID, roleName string, perm Permission) error {
	if err := c.requireSpacePermission(ctx, spaceID, actorID, PermRoleManage); err != nil {
		return err
	}

	return c.denyRoomRolePermissionInternal(ctx, spaceID, roomID, roleName, perm)
}

// ClearRoomRolePermission clears both grant and denial for a permission at room level.
// Authorization: Caller must have PermRoleManage in the space.
func (c *ChattoCore) ClearRoomRolePermission(ctx context.Context, actorID, spaceID, roomID, roleName string, perm Permission) error {
	if err := c.requireSpacePermission(ctx, spaceID, actorID, PermRoleManage); err != nil {
		return err
	}

	return c.clearRoomRolePermissionInternal(ctx, spaceID, roomID, roleName, perm)
}

// GetRoleRoomPermissions returns the room-level grants and denials for a role in a specific room.
func (c *ChattoCore) GetRoleRoomPermissions(ctx context.Context, spaceID, roomID, roleName string) (grants []Permission, denials []Permission, err error) {
	kv, err := c.getSpaceRBACKV(ctx, spaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get space RBAC KV: %w", err)
	}

	// Scan all allow keys for this role and filter to roomID
	allowKeys, err := listKeysWithPattern(ctx, kv, rbac.AllowPatternForSubject(roleName))
	if err != nil {
		return nil, nil, err
	}
	for _, key := range allowKeys {
		parts := rbac.ParseAllowKey(key)
		if parts.ObjectId == roomID {
			perm := ReconstructPermission(parts.Verb, parts.ObjectType)
			if perm != "" {
				grants = append(grants, Permission(perm))
			}
		}
	}

	// Scan all deny keys for this role and filter to roomID
	denyKeys, err := listKeysWithPattern(ctx, kv, rbac.DenyPatternForSubject(roleName))
	if err != nil {
		return nil, nil, err
	}
	for _, key := range denyKeys {
		parts := rbac.ParseDenyKey(key)
		if parts.ObjectId == roomID {
			perm := ReconstructPermission(parts.Verb, parts.ObjectType)
			if perm != "" {
				denials = append(denials, Permission(perm))
			}
		}
	}

	return grants, denials, nil
}

// ============================================================================
// Role Assignment Operations
// ============================================================================

// ErrCannotAssignHigherRole is returned when a user tries to assign a role equal to or higher than their own.
var ErrCannotAssignHigherRole = errors.New("cannot assign role equal to or higher than your own")

// AssignRole assigns a role to a user.
// Pass SystemActorID to bypass permission check (for internal/bootstrap use).
// Hierarchy check: actor must outrank the role being assigned (actor's position < role's position).
func (c *ChattoCore) AssignRole(ctx context.Context, actorID, spaceID, userID, roleName string) error {
	// Member role is implicit for all space members - no need to store in KV
	if roleName == RoleEveryone {
		return nil // No-op, they already have it via space membership
	}

	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return err
	}

	// Check permission and hierarchy (skip if actorID is system - internal/bootstrap use)
	if actorID != SystemActorID {
		if err := c.requireSpacePermission(ctx, spaceID, actorID, PermRoleAssign); err != nil {
			return err
		}

		// Hierarchy check: actor must outrank the role being assigned
		role, err := engine.GetRole(ctx, roleName)
		if err != nil {
			if errors.Is(err, rbac.ErrRoleNotFound) {
				return ErrRoleNotFound
			}
			return err
		}
		canManage, err := engine.CanUserManageRole(ctx, actorID, role.Position)
		if err != nil {
			return err
		}
		if !canManage {
			return ErrCannotAssignHigherRole
		}
	}

	if err := engine.AssignRole(ctx, userID, roleName); err != nil {
		if errors.Is(err, rbac.ErrRoleNotFound) {
			return ErrRoleNotFound
		}
		return err
	}

	c.logger.Debug("Assigned role", "role", roleName, "user_id", userID, "space_id", spaceID)

	return nil
}

// ErrCannotRevokeSelfAdmin is returned when an admin tries to remove their own admin role.
var ErrCannotRevokeSelfAdmin = errors.New("cannot revoke your own admin role")

// ErrCannotRevokeHigherRole is returned when a user tries to revoke a role equal to or higher than their own.
var ErrCannotRevokeHigherRole = errors.New("cannot revoke role equal to or higher than your own")

// ErrCannotManageHigherUser is returned when a user tries to modify roles for a user with equal or higher rank.
var ErrCannotManageHigherUser = errors.New("cannot modify roles for a user with equal or higher rank")

// RevokeRole removes a role from a user.
// Returns ErrCannotRevokeSelfAdmin if an admin tries to remove their own admin role.
// Hierarchy checks:
//   - Actor must outrank the role being revoked (actor's position < role's position)
//   - Actor must outrank the target user (actor's position < target's position)
func (c *ChattoCore) RevokeRole(ctx context.Context, actorID, spaceID, userID, roleName string) error {
	// Member role is implicit and cannot be revoked (leave space instead)
	if roleName == RoleEveryone {
		return nil // No-op for idempotency
	}

	// Check permission
	if err := c.requireSpacePermission(ctx, spaceID, actorID, PermRoleAssign); err != nil {
		return err
	}

	// Prevent owners from removing their own owner role
	if roleName == RoleOwner && actorID == userID {
		return ErrCannotRevokeSelfAdmin
	}

	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return err
	}

	// Hierarchy check 1: actor must outrank the role being revoked
	role, err := engine.GetRole(ctx, roleName)
	if err != nil {
		if errors.Is(err, rbac.ErrRoleNotFound) {
			return ErrRoleNotFound
		}
		return err
	}
	canManageRole, err := engine.CanUserManageRole(ctx, actorID, role.Position)
	if err != nil {
		return err
	}
	if !canManageRole {
		return ErrCannotRevokeHigherRole
	}

	// Hierarchy check 2: actor must outrank the target user
	// This prevents peer-level manipulation (e.g., Admin A demoting Admin B)
	if actorID != userID {
		canManageUser, err := c.CanManageSpaceUser(ctx, spaceID, actorID, userID)
		if err != nil {
			return err
		}
		if !canManageUser {
			return ErrCannotManageHigherUser
		}
	}

	if err := engine.RevokeRole(ctx, userID, roleName); err != nil {
		if errors.Is(err, rbac.ErrRoleNotFound) {
			return ErrRoleNotFound
		}
		return err
	}

	c.logger.Debug("Revoked role", "role", roleName, "user_id", userID, "space_id", spaceID)

	return nil
}

// CanManageSpaceUser checks if actor can manage target based on role hierarchy.
// Returns true if actor's highest role position < target's highest role position.
// This is used for operations like kick, mute, etc.
func (c *ChattoCore) CanManageSpaceUser(ctx context.Context, spaceID, actorID, targetID string) (bool, error) {
	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return false, err
	}

	// Get actor's highest position (accounting for virtual member role)
	actorPos, err := c.getUserHighestPositionWithMember(ctx, engine, spaceID, actorID)
	if err != nil {
		return false, err
	}

	// Get target's highest position (accounting for virtual member role)
	targetPos, err := c.getUserHighestPositionWithMember(ctx, engine, spaceID, targetID)
	if err != nil {
		return false, err
	}

	return actorPos < targetPos, nil
}

// getUserHighestPositionWithMember returns the user's highest position, accounting for the virtual member role.
// If the user has no explicit roles, returns PositionEveryone (lowest rank).
//
// Note: This function is used by CanManageSpaceUser which is called after permission checks
// that already verify space membership. Non-members would be rejected before reaching here.
// Members with no explicit roles effectively have the "everyone" role (lowest rank).
func (c *ChattoCore) getUserHighestPositionWithMember(ctx context.Context, engine *rbac.Engine, spaceID, userID string) (int32, error) {
	pos, err := engine.GetUserHighestPosition(ctx, userID)
	if err != nil {
		return rbac.PositionEveryone, err
	}
	return pos, nil
}

// ReorderSpaceRoles reorders custom roles in a space.
// System roles (owner, moderator, everyone) maintain fixed positions and should not be included.
// Positions are assigned based on array index (first role = position 1, second = 2, etc).
// Returns all roles sorted by position.
func (c *ChattoCore) ReorderSpaceRoles(ctx context.Context, actorID, spaceID string, roleNames []string) ([]*corev1.Role, error) {
	// Check permission
	if err := c.requireSpacePermission(ctx, spaceID, actorID, PermRoleManage); err != nil {
		return nil, err
	}

	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return nil, err
	}

	roles, err := engine.ReorderRoles(ctx, roleNames)
	if err != nil {
		return nil, err
	}

	c.logger.Info("Reordered roles", "space_id", spaceID, "order", roleNames)

	return roles, nil
}

// GetUserRoles returns all roles assigned to a user in a space.
// The virtual "everyone" role is included automatically for space members.
func (c *ChattoCore) GetUserRoles(ctx context.Context, spaceID, userID string) ([]string, error) {
	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return nil, err
	}

	// Get explicitly assigned roles from KV
	roles, err := engine.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, err
	}

	// Add virtual "everyone" role if user is a space member
	isMember, err := c.SpaceMembershipExists(ctx, userID, spaceID)
	if err != nil {
		return nil, err
	}
	if isMember {
		roles = append([]string{RoleEveryone}, roles...)
	}

	return roles, nil
}

// GetUserEffectiveSpacePermissions returns all permissions the user effectively has in a space.
// Delegates to PermissionResolver.HasSpacePermission for each space-scoped permission,
// ensuring consistent resolution logic (deny-always-wins, instance-authority-first).
func (c *ChattoCore) GetUserEffectiveSpacePermissions(ctx context.Context, spaceID, userID string) ([]Permission, error) {
	// DM space uses simplified permissions - return a fixed set
	if IsDMSpace(spaceID) {
		return []Permission{
			PermRoomJoin,
			PermMessagePost,
			PermMessageReply,
			PermMessageReact,
			PermMessageEditOwn,
			PermMessageDeleteOwn,
		}, nil
	}

	var result []Permission
	for _, permMeta := range PermissionsForScope(ScopeSpace) {
		perm := Permission(permMeta.Permission)
		has, err := c.permissionResolver.HasSpacePermission(ctx, userID, spaceID, perm)
		if err != nil {
			return nil, fmt.Errorf("failed to check permission %s: %w", perm, err)
		}
		if has {
			result = append(result, perm)
		}
	}

	return result, nil
}

// GetRoleUsers returns all users assigned to a role in a space.
// For the "member" role, this returns all space members (since it's implicit).
func (c *ChattoCore) GetRoleUsers(ctx context.Context, spaceID, roleName string) ([]string, error) {
	// Member role is implicit - return all space members
	if roleName == RoleEveryone {
		return c.GetSpaceMemberIDs(ctx, spaceID)
	}

	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return nil, err
	}

	users, err := engine.GetRoleUsers(ctx, roleName)
	if err != nil {
		if errors.Is(err, rbac.ErrRoleNotFound) {
			return nil, ErrRoleNotFound
		}
		return nil, err
	}

	return users, nil
}

// IsSpaceAdmin checks if a user has the admin role in a space.
func (c *ChattoCore) IsSpaceAdmin(ctx context.Context, spaceID, userID string) (bool, error) {
	// DM space has no admin role
	if IsDMSpace(spaceID) {
		return false, nil
	}

	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return false, err
	}

	return engine.HasUserAdmin(ctx, userID)
}

// RevokeAllUserRoles removes all role assignments for a user in a space.
// This is called during cleanup when a user leaves a space.
// Authorization: Internal use only (no permission check needed).
func (c *ChattoCore) RevokeAllUserRoles(ctx context.Context, spaceID, userID string) error {
	// DM space has no roles
	if IsDMSpace(spaceID) {
		return nil
	}

	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return err
	}

	if err := engine.RevokeAllUserRoles(ctx, userID); err != nil {
		return fmt.Errorf("failed to revoke user roles: %w", err)
	}

	c.logger.Debug("Revoked all roles for user in space", "user_id", userID, "space_id", spaceID)
	return nil
}

// ============================================================================
// Permission Checking (internal)
// ============================================================================

// hasSpacePermission checks if a user has a specific permission in a space.
// This delegates to the unified PermissionResolver which implements hierarchical resolution:
// instance < space < room (more specific scopes override less specific ones).
//
// This is an internal building block. Use the Can* functions in can.go for
// authorization checks, as they may include additional business logic.
func (c *ChattoCore) hasSpacePermission(ctx context.Context, spaceID, userID string, perm Permission) (bool, error) {
	// Delegate to the unified PermissionResolver
	return c.permissionResolver.HasSpacePermission(ctx, userID, spaceID, perm)
}

// hasRoomPermission checks if a user has a permission at the room level.
// Uses the deny-always-wins, instance-authority-first resolution model with room overrides.
func (c *ChattoCore) hasRoomPermission(ctx context.Context, spaceID, roomID, userID string, perm Permission) (bool, error) {
	return c.permissionResolver.HasRoomPermission(ctx, userID, spaceID, roomID, perm)
}

// requireSpacePermission checks if a user has a specific permission and returns
// ErrPermissionDenied if not. This is an internal helper for core operations.
//
// Prefer using Can* functions for authorization checks, as they may include
// additional business logic beyond simple permission checks.
func (c *ChattoCore) requireSpacePermission(ctx context.Context, spaceID, userID string, perm Permission) error {
	has, err := c.hasSpacePermission(ctx, spaceID, userID, perm)
	if err != nil {
		return err
	}
	if !has {
		return ErrPermissionDenied
	}
	return nil
}

// ============================================================================
// Default Roles Setup
// ============================================================================

// CreateDefaultRoles creates the default roles and permissions for a space.
// Owner, admin, and moderator are explicitly created in KV.
// Everyone role is virtual (not stored in KV).
// This should be called when a space is created.
func (c *ChattoCore) CreateDefaultRoles(ctx context.Context, spaceID string) error {
	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("failed to get space RBAC engine: %w", err)
	}

	// Create owner role (position 0) - explicitly stored in KV
	if _, err := engine.CreateRoleWithPosition(ctx, RoleOwner, "Owner", "Full space control", rbac.PositionOwner); err != nil {
		// Ignore if already exists (idempotent)
		if !errors.Is(err, rbac.ErrRoleAlreadyExists) {
			return fmt.Errorf("failed to create owner role: %w", err)
		}
	}

	// Create admin role (position 1) - explicitly stored in KV
	if _, err := engine.CreateRoleWithPosition(ctx, RoleAdmin, "Admin", "Can manage space settings, roles, and members", rbac.PositionAdmin); err != nil {
		if !errors.Is(err, rbac.ErrRoleAlreadyExists) {
			return fmt.Errorf("failed to create admin role: %w", err)
		}
	}

	// Create moderator role (position 2) - explicitly stored in KV
	if _, err := engine.CreateRoleWithPosition(ctx, RoleModerator, "Moderator", "Can manage rooms and remove members", rbac.PositionModerator); err != nil {
		if !errors.Is(err, rbac.ErrRoleAlreadyExists) {
			return fmt.Errorf("failed to create moderator role: %w", err)
		}
	}

	// Initialize permissions using the permission model
	// This writes permissions with the unified KV key patterns
	if err := c.InitSpaceDefaults(ctx, spaceID); err != nil {
		return fmt.Errorf("failed to initialize unified space defaults: %w", err)
	}

	c.logger.Info("Created default roles", "space_id", spaceID)

	return nil
}

// DefaultInstanceEveryoneSpacePermissions returns the space permissions granted to
// the instance:everyone role by default when a new space is created.
// These permissions are for users who are NOT yet members of the space.
func DefaultInstanceEveryoneSpacePermissions() []Permission {
	return []Permission{
		PermSpaceJoin, // Allows authenticated users to join the space
	}
}

// HasSpaceUserPermissionViaRoles checks if a user would have a permission through roles only
// (ignoring any user-specific grants/denials). Used for UI to show baseline state.
// Implements deny-override: if ANY role denies, permission is blocked regardless of grants.
func (c *ChattoCore) HasSpaceUserPermissionViaRoles(ctx context.Context, spaceID, userID string, perm Permission) (bool, error) {
	// DM space uses simplified permissions
	if IsDMSpace(spaceID) {
		return isDMPermissionAllowed(perm), nil
	}

	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return false, err
	}

	// Convert permission to key parts
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return false, fmt.Errorf("%w: %s", ErrInvalidPermission, perm)
	}

	// Get user's explicitly assigned roles
	roles, err := engine.GetUserRoles(ctx, userID)
	if err != nil {
		return false, err
	}

	// Check if user is a space member (for implicit member role)
	isMember, err := c.SpaceMembershipExists(ctx, userID, spaceID)
	if err != nil {
		return false, err
	}

	// Note: Admin roles are NOT special-cased - they work like any other role
	// and must have permissions explicitly granted.

	// Check role denials first (deny-override pattern)
	// Check member role denials (if user is a member)
	if isMember {
		memberDenies, err := engine.RoleHasPermissionDenial(ctx, RoleEveryone, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)
		if err != nil {
			return false, err
		}
		if memberDenies {
			return false, nil
		}
	}

	// Check explicit role denials (including admin - admin is no longer immune)
	for _, roleName := range roles {
		if roleName == RoleEveryone {
			continue // Already checked above
		}
		denies, err := engine.RoleHasPermissionDenial(ctx, roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)
		if err != nil {
			return false, err
		}
		if denies {
			return false, nil
		}
	}

	// Check role grants
	// Check member role grants (if user is a member)
	if isMember {
		memberHasPerm, err := engine.RoleHasPermission(ctx, RoleEveryone, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)
		if err != nil {
			return false, err
		}
		if memberHasPerm {
			return true, nil
		}
	}

	// Check explicit role grants (including admin - admin must have permissions explicitly granted)
	for _, roleName := range roles {
		if roleName == RoleEveryone {
			continue // Already checked above
		}
		has, err := engine.RoleHasPermission(ctx, roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)
		if err != nil {
			return false, err
		}
		if has {
			return true, nil
		}
	}

	return false, nil
}

// HasSpaceUserPermissionDeniedViaRoles checks if any of the user's roles deny a specific permission.
// Used for UI to show when a permission is blocked via roles.
func (c *ChattoCore) HasSpaceUserPermissionDeniedViaRoles(ctx context.Context, spaceID, userID string, perm Permission) (bool, error) {
	// DM space uses simplified permissions - no role denials
	if IsDMSpace(spaceID) {
		return false, nil
	}

	engine, err := c.spaceRBACEngine(ctx, spaceID)
	if err != nil {
		return false, err
	}

	// Convert permission to key parts
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return false, fmt.Errorf("%w: %s", ErrInvalidPermission, perm)
	}

	// Get user's explicitly assigned roles
	roles, err := engine.GetUserRoles(ctx, userID)
	if err != nil {
		return false, err
	}

	// Check if user is a space member (for implicit member role)
	isMember, err := c.SpaceMembershipExists(ctx, userID, spaceID)
	if err != nil {
		return false, err
	}

	// Check member role denials (if user is a member)
	if isMember {
		memberDenies, err := engine.RoleHasPermissionDenial(ctx, RoleEveryone, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)
		if err != nil {
			return false, err
		}
		if memberDenies {
			return true, nil
		}
	}

	// Check explicit role denials
	for _, roleName := range roles {
		if roleName == RoleEveryone {
			continue
		}
		denies, err := engine.RoleHasPermissionDenial(ctx, roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)
		if err != nil {
			return false, err
		}
		if denies {
			return true, nil
		}
	}

	return false, nil
}

// ============================================================================
// Instance Role Space Permissions
// ============================================================================
// These functions allow space admins to configure space-level permissions
// for users based on their instance roles. Instance roles appear in the
// space admin UI with an "instance:" prefix.

// grantInstanceRoleSpacePermissionInternal grants a space permission to an instance role.
// Internal use only (no authorization check) - for use during space creation.
func (c *ChattoCore) grantInstanceRoleSpacePermissionInternal(ctx context.Context, spaceID, instanceRole string, perm Permission) error {
	if err := c.GrantSpaceRolePermission(ctx, spaceID, instanceRole, perm); err != nil {
		return fmt.Errorf("failed to grant instance role permission: %w", err)
	}

	c.logger.Debug("Granted instance role space permission (internal)",
		"instance_role", instanceRole, "permission", perm, "space_id", spaceID)
	return nil
}

// GrantInstanceRoleSpacePermission grants a space permission to an instance role.
// Space admins can use this to give users with specific instance roles extra
// permissions in their space (e.g., "instance:staff" gets "rooms.create").
func (c *ChattoCore) GrantInstanceRoleSpacePermission(ctx context.Context, actorID, spaceID, instanceRole string, perm Permission) error {
	// Check permission
	if err := c.requireSpacePermission(ctx, spaceID, actorID, PermRoleManage); err != nil {
		return err
	}

	// Validate instance role exists
	_, err := c.GetInstanceRole(ctx, instanceRole)
	if err != nil {
		return err
	}

	// Validate permission
	if err := ValidatePermission(perm); err != nil {
		return err
	}

	return c.grantInstanceRoleSpacePermissionInternal(ctx, spaceID, instanceRole, perm)
}

// DenyInstanceRoleSpacePermission denies a space permission for an instance role.
// Space admins can use this to restrict users with specific instance roles
// (e.g., deny "rooms.join" to "instance:member" to make a staff-only space).
func (c *ChattoCore) DenyInstanceRoleSpacePermission(ctx context.Context, actorID, spaceID, instanceRole string, perm Permission) error {
	// Check permission
	if err := c.requireSpacePermission(ctx, spaceID, actorID, PermRoleManage); err != nil {
		return err
	}

	// Validate instance role exists
	_, err := c.GetInstanceRole(ctx, instanceRole)
	if err != nil {
		return err
	}

	// Validate permission
	if err := ValidatePermission(perm); err != nil {
		return err
	}

	if err := c.DenySpaceRolePermission(ctx, spaceID, instanceRole, perm); err != nil {
		return fmt.Errorf("failed to deny instance role permission: %w", err)
	}

	c.logger.Debug("Denied instance role space permission",
		"instance_role", instanceRole, "permission", perm, "space_id", spaceID)
	return nil
}

// ClearInstanceRoleSpacePermission clears both grant and denial for a permission,
// returning it to a neutral state (no space-level configuration for this instance role).
func (c *ChattoCore) ClearInstanceRoleSpacePermission(ctx context.Context, actorID, spaceID, instanceRole string, perm Permission) error {
	// Check permission
	if err := c.requireSpacePermission(ctx, spaceID, actorID, PermRoleManage); err != nil {
		return err
	}

	// Validate instance role exists
	_, err := c.GetInstanceRole(ctx, instanceRole)
	if err != nil {
		return err
	}

	// Validate permission
	if err := ValidatePermission(perm); err != nil {
		return err
	}

	if err := c.ClearSpaceRolePermission(ctx, spaceID, instanceRole, perm); err != nil {
		return fmt.Errorf("failed to clear instance role permission: %w", err)
	}

	c.logger.Debug("Cleared instance role space permission",
		"instance_role", instanceRole, "permission", perm, "space_id", spaceID)
	return nil
}

// GetInstanceRoleSpacePermissions returns the space permissions granted and denied
// for an instance role in a specific space.
func (c *ChattoCore) GetInstanceRoleSpacePermissions(ctx context.Context, spaceID, instanceRole string) (grants []Permission, denials []Permission, err error) {
	kv, err := c.getSpaceRBACKV(ctx, spaceID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get space RBAC KV: %w", err)
	}

	// Instance role name is used as-is in the KV key (e.g., "instance-admin", "instance-moderator")
	// Key format: allow.{instanceRole}.{verb}.{objectType}.{objectId}
	grantPattern := rbac.AllowPatternForSubject(instanceRole)
	grantLister, err := kv.ListKeysFiltered(ctx, grantPattern)
	if err == nil {
		for key := range grantLister.Keys() {
			parts := rbac.ParseAllowKey(key)
			if parts.Verb != "" && parts.ObjectType != "" {
				perm := ReconstructPermission(parts.Verb, parts.ObjectType)
				if perm != "" {
					grants = append(grants, Permission(perm))
				}
			}
		}
	}

	// Key format: deny.{instanceRole}.{verb}.{objectType}.{objectId}
	denyPattern := rbac.DenyPatternForSubject(instanceRole)
	denyLister, err := kv.ListKeysFiltered(ctx, denyPattern)
	if err == nil {
		for key := range denyLister.Keys() {
			parts := rbac.ParseDenyKey(key)
			if parts.Verb != "" && parts.ObjectType != "" {
				perm := ReconstructPermission(parts.Verb, parts.ObjectType)
				if perm != "" {
					denials = append(denials, Permission(perm))
				}
			}
		}
	}

	return grants, denials, nil
}

// InstanceRoleSpaceConfig represents an instance role's space-level permission configuration.
type InstanceRoleSpaceConfig struct {
	Role    *RoleWithPermissions // The instance role info
	Grants  []Permission                 // Space permissions granted to this instance role
	Denials []Permission                 // Space permissions denied for this instance role
}

// ListInstanceRolesWithSpacePermissions returns instance-only roles with their
// space-level permission configurations. Universal roles (everyone)
// are excluded since they're managed as space roles.
func (c *ChattoCore) ListInstanceRolesWithSpacePermissions(ctx context.Context, spaceID string) ([]InstanceRoleSpaceConfig, error) {
	// Get all instance roles
	instanceRoles, err := c.ListInstanceRoles(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list instance roles: %w", err)
	}

	result := make([]InstanceRoleSpaceConfig, 0, len(instanceRoles))
	for _, role := range instanceRoles {
		// Skip the everyone virtual role — it's not a candidate for cross-scope grants.
		if role.Name == RoleEveryone {
			continue
		}

		grants, denials, err := c.GetInstanceRoleSpacePermissions(ctx, spaceID, role.Name)
		if err != nil {
			return nil, err
		}

		roleCopy := role // Copy to avoid pointer issues
		result = append(result, InstanceRoleSpaceConfig{
			Role:    &roleCopy,
			Grants:  grants,
			Denials: denials,
		})
	}

	return result, nil
}


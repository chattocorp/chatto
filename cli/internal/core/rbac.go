package core

// rbac.go is the unified RBAC surface on ChattoCore. It used to live in
// instance_rbac.go (server-tier) and space_rbac.go (space-tier); after Phase
// 5 of #330 there is only one tier — every role and grant lives in
// SERVER_RBAC — so the two files have been merged here.
//
// The remaining naming drift (Instance- vs Space-prefixed methods, spaceID
// parameters that the engine ignores) is preserved deliberately to keep the
// public API stable for resolvers, tests, and bootstrap. Each pair shares
// the same engine; the only difference is the auth gate the wrapper applies
// before delegating.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/core/rbac"
)

// SystemActorID is used for internal/bootstrap operations that bypass permission checks.
// SECURITY: This value cannot be forged by external users because user IDs are always
// generated with a "U" prefix (via NewUserID), e.g., "U1234567890abcd". The string "system"
// can never match a valid user ID.
const SystemActorID = "system"

// Errors specific to role-assignment hierarchy enforcement.
var (
	// ErrCannotAssignHigherRole is returned when a user tries to assign a role equal to or higher than their own.
	ErrCannotAssignHigherRole = errors.New("cannot assign role equal to or higher than your own")
	// ErrCannotRevokeSelfAdmin is returned when an admin tries to remove their own admin role.
	ErrCannotRevokeSelfAdmin = errors.New("cannot revoke your own admin role")
	// ErrCannotRevokeHigherRole is returned when a user tries to revoke a role equal to or higher than their own.
	ErrCannotRevokeHigherRole = errors.New("cannot revoke role equal to or higher than your own")
	// ErrCannotManageHigherUser is returned when a user tries to modify roles for a user with equal or higher rank.
	ErrCannotManageHigherUser = errors.New("cannot modify roles for a user with equal or higher rank")
)

// RoleWithPermissions represents a role with its grants and denials, used by
// the GraphQL surface and admin tooling.
type RoleWithPermissions struct {
	Name              string
	DisplayName       string
	Description       string
	Permissions       []Permission // Permissions granted (allowed) by this role
	PermissionDenials []Permission // Permissions denied by this role
	IsSystem          bool
	Position          int32 // Lower = higher rank. Owner=0, Everyone=MAX
}

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
// Initialization
// ============================================================================

const rbacDefaultsSentinel = "defaults_initialized"

// initInstanceRBAC creates the system roles and grants default permissions.
// Owner, admin, and moderator roles are explicitly created in KV.
// The everyone role remains virtual (not stored in KV).
// Called during ChattoCore initialization. Idempotent - safe to call multiple times.
// Uses a sentinel key to detect whether defaults have been fully written,
// which correctly handles the case where a previous boot was interrupted
// after creating the bucket but before writing all defaults.
func (c *ChattoCore) initInstanceRBAC(ctx context.Context) error {
	engine := c.storage.serverRBACEngine

	if _, err := engine.CreateRoleWithPosition(ctx, RoleOwner, "Owner", "Full server control", rbac.PositionOwner); err != nil {
		if !errors.Is(err, rbac.ErrRoleAlreadyExists) {
			return fmt.Errorf("failed to create owner role: %w", err)
		}
	}
	if _, err := engine.CreateRoleWithPosition(ctx, RoleAdmin, "Admin", "Full administrative access to the server", rbac.PositionAdmin); err != nil {
		if !errors.Is(err, rbac.ErrRoleAlreadyExists) {
			return fmt.Errorf("failed to create admin role: %w", err)
		}
	}
	if _, err := engine.CreateRoleWithPosition(ctx, RoleModerator, "Moderator", "View access to admin panels without management permissions", rbac.PositionModerator); err != nil {
		if !errors.Is(err, rbac.ErrRoleAlreadyExists) {
			return fmt.Errorf("failed to create moderator role: %w", err)
		}
	}

	_, err := c.storage.serverRBACKV.Get(ctx, rbacDefaultsSentinel)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		if err := c.InitInstanceDefaults(ctx); err != nil {
			return fmt.Errorf("failed to initialize unified instance defaults: %w", err)
		}
		if _, err := c.storage.serverRBACKV.Put(ctx, rbacDefaultsSentinel, []byte("1")); err != nil {
			return fmt.Errorf("failed to write RBAC sentinel key: %w", err)
		}
		c.logger.Info("Initialized instance RBAC with default permissions")
	} else if err != nil {
		return fmt.Errorf("failed to check RBAC sentinel key: %w", err)
	} else {
		c.logger.Info("Instance RBAC already configured, skipping default initialization")
	}

	return nil
}

// CreateDefaultRoles creates the default roles and permissions for a space.
// Owner, admin, and moderator are explicitly created in KV.
// Everyone role is virtual (not stored in KV).
// This should be called when a space is created.
func (c *ChattoCore) CreateDefaultRoles(ctx context.Context, spaceID string) error {
	engine := c.storage.serverRBACEngine

	if _, err := engine.CreateRoleWithPosition(ctx, RoleOwner, "Owner", "Full space control", rbac.PositionOwner); err != nil {
		if !errors.Is(err, rbac.ErrRoleAlreadyExists) {
			return fmt.Errorf("failed to create owner role: %w", err)
		}
	}
	if _, err := engine.CreateRoleWithPosition(ctx, RoleAdmin, "Admin", "Can manage space settings, roles, and members", rbac.PositionAdmin); err != nil {
		if !errors.Is(err, rbac.ErrRoleAlreadyExists) {
			return fmt.Errorf("failed to create admin role: %w", err)
		}
	}
	if _, err := engine.CreateRoleWithPosition(ctx, RoleModerator, "Moderator", "Can manage rooms and remove members", rbac.PositionModerator); err != nil {
		if !errors.Is(err, rbac.ErrRoleAlreadyExists) {
			return fmt.Errorf("failed to create moderator role: %w", err)
		}
	}

	if err := c.InitSpaceDefaults(ctx, spaceID); err != nil {
		return fmt.Errorf("failed to initialize unified space defaults: %w", err)
	}

	c.logger.Info("Created default roles", "space_id", spaceID)
	return nil
}

// ============================================================================
// Permission Checking
// ============================================================================

// getRolesWithPositions returns the user's roles (including implicit
// "everyone") sorted by hierarchy position (lower = higher rank = checked first).
func (c *ChattoCore) getRolesWithPositions(ctx context.Context, userID string) ([]roleWithPosition, error) {
	roles, err := c.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user instance roles: %w", err)
	}

	if !slices.Contains(roles, RoleEveryone) {
		roles = append(roles, RoleEveryone)
	}

	engine := c.storage.serverRBACEngine
	result := make([]roleWithPosition, 0, len(roles))
	for _, name := range roles {
		pos := rbac.PositionEveryone
		if role, err := engine.GetRole(ctx, name); err == nil && role != nil {
			pos = role.Position
		}
		result = append(result, roleWithPosition{name: name, position: pos})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].position < result[j].position
	})

	return result, nil
}

// HasInstancePermission checks if a user has a specific instance permission.
// This delegates to the unified PermissionResolver which implements hierarchical resolution.
//
// Note: Config-based admin check (owners.emails) should be done separately
// by the caller before calling this function.
func (c *ChattoCore) HasInstancePermission(ctx context.Context, userID string, perm Permission) (bool, error) {
	return c.permissionResolver.HasInstancePermission(ctx, userID, perm)
}

// IsInstanceAdmin checks if a user has the instance admin role via RBAC.
// Does NOT check config fallback (owners.emails) - caller should check that separately.
func (c *ChattoCore) IsInstanceAdmin(ctx context.Context, userID string) (bool, error) {
	return c.storage.serverRBACEngine.HasRole(ctx, userID, RoleAdmin)
}

// IsInstanceOwner checks if a user has the instance owner role via RBAC.
func (c *ChattoCore) IsInstanceOwner(ctx context.Context, userID string) (bool, error) {
	return c.storage.serverRBACEngine.HasRole(ctx, userID, RoleOwner)
}

// HasUserPermissionViaRoles checks if a user would have a permission through roles only
// (ignoring any user-specific grants/denials). Used for UI to show baseline state.
// Uses hierarchy-wins: roles are checked in rank order (lower position first),
// and the first explicit grant or deny found wins. This matches the actual
// authorization logic in walkInstancePermission.
func (c *ChattoCore) HasUserPermissionViaRoles(ctx context.Context, userID string, perm Permission) (bool, error) {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return false, nil
	}

	rolesWithPos, err := c.getRolesWithPositions(ctx, userID)
	if err != nil {
		return false, err
	}

	kv := c.storage.serverRBACEngine.KV()

	for _, rp := range rolesWithPos {
		if _, err := kv.Get(ctx, rbac.AllowKey(rp.name, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)); err == nil {
			return true, nil
		}
		if _, err := kv.Get(ctx, rbac.DenyKey(rp.name, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)); err == nil {
			return false, nil
		}
	}

	return false, nil
}

// HasUserPermissionDeniedViaRoles checks if a user has a permission denied through their roles
// (ignoring any user-specific grants/denials). Used for UI to show when a permission is blocked via roles.
// Uses hierarchy-wins: returns true only if the first explicit decision found (walking roles
// in rank order) is a deny. A higher-ranked role's grant beats a lower-ranked role's deny.
func (c *ChattoCore) HasUserPermissionDeniedViaRoles(ctx context.Context, userID string, perm Permission) (bool, error) {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return false, nil
	}

	rolesWithPos, err := c.getRolesWithPositions(ctx, userID)
	if err != nil {
		return false, err
	}

	kv := c.storage.serverRBACEngine.KV()

	for _, rp := range rolesWithPos {
		if _, err := kv.Get(ctx, rbac.AllowKey(rp.name, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)); err == nil {
			return false, nil
		}
		if _, err := kv.Get(ctx, rbac.DenyKey(rp.name, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)); err == nil {
			return true, nil
		}
	}

	return false, nil
}

// hasSpacePermission checks if a user has a specific permission in a space.
// This delegates to the unified PermissionResolver which implements hierarchical resolution:
// instance < space < room (more specific scopes override less specific ones).
//
// This is an internal building block. Use the Can* functions in can.go for
// authorization checks, as they may include additional business logic.
func (c *ChattoCore) hasSpacePermission(ctx context.Context, spaceID, userID string, perm Permission) (bool, error) {
	return c.permissionResolver.HasSpacePermission(ctx, userID, spaceID, perm)
}

// hasRoomPermission checks if a user has a permission at the room level.
// Uses the deny-always-wins, instance-authority-first resolution model with room overrides.
func (c *ChattoCore) hasRoomPermission(ctx context.Context, spaceID, roomID, userID string, perm Permission) (bool, error) {
	return c.permissionResolver.HasRoomPermission(ctx, userID, spaceID, roomID, perm)
}

// HasSpaceUserPermissionViaRoles is a thin wrapper around HasUserPermissionViaRoles
// that special-cases the DM system space (its permissions are static). Both
// scopes share the unified SERVER_RBAC store; the hierarchy-wins resolution
// from HasUserPermissionViaRoles matches what the production PermissionResolver
// actually enforces (ADR-005). The deny-override implementation that lived
// here previously caused the matrix UI to disagree with the enforced state.
func (c *ChattoCore) HasSpaceUserPermissionViaRoles(ctx context.Context, spaceID, userID string, perm Permission) (bool, error) {
	if IsDMSpace(spaceID) {
		return isDMPermissionAllowed(perm), nil
	}
	return c.HasUserPermissionViaRoles(ctx, userID, perm)
}

// HasSpaceUserPermissionDeniedViaRoles is a thin wrapper around
// HasUserPermissionDeniedViaRoles that returns false for DM-space permissions
// (they're never role-denied). See HasSpaceUserPermissionViaRoles for why
// the dedicated implementation is gone.
func (c *ChattoCore) HasSpaceUserPermissionDeniedViaRoles(ctx context.Context, spaceID, userID string, perm Permission) (bool, error) {
	if IsDMSpace(spaceID) {
		return false, nil
	}
	return c.HasUserPermissionDeniedViaRoles(ctx, userID, perm)
}

// ============================================================================
// Server-tier Role Assignment
// ============================================================================

// AssignInstanceOwnerRole assigns the owner role to a user.
func (c *ChattoCore) AssignInstanceOwnerRole(ctx context.Context, userID string) error {
	if err := c.storage.serverRBACEngine.AssignRole(ctx, userID, RoleOwner); err != nil {
		return fmt.Errorf("failed to assign owner role: %w", err)
	}
	c.logger.Info("Assigned instance owner role", "user_id", userID)
	return nil
}

// AssignInstanceAdminRole assigns the admin role to a user.
func (c *ChattoCore) AssignInstanceAdminRole(ctx context.Context, userID string) error {
	if err := c.storage.serverRBACEngine.AssignRole(ctx, userID, RoleAdmin); err != nil {
		return fmt.Errorf("failed to assign admin role: %w", err)
	}
	c.logger.Info("Assigned instance admin role", "user_id", userID)
	return nil
}

// RevokeInstanceAdminRole removes the admin role from a user.
func (c *ChattoCore) RevokeInstanceAdminRole(ctx context.Context, userID string) error {
	if err := c.storage.serverRBACEngine.RevokeRole(ctx, userID, RoleAdmin); err != nil {
		return fmt.Errorf("failed to revoke admin role: %w", err)
	}
	c.logger.Info("Revoked instance admin role", "user_id", userID)
	return nil
}

// ListInstanceAdmins returns all user IDs with the admin role assigned via RBAC.
// Does NOT include config-based admins (owners.emails).
func (c *ChattoCore) ListInstanceAdmins(ctx context.Context) ([]string, error) {
	return c.storage.serverRBACEngine.GetRoleUsers(ctx, RoleAdmin)
}

// AssignServerRole assigns any instance role to a user.
// The role must exist (system or custom). The everyone role cannot be assigned (it's implicit).
// Pass SystemActorID as actorID to bypass hierarchy check (for internal/bootstrap use).
// Hierarchy check: actor must outrank the role being assigned (actor's position < role's position).
func (c *ChattoCore) AssignServerRole(ctx context.Context, actorID, userID, roleName string) error {
	if roleName == RoleEveryone {
		return ErrImplicitRole
	}

	engine := c.storage.serverRBACEngine

	if actorID != SystemActorID {
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

	c.logger.Info("Assigned instance role", "role", roleName, "user_id", userID, "actor_id", actorID)
	return nil
}

// RevokeServerRole removes an instance role from a user.
// The role must exist (system or custom). The everyone role cannot be revoked (it's implicit).
// Pass SystemActorID as actorID to bypass hierarchy and self-demote checks (for internal/bootstrap use).
//
// Checks (in order):
//   - Owners cannot revoke their own owner role (lockout prevention).
//   - Actor must outrank the role being revoked (role-position hierarchy).
//   - Actor must outrank the target user (user-position hierarchy) — peers cannot demote each other.
func (c *ChattoCore) RevokeServerRole(ctx context.Context, actorID, userID, roleName string) error {
	if roleName == RoleEveryone {
		return ErrImplicitRole
	}

	engine := c.storage.serverRBACEngine

	if actorID != SystemActorID {
		if roleName == RoleOwner && actorID == userID {
			return ErrCannotRevokeSelfAdmin
		}

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
			return ErrCannotRevokeHigherRole
		}

		if actorID != userID {
			actorPos, err := engine.GetUserHighestPosition(ctx, actorID)
			if err != nil {
				return err
			}
			targetPos, err := engine.GetUserHighestPosition(ctx, userID)
			if err != nil {
				return err
			}
			if actorPos >= targetPos {
				return ErrCannotManageHigherUser
			}
		}
	}

	if err := engine.RevokeRole(ctx, userID, roleName); err != nil {
		if errors.Is(err, rbac.ErrRoleNotFound) {
			return ErrRoleNotFound
		}
		return err
	}

	c.logger.Info("Revoked instance role", "role", roleName, "user_id", userID, "actor_id", actorID)
	return nil
}

// GetRoleUsers returns all user IDs explicitly assigned to a role.
// The implicit `everyone` role returns []; all authenticated users carry it.
func (c *ChattoCore) GetRoleUsers(ctx context.Context, roleName string) ([]string, error) {
	if roleName == RoleEveryone {
		return []string{}, nil
	}

	users, err := c.storage.serverRBACEngine.GetRoleUsers(ctx, roleName)
	if err != nil {
		if errors.Is(err, rbac.ErrRoleNotFound) {
			return nil, ErrRoleNotFound
		}
		return nil, err
	}
	return users, nil
}

// GetUserRoles returns the explicit role assignments for a user. The implicit
// `everyone` role is omitted — callers that need it can prepend it themselves
// based on the relevant scope (e.g. space membership).
func (c *ChattoCore) GetUserRoles(ctx context.Context, userID string) ([]string, error) {
	assignedRoles, err := c.storage.serverRBACEngine.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user roles: %w", err)
	}

	result := make([]string, 0, len(assignedRoles))
	for _, role := range assignedRoles {
		if role != RoleEveryone {
			result = append(result, role)
		}
	}

	return result, nil
}

// ============================================================================
// Server-tier Permission Operations
// ============================================================================

// RevokeInstancePermission removes a permission grant from a role.
// This only removes grants, not denials. Use ClearInstancePermissionState
// to remove both. Idempotent — revoking a non-granted permission is a no-op.
//
// (GrantInstancePermission, DenyInstancePermission, and
// ClearInstancePermissionState live in permission_ops.go, alongside the
// space-tier and room-tier counterparts.)
func (c *ChattoCore) RevokeInstancePermission(ctx context.Context, roleName string, perm Permission) error {
	parts := perm.KeyParts()
	if err := c.storage.serverRBACEngine.RevokeRolePermission(ctx, roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny); err != nil {
		return err
	}
	c.logger.Info("Revoked instance permission", "role", roleName, "permission", perm)
	return nil
}

// GetServerRolePermissions returns all permissions granted to an instance role.
// Note: Admin roles are NOT special-cased - permissions are explicitly stored in KV.
func (c *ChattoCore) GetServerRolePermissions(ctx context.Context, roleName string) ([]Permission, error) {
	perms, err := c.storage.serverRBACEngine.GetRolePermissions(ctx, roleName)
	if err != nil {
		return nil, err
	}

	var result []Permission
	for _, p := range perms {
		perm := ReconstructPermission(p.Verb, p.ObjectType)
		if perm != "" {
			result = append(result, perm)
		}
	}
	return result, nil
}

// GetServerRolePermissionDenials returns all permissions denied by an instance role.
// Note: Admin roles are NOT special-cased - they can have denials like any other role.
func (c *ChattoCore) GetServerRolePermissionDenials(ctx context.Context, roleName string) ([]Permission, error) {
	perms, err := c.storage.serverRBACEngine.GetRolePermissionDenials(ctx, roleName)
	if err != nil {
		return nil, err
	}

	var result []Permission
	for _, p := range perms {
		perm := ReconstructPermission(p.Verb, p.ObjectType)
		if perm != "" {
			result = append(result, perm)
		}
	}
	return result, nil
}

// AllInstancePermissions returns all defined instance permissions.
// Exposed as a method for consistency with other core APIs.
func (c *ChattoCore) AllInstancePermissions() []Permission {
	perms := PermissionsForScope(ScopeServer)
	result := make([]Permission, len(perms))
	for i, p := range perms {
		result[i] = p.Permission
	}
	return result
}

// GetUserInstancePermissions returns all instance permissions the user has.
// Uses hierarchy-wins for each permission: roles are checked in rank order,
// and the first explicit grant or deny found determines the result.
// This matches the actual authorization logic in walkInstancePermission.
func (c *ChattoCore) GetUserInstancePermissions(ctx context.Context, userID string) ([]Permission, error) {
	var result []Permission
	for _, meta := range PermissionsForScope(ScopeServer) {
		has, err := c.HasUserPermissionViaRoles(ctx, userID, meta.Permission)
		if err != nil {
			return nil, fmt.Errorf("failed to check permission %s: %w", meta.Permission, err)
		}
		if has {
			result = append(result, meta.Permission)
		}
	}
	return result, nil
}

// ============================================================================
// Server-tier Role CRUD
// ============================================================================

// ListServerRoles returns all instance roles with their permissions.
// Note: Admin roles are NOT special-cased - permissions are read from KV like any other role.
func (c *ChattoCore) ListServerRoles(ctx context.Context) ([]RoleWithPermissions, error) {
	roles, err := c.storage.serverRBACEngine.ListRoles(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]RoleWithPermissions, 0, len(roles))
	for _, role := range roles {
		perms, _ := c.GetServerRolePermissions(ctx, role.Name)
		denials, _ := c.GetServerRolePermissionDenials(ctx, role.Name)

		result = append(result, RoleWithPermissions{
			Name:              role.Name,
			DisplayName:       role.DisplayName,
			Description:       role.Description,
			Permissions:       perms,
			PermissionDenials: denials,
			IsSystem:          IsSystemRole(role.Name),
			Position:          role.Position,
		})
	}

	return result, nil
}

// CreateServerRole creates a new custom server role.
// Role names must be lowercase letters only (e.g., "editor", "moderator").
// System role names (owner, admin, moderator, everyone) are reserved.
func (c *ChattoCore) CreateServerRole(ctx context.Context, name, displayName, description string) (*RoleWithPermissions, error) {
	if err := rbac.ValidateRoleName(name); err != nil {
		return nil, ErrInvalidRoleName
	}

	if IsSystemRole(name) {
		return nil, ErrRoleAlreadyExists
	}

	role, err := c.storage.serverRBACEngine.CreateRole(ctx, name, displayName, description)
	if err != nil {
		if errors.Is(err, rbac.ErrRoleAlreadyExists) {
			return nil, ErrRoleAlreadyExists
		}
		if errors.Is(err, rbac.ErrInvalidRoleName) {
			return nil, ErrInvalidRoleName
		}
		return nil, err
	}

	c.logger.Info("Created instance role", "name", name, "display_name", displayName, "position", role.Position)

	return &RoleWithPermissions{
		Name:              role.Name,
		DisplayName:       role.DisplayName,
		Description:       role.Description,
		Permissions:       []Permission{},
		PermissionDenials: []Permission{},
		IsSystem:          false,
		Position:          role.Position,
	}, nil
}

// UpdateServerRole updates an existing role's metadata.
// The role name cannot be changed.
func (c *ChattoCore) UpdateServerRole(ctx context.Context, name, displayName, description string) (*RoleWithPermissions, error) {
	role, err := c.storage.serverRBACEngine.UpdateRole(ctx, name, displayName, description)
	if err != nil {
		if errors.Is(err, rbac.ErrRoleNotFound) {
			return nil, ErrRoleNotFound
		}
		return nil, err
	}

	c.logger.Info("Updated instance role", "name", name, "display_name", displayName)

	perms, _ := c.GetServerRolePermissions(ctx, name)
	denials, _ := c.GetServerRolePermissionDenials(ctx, name)
	return &RoleWithPermissions{
		Name:              role.Name,
		DisplayName:       role.DisplayName,
		Description:       role.Description,
		Permissions:       perms,
		PermissionDenials: denials,
		IsSystem:          IsSystemRole(name),
		Position:          role.Position,
	}, nil
}

// GetServerRole returns a single instance role by name.
// Note: Admin roles are NOT special-cased - permissions are read from KV like any other role.
func (c *ChattoCore) GetServerRole(ctx context.Context, name string) (*RoleWithPermissions, error) {
	role, err := c.storage.serverRBACEngine.GetRole(ctx, name)
	if err != nil {
		if errors.Is(err, rbac.ErrRoleNotFound) {
			return nil, ErrRoleNotFound
		}
		return nil, err
	}

	perms, _ := c.GetServerRolePermissions(ctx, name)
	denials, _ := c.GetServerRolePermissionDenials(ctx, name)

	return &RoleWithPermissions{
		Name:              role.Name,
		DisplayName:       role.DisplayName,
		Description:       role.Description,
		Permissions:       perms,
		PermissionDenials: denials,
		IsSystem:          IsSystemRole(name),
		Position:          role.Position,
	}, nil
}

// DeleteServerRole deletes a custom role and all its associated data.
// This includes: the role definition, all permission grants, and all user assignments.
// System roles (owner, admin, moderator, everyone) cannot be deleted.
func (c *ChattoCore) DeleteServerRole(ctx context.Context, name string) error {
	if IsSystemRole(name) {
		return ErrCannotDeleteSystemRole
	}

	if err := c.storage.serverRBACEngine.DeleteRole(ctx, name); err != nil {
		if errors.Is(err, rbac.ErrRoleNotFound) {
			return ErrRoleNotFound
		}
		if errors.Is(err, rbac.ErrCannotDeleteSystemRole) {
			return ErrCannotDeleteSystemRole
		}
		return err
	}

	c.logger.Info("Deleted instance role", "role", name)
	return nil
}

// ReorderServerRoles reorders custom instance roles.
// System roles (owner, admin, moderator, everyone) maintain fixed positions and should not be included.
// Positions are assigned based on array index (first role = position 3, second = 4, etc).
// Note: Position 0 = owner, 1 = admin, 2 = moderator, position MAX = everyone.
// Returns all roles sorted by position.
func (c *ChattoCore) ReorderServerRoles(ctx context.Context, roleNames []string) ([]RoleWithPermissions, error) {
	for _, name := range roleNames {
		if IsSystemRole(name) {
			return nil, fmt.Errorf("cannot reorder system role: %s", name)
		}
	}

	roles, err := c.storage.serverRBACEngine.ReorderRoles(ctx, roleNames)
	if err != nil {
		return nil, err
	}

	result := make([]RoleWithPermissions, 0, len(roles))
	for _, role := range roles {
		perms, _ := c.GetServerRolePermissions(ctx, role.Name)
		denials, _ := c.GetServerRolePermissionDenials(ctx, role.Name)

		result = append(result, RoleWithPermissions{
			Name:              role.Name,
			DisplayName:       role.DisplayName,
			Description:       role.Description,
			Permissions:       perms,
			PermissionDenials: denials,
			IsSystem:          IsSystemRole(role.Name),
			Position:          role.Position,
		})
	}

	c.logger.Info("Reordered instance roles", "order", roleNames)
	return result, nil
}

// GetRoomRolePermissions returns the room-level grants and denials for a role in a specific room.
func (c *ChattoCore) GetRoomRolePermissions(ctx context.Context, roomID, roleName string) (grants []Permission, denials []Permission, err error) {
	kv := c.storage.serverRBACKV

	allowKeys, err := listKeysWithPattern(ctx, kv, rbac.AllowPatternForSubject(roleName))
	if err != nil {
		return nil, nil, err
	}
	for _, key := range allowKeys {
		parts := rbac.ParseAllowKey(key)
		if parts.ObjectId == roomID {
			perm := ReconstructPermission(parts.Verb, parts.ObjectType)
			if perm != "" {
				grants = append(grants, perm)
			}
		}
	}

	denyKeys, err := listKeysWithPattern(ctx, kv, rbac.DenyPatternForSubject(roleName))
	if err != nil {
		return nil, nil, err
	}
	for _, key := range denyKeys {
		parts := rbac.ParseDenyKey(key)
		if parts.ObjectId == roomID {
			perm := ReconstructPermission(parts.Verb, parts.ObjectType)
			if perm != "" {
				denials = append(denials, perm)
			}
		}
	}

	return grants, denials, nil
}

// CanManageUser checks whether actor outranks target by role-hierarchy
// position. Returns true if actor's highest position < target's highest
// position (lower position = higher rank). Used for kick/mute/role-assignment
// gates. Callers must verify space membership separately if relevant; users
// with no explicit roles fall back to PositionEveryone.
func (c *ChattoCore) CanManageUser(ctx context.Context, actorID, targetID string) (bool, error) {
	engine := c.storage.serverRBACEngine

	actorPos, err := engine.GetUserHighestPosition(ctx, actorID)
	if err != nil {
		return false, err
	}
	targetPos, err := engine.GetUserHighestPosition(ctx, targetID)
	if err != nil {
		return false, err
	}

	return actorPos < targetPos, nil
}

// GetUserEffectiveSpacePermissions returns all permissions the user effectively has in a space.
// Delegates to PermissionResolver.HasSpacePermission for each space-scoped permission,
// ensuring consistent resolution logic (deny-always-wins, instance-authority-first).
func (c *ChattoCore) GetUserEffectiveSpacePermissions(ctx context.Context, spaceID, userID string) ([]Permission, error) {
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
		perm := permMeta.Permission
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

// RevokeAllUserRoles removes every role assignment for a user. Post-#330
// roles are server-wide, so this clears the user from every role they hold.
// Used during LeaveSpace cleanup and account deletion.
// Authorization: Internal use only (no permission check needed).
func (c *ChattoCore) RevokeAllUserRoles(ctx context.Context, userID string) error {
	if err := c.storage.serverRBACEngine.RevokeAllUserRoles(ctx, userID); err != nil {
		return fmt.Errorf("failed to revoke user roles: %w", err)
	}

	c.logger.Debug("Revoked all roles for user", "user_id", userID)
	return nil
}

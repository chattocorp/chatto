package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/core/rbac"
)

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
	engine := c.instanceRBACEngine

	// Create owner role (position 0) - explicitly stored in KV
	if _, err := engine.CreateRoleWithPosition(ctx, InstRoleOwner, "Instance Owner", "Full instance control", rbac.PositionOwner); err != nil {
		// Ignore if already exists (idempotent)
		if !errors.Is(err, rbac.ErrRoleAlreadyExists) {
			return fmt.Errorf("failed to create instance-owner role: %w", err)
		}
	}

	// Create admin role (position 1) - explicitly stored in KV
	if _, err := engine.CreateRoleWithPosition(ctx, InstRoleAdmin, "Instance Admin", "Full access to all instance-level features", rbac.PositionAdmin); err != nil {
		if !errors.Is(err, rbac.ErrRoleAlreadyExists) {
			return fmt.Errorf("failed to create instance-admin role: %w", err)
		}
	}

	// Create moderator role (position 2) - explicitly stored in KV
	if _, err := engine.CreateRoleWithPosition(ctx, InstRoleModerator, "Instance Moderator", "View access to admin panels without management permissions", rbac.PositionModerator); err != nil {
		if !errors.Is(err, rbac.ErrRoleAlreadyExists) {
			return fmt.Errorf("failed to create instance-moderator role: %w", err)
		}
	}

	// Check sentinel key to determine if defaults have been fully written.
	// This is safer than checking "is the bucket new" because a previous boot
	// may have created the bucket but crashed before writing all defaults.
	_, err := c.storage.instanceRBACKV.Get(ctx, rbacDefaultsSentinel)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		if err := c.InitInstanceDefaults(ctx); err != nil {
			return fmt.Errorf("failed to initialize unified instance defaults: %w", err)
		}
		if _, err := c.storage.instanceRBACKV.Put(ctx, rbacDefaultsSentinel, []byte("1")); err != nil {
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

// ============================================================================
// Permission Checking
// ============================================================================

// instanceRoleWithPosition pairs a role name with its hierarchy position.
type instanceRoleWithPosition struct {
	name     string
	position int32
}

// getInstanceRolesWithPositions returns the user's instance roles (including implicit
// "everyone") sorted by hierarchy position (lower = higher rank = checked first).
func (c *ChattoCore) getInstanceRolesWithPositions(ctx context.Context, userID string) ([]instanceRoleWithPosition, error) {
	roles, err := c.GetUserInstanceRoles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user instance roles: %w", err)
	}

	// Always include "everyone" for authenticated users
	if !slices.Contains(roles, InstRoleEveryone) {
		roles = append(roles, InstRoleEveryone)
	}

	engine := c.instanceRBACEngine
	result := make([]instanceRoleWithPosition, 0, len(roles))
	for _, name := range roles {
		pos := rbac.PositionEveryone
		if role, err := engine.GetRole(ctx, name); err == nil && role != nil {
			pos = role.Position
		}
		result = append(result, instanceRoleWithPosition{name: name, position: pos})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].position < result[j].position
	})

	return result, nil
}

// HasInstancePermission checks if a user has a specific instance permission.
// This delegates to the unified PermissionResolver which implements hierarchical resolution.
//
// Note: Config-based admin check (admin.emails) should be done separately
// by the caller before calling this function.
func (c *ChattoCore) HasInstancePermission(ctx context.Context, userID string, perm Permission) (bool, error) {
	// Delegate to the unified PermissionResolver
	return c.permissionResolver.HasInstancePermission(ctx, userID, Permission(perm))
}

// IsInstanceAdmin checks if a user has the instance admin role via RBAC.
// Does NOT check config fallback (admin.emails) - caller should check that separately.
func (c *ChattoCore) IsInstanceAdmin(ctx context.Context, userID string) (bool, error) {
	return c.instanceRBACEngine.HasRole(ctx, userID, InstRoleAdmin)
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

	rolesWithPos, err := c.getInstanceRolesWithPositions(ctx, userID)
	if err != nil {
		return false, err
	}

	kv := c.instanceRBACEngine.KV()

	// Check each role in hierarchy order - first explicit permission wins
	for _, rp := range rolesWithPos {
		// Check for grant
		_, err := kv.Get(ctx, rbac.AllowKey(rp.name, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
		if err == nil {
			return true, nil
		}

		// Check for deny
		_, err = kv.Get(ctx, rbac.DenyKey(rp.name, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
		if err == nil {
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

	rolesWithPos, err := c.getInstanceRolesWithPositions(ctx, userID)
	if err != nil {
		return false, err
	}

	kv := c.instanceRBACEngine.KV()

	// Check each role in hierarchy order - first explicit permission wins
	for _, rp := range rolesWithPos {
		// Check for grant — if found first, the permission is NOT denied
		_, err := kv.Get(ctx, rbac.AllowKey(rp.name, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
		if err == nil {
			return false, nil
		}

		// Check for deny — if found first, the permission IS denied
		_, err = kv.Get(ctx, rbac.DenyKey(rp.name, parts.Verb, parts.ObjectType, rbac.ObjectIdAny))
		if err == nil {
			return true, nil
		}
	}

	return false, nil
}

// ============================================================================
// Role Assignment Operations
// ============================================================================

// AssignInstanceOwnerRole assigns the owner role to a user.
func (c *ChattoCore) AssignInstanceOwnerRole(ctx context.Context, userID string) error {
	if err := c.instanceRBACEngine.AssignRole(ctx, userID, InstRoleOwner); err != nil {
		return fmt.Errorf("failed to assign owner role: %w", err)
	}
	c.logger.Info("Assigned instance owner role", "user_id", userID)
	return nil
}

// AssignInstanceAdminRole assigns the admin role to a user.
func (c *ChattoCore) AssignInstanceAdminRole(ctx context.Context, userID string) error {
	if err := c.instanceRBACEngine.AssignRole(ctx, userID, InstRoleAdmin); err != nil {
		return fmt.Errorf("failed to assign admin role: %w", err)
	}
	c.logger.Info("Assigned instance admin role", "user_id", userID)
	return nil
}

// IsInstanceOwner checks if a user has the instance owner role via RBAC.
func (c *ChattoCore) IsInstanceOwner(ctx context.Context, userID string) (bool, error) {
	return c.instanceRBACEngine.HasRole(ctx, userID, InstRoleOwner)
}

// firstOwnerMarkerKey is the KV key used to atomically track whether the first owner has been assigned.
const firstOwnerMarkerKey = "first_owner_assigned"

// IsInstanceFresh checks if the instance has been bootstrapped.
// Returns true if no owner has been assigned yet (fresh instance).
func (c *ChattoCore) IsInstanceFresh(ctx context.Context) (bool, error) {
	kv := c.storage.instanceRBACKV
	_, err := kv.Get(ctx, firstOwnerMarkerKey)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return true, nil // No marker = fresh instance
		}
		return false, fmt.Errorf("failed to check instance state: %w", err)
	}
	return false, nil // Marker exists = already bootstrapped
}

// PromoteFirstUserToOwner atomically promotes a user to owner if they are the first user.
// Uses kv.Create() to ensure only one user can be promoted even under concurrent registrations.
// Returns true if the user was promoted, false if another user was already promoted.
func (c *ChattoCore) PromoteFirstUserToOwner(ctx context.Context, userID string) (bool, error) {
	kv := c.storage.instanceRBACKV

	// Atomically try to create the marker - only one caller can succeed
	_, err := kv.Create(ctx, firstOwnerMarkerKey, []byte(userID))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			// Another user was already promoted
			return false, nil
		}
		return false, fmt.Errorf("failed to claim first owner: %w", err)
	}

	// We won the race - now assign the owner role
	if err := c.AssignInstanceOwnerRole(ctx, userID); err != nil {
		// Best effort: try to delete the marker so another user can try
		// This is unlikely to fail, but if it does, manual intervention is needed
		_ = kv.Delete(ctx, firstOwnerMarkerKey)
		return false, fmt.Errorf("failed to assign owner role: %w", err)
	}

	c.logger.Info("First user promoted to instance owner", "user_id", userID)
	return true, nil
}

// RevokeInstanceAdminRole removes the admin role from a user.
func (c *ChattoCore) RevokeInstanceAdminRole(ctx context.Context, userID string) error {
	if err := c.instanceRBACEngine.RevokeRole(ctx, userID, InstRoleAdmin); err != nil {
		return fmt.Errorf("failed to revoke admin role: %w", err)
	}
	c.logger.Info("Revoked instance admin role", "user_id", userID)
	return nil
}

// ListInstanceAdmins returns all user IDs with the admin role assigned via RBAC.
// Does NOT include config-based admins (admin.emails).
func (c *ChattoCore) ListInstanceAdmins(ctx context.Context) ([]string, error) {
	return c.instanceRBACEngine.GetRoleUsers(ctx, InstRoleAdmin)
}

// AssignInstanceRole assigns any instance role to a user.
// The role must exist (system or custom). The everyone role cannot be assigned (it's implicit).
// Pass SystemActorID as actorID to bypass hierarchy check (for internal/bootstrap use).
// Hierarchy check: actor must outrank the role being assigned (actor's position < role's position).
func (c *ChattoCore) AssignInstanceRole(ctx context.Context, actorID, userID, roleName string) error {
	// Everyone role is implicit - cannot be explicitly assigned
	if roleName == InstRoleEveryone {
		return ErrImplicitRole
	}

	engine := c.instanceRBACEngine

	// Hierarchy check (skip for system actor - internal/bootstrap use)
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

// RevokeInstanceRole removes an instance role from a user.
// The role must exist (system or custom). The everyone role cannot be revoked (it's implicit).
// Pass SystemActorID as actorID to bypass hierarchy check (for internal/bootstrap use).
// Hierarchy check: actor must outrank the role being revoked (actor's position < role's position).
func (c *ChattoCore) RevokeInstanceRole(ctx context.Context, actorID, userID, roleName string) error {
	// Everyone role is implicit - cannot be revoked
	if roleName == InstRoleEveryone {
		return ErrImplicitRole
	}

	engine := c.instanceRBACEngine

	// Hierarchy check (skip for system actor - internal/bootstrap use)
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
			return ErrCannotRevokeHigherRole
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

// ListInstanceRoleUsers returns all user IDs with a specific instance role.
// For the everyone role, returns an empty list (all users are implicit members).
func (c *ChattoCore) ListInstanceRoleUsers(ctx context.Context, roleName string) ([]string, error) {
	// Everyone role is implicit for all users - return empty list
	// (the frontend shows a special message for this case)
	if roleName == InstRoleEveryone {
		return []string{}, nil
	}

	users, err := c.instanceRBACEngine.GetRoleUsers(ctx, roleName)
	if err != nil {
		if errors.Is(err, rbac.ErrRoleNotFound) {
			return nil, ErrRoleNotFound
		}
		return nil, err
	}
	return users, nil
}

// GetUserInstanceRoles returns all instance roles assigned to a user.
// Note: "everyone" is not returned as it applies to all authenticated users implicitly.
func (c *ChattoCore) GetUserInstanceRoles(ctx context.Context, userID string) ([]string, error) {
	// Get explicitly assigned roles via engine
	assignedRoles, err := c.instanceRBACEngine.GetUserRoles(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user roles: %w", err)
	}

	// Filter out implicit roles (everyone is always implicit)
	result := make([]string, 0, len(assignedRoles))
	for _, role := range assignedRoles {
		if role != InstRoleEveryone {
			result = append(result, role)
		}
	}

	return result, nil
}

// ============================================================================
// Permission Management Operations
// ============================================================================

// GrantInstancePermission adds a permission to an instance role.
// Note: Admin role has all permissions implicitly - this is primarily for other roles.
func (c *ChattoCore) GrantInstancePermission(ctx context.Context, roleName string, perm Permission) error {
	parts := perm.KeyParts()
	if err := c.instanceRBACEngine.GrantRolePermission(ctx, roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny); err != nil {
		return err
	}
	c.logger.Info("Granted instance permission", "role", roleName, "permission", perm)
	return nil
}

// RevokeInstancePermission removes a permission grant from an instance role.
// Note: This only removes grants, not denials. Use ClearInstancePermissionState to remove both.
// Note: Admin role has all permissions implicitly - this is primarily for the everyone role.
func (c *ChattoCore) RevokeInstancePermission(ctx context.Context, roleName string, perm Permission) error {
	parts := perm.KeyParts()
	if err := c.instanceRBACEngine.RevokeRolePermission(ctx, roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny); err != nil {
		return err
	}
	c.logger.Info("Revoked instance permission", "role", roleName, "permission", perm)
	return nil
}

// DenyInstancePermission adds a permission denial to an instance role.
// Users with this role will be blocked from this permission regardless of what other roles grant it.
// Note: Admin role is immune to role denials.
func (c *ChattoCore) DenyInstancePermission(ctx context.Context, roleName string, perm Permission) error {
	parts := perm.KeyParts()
	if err := c.instanceRBACEngine.DenyRolePermission(ctx, roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny); err != nil {
		return err
	}
	c.logger.Info("Denied instance permission", "role", roleName, "permission", perm)
	return nil
}

// ClearInstancePermissionState removes both grant and denial for a permission on an instance role.
// This returns the permission to a neutral state.
func (c *ChattoCore) ClearInstancePermissionState(ctx context.Context, roleName string, perm Permission) error {
	parts := perm.KeyParts()
	if err := c.instanceRBACEngine.ClearRolePermissionState(ctx, roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny); err != nil {
		return err
	}
	c.logger.Info("Cleared instance permission state", "role", roleName, "permission", perm)
	return nil
}

// GetInstanceRolePermissions returns all permissions granted to an instance role.
// Note: Admin roles are NOT special-cased - permissions are explicitly stored in KV.
func (c *ChattoCore) GetInstanceRolePermissions(ctx context.Context, roleName string) ([]Permission, error) {
	perms, err := c.instanceRBACEngine.GetRolePermissions(ctx, roleName)
	if err != nil {
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

// GetInstanceRolePermissionDenials returns all permissions denied by an instance role.
// Note: Admin roles are NOT special-cased - they can have denials like any other role.
func (c *ChattoCore) GetInstanceRolePermissionDenials(ctx context.Context, roleName string) ([]Permission, error) {
	perms, err := c.instanceRBACEngine.GetRolePermissionDenials(ctx, roleName)
	if err != nil {
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

// RoleWithPermissions represents an instance role with its permissions.
type RoleWithPermissions struct {
	Name              string
	DisplayName       string
	Description       string
	Permissions       []Permission // Permissions granted (allowed) by this role
	PermissionDenials []Permission // Permissions denied by this role
	IsSystem          bool
	Position          int32 // Lower = higher rank. Owner=0, Everyone=MAX
}

// AllInstancePermissions returns all defined instance permissions.
// Exposed as a method for consistency with other core APIs.
func (c *ChattoCore) AllInstancePermissions() []Permission {
	perms := PermissionsForScope(ScopeInstance)
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
	for _, meta := range PermissionsForScope(ScopeInstance) {
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

// ListInstanceRoles returns all instance roles with their permissions.
// Note: Admin roles are NOT special-cased - permissions are read from KV like any other role.
func (c *ChattoCore) ListInstanceRoles(ctx context.Context) ([]RoleWithPermissions, error) {
	roles, err := c.instanceRBACEngine.ListRoles(ctx)
	if err != nil {
		return nil, err
	}

	result := make([]RoleWithPermissions, 0, len(roles))
	for _, role := range roles {
		perms, _ := c.GetInstanceRolePermissions(ctx, role.Name)
		denials, _ := c.GetInstanceRolePermissionDenials(ctx, role.Name)

		result = append(result, RoleWithPermissions{
			Name:              role.Name,
			DisplayName:       role.DisplayName,
			Description:       role.Description,
			Permissions:       perms,
			PermissionDenials: denials,
			IsSystem:          IsInstanceSystemRole(role.Name),
			Position:          role.Position,
		})
	}

	return result, nil
}

// ============================================================================
// Role CRUD Operations
// ============================================================================

// CreateInstanceRole creates a new custom instance role.
// Role names must start with "instance-" prefix (e.g., "instance-editor").
// System role names (instance-admin, everyone) are reserved.
func (c *ChattoCore) CreateInstanceRole(ctx context.Context, name, displayName, description string) (*RoleWithPermissions, error) {
	// Validate name has instance- prefix and correct format
	if err := rbac.ValidateInstanceRoleName(name); err != nil {
		return nil, ErrInvalidRoleName
	}

	// Validate name is not a system role
	if IsInstanceSystemRole(name) {
		return nil, ErrRoleAlreadyExists
	}

	role, err := c.instanceRBACEngine.CreateRole(ctx, name, displayName, description)
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

// UpdateInstanceRole updates an existing role's metadata.
// The role name cannot be changed.
func (c *ChattoCore) UpdateInstanceRole(ctx context.Context, name, displayName, description string) (*RoleWithPermissions, error) {
	role, err := c.instanceRBACEngine.UpdateRole(ctx, name, displayName, description)
	if err != nil {
		if errors.Is(err, rbac.ErrRoleNotFound) {
			return nil, ErrRoleNotFound
		}
		return nil, err
	}

	c.logger.Info("Updated instance role", "name", name, "display_name", displayName)

	perms, _ := c.GetInstanceRolePermissions(ctx, name)
	denials, _ := c.GetInstanceRolePermissionDenials(ctx, name)
	return &RoleWithPermissions{
		Name:              role.Name,
		DisplayName:       role.DisplayName,
		Description:       role.Description,
		Permissions:       perms,
		PermissionDenials: denials,
		IsSystem:          IsInstanceSystemRole(name),
		Position:          role.Position,
	}, nil
}

// GetInstanceRole returns a single instance role by name.
// Note: Admin roles are NOT special-cased - permissions are read from KV like any other role.
func (c *ChattoCore) GetInstanceRole(ctx context.Context, name string) (*RoleWithPermissions, error) {
	role, err := c.instanceRBACEngine.GetRole(ctx, name)
	if err != nil {
		if errors.Is(err, rbac.ErrRoleNotFound) {
			return nil, ErrRoleNotFound
		}
		return nil, err
	}

	perms, _ := c.GetInstanceRolePermissions(ctx, name)
	denials, _ := c.GetInstanceRolePermissionDenials(ctx, name)

	return &RoleWithPermissions{
		Name:              role.Name,
		DisplayName:       role.DisplayName,
		Description:       role.Description,
		Permissions:       perms,
		PermissionDenials: denials,
		IsSystem:          IsInstanceSystemRole(name),
		Position:          role.Position,
	}, nil
}

// DeleteInstanceRole deletes a custom role and all its associated data.
// This includes: the role definition, all permission grants, and all user assignments.
// System roles (owner, admin, moderator, everyone) cannot be deleted.
func (c *ChattoCore) DeleteInstanceRole(ctx context.Context, name string) error {
	if IsInstanceSystemRole(name) {
		return ErrCannotDeleteSystemRole
	}

	if err := c.instanceRBACEngine.DeleteRole(ctx, name); err != nil {
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

// ReorderInstanceRoles reorders custom instance roles.
// System roles (owner, admin, moderator, everyone) maintain fixed positions and should not be included.
// Positions are assigned based on array index (first role = position 3, second = 4, etc).
// Note: Position 0 = owner, 1 = admin, 2 = moderator, position MAX = everyone.
// Returns all roles sorted by position.
func (c *ChattoCore) ReorderInstanceRoles(ctx context.Context, roleNames []string) ([]RoleWithPermissions, error) {
	// Validate no system roles in the list
	for _, name := range roleNames {
		if IsInstanceSystemRole(name) {
			return nil, fmt.Errorf("cannot reorder system role: %s", name)
		}
	}

	// Reorder in engine (custom roles get positions 3+; system roles have fixed positions:
	// owner=0, admin=1, moderator=2, verified=MaxInt32-1, everyone=MaxInt32)
	roles, err := c.instanceRBACEngine.ReorderRoles(ctx, roleNames)
	if err != nil {
		return nil, err
	}

	// Convert to RoleWithPermissions
	result := make([]RoleWithPermissions, 0, len(roles))
	for _, role := range roles {
		perms, _ := c.GetInstanceRolePermissions(ctx, role.Name)
		denials, _ := c.GetInstanceRolePermissionDenials(ctx, role.Name)

		result = append(result, RoleWithPermissions{
			Name:              role.Name,
			DisplayName:       role.DisplayName,
			Description:       role.Description,
			Permissions:       perms,
			PermissionDenials: denials,
			IsSystem:          IsInstanceSystemRole(role.Name),
			Position:          role.Position,
		})
	}

	c.logger.Info("Reordered instance roles", "order", roleNames)
	return result, nil
}

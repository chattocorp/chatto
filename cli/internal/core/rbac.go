package core

// rbac.go is the unified RBAC surface on ChattoCore. It used to live in
// instance_rbac.go (server-tier) and space_rbac.go (space-tier); after Phase
// 5 of #330 there is only one tier. Role and permission state is now derived
// from the RBAC event-sourced aggregate.
//
// The remaining naming drift (Instance- vs Space-prefixed methods, spaceID
// parameters that the engine ignores) is preserved deliberately to keep the
// public API stable for resolvers, tests, and bootstrap.

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
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
	Position          int32 // Higher = higher rank. Everyone=0, Owner=1000
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

// initServerRBAC exists for older tests and tools that explicitly ask for the
// historical bootstrap step. NewChattoCore seeds the default RBAC aggregate
// directly on fresh servers.
func (c *ChattoCore) initServerRBAC(ctx context.Context) error {
	return c.CreateDefaultRoles(ctx)
}

// CreateDefaultRoles appends the system roles and their default permissions to
// the RBAC aggregate. It is idempotent at the projection level.
func (c *ChattoCore) CreateDefaultRoles(ctx context.Context) error {
	if _, err := c.appendRBACBatch(ctx, rbacSeedEntries(defaultRBACRoles(), nil, defaultRBACDecisions()), nil); err != nil {
		return fmt.Errorf("seed default RBAC roles: %w", err)
	}
	c.logger.Info("Created default roles")
	return nil
}

// ============================================================================
// Permission Checking
// ============================================================================

// HasServerPermission checks if a user has a specific server permission.
// This delegates to the unified PermissionResolver which implements hierarchical resolution.
//
// Note: Config-based admin check (owners.emails) should be done separately
// by the caller before calling this function.
func (c *ChattoCore) HasServerPermission(ctx context.Context, userID string, perm Permission) (bool, error) {
	return c.permissionResolver.HasServerPermission(ctx, userID, perm)
}

// IsServerAdmin checks if a user has the admin role via RBAC.
// Does NOT check config fallback (owners.emails) - caller should check that separately.
func (c *ChattoCore) IsServerAdmin(ctx context.Context, userID string) (bool, error) {
	return c.RBAC.HasRole(userID, RoleAdmin), nil
}

// IsServerOwner checks if a user has the owner role via RBAC.
func (c *ChattoCore) IsServerOwner(ctx context.Context, userID string) (bool, error) {
	return c.RBAC.HasRole(userID, RoleOwner), nil
}

// ResolveUserPermission returns the walker's decision (allow / deny / none)
// for a user-permission pair. Single source of truth for both the bool
// authorizer and the inspector. Pass roomID="" for server-scope, KindDM
// to activate the DM boundary deny-list.
func (c *ChattoCore) ResolveUserPermission(ctx context.Context, userID string, kind RoomKind, roomID string, perm Permission) (DecisionKind, error) {
	return c.permissionResolver.Resolve(ctx, userID, kind, roomID, perm)
}

// HasUserPermissionViaRoles is a bool wrapper around ResolveUserPermission
// for server-scope checks. Kept as a thin convenience for the inspector and
// tests; new code should call ResolveUserPermission directly when it cares
// about the deny-vs-none distinction.
func (c *ChattoCore) HasUserPermissionViaRoles(ctx context.Context, userID string, perm Permission) (bool, error) {
	decision, err := c.ResolveUserPermission(ctx, userID, KindChannel, "", perm)
	return decision == DecisionAllow, err
}

// HasUserPermissionDeniedViaRoles is the sibling to HasUserPermissionViaRoles
// that reports whether the resolver's first decision is an explicit deny.
// "No decision" returns false here — only an explicit deny does.
func (c *ChattoCore) HasUserPermissionDeniedViaRoles(ctx context.Context, userID string, perm Permission) (bool, error) {
	decision, err := c.ResolveUserPermission(ctx, userID, KindChannel, "", perm)
	return decision == DecisionDeny, err
}

// hasServerPermission checks a server-wide permission via the unified
// hierarchy-wins resolver. Internal building block — use the Can* helpers
// in can.go for authorization checks.
func (c *ChattoCore) hasServerPermission(ctx context.Context, userID string, perm Permission) (bool, error) {
	return c.permissionResolver.HasSpacePermission(ctx, userID, KindChannel, perm)
}

// hasKindPermission is the kind-sensitive variant of hasServerPermission.
// For KindDM the resolver applies the DM boundary deny-list first; for
// KindChannel it behaves like hasServerPermission.
func (c *ChattoCore) hasKindPermission(ctx context.Context, kind RoomKind, userID string, perm Permission) (bool, error) {
	return c.permissionResolver.HasSpacePermission(ctx, userID, kind, perm)
}

// hasRoomPermission checks a permission at the room level. Room-scoped
// grants/denials take precedence over server-scoped ones for the same
// role; across roles the hierarchy walk decides.
func (c *ChattoCore) hasRoomPermission(ctx context.Context, kind RoomKind, roomID, userID string, perm Permission) (bool, error) {
	return c.permissionResolver.HasRoomPermission(ctx, userID, kind, roomID, perm)
}

// hasGroupPermission checks a permission at the room-group level (no room
// context). Used for group-scoped capability gates like room.create. For
// permissions that are also ScopeServer, the resolver falls back to the
// server tier when no group decision exists.
func (c *ChattoCore) hasGroupPermission(ctx context.Context, kind RoomKind, groupID, userID string, perm Permission) (bool, error) {
	decision, err := c.permissionResolver.ResolveGroup(ctx, userID, kind, groupID, perm)
	if err != nil {
		return false, err
	}
	return decision == DecisionAllow, nil
}

// ============================================================================
// Server-tier Role Assignment
// ============================================================================

// AssignOwnerRole assigns the owner role to a user.
func (c *ChattoCore) AssignOwnerRole(ctx context.Context, userID string) error {
	return c.AssignServerRole(ctx, SystemActorID, userID, RoleOwner)
}

// AssignAdminRole assigns the admin role to a user.
func (c *ChattoCore) AssignAdminRole(ctx context.Context, userID string) error {
	return c.AssignServerRole(ctx, SystemActorID, userID, RoleAdmin)
}

// RevokeAdminRole removes the admin role from a user.
func (c *ChattoCore) RevokeAdminRole(ctx context.Context, userID string) error {
	return c.RevokeServerRole(ctx, SystemActorID, userID, RoleAdmin)
}

// ListAdmins returns all user IDs with the admin role assigned via RBAC.
// Does NOT include config-based admins (owners.emails).
func (c *ChattoCore) ListAdmins(ctx context.Context) ([]string, error) {
	return c.RBAC.GetRoleUsers(RoleAdmin), nil
}

// AssignServerRole assigns any role to a user.
// The role must exist (system or custom). The everyone role cannot be assigned (it's implicit).
// Pass SystemActorID as actorID to bypass hierarchy checks (for internal/bootstrap use).
//
// Hierarchy checks (mirroring RevokeServerRole for symmetry):
//   - Actor must outrank the role being assigned (role-position hierarchy).
//   - Actor must outrank the target user (user-position hierarchy) — peers
//     cannot decorate each other with new roles, only system bootstrap or
//     a strictly-higher-ranked admin can.
func (c *ChattoCore) AssignServerRole(ctx context.Context, actorID, userID, roleName string) error {
	if roleName == RoleEveryone {
		return ErrImplicitRole
	}

	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_RbacRoleAssigned{
		RbacRoleAssigned: &corev1.RbacRoleAssignedEvent{UserId: userID, RoleName: roleName},
	}})

	if _, err := c.appendRBACEvent(ctx, event, func() error {
		role, ok := c.RBAC.GetRole(roleName)
		if !ok {
			return ErrRoleNotFound
		}
		if actorID != SystemActorID {
			canManage := c.RBAC.GetUserHighestPosition(actorID) == PositionOwner || c.RBAC.GetUserHighestPosition(actorID) > role.GetPosition()
			if !canManage {
				return ErrCannotAssignHigherRole
			}
			if actorID != userID {
				if c.RBAC.GetUserHighestPosition(actorID) <= c.RBAC.GetUserHighestPosition(userID) {
					return ErrCannotManageHigherUser
				}
			}
		}
		if c.RBAC.HasRole(userID, roleName) {
			return errRBACNoop
		}
		return nil
	}); err != nil {
		if errors.Is(err, errRBACNoop) {
			return nil
		}
		return err
	}

	c.logger.Info("Assigned role", "role", roleName, "user_id", userID, "actor_id", actorID)
	return nil
}

// RevokeServerRole removes an role from a user.
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

	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_RbacRoleRevoked{
		RbacRoleRevoked: &corev1.RbacRoleRevokedEvent{UserId: userID, RoleName: roleName},
	}})

	if _, err := c.appendRBACEvent(ctx, event, func() error {
		if roleName == RoleOwner && actorID == userID {
			return ErrCannotRevokeSelfAdmin
		}
		role, ok := c.RBAC.GetRole(roleName)
		if !ok {
			return ErrRoleNotFound
		}
		if actorID != SystemActorID {
			canManage := c.RBAC.GetUserHighestPosition(actorID) == PositionOwner || c.RBAC.GetUserHighestPosition(actorID) > role.GetPosition()
			if !canManage {
				return ErrCannotRevokeHigherRole
			}
			if actorID != userID {
				if c.RBAC.GetUserHighestPosition(actorID) <= c.RBAC.GetUserHighestPosition(userID) {
					return ErrCannotManageHigherUser
				}
			}
		}
		return nil
	}); err != nil {
		return err
	}

	c.logger.Info("Revoked role", "role", roleName, "user_id", userID, "actor_id", actorID)
	return nil
}

// GetRoleUsers returns all user IDs explicitly assigned to a role.
// The implicit `everyone` role returns []; all authenticated users carry it.
func (c *ChattoCore) GetRoleUsers(ctx context.Context, roleName string) ([]string, error) {
	if roleName == RoleEveryone {
		return []string{}, nil
	}
	if !c.RBAC.RoleExists(roleName) {
		return nil, ErrRoleNotFound
	}
	return c.RBAC.GetRoleUsers(roleName), nil
}

// GetUserRoles returns the explicit role assignments for a user. The implicit
// `everyone` role is omitted — callers that need it can prepend it themselves
// based on the relevant scope (e.g. space membership).
func (c *ChattoCore) GetUserRoles(ctx context.Context, userID string) ([]string, error) {
	assignedRoles := c.RBAC.GetUserRoles(userID)
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

// RevokeServerPermission removes a permission grant from a role.
// This only removes grants, not denials. Use ClearServerPermissionState
// to remove both. Idempotent — revoking a non-granted permission is a no-op.
//
// (GrantServerPermission, DenyServerPermission, and
// ClearServerPermissionState live in permission_ops.go, alongside the
// space-tier and room-tier counterparts.)
func (c *ChattoCore) RevokeServerPermission(ctx context.Context, roleName string, perm Permission) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacPermissionCleared{
		RbacPermissionCleared: rbacRolePermissionClearedEvent(ScopeServer, "", roleName, perm),
	}})
	if _, err := c.appendRBACEvent(ctx, event, func() error {
		if !c.RBAC.RoleExists(roleName) {
			return ErrRoleNotFound
		}
		if c.RBAC.GetDecision(ScopeServer, "", roleName, perm) != DecisionAllow {
			return errRBACNoop
		}
		return nil
	}); err != nil {
		if errors.Is(err, errRBACNoop) {
			return nil
		}
		return err
	}
	c.logger.Info("Revoked server permission", "role", roleName, "permission", perm)
	return nil
}

// GetServerRolePermissions returns all permissions granted to an role.
// Note: Admin roles are NOT special-cased - permissions are materialized in the RBAC projection.
func (c *ChattoCore) GetServerRolePermissions(ctx context.Context, roleName string) ([]Permission, error) {
	if !c.RBAC.RoleExists(roleName) {
		return nil, ErrRoleNotFound
	}
	grants, _ := c.RBAC.DecisionsForRoleServer(roleName)
	return grants, nil
}

// GetServerRolePermissionDenials returns all permissions denied by an role.
// Note: Admin roles are NOT special-cased - they can have denials like any other role.
func (c *ChattoCore) GetServerRolePermissionDenials(ctx context.Context, roleName string) ([]Permission, error) {
	if !c.RBAC.RoleExists(roleName) {
		return nil, ErrRoleNotFound
	}
	_, denials := c.RBAC.DecisionsForRoleServer(roleName)
	return denials, nil
}

// AllServerPermissions returns all defined server permissions.
// Exposed as a method for consistency with other core APIs.
func (c *ChattoCore) AllServerPermissions() []Permission {
	perms := PermissionsForScope(ScopeServer)
	result := make([]Permission, len(perms))
	for i, p := range perms {
		result[i] = p.Permission
	}
	return result
}

// GetUserServerPermissions returns all server permissions the user has,
// using the unified resolver.
func (c *ChattoCore) GetUserServerPermissions(ctx context.Context, userID string) ([]Permission, error) {
	var result []Permission
	for _, meta := range PermissionsForScope(ScopeServer) {
		decision, err := c.ResolveUserPermission(ctx, userID, KindChannel, "", meta.Permission)
		if err != nil {
			return nil, fmt.Errorf("failed to check permission %s: %w", meta.Permission, err)
		}
		if decision == DecisionAllow {
			result = append(result, meta.Permission)
		}
	}
	return result, nil
}

// ============================================================================
// Server-tier Role CRUD
// ============================================================================

// ListServerRoles returns all roles with their permissions.
// Note: Admin roles are NOT special-cased - permissions are read from the RBAC projection.
func (c *ChattoCore) ListServerRoles(ctx context.Context) ([]RoleWithPermissions, error) {
	roles := c.RBAC.ListRoles()
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
	if err := ValidateRoleName(name); err != nil {
		return nil, ErrInvalidRoleName
	}
	if err := validateRoleMetadata(displayName, description); err != nil {
		return nil, err
	}
	if IsSystemRole(name) {
		return nil, ErrRoleAlreadyExists
	}

	var role *corev1.Role
	event := newEvent(SystemActorID, &corev1.Event{})
	if _, err := c.appendRBACEvent(ctx, event, func() error {
		if c.RBAC.RoleExists(name) {
			return ErrRoleAlreadyExists
		}
		role = &corev1.Role{
			Name:        name,
			DisplayName: displayName,
			Description: description,
			Position:    c.RBAC.NextAvailablePosition(),
		}
		event.Event = &corev1.Event_RbacRoleCreated{
			RbacRoleCreated: &corev1.RbacRoleCreatedEvent{
				RoleName:    role.GetName(),
				DisplayName: role.GetDisplayName(),
				Description: role.GetDescription(),
				Rank:        role.GetPosition(),
			},
		}
		return nil
	}); err != nil {
		return nil, err
	}

	c.logger.Info("Created role", "name", name, "display_name", displayName, "position", role.GetPosition())

	return &RoleWithPermissions{
		Name:              role.GetName(),
		DisplayName:       role.GetDisplayName(),
		Description:       role.GetDescription(),
		Permissions:       []Permission{},
		PermissionDenials: []Permission{},
		IsSystem:          false,
		Position:          role.GetPosition(),
	}, nil
}

// UpdateServerRole updates an existing role's metadata.
// The role name cannot be changed.
func (c *ChattoCore) UpdateServerRole(ctx context.Context, name, displayName, description string) (*RoleWithPermissions, error) {
	if err := validateRoleMetadata(displayName, description); err != nil {
		return nil, err
	}

	var updated *corev1.Role
	if _, err := c.appendRBACEvent(ctx, newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacRoleDisplayNameChanged{
		RbacRoleDisplayNameChanged: &corev1.RbacRoleDisplayNameChangedEvent{RoleName: name, DisplayName: displayName},
	}}), func() error {
		existing, ok := c.RBAC.GetRole(name)
		if !ok {
			return ErrRoleNotFound
		}
		if existing.GetDisplayName() == displayName {
			updated = existing
			return errRBACNoop
		}
		updated = &corev1.Role{
			Name:        existing.GetName(),
			DisplayName: displayName,
			Description: existing.GetDescription(),
			Position:    existing.GetPosition(),
		}
		return nil
	}); err != nil {
		if !errors.Is(err, errRBACNoop) {
			return nil, err
		}
	}

	if _, err := c.appendRBACEvent(ctx, newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacRoleDescriptionChanged{
		RbacRoleDescriptionChanged: &corev1.RbacRoleDescriptionChangedEvent{RoleName: name, Description: description},
	}}), func() error {
		existing, ok := c.RBAC.GetRole(name)
		if !ok {
			return ErrRoleNotFound
		}
		if existing.GetDescription() == description {
			updated = existing
			return errRBACNoop
		}
		updated = &corev1.Role{
			Name:        existing.GetName(),
			DisplayName: existing.GetDisplayName(),
			Description: description,
			Position:    existing.GetPosition(),
		}
		return nil
	}); err != nil {
		if !errors.Is(err, errRBACNoop) {
			return nil, err
		}
	}

	if updated == nil {
		existing, ok := c.RBAC.GetRole(name)
		if !ok {
			return nil, ErrRoleNotFound
		}
		updated = existing
	}

	c.logger.Info("Updated role", "name", name, "display_name", displayName)

	perms, _ := c.GetServerRolePermissions(ctx, name)
	denials, _ := c.GetServerRolePermissionDenials(ctx, name)
	return &RoleWithPermissions{
		Name:              updated.Name,
		DisplayName:       updated.DisplayName,
		Description:       updated.Description,
		Permissions:       perms,
		PermissionDenials: denials,
		IsSystem:          IsSystemRole(name),
		Position:          updated.Position,
	}, nil
}

// GetServerRole returns a single role by name.
// Note: Admin roles are NOT special-cased - permissions are read from the RBAC projection.
func (c *ChattoCore) GetServerRole(ctx context.Context, name string) (*RoleWithPermissions, error) {
	role, ok := c.RBAC.GetRole(name)
	if !ok {
		return nil, ErrRoleNotFound
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

	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacRoleDeleted{
		RbacRoleDeleted: &corev1.RbacRoleDeletedEvent{RoleName: name},
	}})
	if _, err := c.appendRBACEvent(ctx, event, func() error {
		if !c.RBAC.RoleExists(name) {
			return ErrRoleNotFound
		}
		return nil
	}); err != nil {
		return err
	}

	c.logger.Info("Deleted role", "role", name)
	return nil
}

// ReorderServerRoles reorders custom roles.
// System roles (owner, admin, moderator, everyone) maintain fixed positions and should not be included.
// Positions are assigned from PositionCustomFirst upward while skipping system-role positions.
// Note: everyone=0, moderator=100, admin=900, owner=1000.
// Returns all roles sorted by position.
func (c *ChattoCore) ReorderServerRoles(ctx context.Context, roleNames []string) ([]RoleWithPermissions, error) {
	for _, name := range roleNames {
		if IsSystemRole(name) {
			return nil, fmt.Errorf("cannot reorder system role: %s", name)
		}
	}

	event := newEvent(SystemActorID, &corev1.Event{})
	if _, err := c.appendRBACEvent(ctx, event, func() error {
		customRoles := make(map[string]struct{})
		for _, role := range c.RBAC.ListRoles() {
			if role.GetName() == "" || IsSystemRole(role.GetName()) {
				continue
			}
			customRoles[role.GetName()] = struct{}{}
		}
		if len(roleNames) != len(customRoles) {
			return fmt.Errorf("role reorder must include every custom role exactly once")
		}

		seen := make(map[string]struct{}, len(roleNames))
		for _, name := range roleNames {
			if _, ok := seen[name]; ok {
				return fmt.Errorf("duplicate role in reorder: %s", name)
			}
			seen[name] = struct{}{}
			if _, ok := customRoles[name]; !ok {
				return fmt.Errorf("role %s: %w", name, ErrRoleNotFound)
			}
		}
		event.Event = &corev1.Event_RbacRolesReordered{
			RbacRolesReordered: &corev1.RbacRolesReorderedEvent{RoleNames: roleNames},
		}
		return nil
	}); err != nil {
		return nil, err
	}

	allRoles := c.RBAC.ListRoles()
	result := make([]RoleWithPermissions, 0, len(allRoles))
	for _, role := range allRoles {
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

	c.logger.Info("Reordered roles", "order", roleNames)
	return result, nil
}

// GetRoomRolePermissions returns the per-room override grants and denials
// for a role in a specific room. Reads ADR-031's room_allow / room_deny
// key families.
func (c *ChattoCore) GetRoomRolePermissions(ctx context.Context, roomID, roleName string) (grants []Permission, denials []Permission, err error) {
	grants, denials = c.RBAC.DecisionsFor(ScopeRoom, roomID, roleName)
	return grants, denials, nil
}

// GetGroupRolePermissions returns the set-scope grants and denials for a role
// in a specific room group (ADR-031).
func (c *ChattoCore) GetGroupRolePermissions(ctx context.Context, groupID, roleName string) (grants []Permission, denials []Permission, err error) {
	grants, denials = c.RBAC.DecisionsFor(ScopeGroup, groupID, roleName)
	return grants, denials, nil
}

// GrantGroupPermission writes a group-scope grant for a role on a specific room group.
func (c *ChattoCore) GrantGroupPermission(ctx context.Context, groupID, roleName string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeGroup) && !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at group scope", perm)
	}
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacPermissionGranted{
		RbacPermissionGranted: rbacRolePermissionGrantedEvent(ScopeGroup, groupID, roleName, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// DenyGroupPermission writes a group-scope deny for a role on a specific room group.
func (c *ChattoCore) DenyGroupPermission(ctx context.Context, groupID, roleName string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeGroup) && !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at group scope", perm)
	}
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacPermissionDenied{
		RbacPermissionDenied: rbacRolePermissionDeniedEvent(ScopeGroup, groupID, roleName, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// ClearGroupPermissionState removes both allow and deny for a role on a set.
func (c *ChattoCore) ClearGroupPermissionState(ctx context.Context, groupID, roleName string, perm Permission) error {
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacPermissionCleared{
		RbacPermissionCleared: rbacRolePermissionClearedEvent(ScopeGroup, groupID, roleName, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// OutranksUser reports whether actor outranks target by role-hierarchy
// position (higher position = higher rank). Users with no explicit roles
// fall back to PositionEveryone.
//
// This is a HIERARCHY CHECK, not an authorization check. To gate an
// action against another user, callers MUST also check the relevant
// permission. See .claude/rules/authorization.md (`permission AND
// OutranksUser`).
func (c *ChattoCore) OutranksUser(ctx context.Context, actorID, targetID string) (bool, error) {
	return c.RBAC.GetUserHighestPosition(actorID) > c.RBAC.GetUserHighestPosition(targetID), nil
}

// GetUserEffectiveSpacePermissions returns all permissions the user effectively has for a
// room kind. Delegates to PermissionResolver.HasSpacePermission for each space-scoped
// permission, ensuring consistent resolution logic (unified hierarchy-wins; DM rooms
// additionally have their boundary deny-list applied).
func (c *ChattoCore) GetUserEffectiveSpacePermissions(ctx context.Context, kind RoomKind, userID string) ([]Permission, error) {
	if kind == KindDM {
		return []Permission{
			PermRoomJoin,
			PermMessagePost,
			PermMessageReact,
		}, nil
	}

	var result []Permission
	for _, permMeta := range PermissionsForScope(ScopeServer) {
		perm := permMeta.Permission
		has, err := c.permissionResolver.HasSpacePermission(ctx, userID, kind, perm)
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
	roles := c.RBAC.GetUserRoles(userID)
	entries := make([]events.BatchEntry, 0, len(roles))
	for _, roleName := range roles {
		event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacRoleRevoked{
			RbacRoleRevoked: &corev1.RbacRoleRevokedEvent{UserId: userID, RoleName: roleName},
		}})
		entries = append(entries, events.BatchEntry{Subject: rbacSubjectForEvent(event), Event: event})
	}
	if _, err := c.appendRBACBatch(ctx, entries, nil); err != nil {
		return fmt.Errorf("failed to revoke user roles: %w", err)
	}

	c.logger.Debug("Revoked all roles for user", "user_id", userID)
	return nil
}

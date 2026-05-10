package rbac

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"slices"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// Position constants for role hierarchy.
// Lower position = higher rank (more power).
//
// WARNING: Do not perform arithmetic on PositionEveryone
// as it uses MaxInt32 to ensure it's always the lowest rank.
// Adding to PositionEveryone would cause integer overflow.
const (
	PositionOwner     int32 = 0             // Owner is highest rank
	PositionAdmin     int32 = 1             // Admin is second-highest (instance only)
	PositionModerator int32 = 2             // Moderator (instance: 2, space: 1)
	PositionEveryone  int32 = math.MaxInt32 // Everyone is always the lowest rank
)

// Engine provides generic RBAC operations against a KV bucket.
// It handles role CRUD, permission grants, and role assignments.
// Scope-specific behavior (implicit roles, config fallback) is handled by adapters.
type Engine struct {
	kv     jetstream.KeyValue
	config Config
	logger *slog.Logger
}

// Config configures the RBAC engine behavior.
type Config struct {
	// SystemRoles that cannot be deleted or reordered.
	SystemRoles []string

	// AdminRole name for the administrative role.
	// This is only used by HasUserAdmin to check if a user has the admin role.
	// Admin roles do NOT have implicit permissions - permissions must be explicitly granted.
	AdminRole string

	// VirtualRoles are system roles that are not stored in KV but returned by
	// ListRoles and GetRole. They have fixed definitions and positions.
	// Role assignments and permissions are still stored in KV.
	VirtualRoles []*corev1.Role

	// ValidateVerbObjectType validates a verb+objectType combination.
	// Used to ensure only valid permission types are stored.
	// If nil, no validation is performed.
	ValidateVerbObjectType func(verb, objectType string) error

	// Logger for RBAC operations. If nil, no logging is performed.
	Logger *slog.Logger
}

// NewEngine creates a new RBAC engine with the given KV bucket and configuration.
func NewEngine(kv jetstream.KeyValue, config Config) *Engine {
	return &Engine{
		kv:     kv,
		config: config,
		logger: config.Logger,
	}
}

// KV returns the underlying KV bucket for direct access.
// Use this for permission resolution that needs to check keys directly.
func (e *Engine) KV() jetstream.KeyValue {
	return e.kv
}

// ============================================================================
// Key Construction Helpers
// ============================================================================
// These methods construct keys using the new format:
// allow.{subject}.{verb}.{objectType}.{objectId}
// deny.{subject}.{verb}.{objectType}.{objectId}

// allowKey returns the KV key for a permission grant on a subject.
func (e *Engine) allowKey(subject, verb, objectType, objectId string) string {
	return AllowKey(subject, verb, objectType, objectId)
}

// denyKey returns the KV key for a permission denial on a subject.
func (e *Engine) denyKey(subject, verb, objectType, objectId string) string {
	return DenyKey(subject, verb, objectType, objectId)
}

// allowPatternForSubject returns a pattern matching all grants for a subject.
func (e *Engine) allowPatternForSubject(subject string) string {
	return AllowPatternForSubject(subject)
}

// denyPatternForSubject returns a pattern matching all denials for a subject.
func (e *Engine) denyPatternForSubject(subject string) string {
	return DenyPatternForSubject(subject)
}

// isSystemRole checks if the role name is a protected system role.
func (e *Engine) isSystemRole(name string) bool {
	return slices.Contains(e.config.SystemRoles, name)
}

func (e *Engine) log() *slog.Logger {
	if e.logger == nil {
		return slog.Default()
	}
	return e.logger
}

// isVirtualRole checks if the role name is a virtual system role.
func (e *Engine) isVirtualRole(name string) bool {
	for _, vr := range e.config.VirtualRoles {
		if vr.Name == name {
			return true
		}
	}
	return false
}

// getVirtualRole returns the virtual role definition, or nil if not virtual.
func (e *Engine) getVirtualRole(name string) *corev1.Role {
	for _, vr := range e.config.VirtualRoles {
		if vr.Name == name {
			return vr
		}
	}
	return nil
}

// ============================================================================
// Role CRUD Operations
// ============================================================================

// CreateRole creates a new role with auto-assigned position.
// The position is set to the next available position (after existing custom roles).
// Returns ErrRoleAlreadyExists if a role with the same name already exists.
// Returns ErrInvalidRoleName if the name doesn't match the required format.
func (e *Engine) CreateRole(ctx context.Context, name, displayName, description string) (*corev1.Role, error) {
	// Auto-assign position based on existing roles
	position, err := e.GetNextAvailablePosition(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get next position: %w", err)
	}
	return e.CreateRoleWithPosition(ctx, name, displayName, description, position)
}

// CreateRoleWithPosition creates a new role with an explicit position.
// Returns ErrRoleAlreadyExists if a role with the same name already exists.
// Returns ErrInvalidRoleName if the name doesn't match the required format.
func (e *Engine) CreateRoleWithPosition(ctx context.Context, name, displayName, description string, position int32) (*corev1.Role, error) {
	if err := ValidateRoleName(name); err != nil {
		return nil, err
	}

	role := &corev1.Role{
		Name:        name,
		DisplayName: displayName,
		Description: description,
		Position:    position,
	}

	data, err := proto.Marshal(role)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal role: %w", err)
	}

	_, err = e.kv.Create(ctx, RoleKey(name), data)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyExists) {
			return nil, ErrRoleAlreadyExists
		}
		return nil, fmt.Errorf("failed to store role: %w", err)
	}

	e.log().Info("Created role", "name", name, "display_name", displayName, "position", position)
	return role, nil
}

// GetRole retrieves a role by name.
// Returns ErrRoleNotFound if the role doesn't exist.
func (e *Engine) GetRole(ctx context.Context, name string) (*corev1.Role, error) {
	// Check virtual roles first
	if vr := e.getVirtualRole(name); vr != nil {
		return vr, nil
	}

	entry, err := e.kv.Get(ctx, RoleKey(name))
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, ErrRoleNotFound
		}
		return nil, fmt.Errorf("failed to get role: %w", err)
	}

	role := &corev1.Role{}
	if err := proto.Unmarshal(entry.Value(), role); err != nil {
		return nil, fmt.Errorf("failed to unmarshal role: %w", err)
	}

	return role, nil
}

// ListRoles retrieves all roles, sorted by position (lowest first = highest rank).
// Virtual roles are merged with KV-stored custom roles.
func (e *Engine) ListRoles(ctx context.Context) ([]*corev1.Role, error) {
	// Start with virtual roles
	virtualNames := make(map[string]bool)
	var roles []*corev1.Role
	for _, vr := range e.config.VirtualRoles {
		virtualNames[vr.Name] = true
		roles = append(roles, vr)
	}

	// Add custom roles from KV (skip any that match virtual role names)
	keyLister, err := e.kv.ListKeysFiltered(ctx, RoleKeyPattern)
	if err == nil {
		for key := range keyLister.Keys() {
			entry, err := e.kv.Get(ctx, key)
			if err != nil {
				return nil, fmt.Errorf("failed to get role %s: %w", key, err)
			}

			role := &corev1.Role{}
			if err := proto.Unmarshal(entry.Value(), role); err != nil {
				return nil, fmt.Errorf("failed to unmarshal role %s: %w", key, err)
			}

			// Skip if this is a virtual role (virtual takes precedence)
			if virtualNames[role.Name] {
				continue
			}

			roles = append(roles, role)
		}
	}

	// Sort by position (lower = higher rank)
	slices.SortFunc(roles, func(a, b *corev1.Role) int {
		return int(a.Position - b.Position)
	})

	return roles, nil
}

// UpdateRole updates an existing role's display name and description.
// The role name (identifier) and position are preserved.
// Returns ErrRoleNotFound if the role doesn't exist.
func (e *Engine) UpdateRole(ctx context.Context, name, displayName, description string) (*corev1.Role, error) {
	// Get existing role to preserve position
	existing, err := e.GetRole(ctx, name)
	if err != nil {
		return nil, err
	}

	role := &corev1.Role{
		Name:        name,
		DisplayName: displayName,
		Description: description,
		Position:    existing.Position, // Preserve position
	}

	data, err := proto.Marshal(role)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal role: %w", err)
	}

	_, err = e.kv.Put(ctx, RoleKey(name), data)
	if err != nil {
		return nil, fmt.Errorf("failed to update role: %w", err)
	}

	e.log().Info("Updated role", "name", name, "display_name", displayName)
	return role, nil
}

// DeleteRole deletes a role and all associated data.
// This includes: the role definition, all permission grants, and all user assignments.
// Returns ErrCannotDeleteSystemRole if the role is a system role.
// Returns ErrRoleNotFound if the role doesn't exist.
func (e *Engine) DeleteRole(ctx context.Context, name string) error {
	if e.isSystemRole(name) {
		return ErrCannotDeleteSystemRole
	}

	// Verify role exists
	_, err := e.GetRole(ctx, name)
	if err != nil {
		return err
	}

	// Delete all permission grants for this role
	permPattern := e.allowPatternForSubject(name)
	if permKeys, err := e.kv.ListKeysFiltered(ctx, permPattern); err == nil {
		for key := range permKeys.Keys() {
			if err := e.kv.Delete(ctx, key); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
				e.log().Warn("Failed to delete role permission grant", "key", key, "error", err)
			}
		}
	}

	// Delete all permission denials for this role
	denyPattern := e.denyPatternForSubject(name)
	if denyKeys, err := e.kv.ListKeysFiltered(ctx, denyPattern); err == nil {
		for key := range denyKeys.Keys() {
			if err := e.kv.Delete(ctx, key); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
				e.log().Warn("Failed to delete role permission denial", "key", key, "error", err)
			}
		}
	}

	// Delete all user assignments for this role
	assignPattern := MemberPatternForRole(name)
	if assignKeys, err := e.kv.ListKeysFiltered(ctx, assignPattern); err == nil {
		for key := range assignKeys.Keys() {
			if err := e.kv.Delete(ctx, key); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
				e.log().Warn("Failed to delete role assignment", "key", key, "error", err)
			}
		}
	}

	// Delete the role definition
	if err := e.kv.Delete(ctx, RoleKey(name)); err != nil {
		return fmt.Errorf("failed to delete role: %w", err)
	}

	e.log().Info("Deleted role", "name", name)
	return nil
}

// ============================================================================
// Role Permission Operations
// ============================================================================

// GrantRolePermission adds a permission grant to a role.
// The operation is idempotent - granting an existing permission is a no-op.
// Also removes any existing denial for the same permission on this role.
// Returns ErrRoleNotFound if the role doesn't exist (unless it's a virtual role).
// Returns ErrInvalidPermission if the verb+objectType combination is not valid.
//
// Parameters:
//   - roleName: the role to grant the permission to
//   - verb: the action (e.g., "create", "delete-own", "view-users")
//   - objectType: what the permission applies to (e.g., "room", "message", "admin")
//   - objectId: specific object ID or "any" for wildcard
func (e *Engine) GrantRolePermission(ctx context.Context, roleName, verb, objectType, objectId string) error {
	if e.config.ValidateVerbObjectType != nil {
		if err := e.config.ValidateVerbObjectType(verb, objectType); err != nil {
			return err
		}
	}

	// Virtual roles don't need existence check - they always exist
	if !e.isVirtualRole(roleName) {
		// Verify role exists in KV
		_, err := e.GetRole(ctx, roleName)
		if err != nil {
			return err
		}
	}

	// Remove any existing denial (grant replaces denial)
	_ = e.kv.Delete(ctx, e.denyKey(roleName, verb, objectType, objectId))

	// Store permission (idempotent - Put overwrites if exists)
	_, err := e.kv.Put(ctx, e.allowKey(roleName, verb, objectType, objectId), []byte{})
	if err != nil {
		return fmt.Errorf("failed to grant permission: %w", err)
	}

	e.log().Debug("Granted permission", "role", roleName, "verb", verb, "objectType", objectType, "objectId", objectId)
	return nil
}

// RevokeRolePermission removes a permission grant from a role.
// This only removes grants, not denials. Use ClearRolePermissionState to remove both.
// The operation is idempotent - revoking a non-existent permission is a no-op.
// Returns ErrRoleNotFound if the role doesn't exist (unless it's a virtual role).
func (e *Engine) RevokeRolePermission(ctx context.Context, roleName, verb, objectType, objectId string) error {
	// Virtual roles don't need existence check - they always exist
	if !e.isVirtualRole(roleName) {
		// Verify role exists in KV
		_, err := e.GetRole(ctx, roleName)
		if err != nil {
			return err
		}
	}

	// Delete permission (idempotent - ignore if not found)
	err := e.kv.Delete(ctx, e.allowKey(roleName, verb, objectType, objectId))
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to revoke permission: %w", err)
	}

	e.log().Debug("Revoked permission", "role", roleName, "verb", verb, "objectType", objectType, "objectId", objectId)
	return nil
}

// RolePermission represents a permission granted or denied to a role.
type RolePermission struct {
	Verb       string
	ObjectType string
	ObjectId   string
}

// GetRolePermissions returns all permissions granted to a role.
// Returns an empty slice if no permissions are granted.
// Returns ErrRoleNotFound if the role doesn't exist (unless it's a virtual role).
func (e *Engine) GetRolePermissions(ctx context.Context, roleName string) ([]RolePermission, error) {
	// Virtual roles don't need existence check - they always exist
	// Permissions are still stored in KV even for virtual roles
	if !e.isVirtualRole(roleName) {
		// Verify role exists in KV
		_, err := e.GetRole(ctx, roleName)
		if err != nil {
			return nil, err
		}
	}

	pattern := e.allowPatternForSubject(roleName)
	keyLister, err := e.kv.ListKeysFiltered(ctx, pattern)
	if err != nil {
		// No keys found is not an error
		return []RolePermission{}, nil
	}

	var permissions []RolePermission
	for key := range keyLister.Keys() {
		parts := ParseAllowKey(key)
		if parts.Subject != "" {
			permissions = append(permissions, RolePermission{
				Verb:       parts.Verb,
				ObjectType: parts.ObjectType,
				ObjectId:   parts.ObjectId,
			})
		}
	}

	return permissions, nil
}

// DenyRolePermission adds a permission denial to a role.
// When any role denies a permission, it overrides grants from other roles (deny-override pattern).
// The operation is idempotent - denying an existing denial is a no-op.
// Also removes any existing grant for the same permission on this role.
// Returns ErrRoleNotFound if the role doesn't exist (unless it's a virtual role).
// Returns ErrInvalidPermission if the verb+objectType combination is not valid.
func (e *Engine) DenyRolePermission(ctx context.Context, roleName, verb, objectType, objectId string) error {
	if e.config.ValidateVerbObjectType != nil {
		if err := e.config.ValidateVerbObjectType(verb, objectType); err != nil {
			return err
		}
	}

	// Virtual roles don't need existence check - they always exist
	if !e.isVirtualRole(roleName) {
		// Verify role exists in KV
		_, err := e.GetRole(ctx, roleName)
		if err != nil {
			return err
		}
	}

	// Remove any existing grant (denial takes precedence, but clean up)
	_ = e.kv.Delete(ctx, e.allowKey(roleName, verb, objectType, objectId))

	// Store denial (idempotent - Put overwrites if exists)
	_, err := e.kv.Put(ctx, e.denyKey(roleName, verb, objectType, objectId), []byte{})
	if err != nil {
		return fmt.Errorf("failed to deny permission: %w", err)
	}

	e.log().Debug("Denied permission", "role", roleName, "verb", verb, "objectType", objectType, "objectId", objectId)
	return nil
}

// ClearRolePermissionState removes both grant and denial for a permission on a role.
// This returns the permission to a neutral state (neither granted nor denied).
// The operation is idempotent.
// Returns ErrRoleNotFound if the role doesn't exist (unless it's a virtual role).
func (e *Engine) ClearRolePermissionState(ctx context.Context, roleName, verb, objectType, objectId string) error {
	// Virtual roles don't need existence check - they always exist
	if !e.isVirtualRole(roleName) {
		// Verify role exists in KV
		_, err := e.GetRole(ctx, roleName)
		if err != nil {
			return err
		}
	}

	// Remove both grant and denial
	err1 := e.kv.Delete(ctx, e.allowKey(roleName, verb, objectType, objectId))
	err2 := e.kv.Delete(ctx, e.denyKey(roleName, verb, objectType, objectId))

	// Ignore not found errors
	if err1 != nil && !errors.Is(err1, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to clear grant: %w", err1)
	}
	if err2 != nil && !errors.Is(err2, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to clear denial: %w", err2)
	}

	e.log().Debug("Cleared permission state", "role", roleName, "verb", verb, "objectType", objectType, "objectId", objectId)
	return nil
}

// GetRolePermissionDenials returns all permissions denied by a role.
// Returns an empty slice if no permissions are denied.
// Returns ErrRoleNotFound if the role doesn't exist (unless it's a virtual role).
func (e *Engine) GetRolePermissionDenials(ctx context.Context, roleName string) ([]RolePermission, error) {
	// Virtual roles don't need existence check - they always exist
	// Denials are still stored in KV even for virtual roles
	if !e.isVirtualRole(roleName) {
		// Verify role exists in KV
		_, err := e.GetRole(ctx, roleName)
		if err != nil {
			return nil, err
		}
	}

	pattern := e.denyPatternForSubject(roleName)
	keyLister, err := e.kv.ListKeysFiltered(ctx, pattern)
	if err != nil {
		// No keys found is not an error
		return []RolePermission{}, nil
	}

	var denials []RolePermission
	for key := range keyLister.Keys() {
		parts := ParseDenyKey(key)
		if parts.Subject != "" {
			denials = append(denials, RolePermission{
				Verb:       parts.Verb,
				ObjectType: parts.ObjectType,
				ObjectId:   parts.ObjectId,
			})
		}
	}

	return denials, nil
}

// ============================================================================
// Role Assignment Operations
// ============================================================================

// AssignRole assigns a role to a user.
// The operation is idempotent - assigning an existing role is a no-op.
// Returns ErrRoleNotFound if the role doesn't exist (unless it's a virtual role).
func (e *Engine) AssignRole(ctx context.Context, userID, roleName string) error {
	// Virtual roles don't need existence check - they always exist
	if !e.isVirtualRole(roleName) {
		// Verify role exists in KV
		_, err := e.GetRole(ctx, roleName)
		if err != nil {
			return err
		}
	}

	// Store assignment (idempotent - Put overwrites if exists)
	_, err := e.kv.Put(ctx, MemberKey(roleName, userID), []byte{})
	if err != nil {
		return fmt.Errorf("failed to assign role: %w", err)
	}

	e.log().Debug("Assigned role", "role", roleName, "user_id", userID)
	return nil
}

// RevokeRole removes a role from a user.
// The operation is idempotent - revoking a non-assigned role is a no-op.
// Returns ErrRoleNotFound if the role doesn't exist (unless it's a virtual role).
func (e *Engine) RevokeRole(ctx context.Context, userID, roleName string) error {
	// Virtual roles don't need existence check - they always exist
	if !e.isVirtualRole(roleName) {
		// Verify role exists in KV
		_, err := e.GetRole(ctx, roleName)
		if err != nil {
			return err
		}
	}

	// Delete assignment (idempotent - ignore if not found)
	err := e.kv.Delete(ctx, MemberKey(roleName, userID))
	if err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to revoke role: %w", err)
	}

	e.log().Debug("Revoked role", "role", roleName, "user_id", userID)
	return nil
}

// GetUserRoles returns all roles assigned to a user.
// Returns an empty slice if no roles are assigned.
func (e *Engine) GetUserRoles(ctx context.Context, userID string) ([]string, error) {
	pattern := MemberPatternForUser(userID)
	keyLister, err := e.kv.ListKeysFiltered(ctx, pattern)
	if err != nil {
		// No keys found is not an error
		return []string{}, nil
	}

	var roles []string
	for key := range keyLister.Keys() {
		// Extract role name from key: role_assignment.{roleName}.{userID}
		withoutPrefix := key[len(MemberKeyPrefix):]
		roleName := withoutPrefix[:len(withoutPrefix)-len(userID)-1] // -1 for the dot
		roles = append(roles, roleName)
	}

	return roles, nil
}

// GetRoleUsers returns all users assigned to a role.
// Returns an empty slice if no users are assigned.
// Returns ErrRoleNotFound if the role doesn't exist (unless it's a virtual role).
func (e *Engine) GetRoleUsers(ctx context.Context, roleName string) ([]string, error) {
	// Virtual roles don't need existence check - they always exist
	if !e.isVirtualRole(roleName) {
		// Verify role exists in KV
		_, err := e.GetRole(ctx, roleName)
		if err != nil {
			return nil, err
		}
	}

	pattern := MemberPatternForRole(roleName)
	keyLister, err := e.kv.ListKeysFiltered(ctx, pattern)
	if err != nil {
		// No keys found is not an error
		return []string{}, nil
	}

	var users []string
	prefix := MemberKeyPrefix + roleName + "."
	for key := range keyLister.Keys() {
		userID := key[len(prefix):]
		users = append(users, userID)
	}

	return users, nil
}


// ============================================================================
// Permission Checking
// ============================================================================

// HasPermissionViaRoles checks if a user has a permission through their assigned roles.
// This is a low-level check that:
//   - Returns false if any of the user's roles DENIES the permission (deny-override pattern)
//   - Returns true if any of the user's roles GRANTS the permission
//   - Returns false otherwise
//
// This does NOT check:
//   - User-level overrides (grants/denials) - adapters should check these separately
//   - Implicit roles (like "everyone") - adapters should handle these
//
// Note: Admin roles are NOT special-cased here. They work like any other role
// and must have permissions explicitly granted.
func (e *Engine) HasPermissionViaRoles(ctx context.Context, userID, verb, objectType, objectId string) (bool, error) {
	// Get all roles for the user
	roles, err := e.GetUserRoles(ctx, userID)
	if err != nil {
		return false, fmt.Errorf("failed to get user roles: %w", err)
	}

	// Check if ANY role denies this permission (deny-override pattern).
	// We must check ALL denials before ANY grants because a single denial blocks the permission.
	// This requires two loops through roles - merging them would require storing intermediate
	// results, adding complexity without meaningful benefit since the expensive part (KV lookups)
	// is the same count either way.
	for _, roleName := range roles {
		_, err := e.kv.Get(ctx, e.denyKey(roleName, verb, objectType, objectId))
		if err == nil {
			return false, nil // Denied by role
		}
		if !errors.Is(err, jetstream.ErrKeyNotFound) {
			return false, fmt.Errorf("failed to check permission denial: %w", err)
		}
	}

	// Check if any role grants the permission
	for _, roleName := range roles {
		_, err := e.kv.Get(ctx, e.allowKey(roleName, verb, objectType, objectId))
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, jetstream.ErrKeyNotFound) {
			return false, fmt.Errorf("failed to check permission: %w", err)
		}
	}

	return false, nil
}

// HasUserAdmin checks if a user has the admin role assigned.
// Returns false if no AdminRole is configured.
func (e *Engine) HasUserAdmin(ctx context.Context, userID string) (bool, error) {
	if e.config.AdminRole == "" {
		return false, nil
	}

	_, err := e.kv.Get(ctx, MemberKey(e.config.AdminRole, userID))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return false, nil
	}
	return false, fmt.Errorf("failed to check admin role: %w", err)
}

// ============================================================================
// Utility Operations
// ============================================================================

// RoleExists checks if a role exists (virtual or stored in KV).
func (e *Engine) RoleExists(ctx context.Context, name string) (bool, error) {
	// Virtual roles always exist
	if e.isVirtualRole(name) {
		return true, nil
	}

	_, err := e.kv.Get(ctx, RoleKey(name))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return false, nil
	}
	return false, fmt.Errorf("failed to check role: %w", err)
}

// HasRole checks if a user has a specific role assigned.
func (e *Engine) HasRole(ctx context.Context, userID, roleName string) (bool, error) {
	_, err := e.kv.Get(ctx, MemberKey(roleName, userID))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return false, nil
	}
	return false, fmt.Errorf("failed to check role: %w", err)
}

// RevokeAllUserRoles removes all role assignments for a user.
// This is used during cleanup (e.g., when a user leaves a space).
// The operation is best-effort - individual deletion failures are logged but don't fail the operation.
func (e *Engine) RevokeAllUserRoles(ctx context.Context, userID string) error {
	pattern := MemberPatternForUser(userID)
	keyLister, err := e.kv.ListKeysFiltered(ctx, pattern)
	if err != nil {
		// No keys found is not an error
		return nil
	}

	for key := range keyLister.Keys() {
		if err := e.kv.Delete(ctx, key); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
			e.log().Warn("Failed to delete role assignment during user cleanup", "key", key, "error", err)
		}
	}

	e.log().Debug("Revoked all roles for user", "user_id", userID)
	return nil
}

// RoleHasPermission checks if a role has a specific permission granted.
// For virtual roles, permissions are still checked in KV.
// Note: Admin roles are NOT special-cased - they must have permissions explicitly granted.
func (e *Engine) RoleHasPermission(ctx context.Context, roleName, verb, objectType, objectId string) (bool, error) {
	// Check permission in KV (works for both virtual and stored roles)
	_, err := e.kv.Get(ctx, e.allowKey(roleName, verb, objectType, objectId))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return false, nil
	}
	return false, fmt.Errorf("failed to check permission: %w", err)
}

// RoleHasPermissionDenial checks if a role has a specific permission denied.
// Note: Admin roles are NOT special-cased - they can have denials like any other role.
func (e *Engine) RoleHasPermissionDenial(ctx context.Context, roleName, verb, objectType, objectId string) (bool, error) {
	// Check denial in KV
	_, err := e.kv.Get(ctx, e.denyKey(roleName, verb, objectType, objectId))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return false, nil
	}
	return false, fmt.Errorf("failed to check permission denial: %w", err)
}

// Note: ParseMemberKey, ParseAllowKey, and ParseDenyKey are defined in keys.go

// ============================================================================
// Role Hierarchy Operations
// ============================================================================

// GetNextAvailablePosition returns the next available position for a new custom role.
// It finds the highest position among non-everyone roles and adds 1.
// Returns 1 if no custom roles exist (admin is at 0).
func (e *Engine) GetNextAvailablePosition(ctx context.Context) (int32, error) {
	roles, err := e.ListRoles(ctx)
	if err != nil {
		return 1, err
	}

	var maxPos int32 = 0
	for _, role := range roles {
		// Skip virtual roles (they're always at fixed max positions)
		if role.Position == PositionEveryone {
			continue
		}
		if role.Position > maxPos {
			maxPos = role.Position
		}
	}

	return maxPos + 1, nil
}

// UpdateRolePosition updates only the position of a role.
// Returns ErrRoleNotFound if the role doesn't exist.
func (e *Engine) UpdateRolePosition(ctx context.Context, name string, position int32) (*corev1.Role, error) {
	existing, err := e.GetRole(ctx, name)
	if err != nil {
		return nil, err
	}

	role := &corev1.Role{
		Name:        existing.Name,
		DisplayName: existing.DisplayName,
		Description: existing.Description,
		Position:    position,
	}

	data, err := proto.Marshal(role)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal role: %w", err)
	}

	_, err = e.kv.Put(ctx, RoleKey(name), data)
	if err != nil {
		return nil, fmt.Errorf("failed to update role position: %w", err)
	}

	e.log().Debug("Updated role position", "name", name, "position", position)
	return role, nil
}

// ReorderRoles sets positions for custom roles based on the provided order.
// System roles maintain fixed positions and are skipped.
// Positions are assigned starting from a configured base position.
// Returns all roles sorted by position.
func (e *Engine) ReorderRoles(ctx context.Context, orderedNames []string) ([]*corev1.Role, error) {
	// Validate that all provided names are custom roles (not system roles)
	for _, name := range orderedNames {
		if e.isSystemRole(name) {
			return nil, fmt.Errorf("cannot reorder system role: %s", name)
		}
		// Verify role exists
		if _, err := e.GetRole(ctx, name); err != nil {
			return nil, fmt.Errorf("role %s: %w", name, err)
		}
	}

	// Assign positions based on order (starting from 1, since admin is 0)
	for i, name := range orderedNames {
		position := int32(i + 1) // 1, 2, 3, ...
		if _, err := e.UpdateRolePosition(ctx, name, position); err != nil {
			return nil, fmt.Errorf("failed to update position for %s: %w", name, err)
		}
	}

	e.log().Info("Reordered roles", "order", orderedNames)

	// Return all roles sorted by position
	return e.ListRoles(ctx)
}

// GetUserHighestPosition returns the lowest position number among the user's roles.
// Lower position = higher rank, so this returns the user's "power level".
// Returns PositionEveryone if the user has no assigned roles.
func (e *Engine) GetUserHighestPosition(ctx context.Context, userID string) (int32, error) {
	roleNames, err := e.GetUserRoles(ctx, userID)
	if err != nil {
		return PositionEveryone, err
	}

	if len(roleNames) == 0 {
		return PositionEveryone, nil
	}

	minPos := PositionEveryone
	for _, roleName := range roleNames {
		// GetRole handles both virtual and KV-stored roles
		role, err := e.GetRole(ctx, roleName)
		if err != nil {
			// Skip roles that no longer exist
			continue
		}
		if role.Position < minPos {
			minPos = role.Position
		}
	}

	return minPos, nil
}

// CanUserManageRole checks if a user can assign a role based on role hierarchy.
// This checks if the actor outranks the ROLE being assigned/modified.
// Owners (position 0) can manage any role including owner.
// Other users can only manage roles with position > their own highest position.
//
// NOTE: For revoking roles, also check CanUserManageUser to ensure the actor
// outranks the TARGET USER. This prevents peer-level demotion (e.g., Admin A
// demoting Admin B).
func (e *Engine) CanUserManageRole(ctx context.Context, userID string, rolePosition int32) (bool, error) {
	userPos, err := e.GetUserHighestPosition(ctx, userID)
	if err != nil {
		return false, err
	}
	// Owners (position 0) can manage any role including owner
	if userPos == PositionOwner {
		return true, nil
	}
	// Others can only manage roles strictly below them
	return userPos < rolePosition, nil
}

// CanUserManageUser checks if actor can manage target based on role hierarchy.
// Returns true if actor's highest role position < target's highest role position.
func (e *Engine) CanUserManageUser(ctx context.Context, actorID, targetID string) (bool, error) {
	actorPos, err := e.GetUserHighestPosition(ctx, actorID)
	if err != nil {
		return false, err
	}
	targetPos, err := e.GetUserHighestPosition(ctx, targetID)
	if err != nil {
		return false, err
	}
	return actorPos < targetPos, nil
}

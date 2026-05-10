package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/core/rbac"
)

// ============================================================================
// Permission Operations
// ============================================================================
//
// These functions manage permissions using the unified hierarchical model.
//
// Key patterns (in the SERVER_RBAC bucket):
//   - allow.{subject}.{verb}.{objectType}.{objectId}  - Permission grant
//   - deny.{subject}.{verb}.{objectType}.{objectId}   - Permission denial
//
// Subject disambiguation via naming conventions:
//   - Role: lowercase word (e.g., "owner", "admin", "moderator")
//   - User ID: starts with "U" (e.g., "U9mP2qR5tYz3wK")
//
// ObjectId is "any" for the role's server-level default and a specific room
// ID for room-level overrides.

// ============================================================================
// Instance-Level Operations
// ============================================================================

// GrantInstancePermission grants a permission to a role's server-level
// default. Accepts any valid permission — server- and space-scope grants
// share the same KV row post-#330. Use GrantRoomRolePermission for
// per-room overrides.
// Uses key format: allow.{roleName}.{verb}.{objectType}.any
func (c *ChattoCore) GrantInstancePermission(ctx context.Context, roleName string, perm Permission) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}

	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}

	kv := c.storage.serverRBACEngine.KV()
	key := rbac.AllowKey(roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)

	if _, err := kv.Put(ctx, key, []byte("1")); err != nil {
		return fmt.Errorf("failed to grant permission: %w", err)
	}

	// Remove any denial for this permission
	denyKey := rbac.DenyKey(roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)
	_ = kv.Delete(ctx, denyKey) // Ignore not found error

	c.logger.Debug("Granted unified instance role permission", "role", roleName, "permission", perm)
	return nil
}

// DenyInstancePermission denies a permission at a role's server-level
// default. See GrantInstancePermission for the scope rationale.
// Uses key format: deny.{roleName}.{verb}.{objectType}.any
func (c *ChattoCore) DenyInstancePermission(ctx context.Context, roleName string, perm Permission) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}

	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}

	kv := c.storage.serverRBACEngine.KV()
	key := rbac.DenyKey(roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)

	if _, err := kv.Put(ctx, key, []byte("1")); err != nil {
		return fmt.Errorf("failed to deny permission: %w", err)
	}

	// Remove any grant for this permission
	grantKey := rbac.AllowKey(roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)
	_ = kv.Delete(ctx, grantKey) // Ignore not found error

	c.logger.Debug("Denied unified instance role permission", "role", roleName, "permission", perm)
	return nil
}

// ClearInstancePermissionState clears both grant and denial for a permission.
func (c *ChattoCore) ClearInstancePermissionState(ctx context.Context, roleName string, perm Permission) error {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}

	kv := c.storage.serverRBACEngine.KV()

	grantKey := rbac.AllowKey(roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)
	denyKey := rbac.DenyKey(roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)

	if err := kv.Delete(ctx, grantKey); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to clear grant: %w", err)
	}
	if err := kv.Delete(ctx, denyKey); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to clear denial: %w", err)
	}

	c.logger.Debug("Cleared unified instance role permission", "role", roleName, "permission", perm)
	return nil
}

// ============================================================================
// Room-Level Operations
// ============================================================================

// GrantRoomRolePermission grants a permission to a role at the room level.
// Uses key format: allow.{roleName}.{verb}.{objectType}.{roomID}
func (c *ChattoCore) grantRoomRolePermissionInternal(ctx context.Context, spaceID, roomID, roleName string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at room scope", perm)
	}

	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}

	kv := c.storage.serverRBACKV

	key := rbac.AllowKey(roleName, parts.Verb, parts.ObjectType, roomID)

	if _, err := kv.Put(ctx, key, []byte("1")); err != nil {
		return fmt.Errorf("failed to grant permission: %w", err)
	}

	// Remove any denial for this permission
	denyKey := rbac.DenyKey(roleName, parts.Verb, parts.ObjectType, roomID)
	_ = kv.Delete(ctx, denyKey) // Ignore not found error

	c.logger.Debug("Granted room role permission",
		"space", spaceID, "room", roomID, "role", roleName, "permission", perm)
	return nil
}

// DenyRoomRolePermission denies a permission for a role at the room level.
// Uses key format: deny.{roleName}.{verb}.{objectType}.{roomID}
func (c *ChattoCore) denyRoomRolePermissionInternal(ctx context.Context, spaceID, roomID, roleName string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at room scope", perm)
	}

	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}

	kv := c.storage.serverRBACKV

	key := rbac.DenyKey(roleName, parts.Verb, parts.ObjectType, roomID)

	if _, err := kv.Put(ctx, key, []byte("1")); err != nil {
		return fmt.Errorf("failed to deny permission: %w", err)
	}

	// Remove any grant for this permission
	grantKey := rbac.AllowKey(roleName, parts.Verb, parts.ObjectType, roomID)
	_ = kv.Delete(ctx, grantKey) // Ignore not found error

	c.logger.Debug("Denied room role permission",
		"space", spaceID, "room", roomID, "role", roleName, "permission", perm)
	return nil
}

// ClearRoomRolePermission clears both grant and denial for a permission at room level.
func (c *ChattoCore) clearRoomRolePermissionInternal(ctx context.Context, spaceID, roomID, roleName string, perm Permission) error {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}

	kv := c.storage.serverRBACKV

	grantKey := rbac.AllowKey(roleName, parts.Verb, parts.ObjectType, roomID)
	denyKey := rbac.DenyKey(roleName, parts.Verb, parts.ObjectType, roomID)

	if err := kv.Delete(ctx, grantKey); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to clear grant: %w", err)
	}
	if err := kv.Delete(ctx, denyKey); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to clear denial: %w", err)
	}

	c.logger.Debug("Cleared room role permission",
		"space", spaceID, "room", roomID, "role", roleName, "permission", perm)
	return nil
}

// ============================================================================
// Announcements Room Setup
// ============================================================================

// AnnouncementsRoomName is the canonical name for announcement-only rooms.
const AnnouncementsRoomName = "announcements"

// SetupAnnouncementsRoomPermissions configures an announcements room so that only
// owner, admin, and moderator roles can post new root messages.
// Everyone else can read and post in threads, but cannot start new conversations.
// This is idempotent and safe to call multiple times.
func (c *ChattoCore) SetupAnnouncementsRoomPermissions(ctx context.Context, spaceID, roomID string) error {
	// Deny message.post to everyone at room level
	if err := c.denyRoomRolePermissionInternal(ctx, spaceID, roomID, RoleEveryone, PermMessagePost); err != nil {
		return fmt.Errorf("failed to deny %s for everyone: %w", PermMessagePost, err)
	}

	// Grant message.post to owner, admin, and moderator at room level
	for _, roleName := range []string{RoleOwner, RoleAdmin, RoleModerator} {
		if err := c.grantRoomRolePermissionInternal(ctx, spaceID, roomID, roleName, PermMessagePost); err != nil {
			return fmt.Errorf("failed to grant %s for %s: %w", PermMessagePost, roleName, err)
		}
	}

	// message.post-in-thread is left untouched — everyone can reply in threads
	// via default space permissions.

	c.logger.Debug("Set up announcements room permissions", "space", spaceID, "room", roomID)
	return nil
}

// ============================================================================
// Initialization Helpers
// ============================================================================

// InitSpaceDefaults sets up the default space-scoped permission grants.
// Post-#330 these land in the same SERVER_RBAC bucket as instance grants;
// idempotent re-grants are harmless.
func (c *ChattoCore) InitSpaceDefaults(ctx context.Context, spaceID string) error {
	for _, perm := range PermissionsForScope(ScopeSpace) {
		if err := c.GrantInstancePermission(ctx, RoleOwner, perm.Permission); err != nil {
			return fmt.Errorf("failed to grant owner permission %s: %w", perm.Permission, err)
		}
	}

	for _, perm := range DefaultSpaceAdminPermissions() {
		if err := c.GrantInstancePermission(ctx, RoleAdmin, perm); err != nil {
			return fmt.Errorf("failed to grant admin permission %s: %w", perm, err)
		}
	}

	for _, perm := range DefaultSpaceModeratorPermissions() {
		if err := c.GrantInstancePermission(ctx, RoleModerator, perm); err != nil {
			return fmt.Errorf("failed to grant moderator permission %s: %w", perm, err)
		}
	}

	for _, perm := range DefaultSpaceEveryonePermissions() {
		if err := c.GrantInstancePermission(ctx, RoleEveryone, perm); err != nil {
			return fmt.Errorf("failed to grant everyone permission %s: %w", perm, err)
		}
	}

	c.logger.Info("Initialized unified space defaults", "space_id", spaceID)
	return nil
}

// InitInstanceDefaults sets up the default instance-level permissions using keys.
// This should be called during instance RBAC initialization.
func (c *ChattoCore) InitInstanceDefaults(ctx context.Context) error {
	// Grant all permissions to owner role
	for _, perm := range PermissionsForScope(ScopeServer) {
		if err := c.GrantInstancePermission(ctx, RoleOwner, perm.Permission); err != nil {
			return fmt.Errorf("failed to grant owner permission %s: %w", perm.Permission, err)
		}
	}

	// Grant default admin permissions
	for _, perm := range DefaultInstanceAdminPermissions() {
		if err := c.GrantInstancePermission(ctx, RoleAdmin, perm); err != nil {
			return fmt.Errorf("failed to grant admin permission %s: %w", perm, err)
		}
	}

	// Grant moderator permissions (subset - no admin.* permissions)
	for _, perm := range DefaultInstanceModeratorPermissions() {
		if err := c.GrantInstancePermission(ctx, RoleModerator, perm); err != nil {
			return fmt.Errorf("failed to grant moderator permission %s: %w", perm, err)
		}
	}

	// Grant default everyone permissions
	for _, perm := range DefaultInstanceEveryonePermissions() {
		if err := c.GrantInstancePermission(ctx, RoleEveryone, perm); err != nil {
			return fmt.Errorf("failed to grant everyone permission %s: %w", perm, err)
		}
	}

	c.logger.Info("Initialized unified instance defaults")
	return nil
}

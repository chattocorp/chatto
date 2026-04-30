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
// Key patterns (same in both INSTANCE_RBAC and SPACE_RBAC buckets):
//   - allow.{subject}.{object}.{perm}  - Permission grant
//   - deny.{subject}.{object}.{perm}   - Permission denial
//
// Subject disambiguation via naming conventions:
//   - Instance role: "instance-" prefix (e.g., "instance-admin")
//   - Space role: lowercase word (e.g., "admin", "moderator")
//   - User ID: starts with "U" (e.g., "U9mP2qR5tYz3wK")
//
// Object values:
//   - "instance" (literal) - for instance-level permissions
//   - Space ID - for space-level permissions
//   - Room ID - for room-level permissions

// ============================================================================
// Instance-Level Operations
// ============================================================================

// GrantInstanceRolePermission grants a permission to an instance role.
// Uses key format: allow.{roleName}.{verb}.{objectType}.any.
//
// Every defined permission can be configured at instance scope — an
// instance-level grant propagates down through the harmonized resolver's
// parent-tier fallback to every space and room (subject to membership).
func (c *ChattoCore) GrantInstanceRolePermission(ctx context.Context, roleName string, perm Permission) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}

	kv := c.instanceRBACEngine.KV()
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

// DenyInstanceRolePermission denies a permission for an instance role.
// Uses key format: deny.{roleName}.{verb}.{objectType}.any.
func (c *ChattoCore) DenyInstanceRolePermission(ctx context.Context, roleName string, perm Permission) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}

	kv := c.instanceRBACEngine.KV()
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

// ClearInstanceRolePermission clears both grant and denial for a permission.
func (c *ChattoCore) ClearInstanceRolePermission(ctx context.Context, roleName string, perm Permission) error {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}

	kv := c.instanceRBACEngine.KV()

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
// Space-Level Operations
// ============================================================================

// GrantSpaceRolePermission grants a permission to a role at the space level.
// The roleName can be a space role (e.g., "admin") or an instance role (e.g., "instance-admin").
// Uses key format: allow.{roleName}.{verb}.{objectType}.any
func (c *ChattoCore) GrantSpaceRolePermission(ctx context.Context, spaceID, roleName string, perm Permission) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}

	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}

	kv, err := c.getSpaceRBACKV(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("failed to get space RBAC KV: %w", err)
	}

	key := rbac.AllowKey(roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)

	if _, err := kv.Put(ctx, key, []byte("1")); err != nil {
		return fmt.Errorf("failed to grant permission: %w", err)
	}

	// Remove any denial for this permission
	denyKey := rbac.DenyKey(roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)
	_ = kv.Delete(ctx, denyKey) // Ignore not found error

	c.logger.Debug("Granted space role permission",
		"space", spaceID, "role", roleName, "permission", perm)
	return nil
}

// DenySpaceRolePermission denies a permission for a role at the space level.
// Uses key format: deny.{roleName}.{verb}.{objectType}.any
func (c *ChattoCore) DenySpaceRolePermission(ctx context.Context, spaceID, roleName string, perm Permission) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}

	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}

	kv, err := c.getSpaceRBACKV(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("failed to get space RBAC KV: %w", err)
	}

	key := rbac.DenyKey(roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)

	if _, err := kv.Put(ctx, key, []byte("1")); err != nil {
		return fmt.Errorf("failed to deny permission: %w", err)
	}

	// Remove any grant for this permission
	grantKey := rbac.AllowKey(roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)
	_ = kv.Delete(ctx, grantKey) // Ignore not found error

	c.logger.Debug("Denied space role permission",
		"space", spaceID, "role", roleName, "permission", perm)
	return nil
}

// ClearSpaceRolePermission clears both grant and denial for a permission at space level.
func (c *ChattoCore) ClearSpaceRolePermission(ctx context.Context, spaceID, roleName string, perm Permission) error {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}

	kv, err := c.getSpaceRBACKV(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("failed to get space RBAC KV: %w", err)
	}

	grantKey := rbac.AllowKey(roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)
	denyKey := rbac.DenyKey(roleName, parts.Verb, parts.ObjectType, rbac.ObjectIdAny)

	if err := kv.Delete(ctx, grantKey); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to clear grant: %w", err)
	}
	if err := kv.Delete(ctx, denyKey); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to clear denial: %w", err)
	}

	c.logger.Debug("Cleared space role permission",
		"space", spaceID, "role", roleName, "permission", perm)
	return nil
}

// ============================================================================
// Room-Level Operations
// ============================================================================

// GrantRoomRolePermission grants a permission to a role at the room level.
// The roleName can be a space role (e.g., "admin") or an instance role (e.g., "instance-admin").
// Uses key format: allow.{roleName}.{verb}.{objectType}.{roomID}
func (c *ChattoCore) grantRoomRolePermissionInternal(ctx context.Context, spaceID, roomID, roleName string, perm Permission) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}

	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}

	kv, err := c.getSpaceRBACKV(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("failed to get space RBAC KV: %w", err)
	}

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
	if err := ValidatePermission(perm); err != nil {
		return err
	}

	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}

	kv, err := c.getSpaceRBACKV(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("failed to get space RBAC KV: %w", err)
	}

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

	kv, err := c.getSpaceRBACKV(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("failed to get space RBAC KV: %w", err)
	}

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
	if err := c.denyRoomRolePermissionInternal(ctx, spaceID, roomID, SpaceRoleEveryone, PermMessagePost); err != nil {
		return fmt.Errorf("failed to deny %s for everyone: %w", PermMessagePost, err)
	}

	// Grant message.post to owner, admin, and moderator at room level
	for _, roleName := range []string{SpaceRoleOwner, SpaceRoleAdmin, SpaceRoleModerator} {
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

// InitSpaceDefaults sets up the default permissions for a space using keys.
// This should be called when a space is created.
func (c *ChattoCore) InitSpaceDefaults(ctx context.Context, spaceID string) error {
	// Grant all space permissions to owner role
	for _, perm := range PermissionsForScope(ScopeSpace) {
		if err := c.GrantSpaceRolePermission(ctx, spaceID, SpaceRoleOwner, perm.Permission); err != nil {
			return fmt.Errorf("failed to grant owner permission %s: %w", perm.Permission, err)
		}
	}

	// Grant default admin permissions
	for _, perm := range DefaultSpaceAdminPermissions() {
		if err := c.GrantSpaceRolePermission(ctx, spaceID, SpaceRoleAdmin, perm); err != nil {
			return fmt.Errorf("failed to grant admin permission %s: %w", perm, err)
		}
	}

	// Grant default moderator permissions
	for _, perm := range DefaultSpaceModeratorPermissions() {
		if err := c.GrantSpaceRolePermission(ctx, spaceID, SpaceRoleModerator, perm); err != nil {
			return fmt.Errorf("failed to grant moderator permission %s: %w", perm, err)
		}
	}

	// Grant default everyone permissions using keys
	for _, perm := range DefaultSpaceEveryonePermissions() {
		if err := c.GrantSpaceRolePermission(ctx, spaceID, SpaceRoleEveryone, perm); err != nil {
			return fmt.Errorf("failed to grant everyone permission %s: %w", perm, err)
		}
	}

	c.logger.Info("Initialized unified space defaults", "space_id", spaceID)
	return nil
}

// InitInstanceDefaults sets up the default instance-level permissions.
//
//   - instance-owner and instance-admin get every defined permission.
//   - instance-moderator gets the read-only admin set.
//   - instance-everyone gets the universal user-behavior floor (post,
//     react, leave, etc.) which propagates into every space the user
//     joins. Per-space overrides are configured by denying on
//     instance-everyone at space scope.
func (c *ChattoCore) InitInstanceDefaults(ctx context.Context) error {
	for _, perm := range DefaultInstanceFullAllows() {
		if err := c.GrantInstanceRolePermission(ctx, InstRoleOwner, perm); err != nil {
			return fmt.Errorf("failed to grant owner permission %s: %w", perm, err)
		}
		if err := c.GrantInstanceRolePermission(ctx, InstRoleAdmin, perm); err != nil {
			return fmt.Errorf("failed to grant admin permission %s: %w", perm, err)
		}
	}

	for _, perm := range DefaultInstanceModeratorPermissions() {
		if err := c.GrantInstanceRolePermission(ctx, InstRoleModerator, perm); err != nil {
			return fmt.Errorf("failed to grant moderator permission %s: %w", perm, err)
		}
	}

	for _, perm := range DefaultInstanceEveryonePermissions() {
		if err := c.GrantInstanceRolePermission(ctx, InstRoleEveryone, perm); err != nil {
			return fmt.Errorf("failed to grant everyone permission %s: %w", perm, err)
		}
	}

	c.logger.Info("Initialized unified instance defaults")
	return nil
}

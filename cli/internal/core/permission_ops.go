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
// share the same KV row post-#330. Use GrantRoomPermission for
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
// Per-User Operations
// ============================================================================
//
// User-level grants/denies sit alongside role-based grants in the same KV.
// The walker consults user-level decisions FIRST (before any role), so an
// explicit user-deny blocks the action even for owners and an explicit
// user-grant allows it even when no role grants it. Useful for one-off
// moderation (suspend a single user) and ad-hoc privileges (this single
// user can administer room X without needing a new role).

// GrantUserPermission grants a permission directly to a user at server scope.
// Beats any role-level decision when evaluated by the resolver.
func (c *ChattoCore) GrantUserPermission(ctx context.Context, userID string, perm Permission) error {
	return c.putPermissionKey(ctx, userID, perm, rbac.ObjectIdAny, true)
}

// DenyUserPermission denies a permission directly to a user at server scope.
// Beats any role-level grant — user-level decisions are checked before
// the role-hierarchy walk.
func (c *ChattoCore) DenyUserPermission(ctx context.Context, userID string, perm Permission) error {
	return c.putPermissionKey(ctx, userID, perm, rbac.ObjectIdAny, false)
}

// ClearUserPermissionState clears both the grant and denial for a user-level
// permission at server scope.
func (c *ChattoCore) ClearUserPermissionState(ctx context.Context, userID string, perm Permission) error {
	return c.clearPermissionKey(ctx, userID, perm, rbac.ObjectIdAny)
}

// GrantUserRoomPermission grants a permission directly to a user for a
// specific room. Beats any role-level decision at the same scope.
func (c *ChattoCore) GrantUserRoomPermission(ctx context.Context, roomID, userID string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at room scope", perm)
	}
	return c.putPermissionKey(ctx, userID, perm, roomID, true)
}

// DenyUserRoomPermission denies a permission directly to a user for a
// specific room.
func (c *ChattoCore) DenyUserRoomPermission(ctx context.Context, roomID, userID string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at room scope", perm)
	}
	return c.putPermissionKey(ctx, userID, perm, roomID, false)
}

// ClearUserRoomPermissionState clears both the grant and denial for a
// user-level permission for a specific room.
func (c *ChattoCore) ClearUserRoomPermissionState(ctx context.Context, roomID, userID string, perm Permission) error {
	return c.clearPermissionKey(ctx, userID, perm, roomID)
}

// putPermissionKey is the shared implementation for grant/deny ops. The
// `allow` flag selects which key to write (allow vs deny) and which to
// delete (the opposite, to keep the pair mutually exclusive).
func (c *ChattoCore) putPermissionKey(ctx context.Context, subject string, perm Permission, objectID string, allow bool) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}
	kv := c.storage.serverRBACEngine.KV()
	allowKey := rbac.AllowKey(subject, parts.Verb, parts.ObjectType, objectID)
	denyKey := rbac.DenyKey(subject, parts.Verb, parts.ObjectType, objectID)

	writeKey, deleteKey := allowKey, denyKey
	verb := "Granted"
	if !allow {
		writeKey, deleteKey = denyKey, allowKey
		verb = "Denied"
	}
	if _, err := kv.Put(ctx, writeKey, []byte("1")); err != nil {
		return fmt.Errorf("failed to write permission key: %w", err)
	}
	_ = kv.Delete(ctx, deleteKey)
	c.logger.Debug(verb+" permission", "subject", subject, "permission", perm, "objectID", objectID)
	return nil
}

// clearPermissionKey deletes both the allow and deny keys for a (subject,
// permission, objectID) tuple. Not finding either is not an error.
func (c *ChattoCore) clearPermissionKey(ctx context.Context, subject string, perm Permission, objectID string) error {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}
	kv := c.storage.serverRBACEngine.KV()
	allowKey := rbac.AllowKey(subject, parts.Verb, parts.ObjectType, objectID)
	denyKey := rbac.DenyKey(subject, parts.Verb, parts.ObjectType, objectID)
	if err := kv.Delete(ctx, allowKey); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to clear grant: %w", err)
	}
	if err := kv.Delete(ctx, denyKey); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to clear denial: %w", err)
	}
	c.logger.Debug("Cleared permission", "subject", subject, "permission", perm, "objectID", objectID)
	return nil
}

// ============================================================================
// Room-Level Operations
// ============================================================================

// GrantRoomPermission grants a permission to a role for a specific room.
// Uses key format: allow.{roleName}.{verb}.{objectType}.{roomID}
func (c *ChattoCore) GrantRoomPermission(ctx context.Context, roomID, roleName string, perm Permission) error {
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

	denyKey := rbac.DenyKey(roleName, parts.Verb, parts.ObjectType, roomID)
	_ = kv.Delete(ctx, denyKey)

	c.logger.Debug("Granted room role permission", "room", roomID, "role", roleName, "permission", perm)
	return nil
}

// DenyRoomPermission denies a permission for a role at a specific room.
// Uses key format: deny.{roleName}.{verb}.{objectType}.{roomID}
func (c *ChattoCore) DenyRoomPermission(ctx context.Context, roomID, roleName string, perm Permission) error {
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

	grantKey := rbac.AllowKey(roleName, parts.Verb, parts.ObjectType, roomID)
	_ = kv.Delete(ctx, grantKey)

	c.logger.Debug("Denied room role permission", "room", roomID, "role", roleName, "permission", perm)
	return nil
}

// ClearRoomPermissionState removes both grant and denial for a permission at room level.
func (c *ChattoCore) ClearRoomPermissionState(ctx context.Context, roomID, roleName string, perm Permission) error {
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

	c.logger.Debug("Cleared room role permission", "room", roomID, "role", roleName, "permission", perm)
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
func (c *ChattoCore) SetupAnnouncementsRoomPermissions(ctx context.Context, roomID string) error {
	if err := c.DenyRoomPermission(ctx, roomID, RoleEveryone, PermMessagePost); err != nil {
		return fmt.Errorf("failed to deny %s for everyone: %w", PermMessagePost, err)
	}

	for _, roleName := range []string{RoleOwner, RoleAdmin, RoleModerator} {
		if err := c.GrantRoomPermission(ctx, roomID, roleName, PermMessagePost); err != nil {
			return fmt.Errorf("failed to grant %s for %s: %w", PermMessagePost, roleName, err)
		}
	}

	// message.post-in-thread is left untouched — everyone can reply in threads
	// via default space permissions.

	c.logger.Debug("Set up announcements room permissions", "room", roomID)
	return nil
}

// ============================================================================
// Initialization Helpers
// ============================================================================

// InitDefaultPermissions seeds the system roles with their default permission
// grants in SERVER_RBAC. Idempotent — safe to call on every boot.
//
// Owner and Admin receive the same enumerated permission set
// (`DefaultOwnerPermissions` / `DefaultAdminPermissions`). They are
// distinguished by rank, not capabilities. Moderator gets
// `DefaultModeratorPermissions`, Everyone gets `DefaultEveryonePermissions`.
func (c *ChattoCore) InitDefaultPermissions(ctx context.Context) error {
	for _, perm := range DefaultOwnerPermissions() {
		if err := c.GrantInstancePermission(ctx, RoleOwner, perm); err != nil {
			return fmt.Errorf("failed to grant owner permission %s: %w", perm, err)
		}
	}

	for _, perm := range DefaultAdminPermissions() {
		if err := c.GrantInstancePermission(ctx, RoleAdmin, perm); err != nil {
			return fmt.Errorf("failed to grant admin permission %s: %w", perm, err)
		}
	}

	for _, perm := range DefaultModeratorPermissions() {
		if err := c.GrantInstancePermission(ctx, RoleModerator, perm); err != nil {
			return fmt.Errorf("failed to grant moderator permission %s: %w", perm, err)
		}
	}

	for _, perm := range DefaultEveryonePermissions() {
		if err := c.GrantInstancePermission(ctx, RoleEveryone, perm); err != nil {
			return fmt.Errorf("failed to grant everyone permission %s: %w", perm, err)
		}
	}

	c.logger.Info("Initialized default permissions")
	return nil
}

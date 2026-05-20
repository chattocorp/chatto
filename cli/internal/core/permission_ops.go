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

// GrantServerPermission grants a permission to a role's server-level
// default. Accepts any valid permission — server- and space-scope grants
// share the same KV row post-#330. Use GrantRoomPermission for
// per-room overrides.
// Uses key format: allow.{roleName}.{verb}.{objectType}.any
func (c *ChattoCore) GrantServerPermission(ctx context.Context, roleName string, perm Permission) error {
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

	c.logger.Debug("Granted unified role permission", "role", roleName, "permission", perm)
	return nil
}

// DenyServerPermission denies a permission at a role's server-level
// default. See GrantServerPermission for the scope rationale.
// Uses key format: deny.{roleName}.{verb}.{objectType}.any
func (c *ChattoCore) DenyServerPermission(ctx context.Context, roleName string, perm Permission) error {
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

	c.logger.Debug("Denied unified role permission", "role", roleName, "permission", perm)
	return nil
}

// ClearServerPermissionState clears both grant and denial for a permission.
func (c *ChattoCore) ClearServerPermissionState(ctx context.Context, roleName string, perm Permission) error {
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

	c.logger.Debug("Cleared unified role permission", "role", roleName, "permission", perm)
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
// Uses legacy server-scope key format: allow.{userID}.{verb}.{type}.any
func (c *ChattoCore) GrantUserPermission(ctx context.Context, userID string, perm Permission) error {
	return c.writePermissionKey(ctx, userID, perm, true,
		rbac.AllowKey(userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType, rbac.ObjectIdAny),
		rbac.DenyKey(userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType, rbac.ObjectIdAny))
}

// DenyUserPermission denies a permission directly to a user at server scope.
// Beats any role-level grant — user-level decisions are checked before
// the role-hierarchy walk.
func (c *ChattoCore) DenyUserPermission(ctx context.Context, userID string, perm Permission) error {
	return c.writePermissionKey(ctx, userID, perm, false,
		rbac.AllowKey(userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType, rbac.ObjectIdAny),
		rbac.DenyKey(userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType, rbac.ObjectIdAny))
}

// ClearUserPermissionState clears both the grant and denial for a user-level
// permission at server scope.
func (c *ChattoCore) ClearUserPermissionState(ctx context.Context, userID string, perm Permission) error {
	return c.clearKeyPair(ctx, perm,
		rbac.AllowKey(userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType, rbac.ObjectIdAny),
		rbac.DenyKey(userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType, rbac.ObjectIdAny))
}

// GrantUserRoomPermission grants a permission directly to a user for a
// specific room. Beats any role-level decision at the same scope.
// Uses ADR-031 key format: room_allow.{roomID}.{userID}.{verb}.{type}
func (c *ChattoCore) GrantUserRoomPermission(ctx context.Context, roomID, userID string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at room scope", perm)
	}
	return c.writePermissionKey(ctx, userID, perm, true,
		rbac.RoomAllowKey(roomID, userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType),
		rbac.RoomDenyKey(roomID, userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType))
}

// DenyUserRoomPermission denies a permission directly to a user for a
// specific room. Uses ADR-031 key format: room_deny.{roomID}.{userID}.{verb}.{type}
func (c *ChattoCore) DenyUserRoomPermission(ctx context.Context, roomID, userID string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at room scope", perm)
	}
	return c.writePermissionKey(ctx, userID, perm, false,
		rbac.RoomAllowKey(roomID, userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType),
		rbac.RoomDenyKey(roomID, userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType))
}

// ClearUserRoomPermissionState clears both the grant and denial for a
// user-level permission for a specific room.
func (c *ChattoCore) ClearUserRoomPermissionState(ctx context.Context, roomID, userID string, perm Permission) error {
	return c.clearKeyPair(ctx, perm,
		rbac.RoomAllowKey(roomID, userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType),
		rbac.RoomDenyKey(roomID, userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType))
}

// GrantUserGroupPermission grants a permission directly to a user at the
// scope of a specific room group. Beats any role-level decision at that
// group's scope. Uses ADR-031 key format:
// group_allow.{groupID}.{userID}.{verb}.{type}
func (c *ChattoCore) GrantUserGroupPermission(ctx context.Context, groupID, userID string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeGroup) && !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at group scope", perm)
	}
	return c.writePermissionKey(ctx, userID, perm, true,
		rbac.GroupAllowKey(groupID, userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType),
		rbac.GroupDenyKey(groupID, userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType))
}

// DenyUserGroupPermission denies a permission directly to a user at the
// scope of a specific room group.
func (c *ChattoCore) DenyUserGroupPermission(ctx context.Context, groupID, userID string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeGroup) && !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at group scope", perm)
	}
	return c.writePermissionKey(ctx, userID, perm, false,
		rbac.GroupAllowKey(groupID, userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType),
		rbac.GroupDenyKey(groupID, userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType))
}

// ClearUserGroupPermissionState clears both the grant and denial for a
// user-level permission at a specific room group's scope.
func (c *ChattoCore) ClearUserGroupPermissionState(ctx context.Context, groupID, userID string, perm Permission) error {
	return c.clearKeyPair(ctx, perm,
		rbac.GroupAllowKey(groupID, userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType),
		rbac.GroupDenyKey(groupID, userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType))
}

// GetUserExplicitServerOverride returns the user's explicit user-level
// allow/deny at server scope for the given permission, or DecisionNone
// when there's no user-level override.
func (c *ChattoCore) GetUserExplicitServerOverride(ctx context.Context, userID string, perm Permission) (DecisionKind, error) {
	return c.probeUserExplicit(ctx,
		rbac.AllowKey(userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType, rbac.ObjectIdAny),
		rbac.DenyKey(userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType, rbac.ObjectIdAny))
}

// GetUserExplicitGroupOverride returns the user's explicit user-level
// allow/deny at the given room group's scope, or DecisionNone.
func (c *ChattoCore) GetUserExplicitGroupOverride(ctx context.Context, groupID, userID string, perm Permission) (DecisionKind, error) {
	return c.probeUserExplicit(ctx,
		rbac.GroupAllowKey(groupID, userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType),
		rbac.GroupDenyKey(groupID, userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType))
}

// GetUserExplicitRoomOverride returns the user's explicit user-level
// allow/deny at the given room's scope, or DecisionNone.
func (c *ChattoCore) GetUserExplicitRoomOverride(ctx context.Context, roomID, userID string, perm Permission) (DecisionKind, error) {
	return c.probeUserExplicit(ctx,
		rbac.RoomAllowKey(roomID, userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType),
		rbac.RoomDenyKey(roomID, userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType))
}

// probeUserExplicit checks an (allow, deny) key pair on SERVER_RBAC and
// returns DecisionAllow / DecisionDeny / DecisionNone. The allow key is
// preferred when both are somehow present — the write path keeps them
// mutually exclusive, so that should never happen, but choosing
// deterministically here keeps the read side robust.
func (c *ChattoCore) probeUserExplicit(ctx context.Context, allowKey, denyKey string) (DecisionKind, error) {
	kv := c.storage.serverRBACEngine.KV()
	if _, err := kv.Get(ctx, allowKey); err == nil {
		return DecisionAllow, nil
	} else if !errors.Is(err, jetstream.ErrKeyNotFound) {
		return DecisionNone, fmt.Errorf("probe allow %s: %w", allowKey, err)
	}
	if _, err := kv.Get(ctx, denyKey); err == nil {
		return DecisionDeny, nil
	} else if !errors.Is(err, jetstream.ErrKeyNotFound) {
		return DecisionNone, fmt.Errorf("probe deny %s: %w", denyKey, err)
	}
	return DecisionNone, nil
}

// writePermissionKey writes one of the (allow, deny) keys and deletes the
// other to keep the pair mutually exclusive. The `allow` flag selects which.
func (c *ChattoCore) writePermissionKey(ctx context.Context, subject string, perm Permission, allow bool, allowKey, denyKey string) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}
	kv := c.storage.serverRBACEngine.KV()

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
	c.logger.Debug(verb+" permission", "subject", subject, "permission", perm, "key", writeKey)
	return nil
}

// clearKeyPair deletes both the allow and deny keys for a permission.
// Not finding either is not an error.
func (c *ChattoCore) clearKeyPair(ctx context.Context, perm Permission, allowKey, denyKey string) error {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}
	kv := c.storage.serverRBACEngine.KV()
	if err := kv.Delete(ctx, allowKey); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to clear grant: %w", err)
	}
	if err := kv.Delete(ctx, denyKey); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("failed to clear denial: %w", err)
	}
	c.logger.Debug("Cleared permission", "permission", perm, "allow_key", allowKey, "deny_key", denyKey)
	return nil
}

// ============================================================================
// Room-Level Operations
// ============================================================================

// GrantRoomPermission grants a permission to a role for a specific room.
// Uses ADR-031 key format: room_allow.{roomID}.{roleName}.{verb}.{objectType}
func (c *ChattoCore) GrantRoomPermission(ctx context.Context, roomID, roleName string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at room scope", perm)
	}

	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}

	kv := c.storage.serverRBACKV

	key := rbac.RoomAllowKey(roomID, roleName, parts.Verb, parts.ObjectType)

	if _, err := kv.Put(ctx, key, []byte("1")); err != nil {
		return fmt.Errorf("failed to grant permission: %w", err)
	}

	denyKey := rbac.RoomDenyKey(roomID, roleName, parts.Verb, parts.ObjectType)
	_ = kv.Delete(ctx, denyKey)

	c.logger.Debug("Granted room role permission", "room", roomID, "role", roleName, "permission", perm)
	return nil
}

// DenyRoomPermission denies a permission for a role at a specific room.
// Uses ADR-031 key format: room_deny.{roomID}.{roleName}.{verb}.{objectType}
func (c *ChattoCore) DenyRoomPermission(ctx context.Context, roomID, roleName string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at room scope", perm)
	}

	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}

	kv := c.storage.serverRBACKV

	key := rbac.RoomDenyKey(roomID, roleName, parts.Verb, parts.ObjectType)

	if _, err := kv.Put(ctx, key, []byte("1")); err != nil {
		return fmt.Errorf("failed to deny permission: %w", err)
	}

	grantKey := rbac.RoomAllowKey(roomID, roleName, parts.Verb, parts.ObjectType)
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

	grantKey := rbac.RoomAllowKey(roomID, roleName, parts.Verb, parts.ObjectType)
	denyKey := rbac.RoomDenyKey(roomID, roleName, parts.Verb, parts.ObjectType)

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
//
// Implementation: a single room-scope deny of `message.post` on the
// `everyone` role. The resolver walks roles in descending rank, so
// higher-ranked roles' server-scope grants of `message.post` resolve
// before the walker descends to `everyone` — no explicit per-role
// grants needed.
func (c *ChattoCore) SetupAnnouncementsRoomPermissions(ctx context.Context, roomID string) error {
	if err := c.DenyRoomPermission(ctx, roomID, RoleEveryone, PermMessagePost); err != nil {
		return fmt.Errorf("failed to deny %s for everyone: %w", PermMessagePost, err)
	}

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
//
// Permissions are written at server scope. Channel-room permissions
// (those also marked ScopeGroup / ScopeRoom) cascade into groups and
// rooms via the resolver, so they are configured once here and apply
// everywhere unless an operator adds a per-group or per-room override.
// New groups deliberately ship with no explicit grants —
// `SeedDefaultRoomGroupPermissions` exists as an opt-in tool for admin
// flows that want to materialise the defaults into a group, but no
// automatic boot path calls it any more.
func (c *ChattoCore) InitDefaultPermissions(ctx context.Context) error {
	roleDefaults := []struct {
		role  string
		perms []Permission
	}{
		{RoleOwner, DefaultOwnerPermissions()},
		{RoleAdmin, DefaultAdminPermissions()},
		{RoleModerator, DefaultModeratorPermissions()},
		{RoleEveryone, DefaultEveryonePermissions()},
	}

	for _, spec := range roleDefaults {
		for _, perm := range spec.perms {
			if !PermissionAppliesAtScope(perm, ScopeServer) {
				continue
			}
			if err := c.GrantServerPermission(ctx, spec.role, perm); err != nil {
				return fmt.Errorf("failed to grant %s permission %s: %w", spec.role, perm, err)
			}
		}
	}

	c.logger.Info("Initialized default permissions")
	return nil
}

// SeedDefaultRoomGroupPermissions writes the default channel-room permission
// grants onto a specific room group. Idempotent — uses kv.Create so existing
// keys (operator edits) are preserved.
//
// **Not** called automatically from any boot or `CreateRoomGroup` path —
// new groups start empty and inherit defaults from the server-scope
// cascade. This function exists for admin-UI affordances like a "Copy
// server defaults into this group" button, where the operator opts in
// to materialising the defaults explicitly (e.g. as a starting point
// before applying group-specific overrides).
//
// Only permissions with ScopeGroup in their metadata are seeded — those are
// the ones the resolver reads at group scope when checking channel-room
// permissions.
func (c *ChattoCore) SeedDefaultRoomGroupPermissions(ctx context.Context, groupID string) error {
	roleDefaults := []struct {
		role  string
		perms []Permission
	}{
		{RoleOwner, DefaultOwnerPermissions()},
		{RoleAdmin, DefaultAdminPermissions()},
		{RoleModerator, DefaultModeratorPermissions()},
		{RoleEveryone, DefaultEveryonePermissions()},
	}

	for _, spec := range roleDefaults {
		for _, perm := range spec.perms {
			if !PermissionAppliesAtScope(perm, ScopeGroup) {
				continue
			}
			if err := c.grantSetPermissionIfMissing(ctx, groupID, spec.role, perm); err != nil {
				return fmt.Errorf("seed %s on set %s for %s: %w", perm, groupID, spec.role, err)
			}
		}
	}

	c.logger.Info("Seeded default room-set permissions", "group_id", groupID)
	return nil
}

// grantSetPermissionIfMissing writes a set-scope grant only if neither the
// grant nor a corresponding deny already exists for that (set, role, perm).
// This preserves operator edits across boot-time re-seeding.
func (c *ChattoCore) grantSetPermissionIfMissing(ctx context.Context, groupID, roleName string, perm Permission) error {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}
	kv := c.storage.serverRBACEngine.KV()

	allowKey := rbac.GroupAllowKey(groupID, roleName, parts.Verb, parts.ObjectType)
	denyKey := rbac.GroupDenyKey(groupID, roleName, parts.Verb, parts.ObjectType)

	// If a deny already exists, leave the operator's choice alone.
	if _, err := kv.Get(ctx, denyKey); err == nil {
		return nil
	} else if !errors.Is(err, jetstream.ErrKeyNotFound) {
		return fmt.Errorf("check existing deny: %w", err)
	}

	// kv.Create fails if the allow key already exists — that's the
	// idempotency boundary; we don't overwrite operator edits.
	_, err := kv.Create(ctx, allowKey, []byte("1"))
	if err != nil && !errors.Is(err, jetstream.ErrKeyExists) {
		return fmt.Errorf("create allow: %w", err)
	}
	return nil
}

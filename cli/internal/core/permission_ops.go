package core

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"
)

// ============================================================================
// Permission Operations
// ============================================================================
//
// These ChattoCore methods are thin wrappers around the RBAC engine's
// scope-aware Grant / Deny / Clear primitives. They apply scope-validity
// checks (PermissionAppliesAtScope) and permission-shape validation
// (ValidatePermission), then delegate to engine.Grant / engine.Deny /
// engine.Clear with the appropriate scope tag.
//
// Subject disambiguation by naming convention:
//   - Role: lowercase word (e.g., "owner", "admin", "moderator")
//   - User ID: starts with "U" (e.g., "U9mP2qR5tYz3wK")

// ----------------------------------------------------------------------------
// Server-scope role grants
// ----------------------------------------------------------------------------

// GrantServerPermission grants a permission to a role's server-level default.
func (c *ChattoCore) GrantServerPermission(ctx context.Context, roleName string, perm Permission) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}
	return c.storage.serverRBACEngine.Grant(ctx, ScopeServer, "", roleName, perm)
}

// DenyServerPermission denies a permission at a role's server-level default.
func (c *ChattoCore) DenyServerPermission(ctx context.Context, roleName string, perm Permission) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}
	return c.storage.serverRBACEngine.Deny(ctx, ScopeServer, "", roleName, perm)
}

// ClearServerPermissionState clears both grant and denial for a permission.
func (c *ChattoCore) ClearServerPermissionState(ctx context.Context, roleName string, perm Permission) error {
	return c.storage.serverRBACEngine.Clear(ctx, ScopeServer, "", roleName, perm)
}

// ----------------------------------------------------------------------------
// User-level overrides
// ----------------------------------------------------------------------------
//
// User-level grants/denies sit alongside role-based grants in the same KV.
// The walker consults user-level decisions FIRST (before any role), so an
// explicit user-deny blocks the action even for owners and an explicit
// user-grant allows it even when no role grants it.

// GrantUserPermission grants a permission directly to a user at server scope.
func (c *ChattoCore) GrantUserPermission(ctx context.Context, userID string, perm Permission) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}
	return c.storage.serverRBACEngine.Grant(ctx, ScopeServer, "", userID, perm)
}

// DenyUserPermission denies a permission directly to a user at server scope.
func (c *ChattoCore) DenyUserPermission(ctx context.Context, userID string, perm Permission) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}
	return c.storage.serverRBACEngine.Deny(ctx, ScopeServer, "", userID, perm)
}

// ClearUserPermissionState clears both the grant and denial for a user-level
// permission at server scope.
func (c *ChattoCore) ClearUserPermissionState(ctx context.Context, userID string, perm Permission) error {
	return c.storage.serverRBACEngine.Clear(ctx, ScopeServer, "", userID, perm)
}

// GrantUserRoomPermission grants a permission directly to a user for a specific room.
func (c *ChattoCore) GrantUserRoomPermission(ctx context.Context, roomID, userID string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at room scope", perm)
	}
	return c.storage.serverRBACEngine.Grant(ctx, ScopeRoom, roomID, userID, perm)
}

// DenyUserRoomPermission denies a permission directly to a user for a specific room.
func (c *ChattoCore) DenyUserRoomPermission(ctx context.Context, roomID, userID string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at room scope", perm)
	}
	return c.storage.serverRBACEngine.Deny(ctx, ScopeRoom, roomID, userID, perm)
}

// ClearUserRoomPermissionState clears both the grant and denial for a
// user-level permission for a specific room.
func (c *ChattoCore) ClearUserRoomPermissionState(ctx context.Context, roomID, userID string, perm Permission) error {
	return c.storage.serverRBACEngine.Clear(ctx, ScopeRoom, roomID, userID, perm)
}

// GrantUserGroupPermission grants a permission directly to a user at a room
// group's scope.
func (c *ChattoCore) GrantUserGroupPermission(ctx context.Context, groupID, userID string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeGroup) && !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at group scope", perm)
	}
	return c.storage.serverRBACEngine.Grant(ctx, ScopeGroup, groupID, userID, perm)
}

// DenyUserGroupPermission denies a permission directly to a user at a room
// group's scope.
func (c *ChattoCore) DenyUserGroupPermission(ctx context.Context, groupID, userID string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeGroup) && !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at group scope", perm)
	}
	return c.storage.serverRBACEngine.Deny(ctx, ScopeGroup, groupID, userID, perm)
}

// ClearUserGroupPermissionState clears both the grant and denial for a
// user-level permission at a specific room group's scope.
func (c *ChattoCore) ClearUserGroupPermissionState(ctx context.Context, groupID, userID string, perm Permission) error {
	return c.storage.serverRBACEngine.Clear(ctx, ScopeGroup, groupID, userID, perm)
}

// ----------------------------------------------------------------------------
// Room-scope role grants
// ----------------------------------------------------------------------------

// GrantRoomPermission grants a permission to a role for a specific room.
func (c *ChattoCore) GrantRoomPermission(ctx context.Context, roomID, roleName string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at room scope", perm)
	}
	return c.storage.serverRBACEngine.Grant(ctx, ScopeRoom, roomID, roleName, perm)
}

// DenyRoomPermission denies a permission for a role at a specific room.
func (c *ChattoCore) DenyRoomPermission(ctx context.Context, roomID, roleName string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at room scope", perm)
	}
	return c.storage.serverRBACEngine.Deny(ctx, ScopeRoom, roomID, roleName, perm)
}

// ClearRoomPermissionState removes both grant and denial for a permission at
// room level.
func (c *ChattoCore) ClearRoomPermissionState(ctx context.Context, roomID, roleName string, perm Permission) error {
	return c.storage.serverRBACEngine.Clear(ctx, ScopeRoom, roomID, roleName, perm)
}

// ----------------------------------------------------------------------------
// User-override read helpers
// ----------------------------------------------------------------------------

// GetUserExplicitServerOverride returns the user's explicit user-level
// allow/deny at server scope for the given permission, or DecisionNone when
// there's no user-level override.
func (c *ChattoCore) GetUserExplicitServerOverride(ctx context.Context, userID string, perm Permission) (DecisionKind, error) {
	return c.probeUserExplicit(ctx,
		AllowKey(userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType, ObjectIdAny),
		DenyKey(userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType, ObjectIdAny))
}

// GetUserExplicitGroupOverride returns the user's explicit user-level
// allow/deny at the given room group's scope, or DecisionNone.
func (c *ChattoCore) GetUserExplicitGroupOverride(ctx context.Context, groupID, userID string, perm Permission) (DecisionKind, error) {
	return c.probeUserExplicit(ctx,
		GroupAllowKey(groupID, userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType),
		GroupDenyKey(groupID, userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType))
}

// GetUserExplicitRoomOverride returns the user's explicit user-level
// allow/deny at the given room's scope, or DecisionNone.
func (c *ChattoCore) GetUserExplicitRoomOverride(ctx context.Context, roomID, userID string, perm Permission) (DecisionKind, error) {
	return c.probeUserExplicit(ctx,
		RoomAllowKey(roomID, userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType),
		RoomDenyKey(roomID, userID, perm.KeyParts().Verb, perm.KeyParts().ObjectType))
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

// ============================================================================
// Announcements Room Setup
// ============================================================================

// AnnouncementsRoomName is the canonical name for announcement-only rooms.
const AnnouncementsRoomName = "announcements"

// SetupAnnouncementsRoomPermissions configures an announcements room so that
// only owner, admin, and moderator roles can post new root messages. Everyone
// else can read and post in threads, but cannot start new conversations. This
// is idempotent and safe to call multiple times.
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

	allowKey := GroupAllowKey(groupID, roleName, parts.Verb, parts.ObjectType)
	denyKey := GroupDenyKey(groupID, roleName, parts.Verb, parts.ObjectType)

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

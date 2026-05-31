package core

import (
	"context"
	"errors"
	"fmt"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// Permission Operations
// ============================================================================
//
// These ChattoCore methods append scoped RBAC Grant / Deny / Clear facts.
// They apply scope-validity checks (PermissionAppliesAtScope) and
// permission-shape validation (ValidatePermission), then wait for the local
// RBAC projection to catch up before returning.
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
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacPermissionGranted{
		RbacPermissionGranted: rbacPermissionGrantedEvent(ScopeServer, "", roleName, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, func() error {
		if c.RBAC.GetDecision(ScopeServer, "", roleName, perm) == DecisionAllow {
			return errRBACNoop
		}
		return nil
	})
	if errors.Is(err, errRBACNoop) {
		return nil
	}
	return err
}

// DenyServerPermission denies a permission at a role's server-level default.
func (c *ChattoCore) DenyServerPermission(ctx context.Context, roleName string, perm Permission) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacPermissionDenied{
		RbacPermissionDenied: rbacPermissionDeniedEvent(ScopeServer, "", roleName, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// ClearServerPermissionState clears both grant and denial for a permission.
func (c *ChattoCore) ClearServerPermissionState(ctx context.Context, roleName string, perm Permission) error {
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacPermissionCleared{
		RbacPermissionCleared: rbacPermissionClearedEvent(ScopeServer, "", roleName, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// ----------------------------------------------------------------------------
// User-level overrides
// ----------------------------------------------------------------------------
//
// User-level grants/denies sit alongside role-based decisions in the RBAC
// projection. The walker consults user-level decisions FIRST (before any role), so an
// explicit user-deny blocks the action even for owners and an explicit
// user-grant allows it even when no role grants it.

// GrantUserPermission grants a permission directly to a user at server scope.
func (c *ChattoCore) GrantUserPermission(ctx context.Context, userID string, perm Permission) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacPermissionGranted{
		RbacPermissionGranted: rbacPermissionGrantedEvent(ScopeServer, "", userID, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// DenyUserPermission denies a permission directly to a user at server scope.
func (c *ChattoCore) DenyUserPermission(ctx context.Context, userID string, perm Permission) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacPermissionDenied{
		RbacPermissionDenied: rbacPermissionDeniedEvent(ScopeServer, "", userID, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// ClearUserPermissionState clears both the grant and denial for a user-level
// permission at server scope.
func (c *ChattoCore) ClearUserPermissionState(ctx context.Context, userID string, perm Permission) error {
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacPermissionCleared{
		RbacPermissionCleared: rbacPermissionClearedEvent(ScopeServer, "", userID, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// GrantUserRoomPermission grants a permission directly to a user for a specific room.
func (c *ChattoCore) GrantUserRoomPermission(ctx context.Context, roomID, userID string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at room scope", perm)
	}
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacPermissionGranted{
		RbacPermissionGranted: rbacPermissionGrantedEvent(ScopeRoom, roomID, userID, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// DenyUserRoomPermission denies a permission directly to a user for a specific room.
func (c *ChattoCore) DenyUserRoomPermission(ctx context.Context, roomID, userID string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at room scope", perm)
	}
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacPermissionDenied{
		RbacPermissionDenied: rbacPermissionDeniedEvent(ScopeRoom, roomID, userID, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// ClearUserRoomPermissionState clears both the grant and denial for a
// user-level permission for a specific room.
func (c *ChattoCore) ClearUserRoomPermissionState(ctx context.Context, roomID, userID string, perm Permission) error {
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacPermissionCleared{
		RbacPermissionCleared: rbacPermissionClearedEvent(ScopeRoom, roomID, userID, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// GrantUserGroupPermission grants a permission directly to a user at a room
// group's scope.
func (c *ChattoCore) GrantUserGroupPermission(ctx context.Context, groupID, userID string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeGroup) && !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at group scope", perm)
	}
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacPermissionGranted{
		RbacPermissionGranted: rbacPermissionGrantedEvent(ScopeGroup, groupID, userID, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// DenyUserGroupPermission denies a permission directly to a user at a room
// group's scope.
func (c *ChattoCore) DenyUserGroupPermission(ctx context.Context, groupID, userID string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeGroup) && !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at group scope", perm)
	}
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacPermissionDenied{
		RbacPermissionDenied: rbacPermissionDeniedEvent(ScopeGroup, groupID, userID, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// ClearUserGroupPermissionState clears both the grant and denial for a
// user-level permission at a specific room group's scope.
func (c *ChattoCore) ClearUserGroupPermissionState(ctx context.Context, groupID, userID string, perm Permission) error {
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacPermissionCleared{
		RbacPermissionCleared: rbacPermissionClearedEvent(ScopeGroup, groupID, userID, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// ----------------------------------------------------------------------------
// Room-scope role grants
// ----------------------------------------------------------------------------

// GrantRoomPermission grants a permission to a role for a specific room.
func (c *ChattoCore) GrantRoomPermission(ctx context.Context, roomID, roleName string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at room scope", perm)
	}
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacPermissionGranted{
		RbacPermissionGranted: rbacPermissionGrantedEvent(ScopeRoom, roomID, roleName, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// DenyRoomPermission denies a permission for a role at a specific room.
func (c *ChattoCore) DenyRoomPermission(ctx context.Context, roomID, roleName string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at room scope", perm)
	}
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacPermissionDenied{
		RbacPermissionDenied: rbacPermissionDeniedEvent(ScopeRoom, roomID, roleName, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// ClearRoomPermissionState removes both grant and denial for a permission at
// room level.
func (c *ChattoCore) ClearRoomPermissionState(ctx context.Context, roomID, roleName string, perm Permission) error {
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacPermissionCleared{
		RbacPermissionCleared: rbacPermissionClearedEvent(ScopeRoom, roomID, roleName, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// ----------------------------------------------------------------------------
// User-override read helpers
// ----------------------------------------------------------------------------

// GetUserExplicitServerOverride returns the user's explicit user-level
// allow/deny at server scope for the given permission, or DecisionNone when
// there's no user-level override.
func (c *ChattoCore) GetUserExplicitServerOverride(ctx context.Context, userID string, perm Permission) (DecisionKind, error) {
	return c.RBAC.GetDecision(ScopeServer, "", userID, perm), nil
}

// GetUserExplicitGroupOverride returns the user's explicit user-level
// allow/deny at the given room group's scope, or DecisionNone.
func (c *ChattoCore) GetUserExplicitGroupOverride(ctx context.Context, groupID, userID string, perm Permission) (DecisionKind, error) {
	return c.RBAC.GetDecision(ScopeGroup, groupID, userID, perm), nil
}

// GetUserExplicitRoomOverride returns the user's explicit user-level
// allow/deny at the given room's scope, or DecisionNone.
func (c *ChattoCore) GetUserExplicitRoomOverride(ctx context.Context, roomID, userID string, perm Permission) (DecisionKind, error) {
	return c.RBAC.GetDecision(ScopeRoom, roomID, userID, perm), nil
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
// grants through RBAC events. Idempotent at the projection level.
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
	if c.RBAC.GetDecision(ScopeGroup, groupID, roleName, perm) != DecisionNone {
		return nil
	}
	return c.GrantGroupPermission(ctx, groupID, roleName, perm)
}

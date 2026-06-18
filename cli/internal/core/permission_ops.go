package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
func (c *ChattoCore) GrantServerPermission(ctx context.Context, actorID, roleName string, perm Permission) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_RbacPermissionGranted{
		RbacPermissionGranted: rbacRolePermissionGrantedEvent(ScopeServer, "", roleName, perm),
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
func (c *ChattoCore) DenyServerPermission(ctx context.Context, actorID, roleName string, perm Permission) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_RbacPermissionDenied{
		RbacPermissionDenied: rbacRolePermissionDeniedEvent(ScopeServer, "", roleName, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// ClearServerPermissionState clears both grant and denial for a permission.
func (c *ChattoCore) ClearServerPermissionState(ctx context.Context, actorID, roleName string, perm Permission) error {
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_RbacPermissionCleared{
		RbacPermissionCleared: rbacRolePermissionClearedEvent(ScopeServer, "", roleName, perm),
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
func (c *ChattoCore) GrantUserPermission(ctx context.Context, actorID, userID string, perm Permission) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_RbacPermissionGranted{
		RbacPermissionGranted: rbacUserPermissionGrantedEvent(ScopeServer, "", userID, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// DenyUserPermission denies a permission directly to a user at server scope.
func (c *ChattoCore) DenyUserPermission(ctx context.Context, actorID, userID string, perm Permission) error {
	if err := ValidatePermission(perm); err != nil {
		return err
	}
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_RbacPermissionDenied{
		RbacPermissionDenied: rbacUserPermissionDeniedEvent(ScopeServer, "", userID, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// ClearUserPermissionState clears both the grant and denial for a user-level
// permission at server scope.
func (c *ChattoCore) ClearUserPermissionState(ctx context.Context, actorID, userID string, perm Permission) error {
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_RbacPermissionCleared{
		RbacPermissionCleared: rbacUserPermissionClearedEvent(ScopeServer, "", userID, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// GrantUserRoomPermission grants a permission directly to a user for a specific room.
func (c *ChattoCore) GrantUserRoomPermission(ctx context.Context, actorID, roomID, userID string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at room scope", perm)
	}
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_RbacPermissionGranted{
		RbacPermissionGranted: rbacUserPermissionGrantedEvent(ScopeRoom, roomID, userID, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// DenyUserRoomPermission denies a permission directly to a user for a specific room.
func (c *ChattoCore) DenyUserRoomPermission(ctx context.Context, actorID, roomID, userID string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at room scope", perm)
	}
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_RbacPermissionDenied{
		RbacPermissionDenied: rbacUserPermissionDeniedEvent(ScopeRoom, roomID, userID, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// ClearUserRoomPermissionState clears both the grant and denial for a
// user-level permission for a specific room.
func (c *ChattoCore) ClearUserRoomPermissionState(ctx context.Context, actorID, roomID, userID string, perm Permission) error {
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_RbacPermissionCleared{
		RbacPermissionCleared: rbacUserPermissionClearedEvent(ScopeRoom, roomID, userID, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// GrantUserGroupPermission grants a permission directly to a user at a room
// group's scope.
func (c *ChattoCore) GrantUserGroupPermission(ctx context.Context, actorID, groupID, userID string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeGroup) && !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at group scope", perm)
	}
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_RbacPermissionGranted{
		RbacPermissionGranted: rbacUserPermissionGrantedEvent(ScopeGroup, groupID, userID, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// DenyUserGroupPermission denies a permission directly to a user at a room
// group's scope.
func (c *ChattoCore) DenyUserGroupPermission(ctx context.Context, actorID, groupID, userID string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeGroup) && !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at group scope", perm)
	}
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_RbacPermissionDenied{
		RbacPermissionDenied: rbacUserPermissionDeniedEvent(ScopeGroup, groupID, userID, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// ClearUserGroupPermissionState clears both the grant and denial for a
// user-level permission at a specific room group's scope.
func (c *ChattoCore) ClearUserGroupPermissionState(ctx context.Context, actorID, groupID, userID string, perm Permission) error {
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_RbacPermissionCleared{
		RbacPermissionCleared: rbacUserPermissionClearedEvent(ScopeGroup, groupID, userID, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// ----------------------------------------------------------------------------
// Room-scope role grants
// ----------------------------------------------------------------------------

// GrantRoomPermission grants a permission to a role for a specific room.
func (c *ChattoCore) GrantRoomPermission(ctx context.Context, actorID, roomID, roleName string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at room scope", perm)
	}
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_RbacPermissionGranted{
		RbacPermissionGranted: rbacRolePermissionGrantedEvent(ScopeRoom, roomID, roleName, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// DenyRoomPermission denies a permission for a role at a specific room.
func (c *ChattoCore) DenyRoomPermission(ctx context.Context, actorID, roomID, roleName string, perm Permission) error {
	if !PermissionAppliesAtScope(perm, ScopeRoom) {
		return fmt.Errorf("permission %s does not apply at room scope", perm)
	}
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_RbacPermissionDenied{
		RbacPermissionDenied: rbacRolePermissionDeniedEvent(ScopeRoom, roomID, roleName, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, nil)
	return err
}

// ClearRoomPermissionState removes both grant and denial for a permission at
// room level.
func (c *ChattoCore) ClearRoomPermissionState(ctx context.Context, actorID, roomID, roleName string, perm Permission) error {
	event := newEvent(actorID, &corev1.Event{Event: &corev1.Event_RbacPermissionCleared{
		RbacPermissionCleared: rbacRolePermissionClearedEvent(ScopeRoom, roomID, roleName, perm),
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
// owners can post new root messages via the effective-owner override. Everyone
// else can read, join, post in threads, react, and echo, but cannot start new
// conversations. This is idempotent and safe to call multiple times.
func (c *ChattoCore) SetupAnnouncementsRoomPermissions(ctx context.Context, roomID string) error {
	if err := c.SeedDefaultChannelRoomPermissions(ctx, roomID, AnnouncementsRoomName); err != nil {
		return err
	}
	c.logger.Debug("Set up announcements room permissions", "room", roomID)
	return nil
}

// SeedDefaultChannelRoomPermissions materializes default room-tier exceptions
// for a channel room. Normal rooms inherit broad server-tier member defaults;
// announcements add a local everyone/message.post denial.
func (c *ChattoCore) SeedDefaultChannelRoomPermissions(ctx context.Context, roomID, roomName string) error {
	if roomID == "" {
		return fmt.Errorf("roomID is required")
	}

	if strings.EqualFold(roomName, AnnouncementsRoomName) {
		for _, perm := range DefaultAnnouncementsEveryonePermissions() {
			if err := c.grantRoomPermissionIfMissing(ctx, roomID, RoleEveryone, perm); err != nil {
				return fmt.Errorf("seed announcements everyone %s: %w", perm, err)
			}
		}
		for _, perm := range DefaultAnnouncementsEveryoneDenials() {
			if err := c.denyRoomPermissionIfMissing(ctx, roomID, RoleEveryone, perm); err != nil {
				return fmt.Errorf("seed announcements everyone denial %s: %w", perm, err)
			}
		}
		return c.seedDefaultRoomStaffPermissions(ctx, roomID)
	}

	for _, perm := range DefaultRoomEveryonePermissions() {
		if err := c.grantRoomPermissionIfMissing(ctx, roomID, RoleEveryone, perm); err != nil {
			return fmt.Errorf("seed room everyone %s: %w", perm, err)
		}
	}
	return c.seedDefaultRoomStaffPermissions(ctx, roomID)
}

// EnsureDefaultChannelRoomPermissions backfills default room-tier grants for
// existing rooms. It preserves operator edits by only writing when no
// decision exists.
func (c *ChattoCore) EnsureDefaultChannelRoomPermissions(ctx context.Context) error {
	rooms, err := c.ListRooms(ctx, KindChannel)
	if err != nil {
		return fmt.Errorf("list channel rooms: %w", err)
	}
	for _, room := range rooms {
		if err := c.SeedDefaultChannelRoomPermissions(ctx, room.Id, room.Name); err != nil {
			return fmt.Errorf("ensure room permissions for %s: %w", room.Id, err)
		}
	}
	return nil
}

func (c *ChattoCore) seedDefaultRoomStaffPermissions(ctx context.Context, roomID string) error {
	for _, perm := range DefaultRoomModeratorPermissions() {
		if err := c.grantRoomPermissionIfMissing(ctx, roomID, RoleModerator, perm); err != nil {
			return fmt.Errorf("seed room moderator permission %s %s: %w", RoleModerator, perm, err)
		}
	}
	for _, perm := range DefaultRoomAdminPermissions() {
		if err := c.grantRoomPermissionIfMissing(ctx, roomID, RoleAdmin, perm); err != nil {
			return fmt.Errorf("seed room admin permission %s %s: %w", RoleAdmin, perm, err)
		}
	}
	for _, roleName := range []string{RoleModerator, RoleAdmin} {
		for _, perm := range DefaultAnnouncementsPosterPermissions() {
			if err := c.grantRoomPermissionIfMissing(ctx, roomID, roleName, perm); err != nil {
				return fmt.Errorf("seed room poster permission %s %s: %w", roleName, perm, err)
			}
		}
	}
	return nil
}

// ============================================================================
// Initialization Helpers
// ============================================================================

// InitDefaultPermissions seeds the system roles with their default permission
// grants through RBAC events. Idempotent at the projection level.
//
// Owners receive no persisted permission grants here; effective owners are
// granted every known permission by the resolver. Admin gets
// `DefaultAdminPermissions`, Moderator gets `DefaultModeratorPermissions`,
// Everyone gets `DefaultSeedEveryonePermissions`.
//
// Permissions are written at server scope. Room and message defaults on
// everyone act as the broad baseline; room/group decisions are local
// exceptions.
func (c *ChattoCore) InitDefaultPermissions(ctx context.Context) error {
	roleDefaults := []struct {
		role  string
		perms []Permission
	}{
		{RoleAdmin, DefaultAdminPermissions()},
		{RoleModerator, DefaultModeratorPermissions()},
		{RoleEveryone, DefaultSeedEveryonePermissions()},
	}

	for _, spec := range roleDefaults {
		for _, perm := range spec.perms {
			if !PermissionAppliesAtScope(perm, ScopeServer) {
				continue
			}
			if err := c.GrantServerPermission(ctx, SystemActorID, spec.role, perm); err != nil {
				return fmt.Errorf("failed to grant %s permission %s: %w", spec.role, perm, err)
			}
		}
	}

	c.logger.Info("Initialized default permissions")
	return nil
}

// EnsureDefaultRolePermissions backfills missing default grants for system
// roles. It preserves operator edits by only writing when neither an allow nor
// a deny exists for that role/permission pair.
func (c *ChattoCore) EnsureDefaultRolePermissions(ctx context.Context) error {
	roleDefaults := []struct {
		role  string
		perms []Permission
	}{
		{RoleAdmin, DefaultAdminPermissions()},
		{RoleModerator, DefaultModeratorPermissions()},
		{RoleEveryone, DefaultEveryonePermissions()},
	}

	for _, spec := range roleDefaults {
		for _, perm := range spec.perms {
			if !PermissionAppliesAtScope(perm, ScopeServer) {
				continue
			}
			if err := c.grantServerPermissionIfMissing(ctx, spec.role, perm); err != nil {
				return fmt.Errorf("ensure default %s permission %s: %w", spec.role, perm, err)
			}
		}
	}

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
	return c.GrantGroupPermission(ctx, SystemActorID, groupID, roleName, perm)
}

func (c *ChattoCore) grantRoomPermissionIfMissing(ctx context.Context, roomID, roleName string, perm Permission) error {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}
	if c.RBAC.GetDecision(ScopeRoom, roomID, roleName, perm) != DecisionNone {
		return nil
	}
	return c.GrantRoomPermission(ctx, SystemActorID, roomID, roleName, perm)
}

func (c *ChattoCore) denyRoomPermissionIfMissing(ctx context.Context, roomID, roleName string, perm Permission) error {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}
	if c.RBAC.GetDecision(ScopeRoom, roomID, roleName, perm) != DecisionNone {
		return nil
	}
	return c.DenyRoomPermission(ctx, SystemActorID, roomID, roleName, perm)
}

func (c *ChattoCore) grantServerPermissionIfMissing(ctx context.Context, roleName string, perm Permission) error {
	parts := perm.KeyParts()
	if parts.Verb == "" || parts.ObjectType == "" {
		return fmt.Errorf("invalid permission: %s", perm)
	}
	if c.RBAC.GetDecision(ScopeServer, "", roleName, perm) != DecisionNone {
		return nil
	}
	event := newEvent(SystemActorID, &corev1.Event{Event: &corev1.Event_RbacPermissionGranted{
		RbacPermissionGranted: rbacRolePermissionGrantedEvent(ScopeServer, "", roleName, perm),
	}})
	_, err := c.appendRBACEvent(ctx, event, func() error {
		if c.RBAC.GetDecision(ScopeServer, "", roleName, perm) != DecisionNone {
			return errRBACNoop
		}
		return nil
	})
	if errors.Is(err, errRBACNoop) {
		return nil
	}
	return err
}

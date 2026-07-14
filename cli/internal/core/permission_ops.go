package core

import (
	"context"
	"errors"
	"fmt"

	"hmans.de/chatto/internal/events"
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
	// This is an explicit configuration command, not bootstrap. Preserve its
	// contract when a regular room already has an initialization marker.
	for _, perm := range DefaultAnnouncementsEveryonePermissions() {
		if err := c.GrantRoomPermission(ctx, SystemActorID, roomID, RoleEveryone, perm); err != nil {
			return fmt.Errorf("configure announcements everyone %s: %w", perm, err)
		}
	}
	for _, perm := range DefaultAnnouncementsEveryoneDenials() {
		if err := c.DenyRoomPermission(ctx, SystemActorID, roomID, RoleEveryone, perm); err != nil {
			return fmt.Errorf("configure announcements everyone denial %s: %w", perm, err)
		}
	}
	c.logger.Debug("Set up announcements room permissions", "room", roomID)
	return nil
}

// SeedDefaultChannelRoomPermissions atomically initializes one channel room's
// default permission decisions and durable version marker. Once the marker is
// present, explicit clears are never recreated.
func (c *ChattoCore) SeedDefaultChannelRoomPermissions(ctx context.Context, roomID, roomName string) error {
	if roomID == "" {
		return fmt.Errorf("roomID is required")
	}
	return c.ensureRBACDefaultsInitialized(
		ctx,
		ScopeRoom,
		roomID,
		roomRBACDefaultsVersion,
		rbacDefaultsFillMissing,
		defaultChannelRoomDecisions(roomID, roomName),
		0,
	)
}

// EnsureDefaultChannelRoomPermissions adopts rooms at or before the durable
// server cutoff without changing their decisions. An unmarked room created
// after that cutoff receives any missing defaults plus its marker atomically.
func (c *ChattoCore) EnsureDefaultChannelRoomPermissions(ctx context.Context) error {
	serverInitialized := c.RBAC.DefaultsVersion(ScopeServer, "") >= serverRBACDefaultsVersion
	roomStreamCutoff := c.RBAC.ServerDefaultsRoomStreamCutoff()
	if serverInitialized && roomStreamCutoff > 0 {
		if err := c.RoomDirectoryProjector.WaitFor(ctx, events.SubjectPosition(events.RoomSubjectFilter(), roomStreamCutoff)); err != nil {
			return fmt.Errorf("wait for channel rooms through defaults cutoff: %w", err)
		}
	}
	rooms, err := c.ListRooms(ctx, KindChannel)
	if err != nil {
		return fmt.Errorf("list channel rooms: %w", err)
	}
	for _, room := range rooms {
		seedMode := rbacDefaultsAdoptOnly
		if c.shouldRecoverUnmarkedRoom(room.Id, serverInitialized, roomStreamCutoff) {
			seedMode = rbacDefaultsFillMissing
		}
		if err := c.ensureRBACDefaultsInitialized(
			ctx,
			ScopeRoom,
			room.Id,
			roomRBACDefaultsVersion,
			seedMode,
			defaultChannelRoomDecisions(room.Id, room.Name),
			0,
		); err != nil {
			return fmt.Errorf("ensure room permissions for %s: %w", room.Id, err)
		}
	}
	return nil
}

func (c *ChattoCore) shouldRecoverUnmarkedRoom(roomID string, serverInitialized bool, roomStreamCutoff uint64) bool {
	createdSeq, ok := c.RoomCatalog.CreatedSeq(roomID)
	return serverInitialized && ok && createdSeq > roomStreamCutoff
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

// EnsureDefaultRolePermissions atomically records the server defaults marker.
// Current defaults are included only when RBAC has no permission decisions at
// any scope; existing installations are marked without changing their state.
func (c *ChattoCore) EnsureDefaultRolePermissions(ctx context.Context) error {
	roomStreamCutoff, err := c.EventPublisher.LastSubjectSeq(ctx, events.RoomSubjectFilter())
	if err != nil {
		return fmt.Errorf("read room stream cutoff for RBAC defaults: %w", err)
	}
	return c.ensureRBACDefaultsInitialized(
		ctx,
		ScopeServer,
		"",
		serverRBACDefaultsVersion,
		rbacDefaultsSeedWhenEmpty,
		defaultRBACDecisions(),
		roomStreamCutoff,
	)
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

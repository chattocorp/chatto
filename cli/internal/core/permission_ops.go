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

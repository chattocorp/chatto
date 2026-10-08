package core

import (
	"context"
	"errors"
	"fmt"
)

// CanAssignRoleToUser reports whether the actor may assign a role to a
// concrete account. It applies the same rules as the assignment command.
func (c *ChattoCore) CanAssignRoleToUser(ctx context.Context, actorID, targetUserID, roleName string) (bool, error) {
	if roleName == RoleEveryone {
		return false, nil
	}
	if isBot, _, ok := c.userModel.isBotAndOwner(targetUserID); ok && isBot && roleName == RoleOwner {
		return false, nil
	}
	return permissionDeniedAsFalse(c.requireRoleChangeForAccount(ctx, actorID, targetUserID, roleName))
}

// CanRevokeRoleFromUser reports whether a role revocation is available for a
// concrete target. It applies the revocation command's rules, including the
// self-revocation and configured-owner protections.
func (c *ChattoCore) CanRevokeRoleFromUser(ctx context.Context, actorID, targetUserID, roleName string) (bool, error) {
	if roleName == RoleEveryone || isProtectedSelfRoleRevocation(actorID, targetUserID, roleName) {
		return false, nil
	}
	if roleName == RoleOwner {
		configured, err := c.isConfiguredOwner(ctx, targetUserID)
		if err != nil {
			return false, err
		}
		if configured {
			return false, nil
		}
	}
	return permissionDeniedAsFalse(c.requireRoleChangeForAccount(ctx, actorID, targetUserID, roleName))
}

// permissionDeniedAsFalse turns an authorization result into a capability
// flag. Other errors remain errors.
func permissionDeniedAsFalse(err error) (bool, error) {
	if errors.Is(err, ErrPermissionDenied) {
		return false, nil
	}
	return err == nil, err
}

func isProtectedSelfRoleRevocation(actorID, targetUserID, roleName string) bool {
	return actorID == targetUserID && (roleName == RoleOwner || roleName == RoleAdmin)
}

func (c *ChattoCore) requireRoleAssignmentWithinAuthority(ctx context.Context, actorID, roleName string) error {
	if actorID == SystemActorID {
		return nil
	}
	if actorID == "" {
		return ErrNotAuthenticated
	}
	canAssign, err := c.CanAssignRoles(ctx, actorID)
	if err != nil {
		return fmt.Errorf("check role.assign: %w", err)
	}
	if !canAssign {
		return ErrPermissionDenied
	}
	if c.isServerOwner(actorID) {
		return nil
	}
	if err := c.requireRoleBelowActor(actorID, roleName); err != nil {
		return err
	}
	return c.requireRoleDecisionsWithinAuthority(ctx, actorID, roleName)
}

// requireRoleChangeForAccount authorizes assigning or revoking a role for one
// account. The role must rank
// below the actor and stay within the actor's authority, and the actor must
// outrank the account unless it is their own human account.
func (c *ChattoCore) requireRoleChangeForAccount(ctx context.Context, actorID, targetUserID, roleName string) error {
	if err := c.requireRoleAssignmentWithinAuthority(ctx, actorID, roleName); err != nil {
		return err
	}
	return c.requireOutranksOtherAccount(actorID, targetUserID)
}

// requireRoleDeletionWithinAuthority bounds role deletion like revocation from
// every holder: the actor must be able to manage the role, and must hold
// every permission that the role allows, at the same scope.
func (c *ChattoCore) requireRoleDeletionWithinAuthority(ctx context.Context, actorID, roleName string) error {
	if c.actorIsHierarchyExempt(actorID) {
		return nil
	}
	if err := c.requireRoleManageable(ctx, actorID, roleName); err != nil {
		return err
	}
	return c.requireRoleDecisionsWithinAuthority(ctx, actorID, roleName)
}

// requireBotGrantsWithinAuthority requires the actor to hold every permission
// that the bot is allowed. A new owner sets a
// new ceiling for these grants, so a manager who hands the bot to an account
// with more authority must not unlock grants beyond their own.
func (c *ChattoCore) requireBotGrantsWithinAuthority(ctx context.Context, actorID, botID string) error {
	if c.actorIsHierarchyExempt(actorID) {
		return nil
	}
	for _, decision := range c.rbacModel.userPermissionDecisions(botID) {
		if decision.Decision != DecisionAllow {
			continue
		}
		if _, known := GetPermissionMetadata(decision.Permission); !known {
			continue
		}
		if err := c.requirePermissionDecisionWithinAuthority(ctx, actorID, decision.Scope, decision.ScopeID, decision.Permission); err != nil {
			return err
		}
	}
	return nil
}

// requireRoleDecisionsWithinAuthority requires the actor to effectively hold
// every permission that the role allows at the same scope. Roles only grant
// (ADR-116), so stored role denies do not matter.
func (c *ChattoCore) requireRoleDecisionsWithinAuthority(ctx context.Context, actorID, roleName string) error {
	for _, decision := range c.rbacModel.rolePermissionDecisions(roleName) {
		if decision.Decision != DecisionAllow {
			continue
		}
		// Retired permissions, such as room.ban-member from 0.4, stay in the
		// log but confer no authority, so they cannot exceed the actor's.
		if _, known := GetPermissionMetadata(decision.Permission); !known {
			continue
		}
		if err := c.requirePermissionDecisionWithinAuthority(ctx, actorID, decision.Scope, decision.ScopeID, decision.Permission); err != nil {
			return err
		}
	}
	return nil
}

// requirePermissionDecisionWithinAuthority bounds a change to one stored
// permission decision by the actor's own effective authority: the actor must
// hold perm at the exact scope of the decision. The rule applies to allows,
// denies, and clears alike, so a delegated manager can neither grant authority
// they lack nor remove a restriction that they could not grant back. Effective
// owners pass because they hold every permission while privileged mode is
// active, which every RBAC management permission already requires.
//
// One exception lets room managers open their rooms: at room or room-group
// scope, holding room.manage there is enough for a room permission that does
// not need privileged mode, such as room.join or message.post (ADR-116). A
// holder of room.manage can already add any account to the room, so this
// gives no new access to the room.
func (c *ChattoCore) requirePermissionDecisionWithinAuthority(ctx context.Context, actorID string, scope PermissionScope, scopeID string, perm Permission) error {
	can, err := c.actorCanSetDecision(ctx, actorID, ScopedRolePermissionDecision{Scope: scope, ScopeID: scopeID, Permission: perm})
	if err != nil {
		return err
	}
	if !can {
		return ErrPermissionDenied
	}
	return nil
}

// actorCanSetDecision reports whether the actor's authority covers a change
// to decision (requirePermissionDecisionWithinAuthority). The permission
// matrices use it to tell which cells the viewer can change.
func (c *ChattoCore) actorCanSetDecision(ctx context.Context, actorID string, decision ScopedRolePermissionDecision) (bool, error) {
	has, err := c.actorHasScopedPermission(ctx, actorID, decision)
	if err != nil || has || !roomManagersCanSet(decision.Scope, decision.Permission) {
		return has, err
	}
	decision.Permission = PermRoomManage
	return c.actorHasScopedPermission(ctx, actorID, decision)
}

// roomManagersCanSet reports whether a holder of room.manage at a room or
// room group may set perm there without holding it: perm applies to rooms and
// does not need privileged mode.
func roomManagersCanSet(scope PermissionScope, perm Permission) bool {
	if scope != ScopeRoom && scope != ScopeGroup {
		return false
	}
	meta, known := GetPermissionMetadata(perm)
	return known && !meta.RequiresPrivilegedMode && PermissionAppliesAtScope(perm, ScopeRoom)
}

func (c *ChattoCore) actorHasScopedPermission(ctx context.Context, actorID string, decision ScopedRolePermissionDecision) (bool, error) {
	switch decision.Scope {
	case ScopeServer:
		return c.HasServerPermission(ctx, actorID, decision.Permission)
	case ScopeDM:
		return c.hasRoomPermission(ctx, KindDM, "", actorID, decision.Permission)
	case ScopeGroup:
		return c.hasGroupPermission(ctx, KindChannel, decision.ScopeID, actorID, decision.Permission)
	case ScopeRoom:
		return c.hasRoomPermission(ctx, KindChannel, decision.ScopeID, actorID, decision.Permission)
	default:
		return false, nil
	}
}

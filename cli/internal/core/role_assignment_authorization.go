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
	return permissionDeniedAsFalse(c.requireRoleChangeForAccount(ctx, actorID, targetUserID, roleName, false))
}

// CanRevokeRoleFromUser reports whether a role revocation is available for a
// concrete target. It includes target-specific safety rules that the generic
// role-authority comparison cannot express.
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
	return permissionDeniedAsFalse(c.requireRoleChangeForAccount(ctx, actorID, targetUserID, roleName, true))
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

func (c *ChattoCore) requireRoleAssignmentWithinAuthority(ctx context.Context, actorID, roleName string, includeDenials bool) error {
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
	return c.requireRoleDecisionsWithinAuthority(ctx, actorID, roleName, includeDenials)
}

// requireRoleChangeForAccount authorizes assigning (includeDenials false) or
// revoking (includeDenials true) a role for one account. The role must rank
// below the actor and stay within the actor's authority, and the actor must
// outrank the account unless it is their own human account.
func (c *ChattoCore) requireRoleChangeForAccount(ctx context.Context, actorID, targetUserID, roleName string, includeDenials bool) error {
	if err := c.requireRoleAssignmentWithinAuthority(ctx, actorID, roleName, includeDenials); err != nil {
		return err
	}
	return c.requireOutranksForRoleChange(actorID, targetUserID)
}

// requireRoleDeletionWithinAuthority bounds role deletion like revocation from
// every holder: the role must rank below the actor, and the actor must hold
// every permission that the role allows or denies, at the same scope.
func (c *ChattoCore) requireRoleDeletionWithinAuthority(ctx context.Context, actorID, roleName string) error {
	if c.actorIsHierarchyExempt(actorID) {
		return nil
	}
	if err := c.requireRoleBelowActor(actorID, roleName); err != nil {
		return err
	}
	return c.requireRoleDecisionsWithinAuthority(ctx, actorID, roleName, true)
}

// requireRoleDecisionsWithinAuthority requires the actor to effectively hold
// every permission that the role explicitly allows at the same scope. With
// includeDenials, it also requires every permission that the role denies,
// because removing the role from a user removes those restrictions.
func (c *ChattoCore) requireRoleDecisionsWithinAuthority(ctx context.Context, actorID, roleName string, includeDenials bool) error {
	for _, decision := range c.rbacModel.rolePermissionDecisions(roleName) {
		if decision.Decision != DecisionAllow && (!includeDenials || decision.Decision != DecisionDeny) {
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
func (c *ChattoCore) requirePermissionDecisionWithinAuthority(ctx context.Context, actorID string, scope PermissionScope, scopeID string, perm Permission) error {
	has, err := c.actorHasScopedPermission(ctx, actorID, ScopedRolePermissionDecision{Scope: scope, ScopeID: scopeID, Permission: perm})
	if err != nil {
		return err
	}
	if !has {
		return ErrPermissionDenied
	}
	return nil
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

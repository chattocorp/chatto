package core

import (
	"context"
	"errors"
	"fmt"
)

// CanAssignRole reports whether an actor with role.assign may grant a specific
// role without granting authority they do not currently possess.
func (c *ChattoCore) CanAssignRole(ctx context.Context, actorID, roleName string) (bool, error) {
	if roleName == RoleEveryone {
		return false, nil
	}
	if err := c.requireRoleAssignmentWithinAuthority(ctx, actorID, roleName, false); err != nil {
		if errors.Is(err, ErrPermissionDenied) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CanRevokeRole reports whether an actor with role.assign may revoke a
// specific role. Explicit denials are included because removing a restriction
// can restore authority to the target user.
func (c *ChattoCore) CanRevokeRole(ctx context.Context, actorID, roleName string) (bool, error) {
	if roleName == RoleEveryone {
		return false, nil
	}
	if err := c.requireRoleAssignmentWithinAuthority(ctx, actorID, roleName, true); err != nil {
		if errors.Is(err, ErrPermissionDenied) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// CanRevokeRoleFromUser reports whether a role revocation is available for a
// concrete target. It includes target-specific safety rules that the generic
// role-authority comparison cannot express.
func (c *ChattoCore) CanRevokeRoleFromUser(ctx context.Context, actorID, targetUserID, roleName string) (bool, error) {
	if isProtectedSelfRoleRevocation(actorID, targetUserID, roleName) {
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
	return c.CanRevokeRole(ctx, actorID, roleName)
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
	isOwner, err := c.IsServerOwner(ctx, actorID)
	if err != nil {
		return err
	}
	if isOwner {
		return nil
	}
	if roleName == RoleOwner {
		return ErrPermissionDenied
	}
	return c.requireRoleDecisionsWithinAuthority(ctx, actorID, roleName, includeDenials)
}

// requireRoleDeletionWithinAuthority bounds role deletion like revocation from
// every holder: the actor must hold every permission that the role allows or
// denies, at the same scope.
func (c *ChattoCore) requireRoleDeletionWithinAuthority(ctx context.Context, actorID, roleName string) error {
	if actorID == SystemActorID {
		return nil
	}
	return c.requireRoleDecisionsWithinAuthority(ctx, actorID, roleName, true)
}

// requireNotOwnDirectDecisions prevents a non-owner from editing their own
// direct decisions. Direct decisions survive role changes, so a self-edit
// could copy role authority into a decision that outlasts the role.
func (c *ChattoCore) requireNotOwnDirectDecisions(actorID, targetUserID string) error {
	if actorID == targetUserID && !c.isServerOwner(actorID) {
		return ErrPermissionDenied
	}
	return nil
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

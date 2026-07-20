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
	for _, decision := range c.RBAC.RolePermissionDecisions(roleName) {
		if decision.Decision != DecisionAllow && (!includeDenials || decision.Decision != DecisionDeny) {
			continue
		}
		has, err := c.actorHasScopedPermission(ctx, actorID, decision)
		if err != nil {
			return err
		}
		if !has {
			return ErrPermissionDenied
		}
	}
	return nil
}

func (c *ChattoCore) actorHasScopedPermission(ctx context.Context, actorID string, decision ScopedRolePermissionDecision) (bool, error) {
	switch decision.Scope {
	case ScopeServer:
		return c.HasServerPermission(ctx, actorID, decision.Permission)
	case ScopeGroup:
		return c.hasGroupPermission(ctx, KindChannel, decision.ScopeID, actorID, decision.Permission)
	case ScopeRoom:
		return c.hasRoomPermission(ctx, KindChannel, decision.ScopeID, actorID, decision.Permission)
	default:
		return false, nil
	}
}

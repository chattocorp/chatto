package core

import (
	"context"
	"fmt"
)

// Role hierarchy for administration (ADR-115).
//
// Role order is an administrative rank. It decides who may manage whom; it
// never changes how a permission resolves. An account ranks at the position of
// its highest assigned role. Accounts without roles rank with everyone, at the
// bottom. Effective owners rank above every role and are exempt from these
// rules, so an owner can always recover the server.
//
// A non-owner may act on another account only when they rank strictly above
// it, and may assign or revoke only roles that rank strictly below their own
// highest role. Everyone ranks below every account. Holders of role.manage
// may move, edit, and delete every role except owner (requireRoleManageable,
// MoveServerRole); others who edit a role's decisions, such as room managers,
// need the role to rank below them.
// The system actor is exempt. Callers decide whether an action on the actor's
// own account uses these rules.
//
// Bots hold no roles. A bot ranks exactly like its human owner, both when it
// acts and when someone acts on it. Managing a bot therefore requires
// outranking its owner, unless the actor is that owner.

// accountRank returns the administrative rank of an account. A bot ranks like
// its owner.
func (c *ChattoCore) accountRank(userID string) int32 {
	if isBot, ownerID, ok := c.userModel.isBotAndOwner(userID); ok && isBot {
		if ownerID == "" {
			return PositionEveryone
		}
		userID = ownerID
	}
	if c.isServerOwner(userID) {
		return rankOwner
	}
	rank := PositionEveryone
	for _, roleName := range c.rbacModel.userRoles(userID) {
		if role, ok := c.rbacModel.role(roleName); ok && role.GetPosition() > rank {
			rank = role.GetPosition()
		}
	}
	return rank
}

// actorIsHierarchyExempt reports whether the actor bypasses the hierarchy.
func (c *ChattoCore) actorIsHierarchyExempt(actorID string) bool {
	return actorID == SystemActorID || c.isServerOwner(actorID)
}

// requireOutranksAccount requires the actor to rank strictly above the target
// account. The owner of a bot always manages their own bot.
func (c *ChattoCore) requireOutranksAccount(actorID, targetUserID string) error {
	if c.actorIsHierarchyExempt(actorID) {
		return nil
	}
	if isBot, ownerID, ok := c.userModel.isBotAndOwner(targetUserID); ok && isBot && ownerID == actorID {
		return nil
	}
	if !c.outranks(actorID, targetUserID) {
		return ErrPermissionDenied
	}
	return nil
}

// requireOutranksOtherAccount applies requireOutranksAccount unless the actor
// acts on their own account.
func (c *ChattoCore) requireOutranksOtherAccount(actorID, targetUserID string) error {
	if actorID == targetUserID {
		return nil
	}
	return c.requireOutranksAccount(actorID, targetUserID)
}

// outranks reports whether actorID ranks strictly above targetUserID. Nobody
// outranks an owner.
func (c *ChattoCore) outranks(actorID, targetUserID string) bool {
	if c.isServerOwner(targetUserID) {
		return false
	}
	return c.accountRank(actorID) > c.accountRank(targetUserID)
}

// requireRoleBelowActor requires the role to rank strictly below the actor's
// highest role. The everyone role ranks below every account, so any role or
// room manager can edit its decisions. Only owners may manage the owner role.
func (c *ChattoCore) requireRoleBelowActor(actorID, roleName string) error {
	if c.actorIsHierarchyExempt(actorID) || roleName == RoleEveryone {
		return nil
	}
	if roleName == RoleOwner {
		return ErrPermissionDenied
	}
	role, ok := c.rbacModel.role(roleName)
	if !ok {
		return ErrRoleNotFound
	}
	if role.GetPosition() >= c.accountRank(actorID) {
		return ErrPermissionDenied
	}
	return nil
}

// requireRoleManageable authorizes a change to a role definition: its
// metadata, its permission decisions, or its deletion. Holders of role.manage
// may change every role except owner, which stays owner-only. Other actors,
// such as room managers who edit a role's room decisions, need the role to
// rank below their highest role (ADR-115).
func (c *ChattoCore) requireRoleManageable(ctx context.Context, actorID, roleName string) error {
	if roleName != RoleOwner && !c.actorIsHierarchyExempt(actorID) {
		canManage, err := c.CanManageRoles(ctx, actorID)
		if err != nil {
			return fmt.Errorf("check role.manage: %w", err)
		}
		if canManage {
			return nil
		}
	}
	return c.requireRoleBelowActor(actorID, roleName)
}

// ViewerHighestRole returns the name of the role at which actorID ranks:
// owner for effective owners, the highest assigned role otherwise, and
// everyone without roles. A bot ranks at its owner's highest role.
// Clients use it with the role order to show which actions can succeed.
func (c *ChattoCore) ViewerHighestRole(actorID string) string {
	userID := actorID
	if isBot, ownerID, ok := c.userModel.isBotAndOwner(actorID); ok && isBot {
		if ownerID == "" {
			return RoleEveryone
		}
		userID = ownerID
	}
	if c.isServerOwner(userID) {
		return RoleOwner
	}
	// Among roles with equal positions, prefer the one that comes first in
	// role order, which ListServerRoles sorts by name in reverse.
	highest, highestPosition := RoleEveryone, PositionEveryone
	for _, roleName := range c.rbacModel.userRoles(userID) {
		role, ok := c.rbacModel.role(roleName)
		if !ok {
			continue
		}
		position := role.GetPosition()
		if position > highestPosition || (position == highestPosition && roleName > highest) {
			highest, highestPosition = roleName, position
		}
	}
	return highest
}

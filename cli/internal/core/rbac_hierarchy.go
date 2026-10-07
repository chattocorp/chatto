package core

// Role hierarchy for administration (ADR-114).
//
// Role order is an administrative rank. It decides who may manage whom; it
// never changes how a permission resolves. An account ranks at the position of
// its highest assigned role. Accounts without roles rank with everyone, at the
// bottom. Effective owners rank above every role and are exempt from these
// rules, so an owner can always recover the server.
//
// A non-owner may act on another account only when they rank strictly above
// it, and may assign, revoke, edit, delete, or move only roles that rank
// strictly below their own highest role. Everyone ranks below every account. The system actor is exempt. Callers
// decide whether an action on the actor's own account uses these rules.
//
// Bots rank by their own roles. As actors, bots never rank above their human
// owner, like their permissions never exceed the owner's. Managing a bot also
// requires outranking its owner, unless the actor is that owner. Role changes
// on a bot always require outranking the bot itself, so an owner cannot
// remove a restriction role that a higher-ranked account assigned.

// accountRank returns the administrative rank of an account.
func (c *ChattoCore) accountRank(userID string) int32 {
	if c.isServerOwner(userID) {
		return PositionOwner
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
// account. For a bot target, the actor must also outrank the bot's owner,
// unless the actor is that owner, who always manages their own bot.
func (c *ChattoCore) requireOutranksAccount(actorID, targetUserID string) error {
	if c.actorIsHierarchyExempt(actorID) {
		return nil
	}
	if isBot, ownerID, ok := c.userModel.isBotAndOwner(targetUserID); ok && isBot {
		if ownerID == actorID {
			return nil
		}
		if ownerID != "" && !c.outranks(actorID, ownerID) {
			return ErrPermissionDenied
		}
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

func (c *ChattoCore) outranks(actorID, targetUserID string) bool {
	if c.isServerOwner(targetUserID) {
		return false
	}
	return c.actorRank(actorID) > c.accountRank(targetUserID)
}

// actorRank is the rank with which an account acts. A bot acts with at most
// its owner's rank.
func (c *ChattoCore) actorRank(actorID string) int32 {
	rank := c.accountRank(actorID)
	if isBot, ownerID, ok := c.userModel.isBotAndOwner(actorID); ok && isBot {
		if ownerRank := c.accountRank(ownerID); ownerRank < rank {
			rank = ownerRank
		}
	}
	return rank
}

// requireOutranksForRoleChange authorizes a role assignment or revocation for
// an account. Humans may change their own roles. For a bot, the actor must
// outrank the bot and, unless they own it, its owner.
func (c *ChattoCore) requireOutranksForRoleChange(actorID, targetUserID string) error {
	if c.actorIsHierarchyExempt(actorID) {
		return nil
	}
	isBot, ownerID, ok := c.userModel.isBotAndOwner(targetUserID)
	if !ok || !isBot {
		return c.requireOutranksOtherAccount(actorID, targetUserID)
	}
	if !c.outranks(actorID, targetUserID) {
		return ErrPermissionDenied
	}
	if ownerID != "" && ownerID != actorID && !c.outranks(actorID, ownerID) {
		return ErrPermissionDenied
	}
	return nil
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
	if role.GetPosition() >= c.actorRank(actorID) {
		return ErrPermissionDenied
	}
	return nil
}

// HierarchyAllowsActingOn reports whether the role hierarchy lets actorID act
// on targetUserID. It does not check permissions. Clients use it to show which
// actions can succeed.
func (c *ChattoCore) HierarchyAllowsActingOn(actorID, targetUserID string) bool {
	return c.requireOutranksAccount(actorID, targetUserID) == nil
}

// RoleRanksBelowActor reports whether the role hierarchy lets actorID manage
// roleName. It does not check permissions.
func (c *ChattoCore) RoleRanksBelowActor(actorID, roleName string) bool {
	return c.requireRoleBelowActor(actorID, roleName) == nil
}

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
// Bots rank by their own roles. Acting on a bot also requires outranking its
// human owner, unless the actor is that owner.

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

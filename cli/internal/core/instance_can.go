package core

import "context"

// This file provides semantic helper functions for instance-level permission checks.
// These wrap the low-level HasInstancePermission function with business-meaningful names,
// making code more readable and permission usage easier to audit.
//
// Each function returns (bool, error) where:
//   - bool indicates whether the user has the permission
//   - error is non-nil only if there was a system error checking permissions
//
// Note: These functions check RBAC permissions only. Config-based admin check
// (owners.emails) should be done separately by the caller.

// CanAdminAccess checks if a user can access the admin panel.
// Only instance admins have this permission.
func (c *ChattoCore) CanAdminAccess(ctx context.Context, userID string) (bool, error) {
	return c.HasInstancePermission(ctx, userID, PermAdminAccess)
}

// CanAdminUsersView checks if a user can view the users page in admin.
func (c *ChattoCore) CanAdminUsersView(ctx context.Context, userID string) (bool, error) {
	return c.HasInstancePermission(ctx, userID, PermAdminUsersView)
}

// CanAdminUsersManage checks if a user can edit user role assignments.
func (c *ChattoCore) CanAdminUsersManage(ctx context.Context, userID string) (bool, error) {
	return c.HasInstancePermission(ctx, userID, PermAdminUsersManage)
}

// CanAdminRolesView checks if a user can view the roles page in admin.
func (c *ChattoCore) CanAdminRolesView(ctx context.Context, userID string) (bool, error) {
	return c.HasInstancePermission(ctx, userID, PermAdminRolesView)
}

// CanAdminRolesManage checks if a user can create/edit instance roles and their permissions.
func (c *ChattoCore) CanAdminRolesManage(ctx context.Context, userID string) (bool, error) {
	return c.HasInstancePermission(ctx, userID, PermAdminRolesManage)
}

// CanAdminSystemView checks if a user can view the system and data pages in admin.
func (c *ChattoCore) CanAdminSystemView(ctx context.Context, userID string) (bool, error) {
	return c.HasInstancePermission(ctx, userID, PermAdminSystemView)
}

// CanDMView checks if a user can access the DM space and read DMs.
// Verified users have this permission by default.
func (c *ChattoCore) CanDMView(ctx context.Context, userID string) (bool, error) {
	return c.HasInstancePermission(ctx, userID, PermDMView)
}

// CanDMWrite checks if a user can start DM conversations and send messages.
// Verified users have this permission by default.
func (c *ChattoCore) CanDMWrite(ctx context.Context, userID string) (bool, error) {
	return c.HasInstancePermission(ctx, userID, PermDMWrite)
}

// CanAdminManageUser checks if an actor can perform admin user-management
// actions (e.g. editing identity, clearing cooldowns) on a target user based
// on instance role hierarchy. Self-management is always allowed; otherwise
// the actor's highest role must outrank the target's highest role.
//
// Note: this checks RBAC hierarchy only. Config-based admins (owners.emails)
// are not visible to the RBAC engine and should bypass this check at the
// resolver layer (they outrank everyone).
func (c *ChattoCore) CanAdminManageUser(ctx context.Context, actorID, targetID string) (bool, error) {
	if actorID == targetID {
		return true, nil
	}
	return c.storage.serverRBACEngine.CanUserManageUser(ctx, actorID, targetID)
}

// CanDeleteUser checks if an actor can delete a specific user account.
// Returns true if:
//   - The actor is deleting their own account and has user.delete.self permission, OR
//   - The actor has the user.delete permission (admin capability)
func (c *ChattoCore) CanDeleteUser(ctx context.Context, actorID, targetUserID string) (bool, error) {
	if actorID == targetUserID {
		return c.HasInstancePermission(ctx, actorID, PermUserDeleteSelf)
	}

	return c.HasInstancePermission(ctx, actorID, PermUserDelete)
}

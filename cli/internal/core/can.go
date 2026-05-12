package core

import "context"

// can.go provides semantic helper functions for permission checks. These wrap
// the low-level HasInstancePermission / hasServerPermission / hasRoomPermission
// calls with business-meaningful names, making code more readable and
// permission usage easier to audit.
//
// Each function returns (bool, error) where:
//   - bool indicates whether the user has the permission
//   - error is non-nil only if there was a system error checking permissions
//
// Note: These functions check RBAC permissions only. Config-based admin
// status (owners.emails) is materialised as a real owner-role assignment
// elsewhere, so the resolver layer doesn't need a separate fallback.

// ============================================================================
// Server-tier Permissions
// ============================================================================

// CanAdminAccess checks if a user can access the admin panel.
// Only server admins have this permission.
func (c *ChattoCore) CanAdminAccess(ctx context.Context, userID string) (bool, error) {
	return c.HasInstancePermission(ctx, userID, PermAdminAccess)
}

// CanAdminUsersView checks if a user can view the users page in admin.
func (c *ChattoCore) CanAdminUsersView(ctx context.Context, userID string) (bool, error) {
	return c.HasInstancePermission(ctx, userID, PermAdminUsersView)
}

// CanAssignRoles checks if a user can assign/revoke roles to/from users.
// Backed by the canonical role.assign permission. Subsumes the previous
// CanAdminUsersManage (which was a duplicate "edit role assignments").
func (c *ChattoCore) CanAssignRoles(ctx context.Context, userID string) (bool, error) {
	return c.HasInstancePermission(ctx, userID, PermRoleAssign)
}

// CanManageRoles checks if a user can create, edit, delete, and reorder
// roles and their permissions. Subsumes the previous CanAdminRolesManage /
// CanSpaceRolesManage pair (which were duplicates).
func (c *ChattoCore) CanManageRoles(ctx context.Context, userID string) (bool, error) {
	return c.HasInstancePermission(ctx, userID, PermRoleManage)
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
// on role hierarchy. Self-management is always allowed; otherwise the
// actor's highest role must outrank the target's highest role.
func (c *ChattoCore) CanAdminManageUser(ctx context.Context, actorID, targetID string) (bool, error) {
	if actorID == targetID {
		return true, nil
	}
	return c.storage.serverRBACEngine.CanUserManageUser(ctx, actorID, targetID)
}

// CanDeleteUser checks if an actor can delete a specific user account.
// Returns true if:
//   - The actor is deleting their own account and has user.delete-self permission, OR
//   - The actor has the user.delete permission (admin capability)
func (c *ChattoCore) CanDeleteUser(ctx context.Context, actorID, targetUserID string) (bool, error) {
	if actorID == targetUserID {
		return c.HasInstancePermission(ctx, actorID, PermUserDeleteSelf)
	}

	return c.HasInstancePermission(ctx, actorID, PermUserDelete)
}

// ============================================================================
// Server-tier Admin Permissions
// ============================================================================

// adminPermissions is the set of admin-level server permissions.
// Used by HasAnyAdminPermission to determine "should the Admin link appear".
var adminPermissions = []Permission{
	PermServerManage,
	PermRoleManage,
	PermRoleAssign,
	PermMemberInvite,
	PermMemberRemove,
	PermRoomManage,
}

// HasAnyAdminPermission checks if a user has any admin-level permission.
// Used to determine whether the server admin link should be visible.
func (c *ChattoCore) HasAnyAdminPermission(ctx context.Context, userID string) (bool, error) {
	for _, perm := range adminPermissions {
		has, err := c.hasServerPermission(ctx, userID, perm)
		if err != nil {
			return false, err
		}
		if has {
			return true, nil
		}
	}
	return false, nil
}

// CanManageServer checks if a user can update server settings (name, description, logo).
func (c *ChattoCore) CanManageServer(ctx context.Context, userID string) (bool, error) {
	return c.hasServerPermission(ctx, userID, PermServerManage)
}

// CanInviteMembers checks if a user can invite new members to the server.
func (c *ChattoCore) CanInviteMembers(ctx context.Context, userID string) (bool, error) {
	return c.hasServerPermission(ctx, userID, PermMemberInvite)
}

// CanRemoveMembers checks if a user can remove other members from the server.
func (c *ChattoCore) CanRemoveMembers(ctx context.Context, userID string) (bool, error) {
	return c.hasServerPermission(ctx, userID, PermMemberRemove)
}

// CanManageAnyRoom checks if a user can update or delete any room.
// "Any" room as opposed to a specific room — for per-room checks, use the
// room-level resolver via PermissionResolver.HasRoomPermission.
func (c *ChattoCore) CanManageAnyRoom(ctx context.Context, userID string) (bool, error) {
	return c.hasServerPermission(ctx, userID, PermRoomManage)
}

// ============================================================================
// Server-tier Member Permissions
// ============================================================================

// CanBrowseRooms checks if a user can view the list of rooms.
// DM-sensitive: for KindDM the resolver short-circuits to a static rule
// (DM rooms aren't listed via this API).
func (c *ChattoCore) CanBrowseRooms(ctx context.Context, userID string, kind RoomKind) (bool, error) {
	return c.hasKindPermission(ctx, kind, userID, PermRoomList)
}

// CanCreateRoom checks if a user can create new rooms.
// DM-sensitive: see CanBrowseRooms.
func (c *ChattoCore) CanCreateRoom(ctx context.Context, userID string, kind RoomKind) (bool, error) {
	return c.hasKindPermission(ctx, kind, userID, PermRoomCreate)
}

// CanJoinRoom checks if a user can join existing rooms.
// DM-sensitive: DMs grant join implicitly to participants.
func (c *ChattoCore) CanJoinRoom(ctx context.Context, userID string, kind RoomKind) (bool, error) {
	return c.hasKindPermission(ctx, kind, userID, PermRoomJoin)
}

// ============================================================================
// Room-Scoped Permissions
// ============================================================================

// CanPostMessage checks if a user can post new root messages in a specific room.
// Uses room-level permission resolution (checks room overrides, then server defaults).
func (c *ChattoCore) CanPostMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, kind, roomID, userID, PermMessagePost)
}

// CanPostInThread checks if a user can post messages in a thread.
// Uses room-level permission resolution (checks room overrides, then server defaults).
func (c *ChattoCore) CanPostInThread(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, kind, roomID, userID, PermMessagePostInThread)
}

// CanReply checks if a user can use reply attribution (inReplyTo) on room-level messages.
// Uses room-level permission resolution (checks room overrides, then server defaults).
func (c *ChattoCore) CanReply(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, kind, roomID, userID, PermMessageReply)
}

// CanReplyInThread checks if a user can use reply attribution (inReplyTo) on thread messages.
// Uses room-level permission resolution (checks room overrides, then server defaults).
func (c *ChattoCore) CanReplyInThread(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, kind, roomID, userID, PermMessageReplyInThread)
}

// CanReactToMessage checks if a user can add/remove reactions in a specific room.
func (c *ChattoCore) CanReactToMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, kind, roomID, userID, PermMessageReact)
}

// CanEchoMessage checks if a user can echo thread replies to the main channel.
func (c *ChattoCore) CanEchoMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, kind, roomID, userID, PermMessageEcho)
}

// CanEditOwnMessage checks if a user can edit their own messages in a specific room.
func (c *ChattoCore) CanEditOwnMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, kind, roomID, userID, PermMessageEditOwn)
}

// CanEditAnyMessage checks if a user can edit any user's messages in a specific room.
func (c *ChattoCore) CanEditAnyMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, kind, roomID, userID, PermMessageEditAny)
}

// CanDeleteOwnMessage checks if a user can delete their own messages in a specific room.
func (c *ChattoCore) CanDeleteOwnMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, kind, roomID, userID, PermMessageDeleteOwn)
}

// CanDeleteAnyMessage checks if a user can delete any user's messages in a specific room.
func (c *ChattoCore) CanDeleteAnyMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, kind, roomID, userID, PermMessageDeleteAny)
}

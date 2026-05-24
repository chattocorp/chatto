package core

import "context"

// can.go provides semantic helper functions for permission checks. These wrap
// the low-level HasServerPermission / hasServerPermission / hasRoomPermission
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
	return c.HasServerPermission(ctx, userID, PermAdminAccess)
}

// CanAdminUsersView checks if a user can view the users page in admin.
func (c *ChattoCore) CanAdminUsersView(ctx context.Context, userID string) (bool, error) {
	return c.HasServerPermission(ctx, userID, PermAdminUsersView)
}

// CanAssignRoles checks if a user can assign/revoke roles to/from users.
// Backed by the canonical role.assign permission. Subsumes the previous
// CanAdminUsersManage (which was a duplicate "edit role assignments").
func (c *ChattoCore) CanAssignRoles(ctx context.Context, userID string) (bool, error) {
	return c.HasServerPermission(ctx, userID, PermRoleAssign)
}

// CanManageRoles checks if a user can create, edit, delete, and reorder
// roles and their permissions. Subsumes the previous CanAdminRolesManage /
// CanSpaceRolesManage pair (which were duplicates).
func (c *ChattoCore) CanManageRoles(ctx context.Context, userID string) (bool, error) {
	return c.HasServerPermission(ctx, userID, PermRoleManage)
}

// CanAdminSystemView checks if a user can view the system and data pages in admin.
func (c *ChattoCore) CanAdminSystemView(ctx context.Context, userID string) (bool, error) {
	return c.HasServerPermission(ctx, userID, PermAdminSystemView)
}

// CanAdminAuditView checks if a user can view the audit log (event log)
// page in admin. The event-log inspection view in /server-admin/event-log
// is the first concrete use; future log exports / search endpoints gate
// on the same permission.
func (c *ChattoCore) CanAdminAuditView(ctx context.Context, userID string) (bool, error) {
	return c.HasServerPermission(ctx, userID, PermAdminAuditView)
}

// CanDMView checks if a user can access the DM space and read DMs.
// Verified users have this permission by default.
func (c *ChattoCore) CanDMView(ctx context.Context, userID string) (bool, error) {
	return c.HasServerPermission(ctx, userID, PermDMView)
}

// CanDMWrite checks if a user can start DM conversations and send messages.
// Verified users have this permission by default.
func (c *ChattoCore) CanDMWrite(ctx context.Context, userID string) (bool, error) {
	return c.HasServerPermission(ctx, userID, PermDMWrite)
}

// CanDeleteUser checks if an actor can delete a specific user account.
// Returns true if:
//   - The actor is deleting their own account and has user.delete-self, OR
//   - The actor has user.delete-any (the admin power).
//
// NOTE: For cross-user deletion, callers must additionally check that the
// actor strictly outranks the target — same shape as message moderation
// and identity edits. Enforce that at the API boundary, not here.
func (c *ChattoCore) CanDeleteUser(ctx context.Context, actorID, targetUserID string) (bool, error) {
	if actorID == targetUserID {
		return c.HasServerPermission(ctx, actorID, PermUserDeleteSelf)
	}

	return c.HasServerPermission(ctx, actorID, PermUserDeleteAny)
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
	PermRoomManage,
	PermUserDeleteAny,
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

// CanManageAnyRoom checks if a user can update or delete any room.
// "Any" room as opposed to a specific room — for per-room checks, use the
// room-level resolver via PermissionResolver.HasRoomPermission.
func (c *ChattoCore) CanManageAnyRoom(ctx context.Context, userID string) (bool, error) {
	return c.hasServerPermission(ctx, userID, PermRoomManage)
}

// ============================================================================
// Server-tier Member Permissions
// ============================================================================

// CanSeeRoom checks if a user can see a specific room in the directory
// or any other surface that enumerates rooms (e.g. the group "Join all"
// affordance). A user can see a room iff they are already a member OR
// `room.list` resolves to allow at the room (room → group → server walk).
//
// `room.list` is distinct from `room.join`: a restricted room can be
// visible in the directory (request-access flow) without being directly
// joinable. Pair with `CanJoinRoomAt` to decide whether to show a "Join"
// button vs a "Restricted" indicator.
//
// DM-sensitive: for KindDM this returns false. DM rooms aren't surfaced
// through the channel room-list API; they use their own listing path.
func (c *ChattoCore) CanSeeRoom(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	if kind == KindDM {
		return false, nil
	}
	isMember, err := c.RoomMembershipExists(ctx, kind, userID, roomID)
	if err != nil {
		return false, err
	}
	if isMember {
		return true, nil
	}
	return c.hasRoomPermission(ctx, kind, roomID, userID, PermRoomList)
}

// CanCreateRoom checks if a user can create new rooms. When groupID is
// non-empty, the check is scoped to that room group (a role granted
// room.create at server scope can create in any group; a role granted only
// at a group's scope can create only in that group). DM rooms are
// creation-locked at this layer (the DM boundary in the resolver denies
// room.create unconditionally); DMs are created via FindOrCreateDM.
func (c *ChattoCore) CanCreateRoom(ctx context.Context, userID string, kind RoomKind, groupID string) (bool, error) {
	if kind == KindChannel && groupID != "" {
		return c.hasGroupPermission(ctx, kind, groupID, userID, PermRoomCreate)
	}
	return c.hasKindPermission(ctx, kind, userID, PermRoomCreate)
}

// CanJoinRoom checks if a user can join existing rooms at the server tier
// (no specific room context). Used as a top-level "is the join action
// available at all" check. For per-room decisions — including "is this
// user implicitly a member of this global room" — use CanJoinRoomAt,
// which walks the room → group → server hierarchy.
//
// DM-sensitive: DMs grant join implicitly to participants.
func (c *ChattoCore) CanJoinRoom(ctx context.Context, userID string, kind RoomKind) (bool, error) {
	return c.hasKindPermission(ctx, kind, userID, PermRoomJoin)
}

// CanJoinRoomAt checks if a user can join a specific room. Uses room-scope
// permission resolution (room override > group override > server default).
// This is the gate for global-room implicit membership: a global room's
// members are exactly the users for whom this returns true.
func (c *ChattoCore) CanJoinRoomAt(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, kind, roomID, userID, PermRoomJoin)
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

// CanReactToMessage checks if a user can add/remove reactions in a specific room.
func (c *ChattoCore) CanReactToMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, kind, roomID, userID, PermMessageReact)
}

// CanEchoMessage checks if a user can echo thread replies to the main channel.
func (c *ChattoCore) CanEchoMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, kind, roomID, userID, PermMessageEcho)
}

// CanManageOthersMessage checks if a user can edit/delete other users'
// messages in a specific room. Authors editing/deleting their own messages
// don't need this permission — that's always allowed and gated only by
// authorship + the edit window in core. Callers that operate on someone
// else's message must ALSO check that the actor strictly outranks the
// author via requireOutranksAuthor.
func (c *ChattoCore) CanManageOthersMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, kind, roomID, userID, PermMessageManage)
}

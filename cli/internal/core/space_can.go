package core

import "context"

// This file provides semantic helper functions for permission checks.
// These wrap the low-level HasPermission function with business-meaningful names,
// making code more readable and permission usage easier to audit.
//
// Each function returns (bool, error) where:
//   - bool indicates whether the user has the permission
//   - error is non-nil only if there was a system error checking permissions
//
// Example usage:
//
//	if can, err := c.CanAdminSpaceManage(ctx, userID, spaceID); err != nil {
//	    return fmt.Errorf("failed to check permission: %w", err)
//	} else if !can {
//	    return ErrPermissionDenied
//	}

// ============================================================================
// Admin Permissions
// ============================================================================

// spaceAdminPermissions is the set of admin-level space permissions.
// Used by HasAnyAdminPermission to check for space admin access.
var spaceAdminPermissions = []Permission{
	PermSpaceManage,
	PermSpaceDelete,
	PermRoleManage,
	PermRoleAssign,
	PermMemberInvite,
	PermMemberRemove,
	PermRoomManage,
}

// HasAnyAdminPermission checks if a user has any admin.* permission in a space.
// This is used to determine if the user should see the Space Admin link.
func (c *ChattoCore) HasAnyAdminPermission(ctx context.Context, userID, spaceID string) (bool, error) {
	for _, perm := range spaceAdminPermissions {
		has, err := c.hasSpacePermission(ctx, spaceID, userID, perm)
		if err != nil {
			return false, err
		}
		if has {
			return true, nil
		}
	}
	return false, nil
}

// CanAdminSpaceManage checks if a user can update space settings (name, description, logo).
func (c *ChattoCore) CanAdminSpaceManage(ctx context.Context, userID, spaceID string) (bool, error) {
	return c.hasSpacePermission(ctx, spaceID, userID, PermSpaceManage)
}

// CanAdminSpaceDelete checks if a user can delete a space entirely.
func (c *ChattoCore) CanAdminSpaceDelete(ctx context.Context, userID, spaceID string) (bool, error) {
	return c.hasSpacePermission(ctx, spaceID, userID, PermSpaceDelete)
}

// CanSpaceRolesManage checks if a user can create, update, delete roles and grant/revoke permissions in a space.
func (c *ChattoCore) CanSpaceRolesManage(ctx context.Context, userID, spaceID string) (bool, error) {
	return c.hasSpacePermission(ctx, spaceID, userID, PermRoleManage)
}

// CanSpaceRolesAssign checks if a user can assign or revoke roles to/from other users in a space.
func (c *ChattoCore) CanSpaceRolesAssign(ctx context.Context, userID, spaceID string) (bool, error) {
	return c.hasSpacePermission(ctx, spaceID, userID, PermRoleAssign)
}

// CanAdminMembersInvite checks if a user can invite new members to the space.
func (c *ChattoCore) CanAdminMembersInvite(ctx context.Context, userID, spaceID string) (bool, error) {
	return c.hasSpacePermission(ctx, spaceID, userID, PermMemberInvite)
}

// CanAdminMembersRemove checks if a user can remove other members from the space.
func (c *ChattoCore) CanAdminMembersRemove(ctx context.Context, userID, spaceID string) (bool, error) {
	return c.hasSpacePermission(ctx, spaceID, userID, PermMemberRemove)
}

// CanAdminRoomsManage checks if a user can update or delete any room in the space.
func (c *ChattoCore) CanAdminRoomsManage(ctx context.Context, userID, spaceID string) (bool, error) {
	return c.hasSpacePermission(ctx, spaceID, userID, PermRoomManage)
}

// ============================================================================
// Normal Permissions
// ============================================================================

// CanBrowseRooms checks if a user can view the list of rooms in the space.
func (c *ChattoCore) CanBrowseRooms(ctx context.Context, userID, spaceID string) (bool, error) {
	return c.hasSpacePermission(ctx, spaceID, userID, PermRoomList)
}

// CanCreateRoom checks if a user can create new rooms in the space.
func (c *ChattoCore) CanCreateRoom(ctx context.Context, userID, spaceID string) (bool, error) {
	return c.hasSpacePermission(ctx, spaceID, userID, PermRoomCreate)
}

// CanJoinRoom checks if a user can join existing rooms in the space.
func (c *ChattoCore) CanJoinRoom(ctx context.Context, userID, spaceID string) (bool, error) {
	return c.hasSpacePermission(ctx, spaceID, userID, PermRoomJoin)
}

// ============================================================================
// Room-Scoped Permissions
// ============================================================================

// CanPostMessage checks if a user can post new root messages in a specific room.
// Uses room-level permission resolution (checks room overrides, then space defaults).
func (c *ChattoCore) CanPostMessage(ctx context.Context, userID, spaceID, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, spaceID, roomID, userID, PermMessagePost)
}

// CanPostInThread checks if a user can post messages in a thread.
// Uses room-level permission resolution (checks room overrides, then space defaults).
func (c *ChattoCore) CanPostInThread(ctx context.Context, userID, spaceID, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, spaceID, roomID, userID, PermMessagePostInThread)
}

// CanReply checks if a user can use reply attribution (inReplyTo) on room-level messages.
// Uses room-level permission resolution (checks room overrides, then space defaults).
func (c *ChattoCore) CanReply(ctx context.Context, userID, spaceID, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, spaceID, roomID, userID, PermMessageReply)
}

// CanReplyInThread checks if a user can use reply attribution (inReplyTo) on thread messages.
// Uses room-level permission resolution (checks room overrides, then space defaults).
func (c *ChattoCore) CanReplyInThread(ctx context.Context, userID, spaceID, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, spaceID, roomID, userID, PermMessageReplyInThread)
}

// CanReactToMessage checks if a user can add/remove reactions in a specific room.
func (c *ChattoCore) CanReactToMessage(ctx context.Context, userID, spaceID, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, spaceID, roomID, userID, PermMessageReact)
}

// CanEchoMessage checks if a user can echo thread replies to the main channel.
func (c *ChattoCore) CanEchoMessage(ctx context.Context, userID, spaceID, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, spaceID, roomID, userID, PermMessageEcho)
}

// CanEditOwnMessage checks if a user can edit their own messages in a specific room.
func (c *ChattoCore) CanEditOwnMessage(ctx context.Context, userID, spaceID, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, spaceID, roomID, userID, PermMessageEditOwn)
}

// CanEditAnyMessage checks if a user can edit any user's messages in a specific room.
func (c *ChattoCore) CanEditAnyMessage(ctx context.Context, userID, spaceID, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, spaceID, roomID, userID, PermMessageEditAny)
}

// CanDeleteOwnMessage checks if a user can delete their own messages in a specific room.
func (c *ChattoCore) CanDeleteOwnMessage(ctx context.Context, userID, spaceID, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, spaceID, roomID, userID, PermMessageDeleteOwn)
}

// CanDeleteAnyMessage checks if a user can delete any user's messages in a specific room.
func (c *ChattoCore) CanDeleteAnyMessage(ctx context.Context, userID, spaceID, roomID string) (bool, error) {
	return c.hasRoomPermission(ctx, spaceID, roomID, userID, PermMessageDeleteAny)
}


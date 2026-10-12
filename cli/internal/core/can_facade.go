package core

import (
	"context"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// CanAdminUsersView delegates to shared authorization policy.
func (c *ChattoCore) CanAdminUsersView(ctx context.Context, userID string) (bool, error) {
	return c.authorization.CanAdminUsersView(ctx, userID)
}

// CanAssignRoles delegates to shared authorization policy.
func (c *ChattoCore) CanAssignRoles(ctx context.Context, userID string) (bool, error) {
	return c.authorization.CanAssignRoles(ctx, userID)
}

// CanManageRoles delegates to shared authorization policy.
func (c *ChattoCore) CanManageRoles(ctx context.Context, userID string) (bool, error) {
	return c.authorization.CanManageRoles(ctx, userID)
}

// CanAdminSystemView delegates to shared authorization policy.
func (c *ChattoCore) CanAdminSystemView(ctx context.Context, userID string) (bool, error) {
	return c.authorization.CanAdminSystemView(ctx, userID)
}

// CanAdminAuditView delegates to shared authorization policy.
func (c *ChattoCore) CanAdminAuditView(ctx context.Context, userID string) (bool, error) {
	return c.authorization.CanAdminAuditView(ctx, userID)
}

// CanManageUserPermissions delegates to shared authorization policy.
func (c *ChattoCore) CanManageUserPermissions(ctx context.Context, userID string) (bool, error) {
	return c.authorization.CanManageUserPermissions(ctx, userID)
}

// CanManageUserAccounts delegates to shared authorization policy.
func (c *ChattoCore) CanManageUserAccounts(ctx context.Context, userID string) (bool, error) {
	return c.authorization.CanManageUserAccounts(ctx, userID)
}

// CanCreateBots delegates to shared authorization policy.
func (c *ChattoCore) CanCreateBots(ctx context.Context, userID string) (bool, error) {
	return c.authorization.CanCreateBots(ctx, userID)
}

// CanManageBots delegates to shared authorization policy.
func (c *ChattoCore) CanManageBots(ctx context.Context, userID string) (bool, error) {
	return c.authorization.CanManageBots(ctx, userID)
}

// CanStartDM delegates to shared authorization policy.
func (c *ChattoCore) CanStartDM(ctx context.Context, userID string) (bool, error) {
	return c.authorization.CanStartDM(ctx, userID)
}

// CanDeleteUser delegates to shared authorization policy.
func (c *ChattoCore) CanDeleteUser(ctx context.Context, actorID, targetUserID string) (bool, error) {
	return c.authorization.CanDeleteUser(ctx, actorID, targetUserID)
}

// HasAnyAdminPermission delegates to shared authorization policy.
func (c *ChattoCore) HasAnyAdminPermission(ctx context.Context, userID string) (bool, error) {
	return c.authorization.HasAnyAdminPermission(ctx, userID)
}

// HasAnyPrivilegedModeEntitlement delegates to shared authorization policy.
func (c *ChattoCore) HasAnyPrivilegedModeEntitlement(ctx context.Context, userID string) (bool, error) {
	return c.authorization.HasAnyPrivilegedModeEntitlement(ctx, userID)
}

// CanManageServer delegates to shared authorization policy.
func (c *ChattoCore) CanManageServer(ctx context.Context, userID string) (bool, error) {
	return c.authorization.CanManageServer(ctx, userID)
}

// CanManageAnyRoom delegates to shared authorization policy.
func (c *ChattoCore) CanManageAnyRoom(ctx context.Context, userID string) (bool, error) {
	return c.authorization.CanManageAnyRoom(ctx, userID)
}

// CanManageRoomGroup delegates to shared authorization policy.
func (c *ChattoCore) CanManageRoomGroup(ctx context.Context, userID, groupID string) (bool, error) {
	return c.authorization.CanManageRoomGroup(ctx, userID, groupID)
}

// CanSeeRoom delegates to shared authorization policy.
func (c *ChattoCore) CanSeeRoom(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.authorization.CanSeeRoom(ctx, userID, kind, roomID)
}

// CanCreateRoom delegates to shared authorization policy.
func (c *ChattoCore) CanCreateRoom(ctx context.Context, userID string, kind RoomKind, groupID string) (bool, error) {
	return c.authorization.CanCreateRoom(ctx, userID, kind, groupID)
}

// CanJoinRoom delegates to shared authorization policy.
func (c *ChattoCore) CanJoinRoom(ctx context.Context, userID string, kind RoomKind) (bool, error) {
	return c.authorization.CanJoinRoom(ctx, userID, kind)
}

// CanJoinRoomAt delegates to shared authorization policy.
func (c *ChattoCore) CanJoinRoomAt(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.authorization.CanJoinRoomAt(ctx, userID, kind, roomID)
}

// CanReadMessages delegates to shared authorization policy.
func (c *ChattoCore) CanReadMessages(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.authorization.CanReadMessages(ctx, userID, kind, roomID)
}

// CanReadMessageInteractions delegates to shared authorization policy.
func (c *ChattoCore) CanReadMessageInteractions(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.authorization.CanReadMessageInteractions(ctx, userID, kind, roomID)
}

// CanAccessRoomMessages delegates to shared authorization policy.
func (c *ChattoCore) CanAccessRoomMessages(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.authorization.CanAccessRoomMessages(ctx, userID, kind, roomID)
}

// CanReadThreadMessages delegates to shared authorization policy.
func (c *ChattoCore) CanReadThreadMessages(ctx context.Context, userID string, kind RoomKind, roomID, threadRootEventID string) (bool, error) {
	return c.authorization.CanReadThreadMessages(ctx, userID, kind, roomID, threadRootEventID)
}

// CanReadMessage delegates to shared authorization policy.
func (c *ChattoCore) CanReadMessage(ctx context.Context, userID string, kind RoomKind, roomID, messageEventID string) (bool, error) {
	return c.authorization.CanReadMessage(ctx, userID, kind, roomID, messageEventID)
}

// CanReadMessageEvent delegates to shared authorization policy.
func (c *ChattoCore) CanReadMessageEvent(ctx context.Context, userID string, kind RoomKind, roomID string, event *evtv1.Event) (bool, error) {
	return c.authorization.CanReadMessageEvent(ctx, userID, kind, roomID, event)
}

// CanPostMessage delegates to shared authorization policy.
func (c *ChattoCore) CanPostMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.authorization.CanPostMessage(ctx, userID, kind, roomID)
}

// CanPostInThread delegates to shared authorization policy.
func (c *ChattoCore) CanPostInThread(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.authorization.CanPostInThread(ctx, userID, kind, roomID)
}

// CanReplyInThread delegates to shared authorization policy.
func (c *ChattoCore) CanReplyInThread(ctx context.Context, userID string, kind RoomKind, roomID, threadRootEventID string) (bool, error) {
	return c.authorization.CanReplyInThread(ctx, userID, kind, roomID, threadRootEventID)
}

// CanAttachFiles delegates to shared authorization policy.
func (c *ChattoCore) CanAttachFiles(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.authorization.CanAttachFiles(ctx, userID, kind, roomID)
}

// CanReactToMessage delegates to shared authorization policy.
func (c *ChattoCore) CanReactToMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.authorization.CanReactToMessage(ctx, userID, kind, roomID)
}

// CanEchoMessage delegates to shared authorization policy.
func (c *ChattoCore) CanEchoMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.authorization.CanEchoMessage(ctx, userID, kind, roomID)
}

// CanManageOthersMessage delegates to shared authorization policy.
func (c *ChattoCore) CanManageOthersMessage(ctx context.Context, userID string, kind RoomKind, roomID string) (bool, error) {
	return c.authorization.CanManageOthersMessage(ctx, userID, kind, roomID)
}

package core

import (
	"context"

	"github.com/chattocorp/chatto/cli/internal/readmodel"
)

// RoomMembershipExists delegates to AuthorizationModel, which owns the
// effective-membership check.
func (c *ChattoCore) RoomMembershipExists(ctx context.Context, kind RoomKind, user_id, room_id string) (bool, error) {
	return c.authorization.RoomMembershipExists(ctx, kind, user_id, room_id)
}

// JoinRoom delegates to the owning RoomMembershipModel.
func (c *ChattoCore) JoinRoom(ctx context.Context, actorID string, kind RoomKind, user_id, room_id string) (*readmodel.RoomMembership, error) {
	return c.roomMemberships.JoinRoom(ctx, actorID, kind, user_id, room_id)
}

// AddMember delegates to the owning RoomMembershipModel.
func (c *ChattoCore) AddMember(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID string) (*readmodel.RoomMembership, error) {
	return c.roomMemberships.AddMember(ctx, actorID, kind, roomID, targetUserID)
}

// LeaveRoom delegates to the owning RoomMembershipModel.
func (c *ChattoCore) LeaveRoom(ctx context.Context, actorID string, kind RoomKind, user_id, room_id string) error {
	return c.roomMemberships.LeaveRoom(ctx, actorID, kind, user_id, room_id)
}

// deleteUserRoomMembershipsInSpace delegates to the owning RoomMembershipModel.
func (c *ChattoCore) deleteUserRoomMembershipsInSpace(ctx context.Context, user_id string, kind RoomKind) error {
	return c.roomMemberships.deleteUserRoomMembershipsInSpace(ctx, user_id, kind)
}

// GetRoomMembersList delegates to the owning RoomMembershipModel.
func (c *ChattoCore) GetRoomMembersList(ctx context.Context, kind RoomKind, room_id string) ([]*readmodel.RoomMembership, error) {
	return c.roomMemberships.GetRoomMembersList(ctx, kind, room_id)
}

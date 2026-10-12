package core

import (
	"context"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

// GetRoomMembership delegates to the owning RoomMembershipModel.
func (c *ChattoCore) GetRoomMembership(ctx context.Context, kind RoomKind, user_id, room_id string) (*evtv1.RoomMembership, error) {
	return c.roomMemberships.GetRoomMembership(ctx, kind, user_id, room_id)
}

// RoomMembershipExists delegates to the owning RoomMembershipModel.
func (c *ChattoCore) RoomMembershipExists(ctx context.Context, kind RoomKind, user_id, room_id string) (bool, error) {
	return c.roomMemberships.RoomMembershipExists(ctx, kind, user_id, room_id)
}

// JoinRoom delegates to the owning RoomMembershipModel.
func (c *ChattoCore) JoinRoom(ctx context.Context, actorID string, kind RoomKind, user_id, room_id string) (*evtv1.RoomMembership, error) {
	return c.roomMemberships.JoinRoom(ctx, actorID, kind, user_id, room_id)
}

// AddMember delegates to the owning RoomMembershipModel.
func (c *ChattoCore) AddMember(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID string) (*evtv1.RoomMembership, error) {
	return c.roomMemberships.AddMember(ctx, actorID, kind, roomID, targetUserID)
}

// LeaveRoom delegates to the owning RoomMembershipModel.
func (c *ChattoCore) LeaveRoom(ctx context.Context, actorID string, kind RoomKind, user_id, room_id string) error {
	return c.roomMemberships.LeaveRoom(ctx, actorID, kind, user_id, room_id)
}

// RemoveMember delegates to the owning RoomMembershipModel.
func (c *ChattoCore) RemoveMember(ctx context.Context, actorID string, kind RoomKind, roomID, targetUserID string) (bool, error) {
	return c.roomMemberships.RemoveMember(ctx, actorID, kind, roomID, targetUserID)
}

// GetUserRoomMemberships delegates to the owning RoomMembershipModel.
func (c *ChattoCore) GetUserRoomMemberships(ctx context.Context, kind RoomKind, user_id string) ([]*evtv1.RoomMembership, error) {
	return c.roomMemberships.GetUserRoomMemberships(ctx, kind, user_id)
}

// deleteUserRoomMembershipsInSpace delegates to the owning RoomMembershipModel.
func (c *ChattoCore) deleteUserRoomMembershipsInSpace(ctx context.Context, user_id string, kind RoomKind) error {
	return c.roomMemberships.deleteUserRoomMembershipsInSpace(ctx, user_id, kind)
}

// GetRoomMembersList delegates to the owning RoomMembershipModel.
func (c *ChattoCore) GetRoomMembersList(ctx context.Context, kind RoomKind, room_id string) ([]*evtv1.RoomMembership, error) {
	return c.roomMemberships.GetRoomMembersList(ctx, kind, room_id)
}

// ListRoomMemberReferences delegates to the owning RoomMembershipModel.
func (c *ChattoCore) ListRoomMemberReferences(ctx context.Context, actorID, roomID string) ([]*evtv1.User, error) {
	return c.roomMemberships.ListRoomMemberReferences(ctx, actorID, roomID)
}

// ListRoomMemberReferencesForList delegates to the owning RoomMembershipModel.
func (c *ChattoCore) ListRoomMemberReferencesForList(ctx context.Context, actorID, roomID string) ([]*evtv1.User, error) {
	return c.roomMemberships.ListRoomMemberReferencesForList(ctx, actorID, roomID)
}

// ListRoomMemberIDsForList delegates to the owning RoomMembershipModel.
func (c *ChattoCore) ListRoomMemberIDsForList(ctx context.Context, actorID, roomID string) ([]string, error) {
	return c.roomMemberships.ListRoomMemberIDsForList(ctx, actorID, roomID)
}

// ListActiveRoomMemberIDs delegates to the owning RoomMembershipModel.
func (c *ChattoCore) ListActiveRoomMemberIDs(ctx context.Context, actorID, roomID string) ([]string, error) {
	return c.roomMemberships.ListActiveRoomMemberIDs(ctx, actorID, roomID)
}

// ListRoomMemberReferencesForLookup delegates to the owning RoomMembershipModel.
func (c *ChattoCore) ListRoomMemberReferencesForLookup(ctx context.Context, actorID, roomID string) ([]*evtv1.User, error) {
	return c.roomMemberships.ListRoomMemberReferencesForLookup(ctx, actorID, roomID)
}

// GetRoomMemberReferencesForLookup delegates to the owning RoomMembershipModel.
func (c *ChattoCore) GetRoomMemberReferencesForLookup(ctx context.Context, actorID, roomID string, userIDs []string) ([]*evtv1.User, error) {
	return c.roomMemberships.GetRoomMemberReferencesForLookup(ctx, actorID, roomID, userIDs)
}

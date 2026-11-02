package core

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// The running core supplies infrastructure and projectors only. These owners
// are assembled separately, without core callbacks for membership or layout.
func TestRoomOwnersShareProjectedState(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := c.CreateUser(ctx, SystemActorID, "owned-membership", "Member", "password")
	require.NoError(t, err)
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "owned-room", "")
	require.NoError(t, err)
	policy := newAuthorizationModel(
		c.roomModel, c.userModel, c.rbacModel,
		NewPermissionResolver(c.roomModel, c.userModel, c.rbacModel, c.contentView),
		c.EventPublisher, c.contentView, c,
	)
	readState := newReadStateModel(c.readStateModel.index, c.roomModel, policy, c, c.notificationOccurrences, c.logger)
	memberships := newRoomMembershipModel(
		c.roomModel, c.userModel, c.EventPublisher, policy,
		readState, c.callModel, c.notificationMaterializer, c.logger,
	)
	layout := newRoomLayoutModel(c.roomModel, c.EventPublisher, policy, c.logger)

	_, err = memberships.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	require.NoError(t, err)
	joined, err := c.RoomMembershipExists(ctx, KindChannel, user.Id, room.Id)
	require.NoError(t, err)
	require.True(t, joined)
	_, initialized, err := readState.PeekLastReadEventID(ctx, user.Id, room.Id)
	require.NoError(t, err)
	require.True(t, initialized)

	// Public layout authorization must survive extraction, including denials.
	_, err = layout.AdminCreateRoomGroup(ctx, user.Id, "Denied", "")
	require.ErrorIs(t, err, ErrPermissionDenied)
	manager, err := c.CreateUser(ctx, SystemActorID, "owned-manager", "Manager", "password")
	require.NoError(t, err)
	require.NoError(t, c.AssignServerRole(ctx, SystemActorID, manager.Id, RoleAdmin))
	group, err := layout.AdminCreateRoomGroup(ctx, manager.Id, "Owned group", "")
	require.NoError(t, err)
	_, err = layout.AdminMoveRoomToGroup(ctx, manager.Id, room.Id, group.Id)
	require.NoError(t, err)
	projected, err := c.GetRoomGroup(ctx, group.Id)
	require.NoError(t, err)
	require.Contains(t, projected.RoomIds, room.Id)

	_, err = memberships.BanMember(ctx, SystemActorID, KindChannel, room.Id, user.Id, "test removal", nil)
	require.NoError(t, err)
	_, err = memberships.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	require.ErrorIs(t, err, ErrPermissionDenied)
	require.NoError(t, memberships.UnbanMember(ctx, SystemActorID, KindChannel, room.Id, user.Id, "test restore"))
	_, err = memberships.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	require.NoError(t, err)
	require.NoError(t, memberships.LeaveRoom(ctx, user.Id, KindChannel, user.Id, room.Id))
	joined, err = c.RoomMembershipExists(ctx, KindChannel, user.Id, room.Id)
	require.NoError(t, err)
	require.False(t, joined)

	commands := &RoomCommandModel{rooms: c.roomModel, publisher: c.EventPublisher, logger: c.logger}
	require.NoError(t, commands.DeleteRoom(ctx, SystemActorID, KindChannel, room.Id))
	projected, err = c.GetRoomGroup(ctx, group.Id)
	require.NoError(t, err)
	require.NotContains(t, projected.RoomIds, room.Id)
}

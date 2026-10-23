package connectapi

import (
	"testing"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"github.com/stretchr/testify/require"
)

func TestRoomDirectoryRejoinPresence(t *testing.T) {
	for _, allowed := range []bool{false, true} {
		view := apiRoomWithViewerState(&core.DirectoryRoom{
			ViewerState: core.DirectoryRoomViewerState{CanRejoin: allowed},
		})
		require.NotNil(t, view.ViewerState.CanRejoin)
		require.Equal(t, allowed, view.ViewerState.GetCanRejoin())
	}
}

func TestRoomInvitationServicePrivacyAndDecline(t *testing.T) {
	env := newConnectAPITestEnv(t)
	s := &roomInvitationService{api: env.api}
	ctx := withCaller(env.ctx, env.viewer)
	recipient, err := env.core.CreateUser(env.ctx, core.SystemActorID, "invite-target", "Invite Target", "password123")
	require.NoError(t, err)
	other, err := env.core.CreateUser(env.ctx, core.SystemActorID, "invite-other", "Other", "password123")
	require.NoError(t, err)
	room, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, "", "invited", "Invited")
	require.NoError(t, err)
	_, err = env.core.JoinRoom(env.ctx, env.viewer.Id, core.KindChannel, env.viewer.Id, room.Id)
	require.NoError(t, err)
	require.NoError(t, env.core.GrantUserRoomPermission(env.ctx, core.SystemActorID, room.Id, env.viewer.Id, core.PermRoomInvite))
	require.NoError(t, env.core.DenyRoomPermission(env.ctx, core.SystemActorID, room.Id, core.RoleEveryone, core.PermRoomList))
	created, err := s.CreateRoomInvitation(ctx, connect.NewRequest(&apiv1.CreateRoomInvitationRequest{RoomId: room.Id, RecipientId: recipient.Id}))
	require.NoError(t, err)
	id := created.Msg.Invitation.Id
	_, err = s.GetRoomInvitation(withCaller(env.ctx, other), connect.NewRequest(&apiv1.GetRoomInvitationRequest{InvitationId: id}))
	require.Equal(t, connect.CodeNotFound, errorCode(err))
	got, err := s.GetRoomInvitation(withCaller(env.ctx, recipient), connect.NewRequest(&apiv1.GetRoomInvitationRequest{InvitationId: id}))
	require.NoError(t, err)
	require.True(t, got.Msg.Invitation.CanAccept)
	declined, err := s.DeclineRoomInvitation(withCaller(env.ctx, recipient), connect.NewRequest(&apiv1.DeclineRoomInvitationRequest{InvitationId: id}))
	require.NoError(t, err)
	require.Equal(t, apiv1.RoomInvitationStatus_ROOM_INVITATION_STATUS_DECLINED, declined.Msg.Invitation.Status)
	_, err = env.directory.GetRoom(withCaller(env.ctx, recipient), connect.NewRequest(&apiv1.GetRoomRequest{RoomId: room.Id}))
	require.Equal(t, connect.CodePermissionDenied, errorCode(err))
	list, err := s.ListRoomInvitations(ctx, connect.NewRequest(&apiv1.ListRoomInvitationsRequest{Sent: true}))
	require.NoError(t, err)
	require.Len(t, list.Msg.Invitations, 1)
	_, err = s.GetRoomInvitation(env.ctx, connect.NewRequest(&apiv1.GetRoomInvitationRequest{InvitationId: id}))
	require.Equal(t, connect.CodeUnauthenticated, errorCode(err))
}

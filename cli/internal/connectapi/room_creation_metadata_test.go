package connectapi

import (
	"testing"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestAPIRoomCreationMetadata(t *testing.T) {
	t.Parallel()
	room := apiRoom(&readmodel.Room{CreatedAt: &timestamppb.Timestamp{Seconds: 123}})
	require.Equal(t, int64(123), room.GetCreatedAt().GetSeconds())
	require.Nil(t, apiRoom(&readmodel.Room{}).CreatedAt)
}

func TestRoomInformationMemberCountIncludesUniversalMembers(t *testing.T) {
	t.Parallel()
	env := newConnectAPITestEnv(t)
	room, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, "", "universal-count", "", core.WithUniversalRoom(true))
	require.NoError(t, err)
	ctx := withCaller(env.ctx, env.viewer)
	read := func() *apiv1.ListMembersResponse {
		t.Helper()
		response, err := env.rooms.ListMembers(ctx, connect.NewRequest(&apiv1.ListMembersRequest{RoomId: room.Id}))
		require.NoError(t, err)
		return response.Msg
	}
	before := read().GetPage().GetTotalCount()
	member, err := env.core.CreateUser(env.ctx, core.SystemActorID, "universal-count-member", "Universal Member", "password")
	require.NoError(t, err)
	// No explicit join: universal membership alone must increase the count.
	after := read()
	require.Equal(t, before+1, after.GetPage().GetTotalCount())
	require.Contains(t, after.GetUserIds(), member.Id)
	require.NoError(t, env.core.DenyUserRoomPermission(env.ctx, core.SystemActorID, room.Id, member.Id, core.PermRoomJoin))
	after = read()
	require.Equal(t, before, after.GetPage().GetTotalCount())
	require.NotContains(t, after.GetUserIds(), member.Id)
}

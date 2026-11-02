package connectapi

import (
	"testing"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"github.com/stretchr/testify/require"
)

func TestRoomNoticeAPIAndDirectoryPrivacy(t *testing.T) {
	t.Parallel()
	env := newConnectAPITestEnv(t)
	room, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, "", "notice-api", "", core.WithUniversalRoom(true))
	require.NoError(t, err)
	ctx := withCaller(env.ctx, env.viewer)
	read, err := env.rooms.GetNotice(ctx, connect.NewRequest(&apiv1.GetNoticeRequest{RoomId: room.Id}))
	require.NoError(t, err)
	require.Empty(t, read.Msg.Notice.Body)
	require.NoError(t, env.core.GrantUserRoomPermission(env.ctx, core.SystemActorID, room.Id, env.viewer.Id, core.PermRoomManageNotice))
	published, err := env.rooms.PublishNotice(ctx, connect.NewRequest(&apiv1.PublishNoticeRequest{RoomId: room.Id, Body: "Only members"}))
	require.NoError(t, err)
	require.NotEmpty(t, published.Msg.Notice.PublicationId)
	require.NotNil(t, published.Msg.Notice.PublishedAt)
	_, err = env.rooms.ClearNotice(ctx, connect.NewRequest(&apiv1.ClearNoticeRequest{RoomId: room.Id, ExpectedVersion: "stale"}))
	require.Equal(t, connect.CodeAborted, errorCode(err))
	cleared, err := env.rooms.ClearNotice(ctx, connect.NewRequest(&apiv1.ClearNoticeRequest{RoomId: room.Id, ExpectedVersion: published.Msg.Notice.Version}))
	require.NoError(t, err)
	require.Empty(t, cleared.Msg.Notice.Body)
	require.Nil(t, cleared.Msg.Notice.PublishedAt)
	require.NotEmpty(t, cleared.Msg.Notice.Version)
	// Generic room assembly must never copy member-only notice content.
	public := apiRoom(&readmodel.Room{Id: room.Id, Notice: &readmodel.RoomNotice{Body: "private"}})
	require.NotContains(t, public.String(), "private")
}

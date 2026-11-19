package connectapi

import (
	"fmt"
	"testing"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"github.com/stretchr/testify/require"
)

func TestThreadParticipantPreviewSupportsWideHeaders(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("participant-preview")
	root := env.post(room.Id, env.viewer.Id, "root", "")
	for i := range 14 {
		user, err := env.core.CreateUser(env.ctx, core.SystemActorID, fmt.Sprintf("preview-user-%d", i), "Preview user", "")
		require.NoError(t, err)
		_, err = env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id)
		require.NoError(t, err)
		env.post(room.Id, user.Id, "reply", root.Id)
	}
	ctx := withCaller(env.ctx, env.viewer)
	response, err := env.rooms.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{RoomId: room.Id, Limit: 100}))
	require.NoError(t, err)
	event := timelinePageEvent(response.Msg.Page, root.Id)
	require.NotNil(t, event)
	thread := event.GetMessagePosted().GetMessage().GetThread()
	require.NotNil(t, thread)
	require.EqualValues(t, 14, thread.ParticipantCount)
	require.Len(t, thread.ParticipantPreviewUserIds, 12)
	require.NotContains(t, thread.ParticipantPreviewUserIds, env.viewer.Id)
	require.NoError(t, env.core.FollowThread(env.ctx, core.KindChannel, env.viewer.Id, room.Id, root.Id))
	followed, err := env.threads.ListFollowedThreads(ctx, connect.NewRequest(&apiv1.ListFollowedThreadsRequest{}))
	require.NoError(t, err)
	require.Len(t, followed.Msg.Threads, 1)
	require.EqualValues(t, 14, followed.Msg.Threads[0].Thread.ParticipantCount)
	require.Equal(t, thread.ParticipantPreviewUserIds, followed.Msg.Threads[0].Thread.ParticipantPreviewUserIds)
	// A root author who replies after the preview fills must still appear first.
	env.post(room.Id, env.viewer.Id, "root author reply", root.Id)
	response, err = env.rooms.GetRoomEvents(ctx, connect.NewRequest(&apiv1.GetRoomEventsRequest{RoomId: room.Id, Limit: 100}))
	require.NoError(t, err)
	thread = timelinePageEvent(response.Msg.Page, root.Id).GetMessagePosted().GetMessage().GetThread()
	require.EqualValues(t, 15, thread.ParticipantCount)
	require.Len(t, thread.ParticipantPreviewUserIds, 12)
	require.Equal(t, env.viewer.Id, thread.ParticipantPreviewUserIds[0])
}

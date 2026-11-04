package connectapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	"github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1/apiv1connect"
	"github.com/stretchr/testify/require"
)

func TestRoomGroupArchiveFiltersThroughMountedHandlers(t *testing.T) {
	env := newConnectAPITestEnv(t)
	require.NoError(t, env.core.AssignOwnerRole(env.ctx, env.viewer.Id))
	group, err := env.core.RoomLayout().CreateRoomGroup(env.ctx, core.SystemActorID, "Archive filter", "")
	require.NoError(t, err)
	room, err := env.core.CreateRoom(env.ctx, core.SystemActorID, core.KindChannel, group.Id, "Archived", "")
	require.NoError(t, err)
	_, err = env.core.ArchiveRoom(env.ctx, core.SystemActorID, core.KindChannel, room.Id)
	require.NoError(t, err)
	mux := http.NewServeMux()
	for _, handler := range env.api.Handlers() {
		mux.Handle(handler.ServicePath, handler.Handler)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(withCaller(r.Context(), env.viewer)))
	}))
	t.Cleanup(server.Close)
	client := apiv1connect.NewRoomDirectoryServiceClient(server.Client(), server.URL, connect.WithProtoJSON())
	read, err := client.GetRoomGroup(env.ctx, connect.NewRequest(&apiv1.GetRoomGroupRequest{GroupId: group.Id, ArchiveFilter: apiv1.RoomArchiveFilter_ROOM_ARCHIVE_FILTER_ALL}))
	require.NoError(t, err)
	require.Len(t, read.Msg.GetGroup().GetItems(), 1)
	_, err = client.GetRoomGroup(env.ctx, connect.NewRequest(&apiv1.GetRoomGroupRequest{GroupId: group.Id, ArchiveFilter: apiv1.RoomArchiveFilter(99)}))
	require.Equal(t, connect.CodeInvalidArgument, errorCode(err))
}

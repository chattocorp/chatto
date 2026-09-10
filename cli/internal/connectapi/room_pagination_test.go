package connectapi

import (
	"fmt"
	"slices"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestRoomDirectoryPagination(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ctx := withCaller(env.ctx, env.viewer)
	for i := 0; i < 103; i++ {
		env.createJoinedRoom(fmt.Sprintf("paged-room-%03d", i))
	}
	request := func(limit, offset int32) *apiv1.ListRoomsResponse {
		t.Helper()
		response, err := env.directory.ListRooms(ctx, connect.NewRequest(&apiv1.ListRoomsRequest{Scope: apiv1.RoomDirectoryScope_ROOM_DIRECTORY_SCOPE_CHANNELS, Page: &apiv1.PageRequest{Limit: limit, Offset: offset}}))
		require.NoError(t, err)
		return response.Msg
	}
	first := request(0, 0)
	require.Len(t, first.Rooms, 50)
	require.True(t, first.Page.HasMore)
	require.GreaterOrEqual(t, first.Page.TotalCount, int64(103))
	capped := request(500, 0)
	require.Len(t, capped.Rooms, 100)
	var ids []string
	for offset := int32(0); ; {
		page := request(37, offset)
		require.Equal(t, first.Page.TotalCount, page.Page.TotalCount)
		for _, entry := range page.Rooms {
			ids = append(ids, entry.Room.Id)
		}
		offset += int32(len(page.Rooms))
		if !page.Page.HasMore {
			break
		}
	}
	require.Len(t, ids, int(first.Page.TotalCount))
	require.True(t, slices.IsSorted(ids))
	require.Equal(t, len(ids), len(slices.Compact(slices.Clone(ids))))
	beyond := request(50, int32(first.Page.TotalCount)+1)
	require.Empty(t, beyond.Rooms)
	require.False(t, beyond.Page.HasMore)
	require.Equal(t, first.Page.TotalCount, beyond.Page.TotalCount)
}

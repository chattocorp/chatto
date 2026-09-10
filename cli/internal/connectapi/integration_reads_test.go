package connectapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"connectrpc.com/connect"
	"github.com/stretchr/testify/require"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestIntegrationRelationshipReads(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("relationship-reads")
	root := env.post(room.Id, env.viewer.Id, "root", "")
	ctx := withCaller(env.ctx, env.viewer)
	var ids []string
	for i := 0; i < 7; i++ {
		user, err := env.core.CreateUser(env.ctx, core.SystemActorID, fmt.Sprintf("relationship-reader-%d", i), "Reader", "password")
		require.NoError(t, err)
		_, err = env.core.JoinRoom(env.ctx, user.Id, core.KindChannel, user.Id, room.Id)
		require.NoError(t, err)
		ids = append(ids, user.Id)
		env.post(room.Id, user.Id, "reply", root.Id)
		_, err = env.messages.AddReaction(withCaller(env.ctx, user), connect.NewRequest(&apiv1.AddReactionRequest{RoomId: room.Id, MessageEventId: root.Id, Emoji: "thumbsup"}))
		require.NoError(t, err)
	}
	slices.Sort(ids)
	for _, offset := range []int32{0, 4, 7} {
		page := &apiv1.PageRequest{Limit: 4, Offset: offset}
		reactions, err := env.messages.ListReactionUsers(ctx, connect.NewRequest(&apiv1.ListReactionUsersRequest{RoomId: room.Id, MessageEventId: root.Id, Emoji: "thumbsup", Page: page}))
		require.NoError(t, err)
		participants, err := env.threads.ListThreadParticipants(ctx, connect.NewRequest(&apiv1.ListThreadParticipantsRequest{RoomId: room.Id, ThreadRootEventId: root.Id, Page: page}))
		require.NoError(t, err)
		want := ids[int(offset):min(int(offset)+4, len(ids))]
		require.Equal(t, want, reactions.Msg.UserIds)
		require.Equal(t, want, participants.Msg.UserIds)
		require.EqualValues(t, 7, reactions.Msg.Page.TotalCount)
		require.EqualValues(t, 7, participants.Msg.Page.TotalCount)
		require.Equal(t, offset == 0, reactions.Msg.Page.HasMore)
		require.Equal(t, offset == 0, participants.Msg.Page.HasMore)
	}
	empty, err := env.messages.ListReactionUsers(ctx, connect.NewRequest(&apiv1.ListReactionUsersRequest{RoomId: room.Id, MessageEventId: root.Id, Emoji: "heart"}))
	require.NoError(t, err)
	require.Empty(t, empty.Msg.UserIds)
	outsider, err := env.core.CreateUser(env.ctx, core.SystemActorID, "relationship-outsider", "Outsider", "password")
	require.NoError(t, err)
	_, err = env.messages.ListReactionUsers(withCaller(env.ctx, outsider), connect.NewRequest(&apiv1.ListReactionUsersRequest{RoomId: room.Id, MessageEventId: root.Id, Emoji: "thumbsup"}))
	require.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
	_, err = env.threads.ListThreadParticipants(withCaller(env.ctx, outsider), connect.NewRequest(&apiv1.ListThreadParticipantsRequest{RoomId: room.Id, ThreadRootEventId: root.Id}))
	require.Equal(t, connect.CodePermissionDenied, connect.CodeOf(err))
}

func TestIntegrationReadMarkers(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room, err := env.core.CreateRoom(env.ctx, env.viewer.Id, core.KindChannel, "", "marker-reads", "", core.WithUniversalRoom(true))
	require.NoError(t, err)
	_, err = env.core.JoinRoom(env.ctx, env.viewer.Id, core.KindChannel, env.viewer.Id, room.Id)
	require.NoError(t, err)
	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply := env.post(room.Id, env.viewer.Id, "reply", root.Id)
	reader, err := env.core.CreateUser(env.ctx, core.SystemActorID, "marker-reader", "Reader", "password")
	require.NoError(t, err)
	ctx := withCaller(env.ctx, reader)
	roomReq := connect.NewRequest(&apiv1.GetRoomReadStateRequest{RoomId: room.Id})
	threadReq := connect.NewRequest(&apiv1.GetThreadReadStateRequest{RoomId: room.Id, ThreadRootEventId: root.Id})
	roomState, err := env.rooms.GetRoomReadState(ctx, roomReq)
	require.NoError(t, err)
	require.Nil(t, roomState.Msg.State.Marker)
	_, exists, err := env.core.PeekLastReadEventID(env.ctx, reader.Id, room.Id)
	require.NoError(t, err)
	require.False(t, exists, "reading must not initialize a marker")
	threadState, err := env.threads.GetThreadReadState(ctx, threadReq)
	require.NoError(t, err)
	require.Nil(t, threadState.Msg.State.Marker)
	_, err = env.rooms.MarkRoomAsRead(ctx, connect.NewRequest(&apiv1.MarkRoomAsReadRequest{RoomId: room.Id, UpToEventId: root.Id}))
	require.NoError(t, err)
	_, err = env.threads.MarkThreadAsRead(ctx, connect.NewRequest(&apiv1.MarkThreadAsReadRequest{RoomId: room.Id, ThreadRootEventId: root.Id, UpToEventId: reply.Id}))
	require.NoError(t, err)
	roomState, err = env.rooms.GetRoomReadState(ctx, roomReq)
	require.NoError(t, err)
	require.Equal(t, root.Id, roomState.Msg.State.Marker.LastReadEventId)
	require.NotNil(t, roomState.Msg.State.Marker.LastReadAt)
	threadState, err = env.threads.GetThreadReadState(ctx, threadReq)
	require.NoError(t, err)
	require.Equal(t, reply.Id, threadState.Msg.State.Marker.LastReadEventId)
	require.NotNil(t, threadState.Msg.State.Marker.LastReadAt)
	private := env.createJoinedRoom("marker-private")
	rooms, err := env.rooms.BatchGetRoomReadStates(ctx, connect.NewRequest(&apiv1.BatchGetRoomReadStatesRequest{RoomIds: []string{private.Id, room.Id, "missing", room.Id}}))
	require.NoError(t, err)
	require.Len(t, rooms.Msg.States, 1)
	require.Equal(t, room.Id, rooms.Msg.States[0].RoomId)
	target := &apiv1.ThreadReadStateTarget{RoomId: room.Id, ThreadRootEventId: root.Id}
	threads, err := env.threads.BatchGetThreadReadStates(ctx, connect.NewRequest(&apiv1.BatchGetThreadReadStatesRequest{Targets: []*apiv1.ThreadReadStateTarget{{RoomId: private.Id, ThreadRootEventId: root.Id}, target, {RoomId: room.Id, ThreadRootEventId: "missing"}, target}}))
	require.NoError(t, err)
	require.Len(t, threads.Msg.States, 1)
	require.Equal(t, reply.Id, threads.Msg.States[0].Marker.LastReadEventId)
}

func TestIntegrationReactionAliasesAndRetractions(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("relationship-alias")
	ctx := withCaller(env.ctx, env.viewer)
	root := env.post(room.Id, env.viewer.Id, "root", "")
	reply, err := env.core.PostMessage(env.ctx, core.KindChannel, room.Id, env.viewer.Id, "reply", nil, root.Id, root.Id, nil, true)
	require.NoError(t, err)
	echo, ok := env.core.ChannelEchoEventID(reply.Id)
	require.True(t, ok)
	_, err = env.messages.AddReaction(ctx, connect.NewRequest(&apiv1.AddReactionRequest{RoomId: room.Id, MessageEventId: echo, Emoji: "thumbsup"}))
	require.NoError(t, err)
	for _, id := range []string{reply.Id, echo} {
		result, err := env.messages.ListReactionUsers(ctx, connect.NewRequest(&apiv1.ListReactionUsersRequest{RoomId: room.Id, MessageEventId: id, Emoji: "thumbsup"}))
		require.NoError(t, err)
		require.Equal(t, []string{env.viewer.Id}, result.Msg.UserIds)
	}
	require.NoError(t, env.core.DeleteMessage(env.ctx, env.viewer.Id, core.KindChannel, room.Id, reply.Id))
	_, err = env.messages.ListReactionUsers(ctx, connect.NewRequest(&apiv1.ListReactionUsersRequest{RoomId: room.Id, MessageEventId: reply.Id, Emoji: "thumbsup"}))
	require.Equal(t, connect.CodeNotFound, connect.CodeOf(err))
	participants, err := env.threads.ListThreadParticipants(ctx, connect.NewRequest(&apiv1.ListThreadParticipantsRequest{RoomId: room.Id, ThreadRootEventId: root.Id}))
	require.NoError(t, err)
	require.Empty(t, participants.Msg.UserIds)
}

func TestIntegrationReadsThroughJSON(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("json-reads")
	root := env.post(room.Id, env.viewer.Id, "root", "")
	mux := http.NewServeMux()
	for _, handler := range env.api.Handlers() {
		mux.Handle(handler.ServicePath, handler.Handler)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mux.ServeHTTP(w, r.WithContext(withCaller(r.Context(), env.viewer)))
	}))
	t.Cleanup(server.Close)
	cases := []struct {
		method string
		body   any
		status int
	}{
		{"RoomDirectoryService/ListRooms", map[string]any{"page": map[string]any{"limit": 1}}, 200},
		{"RoomDirectoryService/ListRooms", map[string]any{"page": map[string]any{"limit": 501}}, 400},
		{"RoomDirectoryService/ListRooms", map[string]any{"page": map[string]any{"offset": -1}}, 400},
		{"MessageService/ListReactionUsers", map[string]any{"roomId": room.Id, "messageEventId": root.Id, "emoji": "heart"}, 200},
		{"ThreadService/ListThreadParticipants", map[string]any{"roomId": room.Id, "threadRootEventId": root.Id}, 200},
		{"RoomService/GetRoomReadState", map[string]any{"roomId": room.Id}, 200},
		{"ThreadService/GetThreadReadState", map[string]any{"roomId": room.Id, "threadRootEventId": root.Id}, 200},
		{"RoomService/BatchGetRoomReadStates", map[string]any{"roomIds": []string{room.Id}}, 200},
		{"ThreadService/BatchGetThreadReadStates", map[string]any{"targets": []any{map[string]any{"roomId": room.Id, "threadRootEventId": root.Id}}}, 200},
		{"RoomService/BatchGetRoomReadStates", map[string]any{"roomIds": slices.Repeat([]string{room.Id}, 101)}, 400},
		{"ThreadService/BatchGetThreadReadStates", map[string]any{"targets": []any{map[string]any{"roomId": room.Id}}}, 400},
		{"MessageService/ListReactionUsers", map[string]any{"roomId": room.Id, "messageEventId": root.Id, "emoji": "heart", "page": map[string]any{"limit": 501}}, 400},
		{"ThreadService/ListThreadParticipants", map[string]any{"roomId": room.Id, "threadRootEventId": root.Id, "page": map[string]any{"offset": -1}}, 400},
	}
	for _, tc := range cases {
		t.Run(tc.method+fmt.Sprint(tc.status), func(t *testing.T) {
			data, err := json.Marshal(tc.body)
			require.NoError(t, err)
			req, err := http.NewRequest(http.MethodPost, server.URL+"/chatto.api.v1."+tc.method, bytes.NewReader(data))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Connect-Protocol-Version", "1")
			resp, err := server.Client().Do(req)
			require.NoError(t, err)
			defer resp.Body.Close()
			require.Equal(t, tc.status, resp.StatusCode)
		})
	}
}

package core

import (
	"context"
	"fmt"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// limitCompoundBatch leaves space for only a prefix of a command. JetStream
// must reject the entire atomic batch without storing that prefix.
func limitCompoundBatch(t *testing.T, ctx context.Context, core *ChattoCore, slots int) {
	t.Helper()
	stream, err := core.js.Stream(ctx, "EVT")
	require.NoError(t, err)
	info, err := stream.Info(ctx)
	require.NoError(t, err)
	cfg := info.Config
	cfg.Discard = jetstream.DiscardNew
	cfg.MaxMsgs = int64(info.State.Msgs) + int64(slots)
	_, err = core.js.UpdateStream(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := core.js.UpdateStream(context.Background(), info.Config)
		require.NoError(t, err)
	})
}

func TestCompoundRoomUpdateRejectsEveryPartialBatch(t *testing.T) {
	for slots := 0; slots < 4; slots++ {
		t.Run(fmt.Sprint(slots), func(t *testing.T) {
			core, _ := setupTestCore(t)
			ctx := testContext(t)
			actor, err := core.CreateUser(ctx, SystemActorID, "compound-manager", "Manager", "password123")
			require.NoError(t, err)
			room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "before", "before")
			require.NoError(t, err)
			require.NoError(t, core.GrantUserRoomPermission(ctx, SystemActorID, room.Id, actor.Id, PermRoomManage))
			before, err := core.GetRoom(ctx, KindChannel, room.Id)
			require.NoError(t, err)
			filter := evtstream.RoomAggregate(room.Id).AllEventsFilter()
			beforeSeq, err := core.EventPublisher.LastSubjectSeq(ctx, filter)
			require.NoError(t, err)
			_, err = core.RoomCommands().updateRoom(ctx, RoomUpdateInput{
				ActorID: actor.Id, RoomID: room.Id, Name: proto.String("after"), Description: proto.String("after"),
				Universal: proto.Bool(true), SlowModeSeconds: proto.Uint32(30),
				ThreadingMode: evtv1.RoomThreadingMode_ROOM_THREADING_MODE_REQUIRED.Enum(),
			}, func(ctx context.Context) error {
				limitCompoundBatch(t, ctx, core, slots)
				return nil
			})
			require.Error(t, err)
			afterSeq, err := core.EventPublisher.LastSubjectSeq(ctx, filter)
			require.NoError(t, err)
			require.Equal(t, beforeSeq, afterSeq, "a rejected batch must not persist any room field")
			after, err := core.GetRoom(ctx, KindChannel, room.Id)
			require.NoError(t, err)
			require.True(t, proto.Equal(before, after))
		})
	}
}

func TestCompoundRoomUpdateRetriesAcrossReplicas(t *testing.T) {
	for _, scenario := range []string{"omitted-name", "name-collision", "permission-revoked"} {
		t.Run(scenario, func(t *testing.T) {
			first, nc := setupTestCore(t)
			ctx := testContext(t)
			actor, err := first.CreateUser(ctx, SystemActorID, "compound-manager", "Manager", "password123")
			require.NoError(t, err)
			room, err := first.CreateRoom(ctx, SystemActorID, KindChannel, "", "before", "before")
			require.NoError(t, err)
			require.NoError(t, first.GrantUserRoomPermission(ctx, SystemActorID, room.Id, actor.Id, PermRoomManage))
			concurrentManagerID := newRoomManagerForTest(t, ctx, first, "compound-concurrent-manager", room.Id)
			second, err := NewChattoCore(ctx, nc, first.config)
			require.NoError(t, err)
			startCoreServices(t, second)
			input := RoomUpdateInput{ActorID: actor.Id, RoomID: room.Id, Description: proto.String("after"),
				Universal: proto.Bool(true), SlowModeSeconds: proto.Uint32(30),
				ThreadingMode: evtv1.RoomThreadingMode_ROOM_THREADING_MODE_REQUIRED.Enum()}
			if scenario == "name-collision" {
				input.Name = proto.String("claimed")
			}
			attempts := 0
			updated, updateErr := first.RoomCommands().updateRoom(ctx, input, func(ctx context.Context) error {
				attempts++
				if attempts != 1 {
					return nil
				}
				if scenario == "name-collision" {
					_, err := second.CreateRoom(ctx, SystemActorID, KindChannel, "", "claimed", "")
					return err
				}
				if scenario == "permission-revoked" {
					if err := second.DenyUserRoomPermission(ctx, SystemActorID, room.Id, actor.Id, PermRoomManage); err != nil {
						return err
					}
				}
				_, err := second.RoomCommands().UpdateRoom(ctx, RoomUpdateInput{
					ActorID: concurrentManagerID, RoomID: room.Id, Name: proto.String("concurrent"),
				})
				return err
			})
			if scenario == "omitted-name" {
				require.NoError(t, updateErr)
				require.Equal(t, 2, attempts)
				require.Equal(t, "concurrent", updated.Name)
				require.Equal(t, "after", updated.Description)
				require.True(t, updated.Universal)
				require.EqualValues(t, 30, updated.SlowModeSeconds)
				require.Equal(t, *input.ThreadingMode, updated.ThreadingMode)
				var previous uint64
				for _, eventType := range []string{evtstream.EventRoomUpdated, evtstream.EventRoomUniversalChanged, evtstream.EventRoomSlowModeChanged, evtstream.EventRoomThreadingModeChanged} {
					seq, err := first.EventPublisher.LastSubjectSeq(ctx, evtstream.RoomAggregate(room.Id).Subject(eventType))
					require.NoError(t, err)
					if previous != 0 {
						require.Equal(t, previous+1, seq, "all changed fields must form one contiguous batch")
					}
					previous = seq
				}
				_, err = first.RoomCommands().UpdateRoom(ctx, input)
				require.NoError(t, err)
				seq, err := first.EventPublisher.LastSubjectSeq(ctx, evtstream.RoomAggregate(room.Id).AllEventsFilter())
				require.NoError(t, err)
				require.Equal(t, previous, seq, "an unchanged patch must not append facts")
				return
			}
			if scenario == "name-collision" {
				require.ErrorIs(t, updateErr, ErrRoomNameExists)
			} else {
				require.ErrorIs(t, updateErr, ErrPermissionDenied)
			}
			unchanged, err := first.GetRoom(ctx, KindChannel, room.Id)
			require.NoError(t, err)
			require.Equal(t, "before", unchanged.Description)
			require.False(t, unchanged.Universal)
			require.Zero(t, unchanged.SlowModeSeconds)
			require.Equal(t, evtv1.RoomThreadingMode_ROOM_THREADING_MODE_ENABLED, unchanged.ThreadingMode)
		})
	}
}

func TestCompoundReplyEchoDM(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := core.CreateUser(ctx, SystemActorID, "compound-dm-author", "Author", "password123")
	require.NoError(t, err)
	peer, err := core.CreateUser(ctx, SystemActorID, "compound-dm-peer", "Peer", "password123")
	require.NoError(t, err)
	room, _, err := core.FindOrCreateDM(ctx, author.Id, []string{peer.Id})
	require.NoError(t, err)
	root, err := core.Messages().PostMessage(ctx, MessagePostInput{ActorID: author.Id, RoomID: room.Id, Body: "root"})
	require.NoError(t, err)
	reply, err := core.Messages().PostMessage(ctx, MessagePostInput{ActorID: author.Id, RoomID: room.Id, Body: "reply", ThreadRootEventID: root.Event.Id, AlsoSendToChannel: true})
	require.NoError(t, err)
	echoID, exists := core.roomModel.channelEchoEventID(reply.Event.Id)
	require.True(t, exists)
	echo, err := core.GetRoomEventByEventID(ctx, KindDM, room.Id, echoID)
	require.NoError(t, err)
	require.Equal(t, reply.Event.Id, echo.GetMessagePosted().GetEchoOfEventId())
	meta, err := core.GetThreadMetadata(ctx, KindDM, room.Id, root.Event.Id)
	require.NoError(t, err)
	require.EqualValues(t, 1, meta.ReplyCount)
	agg := evtstream.RoomAggregate(room.Id)
	_, bodySeq, err := core.EventPublisher.SubjectEvents(ctx, agg.Subject(evtstream.EventMessageBody))
	require.NoError(t, err)
	posts, echoSeq, err := core.EventPublisher.SubjectEvents(ctx, agg.Subject(evtstream.EventMessagePosted))
	require.NoError(t, err)
	require.Len(t, posts, 3)
	require.Equal(t, bodySeq+2, echoSeq, "body, reply, and echo must be adjacent")
}

func TestCompoundReplyEchoRejectsEveryPartialBatch(t *testing.T) {
	for _, existingThread := range []bool{false, true} {
		batchSize := 4 // body, initial thread, reply, echo
		if existingThread {
			batchSize = 3
		}
		for slots := 0; slots < batchSize; slots++ {
			t.Run(fmt.Sprintf("existing=%t/slots=%d", existingThread, slots), func(t *testing.T) {
				core, _ := setupTestCore(t)
				ctx := testContext(t)
				actor, err := core.CreateUser(ctx, SystemActorID, "compound-author", "Author", "password123")
				require.NoError(t, err)
				room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "echo", "")
				require.NoError(t, err)
				require.NoError(t, core.GrantUserRoomPermission(ctx, SystemActorID, room.Id, actor.Id, PermMessageEcho))
				_, err = core.JoinRoom(ctx, actor.Id, KindChannel, actor.Id, room.Id)
				require.NoError(t, err)
				root, err := core.Messages().PostMessage(ctx, MessagePostInput{ActorID: actor.Id, RoomID: room.Id, Body: "root", CreateThread: existingThread})
				require.NoError(t, err)
				filter := evtstream.RoomAggregate(room.Id).AllEventsFilter()
				beforeSeq, err := core.EventPublisher.LastSubjectSeq(ctx, filter)
				require.NoError(t, err)
				_, err = core.PostMessage(ctx, KindChannel, room.Id, actor.Id, "reply", nil, root.Event.Id, "", nil, true,
					withPostMessageAttemptPrepared(func(ctx context.Context) error {
						limitCompoundBatch(t, ctx, core, slots)
						return nil
					}))
				require.Error(t, err)
				afterSeq, err := core.EventPublisher.LastSubjectSeq(ctx, filter)
				require.NoError(t, err)
				require.Equal(t, beforeSeq, afterSeq, "no body, thread, reply, or echo may survive a rejected batch")
			})
		}
	}
}

func TestCompoundReplyEchoRetriesAcrossReplicas(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		t.Run(fmt.Sprintf("revoke=%t", revoke), func(t *testing.T) {
			first, nc := setupTestCore(t)
			ctx := testContext(t)
			actor, err := first.CreateUser(ctx, SystemActorID, "compound-author", "Author", "password123")
			require.NoError(t, err)
			room, err := first.CreateRoom(ctx, SystemActorID, KindChannel, "", "echo", "")
			require.NoError(t, err)
			_, err = first.JoinRoom(ctx, actor.Id, KindChannel, actor.Id, room.Id)
			require.NoError(t, err)
			require.NoError(t, first.GrantUserRoomPermission(ctx, SystemActorID, room.Id, actor.Id, PermMessageEcho))
			root, err := first.Messages().PostMessage(ctx, MessagePostInput{ActorID: actor.Id, RoomID: room.Id, Body: "root"})
			require.NoError(t, err)
			second, err := NewChattoCore(ctx, nc, first.config)
			require.NoError(t, err)
			startCoreServices(t, second)
			input := MessagePostInput{ActorID: actor.Id, RoomID: room.Id, Body: "reply", ThreadRootEventID: root.Event.Id, AlsoSendToChannel: true}
			attempts := 0
			reply, postErr := first.PostMessage(ctx, KindChannel, room.Id, actor.Id, input.Body, nil, root.Event.Id, "", nil, true,
				withPostMessageCommitAuthorization(func(ctx context.Context, threadID string) error {
					_, err := first.Messages().AuthorizePost(ctx, authorizationInputForPost(input, threadID))
					return err
				}),
				withPostMessageAttemptPrepared(func(ctx context.Context) error {
					attempts++
					if attempts != 1 {
						return nil
					}
					_, err := second.Messages().PostMessage(ctx, MessagePostInput{ActorID: actor.Id, RoomID: room.Id, Body: "concurrent reply", ThreadRootEventID: root.Event.Id})
					if err != nil {
						return err
					}
					if revoke {
						return second.DenyUserRoomPermission(ctx, SystemActorID, room.Id, actor.Id, PermMessageEcho)
					}
					return nil
				}))
			if revoke {
				require.ErrorIs(t, postErr, ErrPermissionDenied)
				require.Nil(t, reply)
			} else {
				require.NoError(t, postErr)
				require.Equal(t, 2, attempts)
				echoID, exists := first.roomModel.channelEchoEventID(reply.Id)
				require.True(t, exists, "the echo must be projected before return")
				echo, err := first.GetRoomEventByEventID(ctx, KindChannel, room.Id, echoID)
				require.NoError(t, err)
				require.Equal(t, reply.Id, echo.GetMessagePosted().GetEchoOfEventId())
			}
			agg := evtstream.RoomAggregate(room.Id)
			created, _, err := first.EventPublisher.SubjectEvents(ctx, agg.Subject(evtstream.EventThreadCreated))
			require.NoError(t, err)
			require.Len(t, created, 1, "the retry must not create the thread a second time")
			posts, _, err := first.EventPublisher.SubjectEvents(ctx, agg.Subject(evtstream.EventMessagePosted))
			require.NoError(t, err)
			wantPosts := 4 // root, concurrent reply, requested reply, echo
			if revoke {
				wantPosts = 2
			}
			require.Len(t, posts, wantPosts)
		})
	}
}

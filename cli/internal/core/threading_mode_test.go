package core

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestRoomThreadingModeConfigurationAndLegacyDefault(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	manager, err := chatto.CreateUser(ctx, SystemActorID, "thread-mode-manager", "Thread Mode Manager", "password123")
	require.NoError(t, err)
	room, err := chatto.CreateRoom(ctx, manager.Id, KindChannel, "", "thread-mode-config", "")
	require.NoError(t, err)
	require.Equal(t, corev1.RoomThreadingMode_ROOM_THREADING_MODE_ENABLED, room.GetThreadingMode())
	require.NoError(t, chatto.GrantUserRoomPermission(ctx, SystemActorID, room.Id, manager.Id, PermRoomManage))

	mode := corev1.RoomThreadingMode_ROOM_THREADING_MODE_ENCOURAGED
	updated, err := chatto.RoomCommands().UpdateRoom(ctx, RoomUpdateInput{
		ActorID: manager.Id, RoomID: room.Id, ThreadingMode: &mode,
	})
	require.NoError(t, err)
	require.Equal(t, mode, updated.GetThreadingMode())

	events, _, err := chatto.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventRoomThreadingModeChanged))
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, mode, events[0].GetRoomThreadingModeChanged().GetThreadingMode())

	_, err = chatto.RoomCommands().UpdateRoom(ctx, RoomUpdateInput{
		ActorID: manager.Id, RoomID: room.Id, ThreadingMode: &mode,
	})
	require.NoError(t, err)
	events, _, err = chatto.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventRoomThreadingModeChanged))
	require.NoError(t, err)
	require.Len(t, events, 1, "an unchanged mode must not write another event")

	invalid := corev1.RoomThreadingMode(99)
	_, err = chatto.RoomCommands().UpdateRoom(ctx, RoomUpdateInput{
		ActorID: manager.Id, RoomID: room.Id, ThreadingMode: &invalid,
	})
	require.ErrorIs(t, err, ErrInvalidArgument)

	legacy := NewRoomCatalogProjection()
	require.NoError(t, legacy.Apply(&corev1.Event{Event: &corev1.Event_RoomCreated{RoomCreated: &corev1.RoomCreatedEvent{
		RoomId: "legacy-room", Name: "Legacy", Kind: corev1.RoomKind_ROOM_KIND_CHANNEL,
	}}}, 1))
	legacyRoom, ok := legacy.Get("legacy-room")
	require.True(t, ok)
	require.Equal(t, corev1.RoomThreadingMode_ROOM_THREADING_MODE_ENABLED, legacyRoom.GetThreadingMode())
}

func TestRequiredThreadingCreatesRootsAndRoutesRootReplies(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "required-thread-user", "Required Thread User", "password123")
	require.NoError(t, err)
	room, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "required-thread-room", "",
		WithRoomThreadingMode(corev1.RoomThreadingMode_ROOM_THREADING_MODE_REQUIRED))
	require.NoError(t, err)
	_, err = chatto.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	require.NoError(t, err)
	require.NoError(t, chatto.DenyUserRoomPermission(ctx, SystemActorID, room.Id, user.Id, PermMessagePostInThread))

	root, err := chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: user.Id, RoomID: room.Id, Body: "automatic required thread",
	})
	require.NoError(t, err, "automatic required roots need only message.post")
	metadata, err := chatto.GetThreadMetadata(ctx, KindChannel, room.Id, root.Event.Id)
	require.NoError(t, err)
	require.True(t, metadata.Exists)

	_, err = chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: user.Id, RoomID: room.Id, Body: "flat root reply", InReplyTo: root.Event.Id,
	})
	require.ErrorIs(t, err, ErrRoomThreadingPolicy)

	_, err = chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: user.Id, RoomID: room.Id, Body: "thread reply", InReplyTo: root.Event.Id, ThreadRootEventID: root.Event.Id,
	})
	require.ErrorIs(t, err, ErrPermissionDenied, "actual replies still require message.post-in-thread")

	require.NoError(t, chatto.ClearUserRoomPermissionState(ctx, SystemActorID, room.Id, user.Id, PermMessagePostInThread))
	reply, err := chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: user.Id, RoomID: room.Id, Body: "thread reply", InReplyTo: root.Event.Id, ThreadRootEventID: root.Event.Id,
	})
	require.NoError(t, err)
	require.Equal(t, root.Event.Id, reply.Event.GetMessagePosted().GetInThread())
}

func TestEncouragedAndDisabledThreadingPolicy(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "thread-policy-user", "Thread Policy User", "password123")
	require.NoError(t, err)
	room, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "thread-policy-room", "",
		WithRoomThreadingMode(corev1.RoomThreadingMode_ROOM_THREADING_MODE_ENCOURAGED))
	require.NoError(t, err)
	_, err = chatto.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	require.NoError(t, err)

	root, err := chatto.Messages().PostMessage(ctx, MessagePostInput{ActorID: user.Id, RoomID: room.Id, Body: "ordinary encouraged root"})
	require.NoError(t, err)
	metadata, err := chatto.GetThreadMetadata(ctx, KindChannel, room.Id, root.Event.Id)
	require.NoError(t, err)
	require.False(t, metadata.Exists, "encouraged must not create a thread for every quick root post")
	_, err = chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: user.Id, RoomID: room.Id, Body: "allowed flat reply", InReplyTo: root.Event.Id,
	})
	require.NoError(t, err)

	threadedRoot, err := chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: user.Id, RoomID: room.Id, Body: "historical thread", CreateThread: true,
	})
	require.NoError(t, err)
	room, err = chatto.SetRoomThreadingMode(ctx, SystemActorID, KindChannel, room.Id, corev1.RoomThreadingMode_ROOM_THREADING_MODE_DISABLED)
	require.NoError(t, err)

	_, err = chatto.Messages().PostMessage(ctx, MessagePostInput{ActorID: user.Id, RoomID: room.Id, Body: "ordinary disabled root"})
	require.NoError(t, err)
	_, err = chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: user.Id, RoomID: room.Id, Body: "flat attribution remains", InReplyTo: root.Event.Id,
	})
	require.NoError(t, err)
	_, err = chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: user.Id, RoomID: room.Id, Body: "forbidden thread reply", ThreadRootEventID: threadedRoot.Event.Id,
	})
	require.ErrorIs(t, err, ErrRoomThreadingPolicy)
	_, err = chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: user.Id, RoomID: room.Id, Body: "forbidden thread", CreateThread: true,
	})
	require.ErrorIs(t, err, ErrRoomThreadingPolicy)
	require.ErrorIs(t, chatto.Messages().SendTypingIndicator(ctx, TypingIndicatorInput{
		ActorID: user.Id, RoomID: room.Id, ThreadRootEventID: &threadedRoot.Event.Id,
	}), ErrRoomThreadingPolicy)

	events, err := chatto.GetThreadEvents(ctx, KindChannel, room.Id, threadedRoot.Event.Id)
	require.NoError(t, err)
	require.NotEmpty(t, events, "disabling threads must not hide historical threads")
}

func TestThreadingModeChangeConflictsWithInFlightMessage(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "thread-mode-race", "Thread Mode Race", "password123")
	require.NoError(t, err)
	room, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "thread-mode-race", "")
	require.NoError(t, err)
	_, err = chatto.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	require.NoError(t, err)

	authorizationChecks := 0
	authorize := func(attemptCtx context.Context, _ string) error {
		authorizationChecks++
		_, authErr := chatto.Messages().AuthorizePost(attemptCtx, MessagePostAuthorizationInput{
			ActorID: user.Id, RoomID: room.Id, Body: "must not become a flat required root",
		})
		if authErr != nil {
			return authErr
		}
		if authorizationChecks == 1 {
			_, authErr = chatto.SetRoomThreadingMode(attemptCtx, SystemActorID, KindChannel, room.Id, corev1.RoomThreadingMode_ROOM_THREADING_MODE_REQUIRED)
		}
		return authErr
	}

	_, err = chatto.PostMessage(ctx, KindChannel, room.Id, user.Id, "must not become a flat required root", nil, "", "", nil, false,
		withPostMessageCommitAuthorization(authorize))
	require.ErrorIs(t, err, ErrRoomThreadingPolicy)
	require.GreaterOrEqual(t, authorizationChecks, 2)

	posted, _, readErr := chatto.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventMessagePosted))
	require.NoError(t, readErr)
	require.Empty(t, posted)
}

func TestDMThreadingModeIsRejected(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := chatto.CreateUser(ctx, SystemActorID, "thread-dm-owner", "Thread DM Owner", "password123")
	require.NoError(t, err)
	peer, err := chatto.CreateUser(ctx, SystemActorID, "thread-dm-peer", "Thread DM Peer", "password123")
	require.NoError(t, err)
	dm, _, err := chatto.FindOrCreateDM(ctx, owner.Id, []string{peer.Id})
	require.NoError(t, err)
	require.Equal(t, corev1.RoomThreadingMode_ROOM_THREADING_MODE_UNSPECIFIED, dm.GetThreadingMode())
	_, err = chatto.SetRoomThreadingMode(ctx, owner.Id, KindDM, dm.Id, corev1.RoomThreadingMode_ROOM_THREADING_MODE_ENABLED)
	require.True(t, errors.Is(err, ErrInvalidArgument))
}

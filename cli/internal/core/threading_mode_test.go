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

	require.NoError(t, legacy.Apply(&corev1.Event{Event: &corev1.Event_RoomThreadingModeChanged{
		RoomThreadingModeChanged: &corev1.RoomThreadingModeChangedEvent{
			RoomId: "legacy-room", ThreadingMode: corev1.RoomThreadingMode(99),
		},
	}}, 2))
	legacyRoom, ok = legacy.Get("legacy-room")
	require.True(t, ok)
	require.Equal(t, corev1.RoomThreadingMode_ROOM_THREADING_MODE_DISABLED, legacyRoom.GetThreadingMode(), "unknown future policies must fail closed")
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

	echoedReply, err := chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: user.Id, RoomID: room.Id, Body: "echoed thread reply", InReplyTo: reply.Event.Id,
		ThreadRootEventID: root.Event.Id, AlsoSendToChannel: true,
	})
	require.NoError(t, err)
	echoID, exists := chatto.roomModel.channelEchoEventID(echoedReply.Event.Id)
	require.True(t, exists)
	replyToEcho, err := chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: user.Id, RoomID: room.Id, Body: "reply addressed to the visible echo", InReplyTo: echoID,
	})
	require.NoError(t, err)
	require.Equal(t, root.Event.Id, replyToEcho.Event.GetMessagePosted().GetInThread())
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
	plainThreadReply, err := chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: user.Id, RoomID: room.Id, Body: "historical reply without echo", ThreadRootEventID: threadedRoot.Event.Id,
	})
	require.NoError(t, err)
	echoedThreadReply, err := chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: user.Id, RoomID: room.Id, Body: "historical reply with echo", ThreadRootEventID: threadedRoot.Event.Id, AlsoSendToChannel: true,
	})
	require.NoError(t, err)
	room, err = chatto.SetRoomThreadingMode(ctx, SystemActorID, KindChannel, room.Id, corev1.RoomThreadingMode_ROOM_THREADING_MODE_DISABLED)
	require.NoError(t, err)
	echoID, exists := chatto.roomModel.channelEchoEventID(echoedThreadReply.Event.Id)
	require.True(t, exists)

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
	_, err = chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: user.Id, RoomID: room.Id, Body: "forbidden reply to echo", InReplyTo: echoID,
	})
	require.ErrorIs(t, err, ErrRoomThreadingPolicy)
	require.ErrorIs(t, chatto.Messages().SendTypingIndicator(ctx, TypingIndicatorInput{
		ActorID: user.Id, RoomID: room.Id, ThreadRootEventID: &threadedRoot.Event.Id,
	}), ErrRoomThreadingPolicy)
	addEcho := true
	_, _, err = chatto.Messages().UpdateMessage(ctx, MessageUpdateInput{
		ActorID: user.Id, RoomID: room.Id, EventID: plainThreadReply.Event.Id, AlsoSendToChannel: &addEcho,
	})
	require.ErrorIs(t, err, ErrRoomThreadingPolicy, "disabled rooms must not gain echoes through historical reply edits")
	removeEcho := false
	_, _, err = chatto.Messages().UpdateMessage(ctx, MessageUpdateInput{
		ActorID: user.Id, RoomID: room.Id, EventID: echoedThreadReply.Event.Id, AlsoSendToChannel: &removeEcho,
	})
	require.NoError(t, err, "authors may remove an existing historical echo while threading is disabled")
	_, echoExists := chatto.roomModel.channelEchoEventID(echoedThreadReply.Event.Id)
	require.False(t, echoExists)

	events, err := chatto.GetThreadEvents(ctx, KindChannel, room.Id, threadedRoot.Event.Id)
	require.NoError(t, err)
	require.NotEmpty(t, events, "disabling threads must not hide historical threads")
}

func TestThreadReplyEchoRevalidatesThreadingModeAfterOCCConflict(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := chatto.CreateUser(ctx, SystemActorID, "echo-policy-race-user", "Echo Policy Race User", "password123")
	require.NoError(t, err)
	room, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "echo-policy-race-room", "")
	require.NoError(t, err)
	_, err = chatto.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	require.NoError(t, err)

	root, err := chatto.Messages().PostMessage(ctx, MessagePostInput{
		ActorID: user.Id, RoomID: room.Id, Body: "thread root", CreateThread: true,
	})
	require.NoError(t, err)

	echoAttempts := 0
	reply, err := chatto.PostMessage(
		ctx, KindChannel, room.Id, user.Id, "reply racing the room policy", nil, root.Event.Id, "", nil, true,
		withThreadReplyEchoAttemptPrepared(func(attemptCtx context.Context) error {
			echoAttempts++
			if echoAttempts != 1 {
				return nil
			}
			_, err := chatto.SetRoomThreadingMode(attemptCtx, SystemActorID, KindChannel, room.Id, corev1.RoomThreadingMode_ROOM_THREADING_MODE_DISABLED)
			return err
		}),
	)
	require.NoError(t, err, "the committed thread reply remains successful when its best-effort echo is rejected")
	require.Equal(t, 1, echoAttempts, "the OCC retry must reject the disabled policy before preparing another echo")
	require.Equal(t, root.Event.Id, reply.GetMessagePosted().GetInThread())

	_, echoExists := chatto.roomModel.channelEchoEventID(reply.Id)
	require.False(t, echoExists, "a mode change committed before the echo batch must prevent the channel echo")
	posted, _, err := chatto.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventMessagePosted))
	require.NoError(t, err)
	require.Len(t, posted, 2, "only the root and committed thread reply should be published")
}

func TestThreadingModeChangeReauthorizesAfterManageRevocation(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	manager, err := chatto.CreateUser(ctx, SystemActorID, "thread-mode-auth-race", "Thread Mode Auth Race", "password123")
	require.NoError(t, err)
	room, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "thread-mode-auth-race", "")
	require.NoError(t, err)
	require.NoError(t, chatto.GrantUserRoomPermission(ctx, SystemActorID, room.Id, manager.Id, PermRoomManage))

	checks := 0
	_, err = chatto.setRoomThreadingMode(ctx, manager.Id, KindChannel, room.Id, corev1.RoomThreadingMode_ROOM_THREADING_MODE_REQUIRED, func(attemptCtx context.Context) error {
		checks++
		if checks == 1 {
			return chatto.DenyUserRoomPermission(attemptCtx, SystemActorID, room.Id, manager.Id, PermRoomManage)
		}
		_, authorizeErr := chatto.RoomCommands().authorizeRoomManage(attemptCtx, manager.Id, room.Id)
		return authorizeErr
	})
	require.ErrorIs(t, err, ErrPermissionDenied)
	require.GreaterOrEqual(t, checks, 2)
	unchanged, err := chatto.GetRoom(ctx, KindChannel, room.Id)
	require.NoError(t, err)
	require.Equal(t, corev1.RoomThreadingMode_ROOM_THREADING_MODE_ENABLED, EffectiveRoomThreadingMode(unchanged))
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

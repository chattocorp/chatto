package core

import (
	"errors"
	"testing"
)

func TestRoomTimelineReadModelRequiresMembership(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "timeline-read-authz", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	member, err := core.CreateUser(ctx, SystemActorID, "timeline-reader", "Timeline Reader", "password")
	if err != nil {
		t.Fatalf("CreateUser member: %v", err)
	}
	if _, err := core.JoinRoom(ctx, member.Id, KindChannel, member.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom member: %v", err)
	}
	outsider, err := core.CreateUser(ctx, SystemActorID, "timeline-outsider", "Timeline Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}

	if _, err := core.RoomTimelineReads().GetRoomEvents(ctx, RoomTimelineEventsInput{
		ActorID: outsider.Id,
		RoomID:  room.Id,
	}); !errors.Is(err, ErrNotRoomMember) {
		t.Fatalf("GetRoomEvents outsider error = %v, want ErrNotRoomMember", err)
	}

	if _, err := core.RoomTimelineReads().GetRoomEvents(ctx, RoomTimelineEventsInput{
		ActorID: member.Id,
		RoomID:  room.Id,
	}); err != nil {
		t.Fatalf("GetRoomEvents member: %v", err)
	}

	message, err := core.PostMessage(ctx, KindChannel, room.Id, member.Id, "visible", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if _, err := core.RoomTimelineReads().GetRoomEventsAround(ctx, outsider.Id, room.Id, message.Id, 3); !errors.Is(err, ErrNotRoomMember) {
		t.Fatalf("GetRoomEventsAround outsider error = %v, want ErrNotRoomMember", err)
	}

	if _, err := core.RoomTimelineReads().GetMessage(ctx, outsider.Id, room.Id, message.Id); !errors.Is(err, ErrNotRoomMember) {
		t.Fatalf("GetMessage outsider error = %v, want ErrNotRoomMember", err)
	}
}

func TestRoomTimelineReadModelGetsMessages(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "message-link-target", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	user, err := core.CreateUser(ctx, SystemActorID, "message-link-reader", "Message Link Reader", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	root, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Post root: %v", err)
	}
	reply, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "reply", nil, root.Id, "", nil, false)
	if err != nil {
		t.Fatalf("Post reply: %v", err)
	}

	rootResult, err := core.RoomTimelineReads().GetMessage(ctx, user.Id, room.Id, root.Id)
	if err != nil {
		t.Fatalf("GetMessage root: %v", err)
	}
	if rootResult.Event.GetId() != root.Id || rootResult.Event.GetMessagePosted().GetInThread() != "" {
		t.Fatalf("root message = event %q thread %q, want event %q no thread", rootResult.Event.GetId(), rootResult.Event.GetMessagePosted().GetInThread(), root.Id)
	}

	replyResult, err := core.RoomTimelineReads().GetMessage(ctx, user.Id, room.Id, reply.Id)
	if err != nil {
		t.Fatalf("GetMessage reply: %v", err)
	}
	if replyResult.Event.GetId() != reply.Id || replyResult.Event.GetMessagePosted().GetInThread() != root.Id {
		t.Fatalf("reply message = event %q thread %q, want event %q thread %q", replyResult.Event.GetId(), replyResult.Event.GetMessagePosted().GetInThread(), reply.Id, root.Id)
	}

	if _, err := core.RoomTimelineReads().GetMessage(ctx, user.Id, room.Id, "missing-event"); !errors.Is(err, ErrMessageNotFound) {
		t.Fatalf("missing message error = %v, want ErrMessageNotFound", err)
	}
}

func TestRoomTimelineReadModelValidatesThreadRoot(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "timeline-thread-authz", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	user, err := core.CreateUser(ctx, SystemActorID, "timeline-thread-reader", "Timeline Thread Reader", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	root, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Post root: %v", err)
	}
	reply, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "reply", nil, root.Id, "", nil, false)
	if err != nil {
		t.Fatalf("Post reply: %v", err)
	}

	if _, err := core.RoomTimelineReads().GetThreadEvents(ctx, ThreadTimelineEventsInput{
		ActorID:           user.Id,
		RoomID:            room.Id,
		ThreadRootEventID: "missing-root",
	}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing root error = %v, want ErrNotFound", err)
	}

	if _, err := core.RoomTimelineReads().GetThreadEvents(ctx, ThreadTimelineEventsInput{
		ActorID:           user.Id,
		RoomID:            room.Id,
		ThreadRootEventID: reply.Id,
	}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("reply root error = %v, want ErrInvalidArgument", err)
	}

	outsider, err := core.CreateUser(ctx, SystemActorID, "timeline-thread-outsider", "Timeline Thread Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if _, err := core.RoomTimelineReads().GetThreadEventsAround(ctx, outsider.Id, room.Id, root.Id, reply.Id, 3); !errors.Is(err, ErrNotRoomMember) {
		t.Fatalf("GetThreadEventsAround outsider error = %v, want ErrNotRoomMember", err)
	}
}

func TestRoomTimelineReadModelThreadAroundComputesTargetIndex(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "timeline-thread-around", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	user, err := core.CreateUser(ctx, SystemActorID, "timeline-around-reader", "Timeline Around Reader", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	root, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Post root: %v", err)
	}
	if _, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "reply one", nil, root.Id, "", nil, false); err != nil {
		t.Fatalf("Post reply one: %v", err)
	}
	reply2, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "reply two", nil, root.Id, "", nil, false)
	if err != nil {
		t.Fatalf("Post reply two: %v", err)
	}
	if _, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "reply three", nil, root.Id, "", nil, false); err != nil {
		t.Fatalf("Post reply three: %v", err)
	}

	result, err := core.RoomTimelineReads().GetThreadEventsAround(ctx, user.Id, room.Id, root.Id, reply2.Id, 3)
	if err != nil {
		t.Fatalf("GetThreadEventsAround: %v", err)
	}
	if result.TargetIndex != 2 {
		t.Fatalf("TargetIndex = %d, want 2", result.TargetIndex)
	}
	if result.Kind != KindChannel {
		t.Fatalf("Kind = %v, want KindChannel", result.Kind)
	}
	if result.Root == nil || result.Root.Event.Id != root.Id {
		t.Fatalf("Root = %+v, want %s", result.Root, root.Id)
	}
	if len(result.Replies.Events) != 3 {
		t.Fatalf("reply count = %d, want 3", len(result.Replies.Events))
	}
}

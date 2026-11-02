package core

import (
	"errors"
	"testing"
)

func TestRoomTimelineReadModelMessageHydrationStateRequiresRoomModel(t *testing.T) {
	if _, err := (&RoomTimelineReadModel{}).MessageHydrationState("ENV-M1"); err == nil {
		t.Fatal("MessageHydrationState without room model error = nil")
	}
	if _, err := (&RoomTimelineReadModel{rooms: &RoomModel{}}).MessageHydrationState("ENV-M1"); err == nil {
		t.Fatal("MessageHydrationState without timeline projection error = nil")
	}
}

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

func TestRoomTimelineReadModelRequiresMessageReadWithoutGrantingWrite(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)

	room, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "timeline-read-permission", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	member, err := chatto.CreateUser(ctx, SystemActorID, "permission-reader", "Permission Reader", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := chatto.JoinRoom(ctx, member.Id, KindChannel, member.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	message, err := chatto.PostMessage(ctx, KindChannel, room.Id, member.Id, "before denial", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage before denial: %v", err)
	}
	if err := chatto.ThreadFollows().FollowThread(ctx, member.Id, room.Id, message.Id); err != nil {
		t.Fatalf("FollowThread before denial: %v", err)
	}
	if err := chatto.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageRead); err != nil {
		t.Fatalf("DenyRoomPermission: %v", err)
	}
	if err := chatto.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageReadInteractions); err != nil {
		t.Fatalf("DenyRoomPermission message.read-interactions: %v", err)
	}

	for name, read := range map[string]func() error{
		"room timeline": func() error {
			_, err := chatto.RoomTimelineReads().GetRoomEvents(ctx, RoomTimelineEventsInput{ActorID: member.Id, RoomID: room.Id})
			return err
		},
		"message": func() error {
			_, err := chatto.RoomTimelineReads().GetMessage(ctx, member.Id, room.Id, message.Id)
			return err
		},
		"read marker": func() error {
			_, err := chatto.ReadState().MarkRoomAsRead(ctx, member.Id, room.Id, message.Id)
			return err
		},
		"follow thread": func() error {
			return chatto.ThreadFollows().FollowThread(ctx, member.Id, room.Id, message.Id)
		},
		"unfollow thread": func() error {
			return chatto.ThreadFollows().UnfollowThread(ctx, member.Id, room.Id, message.Id)
		},
	} {
		if err := read(); !errors.Is(err, ErrPermissionDenied) {
			t.Errorf("%s error = %v, want ErrPermissionDenied", name, err)
		}
	}

	if _, err := chatto.PostMessage(ctx, KindChannel, room.Id, member.Id, "write-only post", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage while message.read is denied: %v", err)
	}
	if err := chatto.GrantUserRoomPermission(ctx, SystemActorID, room.Id, member.Id, PermMessageRead); err != nil {
		t.Fatalf("GrantUserRoomPermission: %v", err)
	}
	if _, err := chatto.RoomTimelineReads().GetMessage(ctx, member.Id, room.Id, message.Id); err != nil {
		t.Fatalf("GetMessage after direct room grant: %v", err)
	}
}

func TestRoomTimelineReadModelInteractionScopedAccess(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)

	author, err := chatto.CreateUser(ctx, SystemActorID, "interaction-author", "Interaction Author", "password")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	reader, err := chatto.CreateUser(ctx, SystemActorID, "interaction-reader", "Interaction Reader", "password")
	if err != nil {
		t.Fatalf("CreateUser reader: %v", err)
	}
	room, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "interaction-access", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{author.GetId(), reader.GetId()} {
		if _, err := chatto.JoinRoom(ctx, userID, KindChannel, userID, room.GetId()); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}

	mentionedRoot, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "mentioned thread", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage mentioned root: %v", err)
	}
	earlierReply, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "earlier reply", nil, mentionedRoot.GetId(), "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage earlier reply: %v", err)
	}
	unrelatedRoot, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "unrelated thread", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage unrelated root: %v", err)
	}
	readerReply, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), reader.GetId(), "reply authored by reader", nil, unrelatedRoot.GetId(), "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage reader reply: %v", err)
	}
	readerRoot, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), reader.GetId(), "reader-authored root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage reader root: %v", err)
	}
	if err := chatto.DenyUserRoomPermission(ctx, SystemActorID, room.GetId(), reader.GetId(), PermMessageRead); err != nil {
		t.Fatalf("DenyUserRoomPermission message.read: %v", err)
	}
	if err := chatto.GrantUserRoomPermission(ctx, SystemActorID, room.GetId(), reader.GetId(), PermMessageReadInteractions); err != nil {
		t.Fatalf("GrantUserRoomPermission message.read-interactions: %v", err)
	}

	if _, err := chatto.RoomTimelineReads().GetMessage(ctx, reader.GetId(), room.GetId(), mentionedRoot.GetId()); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("GetMessage before mention error = %v, want ErrPermissionDenied", err)
	}
	if _, err := chatto.RoomTimelineReads().GetMessage(ctx, reader.GetId(), room.GetId(), readerReply.GetId()); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("GetMessage for authored reply error = %v, want ErrPermissionDenied", err)
	}
	if _, err := chatto.RoomTimelineReads().GetMessage(ctx, reader.GetId(), room.GetId(), readerRoot.GetId()); err != nil {
		t.Fatalf("GetMessage for authored root: %v", err)
	}

	mentionReply, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "hello @interaction-reader", nil, mentionedRoot.GetId(), "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage mention reply: %v", err)
	}
	thread, err := chatto.RoomTimelineReads().GetThreadEvents(ctx, ThreadTimelineEventsInput{
		ActorID: reader.GetId(), RoomID: room.GetId(), ThreadRootEventID: mentionedRoot.GetId(), Limit: 20,
	})
	if err != nil {
		t.Fatalf("GetThreadEvents after mention: %v", err)
	}
	if thread.Root.GetId() != mentionedRoot.GetId() || len(thread.Replies.Events) != 2 ||
		thread.Replies.Events[0].GetId() != earlierReply.GetId() || thread.Replies.Events[1].GetId() != mentionReply.GetId() {
		t.Fatalf("interaction thread = root %v replies %+v; want root and both replies", thread.Root, thread.Replies.Events)
	}

	page, err := chatto.RoomTimelineReads().GetRoomEvents(ctx, RoomTimelineEventsInput{ActorID: reader.GetId(), RoomID: room.GetId(), Limit: 20})
	if err != nil {
		t.Fatalf("GetRoomEvents after mention: %v", err)
	}
	visibleRoots := make(map[string]bool)
	for _, event := range page.Page.Events {
		visibleRoots[event.GetId()] = true
	}
	if !visibleRoots[mentionedRoot.GetId()] || !visibleRoots[readerRoot.GetId()] || visibleRoots[unrelatedRoot.GetId()] {
		t.Fatalf("interaction-scoped room roots = %v", visibleRoots)
	}

	withoutMention := "mention removed by edit"
	if _, _, err := chatto.Messages().UpdateMessage(ctx, MessageUpdateInput{
		ActorID: author.GetId(), RoomID: room.GetId(), EventID: mentionReply.GetId(), Body: &withoutMention,
	}); err != nil {
		t.Fatalf("UpdateMessage mention source: %v", err)
	}
	if err := chatto.DeleteMessage(ctx, author.GetId(), KindChannel, room.GetId(), mentionReply.GetId()); err != nil {
		t.Fatalf("DeleteMessage mention source: %v", err)
	}
	if allowed, err := chatto.CanReadThreadMessages(ctx, reader.GetId(), KindChannel, room.GetId(), mentionedRoot.GetId()); err != nil || !allowed {
		t.Fatalf("CanReadThreadMessages after edit and retraction = %v, %v; want true", allowed, err)
	}

	if err := chatto.DenyUserRoomPermission(ctx, SystemActorID, room.GetId(), reader.GetId(), PermMessageReadInteractions); err != nil {
		t.Fatalf("DenyUserRoomPermission message.read-interactions: %v", err)
	}
	if _, err := chatto.RoomTimelineReads().roomTimelineVisibility(ctx, reader.GetId(), KindChannel, room.GetId()); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("roomTimelineVisibility after permission loss error = %v, want ErrPermissionDenied", err)
	}
	if _, err := chatto.RoomTimelineReads().GetThreadEvents(ctx, ThreadTimelineEventsInput{
		ActorID: reader.GetId(), RoomID: room.GetId(), ThreadRootEventID: mentionedRoot.GetId(),
	}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("GetThreadEvents after permission loss error = %v, want ErrPermissionDenied", err)
	}
	if err := chatto.GrantUserRoomPermission(ctx, SystemActorID, room.GetId(), reader.GetId(), PermMessageReadInteractions); err != nil {
		t.Fatalf("GrantUserRoomPermission message.read-interactions: %v", err)
	}
	if err := chatto.LeaveRoom(ctx, reader.GetId(), KindChannel, reader.GetId(), room.GetId()); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}
	if _, err := chatto.RoomTimelineReads().GetThreadEvents(ctx, ThreadTimelineEventsInput{
		ActorID: reader.GetId(), RoomID: room.GetId(), ThreadRootEventID: mentionedRoot.GetId(),
	}); !errors.Is(err, ErrNotRoomMember) {
		t.Fatalf("GetThreadEvents after membership loss error = %v, want ErrNotRoomMember", err)
	}
	if _, err := chatto.JoinRoom(ctx, reader.GetId(), KindChannel, reader.GetId(), room.GetId()); err != nil {
		t.Fatalf("JoinRoom after leave: %v", err)
	}
	if _, err := chatto.RoomTimelineReads().GetThreadEvents(ctx, ThreadTimelineEventsInput{
		ActorID: reader.GetId(), RoomID: room.GetId(), ThreadRootEventID: mentionedRoot.GetId(),
	}); err != nil {
		t.Fatalf("GetThreadEvents after permission and membership restoration: %v", err)
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

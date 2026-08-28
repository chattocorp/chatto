package core

import (
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestRoomTimelineProjectionOrdersPinsByDurableSequence(t *testing.T) {
	projection := NewRoomTimelineProjection()
	for sequence, messageID := range []string{"M1", "M2"} {
		posted := newEvent("author", &evtv1.Event{Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: "R1"}}})
		posted.Id = messageID
		if err := projection.Apply(posted, uint64(sequence+1)); err != nil {
			t.Fatalf("Apply posted %s: %v", messageID, err)
		}
	}
	first := newEvent("manager", &evtv1.Event{Event: &evtv1.Event_MessagePinned{MessagePinned: &evtv1.MessagePinnedEvent{RoomId: "R1", MessageEventId: "M1"}}})
	first.Id = "P1"
	first.CreatedAt = timestamppb.New(time.Unix(200, 0))
	second := newEvent("manager", &evtv1.Event{Event: &evtv1.Event_MessagePinned{MessagePinned: &evtv1.MessagePinnedEvent{RoomId: "R1", MessageEventId: "M2"}}})
	second.Id = "P2"
	second.CreatedAt = timestamppb.New(time.Unix(100, 0))
	if err := projection.Apply(first, 3); err != nil {
		t.Fatalf("Apply first pin: %v", err)
	}
	if err := projection.Apply(second, 4); err != nil {
		t.Fatalf("Apply second pin: %v", err)
	}

	items := projection.PinnedMessages("R1")
	if len(items) != 2 || items[0].PinEventID != "P2" || items[0].PinSequence != 4 {
		t.Fatalf("PinnedMessages = %+v, want P2 first by durable sequence", items)
	}
	if got := projection.LatestPinEventID("R1"); got != "P2" {
		t.Fatalf("LatestPinEventID = %q, want P2", got)
	}
	unpinned := newEvent("manager", &evtv1.Event{Event: &evtv1.Event_MessageUnpinned{MessageUnpinned: &evtv1.MessageUnpinnedEvent{RoomId: "R1", MessageEventId: "M2"}}})
	if err := projection.Apply(unpinned, 5); err != nil {
		t.Fatalf("Apply unpin: %v", err)
	}
	if got := projection.LatestPinEventID("R1"); got != "P2" {
		t.Fatalf("LatestPinEventID after unpin = %q, want stable P2", got)
	}
}

func TestRoomTimelineProjectionPinnedMessagesLifecycle(t *testing.T) {
	projection := NewRoomTimelineProjection()
	posted := newEvent("author", &evtv1.Event{Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: "R1"}}})
	posted.Id = "M1"
	if err := projection.Apply(posted, 1); err != nil {
		t.Fatalf("Apply posted: %v", err)
	}
	pinned := newEvent("manager", &evtv1.Event{Event: &evtv1.Event_MessagePinned{MessagePinned: &evtv1.MessagePinnedEvent{RoomId: "R1", MessageEventId: "M1"}}})
	pinned.Id = "P1"
	if err := projection.Apply(pinned, 2); err != nil {
		t.Fatalf("Apply pinned: %v", err)
	}
	items := projection.PinnedMessages("R1")
	if len(items) != 1 || items[0].MessageEventID != "M1" {
		t.Fatalf("PinnedMessages = %+v", items)
	}
	if state := projection.MessageHydrationState("M1"); !state.Pinned {
		t.Fatalf("MessageHydrationState pinned = false, want true")
	}

	snapshot, err := projection.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	restored := NewRoomTimelineProjection()
	if err := restored.Restore(snapshot); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got := restored.PinnedMessages("R1"); len(got) != 1 || got[0].PinEventID != "P1" || got[0].PinSequence != 2 {
		t.Fatalf("restored PinnedMessages = %+v", got)
	}
	if got := restored.LatestPinEventID("R1"); got != "P1" {
		t.Fatalf("restored LatestPinEventID = %q, want P1", got)
	}

	retracted := newEvent("author", &evtv1.Event{Event: &evtv1.Event_MessageRetracted{MessageRetracted: &evtv1.MessageRetractedEvent{RoomId: "R1", EventId: "M1"}}})
	if err := restored.Apply(retracted, 3); err != nil {
		t.Fatalf("Apply retracted: %v", err)
	}
	if got := restored.PinnedMessages("R1"); len(got) != 0 {
		t.Fatalf("PinnedMessages after retraction = %+v", got)
	}
	if state := restored.MessageHydrationState("M1"); state.Pinned {
		t.Fatalf("MessageHydrationState pinned after retraction = true, want false")
	}
	if got := restored.LatestPinEventID("R1"); got != "P1" {
		t.Fatalf("LatestPinEventID after retraction = %q, want stable P1", got)
	}
}

func TestRoomTimelineProjectionEchoInheritsCanonicalPinState(t *testing.T) {
	projection := NewRoomTimelineProjection()
	original := newEvent("author", &evtv1.Event{Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: "R1", InThread: "ROOT"}}})
	original.Id = "M1"
	echo := newEvent("author", &evtv1.Event{Event: &evtv1.Event_MessagePosted{MessagePosted: &evtv1.MessagePostedEvent{RoomId: "R1", EchoOfEventId: "M1", EchoFromThreadRootEventId: "ROOT"}}})
	echo.Id = "E1"
	pinned := newEvent("manager", &evtv1.Event{Event: &evtv1.Event_MessagePinned{MessagePinned: &evtv1.MessagePinnedEvent{RoomId: "R1", MessageEventId: "M1"}}})
	if err := projection.Apply(original, 1); err != nil {
		t.Fatalf("Apply original: %v", err)
	}
	if err := projection.Apply(echo, 2); err != nil {
		t.Fatalf("Apply echo: %v", err)
	}
	if err := projection.Apply(pinned, 3); err != nil {
		t.Fatalf("Apply pinned: %v", err)
	}
	if state := projection.MessageHydrationState("E1"); !state.Pinned {
		t.Fatalf("echo MessageHydrationState pinned = false, want true")
	}
}

func TestPinnedMessageCommandsAuthorizationIdempotenceAndDMRejection(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	manager, err := chatto.CreateUser(ctx, SystemActorID, "pin-manager", "Pin Manager", "password")
	if err != nil {
		t.Fatalf("CreateUser manager: %v", err)
	}
	room, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "pinned-messages", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := chatto.JoinRoom(ctx, manager.Id, KindChannel, manager.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	message, err := chatto.PostMessage(ctx, KindChannel, room.Id, manager.Id, "important", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	input := PinnedMessageMutationInput{ActorID: manager.Id, RoomID: room.Id, MessageEventID: message.Id}
	if _, err := chatto.RoomCommands().CreatePinnedMessage(ctx, input); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("CreatePinnedMessage without room.manage error = %v", err)
	}
	if err := chatto.GrantRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermRoomManage); err != nil {
		t.Fatalf("GrantRoomPermission: %v", err)
	}
	outsider, err := chatto.CreateUser(ctx, SystemActorID, "pin-outsider", "Pin Outsider", "password")
	if err != nil {
		t.Fatalf("CreateUser outsider: %v", err)
	}
	if err := chatto.GrantUserPermission(ctx, SystemActorID, outsider.Id, PermRoomManage); err != nil {
		t.Fatalf("GrantUserPermission outsider: %v", err)
	}
	outsiderInput := PinnedMessageMutationInput{ActorID: outsider.Id, RoomID: room.Id, MessageEventID: message.Id}
	if _, err := chatto.RoomCommands().CreatePinnedMessage(ctx, outsiderInput); !errors.Is(err, ErrNotRoomMember) {
		t.Fatalf("CreatePinnedMessage nonmember error = %v, want not room member", err)
	}
	if _, err := chatto.RoomCommands().DeletePinnedMessage(ctx, outsiderInput); !errors.Is(err, ErrNotRoomMember) {
		t.Fatalf("DeletePinnedMessage nonmember error = %v, want not room member", err)
	}
	first, err := chatto.RoomCommands().CreatePinnedMessage(ctx, input)
	if err != nil {
		t.Fatalf("CreatePinnedMessage: %v", err)
	}
	second, err := chatto.RoomCommands().CreatePinnedMessage(ctx, input)
	if err != nil {
		t.Fatalf("idempotent CreatePinnedMessage: %v", err)
	}
	if first.PinEventID == "" || second.PinEventID != first.PinEventID {
		t.Fatalf("idempotent pin states = %+v / %+v", first, second)
	}
	if first.RoomID != room.Id {
		t.Fatalf("CreatePinnedMessage RoomID = %q, want %q", first.RoomID, room.Id)
	}
	page, err := chatto.RoomTimelineReads().ListPinnedMessages(ctx, PinnedMessageListInput{ActorID: manager.Id, RoomID: room.Id, Limit: 50})
	if err != nil || len(page.Items) != 1 || page.Items[0].Event.GetId() != message.Id || page.LatestPinEventID != first.PinEventID {
		t.Fatalf("ListPinnedMessages = %+v, %v", page, err)
	}
	if err := chatto.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageRead); err != nil {
		t.Fatalf("DenyRoomPermission message.read: %v", err)
	}
	if err := chatto.DenyRoomPermission(ctx, SystemActorID, room.Id, RoleEveryone, PermMessageReadInteractions); err != nil {
		t.Fatalf("DenyRoomPermission message.read-interactions: %v", err)
	}
	if _, err := chatto.RoomTimelineReads().ListPinnedMessages(ctx, PinnedMessageListInput{ActorID: manager.Id, RoomID: room.Id, Limit: 50}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("ListPinnedMessages without message.read error = %v, want permission denied", err)
	}
	if _, err := chatto.RoomCommands().CreatePinnedMessage(ctx, input); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("CreatePinnedMessage without message.read error = %v, want permission denied", err)
	}
	if err := chatto.GrantUserRoomPermission(ctx, SystemActorID, room.Id, manager.Id, PermMessageRead); err != nil {
		t.Fatalf("GrantUserRoomPermission message.read: %v", err)
	}
	pinEvents, _, err := chatto.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventMessagePinned))
	if err != nil || len(pinEvents) != 1 {
		t.Fatalf("pinned event count = %d, %v", len(pinEvents), err)
	}
	if _, err := chatto.RoomCommands().DeletePinnedMessage(ctx, input); err != nil {
		t.Fatalf("DeletePinnedMessage: %v", err)
	}
	if _, err := chatto.RoomCommands().DeletePinnedMessage(ctx, input); err != nil {
		t.Fatalf("idempotent DeletePinnedMessage: %v", err)
	}
	page, err = chatto.RoomTimelineReads().ListPinnedMessages(ctx, PinnedMessageListInput{ActorID: manager.Id, RoomID: room.Id, Limit: 50})
	if err != nil || len(page.Items) != 0 || page.LatestPinEventID != first.PinEventID {
		t.Fatalf("ListPinnedMessages after delete = %+v, %v", page, err)
	}

	participant, err := chatto.CreateUser(ctx, SystemActorID, "pin-dm-participant", "DM Participant", "password")
	if err != nil {
		t.Fatalf("CreateUser participant: %v", err)
	}
	dm, _, err := chatto.RoomCommands().StartDM(ctx, RoomStartDMInput{ActorID: manager.Id, ParticipantIDs: []string{participant.Id}})
	if err != nil {
		t.Fatalf("StartDM: %v", err)
	}
	if _, err := chatto.RoomCommands().CreatePinnedMessage(ctx, PinnedMessageMutationInput{ActorID: manager.Id, RoomID: dm.Id, MessageEventID: message.Id}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("CreatePinnedMessage DM error = %v, want invalid argument", err)
	}
	if _, err := chatto.RoomTimelineReads().ListPinnedMessages(ctx, PinnedMessageListInput{ActorID: manager.Id, RoomID: dm.Id, Limit: 50}); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("ListPinnedMessages DM error = %v, want invalid argument", err)
	}
}

func TestPinnedMessagesAndReactionsUseThreadInteractions(t *testing.T) {
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chatto.CreateUser(ctx, SystemActorID, "interaction-pin-author", "Interaction Pin Author", "password")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	reader, err := chatto.CreateUser(ctx, SystemActorID, "interaction-pin-reader", "Interaction Pin Reader", "password")
	if err != nil {
		t.Fatalf("CreateUser reader: %v", err)
	}
	room, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "interaction-pins", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{author.GetId(), reader.GetId()} {
		if _, err := chatto.JoinRoom(ctx, userID, KindChannel, userID, room.GetId()); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}
	if err := chatto.GrantUserRoomPermission(ctx, SystemActorID, room.GetId(), author.GetId(), PermRoomManage); err != nil {
		t.Fatalf("GrantUserRoomPermission room.manage: %v", err)
	}
	visibleRoot, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "visible pin root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage visible root: %v", err)
	}
	hiddenRoot, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "hidden pin root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage hidden root: %v", err)
	}
	visiblePin, err := chatto.RoomCommands().CreatePinnedMessage(ctx, PinnedMessageMutationInput{ActorID: author.GetId(), RoomID: room.GetId(), MessageEventID: visibleRoot.GetId()})
	if err != nil {
		t.Fatalf("CreatePinnedMessage visible: %v", err)
	}
	hiddenPin, err := chatto.RoomCommands().CreatePinnedMessage(ctx, PinnedMessageMutationInput{ActorID: author.GetId(), RoomID: room.GetId(), MessageEventID: hiddenRoot.GetId()})
	if err != nil {
		t.Fatalf("CreatePinnedMessage hidden: %v", err)
	}
	if err := chatto.DenyUserRoomPermission(ctx, SystemActorID, room.GetId(), reader.GetId(), PermMessageRead); err != nil {
		t.Fatalf("DenyUserRoomPermission message.read: %v", err)
	}
	if err := chatto.GrantUserRoomPermission(ctx, SystemActorID, room.GetId(), reader.GetId(), PermMessageReadInteractions); err != nil {
		t.Fatalf("GrantUserRoomPermission message.read-interactions: %v", err)
	}
	if _, err := chatto.PostMessage(ctx, KindChannel, room.GetId(), author.GetId(), "pin ping @interaction-pin-reader", nil, visibleRoot.GetId(), "", nil, false); err != nil {
		t.Fatalf("PostMessage mention: %v", err)
	}

	page, err := chatto.RoomTimelineReads().ListPinnedMessages(ctx, PinnedMessageListInput{ActorID: reader.GetId(), RoomID: room.GetId(), Limit: 20})
	if err != nil || len(page.Items) != 1 || page.Items[0].Event.GetId() != visibleRoot.GetId() || page.LatestPinEventID != visiblePin.PinEventID || page.LatestPinEventID == hiddenPin.PinEventID {
		t.Fatalf("interaction-scoped pins = %+v, %v; want only visible pin and marker", page, err)
	}
	if added, err := chatto.ReactionModel().AddReaction(ctx, ReactionMutationInput{
		ActorID: reader.GetId(), RoomID: room.GetId(), MessageEventID: visibleRoot.GetId(), Emoji: "thumbsup",
	}); err != nil || !added {
		t.Fatalf("AddReaction visible root = %v, %v; want added", added, err)
	}
	if _, err := chatto.ReactionModel().AddReaction(ctx, ReactionMutationInput{
		ActorID: reader.GetId(), RoomID: room.GetId(), MessageEventID: hiddenRoot.GetId(), Emoji: "thumbsup",
	}); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("AddReaction hidden root error = %v, want ErrPermissionDenied", err)
	}
}

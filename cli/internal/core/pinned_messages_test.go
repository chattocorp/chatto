package core

import (
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestRoomTimelineProjectionOrdersPinsByDurableSequence(t *testing.T) {
	projection := NewRoomTimelineProjection()
	for sequence, messageID := range []string{"M1", "M2"} {
		posted := newEvent("author", &corev1.Event{Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1"}}})
		posted.Id = messageID
		if err := projection.Apply(posted, uint64(sequence+1)); err != nil {
			t.Fatalf("Apply posted %s: %v", messageID, err)
		}
	}
	first := newEvent("manager", &corev1.Event{Event: &corev1.Event_MessagePinned{MessagePinned: &corev1.MessagePinnedEvent{RoomId: "R1", MessageEventId: "M1"}}})
	first.Id = "P1"
	first.CreatedAt = timestamppb.New(time.Unix(200, 0))
	second := newEvent("manager", &corev1.Event{Event: &corev1.Event_MessagePinned{MessagePinned: &corev1.MessagePinnedEvent{RoomId: "R1", MessageEventId: "M2"}}})
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
	unpinned := newEvent("manager", &corev1.Event{Event: &corev1.Event_MessageUnpinned{MessageUnpinned: &corev1.MessageUnpinnedEvent{RoomId: "R1", MessageEventId: "M2"}}})
	if err := projection.Apply(unpinned, 5); err != nil {
		t.Fatalf("Apply unpin: %v", err)
	}
	if got := projection.LatestPinEventID("R1"); got != "P2" {
		t.Fatalf("LatestPinEventID after unpin = %q, want stable P2", got)
	}
}

func TestRoomTimelineProjectionPinnedMessagesLifecycle(t *testing.T) {
	projection := NewRoomTimelineProjection()
	posted := newEvent("author", &corev1.Event{Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1"}}})
	posted.Id = "M1"
	if err := projection.Apply(posted, 1); err != nil {
		t.Fatalf("Apply posted: %v", err)
	}
	pinned := newEvent("manager", &corev1.Event{Event: &corev1.Event_MessagePinned{MessagePinned: &corev1.MessagePinnedEvent{RoomId: "R1", MessageEventId: "M1"}}})
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

	retracted := newEvent("author", &corev1.Event{Event: &corev1.Event_MessageRetracted{MessageRetracted: &corev1.MessageRetractedEvent{RoomId: "R1", EventId: "M1"}}})
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
	original := newEvent("author", &corev1.Event{Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1", InThread: "ROOT"}}})
	original.Id = "M1"
	echo := newEvent("author", &corev1.Event{Event: &corev1.Event_MessagePosted{MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1", EchoOfEventId: "M1", EchoFromThreadRootEventId: "ROOT"}}})
	echo.Id = "E1"
	pinned := newEvent("manager", &corev1.Event{Event: &corev1.Event_MessagePinned{MessagePinned: &corev1.MessagePinnedEvent{RoomId: "R1", MessageEventId: "M1"}}})
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

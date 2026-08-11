package core

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

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
	if len(items) != 1 || items[0].MessageEventID != "M1" || items[0].ActorID != "manager" {
		t.Fatalf("PinnedMessages = %+v", items)
	}

	snapshot, err := projection.Snapshot()
	if err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	restored := NewRoomTimelineProjection()
	if err := restored.Restore(snapshot); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got := restored.PinnedMessages("R1"); len(got) != 1 || got[0].PinEventID != "P1" {
		t.Fatalf("restored PinnedMessages = %+v", got)
	}

	retracted := newEvent("author", &corev1.Event{Event: &corev1.Event_MessageRetracted{MessageRetracted: &corev1.MessageRetractedEvent{RoomId: "R1", EventId: "M1"}}})
	if err := restored.Apply(retracted, 3); err != nil {
		t.Fatalf("Apply retracted: %v", err)
	}
	if got := restored.PinnedMessages("R1"); len(got) != 0 {
		t.Fatalf("PinnedMessages after retraction = %+v", got)
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
	page, err := chatto.RoomTimelineReads().ListPinnedMessages(ctx, PinnedMessageListInput{ActorID: manager.Id, RoomID: room.Id, Limit: 50})
	if err != nil || len(page.Items) != 1 || page.Items[0].Event.GetId() != message.Id {
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

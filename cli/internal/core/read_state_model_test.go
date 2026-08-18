package core

import (
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func subscribeRoomReadLiveEvents(t *testing.T, nc *nats.Conn, userID string) *nats.Subscription {
	t.Helper()

	sub, err := nc.SubscribeSync(subjects.LiveSyncUserEvent(userID, "room_read"))
	if err != nil {
		t.Fatalf("SubscribeSync room_read: %v", err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatalf("Flush subscription: %v", err)
	}
	return sub
}

func expectRoomReadLiveEvent(t *testing.T, sub *nats.Subscription, roomID string) {
	t.Helper()

	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("waiting for room_read live event: %v", err)
	}
	var live corev1.LiveEvent
	if err := proto.Unmarshal(msg.Data, &live); err != nil {
		t.Fatalf("unmarshal room_read live event: %v", err)
	}
	event := live.GetRoomMarkedAsRead()
	if event == nil {
		t.Fatalf("expected RoomMarkedAsReadEvent, got %T", live.Event)
	}
	if event.GetRoomId() != roomID {
		t.Fatalf("room_read room id = %q, want %q", event.GetRoomId(), roomID)
	}
}

func expectNoRoomReadLiveEvent(t *testing.T, sub *nats.Subscription) {
	t.Helper()

	if msg, err := sub.NextMsg(200 * time.Millisecond); err == nil {
		var live corev1.LiveEvent
		if unmarshalErr := proto.Unmarshal(msg.Data, &live); unmarshalErr != nil {
			t.Fatalf("unexpected room_read live event with invalid payload: %v", unmarshalErr)
		}
		t.Fatalf("unexpected room_read live event: %T", live.Event)
	} else if !errors.Is(err, nats.ErrTimeout) {
		t.Fatalf("waiting for absent room_read live event: %v", err)
	}
}

func TestReadStateModel_MarkRoomAsReadSkipsLiveEventWhenCursorUnchanged(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	poster, _ := core.CreateUser(ctx, "system", "read-signal-poster", "Read Signal Poster", "password123")
	reader, _ := core.CreateUser(ctx, "system", "read-signal-reader", "Read Signal Reader", "password123")
	if _, err := core.JoinRoom(ctx, poster.Id, KindChannel, poster.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom poster: %v", err)
	}
	if _, err := core.JoinRoom(ctx, reader.Id, KindChannel, reader.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom reader: %v", err)
	}

	posted, err := core.PostMessage(ctx, KindChannel, room.Id, poster.Id, "already read", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := core.SetLastReadEventID(ctx, KindChannel, reader.Id, room.Id, posted.Id); err != nil {
		t.Fatalf("SetLastReadEventID: %v", err)
	}

	sub := subscribeRoomReadLiveEvents(t, nc, reader.Id)
	if _, err := core.ReadState().MarkRoomAsRead(ctx, reader.Id, room.Id, ""); err != nil {
		t.Fatalf("MarkRoomAsRead: %v", err)
	}

	expectNoRoomReadLiveEvent(t, sub)
}

func TestReadStateModel_MarkRoomAsReadPublishesLiveEventWhenCursorAdvances(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	poster, _ := core.CreateUser(ctx, "system", "read-advance-poster", "Read Advance Poster", "password123")
	reader, _ := core.CreateUser(ctx, "system", "read-advance-reader", "Read Advance Reader", "password123")
	if _, err := core.JoinRoom(ctx, poster.Id, KindChannel, poster.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom poster: %v", err)
	}
	if _, err := core.JoinRoom(ctx, reader.Id, KindChannel, reader.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom reader: %v", err)
	}

	first, err := core.PostMessage(ctx, KindChannel, room.Id, poster.Id, "first", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage first: %v", err)
	}
	if err := core.SetLastReadEventID(ctx, KindChannel, reader.Id, room.Id, first.Id); err != nil {
		t.Fatalf("SetLastReadEventID: %v", err)
	}
	if _, err := core.PostMessage(ctx, KindChannel, room.Id, poster.Id, "second", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage second: %v", err)
	}

	sub := subscribeRoomReadLiveEvents(t, nc, reader.Id)
	if _, err := core.ReadState().MarkRoomAsRead(ctx, reader.Id, room.Id, ""); err != nil {
		t.Fatalf("MarkRoomAsRead: %v", err)
	}

	expectRoomReadLiveEvent(t, sub, room.Id)
}

func TestReadStateModel_MarkRoomAsReadPublishesLiveEventWhenOccurrencesBecomeRead(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "General", "General discussion")
	poster, _ := core.CreateUser(ctx, "system", "read-notify-poster", "Read Notify Poster", "password123")
	reader, _ := core.CreateUser(ctx, "system", "read-notify-reader", "Read Notify Reader", "password123")
	if _, err := core.JoinRoom(ctx, poster.Id, KindChannel, poster.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom poster: %v", err)
	}
	if _, err := core.JoinRoom(ctx, reader.Id, KindChannel, reader.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom reader: %v", err)
	}

	first, err := core.PostMessage(ctx, KindChannel, room.Id, poster.Id, "first", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage first: %v", err)
	}
	second, err := core.PostMessage(ctx, KindChannel, room.Id, poster.Id, "second", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage second: %v", err)
	}
	if err := core.SetLastReadEventID(ctx, KindChannel, reader.Id, room.Id, second.Id); err != nil {
		t.Fatalf("SetLastReadEventID: %v", err)
	}
	firstEntry, ok := core.roomModel.timelineEntry(first.Id)
	if !ok {
		t.Fatal("first message missing from timeline")
	}
	notification, _, err := core.NotificationOccurrences().Create(ctx, CreateNotificationOccurrenceInput{
		RecipientID:          reader.Id,
		SourceEventID:        first.Id,
		SourceCreated:        first.GetCreatedAt().AsTime(),
		ActorID:              poster.Id,
		Signal:               testNotificationSignal(corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_DIRECT_MENTION, room.Id, first.Id),
		Intensity:            corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
		InitialReadState:     corev1.NotificationReadState_NOTIFICATION_READ_STATE_UNREAD,
		SkipReadLookup:       true,
		SourceStreamSequence: firstEntry.StreamSeq,
	})
	if err != nil {
		t.Fatalf("Create occurrence: %v", err)
	}

	sub := subscribeRoomReadLiveEvents(t, nc, reader.Id)
	if _, err := core.ReadState().MarkRoomAsRead(ctx, reader.Id, room.Id, ""); err != nil {
		t.Fatalf("MarkRoomAsRead: %v", err)
	}

	expectRoomReadLiveEvent(t, sub, room.Id)
	remaining, err := core.NotificationOccurrences().List(ctx, reader.Id)
	if err != nil {
		t.Fatalf("List occurrences: %v", err)
	}
	for _, item := range remaining {
		if item.GetId() == notification.GetId() && item.GetReadState() != corev1.NotificationReadState_NOTIFICATION_READ_STATE_READ {
			t.Fatalf("notification %s state = %v, want read", notification.GetId(), item.GetReadState())
		}
	}
}

func TestReadStateModel_MarkRoomAsReadCoversReactionToReadMessage(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := chattoCore.CreateUser(ctx, SystemActorID, "reaction-read-author", "Reaction Read Author", "password")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	reactor, err := chattoCore.CreateUser(ctx, SystemActorID, "reaction-read-reactor", "Reaction Read Reactor", "password")
	if err != nil {
		t.Fatalf("CreateUser reactor: %v", err)
	}
	room, err := chattoCore.CreateRoom(ctx, author.Id, KindChannel, "", "reaction-read-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	for _, userID := range []string{author.Id, reactor.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, author.Id, "react here", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if added, err := chattoCore.ReactionModel().AddReaction(ctx, ReactionMutationInput{
		ActorID: reactor.Id, RoomID: room.Id, MessageEventID: posted.Id, Emoji: "thumbsup",
	}); err != nil || !added {
		t.Fatalf("AddReaction = (%v, %v)", added, err)
	}
	occurrences, err := chattoCore.NotificationOccurrences().List(ctx, author.Id)
	if err != nil || len(occurrences) != 1 {
		t.Fatalf("List reaction occurrences = (%d, %v), want one", len(occurrences), err)
	}
	if occurrences[0].GetReadState() != corev1.NotificationReadState_NOTIFICATION_READ_STATE_UNREAD {
		t.Fatalf("reaction occurrence starts %v, want UNREAD", occurrences[0].GetReadState())
	}

	if _, err := chattoCore.ReadState().MarkRoomAsRead(ctx, author.Id, room.Id, posted.Id); err != nil {
		t.Fatalf("MarkRoomAsRead: %v", err)
	}
	updated, err := chattoCore.NotificationOccurrences().Get(ctx, author.Id, occurrences[0].GetId())
	if err != nil {
		t.Fatalf("Get reaction occurrence: %v", err)
	}
	if updated.GetReadState() != corev1.NotificationReadState_NOTIFICATION_READ_STATE_READ {
		t.Fatalf("reaction occurrence state = %v, want READ", updated.GetReadState())
	}
}

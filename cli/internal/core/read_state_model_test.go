package core

import (
	"errors"
	"hmans.de/chatto/internal/pb/chatto/core/notification/v1"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core/subjects"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	pubsubv1 "hmans.de/chatto/internal/pb/chatto/core/pubsub/v1"
)

func subscribeRoomReadPubSubEvents(t *testing.T, nc *nats.Conn, userID string) *nats.Subscription {
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

func expectRoomReadPubSubEvent(t *testing.T, sub *nats.Subscription, roomID string) {
	t.Helper()

	msg, err := sub.NextMsg(2 * time.Second)
	if err != nil {
		t.Fatalf("waiting for room_read pubsub event: %v", err)
	}
	var pubsub pubsubv1.PubSubEvent
	if err := proto.Unmarshal(msg.Data, &pubsub); err != nil {
		t.Fatalf("unmarshal room_read pubsub event: %v", err)
	}
	event := pubsub.GetRoomReadStateChanged()
	if event == nil {
		t.Fatalf("expected RoomReadStateChangedEvent, got %T", pubsub.Event)
	}
	if event.GetRoomId() != roomID {
		t.Fatalf("room_read room id = %q, want %q", event.GetRoomId(), roomID)
	}
}

func expectNoRoomReadPubSubEvent(t *testing.T, sub *nats.Subscription) {
	t.Helper()

	if msg, err := sub.NextMsg(200 * time.Millisecond); err == nil {
		var pubsub pubsubv1.PubSubEvent
		if unmarshalErr := proto.Unmarshal(msg.Data, &pubsub); unmarshalErr != nil {
			t.Fatalf("unexpected room_read pubsub event with invalid payload: %v", unmarshalErr)
		}
		t.Fatalf("unexpected room_read pubsub event: %T", pubsub.Event)
	} else if !errors.Is(err, nats.ErrTimeout) {
		t.Fatalf("waiting for absent room_read pubsub event: %v", err)
	}
}

func TestReadStateModel_MarkRoomAsReadSkipsPubSubEventWhenCursorUnchanged(t *testing.T) {
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
	if _, err := core.NotificationPolicy().SetRoomNotificationMode(
		ctx, reader.Id, room.Id, notificationTestSignalRoomMessage,
		evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF,
	); err != nil {
		t.Fatalf("disable room-message Badge: %v", err)
	}

	posted, err := core.PostMessage(ctx, KindChannel, room.Id, poster.Id, "already read", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := core.SetLastReadEventID(ctx, KindChannel, reader.Id, room.Id, posted.Id); err != nil {
		t.Fatalf("SetLastReadEventID: %v", err)
	}

	sub := subscribeRoomReadPubSubEvents(t, nc, reader.Id)
	if _, err := core.ReadState().MarkRoomAsRead(ctx, reader.Id, room.Id, ""); err != nil {
		t.Fatalf("MarkRoomAsRead: %v", err)
	}

	expectNoRoomReadPubSubEvent(t, sub)
}

func TestReadStateModel_MarkRoomAsReadPublishesPubSubEventWhenCursorAdvances(t *testing.T) {
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

	sub := subscribeRoomReadPubSubEvents(t, nc, reader.Id)
	if _, err := core.ReadState().MarkRoomAsRead(ctx, reader.Id, room.Id, ""); err != nil {
		t.Fatalf("MarkRoomAsRead: %v", err)
	}

	expectRoomReadPubSubEvent(t, sub, room.Id)
}

func TestReadStateModel_MarkRoomAsReadPublishesPubSubEventWhenOccurrencesBecomeRead(t *testing.T) {
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
		Signal:               testNotificationSignal(notificationTestSignalDirectMention, room.Id, first.Id),
		Mode:                 evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION,
		AttentionLevel:       notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
		SkipReadLookup:       true,
		SourceStreamSequence: firstEntry.StreamSeq,
	})
	if err != nil {
		t.Fatalf("Create occurrence: %v", err)
	}

	sub := subscribeRoomReadPubSubEvents(t, nc, reader.Id)
	if _, err := core.ReadState().MarkRoomAsRead(ctx, reader.Id, room.Id, ""); err != nil {
		t.Fatalf("MarkRoomAsRead: %v", err)
	}

	expectRoomReadPubSubEvent(t, sub, room.Id)
	remaining, err := core.NotificationOccurrences().List(ctx, reader.Id)
	if err != nil {
		t.Fatalf("List occurrences: %v", err)
	}
	for _, item := range remaining {
		if item.GetId() == notification.GetId() && !item.GetRead() {
			t.Fatalf("notification %s remains unread", notification.GetId())
		}
	}
}

func TestNotificationReadBoundaryReconciliationRepairsInterruptedHandshake(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	room, _ := chattoCore.CreateRoom(ctx, "test-user", KindChannel, "", "Read repair", "")
	poster, _ := chattoCore.CreateUser(ctx, SystemActorID, "read-repair-poster", "Read Repair Poster", "password123")
	reader, _ := chattoCore.CreateUser(ctx, SystemActorID, "read-repair-reader", "Read Repair Reader", "password123")
	for _, userID := range []string{poster.Id, reader.Id} {
		if _, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id); err != nil {
			t.Fatalf("JoinRoom %s: %v", userID, err)
		}
	}
	posted, err := chattoCore.PostMessage(ctx, KindChannel, room.Id, poster.Id, "covered", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	entry, ok := chattoCore.roomModel.timelineEntry(posted.Id)
	if !ok {
		t.Fatal("posted message missing from timeline")
	}
	occurrence, _, err := chattoCore.NotificationOccurrences().Create(ctx, CreateNotificationOccurrenceInput{
		RecipientID: reader.Id, SourceEventID: posted.Id, SourceCreated: posted.GetCreatedAt().AsTime(), ActorID: poster.Id,
		Signal: testNotificationSignal(notificationTestSignalDirectMention, room.Id, posted.Id),
		Mode:   evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION, AttentionLevel: notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
		SourceStreamSequence: entry.StreamSeq, SkipReadLookup: true,
	})
	if err != nil {
		t.Fatalf("Create occurrence: %v", err)
	}

	before, err := chattoCore.NotificationOccurrences().Get(ctx, reader.Id, occurrence.GetId())
	if err != nil || before.GetRead() {
		t.Fatalf("before repair = %+v, %v, want unread", before, err)
	}
	// Simulate a stop after the durable boundary write but before the matching
	// NOTIFICATIONS read fact was appended.
	if _, err := chattoCore.NotificationOccurrences().recordNotificationReadBoundary(ctx, reader.Id, room.Id, "", posted.Id); err != nil {
		t.Fatalf("record read boundary: %v", err)
	}
	for {
		after, err := chattoCore.NotificationOccurrences().Get(ctx, reader.Id, occurrence.GetId())
		if err == nil && after.GetRead() {
			break
		}
		select {
		case <-ctx.Done():
			t.Fatalf("watched boundary did not repair occurrence: %+v, %v", after, err)
		case <-time.After(time.Millisecond):
		}
	}
	if repaired, err := chattoCore.NotificationOccurrences().reconcileCoveredUnread(ctx); err != nil || repaired != 0 {
		t.Fatalf("idempotent reconcileCoveredUnread = (%d, %v), want (0, nil)", repaired, err)
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
	if occurrences[0].GetRead() {
		t.Fatal("reaction occurrence starts read, want unread")
	}

	if _, err := chattoCore.ReadState().MarkRoomAsRead(ctx, author.Id, room.Id, posted.Id); err != nil {
		t.Fatalf("MarkRoomAsRead: %v", err)
	}
	updated, err := chattoCore.NotificationOccurrences().Get(ctx, author.Id, occurrences[0].GetId())
	if err != nil {
		t.Fatalf("Get reaction occurrence: %v", err)
	}
	if !updated.GetRead() {
		t.Fatal("reaction occurrence remains unread")
	}
}

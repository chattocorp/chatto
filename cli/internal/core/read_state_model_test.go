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

func TestReadStateModel_MarkRoomAsReadPublishesLiveEventWhenNotificationsDismissed(t *testing.T) {
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
	notification, err := core.CreateNotification(ctx, reader.Id, poster.Id, &corev1.Notification{
		Notification: &corev1.Notification_Mention{
			Mention: &corev1.MentionNotification{RoomId: room.Id, EventId: first.Id},
		},
	})
	if err != nil {
		t.Fatalf("CreateNotification: %v", err)
	}

	sub := subscribeRoomReadLiveEvents(t, nc, reader.Id)
	if _, err := core.ReadState().MarkRoomAsRead(ctx, reader.Id, room.Id, ""); err != nil {
		t.Fatalf("MarkRoomAsRead: %v", err)
	}

	expectRoomReadLiveEvent(t, sub, room.Id)
	remaining, err := core.GetNotifications(ctx, reader.Id)
	if err != nil {
		t.Fatalf("GetNotifications: %v", err)
	}
	for _, item := range remaining {
		if item.GetId() == notification.GetId() {
			t.Fatalf("notification %s was not dismissed", notification.GetId())
		}
	}
}

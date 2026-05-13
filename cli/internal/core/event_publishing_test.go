package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestEventPublishingHelpers_RejectInvalidEvents(t *testing.T) {
	core := &ChattoCore{}
	ctx := testContext(t)

	t.Run("publishServerEvent rejects nil pointer", func(t *testing.T) {
		err := core.publishServerEvent(ctx, "space.test", nil)
		if !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("expected ErrInvalidEvent, got: %v", err)
		}
	})

	t.Run("publishServerEvent rejects unset oneof payload", func(t *testing.T) {
		err := core.publishServerEvent(ctx, "space.test", &corev1.Event{})
		if !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("expected ErrInvalidEvent, got: %v", err)
		}
	})

	t.Run("publishLiveServerEvent rejects invalid payload", func(t *testing.T) {
		err := core.publishLiveServerEvent(ctx, "live.space.test", &corev1.Event{})
		if !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("expected ErrInvalidEvent, got: %v", err)
		}
	})

	t.Run("publishLiveEvent rejects invalid payload", func(t *testing.T) {
		err := core.publishLiveEvent(ctx, "live.server.test", &corev1.Event{})
		if !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("expected ErrInvalidEvent, got: %v", err)
		}
	})

	t.Run("publishServerEventWithAck rejects invalid payload", func(t *testing.T) {
		seq, err := core.publishServerEventWithAck(ctx, "space.test", &corev1.Event{})
		if seq != 0 {
			t.Fatalf("expected sequence 0 on error, got: %d", seq)
		}
		if !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("expected ErrInvalidEvent, got: %v", err)
		}
	})

	t.Run("publishServerEventWithOCC rejects invalid payload", func(t *testing.T) {
		seq, err := core.publishServerEventWithOCC(ctx, "space.test", &corev1.Event{})
		if seq != 0 {
			t.Fatalf("expected sequence 0 on error, got: %d", seq)
		}
		if !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("expected ErrInvalidEvent, got: %v", err)
		}
	})
}

// setupRoomWithMessage creates a user, a room, joins the user, and posts one
// message. Returns the resulting MessagePostedEvent so the test can pull
// MessageBodyId / event id off it.
func setupRoomWithMessage(t *testing.T, core *ChattoCore, ctx context.Context, body string) (room, user struct{ Id string }, event *corev1.Event) {
	t.Helper()

	createdUser, err := core.CreateUser(ctx, "system", "msguser", "msguser", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	createdRoom, err := core.CreateRoom(ctx, createdUser.Id, KindChannel, "", "general", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, createdUser.Id, KindChannel, createdUser.Id, createdRoom.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	posted, err := core.PostMessage(ctx, KindChannel, createdRoom.Id, createdUser.Id, body, nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	room.Id = createdRoom.Id
	user.Id = createdUser.Id
	event = posted
	return
}

// TestDeleteMessage_PublishesLiveEvent verifies that calling DeleteMessage on
// the core publishes a MessageDeletedEvent on the live room subject. This is
// the publish side of the chain — without it, no client receives the
// deletion. A future refactor that drops the publish would silently break the
// frontend's ability to update.
func TestDeleteMessage_PublishesLiveEvent(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := testContext(t)

	room, user, event := setupRoomWithMessage(t, core, ctx, "delete me")
	posted := event.GetMessagePosted()
	if posted == nil {
		t.Fatal("expected MessagePostedEvent")
	}

	subject := subjects.LiveRoomEvent(string(KindChannel), room.Id, "message_deleted")
	received := make(chan *nats.Msg, 1)
	sub, err := nc.Subscribe(subject, func(msg *nats.Msg) {
		select {
		case received <- msg:
		default:
		}
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	if err := core.DeleteMessage(ctx, user.Id, KindChannel, room.Id, posted.MessageBodyId); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	_ = nc.Flush()

	select {
	case msg := <-received:
		var got corev1.Event
		if err := proto.Unmarshal(msg.Data, &got); err != nil {
			t.Fatalf("unmarshal published event: %v", err)
		}
		deleted := got.GetMessageDeleted()
		if deleted == nil {
			t.Fatalf("expected MessageDeletedEvent, got %T", got.Event)
		}
		if deleted.RoomId != room.Id {
			t.Errorf("RoomId = %q, want %q", deleted.RoomId, room.Id)
		}
		if deleted.SpaceId != ServerSpaceID {
			t.Errorf("SpaceId = %q, want %q", deleted.SpaceId, ServerSpaceID)
		}
		if deleted.MessageEventId != event.Id {
			t.Errorf("MessageEventId = %q, want %q", deleted.MessageEventId, event.Id)
		}
		if got.ActorId != user.Id {
			t.Errorf("ActorId = %q, want %q", got.ActorId, user.Id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for MessageDeletedEvent on live subject")
	}
}

// TestEditMessage_PublishesLiveEvent verifies that calling EditMessage
// publishes a MessageUpdatedEvent on the live room subject — same contract as
// the deletion path.
func TestEditMessage_PublishesLiveEvent(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := testContext(t)

	room, user, event := setupRoomWithMessage(t, core, ctx, "original")
	posted := event.GetMessagePosted()
	if posted == nil {
		t.Fatal("expected MessagePostedEvent")
	}

	subject := subjects.LiveRoomEvent(string(KindChannel), room.Id, "message_updated")
	received := make(chan *nats.Msg, 1)
	sub, err := nc.Subscribe(subject, func(msg *nats.Msg) {
		select {
		case received <- msg:
		default:
		}
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	if err := core.EditMessage(ctx, user.Id, KindChannel, room.Id, posted.MessageBodyId, "edited"); err != nil {
		t.Fatalf("EditMessage: %v", err)
	}
	_ = nc.Flush()

	select {
	case msg := <-received:
		var got corev1.Event
		if err := proto.Unmarshal(msg.Data, &got); err != nil {
			t.Fatalf("unmarshal published event: %v", err)
		}
		updated := got.GetMessageUpdated()
		if updated == nil {
			t.Fatalf("expected MessageUpdatedEvent, got %T", got.Event)
		}
		if updated.RoomId != room.Id {
			t.Errorf("RoomId = %q, want %q", updated.RoomId, room.Id)
		}
		if updated.MessageEventId != event.Id {
			t.Errorf("MessageEventId = %q, want %q", updated.MessageEventId, event.Id)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for MessageUpdatedEvent on live subject")
	}
}

// TestStreamMyEvents_DeliversMessageDeleted is the integration test for
// the room-id-extraction switch in StreamMyEvents (cli/internal/core/core.go).
// If a future refactor drops the MessageDeleted case from that switch, the
// event would be silently dropped (the rule doc explicitly warns about this).
// This test catches that regression by subscribing as a real space member and
// asserting the event flows through end-to-end.
func TestStreamMyEvents_DeliversMessageDeleted(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	author, err := core.CreateUser(ctx, "system", "author", "Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	viewer, err := core.CreateUser(ctx, "system", "viewer", "Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}

	room, err := core.CreateRoom(ctx, author.Id, KindChannel, "", "general", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, author.Id, KindChannel, author.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom author: %v", err)
	}
	if _, err := core.JoinRoom(ctx, viewer.Id, KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom viewer: %v", err)
	}

	posted, err := core.PostMessage(ctx, KindChannel, room.Id, author.Id, "hello", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	postedMsg := posted.GetMessagePosted()
	if postedMsg == nil {
		t.Fatal("expected MessagePostedEvent")
	}

	// Subscribe as viewer — they should receive the deletion event.
	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	eventChan, err := core.StreamMyEvents(subCtx, viewer.Id)
	if err != nil {
		t.Fatalf("StreamMyEvents: %v", err)
	}

	// Let subscription establish before publishing.
	time.Sleep(100 * time.Millisecond)

	if err := core.DeleteMessage(ctx, author.Id, KindChannel, room.Id, postedMsg.MessageBodyId); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}

	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev := <-eventChan:
			deleted := ev.GetMessageDeleted()
			if deleted == nil {
				continue
			}
			if deleted.RoomId != room.Id {
				t.Errorf("RoomId = %q, want %q", deleted.RoomId, room.Id)
			}
			if deleted.MessageEventId != posted.Id {
				t.Errorf("MessageEventId = %q, want %q", deleted.MessageEventId, posted.Id)
			}
			return
		case <-timeout:
			t.Fatal("viewer never received MessageDeletedEvent — room-id extraction switch likely dropped it")
		}
	}
}

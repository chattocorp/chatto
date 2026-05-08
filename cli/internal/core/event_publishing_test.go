package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/core/subjects"
)

func TestEventPublishingHelpers_RejectInvalidEvents(t *testing.T) {
	core := &ChattoCore{}
	ctx := testContext(t)

	t.Run("publishSpaceEvent rejects nil pointer", func(t *testing.T) {
		err := core.publishSpaceEvent(ctx, "space.test", nil)
		if !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("expected ErrInvalidEvent, got: %v", err)
		}
	})

	t.Run("publishSpaceEvent rejects unset oneof payload", func(t *testing.T) {
		err := core.publishSpaceEvent(ctx, "space.test", &corev1.SpaceEvent{})
		if !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("expected ErrInvalidEvent, got: %v", err)
		}
	})

	t.Run("publishLiveSpaceEvent rejects invalid payload", func(t *testing.T) {
		err := core.publishLiveSpaceEvent(ctx, "live.space.test", &corev1.SpaceEvent{})
		if !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("expected ErrInvalidEvent, got: %v", err)
		}
	})

	t.Run("publishInstanceEvent rejects invalid payload", func(t *testing.T) {
		err := core.publishInstanceEvent(ctx, "live.instance.test", &corev1.InstanceEvent{})
		if !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("expected ErrInvalidEvent, got: %v", err)
		}
	})

	t.Run("publishSpaceEventWithAck rejects invalid payload", func(t *testing.T) {
		seq, err := core.publishSpaceEventWithAck(ctx, "space.test", &corev1.SpaceEvent{})
		if seq != 0 {
			t.Fatalf("expected sequence 0 on error, got: %d", seq)
		}
		if !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("expected ErrInvalidEvent, got: %v", err)
		}
	})

	t.Run("publishSpaceEventWithOCC rejects invalid payload", func(t *testing.T) {
		seq, err := core.publishSpaceEventWithOCC(ctx, "space123", "space.test", &corev1.SpaceEvent{})
		if seq != 0 {
			t.Fatalf("expected sequence 0 on error, got: %d", seq)
		}
		if !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("expected ErrInvalidEvent, got: %v", err)
		}
	})
}

// setupRoomWithMessage creates a space, a user, a room, joins the user, and
// posts one message. Returns the resulting MessagePostedEvent so the test can
// pull MessageBodyId / event id off it.
func setupRoomWithMessage(t *testing.T, core *ChattoCore, ctx context.Context, body string) (space, room, user struct{ Id string }, event *corev1.SpaceEvent) {
	t.Helper()

	createdSpace, err := core.CreateSpace(ctx, "system", "Test Space", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	createdUser, err := core.CreateUser(ctx, "system", "msguser", "msguser", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := core.JoinSpace(ctx, createdUser.Id, createdSpace.Id); err != nil {
		t.Fatalf("JoinSpace: %v", err)
	}
	createdRoom, err := core.CreateRoom(ctx, createdUser.Id, createdSpace.Id, "general", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, createdUser.Id, createdSpace.Id, createdUser.Id, createdRoom.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	posted, err := core.PostMessage(ctx, createdSpace.Id, createdRoom.Id, createdUser.Id, body, nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	space.Id = createdSpace.Id
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

	space, room, user, event := setupRoomWithMessage(t, core, ctx, "delete me")
	posted := event.GetMessagePosted()
	if posted == nil {
		t.Fatal("expected MessagePostedEvent")
	}

	subject := subjects.LiveRoomEvent(kindForSpace(space.Id), room.Id, "message_deleted")
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

	if err := core.DeleteMessage(ctx, user.Id, space.Id, room.Id, posted.MessageBodyId); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}
	_ = nc.Flush()

	select {
	case msg := <-received:
		var got corev1.SpaceEvent
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
		if deleted.SpaceId != space.Id {
			t.Errorf("SpaceId = %q, want %q", deleted.SpaceId, space.Id)
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

	space, room, user, event := setupRoomWithMessage(t, core, ctx, "original")
	posted := event.GetMessagePosted()
	if posted == nil {
		t.Fatal("expected MessagePostedEvent")
	}

	subject := subjects.LiveRoomEvent(kindForSpace(space.Id), room.Id, "message_updated")
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

	if err := core.EditMessage(ctx, user.Id, space.Id, room.Id, posted.MessageBodyId, "edited"); err != nil {
		t.Fatalf("EditMessage: %v", err)
	}
	_ = nc.Flush()

	select {
	case msg := <-received:
		var got corev1.SpaceEvent
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

// TestStreamMySpaceEvents_DeliversMessageDeleted is the integration test for
// the room-id-extraction switch in StreamMySpaceEvents (cli/internal/core/core.go).
// If a future refactor drops the MessageDeleted case from that switch, the
// event would be silently dropped (the rule doc explicitly warns about this).
// This test catches that regression by subscribing as a real space member and
// asserting the event flows through end-to-end.
func TestStreamMySpaceEvents_DeliversMessageDeleted(t *testing.T) {
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

	space, err := core.CreateSpace(ctx, author.Id, "Test Space", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	if _, err := core.JoinSpace(ctx, viewer.Id, space.Id); err != nil {
		t.Fatalf("JoinSpace viewer: %v", err)
	}
	room, err := core.CreateRoom(ctx, author.Id, space.Id, "general", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, author.Id, space.Id, author.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom author: %v", err)
	}
	if _, err := core.JoinRoom(ctx, viewer.Id, space.Id, viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom viewer: %v", err)
	}

	posted, err := core.PostMessage(ctx, space.Id, room.Id, author.Id, "hello", nil, "", "", nil, false)
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
	eventChan, err := core.StreamMySpaceEvents(subCtx, space.Id, viewer.Id)
	if err != nil {
		t.Fatalf("StreamMySpaceEvents: %v", err)
	}

	// Let subscription establish before publishing.
	time.Sleep(100 * time.Millisecond)

	if err := core.DeleteMessage(ctx, author.Id, space.Id, room.Id, postedMsg.MessageBodyId); err != nil {
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

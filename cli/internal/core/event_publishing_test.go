package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestEventPublishingHelpers_RejectInvalidEvents(t *testing.T) {
	core := &ChattoCore{}
	ctx := testContext(t)

	t.Run("publishLiveEvent rejects invalid payload", func(t *testing.T) {
		err := core.publishLiveEvent(ctx, "live.sync.test", &corev1.LiveEvent{})
		if !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("expected ErrInvalidEvent, got: %v", err)
		}
	})
}

func TestRoomMutationsDoNotWriteServerEvents(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "serverevents-user", "Server Events User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	other, err := core.CreateUser(ctx, "system", "serverevents-other", "Server Events Other", "password123")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}

	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "serverevents_room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if _, err := core.UpdateRoom(ctx, user.Id, KindChannel, room.Id, "serverevents_room_2", "updated"); err != nil {
		t.Fatalf("UpdateRoom: %v", err)
	}
	if _, err := core.ArchiveRoom(ctx, user.Id, KindChannel, room.Id); err != nil {
		t.Fatalf("ArchiveRoom: %v", err)
	}
	if _, err := core.UnarchiveRoom(ctx, user.Id, KindChannel, room.Id); err != nil {
		t.Fatalf("UnarchiveRoom: %v", err)
	}
	if _, _, err := core.FindOrCreateDM(ctx, user.Id, []string{other.Id}); err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	if err := core.DeleteRoom(ctx, user.Id, KindChannel, room.Id); err != nil {
		t.Fatalf("DeleteRoom: %v", err)
	}

	count, err := countStreamMessages(ctx, core.storage.serverEventsStream, []string{"server.>"})
	if err != nil {
		t.Fatalf("count SERVER_EVENTS messages: %v", err)
	}
	if count != 0 {
		t.Fatalf("SERVER_EVENTS got %d runtime messages, want 0", count)
	}
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

// TestStreamMyEvents_DeliversMessageRetracted is the integration test for
// the room-id-extraction switch in StreamMyEvents (cli/internal/core/core.go).
// If a future refactor drops the MessageRetracted case from that switch, the
// event would be silently dropped (the rule doc explicitly warns about this).
// This test catches that regression by subscribing as a real space member and
// asserting the event flows through end-to-end.
func TestStreamMyEvents_DeliversMessageRetracted(t *testing.T) {
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

	// StreamMyEvents receives the canonical live.evt.> republish.
	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev := <-eventChan:
			retracted := ev.GetMessageRetracted()
			if retracted == nil {
				continue
			}
			if retracted.RoomId != room.Id {
				t.Errorf("RoomId = %q, want %q", retracted.RoomId, room.Id)
			}
			if retracted.EventId != posted.Id {
				t.Errorf("EventId = %q, want %q", retracted.EventId, posted.Id)
			}
			return
		case <-timeout:
			t.Fatal("viewer never received MessageRetractedEvent from live.evt republish")
		}
	}
}

func TestStreamMyEvents_DeliversDMEventsWhenMessagePostDenied(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	creator, err := core.CreateUser(ctx, "system", "dm-creator", "DM Creator", "password123")
	if err != nil {
		t.Fatalf("CreateUser creator: %v", err)
	}
	target, err := core.CreateUser(ctx, "system", "dm-target", "DM Target", "password123")
	if err != nil {
		t.Fatalf("CreateUser target: %v", err)
	}
	if err := core.DenyServerPermission(ctx, RoleEveryone, PermMessagePost); err != nil {
		t.Fatalf("DenyServerPermission message.post: %v", err)
	}
	canPostMessage, err := core.HasServerPermission(ctx, target.Id, PermMessagePost)
	if err != nil {
		t.Fatalf("HasServerPermission message.post: %v", err)
	}
	if canPostMessage {
		t.Fatal("target should not have message.post")
	}

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	eventChan, err := core.StreamMyEvents(subCtx, target.Id)
	if err != nil {
		t.Fatalf("StreamMyEvents: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	room, created, err := core.FindOrCreateDM(ctx, creator.Id, []string{target.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	if !created {
		t.Fatal("expected new DM room")
	}
	if _, err := core.PostMessage(ctx, KindDM, room.Id, creator.Id, "private hello", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev, ok := <-eventChan:
			if !ok {
				t.Fatal("event stream closed unexpectedly")
			}
			if liveEventRoomID(ev) == room.Id && ev.GetMessagePosted() != nil {
				return
			}
		case <-timeout:
			t.Fatal("target did not receive DM message after message.post was denied")
		}
	}
}

func TestStreamMyEvents_DeliversRawEVTRepublish(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	author, err := core.CreateUser(ctx, "system", "evt-author", "EVT Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	viewer, err := core.CreateUser(ctx, "system", "evt-viewer", "EVT Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	room, err := core.CreateRoom(ctx, author.Id, KindChannel, "", "evt-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, author.Id, KindChannel, author.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom author: %v", err)
	}
	if _, err := core.JoinRoom(ctx, viewer.Id, KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom viewer: %v", err)
	}

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	eventChan, err := core.StreamMyEvents(subCtx, viewer.Id)
	if err != nil {
		t.Fatalf("StreamMyEvents: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	event := newEvent(author.Id, &corev1.Event{
		Event: &corev1.Event_MessageEdited{
			MessageEdited: &corev1.MessageEditedEvent{
				RoomId:  room.Id,
				EventId: "E-raw-evt",
				Body: &corev1.MessageBody{
					AuthorId:      author.Id,
					EncryptedBody: []byte("edited through raw EVT"),
				},
			},
		},
	})
	if _, err := core.RoomTimelineProjector.AppendEventuallyAndWait(ctx, core.EventPublisher, events.RoomAggregate(room.Id), event); err != nil {
		t.Fatalf("append raw EVT event: %v", err)
	}

	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev := <-eventChan:
			edited := ev.GetMessageEdited()
			if edited == nil {
				continue
			}
			if edited.EventId != "E-raw-evt" {
				t.Errorf("EventId = %q, want E-raw-evt", edited.EventId)
			}
			return
		case <-timeout:
			t.Fatal("viewer never received MessageEditedEvent from live.evt republish")
		}
	}
}

func liveEventRoomID(event *corev1.Event) string {
	switch e := event.GetEvent().(type) {
	case *corev1.Event_RoomCreated:
		return e.RoomCreated.GetRoomId()
	case *corev1.Event_RoomUpdated:
		return e.RoomUpdated.GetRoomId()
	case *corev1.Event_RoomDeleted:
		return e.RoomDeleted.GetRoomId()
	case *corev1.Event_RoomArchived:
		return e.RoomArchived.GetRoomId()
	case *corev1.Event_RoomUnarchived:
		return e.RoomUnarchived.GetRoomId()
	case *corev1.Event_UserJoinedRoom:
		return e.UserJoinedRoom.GetRoomId()
	case *corev1.Event_UserLeftRoom:
		return e.UserLeftRoom.GetRoomId()
	case *corev1.Event_MessagePosted:
		return e.MessagePosted.GetRoomId()
	case *corev1.Event_MessageEdited:
		return e.MessageEdited.GetRoomId()
	case *corev1.Event_MessageRetracted:
		return e.MessageRetracted.GetRoomId()
	case *corev1.Event_ReactionAdded:
		return e.ReactionAdded.GetRoomId()
	case *corev1.Event_ReactionRemoved:
		return e.ReactionRemoved.GetRoomId()
	default:
		return ""
	}
}

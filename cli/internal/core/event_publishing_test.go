package core

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

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

	if _, err := core.js.Stream(ctx, "SERVER_EVENTS"); !errors.Is(err, jetstream.ErrStreamNotFound) {
		t.Fatalf("legacy stream SERVER_EVENTS lookup error = %v, want ErrStreamNotFound", err)
	}
}

// setupRoomWithMessage creates a user, a room, joins the user, and posts one
// message. Returns the resulting event so the test can use the durable envelope id.
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

	if err := core.DeleteMessage(ctx, author.Id, KindChannel, room.Id, posted.Id); err != nil {
		t.Fatalf("DeleteMessage: %v", err)
	}

	// StreamMyEvents receives the canonical live.evt.> republish.
	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev := <-eventChan:
			retracted := EventMessageRetracted(ev)
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

func TestStreamMyEvents_DoesNotDeliverMessageBodyEvent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	author, err := core.CreateUser(ctx, "system", "body-event-author", "Body Event Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	viewer, err := core.CreateUser(ctx, "system", "body-event-viewer", "Body Event Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}

	room, err := core.CreateRoom(ctx, author.Id, KindChannel, "", "body-event-room", "")
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

	posted, err := core.PostMessage(ctx, KindChannel, room.Id, author.Id, "private payload should not stream", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}

	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev := <-eventChan:
			if evt := ev.EVTEvent(); evt != nil && evt.GetMessageBody() != nil {
				t.Fatal("StreamMyEvents delivered private MessageBodyEvent")
			}
			msg := EventMessagePosted(ev)
			if msg == nil {
				continue
			}
			if ev.ID() != posted.Id {
				t.Fatalf("posted event id = %q, want %q", ev.ID(), posted.Id)
			}
			return
		case <-timeout:
			t.Fatal("viewer never received public MessagePostedEvent")
		}
	}
}

func TestStreamMyEvents_ClosesWhenLiveEVTProjectionReadinessFails(t *testing.T) {
	harness := newTestEventHarness(t)
	ctx := testContext(t)
	roomID := "R-projection-fail"
	userID := "U-projection-fail"
	event := &corev1.Event{
		Id:      "E-projection-fail",
		ActorId: userID,
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{RoomId: roomID},
		},
	}
	subject := events.RoomAggregate(roomID).SubjectFor(event)
	seq, err := harness.publisher.Append(ctx, subject, event)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}

	// Use a projector whose subject filters do not consume room events. The
	// projection readiness wait fails immediately, matching the production
	// failure mode without waiting for the timeout.
	wrongProjector := harness.projector(NewAssetProjection())
	core := &ChattoCore{logger: testCoreLogger()}
	core.roomModel = newRoomModel(
		nil,
		nil,
		nil,
		nil,
		NewRoomTimelineProjection(),
		wrongProjector,
		NewThreadProjection(),
		wrongProjector,
		nil,
		nil,
	)
	service := NewMyEventsModel(core)
	msg := &nats.Msg{
		Subject: events.LiveSubjectRoot + strings.TrimPrefix(subject, events.SubjectRoot),
		Header:  nats.Header{nats.JSSequence: []string{strconv.FormatUint(seq, 10)}},
	}

	delivered, ok, closeStream := service.filterLiveEVTEvent(ctx, userID, map[string]struct{}{roomID: {}}, msg, event)
	if delivered != nil || ok {
		t.Fatalf("filterLiveEVTEvent delivered %T/%v, want dropped", delivered, ok)
	}
	if !closeStream {
		t.Fatal("filterLiveEVTEvent closeStream = false, want true")
	}
}

func TestMyEventsFilter_DeliversUniversalDisableToPriorEffectiveMember(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	actor, err := core.CreateUser(ctx, "system", "universal-disable-actor", "Universal Actor", "password123")
	if err != nil {
		t.Fatalf("CreateUser actor: %v", err)
	}
	viewer, err := core.CreateUser(ctx, "system", "universal-disable-viewer", "Universal Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	room, err := core.CreateRoom(ctx, actor.Id, KindChannel, "", "universal-disable-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.SetRoomUniversal(ctx, actor.Id, KindChannel, room.Id, true); err != nil {
		t.Fatalf("SetRoomUniversal true: %v", err)
	}
	if exists, err := core.RoomMembershipExists(ctx, KindChannel, viewer.Id, room.Id); err != nil || !exists {
		t.Fatalf("RoomMembershipExists before disable = %v, %v; want true, nil", exists, err)
	}
	if _, err := core.SetRoomUniversal(ctx, actor.Id, KindChannel, room.Id, false); err != nil {
		t.Fatalf("SetRoomUniversal false: %v", err)
	}
	if exists, err := core.RoomMembershipExists(ctx, KindChannel, viewer.Id, room.Id); err != nil || exists {
		t.Fatalf("RoomMembershipExists after disable = %v, %v; want false, nil", exists, err)
	}

	service := NewMyEventsModel(core)
	memberRooms := map[string]struct{}{room.Id: {}}
	event := &corev1.Event{
		Id:      NewEventID(),
		ActorId: actor.Id,
		Event: &corev1.Event_RoomUniversalChanged{
			RoomUniversalChanged: &corev1.RoomUniversalChangedEvent{
				RoomId:    room.Id,
				Universal: false,
			},
		},
	}

	delivered, ok := service.filterReadyEVTRoomSubjectEvent(viewer.Id, memberRooms, room.Id, event, 123)
	if !ok || delivered == nil {
		t.Fatalf("filterReadyEVTRoomSubjectEvent delivered %T/%v, want RoomUniversalChangedEvent", delivered, ok)
	}
	if delivered.EVTEvent() != event {
		t.Fatalf("delivered EVT event = %p, want %p", delivered.EVTEvent(), event)
	}
	if delivered.DeliverySeq() != 123 {
		t.Fatalf("DeliverySeq = %d, want 123", delivered.DeliverySeq())
	}
	if _, stillCached := memberRooms[room.Id]; stillCached {
		t.Fatal("memberRooms still contains room after universal disable")
	}

	nextEvent := &corev1.Event{
		Id:      NewEventID(),
		ActorId: actor.Id,
		Event: &corev1.Event_RoomUpdated{
			RoomUpdated: &corev1.RoomUpdatedEvent{RoomId: room.Id},
		},
	}
	delivered, ok = service.filterReadyEVTRoomSubjectEvent(viewer.Id, memberRooms, room.Id, nextEvent, 124)
	if ok || delivered != nil {
		t.Fatalf("next room event delivered %T/%v after universal disable, want dropped", delivered, ok)
	}
}

func TestStreamMyEvents_DeleteEchoDeliversOnlyEchoRetract(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	author, err := core.CreateUser(ctx, "system", "echo-author", "Echo Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	viewer, err := core.CreateUser(ctx, "system", "echo-viewer", "Echo Viewer", "password123")
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

	root, err := core.PostMessage(ctx, KindChannel, room.Id, author.Id, "root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Post root: %v", err)
	}
	reply, err := core.PostMessage(ctx, KindChannel, room.Id, author.Id, "reply with echo", nil, root.Id, "", nil, true)
	if err != nil {
		t.Fatalf("Post reply with echo: %v", err)
	}
	roomEvents, err := core.GetRoomEvents(ctx, KindChannel, room.Id, 50, nil)
	if err != nil {
		t.Fatalf("GetRoomEvents: %v", err)
	}
	echoID := ""
	for _, event := range roomEvents.Events {
		if msg := event.GetMessagePosted(); msg != nil && msg.GetEchoOfEventId() == reply.Id {
			echoID = event.Id
			break
		}
	}
	if echoID == "" {
		t.Fatal("expected echoed reply in room events")
	}

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	eventChan, err := core.StreamMyEvents(subCtx, viewer.Id)
	if err != nil {
		t.Fatalf("StreamMyEvents: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if err := core.DeleteMessage(ctx, author.Id, KindChannel, room.Id, echoID); err != nil {
		t.Fatalf("Delete echo: %v", err)
	}

	timeout := time.After(300 * time.Millisecond)
	seenEchoRetract := false
	for {
		select {
		case ev := <-eventChan:
			retracted := EventMessageRetracted(ev)
			if retracted == nil {
				continue
			}
			if retracted.GetEventId() == reply.Id {
				t.Fatal("deleting echo should not deliver a retraction for the original reply")
			}
			if retracted.GetEventId() == echoID {
				seenEchoRetract = true
			}
		case <-timeout:
			if !seenEchoRetract {
				t.Fatal("viewer never received MessageRetractedEvent for echo")
			}
			return
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
	if err := core.DenyServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost); err != nil {
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
			if liveEventRoomID(ev) == room.Id && EventMessagePosted(ev) != nil {
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
			edited := EventMessageEdited(ev)
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

func liveEventRoomID(event EventEnvelope) string {
	evt := event.EVTEvent()
	if evt == nil {
		return ""
	}
	switch e := evt.GetEvent().(type) {
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

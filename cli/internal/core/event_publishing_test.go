package core

import (
	"context"
	"errors"
	"testing"
	"time"

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
	eventChan, err := core.StreamMyEvents(subCtx, viewer.Id, 0)
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
	eventChan, err := core.StreamMyEvents(subCtx, viewer.Id, 0)
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
	eventChan, err := core.StreamMyEvents(subCtx, viewer.Id, 0)
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
	eventChan, err := core.StreamMyEvents(subCtx, target.Id, 0)
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
	eventChan, err := core.StreamMyEvents(subCtx, viewer.Id, 0)
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

func TestStreamMyEvents_ReplaysMissedReactionAfterCursor(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	author, err := core.CreateUser(ctx, "system", "replay-reaction-author", "Replay Reaction Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	viewer, err := core.CreateUser(ctx, "system", "replay-reaction-viewer", "Replay Reaction Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	room, err := core.CreateRoom(ctx, author.Id, KindChannel, "", "replay-reactions", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, author.Id, KindChannel, author.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom author: %v", err)
	}
	if _, err := core.JoinRoom(ctx, viewer.Id, KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom viewer: %v", err)
	}

	posted, err := core.PostMessage(ctx, KindChannel, room.Id, author.Id, "react to this", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	afterSeq, err := core.GetEventSequence(ctx, KindChannel, room.Id, posted.Id)
	if err != nil {
		t.Fatalf("GetEventSequence: %v", err)
	}
	if afterSeq == 0 {
		t.Fatal("message sequence was not projected")
	}
	if added, err := core.AddReaction(ctx, KindChannel, room.Id, posted.Id, "thumbsup", author.Id); err != nil {
		t.Fatalf("AddReaction: %v", err)
	} else if !added {
		t.Fatal("AddReaction returned false, want true")
	}

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	eventChan, err := core.StreamMyEvents(subCtx, viewer.Id, afterSeq)
	if err != nil {
		t.Fatalf("StreamMyEvents: %v", err)
	}

	timeout := time.After(2 * time.Second)
	for {
		select {
		case ev, ok := <-eventChan:
			if !ok {
				t.Fatal("event stream closed unexpectedly")
			}
			evt := ev.EVTEvent()
			if evt == nil {
				continue
			}
			reaction := evt.GetReactionAdded()
			if reaction == nil {
				continue
			}
			if reaction.GetRoomId() != room.Id {
				t.Fatalf("ReactionAdded room = %q, want %q", reaction.GetRoomId(), room.Id)
			}
			if reaction.GetMessageEventId() != posted.Id {
				t.Fatalf("ReactionAdded messageEventId = %q, want %q", reaction.GetMessageEventId(), posted.Id)
			}
			if ev.DeliverySeq() <= afterSeq {
				t.Fatalf("DeliverySeq = %d, want > %d", ev.DeliverySeq(), afterSeq)
			}
			return
		case <-timeout:
			t.Fatal("viewer never received missed ReactionAddedEvent replay")
		}
	}
}

func TestStreamMyEvents_ReplayDeduplicatesLegacyAssetEvents(t *testing.T) {
	core := &ChattoCore{
		RoomTimeline: NewRoomTimelineProjection(),
		Assets:       NewAssetProjection(),
	}
	roomID := "R-legacy-assets"
	assetID := "A-legacy-video"
	created := testCoreAssetCreatedEvent(roomID, assetID, "video/mp4")
	started := testCoreAssetProcessingStartedEvent("E-legacy-started", assetID)

	for _, projection := range []interface {
		Apply(*corev1.Event, uint64) error
	}{core.RoomTimeline, core.Assets} {
		if err := projection.Apply(created, 10); err != nil {
			t.Fatalf("Apply created: %v", err)
		}
		if err := projection.Apply(started, 11); err != nil {
			t.Fatalf("Apply started: %v", err)
		}
	}

	candidates, err := core.collectMissedEventsReplay(
		map[string]struct{}{roomID: {}},
		10,
		11,
		10,
	)
	if err != nil {
		t.Fatalf("collectMissedEventsReplay: %v", err)
	}
	if len(candidates) != 1 {
		t.Fatalf("replay candidates = %d, want 1", len(candidates))
	}
	if candidates[0].event.GetId() != started.GetId() {
		t.Fatalf("replayed event id = %q, want %q", candidates[0].event.GetId(), started.GetId())
	}
}

func TestStreamMyEvents_ReplayUsesSingleGlobalCutoffOrder(t *testing.T) {
	core := &ChattoCore{
		RoomTimeline: NewRoomTimelineProjection(),
		Assets:       NewAssetProjection(),
	}
	roomID := "R-global-replay"
	assetID := "A-global-replay"

	created := testCoreAssetCreatedEvent(roomID, assetID, "video/mp4")
	if err := core.Assets.Apply(created, 10); err != nil {
		t.Fatalf("Apply asset created: %v", err)
	}
	roomEvent := &corev1.Event{
		Id: "E-room-between-tails",
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{RoomId: roomID},
		},
	}
	if err := core.RoomTimeline.Apply(roomEvent, 15); err != nil {
		t.Fatalf("Apply room event: %v", err)
	}
	assetEvent := testCoreAssetProcessingStartedEvent("E-asset-high-tail", assetID)
	if err := core.Assets.Apply(assetEvent, 20); err != nil {
		t.Fatalf("Apply asset event: %v", err)
	}

	candidates, err := core.collectMissedEventsReplay(
		map[string]struct{}{roomID: {}},
		10,
		20,
		10,
	)
	if err != nil {
		t.Fatalf("collectMissedEventsReplay: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("replay candidates = %d, want 2", len(candidates))
	}
	if candidates[0].seq != 15 || candidates[0].event.GetId() != roomEvent.GetId() {
		t.Fatalf("first candidate = seq %d id %q, want seq 15 id %q", candidates[0].seq, candidates[0].event.GetId(), roomEvent.GetId())
	}
	if candidates[1].seq != 20 || candidates[1].event.GetId() != assetEvent.GetId() {
		t.Fatalf("second candidate = seq %d id %q, want seq 20 id %q", candidates[1].seq, candidates[1].event.GetId(), assetEvent.GetId())
	}
}

func TestStreamMyEvents_DoesNotReplayRoomEventsAfterViewerLeft(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	author, err := core.CreateUser(ctx, "system", "replay-left-author", "Replay Left Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	viewer, err := core.CreateUser(ctx, "system", "replay-left-viewer", "Replay Left Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	room, err := core.CreateRoom(ctx, author.Id, KindChannel, "", "replay-left", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, author.Id, KindChannel, author.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom author: %v", err)
	}
	if _, err := core.JoinRoom(ctx, viewer.Id, KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom viewer: %v", err)
	}

	posted, err := core.PostMessage(ctx, KindChannel, room.Id, author.Id, "before leave", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage before leave: %v", err)
	}
	afterSeq, err := core.GetEventSequence(ctx, KindChannel, room.Id, posted.Id)
	if err != nil {
		t.Fatalf("GetEventSequence: %v", err)
	}
	if err := core.LeaveRoom(ctx, viewer.Id, KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}
	if _, err := core.PostMessage(ctx, KindChannel, room.Id, author.Id, "after leave", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage after leave: %v", err)
	}

	subCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	eventChan, err := core.StreamMyEvents(subCtx, viewer.Id, afterSeq)
	if err != nil {
		t.Fatalf("StreamMyEvents: %v", err)
	}

	select {
	case ev, ok := <-eventChan:
		if !ok {
			t.Fatal("event stream closed unexpectedly")
		}
		if liveEventRoomID(ev) == room.Id {
			t.Fatalf("replayed room event after viewer left: %T", ev.EVTEvent().GetEvent())
		}
	case <-time.After(150 * time.Millisecond):
	}
}

func TestStreamMyEvents_ReplayBudgetRequiresFullRefresh(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	author, err := core.CreateUser(ctx, "system", "replay-budget-author", "Replay Budget Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	viewer, err := core.CreateUser(ctx, "system", "replay-budget-viewer", "Replay Budget Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	room, err := core.CreateRoom(ctx, author.Id, KindChannel, "", "replay-budget", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, author.Id, KindChannel, author.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom author: %v", err)
	}
	if _, err := core.JoinRoom(ctx, viewer.Id, KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom viewer: %v", err)
	}

	first, err := core.PostMessage(ctx, KindChannel, room.Id, author.Id, "first", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage first: %v", err)
	}
	afterSeq, err := core.GetEventSequence(ctx, KindChannel, room.Id, first.Id)
	if err != nil {
		t.Fatalf("GetEventSequence first: %v", err)
	}
	for _, body := range []string{"second", "third"} {
		if _, err := core.PostMessage(ctx, KindChannel, room.Id, author.Id, body, nil, "", "", nil, false); err != nil {
			t.Fatalf("PostMessage %s: %v", body, err)
		}
	}

	memberRooms := map[string]struct{}{room.Id: {}}
	if _, err := core.collectMissedRoomEventsReplay(memberRooms, afterSeq, 0, 1); !errors.Is(err, ErrEventReplayTooLarge) {
		t.Fatalf("collectMissedRoomEventsReplay error = %v, want ErrEventReplayTooLarge", err)
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

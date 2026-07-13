package core

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestMyEventsHubPrefiltersMessageBodiesBeforeDecode(t *testing.T) {
	core := &ChattoCore{logger: testCoreLogger()}
	model := NewMyEventsModel(core)
	msg := &nats.Msg{
		Subject: events.LiveSubjectRoot + events.AggregateRoom + ".room-1." + events.EventMessageBody,
		Header:  nats.Header{nats.JSSequence: []string{"1"}},
		Data:    []byte("not a protobuf event"),
	}

	if discontinuity := model.hub.handleLiveEVT(context.Background(), msg); discontinuity {
		t.Fatal("private message body caused a delivery discontinuity")
	}
	if got := model.hub.decoded.Load(); got != 0 {
		t.Fatalf("decoded events = %d, want 0", got)
	}
	if got := model.hub.prefiltered.Load(); got != 1 {
		t.Fatalf("prefiltered events = %d, want 1", got)
	}
}

func TestMyEventsHubSharesDecodedEventAcrossUserSessions(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := testContext(t)

	author, err := core.CreateUser(ctx, SystemActorID, "hub-author", "Hub Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	viewer, err := core.CreateUser(ctx, SystemActorID, "hub-viewer", "Hub Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	room, err := core.CreateRoom(ctx, author.Id, KindChannel, "", "hub-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, author.Id, KindChannel, author.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom author: %v", err)
	}
	if _, err := core.JoinRoom(ctx, viewer.Id, KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom viewer: %v", err)
	}

	select {
	case <-core.myEventsModel.hub.ready:
	case <-ctx.Done():
		t.Fatal("myEvents hub did not become ready")
	}
	baselineSubscriptions := nc.NumSubscriptions()
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stream1, err := core.StreamMyEventsWithOptions(streamCtx, viewer.Id, StreamMyEventsOptions{})
	if err != nil {
		t.Fatalf("StreamMyEvents first session: %v", err)
	}
	stream2, err := core.StreamMyEventsWithOptions(streamCtx, viewer.Id, StreamMyEventsOptions{})
	if err != nil {
		t.Fatalf("StreamMyEvents second session: %v", err)
	}
	if got := nc.NumSubscriptions(); got != baselineSubscriptions {
		t.Fatalf("NATS subscriptions after opening streams = %d, want %d", got, baselineSubscriptions)
	}

	core.myEventsModel.hub.mu.Lock()
	state := core.myEventsModel.hub.users[viewer.Id]
	if state == nil || len(state.subscribers) != 2 {
		core.myEventsModel.hub.mu.Unlock()
		t.Fatalf("shared user state = %#v, want two subscribers", state)
	}
	core.myEventsModel.hub.mu.Unlock()

	posted, err := core.PostMessage(ctx, KindChannel, room.Id, author.Id, "shared decode", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	event1 := receiveEVTEventByID(t, stream1, posted.Id)
	event2 := receiveEVTEventByID(t, stream2, posted.Id)
	if event1 != event2 {
		t.Fatal("sessions received different decoded event pointers")
	}
}

func TestMyEventsHubDisconnectsOnlySubscriberOverByteLimit(t *testing.T) {
	core := &ChattoCore{logger: testCoreLogger()}
	model := NewMyEventsModel(core)
	hub := model.hub
	ch := make(chan myEventsDelivery, 1)
	done := make(chan struct{})
	sub := &myEventsSubscription{C: ch, ch: ch, Done: done, done: done, id: 1, userID: "user-1"}
	state := &myEventsUserState{
		memberRooms: map[string]struct{}{},
		subscribers: map[uint64]*myEventsSubscription{1: sub},
	}
	hub.users[sub.userID] = state
	hub.subscribers[sub.id] = sub

	hub.mu.Lock()
	hub.enqueueUserLocked(state, NewHeartbeatEventEnvelope("event-1", nil), myEventsSubscriberByteLimit+1)
	hub.mu.Unlock()

	if _, ok := <-sub.C; ok {
		t.Fatal("over-limit subscriber channel remained open")
	}
	select {
	case <-sub.Done:
	default:
		t.Fatal("over-limit subscriber did not signal termination")
	}
	if _, ok := hub.subscribers[sub.id]; ok {
		t.Fatal("over-limit subscriber remained registered")
	}
	if got := model.slowDisconnects.Load(); got != 1 {
		t.Fatalf("slow disconnects = %d, want 1", got)
	}
}

func TestPresenceHubOverflowMarksSubscriptionLagged(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	sub, err := core.PresenceHub.Subscribe(ctx)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer core.PresenceHub.Unsubscribe(sub)

	for i := 0; i < cap(sub.ch); i++ {
		sub.ch <- PresenceUpdate{UserID: "buffered-" + strconv.Itoa(i), Status: PresenceStatusOnline}
	}
	if err := core.SetPresence(ctx, "overflow-user", PresenceStatusOnline); err != nil {
		t.Fatalf("SetPresence: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for !sub.Lagged() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !sub.Lagged() {
		t.Fatal("overflowed presence subscription was not marked lagged")
	}
}

func receiveEVTEventByID(t *testing.T, stream <-chan EventEnvelope, eventID string) *corev1.Event {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case envelope, ok := <-stream:
			if !ok {
				t.Fatal("event stream closed")
			}
			if envelope.ID() == eventID && envelope.EVTEvent() != nil {
				return envelope.EVTEvent()
			}
		case <-timer.C:
			t.Fatalf("event %q was not delivered", eventID)
		}
	}
}

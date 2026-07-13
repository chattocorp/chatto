package core

import (
	"context"
	"errors"
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

func TestMyEventsHubRegistersAfterMembershipBacklog(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := core.CreateUser(ctx, SystemActorID, "barrier-author", "Barrier Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	viewer, err := core.CreateUser(ctx, SystemActorID, "barrier-viewer", "Barrier Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	room, err := core.CreateRoom(ctx, author.Id, KindChannel, "", "barrier-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, author.Id, KindChannel, author.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom author: %v", err)
	}

	hub := core.myEventsModel.hub
	hub.mu.Lock()
	_, joinErr := core.JoinRoom(ctx, viewer.Id, KindChannel, viewer.Id, room.Id)
	leaveErr := core.LeaveRoom(ctx, viewer.Id, KindChannel, viewer.Id, room.Id)
	if joinErr != nil || leaveErr != nil {
		hub.mu.Unlock()
		t.Fatalf("queue membership backlog: join=%v leave=%v", joinErr, leaveErr)
	}
	result := make(chan *myEventsSubscription, 1)
	errs := make(chan error, 1)
	go func() {
		sub, err := hub.Subscribe(ctx, viewer.Id)
		result <- sub
		errs <- err
	}()
	hub.mu.Unlock()

	var sub *myEventsSubscription
	select {
	case sub = <-result:
	case <-ctx.Done():
		t.Fatal("subscription did not cross the dispatcher registration boundary")
	}
	if err := <-errs; err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer hub.Unsubscribe(sub)

	hub.mu.Lock()
	_, staleMember := hub.users[viewer.Id].memberRooms[room.Id]
	hub.mu.Unlock()
	if staleMember {
		t.Fatal("pre-registration join backlog re-granted room visibility")
	}
}

func TestMyEventsHubVisibilityTailIgnoresOrdinaryRoomTraffic(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "tail-author", "Tail Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	room, err := core.CreateRoom(ctx, user.Id, KindChannel, "", "tail-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	hub := core.myEventsModel.hub
	before, beforeTail, err := hub.roomVisibilityTails(ctx)
	if err != nil {
		t.Fatalf("roomVisibilityTails before message: %v", err)
	}
	if _, err := core.PostMessage(ctx, KindChannel, room.Id, user.Id, "ordinary traffic", nil, "", "", nil, false); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	after, afterTail, err := hub.roomVisibilityTails(ctx)
	if err != nil {
		t.Fatalf("roomVisibilityTails after message: %v", err)
	}
	if before != after || beforeTail != afterTail {
		t.Fatalf("visibility tail changed for ordinary room traffic: before=%v/%d after=%v/%d", before, beforeTail, after, afterTail)
	}
}

func TestMyEventsHubIgnoresLateVisibilityFactsCoveredBySnapshot(t *testing.T) {
	core := &ChattoCore{logger: testCoreLogger()}
	hub := NewMyEventsModel(core).hub
	ch := make(chan myEventsDelivery, 1)
	done := make(chan struct{})
	sub := &myEventsSubscription{C: ch, ch: ch, Done: done, done: done, id: 1, userID: "viewer"}
	state := &myEventsUserState{
		memberRooms:     map[string]struct{}{},
		roomSnapshotSeq: 42,
		subscribers:     map[uint64]*myEventsSubscription{1: sub},
	}
	hub.users[sub.userID] = state
	hub.subscribers[sub.id] = sub
	join := newEvent(sub.userID, &corev1.Event{
		Event: &corev1.Event_UserJoinedRoom{UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: "room-1"}},
	})

	hub.fanoutReadyRoomEvent("room-1", join, 42, 1)
	if _, ok := state.memberRooms["room-1"]; ok {
		t.Fatal("late pre-snapshot join re-granted room visibility")
	}
	select {
	case <-sub.C:
		t.Fatal("late pre-snapshot fact was delivered")
	default:
	}

	hub.fanoutReadyRoomEvent("room-1", join, 43, 1)
	if _, ok := state.memberRooms["room-1"]; !ok {
		t.Fatal("post-snapshot join did not grant room visibility")
	}
	select {
	case <-sub.C:
	default:
		t.Fatal("post-snapshot fact was not delivered")
	}
}

func TestMyEventsHubRejectsSnapshotAcrossProcessedVisibilityChange(t *testing.T) {
	core := &ChattoCore{logger: testCoreLogger()}
	hub := NewMyEventsModel(core).hub
	hub.accepting = true
	hub.generation = 1
	hub.visibilityVersion = 2
	ch := make(chan myEventsDelivery, 1)
	done := make(chan struct{})
	sub := &myEventsSubscription{C: ch, ch: ch, Done: done, done: done, userID: "viewer"}
	request := &myEventsRegistration{
		generation:        1,
		visibilityVersion: 1,
		userID:            sub.userID,
		memberRooms:       map[string]struct{}{},
		sub:               sub,
		ctx:               context.Background(),
		result:            make(chan error, 1),
	}

	hub.handleRegistration(request)
	if err := <-request.result; !errors.Is(err, errMyEventsIngressChanged) {
		t.Fatalf("registration error = %v, want ingress changed", err)
	}
	if len(hub.subscribers) != 0 {
		t.Fatal("stale visibility snapshot was admitted")
	}
}

func TestMyEventsHubQuarantineBlocksAdmissionUntilNextGeneration(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	hub := core.myEventsModel.hub
	hub.quarantine("test discontinuity")

	ch := make(chan myEventsDelivery, 1)
	done := make(chan struct{})
	sub := &myEventsSubscription{C: ch, ch: ch, Done: done, done: done, userID: "user-1"}
	registered := make(chan error, 1)
	go func() {
		registered <- hub.registerAtIngressBoundary(ctx, sub, map[string]struct{}{}, 0, hub.visibilityVersion)
	}()

	select {
	case err := <-registered:
		t.Fatalf("registration completed during quarantine: %v", err)
	case <-time.After(50 * time.Millisecond):
	}
	hub.beginGeneration()
	select {
	case err := <-registered:
		if err != nil {
			t.Fatalf("registration after fresh generation: %v", err)
		}
	case <-ctx.Done():
		t.Fatal("registration did not resume in fresh generation")
	}
	hub.Unsubscribe(sub)
}

func TestMyEventsHubQuarantineInterruptsPendingRegistration(t *testing.T) {
	core := &ChattoCore{logger: testCoreLogger()}
	hub := NewMyEventsModel(core).hub
	hub.beginGeneration()
	ctx := testContext(t)
	ch := make(chan myEventsDelivery, 1)
	done := make(chan struct{})
	sub := &myEventsSubscription{C: ch, ch: ch, Done: done, done: done, userID: "user-1"}
	registered := make(chan error, 1)
	go func() {
		registered <- hub.registerAtIngressBoundary(ctx, sub, map[string]struct{}{}, 0, hub.visibilityVersion)
	}()

	deadline := time.Now().Add(time.Second)
	for len(hub.registrations) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(hub.registrations) == 0 {
		t.Fatal("registration did not reach dispatcher queue")
	}
	hub.quarantine("test discontinuity")
	select {
	case err := <-registered:
		if !errors.Is(err, errMyEventsIngressChanged) {
			t.Fatalf("registration error = %v, want ingress changed", err)
		}
	case <-time.After(time.Second):
		t.Fatal("quarantine did not interrupt pending registration")
	}
}

func TestMyEventsHubTerminationInterruptsBlockedForwarding(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := core.CreateUser(ctx, SystemActorID, "blocked-forwarder", "Blocked Forwarder", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	stream, err := core.StreamMyEventsWithOptions(ctx, user.Id, StreamMyEventsOptions{})
	if err != nil {
		t.Fatalf("StreamMyEvents: %v", err)
	}

	hub := core.myEventsModel.hub
	hub.mu.Lock()
	state := hub.users[user.Id]
	var sub *myEventsSubscription
	for _, candidate := range state.subscribers {
		sub = candidate
	}
	hub.enqueueUserLocked(state, NewHeartbeatEventEnvelope("blocked", nil), 1)
	hub.mu.Unlock()

	deadline := time.Now().Add(time.Second)
	for sub.queuedBytes.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if sub.queuedBytes.Load() != 0 {
		t.Fatal("stream did not begin blocked downstream forwarding")
	}
	hub.quarantine("test terminal signal")
	select {
	case _, ok := <-stream:
		if ok {
			t.Fatal("blocked stream delivered after terminal signal")
		}
	case <-time.After(time.Second):
		t.Fatal("terminal signal did not interrupt blocked forwarding")
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

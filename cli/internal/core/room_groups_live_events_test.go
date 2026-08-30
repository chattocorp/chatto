package core

import (
	"hmans.de/chatto/internal/pb/chatto/core/live/v1"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

// subscribeRoomGroupsUpdated installs a NATS Core subscriber on the live sync
// config subject that carries RoomGroupsUpdatedEvent and returns a
// channel that drains incoming messages plus a cleanup function. Tests
// fail loudly on subscribe errors — a silently-missing publish would
// look identical to a silently-missing subscriber.
func subscribeRoomGroupsUpdated(t *testing.T, nc *nats.Conn) (<-chan *nats.Msg, func()) {
	t.Helper()
	subject := subjects.LiveSyncConfigEvent("room_groups_updated")
	received := make(chan *nats.Msg, 16)
	sub, err := nc.Subscribe(subject, func(msg *nats.Msg) {
		select {
		case received <- msg:
		default:
		}
	})
	if err != nil {
		t.Fatalf("Subscribe(%s): %v", subject, err)
	}
	return received, func() { _ = sub.Unsubscribe() }
}

// drainExisting drains any messages already buffered on the channel so a
// follow-up `expectRoomGroupsUpdated` only sees events from the next
// mutation. Tests typically arrange-then-act-then-assert, but the seed
// "Lobby" group created during setupTestCore fires its own publish, and
// any prior mutator in the test will too — drain before each assert.
func drainExisting(ch <-chan *nats.Msg) {
	for {
		select {
		case <-ch:
		default:
			return
		}
	}
}

// expectRoomGroupsUpdated waits for one RoomGroupsUpdatedEvent on the
// subscribed channel and asserts its actor matches. Times out fast so a
// missing publish surfaces as a clear failure, not a hung test.
func expectRoomGroupsUpdated(t *testing.T, ch <-chan *nats.Msg, wantActorID string) {
	t.Helper()
	select {
	case msg := <-ch:
		var got livev1.LiveEvent
		if err := proto.Unmarshal(msg.Data, &got); err != nil {
			t.Fatalf("unmarshal published event: %v", err)
		}
		if got.GetRoomGroupsUpdated() == nil {
			t.Fatalf("expected RoomGroupsUpdatedEvent, got %T", got.Event)
		}
		if got.ActorId != wantActorID {
			t.Errorf("ActorId = %q, want %q", got.ActorId, wantActorID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for RoomGroupsUpdatedEvent")
	}
}

// TestRoomLayout_LiveEventOnCreateGroup pins the live-event contract for
// CreateRoomGroup. The admin UI relies on this fanout to refresh group
// lists on every connected client without a manual reload.
func TestRoomLayout_LiveEventOnCreateGroup(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := testContext(t)

	ch, cleanup := subscribeRoomGroupsUpdated(t, nc)
	defer cleanup()
	drainExisting(ch)

	if _, err := core.CreateRoomGroup(ctx, "actor", "Engineering", ""); err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	_ = nc.Flush()

	expectRoomGroupsUpdated(t, ch, "actor")
}

func TestRoomLayout_NotificationWaitsForProjectionCatchUp(t *testing.T) {
	harness := newTestEventHarness(t)
	ctx := testContext(t)

	groupLayout := NewRoomGroupLayoutProjection()
	groupLayoutProjector := harness.projector(groupLayout)
	core := &ChattoCore{
		nc:             harness.nc,
		logger:         testCoreLogger(),
		EventPublisher: harness.publisher,
	}
	core.roomModel = newTestRoomModel(t, nil, nil, groupLayout, groupLayoutProjector, nil, nil, nil, nil, nil, nil)

	projectedGroups := make(chan []string, 1)
	subject := subjects.LiveSyncConfigEvent("room_groups_updated")
	sub, err := harness.nc.Subscribe(subject, func(_ *nats.Msg) {
		groups := core.roomModel.roomGroups()
		ids := make([]string, 0, len(groups))
		for _, group := range groups {
			ids = append(ids, group.GetId())
		}
		projectedGroups <- ids
	})
	if err != nil {
		t.Fatalf("Subscribe(%s): %v", subject, err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	if err := harness.nc.Flush(); err != nil {
		t.Fatalf("flush subscription: %v", err)
	}

	created := newEvent("actor", groupCreatedEvent("G-engineering", "Engineering", ""))
	createdSubject := evtstream.GroupAggregate("G-engineering").SubjectFor(created)
	if _, err := harness.publisher.AppendEventually(ctx, createdSubject, created); err != nil {
		t.Fatalf("append group mutation: %v", err)
	}

	notificationFinished := make(chan struct{})
	go func() {
		core.notifyRoomLayoutChanged(ctx, "actor", "test")
		close(notificationFinished)
	}()
	if err := harness.nc.Flush(); err != nil {
		t.Fatalf("flush while projection is stopped: %v", err)
	}

	select {
	case groups := <-projectedGroups:
		t.Fatalf("received room-layout invalidation before projection catch-up (groups %v)", groups)
	case <-time.After(25 * time.Millisecond):
	}
	select {
	case <-notificationFinished:
		t.Fatal("room-layout notification finished before projection catch-up")
	default:
	}

	startTestProjector(t, groupLayoutProjector)
	select {
	case <-notificationFinished:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for notification after projection catch-up")
	}
	if err := harness.nc.Flush(); err != nil {
		t.Fatalf("flush notification: %v", err)
	}

	select {
	case groups := <-projectedGroups:
		if !equalStrings(groups, []string{"G-engineering"}) {
			t.Errorf("projected groups when notification arrived = %v, want [G-engineering]", groups)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for RoomGroupsUpdatedEvent")
	}
}

func TestRoomLayout_LiveEventOnUpdateGroup(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := testContext(t)

	group, err := core.CreateRoomGroup(ctx, "actor", "Engineering", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}

	ch, cleanup := subscribeRoomGroupsUpdated(t, nc)
	defer cleanup()
	drainExisting(ch)

	if _, err := core.UpdateRoomGroup(ctx, "actor", group.Id, "Eng", "the eng team"); err != nil {
		t.Fatalf("UpdateRoomGroup: %v", err)
	}
	_ = nc.Flush()

	expectRoomGroupsUpdated(t, ch, "actor")
}

func TestRoomLayout_LiveEventOnDeleteGroup(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := testContext(t)

	group, err := core.CreateRoomGroup(ctx, "actor", "Engineering", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}

	ch, cleanup := subscribeRoomGroupsUpdated(t, nc)
	defer cleanup()
	drainExisting(ch)

	if err := core.DeleteRoomGroup(ctx, "actor", group.Id); err != nil {
		t.Fatalf("DeleteRoomGroup: %v", err)
	}
	_ = nc.Flush()

	expectRoomGroupsUpdated(t, ch, "actor")
}

func TestRoomLayout_LiveEventOnReorderGroups(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := testContext(t)

	a, _ := core.CreateRoomGroup(ctx, "actor", "A", "")
	b, _ := core.CreateRoomGroup(ctx, "actor", "B", "")

	groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered: %v", err)
	}
	ordered := make([]string, len(groups))
	// Reverse the order so the reorder is observably non-identity.
	for i, g := range groups {
		ordered[len(groups)-1-i] = g.Id
	}

	ch, cleanup := subscribeRoomGroupsUpdated(t, nc)
	defer cleanup()
	drainExisting(ch)

	if err := core.ReorderRoomGroups(ctx, "actor", ordered); err != nil {
		t.Fatalf("ReorderRoomGroups: %v", err)
	}
	_ = nc.Flush()

	expectRoomGroupsUpdated(t, ch, "actor")
	_ = a
	_ = b
}

// TestRoomLayout_LiveEventOnMoveRoomToGroup covers the cross-group room
// move — the operation that exposed the original bug, since the admin
// drag-and-drop emits this mutation and the sidebar needs to reflect it
// on every other client.
func TestRoomLayout_LiveEventOnMoveRoomToGroup(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := testContext(t)

	target, err := core.CreateRoomGroup(ctx, "actor", "Target", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	room, err := core.CreateRoom(ctx, "actor", KindChannel, "", "mover", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	ch, cleanup := subscribeRoomGroupsUpdated(t, nc)
	defer cleanup()
	drainExisting(ch)

	if err := core.MoveRoomToGroup(ctx, "actor", room.Id, target.Id); err != nil {
		t.Fatalf("MoveRoomToGroup: %v", err)
	}
	_ = nc.Flush()

	expectRoomGroupsUpdated(t, ch, "actor")
}

func TestRoomLayout_MoveNotificationWaitsForProjectionCatchUp(t *testing.T) {
	harness := newTestEventHarness(t)
	ctx := testContext(t)

	groupLayout := NewRoomGroupLayoutProjection()
	groupLayoutProjector := harness.projector(groupLayout)
	core := &ChattoCore{
		nc:             harness.nc,
		logger:         testCoreLogger(),
		EventPublisher: harness.publisher,
	}
	core.roomModel = newTestRoomModel(t, nil, nil, groupLayout, groupLayoutProjector, nil, nil, nil, nil, nil, nil)

	setupEvents := []*evtv1.Event{
		newEvent("actor", groupCreatedEvent("G-source", "Source", "")),
		newEvent("actor", groupCreatedEvent("G-target", "Target", "")),
		newEvent("actor", roomAddedToGroupEvent("G-source", "R1")),
	}
	for i, event := range setupEvents {
		subject := evtstream.GroupAggregate(groupIDOfTestGroupEvent(t, event)).SubjectFor(event)
		seq, err := harness.publisher.AppendEventually(ctx, subject, event)
		if err != nil {
			t.Fatalf("append setup event %d: %v", i, err)
		}
		if err := groupLayout.Apply(event, seq); err != nil {
			t.Fatalf("project setup event %d: %v", i, err)
		}
	}

	type notificationObservation struct {
		msg     *nats.Msg
		groupID string
	}
	observations := make(chan notificationObservation, 4)
	subject := subjects.LiveSyncConfigEvent("room_groups_updated")
	sub, err := harness.nc.Subscribe(subject, func(msg *nats.Msg) {
		observations <- notificationObservation{
			msg:     msg,
			groupID: core.roomModel.roomGroupForRoom("R1"),
		}
	})
	if err != nil {
		t.Fatalf("Subscribe(%s): %v", subject, err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	if err := harness.nc.Flush(); err != nil {
		t.Fatalf("flush subscription: %v", err)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- core.MoveRoomToGroup(ctx, "actor", "R1", "G-target")
	}()

	addedSubject := evtstream.GroupAggregate("G-target").Subject(evtstream.EventRoomAddedToGroup)
	deadline := time.Now().Add(2 * time.Second)
	for {
		addedEvents, _, err := core.EventPublisher.SubjectEvents(ctx, addedSubject)
		if err != nil {
			t.Fatalf("read target room-added events: %v", err)
		}
		if len(addedEvents) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for move batch to commit")
		}
		time.Sleep(time.Millisecond)
	}
	if err := harness.nc.Flush(); err != nil {
		t.Fatalf("flush while projection is stopped: %v", err)
	}

	select {
	case observation := <-observations:
		t.Fatalf("received room-layout invalidation before projection catch-up (projected group %q)", observation.groupID)
	case <-time.After(25 * time.Millisecond):
	}
	select {
	case err := <-errCh:
		t.Fatalf("MoveRoomToGroup returned before projection catch-up: %v", err)
	default:
	}
	if got := core.roomModel.roomGroupForRoom("R1"); got != "G-source" {
		t.Fatalf("projected room group before catch-up = %q, want G-source", got)
	}

	startTestProjector(t, groupLayoutProjector)
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("MoveRoomToGroup after projection catch-up: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for MoveRoomToGroup after projection catch-up")
	}
	if err := harness.nc.Flush(); err != nil {
		t.Fatalf("flush notification: %v", err)
	}

	select {
	case observation := <-observations:
		var event livev1.LiveEvent
		if err := proto.Unmarshal(observation.msg.Data, &event); err != nil {
			t.Fatalf("unmarshal published event: %v", err)
		}
		if event.GetRoomGroupsUpdated() == nil {
			t.Fatalf("expected RoomGroupsUpdatedEvent, got %T", event.Event)
		}
		if event.GetActorId() != "actor" {
			t.Errorf("ActorId = %q, want actor", event.GetActorId())
		}
		if observation.groupID != "G-target" {
			t.Errorf("projected room group when notification arrived = %q, want G-target", observation.groupID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for RoomGroupsUpdatedEvent")
	}

	select {
	case <-observations:
		t.Fatal("received more than one room-layout invalidation")
	case <-time.After(25 * time.Millisecond):
	}
}

func TestRoomLayout_LiveEventOnCreateRoom(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := testContext(t)

	ch, cleanup := subscribeRoomGroupsUpdated(t, nc)
	defer cleanup()
	drainExisting(ch)

	if _, err := core.CreateRoom(ctx, "actor", KindChannel, "", "general-2", ""); err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	_ = nc.Flush()

	// CreateRoom on a channel routes through MoveRoomToGroup, which also
	// publishes. We expect at least one event with the right shape; the
	// helper consumes one and that's enough proof that the room layout
	// fanout fired.
	expectRoomGroupsUpdated(t, ch, "actor")
}

// TestRoomLayout_LiveEventOnReorderRoomsInGroup covers the intra-group
// reorder mutation — the drag-and-drop path inside a single group, where
// the membership set doesn't change but the room order does. The
// frontend needs the live event to refresh the sidebar order on every
// other client.
func TestRoomLayout_LiveEventOnReorderRoomsInGroup(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := testContext(t)

	group, err := core.CreateRoomGroup(ctx, "actor", "Group", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	r1, _ := core.CreateRoom(ctx, "actor", KindChannel, group.Id, "alpha", "")
	r2, _ := core.CreateRoom(ctx, "actor", KindChannel, group.Id, "beta", "")
	r3, _ := core.CreateRoom(ctx, "actor", KindChannel, group.Id, "gamma", "")

	ch, cleanup := subscribeRoomGroupsUpdated(t, nc)
	defer cleanup()
	drainExisting(ch)

	// Reverse the order so the change is observably non-identity.
	if err := core.ReorderRoomsInGroup(ctx, "actor", group.Id, []string{r3.Id, r2.Id, r1.Id}); err != nil {
		t.Fatalf("ReorderRoomsInGroup: %v", err)
	}
	_ = nc.Flush()

	expectRoomGroupsUpdated(t, ch, "actor")

	// Verify the new order actually stuck.
	got, err := core.GetRoomGroup(ctx, group.Id)
	if err != nil {
		t.Fatalf("GetRoomGroup: %v", err)
	}
	want := []string{r3.Id, r2.Id, r1.Id}
	if !equalStrings(got.RoomIds, want) {
		t.Errorf("post-reorder RoomIds = %v, want %v", got.RoomIds, want)
	}
}

// TestReorderRoomsInGroup_RejectsSetMismatch verifies the validation
// guard: passing a list that adds or omits a room ID is rejected before
// any write lands.
func TestReorderRoomsInGroup_RejectsSetMismatch(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	group, _ := core.CreateRoomGroup(ctx, "actor", "G", "")
	a, _ := core.CreateRoom(ctx, "actor", KindChannel, group.Id, "a", "")
	b, _ := core.CreateRoom(ctx, "actor", KindChannel, group.Id, "b", "")

	if err := core.ReorderRoomsInGroup(ctx, "actor", group.Id, []string{a.Id}); err == nil {
		t.Error("expected ErrRoomGroupOrderMismatch for missing room, got nil")
	}
	if err := core.ReorderRoomsInGroup(ctx, "actor", group.Id, []string{a.Id, b.Id, "RbogusXX"}); err == nil {
		t.Error("expected ErrRoomGroupOrderMismatch for extra room, got nil")
	}
	if err := core.ReorderRoomsInGroup(ctx, "actor", group.Id, []string{a.Id, a.Id}); err == nil {
		t.Error("expected ErrRoomGroupOrderMismatch for duplicate room, got nil")
	}

	// On rejection, the group's room order should still match the original.
	got, _ := core.GetRoomGroup(ctx, group.Id)
	if !equalStrings(got.RoomIds, []string{a.Id, b.Id}) {
		t.Errorf("room order changed after rejected reorder: %v", got.RoomIds)
	}
}

func TestRoomLayout_LiveEventOnDeleteRoom(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, "actor", KindChannel, "", "doomed", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}

	ch, cleanup := subscribeRoomGroupsUpdated(t, nc)
	defer cleanup()
	drainExisting(ch)

	if err := core.DeleteRoom(ctx, "actor", KindChannel, room.Id); err != nil {
		t.Fatalf("DeleteRoom: %v", err)
	}
	_ = nc.Flush()

	expectRoomGroupsUpdated(t, ch, "actor")
}

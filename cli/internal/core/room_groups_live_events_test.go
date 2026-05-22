package core

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// subscribeRoomGroupsUpdated installs a NATS Core subscriber on the live
// config subject that carries RoomGroupsUpdatedEvent and returns a
// channel that drains incoming messages plus a cleanup function. Tests
// fail loudly on subscribe errors — a silently-missing publish would
// look identical to a silently-missing subscriber.
func subscribeRoomGroupsUpdated(t *testing.T, nc *nats.Conn) (<-chan *nats.Msg, func()) {
	t.Helper()
	subject := subjects.LiveConfigEvent("room_groups_updated")
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
		var got corev1.Event
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

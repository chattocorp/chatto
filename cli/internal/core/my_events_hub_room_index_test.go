package core

import (
	"testing"
	"time"
)

// assertMyEventsRoomIndex fails when hub.roomMembers does not equal the index
// derived from hub.users and their memberRooms.
func assertMyEventsRoomIndex(t *testing.T, hub *MyEventsHub) {
	t.Helper()
	hub.mu.Lock()
	defer hub.mu.Unlock()
	want := make(map[string]map[myEventsPrincipal]struct{})
	for principal, state := range hub.users {
		for roomID := range state.memberRooms {
			if want[roomID] == nil {
				want[roomID] = make(map[myEventsPrincipal]struct{})
			}
			want[roomID][principal] = struct{}{}
		}
	}
	if len(hub.roomMembers) != len(want) {
		t.Fatalf("room index has %d rooms, want %d", len(hub.roomMembers), len(want))
	}
	for roomID, members := range want {
		got := hub.roomMembers[roomID]
		if len(got) != len(members) {
			t.Fatalf("room index for %s has %d principals, want %d", roomID, len(got), len(members))
		}
		for principal := range members {
			if _, ok := got[principal]; !ok {
				t.Fatalf("room index for %s is missing principal %s", roomID, principal.userID)
			}
		}
	}
}

// receiveDeliveryUntil drains sub until it receives untilID. It fails if
// forbiddenID arrives first or if untilID does not arrive in time.
func receiveDeliveryUntil(t *testing.T, sub *myEventsSubscription, untilID, forbiddenID string) {
	t.Helper()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case delivery, ok := <-sub.C:
			if !ok {
				t.Fatal("subscription closed")
			}
			switch delivery.event.ID() {
			case forbiddenID:
				t.Fatalf("event %q was delivered", forbiddenID)
			case untilID:
				return
			}
		case <-timer.C:
			t.Fatalf("event %q was not delivered", untilID)
		}
	}
}

// A principal that joins a room after its stream registered must enter the
// room index, receive room-member facts, and leave the index again when it
// leaves the room.
func TestMyEventsHubRoomIndexFollowsMembershipAfterRegistration(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	author, err := core.CreateUser(ctx, SystemActorID, "index-author", "Index Author", "password123")
	if err != nil {
		t.Fatalf("CreateUser author: %v", err)
	}
	viewer, err := core.CreateUser(ctx, SystemActorID, "index-viewer", "Index Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser viewer: %v", err)
	}
	room, err := core.CreateRoom(ctx, author.Id, KindChannel, "", "index-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	fenceRoom, err := core.CreateRoom(ctx, author.Id, KindChannel, "", "index-fence", "")
	if err != nil {
		t.Fatalf("CreateRoom fence: %v", err)
	}
	for _, roomID := range []string{room.Id, fenceRoom.Id} {
		if _, err := core.JoinRoom(ctx, author.Id, KindChannel, author.Id, roomID); err != nil {
			t.Fatalf("JoinRoom author: %v", err)
		}
	}
	if _, err := core.JoinRoom(ctx, viewer.Id, KindChannel, viewer.Id, fenceRoom.Id); err != nil {
		t.Fatalf("JoinRoom viewer fence: %v", err)
	}

	hub := core.myEventsModel.hub
	sub, err := hub.Subscribe(ctx, viewer.Id)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer hub.Unsubscribe(sub)
	assertMyEventsRoomIndex(t, hub)

	// post publishes a message in roomID and returns its event ID.
	post := func(roomID, body string) string {
		t.Helper()
		event, err := core.PostMessage(ctx, KindChannel, roomID, author.Id, body, nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("PostMessage: %v", err)
		}
		return event.Id
	}

	// Before the join, a message in the room must not reach the viewer. The
	// fence message is in a later stream position, so the hub processed the
	// room message before the fence arrives.
	before := post(room.Id, "before join")
	receiveDeliveryUntil(t, sub, post(fenceRoom.Id, "fence 1"), before)

	if _, err := core.JoinRoom(ctx, viewer.Id, KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom viewer: %v", err)
	}
	receiveDeliveryUntil(t, sub, post(room.Id, "after join"), "")
	assertMyEventsRoomIndex(t, hub)
	hub.mu.Lock()
	_, indexed := hub.roomMembers[room.Id][sub.principal]
	hub.mu.Unlock()
	if !indexed {
		t.Fatal("joined viewer is not in the room index")
	}

	if err := core.LeaveRoom(ctx, viewer.Id, KindChannel, viewer.Id, room.Id); err != nil {
		t.Fatalf("LeaveRoom viewer: %v", err)
	}
	after := post(room.Id, "after leave")
	receiveDeliveryUntil(t, sub, post(fenceRoom.Id, "fence 2"), after)
	assertMyEventsRoomIndex(t, hub)
}

// The index must follow every change to the user set and to memberRooms:
// registration, a refresh, removal of the last session, and quarantine.
func TestMyEventsHubRoomIndexTracksUserLifecycle(t *testing.T) {
	core := &ChattoCore{logger: testCoreLogger()}
	hub := NewMyEventsModel(core).hub
	register := func(userID string, id uint64, rooms ...string) *myEventsSubscription {
		sub := newMyEventsSubscription(entitledMyEventsPrincipal(userID))
		sub.id = id
		memberRooms := make(map[string]struct{})
		for _, roomID := range rooms {
			memberRooms[roomID] = struct{}{}
		}
		hub.mu.Lock()
		hub.subscribers[sub.id] = sub
		hub.addUserLocked(sub.principal, &myEventsUserState{
			memberRooms: memberRooms,
			subscribers: map[uint64]*myEventsSubscription{sub.id: sub},
		})
		hub.mu.Unlock()
		return sub
	}

	first := register("user-1", 1, "room-a", "room-b")
	register("user-2", 2, "room-b")
	assertMyEventsRoomIndex(t, hub)

	hub.mu.Lock()
	hub.setMemberRoomsLocked(first.principal, hub.users[first.principal], map[string]struct{}{"room-c": {}})
	hub.mu.Unlock()
	assertMyEventsRoomIndex(t, hub)
	hub.mu.Lock()
	_, staleRoom := hub.roomMembers["room-a"]
	hub.mu.Unlock()
	if staleRoom {
		t.Fatal("refresh left a room without members in the index")
	}

	hub.Unsubscribe(first)
	assertMyEventsRoomIndex(t, hub)

	hub.quarantine("test")
	assertMyEventsRoomIndex(t, hub)
	if len(hub.roomMembers) != 0 {
		t.Fatalf("quarantine left %d rooms in the index", len(hub.roomMembers))
	}
}

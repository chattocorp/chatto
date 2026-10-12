package core

import (
	"errors"
	"testing"
)

// The runtime supplies live projections, but the command has no core callbacks.
// This catches accidental delegation of lifecycle persistence back to core.
func TestRoomCommandModelLifecycleWithoutCoreCallbacks(t *testing.T) {
	t.Parallel()
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	commands := &RoomCommandModel{rooms: core.roomModel, publisher: core.EventPublisher, rbac: core.rbacModel, logger: testCoreLogger()}
	room, err := commands.createRoom(ctx, SystemActorID, KindChannel, "", "isolated-lifecycle", "", WithAnnouncementsRoomDefaults())
	if err != nil {
		t.Fatal(err)
	}
	if room.GroupId == "" {
		t.Fatal("expected default group")
	}
	group, err := commands.rooms.GetRoomGroup(ctx, room.GroupId)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, id := range group.RoomIds {
		if id == room.Id {
			found = true
		}
	}
	if !found {
		t.Fatal("created room missing from group on return")
	}
	if got := commands.rbac.decision(ScopeRoom, room.Id, RoleEveryone, PermMessagePost); got != DecisionDeny {
		t.Fatalf("default permission = %v", got)
	}
	if _, err := commands.createRoom(ctx, SystemActorID, KindChannel, "", room.Name, ""); !errors.Is(err, ErrRoomNameExists) {
		t.Fatalf("duplicate name: %v", err)
	}
	for _, archived := range []bool{true, false} {
		if archived {
			_, err = commands.archiveRoom(ctx, SystemActorID, KindChannel, room.Id)
		} else {
			_, err = commands.unarchiveRoom(ctx, SystemActorID, KindChannel, room.Id)
		}
		if err != nil {
			t.Fatal(err)
		}
		got, err := commands.rooms.GetRoom(ctx, KindChannel, room.Id)
		if err != nil {
			t.Fatal(err)
		}
		if got.Archived != archived || got.GroupId != room.GroupId {
			t.Fatalf("archive state/group changed incorrectly: archived=%v group=%s", got.Archived, got.GroupId)
		}
	}
	if _, err := commands.archiveRoom(ctx, SystemActorID, KindDM, room.Id); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong kind: %v", err)
	}
}

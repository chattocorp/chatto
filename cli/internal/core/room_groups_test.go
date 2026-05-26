package core

import (
	"errors"
	"sync"
	"testing"
)

func TestCreateRoomGroup(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	set, err := core.CreateRoomGroup(ctx, "actor", "Engineering", "Eng team rooms")
	if err != nil {
		t.Fatalf("CreateRoomGroup failed: %v", err)
	}
	if set.Name != "Engineering" {
		t.Errorf("Name = %q, want %q", set.Name, "Engineering")
	}
	if set.Description != "Eng team rooms" {
		t.Errorf("Description = %q, want %q", set.Description, "Eng team rooms")
	}
	if set.Id == "" {
		t.Error("Expected an ID to be assigned")
	}

	// Verify persisted. The seed "Lobby" group is created at boot, so the
	// reconciled list contains it plus the just-created Engineering group.
	groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered failed: %v", err)
	}
	found := false
	for _, g := range groups {
		if g.Id == set.Id {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("New group not present in reconciled list: %+v", groups)
	}
}

func TestCreateRoomGroup_TrimsName(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	set, err := core.CreateRoomGroup(ctx, "actor", "  General  ", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup failed: %v", err)
	}
	if set.Name != "General" {
		t.Errorf("Name = %q, want trimmed %q", set.Name, "General")
	}
}

func TestCreateRoomGroup_EmptyNameRejected(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	for _, name := range []string{"", "   ", "\t"} {
		_, err := core.CreateRoomGroup(ctx, "actor", name, "")
		if !errors.Is(err, ErrRoomGroupNameEmpty) {
			t.Errorf("CreateRoomGroup(%q) error = %v, want ErrRoomGroupNameEmpty", name, err)
		}
	}
}

func TestUpdateRoomGroup(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	set, _ := core.CreateRoomGroup(ctx, "actor", "Old Name", "old desc")
	updated, err := core.UpdateRoomGroup(ctx, "actor", set.Id, "New Name", "new desc")
	if err != nil {
		t.Fatalf("UpdateRoomGroup failed: %v", err)
	}
	if updated.Name != "New Name" || updated.Description != "new desc" {
		t.Errorf("Update mismatch: %+v", updated)
	}
	if updated.Id != set.Id {
		t.Errorf("ID changed: %q → %q", set.Id, updated.Id)
	}
}

func TestUpdateRoomGroup_NotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, err := core.UpdateRoomGroup(ctx, "actor", "nonexistent", "x", "")
	if !errors.Is(err, ErrRoomGroupNotFound) {
		t.Errorf("UpdateRoomGroup on missing set: err = %v, want ErrRoomGroupNotFound", err)
	}
}

func TestGetRoomGroup(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	created, _ := core.CreateRoomGroup(ctx, "actor", "Engineering", "")
	got, err := core.GetRoomGroup(ctx, created.Id)
	if err != nil {
		t.Fatalf("GetRoomGroup failed: %v", err)
	}
	if got.Id != created.Id || got.Name != "Engineering" {
		t.Errorf("GetRoomGroup mismatch: got %+v, want id=%q name=%q", got, created.Id, "Engineering")
	}
}

func TestGetRoomGroup_NotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, err := core.GetRoomGroup(ctx, "nonexistent")
	if !errors.Is(err, ErrRoomGroupNotFound) {
		t.Errorf("GetRoomGroup on missing set: err = %v, want ErrRoomGroupNotFound", err)
	}
}

func TestDeleteRoomGroup_Empty(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	set, _ := core.CreateRoomGroup(ctx, "actor", "Empty", "")
	if err := core.DeleteRoomGroup(ctx, "actor", set.Id); err != nil {
		t.Fatalf("DeleteRoomGroup failed: %v", err)
	}

	_, err := core.GetRoomGroup(ctx, set.Id)
	if !errors.Is(err, ErrRoomGroupNotFound) {
		t.Errorf("Set still exists after deletion: err = %v", err)
	}
}

func TestDeleteRoomGroup_RejectsNonEmpty(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	set, _ := core.CreateRoomGroup(ctx, "actor", "WithRooms", "")
	room, _ := core.CreateRoom(ctx, "actor", KindChannel, "", "general", "")
	if err := core.MoveRoomToGroup(ctx, "actor", room.Id, set.Id); err != nil {
		t.Fatalf("MoveRoomToGroup failed: %v", err)
	}

	err := core.DeleteRoomGroup(ctx, "actor", set.Id)
	if !errors.Is(err, ErrRoomGroupHasRooms) {
		t.Errorf("DeleteRoomGroup on populated set: err = %v, want ErrRoomGroupHasRooms", err)
	}
}

func TestMoveRoomToSet(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	setA, _ := core.CreateRoomGroup(ctx, "actor", "A", "")
	setB, _ := core.CreateRoomGroup(ctx, "actor", "B", "")
	room, _ := core.CreateRoom(ctx, "actor", KindChannel, "", "general", "")

	if err := core.MoveRoomToGroup(ctx, "actor", room.Id, setA.Id); err != nil {
		t.Fatalf("MoveRoomToGroup A failed: %v", err)
	}

	gotA, _ := core.GetRoomGroup(ctx, setA.Id)
	if len(gotA.RoomIds) != 1 || gotA.RoomIds[0] != room.Id {
		t.Errorf("Set A should contain the room: %+v", gotA.RoomIds)
	}

	// Move to set B; room should leave A
	if err := core.MoveRoomToGroup(ctx, "actor", room.Id, setB.Id); err != nil {
		t.Fatalf("MoveRoomToGroup B failed: %v", err)
	}

	gotA, _ = core.GetRoomGroup(ctx, setA.Id)
	gotB, _ := core.GetRoomGroup(ctx, setB.Id)
	if len(gotA.RoomIds) != 0 {
		t.Errorf("Set A should be empty after move: %+v", gotA.RoomIds)
	}
	if len(gotB.RoomIds) != 1 || gotB.RoomIds[0] != room.Id {
		t.Errorf("Set B should contain the room: %+v", gotB.RoomIds)
	}
}

func TestMoveRoomToSet_TargetNotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "actor", KindChannel, "", "general", "")
	err := core.MoveRoomToGroup(ctx, "actor", room.Id, "nonexistent")
	if !errors.Is(err, ErrRoomGroupNotFound) {
		t.Errorf("err = %v, want ErrRoomGroupNotFound", err)
	}
}

func TestMoveRoomToSet_Idempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	set, _ := core.CreateRoomGroup(ctx, "actor", "S", "")
	room, _ := core.CreateRoom(ctx, "actor", KindChannel, "", "general", "")

	if err := core.MoveRoomToGroup(ctx, "actor", room.Id, set.Id); err != nil {
		t.Fatalf("first move failed: %v", err)
	}
	if err := core.MoveRoomToGroup(ctx, "actor", room.Id, set.Id); err != nil {
		t.Fatalf("second move (idempotent) failed: %v", err)
	}

	got, _ := core.GetRoomGroup(ctx, set.Id)
	if len(got.RoomIds) != 1 {
		t.Errorf("Room appears %d times in set, want exactly 1", len(got.RoomIds))
	}
}

func TestMoveRoomToSet_ConcurrentMovesLeaveSingleAssignment(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	setA, _ := core.CreateRoomGroup(ctx, "actor", "A", "")
	setB, _ := core.CreateRoomGroup(ctx, "actor", "B", "")
	room, _ := core.CreateRoom(ctx, "actor", KindChannel, "", "general", "")

	for i := 0; i < 25; i++ {
		if err := core.MoveRoomToGroup(ctx, "actor", room.Id, setA.Id); err != nil {
			t.Fatalf("reset move to A failed: %v", err)
		}

		start := make(chan struct{})
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for _, target := range []string{setB.Id, setA.Id} {
			wg.Add(1)
			go func(target string) {
				defer wg.Done()
				<-start
				errs <- core.MoveRoomToGroup(ctx, "actor", room.Id, target)
			}(target)
		}
		close(start)
		wg.Wait()
		close(errs)
		for err := range errs {
			if err != nil {
				t.Fatalf("concurrent MoveRoomToGroup failed: %v", err)
			}
		}

		groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
		if err != nil {
			t.Fatalf("ListRoomGroupsOrdered: %v", err)
		}
		assignments := 0
		for _, group := range groups {
			for _, roomID := range group.RoomIds {
				if roomID == room.Id {
					assignments++
				}
			}
		}
		if assignments != 1 {
			t.Fatalf("iteration %d: room has %d group assignments, want exactly 1", i, assignments)
		}
	}
}

func TestReorderRoomGroups(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// The boot seed creates one "Lobby" group; capture it so we can include
	// it in the reorder list (ReorderRoomGroups requires a permutation of
	// every existing group).
	seedGroups, _ := core.ListRoomGroupsOrdered(ctx, KindChannel)
	seedID := seedGroups[0].Id

	a, _ := core.CreateRoomGroup(ctx, "actor", "A", "")
	b, _ := core.CreateRoomGroup(ctx, "actor", "B", "")
	c, _ := core.CreateRoomGroup(ctx, "actor", "C", "")

	if err := core.ReorderRoomGroups(ctx, "actor", []string{c.Id, a.Id, b.Id, seedID}); err != nil {
		t.Fatalf("ReorderRoomGroups failed: %v", err)
	}

	gs, _ := core.ListRoomGroupsOrdered(ctx, KindChannel)
	got := []string{gs[0].Id, gs[1].Id, gs[2].Id, gs[3].Id}
	want := []string{c.Id, a.Id, b.Id, seedID}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("position %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReorderRoomGroups_RejectsIncompleteList(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	a, _ := core.CreateRoomGroup(ctx, "actor", "A", "")
	_, _ = core.CreateRoomGroup(ctx, "actor", "B", "")

	// Missing the seed group + one of the created groups.
	err := core.ReorderRoomGroups(ctx, "actor", []string{a.Id})
	if !errors.Is(err, ErrRoomGroupOrderMismatch) {
		t.Errorf("err = %v, want ErrRoomGroupOrderMismatch", err)
	}
}

func TestSeedSetIncludesPreExistingRooms(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Rooms created at boot or via the test helpers (e.g. before #454)
	// land in the seed "Lobby" group so the layout invariant ("every channel
	// room belongs to exactly one set") is preserved.
	room, err := core.CreateRoom(ctx, "actor", KindChannel, "", "general", "")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	// The boot-time hook already ran in setupTestCore; CreateRoom with
	// groupID="" also lands the room in the seed set if there is one.
	// Re-run the migration hook to verify idempotence + that an
	// orphaned room would get adopted.
	if err := core.ensureChannelRoomsAreInAGroup(ctx); err != nil {
		t.Fatalf("ensureChannelRoomsAreInAGroup failed: %v", err)
	}

	groups, _ := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if len(groups) == 0 {
		t.Fatal("Expected seed group to exist")
	}

	// The room should be in exactly one group, with its proto GroupId stamped.
	count := 0
	var assignedGroupID string
	for _, g := range groups {
		for _, rid := range g.RoomIds {
			if rid == room.Id {
				count++
				assignedGroupID = g.Id
			}
		}
	}
	if count != 1 {
		t.Errorf("Room appears in %d groups, want exactly 1", count)
	}

	refreshed, _ := core.GetRoom(ctx, KindChannel, room.Id)
	if refreshed.GroupId != assignedGroupID {
		t.Errorf("Room.GroupId = %q, want %q (the group it appears in)", refreshed.GroupId, assignedGroupID)
	}
}

func TestReorderRoomGroups_RejectsUnknownID(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	seedGroups, _ := core.ListRoomGroupsOrdered(ctx, KindChannel)
	seedID := seedGroups[0].Id
	a, _ := core.CreateRoomGroup(ctx, "actor", "A", "")
	err := core.ReorderRoomGroups(ctx, "actor", []string{seedID, a.Id, "unknown"})
	if !errors.Is(err, ErrRoomGroupOrderMismatch) {
		t.Errorf("err = %v, want ErrRoomGroupOrderMismatch", err)
	}
}

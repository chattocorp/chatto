package core

import (
	"errors"
	"testing"
)

func TestCreateRoomSet(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	set, err := core.CreateRoomSet(ctx, "actor", "Engineering", "Eng team rooms")
	if err != nil {
		t.Fatalf("CreateRoomSet failed: %v", err)
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

	// Verify persisted. The seed "Rooms" set is created at boot, so the
	// layout contains it plus the just-created Engineering set.
	layout, err := core.GetRoomLayout(ctx, KindChannel)
	if err != nil {
		t.Fatalf("GetRoomLayout failed: %v", err)
	}
	if layout == nil {
		t.Fatal("Expected layout to exist")
	}
	if findSetIndex(layout, set.Id) == -1 {
		t.Errorf("New set not present in layout: %+v", layout.Sets)
	}
}

func TestCreateRoomSet_TrimsName(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	set, err := core.CreateRoomSet(ctx, "actor", "  General  ", "")
	if err != nil {
		t.Fatalf("CreateRoomSet failed: %v", err)
	}
	if set.Name != "General" {
		t.Errorf("Name = %q, want trimmed %q", set.Name, "General")
	}
}

func TestCreateRoomSet_EmptyNameRejected(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	for _, name := range []string{"", "   ", "\t"} {
		_, err := core.CreateRoomSet(ctx, "actor", name, "")
		if !errors.Is(err, ErrRoomSetNameEmpty) {
			t.Errorf("CreateRoomSet(%q) error = %v, want ErrRoomSetNameEmpty", name, err)
		}
	}
}

func TestUpdateRoomSet(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	set, _ := core.CreateRoomSet(ctx, "actor", "Old Name", "old desc")
	updated, err := core.UpdateRoomSet(ctx, "actor", set.Id, "New Name", "new desc")
	if err != nil {
		t.Fatalf("UpdateRoomSet failed: %v", err)
	}
	if updated.Name != "New Name" || updated.Description != "new desc" {
		t.Errorf("Update mismatch: %+v", updated)
	}
	if updated.Id != set.Id {
		t.Errorf("ID changed: %q → %q", set.Id, updated.Id)
	}
}

func TestUpdateRoomSet_NotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, err := core.UpdateRoomSet(ctx, "actor", "nonexistent", "x", "")
	if !errors.Is(err, ErrRoomSetNotFound) {
		t.Errorf("UpdateRoomSet on missing set: err = %v, want ErrRoomSetNotFound", err)
	}
}

func TestGetRoomSet(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	created, _ := core.CreateRoomSet(ctx, "actor", "Engineering", "")
	got, err := core.GetRoomSet(ctx, created.Id)
	if err != nil {
		t.Fatalf("GetRoomSet failed: %v", err)
	}
	if got.Id != created.Id || got.Name != "Engineering" {
		t.Errorf("GetRoomSet mismatch: got %+v, want id=%q name=%q", got, created.Id, "Engineering")
	}
}

func TestGetRoomSet_NotFound(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	_, err := core.GetRoomSet(ctx, "nonexistent")
	if !errors.Is(err, ErrRoomSetNotFound) {
		t.Errorf("GetRoomSet on missing set: err = %v, want ErrRoomSetNotFound", err)
	}
}

func TestDeleteRoomSet_Empty(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	set, _ := core.CreateRoomSet(ctx, "actor", "Empty", "")
	if err := core.DeleteRoomSet(ctx, "actor", set.Id); err != nil {
		t.Fatalf("DeleteRoomSet failed: %v", err)
	}

	_, err := core.GetRoomSet(ctx, set.Id)
	if !errors.Is(err, ErrRoomSetNotFound) {
		t.Errorf("Set still exists after deletion: err = %v", err)
	}
}

func TestDeleteRoomSet_RejectsNonEmpty(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	set, _ := core.CreateRoomSet(ctx, "actor", "WithRooms", "")
	room, _ := core.CreateRoom(ctx, "actor", KindChannel, "", "general", "")
	if err := core.MoveRoomToSet(ctx, "actor", room.Id, set.Id); err != nil {
		t.Fatalf("MoveRoomToSet failed: %v", err)
	}

	err := core.DeleteRoomSet(ctx, "actor", set.Id)
	if !errors.Is(err, ErrRoomSetHasRooms) {
		t.Errorf("DeleteRoomSet on populated set: err = %v, want ErrRoomSetHasRooms", err)
	}
}

func TestMoveRoomToSet(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	setA, _ := core.CreateRoomSet(ctx, "actor", "A", "")
	setB, _ := core.CreateRoomSet(ctx, "actor", "B", "")
	room, _ := core.CreateRoom(ctx, "actor", KindChannel, "", "general", "")

	if err := core.MoveRoomToSet(ctx, "actor", room.Id, setA.Id); err != nil {
		t.Fatalf("MoveRoomToSet A failed: %v", err)
	}

	gotA, _ := core.GetRoomSet(ctx, setA.Id)
	if len(gotA.RoomIds) != 1 || gotA.RoomIds[0] != room.Id {
		t.Errorf("Set A should contain the room: %+v", gotA.RoomIds)
	}

	// Move to set B; room should leave A
	if err := core.MoveRoomToSet(ctx, "actor", room.Id, setB.Id); err != nil {
		t.Fatalf("MoveRoomToSet B failed: %v", err)
	}

	gotA, _ = core.GetRoomSet(ctx, setA.Id)
	gotB, _ := core.GetRoomSet(ctx, setB.Id)
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
	err := core.MoveRoomToSet(ctx, "actor", room.Id, "nonexistent")
	if !errors.Is(err, ErrRoomSetNotFound) {
		t.Errorf("err = %v, want ErrRoomSetNotFound", err)
	}
}

func TestMoveRoomToSet_Idempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	set, _ := core.CreateRoomSet(ctx, "actor", "S", "")
	room, _ := core.CreateRoom(ctx, "actor", KindChannel, "", "general", "")

	if err := core.MoveRoomToSet(ctx, "actor", room.Id, set.Id); err != nil {
		t.Fatalf("first move failed: %v", err)
	}
	if err := core.MoveRoomToSet(ctx, "actor", room.Id, set.Id); err != nil {
		t.Fatalf("second move (idempotent) failed: %v", err)
	}

	got, _ := core.GetRoomSet(ctx, set.Id)
	if len(got.RoomIds) != 1 {
		t.Errorf("Room appears %d times in set, want exactly 1", len(got.RoomIds))
	}
}

func TestReorderRoomSets(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// The boot seed creates one "Rooms" set; capture it so we can include
	// it in the reorder list (ReorderRoomSets requires every existing set).
	seedLayout, _ := core.GetRoomLayout(ctx, KindChannel)
	seedID := seedLayout.Sets[0].Id

	a, _ := core.CreateRoomSet(ctx, "actor", "A", "")
	b, _ := core.CreateRoomSet(ctx, "actor", "B", "")
	c, _ := core.CreateRoomSet(ctx, "actor", "C", "")

	if err := core.ReorderRoomSets(ctx, "actor", []string{c.Id, a.Id, b.Id, seedID}); err != nil {
		t.Fatalf("ReorderRoomSets failed: %v", err)
	}

	layout, _ := core.GetRoomLayout(ctx, KindChannel)
	got := []string{layout.Sets[0].Id, layout.Sets[1].Id, layout.Sets[2].Id, layout.Sets[3].Id}
	want := []string{c.Id, a.Id, b.Id, seedID}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("position %d: got %q, want %q", i, got[i], want[i])
		}
	}
}

func TestReorderRoomSets_RejectsIncompleteList(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	a, _ := core.CreateRoomSet(ctx, "actor", "A", "")
	_, _ = core.CreateRoomSet(ctx, "actor", "B", "")

	// Missing the seed set + one of the created sets.
	err := core.ReorderRoomSets(ctx, "actor", []string{a.Id})
	if !errors.Is(err, ErrRoomSetNotFound) {
		t.Errorf("err = %v, want ErrRoomSetNotFound", err)
	}
}

func TestSeedSetIncludesPreExistingRooms(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Rooms created at boot or via the test helpers (e.g. before #454)
	// land in the seed "Rooms" set so the layout invariant ("every channel
	// room belongs to exactly one set") is preserved.
	room, err := core.CreateRoom(ctx, "actor", KindChannel, "", "general", "")
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	// The boot-time hook already ran in setupTestCore; CreateRoom with
	// setID="" also lands the room in the seed set if there is one.
	// Re-run the migration hook to verify idempotence + that an
	// orphaned room would get adopted.
	if err := core.ensureChannelRoomsAreInASet(ctx); err != nil {
		t.Fatalf("ensureChannelRoomsAreInASet failed: %v", err)
	}

	layout, _ := core.GetRoomLayout(ctx, KindChannel)
	if layout == nil || len(layout.Sets) == 0 {
		t.Fatal("Expected seed set to exist")
	}

	// The room should be in exactly one set, with its proto SetId stamped.
	count := 0
	var assignedSetID string
	for _, set := range layout.Sets {
		for _, rid := range set.RoomIds {
			if rid == room.Id {
				count++
				assignedSetID = set.Id
			}
		}
	}
	if count != 1 {
		t.Errorf("Room appears in %d sets, want exactly 1", count)
	}

	refreshed, _ := core.GetRoom(ctx, KindChannel, room.Id)
	if refreshed.SetId != assignedSetID {
		t.Errorf("Room.SetId = %q, want %q (the set it appears in)", refreshed.SetId, assignedSetID)
	}
}

func TestReorderRoomSets_RejectsUnknownID(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	seedLayout, _ := core.GetRoomLayout(ctx, KindChannel)
	seedID := seedLayout.Sets[0].Id
	a, _ := core.CreateRoomSet(ctx, "actor", "A", "")
	err := core.ReorderRoomSets(ctx, "actor", []string{seedID, a.Id, "unknown"})
	if !errors.Is(err, ErrRoomSetNotFound) {
		t.Errorf("err = %v, want ErrRoomSetNotFound", err)
	}
}

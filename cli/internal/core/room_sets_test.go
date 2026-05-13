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

	// Verify persisted
	layout, err := core.GetRoomLayout(ctx, KindChannel)
	if err != nil {
		t.Fatalf("GetRoomLayout failed: %v", err)
	}
	if layout == nil || len(layout.Sets) != 1 || layout.Sets[0].Id != set.Id {
		t.Fatalf("set not persisted in layout: %+v", layout)
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
	room, _ := core.CreateRoom(ctx, "actor", KindChannel, "general", "")
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
	room, _ := core.CreateRoom(ctx, "actor", KindChannel, "general", "")

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

	room, _ := core.CreateRoom(ctx, "actor", KindChannel, "general", "")
	err := core.MoveRoomToSet(ctx, "actor", room.Id, "nonexistent")
	if !errors.Is(err, ErrRoomSetNotFound) {
		t.Errorf("err = %v, want ErrRoomSetNotFound", err)
	}
}

func TestMoveRoomToSet_Idempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	set, _ := core.CreateRoomSet(ctx, "actor", "S", "")
	room, _ := core.CreateRoom(ctx, "actor", KindChannel, "general", "")

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

	a, _ := core.CreateRoomSet(ctx, "actor", "A", "")
	b, _ := core.CreateRoomSet(ctx, "actor", "B", "")
	c, _ := core.CreateRoomSet(ctx, "actor", "C", "")

	if err := core.ReorderRoomSets(ctx, "actor", []string{c.Id, a.Id, b.Id}); err != nil {
		t.Fatalf("ReorderRoomSets failed: %v", err)
	}

	layout, _ := core.GetRoomLayout(ctx, KindChannel)
	got := []string{layout.Sets[0].Id, layout.Sets[1].Id, layout.Sets[2].Id}
	want := []string{c.Id, a.Id, b.Id}
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

	err := core.ReorderRoomSets(ctx, "actor", []string{a.Id})
	if !errors.Is(err, ErrRoomSetNotFound) {
		t.Errorf("err = %v, want ErrRoomSetNotFound", err)
	}
}

func TestReorderRoomSets_RejectsUnknownID(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	a, _ := core.CreateRoomSet(ctx, "actor", "A", "")
	err := core.ReorderRoomSets(ctx, "actor", []string{a.Id, "unknown"})
	if !errors.Is(err, ErrRoomSetNotFound) {
		t.Errorf("err = %v, want ErrRoomSetNotFound", err)
	}
}

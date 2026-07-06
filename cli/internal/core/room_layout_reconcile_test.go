package core

import "testing"

// equalStrings is a small test helper shared across room-layout and
// room-groups tests. Used to live in room_layout_migration_test.go
// before phase 6 retired the legacy-shape migration tests.
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestReconcile_OrderMatchesLayoutWhenConsistent verifies that the
// reconciler preserves layout ordering when it's well-formed.
func TestReconcile_OrderMatchesLayoutWhenConsistent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	a, _ := core.CreateRoomGroup(ctx, "actor", "A", "")
	b, _ := core.CreateRoomGroup(ctx, "actor", "B", "")
	c, _ := core.CreateRoomGroup(ctx, "actor", "C", "")

	// Explicit reorder via the validated path.
	seed, _ := core.ListRoomGroupsOrdered(ctx, KindChannel)
	var seedID string
	for _, g := range seed {
		if g.Id != a.Id && g.Id != b.Id && g.Id != c.Id {
			seedID = g.Id
			break
		}
	}
	want := []string{c.Id, a.Id, seedID, b.Id}
	if err := core.ReorderRoomGroups(ctx, "actor", want); err != nil {
		t.Fatalf("ReorderRoomGroups: %v", err)
	}

	groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered: %v", err)
	}
	got := make([]string, len(groups))
	for i, g := range groups {
		got[i] = g.Id
	}
	if !equalStrings(got, want) {
		t.Errorf("order = %v, want %v", got, want)
	}
}

// TestReorderRoomGroups_RejectsDuplicates verifies the validation
// guard on the validated write path. A duplicate ID in the input is
// rejected before the layout is rewritten.
func TestReorderRoomGroups_RejectsDuplicates(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	a, _ := core.CreateRoomGroup(ctx, "actor", "A", "")
	b, _ := core.CreateRoomGroup(ctx, "actor", "B", "")

	seed, _ := core.ListRoomGroupsOrdered(ctx, KindChannel)
	var seedID string
	for _, g := range seed {
		if g.Id != a.Id && g.Id != b.Id {
			seedID = g.Id
			break
		}
	}

	// Same length as the existing set, but contains a duplicate
	// instead of `b.Id`.
	err := core.ReorderRoomGroups(ctx, "actor", []string{seedID, a.Id, a.Id})
	if err == nil {
		t.Fatal("expected ErrRoomGroupOrderMismatch for duplicate IDs")
	}
}

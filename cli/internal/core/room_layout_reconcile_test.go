package core

import (
	"testing"

	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// writeRawLayoutOrder writes a `group_ids`-only layout to the KV,
// overriding whatever the seed wrote. Used to exercise stale /
// orphan / duplicate reconciliation paths without going through the
// validated mutators.
func writeRawLayoutOrder(t *testing.T, core *ChattoCore, groupIDs []string) {
	t.Helper()
	ctx := testContext(t)
	data, err := proto.Marshal(&corev1.RoomLayout{GroupIds: groupIDs})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := core.storage.serverConfigKV.Put(ctx, roomLayoutKey, data); err != nil {
		t.Fatalf("put layout: %v", err)
	}
}

// TestReconcile_DropsStaleLayoutEntries verifies that a layout entry
// pointing at a group ID with no matching doc is silently dropped on
// read. This is the load-bearing resilience guarantee: a corrupted
// layout doesn't make existing groups disappear, and a rogue write
// referencing fictional IDs can't display ghosts.
func TestReconcile_DropsStaleLayoutEntries(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	good, _ := core.CreateRoomGroup(ctx, "actor", "Real", "")

	// Inject a stale ID into the layout.
	writeRawLayoutOrder(t, core, []string{"G_does_not_exist", good.Id})

	groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered: %v", err)
	}
	for _, g := range groups {
		if g.Id == "G_does_not_exist" {
			t.Error("stale layout entry should have been dropped")
		}
	}
	var sawGood bool
	for _, g := range groups {
		if g.Id == good.Id {
			sawGood = true
		}
	}
	if !sawGood {
		t.Error("real group should still be returned alongside stale references")
	}
}

// TestReconcile_AppendsOrphanGroups verifies that a group document
// missing from the layout's `group_ids` is still surfaced at the end of
// the reconciled list. Prevents an empty/buggy layout from making
// groups invisible.
func TestReconcile_AppendsOrphanGroups(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	a, _ := core.CreateRoomGroup(ctx, "actor", "A", "")
	b, _ := core.CreateRoomGroup(ctx, "actor", "B", "")

	// Wipe the layout's ordering — both groups are now orphans.
	writeRawLayoutOrder(t, core, []string{})

	groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered: %v", err)
	}

	ids := map[string]struct{}{}
	for _, g := range groups {
		ids[g.Id] = struct{}{}
	}
	for _, want := range []string{a.Id, b.Id} {
		if _, ok := ids[want]; !ok {
			t.Errorf("orphan group %q missing from reconciled list", want)
		}
	}
}

// TestReconcile_DeduplicatesLayoutEntries verifies that a group ID
// appearing twice in the layout is rendered once.
func TestReconcile_DeduplicatesLayoutEntries(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	g, _ := core.CreateRoomGroup(ctx, "actor", "Dup", "")
	writeRawLayoutOrder(t, core, []string{g.Id, g.Id, g.Id})

	groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered: %v", err)
	}
	count := 0
	for _, x := range groups {
		if x.Id == g.Id {
			count++
		}
	}
	if count != 1 {
		t.Errorf("duplicate group ID appears %d times, want 1", count)
	}
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

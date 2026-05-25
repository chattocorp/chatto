package core

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// writeLegacyLayout seeds the room_layout key with the pre-split shape
// (legacy_sections + legacy_unsorted_room_ids) AND wipes any existing
// per-key group documents, so the test exercises the migration as if
// the server were starting up against pre-split storage.
//
// Because setupTestCore boots Chatto fresh — which seeds a Lobby group
// AND publishes its RoomGroupCreated to the EVT stream — we also have
// to publish RoomGroupDeleted events for any pre-existing groups so the
// RoomGroups projection stays in sync with the wiped KV.
func writeLegacyLayout(t *testing.T, core *ChattoCore, legacy *corev1.RoomLayout) {
	t.Helper()
	ctx := testContext(t)

	// Remove boot-seeded group docs so the migrator only sees what the
	// legacy layout says exists. Mirror the wipe into the EVT stream
	// via RoomGroupDeleted so the projection drops them too.
	bucket := core.storage.serverConfigKV
	keyLister, err := bucket.ListKeysFiltered(ctx, roomGroupKeyPrefix+"*")
	if err != nil {
		t.Fatalf("list room_group keys: %v", err)
	}
	var lastDeleteSeq uint64
	for k := range keyLister.Keys() {
		groupID := strings.TrimPrefix(k, roomGroupKeyPrefix)
		if err := bucket.Delete(ctx, k); err != nil {
			t.Fatalf("delete %s: %v", k, err)
		}
		deletedEvent := newEvent(SystemActorID, &corev1.Event{
			Event: &corev1.Event_RoomGroupDeleted{
				RoomGroupDeleted: &corev1.RoomGroupDeletedEvent{
					GroupId: groupID,
				},
			},
		})
		seq, err := core.EventPublisher.Append(ctx, events.GroupAggregate(groupID).Subject(), deletedEvent)
		if err != nil {
			t.Fatalf("publish RoomGroupDeleted for %s: %v", groupID, err)
		}
		if seq > lastDeleteSeq {
			lastDeleteSeq = seq
		}
	}
	if lastDeleteSeq > 0 {
		if err := core.RoomGroupsProjector.WaitForSeq(ctx, lastDeleteSeq); err != nil {
			t.Fatalf("wait for RoomGroups projection to drain deletes: %v", err)
		}
	}

	data, err := proto.Marshal(legacy)
	if err != nil {
		t.Fatalf("marshal legacy layout: %v", err)
	}
	if _, err := bucket.Put(ctx, roomLayoutKey, data); err != nil {
		t.Fatalf("put legacy layout: %v", err)
	}
}

// readRawLayout returns the on-disk RoomLayout proto, bypassing the
// reconciler. Used to assert post-migration state.
func readRawLayout(t *testing.T, core *ChattoCore) *corev1.RoomLayout {
	t.Helper()
	ctx := testContext(t)
	entry, err := core.storage.serverConfigKV.Get(ctx, roomLayoutKey)
	if err != nil {
		t.Fatalf("get layout: %v", err)
	}
	layout := &corev1.RoomLayout{}
	if err := proto.Unmarshal(entry.Value(), layout); err != nil {
		t.Fatalf("unmarshal layout: %v", err)
	}
	return layout
}

// TestRoomLayoutMigration_LegacySectionsBecomePerKeyGroups covers the
// main-shape → per-key-groups migration. Legacy sections are written
// out as `room_group.{id}` documents, the layout's `group_ids` mirrors
// the legacy order, and legacy fields are cleared.
func TestRoomLayoutMigration_LegacySectionsBecomePerKeyGroups(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Overwrite the boot-seeded layout with a pre-split shape: two
	// legacy sections with rooms inside.
	writeLegacyLayout(t, core, &corev1.RoomLayout{
		LegacySections: []*corev1.RoomGroup{
			{Id: "GsecA", Name: "Section A", RoomIds: []string{"Rroom1", "Rroom2"}},
			{Id: "GsecB", Name: "Section B", RoomIds: []string{"Rroom3"}},
		},
	})

	// First reconciled read triggers the migrator.
	groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered: %v", err)
	}
	if len(groups) != 2 {
		t.Fatalf("groups = %d, want 2", len(groups))
	}
	if groups[0].Id != "GsecA" || groups[0].Name != "Section A" {
		t.Errorf("group 0 = %+v, want GsecA/Section A", groups[0])
	}
	if got, want := groups[0].RoomIds, []string{"Rroom1", "Rroom2"}; !equalStrings(got, want) {
		t.Errorf("group 0 room_ids = %v, want %v", got, want)
	}
	if groups[1].Id != "GsecB" || groups[1].Name != "Section B" {
		t.Errorf("group 1 = %+v, want GsecB/Section B", groups[1])
	}

	// The layout's legacy fields are cleared; new format is in place.
	raw := readRawLayout(t, core)
	if len(raw.LegacySections) != 0 {
		t.Errorf("LegacySections not cleared: %+v", raw.LegacySections)
	}
	if len(raw.LegacyUnsortedRoomIds) != 0 {
		t.Errorf("LegacyUnsortedRoomIds not cleared: %+v", raw.LegacyUnsortedRoomIds)
	}
	if got, want := raw.GroupIds, []string{"GsecA", "GsecB"}; !equalStrings(got, want) {
		t.Errorf("post-migration GroupIds = %v, want %v", got, want)
	}
}

// TestRoomLayoutMigration_UnsortedRoomsAbsorbedIntoFirstGroup covers
// the case where the pre-split shape had unsorted rooms. They get
// absorbed into the first legacy section's group doc.
func TestRoomLayoutMigration_UnsortedRoomsAbsorbedIntoFirstGroup(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	writeLegacyLayout(t, core, &corev1.RoomLayout{
		LegacySections: []*corev1.RoomGroup{
			{Id: "GsecA", Name: "Section A", RoomIds: []string{"Rroom1"}},
		},
		LegacyUnsortedRoomIds: []string{"RorphanA", "RorphanB"},
	})

	groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1", len(groups))
	}
	if got, want := groups[0].RoomIds, []string{"Rroom1", "RorphanA", "RorphanB"}; !equalStrings(got, want) {
		t.Errorf("first group room_ids = %v, want %v", got, want)
	}
}

// TestRoomLayoutMigration_NoSectionsCreatesSeedGroupForUnsorted covers
// the edge case where the legacy layout had ONLY unsorted rooms (no
// sections). The migrator seeds a default group to hold them.
func TestRoomLayoutMigration_NoSectionsCreatesSeedGroupForUnsorted(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	writeLegacyLayout(t, core, &corev1.RoomLayout{
		LegacyUnsortedRoomIds: []string{"RorphanA", "RorphanB"},
	})

	groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("groups = %d, want 1 (seed group)", len(groups))
	}
	if groups[0].Name != SeedDefaultRoomGroupName {
		t.Errorf("seed group name = %q, want %q", groups[0].Name, SeedDefaultRoomGroupName)
	}
	if got, want := groups[0].RoomIds, []string{"RorphanA", "RorphanB"}; !equalStrings(got, want) {
		t.Errorf("seed group room_ids = %v, want %v", got, want)
	}
}

// TestRoomLayoutMigration_Idempotent verifies the migrator short-
// circuits on the second call after the legacy fields have been drained.
func TestRoomLayoutMigration_Idempotent(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	writeLegacyLayout(t, core, &corev1.RoomLayout{
		LegacySections: []*corev1.RoomGroup{
			{Id: "Gsec1", Name: "Sec", RoomIds: []string{"Rroom1"}},
		},
	})

	first, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		t.Fatalf("first list: %v", err)
	}
	second, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		t.Fatalf("second list: %v", err)
	}
	if len(first) != len(second) {
		t.Fatalf("group count drifted: %d → %d", len(first), len(second))
	}
	for i := range first {
		if first[i].Id != second[i].Id {
			t.Errorf("group %d ID changed across calls: %s → %s", i, first[i].Id, second[i].Id)
		}
		if !equalStrings(first[i].RoomIds, second[i].RoomIds) {
			t.Errorf("group %d room_ids changed across calls: %v → %v", i, first[i].RoomIds, second[i].RoomIds)
		}
	}
}

// TestRoomLayoutMigration_AlreadyMigrated covers the no-op path: a
// layout already in the new shape (group_ids set, no legacy fields)
// passes through untouched.
func TestRoomLayoutMigration_AlreadyMigrated(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// setupTestCore boots with the new shape already in place (seed
	// group exists). Capture the order, then re-trigger via a list call.
	before := readRawLayout(t, core)
	if len(before.GroupIds) == 0 {
		t.Fatal("baseline: expected boot-seeded GroupIds")
	}
	_, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered: %v", err)
	}
	after := readRawLayout(t, core)
	if !equalStrings(before.GroupIds, after.GroupIds) {
		t.Errorf("GroupIds changed by reconciler on already-migrated layout: %v → %v", before.GroupIds, after.GroupIds)
	}
}

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

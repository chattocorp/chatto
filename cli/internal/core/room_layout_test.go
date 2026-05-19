package core

import (
	"testing"
)

func TestChattoCore_ListRoomGroupsOrdered_AfterSeed(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Every server boots with a seed "Lobby" group
	// (ensureChannelRoomsAreInAGroup), so a freshly-set-up core has
	// at least one group via the reconciler.
	groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered failed: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("Expected exactly the seed group, got %d", len(groups))
	}
	if groups[0].Name != SeedDefaultRoomGroupName {
		t.Errorf("Seed group name = %q, want %q", groups[0].Name, SeedDefaultRoomGroupName)
	}
}

func TestChattoCore_DeleteRoom_RemovesFromGroup(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Three rooms in the seed group via CreateRoom's default-group lookup.
	_, _ = core.CreateRoom(ctx, "test-user", KindChannel, "", "Keep", "")
	room2, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "Delete", "")
	_, _ = core.CreateRoom(ctx, "test-user", KindChannel, "", "AlsoKeep", "")

	if err := core.DeleteRoom(ctx, "test-user", KindChannel, room2.Id); err != nil {
		t.Fatalf("DeleteRoom failed: %v", err)
	}

	groups, err := core.ListRoomGroupsOrdered(ctx, KindChannel)
	if err != nil {
		t.Fatalf("ListRoomGroupsOrdered after delete failed: %v", err)
	}
	if len(groups) == 0 {
		t.Fatal("Expected a group to still exist")
	}
	for _, g := range groups {
		for _, id := range g.RoomIds {
			if id == room2.Id {
				t.Errorf("Deleted room should not be in group %q", g.Id)
			}
		}
	}
}

func TestChattoCore_DeleteRoom_NoLayout(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, _ := core.CreateRoom(ctx, "test-user", KindChannel, "", "Delete", "")

	// Delete room — should not error even though we just rely on the
	// seed group.
	if err := core.DeleteRoom(ctx, "test-user", KindChannel, room.Id); err != nil {
		t.Fatalf("DeleteRoom should not error: %v", err)
	}
}

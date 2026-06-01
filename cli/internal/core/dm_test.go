package core

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestRoomKindFromLegacySpaceID(t *testing.T) {
	tests := []struct {
		spaceID string
		want    RoomKind
	}{
		{LegacyDMRoomSpaceID, KindDM},
		{"DM", KindDM},
		{"some-other-space", KindChannel},
		{"", KindChannel},
		{"dm", KindChannel},
		{"__dm__", KindChannel},
	}

	for _, tt := range tests {
		t.Run(tt.spaceID, func(t *testing.T) {
			got := RoomKindFromLegacySpaceID(tt.spaceID)
			if got != tt.want {
				t.Errorf("RoomKindFromLegacySpaceID(%q) = %v, want %v", tt.spaceID, got, tt.want)
			}
		})
	}
}

func TestDMRoomID(t *testing.T) {
	t.Run("two participants - order independent", func(t *testing.T) {
		id1 := DMRoomID([]string{"user1", "user2"})
		id2 := DMRoomID([]string{"user2", "user1"})

		if id1 == "" {
			t.Error("DMRoomID returned empty string")
		}
		if id1 != id2 {
			t.Errorf("DMRoomID not order-independent: %q != %q", id1, id2)
		}
	})

	t.Run("three participants - order independent", func(t *testing.T) {
		id1 := DMRoomID([]string{"user1", "user2", "user3"})
		id2 := DMRoomID([]string{"user3", "user1", "user2"})
		id3 := DMRoomID([]string{"user2", "user3", "user1"})

		if id1 == "" {
			t.Error("DMRoomID returned empty string")
		}
		if id1 != id2 || id2 != id3 {
			t.Errorf("DMRoomID not order-independent: %q, %q, %q", id1, id2, id3)
		}
	})

	t.Run("deterministic", func(t *testing.T) {
		participants := []string{"alice", "bob"}
		id1 := DMRoomID(participants)
		id2 := DMRoomID(participants)

		if id1 != id2 {
			t.Errorf("DMRoomID not deterministic: %q != %q", id1, id2)
		}
	})

	t.Run("different participants produce different IDs", func(t *testing.T) {
		id1 := DMRoomID([]string{"user1", "user2"})
		id2 := DMRoomID([]string{"user1", "user3"})
		id3 := DMRoomID([]string{"user1", "user2", "user3"})

		if id1 == id2 {
			t.Errorf("Different participants produced same ID: %q", id1)
		}
		if id1 == id3 {
			t.Errorf("Different participants produced same ID: %q", id1)
		}
	})

	t.Run("returns empty for no participants", func(t *testing.T) {
		if id := DMRoomID([]string{}); id != "" {
			t.Errorf("Empty participants should return empty, got %q", id)
		}
	})

	t.Run("single participant produces valid ID (self-DM)", func(t *testing.T) {
		id := DMRoomID([]string{"user1"})
		if id == "" {
			t.Error("Single participant should produce valid ID for self-DM")
		}
		if len(id) != 14 {
			t.Errorf("Expected length 14, got %d: %q", len(id), id)
		}
	})

	t.Run("has correct length (14 hex chars)", func(t *testing.T) {
		id := DMRoomID([]string{"user1", "user2"})

		if len(id) != 14 {
			t.Errorf("Expected length 14, got %d: %q", len(id), id)
		}
		// Verify it's all hex characters
		for _, c := range id {
			if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
				t.Errorf("Expected hex characters, got %q in %q", c, id)
				break
			}
		}
	})
}

// TestDMBoundaryDeniedPermissions locks down which permissions are
// unconditionally denied in DM context (the privacy/category-mismatch
// floor). Mirrors the boundary set in permission_resolver.go.
func TestDMBoundaryDeniedPermissions(t *testing.T) {
	// Boundary-denied permissions — unconditionally false in DM context
	// regardless of role grants.
	denied := []Permission{
		PermRoomManage,
		PermMessageManage,
		PermMessageEcho,
		PermRoomCreate,
	}

	for _, perm := range denied {
		t.Run(string(perm)+"_boundary_denied", func(t *testing.T) {
			if !dmBoundaryDeniedPermissions[perm] {
				t.Errorf("%s should be in dmBoundaryDeniedPermissions", perm)
			}
		})
	}

	// Permissions that pass the boundary check. These can still resolve
	// to deny via the walker if no role grants them; this test only
	// asserts they aren't *unconditionally* denied.
	notBoundaryDenied := []Permission{
		PermRoomJoin,
		PermMessagePost,
		PermMessagePostInThread,
		PermMessageReact,
	}

	for _, perm := range notBoundaryDenied {
		t.Run(string(perm)+"_not_boundary_denied", func(t *testing.T) {
			if dmBoundaryDeniedPermissions[perm] {
				t.Errorf("%s should NOT be in dmBoundaryDeniedPermissions", perm)
			}
		})
	}
}

func TestDMRoomPermissionDefaults(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()

	userID := "test-user"

	t.Run("CanJoinRoom returns true for DM rooms", func(t *testing.T) {
		can, err := core.CanJoinRoom(ctx, userID, KindDM)
		if err != nil {
			t.Fatalf("CanJoinRoom error: %v", err)
		}
		if !can {
			t.Error("CanJoinRoom should return true for DM rooms")
		}
	})

	t.Run("CanManageServer returns false for a regular user", func(t *testing.T) {
		can, err := core.CanManageServer(ctx, userID)
		if err != nil {
			t.Fatalf("CanManageServer error: %v", err)
		}
		if can {
			t.Error("CanManageServer should return false for a regular user")
		}
	})

	t.Run("CanCreateRoom returns false for DM rooms", func(t *testing.T) {
		can, err := core.CanCreateRoom(ctx, userID, KindDM, "")
		if err != nil {
			t.Fatalf("CanCreateRoom error: %v", err)
		}
		if can {
			t.Error("CanCreateRoom should return false for DM rooms (use FindOrCreateDM)")
		}
	})

	t.Run("CanSeeRoom returns false for DM rooms", func(t *testing.T) {
		// DM rooms aren't surfaced through the channel room-list API; they
		// use the member-room listing path. CanSeeRoom
		// short-circuits to false for KindDM.
		can, err := core.CanSeeRoom(ctx, userID, KindDM, "R_dm_visibility_test")
		if err != nil {
			t.Fatalf("CanSeeRoom error: %v", err)
		}
		if can {
			t.Error("CanSeeRoom should return false for DM rooms (use ListMemberRooms)")
		}
	})
}

func TestFindOrCreateDM(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()

	user1 := "user1"
	user2 := "user2"
	user3 := "user3"

	t.Run("creates new DM between two users", func(t *testing.T) {
		room, created, err := core.FindOrCreateDM(ctx, user1, []string{user2})
		if err != nil {
			t.Fatalf("FindOrCreateDM error: %v", err)
		}
		if !created {
			t.Error("Expected DM to be newly created")
		}
		if room == nil {
			t.Fatal("Expected room to be non-nil")
		}
		if KindOfRoom(room) != KindDM {
			t.Errorf("Expected kind DM, got %s", KindOfRoom(room))
		}

		// Verify both users are members
		isMember1, _ := core.RoomMembershipExists(ctx, KindDM, user1, room.Id)
		isMember2, _ := core.RoomMembershipExists(ctx, KindDM, user2, room.Id)
		if !isMember1 {
			t.Error("user1 should be a member of the DM")
		}
		if !isMember2 {
			t.Error("user2 should be a member of the DM")
		}

		eventsResult, err := core.GetRoomEvents(ctx, KindDM, room.Id, 50, nil)
		if err != nil {
			t.Fatalf("GetRoomEvents for new DM: %v", err)
		}
		if len(eventsResult.Events) != 3 {
			t.Fatalf("expected 3 DM lifecycle events (room created + 2 joins), got %d", len(eventsResult.Events))
		}

		createdCount := 0
		joinedActors := map[string]bool{}
		for _, event := range eventsResult.Events {
			if created := event.GetRoomCreated(); created != nil && created.RoomId == room.Id {
				createdCount++
			}
			if joined := event.GetUserJoinedRoom(); joined != nil && joined.RoomId == room.Id {
				joinedActors[event.ActorId] = true
			}
		}
		if createdCount != 1 {
			t.Errorf("expected 1 DM RoomCreated event, got %d", createdCount)
		}
		for _, userID := range []string{user1, user2} {
			if !joinedActors[userID] {
				t.Errorf("expected DM UserJoinedRoom event for %s", userID)
			}
		}
	})

	t.Run("finds existing DM", func(t *testing.T) {
		// Create DM first
		room1, created1, err := core.FindOrCreateDM(ctx, user1, []string{user2})
		if err != nil {
			t.Fatalf("First FindOrCreateDM error: %v", err)
		}

		// Find same DM (order shouldn't matter)
		room2, created2, err := core.FindOrCreateDM(ctx, user2, []string{user1})
		if err != nil {
			t.Fatalf("Second FindOrCreateDM error: %v", err)
		}

		if room1.Id != room2.Id {
			t.Errorf("Expected same room ID, got %s and %s", room1.Id, room2.Id)
		}
		if created1 && created2 {
			t.Error("Second call should not have created a new DM")
		}
	})

	t.Run("creates group DM with three users", func(t *testing.T) {
		room, created, err := core.FindOrCreateDM(ctx, user1, []string{user2, user3})
		if err != nil {
			t.Fatalf("FindOrCreateDM error: %v", err)
		}
		if !created {
			t.Error("Expected group DM to be newly created")
		}

		// Verify all three users are members
		for _, userID := range []string{user1, user2, user3} {
			isMember, _ := core.RoomMembershipExists(ctx, KindDM, userID, room.Id)
			if !isMember {
				t.Errorf("%s should be a member of the group DM", userID)
			}
		}
	})

	t.Run("different participants create different DMs", func(t *testing.T) {
		room12, _, _ := core.FindOrCreateDM(ctx, user1, []string{user2})
		room13, _, _ := core.FindOrCreateDM(ctx, user1, []string{user3})
		room123, _, _ := core.FindOrCreateDM(ctx, user1, []string{user2, user3})

		if room12.Id == room13.Id {
			t.Error("DM with user2 and DM with user3 should have different IDs")
		}
		if room12.Id == room123.Id {
			t.Error("2-person DM and 3-person DM should have different IDs")
		}
	})

	t.Run("creates self-DM with no other participants", func(t *testing.T) {
		room, created, err := core.FindOrCreateDM(ctx, user1, []string{})
		if err != nil {
			t.Fatalf("FindOrCreateDM error for self-DM: %v", err)
		}
		if !created {
			t.Error("Expected self-DM to be newly created")
		}
		if room == nil {
			t.Fatal("Expected room to be non-nil")
		}
		if KindOfRoom(room) != KindDM {
			t.Errorf("Expected kind DM, got %s", KindOfRoom(room))
		}

		// Verify user is a member
		isMember, _ := core.RoomMembershipExists(ctx, KindDM, user1, room.Id)
		if !isMember {
			t.Error("user1 should be a member of their self-DM")
		}
	})

	t.Run("rejects more than MaxDMParticipants", func(t *testing.T) {
		// Create a list of 11 participants (including creator = 11 total, exceeds limit of 10)
		participants := make([]string, MaxDMParticipants)
		for i := 0; i < MaxDMParticipants; i++ {
			participants[i] = fmt.Sprintf("participant%d", i)
		}
		// user1 + 10 participants = 11 total
		_, _, err := core.FindOrCreateDM(ctx, user1, participants)
		if err == nil {
			t.Errorf("Expected error for %d participants (exceeds limit of %d)", MaxDMParticipants+1, MaxDMParticipants)
		}
	})

	t.Run("allows exactly MaxDMParticipants", func(t *testing.T) {
		// Create a list of 9 participants (including creator = 10 total, at limit)
		participants := make([]string, MaxDMParticipants-1)
		for i := 0; i < MaxDMParticipants-1; i++ {
			participants[i] = fmt.Sprintf("max-participant%d", i)
		}
		// user1 + 9 participants = 10 total
		room, _, err := core.FindOrCreateDM(ctx, user1, participants)
		if err != nil {
			t.Errorf("Expected success for %d participants, got error: %v", MaxDMParticipants, err)
		}
		if room == nil {
			t.Error("Expected room to be non-nil")
		}
	})
}

func listActiveDMRoomsForTest(t *testing.T, core *ChattoCore, ctx context.Context, userID string) []*corev1.Room {
	t.Helper()
	rooms, err := core.ListMemberRooms(ctx, KindDM, userID, MemberRoomListOptions{
		RequireLastMessage:    true,
		SortByLastMessageDesc: true,
	})
	if err != nil {
		t.Fatalf("ListMemberRooms error: %v", err)
	}
	return rooms
}

func TestListMemberRooms_DMConversationPolicy(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	t.Run("returns empty list when no DMs", func(t *testing.T) {
		rooms := listActiveDMRoomsForTest(t, core, ctx, "no-dms-user")
		if len(rooms) != 0 {
			t.Errorf("Expected 0 DMs, got %d", len(rooms))
		}
	})

	t.Run("excludes DMs with no messages", func(t *testing.T) {
		user1, err := core.CreateUser(ctx, "system", "listdm1", "List DM 1", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}
		user2, err := core.CreateUser(ctx, "system", "listdm2", "List DM 2", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		// Create a DM but don't send any messages
		_, _, err = core.FindOrCreateDM(ctx, user1.Id, []string{user2.Id})
		if err != nil {
			t.Fatalf("FindOrCreateDM error: %v", err)
		}

		// Empty DM should NOT appear in the list
		rooms := listActiveDMRoomsForTest(t, core, ctx, user1.Id)
		if len(rooms) != 0 {
			t.Errorf("Expected 0 DMs (empty DM should be filtered), got %d", len(rooms))
		}
	})

	t.Run("includes DMs with messages", func(t *testing.T) {
		user1, err := core.CreateUser(ctx, "system", "listdm3", "List DM 3", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}
		user2, err := core.CreateUser(ctx, "system", "listdm4", "List DM 4", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}
		user3, err := core.CreateUser(ctx, "system", "listdm5", "List DM 5", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		// Create two DMs
		dm1, _, err := core.FindOrCreateDM(ctx, user1.Id, []string{user2.Id})
		if err != nil {
			t.Fatalf("FindOrCreateDM error: %v", err)
		}
		dm2, _, err := core.FindOrCreateDM(ctx, user1.Id, []string{user3.Id})
		if err != nil {
			t.Fatalf("FindOrCreateDM error: %v", err)
		}

		// Post a message only in dm1
		_, err = core.PostMessage(ctx, KindDM, dm1.Id, user1.Id, "Hello!", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("PostMessage error: %v", err)
		}

		// Only dm1 should appear (dm2 has no messages)
		rooms := listActiveDMRoomsForTest(t, core, ctx, user1.Id)
		if len(rooms) != 1 {
			t.Fatalf("Expected 1 DM, got %d", len(rooms))
		}
		if rooms[0].Id != dm1.Id {
			t.Errorf("Expected DM with user2, got room %s", rooms[0].Id)
		}

		// Now post a message in dm2
		_, err = core.PostMessage(ctx, KindDM, dm2.Id, user1.Id, "Hey!", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("PostMessage error: %v", err)
		}

		// Both should now appear
		rooms = listActiveDMRoomsForTest(t, core, ctx, user1.Id)
		if len(rooms) != 2 {
			t.Errorf("Expected 2 DMs after both have messages, got %d", len(rooms))
		}
	})
}

func TestListMemberRooms_DMConversationPolicySortedByLastMessage(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create users
	userA, err := core.CreateUser(ctx, "system", "sorta", "Sort User A", "password123")
	if err != nil {
		t.Fatalf("Failed to create userA: %v", err)
	}
	userB, err := core.CreateUser(ctx, "system", "sortb", "Sort User B", "password123")
	if err != nil {
		t.Fatalf("Failed to create userB: %v", err)
	}
	userC, err := core.CreateUser(ctx, "system", "sortc", "Sort User C", "password123")
	if err != nil {
		t.Fatalf("Failed to create userC: %v", err)
	}

	// Create two DMs: A-B and A-C
	dmAB, _, err := core.FindOrCreateDM(ctx, userA.Id, []string{userB.Id})
	if err != nil {
		t.Fatalf("Failed to create DM A-B: %v", err)
	}
	dmAC, _, err := core.FindOrCreateDM(ctx, userA.Id, []string{userC.Id})
	if err != nil {
		t.Fatalf("Failed to create DM A-C: %v", err)
	}

	// Post message to A-B first
	_, err = core.PostMessage(ctx, KindDM, dmAB.Id, userA.Id, "First message in A-B", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message to A-B: %v", err)
	}

	// Post message to A-C second (more recent)
	_, err = core.PostMessage(ctx, KindDM, dmAC.Id, userA.Id, "Message in A-C", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message to A-C: %v", err)
	}

	// List should have A-C first (more recent)
	rooms := listActiveDMRoomsForTest(t, core, ctx, userA.Id)
	if len(rooms) != 2 {
		t.Fatalf("Expected 2 DMs, got %d", len(rooms))
	}
	if rooms[0].Id != dmAC.Id {
		t.Errorf("Expected A-C first (most recent), got %s", rooms[0].Id)
	}
	if rooms[1].Id != dmAB.Id {
		t.Errorf("Expected A-B second, got %s", rooms[1].Id)
	}

	// Post new message to A-B to make it most recent
	_, err = core.PostMessage(ctx, KindDM, dmAB.Id, userB.Id, "New message in A-B", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post second message to A-B: %v", err)
	}

	// Now A-B should be first
	rooms = listActiveDMRoomsForTest(t, core, ctx, userA.Id)
	if rooms[0].Id != dmAB.Id {
		t.Errorf("Expected A-B first after new message, got %s", rooms[0].Id)
	}
	if rooms[1].Id != dmAC.Id {
		t.Errorf("Expected A-C second after new message, got %s", rooms[1].Id)
	}
}

func TestDMRoomParticipants(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := context.Background()

	user1 := "user1"
	user2 := "user2"
	user3 := "user3"

	t.Run("returns all participants", func(t *testing.T) {
		room, _, _ := core.FindOrCreateDM(ctx, user1, []string{user2, user3})

		members, err := core.GetRoomMembersList(ctx, KindDM, room.Id)
		if err != nil {
			t.Fatalf("GetRoomMembersList error: %v", err)
		}

		participants := make([]string, len(members))
		for i, member := range members {
			participants[i] = member.UserId
		}
		if len(participants) != 3 {
			t.Errorf("Expected 3 participants, got %d", len(participants))
		}

		// Verify all expected participants are present
		participantSet := make(map[string]bool)
		for _, p := range participants {
			participantSet[p] = true
		}
		for _, expected := range []string{user1, user2, user3} {
			if !participantSet[expected] {
				t.Errorf("Expected participant %s not found", expected)
			}
		}
	})
}

func TestEnsureInList(t *testing.T) {
	t.Run("adds ID if not present", func(t *testing.T) {
		list := []string{"a", "b"}
		result := ensureInList(list, "c")
		if len(result) != 3 {
			t.Errorf("Expected 3 items, got %d", len(result))
		}
	})

	t.Run("does not duplicate if already present", func(t *testing.T) {
		list := []string{"a", "b", "c"}
		result := ensureInList(list, "b")
		if len(result) != 3 {
			t.Errorf("Expected 3 items (no duplicate), got %d", len(result))
		}
	})
}

func TestDMUnreadStatus(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create two users
	user1, err := core.CreateUser(ctx, "system", "dmunread1", "DM Unread User 1", "password123")
	if err != nil {
		t.Fatalf("Failed to create user1: %v", err)
	}
	user2, err := core.CreateUser(ctx, "system", "dmunread2", "DM Unread User 2", "password123")
	if err != nil {
		t.Fatalf("Failed to create user2: %v", err)
	}

	// Create a DM conversation
	room, _, err := core.FindOrCreateDM(ctx, user1.Id, []string{user2.Id})
	if err != nil {
		t.Fatalf("Failed to create DM: %v", err)
	}

	t.Run("no unread when no messages", func(t *testing.T) {
		hasUnread, err := core.HasUnread(ctx, KindDM, user2.Id, room.Id)
		if err != nil {
			t.Fatalf("HasUnread error: %v", err)
		}
		if hasUnread {
			t.Error("Expected no unread for empty DM")
		}
	})

	t.Run("user2 has unread after user1 posts", func(t *testing.T) {
		// user1 posts a message
		_, err := core.PostMessage(ctx, KindDM, room.Id, user1.Id, "Hello from user1!", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post message: %v", err)
		}

		// user2 should have unread
		hasUnread, err := core.HasUnread(ctx, KindDM, user2.Id, room.Id)
		if err != nil {
			t.Fatalf("HasUnread error: %v", err)
		}
		if !hasUnread {
			t.Error("Expected user2 to have unread after user1 posted")
		}

		// user1 should NOT have unread (they posted)
		hasUnread, err = core.HasUnread(ctx, KindDM, user1.Id, room.Id)
		if err != nil {
			t.Fatalf("HasUnread error for user1: %v", err)
		}
		if hasUnread {
			t.Error("Expected user1 to NOT have unread (they posted)")
		}
	})

	t.Run("unread clears after marking as read", func(t *testing.T) {
		// Get room's last event
		lastID, _, exists, err := core.GetRoomLastEvent(ctx, KindDM, room.Id)
		if err != nil {
			t.Fatalf("GetRoomLastEvent error: %v", err)
		}
		if !exists {
			t.Fatal("Expected last event to exist after posting")
		}

		// Mark as read for user2
		if err := core.SetLastReadEventID(ctx, KindDM, user2.Id, room.Id, lastID); err != nil {
			t.Fatalf("SetLastReadEventID error: %v", err)
		}

		// user2 should no longer have unread
		hasUnread, err := core.HasUnread(ctx, KindDM, user2.Id, room.Id)
		if err != nil {
			t.Fatalf("HasUnread error: %v", err)
		}
		if hasUnread {
			t.Error("Expected user2 to NOT have unread after marking as read")
		}
	})
}

func TestDMReactions(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create two users
	user1, err := core.CreateUser(ctx, "system", "dmreact1", "DM React User 1", "password123")
	if err != nil {
		t.Fatalf("Failed to create user1: %v", err)
	}
	user2, err := core.CreateUser(ctx, "system", "dmreact2", "DM React User 2", "password123")
	if err != nil {
		t.Fatalf("Failed to create user2: %v", err)
	}

	// Create a DM conversation
	room, _, err := core.FindOrCreateDM(ctx, user1.Id, []string{user2.Id})
	if err != nil {
		t.Fatalf("Failed to create DM: %v", err)
	}

	// Post a message
	event, err := core.PostMessage(ctx, KindDM, room.Id, user1.Id, "Test DM message for reactions", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}
	messageEventID := event.Id

	t.Run("can add reaction to DM message", func(t *testing.T) {
		added, err := core.AddReaction(ctx, KindDM, room.Id, messageEventID, "thumbsup", user2.Id)
		if err != nil {
			t.Fatalf("AddReaction error: %v", err)
		}
		if !added {
			t.Error("Expected reaction to be added")
		}
	})

	t.Run("can get reactions from DM message", func(t *testing.T) {
		reactions, err := core.GetReactions(ctx, messageEventID)
		if err != nil {
			t.Fatalf("GetReactions error: %v", err)
		}
		if len(reactions) == 0 {
			t.Error("Expected at least one reaction")
		}
	})

	t.Run("can remove reaction from DM message", func(t *testing.T) {
		removed, err := core.RemoveReaction(ctx, KindDM, room.Id, messageEventID, "thumbsup", user2.Id)
		if err != nil {
			t.Fatalf("RemoveReaction error: %v", err)
		}
		if !removed {
			t.Error("Expected reaction to be removed")
		}
	})
}

func TestDMNotifications(t *testing.T) {
	core, nc := setupTestCore(t)
	ctx := testContext(t)

	// Create two users
	user1, err := core.CreateUser(ctx, "system", "dmnotify1", "DM Notify User 1", "password123")
	if err != nil {
		t.Fatalf("Failed to create user1: %v", err)
	}
	user2, err := core.CreateUser(ctx, "system", "dmnotify2", "DM Notify User 2", "password123")
	if err != nil {
		t.Fatalf("Failed to create user2: %v", err)
	}

	// Create a DM conversation
	room, _, err := core.FindOrCreateDM(ctx, user1.Id, []string{user2.Id})
	if err != nil {
		t.Fatalf("Failed to create DM: %v", err)
	}

	t.Run("DM message triggers notification to other participants", func(t *testing.T) {
		// Subscribe to user2's notification subject
		notificationReceived := make(chan bool, 1)
		sub, err := nc.Subscribe(subjects.LiveSyncUserEvent(user2.Id, "dm_message"), func(msg *nats.Msg) {
			notificationReceived <- true
		})
		if err != nil {
			t.Fatalf("Failed to subscribe: %v", err)
		}
		defer sub.Unsubscribe()

		// Post a message from user1
		_, err = core.PostMessage(ctx, KindDM, room.Id, user1.Id, "Test DM notification message", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post message: %v", err)
		}

		// Wait for notification with timeout
		select {
		case <-notificationReceived:
			// Success - notification received
		case <-time.After(2 * time.Second):
			t.Error("Expected to receive DM notification for user2")
		}
	})

	t.Run("DM message does not notify sender", func(t *testing.T) {
		// Subscribe to user1's notification subject (the sender)
		notificationReceived := make(chan bool, 1)
		sub, err := nc.Subscribe(subjects.LiveSyncUserEvent(user1.Id, "dm_message"), func(msg *nats.Msg) {
			notificationReceived <- true
		})
		if err != nil {
			t.Fatalf("Failed to subscribe: %v", err)
		}
		defer sub.Unsubscribe()

		// Post a message from user1
		_, err = core.PostMessage(ctx, KindDM, room.Id, user1.Id, "Another test DM message", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post message: %v", err)
		}

		// Sender should NOT receive notification
		select {
		case <-notificationReceived:
			t.Error("Sender should not receive their own DM notification")
		case <-time.After(500 * time.Millisecond):
			// Success - no notification to sender
		}
	})
}

func TestDMThreadReplyEcho(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create two users
	user1, err := core.CreateUser(ctx, "system", "dmecho1", "DM Echo User 1", "password123")
	if err != nil {
		t.Fatalf("Failed to create user1: %v", err)
	}
	user2, err := core.CreateUser(ctx, "system", "dmecho2", "DM Echo User 2", "password123")
	if err != nil {
		t.Fatalf("Failed to create user2: %v", err)
	}

	// Create a DM conversation
	room, _, err := core.FindOrCreateDM(ctx, user1.Id, []string{user2.Id})
	if err != nil {
		t.Fatalf("Failed to create DM: %v", err)
	}

	t.Run("echo works in DM rooms", func(t *testing.T) {
		// Post root message
		rootEvent, err := core.PostMessage(ctx, KindDM, room.Id, user1.Id, "DM thread root", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post root: %v", err)
		}

		// Post thread reply with echo
		replyEvent, err := core.PostMessage(ctx, KindDM, room.Id, user1.Id, "DM thread reply echoed", nil, rootEvent.Id, "", nil, true)
		if err != nil {
			t.Fatalf("Failed to post echo reply in DM: %v", err)
		}

		reply := replyEvent.GetMessagePosted()
		if reply == nil {
			t.Fatal("Expected MessagePosted event for DM reply")
		}

		// Verify echo appears in room events
		roomEventsResult, err := core.GetRoomEvents(ctx, KindDM, room.Id, 50, nil)
		if err != nil {
			t.Fatalf("Failed to get DM room events: %v", err)
		}
		roomEvents := roomEventsResult.Events

		var foundEcho bool
		for _, e := range roomEvents {
			if msg := e.GetMessagePosted(); msg != nil && msg.EchoOfEventId == replyEvent.Id {
				foundEcho = true
				// The DM echo has its own envelope id and links back to the
				// original reply via EchoOfEventId.
				if msg.EchoFromThreadRootEventId != rootEvent.Id {
					t.Errorf("DM echo ThreadRootEventId should be %q, got %q", rootEvent.Id, msg.EchoFromThreadRootEventId)
				}
				break
			}
		}
		if !foundEcho {
			t.Error("Expected echo MessagePostedEvent in DM room events")
		}
	})

	t.Run("echo does not appear in thread events", func(t *testing.T) {
		// Post root and reply with echo
		rootEvent, _ := core.PostMessage(ctx, KindDM, room.Id, user2.Id, "DM root for thread test", nil, "", "", nil, false)
		_, err := core.PostMessage(ctx, KindDM, room.Id, user2.Id, "DM reply for thread test", nil, rootEvent.Id, "", nil, true)
		if err != nil {
			t.Fatalf("Failed to post echo reply: %v", err)
		}

		// Thread events should not contain the echo
		threadEvents, err := core.GetThreadEvents(ctx, KindDM, room.Id, rootEvent.Id)
		if err != nil {
			t.Fatalf("Failed to get DM thread events: %v", err)
		}
		for _, e := range threadEvents {
			if msg := e.GetMessagePosted(); msg != nil && msg.EchoOfEventId != "" {
				t.Error("Echo MessagePostedEvent should NOT appear in DM thread events")
			}
		}
	})

	t.Run("reply_count only increments once with echo in DM", func(t *testing.T) {
		rootEvent, _ := core.PostMessage(ctx, KindDM, room.Id, user1.Id, "DM root for count", nil, "", "", nil, false)
		_, err := core.PostMessage(ctx, KindDM, room.Id, user1.Id, "DM reply with echo", nil, rootEvent.Id, "", nil, true)
		if err != nil {
			t.Fatalf("Failed to post reply: %v", err)
		}

		metadata, err := core.GetThreadMetadata(ctx, KindDM, room.Id, rootEvent.Id)
		if err != nil {
			t.Fatalf("Failed to get metadata: %v", err)
		}
		if metadata.ReplyCount != 1 {
			t.Errorf("Expected ReplyCount=1 in DM (echo should not increment), got %d", metadata.ReplyCount)
		}
	})
}

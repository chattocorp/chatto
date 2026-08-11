package core

import (
	"errors"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestServerMemberUserPage(t *testing.T) {
	oldest := timestamppb.New(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	newest := timestamppb.New(time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC))
	users := []*corev1.User{
		{Id: "nil-z", Login: "Zulu", DisplayName: "Legacy Zulu"},
		{Id: "new", Login: "bravo", DisplayName: "Second Match", CreatedAt: newest},
		{Id: "nil-a", Login: "alpha", DisplayName: "Legacy Alpha"},
		{Id: "old", Login: "FIRST-match", DisplayName: "Oldest", CreatedAt: oldest},
	}

	tests := []struct {
		name      string
		search    string
		limit     int
		offset    int
		wantIDs   []string
		wantTotal int
	}{
		{
			name:      "orders timestamps before nil timestamps and nil timestamps by login",
			wantIDs:   []string{"old", "new", "nil-a", "nil-z"},
			wantTotal: 4,
		},
		{
			name:      "matches login case insensitively",
			search:    "FiRsT",
			wantIDs:   []string{"old"},
			wantTotal: 1,
		},
		{
			name:      "matches display name substrings case insensitively",
			search:    "cOnD mAt",
			wantIDs:   []string{"new"},
			wantTotal: 1,
		},
		{
			name:      "calculates total before pagination",
			limit:     2,
			offset:    1,
			wantIDs:   []string{"new", "nil-a"},
			wantTotal: 4,
		},
		{
			name:      "returns an empty page beyond the result set",
			limit:     2,
			offset:    10,
			wantIDs:   []string{},
			wantTotal: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, total := serverMemberUserPage(users, tt.search, tt.limit, tt.offset)
			if total != tt.wantTotal {
				t.Fatalf("total = %d, want %d", total, tt.wantTotal)
			}
			gotIDs := make([]string, 0, len(got))
			for _, user := range got {
				gotIDs = append(gotIDs, user.GetId())
			}
			if len(gotIDs) != len(tt.wantIDs) {
				t.Fatalf("IDs = %v, want %v", gotIDs, tt.wantIDs)
			}
			for i := range gotIDs {
				if gotIDs[i] != tt.wantIDs[i] {
					t.Fatalf("IDs = %v, want %v", gotIDs, tt.wantIDs)
				}
			}
		})
	}
}

func TestSeedDefaultRooms(t *testing.T) {
	t.Run("creates announcements and general in the seed Lobby group", func(t *testing.T) {
		c, _ := setupTestCore(t)
		ctx := testContext(t)

		if err := c.SeedDefaultRooms(ctx); err != nil {
			t.Fatalf("SeedDefaultRooms failed: %v", err)
		}

		rooms, err := c.ListRooms(ctx, KindChannel)
		if err != nil {
			t.Fatalf("ListRooms failed: %v", err)
		}
		names := map[string]string{}
		universal := map[string]bool{}
		for _, r := range rooms {
			names[r.Name] = r.GroupId
			universal[r.Name] = r.GetUniversal()
		}
		for _, want := range []string{"announcements", "general"} {
			if _, ok := names[want]; !ok {
				t.Errorf("expected room %q after seeding, got %v", want, names)
			}
		}

		groups, err := c.ListRoomGroupsOrdered(ctx, KindChannel)
		if err != nil {
			t.Fatalf("ListRoomGroupsOrdered failed: %v", err)
		}
		if len(groups) != 1 || groups[0].Name != SeedDefaultRoomGroupName {
			t.Fatalf("expected single seed Lobby group, got %+v", groups)
		}
		lobbyID := groups[0].Id
		for name, gid := range names {
			if gid != lobbyID {
				t.Errorf("room %q is in group %q, expected Lobby %q", name, gid, lobbyID)
			}
		}
		if !universal[AnnouncementsRoomName] {
			t.Errorf("%q should be universal after seeding", AnnouncementsRoomName)
		}
		if universal["general"] {
			t.Error("general should not be universal after seeding")
		}
	})

	t.Run("announcements room denies message.post to everyone", func(t *testing.T) {
		c, _ := setupTestCore(t)
		ctx := testContext(t)

		if err := c.SeedDefaultRooms(ctx); err != nil {
			t.Fatalf("SeedDefaultRooms failed: %v", err)
		}

		rooms, _ := c.ListRooms(ctx, KindChannel)
		var announcementsID string
		for _, r := range rooms {
			if r.Name == AnnouncementsRoomName {
				announcementsID = r.Id
				break
			}
		}
		if announcementsID == "" {
			t.Fatal("announcements room not found")
		}

		// Create a regular member and verify they cannot post root messages.
		user, err := c.CreateUser(ctx, "system", "member", "Member", "password123")
		if err != nil {
			t.Fatalf("CreateUser failed: %v", err)
		}
		can, err := c.CanPostMessage(ctx, user.Id, KindChannel, announcementsID)
		if err != nil {
			t.Fatalf("CanPostMessage failed: %v", err)
		}
		if can {
			t.Error("regular member should NOT be able to post root messages in announcements")
		}

		// And they CAN still post in threads.
		canThread, err := c.CanPostInThread(ctx, user.Id, KindChannel, announcementsID)
		if err != nil {
			t.Fatalf("CanPostInThread failed: %v", err)
		}
		if !canThread {
			t.Error("regular member SHOULD be able to post in threads in announcements")
		}

		member, err := c.RoomMembershipExists(ctx, KindChannel, user.Id, announcementsID)
		if err != nil {
			t.Fatalf("RoomMembershipExists failed: %v", err)
		}
		if !member {
			t.Fatal("announcements should grant effective membership to join-eligible members")
		}
		if err := c.LeaveRoom(ctx, user.Id, KindChannel, user.Id, announcementsID); !errors.Is(err, ErrCannotLeaveUniversalRoom) {
			t.Fatalf("expected ErrCannotLeaveUniversalRoom when leaving announcements, got %v", err)
		}
	})

	t.Run("idempotent — no-op when channel rooms already exist", func(t *testing.T) {
		c, _ := setupTestCore(t)
		ctx := testContext(t)

		if _, err := c.CreateRoom(ctx, "test-user", KindChannel, "", "existing", ""); err != nil {
			t.Fatalf("CreateRoom failed: %v", err)
		}

		if err := c.SeedDefaultRooms(ctx); err != nil {
			t.Fatalf("SeedDefaultRooms failed: %v", err)
		}

		rooms, _ := c.ListRooms(ctx, KindChannel)
		if len(rooms) != 1 {
			t.Errorf("expected SeedDefaultRooms to be a no-op when rooms exist; got %d rooms", len(rooms))
		}
	})

	t.Run("safe to call twice", func(t *testing.T) {
		c, _ := setupTestCore(t)
		ctx := testContext(t)

		if err := c.SeedDefaultRooms(ctx); err != nil {
			t.Fatalf("first SeedDefaultRooms failed: %v", err)
		}
		if err := c.SeedDefaultRooms(ctx); err != nil {
			t.Fatalf("second SeedDefaultRooms failed: %v", err)
		}

		rooms, _ := c.ListRooms(ctx, KindChannel)
		if len(rooms) != len(DefaultGlobalRooms) {
			t.Errorf("expected %d rooms after double-seed, got %d", len(DefaultGlobalRooms), len(rooms))
		}
	})
}

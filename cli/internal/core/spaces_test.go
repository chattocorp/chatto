package core

import (
	"testing"
)

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
		for _, r := range rooms {
			names[r.Name] = r.GroupId
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

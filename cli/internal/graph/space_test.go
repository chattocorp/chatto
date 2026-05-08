package graph

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

// ============================================================================
// Space Field Resolver Tests
// ============================================================================

func TestSpaceResolver_Rooms(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("list rooms for space (authorized)", func(t *testing.T) {
		rooms, err := env.resolver.Space().Rooms(env.authContext(), env.testSpace, nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(rooms) == 0 {
			t.Fatal("Expected at least one room")
		}

		// Verify test room is in the list
		found := false
		for _, room := range rooms {
			if room.Id == env.testRoom.Id {
				found = true
				break
			}
		}

		if !found {
			t.Error("Test room not found in rooms list")
		}
	})

	t.Run("list rooms for space (unauthorized - not a member)", func(t *testing.T) {
		// Create a user who is not a member
		user2, err := env.core.CreateUser(env.ctx, "system", "outsider", "outsider", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		rooms, err := env.resolver.Space().Rooms(env.authContextForUser(user2), env.testSpace, nil)
		if !errors.Is(err, ErrNotSpaceMember) {
			t.Errorf("Expected ErrNotSpaceMember, got %v", err)
		}

		if rooms != nil {
			t.Errorf("Expected nil rooms, got %+v", rooms)
		}
	})

	// Phase 3 of #330 / ADR-027: when called on the primary space, the result
	// also contains the caller's DM conversations, so the unified Server sidebar
	// can render them alongside channels. The DM space is auto-resolved as the
	// primary in this fixture (only one user-facing space exists).
	t.Run("primary space includes the caller's DM rooms", func(t *testing.T) {
		other := env.createVerifiedUser(t, "dm-peer", "DM Peer", "password123")

		dm, _, err := env.core.FindOrCreateDM(env.ctx, env.testUser.Id, []string{other.Id})
		if err != nil {
			t.Fatalf("Failed to create DM: %v", err)
		}

		// Post a message so the DM is non-empty (ListDMConversations filters empties).
		if _, err := env.core.PostMessage(env.ctx, core.DMSpaceID, dm.Id, env.testUser.Id, "hi", nil, "", "", nil, false); err != nil {
			t.Fatalf("Failed to post DM message: %v", err)
		}

		rooms, err := env.resolver.Space().Rooms(env.authContext(), env.testSpace, nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		var sawChannel, sawDM bool
		for _, r := range rooms {
			if r.Id == env.testRoom.Id {
				sawChannel = true
			}
			if r.Id == dm.Id && r.SpaceId == core.DMSpaceID {
				sawDM = true
			}
		}
		if !sawChannel {
			t.Error("expected channel room to be in primary space rooms list")
		}
		if !sawDM {
			t.Error("expected DM room to be merged into primary space rooms list")
		}
	})

	// `type` is the explicit room-kind filter introduced for callers like the
	// admin "Manage rooms" page that want channels-only.
	t.Run("type:CHANNEL excludes the caller's DMs", func(t *testing.T) {
		other := env.createVerifiedUser(t, "type-channel-peer", "Peer", "password123")
		dm, _, err := env.core.FindOrCreateDM(env.ctx, env.testUser.Id, []string{other.Id})
		if err != nil {
			t.Fatalf("Failed to create DM: %v", err)
		}
		if _, err := env.core.PostMessage(env.ctx, core.DMSpaceID, dm.Id, env.testUser.Id, "hi", nil, "", "", nil, false); err != nil {
			t.Fatalf("Failed to post DM message: %v", err)
		}

		channelOnly := model.RoomTypeChannel
		rooms, err := env.resolver.Space().Rooms(env.authContext(), env.testSpace, &channelOnly)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		for _, r := range rooms {
			if r.SpaceId == core.DMSpaceID {
				t.Errorf("type:CHANNEL should exclude DM rooms, got %+v", r)
			}
		}
		// Sanity: channel room is still present.
		var sawChannel bool
		for _, r := range rooms {
			if r.Id == env.testRoom.Id {
				sawChannel = true
			}
		}
		if !sawChannel {
			t.Error("expected channel room to be present under type:CHANNEL")
		}
	})

	t.Run("type:DM returns only the caller's DMs on the server space", func(t *testing.T) {
		other := env.createVerifiedUser(t, "type-dm-peer-2", "Peer", "password123")
		dm, _, err := env.core.FindOrCreateDM(env.ctx, env.testUser.Id, []string{other.Id})
		if err != nil {
			t.Fatalf("Failed to create DM: %v", err)
		}
		if _, err := env.core.PostMessage(env.ctx, core.DMSpaceID, dm.Id, env.testUser.Id, "hi", nil, "", "", nil, false); err != nil {
			t.Fatalf("Failed to post DM message: %v", err)
		}

		dmOnly := model.RoomTypeDm
		rooms, err := env.resolver.Space().Rooms(env.authContext(), env.testSpace, &dmOnly)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		for _, r := range rooms {
			if r.SpaceId != core.DMSpaceID {
				t.Errorf("type:DM should exclude channel rooms, got %+v", r)
			}
		}
	})
}

func TestRoomResolver_Type(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("regular room is CHANNEL", func(t *testing.T) {
		got, err := env.resolver.Room().Type(env.authContext(), env.testRoom)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if got != model.RoomTypeChannel {
			t.Errorf("expected CHANNEL, got %v", got)
		}
	})

	t.Run("DM room is DM", func(t *testing.T) {
		other := env.createVerifiedUser(t, "type-dm-peer", "Peer", "password123")
		dm, _, err := env.core.FindOrCreateDM(env.ctx, env.testUser.Id, []string{other.Id})
		if err != nil {
			t.Fatalf("Failed to create DM: %v", err)
		}
		got, err := env.resolver.Room().Type(env.authContext(), dm)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if got != model.RoomTypeDm {
			t.Errorf("expected DM, got %v", got)
		}
	})
}

// ============================================================================
// Room Field Resolver Tests
// ============================================================================

func TestRoomResolver_Members(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("room member can list members", func(t *testing.T) {
		members, err := env.resolver.Room().Members(env.authContext(), env.testRoom)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if members == nil {
			t.Fatal("Expected members, got nil")
		}
		// Should have at least the test user
		if len(members) == 0 {
			t.Error("Expected at least one member")
		}
	})

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		members, err := env.resolver.Room().Members(env.unauthContext(), env.testRoom)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}
		if members != nil {
			t.Errorf("Expected nil members, got %+v", members)
		}
	})

	t.Run("non-room-member is rejected", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-members", "Outsider", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		members, err := env.resolver.Room().Members(env.authContextForUser(outsider), env.testRoom)
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("Expected ErrNotRoomMember, got %v", err)
		}
		if members != nil {
			t.Errorf("Expected nil members, got %+v", members)
		}
	})

	t.Run("space member but not room member is rejected", func(t *testing.T) {
		spaceMember, err := env.core.CreateUser(env.ctx, "system", "spacemember-members", "Space Member", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}
		_, err = env.core.JoinSpace(env.ctx, spaceMember.Id, env.testSpace.Id)
		if err != nil {
			t.Fatalf("Failed to join space: %v", err)
		}

		members, err := env.resolver.Room().Members(env.authContextForUser(spaceMember), env.testRoom)
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("Expected ErrNotRoomMember, got %v", err)
		}
		if members != nil {
			t.Errorf("Expected nil members, got %+v", members)
		}
	})
}

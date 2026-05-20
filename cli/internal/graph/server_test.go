package graph

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

// ============================================================================
// Instance Field Resolver Tests
// ============================================================================

func TestServerResolver_Rooms(t *testing.T) {
	env := setupTestResolver(t)
	instance := &model.Server{}

	t.Run("list rooms (authorized)", func(t *testing.T) {
		rooms, err := env.resolver.Server().Rooms(env.authContext(), instance, nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if len(rooms) == 0 {
			t.Fatal("Expected at least one room")
		}

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

	// Calling Instance.Rooms on the primary space includes the caller's DM
	// conversations so the unified sidebar can render channels and DMs together.
	t.Run("instance rooms include the caller's DM rooms", func(t *testing.T) {
		other := env.createVerifiedUser(t, "dm-peer", "DM Peer", "password123")

		dm, _, err := env.core.FindOrCreateDM(env.ctx, env.testUser.Id, []string{other.Id})
		if err != nil {
			t.Fatalf("Failed to create DM: %v", err)
		}

		if _, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, env.testUser.Id, "hi", nil, "", "", nil, false); err != nil {
			t.Fatalf("Failed to post DM message: %v", err)
		}

		rooms, err := env.resolver.Server().Rooms(env.authContext(), instance, nil)
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
			t.Error("expected channel room to be in instance rooms list")
		}
		if !sawDM {
			t.Error("expected DM room to be merged into instance rooms list")
		}
	})

	t.Run("type:CHANNEL excludes the caller's DMs", func(t *testing.T) {
		other := env.createVerifiedUser(t, "type-channel-peer", "Peer", "password123")
		dm, _, err := env.core.FindOrCreateDM(env.ctx, env.testUser.Id, []string{other.Id})
		if err != nil {
			t.Fatalf("Failed to create DM: %v", err)
		}
		if _, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, env.testUser.Id, "hi", nil, "", "", nil, false); err != nil {
			t.Fatalf("Failed to post DM message: %v", err)
		}

		channelOnly := model.RoomTypeChannel
		rooms, err := env.resolver.Server().Rooms(env.authContext(), instance, &channelOnly)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		for _, r := range rooms {
			if r.SpaceId == core.DMSpaceID {
				t.Errorf("type:CHANNEL should exclude DM rooms, got %+v", r)
			}
		}
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

	t.Run("type:DM returns only the caller's DMs", func(t *testing.T) {
		other := env.createVerifiedUser(t, "type-dm-peer-2", "Peer", "password123")
		dm, _, err := env.core.FindOrCreateDM(env.ctx, env.testUser.Id, []string{other.Id})
		if err != nil {
			t.Fatalf("Failed to create DM: %v", err)
		}
		if _, err := env.core.PostMessage(env.ctx, core.KindDM, dm.Id, env.testUser.Id, "hi", nil, "", "", nil, false); err != nil {
			t.Fatalf("Failed to post DM message: %v", err)
		}

		dmOnly := model.RoomTypeDm
		rooms, err := env.resolver.Server().Rooms(env.authContext(), instance, &dmOnly)
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

// TestServerResolver_Rooms_RoomScopeVisibility covers the per-room
// filtering that makes private channels possible. Visibility follows
// `room.list`: a room-level deny on room.list for `everyone` hides the
// room from regular users; a role with an explicit room-level grant
// on room.list restores visibility.
func TestServerResolver_Rooms_RoomScopeVisibility(t *testing.T) {
	env := setupTestResolver(t)
	instance := &model.Server{}

	// Create a "private" room and deny everyone room.list on it.
	privateRoom, err := env.core.CreateRoom(env.ctx, env.testUser.Id, core.KindChannel, "", "eng-secret", "Engineering Secret")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := env.core.DenyRoomPermission(env.ctx, privateRoom.Id, core.RoleEveryone, core.PermRoomList); err != nil {
		t.Fatalf("DenyRoomPermission: %v", err)
	}

	regular := env.createVerifiedUser(t, "regular-rooms-vis", "Regular", "password123")

	t.Run("private room hidden from regular user", func(t *testing.T) {
		rooms, err := env.resolver.Server().Rooms(env.authContextForUser(regular), instance, nil)
		if err != nil {
			t.Fatalf("Server.Rooms: %v", err)
		}
		for _, r := range rooms {
			if r.Id == privateRoom.Id {
				t.Errorf("expected private room to be filtered out, but it was returned")
			}
		}
	})

	t.Run("public room still visible to regular user", func(t *testing.T) {
		rooms, err := env.resolver.Server().Rooms(env.authContextForUser(regular), instance, nil)
		if err != nil {
			t.Fatalf("Server.Rooms: %v", err)
		}
		var sawPublic bool
		for _, r := range rooms {
			if r.Id == env.testRoom.Id {
				sawPublic = true
				break
			}
		}
		if !sawPublic {
			t.Error("expected the seeded test room to remain visible")
		}
	})

	t.Run("explicit role grant restores visibility", func(t *testing.T) {
		// Create an "engineering" role, grant it room.list on the private
		// room. A user with this role should see it in the directory.
		_, err := env.core.CreateServerRole(env.ctx, "engineering", "Engineering", "Eng team")
		if err != nil {
			t.Fatalf("CreateServerRole: %v", err)
		}
		if err := env.core.GrantRoomPermission(env.ctx, privateRoom.Id, "engineering", core.PermRoomList); err != nil {
			t.Fatalf("GrantRoomPermission: %v", err)
		}
		if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, regular.Id, "engineering"); err != nil {
			t.Fatalf("AssignServerRole: %v", err)
		}

		rooms, err := env.resolver.Server().Rooms(env.authContextForUser(regular), instance, nil)
		if err != nil {
			t.Fatalf("Server.Rooms: %v", err)
		}
		var sawPrivate bool
		for _, r := range rooms {
			if r.Id == privateRoom.Id {
				sawPrivate = true
				break
			}
		}
		if !sawPrivate {
			t.Error("expected user with role-level grant to see the private room")
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

		members, err := env.resolver.Room().Members(env.authContextForUser(spaceMember), env.testRoom)
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("Expected ErrNotRoomMember, got %v", err)
		}
		if members != nil {
			t.Errorf("Expected nil members, got %+v", members)
		}
	})
}

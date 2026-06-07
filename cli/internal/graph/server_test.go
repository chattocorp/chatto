package graph

import (
	"errors"
	"testing"
	"time"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

// ============================================================================
// Instance Field Resolver Tests
// ============================================================================

func TestServerResolver_VideoProcessingEnabled(t *testing.T) {
	env := setupTestResolver(t)
	instance := &model.Server{}

	enabled, err := env.resolver.Server().VideoProcessingEnabled(env.ctx, instance)
	if err != nil {
		t.Fatalf("VideoProcessingEnabled returned error: %v", err)
	}
	if enabled {
		t.Fatal("VideoProcessingEnabled default = true, want false")
	}

	env.resolver.videoConfig = config.VideoConfig{Enabled: true}
	enabled, err = env.resolver.Server().VideoProcessingEnabled(env.ctx, instance)
	if err != nil {
		t.Fatalf("VideoProcessingEnabled returned error: %v", err)
	}
	if !enabled {
		t.Fatal("VideoProcessingEnabled after enabling = false, want true")
	}
}

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
			if r.Id == dm.Id && core.KindOfRoom(r) == core.KindDM {
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
			if core.KindOfRoom(r) == core.KindDM {
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
			if core.KindOfRoom(r) != core.KindDM {
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

func TestServerResolver_Members(t *testing.T) {
	env := setupTestResolver(t)
	server := &model.Server{}

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		members, err := env.resolver.Server().Members(env.unauthContext(), server, nil, nil, nil)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}
		if members != nil {
			t.Errorf("Expected nil members, got %+v", members)
		}
	})

	t.Run("authenticated user can search and paginate members", func(t *testing.T) {
		regularUser := env.createVerifiedUser(t, "server-member-browser", "Server Member Browser", "password123")

		for _, fixture := range []struct {
			login       string
			displayName string
		}{
			{login: "server-members-page-alpha", displayName: "Server Members Page Target Alpha"},
			{login: "server-members-page-beta", displayName: "Server Members Page Target Beta"},
			{login: "server-members-page-gamma", displayName: "Server Members Page Target Gamma"},
			{login: "server-members-page-other", displayName: "Unrelated User"},
		} {
			env.createVerifiedUser(t, fixture.login, fixture.displayName, "password123")
		}

		search := "server members page target"
		limit := int32(1)
		offset := int32(1)
		members, err := env.resolver.Server().Members(env.authContextForUser(regularUser), server, &search, &limit, &offset)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if members == nil {
			t.Fatal("Expected members connection, got nil")
		}
		if members.TotalCount != 3 {
			t.Errorf("Expected totalCount 3, got %d", members.TotalCount)
		}
		if !members.HasMore {
			t.Error("Expected hasMore for middle page")
		}
		if len(members.Users) != 1 {
			t.Fatalf("Expected 1 user in page, got %d", len(members.Users))
		}
		if members.Users[0].Login == "server-members-page-other" {
			t.Error("Expected search filter to exclude unrelated user")
		}

		offset = 2
		limit = 2
		tail, err := env.resolver.Server().Members(env.authContextForUser(regularUser), server, &search, &limit, &offset)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if tail.TotalCount != 3 {
			t.Errorf("Expected tail totalCount 3, got %d", tail.TotalCount)
		}
		if tail.HasMore {
			t.Error("Expected no more members beyond tail page")
		}
		if len(tail.Users) != 1 {
			t.Fatalf("Expected 1 user in tail page, got %d", len(tail.Users))
		}
	})
}

// ============================================================================
// Room Field Resolver Tests
// ============================================================================

func TestRoomResolver_Members(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("room member can list members", func(t *testing.T) {
		members, err := env.resolver.Room().Members(env.authContext(), env.testRoom, nil, nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if members == nil {
			t.Fatal("Expected members, got nil")
		}
		if len(members.Users) == 0 {
			t.Error("Expected at least one member")
		}
	})

	t.Run("room member pagination is sorted and reports metadata", func(t *testing.T) {
		for _, fixture := range []struct {
			login       string
			displayName string
		}{
			{login: "member-page-charlie", displayName: "Charlie Member"},
			{login: "member-page-alice", displayName: "Alice Member"},
			{login: "member-page-bob", displayName: "Bob Member"},
		} {
			member, err := env.core.CreateUser(env.ctx, "system", fixture.login, fixture.displayName, "password123")
			if err != nil {
				t.Fatalf("Failed to create user %s: %v", fixture.login, err)
			}
			if _, err := env.core.JoinRoom(env.ctx, member.Id, core.KindChannel, member.Id, env.testRoom.Id); err != nil {
				t.Fatalf("Failed to join user %s: %v", fixture.login, err)
			}
		}

		deadline := time.Now().Add(2 * time.Second)
		for {
			memberships, err := env.core.GetRoomMembersList(env.ctx, core.KindChannel, env.testRoom.Id)
			if err != nil {
				t.Fatalf("Failed to get room members: %v", err)
			}
			if len(memberships) == 4 {
				break
			}
			if time.Now().After(deadline) {
				t.Fatalf("Timed out waiting for room membership projection, got %d members", len(memberships))
			}
			time.Sleep(10 * time.Millisecond)
		}

		limit := int32(2)
		offset := int32(1)
		members, err := env.resolver.Room().Members(env.authContext(), env.testRoom, &limit, &offset)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if members.TotalCount != 4 {
			t.Errorf("Expected totalCount 4, got %d", members.TotalCount)
		}
		if !members.HasMore {
			t.Error("Expected hasMore for middle member page")
		}
		if len(members.Users) != 2 {
			t.Fatalf("Expected 2 members in page, got %d", len(members.Users))
		}

		wantLogins := []string{"member-page-bob", "member-page-charlie"}
		for i, want := range wantLogins {
			if members.Users[i].Login != want {
				t.Errorf("Expected member %d login %q, got %q", i, want, members.Users[i].Login)
			}
		}
	})

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		members, err := env.resolver.Room().Members(env.unauthContext(), env.testRoom, nil, nil)
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

		members, err := env.resolver.Room().Members(env.authContextForUser(outsider), env.testRoom, nil, nil)
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

		members, err := env.resolver.Room().Members(env.authContextForUser(spaceMember), env.testRoom, nil, nil)
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("Expected ErrNotRoomMember, got %v", err)
		}
		if members != nil {
			t.Errorf("Expected nil members, got %+v", members)
		}
	})
}

package graph

import (
	"context"
	"errors"
	"strconv"
	"testing"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core"
)

// ============================================================================
// Room Query Resolver Tests
// ============================================================================

func TestQueryResolver_Room(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("get existing room as member", func(t *testing.T) {
		room, err := env.resolver.Query().Room(env.authContext(), env.testRoom.Id)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if room == nil {
			t.Fatal("Expected room, got nil")
		}

		if room.Id != env.testRoom.Id {
			t.Errorf("Expected room ID %s, got %s", env.testRoom.Id, room.Id)
		}

		if room.Name != env.testRoom.Name {
			t.Errorf("Expected room name %s, got %s", env.testRoom.Name, room.Name)
		}
	})

	t.Run("get non-existent room", func(t *testing.T) {
		room, err := env.resolver.Query().Room(env.authContext(), "nonexistent")
		if err == nil {
			t.Fatal("Expected error for non-existent room")
		}

		if room != nil {
			t.Errorf("Expected nil room, got %+v", room)
		}
	})

	t.Run("get room without authentication", func(t *testing.T) {
		room, err := env.resolver.Query().Room(env.unauthContext(), env.testRoom.Id)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}

		if room != nil {
			t.Errorf("Expected nil room, got %+v", room)
		}
	})

	t.Run("get room without membership", func(t *testing.T) {
		// Create another user who is not a member of the test room
		otherUser, err := env.core.CreateUser(env.ctx, "system", "otheruser", "otheruser", "password456")
		if err != nil {
			t.Fatalf("Failed to create other user: %v", err)
		}

		// Other user joins the space but NOT the room

		// Try to query the room as the other user (who is not a room member)
		room, err := env.resolver.Query().Room(env.authContextForUser(otherUser), env.testRoom.Id)
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("Expected ErrNotRoomMember, got %v", err)
		}

		if room != nil {
			t.Errorf("Expected nil room, got %+v", room)
		}
	})
}

// Space Query/discovery resolvers were retired in PR(a); the type is gone from
// the GraphQL surface. Public discovery now happens via the unauthenticated
// `instance` query, which exposes the server name, logo, banner, etc.

// ============================================================================
// User Query Resolver Tests
// ============================================================================

func TestViewerResolver_User(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("authenticated user", func(t *testing.T) {
		viewer, err := env.resolver.Query().Viewer(env.authContext())
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if viewer == nil {
			t.Fatal("Expected viewer, got nil")
		}

		user, err := env.resolver.Viewer().User(env.authContext(), viewer)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if user == nil {
			t.Fatal("Expected user, got nil")
		}
		if user.Id != env.testUser.Id {
			t.Errorf("Expected user ID %s, got %s", env.testUser.Id, user.Id)
		}
		if user.Login != env.testUser.Login {
			t.Errorf("Expected login %s, got %s", env.testUser.Login, user.Login)
		}
	})

	t.Run("unauthenticated user has no viewer", func(t *testing.T) {
		viewer, err := env.resolver.Query().Viewer(env.unauthContext())
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if viewer != nil {
			t.Errorf("Expected nil viewer, got %+v", viewer)
		}
	})
}

func TestQueryResolver_User(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("get existing user", func(t *testing.T) {
		user, err := env.resolver.Query().User(env.ctx, env.testUser.Id)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		if user == nil {
			t.Fatal("Expected user, got nil")
		}

		if user.Id != env.testUser.Id {
			t.Errorf("Expected user ID %s, got %s", env.testUser.Id, user.Id)
		}
	})

	t.Run("get non-existent user", func(t *testing.T) {
		user, err := env.resolver.Query().User(env.ctx, "nonexistent")
		if err == nil {
			t.Fatal("Expected error for non-existent user")
		}

		if user != nil {
			t.Errorf("Expected nil user, got %+v", user)
		}
	})
}

// ============================================================================
// Users Query Resolver Tests (Admin Only)
// ============================================================================

func TestQueryResolver_Users(t *testing.T) {
	env := setupTestResolver(t)

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		users, err := env.resolver.Query().Users(env.unauthContext(), nil, nil, nil)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}
		if users != nil {
			t.Errorf("Expected nil users, got %+v", users)
		}
	})

	t.Run("user without admin.users.view permission is rejected", func(t *testing.T) {
		// Create a second user who is NOT an admin (the first user from setupTestResolver
		// is auto-promoted to server owner, so we need a fresh user)
		regularUser, err := env.core.CreateUser(env.ctx, "system", "regularuser", "Regular User", "password123")
		if err != nil {
			t.Fatalf("Failed to create regular user: %v", err)
		}

		users, err := env.resolver.Query().Users(env.authContextForUser(regularUser), nil, nil, nil)
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("Expected core.ErrPermissionDenied, got %v", err)
		}
		if users != nil {
			t.Errorf("Expected nil users, got %+v", users)
		}
	})

	t.Run("admin can search and paginate users", func(t *testing.T) {
		for _, fixture := range []struct {
			login       string
			displayName string
		}{
			{login: "users-page-alpha", displayName: "Users Page Target Alpha"},
			{login: "users-page-beta", displayName: "Users Page Target Beta"},
			{login: "users-page-gamma", displayName: "Users Page Target Gamma"},
			{login: "users-page-other", displayName: "Unrelated User"},
		} {
			if _, err := env.core.CreateUser(env.ctx, "system", fixture.login, fixture.displayName, "password123"); err != nil {
				t.Fatalf("Failed to create user %s: %v", fixture.login, err)
			}
		}

		search := "users page target"
		limit := int32(1)
		offset := int32(1)
		users, err := env.resolver.Query().Users(env.authContext(), &search, &limit, &offset)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if users == nil {
			t.Fatal("Expected users connection, got nil")
		}
		if users.TotalCount != 3 {
			t.Errorf("Expected totalCount 3, got %d", users.TotalCount)
		}
		if !users.HasMore {
			t.Error("Expected hasMore for middle page")
		}
		if len(users.Users) != 1 {
			t.Fatalf("Expected 1 user in page, got %d", len(users.Users))
		}
		if users.Users[0].Login == "users-page-other" {
			t.Error("Expected search filter to exclude unrelated user")
		}

		offset = 2
		limit = 2
		tail, err := env.resolver.Query().Users(env.authContext(), &search, &limit, &offset)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if tail.TotalCount != 3 {
			t.Errorf("Expected tail totalCount 3, got %d", tail.TotalCount)
		}
		if tail.HasMore {
			t.Error("Expected no more users beyond tail page")
		}
		if len(tail.Users) != 1 {
			t.Fatalf("Expected 1 user in tail page, got %d", len(tail.Users))
		}
	})
}

// ============================================================================
// RoomEvents Query Resolver Tests
// ============================================================================

func TestQueryResolver_RoomEvents(t *testing.T) {
	env := setupTestResolver(t)

	// Post a message to have some events
	_, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "Test message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		events, err := env.resolver.Room().Events(env.unauthContext(), env.testRoom, nil, nil, nil)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}
		if events != nil {
			t.Errorf("Expected nil events, got %+v", events)
		}
	})

	t.Run("non-room-member is rejected", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-events", "Outsider", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		events, err := env.resolver.Room().Events(env.authContextForUser(outsider), env.testRoom, nil, nil, nil)
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("Expected ErrNotRoomMember, got %v", err)
		}
		if events != nil {
			t.Errorf("Expected nil events, got %+v", events)
		}
	})

	t.Run("space member but not room member is rejected", func(t *testing.T) {
		spaceMember, err := env.core.CreateUser(env.ctx, "system", "spacemember-events", "Space Member", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		events, err := env.resolver.Room().Events(env.authContextForUser(spaceMember), env.testRoom, nil, nil, nil)
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("Expected ErrNotRoomMember, got %v", err)
		}
		if events != nil {
			t.Errorf("Expected nil events, got %+v", events)
		}
	})

	t.Run("room member can fetch events", func(t *testing.T) {
		result, err := env.resolver.Room().Events(env.authContext(), env.testRoom, nil, nil, nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("Expected result, got nil")
		}
		// Should have at least the message we posted
		if len(result.Events) == 0 {
			t.Error("Expected at least one event")
		}
	})
}

// ============================================================================
// RoomEventsAround Query Resolver Tests
// ============================================================================

func TestQueryResolver_RoomEventsAround(t *testing.T) {
	env := setupTestResolver(t)

	// Post 20 messages to have a range of events
	var eventIDs []string
	for i := 0; i < 20; i++ {
		event, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id,
			"Around msg "+strconv.Itoa(i), nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post message %d: %v", i, err)
		}
		eventIDs = append(eventIDs, event.Id)
	}

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		result, err := env.resolver.Room().EventsAround(env.unauthContext(), env.testRoom, eventIDs[10], nil)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}
		if result != nil {
			t.Errorf("Expected nil result, got %+v", result)
		}
	})

	t.Run("non-room-member is rejected", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-around", "Outsider", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		result, err := env.resolver.Room().EventsAround(env.authContextForUser(outsider), env.testRoom, eventIDs[10], nil)
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("Expected ErrNotRoomMember, got %v", err)
		}
		if result != nil {
			t.Errorf("Expected nil result, got %+v", result)
		}
	})

	t.Run("returns events centered around target", func(t *testing.T) {
		limit := int32(10)
		result, err := env.resolver.Room().EventsAround(env.authContext(), env.testRoom, eventIDs[10], &limit)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("Expected result, got nil")
		}

		// Should contain events
		if len(result.Events) == 0 {
			t.Fatal("Expected events in result")
		}

		// Target index should be valid
		if result.TargetIndex < 0 || int(result.TargetIndex) >= len(result.Events) {
			t.Errorf("Target index %d out of range [0, %d)", result.TargetIndex, len(result.Events))
		}

		// The event at TargetIndex should be the one we asked for
		targetEvent := result.Events[result.TargetIndex]
		if targetEvent.ID() != eventIDs[10] {
			t.Errorf("Expected target event ID %s at index %d, got %s", eventIDs[10], result.TargetIndex, targetEvent.ID())
		}
	})

	t.Run("target at beginning has no older events", func(t *testing.T) {
		limit := int32(10)
		result, err := env.resolver.Room().EventsAround(env.authContext(), env.testRoom, eventIDs[0], &limit)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("Expected result, got nil")
		}

		// First message in the room — hasOlder should be false
		if result.HasOlder {
			t.Error("Expected HasOlder=false for first event in room")
		}
	})

	t.Run("target at end has no newer events", func(t *testing.T) {
		limit := int32(10)
		result, err := env.resolver.Room().EventsAround(env.authContext(), env.testRoom, eventIDs[19], &limit)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("Expected result, got nil")
		}

		// Last message in the room — hasNewer should be false
		if result.HasNewer {
			t.Error("Expected HasNewer=false for last event in room")
		}
	})

	t.Run("nonexistent event returns error", func(t *testing.T) {
		result, err := env.resolver.Room().EventsAround(env.authContext(), env.testRoom, "nonexistent-event-id", nil)
		if err == nil {
			t.Fatal("Expected error for nonexistent event")
		}
		if result != nil {
			t.Errorf("Expected nil result, got %+v", result)
		}
	})

	t.Run("default limit returns results", func(t *testing.T) {
		// nil limit should use default
		result, err := env.resolver.Room().EventsAround(env.authContext(), env.testRoom, eventIDs[10], nil)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("Expected result, got nil")
		}
		if len(result.Events) == 0 {
			t.Error("Expected events with default limit")
		}
	})
}

// ============================================================================
// RoomEvents Forward Pagination Tests (after parameter)
// ============================================================================

func TestQueryResolver_RoomEventsForward(t *testing.T) {
	env := setupTestResolver(t)

	// Post 10 messages
	for i := 0; i < 10; i++ {
		_, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id,
			"Forward msg "+strconv.Itoa(i), nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("Failed to post message %d: %v", i, err)
		}
	}

	t.Run("forward pagination returns events after cursor", func(t *testing.T) {
		// First fetch a page so we know the cursor of an event in the middle.
		allResult, err := env.resolver.Room().Events(env.authContext(), env.testRoom, nil, nil, nil)
		if err != nil {
			t.Fatalf("Failed to get events: %v", err)
		}
		if len(allResult.Events) < 5 {
			t.Fatalf("Expected at least 5 events, got %d", len(allResult.Events))
		}

		// Use the 5th event's ID to find its cursor by re-fetching just that
		// event-and-onward via roomEventsAround. Simpler: take the cursor
		// from the previous response — startCursor is the cursor of the
		// first event, so paginate forward from there.
		if allResult.StartCursor == nil {
			t.Fatal("Expected startCursor on the initial page")
		}
		afterCursor := *allResult.StartCursor

		limit := int32(50)
		forwardResult, err := env.resolver.Room().Events(env.authContext(), env.testRoom, &limit, nil, &afterCursor)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Should return events after the cursor (the cursor event itself excluded).
		if len(forwardResult.Events) == 0 {
			t.Fatal("Expected events after cursor")
		}
		// First event in the forward page must be different from the cursor's event.
		if forwardResult.Events[0].ID() == allResult.Events[0].ID() {
			t.Errorf("Forward pagination returned the cursor event itself")
		}
	})
}

// ============================================================================
// RoomEvent Query Resolver Tests
// ============================================================================

func TestQueryResolver_RoomEventByEventID(t *testing.T) {
	env := setupTestResolver(t)

	// Post a message to have an event
	event, err := env.core.PostMessage(env.ctx, core.KindChannel, env.testRoom.Id, env.testUser.Id, "Test message for single event", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("Failed to post message: %v", err)
	}

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		result, err := env.resolver.Room().Event(env.unauthContext(), env.testRoom, event.Id)
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("Expected ErrNotAuthenticated, got %v", err)
		}
		if result != nil {
			t.Errorf("Expected nil event, got %+v", result)
		}
	})

	t.Run("non-room-member is rejected", func(t *testing.T) {
		outsider, err := env.core.CreateUser(env.ctx, "system", "outsider-event", "Outsider", "password123")
		if err != nil {
			t.Fatalf("Failed to create user: %v", err)
		}

		result, err := env.resolver.Room().Event(env.authContextForUser(outsider), env.testRoom, event.Id)
		if !errors.Is(err, ErrNotRoomMember) {
			t.Errorf("Expected ErrNotRoomMember, got %v", err)
		}
		if result != nil {
			t.Errorf("Expected nil event, got %+v", result)
		}
	})

	t.Run("room member can fetch single event", func(t *testing.T) {
		result, err := env.resolver.Room().Event(env.authContext(), env.testRoom, event.Id)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if result == nil {
			t.Fatal("Expected event, got nil")
		}
		if result.ID() != event.Id {
			t.Errorf("Expected event ID %s, got %s", event.Id, result.ID())
		}
	})
}

// ============================================================================
// Admin Query Resolver Tests
// ============================================================================

func TestQueryResolver_Viewer(t *testing.T) {
	t.Run("unauthenticated returns nil", func(t *testing.T) {
		env := setupTestResolver(t)
		viewer, err := env.resolver.Query().Viewer(env.unauthContext())
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if viewer != nil {
			t.Error("Expected nil viewer for unauthenticated user")
		}
	})

	t.Run("non-admin viewer canViewAdmin is false", func(t *testing.T) {
		env := setupTestResolver(t)
		// Create a second user who is NOT an admin (the first user from setupTestResolver
		// is auto-promoted to server owner, so we need a fresh user)
		regularUser, err := env.core.CreateUser(env.ctx, "system", "regularuser", "Regular User", "password123")
		if err != nil {
			t.Fatalf("Failed to create regular user: %v", err)
		}

		viewer, err := env.resolver.Query().Viewer(env.authContextForUser(regularUser))
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if viewer == nil {
			t.Fatal("Expected non-nil viewer for authenticated user")
		}

		canViewAdmin, err := env.resolver.Viewer().CanViewAdmin(env.authContextForUser(regularUser), viewer)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if canViewAdmin {
			t.Error("Expected canViewAdmin=false for non-admin user")
		}
	})

	t.Run("config admin viewer canViewAdmin is true", func(t *testing.T) {
		// Use the verified email for the test user (created in createTestData)
		env := setupTestResolverWithAdmin(t, []string{"testuser@example.com"})

		viewer, err := env.resolver.Query().Viewer(env.authContext())
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if viewer == nil {
			t.Fatal("Expected non-nil viewer for authenticated user")
		}

		canViewAdmin, err := env.resolver.Viewer().CanViewAdmin(env.authContext(), viewer)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !canViewAdmin {
			t.Error("Expected canViewAdmin=true for config admin user")
		}
	})
}

func TestQueryResolver_Admin(t *testing.T) {
	t.Run("unauthenticated returns nil", func(t *testing.T) {
		env := setupTestResolver(t)
		admin, err := env.resolver.Query().Admin(env.unauthContext())
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if admin != nil {
			t.Error("Expected nil for unauthenticated user")
		}
	})

	t.Run("non-admin returns nil", func(t *testing.T) {
		env := setupTestResolver(t)
		// Create a second user who is NOT an admin (the first user from setupTestResolver
		// is auto-promoted to server owner, so we need a fresh user)
		regularUser, err := env.core.CreateUser(env.ctx, "system", "regularuser", "Regular User", "password123")
		if err != nil {
			t.Fatalf("Failed to create regular user: %v", err)
		}

		admin, err := env.resolver.Query().Admin(env.authContextForUser(regularUser))
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if admin != nil {
			t.Error("Expected nil for non-admin user")
		}
	})

	t.Run("admin returns AdminQueries", func(t *testing.T) {
		// Use the verified email for the test user (created in createTestData)
		env := setupTestResolverWithAdmin(t, []string{"testuser@example.com"})
		admin, err := env.resolver.Query().Admin(env.authContext())
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if admin == nil {
			t.Fatal("Expected AdminQueries for admin user, got nil")
		}
		// Verify it contains system info
		if admin.SystemInfo == nil {
			t.Error("Expected SystemInfo to be populated")
		}
	})
}

// ============================================================================
// Instance Query Resolver Tests
// ============================================================================

func TestQueryResolver_Server(t *testing.T) {
	t.Run("returns instance with version", func(t *testing.T) {
		resolver := &Resolver{
			version:    "1.0.0",
			authConfig: config.AuthConfig{},
		}

		instance, err := resolver.Query().Server(context.Background())
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if instance.Version != "1.0.0" {
			t.Errorf("Expected version '1.0.0', got %q", instance.Version)
		}
	})

	t.Run("returns empty auth providers when none configured", func(t *testing.T) {
		resolver := &Resolver{
			version:    "1.0.0",
			authConfig: config.AuthConfig{},
		}

		instance, err := resolver.Query().Server(context.Background())
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if len(instance.EnabledAuthProviders) != 0 {
			t.Errorf("Expected 0 providers, got %d", len(instance.EnabledAuthProviders))
		}
	})

	t.Run("returns nil welcome message when core not initialized", func(t *testing.T) {
		// Without a core, the welcome message should be nil
		resolver := &Resolver{
			version:    "1.0.0",
			authConfig: config.AuthConfig{},
		}

		instance, err := resolver.Query().Server(context.Background())
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}

		// Get the config from the instance
		configResolver := resolver.ServerConfig()
		serverConfig, err := resolver.Server().Config(context.Background(), instance)
		if err != nil {
			t.Fatalf("Unexpected error getting config: %v", err)
		}

		// Check welcome message is nil when core is not initialized
		welcomeMsg, err := configResolver.WelcomeMessage(context.Background(), serverConfig)
		if err != nil {
			t.Fatalf("Unexpected error getting welcome message: %v", err)
		}
		if welcomeMsg != nil {
			t.Errorf("Expected nil, got %q", *welcomeMsg)
		}
	})

	t.Run("works without authentication", func(t *testing.T) {
		// Instance should work for unauthenticated users (login page)
		resolver := &Resolver{
			version:    "1.0.0",
			authConfig: config.AuthConfig{},
		}

		// Use empty context (no auth)
		instance, err := resolver.Query().Server(context.Background())
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if instance.Version != "1.0.0" {
			t.Error("Expected version to be returned for unauthenticated request")
		}
	})
}

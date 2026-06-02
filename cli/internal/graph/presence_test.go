package graph

import (
	"testing"

	"hmans.de/chatto/internal/graph/model"
)

// ============================================================================
// User PresenceStatus Field Resolver Tests
// ============================================================================

func TestUserResolver_PresenceStatus(t *testing.T) {
	env := setupTestResolver(t)
	userResolver := env.resolver.User()

	t.Run("user with no presence returns offline", func(t *testing.T) {
		// Create a fresh user who has no presence set
		freshUser, err := env.core.CreateUser(env.ctx, "system", "fresh-presence", "Fresh Presence", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		status, err := userResolver.PresenceStatus(env.ctx, freshUser)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if status != model.PresenceStatusOffline {
			t.Errorf("expected Offline, got %s", status)
		}
	})

	t.Run("user with online presence returns online", func(t *testing.T) {
		onlineUser, err := env.core.CreateUser(env.ctx, "system", "online-presence", "Online Presence", "password123")
		if err != nil {
			t.Fatalf("failed to create user: %v", err)
		}

		// Set presence directly
		err = env.core.SetPresence(env.ctx, onlineUser.Id, "ONLINE")
		if err != nil {
			t.Fatalf("failed to set presence: %v", err)
		}

		status, err := userResolver.PresenceStatus(env.ctx, onlineUser)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if status != model.PresenceStatusOnline {
			t.Errorf("expected Online, got %s", status)
		}
	})

	t.Run("querying own presence works", func(t *testing.T) {
		// Set presence directly
		err := env.core.SetPresence(env.ctx, env.testUser.Id, "ONLINE")
		if err != nil {
			t.Fatalf("failed to set presence: %v", err)
		}

		// Query own presence
		status, err := userResolver.PresenceStatus(env.authContext(), env.testUser)
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if status != model.PresenceStatusOnline {
			t.Errorf("expected Online, got %s", status)
		}
	})
}

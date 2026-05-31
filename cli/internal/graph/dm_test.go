package graph

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/graph/model"
)

// ============================================================================
// StartDM Mutation Tests
// ============================================================================

func TestMutationResolver_StartDM(t *testing.T) {
	env := setupTestResolver(t)
	mutation := env.resolver.Mutation()

	// Create a second user for DM
	user2, err := env.core.CreateUser(env.ctx, "system", "dmuser2", "DM User 2", "password123")
	if err != nil {
		t.Fatalf("failed to create user2: %v", err)
	}

	t.Run("unauthenticated user is rejected", func(t *testing.T) {
		_, err := mutation.StartDm(env.unauthContext(), model.StartDMInput{ParticipantIds: []string{user2.Id}})
		if !errors.Is(err, ErrNotAuthenticated) {
			t.Errorf("expected ErrNotAuthenticated, got %v", err)
		}
	})

	t.Run("authenticated user can start DM", func(t *testing.T) {
		room, err := mutation.StartDm(env.authContext(), model.StartDMInput{ParticipantIds: []string{user2.Id}})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if room == nil {
			t.Fatal("expected room, got nil")
		}
		if room.Id == "" {
			t.Error("expected non-empty room ID")
		}
	})

	t.Run("starting DM with same participant returns same room", func(t *testing.T) {
		// First DM
		room1, err := mutation.StartDm(env.authContext(), model.StartDMInput{ParticipantIds: []string{user2.Id}})
		if err != nil {
			t.Fatalf("first StartDm failed: %v", err)
		}

		// Second DM with same participant
		room2, err := mutation.StartDm(env.authContext(), model.StartDMInput{ParticipantIds: []string{user2.Id}})
		if err != nil {
			t.Fatalf("second StartDm failed: %v", err)
		}

		if room1.Id != room2.Id {
			t.Errorf("expected same room ID, got %s and %s", room1.Id, room2.Id)
		}
	})

	t.Run("starting DM with multiple participants", func(t *testing.T) {
		user3, err := env.core.CreateUser(env.ctx, "system", "dmuser3", "DM User 3", "password123")
		if err != nil {
			t.Fatalf("failed to create user3: %v", err)
		}

		room, err := mutation.StartDm(env.authContext(), model.StartDMInput{ParticipantIds: []string{user2.Id, user3.Id}})
		if err != nil {
			t.Fatalf("expected success, got error: %v", err)
		}
		if room == nil {
			t.Fatal("expected room, got nil")
		}
	})

	t.Run("user with denied message.post permission cannot start DM", func(t *testing.T) {
		blockedUser, err := env.core.CreateUser(env.ctx, "system", "dm-blocked", "Blocked DM", "password123")
		if err != nil {
			t.Fatalf("failed to create blocked user: %v", err)
		}

		// Create a restriction role, deny message.post on it, and assign to user
		if _, err := env.core.CreateServerRole(env.ctx, "dmblocked", "DM Blocked", ""); err != nil {
			t.Fatalf("failed to create role: %v", err)
		}
		if err := env.core.DenyServerPermission(env.ctx, "dmblocked", core.PermMessagePost); err != nil {
			t.Fatalf("failed to deny permission: %v", err)
		}
		if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, blockedUser.Id, "dmblocked"); err != nil {
			t.Fatalf("failed to assign role: %v", err)
		}

		_, err = mutation.StartDm(env.authContextForUser(blockedUser), model.StartDMInput{ParticipantIds: []string{user2.Id}})
		if !errors.Is(err, core.ErrPermissionDenied) {
			t.Errorf("expected ErrPermissionDenied, got %v", err)
		}
	})

	// Note: Starting DM with nonexistent user currently succeeds (no validation on participant IDs).
	// This could be considered a feature (lazy validation) or a bug. Not testing this case
	// until the expected behavior is clarified.
}

package core

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/config"
)

func TestCreateUser_RespectsMaxUsersLimit(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create + verify the first user before applying the limit.
	u1, err := core.CreateUser(ctx, "system", "signup-user-1", "U1", "password123")
	if err != nil {
		t.Fatalf("create u1: %v", err)
	}
	if err := core.AddVerifiedEmailDirect(ctx, u1.Id, "u1@example.com"); err != nil {
		t.Fatalf("verify u1: %v", err)
	}

	// Now lock the door at 1 verified user.
	one := 1
	core.config.Limits = config.LimitsConfig{MaxUsers: &one}

	if _, err := core.CreateUser(ctx, "system", "signup-user-2", "U2", "password123"); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("signup should be blocked when at verified-user limit, got %v", err)
	}
}

func TestCountSpacesAndUsers(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Baseline includes the system DM space (auto-created on init).
	baselineSpaces, _ := core.CountSpaces(ctx)
	baselineUsers, _ := core.CountVerifiedUsers(ctx)

	u, _ := core.CreateUser(ctx, "system", "count-user", "Count", "password123")
	if _, err := core.CreateSpace(ctx, u.Id, "S1", ""); err != nil {
		t.Fatalf("create space: %v", err)
	}
	if err := core.AddVerifiedEmailDirect(ctx, u.Id, "count@example.com"); err != nil {
		t.Fatalf("verify email: %v", err)
	}

	if got, _ := core.CountSpaces(ctx); got != baselineSpaces+1 {
		t.Errorf("CountSpaces = %d, want %d", got, baselineSpaces+1)
	}
	if got, _ := core.CountVerifiedUsers(ctx); got != baselineUsers+1 {
		t.Errorf("CountVerifiedUsers = %d, want %d", got, baselineUsers+1)
	}
}

package core

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/config"
)

func TestCreateSpace_RespectsMaxSpacesLimit(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Baseline includes the system DM space; the limit is checked against the raw count,
	// so we offset by it.
	baseline, _ := core.CountSpaces(ctx)

	user, err := core.CreateUser(ctx, "system", "limit-user", "Limit User", "password123")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	atBaseline := baseline
	core.config.Limits = config.LimitsConfig{MaxSpaces: &atBaseline}
	if _, err := core.CreateSpace(ctx, user.Id, "Locked", ""); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("expected ErrLimitExceeded when at baseline, got %v", err)
	}

	allowTwoMore := baseline + 2
	core.config.Limits = config.LimitsConfig{MaxSpaces: &allowTwoMore}

	if _, err := core.CreateSpace(ctx, user.Id, "First", ""); err != nil {
		t.Fatalf("first space should succeed: %v", err)
	}
	if _, err := core.CreateSpace(ctx, user.Id, "Second", ""); err != nil {
		t.Fatalf("second space should succeed: %v", err)
	}
	if _, err := core.CreateSpace(ctx, user.Id, "Third", ""); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("third space should be blocked, got %v", err)
	}
}

func TestCreateSpace_UnlimitedByDefault(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "unlim-user", "Unlim User", "password123")
	if err != nil {
		t.Fatalf("create user: %v", err)
	}

	for i, name := range []string{"a", "b", "c", "d"} {
		if _, err := core.CreateSpace(ctx, user.Id, name, ""); err != nil {
			t.Fatalf("space %d (%q) should succeed under default unlimited: %v", i, name, err)
		}
	}
}

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

func TestAddVerifiedEmail_RaceSafeGate(t *testing.T) {
	// The verification check is the hard gate against the race where two users
	// signed up while both were under the limit but one would push us over after
	// verification. We simulate this by creating both users under no limit, then
	// applying the limit, then verifying — the second verification must fail.
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	u1, _ := core.CreateUser(ctx, "system", "race-user-1", "U1", "password123")
	u2, _ := core.CreateUser(ctx, "system", "race-user-2", "U2", "password123")

	one := 1
	core.config.Limits = config.LimitsConfig{MaxUsers: &one}

	if err := core.AddVerifiedEmailDirect(ctx, u1.Id, "u1@example.com"); err != nil {
		t.Fatalf("first verification should succeed: %v", err)
	}

	err := core.AddVerifiedEmailDirect(ctx, u2.Id, "u2@example.com")
	if !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("second verification should hit the limit, got %v", err)
	}

	// Claim should have been rolled back so the email is still free.
	claimed, _ := core.IsEmailClaimed(ctx, "u2@example.com")
	if claimed {
		t.Errorf("expected email claim to be rolled back when limit is hit")
	}
}

func TestAddVerifiedEmail_AdditionalEmailNotBlocked(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	one := 1
	core.config.Limits = config.LimitsConfig{MaxUsers: &one}

	u, _ := core.CreateUser(ctx, "system", "multi-email-user", "Multi", "password123")
	if err := core.AddVerifiedEmailDirect(ctx, u.Id, "primary@example.com"); err != nil {
		t.Fatalf("first email should verify: %v", err)
	}

	// User is already verified — adding a second email must NOT trip the limit.
	if err := core.AddVerifiedEmailDirect(ctx, u.Id, "secondary@example.com"); err != nil {
		t.Fatalf("second email on already-verified user should not be blocked: %v", err)
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

package core

import (
	"errors"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

func TestCreateVerifiedUser_HappyPath(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateVerifiedUser(ctx, "system", "happy-user", "Happy", "password123", "happy@example.com")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	verified, _ := core.HasVerifiedEmail(ctx, user.Id)
	if !verified {
		t.Errorf("expected user to have verified email")
	}

	claimed, _ := core.IsEmailClaimed(ctx, "happy@example.com")
	if !claimed {
		t.Errorf("expected email to be claimed")
	}
}

func TestCreateVerifiedUser_RollsBackOnEmailConflict(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Pre-claim the email under another user.
	other, _ := core.CreateUser(ctx, "system", "other-user", "Other", "password123")
	if err := core.AddVerifiedEmailDirect(ctx, other.Id, "shared@example.com"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// Attempt to create a new user with the same email — must fail and roll back.
	_, err := core.CreateVerifiedUser(ctx, "system", "second-user", "Second", "password123", "shared@example.com")
	if err == nil {
		t.Fatalf("expected failure due to email conflict")
	}
	if !errors.Is(err, ErrEmailAlreadyVerified) {
		t.Errorf("expected ErrEmailAlreadyVerified wrapped, got %v", err)
	}

	// User record must NOT exist (rollback).
	if _, err := core.storage.serverKV.Get(ctx, userByLoginKey("second-user")); !errors.Is(err, jetstream.ErrKeyNotFound) {
		t.Errorf("expected login claim to be rolled back, got err=%v", err)
	}
}

func TestCreateVerifiedUser_LoginCanBeReusedAfterRollback(t *testing.T) {
	// After a rollback, the login should be free for someone else (or a retry) to claim.
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	pre, _ := core.CreateUser(ctx, "system", "pre", "Pre", "password123")
	if err := core.AddVerifiedEmailDirect(ctx, pre.Id, "blocked@example.com"); err != nil {
		t.Fatalf("setup: %v", err)
	}

	// First attempt: blocked by email conflict, login "retry-me" gets rolled back.
	if _, err := core.CreateVerifiedUser(ctx, "system", "retry-me", "Retry", "password123", "blocked@example.com"); err == nil {
		t.Fatalf("expected failure")
	}

	// Second attempt: same login, different email — should succeed because the rollback
	// freed up the login claim.
	user, err := core.CreateVerifiedUser(ctx, "system", "retry-me", "Retry", "password123", "fresh@example.com")
	if err != nil {
		t.Fatalf("expected success on retry with fresh email, got %v", err)
	}
	if user.Login != "retry-me" {
		t.Errorf("unexpected login: %s", user.Login)
	}
}

package core

import (
	"errors"
	"testing"
	"time"
)

func TestChattoCore_AuthRevocationCutoffPreservesMax(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "cutoff-max-user", "Cutoff Max User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	authenticatedAt := time.Now().UTC()
	token, err := core.CreateAuthTokenWithSourceAt(ctx, user.Id, "test", authenticatedAt)
	if err != nil {
		t.Fatalf("CreateAuthTokenWithSourceAt: %v", err)
	}

	older := authenticatedAt.Add(time.Second)
	newer := authenticatedAt.Add(time.Minute)
	if err := core.storeAuthRevocationCutoff(ctx, user.Id, "newer", newer); err != nil {
		t.Fatalf("store newer cutoff: %v", err)
	}
	if err := core.storeAuthRevocationCutoff(ctx, user.Id, "older", older); err != nil {
		t.Fatalf("store older cutoff: %v", err)
	}

	cutoff, ok, err := core.authRevocationCutoff(ctx, user.Id)
	if err != nil {
		t.Fatalf("authRevocationCutoff: %v", err)
	}
	if !ok {
		t.Fatal("expected auth revocation cutoff")
	}
	if !cutoff.Equal(newer) {
		t.Fatalf("cutoff = %s, want %s", cutoff, newer)
	}
	if _, err := core.ValidateAuthToken(ctx, token); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("ValidateAuthToken err = %v, want ErrAuthTokenNotFound", err)
	}
}

func TestChattoCore_AuthRevocationRollbackRemovesOnlyItsOwnCutoff(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "cutoff-rollback-user", "Cutoff Rollback User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	time.Sleep(time.Millisecond)

	first, err := core.BeginCredentialRevocation(ctx, user.Id)
	if err != nil {
		t.Fatalf("BeginCredentialRevocation first: %v", err)
	}
	second, err := core.BeginCredentialRevocation(ctx, user.Id)
	if err != nil {
		t.Fatalf("BeginCredentialRevocation second: %v", err)
	}

	first.Rollback(ctx)
	if _, err := core.VerifyPassword(ctx, user.Login, "password123"); err == nil {
		t.Fatal("password should remain revoked while the second cutoff is pending")
	}

	second.Rollback(ctx)
	if verified, err := core.VerifyPassword(ctx, user.Login, "password123"); err != nil {
		t.Fatalf("password should verify after both cutoffs roll back: %v", err)
	} else if verified.Id != user.Id {
		t.Fatalf("verified user ID = %q, want %q", verified.Id, user.Id)
	}
}

func TestChattoCore_AuthRevocationRollbackKeepsCommittedCutoff(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "cutoff-commit-user", "Cutoff Commit User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	time.Sleep(time.Millisecond)

	committed, err := core.BeginCredentialRevocation(ctx, user.Id)
	if err != nil {
		t.Fatalf("BeginCredentialRevocation committed: %v", err)
	}
	failed, err := core.BeginCredentialRevocation(ctx, user.Id)
	if err != nil {
		t.Fatalf("BeginCredentialRevocation failed: %v", err)
	}

	committed.Commit()
	failed.Rollback(ctx)

	if _, err := core.VerifyPassword(ctx, user.Login, "password123"); err == nil {
		t.Fatal("password should remain revoked after committed cutoff survives concurrent rollback")
	}
}

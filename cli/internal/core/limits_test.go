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

	// Now lock the door at 1 verified account.
	one := 1
	core.config.Limits = config.LimitsConfig{MaxUsers: &one}

	if _, err := core.CreateUser(ctx, "system", "signup-user-2", "U2", "password123"); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("signup should be blocked when at verified-account limit, got %v", err)
	}
}

func TestCountVerifiedAccounts(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	baselineUsers, _ := core.CountVerifiedAccounts(ctx)

	emailUser, _ := core.CreateUser(ctx, "system", "count-email-user", "Count Email", "password123")
	if err := core.AddVerifiedEmailDirect(ctx, emailUser.Id, "count@example.com"); err != nil {
		t.Fatalf("verify email: %v", err)
	}

	ssoUser, _ := core.CreateUser(ctx, "system", "count-sso-user", "Count SSO", "password123")
	if err := core.LinkExternalIdentity(ctx, "oidc-main", "oidc", "https://id.example", "subject-1", ssoUser.Id); err != nil {
		t.Fatalf("link external identity: %v", err)
	}

	if err := core.LinkExternalIdentity(ctx, "github-main", "github", "github-main", "subject-2", ssoUser.Id); err != nil {
		t.Fatalf("link second external identity: %v", err)
	}

	if got, _ := core.CountVerifiedAccounts(ctx); got != baselineUsers+2 {
		t.Errorf("CountVerifiedAccounts = %d, want %d", got, baselineUsers+2)
	}
}

func TestVerifiedFactorAddRespectsMaxUsersLimit(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	verified, err := core.CreateUser(ctx, "system", "verified-factor-1", "Verified 1", "password123")
	if err != nil {
		t.Fatalf("create verified: %v", err)
	}
	if err := core.LinkExternalIdentity(ctx, "oidc-main", "oidc", "https://id.example", "subject-1", verified.Id); err != nil {
		t.Fatalf("link verified factor: %v", err)
	}

	unverified, err := core.CreateUser(ctx, "system", "verified-factor-2", "Verified 2", "password123")
	if err != nil {
		t.Fatalf("create unverified: %v", err)
	}

	one := 1
	core.config.Limits = config.LimitsConfig{MaxUsers: &one}

	if err := core.AddVerifiedEmailDirect(ctx, verified.Id, "verified-factor-1@example.com"); err != nil {
		t.Fatalf("adding another factor to counted account should be allowed: %v", err)
	}
	if err := core.LinkExternalIdentity(ctx, "github-main", "github", "github-main", "subject-2", verified.Id); err != nil {
		t.Fatalf("adding another identity to counted account should be allowed: %v", err)
	}
	if err := core.AddVerifiedEmailDirect(ctx, unverified.Id, "verified-factor-2@example.com"); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("adding first email factor past limit error = %v, want ErrLimitExceeded", err)
	}
	if err := core.LinkExternalIdentity(ctx, "discord-main", "discord", "discord-main", "subject-3", unverified.Id); !errors.Is(err, ErrLimitExceeded) {
		t.Fatalf("adding first SSO factor past limit error = %v, want ErrLimitExceeded", err)
	}
}

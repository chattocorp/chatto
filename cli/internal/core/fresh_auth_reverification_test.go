package core

import (
	"errors"
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/config"
)

// A re-verification upgrades a stale cookie session only when the provider
// identity and the presented cookie session belong to the user who started
// it, and the provider signed the user in during the attempt. Another
// session of the same user stays stale.
func TestFreshAuthReverification(t *testing.T) {
	c, _ := setupTestCore(t)
	const issuer = "https://idp.example"
	ctx := testContext(t)

	owner, err := c.CreateUser(ctx, SystemActorID, "reverify-owner", "Owner", "")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	other, err := c.CreateUser(ctx, SystemActorID, "reverify-other", "Other", "")
	if err != nil {
		t.Fatalf("CreateUser other: %v", err)
	}
	for _, link := range []struct{ subject, userID string }{{"subject-owner", owner.Id}, {"subject-other", other.Id}} {
		if err := c.LinkExternalIdentity(ctx, "sso", "oidc", issuer, link.subject, link.userID); err != nil {
			t.Fatalf("LinkExternalIdentity %s: %v", link.subject, err)
		}
	}
	generation, err := c.CurrentAuthGeneration(ctx, owner.Id)
	if err != nil {
		t.Fatalf("CurrentAuthGeneration: %v", err)
	}
	stale := time.Now().Add(-2 * FreshAuthWindow)
	cookie, _, err := c.createCookieSessionForGeneration(ctx, owner.Id, "oidc_login", generation, stale, "external_identity", "oidc_login")
	if err != nil {
		t.Fatalf("createCookieSessionForGeneration: %v", err)
	}
	otherBrowser, _, err := c.createCookieSessionForGeneration(ctx, owner.Id, "oidc_login", generation, stale, "external_identity", "oidc_login")
	if err != nil {
		t.Fatalf("createCookieSessionForGeneration other browser: %v", err)
	}
	requireStale := func(t *testing.T) {
		t.Helper()
		if err := c.RequireFreshAuthForCookieSession(ctx, cookie); !errors.Is(err, ErrFreshAuthRequired) {
			t.Fatalf("RequireFreshAuthForCookieSession = %v, want fresh auth required", err)
		}
	}
	requireStale(t)

	startedAt := time.Now()
	complete := func(t *testing.T, userID, subject string, authTime time.Time, presented string) error {
		t.Helper()
		return c.CompleteFreshAuthReverification(ctx, userID, startedAt, issuer, subject, authTime, presented)
	}

	for _, tc := range []struct {
		name      string
		userID    string
		subject   string
		authTime  time.Time
		presented string
		want      error
	}{
		{"identity of another account", owner.Id, "subject-other", time.Now(), cookie, ErrFreshAuthReverificationMismatch},
		{"unlinked identity", owner.Id, "subject-unknown", time.Now(), cookie, ErrFreshAuthReverificationMismatch},
		{"session of another account", other.Id, "subject-other", time.Now(), cookie, ErrFreshAuthReverificationMismatch},
		{"no browser session", owner.Id, "subject-owner", time.Now(), "", ErrFreshAuthReverificationMismatch},
		{"no start", "", "subject-owner", time.Now(), cookie, ErrFreshAuthReverificationMismatch},
		{"earlier provider sign-in", owner.Id, "subject-owner", time.Now().Add(-time.Hour), cookie, ErrFreshAuthReverificationStale},
		{"missing auth_time", owner.Id, "subject-owner", time.Time{}, cookie, ErrFreshAuthReverificationStale},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := complete(t, tc.userID, tc.subject, tc.authTime, tc.presented); !errors.Is(err, tc.want) {
				t.Fatalf("CompleteFreshAuthReverification = %v, want %v", err, tc.want)
			}
			requireStale(t)
		})
	}

	t.Run("same account", func(t *testing.T) {
		if err := complete(t, owner.Id, "subject-owner", time.Now(), cookie); err != nil {
			t.Fatalf("CompleteFreshAuthReverification: %v", err)
		}
		if err := c.RequireFreshAuthForCookieSession(ctx, cookie); err != nil {
			t.Fatalf("RequireFreshAuthForCookieSession after re-verification: %v", err)
		}
		if err := c.RequireFreshAuthForCookieSession(ctx, otherBrowser); !errors.Is(err, ErrFreshAuthRequired) {
			t.Fatalf("other browser session = %v, want still stale", err)
		}
	})
}

func TestProviderCanReverify(t *testing.T) {
	identities := []ExternalIdentity{
		{ProviderID: "sso", ProviderType: "oidc", Issuer: "https://idp.example"},
		{ProviderID: "gh", ProviderType: "github", Issuer: "gh"},
	}
	for _, tc := range []struct {
		name     string
		provider config.AuthProviderConfig
		want     bool
	}{
		{"linked OIDC provider", config.AuthProviderConfig{ID: "sso", Type: config.AuthProviderTypeOpenIDConnect, IssuerURL: "https://idp.example"}, true},
		{"OIDC provider with the linked issuer", config.AuthProviderConfig{ID: "renamed", Type: config.AuthProviderTypeOpenIDConnect, IssuerURL: "https://idp.example"}, true},
		{"unlinked OIDC provider", config.AuthProviderConfig{ID: "other", Type: config.AuthProviderTypeOpenIDConnect, IssuerURL: "https://other.example"}, false},
		{"linked provider that cannot force a sign-in", config.AuthProviderConfig{ID: "gh", Type: config.AuthProviderTypeGitHub}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ProviderCanReverify(tc.provider, identities); got != tc.want {
				t.Fatalf("ProviderCanReverify = %v, want %v", got, tc.want)
			}
		})
	}
}

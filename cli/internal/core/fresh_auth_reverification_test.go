package core

import (
	"errors"
	"testing"
	"time"
)

// A re-verification upgrades a stale cookie session only when the provider
// identity belongs to the bound account, the provider signed the user in
// during the attempt, and the callback presents the bound cookie session.
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

	complete := func(t *testing.T, subject string, authTime time.Time, presented string) error {
		t.Helper()
		token, err := c.CreateFreshAuthReverificationStart(ctx, "sso", "/chat/-/settings/account", cookie)
		if err != nil {
			t.Fatalf("CreateFreshAuthReverificationStart: %v", err)
		}
		start, err := c.ConsumeFreshAuthReverificationStart(ctx, token)
		if err != nil {
			t.Fatalf("ConsumeFreshAuthReverificationStart: %v", err)
		}
		if _, err := c.ConsumeFreshAuthReverificationStart(ctx, token); !errors.Is(err, ErrExternalIdentityFlowNotFound) {
			t.Fatalf("second consume = %v, want not found", err)
		}
		return c.CompleteFreshAuthReverification(ctx, start, issuer, subject, authTime, presented)
	}

	for _, tc := range []struct {
		name      string
		subject   string
		authTime  time.Time
		presented string
		want      error
	}{
		{"identity of another account", "subject-other", time.Now(), cookie, ErrFreshAuthReverificationMismatch},
		{"unlinked identity", "subject-unknown", time.Now(), cookie, ErrFreshAuthReverificationMismatch},
		{"another browser", "subject-owner", time.Now(), otherBrowser, ErrFreshAuthReverificationMismatch},
		{"no browser session", "subject-owner", time.Now(), "", ErrFreshAuthReverificationMismatch},
		{"earlier provider sign-in", "subject-owner", time.Now().Add(-time.Hour), cookie, ErrFreshAuthReverificationStale},
		{"missing auth_time", "subject-owner", time.Time{}, cookie, ErrFreshAuthReverificationStale},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := complete(t, tc.subject, tc.authTime, tc.presented); !errors.Is(err, tc.want) {
				t.Fatalf("CompleteFreshAuthReverification = %v, want %v", err, tc.want)
			}
			requireStale(t)
		})
	}

	t.Run("same account", func(t *testing.T) {
		if err := complete(t, "subject-owner", time.Now(), cookie); err != nil {
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

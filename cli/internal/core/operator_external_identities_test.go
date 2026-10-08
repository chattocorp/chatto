package core

import (
	"errors"
	"testing"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestOperatorIdentityUnlinkCountsOnlyConfiguredMethods(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := c.CreateUser(ctx, SystemActorID, "configured-methods", "Configured Methods", "password123")
	if err != nil {
		t.Fatal(err)
	}
	current, err := c.LinkExternalIdentityAs(ctx, SystemActorID, "company", "oidc", "https://current.example.com", "subject", user.Id)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.LinkExternalIdentity(ctx, "company", "oidc", "https://stale.example.com", "subject", user.Id); err != nil {
		t.Fatal(err)
	}
	policy := ExternalIdentitySignInPolicy{Issuers: []string{"https://current.example.com"}}
	if err := c.DisconnectExternalIdentityAs(ctx, SystemActorID, user.Id, current.SubjectHash, policy); !errors.Is(err, ErrExternalIdentityLastMethod) {
		t.Fatalf("disabled password plus stale provider allowed unlink: %v", err)
	}
	policy.PasswordLoginEnabled = true
	if err := c.DisconnectExternalIdentityAs(ctx, SystemActorID, user.Id, current.SubjectHash, policy); err != nil {
		t.Fatal(err)
	}
}

func TestOperatorIdentityConcurrentClaimsAndRemovalsAcrossReplicas(t *testing.T) {
	t.Parallel()
	first, nc := setupTestCore(t)
	ctx := testContext(t)
	second, err := NewChattoCore(ctx, nc, first.config)
	if err != nil {
		t.Fatal(err)
	}
	startCoreServices(t, second)
	users := make([]string, 2)
	for i, login := range []string{"claim-first", "claim-second"} {
		user, err := first.CreateUser(ctx, SystemActorID, login, login, "")
		if err != nil {
			t.Fatal(err)
		}
		users[i] = user.Id
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for i, replica := range []*ChattoCore{first, second} {
		go func() {
			<-start
			_, err := replica.LinkExternalIdentityAs(ctx, SystemActorID, "company", "oidc", "https://example.com", "shared", users[i])
			results <- err
		}()
	}
	close(start)
	var successes, conflicts int
	for range 2 {
		select {
		case err := <-results:
			if err == nil {
				successes++
			} else if errors.Is(err, ErrExternalIdentityAlreadyClaimed) {
				conflicts++
			} else {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("claim outcomes = %d successes, %d conflicts", successes, conflicts)
	}
	owner, err := first.GetUserByExternalIdentity(ctx, "https://example.com", "shared")
	if err != nil || owner == nil {
		t.Fatalf("identity owner = %v, %v", owner, err)
	}
	alternate, err := first.LinkExternalIdentityAs(ctx, SystemActorID, "alternate", "oidc", "https://alternate.example.com", "alternate", owner.Id)
	if err != nil {
		t.Fatal(err)
	}
	policy := ExternalIdentitySignInPolicy{Issuers: []string{"https://example.com", "https://alternate.example.com"}}
	start = make(chan struct{})
	for i, replica := range []*ChattoCore{first, second} {
		hash := []string{externalIdentityHash("https://example.com", "shared"), alternate.SubjectHash}[i]
		go func() {
			<-start
			results <- replica.DisconnectExternalIdentityAs(ctx, SystemActorID, owner.Id, hash, policy)
		}()
	}
	close(start)
	successes, conflicts = 0, 0
	for range 2 {
		select {
		case err := <-results:
			if err == nil {
				successes++
			} else if errors.Is(err, ErrExternalIdentityLastMethod) {
				conflicts++
			} else {
				t.Fatal(err)
			}
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("unlink outcomes = %d successes, %d protected", successes, conflicts)
	}
	// A fresh replica cold-replays the facts and retains exactly one method.
	replayed, err := NewChattoCore(ctx, nc, first.config)
	if err != nil {
		t.Fatal(err)
	}
	startCoreServices(t, replayed)
	identities, err := replayed.ExternalIdentitiesForUser(ctx, owner.Id)
	if err != nil || len(identities) != 1 {
		t.Fatalf("replayed identities = %v, %v", identities, err)
	}
}

func TestOperatorIdentityRejectsMissingDeletedAndBotAccounts(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := c.CreateUser(ctx, SystemActorID, "deleted-identity-user", "Deleted User", "")
	if err != nil {
		t.Fatal(err)
	}
	allowBotCreation(t, ctx, c, user.Id)
	bot, err := c.CreateBot(ctx, user.Id, "identity-bot", "Identity Bot")
	if err != nil {
		t.Fatal(err)
	}
	deleted, err := c.CreateUser(ctx, SystemActorID, "deleted-identity-target", "Deleted Target", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.DeleteUser(ctx, deleted.Id, deleted.Id); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		userID string
		want   error
	}{
		{"missing", ErrNotFound}, {deleted.Id, ErrNotFound}, {bot.User.Id, ErrHumanAccountRequired},
	} {
		if _, err := c.LinkExternalIdentityAs(ctx, SystemActorID, "provider", "oidc", "https://example.com", "subject", tc.userID); !errors.Is(err, tc.want) {
			t.Fatalf("link rejected account = %v, want %v", err, tc.want)
		}
		if _, err := c.ExternalIdentitiesForUser(ctx, tc.userID); !errors.Is(err, tc.want) {
			t.Fatalf("list rejected account = %v", err)
		}
		if err := c.DisconnectExternalIdentityAs(ctx, SystemActorID, tc.userID, "hash", ExternalIdentitySignInPolicy{}); !errors.Is(err, tc.want) {
			t.Fatalf("unlink rejected account = %v", err)
		}
	}
}

func TestOperatorIdentityLinkRetryPreservesLegacyMetadata(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := c.CreateUser(ctx, SystemActorID, "legacy-identity", "Legacy Identity", "")
	if err != nil {
		t.Fatal(err)
	}
	hash := externalIdentityHash("https://example.com", "subject")
	event := newEvent(user.Id, &evtv1.Event{Event: &evtv1.Event_UserOidcSubjectLinked{UserOidcSubjectLinked: &evtv1.UserOIDCSubjectLinkedEvent{UserId: user.Id, SubjectHash: hash}}})
	if _, err := c.appendUserEvent(ctx, user.Id, event, "", nil); err != nil {
		t.Fatal(err)
	}
	identity, err := c.LinkExternalIdentityAs(ctx, SystemActorID, "renamed-provider", "oidc", "https://example.com", "subject", user.Id)
	if err != nil || identity.SubjectHash != hash || identity.Subject != "" || identity.ProviderID != "oidc" {
		t.Fatalf("legacy retry = %v, %v", identity, err)
	}
	facts, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.Id).Subject(evtstream.EventUserExternalIdentityLinked))
	if err != nil || len(facts) != 0 {
		t.Fatalf("legacy retry appended facts = %v, %v", facts, err)
	}
}

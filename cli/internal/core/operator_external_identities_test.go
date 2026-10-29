package core

import (
	"errors"
	"testing"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/cli/internal/testutil"
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
	ns, _ := testutil.StartNATS(t)
	ctx := testContext(t)
	first := startReplica(t, ns, testCoreConfig())
	second := startReplica(t, ns, testCoreConfig())
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
	// A stored password must not bypass the cross-replica guard when password
	// login is disabled. Use self attribution, as in the account API.
	if err := first.SetPasswordHash(ctx, owner.Id, "password123"); err != nil {
		t.Fatal(err)
	}
	start = make(chan struct{})
	for i, replica := range []*ChattoCore{first, second} {
		hash := []string{externalIdentityHash("https://example.com", "shared"), alternate.SubjectHash}[i]
		go func() {
			<-start
			results <- replica.DisconnectExternalIdentityAs(ctx, owner.Id, owner.Id, hash, policy)
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
	replayed := startReplica(t, ns, testCoreConfig())
	identities, err := replayed.ExternalIdentitiesForUser(ctx, owner.Id)
	if err != nil || len(identities) != 1 {
		t.Fatalf("replayed identities = %v, %v", identities, err)
	}
}

func TestLegacyOIDCCompletionAcrossReplicas(t *testing.T) {
	t.Parallel()
	for _, unlink := range []bool{false, true} {
		t.Run(map[bool]string{false: "concurrent completion", true: "concurrent unlink"}[unlink], func(t *testing.T) {
			t.Parallel()
			ns, _ := testutil.StartNATS(t)
			ctx := testContext(t)
			first := startReplica(t, ns, testCoreConfig())
			second := startReplica(t, ns, testCoreConfig())
			user, err := first.CreateUser(ctx, SystemActorID, "legacy-race", "Legacy Race", "password123")
			if err != nil {
				t.Fatal(err)
			}
			issuer, subject := "https://legacy.example", "legacy-subject"
			hash := externalIdentityHash(issuer, subject)
			legacy := newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_UserOidcSubjectLinked{
				UserOidcSubjectLinked: &evtv1.UserOIDCSubjectLinkedEvent{UserId: user.Id, SubjectHash: hash},
			}})
			if _, err := first.appendUserEvent(ctx, user.Id, legacy, "", nil); err != nil {
				t.Fatal(err)
			}
			if err := second.userModel.waitForUsersCurrent(ctx, "legacy fixture", evtstream.UserSubjectFilter()); err != nil {
				t.Fatal(err)
			}
			if err := second.userModel.waitForUserAuthCurrent(ctx, "legacy fixture"); err != nil {
				t.Fatal(err)
			}
			start := make(chan struct{})
			results := make(chan error, 2)
			go func() {
				<-start
				results <- first.CompleteLegacyOIDCIdentityLink(ctx, user.Id, "company", issuer, subject)
			}()
			go func() {
				<-start
				if unlink {
					results <- second.DisconnectExternalIdentityAs(ctx, user.Id, user.Id, hash, ExternalIdentitySignInPolicy{PasswordLoginEnabled: true})
				} else {
					results <- second.CompleteLegacyOIDCIdentityLink(ctx, user.Id, "company", issuer, subject)
				}
			}()
			close(start)
			for range 2 {
				select {
				case err := <-results:
					if err != nil {
						t.Fatal(err)
					}
				case <-ctx.Done():
					t.Fatal(ctx.Err())
				}
			}
			replayed := startReplica(t, ns, testCoreConfig())
			// Completion after restart must also leave missing links missing.
			if err := replayed.CompleteLegacyOIDCIdentityLink(ctx, user.Id, "company", issuer, subject); err != nil {
				t.Fatal(err)
			}
			identities, err := replayed.ExternalIdentitiesForUser(ctx, user.Id)
			if err != nil {
				t.Fatal(err)
			}
			if unlink {
				if len(identities) != 0 {
					t.Fatal("completion recreated the unlinked identity")
				}
			} else {
				if len(identities) != 1 || identities[0].Issuer != issuer {
					t.Fatalf("replayed identities = %v", identities)
				}
				facts, _, err := first.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(user.Id).Subject(evtstream.EventUserExternalIdentityLinked))
				if err != nil || len(facts) != 1 {
					t.Fatalf("completion facts = %d, error = %v", len(facts), err)
				}
			}
		})
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

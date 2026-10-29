package core

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/chattocorp/chatto/cli/internal/config"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/chattocorp/chatto/pkg/events"
	"github.com/stretchr/testify/require"
)

func TestManagedProviderAccountPolicy(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	c.config.AuthProviders = []config.AuthProviderConfig{{ID: "company", Type: "oidc", IssuerURL: "https://company.example", ManageUsernames: true, AllowUnlinking: new(false)}}
	existing, err := c.CreateUser(ctx, SystemActorID, "alice", "Existing", "password123")
	require.NoError(t, err)
	flow := &PendingExternalIdentityFlow{Kind: ExternalIdentityFlowKindCreate, ProviderID: "company", ProviderType: "oidc", Issuer: "https://company.example", Subject: "new-subject", ProviderUsername: " Alice "}
	user, err := c.CreateUserForExternalIdentity(ctx, "forged", "Alice", flow)
	require.NoError(t, err)
	require.Equal(t, "alice-2", user.Login)
	managed, err := c.UsernameManaged(ctx, user.Id)
	require.NoError(t, err)
	require.True(t, managed)
	_, err = c.UpdateUserLogin(ctx, user.Id, "edited")
	require.ErrorIs(t, err, ErrUsernameManaged)
	_, err = c.UpdateOwnUserProfile(ctx, user.Id, new("edited"), nil, nil)
	require.ErrorIs(t, err, ErrUsernameManaged)
	_, err = c.UpdateOwnUserProfile(ctx, user.Id, nil, new("New display name"), nil)
	require.NoError(t, err)
	require.NoError(t, c.SyncProviderUsername(ctx, user.Id, "company", "changed"))
	unchanged, err := c.GetUser(ctx, user.Id)
	require.NoError(t, err)
	require.Equal(t, "alice-2", unchanged.Login)
	c.config.AuthProviders[0].UpdateUsernames = true
	// Releasing the base name must not make a later sign-in drop the suffix.
	_, err = c.AdminUpdateUserLogin(ctx, existing.Id, "previous-alice")
	require.NoError(t, err)
	require.NoError(t, c.SyncProviderUsername(ctx, user.Id, "company", "alice"))
	unchanged, err = c.GetUser(ctx, user.Id)
	require.NoError(t, err)
	require.Equal(t, "alice-2", unchanged.Login, "sync must retain the existing collision suffix")
	require.NoError(t, c.SyncProviderUsername(ctx, user.Id, "company", "changed"))
	changed, err := c.GetUser(ctx, user.Id)
	require.NoError(t, err)
	require.Equal(t, "changed", changed.Login)
	require.Error(t, c.SyncProviderUsername(ctx, user.Id, "company", "invalid@name"))
	_, err = c.AdminUpdateUserLogin(ctx, user.Id, "operator-name")
	require.NoError(t, err)
	require.NoError(t, c.LinkExternalIdentity(ctx, "backup", "github", "backup", "backup-subject", user.Id))
	links, err := c.ExternalIdentitiesForUser(ctx, user.Id)
	require.NoError(t, err)
	policy := ExternalIdentitySignInPolicy{Issuers: []string{"https://company.example", "backup"}}
	for _, link := range links {
		if link.ProviderID != "company" {
			continue
		}
		err = c.DisconnectExternalIdentityAs(ctx, user.Id, user.Id, link.SubjectHash, policy)
		require.ErrorIs(t, err, ErrExternalIdentityUnlinkDisabled)
		require.NoError(t, c.DisconnectExternalIdentityAs(ctx, SystemActorID, user.Id, link.SubjectHash, policy))
	}
	managed, err = c.UsernameManaged(ctx, user.Id)
	require.NoError(t, err)
	require.False(t, managed)
	_, err = c.UpdateUserLogin(ctx, user.Id, "local-name")
	require.NoError(t, err)
}

func TestManagedProviderProfileKeepsUsername(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	c.config.AuthProviders = []config.AuthProviderConfig{{ID: "company", Type: "github", ManageUsernames: true}}
	user, err := c.CreateUserForExternalIdentity(ctx, "ignored", "Original", &PendingExternalIdentityFlow{
		Kind: ExternalIdentityFlowKindCreate, ProviderID: "company", ProviderType: "github",
		Issuer: "company", Subject: "profile-subject", ProviderUsername: "alice",
	})
	require.NoError(t, err)
	lastChange, err := c.GetLastLoginChange(ctx, user.Id)
	require.NoError(t, err)

	for _, login := range []string{"alice", "  alice  "} {
		t.Run(fmt.Sprintf("unchanged %q", login), func(t *testing.T) {
			displayName, bio := "Updated "+login, "Updated bio "+login
			updated, err := c.UpdateOwnUserProfile(ctx, user.Id, &login, &displayName, &bio)
			require.NoError(t, err)
			require.Equal(t, "alice", updated.Login)
			require.Equal(t, NormalizeDisplayName(displayName), updated.DisplayName)
			require.Equal(t, normalizeBio(bio), updated.Bio)
			stored, err := c.GetUser(ctx, user.Id)
			require.NoError(t, err)
			require.Equal(t, updated.DisplayName, stored.DisplayName)
			require.Equal(t, updated.Bio, stored.Bio)
			currentLastChange, err := c.GetLastLoginChange(ctx, user.Id)
			require.NoError(t, err)
			require.Equal(t, lastChange, currentLastChange, "an unchanged username must not start a cooldown")
		})
	}

	before, err := c.GetUser(ctx, user.Id)
	require.NoError(t, err)
	for _, login := range []string{"renamed", "Alice"} {
		t.Run("reject "+login, func(t *testing.T) {
			_, err := c.UpdateOwnUserProfile(ctx, user.Id, &login, new("Must not save"), new("Must not save"))
			require.ErrorIs(t, err, ErrUsernameManaged)
			stored, err := c.GetUser(ctx, user.Id)
			require.NoError(t, err)
			require.Equal(t, before.Login, stored.Login)
			require.Equal(t, before.DisplayName, stored.DisplayName)
			require.Equal(t, before.Bio, stored.Bio)
		})
	}
}

// A case-only sync must not retain its uniqueness exemption after a rename.
func TestManagedProviderCaseOnlySyncRejectsStaleUsername(t *testing.T) {
	t.Parallel()
	for _, reclaim := range []bool{false, true} {
		t.Run(fmt.Sprintf("reclaimed=%t", reclaim), func(t *testing.T) {
			t.Parallel()
			c, _ := setupTestCore(t)
			ctx := testContext(t)
			user, err := c.CreateUser(ctx, SystemActorID, "Alice", "First", "password123")
			require.NoError(t, err)
			intervened := false
			otherID := ""
			_, err = c.applyLoginChangeChecked(ctx, SystemActorID, user.Id, "alice", false, func() error {
				if intervened {
					return nil
				}
				intervened = true
				// Force a rename after the case-only decision, before the append.
				_, err := c.AdminUpdateUserLogin(ctx, user.Id, "renamed")
				require.NoError(t, err)
				if reclaim {
					other, err := c.CreateUser(ctx, SystemActorID, "alice", "Other", "password123")
					require.NoError(t, err)
					otherID = other.Id
				}
				return nil
			})
			require.ErrorIs(t, err, events.ErrConflict)
			stored, err := c.GetUser(ctx, user.Id)
			require.NoError(t, err)
			require.Equal(t, "renamed", stored.Login)
			if reclaim {
				other, err := c.GetUser(ctx, otherID)
				require.NoError(t, err)
				require.Equal(t, "alice", other.Login)
			}
		})
	}
}

func TestManagedProviderConcurrentCreation(t *testing.T) {
	ns, _ := testutil.StartNATS(t)
	cfg := testCoreConfig()
	cfg.AuthProviders = []config.AuthProviderConfig{{ID: "company", Type: "github", ManageUsernames: true}}
	c := startReplica(t, ns, cfg)
	second := startReplica(t, ns, cfg)
	ctx := testContext(t)
	var wg sync.WaitGroup
	results := make(chan string, 4)
	for i := range 4 {
		wg.Go(func() {
			replica := c
			if i%2 == 1 {
				replica = second
			}
			user, err := replica.CreateUserForExternalIdentity(ctx, "ignored", "Person", &PendingExternalIdentityFlow{
				Kind: ExternalIdentityFlowKindCreate, ProviderID: "company", ProviderType: "github", Issuer: "company", Subject: fmt.Sprint(i), ProviderUsername: "person",
			})
			if err != nil {
				t.Errorf("create: %v", err)
				return
			}
			results <- user.Login
		})
	}
	wg.Wait()
	close(results)
	names := map[string]bool{}
	for name := range results {
		require.False(t, names[name])
		names[name] = true
	}
	require.Len(t, names, 4)
	for _, name := range []string{"person", "person-2", "person-3", "person-4"} {
		require.True(t, names[name])
	}
	replayed := startReplica(t, ns, cfg)
	for i := range 4 {
		user, err := replayed.GetUserByExternalIdentity(ctx, "company", fmt.Sprint(i))
		require.NoError(t, err)
		require.NotNil(t, user)
		require.True(t, names[user.Login])
		managed, err := replayed.UsernameManaged(ctx, user.Id)
		require.NoError(t, err)
		require.True(t, managed)
	}
	_, err := c.SuggestProviderUsername("admin")
	require.True(t, errors.Is(err, ErrUsernameBlocked))
}

func TestManagedProviderUsernameValidation(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	c.config.AuthProviders = []config.AuthProviderConfig{{ID: "company", Type: "github", ManageUsernames: true}}
	for _, base := range []string{"", "person@example.com", "admin", "a", strings.Repeat("x", MaxLoginLength+1)} {
		_, err := c.CreateUserForExternalIdentity(ctx, "valid-override", "Person", &PendingExternalIdentityFlow{
			Kind: ExternalIdentityFlowKindCreate, ProviderID: "company", ProviderType: "github", Issuer: "company", Subject: "validation", ProviderUsername: base,
		})
		require.Error(t, err)
	}
	base := strings.Repeat("x", MaxLoginLength)
	_, err := c.CreateUser(ctx, SystemActorID, base, "Existing", "password123")
	require.NoError(t, err)
	user, err := c.CreateUserForExternalIdentity(ctx, "ignored", "Person", &PendingExternalIdentityFlow{
		Kind: ExternalIdentityFlowKindCreate, ProviderID: "company", ProviderType: "github", Issuer: "company", Subject: "long", ProviderUsername: base,
	})
	require.NoError(t, err)
	require.Equal(t, strings.Repeat("x", MaxLoginLength-2)+"-2", user.Login)
	_, err = c.CreateUserForExternalIdentity(ctx, "ignored", "Person", &PendingExternalIdentityFlow{
		Kind: ExternalIdentityFlowKindCreate, ProviderID: "company", ProviderType: "github", Issuer: "company", Subject: "long", ProviderUsername: "different",
	})
	require.ErrorIs(t, err, ErrExternalIdentityAlreadyClaimed)
}

func TestManagedProviderDoesNotRestrictBotUsername(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "bot-policy-owner", "Owner", "password123")
	require.NoError(t, err)
	allowBotCreation(t, ctx, c, owner.Id)
	bot, err := c.CreateBot(ctx, owner.Id, "policy-bot", "Bot")
	require.NoError(t, err)
	c.config.AuthProviders = []config.AuthProviderConfig{{ID: "company", Type: "github", ManageUsernames: true}}
	_, err = c.UpdateOwnUserProfile(ctx, bot.User.Id, new("renamed-bot"), nil, nil)
	require.NoError(t, err)
}

func TestManagedProviderLegacyOIDCRestrictions(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	c.config.AuthProviders = []config.AuthProviderConfig{{ID: "company", Type: "oidc", IssuerURL: "https://company.example", ManageUsernames: true, AllowUnlinking: new(false)}}
	user, err := c.CreateUser(ctx, SystemActorID, "legacy-managed", "Legacy", "password123")
	require.NoError(t, err)
	hash := externalIdentityHash("https://company.example", "legacy-subject")
	_, err = c.appendUserEvent(ctx, user.Id, newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_UserOidcSubjectLinked{
		UserOidcSubjectLinked: &evtv1.UserOIDCSubjectLinkedEvent{UserId: user.Id, SubjectHash: hash},
	}}), "", nil)
	require.NoError(t, err)
	_, err = c.UpdateOwnUserProfile(ctx, user.Id, new("bypass"), nil, nil)
	require.ErrorIs(t, err, ErrUsernameManaged)
	err = c.DisconnectExternalIdentityAs(ctx, user.Id, user.Id, hash, ExternalIdentitySignInPolicy{PasswordLoginEnabled: true})
	require.ErrorIs(t, err, ErrExternalIdentityUnlinkDisabled)
	require.NoError(t, c.CompleteLegacyOIDCIdentityLink(ctx, user.Id, "company", "https://company.example", "legacy-subject"))
	managed, err := c.UsernameManaged(ctx, user.Id)
	require.NoError(t, err)
	require.True(t, managed)
}

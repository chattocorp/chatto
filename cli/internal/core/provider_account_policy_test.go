package core

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestManagedProviderAccountPolicy(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	c.config.AuthProviders = []config.AuthProviderConfig{{ID: "company", Type: "oidc", IssuerURL: "https://company.example", ManageUsernames: true, AllowUnlinking: new(false)}}
	_, err := c.CreateUser(ctx, SystemActorID, "alice", "Existing", "password123")
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
	c.config.AuthProviders[0].SyncUsernames = true
	require.NoError(t, c.SyncProviderUsername(ctx, user.Id, "company", "alice"))
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

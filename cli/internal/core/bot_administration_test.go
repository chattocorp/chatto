package core

import (
	"context"
	"testing"

	"github.com/chattocorp/chatto/cli/internal/authctx"
	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/cli/internal/kms"
	"github.com/chattocorp/chatto/pkg/events"
	"github.com/stretchr/testify/require"
)

// Administrative bot credentials use explicit grants without an interactive
// privileged-mode session. Their human owner's entitlement remains the ceiling.
func TestBotAdministrativeAccountOperations(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "admin-bot-owner", "Owner", "")
	require.NoError(t, err)
	require.NoError(t, c.AssignAdminRole(ctx, owner.Id))
	bot, err := c.CreateBot(ctx, owner.Id, "account-admin-bot", "Account admin")
	require.NoError(t, err)
	target, err := c.CreateUser(ctx, SystemActorID, "managed-human", "Human", "")
	require.NoError(t, err)
	botCtx := authctx.WithCredential(ctx, authctx.RuntimeCredential{Kind: authctx.RuntimeCredentialKindBotAPIKey, UserID: bot.User.Id})
	name := "Updated by automation"
	_, err = c.UpdateManagedUserProfile(botCtx, bot.User.Id, target.Id, nil, &name, nil)
	require.ErrorIs(t, err, ErrPermissionDenied)
	scope := PermissionTargetScope{Kind: MatrixScopeServer}
	require.NoError(t, c.SetUserPermissionState(ctx, owner.Id, bot.User.Id, scope, PermUserManageAccounts, PermissionStateAllow))
	updated, err := c.UpdateManagedUserProfile(botCtx, bot.User.Id, target.Id, nil, &name, nil)
	require.NoError(t, err)
	require.Equal(t, name, updated.GetDisplayName())
	_, err = c.UpdateUserAvatar(botCtx, bot.User.Id, target.Id, createTestImage(32, 32))
	require.NoError(t, err)
	_, err = c.ClearUserAvatar(botCtx, bot.User.Id, target.Id)
	require.NoError(t, err)
	require.NoError(t, c.SetUserDeactivated(botCtx, bot.User.Id, target.Id, true))
	require.NoError(t, c.SetUserDeactivated(botCtx, bot.User.Id, target.Id, false))

	// Permission loss takes effect for a previously accepted bot credential.
	require.NoError(t, c.DenyUserPermission(ctx, SystemActorID, owner.Id, PermUserManageAccounts))
	_, err = c.UpdateManagedUserProfile(botCtx, bot.User.Id, target.Id, nil, &name, nil)
	require.ErrorIs(t, err, ErrPermissionDenied)
	_, err = c.ClearUserAvatar(botCtx, bot.User.Id, target.Id)
	require.ErrorIs(t, err, ErrPermissionDenied)
	require.NoError(t, c.ClearUserPermissionState(ctx, SystemActorID, owner.Id, PermUserManageAccounts))
	_, err = c.UpdateManagedUserProfile(botCtx, bot.User.Id, target.Id, nil, &name, nil)
	require.NoError(t, err)

	// The caller kind no longer blocks administration; target authentication
	// rules still prevent bots from obtaining human login methods.
	otherBot, err := c.CreateBot(ctx, owner.Id, "no-human-login-bot", "Bot")
	require.NoError(t, err)
	err = c.AdminSetUserPasswordAuthorized(botCtx, bot.User.Id, otherBot.User.Id, "password456")
	require.ErrorIs(t, err, ErrHumanAccountRequired)
}

// reassignOnCreateKeyWrapper changes ownership after creation has selected its
// owner but before its final authorization check. No scheduler timing is needed.
type reassignOnCreateKeyWrapper struct {
	kms.KeyWrapper
	reassign func() error
	userID   string
}

func (w *reassignOnCreateKeyWrapper) CreateKey(ctx context.Context, userID string) (string, error) {
	w.userID = userID
	if err := w.reassign(); err != nil {
		return "", err
	}
	return w.KeyWrapper.CreateKey(ctx, userID)
}

// Creation must reject the selected owner if the creator is reassigned before
// commit. A fresh request then creates the bot for the new human owner.
func TestBotCreationConcurrentWithCreatorReassignment(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "creation-owner", "Owner", "")
	require.NoError(t, err)
	require.NoError(t, c.AssignAdminRole(ctx, owner.Id))
	nextOwner, err := c.CreateUser(ctx, SystemActorID, "creation-next-owner", "Next owner", "")
	require.NoError(t, err)
	require.NoError(t, c.AssignAdminRole(ctx, nextOwner.Id))
	creator, err := c.CreateBot(ctx, owner.Id, "concurrent-creator", "Creator")
	require.NoError(t, err)
	require.NoError(t, c.SetUserPermissionState(ctx, owner.Id, creator.User.Id, PermissionTargetScope{Kind: MatrixScopeServer}, PermBotCreate, PermissionStateAllow))
	wrapper := &reassignOnCreateKeyWrapper{KeyWrapper: c.encryption.keyWrapper, reassign: func() error {
		_, err := c.ReassignBotOwner(ctx, nextOwner.Id, creator.User.Id, nextOwner.Id)
		return err
	}}
	c.encryption.keyWrapper = wrapper
	created, createErr := c.CreateBot(ctx, creator.User.Id, "concurrent-created", "Created")
	c.encryption.keyWrapper = wrapper.KeyWrapper
	require.NotEmpty(t, wrapper.userID, "test must reach key creation")
	require.ErrorIs(t, createErr, events.ErrConflict)
	require.Nil(t, created)
	history, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.UserAggregate(wrapper.userID).Subject(evtstream.EventUserAccountCreated))
	require.NoError(t, err)
	require.Empty(t, history, "rejected creation must not persist an account")
	created, err = c.CreateBot(ctx, creator.User.Id, "concurrent-created", "Created")
	require.NoError(t, err)
	require.Equal(t, nextOwner.Id, created.OwnerUserID)
}

func TestBotCanCreateAndManageBotsWithExplicitGrants(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "bot-admin-owner", "Owner", "")
	require.NoError(t, err)
	require.NoError(t, c.AssignAdminRole(ctx, owner.Id))
	otherOwner, err := c.CreateUser(ctx, SystemActorID, "other-bot-owner", "Other owner", "")
	require.NoError(t, err)
	require.NoError(t, c.AssignAdminRole(ctx, otherOwner.Id))
	manager, err := c.CreateBot(ctx, owner.Id, "bot-manager", "Manager")
	require.NoError(t, err)
	target, err := c.CreateBot(ctx, otherOwner.Id, "managed-bot", "Managed")
	require.NoError(t, err)
	botCtx := authctx.WithCredential(ctx, authctx.RuntimeCredential{Kind: authctx.RuntimeCredentialKindBotAPIKey, UserID: manager.User.Id})
	scope := PermissionTargetScope{Kind: MatrixScopeServer}
	_, err = c.CreateBot(botCtx, manager.User.Id, "new-bot", "New")
	require.ErrorIs(t, err, ErrPermissionDenied)
	_, err = c.CreateBotAPIKey(botCtx, manager.User.Id, target.User.Id, "Denied")
	require.ErrorIs(t, err, ErrPermissionDenied)
	for _, permission := range []Permission{PermBotCreate, PermBotManage} {
		require.NoError(t, c.SetUserPermissionState(ctx, owner.Id, manager.User.Id, scope, permission, PermissionStateAllow))
		explanation, err := c.PermResolver().ExplainServerPermission(botCtx, manager.User.Id, permission)
		require.NoError(t, err)
		require.Equal(t, DecisionAllow, explanation.State)
	}
	created, err := c.CreateBot(botCtx, manager.User.Id, "new-bot", "New")
	require.NoError(t, err)
	require.Equal(t, owner.Id, created.OwnerUserID)
	allowed, err := c.CanCreateBots(ctx, created.User.Id)
	require.NoError(t, err)
	require.False(t, allowed, "new bots do not inherit their creator's grants")
	_, err = c.GetBot(botCtx, manager.User.Id, target.User.Id)
	require.NoError(t, err)
	listed, err := c.ListBots(botCtx, manager.User.Id)
	require.NoError(t, err)
	require.Len(t, listed, 3)
	_, err = c.GetUserPermissionMatrix(botCtx, manager.User.Id, target.User.Id)
	require.NoError(t, err)

	// bot.manage intentionally permits credentials for a more powerful bot.
	// The issued credential acts with the target's grants and owner ceiling.
	require.NoError(t, c.SetUserPermissionState(ctx, otherOwner.Id, target.User.Id, scope, PermUserDeleteAny, PermissionStateAllow))
	issued, err := c.CreateBotAPIKey(botCtx, manager.User.Id, target.User.Id, "Automation")
	require.NoError(t, err)
	identity, err := c.ValidateBotAPIKey(ctx, issued.Credential)
	require.NoError(t, err)
	require.Equal(t, target.User.Id, identity.Id)
	allowed, err = c.CanDeleteUser(botCtx, manager.User.Id, otherOwner.Id)
	require.NoError(t, err)
	require.False(t, allowed)
	allowed, err = c.CanDeleteUser(ctx, identity.Id, otherOwner.Id)
	require.NoError(t, err)
	require.True(t, allowed)
	_, err = c.RevokeBotAPIKey(botCtx, manager.User.Id, target.User.Id, issued.KeyID)
	require.NoError(t, err)
	_, err = c.ValidateBotAPIKey(ctx, issued.Credential)
	require.Error(t, err)
	_, err = c.ReassignBotOwner(botCtx, manager.User.Id, target.User.Id, owner.Id)
	require.NoError(t, err)
	_, err = c.ReassignBotOwner(botCtx, manager.User.Id, target.User.Id, manager.User.Id)
	require.ErrorIs(t, err, ErrHumanAccountRequired)

	for _, permission := range []Permission{PermBotCreate, PermBotManage} {
		require.NoError(t, c.DenyUserPermission(ctx, SystemActorID, owner.Id, permission))
		explanation, err := c.PermResolver().ExplainServerPermission(botCtx, manager.User.Id, permission)
		require.NoError(t, err)
		require.Equal(t, DecisionDeny, explanation.State)
	}
	_, err = c.CreateBotAPIKey(botCtx, manager.User.Id, target.User.Id, "Revoked authority")
	require.ErrorIs(t, err, ErrPermissionDenied)
	_, err = c.CreateBot(botCtx, manager.User.Id, "denied-new-bot", "Denied")
	require.ErrorIs(t, err, ErrPermissionDenied)
	_, err = c.GetBot(botCtx, manager.User.Id, target.User.Id)
	require.ErrorIs(t, err, ErrPermissionDenied)
	for _, permission := range []Permission{PermBotCreate, PermBotManage} {
		require.NoError(t, c.ClearUserPermissionState(ctx, SystemActorID, owner.Id, permission))
	}
	deleted, err := c.DeleteBot(botCtx, manager.User.Id, target.User.Id)
	require.NoError(t, err)
	require.True(t, deleted)
}

package core

import (
	"errors"
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestUserDeactivationRevokesAccessAcrossReplicas(t *testing.T) {
	t.Parallel()
	ns, _ := testutil.StartNATS(t)
	first := startReplica(t, ns, testCoreConfig())
	ctx := testContext(t)
	manager := createGenerationZeroPasswordUser(t, first, "manager")
	target := createGenerationZeroPasswordUser(t, first, "target")
	if err := first.GrantUserPermission(ctx, SystemActorID, manager, PermUserManageAccounts); err != nil {
		t.Fatal(err)
	}
	cookie, _, err := first.CreateCookieSession(ctx, target, "password_login")
	if err != nil {
		t.Fatal(err)
	}
	bearer, err := first.CreateBearerSessionWithSource(ctx, target, "password_login")
	if err != nil {
		t.Fatal(err)
	}
	second := startReplica(t, ns, testCoreConfig())
	invalidated, cancel := second.WatchAccountAccessInvalidated(target)
	defer cancel()
	if err := first.SetUserDeactivated(ctx, manager, target, true); err != nil {
		t.Fatal(err)
	}
	select {
	case <-invalidated:
	case <-time.After(5 * time.Second):
		t.Fatal("replica did not invalidate active connection")
	}
	if _, err := second.ValidateCookieCredential(ctx, cookie); !errors.Is(err, ErrCookieSessionNotFound) {
		t.Fatalf("cookie accepted: %v", err)
	}
	if _, err := second.ValidateAuthToken(ctx, bearer.AccessToken); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("access token accepted: %v", err)
	}
	if _, err := second.RefreshBearerSession(ctx, bearer.RefreshToken, testRefreshRequestIDA, ""); err == nil {
		t.Fatal("refresh accepted")
	}
	if _, err := second.VerifyPassword(ctx, "target", "oldpassword123"); err == nil {
		t.Fatal("password login accepted")
	}
	if _, err := second.CreateBearerSessionWithSource(ctx, target, "oauth_code_exchange"); err == nil {
		t.Fatal("new session accepted")
	}
	if _, err := second.CreateAuthTokenWithSourceGeneration(ctx, target, "password_login", 0); err == nil {
		t.Fatal("in-flight login accepted")
	}
	user, err := second.GetUser(ctx, target)
	if err != nil || !user.GetDeactivated() || user.GetDeleted() || user.GetLogin() != "target" {
		t.Fatalf("account data changed: %v", err)
	}
	if _, ok := second.userModel.passwordHash(target); !ok {
		t.Fatal("password erased")
	}
	snapshot, err := second.userModel.users.Projection().Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	restored := newUserProjectionWithDEKResolver(second.userModel.users.Projection().dekResolver)
	if err := restored.Restore(snapshot); err != nil {
		t.Fatal(err)
	}
	if !restored.users[target].user.GetDeactivated() {
		t.Fatal("snapshot lost deactivation")
	}
	late, stopLate := second.WatchAccountAccessInvalidated(target)
	defer stopLate()
	select {
	case <-late:
	default:
		t.Fatal("late watch did not reject inactive account")
	}
	filter := evtstream.UserAggregate(target).AllEventsFilter()
	before, err := first.EventPublisher.LastSubjectSeq(ctx, filter)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.SetUserDeactivated(ctx, manager, target, true); err != nil {
		t.Fatal(err)
	}
	after, err := first.EventPublisher.LastSubjectSeq(ctx, filter)
	if err != nil || before != after {
		t.Fatal("duplicate command appended an event")
	}
	// Cold replay must retain access denial without a cleanup worker or KV scan.
	third := startReplica(t, ns, testCoreConfig())
	if _, err := third.CurrentAuthGeneration(ctx, target); !errors.Is(err, ErrAuthenticationRevoked) {
		t.Fatalf("cold replay accepted account: %v", err)
	}
	if err := first.SetUserDeactivated(ctx, manager, target, false); err != nil {
		t.Fatal(err)
	}
	if _, err := third.VerifyPassword(ctx, "target", "oldpassword123"); err != nil {
		t.Fatalf("reactivated password: %v", err)
	}
	if err := third.RequireAuthenticationAllowed(ctx, target, 0); !errors.Is(err, ErrAuthenticationRevoked) {
		t.Fatalf("reactivation revived old proof: %v", err)
	}
	if _, err := third.ValidateCookieCredential(ctx, cookie); err == nil {
		t.Fatal("reactivation revived cookie")
	}
	fresh, err := third.CreateBearerSessionWithSource(ctx, target, "password_login")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := first.ValidateAuthToken(ctx, fresh.AccessToken); err != nil {
		t.Fatalf("fresh sign-in failed: %v", err)
	}
}

func TestUserDeactivationAuthorization(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	actor := createGenerationZeroPasswordUser(t, c, "actor")
	target := createGenerationZeroPasswordUser(t, c, "target")
	if err := c.SetUserDeactivated(ctx, "", target, true); !errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("anonymous: %v", err)
	}
	if err := c.SetUserDeactivated(ctx, actor, target, true); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("missing permission: %v", err)
	}
	if err := c.GrantUserPermission(ctx, SystemActorID, actor, PermUserManageAccounts); err != nil {
		t.Fatal(err)
	}
	if err := c.SetUserDeactivated(ctx, actor, actor, true); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("self: %v", err)
	}
	if err := c.SetUserDeactivated(ctx, actor, "missing", true); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing target: %v", err)
	}
	if err := c.AssignServerRole(ctx, SystemActorID, target, RoleOwner); err != nil {
		t.Fatal(err)
	}
	if err := c.SetUserDeactivated(ctx, actor, target, true); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("owner: %v", err)
	}
}

func TestUserDeactivationRestoresDirectMessageAccess(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	manager := createGenerationZeroPasswordUser(t, c, "dmmanager")
	target := createGenerationZeroPasswordUser(t, c, "dmtarget")
	require.NoError(t, c.GrantUserPermission(ctx, SystemActorID, manager, PermUserManageAccounts))
	dm, _, err := c.roomCommands.StartDM(ctx, RoomStartDMInput{ActorID: manager, ParticipantIDs: []string{target}})
	require.NoError(t, err)
	input := MessagePostAuthorizationInput{ActorID: manager, RoomID: dm.Id, Body: "hello"}
	_, err = c.messageModel.AuthorizePost(ctx, input)
	require.NoError(t, err)
	require.NoError(t, c.SetUserDeactivated(ctx, manager, target, true))
	_, err = c.messageModel.AuthorizePost(ctx, input)
	require.ErrorIs(t, err, ErrPermissionDenied)
	// Opening the existing history remains possible.
	_, created, err := c.roomCommands.StartDM(ctx, RoomStartDMInput{ActorID: manager, ParticipantIDs: []string{target}})
	require.NoError(t, err)
	require.False(t, created)
	require.NoError(t, c.SetUserDeactivated(ctx, manager, target, false))
	_, err = c.messageModel.AuthorizePost(ctx, input)
	require.NoError(t, err)
}

func TestUserDeactivationBlocksOwnedBotsAndCallAccess(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	manager := createGenerationZeroPasswordUser(t, c, "manager")
	owner := createGenerationZeroPasswordUser(t, c, "botowner")
	require.NoError(t, c.GrantUserPermission(ctx, SystemActorID, manager, PermUserManageAccounts))
	allowBotCreation(t, ctx, c, owner)
	bot, err := c.CreateBot(ctx, owner, "test_bot", "Test Bot")
	require.NoError(t, err)
	webhook, err := c.CreateBotIncomingWebhook(ctx, owner, bot.User.GetId(), "test")
	require.NoError(t, err)
	_, verifier, err := c.ValidateBotAPIKeyCredential(ctx, bot.APIKey)
	require.NoError(t, err)
	invalidated, stop := c.WatchBotAPIKeyInvalidated(bot.User.GetId(), verifier)
	defer stop()
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "test-call", "")
	require.NoError(t, err)
	_, err = c.JoinRoom(ctx, owner, KindChannel, owner, room.GetId())
	require.NoError(t, err)
	_, err = c.AuthorizeCall(ctx, owner, room.GetId(), false)
	require.NoError(t, err)
	require.NoError(t, c.SetUserDeactivated(ctx, manager, owner, true))
	select {
	case <-invalidated:
	default:
		t.Fatal("owned bot connection remained valid")
	}
	_, err = c.ValidateBotAPIKey(ctx, bot.APIKey)
	require.ErrorIs(t, err, ErrAuthTokenNotFound)
	_, err = c.ValidateBotIncomingWebhookCredential(ctx, webhook.Credential)
	require.ErrorIs(t, err, ErrAuthTokenNotFound)
	_, err = c.AuthorizeCall(ctx, owner, room.GetId(), false)
	require.ErrorIs(t, err, ErrPermissionDenied)
	late, cancel := c.WatchBotAPIKeyInvalidated(bot.User.GetId(), verifier)
	defer cancel()
	select {
	case <-late:
	default:
		t.Fatal("late bot watch remained valid")
	}
	require.NoError(t, c.SetUserDeactivated(ctx, manager, owner, false))
	_, err = c.ValidateBotAPIKey(ctx, bot.APIKey)
	require.NoError(t, err)
	_, err = c.AuthorizeCall(ctx, owner, room.GetId(), false)
	require.NoError(t, err)
}

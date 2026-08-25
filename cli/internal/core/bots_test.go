package core

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestBotAPIKeyFormatIsCompactAndAcceptsLegacySecrets(t *testing.T) {
	botID := NewUserID()
	key, err := NewBotAPIKey(botID)
	if err != nil {
		t.Fatalf("NewBotAPIKey: %v", err)
	}
	if got, want := len(key), len(botAPIKeyPrefix)+len(botID)+1+base64.RawURLEncoding.EncodedLen(botAPIKeySecretBytes); got != want {
		t.Fatalf("key length = %d, want %d", got, want)
	}
	if parsedID, ok := parseBotAPIKey(key); !ok || parsedID != botID {
		t.Fatalf("parseBotAPIKey(new) = %q, %v", parsedID, ok)
	}

	legacySecret := make([]byte, legacyBotAPIKeySecretBytes)
	legacyKey := botAPIKeyPrefix + botID + "." + base64.RawURLEncoding.EncodeToString(legacySecret)
	if parsedID, ok := parseBotAPIKey(legacyKey); !ok || parsedID != botID {
		t.Fatalf("parseBotAPIKey(legacy) = %q, %v", parsedID, ok)
	}
}

func TestParseBotAPIKeyRejectsNonCanonicalUserIDs(t *testing.T) {
	encodedSecret := base64.RawURLEncoding.EncodeToString(make([]byte, botAPIKeySecretBytes))
	for _, botID := range []string{
		"*",
		">",
		"U123",
		"R123456789ABCDE",
		"U123456789ABCD*",
	} {
		t.Run(botID, func(t *testing.T) {
			if parsedID, ok := parseBotAPIKey(botAPIKeyPrefix + botID + "." + encodedSecret); ok {
				t.Fatalf("parseBotAPIKey accepted non-canonical bot ID %q as %q", botID, parsedID)
			}
		})
	}
}

func TestBotAccountLifecycleAndAuthentication(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "bot-owner", "Bot Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	bot, err := c.CreateBot(ctx, owner.GetId(), "helper_bot", "Helper Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	if !bot.User.GetIsBot() || bot.OwnerUserID != owner.GetId() {
		t.Fatalf("bot identity = %+v, want explicit bot owned by %s", bot, owner.GetId())
	}
	if !strings.HasPrefix(bot.APIKey, botAPIKeyPrefix+bot.User.GetId()+".") {
		t.Fatalf("API key has unexpected shape")
	}
	members, _, err := c.GetServerMembers(ctx, bot.User.GetLogin(), 10, 0)
	if err != nil || len(members) != 1 || len(members[0].Roles) != 0 {
		t.Fatalf("bot server roles = %+v, %v; want no implicit everyone role", members, err)
	}
	if authenticated, err := c.ValidateBotAPIKey(ctx, bot.APIKey); err != nil || authenticated.GetId() != bot.User.GetId() {
		t.Fatalf("ValidateBotAPIKey = %v, %v", authenticated, err)
	}
	if _, err := c.CreateAuthToken(ctx, bot.User.GetId()); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("CreateAuthToken(bot) err = %v, want ErrAuthTokenNotFound", err)
	}
	if err := c.SetPasswordHash(ctx, bot.User.GetId(), "password123"); !errors.Is(err, ErrHumanAccountRequired) {
		t.Fatalf("SetPasswordHash(bot) err = %v, want ErrHumanAccountRequired", err)
	}
	if err := c.SetUserAvatar(ctx, bot.User.GetId(), &corev1.AssetRecord{Id: "avatar"}); !errors.Is(err, ErrHumanAccountRequired) {
		t.Fatalf("SetUserAvatar(bot) err = %v, want ErrHumanAccountRequired", err)
	}
	if _, err := c.SetUserCustomStatus(ctx, bot.User.GetId(), "🤖", "online", nil); !errors.Is(err, ErrHumanAccountRequired) {
		t.Fatalf("SetUserCustomStatus(bot) err = %v, want ErrHumanAccountRequired", err)
	}

	rotated, err := c.RotateBotAPIKey(ctx, owner.GetId(), bot.User.GetId())
	if err != nil {
		t.Fatalf("RotateBotAPIKey: %v", err)
	}
	if rotated.APIKey == bot.APIKey || rotated.APIKeyRotatedAt.IsZero() {
		t.Fatal("rotation did not return a distinct key and rotation timestamp")
	}
	if _, err := c.ValidateBotAPIKey(ctx, bot.APIKey); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("old key err = %v, want ErrAuthTokenNotFound", err)
	}
	if _, err := c.ValidateBotAPIKey(ctx, rotated.APIKey); err != nil {
		t.Fatalf("rotated key: %v", err)
	}

	deleted, err := c.DeleteBot(ctx, owner.GetId(), bot.User.GetId())
	if err != nil || !deleted {
		t.Fatalf("DeleteBot = %v, %v", deleted, err)
	}
	if _, err := c.ValidateBotAPIKey(ctx, rotated.APIKey); !errors.Is(err, ErrAuthTokenNotFound) {
		t.Fatalf("deleted key err = %v, want ErrAuthTokenNotFound", err)
	}
}

func TestReassignBotOwnerRequiresGlobalManagementAndPreservesBotState(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "reassign-owner", "Reassign Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	newOwner, err := c.CreateUser(ctx, SystemActorID, "reassign-recipient", "Reassign Recipient", "password123")
	if err != nil {
		t.Fatalf("CreateUser recipient: %v", err)
	}
	manager, err := c.CreateUser(ctx, SystemActorID, "reassign-manager", "Reassign Manager", "password123")
	if err != nil {
		t.Fatalf("CreateUser manager: %v", err)
	}
	bot, err := c.CreateBot(ctx, owner.GetId(), "reassign_bot", "Reassign Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	_, verifier, err := c.ValidateBotAPIKeyCredential(ctx, bot.APIKey)
	if err != nil {
		t.Fatalf("ValidateBotAPIKeyCredential: %v", err)
	}
	invalidated, stopWatching := c.WatchBotAPIKeyInvalidated(bot.User.GetId(), verifier)
	defer stopWatching()
	if err := c.GrantUserPermission(ctx, SystemActorID, owner.GetId(), PermMessagePost); err != nil {
		t.Fatalf("grant owner message.post: %v", err)
	}
	if err := c.SetUserPermissionState(ctx, owner.GetId(), bot.User.GetId(), PermissionTargetScope{Kind: MatrixScopeServer}, PermMessagePost, PermissionStateAllow); err != nil {
		t.Fatalf("grant bot message.post: %v", err)
	}
	if err := c.DenyUserPermission(ctx, SystemActorID, newOwner.GetId(), PermMessagePost); err != nil {
		t.Fatalf("deny recipient message.post: %v", err)
	}

	if _, err := c.ReassignBotOwner(ctx, owner.GetId(), bot.User.GetId(), newOwner.GetId()); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("owner-only reassignment err = %v, want ErrPermissionDenied", err)
	}
	if err := c.GrantUserPermission(ctx, SystemActorID, manager.GetId(), PermBotManage); err != nil {
		t.Fatalf("grant manager bot.manage: %v", err)
	}
	fenceBefore, err := c.authorizationFenceSeq(ctx)
	if err != nil {
		t.Fatalf("authorization fence before reassignment: %v", err)
	}
	reassigned, err := c.ReassignBotOwner(ctx, manager.GetId(), bot.User.GetId(), newOwner.GetId())
	if err != nil {
		t.Fatalf("ReassignBotOwner: %v", err)
	}
	if reassigned.OwnerUserID != newOwner.GetId() || reassigned.User.GetBotOwnerUserId() != newOwner.GetId() {
		t.Fatalf("reassigned bot = %+v, want owner %s", reassigned, newOwner.GetId())
	}
	fenceAfter, err := c.authorizationFenceSeq(ctx)
	if err != nil {
		t.Fatalf("authorization fence after reassignment: %v", err)
	}
	if fenceAfter <= fenceBefore {
		t.Fatalf("authorization fence did not advance: before=%d after=%d", fenceBefore, fenceAfter)
	}
	if authenticated, err := c.ValidateBotAPIKey(ctx, bot.APIKey); err != nil || authenticated.GetId() != bot.User.GetId() {
		t.Fatalf("existing bot key after reassignment = %v, %v", authenticated, err)
	}
	select {
	case <-invalidated:
		t.Fatal("owner reassignment invalidated an established bot credential watcher")
	default:
	}
	if _, err := c.GetBot(ctx, owner.GetId(), bot.User.GetId()); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("previous owner GetBot err = %v, want ErrPermissionDenied", err)
	}
	if _, err := c.GetBot(ctx, newOwner.GetId(), bot.User.GetId()); err != nil {
		t.Fatalf("new owner GetBot: %v", err)
	}
	if override, err := c.GetUserExplicitServerOverride(ctx, bot.User.GetId(), PermMessagePost); err != nil || override != DecisionAllow {
		t.Fatalf("stored bot grant after reassignment = %s, %v; want allow", override, err)
	}
	if effective, err := c.PermResolver().Resolve(ctx, bot.User.GetId(), KindChannel, "", PermMessagePost); err != nil || effective != DecisionDeny {
		t.Fatalf("new owner-capped bot permission = %s, %v; want deny", effective, err)
	}

	fenceBeforeNoop, err := c.authorizationFenceSeq(ctx)
	if err != nil {
		t.Fatalf("authorization fence before no-op: %v", err)
	}
	if _, err := c.ReassignBotOwner(ctx, manager.GetId(), bot.User.GetId(), newOwner.GetId()); err != nil {
		t.Fatalf("idempotent ReassignBotOwner: %v", err)
	}
	fenceAfterNoop, err := c.authorizationFenceSeq(ctx)
	if err != nil {
		t.Fatalf("authorization fence after no-op: %v", err)
	}
	if fenceAfterNoop != fenceBeforeNoop {
		t.Fatalf("no-op reassignment advanced authorization fence: before=%d after=%d", fenceBeforeNoop, fenceAfterNoop)
	}

	otherBot, err := c.CreateBot(ctx, owner.GetId(), "recipient_bot", "Recipient Bot")
	if err != nil {
		t.Fatalf("CreateBot recipient: %v", err)
	}
	if _, err := c.ReassignBotOwner(ctx, manager.GetId(), bot.User.GetId(), otherBot.User.GetId()); !errors.Is(err, ErrHumanAccountRequired) {
		t.Fatalf("bot recipient reassignment err = %v, want ErrHumanAccountRequired", err)
	}
	if _, err := c.ReassignBotOwner(ctx, bot.User.GetId(), bot.User.GetId(), owner.GetId()); !errors.Is(err, ErrHumanAccountRequired) {
		t.Fatalf("bot actor reassignment err = %v, want ErrHumanAccountRequired", err)
	}
}

func TestConcurrentBotOwnerReassignmentsConvergeWithoutStaleIndexes(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "concurrent-owner", "Concurrent Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	firstOwner, err := c.CreateUser(ctx, SystemActorID, "concurrent-first", "Concurrent First", "password123")
	if err != nil {
		t.Fatalf("CreateUser first recipient: %v", err)
	}
	secondOwner, err := c.CreateUser(ctx, SystemActorID, "concurrent-second", "Concurrent Second", "password123")
	if err != nil {
		t.Fatalf("CreateUser second recipient: %v", err)
	}
	manager, err := c.CreateUser(ctx, SystemActorID, "concurrent-manager", "Concurrent Manager", "password123")
	if err != nil {
		t.Fatalf("CreateUser manager: %v", err)
	}
	if err := c.GrantUserPermission(ctx, SystemActorID, manager.GetId(), PermBotManage); err != nil {
		t.Fatalf("grant manager bot.manage: %v", err)
	}
	bot, err := c.CreateBot(ctx, owner.GetId(), "concurrent_bot", "Concurrent Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	for _, recipientID := range []string{firstOwner.GetId(), secondOwner.GetId()} {
		go func() {
			<-start
			_, err := c.ReassignBotOwner(ctx, manager.GetId(), bot.User.GetId(), recipientID)
			results <- err
		}()
	}
	close(start)
	for range 2 {
		if err := <-results; err != nil {
			t.Fatalf("concurrent ReassignBotOwner: %v", err)
		}
	}

	current, err := c.GetUser(ctx, bot.User.GetId())
	if err != nil {
		t.Fatalf("GetUser bot: %v", err)
	}
	winnerID := current.GetBotOwnerUserId()
	if winnerID != firstOwner.GetId() && winnerID != secondOwner.GetId() {
		t.Fatalf("final owner = %s, want one concurrent recipient", winnerID)
	}
	if bots := c.userModel.botIDsOwnedBy(owner.GetId()); len(bots) != 0 {
		t.Fatalf("previous owner index = %v, want empty", bots)
	}
	for _, recipientID := range []string{firstOwner.GetId(), secondOwner.GetId()} {
		bots := c.userModel.botIDsOwnedBy(recipientID)
		if recipientID == winnerID {
			if len(bots) != 1 || bots[0] != bot.User.GetId() {
				t.Fatalf("winning owner index = %v, want bot %s", bots, bot.User.GetId())
			}
		} else if len(bots) != 0 {
			t.Fatalf("losing owner index = %v, want empty", bots)
		}
	}
}

func TestReassignBotOwnerIsRaceSafeWithOwnerDeletion(t *testing.T) {
	for _, tc := range []struct {
		name           string
		deletePrevious bool
	}{
		{name: "previous owner", deletePrevious: true},
		{name: "new owner", deletePrevious: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := setupTestCore(t)
			ctx := testContext(t)
			owner, err := c.CreateUser(ctx, SystemActorID, "race-owner", "Race Owner", "password123")
			if err != nil {
				t.Fatalf("CreateUser owner: %v", err)
			}
			newOwner, err := c.CreateUser(ctx, SystemActorID, "race-recipient", "Race Recipient", "password123")
			if err != nil {
				t.Fatalf("CreateUser recipient: %v", err)
			}
			manager, err := c.CreateUser(ctx, SystemActorID, "race-manager", "Race Manager", "password123")
			if err != nil {
				t.Fatalf("CreateUser manager: %v", err)
			}
			if err := c.GrantUserPermission(ctx, SystemActorID, manager.GetId(), PermBotManage); err != nil {
				t.Fatalf("grant manager bot.manage: %v", err)
			}
			bot, err := c.CreateBot(ctx, owner.GetId(), "race_bot", "Race Bot")
			if err != nil {
				t.Fatalf("CreateBot: %v", err)
			}

			deletedOwner := newOwner
			if tc.deletePrevious {
				deletedOwner = owner
			}
			start := make(chan struct{})
			reassigned := make(chan error, 1)
			deleted := make(chan error, 1)
			go func() {
				<-start
				_, err := c.ReassignBotOwner(ctx, manager.GetId(), bot.User.GetId(), newOwner.GetId())
				reassigned <- err
			}()
			go func() {
				<-start
				deleted <- c.DeleteUser(ctx, deletedOwner.GetId(), deletedOwner.GetId())
			}()
			close(start)
			reassignErr := <-reassigned
			if deleteErr := <-deleted; deleteErr != nil {
				t.Fatalf("DeleteUser: %v", deleteErr)
			}

			current, getErr := c.GetUser(ctx, bot.User.GetId())
			if getErr == nil && current.GetBotOwnerUserId() == deletedOwner.GetId() {
				t.Fatalf("bot survived with deleted owner %s after reassignment error %v", deletedOwner.GetId(), reassignErr)
			}
			if tc.deletePrevious && getErr == nil && current.GetBotOwnerUserId() != newOwner.GetId() {
				t.Fatalf("surviving bot owner = %s, want %s", current.GetBotOwnerUserId(), newOwner.GetId())
			}
			if !tc.deletePrevious && reassignErr == nil && !errors.Is(getErr, ErrNotFound) {
				t.Fatalf("successful reassignment to deleted owner left bot active: %+v, %v", current, getErr)
			}
		})
	}
}

func TestBotAPIKeyInvalidationWatchFollowsDurableRotation(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "watch-owner", "Watch Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	bot, err := c.CreateBot(ctx, owner.GetId(), "watch_bot", "Watch Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	_, verifier, err := c.ValidateBotAPIKeyCredential(ctx, bot.APIKey)
	if err != nil {
		t.Fatalf("ValidateBotAPIKeyCredential: %v", err)
	}
	invalidated, stop := c.WatchBotAPIKeyInvalidated(bot.User.GetId(), verifier)
	defer stop()

	if _, err := c.RotateBotAPIKey(ctx, owner.GetId(), bot.User.GetId()); err != nil {
		t.Fatalf("RotateBotAPIKey: %v", err)
	}
	select {
	case <-invalidated:
	case <-time.After(5 * time.Second):
		t.Fatal("bot key watcher remained open after durable rotation reached the projection")
	}
}

func TestGenericAdminMutationsRejectBotAccounts(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "admin-boundary-owner", "Admin Boundary Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	admin, err := c.CreateUser(ctx, SystemActorID, "admin-boundary-admin", "Admin Boundary Admin", "password123")
	if err != nil {
		t.Fatalf("CreateUser admin: %v", err)
	}
	if err := c.AssignAdminRole(ctx, admin.GetId()); err != nil {
		t.Fatalf("AssignAdminRole: %v", err)
	}
	bot, err := c.CreateBot(ctx, owner.GetId(), "admin_boundary_bot", "Admin Boundary Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}

	newName := "Changed"
	if _, err := c.AdminUpdateUser(ctx, admin.GetId(), bot.User.GetId(), AdminUpdateUserInput{DisplayName: &newName}); !errors.Is(err, ErrHumanAccountRequired) {
		t.Fatalf("AdminUpdateUser(bot) err = %v, want ErrHumanAccountRequired", err)
	}
	if err := c.AdminSetUserPasswordAuthorized(ctx, admin.GetId(), bot.User.GetId(), "password456"); !errors.Is(err, ErrHumanAccountRequired) {
		t.Fatalf("AdminSetUserPasswordAuthorized(bot) err = %v, want ErrHumanAccountRequired", err)
	}
	if err := c.AdminClearLoginChangeCooldown(ctx, admin.GetId(), bot.User.GetId()); !errors.Is(err, ErrHumanAccountRequired) {
		t.Fatalf("AdminClearLoginChangeCooldown(bot) err = %v, want ErrHumanAccountRequired", err)
	}
	if err := c.AdminDeleteUserAs(ctx, admin.GetId(), bot.User.GetId()); !errors.Is(err, ErrHumanAccountRequired) {
		t.Fatalf("AdminDeleteUserAs(bot) err = %v, want ErrHumanAccountRequired", err)
	}

	details, err := c.GetAdminMemberDetails(ctx, admin.GetId(), bot.User.GetId())
	if err != nil {
		t.Fatalf("GetAdminMemberDetails(bot): %v", err)
	}
	if details.ViewerCanAssignRoles || details.ViewerCanManageRoles || details.ViewerCanManageUserPermissions || details.Member.ViewerCanDeleteAccount {
		t.Fatalf("bot generic admin capabilities = %+v, want all mutation capabilities false", details)
	}
}

func TestBotPermissionsAreExplicitAndOwnerCapped(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "permission-owner", "Permission Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	bot, err := c.CreateBot(ctx, owner.GetId(), "permission_bot", "Permission Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}

	for _, permission := range []Permission{PermMessageRead, PermMessagePost} {
		if decision, err := c.PermResolver().Resolve(ctx, bot.User.GetId(), KindChannel, "", permission); err != nil || decision != DecisionDeny {
			t.Fatalf("unconfigured bot %s = %s, %v; want deny", permission, decision, err)
		}
	}
	if allowed, err := c.CanCreateBots(ctx, bot.User.GetId()); err != nil || allowed {
		t.Fatalf("bot CanCreateBots = %v, %v; want false", allowed, err)
	}
	if err := c.SetUserPermissionState(ctx, owner.GetId(), bot.User.GetId(), PermissionTargetScope{Kind: MatrixScopeServer}, PermMessagePost, PermissionStateDeny); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("explicit bot denial err = %v, want ErrInvalidArgument", err)
	}
	if decision, err := c.GetUserExplicitServerOverride(ctx, bot.User.GetId(), PermMessagePost); err != nil || decision != DecisionNone {
		t.Fatalf("explicit bot override after rejected denial = %s, %v; want none", decision, err)
	}
	if err := c.SetUserPermissionState(ctx, owner.GetId(), bot.User.GetId(), PermissionTargetScope{Kind: MatrixScopeServer}, PermBotCreate, PermissionStateAllow); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("delegate bot.create err = %v, want ErrInvalidArgument", err)
	}
	if err := c.AssignServerRoleToExistingUser(ctx, SystemActorID, bot.User.GetId(), RoleAdmin); !errors.Is(err, ErrHumanAccountRequired) {
		t.Fatalf("assign bot role err = %v, want ErrHumanAccountRequired", err)
	}

	if err := c.SetUserPermissionState(ctx, owner.GetId(), bot.User.GetId(), PermissionTargetScope{Kind: MatrixScopeServer}, PermMessagePost, PermissionStateAllow); err != nil {
		t.Fatalf("SetUserPermissionState allow: %v", err)
	}
	if decision, err := c.PermResolver().Resolve(ctx, bot.User.GetId(), KindChannel, "", PermMessagePost); err != nil || decision != DecisionAllow {
		t.Fatalf("configured bot message.post = %s, %v; want allow", decision, err)
	}
	if err := c.SetUserPermissionState(ctx, owner.GetId(), bot.User.GetId(), PermissionTargetScope{Kind: MatrixScopeServer}, PermMessageRead, PermissionStateAllow); err != nil {
		t.Fatalf("SetUserPermissionState message.read allow: %v", err)
	}
	if decision, err := c.PermResolver().Resolve(ctx, bot.User.GetId(), KindChannel, "", PermMessageRead); err != nil || decision != DecisionAllow {
		t.Fatalf("configured bot message.read = %s, %v; want allow", decision, err)
	}

	if err := c.DenyServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost); err != nil {
		t.Fatalf("deny owner permission: %v", err)
	}
	if decision, err := c.PermResolver().Resolve(ctx, bot.User.GetId(), KindChannel, "", PermMessagePost); err != nil || decision != DecisionDeny {
		t.Fatalf("owner-capped bot message.post = %s, %v; want deny", decision, err)
	}
	if err := c.DenyServerPermission(ctx, SystemActorID, RoleEveryone, PermMessageRead); err != nil {
		t.Fatalf("deny owner message.read: %v", err)
	}
	if decision, err := c.PermResolver().Resolve(ctx, bot.User.GetId(), KindChannel, "", PermMessageRead); err != nil || decision != DecisionDeny {
		t.Fatalf("owner-capped bot message.read = %s, %v; want deny", decision, err)
	}
	matrix, err := c.GetUserPermissionMatrix(ctx, owner.GetId(), bot.User.GetId())
	if err != nil {
		t.Fatalf("GetUserPermissionMatrix: %v", err)
	}
	var found *PermissionMatrixCell
	for i := range matrix.Cells {
		if matrix.Cells[i].ScopeID == "server" && matrix.Cells[i].Permission == string(PermMessagePost) {
			found = &matrix.Cells[i]
			break
		}
	}
	if found == nil || found.Override != MatrixDecisionAllow || found.Effective != MatrixDecisionDeny || found.AllowPermitted == nil || *found.AllowPermitted {
		t.Fatalf("dormant grant cell = %+v", found)
	}
	if err := c.SetUserPermissionState(ctx, owner.GetId(), bot.User.GetId(), PermissionTargetScope{Kind: MatrixScopeServer}, PermRoomCreate, PermissionStateAllow); !errors.Is(err, ErrBotOwnerPermissionCeiling) {
		t.Fatalf("over-ceiling grant err = %v, want ErrBotOwnerPermissionCeiling", err)
	}
}

func TestBotDMReadUsesMembershipInsteadOfDelegatedMessageRead(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "dm-bot-owner", "DM Bot Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	bot, err := c.CreateBot(ctx, owner.GetId(), "dm_reader_bot", "DM Reader Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	participant, err := c.CreateUser(ctx, SystemActorID, "dm-bot-participant", "DM Bot Participant", "password123")
	if err != nil {
		t.Fatalf("CreateUser participant: %v", err)
	}
	dm, _, err := c.FindOrCreateDM(ctx, participant.GetId(), []string{bot.User.GetId()})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	message, err := c.PostMessage(ctx, KindDM, dm.GetId(), participant.GetId(), "message for bot participant", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if err := c.DenyServerPermission(ctx, SystemActorID, RoleEveryone, PermMessageRead); err != nil {
		t.Fatalf("DenyServerPermission message.read: %v", err)
	}
	if canRead, err := c.CanReadMessages(ctx, bot.User.GetId(), KindChannel, ""); err != nil || canRead {
		t.Fatalf("unconfigured bot channel CanReadMessages = %v, %v; want false", canRead, err)
	}
	if canRead, err := c.CanReadMessages(ctx, bot.User.GetId(), KindDM, dm.GetId()); err != nil || !canRead {
		t.Fatalf("bot DM CanReadMessages = %v, %v; want true", canRead, err)
	}
	if _, err := c.RoomTimelineReads().GetMessage(ctx, bot.User.GetId(), dm.GetId(), message.GetId()); err != nil {
		t.Fatalf("GetMessage as bot DM participant: %v", err)
	}
}

func TestCanonicalUserPermissionManagementUsesBotAuthorization(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "canonical-owner", "Canonical Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	manager, err := c.CreateUser(ctx, SystemActorID, "canonical-manager", "Canonical Manager", "password123")
	if err != nil {
		t.Fatalf("CreateUser manager: %v", err)
	}
	bot, err := c.CreateBot(ctx, owner.GetId(), "canonical_bot", "Canonical Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}

	if err := c.GrantUserPermission(ctx, SystemActorID, manager.GetId(), PermUserManagePermissions); err != nil {
		t.Fatalf("GrantUserPermission user.manage-permissions: %v", err)
	}
	if _, err := c.GetUserPermissionMatrix(ctx, manager.GetId(), bot.User.GetId()); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("user permission manager reading bot matrix err = %v, want ErrPermissionDenied", err)
	}
	if err := c.SetUserPermissionState(ctx, manager.GetId(), bot.User.GetId(), PermissionTargetScope{Kind: MatrixScopeServer}, PermMessagePost, PermissionStateDeny); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("user permission manager writing bot matrix err = %v, want ErrPermissionDenied", err)
	}

	if err := c.GrantUserPermission(ctx, SystemActorID, manager.GetId(), PermBotManage); err != nil {
		t.Fatalf("GrantUserPermission bot.manage: %v", err)
	}
	if _, err := c.GetUserPermissionMatrix(ctx, manager.GetId(), bot.User.GetId()); err != nil {
		t.Fatalf("bot manager GetUserPermissionMatrix: %v", err)
	}
	if err := c.SetUserPermissionState(ctx, manager.GetId(), bot.User.GetId(), PermissionTargetScope{Kind: MatrixScopeServer}, PermMessagePost, PermissionStateAllow); err != nil {
		t.Fatalf("bot manager SetUserPermissionState: %v", err)
	}

	if _, err := c.GetUserPermissionMatrix(ctx, bot.User.GetId(), bot.User.GetId()); !errors.Is(err, ErrHumanAccountRequired) {
		t.Fatalf("bot caller GetUserPermissionMatrix err = %v, want ErrHumanAccountRequired", err)
	}
	if err := c.SetUserPermissionState(ctx, bot.User.GetId(), bot.User.GetId(), PermissionTargetScope{Kind: MatrixScopeServer}, PermMessagePost, PermissionStateDeny); !errors.Is(err, ErrHumanAccountRequired) {
		t.Fatalf("bot caller SetUserPermissionState err = %v, want ErrHumanAccountRequired", err)
	}
	if _, err := c.GetUserPermissionMatrix(ctx, "", "missing-user"); !errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("unauthenticated GetUserPermissionMatrix err = %v, want ErrNotAuthenticated", err)
	}
	if err := c.SetUserPermissionState(ctx, "", "missing-user", PermissionTargetScope{Kind: MatrixScopeServer}, PermMessagePost, PermissionStateDeny); !errors.Is(err, ErrNotAuthenticated) {
		t.Fatalf("unauthenticated SetUserPermissionState err = %v, want ErrNotAuthenticated", err)
	}
}

func TestCanonicalUserPermissionMatrixForBotFiltersHiddenRoomsAndKeepsDirectoryGroups(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "matrix-owner", "Matrix Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	bot, err := c.CreateBot(ctx, owner.GetId(), "matrix_bot", "Matrix Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	group, err := c.CreateRoomGroup(ctx, SystemActorID, "Hidden Operations", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup: %v", err)
	}
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, group.GetId(), "hidden-operations", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := c.DenyRoomPermission(ctx, SystemActorID, room.GetId(), RoleEveryone, PermRoomList); err != nil {
		t.Fatalf("DenyRoomPermission: %v", err)
	}

	matrix, err := c.GetUserPermissionMatrix(ctx, owner.GetId(), bot.User.GetId())
	if err != nil {
		t.Fatalf("GetUserPermissionMatrix: %v", err)
	}
	groupFound := false
	for _, scope := range matrix.Scopes {
		if scope.ID == "group:"+group.GetId() && scope.Label == group.GetName() {
			groupFound = true
		}
		if scope.ID == "room:"+room.GetId() || scope.Label == room.GetName() {
			t.Fatalf("hidden room leaked through bot matrix: %+v", scope)
		}
	}
	if !groupFound {
		t.Fatal("directory-visible group missing from bot matrix")
	}

	empty, err := c.CreateRoomGroup(ctx, SystemActorID, "Empty Automation", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup empty: %v", err)
	}
	if err := c.GrantUserGroupPermission(ctx, SystemActorID, empty.GetId(), owner.GetId(), PermRoomCreate); err != nil {
		t.Fatalf("GrantUserGroupPermission room.create: %v", err)
	}
	err = c.SetUserPermissionState(ctx, owner.GetId(), bot.User.GetId(), PermissionTargetScope{
		Kind: MatrixScopeGroup,
		ID:   empty.GetId(),
	}, PermRoomCreate, PermissionStateAllow)
	if err != nil {
		t.Fatalf("SetUserPermissionState empty group room.create: %v", err)
	}
	matrix, err = c.GetUserPermissionMatrix(ctx, owner.GetId(), bot.User.GetId())
	if err != nil {
		t.Fatalf("GetUserPermissionMatrix after group grant: %v", err)
	}
	var cell *PermissionMatrixCell
	for i := range matrix.Cells {
		if matrix.Cells[i].ScopeID == "group:"+empty.GetId() && matrix.Cells[i].Permission == string(PermRoomCreate) {
			cell = &matrix.Cells[i]
			break
		}
	}
	if cell == nil || cell.Override != MatrixDecisionAllow || cell.Effective != MatrixDecisionAllow || cell.AllowPermitted == nil || !*cell.AllowPermitted {
		t.Fatalf("empty group room.create cell = %+v, want effective owner-bounded allow", cell)
	}
}

func TestSetUserPermissionRejectsNonexistentBotScopesWithoutPersisting(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "scope-owner", "Scope Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	bot, err := c.CreateBot(ctx, owner.GetId(), "scope_bot", "Scope Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}

	for _, test := range []struct {
		name  string
		scope PermissionTargetScope
	}{
		{name: "group", scope: PermissionTargetScope{Kind: MatrixScopeGroup, ID: "missing-group"}},
		{name: "room", scope: PermissionTargetScope{Kind: MatrixScopeRoom, ID: "missing-room"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := c.SetUserPermissionState(ctx, owner.GetId(), bot.User.GetId(), test.scope, PermMessagePost, PermissionStateAllow); !errors.Is(err, ErrInvalidArgument) {
				t.Fatalf("SetUserPermissionState nonexistent %s err = %v, want ErrInvalidArgument", test.name, err)
			}
			var decision DecisionKind
			var err error
			if test.scope.Kind == MatrixScopeGroup {
				decision, err = c.GetUserExplicitGroupOverride(ctx, test.scope.ID, bot.User.GetId(), PermMessagePost)
			} else {
				decision, err = c.GetUserExplicitRoomOverride(ctx, test.scope.ID, bot.User.GetId(), PermMessagePost)
			}
			if err != nil || decision != DecisionNone {
				t.Fatalf("persisted decision after rejected %s scope = %s, %v", test.name, decision, err)
			}
		})
	}
}

func TestSetUserPermissionUsesCurrentRoomGroupForBotOwnerCeiling(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "moving-room-owner", "Moving Room Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	bot, err := c.CreateBot(ctx, owner.GetId(), "moving_room_bot", "Moving Room Bot")
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	oldGroup, err := c.CreateRoomGroup(ctx, SystemActorID, "Old Group", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup old: %v", err)
	}
	newGroup, err := c.CreateRoomGroup(ctx, SystemActorID, "New Group", "")
	if err != nil {
		t.Fatalf("CreateRoomGroup new: %v", err)
	}
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, oldGroup.GetId(), "moving-room", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if err := c.GrantUserGroupPermission(ctx, SystemActorID, oldGroup.GetId(), owner.GetId(), PermRoomManage); err != nil {
		t.Fatalf("GrantUserGroupPermission old group: %v", err)
	}
	if err := c.MoveRoomToGroup(ctx, SystemActorID, room.GetId(), newGroup.GetId()); err != nil {
		t.Fatalf("MoveRoomToGroup: %v", err)
	}

	err = c.SetUserPermissionState(ctx, owner.GetId(), bot.User.GetId(), PermissionTargetScope{
		Kind: MatrixScopeRoom,
		ID:   room.GetId(),
	}, PermRoomManage, PermissionStateAllow)
	if !errors.Is(err, ErrBotOwnerPermissionCeiling) {
		t.Fatalf("SetUserPermissionState after room move err = %v, want ErrBotOwnerPermissionCeiling", err)
	}
}

func TestDeletingOwnerCascadesOwnedBots(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "cascade-owner", "Cascade Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	first, err := c.CreateBot(ctx, owner.GetId(), "first_bot", "First Bot")
	if err != nil {
		t.Fatalf("CreateBot first: %v", err)
	}
	second, err := c.CreateBot(ctx, owner.GetId(), "second_bot", "Second Bot")
	if err != nil {
		t.Fatalf("CreateBot second: %v", err)
	}

	if err := c.DeleteUser(ctx, owner.GetId(), owner.GetId()); err != nil {
		t.Fatalf("DeleteUser owner: %v", err)
	}
	for _, id := range []string{owner.GetId(), first.User.GetId(), second.User.GetId()} {
		if _, err := c.GetUser(ctx, id); !errors.Is(err, ErrNotFound) {
			t.Fatalf("GetUser(%s) err = %v, want ErrNotFound", id, err)
		}
	}
}

func TestHumanAndBotUsernameSuffixRules(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	if _, err := c.CreateUser(ctx, SystemActorID, "reserved_bot", "Reserved", "password123"); !errors.Is(err, ErrHumanLoginReservedForBot) {
		t.Fatalf("human _bot suffix err = %v", err)
	}
	owner, err := c.CreateUser(ctx, SystemActorID, "suffix-owner", "Suffix Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}
	if _, err := c.UpdateUserLogin(ctx, owner.GetId(), "human_bot"); !errors.Is(err, ErrHumanLoginReservedForBot) {
		t.Fatalf("human rename to _bot err = %v, want ErrHumanLoginReservedForBot", err)
	}
	if _, err := c.CreateBot(ctx, owner.GetId(), "missing-suffix", "Missing Suffix"); !errors.Is(err, ErrBotLoginSuffixRequired) {
		t.Fatalf("bot missing suffix err = %v", err)
	}
	uppercase, err := c.CreateBot(ctx, owner.GetId(), "uppercase_BOT", "Uppercase Bot")
	if err != nil {
		t.Fatalf("case-insensitive suffix CreateBot: %v", err)
	}
	invalidLogin := "lost-suffix"
	if _, err := c.UpdateBot(ctx, owner.GetId(), uppercase.User.GetId(), &invalidLogin, nil); !errors.Is(err, ErrBotLoginSuffixRequired) {
		t.Fatalf("bot rename without suffix err = %v, want ErrBotLoginSuffixRequired", err)
	}
}

package core

import (
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

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
	if bot.User.GetAccountKind() != corev1.UserAccountKind_USER_ACCOUNT_KIND_BOT || bot.OwnerUserID != owner.GetId() {
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

	if decision, err := c.PermResolver().Resolve(ctx, bot.User.GetId(), KindChannel, "", PermMessagePost); err != nil || decision != DecisionDeny {
		t.Fatalf("unconfigured bot message.post = %s, %v; want deny", decision, err)
	}
	if allowed, err := c.CanCreateBots(ctx, bot.User.GetId()); err != nil || allowed {
		t.Fatalf("bot CanCreateBots = %v, %v; want false", allowed, err)
	}
	if _, err := c.SetBotPermission(ctx, owner.GetId(), bot.User.GetId(), PermissionTargetScope{Kind: MatrixScopeServer}, PermBotCreate, PermissionStateAllow); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("delegate bot.create err = %v, want ErrInvalidArgument", err)
	}
	if err := c.AssignServerRoleToExistingUser(ctx, SystemActorID, bot.User.GetId(), RoleAdmin); !errors.Is(err, ErrHumanAccountRequired) {
		t.Fatalf("assign bot role err = %v, want ErrHumanAccountRequired", err)
	}

	cell, err := c.SetBotPermission(ctx, owner.GetId(), bot.User.GetId(), PermissionTargetScope{Kind: MatrixScopeServer}, PermMessagePost, PermissionStateAllow)
	if err != nil {
		t.Fatalf("SetBotPermission allow: %v", err)
	}
	if !cell.OwnerGranted || !cell.EffectiveGranted || cell.Delegated != MatrixDecisionAllow {
		t.Fatalf("allowed cell = %+v", cell)
	}
	if decision, err := c.PermResolver().Resolve(ctx, bot.User.GetId(), KindChannel, "", PermMessagePost); err != nil || decision != DecisionAllow {
		t.Fatalf("configured bot message.post = %s, %v; want allow", decision, err)
	}

	if err := c.DenyServerPermission(ctx, SystemActorID, RoleEveryone, PermMessagePost); err != nil {
		t.Fatalf("deny owner permission: %v", err)
	}
	if decision, err := c.PermResolver().Resolve(ctx, bot.User.GetId(), KindChannel, "", PermMessagePost); err != nil || decision != DecisionDeny {
		t.Fatalf("owner-capped bot message.post = %s, %v; want deny", decision, err)
	}
	matrix, err := c.GetBotPermissionMatrix(ctx, owner.GetId(), bot.User.GetId())
	if err != nil {
		t.Fatalf("GetBotPermissionMatrix: %v", err)
	}
	var found *BotPermissionCell
	for i := range matrix.Cells {
		if matrix.Cells[i].ScopeID == "server" && matrix.Cells[i].Permission == string(PermMessagePost) {
			found = &matrix.Cells[i]
			break
		}
	}
	if found == nil || found.Configured != MatrixDecisionAllow || found.Delegated != MatrixDecisionAllow || found.OwnerGranted || found.EffectiveGranted {
		t.Fatalf("dormant grant cell = %+v", found)
	}
	if _, err := c.SetBotPermission(ctx, owner.GetId(), bot.User.GetId(), PermissionTargetScope{Kind: MatrixScopeServer}, PermRoomCreate, PermissionStateAllow); !errors.Is(err, ErrBotOwnerPermissionCeiling) {
		t.Fatalf("over-ceiling grant err = %v, want ErrBotOwnerPermissionCeiling", err)
	}
}

func TestBotPermissionMatrixDoesNotDiscloseHiddenRoomsOrGroups(t *testing.T) {
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

	matrix, err := c.GetBotPermissionMatrix(ctx, owner.GetId(), bot.User.GetId())
	if err != nil {
		t.Fatalf("GetBotPermissionMatrix: %v", err)
	}
	for _, scope := range matrix.Scopes {
		if scope.ID == "group:"+group.GetId() || scope.ID == "room:"+room.GetId() ||
			scope.Label == group.GetName() || scope.Label == room.GetName() {
			t.Fatalf("hidden scope leaked through bot matrix: %+v", scope)
		}
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

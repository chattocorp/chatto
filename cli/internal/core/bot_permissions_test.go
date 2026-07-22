package core

import (
	"context"
	"errors"
	"testing"

	"hmans.de/chatto/internal/events"
)

func TestBotOwnerBoundedAuthorization(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "botowner", "Bot Owner", "password123")
	if err != nil {
		t.Fatalf("CreateUser owner: %v", err)
	}

	if _, err := c.CreateBotAs(ctx, owner.GetId(), "blocked_bot", "Blocked Bot", "Test bot"); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("CreateBotAs without bot.create = %v, want permission denied", err)
	}
	if err := c.GrantUserPermission(ctx, SystemActorID, owner.GetId(), PermBotCreate); err != nil {
		t.Fatalf("grant bot.create: %v", err)
	}
	bot, err := c.CreateBotAs(ctx, owner.GetId(), "bounded_bot", "Bounded Bot", "Test bot")
	if err != nil {
		t.Fatalf("CreateBotAs: %v", err)
	}

	assertPermissionDecision(t, c, ctx, bot.GetId(), PermMessagePost, DecisionAllow)
	if err := c.SetBotPermission(ctx, owner.GetId(), bot.GetId(), ScopeServer, "", PermMessagePost, DecisionDeny); err != nil {
		t.Fatalf("deny bot message.post: %v", err)
	}
	assertPermissionDecision(t, c, ctx, bot.GetId(), PermMessagePost, DecisionDeny)

	if err := c.SetBotPermission(ctx, owner.GetId(), bot.GetId(), ScopeServer, "", PermMessagePost, DecisionAllow); err != nil {
		t.Fatalf("allow bot message.post: %v", err)
	}
	if err := c.DenyUserPermission(ctx, SystemActorID, owner.GetId(), PermMessagePost); err != nil {
		t.Fatalf("deny owner message.post: %v", err)
	}
	assertPermissionDecision(t, c, ctx, bot.GetId(), PermMessagePost, DecisionDeny)

	explanation, err := c.permissionResolver.ExplainServerPermission(ctx, bot.GetId(), PermMessagePost)
	if err != nil {
		t.Fatalf("ExplainServerPermission: %v", err)
	}
	if explanation.OwnerCeiling == nil || explanation.OwnerCeiling.State != DecisionDeny || explanation.DecidedByRole != "@bot-owner-ceiling" {
		t.Fatalf("owner-bounded explanation = %+v", explanation)
	}
	if allowed, err := c.CanStartDM(ctx, bot.GetId()); err != nil || allowed {
		t.Fatalf("CanStartDM while owner denied = %v, %v; want false", allowed, err)
	}
	if err := c.ClearUserPermissionState(ctx, SystemActorID, owner.GetId(), PermMessagePost); err != nil {
		t.Fatal(err)
	}
	if allowed, err := c.CanStartDM(ctx, bot.GetId()); err != nil || !allowed {
		t.Fatalf("CanStartDM after owner restored = %v, %v; want true", allowed, err)
	}
}

func TestUpdateBotPersistsEncryptedProfilePatch(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "updatebotowner", "Update Bot Owner", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.GrantUserPermission(ctx, SystemActorID, owner.GetId(), PermBotCreate); err != nil {
		t.Fatal(err)
	}
	bot, err := c.CreateBotAs(ctx, owner.GetId(), "before_bot", "Before Bot", "Before description")
	if err != nil {
		t.Fatal(err)
	}
	login, displayName, description := "after_bot", "After Bot", "After description"
	updated, err := c.UpdateBot(ctx, owner.GetId(), bot.GetId(), BotUpdateInput{
		Login: &login, DisplayName: &displayName, Description: &description,
	})
	if err != nil {
		t.Fatalf("UpdateBot: %v", err)
	}
	if updated.GetLogin() != login || updated.GetDisplayName() != displayName || updated.GetBot().GetDescription() != description {
		t.Fatalf("updated bot = %+v", updated)
	}
	eventsFound, _, err := c.EventPublisher.SubjectEvents(ctx, events.UserAggregate(bot.GetId()).Subject(events.EventBotDescriptionChanged))
	if err != nil {
		t.Fatal(err)
	}
	if len(eventsFound) != 1 || eventsFound[0].GetBotDescriptionChanged().GetEncryptedDescription() == nil {
		t.Fatalf("bot description events = %+v", eventsFound)
	}
}

func TestBotPermissionManagementBoundsAndAdministration(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	owner, err := c.CreateUser(ctx, SystemActorID, "permissionowner", "Permission Owner", "password123")
	if err != nil {
		t.Fatal(err)
	}
	other, err := c.CreateUser(ctx, SystemActorID, "otherhuman", "Other Human", "password123")
	if err != nil {
		t.Fatal(err)
	}
	admin, err := c.CreateUser(ctx, SystemActorID, "botadmin", "Bot Admin", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.GrantUserPermission(ctx, SystemActorID, owner.GetId(), PermBotCreate); err != nil {
		t.Fatal(err)
	}
	bot, err := c.CreateBotAs(ctx, owner.GetId(), "managed_bot", "Managed Bot", "Test bot")
	if err != nil {
		t.Fatal(err)
	}
	matrix, err := c.GetBotPermissionMatrix(ctx, owner.GetId(), bot.GetId())
	if err != nil {
		t.Fatalf("owner GetBotPermissionMatrix: %v", err)
	}
	if matrix.BotID != bot.GetId() || len(matrix.Cells) == 0 {
		t.Fatalf("bot permission matrix = %+v", matrix)
	}
	if _, err := c.GetBotPermissionMatrix(ctx, other.GetId(), bot.GetId()); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("other GetBotPermissionMatrix = %v, want permission denied", err)
	}

	if err := c.SetBotPermission(ctx, owner.GetId(), bot.GetId(), ScopeServer, "", PermRoomManage, DecisionAllow); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("owner grant beyond ceiling = %v, want permission denied", err)
	}
	if err := c.GrantUserPermission(ctx, SystemActorID, owner.GetId(), PermRoomManage); err != nil {
		t.Fatal(err)
	}
	if err := c.SetBotPermission(ctx, owner.GetId(), bot.GetId(), ScopeServer, "", PermRoomManage, DecisionAllow); err != nil {
		t.Fatalf("owner grant within ceiling: %v", err)
	}
	assertPermissionDecision(t, c, ctx, bot.GetId(), PermRoomManage, DecisionAllow)
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "bot-scope-test", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := c.DenyUserRoomPermission(ctx, SystemActorID, room.GetId(), owner.GetId(), PermRoomManage); err != nil {
		t.Fatal(err)
	}
	matrix, err = c.GetBotPermissionMatrix(ctx, owner.GetId(), bot.GetId())
	if err != nil {
		t.Fatalf("GetBotPermissionMatrix after owner deny: %v", err)
	}
	foundOwnerCeiling := false
	for _, cell := range matrix.Cells {
		if cell.ScopeID == "room:"+room.GetId() && cell.Permission == string(PermRoomManage) {
			foundOwnerCeiling = true
			if cell.OwnerAllowed || cell.Effective != MatrixDecisionDeny {
				t.Fatalf("owner-bounded matrix cell = %+v, want unavailable deny", cell)
			}
		}
	}
	if !foundOwnerCeiling {
		t.Fatal("owner-bounded room permission cell not found")
	}
	if err := c.SetBotPermission(ctx, owner.GetId(), bot.GetId(), ScopeRoom, room.GetId(), PermRoomManage, DecisionAllow); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("room grant beyond owner ceiling = %v, want permission denied", err)
	}
	roomDecision, err := c.ResolveUserPermission(ctx, bot.GetId(), KindChannel, room.GetId(), PermRoomManage)
	if err != nil || roomDecision != DecisionDeny {
		t.Fatalf("room decision under owner deny = %s, %v; want deny", roomDecision, err)
	}

	if allowed, err := c.CanManageBot(ctx, other.GetId(), bot.GetId()); err != nil || allowed {
		t.Fatalf("other CanManageBot = %v, %v; want false", allowed, err)
	}
	if err := c.AssignServerRole(ctx, SystemActorID, admin.GetId(), RoleAdmin); err != nil {
		t.Fatal(err)
	}
	if allowed, err := c.CanManageBot(ctx, admin.GetId(), bot.GetId()); err != nil || !allowed {
		t.Fatalf("admin CanManageBot = %v, %v; want true", allowed, err)
	}
	if err := c.SetBotPermission(ctx, admin.GetId(), bot.GetId(), ScopeServer, "", PermRoomManage, DecisionDeny); err != nil {
		t.Fatalf("admin restrict bot: %v", err)
	}
	assertPermissionDecision(t, c, ctx, bot.GetId(), PermRoomManage, DecisionDeny)

	if err := c.AssignServerRole(ctx, owner.GetId(), bot.GetId(), RoleModerator); !errors.Is(err, ErrPermissionDenied) {
		t.Fatalf("bot owner role assignment = %v, want permission denied", err)
	}
	if err := c.AssignServerRole(ctx, admin.GetId(), bot.GetId(), RoleModerator); err != nil {
		t.Fatalf("admin role assignment: %v", err)
	}
}

func assertPermissionDecision(t *testing.T, c *ChattoCore, ctx context.Context, userID string, perm Permission, want DecisionKind) {
	t.Helper()
	got, err := c.ResolveUserPermission(ctx, userID, KindChannel, "", perm)
	if err != nil {
		t.Fatalf("ResolveUserPermission(%s): %v", perm, err)
	}
	if got != want {
		t.Fatalf("ResolveUserPermission(%s) = %s, want %s", perm, got, want)
	}
}

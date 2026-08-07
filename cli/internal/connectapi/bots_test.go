package connectapi

import (
	"testing"

	"connectrpc.com/connect"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestBotServiceLifecycleAndVisibility(t *testing.T) {
	env := newConnectAPITestEnv(t)
	ownerCtx := withCaller(env.ctx, env.viewer)

	if _, err := env.bots.CreateBot(ownerCtx, connect.NewRequest(&apiv1.CreateBotRequest{
		Login: "blocked_bot", DisplayName: "Blocked Bot", Description: "Blocked",
	})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("CreateBot without bot.create code = %v, want permission_denied", connect.CodeOf(err))
	}
	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.GetId(), core.PermBotCreate); err != nil {
		t.Fatalf("grant bot.create: %v", err)
	}
	created, err := env.bots.CreateBot(ownerCtx, connect.NewRequest(&apiv1.CreateBotRequest{
		Login:       "helper_bot",
		DisplayName: "Helper Bot",
		Description: "Answers questions without sending data to third parties.",
	}))
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	botID := created.Msg.GetBot().GetUser().GetId()
	if created.Msg.GetBot().GetUser().GetBot().GetOwnerId() != env.viewer.GetId() || created.Msg.GetBot().GetUser().GetBot().GetDescription() == "" {
		t.Fatalf("created bot = %+v", created.Msg.GetBot())
	}
	if created.Msg.GetApiKey() == "" || created.Msg.GetBot().GetApiKey().GetCreatedAt() == nil {
		t.Fatalf("CreateBot did not issue an API key: %+v", created.Msg)
	}
	matrix, err := env.bots.GetBotPermissionMatrix(ownerCtx, connect.NewRequest(&apiv1.GetBotPermissionMatrixRequest{BotId: botID}))
	if err != nil {
		t.Fatalf("GetBotPermissionMatrix: %v", err)
	}
	if matrix.Msg.GetMatrix().GetBotId() != botID || len(matrix.Msg.GetMatrix().GetCells()) == 0 {
		t.Fatalf("bot permission matrix = %+v", matrix.Msg.GetMatrix())
	}
	setPermission, err := env.bots.SetBotPermission(ownerCtx, connect.NewRequest(&apiv1.SetBotPermissionRequest{
		BotId:      botID,
		Scope:      &apiv1.BotPermissionScope{Kind: apiv1.BotPermissionScopeKind_BOT_PERMISSION_SCOPE_KIND_SERVER},
		Permission: string(core.PermMessagePost),
		Decision:   apiv1.BotPermissionDecision_BOT_PERMISSION_DECISION_DENY,
	}))
	if err != nil {
		t.Fatalf("SetBotPermission: %v", err)
	}
	if update := setPermission.Msg.GetUpdate(); update.GetPermission() != string(core.PermMessagePost) || update.GetDecision() != apiv1.BotPermissionDecision_BOT_PERMISSION_DECISION_DENY {
		t.Fatalf("SetBotPermission update = %+v", update)
	}

	description := "Answers questions and stores no conversation content."
	displayName := "Updated Helper"
	updated, err := env.bots.UpdateBot(ownerCtx, connect.NewRequest(&apiv1.UpdateBotRequest{
		BotId: botID, DisplayName: &displayName, Description: &description,
	}))
	if err != nil {
		t.Fatalf("UpdateBot: %v", err)
	}
	if updated.Msg.GetBot().GetUser().GetDisplayName() != displayName || updated.Msg.GetBot().GetUser().GetBot().GetDescription() != description {
		t.Fatalf("updated bot = %+v", updated.Msg.GetBot())
	}
	rotated, err := env.bots.RotateBotAPIKey(ownerCtx, connect.NewRequest(&apiv1.RotateBotAPIKeyRequest{BotId: botID}))
	if err != nil {
		t.Fatalf("RotateBotAPIKey: %v", err)
	}
	if rotated.Msg.GetApiKey() == "" || rotated.Msg.GetBot().GetApiKey().GetCreatedAt() == nil {
		t.Fatalf("RotateBotAPIKey response = %+v", rotated.Msg)
	}

	list, err := env.bots.ListBots(ownerCtx, connect.NewRequest(&apiv1.ListBotsRequest{Search: "updated"}))
	if err != nil {
		t.Fatalf("ListBots: %v", err)
	}
	if len(list.Msg.GetBots()) != 1 || list.Msg.GetBots()[0].GetUser().GetId() != botID || list.Msg.GetPage().GetTotalCount() != 1 {
		t.Fatalf("ListBots = %+v", list.Msg)
	}

	other, err := env.core.CreateUser(env.ctx, core.SystemActorID, "bot-api-other", "Bot API Other", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.bots.GetBot(withCaller(env.ctx, other), connect.NewRequest(&apiv1.GetBotRequest{BotId: botID})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("other GetBot code = %v, want permission_denied", connect.CodeOf(err))
	}
	if _, err := env.bots.GetBotPermissionMatrix(withCaller(env.ctx, other), connect.NewRequest(&apiv1.GetBotPermissionMatrixRequest{BotId: botID})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("other GetBotPermissionMatrix code = %v, want permission_denied", connect.CodeOf(err))
	}
	if _, err := env.bots.RotateBotAPIKey(withCaller(env.ctx, other), connect.NewRequest(&apiv1.RotateBotAPIKeyRequest{BotId: botID})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("other RotateBotAPIKey code = %v, want permission_denied", connect.CodeOf(err))
	}
	batch, err := env.bots.BatchGetBots(withCaller(env.ctx, other), connect.NewRequest(&apiv1.BatchGetBotsRequest{BotIds: []string{botID, "missing"}}))
	if err != nil {
		t.Fatalf("other BatchGetBots: %v", err)
	}
	if len(batch.Msg.GetBots()) != 0 {
		t.Fatalf("other BatchGetBots returned inaccessible bots: %+v", batch.Msg.GetBots())
	}

	admin, err := env.core.CreateUser(env.ctx, core.SystemActorID, "bot-api-admin", "Bot API Admin", "password123")
	if err != nil {
		t.Fatal(err)
	}
	if err := env.core.AssignServerRole(env.ctx, core.SystemActorID, admin.GetId(), core.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	adminList, err := env.bots.ListBots(withCaller(env.ctx, admin), connect.NewRequest(&apiv1.ListBotsRequest{}))
	if err != nil {
		t.Fatalf("admin ListBots: %v", err)
	}
	if len(adminList.Msg.GetBots()) != 1 || adminList.Msg.GetBots()[0].GetUser().GetId() != botID {
		t.Fatalf("admin ListBots = %+v, want manageable bot", adminList.Msg)
	}
	ownedList, err := env.bots.ListBots(withCaller(env.ctx, admin), connect.NewRequest(&apiv1.ListBotsRequest{OwnedByCallerOnly: true}))
	if err != nil {
		t.Fatalf("admin owned ListBots: %v", err)
	}
	if len(ownedList.Msg.GetBots()) != 0 || ownedList.Msg.GetPage().GetTotalCount() != 0 {
		t.Fatalf("admin owned ListBots = %+v, want no bots owned by caller", ownedList.Msg)
	}
	if _, err := env.bots.GetBot(withCaller(env.ctx, admin), connect.NewRequest(&apiv1.GetBotRequest{BotId: botID})); err != nil {
		t.Fatalf("admin GetBot: %v", err)
	}
	if _, err := env.bots.RotateBotAPIKey(withCaller(env.ctx, admin), connect.NewRequest(&apiv1.RotateBotAPIKeyRequest{BotId: botID})); connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("admin RotateBotAPIKey code = %v, want permission_denied", connect.CodeOf(err))
	}
	revoked, err := env.bots.RevokeBotAPIKey(withCaller(env.ctx, admin), connect.NewRequest(&apiv1.RevokeBotAPIKeyRequest{BotId: botID}))
	if err != nil {
		t.Fatalf("admin RevokeBotAPIKey: %v", err)
	}
	if revoked.Msg.GetBot().GetApiKey() != nil {
		t.Fatalf("revoked bot API key metadata = %+v, want absent", revoked.Msg.GetBot().GetApiKey())
	}

	deleted, err := env.bots.DeleteBot(ownerCtx, connect.NewRequest(&apiv1.DeleteBotRequest{BotId: botID}))
	if err != nil {
		t.Fatalf("DeleteBot: %v", err)
	}
	if !deleted.Msg.GetDeleted() {
		t.Fatal("DeleteBot deleted = false")
	}
	if _, err := env.bots.GetBot(ownerCtx, connect.NewRequest(&apiv1.GetBotRequest{BotId: botID})); connect.CodeOf(err) != connect.CodeNotFound {
		t.Fatalf("GetBot after delete code = %v, want not_found", connect.CodeOf(err))
	}
}

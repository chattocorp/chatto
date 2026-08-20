package connectapi

import (
	"testing"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestBotServiceLifecycleAndPermissionMatrix(t *testing.T) {
	env := newConnectAPITestEnv(t)
	service := &botService{api: env.api}
	ctx := withCaller(env.ctx, env.viewer)

	created, err := service.CreateBot(ctx, connect.NewRequest(&apiv1.CreateBotRequest{Login: "connect_bot", DisplayName: "Connect Bot"}))
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	bot := created.Msg.GetBot()
	if bot.GetUser().GetAccountKind() != apiv1.UserAccountKind_USER_ACCOUNT_KIND_BOT || bot.GetOwnerUserId() != env.viewer.GetId() {
		t.Fatalf("created bot = %+v", bot)
	}
	if created.Msg.GetApiKey() == "" {
		t.Fatal("CreateBot API key is empty")
	}

	listed, err := service.ListBots(ctx, connect.NewRequest(&apiv1.ListBotsRequest{}))
	if err != nil || len(listed.Msg.GetBots()) != 1 || listed.Msg.GetPage().GetTotalCount() != 1 {
		t.Fatalf("ListBots = %+v, %v", listed, err)
	}
	got, err := service.GetBot(ctx, connect.NewRequest(&apiv1.GetBotRequest{BotUserId: bot.GetUser().GetId()}))
	if err != nil || got.Msg.GetBot().GetUser().GetLogin() != "connect_bot" {
		t.Fatalf("GetBot = %+v, %v", got, err)
	}

	set, err := service.SetBotPermission(ctx, connect.NewRequest(&apiv1.SetBotPermissionRequest{
		BotUserId: bot.GetUser().GetId(), Permission: string(core.PermMessagePost),
		Scope:    &apiv1.BotPermissionScope{Kind: apiv1.BotPermissionScopeKind_BOT_PERMISSION_SCOPE_KIND_SERVER},
		Decision: apiv1.BotPermissionDecision_BOT_PERMISSION_DECISION_ALLOW,
	}))
	if err != nil || !set.Msg.GetCell().GetOwnerGranted() || !set.Msg.GetCell().GetEffectiveGranted() {
		t.Fatalf("SetBotPermission = %+v, %v", set, err)
	}
	matrix, err := service.GetBotPermissionMatrix(ctx, connect.NewRequest(&apiv1.GetBotPermissionMatrixRequest{BotUserId: bot.GetUser().GetId()}))
	if err != nil || matrix.Msg.GetMatrix().GetBotUserId() != bot.GetUser().GetId() {
		t.Fatalf("GetBotPermissionMatrix = %+v, %v", matrix, err)
	}
	botCore, err := env.core.GetUser(env.ctx, bot.GetUser().GetId())
	if err != nil {
		t.Fatalf("GetUser bot: %v", err)
	}
	viewer, err := env.viewerService.GetViewer(withCaller(env.ctx, botCore), connect.NewRequest(&apiv1.GetViewerRequest{}))
	if err != nil {
		t.Fatalf("GetViewer bot: %v", err)
	}
	if profile := viewer.Msg.GetUser().GetProfile(); profile.GetAccountKind() != apiv1.UserAccountKind_USER_ACCOUNT_KIND_BOT {
		t.Fatalf("bot viewer profile = %+v", profile)
	}
	if !apiPermissionGranted(viewer.Msg.GetViewerPermissions().GetPermissions(), string(core.PermMessagePost)) {
		t.Fatal("explicit bot message.post permission is not granted")
	}
	for _, permission := range []core.Permission{core.PermRoomList, core.PermBotCreate, core.PermBotManage} {
		if apiPermissionGranted(viewer.Msg.GetViewerPermissions().GetPermissions(), string(permission)) {
			t.Fatalf("bot unexpectedly granted %s", permission)
		}
	}
	if apiCapabilityGranted(viewer.Msg.GetCapabilities().GetGrants(), viewerCapabilityAdminView) {
		t.Fatal("bot unexpectedly granted admin.view capability")
	}

	rotated, err := service.RotateBotApiKey(ctx, connect.NewRequest(&apiv1.RotateBotApiKeyRequest{BotUserId: bot.GetUser().GetId()}))
	if err != nil || rotated.Msg.GetApiKey() == "" || rotated.Msg.GetApiKey() == created.Msg.GetApiKey() {
		t.Fatalf("RotateBotApiKey = %+v, %v", rotated, err)
	}

	if _, err := service.ListBots(withCaller(env.ctx, botCore), connect.NewRequest(&apiv1.ListBotsRequest{})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("bot caller ListBots code = %v, want failed precondition", connect.CodeOf(err))
	}

	deleted, err := service.DeleteBot(ctx, connect.NewRequest(&apiv1.DeleteBotRequest{BotUserId: bot.GetUser().GetId()}))
	if err != nil || !deleted.Msg.GetDeleted() {
		t.Fatalf("DeleteBot = %+v, %v", deleted, err)
	}
}

func TestBotServiceRejectsInvalidSuffixAndOwnerCeiling(t *testing.T) {
	env := newConnectAPITestEnv(t)
	service := &botService{api: env.api}
	ctx := withCaller(env.ctx, env.viewer)

	if _, err := service.CreateBot(ctx, connect.NewRequest(&apiv1.CreateBotRequest{Login: "no-suffix", DisplayName: "No Suffix"})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("CreateBot invalid suffix code = %v", connect.CodeOf(err))
	}
	created, err := service.CreateBot(ctx, connect.NewRequest(&apiv1.CreateBotRequest{Login: "ceiling_bot", DisplayName: "Ceiling Bot"}))
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	_, err = service.SetBotPermission(ctx, connect.NewRequest(&apiv1.SetBotPermissionRequest{
		BotUserId: created.Msg.GetBot().GetUser().GetId(), Permission: string(core.PermRoomCreate),
		Scope:    &apiv1.BotPermissionScope{Kind: apiv1.BotPermissionScopeKind_BOT_PERMISSION_SCOPE_KIND_SERVER},
		Decision: apiv1.BotPermissionDecision_BOT_PERMISSION_DECISION_ALLOW,
	}))
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("over-ceiling code = %v, want failed precondition", connect.CodeOf(err))
	}
}

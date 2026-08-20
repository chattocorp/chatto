package connectapi

import (
	"testing"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	adminv1 "hmans.de/chatto/internal/pb/chatto/admin/v1"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestBotServiceLifecycleAndCanonicalPermissionMatrix(t *testing.T) {
	env := newConnectAPITestEnv(t)
	service := &botService{api: env.api}
	ctx := withCaller(env.ctx, env.viewer)

	created, err := service.CreateBot(ctx, connect.NewRequest(&apiv1.CreateBotRequest{Login: "connect_bot", DisplayName: "Connect Bot"}))
	if err != nil {
		t.Fatalf("CreateBot: %v", err)
	}
	bot := created.Msg.GetBot()
	if !bot.GetUser().GetIsBot() || bot.GetOwnerUserId() != env.viewer.GetId() {
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

	_, err = env.permissions.SetUserPermission(ctx, connect.NewRequest(&adminv1.SetUserPermissionRequest{
		UserId: bot.GetUser().GetId(), Permission: string(core.PermMessagePost),
		Scope:    &adminv1.PermissionScope{Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER},
		Decision: adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW,
	}))
	if err != nil {
		t.Fatalf("SetUserPermission: %v", err)
	}
	matrix, err := env.permissions.GetUserPermissionMatrix(ctx, connect.NewRequest(&adminv1.GetUserPermissionMatrixRequest{UserId: bot.GetUser().GetId()}))
	if err != nil || matrix.Msg.GetMatrix().GetUserId() != bot.GetUser().GetId() {
		t.Fatalf("GetUserPermissionMatrix = %+v, %v", matrix, err)
	}
	cell := findAPIPermissionCell(matrix.Msg.GetMatrix().GetCells(), "server", string(core.PermMessagePost))
	if cell == nil || cell.GetOverride() != adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW || cell.GetEffective() != adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW || cell.AllowPermitted == nil || !cell.GetAllowPermitted() {
		t.Fatalf("bot user permission matrix cell = %+v", cell)
	}
	botCore, err := env.core.GetUser(env.ctx, bot.GetUser().GetId())
	if err != nil {
		t.Fatalf("GetUser bot: %v", err)
	}
	viewer, err := env.viewerService.GetViewer(withCaller(env.ctx, botCore), connect.NewRequest(&apiv1.GetViewerRequest{}))
	if err != nil {
		t.Fatalf("GetViewer bot: %v", err)
	}
	if profile := viewer.Msg.GetUser().GetProfile(); !profile.GetIsBot() {
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
	_, err = env.permissions.SetUserPermission(ctx, connect.NewRequest(&adminv1.SetUserPermissionRequest{
		UserId: created.Msg.GetBot().GetUser().GetId(), Permission: string(core.PermRoomCreate),
		Scope:    &adminv1.PermissionScope{Kind: adminv1.PermissionScopeKind_PERMISSION_SCOPE_KIND_SERVER},
		Decision: adminv1.PermissionDecision_PERMISSION_DECISION_ALLOW,
	}))
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("over-ceiling code = %v, want failed precondition", connect.CodeOf(err))
	}
}

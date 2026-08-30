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
	if metadata := created.Msg.GetApiKeyMetadata(); metadata.GetId() == "" || metadata.GetName() != "Default key" || len(bot.GetApiKeys()) != 1 {
		t.Fatalf("created API-key metadata = %+v; keys = %+v", metadata, bot.GetApiKeys())
	}

	listed, err := service.ListBots(ctx, connect.NewRequest(&apiv1.ListBotsRequest{}))
	if err != nil || len(listed.Msg.GetBots()) != 1 || listed.Msg.GetPage().GetTotalCount() != 1 {
		t.Fatalf("ListBots = %+v, %v", listed, err)
	}
	got, err := service.GetBot(ctx, connect.NewRequest(&apiv1.GetBotRequest{BotUserId: bot.GetUser().GetId()}))
	if err != nil || got.Msg.GetBot().GetUser().GetLogin() != "connect_bot" {
		t.Fatalf("GetBot = %+v, %v", got, err)
	}
	firstKey, err := service.CreateBotApiKey(ctx, connect.NewRequest(&apiv1.CreateBotApiKeyRequest{
		BotUserId: bot.GetUser().GetId(), Name: "CI",
	}))
	if err != nil || firstKey.Msg.GetApiKey() == "" || firstKey.Msg.GetApiKeyMetadata().GetName() != "CI" || len(firstKey.Msg.GetBot().GetApiKeys()) != 2 {
		t.Fatalf("CreateBotApiKey first = %+v, %v", firstKey, err)
	}
	secondKey, err := service.CreateBotApiKey(ctx, connect.NewRequest(&apiv1.CreateBotApiKeyRequest{
		BotUserId: bot.GetUser().GetId(), Name: "CI",
	}))
	if err != nil || secondKey.Msg.GetApiKey() == "" || secondKey.Msg.GetApiKeyMetadata().GetId() == firstKey.Msg.GetApiKeyMetadata().GetId() || len(secondKey.Msg.GetBot().GetApiKeys()) != 3 {
		t.Fatalf("CreateBotApiKey second = %+v, %v", secondKey, err)
	}
	revokedKey, err := service.RevokeBotApiKey(ctx, connect.NewRequest(&apiv1.RevokeBotApiKeyRequest{
		BotUserId: bot.GetUser().GetId(), KeyId: firstKey.Msg.GetApiKeyMetadata().GetId(),
	}))
	if err != nil || len(revokedKey.Msg.GetBot().GetApiKeys()) != 2 {
		t.Fatalf("RevokeBotApiKey = %+v, %v", revokedKey, err)
	}
	if _, err := env.core.ValidateBotAPIKey(env.ctx, firstKey.Msg.GetApiKey()); err == nil {
		t.Fatal("revoked named API key still authenticates")
	}
	if _, err := env.core.ValidateBotAPIKey(env.ctx, secondKey.Msg.GetApiKey()); err != nil {
		t.Fatalf("unrelated named API key: %v", err)
	}
	env.api.config.Webserver.URL = "https://configured.example"
	webhookCtx := WithRequestBaseURL(ctx, "https://spoofed.example")
	webhook, err := service.CreateBotIncomingWebhook(webhookCtx, connect.NewRequest(&apiv1.CreateBotIncomingWebhookRequest{BotUserId: bot.GetUser().GetId(), Name: "CI"}))
	if err != nil || len(webhook.Msg.GetBot().GetIncomingWebhooks()) != 1 || webhook.Msg.GetWebhookUrl() == "" {
		t.Fatalf("CreateBotIncomingWebhook = %+v, %v", webhook, err)
	}
	webhookID := webhook.Msg.GetBot().GetIncomingWebhooks()[0].GetId()
	if got := webhook.Msg.GetBot().GetIncomingWebhooks()[0].GetLastUsedState(); got != apiv1.CredentialLastUsedState_CREDENTIAL_LAST_USED_STATE_NO_USE_RECORDED {
		t.Fatalf("new webhook last-used state = %v", got)
	}
	got, err = service.GetBot(ctx, connect.NewRequest(&apiv1.GetBotRequest{BotUserId: bot.GetUser().GetId()}))
	if err != nil {
		t.Fatalf("GetBot after webhook creation: %v", err)
	}
	if state := got.Msg.GetBot().GetIncomingWebhooks()[0].GetLastUsedState(); state != apiv1.CredentialLastUsedState_CREDENTIAL_LAST_USED_STATE_NO_USE_RECORDED {
		t.Fatalf("missing webhook usage record state = %v", state)
	}
	if wantPrefix := "https://configured.example/webhooks/incoming/cht_IW_"; len(webhook.Msg.GetWebhookUrl()) < len(wantPrefix) || webhook.Msg.GetWebhookUrl()[:len(wantPrefix)] != wantPrefix {
		t.Fatalf("webhook URL = %q, want prefix %q", webhook.Msg.GetWebhookUrl(), wantPrefix)
	}
	secondWebhook, err := service.CreateBotIncomingWebhook(webhookCtx, connect.NewRequest(&apiv1.CreateBotIncomingWebhookRequest{BotUserId: bot.GetUser().GetId(), Name: "Deployments"}))
	if err != nil || len(secondWebhook.Msg.GetBot().GetIncomingWebhooks()) != 2 {
		t.Fatalf("second CreateBotIncomingWebhook = %+v, %v", secondWebhook, err)
	}
	for _, item := range secondWebhook.Msg.GetBot().GetIncomingWebhooks() {
		want := apiv1.CredentialLastUsedState_CREDENTIAL_LAST_USED_STATE_UNSPECIFIED
		if item.GetName() == "Deployments" {
			want = apiv1.CredentialLastUsedState_CREDENTIAL_LAST_USED_STATE_NO_USE_RECORDED
		}
		if item.GetLastUsedState() != want {
			t.Fatalf("second creation webhook %q state = %v, want %v", item.GetId(), item.GetLastUsedState(), want)
		}
	}
	revokedWebhook, err := service.RevokeBotIncomingWebhook(ctx, connect.NewRequest(&apiv1.RevokeBotIncomingWebhookRequest{BotUserId: bot.GetUser().GetId(), WebhookId: webhookID}))
	if err != nil || len(revokedWebhook.Msg.GetBot().GetIncomingWebhooks()) != 1 {
		t.Fatalf("RevokeBotIncomingWebhook = %+v, %v", revokedWebhook, err)
	}
	botCore, err := env.core.GetUser(env.ctx, bot.GetUser().GetId())
	if err != nil {
		t.Fatalf("GetUser bot: %v", err)
	}
	updated, err := env.account.UpdateProfile(withCaller(env.ctx, botCore), connect.NewRequest(&apiv1.UpdateProfileRequest{
		Login:       stringPtr("updated_connect_bot"),
		DisplayName: stringPtr("Updated Connect Bot"),
		Bio:         stringPtr("**Build helper**"),
	}))
	if err != nil {
		t.Fatalf("bot UpdateProfile: %v", err)
	}
	if user := updated.Msg.GetUser(); user.GetLogin() != "updated_connect_bot" || user.GetDisplayName() != "Updated Connect Bot" || user.GetBio() != "**Build helper**" {
		t.Fatalf("updated bot user = %+v", user)
	}
	recipient, err := env.core.CreateUser(env.ctx, core.SystemActorID, "connect-recipient", "Connect Recipient", "password123")
	if err != nil {
		t.Fatalf("CreateUser recipient: %v", err)
	}
	_, err = service.ReassignBotOwner(ctx, connect.NewRequest(&apiv1.ReassignBotOwnerRequest{
		BotUserId: bot.GetUser().GetId(), OwnerUserId: recipient.GetId(),
	}))
	if connect.CodeOf(err) != connect.CodePermissionDenied {
		t.Fatalf("owner-only ReassignBotOwner code = %v, want permission denied", connect.CodeOf(err))
	}
	if err := env.core.GrantUserPermission(env.ctx, core.SystemActorID, env.viewer.GetId(), core.PermBotManage); err != nil {
		t.Fatalf("GrantUserPermission bot.manage: %v", err)
	}
	reassigned, err := service.ReassignBotOwner(ctx, connect.NewRequest(&apiv1.ReassignBotOwnerRequest{
		BotUserId: bot.GetUser().GetId(), OwnerUserId: recipient.GetId(),
	}))
	if err != nil || reassigned.Msg.GetBot().GetOwnerUserId() != recipient.GetId() {
		t.Fatalf("ReassignBotOwner = %+v, %v", reassigned, err)
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
	if apiCapabilityGranted(viewer.Msg.GetCapabilities().GetGrants(), viewerCapabilityDMStart) {
		t.Fatal("bot unexpectedly granted dm.start capability")
	}

	if _, err := service.ListBots(withCaller(env.ctx, botCore), connect.NewRequest(&apiv1.ListBotsRequest{})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("bot caller ListBots code = %v, want failed precondition", connect.CodeOf(err))
	}
	if _, err := service.CreateBotIncomingWebhook(withCaller(env.ctx, botCore), connect.NewRequest(&apiv1.CreateBotIncomingWebhookRequest{BotUserId: bot.GetUser().GetId(), Name: "Denied"})); connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Fatalf("bot caller CreateBotIncomingWebhook code = %v, want failed precondition", connect.CodeOf(err))
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

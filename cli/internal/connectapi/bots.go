package connectapi

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

type botService struct{ api *API }

func (s *botService) ListBots(ctx context.Context, req *connect.Request[apiv1.ListBotsRequest]) (*connect.Response[apiv1.ListBotsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	bots, err := s.api.core.ListBots(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	search := strings.ToLower(strings.TrimSpace(req.Msg.GetSearch()))
	if search != "" {
		filtered := bots[:0]
		for _, bot := range bots {
			if strings.Contains(strings.ToLower(bot.User.GetLogin()), search) || strings.Contains(strings.ToLower(bot.User.GetDisplayName()), search) {
				filtered = append(filtered, bot)
			}
		}
		bots = filtered
	}
	limit, offset := apiPagination(req.Msg.GetPage(), 20, 100)
	page, total, more := apiSlicePage(bots, limit, offset)
	result, err := newBotAssembler(s.api).assemble(ctx, page)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.ListBotsResponse{Bots: result, Page: apiPageInfo(total, more)}), nil
}

func (s *botService) GetBot(ctx context.Context, req *connect.Request[apiv1.GetBotRequest]) (*connect.Response[apiv1.GetBotResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	bot, err := s.api.core.GetBot(ctx, caller.UserID, req.Msg.GetBotUserId())
	if err != nil {
		return nil, connectError(err)
	}
	mapped, err := newBotAssembler(s.api).assembleOne(ctx, bot)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.GetBotResponse{Bot: mapped}), nil
}

func (s *botService) BatchGetBots(ctx context.Context, req *connect.Request[apiv1.BatchGetBotsRequest]) (*connect.Response[apiv1.BatchGetBotsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	bots := make([]*core.Bot, 0, len(req.Msg.GetBotUserIds()))
	for _, id := range req.Msg.GetBotUserIds() {
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		bot, err := s.api.core.GetBot(ctx, caller.UserID, id)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) || errors.Is(err, core.ErrPermissionDenied) {
				continue
			}
			return nil, connectError(err)
		}
		bots = append(bots, bot)
	}
	result, err := newBotAssembler(s.api).assemble(ctx, bots)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.BatchGetBotsResponse{Bots: result}), nil
}

func (s *botService) CreateBot(ctx context.Context, req *connect.Request[apiv1.CreateBotRequest]) (*connect.Response[apiv1.CreateBotResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	bot, err := s.api.core.CreateBotWithAPIKeyName(ctx, caller.UserID, req.Msg.GetLogin(), req.Msg.GetDisplayName(), req.Msg.GetApiKeyName())
	if err != nil {
		return nil, connectError(err)
	}
	mapped, err := apiBot(ctx, s.api, bot)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.CreateBotResponse{
		Bot: mapped, ApiKey: bot.APIKey, ApiKeyMetadata: apiBotAPIKeyByID(mapped, bot.APIKeyID),
	}), nil
}

func (s *botService) DeleteBot(ctx context.Context, req *connect.Request[apiv1.DeleteBotRequest]) (*connect.Response[apiv1.DeleteBotResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	deleted, err := s.api.core.DeleteBot(ctx, caller.UserID, req.Msg.GetBotUserId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.DeleteBotResponse{Deleted: deleted}), nil
}

func (s *botService) CreateBotApiKey(ctx context.Context, req *connect.Request[apiv1.CreateBotApiKeyRequest]) (*connect.Response[apiv1.CreateBotApiKeyResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	issued, err := s.api.core.CreateBotAPIKey(ctx, caller.UserID, req.Msg.GetBotUserId(), req.Msg.GetName())
	if err != nil {
		return nil, connectError(err)
	}
	mapped, err := apiBot(ctx, s.api, issued.Bot)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.CreateBotApiKeyResponse{
		Bot: mapped, ApiKey: issued.Credential, ApiKeyMetadata: apiBotAPIKeyByID(mapped, issued.KeyID),
	}), nil
}

func (s *botService) RevokeBotApiKey(ctx context.Context, req *connect.Request[apiv1.RevokeBotApiKeyRequest]) (*connect.Response[apiv1.RevokeBotApiKeyResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	bot, err := s.api.core.RevokeBotAPIKey(ctx, caller.UserID, req.Msg.GetBotUserId(), req.Msg.GetKeyId())
	if err != nil {
		return nil, connectError(err)
	}
	mapped, err := newBotAssembler(s.api).assembleOne(ctx, bot)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.RevokeBotApiKeyResponse{Bot: mapped}), nil
}

func (s *botService) CreateBotIncomingWebhook(ctx context.Context, req *connect.Request[apiv1.CreateBotIncomingWebhookRequest]) (*connect.Response[apiv1.CreateBotIncomingWebhookResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	issued, err := s.api.core.CreateBotIncomingWebhook(ctx, caller.UserID, req.Msg.GetBotUserId(), req.Msg.GetName())
	if err != nil {
		return nil, connectError(err)
	}
	mapped, err := apiBot(ctx, s.api, issued.Bot)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.CreateBotIncomingWebhookResponse{
		Bot: mapped, WebhookUrl: s.incomingWebhookURL(ctx, issued.Credential),
	}), nil
}

func (s *botService) RevokeBotIncomingWebhook(ctx context.Context, req *connect.Request[apiv1.RevokeBotIncomingWebhookRequest]) (*connect.Response[apiv1.RevokeBotIncomingWebhookResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	bot, err := s.api.core.RevokeBotIncomingWebhook(ctx, caller.UserID, req.Msg.GetBotUserId(), req.Msg.GetWebhookId())
	if err != nil {
		return nil, connectError(err)
	}
	mapped, err := newBotAssembler(s.api).assembleOne(ctx, bot)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.RevokeBotIncomingWebhookResponse{Bot: mapped}), nil
}

func (s *botService) incomingWebhookURL(ctx context.Context, credential string) string {
	return s.api.absolutizeServerURL(ctx, "/webhooks/incoming/"+credential)
}

func (s *botService) ReassignBotOwner(ctx context.Context, req *connect.Request[apiv1.ReassignBotOwnerRequest]) (*connect.Response[apiv1.ReassignBotOwnerResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	bot, err := s.api.core.ReassignBotOwner(ctx, caller.UserID, req.Msg.GetBotUserId(), req.Msg.GetOwnerUserId())
	if err != nil {
		return nil, connectError(err)
	}
	mapped, err := newBotAssembler(s.api).assembleOne(ctx, bot)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.ReassignBotOwnerResponse{Bot: mapped}), nil
}

func apiBot(ctx context.Context, api *API, bot *core.Bot) (*apiv1.Bot, error) {
	user, err := requiredUserSummary(ctx, api, bot.User)
	if err != nil {
		return nil, err
	}
	out := &apiv1.Bot{User: user, OwnerUserId: bot.OwnerUserID, CreatedAt: bot.User.GetCreatedAt(), ApiKeyCreatedAt: timestamppb.New(bot.APIKeyCreatedAt)}
	for _, key := range bot.APIKeys {
		mapped := &apiv1.BotApiKey{Id: key.ID, Name: key.Name, CreatedAt: timestamppb.New(key.CreatedAt)}
		switch key.LastUsedState {
		case core.BotCredentialLastUsedNoUseRecorded:
			mapped.LastUsedState = apiv1.CredentialLastUsedState_CREDENTIAL_LAST_USED_STATE_NO_USE_RECORDED
		case core.BotCredentialLastUsedRecorded:
			mapped.LastUsedState = apiv1.CredentialLastUsedState_CREDENTIAL_LAST_USED_STATE_RECORDED
			mapped.LastUsedAt = timestamppb.New(key.LastUsedAt)
		case core.BotCredentialLastUsedUnavailable:
			mapped.LastUsedState = apiv1.CredentialLastUsedState_CREDENTIAL_LAST_USED_STATE_UNAVAILABLE
		}
		out.ApiKeys = append(out.ApiKeys, mapped)
	}
	for _, webhook := range bot.IncomingWebhooks {
		mapped := &apiv1.BotIncomingWebhook{
			Id: webhook.ID, Name: webhook.Name, CreatedAt: timestamppb.New(webhook.CreatedAt),
		}
		switch webhook.LastUsedState {
		case core.BotCredentialLastUsedNoUseRecorded:
			mapped.LastUsedState = apiv1.CredentialLastUsedState_CREDENTIAL_LAST_USED_STATE_NO_USE_RECORDED
		case core.BotCredentialLastUsedRecorded:
			mapped.LastUsedState = apiv1.CredentialLastUsedState_CREDENTIAL_LAST_USED_STATE_RECORDED
			mapped.LastUsedAt = timestamppb.New(webhook.LastUsedAt)
		case core.BotCredentialLastUsedUnavailable:
			mapped.LastUsedState = apiv1.CredentialLastUsedState_CREDENTIAL_LAST_USED_STATE_UNAVAILABLE
		}
		out.IncomingWebhooks = append(out.IncomingWebhooks, mapped)
	}
	return out, nil
}

func apiBotAPIKeyByID(bot *apiv1.Bot, keyID string) *apiv1.BotApiKey {
	if bot == nil {
		return nil
	}
	for _, key := range bot.GetApiKeys() {
		if key.GetId() == keyID {
			return key
		}
	}
	return nil
}

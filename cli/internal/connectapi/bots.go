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
	result := make([]*apiv1.Bot, 0, len(page))
	for _, bot := range page {
		mapped, err := apiBot(ctx, s.api, bot)
		if err != nil {
			return nil, err
		}
		result = append(result, mapped)
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
	mapped, err := apiBot(ctx, s.api, bot)
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
	result := make([]*apiv1.Bot, 0, len(req.Msg.GetBotUserIds()))
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
		mapped, err := apiBot(ctx, s.api, bot)
		if err != nil {
			return nil, err
		}
		result = append(result, mapped)
	}
	return connect.NewResponse(&apiv1.BatchGetBotsResponse{Bots: result}), nil
}

func (s *botService) CreateBot(ctx context.Context, req *connect.Request[apiv1.CreateBotRequest]) (*connect.Response[apiv1.CreateBotResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	bot, err := s.api.core.CreateBot(ctx, caller.UserID, req.Msg.GetLogin(), req.Msg.GetDisplayName())
	if err != nil {
		return nil, connectError(err)
	}
	mapped, err := apiBot(ctx, s.api, bot)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.CreateBotResponse{Bot: mapped, ApiKey: bot.APIKey}), nil
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

func (s *botService) RotateBotApiKey(ctx context.Context, req *connect.Request[apiv1.RotateBotApiKeyRequest]) (*connect.Response[apiv1.RotateBotApiKeyResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	bot, err := s.api.core.RotateBotAPIKey(ctx, caller.UserID, req.Msg.GetBotUserId())
	if err != nil {
		return nil, connectError(err)
	}
	mapped, err := apiBot(ctx, s.api, bot)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.RotateBotApiKeyResponse{Bot: mapped, ApiKey: bot.APIKey}), nil
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
	mapped, err := apiBot(ctx, s.api, bot)
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
	if !bot.APIKeyRotatedAt.IsZero() {
		out.ApiKeyRotatedAt = timestamppb.New(bot.APIKeyRotatedAt)
	}
	return out, nil
}

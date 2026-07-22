package connectapi

import (
	"context"
	"errors"
	"sort"
	"strings"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	defaultBotListLimit = 20
	maxBotListLimit     = 100
)

type botService struct {
	api *API
}

func (s *botService) ListBots(ctx context.Context, req *connect.Request[apiv1.ListBotsRequest]) (*connect.Response[apiv1.ListBotsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	bots, err := s.api.core.ListBots(ctx)
	if err != nil {
		return nil, connectError(err)
	}
	query := strings.ToLower(strings.TrimSpace(req.Msg.GetSearch()))
	visible := bots[:0]
	for _, bot := range bots {
		if req.Msg.GetOwnedByCallerOnly() && bot.GetBot().GetOwnerId() != caller.UserID {
			continue
		}
		allowed, err := s.api.core.CanManageBot(ctx, caller.UserID, bot.GetId())
		if err != nil {
			return nil, connectError(err)
		}
		if !allowed {
			continue
		}
		if query != "" && !strings.Contains(strings.ToLower(bot.GetLogin()), query) && !strings.Contains(strings.ToLower(bot.GetDisplayName()), query) {
			continue
		}
		visible = append(visible, bot)
	}
	sort.Slice(visible, func(i, j int) bool {
		left, right := strings.ToLower(visible[i].GetDisplayName()), strings.ToLower(visible[j].GetDisplayName())
		if left == right {
			return strings.ToLower(visible[i].GetLogin()) < strings.ToLower(visible[j].GetLogin())
		}
		return left < right
	})
	limit, offset := apiPagination(req.Msg.GetPage(), defaultBotListLimit, maxBotListLimit)
	total := len(visible)
	if offset > total {
		offset = total
	}
	end := min(offset+limit, total)
	out := make([]*apiv1.Bot, 0, end-offset)
	for _, bot := range visible[offset:end] {
		item, err := s.bot(ctx, bot)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return connect.NewResponse(&apiv1.ListBotsResponse{
		Bots: out,
		Page: apiPageInfo(total, end < total),
	}), nil
}

func (s *botService) GetBot(ctx context.Context, req *connect.Request[apiv1.GetBotRequest]) (*connect.Response[apiv1.GetBotResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	bot, err := s.manageableBot(ctx, caller.UserID, req.Msg.GetBotId())
	if err != nil {
		return nil, err
	}
	item, err := s.bot(ctx, bot)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.GetBotResponse{Bot: item}), nil
}

func (s *botService) BatchGetBots(ctx context.Context, req *connect.Request[apiv1.BatchGetBotsRequest]) (*connect.Response[apiv1.BatchGetBotsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(req.Msg.GetBotIds()))
	out := make([]*apiv1.Bot, 0, len(req.Msg.GetBotIds()))
	for _, botID := range req.Msg.GetBotIds() {
		if _, ok := seen[botID]; ok {
			continue
		}
		seen[botID] = struct{}{}
		bot, err := s.manageableBot(ctx, caller.UserID, botID)
		if err != nil {
			if connect.CodeOf(err) == connect.CodeNotFound || connect.CodeOf(err) == connect.CodePermissionDenied {
				continue
			}
			return nil, err
		}
		item, err := s.bot(ctx, bot)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return connect.NewResponse(&apiv1.BatchGetBotsResponse{Bots: out}), nil
}

func (s *botService) CreateBot(ctx context.Context, req *connect.Request[apiv1.CreateBotRequest]) (*connect.Response[apiv1.CreateBotResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	bot, err := s.api.core.CreateBotAs(ctx, caller.UserID, req.Msg.GetLogin(), req.Msg.GetDisplayName(), req.Msg.GetDescription())
	if err != nil {
		return nil, connectError(err)
	}
	item, err := s.bot(ctx, bot)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.CreateBotResponse{Bot: item}), nil
}

func (s *botService) UpdateBot(ctx context.Context, req *connect.Request[apiv1.UpdateBotRequest]) (*connect.Response[apiv1.UpdateBotResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	bot, err := s.api.core.UpdateBot(ctx, caller.UserID, req.Msg.GetBotId(), core.BotUpdateInput{
		Login: req.Msg.Login, DisplayName: req.Msg.DisplayName, Description: req.Msg.Description,
	})
	if err != nil {
		return nil, connectError(err)
	}
	item, err := s.bot(ctx, bot)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.UpdateBotResponse{Bot: item}), nil
}

func (s *botService) DeleteBot(ctx context.Context, req *connect.Request[apiv1.DeleteBotRequest]) (*connect.Response[apiv1.DeleteBotResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.api.core.DeleteBot(ctx, caller.UserID, req.Msg.GetBotId()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.DeleteBotResponse{Deleted: true}), nil
}

func (s *botService) RotateBotAPIKey(ctx context.Context, req *connect.Request[apiv1.RotateBotAPIKeyRequest]) (*connect.Response[apiv1.RotateBotAPIKeyResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	apiKey, _, err := s.api.core.RotateBotAPIKey(ctx, caller.UserID, req.Msg.GetBotId())
	if err != nil {
		return nil, connectError(err)
	}
	bot, err := s.api.core.GetUser(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, connectError(err)
	}
	item, err := s.bot(ctx, bot)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.RotateBotAPIKeyResponse{Bot: item, ApiKey: apiKey}), nil
}

func (s *botService) RevokeBotAPIKey(ctx context.Context, req *connect.Request[apiv1.RevokeBotAPIKeyRequest]) (*connect.Response[apiv1.RevokeBotAPIKeyResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.api.core.RevokeBotAPIKey(ctx, caller.UserID, req.Msg.GetBotId()); err != nil {
		return nil, connectError(err)
	}
	bot, err := s.api.core.GetUser(ctx, req.Msg.GetBotId())
	if err != nil {
		return nil, connectError(err)
	}
	item, err := s.bot(ctx, bot)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.RevokeBotAPIKeyResponse{Bot: item}), nil
}

func (s *botService) manageableBot(ctx context.Context, actorID, botID string) (*corev1.User, error) {
	bot, err := s.api.core.GetUser(ctx, botID)
	if err != nil {
		return nil, connectError(err)
	}
	if bot.GetBot() == nil {
		return nil, connectError(core.ErrNotFound)
	}
	allowed, err := s.api.core.CanManageBot(ctx, actorID, botID)
	if err != nil {
		return nil, connectError(err)
	}
	if !allowed {
		return nil, connectError(core.ErrPermissionDenied)
	}
	return bot, nil
}

func (s *botService) bot(ctx context.Context, bot *corev1.User) (*apiv1.Bot, error) {
	if bot == nil || bot.GetBot() == nil {
		return nil, connectError(core.ErrNotFound)
	}
	user, err := requiredUserSummary(ctx, s.api, bot)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, connectError(core.ErrNotFound)
		}
		return nil, err
	}
	item := &apiv1.Bot{
		User: user, CreatedAt: bot.GetCreatedAt(),
	}
	status, err := s.api.core.GetBotAPIKeyStatus(ctx, bot.GetId())
	if err != nil {
		return nil, connectError(err)
	}
	if status != nil {
		item.ApiKey = &apiv1.BotAPIKey{CreatedAt: timestamppb.New(status.CreatedAt)}
	}
	return item, nil
}

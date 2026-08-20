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

func (s *botService) UpdateBot(ctx context.Context, req *connect.Request[apiv1.UpdateBotRequest]) (*connect.Response[apiv1.UpdateBotResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	bot, err := s.api.core.UpdateBot(ctx, caller.UserID, req.Msg.GetBotUserId(), req.Msg.Login, req.Msg.DisplayName)
	if err != nil {
		return nil, connectError(err)
	}
	mapped, err := apiBot(ctx, s.api, bot)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.UpdateBotResponse{Bot: mapped}), nil
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

func (s *botService) GetBotPermissionMatrix(ctx context.Context, req *connect.Request[apiv1.GetBotPermissionMatrixRequest]) (*connect.Response[apiv1.GetBotPermissionMatrixResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	matrix, err := s.api.core.GetBotPermissionMatrix(ctx, caller.UserID, req.Msg.GetBotUserId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.GetBotPermissionMatrixResponse{Matrix: apiBotPermissionMatrix(matrix)}), nil
}

func (s *botService) SetBotPermission(ctx context.Context, req *connect.Request[apiv1.SetBotPermissionRequest]) (*connect.Response[apiv1.SetBotPermissionResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	scope, err := coreBotPermissionScope(req.Msg.GetScope())
	if err != nil {
		return nil, err
	}
	state, err := coreBotPermissionState(req.Msg.GetDecision())
	if err != nil {
		return nil, err
	}
	cell, err := s.api.core.SetBotPermission(ctx, caller.UserID, req.Msg.GetBotUserId(), scope, core.Permission(req.Msg.GetPermission()), state)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.SetBotPermissionResponse{Cell: apiBotPermissionCell(*cell)}), nil
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

func apiBotPermissionMatrix(matrix *core.BotPermissionMatrix) *apiv1.BotPermissionMatrix {
	if matrix == nil {
		return nil
	}
	out := &apiv1.BotPermissionMatrix{BotUserId: matrix.BotUserID, ApplicablePermissions: append([]string(nil), matrix.ApplicablePermissions...)}
	for _, scope := range matrix.Scopes {
		out.Scopes = append(out.Scopes, &apiv1.BotPermissionMatrixScope{Id: scope.ID, Label: scope.Label, Kind: apiBotScopeKind(scope.Kind), ParentGroupId: scope.ParentGroupID})
	}
	for _, cell := range matrix.Cells {
		out.Cells = append(out.Cells, apiBotPermissionCell(cell))
	}
	return out
}

func apiBotPermissionCell(cell core.BotPermissionCell) *apiv1.BotPermissionCell {
	return &apiv1.BotPermissionCell{Permission: cell.Permission, ScopeId: cell.ScopeID, Configured: apiBotDecision(cell.Configured), Delegated: apiBotDecision(cell.Delegated), OwnerGranted: cell.OwnerGranted, EffectiveGranted: cell.EffectiveGranted}
}

func apiBotDecision(decision core.MatrixDecision) apiv1.BotPermissionDecision {
	switch decision {
	case core.MatrixDecisionAllow:
		return apiv1.BotPermissionDecision_BOT_PERMISSION_DECISION_ALLOW
	case core.MatrixDecisionDeny:
		return apiv1.BotPermissionDecision_BOT_PERMISSION_DECISION_DENY
	default:
		return apiv1.BotPermissionDecision_BOT_PERMISSION_DECISION_NONE
	}
}

func apiBotScopeKind(kind core.MatrixScopeKind) apiv1.BotPermissionScopeKind {
	switch kind {
	case core.MatrixScopeGroup:
		return apiv1.BotPermissionScopeKind_BOT_PERMISSION_SCOPE_KIND_GROUP
	case core.MatrixScopeRoom:
		return apiv1.BotPermissionScopeKind_BOT_PERMISSION_SCOPE_KIND_ROOM
	default:
		return apiv1.BotPermissionScopeKind_BOT_PERMISSION_SCOPE_KIND_SERVER
	}
}

func coreBotPermissionScope(scope *apiv1.BotPermissionScope) (core.PermissionTargetScope, error) {
	if scope == nil || scope.GetKind() == apiv1.BotPermissionScopeKind_BOT_PERMISSION_SCOPE_KIND_SERVER || scope.GetKind() == apiv1.BotPermissionScopeKind_BOT_PERMISSION_SCOPE_KIND_UNSPECIFIED {
		return core.PermissionTargetScope{Kind: core.MatrixScopeServer}, nil
	}
	if scope.GetId() == "" {
		return core.PermissionTargetScope{}, invalidArgument("scope id is required")
	}
	if scope.GetKind() == apiv1.BotPermissionScopeKind_BOT_PERMISSION_SCOPE_KIND_GROUP {
		return core.PermissionTargetScope{Kind: core.MatrixScopeGroup, ID: scope.GetId()}, nil
	}
	if scope.GetKind() == apiv1.BotPermissionScopeKind_BOT_PERMISSION_SCOPE_KIND_ROOM {
		return core.PermissionTargetScope{Kind: core.MatrixScopeRoom, ID: scope.GetId()}, nil
	}
	return core.PermissionTargetScope{}, invalidArgument("invalid bot permission scope")
}

func coreBotPermissionState(decision apiv1.BotPermissionDecision) (core.PermissionState, error) {
	switch decision {
	case apiv1.BotPermissionDecision_BOT_PERMISSION_DECISION_ALLOW:
		return core.PermissionStateAllow, nil
	case apiv1.BotPermissionDecision_BOT_PERMISSION_DECISION_DENY:
		return core.PermissionStateDeny, nil
	case apiv1.BotPermissionDecision_BOT_PERMISSION_DECISION_NONE:
		return core.PermissionStateNone, nil
	default:
		return "", invalidArgument("invalid bot permission decision")
	}
}

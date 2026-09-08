package connectapi

import (
	"connectrpc.com/connect"
	"context"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	logv1 "hmans.de/chatto/internal/pb/chatto/core/log/v1"
)

func apiBotOutboundWebhook(w *core.BotOutboundWebhook) *apiv1.BotOutboundWebhook {
	if w == nil {
		return nil
	}
	result := &apiv1.BotOutboundWebhook{Id: w.ID, Name: w.Name, CreatedAt: timestamppb.New(w.CreatedAt), Url: w.URL, Enabled: w.Enabled, HasAuthorization: w.HasAuthorization}
	result.LatestDelivery = apiBotWebhookFailure(w.Latest)
	return result
}
func (s *botService) GetBotOutboundWebhook(ctx context.Context, req *connect.Request[apiv1.GetBotOutboundWebhookRequest]) (*connect.Response[apiv1.GetBotOutboundWebhookResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	result, err := s.api.core.GetBotOutboundWebhook(ctx, caller.UserID, req.Msg.GetBotUserId(), req.Msg.GetWebhookId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.GetBotOutboundWebhookResponse{Webhook: apiBotOutboundWebhook(result)}), nil
}
func (s *botService) CreateBotOutboundWebhook(ctx context.Context, req *connect.Request[apiv1.CreateBotOutboundWebhookRequest]) (*connect.Response[apiv1.CreateBotOutboundWebhookResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	result, secret, err := s.api.core.CreateBotOutboundWebhook(ctx, caller.UserID, req.Msg.GetBotUserId(), req.Msg.GetName(), req.Msg.GetUrl(), req.Msg.GetAuthorization(), req.Msg.GetEnabled())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.CreateBotOutboundWebhookResponse{Webhook: apiBotOutboundWebhook(result), SigningSecret: secret}), nil
}
func (s *botService) RevokeBotOutboundWebhook(ctx context.Context, req *connect.Request[apiv1.RevokeBotOutboundWebhookRequest]) (*connect.Response[apiv1.RevokeBotOutboundWebhookResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err = s.api.core.RevokeBotOutboundWebhook(ctx, caller.UserID, req.Msg.GetBotUserId(), req.Msg.GetWebhookId()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.RevokeBotOutboundWebhookResponse{}), nil
}

func (s *botService) ListBotOutboundWebhooks(ctx context.Context, req *connect.Request[apiv1.ListBotOutboundWebhooksRequest]) (*connect.Response[apiv1.ListBotOutboundWebhooksResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	items, err := s.api.core.ListBotOutboundWebhooks(ctx, caller.UserID, req.Msg.GetBotUserId())
	if err != nil {
		return nil, connectError(err)
	}
	response := &apiv1.ListBotOutboundWebhooksResponse{}
	for _, item := range items {
		response.Webhooks = append(response.Webhooks, apiBotOutboundWebhook(item))
	}
	return connect.NewResponse(response), nil
}
func (s *botService) UpdateBotOutboundWebhook(ctx context.Context, req *connect.Request[apiv1.UpdateBotOutboundWebhookRequest]) (*connect.Response[apiv1.UpdateBotOutboundWebhookResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	item, err := s.api.core.UpdateBotOutboundWebhook(ctx, caller.UserID, req.Msg.GetBotUserId(), req.Msg.GetWebhookId(), core.BotOutboundWebhookPatch{Enabled: req.Msg.Enabled, URL: req.Msg.Url, Authorization: req.Msg.Authorization})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.UpdateBotOutboundWebhookResponse{Webhook: apiBotOutboundWebhook(item)}), nil
}

func apiBotWebhookFailure(e *logv1.Entry) *apiv1.BotWebhookDelivery {
	if e == nil {
		return nil
	}
	x := e.GetBotWebhookDeliveryFailed()
	if x == nil {
		return nil
	}
	return &apiv1.BotWebhookDelivery{Id: e.GetId(), Status: apiv1.BotWebhookDeliveryStatus_BOT_WEBHOOK_DELIVERY_STATUS_FAILED, Reason: x.GetReason(), Attempts: x.GetAttempts(), HttpStatus: x.GetHttpStatus(), CompletedAt: e.GetRecordedAt(), SourceEventId: x.GetSourceEventId()}
}

func (s *botService) ListBotWebhookFailures(ctx context.Context, req *connect.Request[apiv1.ListBotWebhookFailuresRequest]) (*connect.Response[apiv1.ListBotWebhookFailuresResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	page, err := s.api.core.ListBotWebhookFailures(ctx, caller.UserID, req.Msg.GetBotUserId(), req.Msg.GetWebhookId(), req.Msg.GetPageSize(), req.Msg.GetCursor())
	if err != nil {
		return nil, connectError(err)
	}
	response := &apiv1.ListBotWebhookFailuresResponse{NextCursor: page.NextCursor}
	for _, entry := range page.Entries {
		response.Failures = append(response.Failures, apiBotWebhookFailure(entry))
	}
	return connect.NewResponse(response), nil
}

package connectapi

import (
	"connectrpc.com/connect"
	"context"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func apiBotOutboundWebhook(w *core.BotOutboundWebhook) *apiv1.BotOutboundWebhook {
	if w == nil {
		return nil
	}
	result := &apiv1.BotOutboundWebhook{Id: w.ID, Enabled: w.Enabled, HasAuthorization: w.HasAuthorization}
	if e := w.Latest; e != nil {
		x := e.GetBotWebhookDeliveryCompleted()
		result.LatestDelivery = &apiv1.BotWebhookDelivery{Id: x.GetDeliveryId(), Status: apiBotWebhookStatus(x.GetStatus()), Reason: x.GetReason(), Attempts: x.GetAttempts(), HttpStatus: x.GetHttpStatus(), CompletedAt: e.GetCreatedAt()}
	}
	return result
}
func (s *botService) GetBotOutboundWebhook(ctx context.Context, req *connect.Request[apiv1.GetBotOutboundWebhookRequest]) (*connect.Response[apiv1.GetBotOutboundWebhookResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	result, err := s.api.core.GetBotOutboundWebhook(ctx, caller.UserID, req.Msg.GetBotUserId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.GetBotOutboundWebhookResponse{Webhook: apiBotOutboundWebhook(result)}), nil
}
func (s *botService) ReplaceBotOutboundWebhook(ctx context.Context, req *connect.Request[apiv1.ReplaceBotOutboundWebhookRequest]) (*connect.Response[apiv1.ReplaceBotOutboundWebhookResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	result, secret, err := s.api.core.ReplaceBotOutboundWebhook(ctx, caller.UserID, req.Msg.GetBotUserId(), req.Msg.GetUrl(), req.Msg.GetAuthorization(), req.Msg.GetEnabled())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.ReplaceBotOutboundWebhookResponse{Webhook: apiBotOutboundWebhook(result), SigningSecret: secret}), nil
}
func (s *botService) DeleteBotOutboundWebhook(ctx context.Context, req *connect.Request[apiv1.DeleteBotOutboundWebhookRequest]) (*connect.Response[apiv1.DeleteBotOutboundWebhookResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err = s.api.core.DeleteBotOutboundWebhook(ctx, caller.UserID, req.Msg.GetBotUserId()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.DeleteBotOutboundWebhookResponse{}), nil
}

func apiBotWebhookStatus(status string) apiv1.BotWebhookDeliveryStatus {
	switch status {
	case "delivered":
		return apiv1.BotWebhookDeliveryStatus_BOT_WEBHOOK_DELIVERY_STATUS_DELIVERED
	case "failed":
		return apiv1.BotWebhookDeliveryStatus_BOT_WEBHOOK_DELIVERY_STATUS_FAILED
	case "skipped":
		return apiv1.BotWebhookDeliveryStatus_BOT_WEBHOOK_DELIVERY_STATUS_SKIPPED
	default:
		return apiv1.BotWebhookDeliveryStatus_BOT_WEBHOOK_DELIVERY_STATUS_UNSPECIFIED
	}
}

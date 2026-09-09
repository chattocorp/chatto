package connectapi

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

type pushNotificationService struct {
	api *API
}

func (s *pushNotificationService) SendTestNotification(ctx context.Context, _ *connect.Request[apiv1.SendTestNotificationRequest]) (*connect.Response[apiv1.SendTestNotificationResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if !s.api.config.Push.IsConfigured() || s.api.core.OnPushTestRequested == nil {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("push notifications are not enabled on this instance"))
	}
	if err := s.api.core.AdmitPushTestNotification(ctx, caller.UserID); err != nil {
		if errors.Is(err, core.ErrPushTestNotificationRateLimited) {
			return nil, connect.NewError(connect.CodeResourceExhausted, errors.New("test push notification rate limit exceeded"))
		}
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("push notification could not be delivered"))
	}

	if err := s.api.core.OnPushTestRequested(ctx, caller.UserID); err != nil {
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("push notification could not be delivered"))
	}
	return connect.NewResponse(&apiv1.SendTestNotificationResponse{}), nil
}

func (s *pushNotificationService) Subscribe(ctx context.Context, req *connect.Request[apiv1.SubscribeRequest]) (*connect.Response[apiv1.SubscribeResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if !s.api.config.Push.IsConfigured() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("push notifications are not enabled on this instance"))
	}

	userAgent := ""
	if req.Msg.UserAgent != nil {
		userAgent = req.Msg.GetUserAgent()
	}
	if req.Msg.GetClientHost() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("client host is required"))
	}
	if req.Msg.GetCleanupToken() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("cleanup token is required"))
	}

	if _, err := s.api.core.SavePushSubscriptionForClientWithCleanupToken(ctx, caller.UserID, req.Msg.GetEndpoint(), req.Msg.GetP256Dh(), req.Msg.GetAuth(), userAgent, req.Msg.GetClientHost(), req.Msg.GetCleanupToken()); err != nil {
		return nil, connectError(err)
	}

	return connect.NewResponse(&apiv1.SubscribeResponse{}), nil
}

func (s *pushNotificationService) Unsubscribe(ctx context.Context, req *connect.Request[apiv1.UnsubscribeRequest]) (*connect.Response[apiv1.UnsubscribeResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.api.core.DeletePushSubscription(ctx, caller.UserID, req.Msg.GetEndpoint()); err != nil {
		return nil, connectError(err)
	}

	return connect.NewResponse(&apiv1.UnsubscribeResponse{}), nil
}

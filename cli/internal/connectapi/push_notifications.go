package connectapi

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

type pushNotificationService struct {
	api *API
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
		return nil, err
	}

	return connect.NewResponse(&apiv1.SubscribeResponse{}), nil
}

func (s *pushNotificationService) Unsubscribe(ctx context.Context, req *connect.Request[apiv1.UnsubscribeRequest]) (*connect.Response[apiv1.UnsubscribeResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.api.core.DeletePushSubscription(ctx, caller.UserID, req.Msg.GetEndpoint()); err != nil {
		return nil, err
	}

	return connect.NewResponse(&apiv1.UnsubscribeResponse{}), nil
}

package connectapi

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	appv1 "hmans.de/chatto/internal/pb/chatto/app/v1"
)

type pushNotificationService struct {
	api *API
}

func (s *pushNotificationService) Subscribe(ctx context.Context, req *connect.Request[appv1.SubscribeRequest]) (*connect.Response[appv1.SubscribeResponse], error) {
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
	if _, err := s.api.core.SavePushSubscription(ctx, caller.UserID, req.Msg.GetEndpoint(), req.Msg.GetP256Dh(), req.Msg.GetAuth(), userAgent); err != nil {
		return nil, connectError(err)
	}

	return connect.NewResponse(&appv1.SubscribeResponse{Subscribed: true}), nil
}

func (s *pushNotificationService) Unsubscribe(ctx context.Context, req *connect.Request[appv1.UnsubscribeRequest]) (*connect.Response[appv1.UnsubscribeResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.api.core.DeletePushSubscription(ctx, caller.UserID, req.Msg.GetEndpoint()); err != nil {
		return nil, connectError(err)
	}

	return connect.NewResponse(&appv1.UnsubscribeResponse{Unsubscribed: true}), nil
}

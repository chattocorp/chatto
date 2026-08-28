package connectapi

import (
	"context"

	"connectrpc.com/connect"
	authv1 "hmans.de/chatto/internal/pb/chatto/auth/v1"
)

type pushSubscriptionCleanupService struct {
	api *API
}

func (s *pushSubscriptionCleanupService) DeleteSubscription(ctx context.Context, req *connect.Request[authv1.DeleteSubscriptionRequest]) (*connect.Response[authv1.DeleteSubscriptionResponse], error) {
	if err := s.api.core.DeletePushSubscriptionByCapability(ctx, req.Msg.GetEndpoint(), req.Msg.GetAuth(), req.Msg.GetCleanupToken()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&authv1.DeleteSubscriptionResponse{Completed: true}), nil
}

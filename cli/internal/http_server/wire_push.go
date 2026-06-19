package http_server

import (
	"context"
	"fmt"

	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	wirev1 "hmans.de/chatto/internal/pb/chatto/wire/v1"
)

func (c *wireConn) handleWireSubscribeToPush(ctx context.Context, userID, requestID string, body *apiv1.SubscribeToPushRequest) (*apiv1.SubscribeToPushResponse, *wirev1.WireError) {
	if !c.server.config.Push.IsConfigured() {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: push notifications are not enabled on this instance", errWireInvalidArgument))
	}
	if body.GetEndpoint() == "" || body.GetP256Dh() == "" || body.GetAuth() == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: endpoint, p256dh, and auth are required", errWireInvalidArgument))
	}

	if _, err := c.server.core.SavePushSubscription(ctx, userID, body.GetEndpoint(), body.GetP256Dh(), body.GetAuth(), body.GetUserAgent()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.SubscribeToPushResponse{Subscribed: true}, nil
}

func (c *wireConn) handleWireUnsubscribeFromPush(ctx context.Context, userID, requestID string, body *apiv1.UnsubscribeFromPushRequest) (*apiv1.UnsubscribeFromPushResponse, *wirev1.WireError) {
	if body.GetEndpoint() == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: endpoint is required", errWireInvalidArgument))
	}

	if err := c.server.core.DeletePushSubscription(ctx, userID, body.GetEndpoint()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.UnsubscribeFromPushResponse{Unsubscribed: true}, nil
}

package http_server

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	wirev1 "hmans.de/chatto/internal/pb/chatto/wire/v1"
)

func (c *wireConn) handleWireUpdateMyPresence(ctx context.Context, userID, requestID string, body *apiv1.UpdateMyPresenceRequest) (*apiv1.UpdateMyPresenceResponse, *wirev1.WireError) {
	status, err := wirePresenceStatus(body.GetStatus())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if err := c.server.core.SetPresence(ctx, userID, status); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.UpdateMyPresenceResponse{Updated: true}, nil
}

func wirePresenceStatus(status corev1.UserPresenceStatus) (string, error) {
	switch status {
	case corev1.UserPresenceStatus_USER_PRESENCE_STATUS_ONLINE:
		return core.PresenceStatusOnline, nil
	case corev1.UserPresenceStatus_USER_PRESENCE_STATUS_AWAY:
		return core.PresenceStatusAway, nil
	case corev1.UserPresenceStatus_USER_PRESENCE_STATUS_DO_NOT_DISTURB:
		return core.PresenceStatusDoNotDisturb, nil
	default:
		return "", fmt.Errorf("%w: status must be ONLINE, AWAY, or DO_NOT_DISTURB", errWireInvalidArgument)
	}
}

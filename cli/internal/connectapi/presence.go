package connectapi

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

type presenceService struct {
	api *API
}

func (s *presenceService) ReportPresence(ctx context.Context, req *connect.Request[apiv1.ReportPresenceRequest]) (*connect.Response[apiv1.ReportPresenceResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	status, err := apiPresenceStatusToCore(req.Msg.Status)
	if err != nil {
		return nil, err
	}
	if err := s.api.core.SetPresenceWithOptions(ctx, caller.UserID, status, req.Msg.UserSelected); err != nil {
		return nil, connectError(err)
	}
	storedStatus, err := s.api.core.GetUserPresence(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}

	return connect.NewResponse(&apiv1.ReportPresenceResponse{
		Status: corePresenceStatusToAPI(storedStatus),
	}), nil
}

func apiPresenceStatusToCore(status apiv1.PresenceStatus) (string, error) {
	switch status {
	case apiv1.PresenceStatus_PRESENCE_STATUS_ONLINE:
		return core.PresenceStatusOnline, nil
	case apiv1.PresenceStatus_PRESENCE_STATUS_AWAY:
		return core.PresenceStatusAway, nil
	case apiv1.PresenceStatus_PRESENCE_STATUS_DO_NOT_DISTURB:
		return core.PresenceStatusDoNotDisturb, nil
	default:
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("status must be ONLINE, AWAY, or DO_NOT_DISTURB"))
	}
}

func corePresenceStatusToAPI(status string) apiv1.PresenceStatus {
	switch status {
	case core.PresenceStatusAway:
		return apiv1.PresenceStatus_PRESENCE_STATUS_AWAY
	case core.PresenceStatusDoNotDisturb:
		return apiv1.PresenceStatus_PRESENCE_STATUS_DO_NOT_DISTURB
	default:
		return apiv1.PresenceStatus_PRESENCE_STATUS_ONLINE
	}
}

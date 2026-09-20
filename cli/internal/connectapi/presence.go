package connectapi

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	"hmans.de/chatto/pkg/events"
)

func (s *accountService) GetPresencePreference(ctx context.Context, req *connect.Request[apiv1.GetPresencePreferenceRequest]) (*connect.Response[apiv1.GetPresencePreferenceResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.api.core.GetPresencePreference(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.GetPresencePreferenceResponse{Preference: p}), nil
}

func (s *accountService) SetPresencePreference(ctx context.Context, req *connect.Request[apiv1.SetPresencePreferenceRequest]) (*connect.Response[apiv1.SetPresencePreferenceResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	p, err := s.api.core.SetPresencePreference(ctx, caller.UserID, req.Msg.Status, req.Msg.ExpectedRevision)
	if errors.Is(err, events.ErrConflict) {
		return nil, connect.NewError(connect.CodeAborted, err)
	}
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.SetPresencePreferenceResponse{Preference: p}), nil
}

func (s *accountService) RefreshPresence(ctx context.Context, req *connect.Request[apiv1.RefreshPresenceRequest]) (*connect.Response[apiv1.RefreshPresenceResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.api.core.SetPresence(ctx, caller.UserID, core.PresenceStatusOnline); err != nil {
		return nil, connectError(err)
	}
	p, err := s.api.core.GetPresencePreference(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.RefreshPresenceResponse{Preference: p}), nil
}

func (s *accountService) SetPresence(ctx context.Context, req *connect.Request[apiv1.SetPresenceRequest]) (*connect.Response[apiv1.SetPresenceResponse], error) {
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

	return connect.NewResponse(&apiv1.SetPresenceResponse{
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
	case apiv1.PresenceStatus_PRESENCE_STATUS_OFFLINE:
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("status must be ONLINE, AWAY, or DO_NOT_DISTURB"))
	default:
		return "", connect.NewError(connect.CodeInvalidArgument, errors.New("status must be ONLINE, AWAY, or DO_NOT_DISTURB"))
	}
}

func corePresenceStatusToAPI(status string) apiv1.PresenceStatus {
	switch status {
	case core.PresenceStatusOffline:
		return apiv1.PresenceStatus_PRESENCE_STATUS_OFFLINE
	case core.PresenceStatusAway:
		return apiv1.PresenceStatus_PRESENCE_STATUS_AWAY
	case core.PresenceStatusDoNotDisturb:
		return apiv1.PresenceStatus_PRESENCE_STATUS_DO_NOT_DISTURB
	default:
		return apiv1.PresenceStatus_PRESENCE_STATUS_ONLINE
	}
}

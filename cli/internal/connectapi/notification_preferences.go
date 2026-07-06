package connectapi

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type notificationPreferencesService struct {
	api *API
}

func (s *notificationPreferencesService) GetServerNotificationPreference(ctx context.Context, _ *connect.Request[apiv1.GetServerNotificationPreferenceRequest]) (*connect.Response[apiv1.GetNotificationPreferenceResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	level, err := s.api.core.GetSpaceNotificationLevel(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	effectiveLevel := level
	if effectiveLevel == corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED {
		effectiveLevel = corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL
	}
	return connect.NewResponse(&apiv1.GetNotificationPreferenceResponse{
		Preference: apiNotificationPreference(level, effectiveLevel),
	}), nil
}

func (s *notificationPreferencesService) UpdateServerNotificationPreference(ctx context.Context, req *connect.Request[apiv1.UpdateServerNotificationPreferenceRequest]) (*connect.Response[apiv1.UpdateNotificationPreferenceResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	level, err := apiNotificationLevelToCore(req.Msg.Level)
	if err != nil {
		return nil, err
	}

	if err := s.api.core.SetSpaceNotificationLevel(ctx, caller.UserID, level); err != nil {
		return nil, connectError(err)
	}
	effectiveLevel := level
	if effectiveLevel == corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED {
		effectiveLevel = corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL
	}
	return connect.NewResponse(&apiv1.UpdateNotificationPreferenceResponse{
		Preference: apiNotificationPreference(level, effectiveLevel),
	}), nil
}

func (s *notificationPreferencesService) GetRoomNotificationPreference(ctx context.Context, req *connect.Request[apiv1.GetRoomNotificationPreferenceRequest]) (*connect.Response[apiv1.GetNotificationPreferenceResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Msg.RoomId) == "" {
		return nil, invalidArgument("room_id is required")
	}

	pref, err := s.api.core.NotificationPreferences().GetRoomNotificationPreference(ctx, caller.UserID, req.Msg.RoomId)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.GetNotificationPreferenceResponse{
		Preference: apiNotificationPreference(pref.Level, pref.EffectiveLevel),
	}), nil
}

func (s *notificationPreferencesService) UpdateRoomNotificationPreference(ctx context.Context, req *connect.Request[apiv1.UpdateRoomNotificationPreferenceRequest]) (*connect.Response[apiv1.UpdateNotificationPreferenceResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Msg.RoomId) == "" {
		return nil, invalidArgument("room_id is required")
	}
	level, err := apiNotificationLevelToCore(req.Msg.Level)
	if err != nil {
		return nil, err
	}

	pref, err := s.api.core.NotificationPreferences().SetRoomNotificationLevel(ctx, caller.UserID, req.Msg.RoomId, level)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.UpdateNotificationPreferenceResponse{
		Preference: apiNotificationPreference(pref.Level, pref.EffectiveLevel),
	}), nil
}

func apiNotificationPreference(level, effectiveLevel corev1.NotificationLevel) *apiv1.NotificationPreference {
	return &apiv1.NotificationPreference{
		Level:          coreNotificationLevelToAPI(level),
		EffectiveLevel: coreNotificationLevelToAPI(effectiveLevel),
	}
}

func apiNotificationLevelToCore(level apiv1.NotificationLevel) (corev1.NotificationLevel, error) {
	switch level {
	case apiv1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT:
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED, nil
	case apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED:
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED, nil
	case apiv1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL:
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL, nil
	case apiv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES:
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES, nil
	default:
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED, invalidArgument("notification level must be DEFAULT, MUTED, NORMAL, or ALL_MESSAGES")
	}
}

func coreNotificationLevelToAPI(level corev1.NotificationLevel) apiv1.NotificationLevel {
	switch level {
	case corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED:
		return apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED
	case corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL:
		return apiv1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL
	case corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES:
		return apiv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES
	default:
		return apiv1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT
	}
}

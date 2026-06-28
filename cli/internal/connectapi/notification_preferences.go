package connectapi

import (
	"context"
	"strings"

	"connectrpc.com/connect"
	appv1 "hmans.de/chatto/internal/pb/chatto/app/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type notificationPreferencesService struct {
	api *API
}

func (s *notificationPreferencesService) GetServerNotificationPreference(ctx context.Context, _ *connect.Request[appv1.GetServerNotificationPreferenceRequest]) (*connect.Response[appv1.GetServerNotificationPreferenceResponse], error) {
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
	return connect.NewResponse(&appv1.GetServerNotificationPreferenceResponse{
		Level:          coreNotificationLevelToAPI(level),
		EffectiveLevel: coreNotificationLevelToAPI(effectiveLevel),
	}), nil
}

func (s *notificationPreferencesService) SetServerNotificationLevel(ctx context.Context, req *connect.Request[appv1.SetServerNotificationLevelRequest]) (*connect.Response[appv1.SetServerNotificationLevelResponse], error) {
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
	return connect.NewResponse(&appv1.SetServerNotificationLevelResponse{
		Level:          coreNotificationLevelToAPI(level),
		EffectiveLevel: coreNotificationLevelToAPI(effectiveLevel),
	}), nil
}

func (s *notificationPreferencesService) GetRoomNotificationPreference(ctx context.Context, req *connect.Request[appv1.GetRoomNotificationPreferenceRequest]) (*connect.Response[appv1.GetRoomNotificationPreferenceResponse], error) {
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
	return connect.NewResponse(&appv1.GetRoomNotificationPreferenceResponse{
		Level:          coreNotificationLevelToAPI(pref.Level),
		EffectiveLevel: coreNotificationLevelToAPI(pref.EffectiveLevel),
	}), nil
}

func (s *notificationPreferencesService) SetRoomNotificationLevel(ctx context.Context, req *connect.Request[appv1.SetRoomNotificationLevelRequest]) (*connect.Response[appv1.SetRoomNotificationLevelResponse], error) {
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
	return connect.NewResponse(&appv1.SetRoomNotificationLevelResponse{
		Level:          coreNotificationLevelToAPI(pref.Level),
		EffectiveLevel: coreNotificationLevelToAPI(pref.EffectiveLevel),
	}), nil
}

func apiNotificationLevelToCore(level appv1.NotificationLevel) (corev1.NotificationLevel, error) {
	switch level {
	case appv1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT:
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED, nil
	case appv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED:
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED, nil
	case appv1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL:
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL, nil
	case appv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES:
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES, nil
	default:
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED, invalidArgument("notification level must be DEFAULT, MUTED, NORMAL, or ALL_MESSAGES")
	}
}

func coreNotificationLevelToAPI(level corev1.NotificationLevel) appv1.NotificationLevel {
	switch level {
	case corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED:
		return appv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED
	case corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL:
		return appv1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL
	case corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES:
		return appv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES
	default:
		return appv1.NotificationLevel_NOTIFICATION_LEVEL_DEFAULT
	}
}

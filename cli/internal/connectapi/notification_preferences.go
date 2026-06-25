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

func (s *notificationPreferencesService) GetRoomNotificationPreference(ctx context.Context, req *connect.Request[apiv1.GetRoomNotificationPreferenceRequest]) (*connect.Response[apiv1.GetRoomNotificationPreferenceResponse], error) {
	return handleAuthedUnary(ctx, func(ctx context.Context, user authenticatedUser) (*apiv1.GetRoomNotificationPreferenceResponse, error) {
		if strings.TrimSpace(req.Msg.RoomId) == "" {
			return nil, invalidArgument("room_id is required")
		}
		// Keep ConnectRPC transport code thin: authenticate the request, translate
		// protobufs/errors, and delegate operation authZ to the core service.
		pref, err := s.api.core.NotificationPreferences().GetRoomNotificationPreference(ctx, user.Id, req.Msg.RoomId)
		if err != nil {
			return nil, err
		}
		return &apiv1.GetRoomNotificationPreferenceResponse{
			Level:          coreNotificationLevelToAPI(pref.Level),
			EffectiveLevel: coreNotificationLevelToAPI(pref.EffectiveLevel),
		}, nil
	})
}

func (s *notificationPreferencesService) SetRoomNotificationLevel(ctx context.Context, req *connect.Request[apiv1.SetRoomNotificationLevelRequest]) (*connect.Response[apiv1.SetRoomNotificationLevelResponse], error) {
	return handleAuthedUnary(ctx, func(ctx context.Context, user authenticatedUser) (*apiv1.SetRoomNotificationLevelResponse, error) {
		if strings.TrimSpace(req.Msg.RoomId) == "" {
			return nil, invalidArgument("room_id is required")
		}
		level, err := apiNotificationLevelToCore(req.Msg.Level)
		if err != nil {
			return nil, err
		}
		// Keep membership checks and response semantics in the shared service so
		// GraphQL and ConnectRPC cannot drift.
		pref, err := s.api.core.NotificationPreferences().SetRoomNotificationLevel(ctx, user.Id, req.Msg.RoomId, level)
		if err != nil {
			return nil, err
		}
		return &apiv1.SetRoomNotificationLevelResponse{
			Level:          coreNotificationLevelToAPI(pref.Level),
			EffectiveLevel: coreNotificationLevelToAPI(pref.EffectiveLevel),
		}, nil
	})
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

package http_server

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type notificationPreferencesService struct {
	server *HTTPServer
}

func (s *notificationPreferencesService) GetRoomNotificationPreference(ctx context.Context, req *connect.Request[apiv1.GetRoomNotificationPreferenceRequest]) (*connect.Response[apiv1.GetRoomNotificationPreferenceResponse], error) {
	user, err := requireConnectAuth(ctx)
	if err != nil {
		return nil, err
	}
	pref, err := s.roomNotificationPreference(ctx, user.Id, req.Msg.RoomId)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.GetRoomNotificationPreferenceResponse{
		Level:          pref.level,
		EffectiveLevel: pref.effectiveLevel,
	}), nil
}

func (s *notificationPreferencesService) SetRoomNotificationLevel(ctx context.Context, req *connect.Request[apiv1.SetRoomNotificationLevelRequest]) (*connect.Response[apiv1.SetRoomNotificationLevelResponse], error) {
	user, err := requireConnectAuth(ctx)
	if err != nil {
		return nil, err
	}
	roomID := req.Msg.RoomId
	if err := s.requireRoomMember(ctx, user.Id, roomID); err != nil {
		return nil, err
	}
	level := apiNotificationLevelToCore(req.Msg.Level)
	if err := s.server.core.SetRoomNotificationLevel(ctx, user.Id, roomID, level); err != nil {
		return nil, connectError(err)
	}
	pref, err := s.roomNotificationPreference(ctx, user.Id, roomID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.SetRoomNotificationLevelResponse{
		Level:          pref.level,
		EffectiveLevel: pref.effectiveLevel,
	}), nil
}

type apiNotificationPreference struct {
	level          apiv1.NotificationLevel
	effectiveLevel apiv1.NotificationLevel
}

func (s *notificationPreferencesService) roomNotificationPreference(ctx context.Context, userID, roomID string) (*apiNotificationPreference, error) {
	if err := s.requireRoomMember(ctx, userID, roomID); err != nil {
		return nil, err
	}
	level, err := s.server.core.GetRoomNotificationLevel(ctx, userID, roomID)
	if err != nil {
		return nil, connectError(err)
	}
	effectiveLevel, err := s.server.core.GetEffectiveNotificationLevel(ctx, userID, roomID)
	if err != nil {
		return nil, connectError(err)
	}
	return &apiNotificationPreference{
		level:          coreNotificationLevelToAPI(level),
		effectiveLevel: coreNotificationLevelToAPI(effectiveLevel),
	}, nil
}

func (s *notificationPreferencesService) requireRoomMember(ctx context.Context, userID, roomID string) error {
	isMember, err := s.server.core.RoomMembershipExists(ctx, core.KindChannel, userID, roomID)
	if err != nil {
		return connectError(err)
	}
	if !isMember {
		return connect.NewError(connect.CodePermissionDenied, errors.New("access denied: not a member of this room"))
	}
	return nil
}

func apiNotificationLevelToCore(level apiv1.NotificationLevel) corev1.NotificationLevel {
	switch level {
	case apiv1.NotificationLevel_NOTIFICATION_LEVEL_MUTED:
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_MUTED
	case apiv1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL:
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL
	case apiv1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES:
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_ALL_MESSAGES
	default:
		return corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED
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

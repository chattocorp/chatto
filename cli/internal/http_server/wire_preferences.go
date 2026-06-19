package http_server

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	wirev1 "hmans.de/chatto/internal/pb/chatto/wire/v1"
)

func (c *wireConn) handleWireGetUserSettings(ctx context.Context, userID, requestID string) (*apiv1.GetUserSettingsResponse, *wirev1.WireError) {
	settings, err := c.userSettingsOrDefault(ctx, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.GetUserSettingsResponse{Settings: settings}, nil
}

func (c *wireConn) handleWireUpdateUserSettings(ctx context.Context, userID, requestID string, body *apiv1.UpdateUserSettingsRequest) (*apiv1.UpdateUserSettingsResponse, *wirev1.WireError) {
	if body == nil || (body.Timezone == nil && body.TimeFormat == nil) {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: at least one of timezone or time_format must be provided", errWireInvalidArgument))
	}

	input := core.UserSettingsInput{
		Timezone:   body.Timezone,
		TimeFormat: body.TimeFormat,
	}
	settings, err := c.server.core.UpdateUserSettings(ctx, userID, input)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	if settings == nil {
		settings = &corev1.ServerUserPreferences{}
	}
	return &apiv1.UpdateUserSettingsResponse{Settings: settings}, nil
}

func (c *wireConn) handleWireSetServerNotificationLevel(ctx context.Context, userID, requestID string, body *apiv1.SetServerNotificationLevelRequest) (*apiv1.SetNotificationLevelResponse, *wirev1.WireError) {
	level := body.GetLevel()
	if err := c.server.core.SetSpaceNotificationLevel(ctx, userID, level); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	effectiveLevel := level
	if effectiveLevel == corev1.NotificationLevel_NOTIFICATION_LEVEL_UNSPECIFIED {
		effectiveLevel = corev1.NotificationLevel_NOTIFICATION_LEVEL_NORMAL
	}
	return &apiv1.SetNotificationLevelResponse{Preference: &apiv1.ViewerNotificationPreferenceView{
		Level:          level,
		EffectiveLevel: effectiveLevel,
	}}, nil
}

func (c *wireConn) handleWireSetRoomNotificationLevel(ctx context.Context, userID, requestID string, body *apiv1.SetRoomNotificationLevelRequest) (*apiv1.SetNotificationLevelResponse, *wirev1.WireError) {
	if body.GetRoomId() == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: room_id is required", errWireInvalidArgument))
	}
	if _, _, err := c.authorizedRoom(ctx, userID, body.GetRoomId()); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	level := body.GetLevel()
	if err := c.server.core.SetRoomNotificationLevel(ctx, userID, body.GetRoomId(), level); err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	effectiveLevel, err := c.server.core.GetEffectiveNotificationLevel(ctx, userID, body.GetRoomId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.SetNotificationLevelResponse{Preference: &apiv1.ViewerNotificationPreferenceView{
		Level:          level,
		EffectiveLevel: effectiveLevel,
	}}, nil
}

func (c *wireConn) userSettingsOrDefault(ctx context.Context, userID string) (*corev1.ServerUserPreferences, error) {
	settings, err := c.server.core.GetUserSettings(ctx, userID)
	if err != nil {
		return nil, err
	}
	if settings == nil {
		return &corev1.ServerUserPreferences{}, nil
	}
	return settings, nil
}

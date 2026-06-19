package http_server

import (
	"context"
	"time"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	wirev1 "hmans.de/chatto/internal/pb/chatto/wire/v1"
)

func (c *wireConn) handleWireGetCurrentUser(ctx context.Context, user *corev1.User, requestID string) (*apiv1.GetCurrentUserResponse, *wirev1.WireError) {
	avatarURL, err := c.server.core.GetUserAvatarURL(ctx, user.GetId(), nil, nil, "")
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	hasVerifiedEmail, err := c.server.core.HasVerifiedEmail(ctx, user.GetId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	presence, err := c.server.core.GetUserPresence(ctx, user.GetId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	settings, err := c.userSettingsOrDefault(ctx, user.GetId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	return &apiv1.GetCurrentUserResponse{
		User: &apiv1.CurrentUserView{
			User:             cloneUser(user),
			AvatarUrl:        avatarURL,
			PresenceStatus:   currentUserPresenceStatus(presence),
			HasVerifiedEmail: hasVerifiedEmail,
			Settings:         settings,
		},
	}, nil
}

func (c *wireConn) handleWireGetAuthenticatedServerSettings(ctx context.Context, requestID string) (*apiv1.GetAuthenticatedServerSettingsResponse, *wirev1.WireError) {
	motd := ""
	if c.server.core != nil && c.server.core.ConfigManager() != nil {
		resolved, err := c.server.core.ConfigManager().GetEffectiveMOTD(ctx)
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		motd = resolved
	}

	assetsCfg := c.server.core.AssetsConfig()
	maxUploadSize := int64(assetsCfg.MaxUploadSize)
	maxVideoUploadSize := maxUploadSize
	if c.server.config.Video.Enabled {
		maxVideoUploadSize = int64(c.server.config.Video.MaxUploadSizeOrDefault())
	}

	settings := &apiv1.AuthenticatedServerSettingsView{
		PushNotificationsEnabled: c.server.config.Push.IsConfigured(),
		VideoProcessingEnabled:   c.server.config.Video.Enabled,
		MaxUploadSize:            maxUploadSize,
		MaxVideoUploadSize:       maxVideoUploadSize,
		MessageEditWindowSeconds: int32(core.MessageEditWindow / time.Second),
		Motd:                     motd,
	}
	if c.server.config.Push.IsConfigured() {
		settings.VapidPublicKey = c.server.config.Push.VAPIDPublicKey
	}
	if c.server.config.LiveKit.IsConfigured() {
		settings.LivekitUrl = c.server.config.LiveKit.URL
	}

	return &apiv1.GetAuthenticatedServerSettingsResponse{Settings: settings}, nil
}

func currentUserPresenceStatus(status string) apiv1.CurrentUserPresenceStatus {
	switch status {
	case core.PresenceStatusOnline:
		return apiv1.CurrentUserPresenceStatus_CURRENT_USER_PRESENCE_STATUS_ONLINE
	case core.PresenceStatusAway:
		return apiv1.CurrentUserPresenceStatus_CURRENT_USER_PRESENCE_STATUS_AWAY
	case core.PresenceStatusDoNotDisturb:
		return apiv1.CurrentUserPresenceStatus_CURRENT_USER_PRESENCE_STATUS_DO_NOT_DISTURB
	default:
		return apiv1.CurrentUserPresenceStatus_CURRENT_USER_PRESENCE_STATUS_OFFLINE
	}
}

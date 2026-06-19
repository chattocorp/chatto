package http_server

import (
	"context"
	"fmt"

	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	wirev1 "hmans.de/chatto/internal/pb/chatto/wire/v1"
)

func (c *wireConn) handleWireGetProfileSettings(ctx context.Context, userID, requestID string) (*apiv1.GetProfileSettingsResponse, *wirev1.WireError) {
	profile, err := c.profileSettingsView(ctx, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.GetProfileSettingsResponse{Profile: profile}, nil
}

func (c *wireConn) handleWireUpdateProfile(ctx context.Context, userID, requestID string, body *apiv1.UpdateProfileRequest) (*apiv1.UpdateProfileResponse, *wirev1.WireError) {
	if body == nil || (body.DisplayName == nil && body.Login == nil) {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: at least one of display_name or login must be provided", errWireInvalidArgument))
	}
	if body.DisplayName != nil && core.NormalizeDisplayName(body.GetDisplayName()) == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: display name cannot be empty", errWireInvalidArgument))
	}

	if body.DisplayName != nil {
		if _, err := c.server.core.UpdateUserDisplayName(ctx, userID, body.GetDisplayName()); err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
	}
	if body.Login != nil {
		if _, err := c.server.core.UpdateUserLogin(ctx, userID, body.GetLogin()); err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
	}

	profile, err := c.profileSettingsView(ctx, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.UpdateProfileResponse{Profile: profile}, nil
}

func (c *wireConn) profileSettingsView(ctx context.Context, userID string) (*apiv1.ProfileSettingsView, error) {
	user, err := c.server.core.GetUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	avatarURL, err := c.server.core.GetUserAvatarURL(ctx, userID, nil, nil, "")
	if err != nil {
		return nil, err
	}
	lastLoginChange, err := c.server.core.GetLastLoginChange(ctx, userID)
	if err != nil {
		return nil, err
	}

	view := &apiv1.ProfileSettingsView{
		User:      cloneUser(user),
		AvatarUrl: avatarURL,
	}
	if !lastLoginChange.IsZero() {
		view.LastLoginChange = timestamppb.New(lastLoginChange)
	}
	return view, nil
}

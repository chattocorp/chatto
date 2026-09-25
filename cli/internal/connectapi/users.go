package connectapi

import (
	"context"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

type userService struct {
	api *API
}

func userSummary(ctx context.Context, api *API, user *evtv1.User, avatar *apiv1.ImageTransformOptions) (*apiv1.User, error) {
	presence, err := api.core.GetUserPresence(ctx, user.GetId())
	if err != nil {
		return nil, err
	}
	return userSummaryWithPresence(ctx, api, user, avatar, presence)
}

func requiredUserSummary(ctx context.Context, api *API, user *evtv1.User) (*apiv1.User, error) {
	if user == nil {
		return nil, core.ErrNotFound
	}
	return userSummary(ctx, api, user, nil)
}

func userSummaryWithPresence(ctx context.Context, api *API, user *evtv1.User, avatar *apiv1.ImageTransformOptions, presence string) (*apiv1.User, error) {
	summary := &apiv1.User{
		Id:             user.GetId(),
		Login:          user.GetLogin(),
		DisplayName:    user.GetDisplayName(),
		Deleted:        user.GetDeleted(),
		PresenceStatus: corePresenceStatusToAPI(presence),
		CustomStatus:   coreCustomStatusToAPI(user.GetCustomStatus()),
	}
	if user.GetIsBot() && !user.GetDeleted() {
		summary.Bot = &apiv1.BotInfo{OwnerUserId: user.GetBotOwnerUserId()}
	}
	if user.GetBio() != "" {
		bio := user.GetBio()
		summary.Bio = &bio
	}
	timezone := ""
	if settings, err := api.core.GetUserSettings(ctx, user.GetId()); err == nil && settings != nil && settings.GetShareTimezone() {
		timezone = settings.GetTimezone()
	}
	if timezone != "" {
		summary.Timezone = &timezone
	}
	avatarURL, err := userAvatarURL(ctx, api, user.GetId(), avatar)
	if err != nil {
		return nil, err
	}
	if avatarURL != "" {
		summary.AvatarUrl = stringPtr(api.absolutizeAssetURL(ctx, avatarURL))
	}
	return summary, nil
}

func userAvatarURL(ctx context.Context, api *API, userID string, avatar *apiv1.ImageTransformOptions) (string, error) {
	if avatar == nil {
		return api.core.GetUserAvatarURL(ctx, userID, nil, nil, "")
	}

	width, height := int(avatar.GetWidth()), int(avatar.GetHeight())
	fit := "cover"
	if avatar.GetFit() == apiv1.ImageFitMode_IMAGE_FIT_MODE_CONTAIN {
		fit = "contain"
	}
	return api.core.GetUserAvatarURL(ctx, userID, &width, &height, fit)
}

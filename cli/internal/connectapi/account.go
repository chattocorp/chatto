package connectapi

import (
	"bytes"
	"context"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type accountService struct {
	api *API
}

func (s *accountService) UpdateProfile(ctx context.Context, req *connect.Request[apiv1.UpdateProfileRequest]) (*connect.Response[apiv1.UpdateProfileResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.DisplayName == nil && req.Msg.Login == nil {
		return nil, invalidArgument("at least one of display_name or login must be provided")
	}

	var updated *corev1.User
	if req.Msg.DisplayName != nil {
		updated, err = s.api.core.UpdateUserDisplayName(ctx, caller.UserID, req.Msg.GetDisplayName())
		if err != nil {
			return nil, connectError(err)
		}
	}
	if req.Msg.Login != nil {
		updated, err = s.api.core.UpdateUserLogin(ctx, caller.UserID, req.Msg.GetLogin())
		if err != nil {
			return nil, connectError(err)
		}
	}
	user, err := s.accountUser(ctx, updated)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.UpdateProfileResponse{User: user}), nil
}

func (s *accountService) UploadAvatar(ctx context.Context, req *connect.Request[apiv1.UploadAvatarRequest]) (*connect.Response[apiv1.UploadAvatarResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if len(req.Msg.GetImage()) == 0 {
		return nil, invalidArgument("image is required")
	}

	asset, err := s.api.core.UploadUserAvatar(ctx, caller.UserID, bytes.NewReader(req.Msg.GetImage()))
	if err != nil {
		return nil, connectError(err)
	}
	if err := s.api.core.SetUserAvatar(ctx, caller.UserID, asset); err != nil {
		s.api.core.CleanupAsset(ctx, core.DeprecatedAssetFromAsset(asset))
		return nil, connectError(err)
	}
	user, err := s.api.core.GetUser(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	responseUser, err := s.accountUser(ctx, user)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.UploadAvatarResponse{User: responseUser}), nil
}

func (s *accountService) DeleteAvatar(ctx context.Context, _ *connect.Request[apiv1.DeleteAvatarRequest]) (*connect.Response[apiv1.DeleteAvatarResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.api.core.DeleteUserAvatar(ctx, caller.UserID); err != nil {
		return nil, connectError(err)
	}
	user, err := s.api.core.GetUser(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	responseUser, err := s.accountUser(ctx, user)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.DeleteAvatarResponse{User: responseUser}), nil
}

func (s *accountService) SetPassword(ctx context.Context, req *connect.Request[apiv1.SetPasswordRequest]) (*connect.Response[apiv1.SetPasswordResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.GetPassword() == "" {
		return nil, invalidArgument("password is required")
	}
	if err := core.ValidatePassword(req.Msg.GetPassword()); err != nil {
		return nil, connectError(err)
	}
	hasPassword, err := s.api.core.HasPassword(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	if !hasPassword {
		if err := s.api.requireFreshCredential(ctx, caller, ""); err != nil {
			return nil, connectError(err)
		}
	}
	if err := s.api.core.SetOwnPassword(ctx, caller.UserID, req.Msg.GetCurrentPassword(), req.Msg.GetPassword()); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.SetPasswordResponse{}), nil
}

func (s *accountService) UpdateSettings(ctx context.Context, req *connect.Request[apiv1.UpdateSettingsRequest]) (*connect.Response[apiv1.UpdateSettingsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	input := core.UserSettingsInput{}
	if req.Msg.Timezone != nil {
		timezone := req.Msg.GetTimezone()
		input.Timezone = &timezone
	}
	if req.Msg.TimeFormat != nil {
		timeFormat := apiTimeFormatToCore(req.Msg.GetTimeFormat())
		input.TimeFormat = &timeFormat
	}
	settings, err := s.api.core.UpdateUserSettings(ctx, caller.UserID, input)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.UpdateSettingsResponse{
		Settings: coreUserSettingsToAPI(settings),
	}), nil
}

func (s *accountService) RequestAccountDeletion(ctx context.Context, _ *connect.Request[apiv1.RequestAccountDeletionRequest]) (*connect.Response[apiv1.RequestAccountDeletionResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}

	token, err := s.api.core.CreateAccountDeletionToken(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.RequestAccountDeletionResponse{
		ConfirmationToken: token,
	}), nil
}

func (s *accountService) DeleteMyAccount(ctx context.Context, req *connect.Request[apiv1.DeleteMyAccountRequest]) (*connect.Response[apiv1.DeleteMyAccountResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.GetConfirmationToken() == "" {
		return nil, invalidArgument("confirmation_token is required")
	}

	if err := s.api.core.ValidateAccountDeletionToken(ctx, req.Msg.GetConfirmationToken(), caller.UserID); err != nil {
		return nil, connectError(err)
	}
	if err := s.api.core.DeleteUser(ctx, caller.UserID, caller.UserID); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.DeleteMyAccountResponse{Deleted: true}), nil
}

func (s *accountService) accountUser(ctx context.Context, user *corev1.User) (*apiv1.User, error) {
	if user == nil {
		return nil, connectError(core.ErrNotFound)
	}
	return (&userService{api: s.api}).userSummary(ctx, user, nil)
}

func apiTimeFormatToCore(format apiv1.TimeFormat) corev1.TimeFormat {
	switch format {
	case apiv1.TimeFormat_TIME_FORMAT_12_HOUR:
		return corev1.TimeFormat_TIME_FORMAT_12H
	case apiv1.TimeFormat_TIME_FORMAT_24_HOUR:
		return corev1.TimeFormat_TIME_FORMAT_24H
	default:
		return corev1.TimeFormat_TIME_FORMAT_UNSPECIFIED
	}
}

package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

type accountService struct {
	api *API
}

func (s *accountService) UpdateProfile(ctx context.Context, req *connect.Request[apiv1.UpdateProfileRequest]) (*connect.Response[apiv1.UpdateProfileResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	req.Msg, err = normalizeUpdateMask(req.Msg)
	if err != nil {
		return nil, err
	}

	updated, err := s.api.core.UpdateOwnUserProfile(ctx, caller.UserID, req.Msg.Login, req.Msg.DisplayName, req.Msg.Bio)
	if err != nil {
		return nil, connectError(err)
	}

	user, err := requiredUserSummary(ctx, s.api, updated)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.UpdateProfileResponse{User: user}), nil
}

func (s *accountService) ChangePassword(ctx context.Context, req *connect.Request[apiv1.ChangePasswordRequest]) (*connect.Response[apiv1.ChangePasswordResponse], error) {
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
	user, err := s.api.core.GetUser(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	responseUser, err := requiredUserSummary(ctx, s.api, user)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.ChangePasswordResponse{User: responseUser}), nil
}

func (s *accountService) UpdateSettings(ctx context.Context, req *connect.Request[apiv1.UpdateSettingsRequest]) (*connect.Response[apiv1.UpdateSettingsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	req.Msg, err = normalizeUpdateMask(req.Msg)
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
	if req.Msg.ShareTimezone != nil {
		shareTimezone := req.Msg.GetShareTimezone()
		input.ShareTimezone = &shareTimezone
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
	// Enforce user.delete-self at redemption so revoking the permission also
	// blocks tokens issued before revocation (see FDR-018). The same gate runs
	// at token issuance in core.
	canDeleteSelf, err := s.api.core.CanDeleteUser(ctx, caller.UserID, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	if !canDeleteSelf {
		return nil, connectError(core.ErrPermissionDenied)
	}

	if err := s.api.core.ValidateAccountDeletionToken(ctx, req.Msg.GetConfirmationToken(), caller.UserID); err != nil {
		return nil, connectError(err)
	}
	if err := s.api.core.DeleteUser(ctx, caller.UserID, caller.UserID); err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.DeleteMyAccountResponse{}), nil
}

func apiTimeFormatToCore(format apiv1.TimeFormat) evtv1.TimeFormat {
	switch format {
	case apiv1.TimeFormat_TIME_FORMAT_12_HOUR:
		return evtv1.TimeFormat_TIME_FORMAT_12H
	case apiv1.TimeFormat_TIME_FORMAT_24_HOUR:
		return evtv1.TimeFormat_TIME_FORMAT_24H
	default:
		return evtv1.TimeFormat_TIME_FORMAT_UNSPECIFIED
	}
}

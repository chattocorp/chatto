package connectapi

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/email"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

type accountService struct {
	api *API
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
		return nil, err
	}
	hasPassword, err := s.api.core.HasPassword(ctx, caller.UserID)
	if err != nil {
		return nil, err
	}
	if !hasPassword {
		if err := s.api.requireFreshCredential(ctx, caller, ""); err != nil {
			return nil, err
		}
	}
	if err := s.api.core.SetOwnPassword(ctx, caller.UserID, req.Msg.GetCurrentPassword(), req.Msg.GetPassword()); err != nil {
		return nil, err
	}
	user, err := s.api.core.GetUser(ctx, caller.UserID)
	if err != nil {
		return nil, err
	}
	responseUser, err := requiredUserSummary(ctx, s.api, user)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.ChangePasswordResponse{User: responseUser}), nil
}

func (s *accountService) ListVerifiedEmails(ctx context.Context, req *connect.Request[apiv1.ListVerifiedEmailsRequest]) (*connect.Response[apiv1.ListVerifiedEmailsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireExpectedUser(caller, req.Msg.GetExpectedUserId()); err != nil {
		return nil, err
	}
	emails, err := s.api.core.GetVerifiedEmails(ctx, caller.UserID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.ListVerifiedEmailsResponse{VerifiedEmails: verifiedEmailsToAPI(emails)}), nil
}

func (s *accountService) RequestEmailVerification(ctx context.Context, req *connect.Request[apiv1.RequestEmailVerificationRequest]) (*connect.Response[apiv1.RequestEmailVerificationResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireExpectedUser(caller, req.Msg.GetExpectedUserId()); err != nil {
		return nil, err
	}
	if s.api.emailSender == nil || !s.api.emailSender.IsEnabled() {
		return nil, connect.NewError(connect.CodeUnavailable, email.ErrEmailDisabled)
	}
	address := strings.ToLower(strings.TrimSpace(req.Msg.GetEmail()))
	verifiedEmails, err := s.api.core.GetVerifiedEmails(ctx, caller.UserID)
	if err != nil {
		return nil, err
	}
	for _, verified := range verifiedEmails {
		if strings.EqualFold(verified.Email, address) {
			return nil, connect.NewError(connect.CodeAlreadyExists, errors.New("email address is already verified"))
		}
	}
	code, err := s.api.core.CreateEmailVerificationCode(ctx, caller.UserID, address)
	if err != nil {
		if errors.Is(err, core.ErrEmailVerificationCodeLimitExceeded) || errors.Is(err, core.ErrEmailVerificationCodeExhausted) {
			return nil, connect.NewError(connect.CodeResourceExhausted, err)
		}
		return nil, err
	}
	serverName := "Chatto"
	if model := s.api.core.ConfigModel(); model != nil {
		if name := strings.TrimSpace(model.GetEffectiveServerName()); name != "" {
			serverName = name
		}
	}
	expiration := emailOTPExpirationText(s.api.config.Auth.EmailOTP.TTLOrDefault())
	err = s.api.emailSender.SendContext(ctx, email.Message{
		To:      address,
		Subject: fmt.Sprintf("Verify your email for %s", serverName),
		Body:    fmt.Sprintf("Use this verification code to add this email address to your %s account:\n\n%s\n\nThis code will expire in %s.\n\nIf you didn't request this, you can ignore this email.", serverName, code, expiration),
	})
	if err != nil {
		_ = s.api.core.CancelEmailVerificationCode(ctx, caller.UserID, address, code)
		return nil, connect.NewError(connect.CodeUnavailable, errors.New("email delivery failed"))
	}
	return connect.NewResponse(&apiv1.RequestEmailVerificationResponse{}), nil
}

func (s *accountService) ConfirmEmailVerification(ctx context.Context, req *connect.Request[apiv1.ConfirmEmailVerificationRequest]) (*connect.Response[apiv1.ConfirmEmailVerificationResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireExpectedUser(caller, req.Msg.GetExpectedUserId()); err != nil {
		return nil, err
	}
	address := strings.ToLower(strings.TrimSpace(req.Msg.GetEmail()))
	if _, err := s.api.core.VerifyEmailCode(ctx, caller.UserID, address, strings.TrimSpace(req.Msg.GetCode())); err != nil {
		if errors.Is(err, core.ErrTokenNotFound) || errors.Is(err, core.ErrTokenExpired) || errors.Is(err, core.ErrEmailVerificationCodeInvalid) || errors.Is(err, core.ErrEmailVerificationCodeExhausted) {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid or expired verification code"))
		}
		return nil, err
	}
	emails, err := s.api.core.GetVerifiedEmails(ctx, caller.UserID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.ConfirmEmailVerificationResponse{VerifiedEmails: verifiedEmailsToAPI(emails)}), nil
}

func (s *accountService) SetPrimaryEmail(ctx context.Context, req *connect.Request[apiv1.SetPrimaryEmailRequest]) (*connect.Response[apiv1.SetPrimaryEmailResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := requireExpectedUser(caller, req.Msg.GetExpectedUserId()); err != nil {
		return nil, err
	}
	if err := s.api.core.SetPrimaryVerifiedEmail(ctx, caller.UserID, req.Msg.GetEmail()); err != nil {
		return nil, err
	}
	emails, err := s.api.core.GetVerifiedEmails(ctx, caller.UserID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.SetPrimaryEmailResponse{VerifiedEmails: verifiedEmailsToAPI(emails)}), nil
}

// requireExpectedUser prevents a stale browser tab from applying an ambient
// cookie session for one account to self-service state that it loaded for a
// different account.
func requireExpectedUser(caller Caller, expectedUserID string) error {
	if expectedUserID == "" || caller.UserID != expectedUserID {
		return connect.NewError(connect.CodeFailedPrecondition, errors.New("authenticated account changed"))
	}
	return nil
}

func verifiedEmailsToAPI(emails []core.VerifiedEmail) []*apiv1.VerifiedEmail {
	out := make([]*apiv1.VerifiedEmail, 0, len(emails))
	for _, verified := range emails {
		out = append(out, &apiv1.VerifiedEmail{
			Email: verified.Email, VerifiedAt: timestamppb.New(verified.VerifiedAt), Primary: verified.Primary,
		})
	}
	return out
}

func emailOTPExpirationText(ttl time.Duration) string {
	switch {
	case ttl%time.Hour == 0:
		return pluralDuration(int(ttl/time.Hour), "hour")
	case ttl%time.Minute == 0:
		return pluralDuration(int(ttl/time.Minute), "minute")
	default:
		return pluralDuration(int(ttl/time.Second), "second")
	}
}

func pluralDuration(value int, unit string) string {
	if value == 1 {
		return fmt.Sprintf("1 %s", unit)
	}
	return fmt.Sprintf("%d %ss", value, unit)
}

func (s *accountService) GetSettings(ctx context.Context, _ *connect.Request[apiv1.GetSettingsRequest]) (*connect.Response[apiv1.GetSettingsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := s.api.core.GetUserSettings(ctx, caller.UserID)
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.GetSettingsResponse{Settings: coreUserSettingsToAPI(settings)}), nil
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
		return nil, err
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
		return nil, err
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
		return nil, err
	}
	if !canDeleteSelf {
		return nil, core.ErrPermissionDenied
	}

	if err := s.api.core.ValidateAccountDeletionToken(ctx, req.Msg.GetConfirmationToken(), caller.UserID); err != nil {
		return nil, err
	}
	if err := s.api.core.DeleteUser(ctx, caller.UserID, caller.UserID); err != nil {
		return nil, err
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

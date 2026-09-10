package connectapi

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	authv1 "hmans.de/chatto/internal/pb/chatto/auth/v1"
)

type serverSetupService struct{ api *API }

func (s *serverSetupService) CompleteSetup(ctx context.Context, req *connect.Request[authv1.CompleteSetupRequest]) (*connect.Response[authv1.CompleteSetupResponse], error) {
	if !s.api.config.Auth.DirectLoginOrDefault() {
		return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("enable direct login before first-run setup"))
	}
	err := s.api.core.CompleteServerSetup(ctx, core.ServerSetupInput{
		ServerName: req.Msg.ServerName, Description: req.Msg.Description,
		Login: req.Msg.Login, DisplayName: req.Msg.DisplayName, Password: req.Msg.Password,
	})
	if err != nil {
		return nil, setupConnectError(err)
	}
	response := connect.NewResponse(&authv1.CompleteSetupResponse{})
	response.Header().Set("Cache-Control", "no-store")
	return response, nil
}

// setupConnectError identifies account fields without exposing submitted values.
// Chatto-Error-Field contains the CompleteSetup request field name. Clients can
// use it to place validation feedback beside the field; other errors stay global.
func setupConnectError(err error) error {
	mapped := connectError(err)
	var field string
	switch {
	case errors.Is(err, core.ErrLoginTooShort), errors.Is(err, core.ErrLoginTooLong),
		errors.Is(err, core.ErrLoginInvalidCharacter), errors.Is(err, core.ErrHumanLoginReservedForBot),
		errors.Is(err, core.ErrUsernameBlocked), errors.Is(err, core.ErrLoginAlreadyTaken):
		field = "login"
	case errors.Is(err, core.ErrDisplayNameTooLong), errors.Is(err, core.ErrDisplayNameInvalidCharacter), errors.Is(err, core.ErrDisplayNameInvalidStart):
		field = "display_name"
	case errors.Is(err, core.ErrPasswordTooShort), errors.Is(err, core.ErrPasswordTooLong):
		field = "password"
	}
	var connectErr *connect.Error
	if field != "" && errors.As(mapped, &connectErr) {
		connectErr.Meta().Set("Chatto-Error-Field", field)
	}
	return mapped
}

package managementapi

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	managementv1 "hmans.de/chatto/internal/pb/chatto/management/v1"
)

type userAdminService struct {
	api *API
}

func (s *userAdminService) CreateUser(ctx context.Context, req *connect.Request[managementv1.CreateUserRequest]) (*connect.Response[managementv1.CreateUserResponse], error) {
	login := strings.TrimSpace(req.Msg.GetLogin())
	displayName := strings.TrimSpace(req.Msg.GetDisplayName())
	password := req.Msg.GetPassword()
	verifiedEmail := strings.TrimSpace(req.Msg.GetVerifiedEmail())
	if login == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("login is required"))
	}
	if displayName == "" {
		displayName = login
	}
	if verifiedEmail != "" {
		claimed, err := s.api.core.IsEmailClaimed(ctx, verifiedEmail)
		if err != nil {
			return nil, managementError(err)
		}
		if claimed {
			return nil, managementError(core.ErrEmailAlreadyVerified)
		}
	}

	var (
		user *managementv1.ManagedUser
		err  error
	)
	if verifiedEmail != "" {
		created, createErr := s.api.core.CreateVerifiedUserAs(ctx, OperatorActorID, login, displayName, password, verifiedEmail)
		err = createErr
		user = managementUser(created)
	} else {
		created, createErr := s.api.core.CreateUserAs(ctx, OperatorActorID, login, displayName, password)
		err = createErr
		user = managementUser(created)
	}
	if err != nil {
		return nil, managementError(err)
	}

	return connect.NewResponse(&managementv1.CreateUserResponse{User: user}), nil
}

func (s *userAdminService) UpdateUser(ctx context.Context, req *connect.Request[managementv1.UpdateUserRequest]) (*connect.Response[managementv1.UpdateUserResponse], error) {
	target, err := s.api.resolveUser(ctx, req.Msg.GetUser())
	if err != nil {
		return nil, err
	}

	newLogin := strings.TrimSpace(req.Msg.GetLogin())
	newDisplayName := strings.TrimSpace(req.Msg.GetDisplayName())
	if newLogin == "" && newDisplayName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("login or display_name is required"))
	}

	updated := target
	if newDisplayName != "" {
		updated, err = s.api.core.AdminUpdateUserDisplayNameAs(ctx, OperatorActorID, target.GetId(), newDisplayName)
		if err != nil {
			return nil, managementError(err)
		}
	}
	if newLogin != "" {
		updated, err = s.api.core.AdminUpdateUserLoginAs(ctx, OperatorActorID, target.GetId(), newLogin)
		if err != nil {
			return nil, managementError(err)
		}
	}

	return connect.NewResponse(&managementv1.UpdateUserResponse{User: managementUser(updated)}), nil
}

func (s *userAdminService) SetUserPassword(ctx context.Context, req *connect.Request[managementv1.SetUserPasswordRequest]) (*connect.Response[managementv1.SetUserPasswordResponse], error) {
	target, err := s.api.resolveUser(ctx, req.Msg.GetUser())
	if err != nil {
		return nil, err
	}
	if req.Msg.GetPassword() == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("password is required"))
	}

	if err := s.api.core.SetPasswordHashAs(ctx, OperatorActorID, target.GetId(), req.Msg.GetPassword()); err != nil {
		return nil, managementError(err)
	}
	updated, err := s.api.core.GetUser(ctx, target.GetId())
	if err != nil {
		return nil, managementError(err)
	}
	return connect.NewResponse(&managementv1.SetUserPasswordResponse{User: managementUser(updated)}), nil
}

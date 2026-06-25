package managementapi

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"connectrpc.com/connect"
	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/core"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	managementv1 "hmans.de/chatto/internal/pb/chatto/management/v1"
	"hmans.de/chatto/internal/pb/chatto/management/v1/managementv1connect"
)

const OperatorActorID = "system:operator"

type Handler struct {
	ServicePath string
	Handler     http.Handler
}

type API struct {
	core *core.ChattoCore
}

func New(c *core.ChattoCore) *API {
	return &API{core: c}
}

func (a *API) Handlers() []Handler {
	userAdminPath, userAdminHandler := managementv1connect.NewUserAdminServiceHandler(&userAdminService{api: a})
	return []Handler{{ServicePath: userAdminPath, Handler: userAdminHandler}}
}

func (a *API) resolveUser(ctx context.Context, selector *managementv1.UserSelector) (*corev1.User, error) {
	if selector == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user selector is required"))
	}
	switch v := selector.Selector.(type) {
	case *managementv1.UserSelector_UserId:
		userID := strings.TrimSpace(v.UserId)
		if userID == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user_id is required"))
		}
		user, err := a.core.GetUser(ctx, userID)
		if err != nil {
			return nil, managementError(err)
		}
		return user, nil
	case *managementv1.UserSelector_Login:
		login := strings.TrimSpace(v.Login)
		if login == "" {
			return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("login is required"))
		}
		user, err := a.core.GetUserByLogin(ctx, login)
		if err != nil {
			return nil, managementError(err)
		}
		return user, nil
	default:
		return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("user selector must set user_id or login"))
	}
}

func managementUser(user *corev1.User) *managementv1.ManagedUser {
	if user == nil {
		return nil
	}
	return &managementv1.ManagedUser{
		Id:          user.GetId(),
		Login:       user.GetLogin(),
		DisplayName: user.GetDisplayName(),
	}
}

func managementError(err error) error {
	if err == nil {
		return nil
	}
	if connect.CodeOf(err) != connect.CodeUnknown {
		return err
	}
	if errors.Is(err, core.ErrNotFound) || errors.Is(err, jetstream.ErrKeyNotFound) {
		return connect.NewError(connect.CodeNotFound, err)
	}
	if errors.Is(err, core.ErrPermissionDenied) {
		return connect.NewError(connect.CodePermissionDenied, err)
	}
	if errors.Is(err, core.ErrUsernameBlocked) ||
		errors.Is(err, core.ErrLoginAlreadyTaken) ||
		errors.Is(err, core.ErrDisplayNameTooLong) ||
		errors.Is(err, core.ErrDisplayNameInvalidCharacter) ||
		errors.Is(err, core.ErrDisplayNameInvalidStart) ||
		errors.Is(err, core.ErrPasswordTooShort) ||
		errors.Is(err, core.ErrPasswordTooLong) ||
		errors.Is(err, core.ErrEmailAlreadyVerified) ||
		errors.Is(err, core.ErrLimitExceeded) ||
		errors.Is(err, core.ErrLoginTooShort) ||
		errors.Is(err, core.ErrLoginTooLong) ||
		errors.Is(err, core.ErrLoginInvalidCharacter) {
		return connect.NewError(connect.CodeInvalidArgument, err)
	}
	return connect.NewError(connect.CodeInternal, err)
}

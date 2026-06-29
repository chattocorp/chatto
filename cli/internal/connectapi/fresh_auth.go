package connectapi

import (
	"context"
	"errors"

	"hmans.de/chatto/internal/authctx"
	"hmans.de/chatto/internal/core"
)

func (a *API) requireFreshCredential(ctx context.Context, caller Caller, currentPassword string) error {
	credential, ok := authctx.CredentialForContext(ctx)
	if !ok || credential.UserID != caller.UserID {
		return core.ErrFreshAuthRequired
	}

	if err := a.requireCredentialFresh(ctx, credential); err == nil {
		return nil
	} else if !errors.Is(err, core.ErrFreshAuthRequired) {
		return err
	}

	if currentPassword == "" {
		return core.ErrFreshAuthRequired
	}
	if err := a.core.VerifyUserPassword(ctx, caller.UserID, currentPassword); err != nil {
		return err
	}
	if err := a.markCredentialFresh(ctx, credential, "password", "current_password"); err != nil {
		return err
	}
	return nil
}

func (a *API) requireCredentialFresh(ctx context.Context, credential authctx.RuntimeCredential) error {
	switch credential.Kind {
	case authctx.RuntimeCredentialKindBearerToken:
		return a.core.RequireFreshAuthForBearerToken(ctx, credential.BearerToken)
	case authctx.RuntimeCredentialKindCookieSession:
		return a.core.RequireFreshAuthForCookieSession(ctx, credential.UserID, credential.CookieSessionID)
	default:
		return core.ErrFreshAuthRequired
	}
}

func (a *API) markCredentialFresh(ctx context.Context, credential authctx.RuntimeCredential, method, source string) error {
	switch credential.Kind {
	case authctx.RuntimeCredentialKindBearerToken:
		return a.core.MarkBearerTokenFresh(ctx, credential.BearerToken, method, source)
	case authctx.RuntimeCredentialKindCookieSession:
		return a.core.MarkCookieSessionFresh(ctx, credential.UserID, credential.CookieSessionID, method, source)
	default:
		return core.ErrFreshAuthRequired
	}
}

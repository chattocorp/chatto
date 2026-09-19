package connectapi

import (
	"context"
	"errors"

	"hmans.de/chatto/internal/authctx"
	"hmans.de/chatto/internal/core"
)

// requireFreshCredentialOrPassword accepts password proof for this operation
// without granting fresh-auth privileges to a delegated session.
func (a *API) requireFreshCredentialOrPassword(ctx context.Context, caller Caller, currentPassword string) error {
	_, _, err := a.verifyFreshCredential(ctx, caller, currentPassword)
	return err
}

func (a *API) requireFreshCredential(ctx context.Context, caller Caller, currentPassword string) error {
	credential, passwordVerified, err := a.verifyFreshCredential(ctx, caller, currentPassword)
	if err != nil || !passwordVerified {
		return err
	}
	return a.markCredentialFresh(ctx, credential, "password", "current_password")
}

// verifyFreshCredential checks either existing freshness or current-password
// proof. The bool reports password verification; only callers that explicitly
// upgrade the credential may persist fresh-auth status afterward.
func (a *API) verifyFreshCredential(ctx context.Context, caller Caller, currentPassword string) (authctx.RuntimeCredential, bool, error) {
	credential, ok := authctx.CredentialForContext(ctx)
	if !ok || credential.UserID != caller.UserID {
		return credential, false, core.ErrFreshAuthRequired
	}

	if err := a.requireCredentialFresh(ctx, credential); err == nil {
		return credential, false, nil
	} else if !errors.Is(err, core.ErrFreshAuthRequired) {
		return credential, false, err
	}

	if currentPassword == "" {
		return credential, false, core.ErrFreshAuthRequired
	}
	if err := a.core.VerifyUserPassword(ctx, caller.UserID, currentPassword); err != nil {
		return credential, false, err
	}
	return credential, true, nil
}

func (a *API) requireCredentialFresh(ctx context.Context, credential authctx.RuntimeCredential) error {
	switch credential.Kind {
	case authctx.RuntimeCredentialKindBearerToken:
		return a.core.RequireFreshAuthForBearerToken(ctx, credential.Handle)
	case authctx.RuntimeCredentialKindCookieSession:
		return a.core.RequireFreshAuthForCookieSession(ctx, credential.Handle)
	default:
		return core.ErrFreshAuthRequired
	}
}

func (a *API) markCredentialFresh(ctx context.Context, credential authctx.RuntimeCredential, method, source string) error {
	switch credential.Kind {
	case authctx.RuntimeCredentialKindBearerToken:
		return a.core.MarkBearerTokenFresh(ctx, credential.Handle, method, source)
	case authctx.RuntimeCredentialKindCookieSession:
		return a.core.MarkCookieSessionFresh(ctx, credential.Handle, method, source)
	default:
		return core.ErrFreshAuthRequired
	}
}

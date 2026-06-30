package authctx

import (
	"context"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// contextKey is an unexported type for context keys to prevent collisions.
type contextKey struct {
	name string
}

var userCtxKey = &contextKey{"user"}
var credentialCtxKey = &contextKey{"runtime_credential"}

// RuntimeCredentialKind identifies the runtime credential that authenticated a
// request.
type RuntimeCredentialKind string

const (
	RuntimeCredentialKindBearerToken   RuntimeCredentialKind = "bearer_token"
	RuntimeCredentialKindCookieSession RuntimeCredentialKind = "cookie_session"
)

// RuntimeCredential identifies the concrete runtime credential that
// authenticated a request. Handle is the opaque credential value as presented
// through the transport identified by Kind.
type RuntimeCredential struct {
	Kind   RuntimeCredentialKind
	UserID string
	Handle string
}

// ForContext extracts the authenticated user from the request context.
// Returns nil if no user is authenticated.
func ForContext(ctx context.Context) *corev1.User {
	raw, _ := ctx.Value(userCtxKey).(*corev1.User)
	return raw
}

// WithUser returns a new context with the authenticated user injected.
func WithUser(ctx context.Context, user *corev1.User) context.Context {
	return context.WithValue(ctx, userCtxKey, user)
}

// CredentialForContext extracts the runtime credential that authenticated the
// request. It returns false for unauthenticated requests or auth paths that do
// not use a persisted runtime credential.
func CredentialForContext(ctx context.Context) (RuntimeCredential, bool) {
	raw, _ := ctx.Value(credentialCtxKey).(RuntimeCredential)
	return raw, raw.Kind != "" && raw.UserID != "" && raw.Handle != ""
}

// WithCredential returns a new context with the authenticating runtime
// credential injected.
func WithCredential(ctx context.Context, credential RuntimeCredential) context.Context {
	return context.WithValue(ctx, credentialCtxKey, credential)
}

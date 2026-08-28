package authctx

import (
	"context"
	"time"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
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
	RuntimeCredentialKindBotAPIKey     RuntimeCredentialKind = "bot_api_key"
)

// RuntimeCredential identifies the concrete runtime credential that
// authenticated a request. Handle identifies the credential for follow-up
// lifecycle checks. It is the presented opaque value for runtime tokens and
// sessions, but a stable, non-secret identifier for credentials such as bot
// API keys. OAuthClientID is present only for OAuth access tokens and lets
// long-lived transports enforce client policy after their initial
// authentication. BotAPIKeyVerifier is the non-secret HMAC verifier generation
// for a bot key; it lets those transports observe durable key rotation without
// retaining the raw key.
type RuntimeCredential struct {
	Kind              RuntimeCredentialKind
	UserID            string
	Handle            string
	OAuthClientID     string
	BotAPIKeyVerifier []byte
	ExpiresAt         time.Time
}

// ForContext extracts the authenticated user from the request context.
// Returns nil if no user is authenticated.
func ForContext(ctx context.Context) *evtv1.User {
	raw, _ := ctx.Value(userCtxKey).(*evtv1.User)
	return raw
}

// WithUser returns a new context with the authenticated user injected.
func WithUser(ctx context.Context, user *evtv1.User) context.Context {
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

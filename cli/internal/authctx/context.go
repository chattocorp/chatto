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

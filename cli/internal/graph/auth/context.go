package auth

import (
	"context"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// contextKey is an unexported type for context keys to prevent collisions
type contextKey struct {
	name string
}

var userCtxKey = &contextKey{"user"}
var connectPresenceReportingCtxKey = &contextKey{"connect_presence_reporting"}

// ForContext extracts the authenticated user from the GraphQL context.
// Returns nil if no user is authenticated.
func ForContext(ctx context.Context) *corev1.User {
	raw, _ := ctx.Value(userCtxKey).(*corev1.User)
	return raw
}

// WithUser returns a new context with the user injected
func WithUser(ctx context.Context, user *corev1.User) context.Context {
	return context.WithValue(ctx, userCtxKey, user)
}

// WithConnectPresenceReporting marks a GraphQL WebSocket connection whose
// client reports live presence through the ConnectRPC PresenceService instead
// of relying on myEvents' legacy implicit presence refresh.
func WithConnectPresenceReporting(ctx context.Context) context.Context {
	return context.WithValue(ctx, connectPresenceReportingCtxKey, true)
}

// UsesConnectPresenceReporting reports whether myEvents should skip legacy
// implicit presence writes for this connection.
func UsesConnectPresenceReporting(ctx context.Context) bool {
	raw, _ := ctx.Value(connectPresenceReportingCtxKey).(bool)
	return raw
}

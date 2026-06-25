package connectapi

import (
	"context"
	"errors"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
)

// Caller is the authenticated identity available to ConnectRPC handlers.
// Keep this intentionally narrow: operation services should receive only the
// actor identity they need, not the full user profile resolved at the HTTP edge.
type Caller struct {
	UserID string
}

func requireCaller(ctx context.Context) (Caller, error) {
	caller, ok := authn.GetInfo(ctx).(Caller)
	if !ok || caller.UserID == "" {
		return Caller{}, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	return caller, nil
}

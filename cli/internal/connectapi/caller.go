package connectapi

import (
	"context"
	"errors"

	"connectrpc.com/authn"
	"connectrpc.com/connect"
)

// Caller is the authenticated identity available to ConnectRPC handlers.
// Keep this intentionally narrow: operation models should receive only the
// actor identity they need, not the full user profile resolved at the HTTP edge.
type Caller struct {
	UserID string
}

// AdminCaller is the authenticated operator identity available to AdminService.
// It is intentionally separate from Caller so normal user bearer/cookie auth
// cannot satisfy operator-only handlers.
type AdminCaller struct {
	TokenName string
}

func requireCaller(ctx context.Context) (Caller, error) {
	caller, ok := authn.GetInfo(ctx).(Caller)
	if !ok || caller.UserID == "" {
		return Caller{}, connect.NewError(connect.CodeUnauthenticated, errors.New("authentication required"))
	}
	return caller, nil
}

func requireAdminCaller(ctx context.Context) (AdminCaller, error) {
	caller, ok := authn.GetInfo(ctx).(AdminCaller)
	if !ok {
		return AdminCaller{}, connect.NewError(connect.CodeUnauthenticated, errors.New("admin token required"))
	}
	return caller, nil
}

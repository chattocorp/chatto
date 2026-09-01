package connectapi

import (
	"context"
	"errors"
	"strings"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
)

// RealtimeMinimumCursorHeader asks a ConnectRPC read to wait until the serving
// replica can return projection state that includes at least this opaque
// realtime boundary.
const RealtimeMinimumCursorHeader = "Chatto-Realtime-Minimum-Cursor"

func (a *API) realtimeConsistencyInterceptor() connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			cursor := strings.TrimSpace(req.Header().Get(RealtimeMinimumCursorHeader))
			if cursor == "" {
				return next(ctx, req)
			}
			caller, err := requireCaller(ctx)
			if err != nil {
				return nil, err
			}
			if err := a.core.WaitForRealtimeCursor(ctx, caller.UserID, cursor); err != nil {
				switch {
				case errors.Is(err, core.ErrRealtimeCursorInvalid):
					return nil, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid realtime minimum cursor"))
				case errors.Is(err, core.ErrRealtimeCursorExpired):
					return nil, connect.NewError(connect.CodeFailedPrecondition, errors.New("realtime minimum cursor expired"))
				case errors.Is(err, context.Canceled):
					return nil, connect.NewError(connect.CodeCanceled, errors.New("realtime consistency wait canceled"))
				case errors.Is(err, context.DeadlineExceeded):
					return nil, connect.NewError(connect.CodeDeadlineExceeded, errors.New("realtime consistency wait timed out"))
				default:
					return nil, connect.NewError(connect.CodeUnavailable, errors.New("realtime consistency boundary unavailable"))
				}
			}
			return next(ctx, req)
		}
	})
}

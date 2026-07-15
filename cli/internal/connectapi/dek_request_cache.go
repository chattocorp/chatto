package connectapi

import (
	"context"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
)

// dekRequestCacheInterceptor gives every unary Connect request one shared,
// request-bounded cache for unwrapped user DEKs. Core hydration methods still
// establish the cache defensively for callers outside Connect.
func dekRequestCacheInterceptor() connect.Interceptor {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			return next(core.WithDEKRequestCache(ctx), req)
		}
	})
}

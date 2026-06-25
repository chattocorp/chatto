package connectapi

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type authenticatedUser = *corev1.User

type authedUnaryHandler[Res any] func(context.Context, authenticatedUser) (*Res, error)

func handleAuthedUnary[Res any](ctx context.Context, handler authedUnaryHandler[Res]) (*connect.Response[Res], error) {
	user, err := requireAuth(ctx)
	if err != nil {
		return nil, err
	}
	resp, err := handler(ctx, user)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(resp), nil
}

func requireAuthedUnaryInterceptor() connect.UnaryInterceptorFunc {
	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			if _, err := requireAuth(ctx); err != nil {
				return nil, err
			}
			resp, err := next(ctx, req)
			if err != nil {
				return nil, connectError(err)
			}
			return resp, nil
		}
	})
}

func invalidArgument(message string) error {
	return connect.NewError(connect.CodeInvalidArgument, errors.New(message))
}

package connectapi

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"
	"hmans.de/chatto/internal/core"
)

func TestHandlerOptionsEstablishDEKRequestCache(t *testing.T) {
	const procedure = "/chatto.test.v1.CacheService/Get"
	handler := connect.NewUnaryHandler(
		procedure,
		func(ctx context.Context, _ *connect.Request[emptypb.Empty]) (*connect.Response[emptypb.Empty], error) {
			if got := core.WithDEKRequestCache(ctx); got != ctx {
				return nil, connect.NewError(connect.CodeInternal, errors.New("handler context does not contain a DEK request cache"))
			}
			return connect.NewResponse(&emptypb.Empty{}), nil
		},
		HandlerOptions()...,
	)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client := connect.NewClient[emptypb.Empty, emptypb.Empty](server.Client(), server.URL+procedure)
	if _, err := client.CallUnary(context.Background(), connect.NewRequest(&emptypb.Empty{})); err != nil {
		t.Fatalf("CallUnary: %v", err)
	}
}

func TestDEKRequestCacheInterceptorPreservesExistingCache(t *testing.T) {
	ctx := core.WithDEKRequestCache(context.Background())
	wrapped := dekRequestCacheInterceptor().WrapUnary(func(got context.Context, _ connect.AnyRequest) (connect.AnyResponse, error) {
		if got != ctx {
			t.Fatal("interceptor replaced the existing DEK request cache context")
		}
		return nil, nil
	})

	if _, err := wrapped(ctx, nil); err != nil {
		t.Fatalf("wrapped handler: %v", err)
	}
}

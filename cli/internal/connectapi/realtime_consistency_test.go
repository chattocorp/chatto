package connectapi

import (
	"context"
	"testing"

	"connectrpc.com/connect"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

func TestRealtimeConsistencyInterceptorValidatesViewerBoundCursor(t *testing.T) {
	env := newConnectAPITestEnv(t)
	plan, err := env.core.PlanRealtimeReplay(env.ctx, env.viewer.Id, "")
	if err != nil {
		t.Fatalf("PlanRealtimeReplay: %v", err)
	}

	request := connect.NewRequest(&apiv1.GetViewerRequest{})
	request.Header().Set(RealtimeMinimumCursorHeader, plan.BoundaryCursor)
	called := false
	wrapped := env.api.realtimeConsistencyInterceptor().WrapUnary(
		func(context.Context, connect.AnyRequest) (connect.AnyResponse, error) {
			called = true
			return connect.NewResponse(&apiv1.GetViewerResponse{}), nil
		},
	)
	if _, err := wrapped(withCaller(env.ctx, env.viewer), request); err != nil {
		t.Fatalf("valid minimum cursor: %v", err)
	}
	if !called {
		t.Fatal("valid minimum cursor did not invoke handler")
	}

	other, err := env.core.CreateUser(env.ctx, env.viewer.Id, "cursor-other", "Cursor Other", "password")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	called = false
	if _, err := wrapped(withCaller(env.ctx, other), request); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("foreign cursor code = %v, want invalid_argument", connect.CodeOf(err))
	}
	if called {
		t.Fatal("foreign cursor invoked handler")
	}
}

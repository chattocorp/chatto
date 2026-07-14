package connectapi

import (
	"bytes"
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/charmbracelet/log"
	"google.golang.org/protobuf/types/known/emptypb"
)

func TestInternalErrorLoggingIncludesProcedureWithoutExposingCause(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := log.Default()
	log.SetDefault(log.New(&logs))
	t.Cleanup(func() { log.SetDefault(previousLogger) })

	const procedure = "/chatto.test.v1.ErrorService/Fail"
	cause := errors.New("database exploded for email=person@example.test")
	handler := connect.NewUnaryHandler(
		procedure,
		func(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[emptypb.Empty], error) {
			return nil, connectError(cause)
		},
		connect.WithInterceptors(internalErrorLoggingInterceptor()),
	)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client := connect.NewClient[emptypb.Empty, emptypb.Empty](
		server.Client(),
		server.URL+procedure,
	)
	_, err := client.CallUnary(context.Background(), connect.NewRequest(&emptypb.Empty{}))
	if connect.CodeOf(err) != connect.CodeInternal {
		t.Fatalf("CallUnary code = %v, want internal (err=%v)", connect.CodeOf(err), err)
	}
	if strings.Contains(err.Error(), "database exploded") || strings.Contains(err.Error(), "person@example.test") {
		t.Fatalf("client error exposed internal cause: %v", err)
	}
	if !strings.Contains(err.Error(), "internal server error") {
		t.Fatalf("client error = %v, want generic internal message", err)
	}

	gotLogs := logs.String()
	if count := strings.Count(gotLogs, "Connect API internal error"); count != 1 {
		t.Fatalf("internal error log count = %d, want 1; logs=%q", count, gotLogs)
	}
	if !strings.Contains(gotLogs, procedure) || !strings.Contains(gotLogs, "database exploded") {
		t.Fatalf("internal error log missing procedure or cause: %q", gotLogs)
	}
	if strings.Contains(gotLogs, "person@example.test") || !strings.Contains(gotLogs, "[redacted]") {
		t.Fatalf("internal error log did not preserve redaction: %q", gotLogs)
	}
}

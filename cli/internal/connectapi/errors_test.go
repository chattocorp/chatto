package connectapi

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

	"connectrpc.com/connect"
	"github.com/charmbracelet/log"
	"google.golang.org/protobuf/types/known/emptypb"
	"hmans.de/chatto/internal/core"
)

func TestHandlerOptionsLogUnmappedErrorsWithoutExposingCause(t *testing.T) {
	var logs bytes.Buffer
	previousLogger := log.Default()
	log.SetDefault(log.New(&logs))
	t.Cleanup(func() { log.SetDefault(previousLogger) })

	const procedure = "/chatto.test.v1.ErrorService/Fail"
	cause := errors.New("database exploded for email=person@example.test")
	handler := connect.NewUnaryHandler(
		procedure,
		func(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[emptypb.Empty], error) {
			return nil, cause
		},
		HandlerOptions()...,
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

func TestConnectErrorMapsInvalidInvitation(t *testing.T) {
	if got := errorCode((core.ErrInvitationInvalid)); got != connect.CodeInvalidArgument {
		t.Fatalf("connectError code = %v, want invalid argument", got)
	}
}

func TestConnectErrorMapsContextTermination(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want connect.Code
	}{
		{name: "canceled", err: context.Canceled, want: connect.CodeCanceled},
		{name: "wrapped canceled", err: fmt.Errorf("operation: %w", context.Canceled), want: connect.CodeCanceled},
		{name: "deadline exceeded", err: context.DeadlineExceeded, want: connect.CodeDeadlineExceeded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := errorCode((tt.err)); got != tt.want {
				t.Fatalf("connectError code = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHandlerOptionsMapCoreErrors(t *testing.T) {
	const procedure = "/chatto.test.v1.ErrorService/Fail"
	tests := []struct {
		name string
		err  error
		want connect.Code
	}{
		{name: "core sentinel", err: fmt.Errorf("load room: %w", core.ErrNotFound), want: connect.CodeNotFound},
		{name: "existing connect error", err: connect.NewError(connect.CodeUnavailable, errors.New("busy")), want: connect.CodeUnavailable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := connect.NewUnaryHandler(
				procedure,
				func(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[emptypb.Empty], error) {
					return nil, tt.err
				},
				HandlerOptions()...,
			)
			server := httptest.NewServer(handler)
			t.Cleanup(server.Close)
			client := connect.NewClient[emptypb.Empty, emptypb.Empty](server.Client(), server.URL+procedure)
			_, err := client.CallUnary(context.Background(), connect.NewRequest(&emptypb.Empty{}))
			if got := connect.CodeOf(err); got != tt.want {
				t.Fatalf("CallUnary code = %v, want %v (err=%v)", got, tt.want, err)
			}
		})
	}
}

func TestConnectErrorUsesFirstMatchingRow(t *testing.T) {
	err := errors.Join(core.ErrNotFound, core.ErrPermissionDenied)
	if got := errorCode(err); got != connect.CodePermissionDenied {
		t.Fatalf("errorCode = %v, want permission denied from the earlier row", got)
	}
}

func TestConnectErrorCodesListEachErrorOnce(t *testing.T) {
	seen := make(map[error]connect.Code)
	for _, row := range connectErrorCodes {
		for _, target := range row.errs {
			if previous, ok := seen[target]; ok {
				t.Fatalf("%v is listed for %v and %v; only the first row can match", target, previous, row.code)
			}
			seen[target] = row.code
			if got := errorCode(target); got != row.code {
				t.Fatalf("errorCode(%v) = %v, want %v", target, got, row.code)
			}
		}
	}
}

func TestConnectErrorKeepsNotModifiedErrors(t *testing.T) {
	err := connectError(connect.NewNotModifiedError(nil))
	if !connect.IsNotModifiedError(err) {
		t.Fatalf("connectError(not modified) = %v, want a not-modified error", err)
	}
}

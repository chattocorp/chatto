package linkpreview

import (
	"context"
	"errors"
	"net"
)

// LogErrorKind returns a fixed category for preview diagnostics. Never log the
// original error or its unwrapped causes: HTTP, DNS, parser, and storage errors
// can contain private URLs, addresses, headers, or fetched content.
func LogErrorKind(err error) string {
	if errors.Is(err, context.Canceled) {
		return "canceled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	if networkError, ok := errors.AsType[net.Error](err); ok {
		if networkError.Timeout() {
			return "timeout"
		}
		return "network"
	}
	return "failed"
}

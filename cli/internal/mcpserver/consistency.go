package mcpserver

import (
	"errors"
	"net/http"
	"strings"

	"github.com/chattocorp/chatto/cli/internal/core"
)

// withRealtimeMinimum applies the same viewer-bound content barrier as
// ConnectRPC, after bearer and scope validation and before any tool effects.
// It does not grant access or replay mutations when the wait fails.
func withRealtimeMinimum(c *core.ChattoCore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cursor := strings.TrimSpace(r.Header.Get("Chatto-Realtime-Minimum-Cursor"))
		if cursor == "" || r.Header.Get("Mcp-Method") != "tools/call" {
			next.ServeHTTP(w, r)
			return
		}
		ctx, actor, err := authenticatedRequest(r.Context())
		if err == nil {
			err = c.WaitForRealtimeCursor(ctx, actor, cursor)
		}
		if err != nil {
			f := classifyToolError(err, "")
			status := http.StatusServiceUnavailable
			if errors.Is(err, core.ErrRealtimeCursorInvalid) || errors.Is(err, core.ErrRealtimeCursorExpired) {
				f = failure("invalid_argument", "Realtime minimum cursor is invalid or expired; refresh the realtime boundary.", "fix_arguments", "after_change")
				status = http.StatusBadRequest
			}
			writeHTTPFailure(w, status, f)
			return
		}
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

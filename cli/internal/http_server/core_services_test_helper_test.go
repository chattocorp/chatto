package http_server

import (
	"testing"

	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/chattocorp/chatto/cli/internal/testutil"
)

// startCoreServices waits for core readiness and owns shutdown. Service errors
// fail the test, and shutdown completes before its NATS fixture is removed.
func startCoreServices(t testing.TB, c *core.ChattoCore) {
	t.Helper()
	testutil.StartService(t, "ChattoCore", c.Run, c.WaitForBoot)
}

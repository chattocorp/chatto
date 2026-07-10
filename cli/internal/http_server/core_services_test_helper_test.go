package http_server

import (
	"context"
	"testing"
	"time"

	"hmans.de/chatto/internal/core"
)

// startCoreServices runs ChattoCore's background services (PresenceHub +
// projectors) for the duration of a test. Mirrors core.startCoreServices,
// which we can't reach across the package boundary.
//
// Blocks until Run's boot phase is complete (projectors started AND
// ensureChannelRoomsAreInAGroup done), so test code can issue reads
// against the projections immediately after this returns without
// racing the background goroutines.
func startCoreServices(t testing.TB, c *core.ChattoCore) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("core.Run did not stop within timeout")
		}
	})
	bootCtx, bootCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer bootCancel()
	if err := c.WaitForBoot(bootCtx); err != nil {
		t.Fatalf("WaitForBoot: %v", err)
	}
}

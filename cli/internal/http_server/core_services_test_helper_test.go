package http_server

import (
	"context"
	"testing"

	"hmans.de/chatto/internal/core"
)

// startCoreServices runs ChattoCore's background services (PresenceHub +
// projectors) for the duration of a test. Mirrors core.startCoreServices,
// which we can't reach across the package boundary.
//
// The HTTP-layer tests need this because the dual-write JoinRoom/LeaveRoom
// paths call WaitForSeq on the membership projector, which blocks
// indefinitely without a running consumer. As new projectors come online
// (ADR-035), they're picked up automatically by core.Run with no change
// here.
func startCoreServices(t *testing.T, c *core.ChattoCore) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = c.Run(ctx) }()
	t.Cleanup(cancel)
}

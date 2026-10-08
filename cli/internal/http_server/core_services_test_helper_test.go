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
//
// It then opens the server to everyone (openServerToEveryone).
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
	openServerToEveryone(t, c)
}

// openServerToEveryone grants everyone the open-room permissions at server
// scope, as an operator who opens the whole server would. New servers start
// closed (ADR-116), but most tests exercise other behavior in open rooms.
// The core package tests the real seeded defaults.
func openServerToEveryone(t testing.TB, c *core.ChattoCore) {
	t.Helper()
	for _, perm := range core.DefaultOpenRoomEveryonePermissions() {
		if err := c.GrantServerPermission(context.Background(), core.SystemActorID, core.RoleEveryone, perm); err != nil {
			t.Fatalf("open server to everyone: grant %s: %v", perm, err)
		}
	}
}

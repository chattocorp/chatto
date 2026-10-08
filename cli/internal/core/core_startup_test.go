package core

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

// blockedBootProjection holds startup open after its sequence frontier is
// current, reproducing the gap between sequence and lifecycle readiness.
type blockedBootProjection struct {
	entered chan struct{}
	release chan struct{}
}

func (*blockedBootProjection) Subjects() []string               { return []string{"evt.startup_test.>"} }
func (*blockedBootProjection) Apply(*evtv1.Event, uint64) error { return nil }
func (p *blockedBootProjection) CompleteStartupReplay() {
	close(p.entered)
	<-p.release
}

func TestCoreBootWaitsForStartupCompletionBeforeSnapshots(t *testing.T) {
	t.Parallel()
	c, _ := newTestCore(t)
	projection := &blockedBootProjection{entered: make(chan struct{}), release: make(chan struct{})}
	projector, err := evtstream.NewProjector(c.js, c.storage.serverEvtStream, projection, events.ProjectorOptions{})
	require.NoError(t, err)
	c.projections = append(c.projections, projectionRegistration{name: "blocked startup", projector: projector})
	lease := &fakeSnapshotWorkerLease{}
	c.projectionSnapshotWorker = &projectionSnapshotWorker{lease: lease, logger: testCoreLogger(), done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(projection.release) }) }
	go func() { done <- c.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		release()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Error("core did not stop")
		}
	})
	select {
	case <-projection.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("startup hook was not called")
	}
	// Sequence catch-up succeeds while the startup hook is still blocked.
	require.NoError(t, c.WaitForProjectionsCurrent(testContext(t)))
	bootCtx, stopBootWait := context.WithTimeout(ctx, 100*time.Millisecond)
	defer stopBootWait()
	require.ErrorIs(t, c.WaitForBoot(bootCtx), context.DeadlineExceeded)
	require.Zero(t, lease.attempts.Load(), "snapshot pass must wait for startup hooks")
	release()
	require.NoError(t, c.WaitForBoot(testContext(t)))
	select {
	case <-c.projectionSnapshotWorker.done:
	case <-time.After(5 * time.Second):
		t.Fatal("snapshot pass did not start after boot")
	}
	require.Positive(t, lease.attempts.Load())
}

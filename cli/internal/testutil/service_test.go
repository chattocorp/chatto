package testutil

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
)

func TestStartServiceStopsBeforeDependencyCleanup(t *testing.T) {
	t.Parallel()
	ready := make(chan struct{})
	stopped := make(chan struct{})
	t.Cleanup(func() {
		select {
		case <-stopped:
		default:
			t.Error("dependency cleanup ran before the service stopped")
		}
	})
	StartService(t, "worker", func(ctx context.Context) error {
		close(ready)
		<-ctx.Done()
		close(stopped)
		return ctx.Err()
	}, func(ctx context.Context) error {
		select {
		case <-ready:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}

func TestServiceReportsPrematureExit(t *testing.T) {
	t.Parallel()
	failure := errors.New("worker failed")
	for _, tc := range []struct {
		name string
		err  error
	}{
		{"nil", nil},
		{"failure", failure},
		{"unsolicited cancellation", context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				service := launchService(func(context.Context) error { return tc.err })
				defer service.cancel()
				synctest.Wait()
				if service.ctx.Err() == nil {
					t.Fatal("early exit did not cancel readiness waits")
				}
				err := service.wait(t.Context())
				if err == nil || (tc.err != nil && !errors.Is(err, tc.err)) {
					t.Fatalf("early exit = %v, want failure preserving %v", err, tc.err)
				}
			})
		})
	}
}

func TestServiceShutdownWaitsForWorker(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		service := launchService(func(ctx context.Context) error {
			<-ctx.Done()
			<-release
			return ctx.Err()
		})
		service.cancel()
		stopped := make(chan error, 1)
		go func() { stopped <- service.wait(t.Context()) }()
		synctest.Wait()
		select {
		case <-stopped:
			t.Fatal("shutdown returned before the worker stopped")
		default:
		}
		close(release)
		if err := <-stopped; err != nil {
			t.Fatalf("shutdown: %v", err)
		}
	})
}

func TestServicePreservesShutdownFailure(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		failure := errors.New("flush failed")
		service := launchService(func(ctx context.Context) error {
			<-ctx.Done()
			return failure
		})
		service.cancel()
		if err := service.wait(t.Context()); !errors.Is(err, failure) {
			t.Fatalf("shutdown = %v, want %v", err, failure)
		}
	})
}

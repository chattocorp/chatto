package testutil

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// StartService starts a service, waits for readiness, and owns its shutdown.
// The test fails if the service exits before cancellation, reports an unexpected
// error, or does not stop. Cleanup waits for shutdown before earlier cleanups
// can close the service's dependencies. The returned stop function is safe to
// call more than once, including before test cleanup.
func StartService(t testing.TB, name string, run func(context.Context) error, waitReady func(context.Context) error) func() {
	t.Helper()
	service := launchService(run)
	stop := sync.OnceFunc(func() {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		service.cancel()
		if err := service.wait(ctx); err != nil {
			t.Errorf("%s shutdown: %v", name, err)
		}
	})
	t.Cleanup(stop)

	ctx, cancel := context.WithTimeout(service.ctx, 5*time.Second)
	defer cancel()
	if err := waitReady(ctx); err != nil {
		select {
		case <-service.done:
			t.Fatalf("%s startup: %v", name, service.err)
		default:
			t.Fatalf("%s readiness: %v", name, err)
		}
	}
	select {
	case <-service.done:
		t.Fatalf("%s startup: %v", name, service.err)
	default:
	}
	return stop
}

// runningService publishes err by closing done. Its context also cancels when
// run returns, so a readiness wait cannot hide an early service failure.
type runningService struct {
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
	err    error
}

func launchService(run func(context.Context) error) *runningService {
	ctx, cancel := context.WithCancel(context.Background())
	service := &runningService{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	go func() {
		err := run(ctx)
		if ctx.Err() == nil {
			if err == nil {
				err = errors.New("service returned without cancellation")
			} else {
				err = fmt.Errorf("service returned before cancellation: %w", err)
			}
		} else if errors.Is(err, context.Canceled) {
			err = nil
		}
		service.err = err
		close(service.done)
		cancel()
	}()
	return service
}

func (s *runningService) wait(ctx context.Context) error {
	select {
	case <-s.done:
		return s.err
	case <-ctx.Done():
		return ctx.Err()
	}
}

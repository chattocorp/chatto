package events

import (
	"context"
	"errors"
	"testing"
	"time"
)

type startupCompletionProjection struct {
	completions int
}

func (*startupCompletionProjection) Subjects() []string {
	return []string{"evt.test.created"}
}

func (*startupCompletionProjection) Apply(struct{}, uint64) error {
	return nil
}

func (p *startupCompletionProjection) CompleteStartupReplay() {
	p.completions++
}

type blockingStartupCompletionProjection struct {
	started chan struct{}
	release chan struct{}
}

func (*blockingStartupCompletionProjection) Subjects() []string {
	return []string{"evt.test.created"}
}

func (*blockingStartupCompletionProjection) Apply(struct{}, uint64) error {
	return nil
}

func (p *blockingStartupCompletionProjection) CompleteStartupReplay() {
	close(p.started)
	<-p.release
}

func TestProjectorCompletesStartupReplayOnceAcrossReentry(t *testing.T) {
	projection := &startupCompletionProjection{}
	projector := NewDecodedProjector(
		nil,
		nil,
		projection,
		func([]byte) (DecodedEvent[struct{}], error) {
			return DecodedEvent[struct{}]{Event: struct{}{}, ID: "test"}, nil
		},
		discardLogger{},
	)
	projector.started = true

	projector.maybeCompleteStartup(time.Now())
	projector.maybeCompleteStartup(time.Now())

	if projection.completions != 1 {
		t.Fatalf("startup replay completions = %d, want 1", projection.completions)
	}
	if err := projector.WaitForStartup(t.Context()); err != nil {
		t.Fatalf("wait for completed startup: %v", err)
	}
}

func TestProjectorWaitForStartupHonorsContext(t *testing.T) {
	projector := NewDecodedProjector(
		nil,
		nil,
		&startupCompletionProjection{},
		func([]byte) (DecodedEvent[struct{}], error) {
			return DecodedEvent[struct{}]{Event: struct{}{}, ID: "test"}, nil
		},
		discardLogger{},
	)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	if err := projector.WaitForStartup(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("wait for startup error = %v, want context cancellation", err)
	}
}

func TestProjectorWaitForStartupReturnsProjectionFailure(t *testing.T) {
	projector := NewDecodedProjector(
		nil,
		nil,
		&startupCompletionProjection{},
		func([]byte) (DecodedEvent[struct{}], error) {
			return DecodedEvent[struct{}]{Event: struct{}{}, ID: "test"}, nil
		},
		discardLogger{},
	)
	projector.fail(0, errors.New("decode failed"))

	if err := projector.WaitForStartup(t.Context()); !errors.Is(err, ErrProjectionFailed) {
		t.Fatalf("wait for startup error = %v, want ErrProjectionFailed", err)
	}
}

func TestProjectorWaitForStartupIncludesCompletionHook(t *testing.T) {
	projection := &blockingStartupCompletionProjection{
		started: make(chan struct{}),
		release: make(chan struct{}),
	}
	projector := NewDecodedProjector(
		nil,
		nil,
		projection,
		func([]byte) (DecodedEvent[struct{}], error) {
			return DecodedEvent[struct{}]{Event: struct{}{}, ID: "test"}, nil
		},
		discardLogger{},
	)
	projector.started = true
	startupFinished := make(chan struct{})
	go func() {
		projector.maybeCompleteStartup(time.Now())
		close(startupFinished)
	}()
	<-projection.started

	waitContext, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
	defer cancel()
	if err := projector.WaitForStartup(waitContext); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("wait during completion hook error = %v, want deadline exceeded", err)
	}

	close(projection.release)
	<-startupFinished
	if err := projector.WaitForStartup(t.Context()); err != nil {
		t.Fatalf("wait after completion hook: %v", err)
	}
}

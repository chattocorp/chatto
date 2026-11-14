package core

import (
	"context"
	"testing"

	"golang.org/x/sync/errgroup"

	jobqueue "github.com/chattocorp/chatto/pkg/jobs"
)

// runWithEffectJobs runs one effect source with a job pool for every effect
// job type. It lets tests drive a model without a booted core.
func runWithEffectJobs(ctx context.Context, c *ChattoCore, source func(context.Context) error) error {
	handlers := map[string]jobqueue.Handler{}
	for kind, effect := range c.effectJobs {
		handlers[kind] = effect.Handle
	}
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return source(ctx) })
	g.Go(func() error { return c.jobs.Run(ctx, c.jobsConsumer, handlers, 4) })
	return g.Wait()
}

// jobsOfType lists the current records of one job type.
func jobsOfType(t testing.TB, ctx context.Context, c *ChattoCore, kind string) []jobqueue.Job {
	t.Helper()
	all, _, err := c.jobs.List(ctx, "", 200)
	if err != nil {
		t.Fatalf("list jobs: %v", err)
	}
	var out []jobqueue.Job
	for _, job := range all {
		if job.Type == kind {
			out = append(out, job)
		}
	}
	return out
}

func TestEffectJobsCoverEveryRegisteredType(t *testing.T) {
	core, _ := newTestCore(t)
	for _, kind := range effectJobTypes() {
		effect := core.effectJobs[kind]
		if effect == nil || effect.Type() != kind {
			t.Errorf("effect job %q is not registered", kind)
		}
	}
	if len(core.effectJobs) != len(effectJobTypes()) {
		t.Errorf("registered %d effect jobs, consumer filters %d types", len(core.effectJobs), len(effectJobTypes()))
	}
}

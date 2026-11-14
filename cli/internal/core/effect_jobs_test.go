package core

import (
	"context"
	"testing"

	"golang.org/x/sync/errgroup"

	jobqueue "github.com/chattocorp/chatto/pkg/jobs"
)

// runWithEffectJobs runs one effect source with the job view and a worker pool
// for every effect job type. It lets tests drive a model without a booted core.
func runWithEffectJobs(ctx context.Context, c *ChattoCore, source func(context.Context) error) error {
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error { return source(ctx) })
	g.Go(func() error { return c.jobs.Run(ctx) })
	for kind, effect := range c.effectJobs {
		g.Go(func() error { return c.jobs.Work(ctx, c.jobConsumers[kind], effect.Handle, 4) })
	}
	return g.Wait()
}

// jobsOfType lists the jobs of one job type.
func jobsOfType(t testing.TB, ctx context.Context, c *ChattoCore, kind string) []jobqueue.Job {
	t.Helper()
	all, _, err := c.jobs.List(ctx, jobqueue.ListOptions{Limit: 200})
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
	for _, kind := range append(effectJobTypes(), jobTypeNotificationAlertDelivery) {
		effect := core.effectJobs[kind]
		if effect == nil || effect.Type() != kind {
			t.Errorf("effect job %q is not registered", kind)
		}
	}
	if len(core.effectJobs) != len(effectJobTypes())+1 {
		t.Errorf("registered %d effect jobs, want %d", len(core.effectJobs), len(effectJobTypes())+1)
	}
}

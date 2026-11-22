package video

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/chattocorp/chatto/cli/internal/natsresources"
	"github.com/chattocorp/chatto/cli/internal/runtimeunit"
	jobqueue "github.com/chattocorp/chatto/pkg/jobs"
)

const runtimeUnitName = "asset-processing"

// Unit runs durable video derivative workers either inside chatto run or as a
// standalone process. It runs media jobs from JOBS; the main server
// performs the durable EVT-to-JOBS handoff.
type Unit struct{}

func (Unit) Name() string { return runtimeUnitName }

func (Unit) Run(ctx context.Context, env runtimeunit.Env) error {
	runtime, err := core.NewAssetProcessingRuntime(ctx, env.NC, env.JS, env.Config.Core, env.Logger)
	if err != nil {
		return err
	}
	processor, err := NewService(runtime.Core(), env.Config.AssetProcessing, env.Logger)
	if err != nil {
		return err
	}

	projectionCtx, stopProjection := context.WithCancel(ctx)
	projectionDone := make(chan error, 1)
	go func() { projectionDone <- runtime.Run(projectionCtx) }()
	if err := runtime.WaitForStartup(ctx); err != nil {
		stopProjection()
		<-projectionDone
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return fmt.Errorf("start asset projection: %w", err)
	}

	// One compatibility pass closes the old message-commit -> local-schedule
	// crash gap. Current writers commit Started atomically with the message.
	runtime.Core().RecoverUnmanifestedVideoAttachments(ctx)

	queue, consumer, err := openJobs(ctx, env)
	if err != nil {
		stopProjection()
		<-projectionDone
		return err
	}
	// Standalone readiness probes match this message after projection replay and
	// the connection to the media job consumer have succeeded.
	env.Logger.Info("Asset-processing worker started",
		"consumer", consumer.CachedInfo().Name,
		"max_concurrent_jobs", env.Config.AssetProcessing.MaxConcurrentJobsOrDefault())

	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan error, 1)
	go func() {
		process := func(ctx context.Context, assetID, messageID string) error {
			return runtime.Core().ProcessMediaJob(ctx, assetID, messageID, processor.ProcessAsset)
		}
		workerDone <- queue.Work(workerCtx, consumer, core.MediaJobHandler(env.Logger, process), env.Config.AssetProcessing.MaxConcurrentJobsOrDefault())
	}()

	var workerErr, projectionErr error
	select {
	case workerErr = <-workerDone:
		stopProjection()
		projectionErr = <-projectionDone
	case projectionErr = <-projectionDone:
		stopWorker()
		workerErr = <-workerDone
	case <-ctx.Done():
		stopWorker()
		stopProjection()
		workerErr = <-workerDone
		projectionErr = <-projectionDone
	}
	stopWorker()
	stopProjection()
	if workerErr != nil && !errors.Is(workerErr, context.Canceled) {
		return workerErr
	}
	if projectionErr != nil && !errors.Is(projectionErr, context.Canceled) {
		return fmt.Errorf("asset projection: %w", projectionErr)
	}
	env.Logger.Info("Asset-processing worker stopped")
	return nil
}

var _ runtimeunit.Unit = Unit{}

// openJobs binds the application-owned job stream and the media consumer. A
// standalone unit runs only media jobs.
func openJobs(ctx context.Context, env runtimeunit.Env) (*jobqueue.Queue, jetstream.Consumer, error) {
	stream, err := env.JS.Stream(ctx, natsresources.Jobs)
	if err != nil {
		return nil, nil, err
	}
	queue, err := core.NewJobQueue(env.JS, stream, env.Config.Core.JobFailedRetention.Duration(), env.Logger, nil)
	if err != nil {
		return nil, nil, err
	}
	consumer, err := stream.CreateOrUpdateConsumer(ctx, core.JobsConsumerConfig(queue, core.JobTypeMedia))
	if err != nil {
		return nil, nil, err
	}
	return queue, consumer, nil
}

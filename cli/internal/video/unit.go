package video

import (
	"context"
	"errors"
	"fmt"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"github.com/chattocorp/chatto/cli/internal/core"
	"github.com/chattocorp/chatto/cli/internal/jobqueue"
	"github.com/chattocorp/chatto/cli/internal/natsresources"
	statev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/runtime_state/v1"
	"github.com/chattocorp/chatto/cli/internal/runtimeunit"
	"github.com/chattocorp/chatto/pkg/jetstreamutil"
)

const runtimeUnitName = "asset-processing"

// Unit runs durable video derivative workers either inside chatto run or as a
// standalone process. It consumes media dispatches from JOBS; the main server
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
	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan error, 1)
	go func() {
		workerDone <- queue.Run(workerCtx, consumer, map[string]jobqueue.Handler{"media": func(ctx context.Context, job *statev1.JobRecord) error {
			request := new(statev1.MediaJob)
			if err := proto.Unmarshal(job.Payload, request); err != nil || request.AssetId == "" || request.MessageEventId == "" {
				return &jobqueue.Failure{Category: "invalid_payload", Permanent: true}
			}
			return runtime.Core().ProcessMediaJob(ctx, request.AssetId, request.MessageEventId, processor.ProcessAsset)
		}}, env.Config.AssetProcessing.MaxConcurrentJobsOrDefault())
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

// openJobs binds application-owned resources; a standalone unit runs only media
// handlers. Revision claims prevent concurrent effects with the shared pool.
func openJobs(ctx context.Context, env runtimeunit.Env) (*jobqueue.Queue, jetstream.Consumer, error) {
	stream, err := env.JS.Stream(ctx, natsresources.Jobs)
	if err != nil {
		return nil, nil, err
	}
	kv, err := env.JS.KeyValue(ctx, natsresources.RuntimeState)
	if err != nil {
		return nil, nil, err
	}
	bound, err := jetstreamutil.NewKeyValue(env.JS, kv)
	if err != nil {
		return nil, nil, err
	}
	consumer, err := stream.CreateOrUpdateConsumer(ctx, jobqueue.ConsumerConfig(jobqueue.MediaConsumerName, []string{"media"}))
	if err != nil {
		return nil, nil, err
	}
	return jobqueue.New(env.JS, bound, env.Logger), consumer, nil
}

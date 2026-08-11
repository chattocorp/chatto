package video

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/runtimeunit"
	"hmans.de/chatto/pkg/events"
)

const (
	runtimeUnitName    = "asset-processing"
	consumerName       = "chatto-asset-processing-v1"
	consumerAckWait    = 2 * time.Minute
	deliveryHeartbeat  = 30 * time.Second
	retryDelay         = 30 * time.Second
	acknowledgeTimeout = 5 * time.Second
	consumerMaxPending = 64
)

// Unit runs durable video derivative workers either inside chatto run or as a
// standalone process. All replicas share one pull consumer on EVT.
type Unit struct{}

type processingRuntime interface {
	WaitForEvent(context.Context, string, uint64) error
	AssetState(string) core.AssetState
}

type assetProcessor interface {
	ProcessAsset(context.Context, string, string) error
}

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

	consumer, err := createConsumer(ctx, env.JS)
	if err != nil {
		stopProjection()
		<-projectionDone
		return err
	}
	env.Logger.Info("Asset-processing worker started",
		"consumer", consumerName,
		"max_concurrent_jobs", env.Config.AssetProcessing.MaxConcurrentJobsOrDefault())

	workerCtx, stopWorker := context.WithCancel(ctx)
	workerDone := make(chan error, 1)
	worker, err := events.NewDurableWorker(
		consumer,
		func(ctx context.Context, delivery events.DurableDelivery) error {
			return processDelivery(ctx, delivery, runtime, processor, env.Logger)
		},
		events.DurableWorkerOptions{
			MaxConcurrent:     env.Config.AssetProcessing.MaxConcurrentJobsOrDefault(),
			FetchMaxWait:      time.Second,
			RetryDelay:        retryDelay,
			AckTimeout:        acknowledgeTimeout,
			HeartbeatInterval: deliveryHeartbeat,
			Logger:            env.Logger,
		},
	)
	if err != nil {
		stopWorker()
		stopProjection()
		<-projectionDone
		return fmt.Errorf("configure asset-processing worker: %w", err)
	}
	go func() {
		workerDone <- worker.Run(workerCtx)
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

func createConsumer(ctx context.Context, js jetstream.JetStream) (jetstream.Consumer, error) {
	evt, err := js.Stream(ctx, "EVT")
	if err != nil {
		return nil, fmt.Errorf("open EVT stream: %w", err)
	}
	consumer, err := evt.CreateOrUpdateConsumer(ctx, jetstream.ConsumerConfig{
		Name:          consumerName,
		Durable:       consumerName,
		Description:   "Shared durable queue for Chatto asset-processing workers",
		DeliverPolicy: jetstream.DeliverAllPolicy,
		AckPolicy:     jetstream.AckExplicitPolicy,
		AckWait:       consumerAckWait,
		MaxDeliver:    -1,
		FilterSubjects: []string{
			evtstream.AssetEventTypeFilter(evtstream.EventAssetProcessingStarted),
			evtstream.RoomEventTypeFilter(evtstream.EventAssetProcessingStarted),
		},
		ReplayPolicy:    jetstream.ReplayInstantPolicy,
		MaxAckPending:   consumerMaxPending,
		MaxRequestBatch: consumerMaxPending,
	})
	if err != nil {
		return nil, fmt.Errorf("create asset-processing consumer: %w", err)
	}
	return consumer, nil
}

func processDelivery(
	ctx context.Context,
	delivery events.DurableDelivery,
	runtime processingRuntime,
	processor assetProcessor,
	logger *log.Logger,
) error {
	var event corev1.Event
	if err := proto.Unmarshal(delivery.Data, &event); err != nil {
		logger.Error("Terminating malformed asset-processing delivery", "error", err)
		return events.TerminateDelivery("invalid Chatto event envelope", err)
	}
	started := event.GetAssetProcessingStarted()
	if started == nil || started.GetAssetId() == "" || started.GetMessageEventId() == "" {
		logger.Error("Terminating invalid asset-processing request", "event_id", event.GetId())
		return events.TerminateDelivery("invalid asset-processing request", errors.New("missing asset-processing request fields"))
	}
	if err := runtime.WaitForEvent(ctx, delivery.Subject, delivery.StreamSequence); err != nil {
		if ctx.Err() == nil {
			logger.Warn("Asset projection did not reach queue delivery", "asset_id", started.GetAssetId(), "error", err)
		}
		return err
	}
	if assetProcessingTerminal(runtime.AssetState(started.GetAssetId())) {
		return nil
	}

	err := processor.ProcessAsset(ctx, started.GetAssetId(), started.GetMessageEventId())

	if assetProcessingTerminal(runtime.AssetState(started.GetAssetId())) {
		return nil
	}
	if err != nil && ctx.Err() == nil {
		logger.Warn("Asset processing remains retryable", "asset_id", started.GetAssetId(), "error", err)
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("asset processing did not reach terminal state")
}

func assetProcessingTerminal(state core.AssetState) bool {
	if state.Deleted {
		return true
	}
	manifest := state.VideoManifest
	return manifest != nil && (manifest.Succeeded != nil || manifest.Failed != nil)
}

var _ runtimeunit.Unit = Unit{}

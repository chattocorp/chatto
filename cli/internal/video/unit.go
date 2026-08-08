package video

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/runtimeunit"
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
	go func() {
		workerDone <- runConsumer(workerCtx, consumer, runtime, processor, env.Config.AssetProcessing.MaxConcurrentJobsOrDefault(), env.Logger)
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

func runConsumer(
	ctx context.Context,
	consumer jetstream.Consumer,
	runtime processingRuntime,
	processor assetProcessor,
	maxConcurrent int,
	logger *log.Logger,
) error {
	for ctx.Err() == nil {
		batch, err := consumer.Fetch(maxConcurrent, jetstream.FetchMaxWait(time.Second))
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("fetch asset-processing work: %w", err)
		}
		var wg sync.WaitGroup
		for msg := range batch.Messages() {
			wg.Add(1)
			go func() {
				defer wg.Done()
				processDelivery(ctx, msg, runtime, processor, logger)
			}()
		}
		wg.Wait()
		if err := batch.Error(); err != nil && ctx.Err() == nil {
			return fmt.Errorf("receive asset-processing work: %w", err)
		}
	}
	return nil
}

func processDelivery(
	ctx context.Context,
	msg jetstream.Msg,
	runtime processingRuntime,
	processor assetProcessor,
	logger *log.Logger,
) {
	metadata, err := msg.Metadata()
	if err != nil {
		logger.Error("Asset-processing delivery metadata unavailable", "error", err)
		_ = msg.NakWithDelay(retryDelay)
		return
	}
	var event corev1.Event
	if err := proto.Unmarshal(msg.Data(), &event); err != nil {
		logger.Error("Terminating malformed asset-processing delivery", "error", err)
		_ = msg.TermWithReason("invalid Chatto event envelope")
		return
	}
	started := event.GetAssetProcessingStarted()
	if started == nil || started.GetAssetId() == "" || started.GetMessageEventId() == "" {
		logger.Error("Terminating invalid asset-processing request", "event_id", event.GetId())
		_ = msg.TermWithReason("invalid asset-processing request")
		return
	}
	if err := runtime.WaitForEvent(ctx, msg.Subject(), metadata.Sequence.Stream); err != nil {
		if ctx.Err() == nil {
			logger.Warn("Asset projection did not reach queue delivery", "asset_id", started.GetAssetId(), "error", err)
		}
		_ = msg.NakWithDelay(retryDelay)
		return
	}
	if assetProcessingTerminal(runtime.AssetState(started.GetAssetId())) {
		ackDelivery(ctx, msg, logger)
		return
	}

	heartbeatDone := make(chan struct{})
	go func() {
		ticker := time.NewTicker(deliveryHeartbeat)
		defer ticker.Stop()
		for {
			select {
			case <-heartbeatDone:
				return
			case <-ticker.C:
				if err := msg.InProgress(); err != nil {
					logger.Warn("Asset-processing delivery heartbeat failed", "asset_id", started.GetAssetId(), "error", err)
				}
			}
		}
	}()
	err = processor.ProcessAsset(ctx, started.GetAssetId(), started.GetMessageEventId())
	close(heartbeatDone)

	if assetProcessingTerminal(runtime.AssetState(started.GetAssetId())) {
		ackDelivery(ctx, msg, logger)
		return
	}
	if err != nil && ctx.Err() == nil {
		logger.Warn("Asset processing remains retryable", "asset_id", started.GetAssetId(), "error", err)
	}
	if ctx.Err() != nil {
		_ = msg.Nak()
	} else {
		_ = msg.NakWithDelay(retryDelay)
	}
}

func assetProcessingTerminal(state core.AssetState) bool {
	if state.Deleted {
		return true
	}
	manifest := state.VideoManifest
	return manifest != nil && (manifest.Succeeded != nil || manifest.Failed != nil)
}

func ackDelivery(ctx context.Context, msg jetstream.Msg, logger *log.Logger) {
	ackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), acknowledgeTimeout)
	defer cancel()
	if err := msg.DoubleAck(ackCtx); err != nil {
		logger.Warn("Asset-processing acknowledgement was not confirmed", "error", err)
	}
}

var _ runtimeunit.Unit = Unit{}

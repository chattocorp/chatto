package core

import (
	"context"
	"fmt"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/pkg/events"
)

// AssetProcessingRuntime is the deliberately small core boundary needed by
// asset-processing workers. It opens existing asset/EVT resources, owns one
// AssetProjection, and exposes media and asset lifecycle operations without
// starting ChattoCore or its boot-time mutations.
type AssetProcessingRuntime struct {
	core   *ChattoCore
	assets events.ProjectionHandle[*AssetProjection]
}

// NewAssetProcessingRuntime opens the resources used by a worker. The main app
// owns resource creation; standalone workers therefore expect EVT and
// SERVER_ASSETS to exist already.
func NewAssetProcessingRuntime(
	ctx context.Context,
	nc *nats.Conn,
	js jetstream.JetStream,
	cfg config.CoreConfig,
	logger *log.Logger,
) (*AssetProcessingRuntime, error) {
	if nc == nil {
		return nil, fmt.Errorf("asset processing runtime requires a NATS connection")
	}
	if js == nil {
		return nil, fmt.Errorf("asset processing runtime requires JetStream")
	}
	if logger == nil {
		logger = log.WithPrefix("core.AssetProcessingRuntime")
	}

	evt, err := js.Stream(ctx, "EVT")
	if err != nil {
		return nil, fmt.Errorf("open EVT stream: %w", err)
	}
	serverAssets, err := js.ObjectStore(ctx, "SERVER_ASSETS")
	if err != nil {
		return nil, fmt.Errorf("open SERVER_ASSETS object store: %w", err)
	}
	s3Client, err := initializeCoreS3(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}
	publisher := evtstream.NewPublisher(js, evt, logger)
	projection := NewAssetProjection()
	assets := evtstream.NewProjectionHandle(js, evt, projection, logger.WithPrefix("AssetsProjector"))

	workerCore := &ChattoCore{
		nc:             nc,
		js:             js,
		logger:         logger,
		storage:        &storage{serverAssets: serverAssets, serverEvtStream: evt},
		config:         cfg,
		s3Client:       s3Client,
		EventPublisher: publisher,
	}
	workerCore.mediaModel = NewMediaModel(workerCore)
	workerCore.assetModel = NewAssetModel(workerCore, assets)

	return &AssetProcessingRuntime{core: workerCore, assets: assets}, nil
}

// Core returns the narrow ChattoCore facade used by the existing media
// processor. Only asset/media methods are initialized on this instance.
func (r *AssetProcessingRuntime) Core() *ChattoCore { return r.core }

// Run maintains the worker's private AssetProjection until shutdown.
func (r *AssetProcessingRuntime) Run(ctx context.Context) error {
	return r.assets.Projector().Run(ctx)
}

// WaitForStartup waits until historical EVT replay has completed.
func (r *AssetProcessingRuntime) WaitForStartup(ctx context.Context) error {
	return r.assets.Projector().WaitForStartup(ctx)
}

// WaitForEvent waits until the asset projection has observed a queue delivery.
func (r *AssetProcessingRuntime) WaitForEvent(ctx context.Context, subject string, seq uint64) error {
	return r.assets.Projector().WaitFor(ctx, events.SubjectPosition(subject, seq))
}

// AssetState returns the worker's projected lifecycle view.
func (r *AssetProcessingRuntime) AssetState(assetID string) AssetState {
	return r.core.GetAssetState(assetID)
}

package core

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/dekstore"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/kms"
	"hmans.de/chatto/internal/projectionsnapshot"
)

// coreInfrastructure contains the storage and event-sourcing primitives that
// must exist before projections and domain services can be constructed.
type coreInfrastructure struct {
	js                     jetstream.JetStream
	storage                *storage
	encryption             *encryptionManager
	dekResolver            *unwrappedDEKResolver
	s3Client               *S3Client
	eventPublisher         *events.Publisher
	snapshotRepository     *projectionsnapshot.Repository
	snapshotStreamIdentity string
}

func initializeCoreInfrastructure(
	ctx context.Context,
	nc *nats.Conn,
	cfg config.CoreConfig,
	logger *log.Logger,
) (*coreInfrastructure, error) {
	js, err := jetstream.New(nc, jetstream.WithDefaultTimeout(30*time.Second))
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	storage, err := newStorage(js, ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	builtinKMS := kms.NewBuiltin(storage.encryptionKV, logger.WithPrefix("core.kms"))
	encryption := &encryptionManager{
		keyWrapper:  builtinKMS,
		legacyKeys:  builtinKMS,
		callKeys:    builtinKMS,
		contentKeys: dekstore.New(storage.runtimeStateKV, logger.WithPrefix("core.dekstore")),
	}

	s3Client, err := initializeCoreS3(ctx, cfg, logger)
	if err != nil {
		return nil, err
	}

	snapshotRepository, snapshotStreamIdentity := initializeProjectionSnapshotRepository(
		ctx,
		js,
		storage,
		s3Client,
		cfg,
		logger,
	)
	dekResolver := newUnwrappedDEKResolver(encryption.keyWrapper, encryption.contentKeys)

	return &coreInfrastructure{
		js:                     js,
		storage:                storage,
		encryption:             encryption,
		dekResolver:            dekResolver,
		s3Client:               s3Client,
		eventPublisher:         events.NewPublisher(js, storage.serverEvtStream, logger),
		snapshotRepository:     snapshotRepository,
		snapshotStreamIdentity: snapshotStreamIdentity,
	}, nil
}

func initializeCoreS3(
	ctx context.Context,
	cfg config.CoreConfig,
	logger *log.Logger,
) (*S3Client, error) {
	if cfg.Assets.StorageBackend != config.StorageBackendS3 {
		return nil, nil
	}

	s3Client, err := NewS3Client(cfg.Assets.S3)
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 client: %w", err)
	}
	if s3Client == nil {
		return nil, nil
	}
	if err := s3Client.EnsureBucket(ctx); err != nil {
		return nil, fmt.Errorf("failed to ensure S3 bucket: %w", err)
	}
	logger.Info("S3 storage initialized", "bucket", s3Client.Bucket())
	return s3Client, nil
}

// initializeProjectionSnapshotRepository keeps snapshot storage best-effort:
// failures disable snapshots and leave normal cold replay available.
func initializeProjectionSnapshotRepository(
	ctx context.Context,
	js jetstream.JetStream,
	storage *storage,
	s3Client *S3Client,
	cfg config.CoreConfig,
	logger *log.Logger,
) (*projectionsnapshot.Repository, string) {
	if !cfg.ProjectionSnapshots {
		return nil, ""
	}

	var snapshotBlobs projectionsnapshot.BlobStore
	if cfg.Assets.StorageBackend == config.StorageBackendS3 && s3Client != nil {
		snapshotBlobs = s3SnapshotBlobStore{client: s3Client}
	} else {
		snapshotStore, err := createJetStreamResourceWithRetry(ctx, func(ctx context.Context) (jetstream.ObjectStore, error) {
			return js.CreateOrUpdateObjectStore(ctx, jetstream.ObjectStoreConfig{
				Bucket:      projectionSnapshotObjectStoreName,
				Description: "Encrypted ephemeral projection snapshots",
				Storage:     jetstream.FileStorage,
				Compression: true,
				Replicas:    cfg.Replicas,
				TTL:         cfg.ProjectionSnapshotRetentionOrDefault(),
			})
		})
		if err != nil {
			logger.Warn("Projection snapshots disabled after object store initialization failure",
				"backend", "nats",
				"stage", "storage_initialize",
				"error", err)
			return nil, ""
		}
		snapshotBlobs = natsSnapshotBlobStore{store: snapshotStore}
	}

	repository, err := projectionsnapshot.NewRepository(snapshotBlobs, projectionsnapshot.RepositoryOptions{
		Pointers:        natsSnapshotPointerStore{kv: storage.runtimeStateKV},
		SecretHex:       cfg.SecretKey,
		ProducerVersion: cfg.Version,
		Logger:          logger.WithPrefix("core.ProjectionSnapshots"),
	})
	if err != nil {
		logger.Warn("Projection snapshots disabled after initialization failure",
			"stage", "initialize",
			"error", err)
		return nil, ""
	}

	streamIdentity, err := events.StreamIdentity(storage.serverEvtStream)
	if err != nil {
		logger.Warn("Projection snapshots disabled after EVT identity read failure",
			"stage", "stream_identity",
			"error", err)
		return nil, ""
	}

	logger.Info("Projection snapshot storage initialized", "backend", repository.Backend())
	return repository, streamIdentity
}

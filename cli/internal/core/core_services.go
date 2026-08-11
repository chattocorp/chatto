package core

import (
	"context"
	"fmt"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core/linkpreview"
	"hmans.de/chatto/internal/lease"
)

// assembleCore installs already-constructed infrastructure, projections, and
// projection-aware models into the application facade without starting work.
func assembleCore(
	nc *nats.Conn,
	cfg config.CoreConfig,
	infra *coreInfrastructure,
	projections *coreProjections,
	logger *log.Logger,
) *ChattoCore {
	configModel := NewConfigModel(
		infra.eventPublisher,
		projections.serverConfig,
	)
	roomModel := newRoomModel(
		projections.roomDirectory,
		projections.roomGroupLayout,
		projections.roomTimeline,
		projections.threads,
		projections.reactions,
	)
	userModel := newUserModel(
		infra.eventPublisher,
		projections.users,
		projections.userAuth,
		projections.contentKeys,
	)

	return &ChattoCore{
		nc:               nc,
		js:               infra.js,
		logger:           logger,
		storage:          infra.storage,
		config:           cfg,
		encryption:       infra.encryption,
		dekResolver:      infra.dekResolver,
		configModel:      configModel,
		roomModel:        roomModel,
		userModel:        userModel,
		rbacModel:        newRBACModel(projections.rbac),
		mentionables:     newMentionablesModel(projections.mentionables),
		invitationModel:  newInvitationModel(infra.eventPublisher, projections.invitations, cfg.SecretKey),
		oauthClientModel: newOAuthClientModel(projections.oauthClients),
		s3Client:         infra.s3Client,
		EventPublisher:   infra.eventPublisher,
		projections:      projections.registrations,
		bootDone:         make(chan struct{}),
	}
}

// initializeCoreServices attaches models and workers that require the complete
// ChattoCore facade. It preserves the final construction phase before Run.
func initializeCoreServices(
	ctx context.Context,
	core *ChattoCore,
	infra *coreInfrastructure,
	projections *coreProjections,
	cfg config.CoreConfig,
	logger *log.Logger,
) error {
	callReconcileLease, err := lease.New(infra.js, infra.storage.memoryCacheKV, lease.Options{
		Name:       callReconcileLeaseName,
		Bucket:     "MEMORY_CACHE",
		TTL:        callReconcileLeaseTTL,
		RenewEvery: callReconcileLeaseRenewEvery,
		RetryEvery: callReconcileLeaseRetryEvery,
		Logger:     logger.WithPrefix("core.CallReconcilerLease"),
	})
	if err != nil {
		return fmt.Errorf("failed to initialize call reconciler lease: %w", err)
	}
	initializeProjectionSnapshotWorker(core, infra, projections, cfg, logger)

	core.mediaModel = NewMediaModel(core)
	core.callModel = NewCallModel(
		infra.eventPublisher,
		projections.callState,
		infra.encryption.callKeys,
		nil,
		callReconcileLease,
		infra.storage.memoryCacheKV,
		logger.WithPrefix("core.CallModel"),
	)
	if err := core.callModel.configureKeyCleanup(ctx, infra.storage.serverEvtStream); err != nil {
		return fmt.Errorf("failed to initialize call-key cleanup: %w", err)
	}
	core.assetModel = NewAssetModel(core, projections.assets)
	if err := core.assetModel.configureCleanup(ctx, infra.storage.serverEvtStream); err != nil {
		return fmt.Errorf("failed to initialize asset cleanup: %w", err)
	}
	core.assetUploadModel = &AssetUploadModel{core: core}
	core.roomCommands = &RoomCommandModel{core: core}
	core.roomDirectoryReads = &RoomDirectoryReadModel{core: core}
	core.messageModel = &MessageModel{core: core}
	core.messageSearchReads = &MessageSearchReadModel{core: core}
	core.notificationPrefs = &NotificationPreferencesModel{core: core}
	core.roomTimelineReads = &RoomTimelineReadModel{
		core:  core,
		rooms: core.roomModel,
	}
	core.readStateModel = &ReadStateModel{
		core:  core,
		index: NewReadStateIndex(infra.storage.runtimeStateKV, logger.WithPrefix("core.ReadStateIndex")),
	}
	core.threadFollows = &ThreadFollowModel{core: core}
	core.reactionModel = &ReactionModel{core: core, mutations: core.EventPublisher}
	core.keyShredding, err = newUserKeyShreddingModel(ctx, core, logger.WithPrefix("core.UserKeyShredding"))
	if err != nil {
		return fmt.Errorf("failed to initialize user-key shredding: %w", err)
	}

	if err := core.seedDefaultRBAC(ctx); err != nil {
		return fmt.Errorf("failed to seed default RBAC: %w", err)
	}

	core.permissionResolver = NewPermissionResolver(core)
	core.linkPreviewCache = linkpreview.NewCache(infra.storage.runtimeStateKV)
	assetsConfig := core.AssetsConfig()
	core.linkPreviewFetcher = linkpreview.NewFetcher(&assetsConfig, NewAssetID, core.storeLinkPreviewImage)

	// Presence owns one KV watcher per process and starts from core.Run with the
	// registered projectors and other long-running models.
	core.presenceModel = NewPresenceModel(infra.js, infra.storage.memoryCacheKV, logger)
	core.PresenceHub = core.presenceModel.hub
	core.myEventsModel = NewMyEventsModel(core)
	return nil
}

func initializeProjectionSnapshotWorker(
	core *ChattoCore,
	infra *coreInfrastructure,
	projections *coreProjections,
	cfg config.CoreConfig,
	logger *log.Logger,
) {
	if len(projections.snapshotJobs) == 0 {
		return
	}

	snapshotLease, err := lease.New(infra.js, infra.storage.memoryCacheKV, lease.Options{
		Name:   projectionSnapshotLeaseName,
		Bucket: "MEMORY_CACHE",
		Logger: logger.WithPrefix("core.ProjectionSnapshotLease"),
	})
	if err != nil {
		logger.Warn("Projection snapshot writer disabled after lease initialization failure",
			"projection", projections.snapshotJobs[0].projectionKey,
			"stage", "lease_initialize",
			"error", err)
		return
	}

	core.projectionSnapshotWorker = &projectionSnapshotWorker{
		jobs:      projections.snapshotJobs,
		lease:     snapshotLease,
		logger:    logger.WithPrefix("core.ProjectionSnapshotWorker"),
		done:      make(chan struct{}),
		retention: cfg.ProjectionSnapshotRetentionOrDefault(),
	}
	if infra.snapshotRepository.Backend() != "s3" ||
		!cfg.ProjectionSnapshotS3CleanupOrDefault() {
		return
	}

	expiryLease, err := lease.New(infra.js, infra.storage.memoryCacheKV, lease.Options{
		Name:   projectionSnapshotExpiryLeaseName,
		Bucket: "MEMORY_CACHE",
		TTL:    projectionSnapshotExpiryInterval,
		Logger: logger.WithPrefix("core.ProjectionSnapshotExpiryLease"),
	})
	if err != nil {
		logger.Warn("Projection snapshot S3 expiry disabled after cooldown initialization failure",
			"backend", infra.snapshotRepository.Backend(),
			"stage", "expiry_initialize",
			"error", err)
		return
	}
	core.projectionSnapshotWorker.expirer = infra.snapshotRepository
	core.projectionSnapshotWorker.expiryLease = expiryLease
}

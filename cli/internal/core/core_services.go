package core

import (
	"context"
	"fmt"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"

	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/core/linkpreview"
	"github.com/chattocorp/chatto/cli/internal/lease"
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
	serverOrigins := make(map[string]struct{}, len(cfg.ServerOrigins))
	for _, origin := range cfg.ServerOrigins {
		serverOrigins[origin] = struct{}{}
	}
	cfg.ServerOrigins = append([]string(nil), cfg.ServerOrigins...)
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
		contentView:      projections.contentView,
		eventReader:      infra.eventReader,
		configModel:      configModel,
		roomModel:        roomModel,
		timelineHydrator: newRoomTimelineHydrator(infra.eventReader),
		userModel:        userModel,
		rbacModel:        newRBACModel(projections.rbac),
		mentionables:     newMentionablesModel(projections.mentionables),
		invitationModel:  newInvitationModel(projections.invitations, cfg.SecretKey),
		oauthClientModel: newOAuthClientModel(projections.oauthClients),
		s3Client:         infra.s3Client,
		serverOrigins:    serverOrigins,
		EventPublisher:   infra.eventPublisher,
		projections:      projections.registrations,
		bootDone:         make(chan struct{}),
	}
}

// initializeCoreServices wires operation models and workers before Run. Room
// models receive their domain owners and shared authorization model. Message
// hydration and transient delivery retain explicit integration capabilities.
func initializeCoreServices(
	ctx context.Context,
	core *ChattoCore,
	infra *coreInfrastructure,
	projections *coreProjections,
	cfg config.CoreConfig,
	logger *log.Logger,
) error {
	callReconcileLease, err := lease.New(infra.storage.memoryCacheKV, lease.Options{
		Name:       callReconcileLeaseName,
		TTL:        callReconcileLeaseTTL,
		RenewEvery: callReconcileLeaseRenewEvery,
		RetryEvery: callReconcileLeaseRetryEvery,
		Logger:     logger.WithPrefix("core.CallReconcilerLease"),
	})
	if err != nil {
		return fmt.Errorf("failed to initialize call reconciler lease: %w", err)
	}
	initializeProjectionSnapshotWorker(core, infra, projections, cfg, logger)
	initializeNeighborhoodDiscovery(core, infra, cfg, logger)

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
	core.credentialUsage = newCredentialUsageRecorder(
		infra.storage.runtimeStateKV,
		logger.WithPrefix("core.CredentialUsage"),
		core.credentialUsageIsActive,
	)
	// The resolver is constructed before room models capture it. It performs no
	// reads until the fully assembled core starts serving operations.
	core.permissionResolver = NewPermissionResolver(core.roomModel, core.userModel, core.rbacModel, core.contentView)
	core.authorization = newAuthorizationModel(
		core.roomModel, core.userModel, core.rbacModel, core.permissionResolver,
		core.EventPublisher, core.contentView, core,
	)
	// The owner models below take the notification models as constructor
	// arguments, so they are constructed first. The occurrence model and the
	// materializer only capture core here. Alert delivery reads the
	// materializer, so it must follow it.
	core.notificationBoundaries = newNotificationBoundaryIndex(
		infra.storage.runtimeStateKV,
		logger.WithPrefix("core.NotificationBoundaryIndex"),
	)
	core.notificationOccurrences = NewNotificationOccurrenceModel(
		core,
		projections.notifications,
		infra.notificationPublisher,
		infra.storage.notificationStream,
		infra.storage.runtimeStateKV,
		logger.WithPrefix("core.NotificationOccurrences"),
	)
	core.notificationMaterializer = NewNotificationMaterializer(core, projections.notificationDecisions)
	core.notificationAlertDelivery = newNotificationAlertDelivery(core)
	// Owner models receive every collaborator through their constructors, in
	// dependency order. None of them is assigned a dependency later.
	core.readStateModel = newReadStateModel(
		NewReadStateIndex(infra.storage.runtimeStateKV, logger.WithPrefix("core.ReadStateIndex")),
		core.roomModel, core.authorization, core, core.notificationOccurrences, core.logger,
	)
	core.roomMemberships = newRoomMembershipModel(
		core.roomModel, core.userModel, core.EventPublisher, core.authorization,
		core.readStateModel, core.callModel, core.notificationMaterializer, core.logger,
	)
	core.roomLayout = newRoomLayoutModel(core.roomModel, core.EventPublisher, core.authorization, core.logger)
	core.roomCommands = newRoomCommandModel(
		core.roomModel, core.permissionResolver, infra.eventPublisher, core.userModel,
		core.rbacModel, core.readStateModel, core.authorization, core.roomMemberships,
		core, core.logger,
	)
	core.messageModel = &MessageModel{core: core}
	core.roomInvitations = &RoomInvitationModel{core: core, projection: projections.roomInvitations}
	core.roomDirectoryReads = newRoomDirectoryReadModel(
		core.roomModel, core.permissionResolver, core.messageModel,
		core.authorization, core.roomMemberships, core.readStateModel, core.roomInvitations,
	)
	core.notificationPolicy = &NotificationPolicyModel{core: core}
	core.roomTimelineReads = newRoomTimelineReadModel(core, core.roomModel)
	core.threadFollows = newThreadFollowModel(
		core.roomModel, core.readStateModel, core.EventPublisher, core.authorization,
		core, core.notificationMaterializer, core.logger,
	)
	core.messageSearchReads = newMessageSearchReadModel(
		core.roomModel, core.userModel, core.roomMemberships, core.authorization,
		core.roomTimelineReads, core.threadFollows,
	)
	core.botWebhooks = newBotWebhookModel(core, projections.botWebhooks)
	if err := core.botWebhooks.initialize(ctx); err != nil {
		return fmt.Errorf("initialize bot webhooks: %w", err)
	}
	pushCleanupLease, err := lease.New(infra.storage.memoryCacheKV, lease.Options{
		Name:   pushSubscriptionReconcileLeaseName,
		TTL:    pushSubscriptionReconcileLeaseTTL,
		Logger: logger.WithPrefix("core.PushSubscriptionCleanupLease"),
	})
	if err != nil {
		return fmt.Errorf("failed to initialize push-subscription cleanup lease: %w", err)
	}
	core.pushSubscriptionCleanup, err = newPushSubscriptionCleanupModel(
		ctx,
		core,
		pushCleanupLease,
		logger.WithPrefix("core.PushSubscriptionCleanup"),
	)
	if err != nil {
		return fmt.Errorf("failed to initialize push-subscription cleanup: %w", err)
	}
	core.reactionModel = &ReactionModel{core: core, mutations: core.EventPublisher}
	core.keyShredding, err = newUserKeyShreddingModel(ctx, core, logger.WithPrefix("core.UserKeyShredding"))
	if err != nil {
		return fmt.Errorf("failed to initialize user-key shredding: %w", err)
	}

	if err := core.seedDefaultRBAC(ctx); err != nil {
		return fmt.Errorf("failed to seed default RBAC: %w", err)
	}
	if err := core.seedUpgradePermissions(ctx); err != nil {
		return fmt.Errorf("seed upgrade permissions: %w", err)
	}
	if err := core.splitInvitationPermissions(ctx); err != nil {
		return fmt.Errorf("split invitation permissions: %w", err)
	}
	if err := core.notificationMaterializer.Initialize(ctx); err != nil {
		return fmt.Errorf("failed to initialize notification materializer: %w", err)
	}
	if err := core.notificationAlertDelivery.initialize(ctx); err != nil {
		return fmt.Errorf("failed to initialize notification alert delivery: %w", err)
	}

	core.linkPreviewCache = linkpreview.NewCache(infra.storage.runtimeStateKV)
	assetsConfig := core.AssetsConfig()
	core.linkPreviewFetcher = linkpreview.NewFetcher(&assetsConfig, NewAssetID, core.storeLinkPreviewImage)

	// Presence owns two KV watchers per process and starts from core.Run with the
	// registered projectors and other long-running models.
	core.presenceModel = NewPresenceModel(infra.storage.memoryCacheKV, infra.storage.runtimeStateKV, logger)
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

	snapshotLease, err := lease.New(infra.storage.memoryCacheKV, lease.Options{
		Name:   projectionSnapshotLeaseName,
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

	expiryLease, err := lease.New(infra.storage.memoryCacheKV, lease.Options{
		Name:   projectionSnapshotExpiryLeaseName,
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

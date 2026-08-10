package core

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sync/errgroup"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core/linkpreview"
	"hmans.de/chatto/internal/evtstream"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// ChattoCore
// ============================================================================

// ChattoCore is the central hub for all Chatto operations.
// It provides a unified API for spaces, users, rooms, and messages,
// managing current JetStream resources internally.
type ChattoCore struct {
	nc                       *nats.Conn
	js                       jetstream.JetStream
	logger                   *log.Logger
	storage                  *storage
	config                   config.CoreConfig
	encryption               *encryptionManager
	dekResolver              *unwrappedDEKResolver
	configModel              *ConfigModel
	roomModel                *RoomModel
	roomCommands             *RoomCommandModel
	roomDirectoryReads       *RoomDirectoryReadModel
	messageModel             *MessageModel
	messageSearchReads       *MessageSearchReadModel
	notificationPrefs        *NotificationPreferencesModel
	roomTimelineReads        *RoomTimelineReadModel
	readStateModel           *ReadStateModel
	threadFollows            *ThreadFollowModel
	reactionModel            *ReactionModel
	userModel                *UserModel
	rbacModel                *RBACModel
	mentionables             *MentionablesModel
	myEventsModel            *MyEventsModel
	presenceModel            *PresenceModel
	mediaModel               *MediaModel
	callModel                *CallModel
	assetModel               *AssetModel
	assetUploadModel         *AssetUploadModel
	keyShredding             *UserKeyShreddingModel
	s3Client                 *S3Client            // Optional S3 client for S3-compatible storage
	permissionResolver       *PermissionResolver  // Hierarchical permission resolver
	linkPreviewCache         *linkpreview.Cache   // Cache for link preview metadata
	linkPreviewFetcher       *linkpreview.Fetcher // Fetcher for link preview metadata
	projectionSnapshotWorker *projectionSnapshotWorker
	natsRecoveryState        atomic.Int32
	natsRecoveryStartedAt    atomic.Int64
	natsRecoveredReconnects  atomic.Uint64

	// VideoMaxUploadSize is the maximum size for video uploads in bytes.
	// When set (> 0), video attachments use this limit instead of the asset limit.
	// Set this after ChattoCore is created, from VideoConfig.
	VideoMaxUploadSize int64

	// VideoUploadsEnabled makes message commits enqueue durable processing work
	// for accepted video-shaped attachments. Worker placement is configured
	// independently; the main process does not hand work to a local callback.
	VideoUploadsEnabled bool

	// OnNotificationCreated is called when a notification is created.
	// Used by the push notification system to send Web Push notifications.
	// Set this after ChattoCore is created.
	OnNotificationCreated func(ctx context.Context, notification *corev1.Notification)

	// OnNotificationDismissed is called when a notification is dismissed.
	// Used by the push notification system to dismiss notifications on other devices.
	// Set this after ChattoCore is created.
	OnNotificationDismissed func(ctx context.Context, userID string, notification *corev1.Notification)

	// OnPushTestRequested sends a test notification to a user's push subscriptions.
	OnPushTestRequested func(ctx context.Context, userID string) error

	// AssetBaseURL is prepended to all asset URLs to make them absolute.
	// When empty, URLs are returned as relative paths (backward compatible).
	// Set from webserver.url config: scheme + host only (no trailing slash).
	AssetBaseURL string

	// PresenceHub is the compatibility handle for PresenceModel's per-process
	// fanout hub. Started by (*ChattoCore).Run through PresenceModel.
	PresenceHub *PresenceHub

	// EventPublisher writes to the EVT event-sourcing stream
	// (ADR-033/034). Exposed for use by the migrate subcommand and
	// future aggregate cutovers; domain code accesses it through
	// higher-level helpers as aggregates migrate.
	EventPublisher *evtstream.Publisher

	// projections is the set of all event-sourcing projections owned by
	// this core. Each registration carries the runtime projector plus
	// operator-facing diagnostics, so lifecycle and admin surfaces cannot
	// drift into separate hand-maintained lists.
	projections []projectionRegistration

	// bootDone is closed by Run once all projectors are started AND
	// boot-time mutations (ensureChannelRoomsAreInAGroup) have
	// completed. Callers that need to issue projection-backed reads
	// during startup — most notably SeedDefaultRooms in cmd/run.go —
	// block on this via WaitForBoot.
	bootDone chan struct{}
}

// Run starts every background component owned by the core, including
// process-wide KV indexes, live models, and every registered projector. It
// blocks until ctx is cancelled or any component returns an error and returns
// the first error observed (or ctx.Err on shutdown).
//
// Call this once per process from an errgroup goroutine; tests typically
// launch it in a bare goroutine with a per-test context that cleanup
// cancels. Background services are not designed to be restarted.
//
// New projectors should be registered during NewChattoCore; they are then
// started automatically here without any additional wiring.
func (c *ChattoCore) Run(ctx context.Context) error {
	g, gctx := errgroup.WithContext(ctx)
	natsStatus := c.nc.StatusChanged(nats.DISCONNECTED, nats.RECONNECTING, nats.CONNECTED, nats.CLOSED)
	defer c.nc.RemoveStatusListener(natsStatus)
	g.Go(func() error { return c.runNATSRecovery(gctx, natsStatus) })

	for _, projection := range c.projections {
		projection := projection
		g.Go(func() error {
			if err := projection.projector.Run(gctx); err != nil {
				if errors.Is(err, context.Canceled) {
					return err
				}
				return fmt.Errorf("%s projection: %w", projection.name, err)
			}
			return nil
		})
	}

	// Block until every projector has entered Run before issuing
	// projection-backed mutations during boot. Without this,
	// ensureChannelRoomsAreInAGroup's reads against an empty
	// projection would silently skip the WaitFor path and leave
	// orphan rooms (rooms created without a group assignment).
	g.Go(func() error {
		if err := c.waitForProjectorsStarted(gctx, 5*time.Second); err != nil {
			return fmt.Errorf("wait for projectors: %w", err)
		}
		// Before issuing boot-time "ensure" mutations, let every
		// projection replay the durable stream as it exists now. A
		// started-but-cold projection would otherwise look empty and
		// append duplicate seed facts on every process restart.
		if err := c.WaitForProjectionsCurrent(gctx); err != nil {
			return fmt.Errorf("wait for projections current: %w", err)
		}
		if err := c.readStateModel.WaitReady(gctx); err != nil {
			return fmt.Errorf("wait for read state index: %w", err)
		}
		c.secureDeleteObsoleteProjectedMessageBodyEvents(gctx)
		// Apply config-designated owners to already-verified users on every
		// boot. Changing owners.emails requires a process restart, so this
		// is the natural point to materialize new config owners as RBAC
		// assignments. The assignment path is idempotent.
		if err := c.applyConfigOwners(gctx); err != nil {
			return fmt.Errorf("apply config owners: %w", err)
		}
		// Seed the default room group and ensure every existing
		// channel room belongs to a set (ADR-031). Idempotent —
		// runs on every boot. Has to happen AFTER projectors are
		// running and caught up because it reads RoomModel's group-layout
		// state and depends on WaitFor actually waiting.
		if err := c.ensureChannelRoomsAreInAGroup(gctx); err != nil {
			return fmt.Errorf("ensure channel rooms in a group: %w", err)
		}
		if c.nc.IsConnected() {
			c.natsRecoveredReconnects.Store(c.nc.Stats().Reconnects)
			c.natsRecoveryState.CompareAndSwap(natsRecoveryStarting, natsRecoveryReady)
		}
		close(c.bootDone)
		return nil
	})

	g.Go(func() error { return c.readStateModel.Run(gctx) })
	g.Go(func() error { return c.presenceModel.Run(gctx) })
	g.Go(func() error { return c.myEventsModel.Run(gctx) })
	g.Go(func() error { return c.callModel.Run(gctx) })
	g.Go(func() error { return c.assetModel.Run(gctx) })
	g.Go(func() error { return c.assetUploadModel.RunCleanup(gctx) })
	g.Go(func() error { return c.keyShredding.Run(gctx) })
	if c.projectionSnapshotWorker != nil {
		g.Go(func() error {
			err := c.projectionSnapshotWorker.Run(gctx, c.bootDone)
			if errors.Is(err, context.Canceled) {
				return err
			}
			// Snapshots are disposable acceleration data. The worker logs the
			// stage-specific failure and must never make core unavailable.
			return nil
		})
	}
	return g.Wait()
}

// AllProjectorsStarted reports whether every registered projector
// has entered its Run body. Test helpers (and any sequenced startup
// code) use this to wait for projector consumers to come online
// before issuing reads that depend on a populated projection — the
// background goroutines launched by Run aren't guaranteed to have
// been scheduled the instant `go core.Run(ctx)` returns.
func (c *ChattoCore) AllProjectorsStarted() bool {
	for _, projection := range c.projections {
		if !projection.projector.Started() {
			return false
		}
	}
	return true
}

// WaitForBoot blocks until Run has finished boot-time setup
// (projectors running + ensureChannelRoomsAreInAGroup done) or ctx
// is cancelled. Callers that issue projection-backed mutations during
// startup — e.g. SeedDefaultRooms in cmd/run.go — must wait here
// first; mutating before boot completes leaves orphan rooms because
// CreateRoom's default-group lookup reads the (still-empty)
// projection.
func (c *ChattoCore) WaitForBoot(ctx context.Context) error {
	select {
	case <-c.bootDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// WaitForProjectionsCurrent blocks until every registered projection has
// applied the latest stream message matching its filters as of this call.
// Intended for boot/import diagnostics, not hot request paths.
func (c *ChattoCore) WaitForProjectionsCurrent(ctx context.Context) error {
	for _, projection := range c.projections {
		if err := projection.projector.WaitForCurrent(ctx); err != nil {
			return fmt.Errorf("%s projection: %w", projection.name, err)
		}
	}
	return nil
}

// ProjectionHealthError returns the first fatal projection error currently
// recorded by any registered projector.
func (c *ChattoCore) ProjectionHealthError() error {
	for _, projection := range c.projections {
		if err := projection.projector.Err(); err != nil {
			return fmt.Errorf("%s projection: %w", projection.name, err)
		}
	}
	return nil
}

// waitForProjectorsStarted polls AllProjectorsStarted with a short
// interval until every projector has entered its Run body or the
// deadline / context elapses. The polling shape mirrors the test
// helper; this version lives in Run so production has the same
// guarantee without test-only code on the path.
func (c *ChattoCore) waitForProjectorsStarted(ctx context.Context, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for !c.AllProjectorsStarted() {
		if time.Now().After(deadline) {
			return fmt.Errorf("projectors did not start within %s", timeout)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond):
		}
	}
	return nil
}

// EventStreamForDebug returns the EVT stream. Intended for the
// `chatto evt list` command and similar low-level operator tooling that
// reads raw stream messages. Domain code goes through EventPublisher /
// Projector instead.
func (c *ChattoCore) EventStreamForDebug(_ context.Context) (jetstream.Stream, error) {
	return c.storage.serverEvtStream, nil
}

func (c *ChattoCore) ServerStore() jetstream.ObjectStore {
	return c.storage.serverAssets
}

// ConfigModel returns the runtime configuration model.
func (c *ChattoCore) ConfigModel() *ConfigModel {
	return c.configModel
}

// PermResolver returns the hierarchical permission resolver for permission checks.
// This implements the server < space < room specificity model.
func (c *ChattoCore) PermResolver() *PermissionResolver {
	return c.permissionResolver
}

// Ready checks if the core is fully initialized and current persistent resources are accessible.
// Returns nil if ready, or an error describing what's not ready.
// Used by the /readyz endpoint to verify the server can handle requests.
func (c *ChattoCore) Ready(ctx context.Context) error {
	if c.nc == nil || !c.nc.IsConnected() {
		return fmt.Errorf("NATS not connected")
	}
	if c.natsRecoveryState.Load() != natsRecoveryReady {
		return fmt.Errorf("NATS recovery is not complete")
	}
	if c.natsRecoveredReconnects.Load() != c.nc.Stats().Reconnects {
		return fmt.Errorf("NATS reconnect has not been recovered")
	}
	if _, err := c.storage.runtimeStateKV.Status(ctx); err != nil {
		return fmt.Errorf("RUNTIME_STATE not ready: %w", err)
	}
	if _, err := c.storage.serverEvtStream.Info(ctx); err != nil {
		return fmt.Errorf("EVT not ready: %w", err)
	}
	if err := c.ProjectionHealthError(); err != nil {
		return fmt.Errorf("projection unhealthy: %w", err)
	}
	return nil
}

// NewChattoCore creates and initializes a new ChattoCore instance.
// This should be called once at application startup.
func NewChattoCore(ctx context.Context, nc *nats.Conn, cfg config.CoreConfig) (*ChattoCore, error) {
	logger := log.WithPrefix("core.ChattoCore")

	infra, err := initializeCoreInfrastructure(ctx, nc, cfg, logger)
	if err != nil {
		return nil, err
	}
	projections, err := initializeCoreProjections(infra, logger)
	if err != nil {
		return nil, err
	}

	core := assembleCore(nc, cfg, infra, projections, logger)

	// ensureChannelRoomsAreInAGroup is deferred to core.Run() — it
	// needs the projectors to be live so its CreateRoomGroup /
	// MoveRoomToGroup calls can actually WaitFor. Doing it here
	// (when projectors haven't been started yet) would leave orphan
	// rooms in any subsequent SeedDefaultRooms call.
	if err := initializeCoreServices(ctx, core, infra, projections, cfg, logger); err != nil {
		return nil, err
	}
	return core, nil
}

func (c *ChattoCore) Subscribe(ctx context.Context, subject string, handler nats.MsgHandler) (*nats.Subscription, error) {
	sub, err := c.nc.Subscribe(subject, handler)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to subject %s: %w", subject, err)
	}

	return sub, nil
}

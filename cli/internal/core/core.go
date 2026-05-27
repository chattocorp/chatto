package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sync/errgroup"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/assets"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core/linkpreview"
	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/encryption"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/migrations"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// ChattoCore
// ============================================================================

// ChattoCore is the central hub for all Chatto operations.
// It provides a unified API for spaces, users, rooms, and messages,
// managing all KV buckets and event streams internally.
type ChattoCore struct {
	nc                 *nats.Conn
	js                 jetstream.JetStream
	logger             *log.Logger
	storage            *storage
	config             config.CoreConfig
	encryption         *encryptionManager
	configManager      *ConfigManager
	s3Client           *S3Client            // Optional S3 client for S3-compatible storage
	permissionResolver *PermissionResolver  // Hierarchical permission resolver
	linkPreviewCache   *linkpreview.Cache   // Cache for link preview metadata
	linkPreviewFetcher *linkpreview.Fetcher // Fetcher for link preview metadata

	// VideoMaxUploadSize is the maximum size for video uploads in bytes.
	// When set (> 0), video attachments use this limit instead of the asset limit.
	// Set this after ChattoCore is created, from VideoConfig.
	VideoMaxUploadSize int64

	// OnNotificationCreated is called when a notification is created.
	// Used by the push notification system to send Web Push notifications.
	// Set this after ChattoCore is created.
	OnNotificationCreated func(ctx context.Context, notification *corev1.Notification)

	// OnNotificationDismissed is called when a notification is dismissed.
	// Used by the push notification system to dismiss notifications on other devices.
	// Set this after ChattoCore is created.
	OnNotificationDismissed func(ctx context.Context, userID string, notification *corev1.Notification)

	// AssetBaseURL is prepended to all asset URLs to make them absolute.
	// When empty, URLs are returned as relative paths (backward compatible).
	// Set from webserver.url config: scheme + host only (no trailing slash).
	AssetBaseURL string

	// PresenceHub runs a single KV watcher on presence.> per process and fans
	// out updates to all space subscriptions. Started by (*ChattoCore).Run.
	PresenceHub *PresenceHub

	// EventPublisher writes to the EVT event-sourcing stream
	// (ADR-033/034). Exposed for use by the migrate subcommand and
	// future aggregate cutovers; domain code accesses it through
	// higher-level helpers as aggregates migrate.
	EventPublisher *events.Publisher

	// RoomMembership is the first event-sourced projection (ADR-033 POC).
	// Populated by RoomMembershipProjector; not yet read by any
	// user-facing resolver. Inspectable via an admin endpoint.
	RoomMembership *RoomMembershipProjection

	// RoomMembershipProjector runs the consumer + apply loop that keeps
	// RoomMembership current. Started by (*ChattoCore).Run; exposed here
	// so writers can call WaitForSeq for read-your-writes.
	RoomMembershipProjector *events.Projector

	// ServerConfig is the projection holding the current server-config
	// snapshot (ADR-035 phase 5 cutover for the config aggregate).
	// Reads of "what's the server's MOTD / welcome message / blocked
	// usernames" go through this projection rather than the legacy
	// INSTANCE_CONFIG KV.
	ServerConfig *ServerConfigProjection

	// ServerConfigProjector runs the consumer + apply loop that keeps
	// ServerConfig current. Started by (*ChattoCore).Run; exposed here
	// so writers (ConfigManager mutations) can call WaitForSeq.
	ServerConfigProjector *events.Projector

	// RoomCatalog is the projection holding per-room metadata
	// (id/name/description/kind/archived) derived from evt.room.>.
	// Coexists with RoomMembership on the same subject family.
	RoomCatalog *RoomCatalogProjection

	// RoomCatalogProjector runs the consumer for RoomCatalog. Exposed
	// for WaitForSeq from room-metadata writers.
	RoomCatalogProjector *events.Projector

	// RoomGroups is the projection holding per-group metadata and
	// ordered room membership, derived from evt.group.>.
	RoomGroups *RoomGroupProjection

	// RoomGroupsProjector runs the consumer for RoomGroups. Exposed
	// for WaitForSeq from group writers.
	RoomGroupsProjector *events.Projector

	// RoomLayout holds the operator-defined inter-group ordering for
	// the sidebar, derived from evt.layout.> events.
	RoomLayout *RoomLayoutProjection

	// RoomLayoutProjector runs the consumer for RoomLayout. Exposed
	// for WaitForSeq from layout writers.
	RoomLayoutProjector *events.Projector

	// RoomTimeline holds an append-only event log per room, derived
	// from the full evt.room.> firehose (#597 phase 2). Source of
	// truth for room timeline reads post-cutover.
	RoomTimeline *RoomTimelineProjection

	// RoomTimelineProjector runs the consumer for RoomTimeline.
	// Exposed for WaitForSeq from message writers.
	RoomTimelineProjector *events.Projector

	// Threads holds an append-only event log per thread root,
	// derived from the same evt.room.> firehose. Source of truth
	// for thread-pane reads post-cutover.
	Threads *ThreadProjection

	// ThreadsProjector runs the consumer for Threads. Exposed for
	// WaitForSeq from message writers that touch threads.
	ThreadsProjector *events.Projector

	// Reactions holds current per-message reaction state derived
	// from durable room-aggregate reaction events.
	Reactions *ReactionProjection

	// ReactionsProjector runs the consumer for Reactions. Exposed
	// for WaitForSeq from reaction writers.
	ReactionsProjector *events.Projector

	// Users holds current user/account/profile/auth lookup state derived
	// from durable user-aggregate events.
	Users *UserProjection

	// UsersProjector runs the consumer for Users. Exposed for
	// WaitForSeq from user/account writers.
	UsersProjector *events.Projector

	// RBAC holds current role, assignment, and permission state derived
	// from durable RBAC aggregate events.
	RBAC *RBACProjection

	// RBACProjector runs the consumer for RBAC. Exposed for WaitForSeq
	// from role and permission writers.
	RBACProjector *events.Projector

	// projectors is the set of all event-sourcing projectors owned by
	// this core. Each new aggregate migration (ADR-035) appends here
	// during NewChattoCore; Run iterates the slice. Adding a projector
	// is one line at construction and zero changes at call sites — see
	// (*ChattoCore).Run.
	projectors []*events.Projector

	// bootDone is closed by Run once all projectors are started AND
	// boot-time mutations (ensureChannelRoomsAreInAGroup) have
	// completed. Callers that need to issue projection-backed reads
	// during startup — most notably SeedDefaultRooms in cmd/run.go —
	// block on this via WaitForBoot.
	bootDone chan struct{}
}

// Run starts every background service owned by the core — currently
// PresenceHub and every registered projector — and blocks until ctx is
// cancelled or any service returns an error. Returns the first error
// observed (or ctx.Err on shutdown).
//
// Call this once per process from an errgroup goroutine; tests typically
// launch it in a bare goroutine with a per-test context that cleanup
// cancels. Background services are not designed to be restarted.
//
// New projectors should be appended to c.projectors during NewChattoCore;
// they are then started automatically here without any additional wiring.
func (c *ChattoCore) Run(ctx context.Context) error {
	g, gctx := errgroup.WithContext(ctx)

	for _, p := range c.projectors {
		p := p
		g.Go(func() error { return p.Run(gctx) })
	}

	// Block until every projector has entered Run before issuing
	// projection-backed mutations during boot. Without this,
	// ensureChannelRoomsAreInAGroup's reads against an empty
	// projection would silently skip the WaitForSeq path and leave
	// orphan rooms (rooms created without a group assignment).
	g.Go(func() error {
		if err := c.waitForProjectorsStarted(gctx, 5*time.Second); err != nil {
			return fmt.Errorf("wait for projectors: %w", err)
		}
		// Seed the default room group and ensure every existing
		// channel room belongs to a set (ADR-031). Idempotent —
		// runs on every boot. Has to happen AFTER projectors are
		// running because it both reads the RoomGroups projection
		// and depends on WaitForSeq actually waiting.
		if err := c.ensureChannelRoomsAreInAGroup(gctx); err != nil {
			return fmt.Errorf("ensure channel rooms in a group: %w", err)
		}
		if c.config.ESBootVerify {
			if err := c.WaitForProjectionsCurrent(gctx); err != nil {
				return fmt.Errorf("wait for projections current: %w", err)
			}
			c.logESBootVerification(gctx)
		}
		close(c.bootDone)
		return nil
	})

	g.Go(func() error { return c.PresenceHub.Run(gctx) })

	return g.Wait()
}

// AllProjectorsStarted reports whether every registered projector
// has entered its Run body. Test helpers (and any sequenced startup
// code) use this to wait for projector consumers to come online
// before issuing reads that depend on a populated projection — the
// background goroutines launched by Run aren't guaranteed to have
// been scheduled the instant `go core.Run(ctx)` returns.
func (c *ChattoCore) AllProjectorsStarted() bool {
	for _, p := range c.projectors {
		if !p.Started() {
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
	for _, p := range c.projectors {
		if err := p.WaitForCurrent(ctx); err != nil {
			return err
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

// assetURL prepends AssetBaseURL to an asset path.
// When AssetBaseURL is empty, returns the path unchanged.
func (c *ChattoCore) assetURL(path string) string {
	if c.AssetBaseURL == "" {
		return path
	}
	return c.AssetBaseURL + path
}

// encryptionManager handles message body encryption/decryption.
type encryptionManager struct {
	keyManager *encryption.KeyManager
}

func (c *ChattoCore) ServerStore() jetstream.ObjectStore {
	return c.storage.serverStore
}

// KeyManager returns the encryption key manager.
// Used by the KMS service to handle encryption operations.
func (c *ChattoCore) KeyManager() *encryption.KeyManager {
	return c.encryption.keyManager
}

// ConfigManager returns the runtime configuration manager.
// Used by GraphQL resolvers to read/write runtime config.
func (c *ChattoCore) ConfigManager() *ConfigManager {
	return c.configManager
}

// PermResolver returns the hierarchical permission resolver for permission checks.
// This implements the server < space < room specificity model.
func (c *ChattoCore) PermResolver() *PermissionResolver {
	return c.permissionResolver
}

// DeleteUserEncryptionKey permanently deletes a user's encryption key (crypto-shredding).
// All messages encrypted with this key become permanently unreadable.
// This is used for GDPR-compliant user deletion.
func (c *ChattoCore) DeleteUserEncryptionKey(ctx context.Context, userID string) error {
	if c.encryption.keyManager == nil {
		return nil // Encryption not configured
	}
	return c.encryption.keyManager.DeleteUserKey(ctx, userID)
}

// AssetsConfig returns the assets configuration as an assets.Config.
func (c *ChattoCore) AssetsConfig() assets.Config {
	maxUploadSize := int64(c.config.Assets.MaxUploadSize)
	if maxUploadSize == 0 {
		maxUploadSize = assets.DefaultMaxUploadSize
	}
	return assets.Config{
		MaxUploadSize: maxUploadSize,
	}
}

// ShouldUseS3 returns true if new uploads should be stored in S3.
func (c *ChattoCore) ShouldUseS3() bool {
	return c.config.Assets.StorageBackend == config.StorageBackendS3 && c.s3Client != nil
}

// GetLinkPreview fetches link preview metadata for a URL.
// Results are cached server-side. Returns nil if the URL cannot be previewed.
func (c *ChattoCore) GetLinkPreview(ctx context.Context, url string) (*corev1.LinkPreview, error) {
	// Check cache first
	cached, err := c.linkPreviewCache.Get(ctx, url)
	if errors.Is(err, linkpreview.ErrCachedFailure) {
		// Negative cache hit - URL previously failed, don't re-fetch
		return nil, nil
	}
	if err != nil {
		c.logger.Warn("Failed to get cached link preview", "url", url, "error", err)
		// Continue to fetch - don't fail on cache errors
	}
	if cached != nil {
		return cached, nil
	}

	// Fetch the preview
	result, err := c.linkPreviewFetcher.Fetch(ctx, url)
	if err != nil {
		// Cache the failure to avoid repeated fetches
		_ = c.linkPreviewCache.SetFailure(ctx, url, err.Error())
		return nil, err
	}

	preview := result.ToProto(url)

	// Cache the result
	if err := c.linkPreviewCache.Set(ctx, url, preview); err != nil {
		c.logger.Warn("Failed to cache link preview", "url", url, "error", err)
	}

	return preview, nil
}

// S3Client returns the S3 client, or nil if S3 is not configured.
func (c *ChattoCore) S3Client() *S3Client {
	return c.s3Client
}

// ServerAssetInfo contains metadata about a server asset.
type ServerAssetInfo struct {
	Size        int64
	ContentType string
}

// GetServerAssetFromAnyBackend retrieves a server asset by probing both NATS and S3 backends.
// It tries NATS first (for backwards compatibility with existing assets), then S3.
// Returns a reader for the asset content and metadata.
// The caller is responsible for closing the reader if it implements io.Closer.
func (c *ChattoCore) GetServerAssetFromAnyBackend(ctx context.Context, assetID string) (io.Reader, *ServerAssetInfo, error) {
	// Try NATS first (backwards compatibility)
	obj, err := c.storage.serverStore.Get(ctx, assetID)
	if err == nil {
		info, _ := obj.Info()
		return obj, &ServerAssetInfo{
			Size:        int64(info.Size),
			ContentType: info.Headers.Get("Content-Type"),
		}, nil
	}

	// If NATS failed and S3 is configured, try S3
	if c.s3Client != nil {
		s3Key := S3KeyServerAsset(assetID)
		reader, s3Info, s3Err := c.s3Client.GetObject(ctx, s3Key)
		if s3Err == nil {
			return reader, &ServerAssetInfo{
				Size:        s3Info.Size,
				ContentType: s3Info.ContentType,
			}, nil
		}
		// Log S3 error but return the original NATS error
		c.logger.Debug("Instance asset not found in either backend",
			"asset_id", assetID,
			"nats_error", err,
			"s3_error", s3Err)
	}

	return nil, nil, err
}

// CleanupAsset deletes an asset from the server object store.
// Used to clean up orphaned assets when subsequent operations fail.
func (c *ChattoCore) CleanupAsset(ctx context.Context, asset *corev1.Asset) {
	if asset == nil {
		return
	}
	if natsAsset := asset.GetNats(); natsAsset != nil {
		if err := c.storage.serverStore.Delete(ctx, natsAsset.Key); err != nil {
			c.logger.Warn("Failed to clean up orphaned asset", "key", natsAsset.Key, "error", err)
		} else {
			c.logger.Info("Cleaned up orphaned asset", "key", natsAsset.Key)
		}
	}
	if s3Asset := asset.GetS3(); s3Asset != nil && c.s3Client != nil {
		// S3Asset.Key stores just the assetID; construct the full S3 path
		s3Key := S3KeyServerAsset(s3Asset.Key)
		if err := c.s3Client.DeleteObjectFromBucket(ctx, s3Asset.GetBucket(), s3Key); err != nil {
			c.logger.Warn("Failed to clean up orphaned S3 asset", "asset_id", s3Asset.Key, "s3_key", s3Key, "error", err)
		} else {
			c.logger.Info("Cleaned up orphaned S3 asset", "asset_id", s3Asset.Key, "s3_key", s3Key)
		}
	}
}

// deleteAsset deletes a server asset from its storage backend (NATS or S3).
// This is a helper for cleaning up old assets when they are replaced.
// For S3, the assetID stored in S3Asset.Key is used to construct the full S3 path.
// The assetType and ownerID are used for logging only.
func (c *ChattoCore) deleteAsset(ctx context.Context, asset *corev1.Asset, assetType, ownerID string) {
	if asset == nil {
		return
	}
	if natsAsset := asset.GetNats(); natsAsset != nil {
		if err := c.storage.serverStore.Delete(ctx, natsAsset.Key); err != nil {
			c.logger.Warn("Failed to delete old "+assetType, "owner_id", ownerID, "key", natsAsset.Key, "error", err)
		} else {
			c.logger.Info("Deleted old "+assetType, "owner_id", ownerID, "key", natsAsset.Key)
		}
	}
	if s3Asset := asset.GetS3(); s3Asset != nil && c.s3Client != nil {
		// S3Asset.Key stores just the assetID; construct the full S3 path
		s3Key := S3KeyServerAsset(s3Asset.Key)
		if err := c.s3Client.DeleteObjectFromBucket(ctx, s3Asset.GetBucket(), s3Key); err != nil {
			c.logger.Warn("Failed to delete old S3 "+assetType, "owner_id", ownerID, "asset_id", s3Asset.Key, "s3_key", s3Key, "error", err)
		} else {
			c.logger.Info("Deleted old S3 "+assetType, "owner_id", ownerID, "asset_id", s3Asset.Key, "s3_key", s3Key)
		}
	}
}

// Ready checks if the core is fully initialized and JetStream resources are accessible.
// Returns nil if ready, or an error describing what's not ready.
// Used by the /readyz endpoint to verify the server can handle requests.
func (c *ChattoCore) Ready(ctx context.Context) error {
	// Check if JetStream is operational by getting the INSTANCE KV bucket status
	_, err := c.storage.serverKV.Status(ctx)
	if err != nil {
		return fmt.Errorf("JetStream not ready: %w", err)
	}
	return nil
}

// NewChattoCore creates and initializes a new ChattoCore instance.
// This should be called once at application startup.
func NewChattoCore(ctx context.Context, nc *nats.Conn, cfg config.CoreConfig) (*ChattoCore, error) {
	logger := log.WithPrefix("core.ChattoCore")

	// Create JetStream context
	js, err := jetstream.New(nc, jetstream.WithDefaultTimeout(30*time.Second))
	if err != nil {
		return nil, fmt.Errorf("failed to create JetStream context: %w", err)
	}

	// Initialize storage (KV buckets)
	storage, err := newStorage(js, ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Initialize encryption manager
	encMgr := &encryptionManager{
		keyManager: encryption.NewKeyManager(storage.encryptionKV),
	}

	// Phase 5 of #330 collapsed the dual instance-/space-RBAC engines into a
	// single server-RBAC engine wrapping SERVER_RBAC. All permission checks
	// go through here.
	storage.serverRBACEngine = NewEngine(storage.serverRBACKV, Config{
		SystemRoles:  []string{RoleOwner, RoleAdmin, RoleModerator, RoleEveryone},
		AdminRole:    RoleOwner,
		VirtualRoles: VirtualRoles(),
		ValidateVerbObjectType: func(verb, objectType string) error {
			perm := ReconstructPermission(verb, objectType)
			if perm == "" {
				return fmt.Errorf("%w: verb=%s, objectType=%s", ErrInvalidPermission, verb, objectType)
			}
			return nil
		},
		Logger: slog.Default().With("component", "server-rbac"),
	})

	// Initialize S3 client if S3 storage is configured
	var s3Client *S3Client
	if cfg.Assets.StorageBackend == config.StorageBackendS3 {
		var err error
		s3Client, err = NewS3Client(cfg.Assets.S3)
		if err != nil {
			return nil, fmt.Errorf("failed to create S3 client: %w", err)
		}
		if s3Client != nil {
			// Ensure the bucket exists
			if err := s3Client.EnsureBucket(ctx); err != nil {
				return nil, fmt.Errorf("failed to ensure S3 bucket: %w", err)
			}
			logger.Info("S3 storage initialized", "bucket", s3Client.Bucket())
		}
	}

	// Build the event-sourcing primitives before any aggregate-specific
	// wiring so projections and managers that need them can be passed the
	// concrete deps at construction. Order: publisher → projections →
	// projectors → managers that depend on them.
	eventPublisher := events.NewPublisher(js, storage.serverEvtStream, logger)

	// newProjector wraps the (projection, logger-prefix) → Projector
	// construction so the per-aggregate wiring below stays one line per
	// projection. Each projector also gets appended to the slice that
	// (*ChattoCore).Run iterates.
	var projectors []*events.Projector
	newProjector := func(p events.Projection, name string) *events.Projector {
		pr := events.NewProjector(js, storage.serverEvtStream, p, logger.WithPrefix("core."+name))
		projectors = append(projectors, pr)
		return pr
	}

	roomMembership := NewRoomMembershipProjection()
	roomMembershipProjector := newProjector(roomMembership, "RoomMembershipProjector")

	serverConfigProjection := NewServerConfigProjection()
	serverConfigProjector := newProjector(serverConfigProjection, "ServerConfigProjector")

	roomCatalog := NewRoomCatalogProjection()
	roomCatalogProjector := newProjector(roomCatalog, "RoomCatalogProjector")

	roomGroups := NewRoomGroupProjection()
	roomGroupsProjector := newProjector(roomGroups, "RoomGroupsProjector")

	roomLayout := NewRoomLayoutProjection()
	roomLayoutProjector := newProjector(roomLayout, "RoomLayoutProjector")

	// Per-room event-log + per-thread event-log projections (#597
	// phase 2). Both consume the full evt.room.> firehose; resolvers
	// do all filtering and rendering at query time. v1 shape — we
	// iterate significantly on this once we observe read patterns.
	roomTimeline := NewRoomTimelineProjection()
	roomTimelineProjector := newProjector(roomTimeline, "RoomTimelineProjector")

	threads := NewThreadProjection()
	threadsProjector := newProjector(threads, "ThreadsProjector")

	reactions := NewReactionProjection()
	reactionsProjector := newProjector(reactions, "ReactionsProjector")

	users := NewUserProjection()
	usersProjector := newProjector(users, "UsersProjector")

	rbac := NewRBACProjection()
	rbacProjector := newProjector(rbac, "RBACProjector")

	// ConfigManager owns event-only server-config writes; it needs the
	// publisher (for ServerConfigChangedEvent), the projector
	// (WaitForSeq for read-your-writes), and the projection (for reads).
	configMgr := NewConfigManager(eventPublisher, serverConfigProjector, serverConfigProjection)

	core := &ChattoCore{
		nc:                      nc,
		js:                      js,
		logger:                  logger,
		storage:                 storage,
		config:                  cfg,
		encryption:              encMgr,
		configManager:           configMgr,
		s3Client:                s3Client,
		EventPublisher:          eventPublisher,
		RoomMembership:          roomMembership,
		RoomMembershipProjector: roomMembershipProjector,
		ServerConfig:            serverConfigProjection,
		ServerConfigProjector:   serverConfigProjector,
		RoomCatalog:             roomCatalog,
		RoomCatalogProjector:    roomCatalogProjector,
		RoomGroups:              roomGroups,
		RoomGroupsProjector:     roomGroupsProjector,
		RoomLayout:              roomLayout,
		RoomLayoutProjector:     roomLayoutProjector,
		RoomTimeline:            roomTimeline,
		RoomTimelineProjector:   roomTimelineProjector,
		Threads:                 threads,
		ThreadsProjector:        threadsProjector,
		Reactions:               reactions,
		ReactionsProjector:      reactionsProjector,
		Users:                   users,
		UsersProjector:          usersProjector,
		RBAC:                    rbac,
		RBACProjector:           rbacProjector,
		projectors:              projectors,
		bootDone:                make(chan struct{}),
	}

	if err := core.migrateRBACToES(ctx); err != nil {
		return nil, fmt.Errorf("failed to migrate RBAC to ES: %w", err)
	}

	// Run boot-time data migrations. Idempotent and cheap on subsequent
	// boots (each migration short-circuits when no legacy data remains).
	// See cli/internal/migrations for the registry, including the
	// ADR-035 ES seed migrations that need the event publisher we just
	// constructed.
	//
	// In the typical embedded-NATS deployment the NATS server has no
	// TCP listener, so we can't run migrations from a second process —
	// the boot path is the only safe place for them.
	if err := migrations.RunAll(
		ctx,
		storage.serverKV, storage.serverConfigKV, storage.serverBodiesKV, storage.serverRuntimeKV, storage.runtimeConfigKV,
		storage.runtimeStateKV,
		storage.serverEventsStream, storage.serverReactionsKV,
		eventPublisher,
		logger,
	); err != nil {
		return nil, fmt.Errorf("failed to run boot migrations: %w", err)
	}

	// Initialize permission resolver (must be done after core struct is created)
	core.permissionResolver = NewPermissionResolver(core)

	// Initialize link preview cache and fetcher
	linkPreviewCache, err := linkpreview.NewCache(ctx, js, cfg.Replicas)
	if err != nil {
		return nil, fmt.Errorf("failed to create link preview cache: %w", err)
	}
	core.linkPreviewCache = linkPreviewCache
	assetsConfig := core.AssetsConfig()
	core.linkPreviewFetcher = linkpreview.NewFetcher(storage.serverStore, &assetsConfig, NewAssetID)

	// ensureChannelRoomsAreInAGroup is deferred to core.Run() — it
	// needs the projectors to be live so its CreateRoomGroup /
	// MoveRoomToGroup calls can actually WaitForSeq. Doing it here
	// (when projectors haven't been started yet) would leave orphan
	// rooms in any subsequent SeedDefaultRooms call.

	// Initialize presence hub (single KV watcher per process). Started
	// by core.Run alongside the projectors.
	core.PresenceHub = NewPresenceHub(storage.presenceKV, logger)

	return core, nil
}

func (c *ChattoCore) Subscribe(ctx context.Context, subject string, handler nats.MsgHandler) (*nats.Subscription, error) {
	sub, err := c.nc.Subscribe(subject, handler)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to subject %s: %w", subject, err)
	}

	return sub, nil
}

// ============================================================================
// Storage
// ============================================================================

// storage encapsulates all JetStream KV buckets and streams used by Chatto Core.
type storage struct {
	serverKV        jetstream.KeyValue
	serverStore     jetstream.ObjectStore
	encryptionKV    jetstream.KeyValue // Encryption keys (excluded from backups)
	runtimeConfigKV jetstream.KeyValue // INSTANCE_CONFIG - runtime configuration overrides
	runtimeStateKV  jetstream.KeyValue // RUNTIME_STATE  - persisted latest-value runtime/user state

	// Server-level KV buckets (#330 phase 4a, 4b, 4c, 4e) and event stream
	// (#330 phase 4d). Shared by the primary and DM spaces; non-primary,
	// non-DM spaces (test-created only in practice) keep their per-space
	// lazycaches below.
	serverConfigKV     jetstream.KeyValue    // SERVER_CONFIG    - rooms, memberships
	serverRuntimeKV    jetstream.KeyValue    // SERVER_RUNTIME   - sequences, timestamps, read state
	serverRBACKV       jetstream.KeyValue    // SERVER_RBAC      - roles, permissions, assignments
	serverRBACEngine   *Engine               // Engine wrapping serverRBACKV
	serverBodiesKV     jetstream.KeyValue    // SERVER_BODIES    - message bodies + attachment metadata records (#330 phase 4c). TODO: rename → SERVER_CONTENT now that it hosts more than bodies.
	serverReactionsKV  jetstream.KeyValue    // SERVER_REACTIONS - emoji reactions (#330 phase 4c)
	serverThreadsKV    jetstream.KeyValue    // SERVER_THREADS   - thread metadata (#330 phase 4c)
	serverAttachments  jetstream.ObjectStore // SERVER_ASSETS    - message attachment binaries (#330 phase 4e)
	serverEventsStream jetstream.Stream      // SERVER_EVENTS    - event stream (#330 phase 4d)
	serverEvtStream    jetstream.Stream      // EVT       - event-sourcing log (ADR-033/034). Coexists with SERVER_EVENTS during migration.

	presenceKV      jetstream.KeyValue    // Instance-level presence bucket
	imageCacheStore jetstream.ObjectStore // Optional: cached resized images (nil if disabled)
	callStateKV     jetstream.KeyValue    // Active voice call participants (ephemeral, memory-backed)
	authTokensKV    jetstream.KeyValue    // Bearer auth tokens with TTL
}

// newStorage initializes all JetStream KV buckets and streams.
func newStorage(js jetstream.JetStream, ctx context.Context, cfg config.CoreConfig) (*storage, error) {
	// Initialize INSTANCE KV bucket for all server-level data
	// Uses subject-based keys: user.{userId}, space.{spaceId}, space_membership.{spaceId}.{userId}, etc.
	serverKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "INSTANCE",
		Description: "Instance-level data (users, spaces, memberships)",
		Storage:     jetstream.FileStorage,
		Replicas:    cfg.Replicas,
		// Enables per-key TTL via jetstream.KeyTTL(...) on Create. Used for short-lived
		// entries like registration tokens and email-verification tokens so they leave
		// the bucket automatically. The duration is how long delete markers from
		// TTL-expiry are kept before purging.
		LimitMarkerTTL: 24 * time.Hour,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create INSTANCE KV bucket: %w", err)
	}

	// Initialize server object store
	serverStore, err := js.CreateOrUpdateObjectStore(ctx, jetstream.ObjectStoreConfig{
		Bucket:      "INSTANCE_ASSETS",
		Description: "Instance-level assets (user avatars, space icons, etc.)",
		Storage:     jetstream.FileStorage,
		Compression: true,
		Replicas:    cfg.Replicas,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create INSTANCE object store: %w", err)
	}

	// Initialize server-level presence KV bucket (memory-based with TTL)
	presenceKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:         "USER_PRESENCE",
		Description:    "Instance-level user presence status",
		Storage:        jetstream.MemoryStorage, // Memory-based for speed, no persistence needed
		TTL:            PresenceTTL,             // Auto-expire entries that aren't refreshed
		History:        1,                       // Only current value needed
		LimitMarkerTTL: PresenceTTL,             // Emit delete markers on TTL expiry so watchers get notified
		Replicas:       cfg.Replicas,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create PRESENCE KV bucket: %w", err)
	}

	// Initialize encryption keys KV bucket (excluded from backups for security)
	// Keys are stored separately so backups contain only encrypted data, not the keys to decrypt it
	encryptionKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "ENCRYPTION_KEYS",
		Description: "User encryption keys (excluded from backups)",
		Storage:     jetstream.FileStorage,
		Replicas:    cfg.Replicas,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create ENCRYPTION_KEYS KV bucket: %w", err)
	}

	// Initialize runtime configuration KV bucket
	runtimeConfigKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "INSTANCE_CONFIG",
		Description: "Runtime configuration overrides",
		Storage:     jetstream.FileStorage,
		Replicas:    cfg.Replicas,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create INSTANCE_CONFIG KV bucket: %w", err)
	}

	runtimeStateKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:         "RUNTIME_STATE",
		Description:    "Persisted latest-value runtime/user state",
		Storage:        jetstream.FileStorage,
		History:        1,
		Compression:    true,
		Replicas:       cfg.Replicas,
		LimitMarkerTTL: 24 * time.Hour,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create RUNTIME_STATE KV bucket: %w", err)
	}

	// Initialize image cache object store (optional, only when enabled)
	var imageCacheStore jetstream.ObjectStore
	if cfg.Assets.Cache.Enabled {
		imageCacheStore, err = js.CreateOrUpdateObjectStore(ctx, jetstream.ObjectStoreConfig{
			Bucket:      "ASSET_CACHE",
			Description: "Cached resized images",
			Storage:     jetstream.FileStorage,
			Compression: true,
			TTL:         cfg.Assets.Cache.TTLOrDefault(),
			Replicas:    cfg.Replicas,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create ASSET_CACHE object store: %w", err)
		}
	}

	// Initialize call state KV bucket (memory-backed, ephemeral)
	// Tracks active voice call participants. Keys: {spaceId}.{roomId} → JSON participant list.
	// Memory storage is intentional: call state is transient and will be repopulated
	// by LiveKit webhooks after a server restart.
	callStateKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "CALL_STATE",
		Description: "Active voice call participants (ephemeral)",
		Storage:     jetstream.MemoryStorage,
		History:     1,
		Replicas:    cfg.Replicas,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create CALL_STATE KV bucket: %w", err)
	}

	// Initialize server-level KV buckets (#330 phase 4a, 4b, 4c). These hold
	// the deployment-wide primary + DM data. Non-primary, non-DM spaces
	// (test-created only in practice) keep their per-space SPACE_{id}_*
	// buckets.
	serverConfigKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "SERVER_CONFIG",
		Description: "Server-level configuration (rooms, memberships)",
		Storage:     jetstream.FileStorage,
		Replicas:    cfg.Replicas,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create SERVER_CONFIG KV bucket: %w", err)
	}

	serverRBACKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "SERVER_RBAC",
		Description: "Server-level RBAC (roles, permissions, assignments)",
		Storage:     jetstream.FileStorage,
		Replicas:    cfg.Replicas,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create SERVER_RBAC KV bucket: %w", err)
	}

	serverRuntimeKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "SERVER_RUNTIME",
		Description: "Server-level runtime state (sequences, read status)",
		Storage:     jetstream.FileStorage,
		Replicas:    cfg.Replicas,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create SERVER_RUNTIME KV bucket: %w", err)
	}

	serverBodiesKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "SERVER_BODIES",
		Description: "Server-level message bodies",
		Storage:     jetstream.FileStorage,
		Replicas:    cfg.Replicas,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create SERVER_BODIES KV bucket: %w", err)
	}

	serverReactionsKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "SERVER_REACTIONS",
		Description: "Server-level emoji reactions",
		Storage:     jetstream.FileStorage,
		Replicas:    cfg.Replicas,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create SERVER_REACTIONS KV bucket: %w", err)
	}

	serverThreadsKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "SERVER_THREADS",
		Description: "Server-level thread metadata",
		Storage:     jetstream.FileStorage,
		Replicas:    cfg.Replicas,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create SERVER_THREADS KV bucket: %w", err)
	}

	serverAttachments, err := js.CreateOrUpdateObjectStore(ctx, jetstream.ObjectStoreConfig{
		Bucket:      "SERVER_ASSETS",
		Description: "Server-level message attachments",
		Storage:     jetstream.FileStorage,
		Compression: true,
		Replicas:    cfg.Replicas,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create SERVER_ASSETS object store: %w", err)
	}

	// Initialize the deployment-wide events stream (#330 phase 4d). Holds all
	// JetStream events for the primary space and the DM system space; non-
	// primary, non-DM spaces (test-created only in production) keep their
	// per-space SPACE_{id}_EVENTS streams.
	serverEventsStream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:               "SERVER_EVENTS",
		Description:        "Server-level event stream (primary + DM)",
		Subjects:           []string{"server.>"},
		Storage:            jetstream.FileStorage,
		Compression:        jetstream.S2Compression,
		AllowAtomicPublish: true,
		Replicas:           cfg.Replicas,
		// Republish every accepted stream message onto a NATS Core
		// subject so subscribers can listen for room events without
		// holding a per-connection JetStream consumer. The republish
		// fires after persistence, so consumers cannot observe an
		// event that didn't durably land on the stream.
		RePublish: &jetstream.RePublish{
			Source:      "server.>",
			Destination: "live.server.>",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create SERVER_EVENTS stream: %w", err)
	}

	// EVT — the event-sourcing log (ADR-033/034). Coexists with
	// SERVER_EVENTS during the per-aggregate migration (ADR-035).
	// Subjects are evt.{aggregateType}.{aggregateId}; live.evt.> is
	// the republish target so projections and live subscribers consume
	// from a single NATS Core path.
	//
	// We deliberately do NOT nest under server.> here — the legacy
	// SERVER_EVENTS stream claims server.> as its subject root, and NATS
	// won't allow two streams to share an overlapping subject space.
	// During the migration window we keep them in separate top-level
	// roots; once SERVER_EVENTS is decommissioned we can revisit the
	// naming if we want to consolidate.
	serverEvtStream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:        "EVT",
		Description: "Event-sourcing log (ADR-033)",
		Subjects:    []string{"evt.>"},
		Storage:     jetstream.FileStorage,
		Compression: jetstream.S2Compression,
		Replicas:    cfg.Replicas,
		// AllowAtomicPublish gates the Nats-Batch-Id / Nats-Batch-Commit
		// protocol on this stream. Used by Publisher.AppendBatch to
		// land multi-aggregate cascades (MoveRoomToGroup, DM creation)
		// adjacently in stream order so projections never observe an
		// intermediate state that breaks an invariant.
		AllowAtomicPublish: true,
		RePublish: &jetstream.RePublish{
			Source:      "evt.>",
			Destination: "live.evt.>",
		},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create EVT stream: %w", err)
	}

	// Initialize auth tokens KV bucket with configurable TTL
	// Stores opaque bearer tokens for cross-origin API authentication.
	// NATS TTL handles automatic token expiry.
	authTokenTTL := cfg.AuthTokenTTL
	if authTokenTTL == 0 {
		authTokenTTL = 90 * 24 * time.Hour // Default 90 days
	}
	authTokensKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:         "AUTH_TOKENS",
		Description:    "Bearer tokens and OAuth authorization codes",
		Storage:        jetstream.FileStorage,
		TTL:            authTokenTTL,
		Replicas:       cfg.Replicas,
		LimitMarkerTTL: time.Minute, // Required for per-key TTL (used by authorization codes)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AUTH_TOKENS KV bucket: %w", err)
	}

	// Return initialized storage and whether RBAC bucket was newly created
	return &storage{
		serverKV:           serverKV,
		serverStore:        serverStore,
		encryptionKV:       encryptionKV,
		runtimeConfigKV:    runtimeConfigKV,
		runtimeStateKV:     runtimeStateKV,
		serverConfigKV:     serverConfigKV,
		serverRBACKV:       serverRBACKV,
		serverRuntimeKV:    serverRuntimeKV,
		serverBodiesKV:     serverBodiesKV,
		serverReactionsKV:  serverReactionsKV,
		serverThreadsKV:    serverThreadsKV,
		serverAttachments:  serverAttachments,
		serverEventsStream: serverEventsStream,
		serverEvtStream:    serverEvtStream,
		// serverRBACEngine is constructed below (after the storage value
		// exists) and assigned in NewChattoCore.
		presenceKV:      presenceKV,
		imageCacheStore: imageCacheStore,
		callStateKV:     callStateKV,
		authTokensKV:    authTokensKV,
	}, nil
}

// ============================================================================
// KV Key Helpers
// ============================================================================

// These helper functions format keys for NATS KV bucket entries. They stay in
// the core package since they're only used here and are integral to how core
// interacts with storage.

// userKey returns the KV key for a user record.
func userKey(userID string) string {
	return fmt.Sprintf("user.%s", userID)
}

// userByLoginKey returns the KV key for a login-to-userID index entry.
// Login names are lowercase to ensure case-insensitive lookups.
func userByLoginKey(login string) string {
	return fmt.Sprintf("user_by_login.%s", strings.ToLower(login))
}

// userAuthPasswordKey returns the KV key for a user's password hash.
// This follows the pattern auth.{userId}.{method}.{field} for future extensibility.
func userAuthPasswordKey(userID string) string {
	return fmt.Sprintf("auth.%s.password", userID)
}

// userAvatarKey returns the KV key for a user's avatar asset reference.
// Avatar assets are stored separately from user profile to avoid overwriting
// the entire user record when the avatar changes.
func userAvatarKey(userID string) string {
	return fmt.Sprintf("user.%s.avatar", userID)
}

// roomKey returns the KV key for a room record in a space bucket.
// Pattern: `room.{kind}.{roomID}` where kind is "channel" or "dm".
func roomKey(kind RoomKind, roomID string) string {
	return fmt.Sprintf("room.%s.%s", kind, roomID)
}

// roomKeyPrefix returns the key prefix for listing all rooms of a given
// kind in a CONFIG bucket. Pattern: `room.{kind}.*`.
func roomKeyPrefix(kind RoomKind) string {
	return fmt.Sprintf("room.%s.*", kind)
}

// roomNameIndexKey returns the KV key that claims a room name within a space.
// Names are lowercased and trimmed so the claim is case-insensitive. The value
// stored at this key is the room ID, which lets us recover from partial failures
// (a stale claim whose room never got written can be reclaimed by the same room
// trying again).
func roomNameIndexKey(name string) string {
	return fmt.Sprintf("room_name_index.%s", strings.ToLower(strings.TrimSpace(name)))
}

// messageBodyKey returns the KV key for a message body in a bodies bucket.
// The key format is {userID}.{bodyID} to enable efficient prefix-based filtering
// when deleting all message bodies for a specific user.
func messageBodyKey(userID, messageBodyID string) string {
	return userID + "." + messageBodyID
}

// eventIDFromBodyKey extracts the event ID portion from a message body key.
// Body keys have the format {userId}.{eventId}.
func eventIDFromBodyKey(bodyKey string) string {
	if idx := strings.IndexByte(bodyKey, '.'); idx >= 0 && idx < len(bodyKey)-1 {
		return bodyKey[idx+1:]
	}
	return bodyKey
}

// ============================================================================
// Event Publishing Helpers
// ============================================================================

// natsPublishFlushTimeout bounds how long a fire-and-forget publish will wait
// for the NATS server to acknowledge buffered bytes. Without a timeout, a
// hung server (e.g. network partition) would block the calling goroutine
// indefinitely instead of surfacing as a normal error.
const natsPublishFlushTimeout = 5 * time.Second

// publishServerEvent publishes a SpaceEvent to NATS via the provided subject.
// Streams automatically capture events based on their subject filters.
// Uses NATS Core publish (fire-and-forget) rather than JetStream publish (which waits for acks).
// Handles marshaling internally for consistent error handling.
func (c *ChattoCore) publishServerEvent(_ context.Context, subject string, event *corev1.Event) error {
	if err := validateEvent(event); err != nil {
		return err
	}

	eventData, err := proto.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	err = c.nc.Publish(subject, eventData)
	if err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}

	// Flush to ensure message is sent to server immediately
	// This is important for stream capture and republishing to work correctly
	err = c.nc.FlushTimeout(natsPublishFlushTimeout)
	if err != nil {
		return fmt.Errorf("failed to flush connection: %w", err)
	}

	return nil
}

// publishLiveServerEvent publishes a SpaceEvent directly to a live.> subject, bypassing JetStream storage.
// Use this for transient space-scoped notifications that don't need to be stored or replayed.
// The subject should already include the "live." prefix.
func (c *ChattoCore) publishLiveServerEvent(_ context.Context, subject string, event *corev1.Event) error {
	if err := validateEvent(event); err != nil {
		return err
	}

	eventData, err := proto.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal live event: %w", err)
	}

	if err := c.nc.Publish(subject, eventData); err != nil {
		return fmt.Errorf("failed to publish live event to %s: %w", subject, err)
	}

	if err := c.nc.FlushTimeout(natsPublishFlushTimeout); err != nil {
		return fmt.Errorf("failed to flush live event to %s: %w", subject, err)
	}
	return nil
}

// publishLiveEvent publishes a LiveEvent directly to a live.server.> subject,
// bypassing JetStream storage. Use this for deployment-wide notifications
// (user events, space lifecycle, config updates). The subject should already
// include the "live.server." prefix.
func (c *ChattoCore) publishLiveEvent(_ context.Context, subject string, event *corev1.Event) error {
	if err := validateEvent(event); err != nil {
		return err
	}

	eventData, err := proto.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal live event: %w", err)
	}

	if err := c.nc.Publish(subject, eventData); err != nil {
		return fmt.Errorf("failed to publish live event to %s: %w", subject, err)
	}

	if err := c.nc.FlushTimeout(natsPublishFlushTimeout); err != nil {
		return fmt.Errorf("failed to flush live event to %s: %w", subject, err)
	}
	return nil
}

// publishServerEventWithAck publishes a SpaceEvent using JetStream and returns the sequence ID.
// This uses synchronous JetStream publish (waits for ack) to get the sequence ID from the PubAck.
// Use this when you need to know the sequence ID immediately (e.g., for message body storage).
func (c *ChattoCore) publishServerEventWithAck(ctx context.Context, subject string, event *corev1.Event) (uint64, error) {
	if err := validateEvent(event); err != nil {
		return 0, err
	}

	eventData, err := proto.Marshal(event)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal event: %w", err)
	}

	ack, err := c.js.Publish(ctx, subject, eventData)
	if err != nil {
		return 0, fmt.Errorf("failed to publish event: %w", err)
	}

	return ack.Sequence, nil
}

const maxOCCRetries = 5

// publishServerEventWithOCC publishes a SpaceEvent to a space stream using Optimistic Concurrency Control.
// It uses the Nats-Expected-Last-Subject-Sequence header to ensure that:
// 1. We know the current state of the subject before publishing
// 2. Concurrent publishes to the same subject are detected and retried
//
// This provides reliable message posting that handles race conditions gracefully.
// The function retries up to 5 times on sequence mismatch errors with exponential backoff.
func (c *ChattoCore) publishServerEventWithOCC(ctx context.Context, subject string, event *corev1.Event) (uint64, error) {
	if err := validateEvent(event); err != nil {
		return 0, err
	}

	eventData, err := proto.Marshal(event)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal event: %w", err)
	}

	stream := c.storage.serverEventsStream

	var lastErr error
	for attempt := 1; attempt <= maxOCCRetries; attempt++ {
		// Get the current last sequence for this subject
		var expectedSeq uint64
		msg, err := stream.GetLastMsgForSubject(ctx, subject)
		if err != nil {
			if !errors.Is(err, jetstream.ErrMsgNotFound) {
				return 0, fmt.Errorf("failed to get last message for subject: %w", err)
			}
			// No messages yet for this subject - expect sequence 0
			expectedSeq = 0
		} else {
			expectedSeq = msg.Sequence
		}

		// Publish with expected last subject sequence
		ack, err := c.js.Publish(ctx, subject, eventData,
			jetstream.WithExpectLastSequencePerSubject(expectedSeq))
		if err == nil {
			return ack.Sequence, nil
		}

		// Check if this is a sequence mismatch error (concurrent publish)
		var jsErr *jetstream.APIError
		if errors.As(err, &jsErr) && jsErr.ErrorCode == jetstream.JSErrCodeStreamWrongLastSequence {
			c.logger.Debug("Sequence mismatch, retrying publish",
				"subject", subject,
				"expected_seq", expectedSeq,
				"attempt", attempt,
				"max_attempts", maxOCCRetries)
			lastErr = err

			// Exponential backoff with jitter to avoid thundering herd
			// Base delay: 1ms, 2ms, 4ms, 8ms, 16ms for attempts 1-5
			// Plus random jitter of 0-5ms to spread out concurrent retries
			baseDelay := time.Duration(1<<(attempt-1)) * time.Millisecond
			jitter := time.Duration(rand.Int63n(int64(5 * time.Millisecond)))
			select {
			case <-ctx.Done():
				return 0, ctx.Err()
			case <-time.After(baseDelay + jitter):
			}
			continue
		}

		// For any other error, fail immediately
		return 0, fmt.Errorf("failed to publish event: %w", err)
	}

	return 0, fmt.Errorf("failed to publish event after %d attempts due to concurrent modifications: %w", maxOCCRetries, lastErr)
}

func validateEvent(event *corev1.Event) error {
	if event == nil || event.Event == nil {
		return fmt.Errorf("%w: event payload is nil or oneof field is unset", ErrInvalidEvent)
	}
	return nil
}

// newEvent fills in the Id, ActorID, and CreatedAt fields of an Event
// envelope if they're not already set. The caller provides the event
// with the concrete oneof variant already populated.
func newEvent(actorID string, event *corev1.Event) *corev1.Event {
	if event.Id == "" {
		event.Id = NewEventID()
	}
	if event.ActorId == "" {
		event.ActorId = actorID
	}
	if event.CreatedAt == nil {
		event.CreatedAt = timestamppb.New(time.Now())
	}
	return event
}

// ============================================================================
// Stream Management
// ============================================================================

// createSpaceResources is now a no-op: all data lives in the deployment-wide
// SERVER_* buckets (eager-created in newStorage). Kept as a stub so callers
// don't have to be edited until the broader Space-retirement pass.
func (c *ChattoCore) createSpaceResources(_ context.Context, _ string) error {
	return nil
}

// purgeRoomEvents removes all events for a specific room from the server stream.
// This is called when a room is deleted to clean up the room's event history.
func (c *ChattoCore) purgeRoomEvents(ctx context.Context, kind RoomKind, roomID string) error {
	stream := c.storage.serverEventsStream

	// Purge all events matching the room's subject pattern
	subjectFilter := subjects.RoomAllEvents(string(kind), roomID)
	if err := stream.Purge(ctx, jetstream.WithPurgeSubject(subjectFilter)); err != nil {
		return fmt.Errorf("failed to purge room events for %s (subject: %s): %w", roomID, subjectFilter, err)
	}

	c.logger.Debug("Purged room events from server stream", "kind", kind, "room_id", roomID, "subject_filter", subjectFilter)

	return nil
}

// ============================================================================
// Event Streaming
// ============================================================================

// isTerminalIteratorError returns true if the error indicates the iterator
// cannot be recovered (connection closed, consumer deleted, etc.).
// Recoverable errors (heartbeat missed, leadership changed) return false.
func isTerminalIteratorError(err error) bool {
	if err == nil {
		return false
	}
	// Terminal errors - cannot recover, must stop
	if errors.Is(err, jetstream.ErrMsgIteratorClosed) ||
		errors.Is(err, jetstream.ErrConnectionClosed) ||
		errors.Is(err, jetstream.ErrServerShutdown) ||
		errors.Is(err, jetstream.ErrConsumerDeleted) {
		return true
	}
	return false
}

// StreamMyEvents creates a unified stream of every event on this
// deployment that is relevant to a specific user.
//
// All events arrive via a single NATS Core subscription on
// `live.server.>`. JetStream-stored events (room messages, thread
// replies, meta lifecycle, server-level member events) are republished
// onto the same subject root by the SERVER_EVENTS stream's RePublish
// config; transient events (reactions, typing, edits, deletes, user/
// space/config notifications) publish directly via NATS Core. The
// subject prefix is what disambiguates the two — payload-wise they're
// identical `corev1.Event` wire protos.
//
// Authorization:
//   - Room events (live.server.room.>) are delivered only for rooms
//     where the user is a member. The membership set is pre-loaded
//     across both kinds (channel + dm) and updated as join/leave/
//     room-deleted events arrive.
//   - DM-kind events are additionally gated on `dm.view`.
//   - User/config/member subjects are filtered by
//     isAuthorizedForLiveEvent.
//   - Presence updates from the per-process PresenceHub are deployment-
//     wide; the hub dedups status flapping.
//
// The subscription also tracks presence liveness: subscribing implies
// the user is online, and a ticker refreshes the KV TTL while the
// connection lives. A synthetic Heartbeat is emitted every 25s so
// clients can detect a dead subscription on an otherwise-healthy
// WebSocket.
//
// The returned channel closes when the context is cancelled or when a
// SessionTerminatedEvent is delivered to the user.
func (c *ChattoCore) StreamMyEvents(ctx context.Context, userID string) (<-chan *corev1.Event, error) {
	canDM, err := c.HasServerPermission(ctx, userID, PermDMView)
	if err != nil {
		return nil, fmt.Errorf("failed to check dm.view permission: %w", err)
	}

	// memberRooms is the per-subscription visibility cache: the user
	// receives live events for rooms they are an explicit member of.
	// Seeded from `room_membership.*` records and mutated on
	// `UserJoinedRoom` / `UserLeftRoom` / `RoomDeleted`, and re-seeded
	// on `RoomGroupsUpdated` to absorb admin-driven membership changes.
	memberRooms := make(map[string]struct{})
	if err := c.populateMemberRoomsCache(ctx, userID, canDM, memberRooms); err != nil {
		return nil, err
	}

	// Single live-subject subscription on live.server.>. Aggregates
	// that have already been migrated to EVT (room memberships, room
	// groups, server config, room/thread messages from #614) still
	// emit legacy mirrors on the live.server.> subject family, the
	// same way RoomMembership and friends have done since #595. We
	// deliberately do NOT subscribe to live.evt.> here even though the
	// EVT stream's RePublish puts events on that subject root —
	// subscribing to both would deliver every migrated event twice
	// (once from each subject), flood the per-subscription channel
	// under load, and tip subscriptions into SlowConsumer state.
	//
	// The 256-message buffer absorbs reaction/typing bursts; on
	// overflow NATS Core drops messages and transitions the
	// subscription to SlowConsumer state — slowConsumerCh below
	// catches that and tears the resolver down so the client can
	// re-subscribe (and pick up missed history via the GraphQL
	// catch-up path) rather than silently miss events.
	msgChan := make(chan *nats.Msg, 256)
	liveSub, err := c.nc.ChanSubscribe(subjects.LiveAllEvents(), msgChan)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to live events: %w", err)
	}
	slowConsumerCh := liveSub.StatusChanged(nats.SubscriptionSlowConsumer)

	presenceSub, err := c.PresenceHub.Subscribe(ctx)
	if err != nil {
		liveSub.Unsubscribe()
		return nil, fmt.Errorf("failed to subscribe to presence hub: %w", err)
	}

	eventChan := make(chan *corev1.Event)

	go func() {
		c.logger.Debug("Server event stream started", "user_id", userID, "can_dm", canDM, "member_rooms", len(memberRooms))

		// Subscribing implies the user is online; refresh on a ticker
		// so the KV TTL doesn't expire while the connection is open.
		if err := c.SetPresence(ctx, userID, PresenceStatusOnline); err != nil {
			c.logger.Warn("Failed to set initial presence", "error", err, "user_id", userID)
		}
		presenceTicker := time.NewTicker(PresenceRefreshInterval)
		defer presenceTicker.Stop()

		heartbeatTicker := time.NewTicker(25 * time.Second)
		defer heartbeatTicker.Stop()

		lastKnownPresence := make(map[string]string, len(presenceSub.Snapshot))
		for k, v := range presenceSub.Snapshot {
			lastKnownPresence[k] = v
		}

		defer func() {
			c.logger.Debug("Server event stream closed", "user_id", userID)
			liveSub.Unsubscribe()
			c.PresenceHub.Unsubscribe(presenceSub)
			close(eventChan)
		}()

		send := func(event *corev1.Event) bool {
			select {
			case <-ctx.Done():
				return false
			case eventChan <- event:
				return true
			}
		}

		for {
			select {
			case <-ctx.Done():
				return

			case <-slowConsumerCh:
				// The NATS Core subscription's buffer overflowed and
				// messages were dropped. Continuing would silently
				// hide missing events, so tear down — the client's
				// eventBus watchdog will re-subscribe (and any UI
				// state that depends on missed messages will be
				// repaired via the usual GraphQL refetch paths).
				dropped, _ := liveSub.Dropped()
				c.logger.Warn("Slow consumer on live events subscription — tearing down",
					"user_id", userID, "dropped", dropped)
				return

			case <-presenceTicker.C:
				if err := c.refreshPresence(ctx, userID); err != nil {
					c.logger.Warn("Failed to refresh presence", "error", err, "user_id", userID)
				}

			case <-heartbeatTicker.C:
				if !send(&corev1.Event{
					Id:        NewEventID(),
					CreatedAt: timestamppb.Now(),
					Event:     &corev1.Event_Heartbeat{Heartbeat: &corev1.HeartbeatEvent{}},
				}) {
					return
				}

			case msg := <-msgChan:
				event, ok := c.filterLiveEvent(ctx, userID, canDM, memberRooms, msg)
				if !ok {
					continue
				}
				if !send(event) {
					return
				}
				// Session termination tears down the subscription.
				// The frontend handles logout on receipt; closing
				// the channel ensures the server tears down too.
				if event.GetSessionTerminated() != nil {
					c.logger.Info("Session terminated - closing event stream", "user_id", userID)
					return
				}

			case update := <-presenceSub.C:
				if last, exists := lastKnownPresence[update.UserID]; exists && last == update.Status {
					continue
				}
				if update.Status == PresenceStatusOffline {
					delete(lastKnownPresence, update.UserID)
				} else {
					lastKnownPresence[update.UserID] = update.Status
				}
				if !send(&corev1.Event{
					CreatedAt: timestamppb.Now(),
					ActorId:   update.UserID,
					Event: &corev1.Event_PresenceChanged{
						PresenceChanged: &corev1.PresenceChangedEvent{Status: update.Status},
					},
				}) {
					return
				}
			}
		}
	}()

	return eventChan, nil
}

// populateMemberRoomsCache (re)builds the per-subscription room
// visibility set in place. The cache contains every channel room the
// user is an explicit member of, plus DM rooms when canDM. Used at
// subscription start and on `RoomGroupsUpdatedEvent` to re-seed after
// admin-driven membership changes (e.g. a user gaining access to a
// room via a group-scope permission edit, then joining).
func (c *ChattoCore) populateMemberRoomsCache(ctx context.Context, userID string, canDM bool, memberRooms map[string]struct{}) error {
	for k := range memberRooms {
		delete(memberRooms, k)
	}

	// Explicit channel memberships. Membership alone qualifies — a user
	// who has joined the room receives its live events regardless of
	// whether they could re-join today (e.g. they joined while the room
	// was open, then `room.join` was denied for everyone). The
	// "leave the room" mutation is the only way to lose live events.
	channelMemberships, err := c.GetUserRoomMemberships(ctx, KindChannel, userID)
	if err != nil {
		return fmt.Errorf("failed to get channel room memberships: %w", err)
	}
	for _, m := range channelMemberships {
		memberRooms[m.RoomId] = struct{}{}
	}

	if canDM {
		dmMemberships, err := c.GetUserRoomMemberships(ctx, KindDM, userID)
		if err != nil {
			return fmt.Errorf("failed to get DM room memberships: %w", err)
		}
		// DM rooms surface via their own listing path; explicit
		// membership is the visibility gate.
		for _, m := range dmMemberships {
			memberRooms[m.RoomId] = struct{}{}
		}
	}

	return nil
}

// filterLiveEvent unmarshals a message from the unified live.> stream
// and applies per-user authorization. Returns the event and true if it
// should be delivered. Mutates memberRooms when the subscriber
// themselves joins/leaves a room or when a room is deleted.
//
// Two routing paths:
//
//  1. Room subjects (live.server.room.{kind}.{roomId}.…):
//     gated on room membership and (for DM-kind) dm.view permission.
//  2. Everything else: delegated to isAuthorizedForLiveEvent.
func (c *ChattoCore) filterLiveEvent(ctx context.Context, userID string, canDM bool, memberRooms map[string]struct{}, msg *nats.Msg) (*corev1.Event, bool) {
	var event corev1.Event
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		c.logger.Warn("Failed to unmarshal live event", "subject", msg.Subject, "error", err)
		return nil, false
	}

	// Path 1: room-scoped events on live.server.room.{kind}.{roomId}.…
	// (both the legacy SERVER_EVENTS republish and ephemeral direct
	// publishes from publishLiveServerEvent). EVT events that need to
	// reach the frontend are mirrored to this same subject family by
	// the producer (see room_membership.go, messages.go, etc.), so
	// only one subject shape arrives here.
	if kind := subjects.ParseKindFromRoomSubject(msg.Subject); kind != "" {
		roomKind := kind
		roomID := subjects.ParseRoomIDFromSubject(msg.Subject)
		if !canDM && roomKind == string(KindDM) {
			return nil, false
		}
		if roomID == "" {
			return nil, false
		}

		// Capture membership before mutating the cache so transition
		// events (self-leave, room-deleted) still reach the member who
		// is transitioning out.
		_, isMember := memberRooms[roomID]

		switch event.Event.(type) {
		case *corev1.Event_UserJoinedRoom:
			if event.ActorId == userID {
				// Membership is the gate: once the user has joined, they
				// receive the room's live events. Visibility is handled
				// upstream by the join action itself; if the user wasn't
				// allowed to join, this event wouldn't have been published.
				memberRooms[roomID] = struct{}{}
				isMember = true
			}
		case *corev1.Event_UserLeftRoom:
			if event.ActorId == userID {
				delete(memberRooms, roomID)
			}
		case *corev1.Event_RoomDeleted:
			delete(memberRooms, roomID)
		}

		// Skip own typing events — the sender doesn't need to see them.
		// Critical for multi-server clients where the frontend's
		// currentUserId may differ from the remote server user ID.
		if event.GetUserTyping() != nil && event.ActorId == userID {
			return nil, false
		}

		if !isMember {
			return nil, false
		}
		return &event, true
	}

	// Path 2: user/config/member subjects.
	if !c.isAuthorizedForLiveEvent(ctx, userID, msg.Subject) {
		return nil, false
	}

	return &event, true
}

// isAuthorizedForLiveEvent checks if a user is authorized to receive a
// non-room live event based on the subject pattern:
//
//   - live.server.config.* → all authenticated users (server config /
//     branding / room-layout updates — public to every member)
//   - live.server.member.* → all authenticated users (single-server membership)
//   - live.server.user.{userId}.* → only the target user, except
//     live.server.user.{userId}.profile_updated which is broadcast.
//
// Room events (`live.server.room.>`) are filtered separately via the
// per-user room-membership cache and never reach this function.
func (c *ChattoCore) isAuthorizedForLiveEvent(_ context.Context, userID, subject string) bool {
	parts := strings.Split(subject, ".")
	if len(parts) < 3 || parts[0] != "live" || parts[1] != "server" {
		c.logger.Warn("Invalid live event subject format", "subject", subject)
		return false
	}

	switch parts[2] {
	case "config", "member":
		return true
	case "user":
		if len(parts) < 5 {
			c.logger.Warn("Invalid user-scoped live event subject", "subject", subject)
			return false
		}
		if parts[4] == "profile_updated" {
			return true
		}
		return parts[3] == userID
	case "room":
		c.logger.Warn("Room subject reached isAuthorizedForLiveEvent — should be filtered upstream", "subject", subject)
		return false
	default:
		c.logger.Warn("Unknown live event scope", "scope", parts[2], "subject", subject)
		return false
	}
}

// PublishServerConfigUpdated publishes an server config update event.
// This notifies all connected clients that the server configuration has changed.
func (c *ChattoCore) PublishServerConfigUpdated(ctx context.Context, actorID string, serverName, motd, welcomeMessage, blockedUsernames string) error {
	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_ConfigUpdated{
			ConfigUpdated: &corev1.ServerConfigUpdatedEvent{
				ServerName:       serverName,
				Motd:             motd,
				WelcomeMessage:   welcomeMessage,
				BlockedUsernames: blockedUsernames,
			},
		},
	})

	return c.publishLiveEvent(ctx, subjects.LiveConfigEvent("updated"), event)
}

// ============================================================================
// Statistics
// ============================================================================

// ServerStats contains aggregate counts surfaced in the admin dashboard.
type ServerStats struct {
	UserCount        int
	ChannelRoomCount int
	DMRoomCount      int
}

// GetStats returns deployment-level counts: registered users, channel rooms,
// DM rooms. Per-space breakdowns went away with the Space tier (ADR-030).
func (c *ChattoCore) GetStats(ctx context.Context) (*ServerStats, error) {
	stats := &ServerStats{}

	userKeys, err := c.storage.serverKV.ListKeysFiltered(ctx, "user.*")
	if err != nil {
		return nil, fmt.Errorf("failed to list user keys: %w", err)
	}
	for range userKeys.Keys() {
		stats.UserCount++
	}

	channelRooms, err := c.ListRooms(ctx, KindChannel)
	if err != nil {
		return nil, fmt.Errorf("failed to list channel rooms: %w", err)
	}
	stats.ChannelRoomCount = len(channelRooms)

	dmRooms, err := c.ListRooms(ctx, KindDM)
	if err != nil {
		return nil, fmt.Errorf("failed to list dm rooms: %w", err)
	}
	stats.DMRoomCount = len(dmRooms)

	return stats, nil
}

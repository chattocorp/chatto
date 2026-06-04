package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
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
	"hmans.de/chatto/internal/dekstore"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/kms"
	"hmans.de/chatto/internal/migrations"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// ChattoCore
// ============================================================================

// ChattoCore is the central hub for all Chatto operations.
// It provides a unified API for spaces, users, rooms, and messages,
// managing current JetStream resources and legacy import handles internally.
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

	// OnVideoProcessingRequested starts best-effort local video processing for
	// an already-declared message-owned asset. The video service registers this
	// callback when enabled; a future durable task queue should replace this
	// process-local handoff.
	OnVideoProcessingRequested func(ctx context.Context, assetID, messageEventID string) error

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

	// ServerConfig is the projection holding current dynamic configuration
	// rebuilt from EVT. The field name is retained for compatibility with
	// existing admin/verification code while the projection now stores more
	// than the old server-config snapshot.
	ServerConfig *ConfigProjection

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

	// ContentKeys holds wrapped per-user DEK epochs used by encrypted
	// message bodies and durable user PII.
	ContentKeys *ContentKeyProjection

	// ContentKeysProjector runs the consumer for ContentKeys. Exposed for
	// WaitForSeq from encryption writers.
	ContentKeysProjector *events.Projector

	// RBAC holds current role, assignment, and permission state derived
	// from durable RBAC aggregate events.
	RBAC *RBACProjection

	// RBACProjector runs the consumer for RBAC. Exposed for WaitForSeq
	// from role and permission writers.
	RBACProjector *events.Projector

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

// Run starts every background service owned by the core — currently
// PresenceHub and every registered projector — and blocks until ctx is
// cancelled or any service returns an error. Returns the first error
// observed (or ctx.Err on shutdown).
//
// Call this once per process from an errgroup goroutine; tests typically
// launch it in a bare goroutine with a per-test context that cleanup
// cancels. Background services are not designed to be restarted.
//
// New projectors should be registered during NewChattoCore; they are then
// started automatically here without any additional wiring.
func (c *ChattoCore) Run(ctx context.Context) error {
	g, gctx := errgroup.WithContext(ctx)

	for _, projection := range c.projections {
		projector := projection.projector
		g.Go(func() error { return projector.Run(gctx) })
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
		// Before issuing boot-time "ensure" mutations, let every
		// projection replay the durable stream as it exists now. A
		// started-but-cold projection would otherwise look empty and
		// append duplicate seed facts on every process restart.
		if err := c.WaitForProjectionsCurrent(gctx); err != nil {
			return fmt.Errorf("wait for projections current: %w", err)
		}
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
		// running and caught up because it reads the RoomGroups
		// projection and depends on WaitForSeq actually waiting.
		if err := c.ensureChannelRoomsAreInAGroup(gctx); err != nil {
			return fmt.Errorf("ensure channel rooms in a group: %w", err)
		}
		if c.config.ESBootVerify {
			if err := c.WaitForProjectionsCurrent(gctx); err != nil {
				return fmt.Errorf("wait for projections current: %w", err)
			}
			if err := c.logESBootVerification(gctx); err != nil {
				if c.config.ESBootVerifyStrict {
					return err
				}
				c.logger.Warn("ES boot verification reported problems", "error", err)
			}
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
	keyWrapper  kms.KeyWrapper
	legacyKeys  kms.LegacyKeyProvider
	contentKeys *dekstore.Store
}

func (c *ChattoCore) ServerStore() jetstream.ObjectStore {
	return c.storage.serverStore
}

// KeyWrapper returns the key-only KMS boundary used by encryption operations.
func (c *ChattoCore) KeyWrapper() kms.KeyWrapper {
	return c.encryption.keyWrapper
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
	return c.DeleteUserEncryptionKeyAs(ctx, userID, userID)
}

func (c *ChattoCore) deleteEncryptionKeyOnly(ctx context.Context, keyRef string) error {
	if c.encryption.keyWrapper == nil {
		return nil
	}
	return c.encryption.keyWrapper.ShredKey(ctx, keyRef)
}

func (c *ChattoCore) DeleteUserEncryptionKeyAs(ctx context.Context, actorID, userID string) error {
	if c.encryption.keyWrapper == nil {
		return nil // Encryption not configured
	}

	if err := c.waitForUserContentKeysCurrent(ctx, userID); err != nil {
		return err
	}

	contentKeyRefs := c.ContentKeys.ContentKeyRefs(userID)
	keyRefs := make(map[string]struct{})
	keyRefs[kms.LegacyUserKeyRef(userID)] = struct{}{}
	for _, keyRef := range c.ContentKeys.KeyRefs(userID) {
		if keyRef != "" {
			keyRefs[keyRef] = struct{}{}
		}
	}
	for _, contentKeyRef := range contentKeyRefs {
		if c.encryption.contentKeys == nil {
			return fmt.Errorf("content key store is not configured")
		}
		stored, err := c.encryption.contentKeys.Get(ctx, contentKeyRef)
		if err != nil {
			return fmt.Errorf("failed to load DEK %s before shredding: %w", contentKeyRef, err)
		}
		if wrappingKeyRef := stored.GetWrappingKeyRef(); wrappingKeyRef != "" {
			keyRefs[wrappingKeyRef] = struct{}{}
		}
	}

	shredded := false
	for _, contentKeyRef := range contentKeyRefs {
		if err := c.encryption.contentKeys.Shred(ctx, contentKeyRef); err != nil {
			return err
		}
		shredded = true
	}

	for keyRef := range keyRefs {
		exists, err := c.encryption.keyWrapper.KeyExists(ctx, keyRef)
		if err != nil {
			return err
		}
		if !exists {
			continue
		}
		if err := c.encryption.keyWrapper.ShredKey(ctx, keyRef); err != nil {
			return err
		}
		shredded = true
	}
	if !shredded {
		return nil
	}

	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_UserKeyShredded{
			UserKeyShredded: &corev1.UserKeyShreddedEvent{UserId: userID},
		},
	})
	seq, err := c.appendUserEvent(ctx, userID, event, "", nil)
	if err != nil {
		return fmt.Errorf("failed to record user key shred event: %w", err)
	}
	return waitForSeqAll(ctx, seq,
		waitForProjection("room timeline", c.RoomTimelineProjector),
		waitForProjection("thread", c.ThreadsProjector),
	)
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
func (c *ChattoCore) CleanupAsset(ctx context.Context, asset *corev1.DeprecatedAsset) {
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
func (c *ChattoCore) deleteAsset(ctx context.Context, asset *corev1.DeprecatedAsset, assetType, ownerID string) {
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

// Ready checks if the core is fully initialized and current persistent resources are accessible.
// Returns nil if ready, or an error describing what's not ready.
// Used by the /readyz endpoint to verify the server can handle requests.
func (c *ChattoCore) Ready(ctx context.Context) error {
	if _, err := c.storage.runtimeStateKV.Status(ctx); err != nil {
		return fmt.Errorf("RUNTIME_STATE not ready: %w", err)
	}
	if _, err := c.storage.serverEvtStream.Info(ctx); err != nil {
		return fmt.Errorf("EVT not ready: %w", err)
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

	// Initialize storage (current resources plus optional legacy import sources).
	storage, err := newStorage(js, ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Initialize encryption manager
	builtinKMS := kms.NewBuiltin(storage.encryptionKV, logger.WithPrefix("core.kms"))
	encMgr := &encryptionManager{
		keyWrapper:  builtinKMS,
		legacyKeys:  builtinKMS,
		contentKeys: dekstore.New(storage.encryptionKV, logger.WithPrefix("core.dekstore")),
	}

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

	// newProjector wraps projection construction into one registration
	// record. Runtime lifecycle and admin diagnostics both consume this
	// same list, so adding a projection has a single wiring point.
	var projections []projectionRegistration
	newProjector := func(p events.Projection, name string, estimate func() (int64, int64, []ProjectionAdminMetric)) *events.Projector {
		loggerName := strings.ReplaceAll(name, " ", "") + "Projector"
		pr := events.NewProjector(js, storage.serverEvtStream, p, logger.WithPrefix("core."+loggerName))
		projections = append(projections, projectionRegistration{
			name:      name,
			projector: pr,
			estimate:  estimate,
		})
		return pr
	}

	roomMembership := NewRoomMembershipProjection()
	roomMembershipProjector := newProjector(roomMembership, "Room Membership", roomMembership.adminProjectionEstimate)

	serverConfigProjection := NewConfigProjection()
	serverConfigProjector := newProjector(serverConfigProjection, "Server Config", serverConfigProjection.adminProjectionEstimate)

	roomCatalog := NewRoomCatalogProjection()
	roomCatalogProjector := newProjector(roomCatalog, "Room Catalog", roomCatalog.adminProjectionEstimate)

	roomGroups := NewRoomGroupProjection()
	roomGroupsProjector := newProjector(roomGroups, "Room Groups", roomGroups.adminProjectionEstimate)

	roomLayout := NewRoomLayoutProjection()
	roomLayoutProjector := newProjector(roomLayout, "Room Layout", roomLayout.adminProjectionEstimate)

	// Per-room event-log + per-thread event-log projections (#597
	// phase 2). Both consume the full evt.room.> firehose; resolvers
	// do all filtering and rendering at query time. v1 shape — we
	// iterate significantly on this once we observe read patterns.
	roomTimeline := NewRoomTimelineProjection()
	roomTimelineProjector := newProjector(roomTimeline, "Room Timeline", roomTimeline.adminProjectionEstimate)

	threads := NewThreadProjection()
	threadsProjector := newProjector(threads, "Threads", threads.adminProjectionEstimate)

	reactions := NewReactionProjection()
	reactionsProjector := newProjector(reactions, "Reactions", reactions.adminProjectionEstimate)

	users := NewUserProjection(encMgr.keyWrapper, encMgr.contentKeys)
	usersProjector := newProjector(users, "Users", users.adminProjectionEstimate)

	contentKeys := NewContentKeyProjection()
	contentKeysProjector := newProjector(contentKeys, "Content Keys", contentKeys.adminProjectionEstimate)

	rbac := NewRBACProjection()
	rbacProjector := newProjector(rbac, "RBAC", rbac.adminProjectionEstimate)

	configService := NewConfigService(eventPublisher, serverConfigProjector, serverConfigProjection)
	configMgr := NewConfigManager(configService, serverConfigProjection)

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
		ContentKeys:             contentKeys,
		ContentKeysProjector:    contentKeysProjector,
		RBAC:                    rbac,
		RBACProjector:           rbacProjector,
		projections:             projections,
		bootDone:                make(chan struct{}),
	}

	if err := core.migrateLegacyCallStateToMemoryCache(ctx); err != nil {
		logger.Warn("Failed to copy legacy CALL_STATE entries into MEMORY_CACHE", "error", err)
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
		encMgr.keyWrapper,
		encMgr.contentKeys,
		logger,
	); err != nil {
		return nil, fmt.Errorf("failed to run boot migrations: %w", err)
	}
	if err := core.migrateAssetCreationsToES(ctx); err != nil {
		return nil, fmt.Errorf("failed to migrate asset creations to ES: %w", err)
	}
	if err := core.migrateVideoManifestsToES(ctx); err != nil {
		return nil, fmt.Errorf("failed to migrate video manifests to ES: %w", err)
	}

	// Initialize permission resolver (must be done after core struct is created)
	core.permissionResolver = NewPermissionResolver(core)

	// Initialize link preview cache and fetcher
	core.linkPreviewCache = linkpreview.NewCache(storage.runtimeStateKV)
	assetsConfig := core.AssetsConfig()
	core.linkPreviewFetcher = linkpreview.NewFetcher(storage.serverStore, &assetsConfig, NewAssetID)

	// ensureChannelRoomsAreInAGroup is deferred to core.Run() — it
	// needs the projectors to be live so its CreateRoomGroup /
	// MoveRoomToGroup calls can actually WaitForSeq. Doing it here
	// (when projectors haven't been started yet) would leave orphan
	// rooms in any subsequent SeedDefaultRooms call.

	// Initialize presence hub (single KV watcher per process). Started
	// by core.Run alongside the projectors.
	core.PresenceHub = NewPresenceHub(storage.memoryCacheKV, logger)

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

// storage encapsulates JetStream resources used by Chatto Core. Current
// resources are provisioned at startup; legacy import resources are opened only
// when they already exist.
type storage struct {
	serverStore     jetstream.ObjectStore
	encryptionKV    jetstream.KeyValue // Encryption keys (excluded from backups)
	serverKV        jetstream.KeyValue // INSTANCE        - legacy user/config import source
	runtimeConfigKV jetstream.KeyValue // INSTANCE_CONFIG - legacy runtime configuration import source
	runtimeStateKV  jetstream.KeyValue // RUNTIME_STATE  - persisted latest-value runtime/user state

	// Legacy import resources. Fresh ES-only deployments do not create these;
	// boot importers treat nil handles as empty sources.
	serverConfigKV     jetstream.KeyValue // SERVER_CONFIG    - legacy rooms, memberships
	serverRuntimeKV    jetstream.KeyValue // SERVER_RUNTIME   - legacy sequences, timestamps, read state
	serverRBACKV       jetstream.KeyValue // SERVER_RBAC      - legacy RBAC import source
	serverBodiesKV     jetstream.KeyValue // SERVER_BODIES    - legacy message bodies + attachment metadata records
	serverReactionsKV  jetstream.KeyValue // SERVER_REACTIONS - legacy emoji reactions
	serverEventsStream jetstream.Stream   // SERVER_EVENTS    - legacy event stream

	serverAttachments jetstream.ObjectStore // SERVER_ASSETS    - message attachment binaries (#330 phase 4e)
	serverEvtStream   jetstream.Stream      // EVT       - event-sourcing log (ADR-033/034).

	memoryCacheKV     jetstream.KeyValue    // MEMORY_CACHE - volatile, memory-backed runtime cache state
	imageCacheStore   jetstream.ObjectStore // Optional: cached resized images (nil if disabled)
	legacyCallStateKV jetstream.KeyValue    // CALL_STATE - legacy active voice call import source
}

// newStorage initializes current JetStream resources and opens existing legacy
// import resources.
func newStorage(js jetstream.JetStream, ctx context.Context, cfg config.CoreConfig) (*storage, error) {
	serverKV, err := openLegacyKeyValue(ctx, js, "INSTANCE")
	if err != nil {
		return nil, fmt.Errorf("failed to open legacy INSTANCE KV bucket: %w", err)
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

	// Initialize encryption keys KV bucket (excluded from backups for security)
	// Keys are stored separately so backups contain only encrypted data, not the keys to decrypt it
	encryptionKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "ENCRYPTION_KEYS",
		Description: "User encryption keys (excluded from backups)",
		Storage:     jetstream.FileStorage,
		History:     1,
		Replicas:    cfg.Replicas,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create ENCRYPTION_KEYS KV bucket: %w", err)
	}

	runtimeConfigKV, err := openLegacyKeyValue(ctx, js, "INSTANCE_CONFIG")
	if err != nil {
		return nil, fmt.Errorf("failed to open legacy INSTANCE_CONFIG KV bucket: %w", err)
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

	memoryCacheKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:         "MEMORY_CACHE",
		Description:    "Volatile memory-backed runtime cache state",
		Storage:        jetstream.MemoryStorage,
		History:        1,
		Replicas:       cfg.Replicas,
		LimitMarkerTTL: PresenceTTL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MEMORY_CACHE KV bucket: %w", err)
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

	legacyCallStateKV, err := openLegacyKeyValue(ctx, js, "CALL_STATE")
	if err != nil {
		return nil, fmt.Errorf("failed to open legacy CALL_STATE KV bucket: %w", err)
	}

	serverConfigKV, err := openLegacyKeyValue(ctx, js, "SERVER_CONFIG")
	if err != nil {
		return nil, fmt.Errorf("failed to open legacy SERVER_CONFIG KV bucket: %w", err)
	}

	serverRBACKV, err := openLegacyKeyValue(ctx, js, "SERVER_RBAC")
	if err != nil {
		return nil, fmt.Errorf("failed to open legacy SERVER_RBAC KV bucket: %w", err)
	}

	serverRuntimeKV, err := openLegacyKeyValue(ctx, js, "SERVER_RUNTIME")
	if err != nil {
		return nil, fmt.Errorf("failed to open legacy SERVER_RUNTIME KV bucket: %w", err)
	}

	serverBodiesKV, err := openLegacyKeyValue(ctx, js, "SERVER_BODIES")
	if err != nil {
		return nil, fmt.Errorf("failed to open legacy SERVER_BODIES KV bucket: %w", err)
	}

	serverReactionsKV, err := openLegacyKeyValue(ctx, js, "SERVER_REACTIONS")
	if err != nil {
		return nil, fmt.Errorf("failed to open legacy SERVER_REACTIONS KV bucket: %w", err)
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

	serverEventsStream, err := openLegacyStream(ctx, js, "SERVER_EVENTS")
	if err != nil {
		return nil, fmt.Errorf("failed to open legacy SERVER_EVENTS stream: %w", err)
	}

	// EVT — the event-sourcing log (ADR-033/034).
	// Subjects are evt.{aggregateType}.{aggregateId}; live.evt.> is
	// the republish target so projections and live subscribers consume
	// from a single NATS Core path.
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
		serverAttachments:  serverAttachments,
		serverEventsStream: serverEventsStream,
		serverEvtStream:    serverEvtStream,
		memoryCacheKV:      memoryCacheKV,
		imageCacheStore:    imageCacheStore,
		legacyCallStateKV:  legacyCallStateKV,
	}, nil
}

func openLegacyKeyValue(ctx context.Context, js jetstream.JetStream, bucket string) (jetstream.KeyValue, error) {
	kv, err := js.KeyValue(ctx, bucket)
	if err == nil {
		return kv, nil
	}
	if errors.Is(err, jetstream.ErrBucketNotFound) {
		return nil, nil
	}
	return nil, err
}

func openLegacyStream(ctx context.Context, js jetstream.JetStream, streamName string) (jetstream.Stream, error) {
	stream, err := js.Stream(ctx, streamName)
	if err == nil {
		return stream, nil
	}
	if errors.Is(err, jetstream.ErrStreamNotFound) {
		return nil, nil
	}
	return nil, err
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

// liveEVTProjectionWaitTimeout bounds the causal barrier between JetStream's
// raw EVT republish and GraphQL delivery. In the normal case the local
// projectors have already advanced and WaitForSeq returns immediately; the
// timeout covers replica lag or a stuck projector without wedging a
// subscription goroutine forever.
const liveEVTProjectionWaitTimeout = 2 * time.Second

// publishLiveEvent publishes a transient LiveEvent directly to a live.sync.>
// subject, bypassing JetStream storage. The subject should already include
// the "live.sync." prefix.
func (c *ChattoCore) publishLiveEvent(_ context.Context, subject string, event *corev1.LiveEvent) error {
	if err := validateLiveEvent(event); err != nil {
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

func validateEvent(event *corev1.Event) error {
	if event == nil || event.Event == nil {
		return fmt.Errorf("%w: event payload is nil or oneof field is unset", ErrInvalidEvent)
	}
	return nil
}

func validateLiveEvent(event *corev1.LiveEvent) error {
	if event == nil || event.Event == nil {
		return fmt.Errorf("%w: live event payload is nil or oneof field is unset", ErrInvalidEvent)
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

// newLiveEvent fills in the Id, ActorID, and CreatedAt fields of a LiveEvent
// envelope if they're not already set. The caller provides the event with the
// concrete oneof variant already populated.
func newLiveEvent(actorID string, event *corev1.LiveEvent) *corev1.LiveEvent {
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

// createSpaceResources is now a no-op: room/user domain state lives in EVT and
// deployment-wide projections. Kept as a stub so callers don't have to be
// edited until the broader Space-retirement pass.
func (c *ChattoCore) createSpaceResources(_ context.Context, _ string) error {
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
// Events arrive via NATS Core subscriptions on two internal subject roots:
// `live.sync.>` carries transient LiveEvent messages and `live.evt.>` is the
// raw singleton republish of committed EVT facts. EVT delivery is not UI-safe
// by itself: filterLiveEvent waits for the relevant local projection(s) to
// reach the republished stream sequence, then applies this user's
// authorization before forwarding the event through GraphQL.
//
// Authorization:
//   - Room events (live.sync.room.> and deliverable live.evt.room.>) are delivered only for rooms
//     where the user is a member. The membership set is pre-loaded
//     across both kinds (channel + dm) and updated as join/leave/
//     room-deleted events arrive.
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
func (c *ChattoCore) StreamMyEvents(ctx context.Context, userID string) (<-chan EventEnvelope, error) {
	// memberRooms is the per-subscription visibility cache: the user
	// receives live events for rooms they are an explicit member of.
	// Seeded from `room_membership.*` records and mutated on
	// `UserJoinedRoom` / `UserLeftRoom` / `RoomDeleted`, and re-seeded
	// on `RoomGroupsUpdated` to absorb admin-driven membership changes.
	memberRooms := make(map[string]struct{})
	if err := c.populateMemberRoomsCache(ctx, userID, memberRooms); err != nil {
		return nil, err
	}

	// live.sync.> is the transient LiveEvent subject root. live.evt.> is
	// the raw committed-event feed from the EVT stream. SERVER_EVENTS no
	// longer participates in live delivery.
	//
	// The 256-message buffer absorbs reaction/typing bursts; on
	// overflow NATS Core drops messages and transitions the
	// subscription to SlowConsumer state — slowConsumerCh below
	// catches that and tears the resolver down so the client can
	// re-subscribe (and pick up missed history via the GraphQL
	// catch-up path) rather than silently miss events.
	msgChan := make(chan *nats.Msg, 256)
	liveSyncSub, err := c.nc.ChanSubscribe(subjects.LiveSyncAllEvents(), msgChan)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to live sync events: %w", err)
	}
	slowSyncConsumerCh := liveSyncSub.StatusChanged(nats.SubscriptionSlowConsumer)

	liveEVTSub, err := c.nc.ChanSubscribe(events.LiveSubjectRoot+">", msgChan)
	if err != nil {
		liveSyncSub.Unsubscribe()
		return nil, fmt.Errorf("failed to subscribe to live EVT events: %w", err)
	}
	slowEVTConsumerCh := liveEVTSub.StatusChanged(nats.SubscriptionSlowConsumer)

	presenceSub, err := c.PresenceHub.Subscribe(ctx)
	if err != nil {
		liveSyncSub.Unsubscribe()
		liveEVTSub.Unsubscribe()
		return nil, fmt.Errorf("failed to subscribe to presence hub: %w", err)
	}

	eventChan := make(chan EventEnvelope)

	go func() {
		c.logger.Debug("Server event stream started", "user_id", userID, "member_rooms", len(memberRooms))

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
			liveSyncSub.Unsubscribe()
			liveEVTSub.Unsubscribe()
			c.PresenceHub.Unsubscribe(presenceSub)
			close(eventChan)
		}()

		send := func(event EventEnvelope) bool {
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

			case <-slowEVTConsumerCh:
				dropped, _ := liveEVTSub.Dropped()
				c.logger.Warn("Slow consumer on live EVT subscription — tearing down",
					"user_id", userID, "dropped", dropped)
				return

			case <-slowSyncConsumerCh:
				dropped, _ := liveSyncSub.Dropped()
				c.logger.Warn("Slow consumer on live sync subscription — tearing down",
					"user_id", userID, "dropped", dropped)
				return

			case <-presenceTicker.C:
				if err := c.refreshPresence(ctx, userID); err != nil {
					c.logger.Warn("Failed to refresh presence", "error", err, "user_id", userID)
				}

			case <-heartbeatTicker.C:
				if !send(NewHeartbeatEventEnvelope(NewEventID(), timestamppb.Now())) {
					return
				}

			case msg := <-msgChan:
				event, ok := c.filterLiveEvent(ctx, userID, memberRooms, msg)
				if !ok {
					continue
				}
				if !send(event) {
					return
				}
				// Session termination tears down the subscription.
				// The frontend handles logout on receipt; closing
				// the channel ensures the server tears down too.
				if EventSessionTerminated(event) != nil {
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
				live := newLiveEvent(update.UserID, &corev1.LiveEvent{
					Event: &corev1.LiveEvent_PresenceChanged{
						PresenceChanged: &corev1.PresenceChangedEvent{Status: update.Status},
					},
				})
				if !send(NewLiveEventEnvelope(live)) {
					return
				}
			}
		}
	}()

	return eventChan, nil
}

// populateMemberRoomsCache (re)builds the per-subscription room
// visibility set in place. The cache contains every channel room the
// user is an explicit member of, plus every DM room they participate in. Used at
// subscription start and on `RoomGroupsUpdatedEvent` to re-seed after
// admin-driven membership changes (e.g. a user gaining access to a
// room via a group-scope permission edit, then joining).
func (c *ChattoCore) populateMemberRoomsCache(ctx context.Context, userID string, memberRooms map[string]struct{}) error {
	for k := range memberRooms {
		delete(memberRooms, k)
	}

	// Explicit channel memberships. Membership alone qualifies — a user
	// who has joined the room receives its live events regardless of
	// whether they could re-join today (e.g. they joined while the room
	// was open, then `room.join` was denied for everyone). The
	// "leave the room" mutation is the only way to lose live events.
	channelRooms, err := c.ListMemberRooms(ctx, KindChannel, userID, MemberRoomListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list channel member rooms: %w", err)
	}
	for _, room := range channelRooms {
		memberRooms[room.Id] = struct{}{}
	}

	dmRooms, err := c.ListMemberRooms(ctx, KindDM, userID, MemberRoomListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list DM member rooms: %w", err)
	}
	// DM rooms surface via their own listing path; explicit
	// membership is the visibility gate.
	for _, room := range dmRooms {
		memberRooms[room.Id] = struct{}{}
	}

	return nil
}

// filterLiveEvent unmarshals a message from one of the live delivery roots
// and applies per-user authorization. Returns the event and true if it
// should be delivered. Mutates memberRooms when the subscriber
// themselves joins/leaves a room or when a room is deleted.
//
// Two routing paths:
//
//  1. Room subjects (live.sync.room.{kind}.{roomId}.…):
//     gated on room membership.
//  2. Everything else: delegated to isAuthorizedForLiveEvent.
func (c *ChattoCore) filterLiveEvent(ctx context.Context, userID string, memberRooms map[string]struct{}, msg *nats.Msg) (EventEnvelope, bool) {
	if strings.HasPrefix(msg.Subject, "live.sync.") {
		var live corev1.LiveEvent
		if err := proto.Unmarshal(msg.Data, &live); err != nil {
			c.logger.Warn("Failed to unmarshal live sync event", "subject", msg.Subject, "error", err)
			return nil, false
		}
		return c.filterLiveSyncEvent(ctx, userID, memberRooms, msg, &live)
	}

	if !strings.HasPrefix(msg.Subject, events.LiveSubjectRoot) {
		c.logger.Warn("Unknown live event subject root", "subject", msg.Subject)
		return nil, false
	}

	var event corev1.Event
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		c.logger.Warn("Failed to unmarshal live event", "subject", msg.Subject, "error", err)
		return nil, false
	}

	return c.filterLiveEVTEvent(ctx, userID, memberRooms, msg, &event)
}

func (c *ChattoCore) filterLiveSyncEvent(ctx context.Context, userID string, memberRooms map[string]struct{}, msg *nats.Msg, event *corev1.LiveEvent) (EventEnvelope, bool) {
	if event == nil || event.Event == nil {
		c.logger.Warn("Dropping live sync event without payload", "subject", msg.Subject)
		return nil, false
	}

	// Path 1: room-scoped transient events on live.sync.room.{kind}.{roomId}.…
	if kind := subjects.ParseKindFromRoomSubject(msg.Subject); kind != "" {
		roomID := subjects.ParseRoomIDFromSubject(msg.Subject)
		if roomID == "" {
			return nil, false
		}

		_, isMember := memberRooms[roomID]

		// Skip own typing events — the sender doesn't need to see them.
		// Critical for multi-server clients where the frontend's
		// currentUserId may differ from the remote server user ID.
		if event.GetUserTyping() != nil && event.ActorId == userID {
			return nil, false
		}

		if !isMember {
			return nil, false
		}
		return NewLiveEventEnvelope(event), true
	}

	// Path 2: user/config/member subjects.
	if !c.isAuthorizedForLiveEvent(ctx, userID, msg.Subject) {
		return nil, false
	}

	return NewLiveEventEnvelope(event), true
}

func (c *ChattoCore) filterLiveEVTEvent(ctx context.Context, userID string, memberRooms map[string]struct{}, msg *nats.Msg, event *corev1.Event) (EventEnvelope, bool) {
	roomID, ok := events.ParseRoomSubject(msg.Subject)
	if !ok {
		return nil, false
	}
	if !isDeliverableLiveEVTRoomEvent(event) {
		return nil, false
	}

	seq, err := strconv.ParseUint(msg.Header.Get(nats.JSSequence), 10, 64)
	if err != nil || seq == 0 {
		c.logger.Warn("live EVT message missing stream sequence", "subject", msg.Subject, "sequence", msg.Header.Get(nats.JSSequence), "error", err)
		return nil, false
	}
	waitCtx, cancel := context.WithTimeout(ctx, liveEVTProjectionWaitTimeout)
	defer cancel()
	if err := c.waitForLiveEVTRoomEvent(waitCtx, event, seq); err != nil {
		c.logger.Warn("Timed out waiting for live EVT projection readiness", "subject", msg.Subject, "sequence", seq, "error", err)
		return nil, false
	}

	_, isMember := memberRooms[roomID]
	switch event.Event.(type) {
	case *corev1.Event_UserJoinedRoom:
		if event.ActorId == userID {
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
	if !isMember {
		return nil, false
	}
	return NewEVTEventEnvelope(event), true
}

func isDeliverableLiveEVTRoomEvent(event *corev1.Event) bool {
	switch event.GetEvent().(type) {
	case *corev1.Event_RoomCreated,
		*corev1.Event_RoomUpdated,
		*corev1.Event_RoomDeleted,
		*corev1.Event_RoomArchived,
		*corev1.Event_RoomUnarchived,
		*corev1.Event_UserJoinedRoom,
		*corev1.Event_UserLeftRoom,
		*corev1.Event_MessagePosted,
		*corev1.Event_MessageEdited,
		*corev1.Event_MessageRetracted,
		*corev1.Event_ReactionAdded,
		*corev1.Event_ReactionRemoved,
		*corev1.Event_AssetProcessingStarted,
		*corev1.Event_AssetProcessingSucceeded,
		*corev1.Event_AssetProcessingFailed,
		*corev1.Event_AssetDeleted:
		return true
	default:
		return false
	}
}

func (c *ChattoCore) waitForLiveEVTRoomEvent(ctx context.Context, event *corev1.Event, seq uint64) error {
	if err := waitForSeqAll(ctx, seq,
		waitForProjection("room timeline", c.RoomTimelineProjector),
		waitForProjection("threads", c.ThreadsProjector),
	); err != nil {
		return err
	}

	switch event.GetEvent().(type) {
	case *corev1.Event_ReactionAdded, *corev1.Event_ReactionRemoved:
		if err := waitForSeqAll(ctx, seq, waitForProjection("reactions", c.ReactionsProjector)); err != nil {
			return err
		}
	}
	switch event.GetEvent().(type) {
	case *corev1.Event_UserJoinedRoom, *corev1.Event_UserLeftRoom, *corev1.Event_RoomDeleted:
		if err := waitForSeqAll(ctx, seq, waitForProjection("room membership", c.RoomMembershipProjector)); err != nil {
			return err
		}
	}
	switch event.GetEvent().(type) {
	case *corev1.Event_RoomCreated,
		*corev1.Event_RoomUpdated,
		*corev1.Event_RoomArchived,
		*corev1.Event_RoomUnarchived,
		*corev1.Event_RoomDeleted:
		if err := waitForSeqAll(ctx, seq, waitForProjection("room catalog", c.RoomCatalogProjector)); err != nil {
			return err
		}
	}
	return nil
}

// isAuthorizedForLiveEvent checks if a user is authorized to receive a
// non-room live event based on the subject pattern:
//
//   - live.sync.config.* → all authenticated users (server config /
//     branding / room-layout updates — public to every member)
//   - live.sync.member.* → all authenticated users (single-server membership)
//   - live.sync.user.{userId}.* → only the target user, except
//     live.sync.user.{userId}.profile_updated which is broadcast.
//
// Room events (`live.sync.room.>`) are filtered separately via the per-user
// room-membership cache and never reach this function.
func (c *ChattoCore) isAuthorizedForLiveEvent(_ context.Context, userID, subject string) bool {
	parts := strings.Split(subject, ".")
	if len(parts) < 3 || parts[0] != "live" || parts[1] != "sync" {
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
	event := newLiveEvent(actorID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_ConfigUpdated{
			ConfigUpdated: &corev1.ServerConfigUpdatedEvent{
				ServerName:       serverName,
				Motd:             motd,
				WelcomeMessage:   welcomeMessage,
				BlockedUsernames: blockedUsernames,
			},
		},
	})

	return c.publishLiveEvent(ctx, subjects.LiveSyncConfigEvent("updated"), event)
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
	stats.UserCount, _, _ = c.Users.Stats()

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

package core

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	"hmans.de/chatto/internal/dekstore"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/kms"
	"hmans.de/chatto/internal/lease"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// ChattoCore
// ============================================================================

// ChattoCore is the central hub for all Chatto operations.
// It provides a unified API for spaces, users, rooms, and messages,
// managing current JetStream resources internally.
type ChattoCore struct {
	nc                 *nats.Conn
	js                 jetstream.JetStream
	logger             *log.Logger
	storage            *storage
	config             config.CoreConfig
	encryption         *encryptionManager
	configManager      *ConfigManager
	roomService        *RoomService
	userService        *UserService
	rbacService        *RBACService
	mentionables       *MentionablesService
	myEventsService    *MyEventsService
	presenceService    *PresenceService
	mediaService       *MediaService
	callService        *CallService
	assetService       *AssetService
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

	// PresenceHub is the compatibility handle for PresenceService's per-process
	// fanout hub. Started by (*ChattoCore).Run through PresenceService.
	PresenceHub *PresenceHub

	// EventPublisher writes to the EVT event-sourcing stream
	// (ADR-033/034). Exposed for use by the migrate subcommand and
	// future aggregate cutovers; domain code accesses it through
	// higher-level helpers as aggregates migrate.
	EventPublisher *events.Publisher

	// RoomDirectory combines the room catalog and membership read models under
	// one evt.room.> projector.
	RoomDirectory *RoomDirectoryProjection

	// RoomDirectoryProjector runs the consumer for RoomDirectory. The
	// room catalog and membership writer paths wait on this projector for
	// read-your-writes.
	RoomDirectoryProjector *events.Projector

	// RoomMembership is the membership index inside RoomDirectory.
	RoomMembership *RoomMembershipProjection

	// RoomBans is the active moderation-ban index inside RoomDirectory.
	RoomBans *RoomBanProjection

	// ServerConfig is the projection holding current dynamic configuration
	// rebuilt from EVT. The field name is retained for compatibility with
	// existing admin/verification code while the projection now stores more
	// than the old server-config snapshot.
	ServerConfig *ConfigProjection

	// ServerConfigProjector runs the consumer + apply loop that keeps
	// ServerConfig current. Started by (*ChattoCore).Run; exposed here
	// so writers (ConfigManager mutations) can call WaitFor.
	ServerConfigProjector *events.Projector

	// RoomCatalog is the room metadata index inside RoomDirectory.
	RoomCatalog *RoomCatalogProjection

	// RoomGroupLayout combines room-group state and sidebar ordering under one
	// projector over evt.group.> plus evt.layout.>.
	RoomGroupLayout *RoomGroupLayoutProjection

	// RoomGroupLayoutProjector runs the consumer for RoomGroupLayout. The
	// room-group and layout writer paths wait on this projector for
	// read-your-writes.
	RoomGroupLayoutProjector *events.Projector

	// RoomGroups is the group state index inside RoomGroupLayout.
	RoomGroups *RoomGroupProjection

	// RoomLayout is the sidebar ordering index inside RoomGroupLayout.
	RoomLayout *RoomLayoutProjection

	// RoomTimeline holds an append-only event log per room, derived
	// from the full evt.room.> firehose (#597 phase 2). Source of
	// truth for room timeline reads post-cutover.
	RoomTimeline *RoomTimelineProjection

	// RoomTimelineProjector runs the consumer for RoomTimeline.
	// Exposed for WaitFor from message writers.
	RoomTimelineProjector *events.Projector

	// CallState holds active voice-call participants derived from durable
	// room-call lifecycle and participant facts.
	CallState *CallStateProjection

	// CallStateProjector runs the consumer for CallState.
	CallStateProjector *events.Projector

	// Assets holds durable asset lifecycle and processing state. It consumes
	// canonical evt.asset.> events plus legacy room-scoped asset events for
	// beta-history compatibility.
	Assets *AssetProjection

	// AssetsProjector runs the consumer for Assets. Exposed for WaitFor from
	// asset writers.
	AssetsProjector *events.Projector

	// Threads holds an append-only event log per thread root,
	// derived from the same evt.room.> firehose. Source of truth
	// for thread-pane reads post-cutover.
	Threads *ThreadProjection

	// ThreadsProjector runs the consumer for Threads. Exposed for
	// WaitFor from message writers that touch threads.
	ThreadsProjector *events.Projector

	// Reactions holds current per-message reaction state derived
	// from durable room-aggregate reaction events.
	Reactions *ReactionProjection

	// ReactionsProjector runs the consumer for Reactions. Exposed
	// for WaitFor from reaction writers.
	ReactionsProjector *events.Projector

	// Users holds current user/account/profile/auth lookup state derived
	// from durable user-aggregate events.
	Users *UserProjection

	// UsersProjector runs the consumer for Users. Exposed for
	// WaitFor from user/account writers.
	UsersProjector *events.Projector

	// ContentKeys holds wrapped per-user DEK epochs used by encrypted
	// message bodies and durable user PII.
	ContentKeys *ContentKeyProjection

	// ContentKeysProjector runs the consumer for ContentKeys. Exposed for
	// WaitFor from encryption writers.
	ContentKeysProjector *events.Projector

	// RBAC holds current role, assignment, and permission state derived
	// from durable RBAC aggregate events.
	RBAC *RBACProjection

	// RBACProjector runs the consumer for RBAC. Exposed for WaitFor
	// from role and permission writers.
	RBACProjector *events.Projector

	// Mentionables owns the global @handle namespace derived from user and
	// RBAC facts.
	Mentionables *MentionablesProjection

	// MentionablesProjector runs the consumer for Mentionables. Exposed for
	// WaitFor from handle-changing user and role writers.
	MentionablesProjector *events.Projector

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
// PresenceService and every registered projector — and blocks until ctx is
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
		projection := projection
		projector := projection.projector
		g.Go(func() error {
			if err := projector.Run(gctx); err != nil {
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
		c.secureDeleteObsoleteProjectedMessageBodyEvents(gctx)
		// Apply config-designated owners to already-verified users on every
		// boot. Changing owners.emails requires a process restart, so this
		// is the natural point to materialize new config owners as RBAC
		// assignments. The assignment path is idempotent.
		if err := c.applyConfigOwners(gctx); err != nil {
			return fmt.Errorf("apply config owners: %w", err)
		}
		if err := c.EnsureDefaultRolePermissions(gctx); err != nil {
			return fmt.Errorf("ensure default role permissions: %w", err)
		}
		// Seed the default room group and ensure every existing
		// channel room belongs to a set (ADR-031). Idempotent —
		// runs on every boot. Has to happen AFTER projectors are
		// running and caught up because it reads the RoomGroups
		// projection and depends on WaitFor actually waiting.
		if err := c.ensureChannelRoomsAreInAGroup(gctx); err != nil {
			return fmt.Errorf("ensure channel rooms in a group: %w", err)
		}
		close(c.bootDone)
		return nil
	})

	g.Go(func() error { return c.presenceService.Run(gctx) })
	g.Go(func() error { return c.callService.Run(gctx) })

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
	callKeys    kms.CallKeyStore
	contentKeys *dekstore.Store
}

func (c *ChattoCore) ServerStore() jetstream.ObjectStore {
	return c.storage.serverAssets
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

	if err := c.userService.waitForContentKeysCurrent(ctx, userID); err != nil {
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
	subject := events.UserAggregate(userID).SubjectFor(event)
	return c.rooms().waitForTimelineAndThreads(ctx, events.SubjectPosition(subject, seq))
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
// It tries the canonical SERVER_ASSETS NATS object store first, then S3.
// Returns a reader for the asset content and metadata.
// The caller is responsible for closing the reader if it implements io.Closer.
func (c *ChattoCore) GetServerAssetFromAnyBackend(ctx context.Context, assetID string) (io.Reader, *ServerAssetInfo, error) {
	obj, err := c.storage.serverAssets.Get(ctx, assetID)
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
		if err := c.storage.serverAssets.Delete(ctx, natsAsset.Key); err != nil {
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
	c.deleteCachedResizesForServerAsset(ctx, assetIDFromAsset(asset), "orphaned asset", "")
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
		if err := c.storage.serverAssets.Delete(ctx, natsAsset.Key); err != nil {
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
	c.deleteCachedResizesForServerAsset(ctx, assetIDFromAsset(asset), assetType, ownerID)
}

func (c *ChattoCore) deleteCachedResizesForServerAsset(ctx context.Context, assetID, assetType, ownerID string) {
	deletedCount, cacheErr := c.DeleteCachedResizesForServerAsset(ctx, assetID)
	if cacheErr != nil {
		c.logger.Warn("Failed to delete cached resizes for server asset",
			"asset_id", assetID,
			"asset_type", assetType,
			"owner_id", ownerID,
			"error", cacheErr)
	} else if deletedCount > 0 {
		c.logger.Debug("Deleted cached resizes for server asset",
			"asset_id", assetID,
			"asset_type", assetType,
			"owner_id", ownerID,
			"deleted_count", deletedCount)
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
	if err := c.ProjectionHealthError(); err != nil {
		return fmt.Errorf("projection unhealthy: %w", err)
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

	// Initialize storage.
	storage, err := newStorage(js, ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize storage: %w", err)
	}

	// Initialize encryption manager
	builtinKMS := kms.NewBuiltin(storage.encryptionKV, logger.WithPrefix("core.kms"))
	encMgr := &encryptionManager{
		keyWrapper:  builtinKMS,
		legacyKeys:  builtinKMS,
		callKeys:    builtinKMS,
		contentKeys: dekstore.New(storage.runtimeStateKV, logger.WithPrefix("core.dekstore")),
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
	// wiring so projections and services that need them can be passed the
	// concrete deps at construction. Order: publisher → projections →
	// projectors → services that depend on them.
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

	roomDirectory := NewRoomDirectoryProjection()
	roomDirectoryProjector := newProjector(roomDirectory, "Room Directory", roomDirectory.adminProjectionEstimate)
	roomMembership := roomDirectory.Membership
	roomBans := roomDirectory.Bans

	serverConfigProjection := NewConfigProjection()
	serverConfigProjector := newProjector(serverConfigProjection, "Server Config", serverConfigProjection.adminProjectionEstimate)

	roomCatalog := roomDirectory.Catalog

	roomGroupLayout := NewRoomGroupLayoutProjection()
	roomGroupLayoutProjector := newProjector(roomGroupLayout, "Room Group Layout", roomGroupLayout.adminProjectionEstimate)
	roomGroups := roomGroupLayout.Groups
	roomLayout := roomGroupLayout.Layout

	// Per-room event-log + per-thread event-log projections (#597
	// phase 2). Both consume the full evt.room.> firehose; resolvers
	// do all filtering and rendering at query time. v1 shape — we
	// iterate significantly on this once we observe read patterns.
	roomTimeline := NewRoomTimelineProjection()
	roomTimelineProjector := newProjector(roomTimeline, "Room Timeline", roomTimeline.adminProjectionEstimate)

	callState := NewCallStateProjection()
	callStateProjector := newProjector(callState, "Call State", callState.adminProjectionEstimate)

	assetProjection := NewAssetProjection()
	assetProjector := newProjector(assetProjection, "Assets", assetProjection.adminProjectionEstimate)

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

	mentionables := NewMentionablesProjection(encMgr.keyWrapper, encMgr.contentKeys)
	mentionablesProjector := newProjector(mentionables, "Mentionables", mentionables.adminProjectionEstimate)

	configService := NewConfigService(eventPublisher, serverConfigProjector, serverConfigProjection)
	configMgr := NewConfigManager(configService, serverConfigProjection)
	roomMgr := newRoomService(
		roomDirectory,
		roomDirectoryProjector,
		roomGroupLayout,
		roomGroupLayoutProjector,
		roomTimeline,
		roomTimelineProjector,
		threads,
		threadsProjector,
		reactions,
		reactionsProjector,
	)
	userMgr := newUserService(eventPublisher, users, usersProjector, contentKeys, contentKeysProjector)
	rbacMgr := newRBACService(rbac, rbacProjector)
	mentionablesMgr := newMentionablesService(mentionables, mentionablesProjector)

	core := &ChattoCore{
		nc:                       nc,
		js:                       js,
		logger:                   logger,
		storage:                  storage,
		config:                   cfg,
		encryption:               encMgr,
		configManager:            configMgr,
		roomService:              roomMgr,
		userService:              userMgr,
		rbacService:              rbacMgr,
		mentionables:             mentionablesMgr,
		s3Client:                 s3Client,
		EventPublisher:           eventPublisher,
		RoomDirectory:            roomDirectory,
		RoomDirectoryProjector:   roomDirectoryProjector,
		RoomMembership:           roomMembership,
		RoomBans:                 roomBans,
		ServerConfig:             serverConfigProjection,
		ServerConfigProjector:    serverConfigProjector,
		RoomCatalog:              roomCatalog,
		RoomGroupLayout:          roomGroupLayout,
		RoomGroupLayoutProjector: roomGroupLayoutProjector,
		RoomGroups:               roomGroups,
		RoomLayout:               roomLayout,
		RoomTimeline:             roomTimeline,
		RoomTimelineProjector:    roomTimelineProjector,
		CallState:                callState,
		CallStateProjector:       callStateProjector,
		Assets:                   assetProjection,
		AssetsProjector:          assetProjector,
		Threads:                  threads,
		ThreadsProjector:         threadsProjector,
		Reactions:                reactions,
		ReactionsProjector:       reactionsProjector,
		Users:                    users,
		UsersProjector:           usersProjector,
		ContentKeys:              contentKeys,
		ContentKeysProjector:     contentKeysProjector,
		RBAC:                     rbac,
		RBACProjector:            rbacProjector,
		Mentionables:             mentionables,
		MentionablesProjector:    mentionablesProjector,
		projections:              projections,
		bootDone:                 make(chan struct{}),
	}

	callReconcileLease, err := lease.New(js, storage.memoryCacheKV, lease.Options{
		Name:       callReconcileLeaseName,
		Bucket:     "MEMORY_CACHE",
		TTL:        callReconcileLeaseTTL,
		RenewEvery: callReconcileLeaseRenewEvery,
		RetryEvery: callReconcileLeaseRetryEvery,
		Logger:     logger.WithPrefix("core.CallReconcilerLease"),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to initialize call reconciler lease: %w", err)
	}

	core.mediaService = NewMediaService(core)
	core.callService = NewCallService(eventPublisher, callState, callStateProjector, encMgr.callKeys, nil, callReconcileLease, storage.memoryCacheKV, logger.WithPrefix("core.CallService"))
	core.assetService = NewAssetService(core)

	if err := core.seedDefaultRBAC(ctx); err != nil {
		return nil, fmt.Errorf("failed to seed default RBAC: %w", err)
	}

	// Initialize permission resolver (must be done after core struct is created)
	core.permissionResolver = NewPermissionResolver(core)

	// Initialize link preview cache and fetcher
	core.linkPreviewCache = linkpreview.NewCache(storage.runtimeStateKV)
	assetsConfig := core.AssetsConfig()
	core.linkPreviewFetcher = linkpreview.NewFetcher(storage.serverAssets, &assetsConfig, NewAssetID)

	// ensureChannelRoomsAreInAGroup is deferred to core.Run() — it
	// needs the projectors to be live so its CreateRoomGroup /
	// MoveRoomToGroup calls can actually WaitFor. Doing it here
	// (when projectors haven't been started yet) would leave orphan
	// rooms in any subsequent SeedDefaultRooms call.

	// Initialize presence service (single KV watcher per process). Started
	// by core.Run alongside the projectors.
	core.presenceService = NewPresenceService(js, storage.memoryCacheKV, logger)
	core.PresenceHub = core.presenceService.hub
	core.myEventsService = NewMyEventsService(core)

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

// storage encapsulates JetStream resources used by Chatto Core.
type storage struct {
	encryptionKV   jetstream.KeyValue // ENCRYPTION_KEYS - KMS KEKs (excluded from backups)
	runtimeStateKV jetstream.KeyValue // RUNTIME_STATE  - persisted latest-value runtime/user state + wrapped app DEKs
	serverBodiesKV jetstream.KeyValue // SERVER_BODIES    - legacy message bodies retained for cleanup

	serverAssets    jetstream.ObjectStore // SERVER_ASSETS - all NATS-backed asset binaries
	serverEvtStream jetstream.Stream      // EVT       - event-sourcing log (ADR-033/034).

	memoryCacheKV   jetstream.KeyValue    // MEMORY_CACHE - volatile, memory-backed runtime cache state
	imageCacheStore jetstream.ObjectStore // Optional: cached resized images (nil if disabled)
}

// newStorage initializes current JetStream resources.
func newStorage(js jetstream.JetStream, ctx context.Context, cfg config.CoreConfig) (*storage, error) {
	// Initialize KMS KEK bucket (excluded from backups for security). App-owned
	// wrapped DEK records live in RUNTIME_STATE so normal backups keep encrypted
	// content together with its wrapped content-key registry, but not the KEKs
	// needed to unwrap it.
	encryptionKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "ENCRYPTION_KEYS",
		Description: "KMS key-encryption keys (excluded from backups)",
		Storage:     jetstream.FileStorage,
		History:     1,
		Replicas:    cfg.Replicas,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create ENCRYPTION_KEYS KV bucket: %w", err)
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

	serverBodiesKV, err := openLegacyKeyValue(ctx, js, "SERVER_BODIES")
	if err != nil {
		return nil, fmt.Errorf("failed to open legacy SERVER_BODIES KV bucket: %w", err)
	}

	serverAssets, err := js.CreateOrUpdateObjectStore(ctx, jetstream.ObjectStoreConfig{
		Bucket:      "SERVER_ASSETS",
		Description: "Server asset binaries (avatars, branding, link previews, attachments)",
		Storage:     jetstream.FileStorage,
		Compression: true,
		Replicas:    cfg.Replicas,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create SERVER_ASSETS object store: %w", err)
	}

	// EVT — the event-sourcing log (ADR-033/034).
	// Subjects are evt.{aggregateType}.{aggregateId}.{eventType}; live.evt.> is
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

	return &storage{
		encryptionKV:    encryptionKV,
		runtimeStateKV:  runtimeStateKV,
		serverBodiesKV:  serverBodiesKV,
		serverAssets:    serverAssets,
		serverEvtStream: serverEvtStream,
		memoryCacheKV:   memoryCacheKV,
		imageCacheStore: imageCacheStore,
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

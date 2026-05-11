package core

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math/rand"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/assets"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core/linkpreview"
	"hmans.de/chatto/internal/core/rbac"
	"hmans.de/chatto/internal/core/subjects"
	"hmans.de/chatto/internal/encryption"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ============================================================================
// ChattoCore
// ============================================================================

// ChattoCore is the central hub for all Chatto operations.
// It provides a unified API for spaces, users, rooms, and messages,
// managing all KV buckets and event streams internally.
type ChattoCore struct {
	nc                   *nats.Conn
	js                   jetstream.JetStream
	logger               *log.Logger
	storage              *storage
	config               config.CoreConfig
	encryption           *encryptionManager
	configManager        *ConfigManager
	roomNameIndexBackfilled sync.Map // tracks which spaces have had their room-name index backfilled
	s3Client             *S3Client           // Optional S3 client for S3-compatible storage
	permissionResolver   *PermissionResolver // Hierarchical permission resolver
	linkPreviewCache     *linkpreview.Cache  // Cache for link preview metadata
	linkPreviewFetcher   *linkpreview.Fetcher // Fetcher for link preview metadata

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
	// out updates to all space subscriptions. Must be started via Run() in an errgroup.
	PresenceHub *PresenceHub

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

func (c *ChattoCore) InstanceStore() jetstream.ObjectStore {
	return c.storage.instanceStore
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
// This implements the instance < space < room specificity model.
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

// InstanceAssetInfo contains metadata about an instance asset.
type InstanceAssetInfo struct {
	Size        int64
	ContentType string
}

// GetInstanceAssetFromAnyBackend retrieves an instance asset by probing both NATS and S3 backends.
// It tries NATS first (for backwards compatibility with existing assets), then S3.
// Returns a reader for the asset content and metadata.
// The caller is responsible for closing the reader if it implements io.Closer.
func (c *ChattoCore) GetInstanceAssetFromAnyBackend(ctx context.Context, assetID string) (io.Reader, *InstanceAssetInfo, error) {
	// Try NATS first (backwards compatibility)
	obj, err := c.storage.instanceStore.Get(ctx, assetID)
	if err == nil {
		info, _ := obj.Info()
		return obj, &InstanceAssetInfo{
			Size:        int64(info.Size),
			ContentType: info.Headers.Get("Content-Type"),
		}, nil
	}

	// If NATS failed and S3 is configured, try S3
	if c.s3Client != nil {
		s3Key := S3KeyInstanceAsset(assetID)
		reader, s3Info, s3Err := c.s3Client.GetObject(ctx, s3Key)
		if s3Err == nil {
			return reader, &InstanceAssetInfo{
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

// CleanupAsset deletes an asset from the instance object store.
// Used to clean up orphaned assets when subsequent operations fail.
func (c *ChattoCore) CleanupAsset(ctx context.Context, asset *corev1.Asset) {
	if asset == nil {
		return
	}
	if natsAsset := asset.GetNats(); natsAsset != nil {
		if err := c.storage.instanceStore.Delete(ctx, natsAsset.Key); err != nil {
			c.logger.Warn("Failed to clean up orphaned asset", "key", natsAsset.Key, "error", err)
		} else {
			c.logger.Info("Cleaned up orphaned asset", "key", natsAsset.Key)
		}
	}
	if s3Asset := asset.GetS3(); s3Asset != nil && c.s3Client != nil {
		// S3Asset.Key stores just the assetID; construct the full S3 path
		s3Key := S3KeyInstanceAsset(s3Asset.Key)
		if err := c.s3Client.DeleteObjectFromBucket(ctx, s3Asset.GetBucket(), s3Key); err != nil {
			c.logger.Warn("Failed to clean up orphaned S3 asset", "asset_id", s3Asset.Key, "s3_key", s3Key, "error", err)
		} else {
			c.logger.Info("Cleaned up orphaned S3 asset", "asset_id", s3Asset.Key, "s3_key", s3Key)
		}
	}
}

// deleteAsset deletes an instance asset from its storage backend (NATS or S3).
// This is a helper for cleaning up old assets when they are replaced.
// For S3, the assetID stored in S3Asset.Key is used to construct the full S3 path.
// The assetType and ownerID are used for logging only.
func (c *ChattoCore) deleteAsset(ctx context.Context, asset *corev1.Asset, assetType, ownerID string) {
	if asset == nil {
		return
	}
	if natsAsset := asset.GetNats(); natsAsset != nil {
		if err := c.storage.instanceStore.Delete(ctx, natsAsset.Key); err != nil {
			c.logger.Warn("Failed to delete old "+assetType, "owner_id", ownerID, "key", natsAsset.Key, "error", err)
		} else {
			c.logger.Info("Deleted old "+assetType, "owner_id", ownerID, "key", natsAsset.Key)
		}
	}
	if s3Asset := asset.GetS3(); s3Asset != nil && c.s3Client != nil {
		// S3Asset.Key stores just the assetID; construct the full S3 path
		s3Key := S3KeyInstanceAsset(s3Asset.Key)
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
	_, err := c.storage.instanceKV.Status(ctx)
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
	storage.serverRBACEngine = rbac.NewEngine(storage.serverRBACKV, rbac.Config{
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

	// Initialize config manager for runtime configuration
	configMgr := NewConfigManager(storage.instanceConfigKV)

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

	core := &ChattoCore{
		nc:                 nc,
		js:                 js,
		logger:             logger,
		storage:            storage,
		config:             cfg,
		encryption:         encMgr,
		configManager:      configMgr,
		s3Client:           s3Client,
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
	core.linkPreviewFetcher = linkpreview.NewFetcher(storage.instanceStore, &assetsConfig, NewAssetID)

	// Initialize DM system space
	if err := core.initDMSpace(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize DM space: %w", err)
	}

	// Initialize instance-level RBAC (roles and permissions)
	if err := core.initInstanceRBAC(ctx); err != nil {
		return nil, fmt.Errorf("failed to initialize instance RBAC: %w", err)
	}

	// Initialize presence hub (single KV watcher per process).
	// Caller must start core.PresenceHub.Run(ctx) in an errgroup.
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
	instanceKV       jetstream.KeyValue
	instanceStore    jetstream.ObjectStore
	encryptionKV     jetstream.KeyValue // Encryption keys (excluded from backups)
	instanceConfigKV jetstream.KeyValue // Runtime configuration overrides

	// Server-level KV buckets (#330 phase 4a, 4b, 4c, 4e) and event stream
	// (#330 phase 4d). Shared by the primary and DM spaces; non-primary,
	// non-DM spaces (test-created only in practice) keep their per-space
	// lazycaches below.
	serverConfigKV     jetstream.KeyValue    // SERVER_CONFIG    - rooms, memberships
	serverRuntimeKV    jetstream.KeyValue    // SERVER_RUNTIME   - sequences, timestamps, read state
	serverRBACKV       jetstream.KeyValue    // SERVER_RBAC      - roles, permissions, assignments
	serverRBACEngine   *rbac.Engine          // rbac.Engine wrapping serverRBACKV
	serverBodiesKV     jetstream.KeyValue    // SERVER_BODIES    - message bodies (#330 phase 4c)
	serverReactionsKV  jetstream.KeyValue    // SERVER_REACTIONS - emoji reactions (#330 phase 4c)
	serverThreadsKV    jetstream.KeyValue    // SERVER_THREADS   - thread metadata (#330 phase 4c)
	serverAttachments  jetstream.ObjectStore // SERVER_ASSETS    - message attachments (#330 phase 4e)
	serverEventsStream jetstream.Stream      // SERVER_EVENTS    - event stream (#330 phase 4d)

	presenceKV            jetstream.KeyValue     // Instance-level presence bucket
	imageCacheStore       jetstream.ObjectStore  // Optional: cached resized images (nil if disabled)
	notificationsKV       jetstream.KeyValue     // User notifications with TTL
	callStateKV           jetstream.KeyValue     // Active voice call participants (ephemeral, memory-backed)
	authTokensKV          jetstream.KeyValue     // Bearer auth tokens with TTL
}

// newStorage initializes all JetStream KV buckets and streams.
func newStorage(js jetstream.JetStream, ctx context.Context, cfg config.CoreConfig) (*storage, error) {
	// Initialize INSTANCE KV bucket for all instance-level data
	// Uses subject-based keys: user.{userId}, space.{spaceId}, space_membership.{spaceId}.{userId}, etc.
	instanceKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
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

	// Initialize instance object store
	instanceStore, err := js.CreateOrUpdateObjectStore(ctx, jetstream.ObjectStoreConfig{
		Bucket:      "INSTANCE_ASSETS",
		Description: "Instance-level assets (user avatars, space icons, etc.)",
		Storage:     jetstream.FileStorage,
		Compression: true,
		Replicas:    cfg.Replicas,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create INSTANCE object store: %w", err)
	}

	// Initialize instance-level presence KV bucket (memory-based with TTL)
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
	instanceConfigKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "INSTANCE_CONFIG",
		Description: "Runtime configuration overrides",
		Storage:     jetstream.FileStorage,
		Replicas:    cfg.Replicas,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create INSTANCE_CONFIG KV bucket: %w", err)
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

	// Initialize notifications KV bucket with 3-month TTL
	notificationsKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "NOTIFICATIONS",
		Description: "User notifications (mentions, DMs, thread replies)",
		Storage:     jetstream.FileStorage,
		TTL:         90 * 24 * time.Hour, // 3 months
		Replicas:    cfg.Replicas,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create NOTIFICATIONS KV bucket: %w", err)
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
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create SERVER_EVENTS stream: %w", err)
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
		instanceKV:       instanceKV,
		instanceStore:    instanceStore,
		encryptionKV:     encryptionKV,
		instanceConfigKV: instanceConfigKV,
		serverConfigKV:     serverConfigKV,
		serverRBACKV:       serverRBACKV,
		serverRuntimeKV:    serverRuntimeKV,
		serverBodiesKV:     serverBodiesKV,
		serverReactionsKV:  serverReactionsKV,
		serverThreadsKV:    serverThreadsKV,
		serverAttachments:  serverAttachments,
		serverEventsStream: serverEventsStream,
		// serverRBACEngine is constructed below (after the storage value
		// exists) and assigned in NewChattoCore.
		presenceKV:            presenceKV,
		imageCacheStore:       imageCacheStore,
		notificationsKV:       notificationsKV,
		callStateKV:           callStateKV,
		authTokensKV:          authTokensKV,
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

// spaceLogoKey returns the KV key for a space's logo asset reference.
// Logo assets are stored separately from space profile to avoid overwriting
// the entire space record when the logo changes.
func spaceLogoKey(spaceID string) string {
	return fmt.Sprintf("space.%s.logo", spaceID)
}

// spaceBannerKey returns the KV key for a space's banner asset reference.
// Banner assets are stored separately from space profile to avoid overwriting
// the entire space record when the banner changes.
func spaceBannerKey(spaceID string) string {
	return fmt.Sprintf("space.%s.banner", spaceID)
}

// spaceKey returns the KV key for a space record.
func spaceKey(spaceID string) string {
	return fmt.Sprintf("space.%s", spaceID)
}

// roomKindKeyFromSpaceID returns the kind segment for the rooms that live
// in a given space. DM space holds DM rooms; everything else holds channels.
//
// The segment is part of the key on disk (e.g., `room.channel.{roomID}`,
// `room.dm.{roomID}`) so list operations can prefix-filter by kind via
// NATS subject matching without loading and deserializing every room
// record. Kind isn't stored on the Room proto — the storage layout is
// the canonical source of truth.
func roomKindKeyFromSpaceID(spaceID string) string {
	if IsDMSpace(spaceID) {
		return "dm"
	}
	return "channel"
}

// roomKey returns the KV key for a room record in a space bucket.
// Pattern: `room.{kind}.{roomID}` where kind is "channel" or "dm".
func roomKey(kind, roomID string) string {
	return fmt.Sprintf("room.%s.%s", kind, roomID)
}

// roomKeyPrefix returns the key prefix for listing all rooms of a given
// kind in a CONFIG bucket. Pattern: `room.{kind}.*`.
func roomKeyPrefix(kind string) string {
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
func (c *ChattoCore) publishServerEvent(_ context.Context, subject string, event *corev1.ServerEvent) error {
	if err := validateSpaceEvent(event); err != nil {
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
func (c *ChattoCore) publishLiveServerEvent(_ context.Context, subject string, event *corev1.ServerEvent) error {
	if err := validateSpaceEvent(event); err != nil {
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

// publishLiveEvent publishes an InstanceEvent directly to a live.instance.> subject, bypassing JetStream storage.
// Use this for instance-scoped notifications (user events, space lifecycle, config updates).
// The subject should already include the "live.instance." prefix.
func (c *ChattoCore) publishLiveEvent(_ context.Context, subject string, event *corev1.LiveEvent) error {
	if err := validateInstanceEvent(event); err != nil {
		return err
	}

	eventData, err := proto.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal instance event: %w", err)
	}

	if err := c.nc.Publish(subject, eventData); err != nil {
		return fmt.Errorf("failed to publish instance event to %s: %w", subject, err)
	}

	if err := c.nc.FlushTimeout(natsPublishFlushTimeout); err != nil {
		return fmt.Errorf("failed to flush instance event to %s: %w", subject, err)
	}
	return nil
}

// publishServerEventWithAck publishes a SpaceEvent using JetStream and returns the sequence ID.
// This uses synchronous JetStream publish (waits for ack) to get the sequence ID from the PubAck.
// Use this when you need to know the sequence ID immediately (e.g., for message body storage).
func (c *ChattoCore) publishServerEventWithAck(ctx context.Context, subject string, event *corev1.ServerEvent) (uint64, error) {
	if err := validateSpaceEvent(event); err != nil {
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
func (c *ChattoCore) publishServerEventWithOCC(ctx context.Context, spaceID, subject string, event *corev1.ServerEvent) (uint64, error) {
	if err := validateSpaceEvent(event); err != nil {
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

func validateSpaceEvent(event *corev1.ServerEvent) error {
	if event == nil || event.Event == nil {
		return fmt.Errorf("%w: space event payload is nil or oneof field is unset", ErrInvalidEvent)
	}
	return nil
}

func validateInstanceEvent(event *corev1.LiveEvent) error {
	if event == nil || event.Event == nil {
		return fmt.Errorf("%w: instance event payload is nil or oneof field is unset", ErrInvalidEvent)
	}
	return nil
}

// newServerEvent fills in the Id, ActorID, and CreatedAt fields of a SpaceEvent if they're not already set.
// The caller provides the event with the concrete event type already set.
func newServerEvent(actorID string, event *corev1.ServerEvent) *corev1.ServerEvent {
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

// newLiveEvent fills in the Id, ActorID, and CreatedAt fields of an InstanceEvent if they're not already set.
// The caller provides the event with the concrete event type already set.
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

// createSpaceResources is now a no-op: all data lives in the deployment-wide
// SERVER_* buckets (eager-created in newStorage). Kept as a stub so callers
// don't have to be edited until the broader Space-retirement pass.
func (c *ChattoCore) createSpaceResources(_ context.Context, _ string) error {
	return nil
}

// purgeRoomEvents removes all events for a specific room from the space stream.
// This is called when a room is deleted to clean up the room's event history.
func (c *ChattoCore) purgeRoomEvents(ctx context.Context, spaceID, roomID string) error {
	stream := c.storage.serverEventsStream

	// Purge all events matching the room's subject pattern
	subjectFilter := subjects.RoomAllEvents(kindForSpace(spaceID), roomID)
	if err := stream.Purge(ctx, jetstream.WithPurgeSubject(subjectFilter)); err != nil {
		return fmt.Errorf("failed to purge room events for %s (subject: %s): %w", roomID, subjectFilter, err)
	}

	c.logger.Debug("Purged room events from space stream", "space_id", spaceID, "room_id", roomID, "subject_filter", subjectFilter)

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

// StreamMyServerEvents creates a unified stream of all events on this
// deployment that are relevant to a specific user. Sources from the
// single SERVER_EVENTS stream (no per-space scoping); per-room
// authorization is applied per event.
//
// Includes:
//   - Server-level live events (member_deleted) via NATS Core
//   - Room events (messages, meta) via a JetStream ordered consumer
//   - Transient room events (reactions, typing, message updates) via NATS Core
//   - Presence changes via the per-process PresenceHub
//
// Authorization:
//   - Room events are delivered only for rooms where the user is a
//     member. The membership set is pre-loaded across both kinds
//     (channel + dm) and updated as join/leave events arrive.
//   - DM-kind events are additionally gated by the user's `dm.view`
//     permission, fetched once at subscription start.
//   - Presence updates are deployment-wide.
//
// The returned channel closes when the context is cancelled or after
// unrecoverable errors. Transient JetStream errors retry with backoff;
// terminal errors (connection closed, consumer deleted) close the channel.
func (c *ChattoCore) StreamMyServerEvents(ctx context.Context, userID string) (<-chan *corev1.ServerEvent, error) {
	stream := c.storage.serverEventsStream

	// Resolve dm.view once. DM-kind events are dropped for users without it,
	// and we skip pre-loading DM memberships entirely so the membership cache
	// stays consistent across the lifetime of the subscription.
	canDM, err := c.HasInstancePermission(ctx, userID, PermDMView)
	if err != nil {
		return nil, fmt.Errorf("failed to check dm.view permission: %w", err)
	}

	memberRooms := make(map[string]struct{})
	channelMemberships, err := c.GetUserRoomMemberships(ctx, "", userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get channel room memberships: %w", err)
	}
	for _, m := range channelMemberships {
		memberRooms[m.RoomId] = struct{}{}
	}
	if canDM {
		dmMemberships, err := c.GetUserRoomMemberships(ctx, DMSpaceID, userID)
		if err != nil {
			return nil, fmt.Errorf("failed to get DM room memberships: %w", err)
		}
		for _, m := range dmMemberships {
			memberRooms[m.RoomId] = struct{}{}
		}
	}

	// Subscribe to live server-level events via NATS Core (member_deleted)
	liveSubject := subjects.LiveMemberAllEvents()
	liveMsgChan := make(chan *nats.Msg, 64)
	liveSub, err := c.nc.ChanSubscribe(liveSubject, liveMsgChan)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to live server events: %w", err)
	}

	// Ordered consumer over the unified SERVER_EVENTS stream covering both
	// channel- and dm-kind subjects. Per-event authorization happens below.
	roomFilterSubjects := subjects.AllRoomEventsFiltersAnyKind()
	cons, err := stream.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{
		FilterSubjects:    roomFilterSubjects,
		DeliverPolicy:     jetstream.DeliverNewPolicy,
		InactiveThreshold: 30 * time.Second,
	})
	if err != nil {
		liveSub.Unsubscribe()
		return nil, fmt.Errorf("failed to create ordered consumer: %w", err)
	}

	// Subscribe to live room events via NATS Core (reactions, message updates/deletes, typing)
	liveRoomSubject := subjects.LiveRoomAllEventsAnyKind()
	liveRoomMsgChan := make(chan *nats.Msg, 64)
	liveRoomSub, err := c.nc.ChanSubscribe(liveRoomSubject, liveRoomMsgChan)
	if err != nil {
		liveSub.Unsubscribe()
		return nil, fmt.Errorf("failed to subscribe to live room events: %w", err)
	}

	// Subscribe to the per-process presence hub instead of creating a per-subscription KV watcher.
	presenceSub, err := c.PresenceHub.Subscribe(ctx)
	if err != nil {
		liveSub.Unsubscribe()
		liveRoomSub.Unsubscribe()
		return nil, fmt.Errorf("failed to subscribe to presence hub: %w", err)
	}

	eventChan := make(chan *corev1.ServerEvent)

	go func() {
		c.logger.Debug("Starting server event stream", "user_id", userID, "can_dm", canDM,
			"live_subject", liveSubject, "room_subjects", roomFilterSubjects, "live_room_subject", liveRoomSubject)

		defer func() {
			c.logger.Debug("Server event stream closed", "user_id", userID)
			liveSub.Unsubscribe()
			liveRoomSub.Unsubscribe()

			// Unsubscribe from presence hub
			c.PresenceHub.Unsubscribe(presenceSub)

			close(eventChan)
		}()

		// Create a channel for room events from JetStream
		roomMsgChan := make(chan jetstream.Msg, 64)
		jsReaderDone := make(chan struct{})

		// Track current iterator for cleanup
		var currentIter jetstream.MessagesContext
		var iterMu sync.Mutex

		// Start goroutine to read from JetStream with retry logic
		go func() {
			defer close(jsReaderDone)

			const maxRetries = 3
			retryCount := 0

			for {
				iter, err := cons.Messages()
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					c.logger.Error("Failed to get message iterator", "error", err)
					return
				}

				// Store iterator reference for external cleanup
				iterMu.Lock()
				currentIter = iter
				iterMu.Unlock()

				// Read messages until error
				for {
					msg, err := iter.Next()
					if err != nil {
						iter.Stop()

						if ctx.Err() != nil {
							return
						}

						// Terminal errors - cannot recover
						if isTerminalIteratorError(err) {
							c.logger.Debug("Iterator terminated", "error", err)
							return
						}

						// Recoverable error - retry with backoff
						retryCount++
						if retryCount > maxRetries {
							c.logger.Warn("Max retries exceeded for room message iterator", "error", err, "retries", retryCount)
							return
						}

						c.logger.Debug("Iterator error, retrying", "error", err, "retry", retryCount)
						select {
						case <-ctx.Done():
							return
						case <-time.After(time.Duration(retryCount) * 100 * time.Millisecond):
							// Continue to outer loop to create new iterator
						}
						break
					}

					// Success - reset retry count
					retryCount = 0

					select {
					case <-ctx.Done():
						iter.Stop()
						return
					case roomMsgChan <- msg:
					}
				}
			}
		}()

		// Goroutine to stop the iterator when context is cancelled
		go func() {
			<-ctx.Done()
			iterMu.Lock()
			if currentIter != nil {
				currentIter.Stop()
			}
			iterMu.Unlock()
		}()

		c.logger.Debug("Server subscription active", "user_id", userID, "member_rooms", len(memberRooms))

		// Initialize dedup map from hub snapshot (contains current presence state at subscribe time)
		lastKnownPresence := make(map[string]string, len(presenceSub.Snapshot))
		for k, v := range presenceSub.Snapshot {
			lastKnownPresence[k] = v
		}

		for {
			select {
			case <-ctx.Done():
				return

			case msg := <-liveMsgChan:
				// Server-level live event (member_deleted).
				// Room events (join/leave/create/delete) come through roomMsgChan (JetStream).
				var event corev1.ServerEvent
				if err := proto.Unmarshal(msg.Data, &event); err != nil {
					c.logger.Warn("Failed to unmarshal live event", "error", err)
					continue
				}

				select {
				case <-ctx.Done():
					return
				case eventChan <- &event:
				}

			case msg := <-roomMsgChan:
				// Room event from JetStream (messages, etc.)
				if !canDM && subjects.ParseKindFromRoomSubject(msg.Subject()) == "dm" {
					continue
				}

				var event corev1.ServerEvent
				if err := proto.Unmarshal(msg.Data(), &event); err != nil {
					c.logger.Warn("Failed to unmarshal room event", "error", err)
					continue
				}

				// Extract room ID and check membership
				roomID := subjects.ParseRoomIDFromSubject(msg.Subject())
				if roomID == "" {
					continue // Shouldn't happen, but skip if we can't parse
				}

				_, isMember := memberRooms[roomID]

				// Update membership cache for join/leave events targeting this user
				switch event.Event.(type) {
				case *corev1.ServerEvent_UserJoinedRoom:
					if event.ActorId == userID {
						memberRooms[roomID] = struct{}{}
						isMember = true
					}
				case *corev1.ServerEvent_UserLeftRoom:
					if event.ActorId == userID {
						delete(memberRooms, roomID)
						// Still deliver the leave event
					}
				case *corev1.ServerEvent_RoomDeleted:
					delete(memberRooms, roomID)
				}

				if isMember {
					select {
					case <-ctx.Done():
						return
					case eventChan <- &event:
					}
				}

			case msg := <-liveRoomMsgChan:
				// Live room event from NATS Core (reactions, message updates/deletes)
				// These events are published directly via publishLiveServerEvent(), bypassing JetStream.
				if !canDM && subjects.ParseKindFromRoomSubject(msg.Subject) == "dm" {
					continue
				}

				var event corev1.ServerEvent
				if err := proto.Unmarshal(msg.Data, &event); err != nil {
					c.logger.Warn("Failed to unmarshal live room event", "error", err)
					continue
				}

				// Extract room ID from the event
				var roomID string
				switch e := event.Event.(type) {
				case *corev1.ServerEvent_ReactionAdded:
					roomID = e.ReactionAdded.RoomId
				case *corev1.ServerEvent_ReactionRemoved:
					roomID = e.ReactionRemoved.RoomId
				case *corev1.ServerEvent_MessageDeleted:
					roomID = e.MessageDeleted.RoomId
				case *corev1.ServerEvent_MessageUpdated:
					roomID = e.MessageUpdated.RoomId
				case *corev1.ServerEvent_UserTyping:
					// Skip own typing events — the sender doesn't need to see them.
					// Critical for multi-instance clients where the frontend's
					// currentUserId may differ from the remote instance user ID.
					if event.ActorId == userID {
						continue
					}
					roomID = e.UserTyping.RoomId
				case *corev1.ServerEvent_VideoProcessingCompleted:
					roomID = e.VideoProcessingCompleted.RoomId
				case *corev1.ServerEvent_CallParticipantJoined:
					roomID = e.CallParticipantJoined.RoomId
				case *corev1.ServerEvent_CallParticipantLeft:
					roomID = e.CallParticipantLeft.RoomId
				}

				if roomID == "" {
					continue // Skip if we can't get room ID
				}

				// Check room membership via cache
				_, isMember := memberRooms[roomID]
				if isMember {
					select {
					case <-ctx.Done():
						return
					case eventChan <- &event:
					}
				}

			case update := <-presenceSub.C:
				// Single-server deployment: every authenticated user is a member.
				// No per-space membership filter is needed.

				// Skip if status hasn't changed (dedup heartbeat refreshes)
				if lastStatus, exists := lastKnownPresence[update.UserID]; exists && lastStatus == update.Status {
					continue
				}

				// Update tracking map
				if update.Status == PresenceStatusOffline {
					delete(lastKnownPresence, update.UserID)
				} else {
					lastKnownPresence[update.UserID] = update.Status
				}

				// Create PresenceChangedEvent
				presenceEvent := &corev1.ServerEvent{
					CreatedAt: timestamppb.Now(),
					ActorId:   update.UserID,
					Event: &corev1.ServerEvent_PresenceChanged{
						PresenceChanged: &corev1.PresenceChangedEvent{
							Status: update.Status,
						},
					},
				}

				select {
				case <-ctx.Done():
					return
				case eventChan <- presenceEvent:
				}
			}
		}
	}()

	return eventChan, nil
}

// StreamMyLiveEvents creates a live stream of instance-level events
// relevant to a specific user. Subscribes to live.instance.> and performs
// server-side authorization filtering to deliver only events the user is
// authorized to see:
//   - User events (instance.user.{userId}.*): Only if userId matches subscriber
//   - Space events (instance.space.{spaceId}.*): Only if user is a space member
//
// Only delivers new events that occur after subscription starts.
// The returned channel will be closed when the context is cancelled.
func (c *ChattoCore) StreamMyLiveEvents(ctx context.Context, userID string) (<-chan *corev1.LiveEvent, error) {
	// Subscribe to all live instance events via NATS Core
	liveSubject := subjects.LiveInstanceAllEvents()
	msgChan := make(chan *nats.Msg, 64)
	sub, err := c.nc.ChanSubscribe(liveSubject, msgChan)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to live instance events: %w", err)
	}

	eventChan := make(chan *corev1.LiveEvent)

	go func() {
		c.logger.Debug("Starting instance event stream (NATS Core)", "user_id", userID, "subject", liveSubject)

		// Set initial presence — subscribing to instance events means the client is online
		if err := c.SetPresence(ctx, userID, PresenceStatusOnline); err != nil {
			c.logger.Warn("Failed to set initial presence", "error", err, "user_id", userID)
		}

		// Presence refresh ticker: refresh every 30s to maintain the 60s TTL
		presenceTicker := time.NewTicker(PresenceRefreshInterval)
		defer presenceTicker.Stop()

		defer func() {
			c.logger.Debug("Instance event stream closed", "user_id", userID)
			sub.Unsubscribe()
			close(eventChan)
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case <-presenceTicker.C:
				// Refresh presence TTL, preserving whatever status the client set via updateMyPresence
				if err := c.refreshPresence(ctx, userID); err != nil {
					c.logger.Warn("Failed to refresh presence", "error", err, "user_id", userID)
				}
			case msg := <-msgChan:
				var event corev1.LiveEvent
				if err := proto.Unmarshal(msg.Data, &event); err != nil {
					c.logger.Warn("Failed to unmarshal event", "error", err)
					continue
				}

				// NewMessageInSpaceEvent authorizes on ROOM membership instead of
				// space membership. This is the correct check for both regular space
				// rooms (room membership implies relevance) and DM rooms (whose
				// participants have a room membership but no space membership of the
				// hidden DM space — so the subject-based space-membership filter
				// below would otherwise drop these events). Treat the room-membership
				// check as the auth decision and skip the subject filter.
				if newMsg := event.GetNewMessageInSpace(); newMsg != nil {
					isMember, err := c.RoomMembershipExists(ctx, newMsg.SpaceId, userID, newMsg.RoomId)
					if err != nil {
						c.logger.Warn("Failed to check room membership for event filtering",
							"error", err, "user_id", userID, "room_id", newMsg.RoomId)
						continue
					}
					if !isMember {
						continue // Skip - user is not a room member
					}
				} else if !c.isAuthorizedForLiveEvent(ctx, userID, msg.Subject) {
					// Server-side authorization filtering based on subject pattern
					// Subject format: live.instance.{type}.{id}.{eventType}
					// - live.instance.user.{userId}.* → only forward to that user
					// - live.instance.space.{spaceId}.* → only forward to space members
					continue
				}

				// Note: No sequence ID available from NATS Core messages
				// The event may already have a sequence ID from the original publish

				select {
				case <-ctx.Done():
					return
				case eventChan <- &event:
					// If this was a session termination event, close the stream.
					// The frontend will receive the event and handle logout;
					// closing the channel ensures the server also tears down the subscription.
					if event.GetSessionTerminated() != nil {
						c.logger.Info("Session terminated - closing instance event stream", "user_id", userID)
						return
					}
				}
			}
		}
	}()

	return eventChan, nil
}

// isAuthorizedForLiveEvent checks if a user is authorized to receive an instance event
// based on the subject pattern:
//   - live.instance.config.* → all authenticated users (instance config is public)
//   - live.instance.user.{userId}.* → only the specific user (except profile_updated)
//   - live.instance.user.{userId}.profile_updated → broadcast to all (profiles are public)
//   - live.instance.space.{spaceId}.* → only space members
func (c *ChattoCore) isAuthorizedForLiveEvent(ctx context.Context, userID, subject string) bool {
	// Parse subject: live.instance.{type}.{id}.{eventType}
	parts := strings.Split(subject, ".")
	if len(parts) < 4 || parts[0] != "live" || parts[1] != "instance" {
		c.logger.Warn("Invalid instance event subject format", "subject", subject)
		return false
	}

	eventScope := parts[2] // "user", "space", or "config"

	// Config events are visible to all authenticated users
	if eventScope == "config" {
		return true
	}

	// For user/space scopes, we need at least 5 parts
	if len(parts) < 5 {
		c.logger.Warn("Invalid instance event subject format", "subject", subject)
		return false
	}

	scopeID := parts[3]   // userId or spaceId
	eventType := parts[4] // e.g., "profile_updated", "registration_completed"

	switch eventScope {
	case "user":
		// Profile updates are broadcast to all authenticated users (profiles are public)
		if eventType == "profile_updated" {
			return true
		}
		// Other user events: only forward to the target user
		return scopeID == userID
	case "space":
		// Space events: every authenticated user is implicitly a server member,
		// so deliver to anyone connected. The subscription itself is auth-gated.
		return true
	default:
		c.logger.Warn("Unknown instance event scope", "scope", eventScope, "subject", subject)
		return false
	}
}

// StreamMyServerConfigEvents streams transient instance-level events to the user.
// These are fire-and-forget events that bypass JetStream (config changes, etc.).
// The returned channel will be closed when the context is cancelled.
func (c *ChattoCore) StreamMyServerConfigEvents(ctx context.Context, userID string) (<-chan *corev1.LiveEvent, error) {
	// Subscribe to live instance config events via NATS Core
	liveSubject := subjects.LiveInstanceConfigAllEvents()
	msgChan := make(chan *nats.Msg, 64)
	sub, err := c.nc.ChanSubscribe(liveSubject, msgChan)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to live instance config events: %w", err)
	}

	eventChan := make(chan *corev1.LiveEvent)

	go func() {
		c.logger.Debug("Starting instance live event stream", "user_id", userID, "subject", liveSubject)

		defer func() {
			c.logger.Debug("Instance live event stream closed", "user_id", userID)
			sub.Unsubscribe()
			close(eventChan)
		}()

		for {
			select {
			case <-ctx.Done():
				return
			case msg := <-msgChan:
				var event corev1.LiveEvent
				if err := proto.Unmarshal(msg.Data, &event); err != nil {
					c.logger.Warn("Failed to unmarshal instance live event", "error", err)
					continue
				}

				// Instance config events are visible to all authenticated users
				// No additional authorization filtering needed

				select {
				case <-ctx.Done():
					return
				case eventChan <- &event:
				}
			}
		}
	}()

	return eventChan, nil
}

// PublishInstanceConfigUpdated publishes an instance config update event.
// This notifies all connected clients that the instance configuration has changed.
func (c *ChattoCore) PublishInstanceConfigUpdated(ctx context.Context, actorID string, instanceName, motd, welcomeMessage, blockedUsernames string) error {
	event := newLiveEvent(actorID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_ConfigUpdated{
			ConfigUpdated: &corev1.ServerConfigUpdatedEvent{
				ServerName:       instanceName,
				Motd:             motd,
				WelcomeMessage:   welcomeMessage,
				BlockedUsernames: blockedUsernames,
			},
		},
	})

	return c.publishLiveEvent(ctx, subjects.LiveInstanceConfigUpdated(), event)
}

// ============================================================================
// Statistics
// ============================================================================

// InstanceStats contains aggregate statistics about the Chatto instance.
type InstanceStats struct {
	UserCount    int
	SpaceCount   int
	RoomCount    int
	MessageCount uint64
}

// GetStats returns aggregate statistics for the Chatto instance.
// This includes counts of users, spaces, rooms, and total room events across all spaces.
func (c *ChattoCore) GetStats(ctx context.Context) (*InstanceStats, error) {
	stats := &InstanceStats{}

	// Count users
	userKeys, err := c.storage.instanceKV.ListKeysFiltered(ctx, "user.*")
	if err != nil {
		return nil, fmt.Errorf("failed to list user keys: %w", err)
	}
	for range userKeys.Keys() {
		stats.UserCount++
	}

	// Count spaces and rooms
	spaces, err := c.ListSpaces(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list spaces: %w", err)
	}
	stats.SpaceCount = len(spaces)

	// For each space, count rooms and messages
	for _, space := range spaces {
		rooms, err := c.ListRoomsBySpace(ctx, space.Id)
		if err != nil {
			c.logger.Warn("Failed to list rooms for space", "space_id", space.Id, "error", err)
			continue
		}
		stats.RoomCount += len(rooms)

		// Count room events in the unified space stream
		stream := c.storage.serverEventsStream
		// Get subject-filtered info for room events
		roomSubjectFilter := subjects.AllRoomEvents(kindForSpace(space.Id))
		info, err := stream.Info(ctx, jetstream.WithSubjectFilter(roomSubjectFilter))
		if err != nil {
			continue
		}
		stats.MessageCount += info.State.Msgs
	}

	return stats, nil
}

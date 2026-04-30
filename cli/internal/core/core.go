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
	"hmans.de/chatto/pkg/lazycache"
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
	ensuredStreams       sync.Map // tracks which space streams have been ensured this process lifetime
	instanceRBACEngine   *rbac.Engine
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

// spaceRBACEngine returns the rbac.Engine for a space.
// Engines are cached per space for performance.
func (c *ChattoCore) spaceRBACEngine(ctx context.Context, spaceID string) (*rbac.Engine, error) {
	return c.getSpaceRBACEngine(ctx, spaceID)
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

	// Initialize instance RBAC engine with virtual roles
	// Owner, admin, and moderator are explicitly created in KV; everyone is virtual
	instanceRBACEngine := rbac.NewEngine(storage.instanceRBACKV, rbac.Config{
		SystemRoles:  []string{InstRoleOwner, InstRoleAdmin, InstRoleModerator, InstRoleEveryone},
		AdminRole:    InstRoleOwner, // Owner is the top admin role for instance
		VirtualRoles: InstanceVirtualRoles(),
		ValidateVerbObjectType: func(verb, objectType string) error {
			perm := ReconstructPermission(verb, objectType)
			if perm == "" {
				return fmt.Errorf("%w: verb=%s, objectType=%s", ErrInvalidPermission, verb, objectType)
			}
			return nil
		},
		Logger: slog.Default().With("component", "instance-rbac"),
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
		instanceRBACEngine: instanceRBACEngine,
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
	instanceRBACKV   jetstream.KeyValue // Instance-level roles and permissions
	instanceConfigKV jetstream.KeyValue // Runtime configuration overrides

	spaceConfigKV    *lazycache.Cache[jetstream.KeyValue]      // SPACE_{id}_CONFIG - rooms, memberships
	spaceRuntimeKV   *lazycache.Cache[jetstream.KeyValue]      // SPACE_{id}_RUNTIME - sequences, timestamps, read status
	spaceRBACKV      *lazycache.Cache[jetstream.KeyValue]      // SPACE_{id}_RBAC - roles, permissions, assignments
	spaceRBACEngines *lazycache.Cache[*rbac.Engine]            // Cached rbac.Engine instances per space
	bodiesKV         *lazycache.Cache[jetstream.KeyValue]      // SPACE_{id}_BODIES - message bodies
	attachments      *lazycache.Cache[jetstream.ObjectStore]   // SPACE_{id}_ASSETS - message attachments
	reactionsKV      *lazycache.Cache[jetstream.KeyValue]      // SPACE_{id}_REACTIONS - emoji reactions
	threadsKV        *lazycache.Cache[jetstream.KeyValue]      // SPACE_{id}_THREADS - thread metadata
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

	// Initialize instance-level RBAC KV bucket
	instanceRBACKV, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      "INSTANCE_RBAC",
		Description: "Instance-level roles and permissions",
		Storage:     jetstream.FileStorage,
		Replicas:    cfg.Replicas,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create INSTANCE_RBAC KV bucket: %w", err)
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
		instanceRBACKV:   instanceRBACKV,
		instanceConfigKV: instanceConfigKV,
		spaceConfigKV:    lazycache.New[jetstream.KeyValue](),
		spaceRuntimeKV:   lazycache.New[jetstream.KeyValue](),
		spaceRBACKV:      lazycache.New[jetstream.KeyValue](),
		spaceRBACEngines: lazycache.New[*rbac.Engine](),
		bodiesKV:         lazycache.New[jetstream.KeyValue](),
		attachments:      lazycache.New[jetstream.ObjectStore](),
		reactionsKV:      lazycache.New[jetstream.KeyValue](),
		threadsKV:        lazycache.New[jetstream.KeyValue](),
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

// roomKey returns the KV key for a room record in a space bucket.
func roomKey(roomID string) string {
	return fmt.Sprintf("room.%s", roomID)
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
// Per-Space Bucket Accessors
// ============================================================================

// getSpaceConfigKV retrieves or creates the CONFIG bucket for a space.
// The bucket contains structural data (rooms, memberships).
func (c *ChattoCore) getSpaceConfigKV(ctx context.Context, spaceID string) (jetstream.KeyValue, error) {
	return c.storage.spaceConfigKV.GetOrCreate(spaceID, func() (jetstream.KeyValue, error) {
		if _, err := c.GetSpace(ctx, spaceID); err != nil {
			return nil, fmt.Errorf("space %s does not exist: %w", spaceID, err)
		}
		bucket, err := c.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket:      fmt.Sprintf("SPACE_%s_CONFIG", spaceID),
			Description: fmt.Sprintf("Configuration (rooms, memberships) for space %s", spaceID),
			Storage:     jetstream.FileStorage,
			Replicas:    c.config.Replicas,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create config bucket for space %s: %w", spaceID, err)
		}
		c.logger.Debug("Created/updated space config bucket", "bucket", fmt.Sprintf("SPACE_%s_CONFIG", spaceID), "space_id", spaceID)
		return bucket, nil
	})
}

// getSpaceRBACKV retrieves or creates the RBAC bucket for a space.
// The bucket contains roles, permissions, and assignments.
func (c *ChattoCore) getSpaceRBACKV(ctx context.Context, spaceID string) (jetstream.KeyValue, error) {
	return c.storage.spaceRBACKV.GetOrCreate(spaceID, func() (jetstream.KeyValue, error) {
		if _, err := c.GetSpace(ctx, spaceID); err != nil {
			return nil, fmt.Errorf("space %s does not exist: %w", spaceID, err)
		}
		bucket, err := c.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket:      fmt.Sprintf("SPACE_%s_RBAC", spaceID),
			Description: fmt.Sprintf("RBAC (roles, permissions, assignments) for space %s", spaceID),
			Storage:     jetstream.FileStorage,
			Replicas:    c.config.Replicas,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create RBAC bucket for space %s: %w", spaceID, err)
		}
		c.logger.Debug("Created/updated space RBAC bucket", "bucket", fmt.Sprintf("SPACE_%s_RBAC", spaceID), "space_id", spaceID)
		return bucket, nil
	})
}

// getSpaceRBACEngine retrieves or creates an rbac.Engine for a space.
// Uses the space's RBAC bucket for storage.
func (c *ChattoCore) getSpaceRBACEngine(ctx context.Context, spaceID string) (*rbac.Engine, error) {
	return c.storage.spaceRBACEngines.GetOrCreate(spaceID, func() (*rbac.Engine, error) {
		kv, err := c.getSpaceRBACKV(ctx, spaceID)
		if err != nil {
			return nil, fmt.Errorf("failed to get space RBAC bucket: %w", err)
		}
		engine := rbac.NewEngine(kv, rbac.Config{
			SystemRoles:  []string{SpaceRoleOwner, SpaceRoleModerator, SpaceRoleEveryone},
			AdminRole:    SpaceRoleOwner,
			VirtualRoles: SpaceVirtualRoles(),
			ValidateVerbObjectType: func(verb, objectType string) error {
				perm := ReconstructPermission(verb, objectType)
				if perm == "" {
					return fmt.Errorf("%w: verb=%s, objectType=%s", ErrInvalidPermission, verb, objectType)
				}
				return nil
			},
			Logger: slog.Default().With("component", "space-rbac", "space_id", spaceID),
		})
		c.logger.Debug("Created space RBAC engine", "space_id", spaceID)
		return engine, nil
	})
}

// getSpaceRuntimeKV retrieves or creates the RUNTIME bucket for a space.
// The bucket contains transient state (sequences, timestamps, read status).
func (c *ChattoCore) getSpaceRuntimeKV(ctx context.Context, spaceID string) (jetstream.KeyValue, error) {
	return c.storage.spaceRuntimeKV.GetOrCreate(spaceID, func() (jetstream.KeyValue, error) {
		if _, err := c.GetSpace(ctx, spaceID); err != nil {
			return nil, fmt.Errorf("space %s does not exist: %w", spaceID, err)
		}
		bucket, err := c.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket:      fmt.Sprintf("SPACE_%s_RUNTIME", spaceID),
			Description: fmt.Sprintf("Runtime state (sequences, read status) for space %s", spaceID),
			Storage:     jetstream.FileStorage,
			Replicas:    c.config.Replicas,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create runtime bucket for space %s: %w", spaceID, err)
		}
		c.logger.Debug("Created/updated space runtime bucket", "bucket", fmt.Sprintf("SPACE_%s_RUNTIME", spaceID), "space_id", spaceID)
		return bucket, nil
	})
}

// getSpaceBodiesKV retrieves or creates the BODIES bucket for a space.
// The bucket stores message bodies, separated from the main space bucket for
// performance, scaling, and operational flexibility.
func (c *ChattoCore) getSpaceBodiesKV(ctx context.Context, spaceID string) (jetstream.KeyValue, error) {
	return c.storage.bodiesKV.GetOrCreate(spaceID, func() (jetstream.KeyValue, error) {
		if _, err := c.GetSpace(ctx, spaceID); err != nil {
			return nil, fmt.Errorf("space %s does not exist: %w", spaceID, err)
		}
		bucket, err := c.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket:      fmt.Sprintf("SPACE_%s_BODIES", spaceID),
			Description: fmt.Sprintf("Message bodies for space %s", spaceID),
			Storage:     jetstream.FileStorage,
			Replicas:    c.config.Replicas,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create bodies bucket for space %s: %w", spaceID, err)
		}
		c.logger.Debug("Created/updated bodies bucket", "bucket", fmt.Sprintf("SPACE_%s_BODIES", spaceID), "space_id", spaceID)
		return bucket, nil
	})
}

// getSpaceAttachments retrieves or creates the ASSETS ObjectStore for a space.
// The bucket stores message attachment binaries with S2 compression.
func (c *ChattoCore) getSpaceAttachments(ctx context.Context, spaceID string) (jetstream.ObjectStore, error) {
	return c.storage.attachments.GetOrCreate(spaceID, func() (jetstream.ObjectStore, error) {
		if _, err := c.GetSpace(ctx, spaceID); err != nil {
			return nil, fmt.Errorf("space %s does not exist: %w", spaceID, err)
		}
		store, err := c.js.CreateOrUpdateObjectStore(ctx, jetstream.ObjectStoreConfig{
			Bucket:      fmt.Sprintf("SPACE_%s_ASSETS", spaceID),
			Description: fmt.Sprintf("Message attachments for space %s", spaceID),
			Storage:     jetstream.FileStorage,
			Compression: true,
			Replicas:    c.config.Replicas,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create attachments store for space %s: %w", spaceID, err)
		}
		c.logger.Debug("Created/updated attachments store", "bucket", fmt.Sprintf("SPACE_%s_ASSETS", spaceID), "space_id", spaceID)
		return store, nil
	})
}

// getSpaceReactionsKV retrieves or creates the REACTIONS bucket for a space.
// The bucket stores emoji reactions to messages.
func (c *ChattoCore) getSpaceReactionsKV(ctx context.Context, spaceID string) (jetstream.KeyValue, error) {
	return c.storage.reactionsKV.GetOrCreate(spaceID, func() (jetstream.KeyValue, error) {
		if _, err := c.GetSpace(ctx, spaceID); err != nil {
			return nil, fmt.Errorf("space %s does not exist: %w", spaceID, err)
		}
		bucket, err := c.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket:      fmt.Sprintf("SPACE_%s_REACTIONS", spaceID),
			Description: fmt.Sprintf("Emoji reactions for space %s", spaceID),
			Storage:     jetstream.FileStorage,
			Replicas:    c.config.Replicas,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create reactions bucket for space %s: %w", spaceID, err)
		}
		c.logger.Debug("Created/updated reactions bucket", "bucket", fmt.Sprintf("SPACE_%s_REACTIONS", spaceID), "space_id", spaceID)
		return bucket, nil
	})
}

// getSpaceThreadsKV retrieves or creates the THREADS bucket for a space.
// The bucket contains thread metadata (reply count, last reply time, participants).
func (c *ChattoCore) getSpaceThreadsKV(ctx context.Context, spaceID string) (jetstream.KeyValue, error) {
	return c.storage.threadsKV.GetOrCreate(spaceID, func() (jetstream.KeyValue, error) {
		if _, err := c.GetSpace(ctx, spaceID); err != nil {
			return nil, fmt.Errorf("space %s does not exist: %w", spaceID, err)
		}
		bucket, err := c.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket:      fmt.Sprintf("SPACE_%s_THREADS", spaceID),
			Description: fmt.Sprintf("Thread metadata for space %s", spaceID),
			Storage:     jetstream.FileStorage,
			Replicas:    c.config.Replicas,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create threads bucket for space %s: %w", spaceID, err)
		}
		c.logger.Debug("Created/updated threads bucket", "bucket", fmt.Sprintf("SPACE_%s_THREADS", spaceID), "space_id", spaceID)
		return bucket, nil
	})
}

// ============================================================================
// Event Publishing Helpers
// ============================================================================

// natsPublishFlushTimeout bounds how long a fire-and-forget publish will wait
// for the NATS server to acknowledge buffered bytes. Without a timeout, a
// hung server (e.g. network partition) would block the calling goroutine
// indefinitely instead of surfacing as a normal error.
const natsPublishFlushTimeout = 5 * time.Second

// publishSpaceEvent publishes a SpaceEvent to NATS via the provided subject.
// Streams automatically capture events based on their subject filters.
// Uses NATS Core publish (fire-and-forget) rather than JetStream publish (which waits for acks).
// Handles marshaling internally for consistent error handling.
func (c *ChattoCore) publishSpaceEvent(_ context.Context, subject string, event *corev1.SpaceEvent) error {
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

// publishLiveSpaceEvent publishes a SpaceEvent directly to a live.> subject, bypassing JetStream storage.
// Use this for transient space-scoped notifications that don't need to be stored or replayed.
// The subject should already include the "live." prefix.
func (c *ChattoCore) publishLiveSpaceEvent(_ context.Context, subject string, event *corev1.SpaceEvent) error {
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

// publishInstanceEvent publishes an InstanceEvent directly to a live.instance.> subject, bypassing JetStream storage.
// Use this for instance-scoped notifications (user events, space lifecycle, config updates).
// The subject should already include the "live.instance." prefix.
func (c *ChattoCore) publishInstanceEvent(_ context.Context, subject string, event *corev1.InstanceEvent) error {
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

// publishSpaceEventWithAck publishes a SpaceEvent using JetStream and returns the sequence ID.
// This uses synchronous JetStream publish (waits for ack) to get the sequence ID from the PubAck.
// Use this when you need to know the sequence ID immediately (e.g., for message body storage).
func (c *ChattoCore) publishSpaceEventWithAck(ctx context.Context, subject string, event *corev1.SpaceEvent) (uint64, error) {
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

// publishSpaceEventWithOCC publishes a SpaceEvent to a space stream using Optimistic Concurrency Control.
// It uses the Nats-Expected-Last-Subject-Sequence header to ensure that:
// 1. We know the current state of the subject before publishing
// 2. Concurrent publishes to the same subject are detected and retried
//
// This provides reliable message posting that handles race conditions gracefully.
// The function retries up to 5 times on sequence mismatch errors with exponential backoff.
func (c *ChattoCore) publishSpaceEventWithOCC(ctx context.Context, spaceID, subject string, event *corev1.SpaceEvent) (uint64, error) {
	if err := validateSpaceEvent(event); err != nil {
		return 0, err
	}

	eventData, err := proto.Marshal(event)
	if err != nil {
		return 0, fmt.Errorf("failed to marshal event: %w", err)
	}

	stream, err := c.getSpaceStream(ctx, spaceID)
	if err != nil {
		return 0, fmt.Errorf("failed to get space stream: %w", err)
	}

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

func validateSpaceEvent(event *corev1.SpaceEvent) error {
	if event == nil || event.Event == nil {
		return fmt.Errorf("%w: space event payload is nil or oneof field is unset", ErrInvalidEvent)
	}
	return nil
}

func validateInstanceEvent(event *corev1.InstanceEvent) error {
	if event == nil || event.Event == nil {
		return fmt.Errorf("%w: instance event payload is nil or oneof field is unset", ErrInvalidEvent)
	}
	return nil
}

// newSpaceEvent fills in the Id, ActorID, and CreatedAt fields of a SpaceEvent if they're not already set.
// The caller provides the event with the concrete event type already set.
func newSpaceEvent(actorID string, event *corev1.SpaceEvent) *corev1.SpaceEvent {
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

// newInstanceEvent fills in the Id, ActorID, and CreatedAt fields of an InstanceEvent if they're not already set.
// The caller provides the event with the concrete event type already set.
func newInstanceEvent(actorID string, event *corev1.InstanceEvent) *corev1.InstanceEvent {
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

// ensureSpaceStream creates or updates the SPACE_{spaceId}_EVENTS stream.
// Uses sync.Map caching to avoid redundant CreateOrUpdateStream calls within a process lifetime.
func (c *ChattoCore) ensureSpaceStream(ctx context.Context, spaceID string) error {
	// Check if already ensured this process lifetime
	if _, ok := c.ensuredStreams.Load(spaceID); ok {
		return nil
	}

	streamName := fmt.Sprintf("SPACE_%s_EVENTS", spaceID)
	subjectPattern := fmt.Sprintf("space.%s.>", spaceID)

	// Note: No RePublish here. Live events (reactions, message updates/deletes, member_deleted)
	// are published directly to live.> subjects via publishLiveSpaceEvent()/publishInstanceEvent().
	_, err := c.js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:               streamName,
		Description:        fmt.Sprintf("Events for space %s", spaceID),
		Subjects:           []string{subjectPattern},
		Storage:            jetstream.FileStorage,
		Compression:        jetstream.S2Compression,
		AllowAtomicPublish: true,
		Replicas:           c.config.Replicas,
	})
	if err != nil {
		return fmt.Errorf("failed to create space stream: %w", err)
	}

	c.ensuredStreams.Store(spaceID, struct{}{})
	c.logger.Debug("Ensured space stream", "stream", streamName, "space_id", spaceID)

	return nil
}

// deleteSpaceStream deletes the SPACE_{spaceId}_EVENTS stream.
// This is called when a space is deleted to clean up orphaned streams.
func (c *ChattoCore) deleteSpaceStream(ctx context.Context, spaceID string) error {
	streamName := fmt.Sprintf("SPACE_%s_EVENTS", spaceID)
	err := c.js.DeleteStream(ctx, streamName)
	if err != nil {
		return fmt.Errorf("failed to delete stream %s: %w", streamName, err)
	}

	c.ensuredStreams.Delete(spaceID)
	c.logger.Debug("Deleted space stream", "stream", streamName, "space_id", spaceID)

	return nil
}

// createSpaceResources creates all NATS KV buckets and object stores for a space.
// Called during space creation to eagerly initialize all resources.
// If creation fails partway, cleans up any successfully created resources.
func (c *ChattoCore) createSpaceResources(ctx context.Context, spaceID string) error {
	// Track created resources for cleanup on failure
	var createdBuckets []string

	cleanup := func() {
		for _, bucketName := range createdBuckets {
			if err := c.js.DeleteKeyValue(ctx, bucketName); err != nil {
				c.logger.Warn("Failed to cleanup bucket during rollback", "bucket", bucketName, "error", err)
			}
		}
	}

	// Create CONFIG bucket
	configBucketName := fmt.Sprintf("SPACE_%s_CONFIG", spaceID)
	configBucket, err := c.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      configBucketName,
		Description: fmt.Sprintf("Configuration (rooms, memberships) for space %s", spaceID),
		Storage:     jetstream.FileStorage,
		Replicas:    c.config.Replicas,
	})
	if err != nil {
		return fmt.Errorf("failed to create CONFIG bucket: %w", err)
	}
	createdBuckets = append(createdBuckets, configBucketName)

	// Create RBAC bucket (roles, permissions, assignments)
	rbacBucketName := fmt.Sprintf("SPACE_%s_RBAC", spaceID)
	rbacBucket, err := c.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      rbacBucketName,
		Description: fmt.Sprintf("RBAC (roles, permissions, assignments) for space %s", spaceID),
		Storage:     jetstream.FileStorage,
		Replicas:    c.config.Replicas,
	})
	if err != nil {
		cleanup()
		return fmt.Errorf("failed to create RBAC bucket: %w", err)
	}
	createdBuckets = append(createdBuckets, rbacBucketName)

	// Create RUNTIME bucket
	runtimeBucketName := fmt.Sprintf("SPACE_%s_RUNTIME", spaceID)
	runtimeBucket, err := c.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      runtimeBucketName,
		Description: fmt.Sprintf("Runtime state (sequences, read status) for space %s", spaceID),
		Storage:     jetstream.FileStorage,
		Replicas:    c.config.Replicas,
	})
	if err != nil {
		cleanup()
		return fmt.Errorf("failed to create RUNTIME bucket: %w", err)
	}
	createdBuckets = append(createdBuckets, runtimeBucketName)

	// Create BODIES bucket
	bodiesBucketName := fmt.Sprintf("SPACE_%s_BODIES", spaceID)
	bodiesBucket, err := c.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      bodiesBucketName,
		Description: fmt.Sprintf("Message bodies for space %s", spaceID),
		Storage:     jetstream.FileStorage,
		Replicas:    c.config.Replicas,
	})
	if err != nil {
		cleanup()
		return fmt.Errorf("failed to create BODIES bucket: %w", err)
	}
	createdBuckets = append(createdBuckets, bodiesBucketName)

	// Create REACTIONS bucket
	reactionsBucketName := fmt.Sprintf("SPACE_%s_REACTIONS", spaceID)
	reactionsBucket, err := c.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      reactionsBucketName,
		Description: fmt.Sprintf("Emoji reactions for space %s", spaceID),
		Storage:     jetstream.FileStorage,
		Replicas:    c.config.Replicas,
	})
	if err != nil {
		cleanup()
		return fmt.Errorf("failed to create REACTIONS bucket: %w", err)
	}
	createdBuckets = append(createdBuckets, reactionsBucketName)

	// Create THREADS bucket
	threadsBucketName := fmt.Sprintf("SPACE_%s_THREADS", spaceID)
	threadsBucket, err := c.js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:      threadsBucketName,
		Description: fmt.Sprintf("Thread metadata for space %s", spaceID),
		Storage:     jetstream.FileStorage,
		Replicas:    c.config.Replicas,
	})
	if err != nil {
		cleanup()
		return fmt.Errorf("failed to create THREADS bucket: %w", err)
	}
	createdBuckets = append(createdBuckets, threadsBucketName)

	// Create ASSETS object store
	assetsBucketName := fmt.Sprintf("SPACE_%s_ASSETS", spaceID)
	assetsStore, err := c.js.CreateOrUpdateObjectStore(ctx, jetstream.ObjectStoreConfig{
		Bucket:      assetsBucketName,
		Description: fmt.Sprintf("Message attachments for space %s", spaceID),
		Storage:     jetstream.FileStorage,
		Compression: true,
		Replicas:    c.config.Replicas,
	})
	if err != nil {
		cleanup()
		return fmt.Errorf("failed to create ASSETS store: %w", err)
	}

	// Populate caches with the newly created resources
	c.storage.spaceConfigKV.Set(spaceID, configBucket)
	c.storage.spaceRBACKV.Set(spaceID, rbacBucket)
	c.storage.spaceRuntimeKV.Set(spaceID, runtimeBucket)
	c.storage.bodiesKV.Set(spaceID, bodiesBucket)
	c.storage.reactionsKV.Set(spaceID, reactionsBucket)
	c.storage.threadsKV.Set(spaceID, threadsBucket)
	c.storage.attachments.Set(spaceID, assetsStore)

	c.logger.Debug("Created all space resources", "space_id", spaceID)
	return nil
}

// getSpaceStream returns the stream for a given space, creating it if needed.
// Used by consumers that need to subscribe to space or room events.
// Room events are stored in the space stream with subjects like space.{spaceId}.room.{roomId}.>
func (c *ChattoCore) getSpaceStream(ctx context.Context, spaceID string) (jetstream.Stream, error) {
	// Lazily ensure stream exists with current config
	if err := c.ensureSpaceStream(ctx, spaceID); err != nil {
		return nil, err
	}

	streamName := fmt.Sprintf("SPACE_%s_EVENTS", spaceID)
	stream, err := c.js.Stream(ctx, streamName)
	if err != nil {
		return nil, fmt.Errorf("failed to get stream %s: %w", streamName, err)
	}
	return stream, nil
}

// purgeRoomEvents removes all events for a specific room from the space stream.
// This is called when a room is deleted to clean up the room's event history.
func (c *ChattoCore) purgeRoomEvents(ctx context.Context, spaceID, roomID string) error {
	stream, err := c.getSpaceStream(ctx, spaceID)
	if err != nil {
		return fmt.Errorf("failed to get space stream: %w", err)
	}

	// Purge all events matching the room's subject pattern
	subjectFilter := subjects.SpaceRoomAllEvents(spaceID, roomID)
	err = stream.Purge(ctx, jetstream.WithPurgeSubject(subjectFilter))
	if err != nil {
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

// StreamMySpaceEvents creates a unified stream of all events within a space that are relevant
// to a specific user. This includes:
// - Space-level events (join/leave, room created/updated/deleted): NATS Core subscription
// - Room events (messages): JetStream consumer for ordering guarantees
// - Transient events (reactions): NATS Core subscription (live.space.{spaceId}.room.>)
// - Presence changes: KV watcher for presence updates
//
// The status parameter sets the user's presence status in the space. Presence is automatically
// cleaned up when the subscription ends.
//
// Room events are filtered based on user authorization - only delivered for rooms where user is a member.
// Room membership is cached and updated when join/leave events are observed.
// Only delivers new events that occur after subscription starts.
// The returned channel will be closed when the context is cancelled or after unrecoverable errors.
//
// Reliability: Transient JetStream errors (heartbeat missed, leadership change) trigger automatic
// retry with backoff. Terminal errors (connection closed, consumer deleted) close the channel.
// Clients should handle channel closure by resubscribing if they want to continue receiving events.
func (c *ChattoCore) StreamMySpaceEvents(ctx context.Context, spaceID, userID string) (<-chan *corev1.SpaceEvent, error) {
	// Get the space stream for room events
	stream, err := c.getSpaceStream(ctx, spaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get space stream: %w", err)
	}

	// Load initial room memberships into a set for O(1) lookups
	memberships, err := c.GetUserRoomMemberships(ctx, spaceID, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get room memberships: %w", err)
	}
	memberRooms := make(map[string]struct{}, len(memberships))
	for _, m := range memberships {
		memberRooms[m.RoomId] = struct{}{}
	}

	// Subscribe to live space-level events via NATS Core (member_deleted)
	liveSubject := subjects.LiveSpaceLevelEvents(spaceID)
	liveMsgChan := make(chan *nats.Msg, 64)
	liveSub, err := c.nc.ChanSubscribe(liveSubject, liveMsgChan)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to live space events: %w", err)
	}

	// Create JetStream consumer for all messages (root + thread replies) and meta events
	// Client-side filtering determines what to display; subscription delivers everything
	roomFilterSubjects := subjects.SpaceAllRoomEventsFilters(spaceID)
	cons, err := stream.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{
		FilterSubjects:    roomFilterSubjects,
		DeliverPolicy:     jetstream.DeliverNewPolicy,
		InactiveThreshold: 30 * time.Second,
	})
	if err != nil {
		liveSub.Unsubscribe()
		return nil, fmt.Errorf("failed to create ordered consumer: %w", err)
	}

	// Subscribe to live room events via NATS Core (reactions, message updates/deletes)
	liveRoomSubject := subjects.LiveSpaceRoomAllEvents(spaceID)
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

	eventChan := make(chan *corev1.SpaceEvent)

	go func() {
		c.logger.Debug("Starting space event stream", "space_id", spaceID, "user_id", userID,
			"live_subject", liveSubject, "room_subjects", roomFilterSubjects, "live_room_subject", liveRoomSubject)

		defer func() {
			c.logger.Debug("Space event stream closed", "space_id", spaceID, "user_id", userID)
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

		c.logger.Debug("Space subscription active", "space_id", spaceID, "user_id", userID, "member_rooms", len(memberRooms))

		// Initialize dedup map from hub snapshot (contains current presence state at subscribe time)
		lastKnownPresence := make(map[string]string, len(presenceSub.Snapshot))
		for k, v := range presenceSub.Snapshot {
			lastKnownPresence[k] = v
		}

		// Lazy cache for space membership lookups during presence filtering.
		// Only caches positive results; invalidated on SpaceMemberDeleted events.
		presenceMemberCache := make(map[string]bool)

		for {
			select {
			case <-ctx.Done():
				return

			case msg := <-liveMsgChan:
				// Space-level live event (member_deleted)
				// Note: Room events (join/leave/create/delete) come through roomMsgChan (JetStream).
				// This handler only receives events published directly via publishLiveSpaceEvent().
				var event corev1.SpaceEvent
				if err := proto.Unmarshal(msg.Data, &event); err != nil {
					c.logger.Warn("Failed to unmarshal live event", "error", err)
					continue
				}

				// Invalidate presence membership cache when a member is removed
				if memberDeleted := event.GetSpaceMemberDeleted(); memberDeleted != nil {
					delete(presenceMemberCache, memberDeleted.UserId)
				}

				select {
				case <-ctx.Done():
					return
				case eventChan <- &event:
				}

			case msg := <-roomMsgChan:
				// Room event from JetStream (messages, etc.)
				var event corev1.SpaceEvent
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
				case *corev1.SpaceEvent_UserJoinedRoom:
					if event.ActorId == userID {
						memberRooms[roomID] = struct{}{}
						isMember = true
					}
				case *corev1.SpaceEvent_UserLeftRoom:
					if event.ActorId == userID {
						delete(memberRooms, roomID)
						// Still deliver the leave event
					}
				case *corev1.SpaceEvent_RoomDeleted:
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
				// These events are published directly via publishLiveSpaceEvent(), bypassing JetStream.
				var event corev1.SpaceEvent
				if err := proto.Unmarshal(msg.Data, &event); err != nil {
					c.logger.Warn("Failed to unmarshal live room event", "error", err)
					continue
				}

				// Extract room ID from the event
				var roomID string
				switch e := event.Event.(type) {
				case *corev1.SpaceEvent_ReactionAdded:
					roomID = e.ReactionAdded.RoomId
				case *corev1.SpaceEvent_ReactionRemoved:
					roomID = e.ReactionRemoved.RoomId
				case *corev1.SpaceEvent_MessageDeleted:
					roomID = e.MessageDeleted.RoomId
				case *corev1.SpaceEvent_MessageUpdated:
					roomID = e.MessageUpdated.RoomId
				case *corev1.SpaceEvent_UserTyping:
					// Skip own typing events — the sender doesn't need to see them.
					// Critical for multi-instance clients where the frontend's
					// currentUserId may differ from the remote instance user ID.
					if event.ActorId == userID {
						continue
					}
					roomID = e.UserTyping.RoomId
				case *corev1.SpaceEvent_VideoProcessingCompleted:
					roomID = e.VideoProcessingCompleted.RoomId
				case *corev1.SpaceEvent_CallParticipantJoined:
					roomID = e.CallParticipantJoined.RoomId
				case *corev1.SpaceEvent_CallParticipantLeft:
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
				// Evict from membership cache when a user goes offline. This ensures
				// a fresh membership lookup when they come back online, correctly
				// handling users who left the space while disconnected.
				if update.Status == PresenceStatusOffline {
					delete(presenceMemberCache, update.UserID)
				}

				// Filter: only deliver updates for users who are members of this space.
				// Use lazy cache to avoid KV reads on every presence event.
				isMember, cached := presenceMemberCache[update.UserID]
				if !cached {
					var err error
					isMember, err = c.SpaceMembershipExists(ctx, update.UserID, spaceID)
					if err != nil {
						if !errors.Is(err, context.Canceled) {
							c.logger.Warn("Failed to check space membership for presence filtering",
								"error", err, "user_id", update.UserID, "space_id", spaceID)
						}
						continue
					}
					// Only cache positive results — when a non-member joins,
					// the next presence event triggers a fresh lookup.
					if isMember {
						presenceMemberCache[update.UserID] = true
					}
				}

				if !isMember {
					continue
				}

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
				presenceEvent := &corev1.SpaceEvent{
					CreatedAt: timestamppb.Now(),
					ActorId:   update.UserID,
					Event: &corev1.SpaceEvent_PresenceChanged{
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

// StreamMyInstanceEvents creates a live stream of instance-level events
// relevant to a specific user. Subscribes to live.instance.> and performs
// server-side authorization filtering to deliver only events the user is
// authorized to see:
//   - User events (instance.user.{userId}.*): Only if userId matches subscriber
//   - Space events (instance.space.{spaceId}.*): Only if user is a space member
//
// Only delivers new events that occur after subscription starts.
// The returned channel will be closed when the context is cancelled.
func (c *ChattoCore) StreamMyInstanceEvents(ctx context.Context, userID string) (<-chan *corev1.InstanceEvent, error) {
	// Subscribe to all live instance events via NATS Core
	liveSubject := subjects.LiveInstanceAllEvents()
	msgChan := make(chan *nats.Msg, 64)
	sub, err := c.nc.ChanSubscribe(liveSubject, msgChan)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to live instance events: %w", err)
	}

	eventChan := make(chan *corev1.InstanceEvent)

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
				var event corev1.InstanceEvent
				if err := proto.Unmarshal(msg.Data, &event); err != nil {
					c.logger.Warn("Failed to unmarshal event", "error", err)
					continue
				}

				// For NewMessageInSpaceEvent, check room membership (not just space membership).
				// This prevents unread indicators from appearing for rooms the user isn't in.
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
				}

				// Server-side authorization filtering based on subject pattern
				// Subject format: live.instance.{type}.{id}.{eventType}
				// - live.instance.user.{userId}.* → only forward to that user
				// - live.instance.space.{spaceId}.* → only forward to space members
				if !c.isAuthorizedForInstanceEvent(ctx, userID, msg.Subject) {
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

// isAuthorizedForInstanceEvent checks if a user is authorized to receive an instance event
// based on the subject pattern:
//   - live.instance.config.* → all authenticated users (instance config is public)
//   - live.instance.user.{userId}.* → only the specific user (except profile_updated)
//   - live.instance.user.{userId}.profile_updated → broadcast to all (profiles are public)
//   - live.instance.space.{spaceId}.* → only space members
func (c *ChattoCore) isAuthorizedForInstanceEvent(ctx context.Context, userID, subject string) bool {
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
		// Space events: forward to all members of the space
		isMember, err := c.SpaceMembershipExists(ctx, userID, scopeID)
		if err != nil {
			c.logger.Warn("Failed to check space membership for event filtering",
				"error", err, "user_id", userID, "space_id", scopeID)
			return false
		}
		return isMember
	default:
		c.logger.Warn("Unknown instance event scope", "scope", eventScope, "subject", subject)
		return false
	}
}

// StreamMyInstanceLiveEvents streams transient instance-level events to the user.
// These are fire-and-forget events that bypass JetStream (config changes, etc.).
// The returned channel will be closed when the context is cancelled.
func (c *ChattoCore) StreamMyInstanceLiveEvents(ctx context.Context, userID string) (<-chan *corev1.InstanceEvent, error) {
	// Subscribe to live instance config events via NATS Core
	liveSubject := subjects.LiveInstanceConfigAllEvents()
	msgChan := make(chan *nats.Msg, 64)
	sub, err := c.nc.ChanSubscribe(liveSubject, msgChan)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to live instance config events: %w", err)
	}

	eventChan := make(chan *corev1.InstanceEvent)

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
				var event corev1.InstanceEvent
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
	event := newInstanceEvent(actorID, &corev1.InstanceEvent{
		Event: &corev1.InstanceEvent_ConfigUpdated{
			ConfigUpdated: &corev1.InstanceConfigUpdatedEvent{
				InstanceName:     instanceName,
				Motd:             motd,
				WelcomeMessage:   welcomeMessage,
				BlockedUsernames: blockedUsernames,
			},
		},
	})

	return c.publishInstanceEvent(ctx, subjects.LiveInstanceConfigUpdated(), event)
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
		stream, err := c.getSpaceStream(ctx, space.Id)
		if err != nil {
			// Stream might not exist yet if space has no activity
			continue
		}
		// Get subject-filtered info for room events
		roomSubjectFilter := subjects.SpaceAllRoomEvents(space.Id)
		info, err := stream.Info(ctx, jetstream.WithSubjectFilter(roomSubjectFilter))
		if err != nil {
			continue
		}
		stats.MessageCount += info.State.Msgs
	}

	return stats, nil
}

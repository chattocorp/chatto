package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/notificationstream"
)

const projectionSnapshotObjectStoreName = "PROJECTION_SNAPSHOTS"

// ============================================================================
// Storage
// ============================================================================

// storage encapsulates JetStream resources used by Chatto Core.
type storage struct {
	encryptionKV   jetstream.KeyValue // ENCRYPTION_KEYS - KMS KEKs (excluded from backups)
	runtimeStateKV jetstream.KeyValue // RUNTIME_STATE  - persisted latest-value runtime/user state + wrapped app DEKs

	serverAssets       jetstream.ObjectStore // SERVER_ASSETS - all NATS-backed asset binaries
	serverEvtStream    jetstream.Stream      // EVT - authoritative domain event log (ADR-033/034).
	notificationStream jetstream.Stream      // NOTIFICATIONS - bounded notification lifecycle event log.

	memoryCacheKV   jetstream.KeyValue    // MEMORY_CACHE - volatile, memory-backed runtime cache state
	imageCacheStore jetstream.ObjectStore // Optional: cached resized images (nil if disabled)
}

// newStorage initializes current JetStream resources.
func newStorage(js jetstream.JetStream, ctx context.Context, cfg config.CoreConfig) (*storage, error) {
	// Initialize KMS KEK bucket (excluded from backups for security). App-owned
	// wrapped DEK records live in RUNTIME_STATE so normal backups keep encrypted
	// content together with its wrapped content-key registry, but not the KEKs
	// needed to unwrap it.
	encryptionKV, err := createJetStreamResourceWithRetry(ctx, func(ctx context.Context) (jetstream.KeyValue, error) {
		return js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket:      "ENCRYPTION_KEYS",
			Description: "KMS key-encryption keys (excluded from backups)",
			Storage:     jetstream.FileStorage,
			History:     1,
			Replicas:    cfg.Replicas,
		})
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create ENCRYPTION_KEYS KV bucket: %w", err)
	}

	runtimeStateKV, err := createJetStreamResourceWithRetry(ctx, func(ctx context.Context) (jetstream.KeyValue, error) {
		return js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
			Bucket:         "RUNTIME_STATE",
			Description:    "Persisted latest-value runtime/user state",
			Storage:        jetstream.FileStorage,
			History:        1,
			Compression:    true,
			Replicas:       cfg.Replicas,
			LimitMarkerTTL: 24 * time.Hour,
		})
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create RUNTIME_STATE KV bucket: %w", err)
	}

	memoryCacheKV, err := createJetStreamResourceWithRetry(ctx, func(ctx context.Context) (jetstream.KeyValue, error) {
		return js.CreateOrUpdateKeyValue(ctx, memoryCacheConfig(cfg))
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create MEMORY_CACHE KV bucket: %w", err)
	}

	// Initialize image cache object store (optional, only when enabled)
	var imageCacheStore jetstream.ObjectStore
	if cfg.Assets.Cache.Enabled {
		imageCacheStore, err = createJetStreamResourceWithRetry(ctx, func(ctx context.Context) (jetstream.ObjectStore, error) {
			return js.CreateOrUpdateObjectStore(ctx, jetstream.ObjectStoreConfig{
				Bucket:      "ASSET_CACHE",
				Description: "Cached resized images",
				Storage:     jetstream.FileStorage,
				Compression: true,
				TTL:         cfg.Assets.Cache.TTLOrDefault(),
				Replicas:    cfg.Replicas,
			})
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create ASSET_CACHE object store: %w", err)
		}
	}

	serverAssets, err := createJetStreamResourceWithRetry(ctx, func(ctx context.Context) (jetstream.ObjectStore, error) {
		return js.CreateOrUpdateObjectStore(ctx, jetstream.ObjectStoreConfig{
			Bucket:      "SERVER_ASSETS",
			Description: "Server asset binaries (avatars, branding, link previews, attachments)",
			Storage:     jetstream.FileStorage,
			Compression: true,
			Replicas:    cfg.Replicas,
		})
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create SERVER_ASSETS object store: %w", err)
	}

	// EVT — the event-sourcing log (ADR-033/034).
	// Subjects are evt.{aggregateType}.{aggregateId}.{eventType}; live.evt.> is
	// the republish target so projections and live subscribers consume
	// from a single NATS Core path.
	evtMetadata, err := prepareEVTStreamMetadata(ctx, js)
	if err != nil {
		return nil, fmt.Errorf("prepare EVT stream metadata: %w", err)
	}
	evtConfig := jetstream.StreamConfig{
		Name:        "EVT",
		Description: "Event-sourcing log (ADR-033)",
		Subjects:    []string{"evt.>"},
		Storage:     jetstream.FileStorage,
		Compression: jetstream.S2Compression,
		Replicas:    cfg.Replicas,
		Metadata:    evtMetadata,
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
	}
	serverEvtStream, err := createJetStreamResourceWithRetry(ctx, func(ctx context.Context) (jetstream.Stream, error) {
		return js.CreateOrUpdateStream(ctx, evtConfig)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create EVT stream: %w", err)
	}
	if !evtstream.ValidIdentity(evtConfig.Metadata[evtstream.IdentityMetadataKey]) {
		info := serverEvtStream.CachedInfo()
		if info == nil {
			return nil, fmt.Errorf("created EVT stream info is unavailable")
		}
		identity, identityErr := evtstream.NewIdentity(info.Created)
		if identityErr != nil {
			return nil, identityErr
		}
		evtConfig.Metadata[evtstream.IdentityMetadataKey] = identity
		serverEvtStream, err = createJetStreamResourceWithRetry(ctx, func(ctx context.Context) (jetstream.Stream, error) {
			return js.CreateOrUpdateStream(ctx, evtConfig)
		})
		if err != nil {
			return nil, fmt.Errorf("persist EVT stream identity: %w", err)
		}
	}

	notificationMetadata, err := prepareNotificationStreamMetadata(ctx, js)
	if err != nil {
		return nil, fmt.Errorf("prepare NOTIFICATIONS stream metadata: %w", err)
	}
	notificationConfig := jetstream.StreamConfig{
		Name:               notificationstream.StreamName,
		Description:        "Bounded notification lifecycle event log",
		Subjects:           notificationstream.Subjects(),
		Storage:            jetstream.FileStorage,
		Compression:        jetstream.S2Compression,
		Replicas:           cfg.Replicas,
		MaxAge:             notificationTTL + notificationPhysicalCleanupGrace,
		Duplicates:         notificationAlertDeliveryTTL,
		AllowMsgTTL:        true,
		AllowAtomicPublish: true,
		Metadata:           notificationMetadata,
	}
	notificationStream, err := createJetStreamResourceWithRetry(ctx, func(ctx context.Context) (jetstream.Stream, error) {
		return js.CreateOrUpdateStream(ctx, notificationConfig)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create %s stream: %w", notificationstream.StreamName, err)
	}
	if !notificationstream.ValidIdentity(notificationConfig.Metadata[notificationstream.IdentityMetadataKey]) {
		info := notificationStream.CachedInfo()
		if info == nil {
			return nil, fmt.Errorf("created NOTIFICATIONS stream info is unavailable")
		}
		identity, identityErr := notificationstream.NewIdentity(info.Created)
		if identityErr != nil {
			return nil, identityErr
		}
		notificationConfig.Metadata[notificationstream.IdentityMetadataKey] = identity
		notificationStream, err = createJetStreamResourceWithRetry(ctx, func(ctx context.Context) (jetstream.Stream, error) {
			return js.CreateOrUpdateStream(ctx, notificationConfig)
		})
		if err != nil {
			return nil, fmt.Errorf("persist NOTIFICATIONS stream identity: %w", err)
		}
	}

	return &storage{
		encryptionKV:       encryptionKV,
		runtimeStateKV:     runtimeStateKV,
		serverAssets:       serverAssets,
		serverEvtStream:    serverEvtStream,
		notificationStream: notificationStream,
		memoryCacheKV:      memoryCacheKV,
		imageCacheStore:    imageCacheStore,
	}, nil
}

func prepareNotificationStreamMetadata(ctx context.Context, js jetstream.JetStream) (map[string]string, error) {
	metadata := make(map[string]string)
	stream, err := js.Stream(ctx, notificationstream.StreamName)
	switch {
	case err == nil:
		info, infoErr := stream.Info(ctx)
		if infoErr != nil {
			return nil, fmt.Errorf("read existing NOTIFICATIONS stream info: %w", infoErr)
		}
		for key, value := range info.Config.Metadata {
			metadata[key] = value
		}
	case errors.Is(err, jetstream.ErrStreamNotFound):
	case err != nil:
		return nil, fmt.Errorf("open existing NOTIFICATIONS stream: %w", err)
	}
	if notificationstream.ValidIdentity(metadata[notificationstream.IdentityMetadataKey]) {
		return metadata, nil
	}
	if stream == nil || stream.CachedInfo() == nil {
		return metadata, nil
	}
	identity, err := notificationstream.NewIdentity(stream.CachedInfo().Created)
	if err != nil {
		return nil, err
	}
	metadata[notificationstream.IdentityMetadataKey] = identity
	return metadata, nil
}

func memoryCacheConfig(cfg config.CoreConfig) jetstream.KeyValueConfig {
	return jetstream.KeyValueConfig{
		Bucket:         "MEMORY_CACHE",
		Description:    "Volatile memory-backed runtime cache state",
		Storage:        jetstream.MemoryStorage,
		History:        1,
		Replicas:       cfg.Replicas,
		LimitMarkerTTL: PresenceTTL,
	}
}

func prepareEVTStreamMetadata(ctx context.Context, js jetstream.JetStream) (map[string]string, error) {
	metadata := make(map[string]string)
	stream, err := js.Stream(ctx, "EVT")
	switch {
	case err == nil:
		info, infoErr := stream.Info(ctx)
		if infoErr != nil {
			return nil, fmt.Errorf("read existing EVT stream info: %w", infoErr)
		}
		for key, value := range info.Config.Metadata {
			metadata[key] = value
		}
	case errors.Is(err, jetstream.ErrStreamNotFound):
	case err != nil:
		return nil, fmt.Errorf("open existing EVT stream: %w", err)
	}
	if evtstream.ValidIdentity(metadata[evtstream.IdentityMetadataKey]) {
		return metadata, nil
	}
	if stream == nil {
		return metadata, nil
	}
	if stream.CachedInfo() == nil {
		return nil, fmt.Errorf("existing EVT stream info is unavailable")
	}
	identity, err := evtstream.NewIdentity(stream.CachedInfo().Created)
	if err != nil {
		return nil, err
	}
	metadata[evtstream.IdentityMetadataKey] = identity
	return metadata, nil
}

func createJetStreamResourceWithRetry[T any](ctx context.Context, create func(context.Context) (T, error)) (T, error) {
	const maxAttempts = 3

	var zero T
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		resource, err := create(ctx)
		if err == nil {
			return resource, nil
		}
		if attempt == maxAttempts || !isTransientJetStreamStoreCreateError(err) {
			return zero, err
		}

		timer := time.NewTimer(time.Duration(attempt) * 25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return zero, ctx.Err()
		case <-timer.C:
		}
	}

	return zero, nil
}

func isTransientJetStreamStoreCreateError(err error) bool {
	type apiErrorProvider interface {
		APIError() *jetstream.APIError
	}

	var provider apiErrorProvider
	if !errors.As(err, &provider) {
		return false
	}
	apiErr := provider.APIError()
	if apiErr == nil {
		return false
	}
	return (apiErr.ErrorCode == 10049 && strings.Contains(apiErr.Description, "error creating store for stream")) ||
		(apiErr.ErrorCode == 10058 && strings.Contains(apiErr.Description, "stream name already in use"))
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

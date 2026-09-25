package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/notificationstream"
	"hmans.de/chatto/pkg/events"
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
	logStream          jetstream.Stream      // LOG - retained operational diagnostics; excluded from backups.
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

	logStream, err := createJetStreamResourceWithRetry(ctx, func(ctx context.Context) (jetstream.Stream, error) {
		return js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
			Name: "LOG", Description: "Retained operational diagnostics", Subjects: []string{"log.>"},
			Storage: jetstream.FileStorage, Compression: jetstream.S2Compression, Replicas: cfg.Replicas,
			Retention: jetstream.LimitsPolicy, MaxAge: cfg.Log.RetentionOrDefault(),
			Duplicates: min(2*time.Minute, cfg.Log.RetentionOrDefault()),
		})
	})
	if err != nil {
		return nil, fmt.Errorf("create LOG stream: %w", err)
	}

	return &storage{
		logStream:          logStream,
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

// createJetStreamResourceWithRetry applies Chatto's startup retry budget to the
// shared provisioning mechanics. The callback retains ownership of configuration.
func createJetStreamResourceWithRetry[T any](ctx context.Context, create func(context.Context) (T, error)) (T, error) {
	return events.CreateJetStreamResourceWithRetry(ctx, events.JetStreamResourceRetryPolicy{
		MaxAttempts: 3,
		RetryDelay:  25 * time.Millisecond,
	}, create)
}

// ============================================================================
// KV Key Helpers
// ============================================================================

// These helper functions format keys for NATS KV bucket entries. They stay in
// the core package since they're only used here and are integral to how core
// interacts with storage.

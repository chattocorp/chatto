package core

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/evtstream"
	"hmans.de/chatto/internal/natsresources"
	"hmans.de/chatto/internal/notificationstream"
	"hmans.de/chatto/pkg/jetstreamutil"
)

// ============================================================================
// Storage
// ============================================================================

// storage encapsulates JetStream resources used by Chatto Core.
type storage struct {
	// Key-value buckets are bound through jetstreamutil.NewKeyValue, so Get reads
	// through the stream leader. Use GetAnyReplica only on hot paths that
	// tolerate an older revision.
	encryptionKV   *jetstreamutil.KeyValue // ENCRYPTION_KEYS - KMS KEKs (excluded from backups)
	runtimeStateKV *jetstreamutil.KeyValue // RUNTIME_STATE  - persisted latest-value runtime/user state + wrapped app DEKs

	serverAssets       jetstream.ObjectStore // SERVER_ASSETS - all NATS-backed asset binaries
	serverEvtStream    jetstream.Stream      // EVT - authoritative domain event log (ADR-033/034).
	logStream          jetstream.Stream      // LOG - retained operational diagnostics; excluded from backups.
	notificationStream jetstream.Stream      // NOTIFICATIONS - bounded notification lifecycle event log.

	memoryCacheKV      *jetstreamutil.KeyValue // MEMORY_CACHE - volatile, memory-backed runtime cache state
	imageCacheStore    jetstream.ObjectStore   // Optional: cached resized images (nil if disabled)
	neighborhoodImages jetstream.ObjectStore   // NEIGHBORHOOD_IMAGES - expiring copies of discovered server images
}

// newStorage initializes current JetStream resources.
//
// Every replica updates each resource to the configuration below at startup,
// and replicas of different versions can run at the same time. A configuration
// change is therefore a deliberate storage decision, not a refactor; see
// TestStorageConfigsArePinned and ADR-114.
func newStorage(js jetstream.JetStream, ctx context.Context, cfg config.CoreConfig) (*storage, error) {
	// App-owned wrapped DEK records live in RUNTIME_STATE so normal backups
	// keep encrypted content together with its wrapped content-key registry,
	// but not the KEKs needed to unwrap it.
	encryptionKV, err := createKeyValue(ctx, js, EncryptionKeysConfig(cfg.Replicas))
	if err != nil {
		return nil, err
	}
	runtimeStateKV, err := createKeyValue(ctx, js, runtimeStateConfig(cfg))
	if err != nil {
		return nil, err
	}
	memoryCacheKV, err := createKeyValue(ctx, js, memoryCacheConfig(cfg))
	if err != nil {
		return nil, err
	}

	var imageCacheStore jetstream.ObjectStore
	if cfg.Assets.Cache.Enabled {
		imageCacheStore, err = createObjectStore(ctx, js, assetCacheConfig(cfg))
		if err != nil {
			return nil, err
		}
	}
	neighborhoodImages, err := createObjectStore(ctx, js, neighborhoodImagesConfig(cfg))
	if err != nil {
		return nil, err
	}
	serverAssets, err := createObjectStore(ctx, js, serverAssetsConfig(cfg))
	if err != nil {
		return nil, err
	}

	serverEvtStream, err := createIdentifiedStream(ctx, js, evtStreamConfig(cfg), evtStreamIdentity)
	if err != nil {
		return nil, err
	}
	notificationStream, err := createIdentifiedStream(ctx, js, notificationsStreamConfig(cfg), notificationStreamIdentity)
	if err != nil {
		return nil, err
	}
	logStream, err := createJetStreamResourceWithRetry(ctx, func(ctx context.Context) (jetstream.Stream, error) {
		return js.CreateOrUpdateStream(ctx, logStreamConfig(cfg))
	})
	if err != nil {
		return nil, fmt.Errorf("create %s stream: %w", natsresources.Log, err)
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
		neighborhoodImages: neighborhoodImages,
	}, nil
}

// EncryptionKeysConfig is the ENCRYPTION_KEYS bucket configuration. The server
// and `chatto keys import` must create the bucket identically. The bucket is
// excluded from backups for security.
func EncryptionKeysConfig(replicas int) jetstream.KeyValueConfig {
	return jetstream.KeyValueConfig{
		Bucket:      natsresources.EncryptionKeys,
		Description: "KMS key-encryption keys (excluded from backups)",
		Storage:     jetstream.FileStorage,
		History:     1,
		Replicas:    replicas,
	}
}

func runtimeStateConfig(cfg config.CoreConfig) jetstream.KeyValueConfig {
	return jetstream.KeyValueConfig{
		Bucket:         natsresources.RuntimeState,
		Description:    "Persisted latest-value runtime/user state",
		Storage:        jetstream.FileStorage,
		History:        1,
		Compression:    true,
		Replicas:       cfg.Replicas,
		LimitMarkerTTL: 24 * time.Hour,
	}
}

// memoryCacheConfig enables per-key TTLs for every MEMORY_CACHE record. The
// limit marker lifetime comes from presence, the bucket's first TTL user.
func memoryCacheConfig(cfg config.CoreConfig) jetstream.KeyValueConfig {
	return jetstream.KeyValueConfig{
		Bucket:         natsresources.MemoryCache,
		Description:    "Volatile memory-backed runtime cache state",
		Storage:        jetstream.MemoryStorage,
		History:        1,
		Replicas:       cfg.Replicas,
		LimitMarkerTTL: PresenceTTL,
	}
}

func assetCacheConfig(cfg config.CoreConfig) jetstream.ObjectStoreConfig {
	return jetstream.ObjectStoreConfig{
		Bucket:      natsresources.AssetCache,
		Description: "Cached resized images",
		Storage:     jetstream.FileStorage,
		Compression: true,
		TTL:         cfg.Assets.Cache.TTLOrDefault(),
		Replicas:    cfg.Replicas,
	}
}

// neighborhoodImagesConfig keeps each image for a fixed period after its
// latest write. Neighborhood discovery rewrites the images that it still
// uses, so unused images expire without a cleanup pass.
func neighborhoodImagesConfig(cfg config.CoreConfig) jetstream.ObjectStoreConfig {
	return jetstream.ObjectStoreConfig{
		Bucket:      natsresources.NeighborhoodImages,
		Description: "Expiring copies of Neighborhood server images",
		Storage:     jetstream.FileStorage,
		TTL:         neighborhoodImageTTL,
		Replicas:    cfg.Replicas,
	}
}

func serverAssetsConfig(cfg config.CoreConfig) jetstream.ObjectStoreConfig {
	return jetstream.ObjectStoreConfig{
		Bucket:      natsresources.ServerAssets,
		Description: "Server asset binaries (avatars, branding, link previews, attachments)",
		Storage:     jetstream.FileStorage,
		Compression: true,
		Replicas:    cfg.Replicas,
	}
}

func projectionSnapshotsConfig(cfg config.CoreConfig) jetstream.ObjectStoreConfig {
	return jetstream.ObjectStoreConfig{
		Bucket:      natsresources.ProjectionSnapshots,
		Description: "Encrypted ephemeral projection snapshots",
		Storage:     jetstream.FileStorage,
		Compression: true,
		Replicas:    cfg.Replicas,
		TTL:         cfg.ProjectionSnapshotRetentionOrDefault(),
	}
}

// evtStreamConfig is the EVT event-sourcing log (ADR-033/034). Subjects are
// evt.{aggregateType}.{aggregateId}.{eventType}; live.evt.> is the republish
// target so projections and live subscribers consume from a single NATS Core
// path. createIdentifiedStream adds the identity metadata.
func evtStreamConfig(cfg config.CoreConfig) jetstream.StreamConfig {
	return jetstream.StreamConfig{
		Name:        natsresources.EVT,
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
	}
}

func notificationsStreamConfig(cfg config.CoreConfig) jetstream.StreamConfig {
	return jetstream.StreamConfig{
		Name:               natsresources.Notifications,
		Description:        "Bounded notification lifecycle event log",
		Subjects:           notificationstream.Subjects(),
		Storage:            jetstream.FileStorage,
		Compression:        jetstream.S2Compression,
		Replicas:           cfg.Replicas,
		MaxAge:             notificationTTL + notificationPhysicalCleanupGrace,
		Duplicates:         notificationAlertDeliveryTTL,
		AllowMsgTTL:        true,
		AllowAtomicPublish: true,
	}
}

func logStreamConfig(cfg config.CoreConfig) jetstream.StreamConfig {
	return jetstream.StreamConfig{
		Name:        natsresources.Log,
		Description: "Retained operational diagnostics",
		Subjects:    []string{"log.>"},
		Storage:     jetstream.FileStorage,
		Compression: jetstream.S2Compression,
		Replicas:    cfg.Replicas,
		Retention:   jetstream.LimitsPolicy,
		MaxAge:      cfg.Log.RetentionOrDefault(),
		Duplicates:  min(2*time.Minute, cfg.Log.RetentionOrDefault()),
	}
}

// createKeyValue creates or updates a bucket and binds leader-routed reads.
func createKeyValue(ctx context.Context, js jetstream.JetStream, cfg jetstream.KeyValueConfig) (*jetstreamutil.KeyValue, error) {
	bucket, err := createJetStreamResourceWithRetry(ctx, func(ctx context.Context) (jetstream.KeyValue, error) {
		return js.CreateOrUpdateKeyValue(ctx, cfg)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create %s KV bucket: %w", cfg.Bucket, err)
	}
	return jetstreamutil.NewKeyValue(js, bucket)
}

// isKeyAbsent reports that a key-value read found no live entry. Bound reads
// report a removal marker as jetstream.ErrKeyNotFound; other reads, such as a
// raw revision read, report it as jetstream.ErrKeyDeleted.
func isKeyAbsent(err error) bool {
	return errors.Is(err, jetstream.ErrKeyNotFound) || errors.Is(err, jetstream.ErrKeyDeleted)
}

// createObjectStore creates or updates an Object Store.
func createObjectStore(ctx context.Context, js jetstream.JetStream, cfg jetstream.ObjectStoreConfig) (jetstream.ObjectStore, error) {
	store, err := createJetStreamResourceWithRetry(ctx, func(ctx context.Context) (jetstream.ObjectStore, error) {
		return js.CreateOrUpdateObjectStore(ctx, cfg)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create %s object store: %w", cfg.Bucket, err)
	}
	return store, nil
}

// streamIdentity is one stream's incarnation identity scheme. The owning
// package defines the metadata key and format.
type streamIdentity struct {
	metadataKey string
	valid       func(string) bool
	derive      func(created time.Time) (string, error)
}

var (
	evtStreamIdentity = streamIdentity{
		metadataKey: evtstream.IdentityMetadataKey,
		valid:       evtstream.ValidIdentity,
		derive:      evtstream.NewIdentity,
	}
	notificationStreamIdentity = streamIdentity{
		metadataKey: notificationstream.IdentityMetadataKey,
		valid:       notificationstream.ValidIdentity,
		derive:      notificationstream.NewIdentity,
	}
)

// createIdentifiedStream creates or updates a stream and keeps its incarnation
// identity in the stream metadata. It keeps the existing metadata. When the
// identity is missing, it derives one from the stream creation time: from the
// existing stream, or from the new stream after a second update.
func createIdentifiedStream(ctx context.Context, js jetstream.JetStream, cfg jetstream.StreamConfig, identity streamIdentity) (jetstream.Stream, error) {
	metadata, err := prepareStreamIdentityMetadata(ctx, js, cfg.Name, identity)
	if err != nil {
		return nil, fmt.Errorf("prepare %s stream metadata: %w", cfg.Name, err)
	}
	cfg.Metadata = metadata
	create := func(ctx context.Context) (jetstream.Stream, error) {
		return js.CreateOrUpdateStream(ctx, cfg)
	}
	stream, err := createJetStreamResourceWithRetry(ctx, create)
	if err != nil {
		return nil, fmt.Errorf("failed to create %s stream: %w", cfg.Name, err)
	}
	if identity.valid(metadata[identity.metadataKey]) {
		return stream, nil
	}
	info := stream.CachedInfo()
	if info == nil {
		return nil, fmt.Errorf("created %s stream info is unavailable", cfg.Name)
	}
	value, err := identity.derive(info.Created)
	if err != nil {
		return nil, err
	}
	metadata[identity.metadataKey] = value
	stream, err = createJetStreamResourceWithRetry(ctx, create)
	if err != nil {
		return nil, fmt.Errorf("persist %s stream identity: %w", cfg.Name, err)
	}
	return stream, nil
}

// prepareStreamIdentityMetadata returns the existing stream metadata. It adds
// an identity derived from the existing stream's creation time when the
// metadata has no valid identity. It returns no identity for a new stream.
func prepareStreamIdentityMetadata(ctx context.Context, js jetstream.JetStream, name string, identity streamIdentity) (map[string]string, error) {
	metadata := make(map[string]string)
	stream, err := js.Stream(ctx, name)
	switch {
	case errors.Is(err, jetstream.ErrStreamNotFound):
		return metadata, nil
	case err != nil:
		return nil, fmt.Errorf("open existing %s stream: %w", name, err)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		return nil, fmt.Errorf("read existing %s stream info: %w", name, err)
	}
	maps.Copy(metadata, info.Config.Metadata)
	if identity.valid(metadata[identity.metadataKey]) {
		return metadata, nil
	}
	value, err := identity.derive(info.Created)
	if err != nil {
		return nil, err
	}
	metadata[identity.metadataKey] = value
	return metadata, nil
}

// createJetStreamResourceWithRetry applies Chatto's startup retry budget to the
// shared provisioning mechanics. The callback retains ownership of configuration.
func createJetStreamResourceWithRetry[T any](ctx context.Context, create func(context.Context) (T, error)) (T, error) {
	return jetstreamutil.CreateJetStreamResourceWithRetry(ctx, jetstreamutil.JetStreamResourceRetryPolicy{
		MaxAttempts: 3,
		RetryDelay:  25 * time.Millisecond,
	}, create)
}

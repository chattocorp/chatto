package core

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/natsresources"
	"hmans.de/chatto/internal/testutil"
)

// TestStorageConfigsArePinned fails on every change to a resource
// configuration. Every replica updates its resources to its own configuration
// at startup, and replicas of different versions can run at the same time.
// Change an expected value only as a deliberate storage decision (ADR-114).
func TestStorageConfigsArePinned(t *testing.T) {
	t.Parallel()

	const day = 24 * time.Hour
	cfg := config.CoreConfig{Replicas: 3}

	keyValues := map[string][2]jetstream.KeyValueConfig{
		"ENCRYPTION_KEYS": {EncryptionKeysConfig(cfg.Replicas), {
			Bucket: "ENCRYPTION_KEYS", Description: "KMS key-encryption keys (excluded from backups)",
			Storage: jetstream.FileStorage, History: 1, Replicas: 3,
		}},
		"RUNTIME_STATE": {runtimeStateConfig(cfg), {
			Bucket: "RUNTIME_STATE", Description: "Persisted latest-value runtime/user state",
			Storage: jetstream.FileStorage, History: 1, Compression: true, Replicas: 3, LimitMarkerTTL: day,
		}},
		"MEMORY_CACHE": {memoryCacheConfig(cfg), {
			Bucket: "MEMORY_CACHE", Description: "Volatile memory-backed runtime cache state",
			Storage: jetstream.MemoryStorage, History: 1, Replicas: 3, LimitMarkerTTL: time.Minute,
		}},
	}
	for name, configs := range keyValues {
		if !reflect.DeepEqual(configs[0], configs[1]) {
			t.Errorf("%s config = %+v, want %+v", name, configs[0], configs[1])
		}
	}

	objectStores := map[string][2]jetstream.ObjectStoreConfig{
		"ASSET_CACHE": {assetCacheConfig(cfg), {
			Bucket: "ASSET_CACHE", Description: "Cached resized images",
			Storage: jetstream.FileStorage, Compression: true, TTL: 7 * day, Replicas: 3,
		}},
		"NEIGHBORHOOD_IMAGES": {neighborhoodImagesConfig(cfg), {
			Bucket: "NEIGHBORHOOD_IMAGES", Description: "Expiring copies of Neighborhood server images",
			Storage: jetstream.FileStorage, TTL: 7 * day, Replicas: 3,
		}},
		"SERVER_ASSETS": {serverAssetsConfig(cfg), {
			Bucket: "SERVER_ASSETS", Description: "Server asset binaries (avatars, branding, link previews, attachments)",
			Storage: jetstream.FileStorage, Compression: true, Replicas: 3,
		}},
		"PROJECTION_SNAPSHOTS": {projectionSnapshotsConfig(cfg), {
			Bucket: "PROJECTION_SNAPSHOTS", Description: "Encrypted ephemeral projection snapshots",
			Storage: jetstream.FileStorage, Compression: true, Replicas: 3, TTL: 7 * day,
		}},
	}
	for name, configs := range objectStores {
		if !reflect.DeepEqual(configs[0], configs[1]) {
			t.Errorf("%s config = %+v, want %+v", name, configs[0], configs[1])
		}
	}

	streams := map[string][2]jetstream.StreamConfig{
		"EVT": {evtStreamConfig(cfg), {
			Name: "EVT", Description: "Event-sourcing log (ADR-033)", Subjects: []string{"evt.>"},
			Storage: jetstream.FileStorage, Compression: jetstream.S2Compression, Replicas: 3,
			AllowAtomicPublish: true,
			RePublish:          &jetstream.RePublish{Source: "evt.>", Destination: "live.evt.>"},
		}},
		"NOTIFICATIONS": {notificationsStreamConfig(cfg), {
			Name: "NOTIFICATIONS", Description: "Bounded notification lifecycle event log",
			Subjects: []string{
				"notifications.signalled", "notifications.read",
				"notifications.removed", "notifications.alert_resolved",
			},
			Storage: jetstream.FileStorage, Compression: jetstream.S2Compression, Replicas: 3,
			MaxAge: 91 * day, Duplicates: 2 * time.Minute, AllowMsgTTL: true, AllowAtomicPublish: true,
		}},
		"LOG": {logStreamConfig(cfg), {
			Name: "LOG", Description: "Retained operational diagnostics", Subjects: []string{"log.>"},
			Storage: jetstream.FileStorage, Compression: jetstream.S2Compression, Replicas: 3,
			Retention: jetstream.LimitsPolicy, MaxAge: 7 * day, Duplicates: 2 * time.Minute,
		}},
	}
	for name, configs := range streams {
		if !reflect.DeepEqual(configs[0], configs[1]) {
			t.Errorf("%s config = %+v, want %+v", name, configs[0], configs[1])
		}
	}
}

// TestNewChattoCoreProvisionsExactlyTheRegisteredResources fails when core
// creates a resource that the natsresources registry does not list, or when
// the registry lists a resource that core never creates. Backups, restores,
// and test resets depend on the registry.
func TestNewChattoCoreProvisionsExactlyTheRegisteredResources(t *testing.T) {
	t.Parallel()

	_, nc := testutil.StartNATS(t)
	ctx := testContext(t)
	cfg := config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets: config.AssetsConfig{
			SigningSecret:  "test-signing-secret",
			StorageBackend: config.StorageBackendNATS,
			Cache:          config.AssetsCacheConfig{Enabled: true},
		},
		ProjectionSnapshots: true,
	}
	core, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatalf("NewChattoCore: %v", err)
	}

	var provisioned []string
	streams := core.js.ListStreams(ctx)
	for info := range streams.Info() {
		provisioned = append(provisioned, info.Config.Name)
	}
	if err := streams.Err(); err != nil {
		t.Fatal(err)
	}
	var registered []string
	for _, resource := range natsresources.Current() {
		registered = append(registered, resource.StreamName())
	}
	slices.Sort(provisioned)
	slices.Sort(registered)
	if !slices.Equal(provisioned, registered) {
		t.Fatalf("provisioned streams = %v, registered = %v", provisioned, registered)
	}
}

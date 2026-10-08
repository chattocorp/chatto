// Package natsresources names Chatto's JetStream resources and records how
// backups treat each of them.
//
// Resource names are persisted contracts: a server with existing data opens
// its resources by these names. Never rename a resource. Configuration stays
// with the runtime code that provisions the resource; this package holds only
// the facts that provisioning, backup, restore, operator commands, and tests
// share. Add every new stream, key-value bucket, and Object Store here.
package natsresources

import "slices"

// Resource names. Each name is the logical name that the JetStream API uses
// for the resource. Key-value buckets and Object Stores have an underlying
// stream with a KV_ or OBJ_ prefix; see Resource.StreamName.
const (
	// EVT is the durable domain event log (ADR-033, ADR-034).
	EVT = "EVT"
	// Notifications is the bounded notification lifecycle event log.
	Notifications = "NOTIFICATIONS"
	// Log is the retained operational diagnostics stream (ADR-098).
	Log = "LOG"
	// EncryptionKeys is the KMS key-encryption-key bucket.
	EncryptionKeys = "ENCRYPTION_KEYS"
	// RuntimeState is the persisted latest-value runtime state bucket (ADR-036).
	RuntimeState = "RUNTIME_STATE"
	// MemoryCache is the volatile memory-backed cache bucket.
	MemoryCache = "MEMORY_CACHE"
	// ServerAssets is the default store for persisted asset binaries.
	ServerAssets = "SERVER_ASSETS"
	// AssetCache holds resized images and Neighborhood image copies for seven days.
	AssetCache = "ASSET_CACHE"
	// NeighborhoodImages is the legacy Neighborhood image cache (ADR-106).
	NeighborhoodImages = "NEIGHBORHOOD_IMAGES"
	// ProjectionSnapshots holds optional encrypted projection snapshots (ADR-050).
	ProjectionSnapshots = "PROJECTION_SNAPSHOTS"
)

// Kind is the JetStream abstraction that a resource uses.
type Kind int

const (
	KindStream Kind = iota
	KindKeyValue
	KindObjectStore
)

// BackupPolicy tells `chatto backup` what to do with a resource.
type BackupPolicy int

const (
	// BackupInclude backs up the resource.
	BackupInclude BackupPolicy = iota
	// BackupSkip never backs up the resource.
	BackupSkip
	// BackupSkipUnlessKeys backs up the resource only with --include-keys.
	BackupSkipUnlessKeys
)

// Resource describes one JetStream resource.
type Resource struct {
	// Name is the logical resource name, for example RUNTIME_STATE.
	Name string
	Kind Kind
	// Backup is the backup policy. SkipReason is the operator-facing reason
	// when the policy skips the resource.
	Backup     BackupPolicy
	SkipReason string
}

// StreamName returns the name of the stream that holds the resource's data.
// Backups and stream listings use this name.
func (r Resource) StreamName() string {
	switch r.Kind {
	case KindKeyValue:
		return KeyValueStream(r.Name)
	case KindObjectStore:
		return ObjectStoreStream(r.Name)
	default:
		return r.Name
	}
}

// KeyValueStream returns the underlying stream name of a key-value bucket.
func KeyValueStream(bucket string) string { return "KV_" + bucket }

// ObjectStoreStream returns the underlying stream name of an Object Store.
func ObjectStoreStream(bucket string) string { return "OBJ_" + bucket }

const (
	skipCache     = "cache (regeneratable)"
	skipEphemeral = "ephemeral (memory storage)"
)

var current = []Resource{
	{Name: EVT, Kind: KindStream, Backup: BackupInclude},
	{Name: Notifications, Kind: KindStream, Backup: BackupInclude},
	{Name: Log, Kind: KindStream, Backup: BackupSkip, SkipReason: "retained diagnostics (not recovery state)"},
	{Name: EncryptionKeys, Kind: KindKeyValue, Backup: BackupSkipUnlessKeys, SkipReason: "security (keys excluded from backups; pass --include-keys to override)"},
	{Name: RuntimeState, Kind: KindKeyValue, Backup: BackupInclude},
	{Name: MemoryCache, Kind: KindKeyValue, Backup: BackupSkip, SkipReason: skipEphemeral},
	{Name: ServerAssets, Kind: KindObjectStore, Backup: BackupInclude},
	{Name: AssetCache, Kind: KindObjectStore, Backup: BackupSkip, SkipReason: skipCache},
	// Snapshots are disposable, but a backup keeps them so that a restored
	// server can start without a full replay.
	{Name: ProjectionSnapshots, Kind: KindObjectStore, Backup: BackupInclude},
}

// legacy lists resources that earlier versions created. Current binaries do
// not provision them, but nothing deletes them, so they can still exist on
// upgraded servers. Backups must keep skipping them.
var legacy = []Resource{
	{Name: NeighborhoodImages, Kind: KindObjectStore, Backup: BackupSkip, SkipReason: skipCache},
	{Name: "USER_PRESENCE", Kind: KindKeyValue, Backup: BackupSkip, SkipReason: skipEphemeral},
	{Name: "CALL_STATE", Kind: KindKeyValue, Backup: BackupSkip, SkipReason: skipEphemeral},
	{Name: "LINK_PREVIEW_CACHE", Kind: KindKeyValue, Backup: BackupSkip, SkipReason: skipCache},
	{Name: "AUTH_TOKENS", Kind: KindKeyValue, Backup: BackupSkip, SkipReason: "security (prevents token leakage)"},
}

// Current returns every resource that the current binary can provision,
// including resources that only optional features create.
func Current() []Resource { return slices.Clone(current) }

// ForStream returns the current or legacy resource whose data lives in the
// named stream.
func ForStream(streamName string) (Resource, bool) {
	for _, resources := range [][]Resource{current, legacy} {
		for _, resource := range resources {
			if resource.StreamName() == streamName {
				return resource, true
			}
		}
	}
	return Resource{}, false
}

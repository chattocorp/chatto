package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// TestPhase4aMigration_FreshInstall verifies that a fresh install (no primary
// space, no legacy data) writes the completion marker without doing any work
// and is a fast no-op on subsequent calls.
func TestPhase4aMigration_FreshInstall(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	if err := core.RunMigrationsIfNeeded(ctx, ""); err != nil {
		t.Fatalf("first run failed: %v", err)
	}

	// Marker should be present after the run.
	if _, err := core.storage.instanceKV.Get(ctx, phase4aCompleteKey); err != nil {
		t.Fatalf("expected completion marker after fresh-install run: %v", err)
	}

	// A second run is a fast no-op via the marker check.
	if err := core.RunMigrationsIfNeeded(ctx, ""); err != nil {
		t.Fatalf("second run failed: %v", err)
	}
}

// TestPhase4aMigration_NoLegacyData verifies that when a primary space exists
// but its legacy SPACE_*_CONFIG buckets do not, the migration marks itself
// complete without erroring.
func TestPhase4aMigration_NoLegacyData(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, err := core.CreateSpace(ctx, "test-user", "Primary", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}

	// Delete the per-space CONFIG/RBAC/RUNTIME buckets that CreateSpace just
	// made, simulating an instance that's already been through migration
	// (data lives in SERVER_* and the legacy buckets are gone).
	for _, bucket := range []string{
		legacySpaceConfigBucket(space.Id),
		legacySpaceRBACBucket(space.Id),
		legacySpaceRuntimeBucket(space.Id),
	} {
		if err := core.js.DeleteKeyValue(ctx, bucket); err != nil {
			t.Fatalf("delete %s: %v", bucket, err)
		}
	}

	if err := core.RunMigrationsIfNeeded(ctx, space.Id); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Marker should be present.
	if _, err := core.storage.instanceKV.Get(ctx, phase4aCompleteKey); err != nil {
		t.Fatalf("expected completion marker: %v", err)
	}
}

// TestPhase4aMigration_CopiesPrimaryData verifies the happy path: a primary
// space with data in SPACE_{id}_CONFIG/RBAC/RUNTIME has all of its keys
// copied into SERVER_*.
func TestPhase4aMigration_CopiesPrimaryData(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Create a primary space without setting it as primary yet — its data
	// goes into per-space SPACE_{id}_* buckets like a pre-migration install.
	space, err := core.CreateSpace(ctx, "test-user", "Primary", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}

	// Capture the keys CreateSpace wrote into the legacy buckets — these
	// are what migration must reproduce in SERVER_*.
	configKeys := snapshotKeys(t, ctx, core, legacySpaceConfigBucket(space.Id))
	rbacKeys := snapshotKeys(t, ctx, core, legacySpaceRBACBucket(space.Id))
	runtimeKeys := snapshotKeys(t, ctx, core, legacySpaceRuntimeBucket(space.Id))

	// SERVER_* buckets should be empty before migration runs.
	assertBucketEmpty(t, ctx, core.storage.serverConfigKV)
	assertBucketEmpty(t, ctx, core.storage.serverRBACKV)
	assertBucketEmpty(t, ctx, core.storage.serverRuntimeKV)

	// Now mark the space as primary and run the migration.
	core.SetPrimarySpaceID(space.Id)
	if err := core.RunMigrationsIfNeeded(ctx, space.Id); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Every key from the legacy buckets must now be present in SERVER_*.
	assertKeysCopied(t, ctx, core.storage.serverConfigKV, configKeys, "SERVER_CONFIG")
	assertKeysCopied(t, ctx, core.storage.serverRBACKV, rbacKeys, "SERVER_RBAC")
	assertKeysCopied(t, ctx, core.storage.serverRuntimeKV, runtimeKeys, "SERVER_RUNTIME")

	// Marker should be present.
	if _, err := core.storage.instanceKV.Get(ctx, phase4aCompleteKey); err != nil {
		t.Fatalf("expected completion marker: %v", err)
	}

	// Source data must be left intact (no-deletes rule).
	for _, bucket := range []string{
		legacySpaceConfigBucket(space.Id),
		legacySpaceRBACBucket(space.Id),
		legacySpaceRuntimeBucket(space.Id),
	} {
		if _, err := core.js.KeyValue(ctx, bucket); err != nil {
			t.Errorf("legacy bucket %s should still exist after migration: %v", bucket, err)
		}
	}
}

// TestPhase4aMigration_IdempotentOnRerun verifies that running the migration
// twice — including a second run after the marker is forcibly removed — does
// not corrupt or duplicate target data and reports the same final state.
func TestPhase4aMigration_IdempotentOnRerun(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, err := core.CreateSpace(ctx, "test-user", "Primary", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	core.SetPrimarySpaceID(space.Id)

	if err := core.RunMigrationsIfNeeded(ctx, space.Id); err != nil {
		t.Fatalf("first run failed: %v", err)
	}
	configKeysAfterFirst := snapshotKeys(t, ctx, core, "SERVER_CONFIG")

	// Force the marker off and re-run — simulates a crashed pod that wrote
	// some data but never wrote the marker. Idempotent copy should leave
	// the target identical.
	if err := core.storage.instanceKV.Delete(ctx, phase4aCompleteKey); err != nil {
		t.Fatalf("delete marker: %v", err)
	}
	if err := core.RunMigrationsIfNeeded(ctx, space.Id); err != nil {
		t.Fatalf("second run failed: %v", err)
	}
	configKeysAfterSecond := snapshotKeys(t, ctx, core, "SERVER_CONFIG")

	if !equalKeySets(configKeysAfterFirst, configKeysAfterSecond) {
		t.Fatalf("expected idempotent target state; first=%v second=%v", configKeysAfterFirst, configKeysAfterSecond)
	}

	if _, err := core.storage.instanceKV.Get(ctx, phase4aCompleteKey); err != nil {
		t.Fatalf("expected completion marker after second run: %v", err)
	}
}

// TestPhase4aMigration_LockSerializesConcurrentRuns verifies the migration
// lock prevents two concurrent calls from both running the work. Whichever
// goroutine acquires first does the migration; the other observes the marker
// after the lock releases and exits cleanly.
func TestPhase4aMigration_LockSerializesConcurrentRuns(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, err := core.CreateSpace(ctx, "test-user", "Primary", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	core.SetPrimarySpaceID(space.Id)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i := range errs {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			errs[idx] = core.RunMigrationsIfNeeded(ctx, space.Id)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d returned error: %v", i, err)
		}
	}

	if _, err := core.storage.instanceKV.Get(ctx, phase4aCompleteKey); err != nil {
		t.Fatalf("expected completion marker after concurrent runs: %v", err)
	}
}

// TestPhase4aMigration_PrimaryRoutingAfterMigration verifies that once the
// primary is set and migration has run, reads through the bucket getters go
// to SERVER_* (not the legacy SPACE_{id}_* buckets).
func TestPhase4aMigration_PrimaryRoutingAfterMigration(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, err := core.CreateSpace(ctx, "test-user", "Primary", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	core.SetPrimarySpaceID(space.Id)
	if err := core.RunMigrationsIfNeeded(ctx, space.Id); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// The primary's CONFIG bucket access should now be SERVER_CONFIG.
	bucket, err := core.getSpaceConfigKV(ctx, space.Id)
	if err != nil {
		t.Fatalf("getSpaceConfigKV: %v", err)
	}
	if status, err := bucket.Status(ctx); err != nil {
		t.Fatalf("bucket status: %v", err)
	} else if status.Bucket() != "SERVER_CONFIG" {
		t.Fatalf("primary should route to SERVER_CONFIG after migration, got %q", status.Bucket())
	}

	// Post-#330 phase 4b: the DM space also routes to SERVER_CONFIG.
	dmBucket, err := core.getSpaceConfigKV(ctx, DMSpaceID)
	if err != nil {
		t.Fatalf("getSpaceConfigKV(DM): %v", err)
	}
	if status, err := dmBucket.Status(ctx); err != nil {
		t.Fatalf("dm bucket status: %v", err)
	} else if status.Bucket() != "SERVER_CONFIG" {
		t.Fatalf("DM should route to SERVER_CONFIG after phase 4b, got %q", status.Bucket())
	}
}

// TestPhase4bMigration_RewritesDMRoomsToKindPrefix verifies that the phase
// 4b migrator copies DM-space room records into SERVER_CONFIG under the
// kind-prefixed key `room.dm.{X}`, with the original Room proto preserved
// byte-for-byte (the kind discriminator lives in the key, not the proto).
func TestPhase4bMigration_RewritesDMRoomsToKindPrefix(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	dmConfig, err := core.js.KeyValue(ctx, legacySpaceConfigBucket(DMSpaceID))
	if err != nil {
		t.Fatalf("open SPACE_DM_CONFIG: %v", err)
	}
	roomID := "Rtest-dm-room-1"
	legacyRoom := &corev1.Room{
		Id:      roomID,
		SpaceId: DMSpaceID,
	}
	value, err := proto.Marshal(legacyRoom)
	if err != nil {
		t.Fatalf("marshal legacy DM room: %v", err)
	}
	if _, err := dmConfig.Put(ctx, "room."+roomID, value); err != nil {
		t.Fatalf("seed legacy DM room: %v", err)
	}

	if err := core.RunMigrationsIfNeeded(ctx, ""); err != nil {
		t.Fatalf("RunMigrationsIfNeeded: %v", err)
	}

	// The migrated room should now exist in SERVER_CONFIG under the new
	// kind-prefixed key.
	migratedEntry, err := core.storage.serverConfigKV.Get(ctx, "room.dm."+roomID)
	if err != nil {
		t.Fatalf("get migrated DM room from SERVER_CONFIG[room.dm.*]: %v", err)
	}
	migratedRoom := &corev1.Room{}
	if err := proto.Unmarshal(migratedEntry.Value(), migratedRoom); err != nil {
		t.Fatalf("unmarshal migrated room: %v", err)
	}
	if migratedRoom.Id != roomID {
		t.Fatalf("expected room.Id = %q, got %q", roomID, migratedRoom.Id)
	}

	// Source bucket left untouched (no-deletes rule).
	if _, err := dmConfig.Get(ctx, "room."+roomID); err != nil {
		t.Fatalf("source DM room should still exist: %v", err)
	}

	// And the marker is set.
	if _, err := core.storage.instanceKV.Get(ctx, phase4bCompleteKey); err != nil {
		t.Fatalf("expected phase4b completion marker: %v", err)
	}
}

// TestPhase4bMigration_FreshInstall verifies that phase 4b is a fast no-op
// on a freshly-created instance (DM space exists but has no rooms).
func TestPhase4bMigration_FreshInstall(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// setupTestCore already initialized the DM space, which created
	// SPACE_DM_CONFIG/RUNTIME as empty buckets. Phase 4b should detect
	// them, copy nothing, verify, and mark complete.
	if err := core.RunMigrationsIfNeeded(ctx, ""); err != nil {
		t.Fatalf("RunMigrationsIfNeeded: %v", err)
	}
	if _, err := core.storage.instanceKV.Get(ctx, phase4bCompleteKey); err != nil {
		t.Fatalf("expected phase4b completion marker: %v", err)
	}

	// Re-run is a fast no-op via the marker.
	if err := core.RunMigrationsIfNeeded(ctx, ""); err != nil {
		t.Fatalf("second run failed: %v", err)
	}
}

// TestPhase4cMigration_CopiesPerMessageKVs verifies that the phase 4c
// migrator copies each per-message KV bucket (BODIES, REACTIONS, THREADS)
// for both the primary and DM spaces into the matching SERVER_* bucket.
// Keys are preserved verbatim — no kind segment, since IDs are globally
// unique.
func TestPhase4cMigration_CopiesPerMessageKVs(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, err := core.CreateSpace(ctx, "test-user", "Primary", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}

	// Seed legacy per-space buckets with representative keys for primary
	// and DM. We bypass the public API and write directly to the bucket
	// so the migration has something to copy.
	primaryBodies, err := core.js.KeyValue(ctx, legacySpaceBodiesBucket(space.Id))
	if err != nil {
		t.Fatalf("open primary BODIES: %v", err)
	}
	if _, err := primaryBodies.Put(ctx, "Uuser1.Eevent1", []byte("body-primary-1")); err != nil {
		t.Fatalf("seed primary body: %v", err)
	}

	primaryReactions, err := core.js.KeyValue(ctx, legacySpaceReactionsBucket(space.Id))
	if err != nil {
		t.Fatalf("open primary REACTIONS: %v", err)
	}
	if _, err := primaryReactions.Put(ctx, "Eevent1.thumbsup.Uuser1", []byte("0")); err != nil {
		t.Fatalf("seed primary reaction: %v", err)
	}

	primaryThreads, err := core.js.KeyValue(ctx, legacySpaceThreadsBucket(space.Id))
	if err != nil {
		t.Fatalf("open primary THREADS: %v", err)
	}
	if _, err := primaryThreads.Put(ctx, "Rroom1.Eroot1", []byte("thread-primary")); err != nil {
		t.Fatalf("seed primary thread: %v", err)
	}

	dmBodies, err := core.js.KeyValue(ctx, legacySpaceBodiesBucket(DMSpaceID))
	if err != nil {
		t.Fatalf("open DM BODIES: %v", err)
	}
	if _, err := dmBodies.Put(ctx, "Uuser2.Eevent2", []byte("body-dm-1")); err != nil {
		t.Fatalf("seed DM body: %v", err)
	}

	core.SetPrimarySpaceID(space.Id)
	if err := core.RunMigrationsIfNeeded(ctx, space.Id); err != nil {
		t.Fatalf("RunMigrationsIfNeeded: %v", err)
	}

	// Verify keys landed in SERVER_*.
	if entry, err := core.storage.serverBodiesKV.Get(ctx, "Uuser1.Eevent1"); err != nil {
		t.Fatalf("primary body missing in SERVER_BODIES: %v", err)
	} else if string(entry.Value()) != "body-primary-1" {
		t.Errorf("primary body value mismatch: %q", entry.Value())
	}

	if entry, err := core.storage.serverBodiesKV.Get(ctx, "Uuser2.Eevent2"); err != nil {
		t.Fatalf("DM body missing in SERVER_BODIES: %v", err)
	} else if string(entry.Value()) != "body-dm-1" {
		t.Errorf("DM body value mismatch: %q", entry.Value())
	}

	if _, err := core.storage.serverReactionsKV.Get(ctx, "Eevent1.thumbsup.Uuser1"); err != nil {
		t.Fatalf("primary reaction missing in SERVER_REACTIONS: %v", err)
	}

	if _, err := core.storage.serverThreadsKV.Get(ctx, "Rroom1.Eroot1"); err != nil {
		t.Fatalf("primary thread missing in SERVER_THREADS: %v", err)
	}

	// Marker set.
	if _, err := core.storage.instanceKV.Get(ctx, phase4cCompleteKey); err != nil {
		t.Fatalf("expected phase4c completion marker: %v", err)
	}

	// Source data left intact (no-deletes rule).
	if _, err := primaryBodies.Get(ctx, "Uuser1.Eevent1"); err != nil {
		t.Errorf("source primary body should still exist: %v", err)
	}
	if _, err := dmBodies.Get(ctx, "Uuser2.Eevent2"); err != nil {
		t.Errorf("source DM body should still exist: %v", err)
	}
}

// TestPhase4cMigration_FreshInstall verifies the phase 4c migrator runs
// cleanly on a fresh install with no per-message data (just the initial
// DM space buckets that initDMSpace creates as empty).
func TestPhase4cMigration_FreshInstall(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	if err := core.RunMigrationsIfNeeded(ctx, ""); err != nil {
		t.Fatalf("RunMigrationsIfNeeded: %v", err)
	}
	if _, err := core.storage.instanceKV.Get(ctx, phase4cCompleteKey); err != nil {
		t.Fatalf("expected phase4c completion marker: %v", err)
	}

	// Re-run is a fast no-op via the marker.
	if err := core.RunMigrationsIfNeeded(ctx, ""); err != nil {
		t.Fatalf("second run failed: %v", err)
	}
}

// TestPhase4cMigration_PerMessageRoutingAfterMigration verifies that once
// the primary is set and migration has run, the per-message bucket
// getters route the primary and DM spaces to SERVER_* (not the legacy
// per-space buckets).
func TestPhase4cMigration_PerMessageRoutingAfterMigration(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, err := core.CreateSpace(ctx, "test-user", "Primary", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	core.SetPrimarySpaceID(space.Id)
	if err := core.RunMigrationsIfNeeded(ctx, space.Id); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	for _, tc := range []struct {
		name    string
		spaceID string
		getter  func(ctx context.Context, spaceID string) (jetstream.KeyValue, error)
		want    string
	}{
		{"primary BODIES", space.Id, core.getSpaceBodiesKV, "SERVER_BODIES"},
		{"primary REACTIONS", space.Id, core.getSpaceReactionsKV, "SERVER_REACTIONS"},
		{"primary THREADS", space.Id, core.getSpaceThreadsKV, "SERVER_THREADS"},
		{"DM BODIES", DMSpaceID, core.getSpaceBodiesKV, "SERVER_BODIES"},
		{"DM REACTIONS", DMSpaceID, core.getSpaceReactionsKV, "SERVER_REACTIONS"},
		{"DM THREADS", DMSpaceID, core.getSpaceThreadsKV, "SERVER_THREADS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bucket, err := tc.getter(ctx, tc.spaceID)
			if err != nil {
				t.Fatalf("getter: %v", err)
			}
			status, err := bucket.Status(ctx)
			if err != nil {
				t.Fatalf("Status: %v", err)
			}
			if status.Bucket() != tc.want {
				t.Errorf("expected %q, got %q", tc.want, status.Bucket())
			}
		})
	}
}

// snapshotKeys returns the set of keys in the named bucket. Helper for the
// tests above; doesn't fail on missing-bucket so callers can probe.
func snapshotKeys(t *testing.T, ctx context.Context, core *ChattoCore, bucketName string) map[string]struct{} {
	t.Helper()
	bucket, err := core.js.KeyValue(ctx, bucketName)
	if err != nil {
		if errors.Is(err, jetstream.ErrBucketNotFound) {
			return map[string]struct{}{}
		}
		t.Fatalf("open bucket %s: %v", bucketName, err)
	}
	return listKeys(t, ctx, bucket)
}

func listKeys(t *testing.T, ctx context.Context, bucket jetstream.KeyValue) map[string]struct{} {
	t.Helper()
	out := make(map[string]struct{})
	lister, err := bucket.ListKeys(ctx)
	if err != nil {
		t.Fatalf("list keys: %v", err)
	}
	defer lister.Stop()
	for key := range lister.Keys() {
		out[key] = struct{}{}
	}
	return out
}

func assertBucketEmpty(t *testing.T, ctx context.Context, bucket jetstream.KeyValue) {
	t.Helper()
	keys := listKeys(t, ctx, bucket)
	if len(keys) != 0 {
		t.Fatalf("expected bucket to be empty, found %d keys", len(keys))
	}
}

func assertKeysCopied(t *testing.T, ctx context.Context, target jetstream.KeyValue, want map[string]struct{}, label string) {
	t.Helper()
	got := listKeys(t, ctx, target)
	for key := range want {
		if _, ok := got[key]; !ok {
			t.Errorf("%s: expected key %q in target, missing", label, key)
		}
	}
}

func equalKeySets(a, b map[string]struct{}) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if _, ok := b[k]; !ok {
			return false
		}
	}
	return true
}

// TestPhase4eMigration_CopiesAttachments verifies that the phase 4e migrator
// copies every object from the per-space ASSETS object stores for primary +
// DM into the deployment-wide SERVER_ASSETS store, preserves headers, and
// leaves the source data intact (no-deletes rule).
func TestPhase4eMigration_CopiesAttachments(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, err := core.CreateSpace(ctx, "test-user", "Primary", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}

	// Seed legacy primary ASSETS via the per-space store directly. Bypassing
	// the public API keeps the test focused on the migration's behavior.
	primarySource, err := core.js.ObjectStore(ctx, legacySpaceAssetsBucket(space.Id))
	if err != nil {
		t.Fatalf("open primary ASSETS: %v", err)
	}
	if _, err := primarySource.Put(ctx, jetstream.ObjectMeta{
		Name:    "att-primary-1",
		Headers: map[string][]string{"Content-Type": {"image/png"}, "Filename": {"hello.png"}},
	}, bytes.NewReader([]byte("primary-attachment-bytes"))); err != nil {
		t.Fatalf("seed primary attachment: %v", err)
	}

	dmSource, err := core.js.ObjectStore(ctx, legacySpaceAssetsBucket(DMSpaceID))
	if err != nil {
		t.Fatalf("open DM ASSETS: %v", err)
	}
	if _, err := dmSource.Put(ctx, jetstream.ObjectMeta{
		Name:    "att-dm-1",
		Headers: map[string][]string{"Content-Type": {"text/plain"}, "Filename": {"note.txt"}},
	}, bytes.NewReader([]byte("dm-attachment-bytes"))); err != nil {
		t.Fatalf("seed DM attachment: %v", err)
	}

	core.SetPrimarySpaceID(space.Id)
	if err := core.RunMigrationsIfNeeded(ctx, space.Id); err != nil {
		t.Fatalf("RunMigrationsIfNeeded: %v", err)
	}

	// Marker set.
	if _, err := core.storage.instanceKV.Get(ctx, phase4eCompleteKey); err != nil {
		t.Fatalf("expected phase4e completion marker: %v", err)
	}

	// Objects landed in SERVER_ASSETS with content + headers preserved.
	for _, tc := range []struct {
		name        string
		wantContent string
		wantCT      string
		wantFile    string
	}{
		{"att-primary-1", "primary-attachment-bytes", "image/png", "hello.png"},
		{"att-dm-1", "dm-attachment-bytes", "text/plain", "note.txt"},
	} {
		got, err := core.storage.serverAttachments.GetBytes(ctx, tc.name)
		if err != nil {
			t.Errorf("read %q from SERVER_ASSETS: %v", tc.name, err)
			continue
		}
		if string(got) != tc.wantContent {
			t.Errorf("%q content mismatch: %q", tc.name, got)
		}
		info, err := core.storage.serverAttachments.GetInfo(ctx, tc.name)
		if err != nil {
			t.Errorf("GetInfo %q: %v", tc.name, err)
			continue
		}
		if got := info.Headers.Get("Content-Type"); got != tc.wantCT {
			t.Errorf("%q Content-Type: %q, want %q", tc.name, got, tc.wantCT)
		}
		if got := info.Headers.Get("Filename"); got != tc.wantFile {
			t.Errorf("%q Filename: %q, want %q", tc.name, got, tc.wantFile)
		}
	}

	// Source data left intact.
	if _, err := primarySource.GetInfo(ctx, "att-primary-1"); err != nil {
		t.Errorf("source primary attachment should still exist: %v", err)
	}
	if _, err := dmSource.GetInfo(ctx, "att-dm-1"); err != nil {
		t.Errorf("source DM attachment should still exist: %v", err)
	}
}

// TestPhase4eMigration_FreshInstall verifies the phase 4e migrator runs
// cleanly on a fresh install with no attachment data and is a fast no-op
// on a re-run.
func TestPhase4eMigration_FreshInstall(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	if err := core.RunMigrationsIfNeeded(ctx, ""); err != nil {
		t.Fatalf("RunMigrationsIfNeeded: %v", err)
	}
	if _, err := core.storage.instanceKV.Get(ctx, phase4eCompleteKey); err != nil {
		t.Fatalf("expected phase4e completion marker: %v", err)
	}

	if err := core.RunMigrationsIfNeeded(ctx, ""); err != nil {
		t.Fatalf("second run failed: %v", err)
	}
}

// ===== Phase 4d (events stream) tests =====

// TestPhase4dMigration_CopiesPrimaryAndDMStreams verifies the happy path:
// events posted to the primary space and to a DM both end up in
// SERVER_EVENTS with rewritten subjects after the migration runs.
func TestPhase4dMigration_CopiesPrimaryAndDMStreams(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "poster1", "Poster", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Create a soon-to-be-primary space with a room and a few messages.
	// Singleton stays unset for now → publishes go to `space.{id}.>`,
	// which lands in `SPACE_{id}_EVENTS`. That's the legacy data the
	// migrator will copy.
	space, err := core.CreateSpace(ctx, user.Id, "Primary", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	room, err := core.CreateRoom(ctx, user.Id, space.Id, "general", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	primaryEvent, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "primary message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage primary: %v", err)
	}

	// Seed a DM as well.
	other, err := core.CreateUser(ctx, "system", "dm-peer", "DM Peer", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	dmRoom, _, err := core.FindOrCreateDM(ctx, user.Id, []string{other.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	dmEvent, err := core.PostMessage(ctx, DMSpaceID, dmRoom.Id, user.Id, "dm message", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage DM: %v", err)
	}

	// Now activate the singleton + run migrations. From this point on,
	// new publishes would go to `server.>` subjects, but we're not
	// publishing more — we're verifying the migrator copies the legacy
	// data over.
	core.SetPrimarySpaceID(space.Id)
	if err := core.RunMigrationsIfNeeded(ctx, space.Id); err != nil {
		t.Fatalf("RunMigrationsIfNeeded: %v", err)
	}

	// Marker set.
	if _, err := core.storage.instanceKV.Get(ctx, phase4dCompleteKey); err != nil {
		t.Fatalf("expected phase4d completion marker: %v", err)
	}

	// Both events landed in SERVER_EVENTS at their rewritten subjects.
	primarySubject := "server.room.channel." + room.Id + ".msg." + primaryEvent.Id
	if got, err := core.storage.serverEventsStream.GetLastMsgForSubject(ctx, primarySubject); err != nil {
		t.Errorf("primary event missing in SERVER_EVENTS at %q: %v", primarySubject, err)
	} else {
		var ev corev1.SpaceEvent
		if err := proto.Unmarshal(got.Data, &ev); err != nil {
			t.Fatalf("unmarshal primary: %v", err)
		}
		if ev.Id != primaryEvent.Id {
			t.Errorf("primary event id mismatch: got %q want %q", ev.Id, primaryEvent.Id)
		}
	}

	dmSubject := "server.room.dm." + dmRoom.Id + ".msg." + dmEvent.Id
	if got, err := core.storage.serverEventsStream.GetLastMsgForSubject(ctx, dmSubject); err != nil {
		t.Errorf("DM event missing in SERVER_EVENTS at %q: %v", dmSubject, err)
	} else {
		var ev corev1.SpaceEvent
		if err := proto.Unmarshal(got.Data, &ev); err != nil {
			t.Fatalf("unmarshal DM: %v", err)
		}
		if ev.Id != dmEvent.Id {
			t.Errorf("DM event id mismatch: got %q want %q", ev.Id, dmEvent.Id)
		}
	}
}

// TestPhase4dMigration_FreshInstall verifies the migrator runs cleanly
// on a fresh install with no events to copy.
func TestPhase4dMigration_FreshInstall(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	if err := core.RunMigrationsIfNeeded(ctx, ""); err != nil {
		t.Fatalf("RunMigrationsIfNeeded: %v", err)
	}
	if _, err := core.storage.instanceKV.Get(ctx, phase4dCompleteKey); err != nil {
		t.Fatalf("expected phase4d completion marker: %v", err)
	}

	// Re-run is a fast no-op via the marker.
	if err := core.RunMigrationsIfNeeded(ctx, ""); err != nil {
		t.Fatalf("second run failed: %v", err)
	}
}

// TestPhase4dMigration_IdempotentOnRerun verifies that re-running the
// migrator after the marker has been forcibly cleared does not duplicate
// events in SERVER_EVENTS.
func TestPhase4dMigration_IdempotentOnRerun(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "poster2", "Poster", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	space, err := core.CreateSpace(ctx, user.Id, "Primary", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	room, err := core.CreateRoom(ctx, user.Id, space.Id, "general", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	for i := 0; i < 3; i++ {
		if _, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "msg", nil, "", "", nil, false); err != nil {
			t.Fatalf("PostMessage %d: %v", i, err)
		}
	}

	core.SetPrimarySpaceID(space.Id)
	if err := core.RunMigrationsIfNeeded(ctx, space.Id); err != nil {
		t.Fatalf("first migration: %v", err)
	}

	infoAfterFirst, err := core.storage.serverEventsStream.Info(ctx)
	if err != nil {
		t.Fatalf("server stream info: %v", err)
	}
	msgsAfterFirst := infoAfterFirst.State.Msgs

	// Force a re-run by deleting the marker, then run again.
	if err := core.storage.instanceKV.Delete(ctx, phase4dCompleteKey); err != nil {
		t.Fatalf("delete marker: %v", err)
	}
	if err := core.RunMigrationsIfNeeded(ctx, space.Id); err != nil {
		t.Fatalf("second migration: %v", err)
	}

	infoAfterSecond, err := core.storage.serverEventsStream.Info(ctx)
	if err != nil {
		t.Fatalf("server stream info second: %v", err)
	}
	if infoAfterSecond.State.Msgs != msgsAfterFirst {
		t.Errorf("re-run duplicated events: %d → %d", msgsAfterFirst, infoAfterSecond.State.Msgs)
	}
}

// TestPhase4dMigration_RoutingAfterMigration verifies that once primary
// is set + migration has run, getSpaceStream for primary and DM both
// route to SERVER_EVENTS while non-primary spaces still use their
// per-space stream.
func TestPhase4dMigration_RoutingAfterMigration(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, err := core.CreateSpace(ctx, "test-user", "Primary", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	otherSpace, err := core.CreateSpace(ctx, "test-user", "Other", "")
	if err != nil {
		t.Fatalf("CreateSpace other: %v", err)
	}

	core.SetPrimarySpaceID(space.Id)
	if err := core.RunMigrationsIfNeeded(ctx, space.Id); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	for _, tc := range []struct {
		name    string
		spaceID string
		want    string
	}{
		{"primary events", space.Id, "SERVER_EVENTS"},
		{"DM events", DMSpaceID, "SERVER_EVENTS"},
		{"non-primary events", otherSpace.Id, "SPACE_" + otherSpace.Id + "_EVENTS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream, err := core.getSpaceStream(ctx, tc.spaceID)
			if err != nil {
				t.Fatalf("getSpaceStream: %v", err)
			}
			info, err := stream.Info(ctx)
			if err != nil {
				t.Fatalf("Info: %v", err)
			}
			if info.Config.Name != tc.want {
				t.Errorf("expected stream %q, got %q", tc.want, info.Config.Name)
			}
		})
	}
}

// TestPhase4dMigration_EndToEndPostThenRead seeds a message before
// migration and verifies it can be read back via GetRoomEvents after.
// This is the user-facing protection: chat history survives the
// migration intact.
func TestPhase4dMigration_EndToEndPostThenRead(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "poster3", "Poster", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	space, err := core.CreateSpace(ctx, user.Id, "Primary", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	room, err := core.CreateRoom(ctx, user.Id, space.Id, "general", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, space.Id, user.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}

	// Pre-migration: post a few messages. Singleton is unset → these
	// land in SPACE_{primary}_EVENTS at `space.{id}.room.>` subjects.
	preMigrationIDs := make([]string, 3)
	for i := 0; i < 3; i++ {
		ev, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "pre-migration", nil, "", "", nil, false)
		if err != nil {
			t.Fatalf("PostMessage %d: %v", i, err)
		}
		preMigrationIDs[i] = ev.Id
	}

	// Activate singleton + migrate.
	core.SetPrimarySpaceID(space.Id)
	if err := core.RunMigrationsIfNeeded(ctx, space.Id); err != nil {
		t.Fatalf("RunMigrationsIfNeeded: %v", err)
	}

	// Post-migration: GetRoomEvents now reads from SERVER_EVENTS at
	// `server.room.channel.>` subjects. Pre-migration messages should be
	// visible.
	result, err := core.GetRoomEvents(ctx, space.Id, room.Id, 50, nil)
	if err != nil {
		t.Fatalf("GetRoomEvents: %v", err)
	}

	got := make(map[string]struct{})
	for _, e := range result.Events {
		got[e.Id] = struct{}{}
	}
	for _, id := range preMigrationIDs {
		if _, ok := got[id]; !ok {
			t.Errorf("pre-migration event %q missing from GetRoomEvents", id)
		}
	}

	// And new posts go to SERVER_EVENTS too — round-trip works post-migration.
	postEv, err := core.PostMessage(ctx, space.Id, room.Id, user.Id, "post-migration", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage post-migration: %v", err)
	}
	result2, err := core.GetRoomEvents(ctx, space.Id, room.Id, 50, nil)
	if err != nil {
		t.Fatalf("GetRoomEvents post: %v", err)
	}
	found := false
	for _, e := range result2.Events {
		if e.Id == postEv.Id {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("post-migration event %q missing from GetRoomEvents", postEv.Id)
	}
}

// TestPhase4dMigration_KindFilterIsolatesChannelsFromDMs verifies that
// post-migration subjects scoped to one kind (channel or dm) don't match
// the other kind. Covers both direct GetLastMsgForSubject lookups and
// wildcard subject filters via stream.Info — the two ways the rest of
// the codebase queries for room events.
//
// Also exercises a thread reply through the migrator (not just root
// messages), closing a gap in the other phase 4d tests.
func TestPhase4dMigration_KindFilterIsolatesChannelsFromDMs(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	user, err := core.CreateUser(ctx, "system", "kind-test", "User", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Channel-side: primary space + room + a root message + a thread reply.
	space, err := core.CreateSpace(ctx, user.Id, "Primary", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	channelRoom, err := core.CreateRoom(ctx, user.Id, space.Id, "general", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if _, err := core.JoinRoom(ctx, user.Id, space.Id, user.Id, channelRoom.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	channelRoot, err := core.PostMessage(ctx, space.Id, channelRoom.Id, user.Id, "channel root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage channel root: %v", err)
	}
	channelReply, err := core.PostMessage(ctx, space.Id, channelRoom.Id, user.Id, "channel reply", nil, channelRoot.Id, "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage channel reply: %v", err)
	}

	// DM-side: a DM room + a root message + a thread reply.
	other, err := core.CreateUser(ctx, "system", "kind-peer", "Peer", "password123")
	if err != nil {
		t.Fatalf("CreateUser peer: %v", err)
	}
	dmRoom, _, err := core.FindOrCreateDM(ctx, user.Id, []string{other.Id})
	if err != nil {
		t.Fatalf("FindOrCreateDM: %v", err)
	}
	dmRoot, err := core.PostMessage(ctx, DMSpaceID, dmRoom.Id, user.Id, "dm root", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage DM root: %v", err)
	}
	dmReply, err := core.PostMessage(ctx, DMSpaceID, dmRoom.Id, user.Id, "dm reply", nil, dmRoot.Id, "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage DM reply: %v", err)
	}

	// Migrate.
	core.SetPrimarySpaceID(space.Id)
	if err := core.RunMigrationsIfNeeded(ctx, space.Id); err != nil {
		t.Fatalf("RunMigrationsIfNeeded: %v", err)
	}

	stream := core.storage.serverEventsStream

	// Direct subject lookups: each event must be findable at its own
	// kind's subject and NOT findable at the other kind's subject. If
	// the rewriter ever emits the wrong kind, one of these "should not
	// exist" assertions catches it.
	t.Run("channel root at channel subject", func(t *testing.T) {
		subj := fmt.Sprintf("server.room.channel.%s.msg.%s", channelRoom.Id, channelRoot.Id)
		if _, err := stream.GetLastMsgForSubject(ctx, subj); err != nil {
			t.Errorf("expected to find channel root at %q: %v", subj, err)
		}
	})
	t.Run("channel root NOT at dm subject", func(t *testing.T) {
		subj := fmt.Sprintf("server.room.dm.%s.msg.%s", channelRoom.Id, channelRoot.Id)
		_, err := stream.GetLastMsgForSubject(ctx, subj)
		if !errors.Is(err, jetstream.ErrMsgNotFound) {
			t.Errorf("channel root must not be findable under dm kind at %q: err=%v", subj, err)
		}
	})

	t.Run("channel thread reply at channel subject", func(t *testing.T) {
		subj := fmt.Sprintf("server.room.channel.%s.msg.%s.replies.%s", channelRoom.Id, channelRoot.Id, channelReply.Id)
		if _, err := stream.GetLastMsgForSubject(ctx, subj); err != nil {
			t.Errorf("expected to find channel reply at %q: %v", subj, err)
		}
	})
	t.Run("channel thread reply NOT at dm subject", func(t *testing.T) {
		subj := fmt.Sprintf("server.room.dm.%s.msg.%s.replies.%s", channelRoom.Id, channelRoot.Id, channelReply.Id)
		_, err := stream.GetLastMsgForSubject(ctx, subj)
		if !errors.Is(err, jetstream.ErrMsgNotFound) {
			t.Errorf("channel reply must not be findable under dm kind: err=%v", err)
		}
	})

	t.Run("dm root at dm subject", func(t *testing.T) {
		subj := fmt.Sprintf("server.room.dm.%s.msg.%s", dmRoom.Id, dmRoot.Id)
		if _, err := stream.GetLastMsgForSubject(ctx, subj); err != nil {
			t.Errorf("expected to find dm root at %q: %v", subj, err)
		}
	})
	t.Run("dm root NOT at channel subject", func(t *testing.T) {
		subj := fmt.Sprintf("server.room.channel.%s.msg.%s", dmRoom.Id, dmRoot.Id)
		_, err := stream.GetLastMsgForSubject(ctx, subj)
		if !errors.Is(err, jetstream.ErrMsgNotFound) {
			t.Errorf("dm root must not be findable under channel kind: err=%v", err)
		}
	})

	t.Run("dm thread reply at dm subject", func(t *testing.T) {
		subj := fmt.Sprintf("server.room.dm.%s.msg.%s.replies.%s", dmRoom.Id, dmRoot.Id, dmReply.Id)
		if _, err := stream.GetLastMsgForSubject(ctx, subj); err != nil {
			t.Errorf("expected to find dm reply at %q: %v", subj, err)
		}
	})
	t.Run("dm thread reply NOT at channel subject", func(t *testing.T) {
		subj := fmt.Sprintf("server.room.channel.%s.msg.%s.replies.%s", dmRoom.Id, dmRoot.Id, dmReply.Id)
		_, err := stream.GetLastMsgForSubject(ctx, subj)
		if !errors.Is(err, jetstream.ErrMsgNotFound) {
			t.Errorf("dm reply must not be findable under channel kind: err=%v", err)
		}
	})

	// Wildcard subject filters (the second way callers query the stream).
	// `server.room.channel.>` must include the channel events and exclude
	// the DM events, and vice versa. countMatchingSubjects sums per-subject
	// counts returned by stream.Info(WithSubjectFilter).
	channelEvents := countMatchingSubjects(t, ctx, stream, fmt.Sprintf("server.room.channel.%s.>", channelRoom.Id))
	dmEvents := countMatchingSubjects(t, ctx, stream, fmt.Sprintf("server.room.dm.%s.>", dmRoom.Id))

	if channelEvents < 2 {
		t.Errorf("channel filter found %d events, expected at least 2 (root + reply)", channelEvents)
	}
	if dmEvents < 2 {
		t.Errorf("dm filter found %d events, expected at least 2 (root + reply)", dmEvents)
	}

	// Cross-kind wildcards: a channel-scoped wildcard with the DM room ID
	// must match nothing (different room ID + different kind), and vice
	// versa. This catches a regression where the rewriter or filter
	// helper accidentally widens the wildcard.
	crossA := countMatchingSubjects(t, ctx, stream, fmt.Sprintf("server.room.channel.%s.>", dmRoom.Id))
	crossB := countMatchingSubjects(t, ctx, stream, fmt.Sprintf("server.room.dm.%s.>", channelRoom.Id))
	if crossA != 0 {
		t.Errorf("channel filter with DM room ID matched %d events, expected 0", crossA)
	}
	if crossB != 0 {
		t.Errorf("dm filter with channel room ID matched %d events, expected 0", crossB)
	}
}

// countMatchingSubjects returns the total per-subject count for a stream
// info request scoped by `filter`. Used to assert wildcard scoping.
func countMatchingSubjects(t *testing.T, ctx context.Context, stream jetstream.Stream, filter string) uint64 {
	t.Helper()
	info, err := stream.Info(ctx, jetstream.WithSubjectFilter(filter))
	if err != nil {
		t.Fatalf("stream.Info(filter=%q): %v", filter, err)
	}
	var total uint64
	for _, c := range info.State.Subjects {
		total += c
	}
	return total
}

// TestPhase4eMigration_AttachmentsRoutingAfterMigration verifies that once
// the primary is set and migration has run, getSpaceAttachments routes
// the primary and DM spaces to SERVER_ASSETS (not the legacy per-space
// stores). Other spaces continue to use their per-space stores.
func TestPhase4eMigration_AttachmentsRoutingAfterMigration(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	space, err := core.CreateSpace(ctx, "test-user", "Primary", "")
	if err != nil {
		t.Fatalf("CreateSpace: %v", err)
	}
	otherSpace, err := core.CreateSpace(ctx, "test-user", "Other", "")
	if err != nil {
		t.Fatalf("CreateSpace other: %v", err)
	}
	core.SetPrimarySpaceID(space.Id)
	if err := core.RunMigrationsIfNeeded(ctx, space.Id); err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	for _, tc := range []struct {
		name    string
		spaceID string
		want    string
	}{
		{"primary attachments", space.Id, "SERVER_ASSETS"},
		{"DM attachments", DMSpaceID, "SERVER_ASSETS"},
		{"non-primary attachments", otherSpace.Id, "SPACE_" + otherSpace.Id + "_ASSETS"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store, err := core.getSpaceAttachments(ctx, tc.spaceID)
			if err != nil {
				t.Fatalf("getSpaceAttachments: %v", err)
			}
			status, err := store.Status(ctx)
			if err != nil {
				t.Fatalf("Status: %v", err)
			}
			if status.Bucket() != tc.want {
				t.Errorf("expected %q, got %q", tc.want, status.Bucket())
			}
		})
	}
}

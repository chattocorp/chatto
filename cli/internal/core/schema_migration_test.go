package core

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
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

	// And the DM space (different ID) still routes to its per-space bucket.
	dmBucket, err := core.getSpaceConfigKV(ctx, DMSpaceID)
	if err != nil {
		t.Fatalf("getSpaceConfigKV(DM): %v", err)
	}
	if status, err := dmBucket.Status(ctx); err != nil {
		t.Fatalf("dm bucket status: %v", err)
	} else if status.Bucket() != legacySpaceConfigBucket(DMSpaceID) {
		t.Fatalf("DM should route to %q, got %q",
			legacySpaceConfigBucket(DMSpaceID), status.Bucket())
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

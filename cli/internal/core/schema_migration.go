package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// Schema migration coordinates the move from the per-space `SPACE_{id}_*`
// data layout to the unified `SERVER_*` layout described in #354 (phase 4 of
// the #330 instance/space → server consolidation).
//
// Migrations run on instance boot. Completion is recorded with a per-phase
// marker key in `KV_INSTANCE`; a leased lock prevents multiple pods of the
// same deployment from running the same migration concurrently. Source data
// is never mutated or deleted — the legacy `SPACE_*` resources stay in place
// until a separate cleanup follow-up retires them.

const (
	// migrationLockKey is the KV_INSTANCE key used as a leased mutex while a
	// migration is in progress. Owner identity + per-key TTL handle crash
	// recovery: a pod that dies mid-migration eventually loses the lock and
	// another pod can take over.
	migrationLockKey = "migration_lock"

	// migrationLockTTL is how long a held lock lives before another pod is
	// allowed to take it. Long enough to comfortably cover phase-4a work on
	// tiny-to-modest instances; if a migration takes longer, the lock will
	// expire and another pod will pick up the (idempotent) work.
	migrationLockTTL = 5 * time.Minute

	// migrationLockAcquireTimeout caps how long we wait for a contended lock
	// before giving up. Generous enough to outlast one TTL window so a
	// crashed-pod scenario resolves automatically.
	migrationLockAcquireTimeout = 10 * time.Minute

	// migrationLockRetryInterval is the wait between attempts when the lock
	// is held by another pod.
	migrationLockRetryInterval = 2 * time.Second

	// phase4aCompleteKey marks phase 4a as done. Presence-as-truth: the
	// cleanup follow-up deletes this alongside the legacy resources, after
	// which the migrator becomes a permanent no-op.
	phase4aCompleteKey = "migration.phase4a_complete"
)

// RunMigrationsIfNeeded runs any pending schema migrations. Idempotent and
// safe to call concurrently from multiple pods (lock-protected).
//
// Should be called once at boot, after `NewChattoCore` and after the primary
// space ID has been resolved (so phase-4a knows what to migrate).
//
// Production deployments always have a primary configured; `primarySpaceID`
// is only ever empty on truly fresh installs that haven't created their
// first space yet, in which case there's no legacy data and the marker is
// written immediately.
func (c *ChattoCore) RunMigrationsIfNeeded(ctx context.Context, primarySpaceID string) error {
	return c.runPhase4aIfNeeded(ctx, primarySpaceID)
}

// runPhase4aIfNeeded migrates the primary space's CONFIG, RBAC and RUNTIME
// KV buckets from `SPACE_{primary}_*` into the shared `SERVER_*` buckets.
// DM space data is intentionally not touched — that fold-in is a separate
// later phase.
func (c *ChattoCore) runPhase4aIfNeeded(ctx context.Context, primarySpaceID string) error {
	if done, err := c.isMigrationComplete(ctx, phase4aCompleteKey); err != nil {
		return fmt.Errorf("phase4a: check completion marker: %w", err)
	} else if done {
		return nil
	}

	release, err := c.acquireMigrationLock(ctx)
	if err != nil {
		return fmt.Errorf("phase4a: acquire lock: %w", err)
	}
	defer release()

	// Re-check after acquiring the lock — another pod may have just finished.
	if done, err := c.isMigrationComplete(ctx, phase4aCompleteKey); err != nil {
		return fmt.Errorf("phase4a: re-check completion marker: %w", err)
	} else if done {
		return nil
	}

	hasLegacy, err := c.legacyPrimaryMetadataExists(ctx, primarySpaceID)
	if err != nil {
		return fmt.Errorf("phase4a: detect legacy data: %w", err)
	}
	if !hasLegacy {
		c.logger.Info("phase4a: no legacy SPACE_{primary}_* metadata buckets found, marking migration complete",
			"primary_space_id", primarySpaceID)
		return c.markMigrationComplete(ctx, phase4aCompleteKey)
	}

	c.logger.Info("phase4a: migrating primary space metadata to SERVER_*",
		"primary_space_id", primarySpaceID)

	if err := c.copyPhase4aData(ctx, primarySpaceID); err != nil {
		return fmt.Errorf("phase4a: copy data: %w", err)
	}

	if err := c.verifyPhase4a(ctx, primarySpaceID); err != nil {
		return fmt.Errorf("phase4a: verify: %w", err)
	}

	if err := c.markMigrationComplete(ctx, phase4aCompleteKey); err != nil {
		return fmt.Errorf("phase4a: mark complete: %w", err)
	}

	c.logger.Info("phase4a: migration complete", "primary_space_id", primarySpaceID)
	return nil
}

// isMigrationComplete returns true if the given completion marker key exists
// in `KV_INSTANCE`.
func (c *ChattoCore) isMigrationComplete(ctx context.Context, markerKey string) (bool, error) {
	_, err := c.storage.instanceKV.Get(ctx, markerKey)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return false, nil
	}
	return false, err
}

// markMigrationComplete writes the given completion marker key in `KV_INSTANCE`.
// Safe to call repeatedly — uses Put semantics; collision is impossible since
// only one pod holds the lock when this runs.
func (c *ChattoCore) markMigrationComplete(ctx context.Context, markerKey string) error {
	_, err := c.storage.instanceKV.Put(ctx, markerKey, []byte("1"))
	return err
}

// legacyPrimaryMetadataExists returns true if any of the legacy primary-space
// metadata buckets (SPACE_{primary}_CONFIG/RBAC/RUNTIME) exist. Used to decide
// whether phase 4a has anything to do.
func (c *ChattoCore) legacyPrimaryMetadataExists(ctx context.Context, primarySpaceID string) (bool, error) {
	for _, bucketName := range []string{
		legacySpaceConfigBucket(primarySpaceID),
		legacySpaceRBACBucket(primarySpaceID),
		legacySpaceRuntimeBucket(primarySpaceID),
	} {
		_, err := c.js.KeyValue(ctx, bucketName)
		if err == nil {
			return true, nil
		}
		if !errors.Is(err, jetstream.ErrBucketNotFound) {
			return false, fmt.Errorf("checking bucket %s: %w", bucketName, err)
		}
	}
	return false, nil
}

// copyPhase4aData copies every key from each legacy primary-space metadata
// bucket into the corresponding `SERVER_*` bucket. Idempotent: keys that
// already exist in the target are skipped (so partial runs from a prior
// crash resume cleanly).
func (c *ChattoCore) copyPhase4aData(ctx context.Context, primarySpaceID string) error {
	if err := c.copyKVBucket(ctx, legacySpaceConfigBucket(primarySpaceID), c.storage.serverConfigKV, "CONFIG"); err != nil {
		return err
	}
	if err := c.copyKVBucket(ctx, legacySpaceRBACBucket(primarySpaceID), c.storage.serverRBACKV, "RBAC"); err != nil {
		return err
	}
	if err := c.copyKVBucket(ctx, legacySpaceRuntimeBucket(primarySpaceID), c.storage.serverRuntimeKV, "RUNTIME"); err != nil {
		return err
	}
	return nil
}

// copyKVBucket copies every key from sourceBucketName into target. Uses
// kv.Create on the target so re-running on partial state is a no-op for keys
// that have already been copied. logTag identifies the bucket type in logs.
func (c *ChattoCore) copyKVBucket(ctx context.Context, sourceBucketName string, target jetstream.KeyValue, logTag string) error {
	source, err := c.js.KeyValue(ctx, sourceBucketName)
	if err != nil {
		if errors.Is(err, jetstream.ErrBucketNotFound) {
			c.logger.Debug("phase4a: source bucket missing, skipping",
				"bucket", sourceBucketName, "tag", logTag)
			return nil
		}
		return fmt.Errorf("open source bucket %s: %w", sourceBucketName, err)
	}

	keysLister, err := source.ListKeys(ctx)
	if err != nil {
		return fmt.Errorf("list keys in %s: %w", sourceBucketName, err)
	}
	defer keysLister.Stop()

	copied := 0
	skipped := 0
	for key := range keysLister.Keys() {
		entry, err := source.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				// Key was deleted between listing and reading — fine, nothing
				// to copy. Source data is supposed to be quiescent during the
				// migration, but we tolerate this rather than failing.
				continue
			}
			return fmt.Errorf("read key %q from %s: %w", key, sourceBucketName, err)
		}

		_, err = target.Create(ctx, key, entry.Value())
		switch {
		case err == nil:
			copied++
		case errors.Is(err, jetstream.ErrKeyExists):
			// Already copied in a previous (crashed?) run. Idempotent skip.
			skipped++
		default:
			return fmt.Errorf("write key %q to target: %w", key, err)
		}
	}

	c.logger.Info("phase4a: copied bucket",
		"source", sourceBucketName,
		"tag", logTag,
		"copied", copied,
		"skipped_existing", skipped,
	)
	return nil
}

// verifyPhase4a checks that every key in the legacy buckets has a corresponding
// entry in the SERVER_* target. A mismatch causes the migration to fail without
// writing the completion marker, so the next boot retries.
func (c *ChattoCore) verifyPhase4a(ctx context.Context, primarySpaceID string) error {
	for _, pair := range []struct {
		sourceBucketName string
		target           jetstream.KeyValue
		tag              string
	}{
		{legacySpaceConfigBucket(primarySpaceID), c.storage.serverConfigKV, "CONFIG"},
		{legacySpaceRBACBucket(primarySpaceID), c.storage.serverRBACKV, "RBAC"},
		{legacySpaceRuntimeBucket(primarySpaceID), c.storage.serverRuntimeKV, "RUNTIME"},
	} {
		if err := c.verifyKVBucketCopy(ctx, pair.sourceBucketName, pair.target, pair.tag); err != nil {
			return err
		}
	}
	return nil
}

// verifyKVBucketCopy walks the source bucket and confirms every key exists in
// the target. Counts both sides and includes them in the error on mismatch.
func (c *ChattoCore) verifyKVBucketCopy(ctx context.Context, sourceBucketName string, target jetstream.KeyValue, tag string) error {
	source, err := c.js.KeyValue(ctx, sourceBucketName)
	if err != nil {
		if errors.Is(err, jetstream.ErrBucketNotFound) {
			return nil // Source missing means nothing to verify.
		}
		return fmt.Errorf("open source bucket %s for verify: %w", sourceBucketName, err)
	}

	keysLister, err := source.ListKeys(ctx)
	if err != nil {
		return fmt.Errorf("list keys in %s for verify: %w", sourceBucketName, err)
	}
	defer keysLister.Stop()

	var sourceCount, missingCount int
	for key := range keysLister.Keys() {
		sourceCount++
		_, err := target.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				missingCount++
				c.logger.Error("phase4a: key missing in target after copy",
					"source_bucket", sourceBucketName,
					"tag", tag,
					"key", key,
				)
				continue
			}
			return fmt.Errorf("verify key %q in target: %w", key, err)
		}
	}

	if missingCount > 0 {
		return fmt.Errorf("verification failed: %d of %d keys from %s missing in target",
			missingCount, sourceCount, sourceBucketName)
	}

	c.logger.Info("phase4a: verified bucket",
		"source", sourceBucketName,
		"tag", tag,
		"keys_verified", sourceCount,
	)
	return nil
}

// acquireMigrationLock takes the `KV_INSTANCE` migration lock. The lock key
// carries a TTL so a crashed pod's lock eventually expires and another pod
// can pick up the (idempotent) work. Returns a release function that the
// caller is expected to defer.
func (c *ChattoCore) acquireMigrationLock(ctx context.Context) (release func(), err error) {
	ownerID := newID("ML")

	deadline := time.Now().Add(migrationLockAcquireTimeout)
	for {
		_, createErr := c.storage.instanceKV.Create(ctx, migrationLockKey, []byte(ownerID), jetstream.KeyTTL(migrationLockTTL))
		if createErr == nil {
			break
		}
		if !errors.Is(createErr, jetstream.ErrKeyExists) {
			return nil, fmt.Errorf("create lock key: %w", createErr)
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("timed out waiting for migration lock after %s", migrationLockAcquireTimeout)
		}
		c.logger.Info("phase4a: waiting for migration lock held by another pod",
			"retry_in", migrationLockRetryInterval)
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(migrationLockRetryInterval):
		}
	}

	release = func() {
		// Best-effort delete; if it fails, the TTL will clean up.
		if err := c.storage.instanceKV.Delete(ctx, migrationLockKey); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
			c.logger.Warn("phase4a: failed to release migration lock; will expire via TTL", "error", err)
		}
	}
	return release, nil
}

// legacy bucket name helpers — kept here so the legacy naming convention is
// expressed in exactly one place during the migration.

func legacySpaceConfigBucket(spaceID string) string {
	return fmt.Sprintf("SPACE_%s_CONFIG", spaceID)
}

func legacySpaceRBACBucket(spaceID string) string {
	return fmt.Sprintf("SPACE_%s_RBAC", spaceID)
}

func legacySpaceRuntimeBucket(spaceID string) string {
	return fmt.Sprintf("SPACE_%s_RUNTIME", spaceID)
}

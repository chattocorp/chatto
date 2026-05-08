package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
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

	// phase4bCompleteKey marks phase 4b as done. Same presence-as-truth shape
	// as phase 4a. The cleanup follow-up deletes both markers along with the
	// legacy SPACE_* resources.
	phase4bCompleteKey = "migration.phase4b_complete"

	// phase4cCompleteKey marks phase 4c as done. Phase 4c migrates the
	// per-message KV buckets (BODIES, REACTIONS, THREADS) for primary +
	// DM into the deployment-wide SERVER_* equivalents.
	phase4cCompleteKey = "migration.phase4c_complete"

	// phase4eCompleteKey marks phase 4e as done. Phase 4e migrates the
	// per-space attachment object stores (SPACE_{id}_ASSETS) for primary +
	// DM into the deployment-wide SERVER_ASSETS object store. Keys
	// (attachment IDs) are globally unique and copied verbatim.
	phase4eCompleteKey = "migration.phase4e_complete"
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
	if err := c.runPhase4aIfNeeded(ctx, primarySpaceID); err != nil {
		return err
	}
	if err := c.runPhase4bIfNeeded(ctx); err != nil {
		return err
	}
	if err := c.runPhase4cIfNeeded(ctx, primarySpaceID); err != nil {
		return err
	}
	if err := c.runPhase4eIfNeeded(ctx, primarySpaceID); err != nil {
		return err
	}
	return nil
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

// runPhase4bIfNeeded folds the DM system space's metadata into SERVER_*
// and rewrites room/membership keys to encode the room kind in the key
// prefix (`room.channel.{X}` and `room.dm.{X}`).
//
// Two pieces of work happen here:
//
//  1. Rewrite primary's already-migrated keys in SERVER_CONFIG from the
//     phase-4a-era format `room.{X}` / `room_membership.{u}.{r}` into
//     the kind-prefixed format `room.channel.{X}` /
//     `room_membership.channel.{u}.{r}`.
//
//  2. Copy DM-space data from SPACE_DM_CONFIG / SPACE_DM_RUNTIME into
//     SERVER_CONFIG / SERVER_RUNTIME, writing rooms and memberships
//     under the `room.dm.*` / `room_membership.dm.*` prefixes.
//
// In both cases the original keys are left in place (no-deletes rule);
// reads use the new prefixes, the dormant old keys cost a little disk
// until the cleanup follow-up retires them.
//
// DM space has no RBAC bucket to migrate (DM permissions are hardcoded;
// see isDMPermissionAllowed). Per-message KVs (BODIES/REACTIONS/THREADS)
// move in a later phase.
func (c *ChattoCore) runPhase4bIfNeeded(ctx context.Context) error {
	if done, err := c.isMigrationComplete(ctx, phase4bCompleteKey); err != nil {
		return fmt.Errorf("phase4b: check completion marker: %w", err)
	} else if done {
		return nil
	}

	release, err := c.acquireMigrationLock(ctx)
	if err != nil {
		return fmt.Errorf("phase4b: acquire lock: %w", err)
	}
	defer release()

	// Re-check after acquiring the lock — another pod may have just finished.
	if done, err := c.isMigrationComplete(ctx, phase4bCompleteKey); err != nil {
		return fmt.Errorf("phase4b: re-check completion marker: %w", err)
	} else if done {
		return nil
	}

	c.logger.Info("phase4b: rewriting primary keys in SERVER_CONFIG to kind-prefixed form")
	if err := c.rewritePrimaryConfigKeysToKindPrefix(ctx); err != nil {
		return fmt.Errorf("phase4b: rewrite primary keys: %w", err)
	}

	if hasLegacyDM, err := c.legacyDMMetadataExists(ctx); err != nil {
		return fmt.Errorf("phase4b: detect legacy DM data: %w", err)
	} else if hasLegacyDM {
		c.logger.Info("phase4b: copying DM space data into SERVER_* with dm-prefixed keys")
		if err := c.copyDMDataToServerLayout(ctx); err != nil {
			return fmt.Errorf("phase4b: copy DM data: %w", err)
		}
	}

	if err := c.markMigrationComplete(ctx, phase4bCompleteKey); err != nil {
		return fmt.Errorf("phase4b: mark complete: %w", err)
	}

	c.logger.Info("phase4b: migration complete")
	return nil
}

// legacyDMMetadataExists returns true if either of the DM system space's
// legacy CONFIG/RUNTIME buckets exist. (RBAC is intentionally absent — DM
// has no roles.)
func (c *ChattoCore) legacyDMMetadataExists(ctx context.Context) (bool, error) {
	for _, bucketName := range []string{
		legacySpaceConfigBucket(DMSpaceID),
		legacySpaceRuntimeBucket(DMSpaceID),
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

// rewritePrimaryConfigKeysToKindPrefix walks SERVER_CONFIG and copies any
// key in the phase-4a-era format (`room.{X}`, `room_membership.{u}.{r}`)
// to its kind-prefixed equivalent (`room.channel.{X}`,
// `room_membership.channel.{u}.{r}`). The original key is left in place
// for rollback safety; it becomes dormant once code switches to the new
// prefixes.
//
// Idempotent: re-runs find no old-format keys to copy (the prefix scans
// `room.*` and `room_membership.*.*` no longer match anything once
// rewrites happen, and Create swallows ErrKeyExists for any key already
// present in the target).
func (c *ChattoCore) rewritePrimaryConfigKeysToKindPrefix(ctx context.Context) error {
	target := c.storage.serverConfigKV

	roomsCopied, roomsSkipped, err := c.copyKeysWithRewrite(ctx, target, "room.*", func(oldKey string) (string, bool) {
		// "room.{X}" — exactly one segment after "room.", and no
		// further dots (NanoIDs don't contain dots).
		suffix := strings.TrimPrefix(oldKey, "room.")
		if suffix == oldKey || strings.Contains(suffix, ".") {
			return "", false
		}
		return "room.channel." + suffix, true
	})
	if err != nil {
		return fmt.Errorf("rewrite room.* keys: %w", err)
	}

	memCopied, memSkipped, err := c.copyKeysWithRewrite(ctx, target, "room_membership.*.*", func(oldKey string) (string, bool) {
		// Old format: "room_membership.{u}.{r}" — exactly two segments
		// after the "room_membership." prefix. New format swaps the
		// order to put roomID first: "room_membership.channel.{r}.{u}".
		suffix := strings.TrimPrefix(oldKey, "room_membership.")
		if suffix == oldKey {
			return "", false
		}
		segments := strings.Split(suffix, ".")
		if len(segments) != 2 {
			return "", false
		}
		userID, roomID := segments[0], segments[1]
		return fmt.Sprintf("room_membership.channel.%s.%s", roomID, userID), true
	})
	if err != nil {
		return fmt.Errorf("rewrite room_membership.*.* keys: %w", err)
	}

	c.logger.Info("phase4b: rewrote primary keys in SERVER_CONFIG",
		"rooms_copied", roomsCopied,
		"rooms_skipped_existing", roomsSkipped,
		"memberships_copied", memCopied,
		"memberships_skipped_existing", memSkipped,
	)
	return nil
}

// copyKeysWithRewrite scans target with filterPattern, computes a new key
// for each match via rewrite, and writes the original value at the new
// key. Returns (copied, skipped, error). The original key stays in place.
func (c *ChattoCore) copyKeysWithRewrite(
	ctx context.Context,
	target jetstream.KeyValue,
	filterPattern string,
	rewrite func(oldKey string) (newKey string, ok bool),
) (copied, skipped int, err error) {
	keysLister, err := target.ListKeysFiltered(ctx, filterPattern)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoKeysFound) {
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("list %q: %w", filterPattern, err)
	}
	defer keysLister.Stop()

	for oldKey := range keysLister.Keys() {
		newKey, ok := rewrite(oldKey)
		if !ok {
			continue
		}
		entry, err := target.Get(ctx, oldKey)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return copied, skipped, fmt.Errorf("read %q: %w", oldKey, err)
		}
		_, err = target.Create(ctx, newKey, entry.Value())
		switch {
		case err == nil:
			copied++
		case errors.Is(err, jetstream.ErrKeyExists):
			skipped++
		default:
			return copied, skipped, fmt.Errorf("write %q: %w", newKey, err)
		}
	}
	return copied, skipped, nil
}

// copyDMDataToServerLayout copies DM-space CONFIG and RUNTIME data into
// the shared SERVER_* buckets, rewriting room/membership keys with the
// `dm` kind prefix. Source data left intact (no-deletes rule).
func (c *ChattoCore) copyDMDataToServerLayout(ctx context.Context) error {
	if err := c.copyDMConfigToServer(ctx); err != nil {
		return err
	}
	if err := c.copyKVBucket(ctx, legacySpaceRuntimeBucket(DMSpaceID), c.storage.serverRuntimeKV, "DM_RUNTIME"); err != nil {
		return err
	}
	return nil
}

// copyDMConfigToServer walks SPACE_DM_CONFIG and writes each key into
// SERVER_CONFIG, rewriting `room.{X}` → `room.dm.{X}` and
// `room_membership.{u}.{r}` → `room_membership.dm.{u}.{r}`. Other keys
// (none expected for DM space, but defensively) copy verbatim.
func (c *ChattoCore) copyDMConfigToServer(ctx context.Context) error {
	sourceName := legacySpaceConfigBucket(DMSpaceID)
	source, err := c.js.KeyValue(ctx, sourceName)
	if err != nil {
		if errors.Is(err, jetstream.ErrBucketNotFound) {
			return nil
		}
		return fmt.Errorf("open %s: %w", sourceName, err)
	}

	keysLister, err := source.ListKeys(ctx)
	if err != nil {
		return fmt.Errorf("list keys in %s: %w", sourceName, err)
	}
	defer keysLister.Stop()

	target := c.storage.serverConfigKV

	copied := 0
	skipped := 0
	for key := range keysLister.Keys() {
		entry, err := source.Get(ctx, key)
		if err != nil {
			if errors.Is(err, jetstream.ErrKeyNotFound) {
				continue
			}
			return fmt.Errorf("read key %q from %s: %w", key, sourceName, err)
		}

		newKey := dmConfigKeyToServerKey(key)
		_, err = target.Create(ctx, newKey, entry.Value())
		switch {
		case err == nil:
			copied++
		case errors.Is(err, jetstream.ErrKeyExists):
			skipped++
		default:
			return fmt.Errorf("write key %q to SERVER_CONFIG: %w", newKey, err)
		}
	}

	c.logger.Info("phase4b: copied DM CONFIG bucket to SERVER_CONFIG",
		"source", sourceName,
		"copied", copied,
		"skipped_existing", skipped,
	)
	return nil
}

// dmConfigKeyToServerKey rewrites a DM-space CONFIG key to its
// SERVER_CONFIG equivalent. `room.{X}` becomes `room.dm.{X}`;
// `room_membership.{u}.{r}` becomes `room_membership.dm.{r}.{u}`
// (note the user/room swap to align with the room-first ordering).
// Any other key shape is returned unchanged.
func dmConfigKeyToServerKey(key string) string {
	if rest, ok := strings.CutPrefix(key, "room."); ok && !strings.Contains(rest, ".") {
		return "room.dm." + rest
	}
	if rest, ok := strings.CutPrefix(key, "room_membership."); ok {
		segments := strings.Split(rest, ".")
		if len(segments) == 2 {
			userID, roomID := segments[0], segments[1]
			return fmt.Sprintf("room_membership.dm.%s.%s", roomID, userID)
		}
	}
	return key
}

// runPhase4cIfNeeded folds the per-message KV buckets (BODIES, REACTIONS,
// THREADS) for the primary and DM spaces into the deployment-wide
// SERVER_BODIES / SERVER_REACTIONS / SERVER_THREADS buckets.
//
// Keys are copied verbatim — the existing key formats (`{userID}.{eventID}`
// for bodies, `{eventID}.{emojiName}.{userID}` for reactions, and
// `{roomID}.{rootEventID}` for threads) are already keyed on globally-unique
// IDs, so no rewriting is needed.
//
// Idempotent (Create + ErrKeyExists swallowed). Source data left intact
// per the no-deletes rule. Verify-after-copy aborts before the marker if
// counts don't line up.
func (c *ChattoCore) runPhase4cIfNeeded(ctx context.Context, primarySpaceID string) error {
	if done, err := c.isMigrationComplete(ctx, phase4cCompleteKey); err != nil {
		return fmt.Errorf("phase4c: check completion marker: %w", err)
	} else if done {
		return nil
	}

	release, err := c.acquireMigrationLock(ctx)
	if err != nil {
		return fmt.Errorf("phase4c: acquire lock: %w", err)
	}
	defer release()

	if done, err := c.isMigrationComplete(ctx, phase4cCompleteKey); err != nil {
		return fmt.Errorf("phase4c: re-check completion marker: %w", err)
	} else if done {
		return nil
	}

	c.logger.Info("phase4c: migrating per-message KVs (BODIES/REACTIONS/THREADS) to SERVER_*",
		"primary_space_id", primarySpaceID)

	// Copy primary's per-message KVs (skipped if no primary, or no legacy
	// buckets to copy from — copyKVBucket handles a missing source).
	if primarySpaceID != "" {
		if err := c.copyKVBucket(ctx, legacySpaceBodiesBucket(primarySpaceID), c.storage.serverBodiesKV, "PRIMARY_BODIES"); err != nil {
			return fmt.Errorf("phase4c: copy primary bodies: %w", err)
		}
		if err := c.copyKVBucket(ctx, legacySpaceReactionsBucket(primarySpaceID), c.storage.serverReactionsKV, "PRIMARY_REACTIONS"); err != nil {
			return fmt.Errorf("phase4c: copy primary reactions: %w", err)
		}
		if err := c.copyKVBucket(ctx, legacySpaceThreadsBucket(primarySpaceID), c.storage.serverThreadsKV, "PRIMARY_THREADS"); err != nil {
			return fmt.Errorf("phase4c: copy primary threads: %w", err)
		}
	}

	// Copy DM per-message KVs.
	if err := c.copyKVBucket(ctx, legacySpaceBodiesBucket(DMSpaceID), c.storage.serverBodiesKV, "DM_BODIES"); err != nil {
		return fmt.Errorf("phase4c: copy DM bodies: %w", err)
	}
	if err := c.copyKVBucket(ctx, legacySpaceReactionsBucket(DMSpaceID), c.storage.serverReactionsKV, "DM_REACTIONS"); err != nil {
		return fmt.Errorf("phase4c: copy DM reactions: %w", err)
	}
	if err := c.copyKVBucket(ctx, legacySpaceThreadsBucket(DMSpaceID), c.storage.serverThreadsKV, "DM_THREADS"); err != nil {
		return fmt.Errorf("phase4c: copy DM threads: %w", err)
	}

	if err := c.verifyPhase4c(ctx, primarySpaceID); err != nil {
		return fmt.Errorf("phase4c: verify: %w", err)
	}

	if err := c.markMigrationComplete(ctx, phase4cCompleteKey); err != nil {
		return fmt.Errorf("phase4c: mark complete: %w", err)
	}

	c.logger.Info("phase4c: migration complete")
	return nil
}

// verifyPhase4c walks the source per-message buckets for primary and DM,
// confirming every key landed in the corresponding SERVER_* target.
func (c *ChattoCore) verifyPhase4c(ctx context.Context, primarySpaceID string) error {
	type pair struct {
		sourceBucketName string
		target           jetstream.KeyValue
		tag              string
	}
	pairs := []pair{
		{legacySpaceBodiesBucket(DMSpaceID), c.storage.serverBodiesKV, "DM_BODIES"},
		{legacySpaceReactionsBucket(DMSpaceID), c.storage.serverReactionsKV, "DM_REACTIONS"},
		{legacySpaceThreadsBucket(DMSpaceID), c.storage.serverThreadsKV, "DM_THREADS"},
	}
	if primarySpaceID != "" {
		pairs = append(pairs,
			pair{legacySpaceBodiesBucket(primarySpaceID), c.storage.serverBodiesKV, "PRIMARY_BODIES"},
			pair{legacySpaceReactionsBucket(primarySpaceID), c.storage.serverReactionsKV, "PRIMARY_REACTIONS"},
			pair{legacySpaceThreadsBucket(primarySpaceID), c.storage.serverThreadsKV, "PRIMARY_THREADS"},
		)
	}
	for _, p := range pairs {
		if err := c.verifyKVBucketCopy(ctx, p.sourceBucketName, p.target, p.tag); err != nil {
			return err
		}
	}
	return nil
}

// runPhase4eIfNeeded folds the per-space attachment object stores
// (`SPACE_{primary}_ASSETS`, `SPACE_DM_ASSETS`) into the deployment-wide
// `SERVER_ASSETS` object store.
//
// Attachment IDs are globally unique, so object names (which are the keys)
// are copied verbatim — no rewriting needed. Headers (Content-Type,
// Filename, Room-Id) are preserved on the target.
//
// Object stores have no `Create`-style atomic insert, only `Put`. Re-running
// on partial state therefore re-uploads bytes for objects already on the
// target. That's wasteful but idempotent in effect — same Name, same
// content, same headers — and the verify-after-copy step still catches a
// torn copy. Source data is left intact per the no-deletes rule.
func (c *ChattoCore) runPhase4eIfNeeded(ctx context.Context, primarySpaceID string) error {
	if done, err := c.isMigrationComplete(ctx, phase4eCompleteKey); err != nil {
		return fmt.Errorf("phase4e: check completion marker: %w", err)
	} else if done {
		return nil
	}

	release, err := c.acquireMigrationLock(ctx)
	if err != nil {
		return fmt.Errorf("phase4e: acquire lock: %w", err)
	}
	defer release()

	if done, err := c.isMigrationComplete(ctx, phase4eCompleteKey); err != nil {
		return fmt.Errorf("phase4e: re-check completion marker: %w", err)
	} else if done {
		return nil
	}

	c.logger.Info("phase4e: migrating per-space attachments to SERVER_ASSETS",
		"primary_space_id", primarySpaceID)

	if primarySpaceID != "" {
		if err := c.copyObjectStore(ctx, legacySpaceAssetsBucket(primarySpaceID), c.storage.serverAttachments, "PRIMARY_ASSETS"); err != nil {
			return fmt.Errorf("phase4e: copy primary assets: %w", err)
		}
	}
	if err := c.copyObjectStore(ctx, legacySpaceAssetsBucket(DMSpaceID), c.storage.serverAttachments, "DM_ASSETS"); err != nil {
		return fmt.Errorf("phase4e: copy DM assets: %w", err)
	}

	if err := c.verifyPhase4e(ctx, primarySpaceID); err != nil {
		return fmt.Errorf("phase4e: verify: %w", err)
	}

	if err := c.markMigrationComplete(ctx, phase4eCompleteKey); err != nil {
		return fmt.Errorf("phase4e: mark complete: %w", err)
	}

	c.logger.Info("phase4e: migration complete")
	return nil
}

// copyObjectStore copies every object from sourceBucketName into target.
// Object stores have no atomic-insert primitive, so this re-uploads bytes
// on re-runs; that's safe (same Name, same content) and the eventual
// verify pass would catch a torn copy. logTag identifies the bucket type
// in logs.
func (c *ChattoCore) copyObjectStore(ctx context.Context, sourceBucketName string, target jetstream.ObjectStore, logTag string) error {
	source, err := c.js.ObjectStore(ctx, sourceBucketName)
	if err != nil {
		if errors.Is(err, jetstream.ErrBucketNotFound) {
			c.logger.Debug("phase4e: source bucket missing, skipping",
				"bucket", sourceBucketName, "tag", logTag)
			return nil
		}
		return fmt.Errorf("open source bucket %s: %w", sourceBucketName, err)
	}

	infos, err := source.List(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoObjectsFound) {
			return nil
		}
		return fmt.Errorf("list objects in %s: %w", sourceBucketName, err)
	}

	copied := 0
	for _, info := range infos {
		if info.Deleted {
			continue
		}
		if err := c.copyOneObject(ctx, source, target, info); err != nil {
			return err
		}
		copied++
	}

	c.logger.Info("phase4e: copied object store",
		"source", sourceBucketName,
		"tag", logTag,
		"copied", copied,
	)
	return nil
}

// copyOneObject reads a single object from source and writes it to target,
// preserving the object's metadata. The source reader is closed before
// returning. ErrObjectNotFound on the read side is tolerated — the source is
// supposed to be quiescent, but races between List and Get are accepted.
func (c *ChattoCore) copyOneObject(ctx context.Context, source, target jetstream.ObjectStore, info *jetstream.ObjectInfo) error {
	obj, err := source.Get(ctx, info.Name)
	if err != nil {
		if errors.Is(err, jetstream.ErrObjectNotFound) {
			return nil
		}
		return fmt.Errorf("read object %q: %w", info.Name, err)
	}
	defer obj.Close()
	if _, err := target.Put(ctx, jetstream.ObjectMeta{
		Name:        info.Name,
		Description: info.Description,
		Headers:     info.Headers,
		Metadata:    info.Metadata,
		Opts:        info.Opts,
	}, obj); err != nil {
		return fmt.Errorf("write object %q to target: %w", info.Name, err)
	}
	return nil
}

// verifyPhase4e walks the source object stores for primary and DM,
// confirming every object is present in `SERVER_ASSETS`.
func (c *ChattoCore) verifyPhase4e(ctx context.Context, primarySpaceID string) error {
	sourceBuckets := []struct {
		name string
		tag  string
	}{
		{legacySpaceAssetsBucket(DMSpaceID), "DM_ASSETS"},
	}
	if primarySpaceID != "" {
		sourceBuckets = append(sourceBuckets,
			struct {
				name string
				tag  string
			}{legacySpaceAssetsBucket(primarySpaceID), "PRIMARY_ASSETS"})
	}
	for _, src := range sourceBuckets {
		if err := c.verifyObjectStoreCopy(ctx, src.name, c.storage.serverAttachments, src.tag); err != nil {
			return err
		}
	}
	return nil
}

// verifyObjectStoreCopy walks the source object store and confirms every
// non-deleted object exists in the target.
func (c *ChattoCore) verifyObjectStoreCopy(ctx context.Context, sourceBucketName string, target jetstream.ObjectStore, tag string) error {
	source, err := c.js.ObjectStore(ctx, sourceBucketName)
	if err != nil {
		if errors.Is(err, jetstream.ErrBucketNotFound) {
			return nil
		}
		return fmt.Errorf("open source bucket %s for verify: %w", sourceBucketName, err)
	}

	infos, err := source.List(ctx)
	if err != nil {
		if errors.Is(err, jetstream.ErrNoObjectsFound) {
			return nil
		}
		return fmt.Errorf("list objects in %s for verify: %w", sourceBucketName, err)
	}

	var sourceCount, missingCount int
	for _, info := range infos {
		if info.Deleted {
			continue
		}
		sourceCount++
		if _, err := target.GetInfo(ctx, info.Name); err != nil {
			if errors.Is(err, jetstream.ErrObjectNotFound) {
				missingCount++
				c.logger.Error("phase4e: object missing in target after copy",
					"source_bucket", sourceBucketName,
					"tag", tag,
					"name", info.Name,
				)
				continue
			}
			return fmt.Errorf("verify object %q in target: %w", info.Name, err)
		}
	}

	if missingCount > 0 {
		return fmt.Errorf("verification failed: %d of %d objects from %s missing in target",
			missingCount, sourceCount, sourceBucketName)
	}

	c.logger.Info("phase4e: verified object store",
		"source", sourceBucketName,
		"tag", tag,
		"objects_verified", sourceCount,
	)
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

func legacySpaceBodiesBucket(spaceID string) string {
	return fmt.Sprintf("SPACE_%s_BODIES", spaceID)
}

func legacySpaceReactionsBucket(spaceID string) string {
	return fmt.Sprintf("SPACE_%s_REACTIONS", spaceID)
}

func legacySpaceThreadsBucket(spaceID string) string {
	return fmt.Sprintf("SPACE_%s_THREADS", spaceID)
}

func legacySpaceAssetsBucket(spaceID string) string {
	return fmt.Sprintf("SPACE_%s_ASSETS", spaceID)
}

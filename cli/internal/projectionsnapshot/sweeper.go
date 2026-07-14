package projectionsnapshot

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"
)

// SweepOptions bounds one cleanup pass. ProjectionKeys must contain every
// snapshot-enabled projection in this binary so all live pointer references are
// protected before inventory begins.
type SweepOptions struct {
	ProjectionKeys []string
	GracePeriod    time.Duration
	MaxDeletes     int
	MaxDeleteBytes int64
	BeforeDelete   func(context.Context) error
}

// SweepResult contains privacy-safe inventory and deletion totals. Eligible
// counts are measured during the completed read-only pass and may differ from
// the second pass if another replica publishes concurrently.
type SweepResult struct {
	ScannedObjects    int
	ScannedBytes      int64
	ReferencedObjects int
	ActivePointers    int
	RecentObjects     int
	EligibleObjects   int
	EligibleBytes     int64
	IgnoredObjects    int
	DeletedObjects    int
	DeletedBytes      int64
	DeleteLimitHit    bool
}

// Sweep removes old unreferenced generations and obsolete pointer objects. It
// first authenticates every known pointer and completes a read-only inventory,
// so pointer or initial listing failures cannot cause deletion. A second
// streaming pass performs bounded deletes; the grace period protects uploads
// and pointer replacements racing either pass.
func (r *Repository) Sweep(ctx context.Context, opts SweepOptions) (SweepResult, error) {
	if opts.GracePeriod <= 0 {
		return SweepResult{}, fmt.Errorf("snapshot cleanup grace period must be positive")
	}
	if opts.MaxDeletes <= 0 {
		return SweepResult{}, fmt.Errorf("snapshot cleanup delete limit must be positive")
	}
	if opts.MaxDeleteBytes <= 0 {
		return SweepResult{}, fmt.Errorf("snapshot cleanup byte limit must be positive")
	}
	projectionKeys := slices.Clone(opts.ProjectionKeys)
	slices.Sort(projectionKeys)
	projectionKeys = slices.Compact(projectionKeys)
	if len(projectionKeys) == 0 || projectionKeys[0] == "" {
		return SweepResult{}, fmt.Errorf("snapshot cleanup projection keys are required")
	}

	referenced := make(map[string]struct{}, len(projectionKeys)*2)
	activePointers := make(map[string]struct{}, len(projectionKeys))
	for _, projectionKey := range projectionKeys {
		activePointers[r.pointerKey(projectionKey)] = struct{}{}
		pointer, err := r.loadPointer(ctx, projectionKey)
		switch {
		case err == nil:
		case errors.Is(err, ErrSnapshotNotFound):
			continue
		default:
			return SweepResult{}, fmt.Errorf("read %s snapshot pointer for cleanup: %w", projectionKey, err)
		}
		for _, id := range []string{pointer.GetCurrentGenerationId(), pointer.GetPreviousGenerationId()} {
			if id != "" {
				referenced[id] = struct{}{}
			}
		}
	}

	cutoff := r.now().UTC().Add(-opts.GracePeriod)
	var result SweepResult
	if err := r.blobs.Walk(ctx, objectPrefix, func(info BlobInfo) error {
		result.recordInventory(info, referenced, activePointers, cutoff)
		return nil
	}); err != nil {
		return result, fmt.Errorf("inventory projection snapshot objects: %w", err)
	}

	err := r.blobs.Walk(ctx, objectPrefix, func(info BlobInfo) error {
		valid, protected := cleanupObjectStatus(info.Key, referenced, activePointers)
		if !valid || protected || info.Size < 0 || info.ModifiedAt.IsZero() || info.ModifiedAt.UTC().After(cutoff) {
			return nil
		}
		if result.DeletedObjects >= opts.MaxDeletes || info.Size > opts.MaxDeleteBytes-result.DeletedBytes {
			result.DeleteLimitHit = true
			return nil
		}
		if opts.BeforeDelete != nil {
			if err := opts.BeforeDelete(ctx); err != nil {
				return fmt.Errorf("check cleanup ownership: %w", err)
			}
		}
		if err := r.blobs.Delete(ctx, info.Key); err != nil {
			if errors.Is(err, ErrBlobNotFound) {
				return nil
			}
			return fmt.Errorf("delete unreferenced snapshot object: %w", err)
		}
		result.DeletedObjects++
		result.DeletedBytes += info.Size
		return nil
	})
	if err != nil {
		return result, fmt.Errorf("sweep projection snapshot objects: %w", err)
	}
	return result, nil
}

func (r *SweepResult) recordInventory(info BlobInfo, referenced, activePointers map[string]struct{}, cutoff time.Time) {
	r.ScannedObjects++
	if info.Size >= 0 {
		r.ScannedBytes = saturatedAdd(r.ScannedBytes, info.Size)
	}
	valid, protected := cleanupObjectStatus(info.Key, referenced, activePointers)
	if !valid || info.Size < 0 || info.ModifiedAt.IsZero() {
		r.IgnoredObjects++
		return
	}
	if protected {
		if strings.HasPrefix(info.Key, objectPrefix+"pointers/") {
			r.ActivePointers++
		} else {
			r.ReferencedObjects++
		}
		return
	}
	if info.ModifiedAt.UTC().After(cutoff) {
		r.RecentObjects++
		return
	}
	r.EligibleObjects++
	r.EligibleBytes = saturatedAdd(r.EligibleBytes, info.Size)
}

func saturatedAdd(left, right int64) int64 {
	if right > math.MaxInt64-left {
		return math.MaxInt64
	}
	return left + right
}

func cleanupObjectStatus(key string, referenced, activePointers map[string]struct{}) (valid, protected bool) {
	if id, ok := generationIDFromObjectKey(key); ok {
		_, protected = referenced[id]
		return true, protected
	}
	if pointerLocatorFromObjectKey(key) != "" {
		_, protected = activePointers[key]
		return true, protected
	}
	return false, false
}

func generationIDFromObjectKey(key string) (string, bool) {
	prefix := objectPrefix + "objects/"
	id, ok := strings.CutPrefix(key, prefix)
	if !ok || strings.Contains(id, "/") {
		return "", false
	}
	if _, err := parseGenerationID(id); err != nil {
		return "", false
	}
	return id, true
}

func pointerLocatorFromObjectKey(key string) string {
	prefix := objectPrefix + "pointers/"
	locator, ok := strings.CutPrefix(key, prefix)
	if !ok || len(locator) != 16 || strings.Contains(locator, "/") {
		return ""
	}
	for i := range len(locator) {
		if _, ok := hexNibble(locator[i]); !ok {
			return ""
		}
	}
	return locator
}

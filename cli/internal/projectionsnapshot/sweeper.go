package projectionsnapshot

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// SweepOptions bounds one cleanup pass.
type SweepOptions struct {
	GracePeriod    time.Duration
	MaxDeletes     int
	MaxDeleteBytes int64
	// BeforeDelete verifies that this worker still owns the cleanup lease. It is
	// called once after inventory succeeds and before the bounded delete batch.
	BeforeDelete func(context.Context) error
}

// SweepResult contains privacy-safe inventory and deletion totals. Eligible
// counts are measured during the completed read-only inventory pass.
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

// Sweep removes old unreferenced generations. It first authenticates every v1
// pointer and completes a read-only inventory, so pointer or listing failures
// cannot cause deletion. Pointer objects are never deleted because object-store
// deletion is not revision-aware. The grace period protects concurrent uploads.
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
	referenced := make(map[string]struct{}, len(projectionV1Keys)*2)
	activePointerCount := 0
	for _, projectionKey := range projectionV1Keys {
		pointer, err := r.loadPointer(ctx, projectionKey)
		switch {
		case err == nil:
			activePointerCount++
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
	result.ActivePointers = activePointerCount
	candidates := make([]BlobInfo, 0, min(opts.MaxDeletes, 16))
	var candidateBytes int64
	generationPrefix := r.generationObjectPrefix()
	if err := r.blobs.Walk(ctx, generationPrefix, func(info BlobInfo) error {
		result.recordInventory(info, referenced, generationPrefix, cutoff)
		id, valid := generationIDFromObjectKey(info.Key, generationPrefix)
		_, protected := referenced[id]
		if !valid || protected || info.Size < 0 || info.ModifiedAt.IsZero() || info.ModifiedAt.UTC().After(cutoff) {
			return nil
		}
		if len(candidates) >= opts.MaxDeletes || info.Size > opts.MaxDeleteBytes-candidateBytes {
			result.DeleteLimitHit = true
			return nil
		}
		candidates = append(candidates, info)
		candidateBytes += info.Size
		return nil
	}); err != nil {
		return result, fmt.Errorf("inventory projection snapshot objects: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}
	if len(candidates) > 0 && opts.BeforeDelete != nil {
		if err := opts.BeforeDelete(ctx); err != nil {
			return result, fmt.Errorf("check cleanup ownership: %w", err)
		}
	}
	for _, info := range candidates {
		if err := r.blobs.Delete(ctx, info.Key); err != nil {
			if errors.Is(err, ErrBlobNotFound) {
				continue
			}
			return result, fmt.Errorf("delete unreferenced snapshot object: %w", err)
		}
		result.DeletedObjects++
		result.DeletedBytes += info.Size
	}
	return result, nil
}

func (r *SweepResult) recordInventory(info BlobInfo, referenced map[string]struct{}, generationPrefix string, cutoff time.Time) {
	r.ScannedObjects++
	if info.Size >= 0 {
		r.ScannedBytes = saturatedAdd(r.ScannedBytes, info.Size)
	}
	id, valid := generationIDFromObjectKey(info.Key, generationPrefix)
	if !valid {
		r.IgnoredObjects++
		return
	}
	if info.Size < 0 || info.ModifiedAt.IsZero() {
		r.IgnoredObjects++
		return
	}
	if _, protected := referenced[id]; protected {
		r.ReferencedObjects++
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

func generationIDFromObjectKey(key, prefix string) (string, bool) {
	id, ok := strings.CutPrefix(key, prefix)
	if !ok || strings.Contains(id, "/") {
		return "", false
	}
	if _, err := parseGenerationID(id); err != nil {
		return "", false
	}
	return id, true
}

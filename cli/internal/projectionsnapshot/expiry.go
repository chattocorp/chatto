package projectionsnapshot

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
)

// ExpireOptions bounds one age-based cleanup pass.
type ExpireOptions struct {
	Retention      time.Duration
	MaxDeletes     int
	MaxDeleteBytes int64
}

// ExpireResult contains privacy-safe S3 inventory and deletion totals.
type ExpireResult struct {
	ScannedObjects  int
	ScannedBytes    int64
	RecentObjects   int
	EligibleObjects int
	EligibleBytes   int64
	IgnoredObjects  int
	DeletedObjects  int
	DeletedBytes    int64
	DeleteLimitHit  bool
}

// Expire removes only marked generation objects in the current per-projection
// path layout when their immutable storage timestamp is older than retention.
// It intentionally does not inspect pointers: expiry may cause a cold replay,
// but EVT remains authoritative.
func (r *Repository) Expire(ctx context.Context, opts ExpireOptions) (ExpireResult, error) {
	if opts.Retention <= 0 {
		return ExpireResult{}, fmt.Errorf("snapshot retention must be positive")
	}
	if opts.MaxDeletes <= 0 {
		return ExpireResult{}, fmt.Errorf("snapshot cleanup delete limit must be positive")
	}
	if opts.MaxDeleteBytes <= 0 {
		return ExpireResult{}, fmt.Errorf("snapshot cleanup byte limit must be positive")
	}

	cutoff := r.now().UTC().Add(-opts.Retention)
	var result ExpireResult
	candidates := make([]BlobInfo, 0, min(opts.MaxDeletes, 16))
	var candidateBytes int64
	if err := r.blobs.Walk(ctx, objectRootPrefix, func(info BlobInfo) error {
		result.ScannedObjects++
		if info.Size >= 0 {
			result.ScannedBytes = saturatedAdd(result.ScannedBytes, info.Size)
		}
		if !isGenerationObjectKey(info.Key) || info.Size < 0 || info.ModifiedAt.IsZero() {
			result.IgnoredObjects++
			return nil
		}
		if info.ModifiedAt.UTC().After(cutoff) {
			result.RecentObjects++
			return nil
		}

		verified, err := r.blobs.Stat(ctx, info.Key)
		if err != nil {
			return fmt.Errorf("inspect expired snapshot candidate: %w", err)
		}
		if verified.Key != info.Key || verified.ContentType != contentType || verified.Purpose != objectPurpose ||
			verified.Size < 0 || verified.ModifiedAt.IsZero() || verified.ModifiedAt.UTC().After(cutoff) {
			result.IgnoredObjects++
			return nil
		}
		result.EligibleObjects++
		result.EligibleBytes = saturatedAdd(result.EligibleBytes, verified.Size)
		if len(candidates) >= opts.MaxDeletes || verified.Size > opts.MaxDeleteBytes-candidateBytes {
			result.DeleteLimitHit = true
			return nil
		}
		candidates = append(candidates, verified)
		candidateBytes += verified.Size
		return nil
	}); err != nil {
		return result, fmt.Errorf("inventory projection snapshot objects: %w", err)
	}

	if err := ctx.Err(); err != nil {
		return result, err
	}
	for _, info := range candidates {
		if err := r.blobs.Delete(ctx, info.Key); err != nil {
			if errors.Is(err, ErrBlobNotFound) {
				continue
			}
			return result, fmt.Errorf("delete expired snapshot object: %w", err)
		}
		result.DeletedObjects++
		result.DeletedBytes += info.Size
	}
	return result, nil
}

func isGenerationObjectKey(key string) bool {
	relative, ok := strings.CutPrefix(key, objectRootPrefix)
	if !ok {
		return false
	}
	parts := strings.Split(relative, "/")
	if len(parts) != 5 || !validProjectionKey(parts[0]) || !validCompatibilityID(parts[1]) || parts[2] != "objects" {
		return false
	}
	if len(parts[3]) != 16 || !isLowerHex(parts[3]) {
		return false
	}
	_, err := parseGenerationID(parts[4])
	return err == nil
}

func isLowerHex(value string) bool {
	for _, ch := range value {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

func saturatedAdd(left, right int64) int64 {
	if right > math.MaxInt64-left {
		return math.MaxInt64
	}
	return left + right
}

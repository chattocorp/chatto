package linkpreview

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ErrCachedFailure is returned by Cache.Get when the URL was previously fetched
// and failed. This distinguishes a negative cache hit from a cache miss (which returns nil, nil).
var ErrCachedFailure = fmt.Errorf("cached failure")

const (
	// RuntimeStateKeyPrefix is the RUNTIME_STATE key prefix for cached link
	// preview metadata.
	RuntimeStateKeyPrefix = "link_preview."

	// SuccessTTL is how long successful previews are cached.
	SuccessTTL = 24 * time.Hour

	// FailureTTL is how long failed previews are cached.
	FailureTTL = 1 * time.Hour
)

// Cache provides caching for link preview results.
type Cache struct {
	kv jetstream.KeyValue
}

// NewCache creates a cache wrapper over RUNTIME_STATE.
func NewCache(kv jetstream.KeyValue) *Cache {
	return &Cache{kv: kv}
}

// cacheKey generates a cache key from a URL.
func cacheKey(rawURL string) string {
	normalized := NormalizeURLString(rawURL)
	hash := sha256.Sum256([]byte(normalized))
	return RuntimeStateKeyPrefix + hex.EncodeToString(hash[:])
}

// Get retrieves a cached link preview.
// Returns nil, nil if not found or stale.
func (c *Cache) Get(ctx context.Context, url string) (*corev1.LinkPreview, error) {
	key := cacheKey(url)

	entry, err := c.kv.Get(ctx, key)
	if err != nil {
		if errors.Is(err, jetstream.ErrKeyNotFound) {
			return nil, nil // Cache miss
		}
		return nil, err
	}

	var cached corev1.CachedLinkPreview
	if err := proto.Unmarshal(entry.Value(), &cached); err != nil {
		return nil, err
	}

	// Check staleness
	fetchedAt := time.Unix(cached.FetchedAtUnix, 0)
	maxAge := SuccessTTL
	if cached.FetchFailed {
		maxAge = FailureTTL
	}

	if time.Since(fetchedAt) > maxAge {
		return nil, nil // Stale entry
	}

	// Signal negative cache hit so callers can distinguish from a cache miss
	if cached.FetchFailed {
		return nil, ErrCachedFailure
	}

	return cached.Preview, nil
}

// Set stores a link preview in the cache.
func (c *Cache) Set(ctx context.Context, url string, preview *corev1.LinkPreview) error {
	cached := &corev1.CachedLinkPreview{
		Url:           url,
		Preview:       preview,
		FetchFailed:   false,
		FetchedAtUnix: time.Now().Unix(),
	}

	data, err := proto.Marshal(cached)
	if err != nil {
		return err
	}

	return c.setWithTTL(ctx, cacheKey(url), data, SuccessTTL)
}

// SetFailure stores a failed fetch in the cache (negative caching).
func (c *Cache) SetFailure(ctx context.Context, url string, reason string) error {
	cached := &corev1.CachedLinkPreview{
		Url:           url,
		Preview:       nil,
		FetchFailed:   true,
		ErrorReason:   reason,
		FetchedAtUnix: time.Now().Unix(),
	}

	data, err := proto.Marshal(cached)
	if err != nil {
		return err
	}

	return c.setWithTTL(ctx, cacheKey(url), data, FailureTTL)
}

func (c *Cache) setWithTTL(ctx context.Context, key string, data []byte, ttl time.Duration) error {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		if _, err := c.kv.Create(ctx, key, data, jetstream.KeyTTL(ttl)); err == nil {
			return nil
		} else if !errors.Is(err, jetstream.ErrKeyExists) {
			return err
		} else {
			lastErr = err
		}

		if err := c.kv.Purge(ctx, key); err != nil && !errors.Is(err, jetstream.ErrKeyNotFound) {
			return err
		}
	}
	return fmt.Errorf("replace cached link preview: %w", lastErr)
}

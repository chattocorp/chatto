package linkpreview

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

func TestCacheStoresSuccessfulPreviewInRuntimeStateWithTTL(t *testing.T) {
	ctx, js, kv := setupRuntimeStateKV(t)
	cache := NewCache(kv)
	url := "https://example.com/article"

	err := cache.Set(ctx, url, &corev1.LinkPreview{
		Url:   url,
		Title: "Example",
	})
	require.NoError(t, err)

	got, err := cache.Get(ctx, url)
	require.NoError(t, err)
	require.Equal(t, "Example", got.GetTitle())
	assertRuntimeStateCacheTTL(t, ctx, js, kv, cacheKey(url))
}

func TestCacheStoresFailedPreviewInRuntimeStateWithTTL(t *testing.T) {
	ctx, js, kv := setupRuntimeStateKV(t)
	cache := NewCache(kv)
	url := "https://example.com/failure"

	err := cache.SetFailure(ctx, url, "no preview")
	require.NoError(t, err)

	got, err := cache.Get(ctx, url)
	require.Nil(t, got)
	require.True(t, errors.Is(err, ErrCachedFailure), "got %v", err)
	assertRuntimeStateCacheTTL(t, ctx, js, kv, cacheKey(url))
}

func TestCacheReplacesExistingPreviewInRuntimeStateWithTTL(t *testing.T) {
	ctx, js, kv := setupRuntimeStateKV(t)
	cache := NewCache(kv)
	url := "https://example.com/change"

	err := cache.Set(ctx, url, &corev1.LinkPreview{Url: url, Title: "old"})
	require.NoError(t, err)
	err = cache.Set(ctx, url, &corev1.LinkPreview{Url: url, Title: "new"})
	require.NoError(t, err)

	got, err := cache.Get(ctx, url)
	require.NoError(t, err)
	require.Equal(t, "new", got.GetTitle())
	assertRuntimeStateCacheTTL(t, ctx, js, kv, cacheKey(url))
}

func TestCacheKeepsCurrentBlueskySnapshots(t *testing.T) {
	ctx, _, kv := setupRuntimeStateKV(t)
	cache := NewCache(kv)
	url := "https://bsky.app/profile/example.test/post/example"

	require.NoError(t, cache.Set(ctx, url, &corev1.LinkPreview{
		Url: url,
		SocialPost: &corev1.SocialPostPreview{
			Provider: "bluesky",
			Url:      url,
			Author:   &corev1.SocialPostAuthor{Handle: "example.test"},
		},
	}))

	got, err := cache.Get(ctx, url)
	require.NoError(t, err)
	require.NotNil(t, got)
}

func TestCacheRefreshesLegacyGenericMastodonPreview(t *testing.T) {
	ctx, _, kv := setupRuntimeStateKV(t)
	cache := NewCache(kv)
	url := "https://mastodon.social/@alice@remote.example/123"

	putLegacyCachedPreview(t, ctx, kv, url, &corev1.CachedLinkPreview{
		Url:           url,
		Preview:       &corev1.LinkPreview{Url: url, Title: "Mastodon"},
		FetchedAtUnix: time.Now().Unix(),
	})

	got, err := cache.Get(ctx, url)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestCacheRefreshesLegacyNegativePreview(t *testing.T) {
	ctx, _, kv := setupRuntimeStateKV(t)
	cache := NewCache(kv)
	url := "https://docs.example.com/"

	putLegacyCachedPreview(t, ctx, kv, url, &corev1.CachedLinkPreview{
		Url:           url,
		FetchFailed:   true,
		ErrorReason:   "no preview",
		FetchedAtUnix: time.Now().Unix(),
	})

	got, err := cache.Get(ctx, url)
	require.NoError(t, err)
	require.Nil(t, got)
}

func TestCacheKeepsCurrentGenericMastodonFallback(t *testing.T) {
	ctx, _, kv := setupRuntimeStateKV(t)
	cache := NewCache(kv)
	url := "https://social.example/@alice/123"

	require.NoError(t, cache.Set(ctx, url, &corev1.LinkPreview{Url: url, Title: "Fallback"}))

	got, err := cache.Get(ctx, url)
	require.NoError(t, err)
	require.Equal(t, "Fallback", got.GetTitle())
}

func TestCacheKeyUsesRuntimeStatePrefix(t *testing.T) {
	key := cacheKey("https://example.com/article")
	require.True(t, strings.HasPrefix(key, RuntimeStateKeyPrefix), "key %q should use runtime-state prefix", key)
	require.NotContains(t, key, "example.com")
}

func putLegacyCachedPreview(t *testing.T, ctx context.Context, kv jetstream.KeyValue, url string, cached *corev1.CachedLinkPreview) {
	t.Helper()
	data, err := proto.Marshal(cached)
	require.NoError(t, err)
	_, err = kv.Put(ctx, cacheKey(url), data)
	require.NoError(t, err)
}

func setupRuntimeStateKV(t *testing.T) (context.Context, jetstream.JetStream, jetstream.KeyValue) {
	t.Helper()

	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	require.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	kv, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:         "RUNTIME_STATE",
		Storage:        jetstream.FileStorage,
		History:        1,
		LimitMarkerTTL: 24 * time.Hour,
	})
	require.NoError(t, err)

	return ctx, js, kv
}

func assertRuntimeStateCacheTTL(t *testing.T, ctx context.Context, js jetstream.JetStream, kv jetstream.KeyValue, key string) {
	t.Helper()

	entry, err := kv.Get(ctx, key)
	require.NoError(t, err)

	stream, err := js.Stream(ctx, "KV_RUNTIME_STATE")
	require.NoError(t, err)
	msg, err := stream.GetMsg(ctx, entry.Revision())
	require.NoError(t, err)
	require.NotEmpty(t, msg.Header.Get("Nats-TTL"), "expected %s to carry per-key TTL", key)
}

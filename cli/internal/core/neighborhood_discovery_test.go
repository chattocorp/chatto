package core

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core/neighborhood"
	cachestatev1 "hmans.de/chatto/internal/pb/chatto/core/cache_state/v1"
)

const neighborhoodTestSelf = "https://self.example"

type fakeNeighborhoodFetcher struct {
	mu          sync.Mutex
	directories map[string][]string
	profiles    map[string]neighborhood.Profile
	image       []byte
	listCalls   int
	imageCalls  int
}

func (f *fakeNeighborhoodFetcher) ListNeighbors(_ context.Context, origin string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls++
	return f.directories[origin], nil
}

func (f *fakeNeighborhoodFetcher) GetProfile(_ context.Context, origin string) (neighborhood.Profile, error) {
	profile, exists := f.profiles[origin]
	if !exists {
		return neighborhood.Profile{}, errors.New("unreachable")
	}
	return profile, nil
}

func (f *fakeNeighborhoodFetcher) FetchImage(context.Context, string, string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.imageCalls++
	return f.image, nil
}

func (f *fakeNeighborhoodFetcher) counts() (int, int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.listCalls, f.imageCalls
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 4, 4))
	img.Set(1, 1, color.NRGBA{R: 255, A: 255})
	var buf bytes.Buffer
	require.NoError(t, png.Encode(&buf, img))
	return buf.Bytes()
}

func newTestNeighborhoodDiscovery(t *testing.T) (*ChattoCore, *neighborhoodDiscovery, *fakeNeighborhoodFetcher, *time.Time) {
	t.Helper()
	core, _ := newTestCore(t)
	fetcher := &fakeNeighborhoodFetcher{
		directories: map[string][]string{
			"https://a.example": {neighborhoodTestSelf, "https://b.example"},
			"https://b.example": {"https://a.example"},
		},
		profiles: map[string]neighborhood.Profile{
			"https://a.example": {Name: "A", Version: "0.5.0", Description: "First", LogoURL: "https://a.example/logo.png", BannerURL: "https://a.example/banner.png"},
			"https://b.example": {Name: "B", Version: "0.5.0"},
		},
		image: testPNG(t),
	}
	// Object store modification times use the wall clock.
	now := time.Now().UTC()
	neighbors := []string{"https://a.example"}
	discovery := &neighborhoodDiscovery{
		kv:          core.storage.memoryCacheKV,
		images:      core.storage.neighborhoodImages,
		fetcher:     fetcher,
		selfOrigins: []string{neighborhoodTestSelf},
		neighbors:   func() []string { return neighbors },
		assetsCfg:   core.AssetsConfig(),
		limits:      neighborhood.DefaultLimits,
		logger:      core.logger,
		now:         func() time.Time { return now },
	}
	return core, discovery, fetcher, &now
}

func TestNeighborhoodDiscoveryStoresDirectoryAndImages(t *testing.T) {
	core, discovery, fetcher, now := newTestNeighborhoodDiscovery(t)
	ctx := testContext(t)

	directory, err := core.NeighborhoodDirectory(ctx)
	require.NoError(t, err)
	require.Nil(t, directory)

	require.NoError(t, discovery.refreshIfDue(ctx))
	directory, err = core.NeighborhoodDirectory(ctx)
	require.NoError(t, err)
	require.Equal(t, timestamppb.New(*now).AsTime(), directory.GetRefreshedAt().AsTime())
	require.Len(t, directory.GetServers(), 2)
	require.False(t, directory.GetIncomplete())

	first := directory.GetServers()[0]
	require.Equal(t, "https://a.example", first.GetOrigin())
	require.Equal(t, "A", first.GetName())
	require.Equal(t, "First", first.GetDescription())
	require.True(t, first.GetDirectNeighbor())
	require.Equal(t, []string{"https://b.example"}, first.GetRecommendedByOrigins())
	require.Equal(t, "https://a.example/logo.png", first.GetLogo().GetSourceUrl())
	require.NotNil(t, first.GetBanner())
	second := directory.GetServers()[1]
	require.Equal(t, "https://b.example", second.GetOrigin())
	require.False(t, second.GetDirectNeighbor())
	require.Nil(t, second.GetLogo())

	reader, info, err := core.OpenNeighborhoodImage(ctx, first.GetLogo().GetObjectName())
	require.NoError(t, err)
	data, err := io.ReadAll(reader)
	require.NoError(t, reader.Close())
	require.NoError(t, err)
	require.Equal(t, int(info.Size), len(data))
	require.Equal(t, "RIFF", string(data[:4]))

	_, _, err = core.OpenNeighborhoodImage(ctx, "../"+first.GetLogo().GetObjectName())
	require.ErrorIs(t, err, ErrNeighborhoodImageNotFound)
	_, _, err = core.OpenNeighborhoodImage(ctx, first.GetBanner().GetObjectName()[:63]+"0")
	require.ErrorIs(t, err, ErrNeighborhoodImageNotFound)

	// A fresh directory is not refreshed again.
	lists, images := fetcher.counts()
	require.NoError(t, discovery.refreshIfDue(ctx))
	lists2, images2 := fetcher.counts()
	require.Equal(t, lists, lists2)
	require.Equal(t, images, images2)

	// An hourly refresh reuses images while their source URLs are unchanged.
	*now = now.Add(neighborhoodRefreshAge)
	require.NoError(t, discovery.refreshIfDue(ctx))
	lists3, images3 := fetcher.counts()
	require.Greater(t, lists3, lists2)
	require.Equal(t, images2, images3)
	refreshed, err := core.NeighborhoodDirectory(ctx)
	require.NoError(t, err)
	require.Equal(t, first.GetLogo().GetObjectName(), refreshed.GetServers()[0].GetLogo().GetObjectName())
}

func TestNeighborhoodDiscoveryPicksUpNeighborChanges(t *testing.T) {
	core, discovery, fetcher, start := newTestNeighborhoodDiscovery(t)
	ctx := testContext(t)
	// The worker reads the clock on its own goroutine.
	var now atomic.Int64
	now.Store(start.UnixNano())
	discovery.now = func() time.Time { return time.Unix(0, now.Load()) }
	var mu sync.Mutex
	neighbors := []string{"https://a.example"}
	discovery.neighbors = func() []string {
		mu.Lock()
		defer mu.Unlock()
		return slices.Clone(neighbors)
	}
	fetcher.profiles["https://c.example"] = neighborhood.Profile{Name: "C", Version: "0.5.0"}
	discovery.checkInterval = 10 * time.Millisecond
	bootDone := make(chan struct{})
	close(bootDone)
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- discovery.Run(runCtx, bootDone) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	require.Eventually(t, func() bool {
		directory, err := core.NeighborhoodDirectory(ctx)
		return err == nil && directory != nil
	}, 5*time.Second, 10*time.Millisecond)

	mu.Lock()
	neighbors = append(neighbors, "https://c.example")
	mu.Unlock()
	now.Add(int64(neighborhoodSourceChangeDelay))

	require.Eventually(t, func() bool {
		directory, err := core.NeighborhoodDirectory(ctx)
		if err != nil || directory == nil {
			return false
		}
		return slices.ContainsFunc(directory.GetServers(), func(server *cachestatev1.NeighborhoodServerRecord) bool {
			return server.GetOrigin() == "https://c.example"
		})
	}, 5*time.Second, 10*time.Millisecond)
}

// failingPutKV rejects every write so that a discovery pass fails after its
// remote crawl.
type failingPutKV struct {
	jetstream.KeyValue
}

func (failingPutKV) Put(context.Context, string, []byte) (uint64, error) {
	return 0, errors.New("write rejected")
}

func TestNeighborhoodDiscoveryWaitsAfterAFailedPass(t *testing.T) {
	_, discovery, fetcher, _ := newTestNeighborhoodDiscovery(t)
	ctx := testContext(t)
	discovery.kv = failingPutKV{KeyValue: discovery.kv}
	discovery.checkInterval = 5 * time.Millisecond
	discovery.failureBackoff = time.Hour
	bootDone := make(chan struct{})
	close(bootDone)
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- discovery.Run(runCtx, bootDone) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	require.Eventually(t, func() bool {
		lists, _ := fetcher.counts()
		return lists > 0
	}, 5*time.Second, 5*time.Millisecond)
	lists, _ := fetcher.counts()

	// Many check intervals pass, but the failed pass does not run again.
	time.Sleep(100 * time.Millisecond)
	listsLater, _ := fetcher.counts()
	require.Equal(t, lists, listsLater)
}

func TestNeighborhoodDiscoveryDue(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	discovery := &neighborhoodDiscovery{now: func() time.Time { return now }}
	directory := func(age time.Duration, fingerprint string) *cachestatev1.NeighborhoodDirectory {
		return &cachestatev1.NeighborhoodDirectory{RefreshedAt: timestamppb.New(now.Add(-age)), SourceFingerprint: fingerprint}
	}
	incomplete := func(value *cachestatev1.NeighborhoodDirectory) *cachestatev1.NeighborhoodDirectory {
		value.Incomplete = true
		return value
	}
	tests := []struct {
		name    string
		current *cachestatev1.NeighborhoodDirectory
		want    bool
	}{
		{name: "missing", current: nil, want: true},
		{name: "fresh", current: directory(time.Minute, "same"), want: false},
		{name: "old", current: directory(neighborhoodRefreshAge, "same"), want: true},
		{name: "future", current: directory(-time.Minute, "same"), want: true},
		{name: "recent Neighbor change", current: directory(5*time.Second, "other"), want: false},
		{name: "settled Neighbor change", current: directory(neighborhoodSourceChangeDelay, "other"), want: true},
		{name: "recent incomplete pass", current: incomplete(directory(time.Minute, "same")), want: false},
		{name: "old incomplete pass", current: incomplete(directory(neighborhoodIncompleteRetryAge, "same")), want: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.want, discovery.due(tt.current, "same"))
		})
	}
}

func TestNeighborhoodSourceFingerprintIgnoresOrder(t *testing.T) {
	require.Equal(t,
		neighborhoodSourceFingerprint([]string{"https://a.example", "https://b.example"}),
		neighborhoodSourceFingerprint([]string{"https://b.example", "https://a.example"}))
	require.NotEqual(t,
		neighborhoodSourceFingerprint([]string{"https://a.example"}),
		neighborhoodSourceFingerprint([]string{"https://a.example", "https://b.example"}))
}

func TestNeighborhoodHTTPClientRejectsRedirects(t *testing.T) {
	client := newNeighborhoodHTTPClient()
	require.ErrorIs(t, client.CheckRedirect(&http.Request{}, nil), http.ErrUseLastResponse)
}

func TestMarshalNeighborhoodDirectoryDropsServersBeyondLimit(t *testing.T) {
	directory := &cachestatev1.NeighborhoodDirectory{}
	for index := range 1000 {
		directory.Servers = append(directory.Servers, &cachestatev1.NeighborhoodServerRecord{
			Origin:               fmt.Sprintf("https://server%d.example", index),
			Description:          string(bytes.Repeat([]byte("x"), neighborhood.MaxDescriptionLength)),
			RecommendedByOrigins: []string{"https://server999.example"},
		})
	}
	data, err := marshalNeighborhoodDirectory(directory)
	require.NoError(t, err)
	require.LessOrEqual(t, len(data), maxNeighborhoodDirectoryBytes)
	require.Less(t, len(directory.Servers), 1000)
	require.NotEmpty(t, directory.Servers)
	for _, server := range directory.Servers {
		require.Empty(t, server.GetRecommendedByOrigins())
	}
}

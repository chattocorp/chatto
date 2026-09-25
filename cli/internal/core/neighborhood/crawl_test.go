package neighborhood

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const self = "https://self.example"

type fakeFetcher struct {
	mu          sync.Mutex
	directories map[string][]string
	unsupported map[string]bool
	profiles    map[string]Profile
	listCalls   []string
	getCalls    []string
}

func (f *fakeFetcher) ListNeighbors(_ context.Context, origin string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.listCalls = append(f.listCalls, origin)
	if f.unsupported[origin] {
		return nil, ErrUnsupported
	}
	directory, exists := f.directories[origin]
	if !exists {
		return nil, errors.New("unreachable")
	}
	return directory, nil
}

func (f *fakeFetcher) GetProfile(_ context.Context, origin string) (Profile, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getCalls = append(f.getCalls, origin)
	profile, exists := f.profiles[origin]
	if !exists {
		return Profile{}, errors.New("unreachable")
	}
	return profile, nil
}

func profiles(origins ...string) map[string]Profile {
	result := make(map[string]Profile, len(origins))
	for _, origin := range origins {
		result[origin] = Profile{Name: "Server " + origin, Version: "0.5.0"}
	}
	return result
}

type crawledServer struct {
	direct        bool
	recommendedBy []string
}

func crawled(result Result) map[string]crawledServer {
	servers := make(map[string]crawledServer, len(result.Servers))
	for _, server := range result.Servers {
		servers[server.Origin] = crawledServer{direct: server.DirectNeighbor, recommendedBy: server.RecommendedBy}
	}
	return servers
}

func testLimits() Limits {
	limits := DefaultLimits
	limits.RequestTimeout = time.Second
	return limits
}

func TestCrawl(t *testing.T) {
	tests := []struct {
		name      string
		neighbors []string
		fetcher   *fakeFetcher
		limits    func(*Limits)
		want      map[string]crawledServer
		wantLists []string
	}{
		{
			name:      "direct recommendation does not need reciprocity",
			neighbors: []string{"https://a.example"},
			fetcher: &fakeFetcher{
				directories: map[string][]string{"https://a.example": {"https://b.example"}},
				profiles:    profiles("https://a.example", "https://b.example"),
			},
			want:      map[string]crawledServer{"https://a.example": {direct: true}},
			wantLists: []string{"https://a.example"},
		},
		{
			name:      "mutual Neighbor expands to mutual recommendations",
			neighbors: []string{"https://a.example"},
			fetcher: &fakeFetcher{
				directories: map[string][]string{
					"https://a.example": {self + "/", "https://B.example", "https://c.example"},
					"https://b.example": {"https://a.example"},
					"https://c.example": {"https://other.example"},
				},
				profiles: profiles("https://a.example", "https://b.example", "https://c.example"),
			},
			want: map[string]crawledServer{
				"https://a.example": {direct: true, recommendedBy: []string{"https://b.example"}},
				"https://b.example": {recommendedBy: []string{"https://a.example"}},
			},
			wantLists: []string{"https://a.example", "https://b.example", "https://c.example"},
		},
		{
			name:      "discovery stops after two mutual hops",
			neighbors: []string{"https://a.example"},
			fetcher: &fakeFetcher{
				directories: map[string][]string{
					"https://a.example": {self, "https://b.example"},
					"https://b.example": {"https://a.example", "https://c.example"},
					"https://c.example": {"https://b.example"},
				},
				profiles: profiles("https://a.example", "https://b.example", "https://c.example"),
			},
			want: map[string]crawledServer{
				"https://a.example": {direct: true, recommendedBy: []string{"https://b.example"}},
				"https://b.example": {recommendedBy: []string{"https://a.example"}},
			},
			wantLists: []string{"https://a.example", "https://b.example"},
		},
		{
			name:      "one-sided loads do not extend discovery past two mutual hops",
			neighbors: []string{"https://x.example", "https://w.example"},
			fetcher: &fakeFetcher{
				directories: map[string][]string{
					"https://x.example": {self, "https://y.example"},
					"https://w.example": {self, "https://z.example"},
					"https://y.example": {"https://x.example", "https://z.example"},
					"https://z.example": {"https://y.example"},
				},
				profiles: profiles("https://x.example", "https://w.example", "https://y.example", "https://z.example"),
			},
			want: map[string]crawledServer{
				"https://x.example": {direct: true, recommendedBy: []string{"https://y.example"}},
				"https://w.example": {direct: true},
				"https://y.example": {recommendedBy: []string{"https://x.example"}},
			},
			wantLists: []string{"https://x.example", "https://w.example", "https://y.example", "https://z.example"},
		},
		{
			name:      "loaded directories add mutual attribution",
			neighbors: []string{"https://a.example", "https://d.example"},
			fetcher: &fakeFetcher{
				directories: map[string][]string{
					"https://a.example": {self, "https://b.example", "https://d.example"},
					"https://b.example": {"https://a.example", "https://d.example"},
					"https://d.example": {"https://a.example", "https://b.example"},
				},
				profiles: profiles("https://a.example", "https://b.example", "https://d.example"),
			},
			want: map[string]crawledServer{
				"https://a.example": {direct: true, recommendedBy: []string{"https://d.example", "https://b.example"}},
				"https://d.example": {direct: true, recommendedBy: []string{"https://a.example", "https://b.example"}},
				"https://b.example": {recommendedBy: []string{"https://a.example", "https://d.example"}},
			},
			wantLists: []string{"https://a.example", "https://b.example", "https://d.example"},
		},
		{
			name:      "failed profile and self entries are omitted",
			neighbors: []string{"https://a.example", "https://offline.example", self},
			fetcher: &fakeFetcher{
				directories: map[string][]string{"https://a.example": {}},
				unsupported: map[string]bool{"https://offline.example": true},
				profiles:    profiles("https://a.example"),
			},
			want:      map[string]crawledServer{"https://a.example": {direct: true}},
			wantLists: []string{"https://a.example", "https://offline.example"},
		},
		{
			name:      "invalid and oversized advertised origins are ignored",
			neighbors: []string{"https://a.example"},
			fetcher: &fakeFetcher{
				directories: map[string][]string{
					"https://a.example": {self, "ftp://b.example", "https://b.example/path", "https://" + strings.Repeat("abcdefghij.", 30) + "example"},
				},
				profiles: profiles("https://a.example"),
			},
			want:      map[string]crawledServer{"https://a.example": {direct: true}},
			wantLists: []string{"https://a.example"},
		},
		{
			name:      "recommenders without a loaded profile are not attributed",
			neighbors: []string{"https://a.example"},
			fetcher: &fakeFetcher{
				directories: map[string][]string{
					"https://a.example": {self, "https://b.example"},
					"https://b.example": {"https://a.example"},
				},
				profiles: profiles("https://a.example"),
			},
			want:      map[string]crawledServer{"https://a.example": {direct: true, recommendedBy: []string{}}},
			wantLists: []string{"https://a.example", "https://b.example"},
		},
		{
			name:      "failed directories are not requested again",
			neighbors: []string{"https://a.example", "https://offline.example"},
			fetcher: &fakeFetcher{
				directories: map[string][]string{
					"https://a.example": {self, "https://offline.example"},
				},
				profiles: profiles("https://a.example"),
			},
			want:      map[string]crawledServer{"https://a.example": {direct: true}},
			wantLists: []string{"https://a.example", "https://offline.example"},
		},
		{
			name:      "request budgets bound the pass",
			neighbors: []string{"https://a.example", "https://b.example", "https://c.example"},
			fetcher: &fakeFetcher{
				directories: map[string][]string{},
				profiles:    profiles("https://a.example", "https://b.example", "https://c.example"),
			},
			limits: func(limits *Limits) {
				limits.DirectoryRequests = 2
				limits.ProfileRequests = 1
			},
			want:      map[string]crawledServer{"https://a.example": {direct: true}},
			wantLists: []string{"https://a.example", "https://b.example"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			limits := testLimits()
			if tt.limits != nil {
				tt.limits(&limits)
			}
			result, err := Crawl(context.Background(), []string{self}, tt.neighbors, tt.fetcher, limits)
			require.NoError(t, err)
			require.Equal(t, tt.want, crawled(result))
			require.ElementsMatch(t, tt.wantLists, tt.fetcher.listCalls)
			require.Len(t, tt.fetcher.getCalls, result.ProfileRequests)
		})
	}
}

func TestCrawlTruncatesProfileText(t *testing.T) {
	fetcher := &fakeFetcher{
		directories: map[string][]string{},
		profiles: map[string]Profile{"https://a.example": {
			Name:        string(make([]byte, MaxNameLength-1)) + "é",
			Description: "valid\xff",
			LogoURL:     "https://a.example/" + strings.Repeat("x", MaxImageURLLength),
			BannerURL:   "https://a.example/banner.png",
		}},
	}
	result, err := Crawl(context.Background(), []string{self}, []string{"https://a.example"}, fetcher, testLimits())
	require.NoError(t, err)
	require.Len(t, result.Servers, 1)
	require.Len(t, result.Servers[0].Profile.Name, MaxNameLength-1)
	require.Equal(t, "valid", result.Servers[0].Profile.Description)
	require.Empty(t, result.Servers[0].Profile.LogoURL)
	require.Equal(t, "https://a.example/banner.png", result.Servers[0].Profile.BannerURL)
}

func TestCrawlReturnsContextError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	neighbors := make([]string, 10)
	for index := range neighbors {
		neighbors[index] = fmt.Sprintf("https://n%d.example", index)
	}
	_, err := Crawl(ctx, []string{self}, neighbors, &fakeFetcher{}, testLimits())
	require.ErrorIs(t, err, context.Canceled)
}

func TestCanonicalOrigin(t *testing.T) {
	tests := map[string]string{
		"https://Example.COM/":     "https://example.com",
		"https://example.com:443":  "https://example.com",
		"http://example.com:8080":  "http://example.com:8080",
		"https://bücher.example":   "https://xn--bcher-kva.example",
		"https://[::1]:8443":       "https://[::1]:8443",
		"https://example.com./":    "https://example.com",
		"https://user@example.com": "",
		"https://example.com/path": "",
		"https://example.com?q":    "",
		"example.com":              "",
		"ftp://example.com":        "",
	}
	for input, want := range tests {
		got, ok := CanonicalOrigin(input)
		require.Equal(t, want != "", ok, input)
		require.Equal(t, want, got, input)
	}
}

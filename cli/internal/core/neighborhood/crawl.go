package neighborhood

import (
	"context"
	"errors"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"hmans.de/chatto/internal/parallel"
)

// Limits bound the remote work of one discovery pass. Each value is a hard
// limit, so the maximum effect of a pass is testable.
type Limits struct {
	// Concurrency is the maximum number of simultaneous remote requests.
	Concurrency int
	// RequestTimeout bounds each remote request.
	RequestTimeout time.Duration
	// NeighborsPerDirectory is the number of advertised origins that
	// discovery reads from one remote directory.
	NeighborsPerDirectory int
	// DirectoryRequests is the maximum number of remote directory requests.
	DirectoryRequests int
	// ProfileRequests is the maximum number of remote profile requests. It
	// is also the maximum number of servers in one result.
	ProfileRequests int
	// MaxDiscoveredOriginLength rejects longer origins that a remote
	// directory advertises. Canonical DNS origins are much shorter.
	MaxDiscoveredOriginLength int
}

// DefaultLimits are the production discovery limits.
var DefaultLimits = Limits{
	Concurrency:               6,
	RequestTimeout:            10 * time.Second,
	NeighborsPerDirectory:     100,
	DirectoryRequests:         150,
	ProfileRequests:           120,
	MaxDiscoveredOriginLength: 300,
}

// Maximum byte lengths of stored public profile text. Longer values are
// truncated at a UTF-8 boundary.
const (
	MaxNameLength        = 200
	MaxVersionLength     = 64
	MaxDescriptionLength = 1000
	// MaxImageURLLength drops longer logo and banner URLs.
	MaxImageURLLength = 2048
)

// ErrUnsupported means that a remote server does not provide a Neighbor
// directory. Discovery treats that server as having no Neighbors.
var ErrUnsupported = errors.New("remote server does not provide a Neighbor directory")

// Profile is the public profile of a discovered server.
type Profile struct {
	Name        string
	Version     string
	Description string
	// LogoURL and BannerURL are absolute remote URLs. They can be empty.
	LogoURL   string
	BannerURL string
}

// Fetcher loads public discovery data from a remote server.
type Fetcher interface {
	// ListNeighbors returns the origins that the remote server advertises.
	// It returns ErrUnsupported when the server has no Neighbor directory.
	ListNeighbors(ctx context.Context, origin string) ([]string, error)
	// GetProfile returns the public profile of the remote server.
	GetProfile(ctx context.Context, origin string) (Profile, error)
}

// Server is one discovered server with a loaded public profile.
type Server struct {
	Origin  string
	Profile Profile
	// DirectNeighbor reports whether the local server advertises Origin.
	DirectNeighbor bool
	// RecommendedBy lists other servers in the same result that mutually
	// recommend Origin, in discovery order.
	RecommendedBy []string
}

// Result is the outcome of one discovery pass.
type Result struct {
	// Servers are in discovery order. The order is not a ranking.
	Servers []Server
	// DirectoryRequests and ProfileRequests count the remote requests.
	DirectoryRequests int
	ProfileRequests   int
	// FailedRequests counts remote requests that did not succeed.
	FailedRequests int
}

// Crawl discovers the Neighborhood of a server.
//
// selfOrigins identifies the local server. neighbors are the origins that the
// local server advertises. Discovery lists each direct Neighbor. When a
// direct Neighbor advertises the local server back, discovery also reads that
// Neighbor's directory. It lists a recommended server when that server
// advertises the recommending server back. Discovery follows at most two
// such mutual hops from the local server. A server appears in the result
// only after its public profile loads.
//
// Remote failures do not stop the pass. Crawl returns an error only when ctx
// ends.
func Crawl(ctx context.Context, selfOrigins []string, neighbors []string, fetcher Fetcher, limits Limits) (Result, error) {
	c := &crawl{
		self:        make(map[string]struct{}, len(selfOrigins)),
		fetcher:     fetcher,
		limits:      limits,
		directories: make(map[string]directory),
		requested:   make(map[string]struct{}),
		recorded:    make(map[string]*recording),
		mutual:      make(map[string]struct{}),
	}
	for _, origin := range selfOrigins {
		if canonical, ok := CanonicalOrigin(origin); ok {
			c.self[canonical] = struct{}{}
		}
	}
	c.ownNeighbors = c.canonicalTargets("", neighbors, 0)
	for _, origin := range c.ownNeighbors {
		c.record(origin, "")
	}

	// First hop: read the directory of each direct Neighbor to find the
	// Neighbors that advertise this server back.
	if err := c.loadDirectories(ctx, c.ownNeighbors); err != nil {
		return Result{}, err
	}
	var expansionSources []string
	for _, origin := range c.ownNeighbors {
		if c.advertisesSelf(origin) {
			expansionSources = append(expansionSources, origin)
		}
	}

	// Second hop: read the directories that mutual direct Neighbors
	// recommend. Round-robin order shares the budget between the sources.
	if err := c.loadDirectories(ctx, c.roundRobinCandidates(expansionSources)); err != nil {
		return Result{}, err
	}
	c.recordMutualRecommendations()

	servers, err := c.loadProfiles(ctx)
	if err != nil {
		return Result{}, err
	}
	return Result{
		Servers:           servers,
		DirectoryRequests: c.directoryRequests,
		ProfileRequests:   c.profileRequests,
		FailedRequests:    c.failedRequests,
	}, nil
}

type crawl struct {
	self         map[string]struct{}
	fetcher      Fetcher
	limits       Limits
	ownNeighbors []string
	// directories holds each loaded remote directory. A failed request has no
	// entry. An unsupported directory has an empty entry.
	directories map[string]directory
	// requested holds each origin whose directory discovery requested.
	requested map[string]struct{}
	// recorded holds each server that discovery lists if its profile loads.
	recorded          map[string]*recording
	order             []string
	mutual            map[string]struct{}
	directoryRequests int
	profileRequests   int
	failedRequests    int
}

type recording struct {
	direct        bool
	recommendedBy []string
}

// canonicalTargets returns the unique canonical origins in advertised, in
// their original order. It removes the source, the local server, and invalid
// values. A positive maxLength also removes longer origins.
func (c *crawl) canonicalTargets(source string, advertised []string, maxLength int) []string {
	targets := make([]string, 0, len(advertised))
	seen := make(map[string]struct{}, len(advertised))
	for _, raw := range advertised {
		origin, ok := CanonicalOrigin(raw)
		if !ok || origin == source || (maxLength > 0 && len(origin) > maxLength) {
			continue
		}
		if _, isSelf := c.self[origin]; isSelf {
			continue
		}
		if _, duplicate := seen[origin]; duplicate {
			continue
		}
		seen[origin] = struct{}{}
		targets = append(targets, origin)
	}
	return targets
}

// record adds source as a recommender of target. An empty source means the
// local server.
func (c *crawl) record(target, source string) {
	entry, exists := c.recorded[target]
	if !exists {
		entry = &recording{}
		c.recorded[target] = entry
		c.order = append(c.order, target)
	}
	if source == "" {
		entry.direct = true
		return
	}
	if !slices.Contains(entry.recommendedBy, source) {
		entry.recommendedBy = append(entry.recommendedBy, source)
	}
}

// directory is one loaded remote Neighbor directory.
type directory struct {
	// targets are the canonical recommendations without the local server.
	targets []string
	// advertised holds each target for membership checks.
	advertised map[string]struct{}
	// advertisesSelf reports whether the directory lists the local server.
	advertisesSelf bool
}

// newDirectory canonicalizes the first NeighborsPerDirectory values of one
// remote directory.
func (c *crawl) newDirectory(origin string, raw []string) directory {
	if len(raw) > c.limits.NeighborsPerDirectory {
		raw = raw[:c.limits.NeighborsPerDirectory]
	}
	loaded := directory{targets: c.canonicalTargets(origin, raw, c.limits.MaxDiscoveredOriginLength)}
	loaded.advertised = make(map[string]struct{}, len(loaded.targets))
	for _, target := range loaded.targets {
		loaded.advertised[target] = struct{}{}
	}
	for _, value := range raw {
		if canonical, ok := CanonicalOrigin(value); ok {
			if _, isSelf := c.self[canonical]; isSelf {
				loaded.advertisesSelf = true
				break
			}
		}
	}
	return loaded
}

// advertisesSelf reports whether the loaded directory of origin contains the
// local server.
func (c *crawl) advertisesSelf(origin string) bool {
	return c.directories[origin].advertisesSelf
}

// advertises reports whether the loaded directory of source contains target.
func (c *crawl) advertises(source, target string) bool {
	_, exists := c.directories[source].advertised[target]
	return exists
}

func (c *crawl) roundRobinCandidates(sources []string) []string {
	queues := make([][]string, len(sources))
	for index, source := range sources {
		queues[index] = slices.Clone(c.directories[source].targets)
	}
	var candidates []string
	seen := make(map[string]struct{})
	for remaining := true; remaining; {
		remaining = false
		for index := range queues {
			for len(queues[index]) > 0 {
				candidate := queues[index][0]
				queues[index] = queues[index][1:]
				if _, requested := c.requested[candidate]; requested {
					continue
				}
				if _, duplicate := seen[candidate]; duplicate {
					continue
				}
				seen[candidate] = struct{}{}
				candidates = append(candidates, candidate)
				remaining = true
				break
			}
		}
	}
	return candidates
}

// recordMutualRecommendations applies the mutual-hop rule to every loaded
// directory until no new server becomes eligible. The local server is always
// eligible. A server becomes eligible when an eligible source recommends it
// and it advertises that source back. Discovery reads new directories only
// from direct Neighbors, which bounds the result to two mutual hops.
func (c *crawl) recordMutualRecommendations() {
	for _, origin := range c.ownNeighbors {
		if c.advertisesSelf(origin) {
			c.mutual[origin] = struct{}{}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, source := range c.eligibleSources() {
			if _, loaded := c.directories[source]; !loaded {
				continue
			}
			for _, target := range c.directories[source].targets {
				if _, loaded := c.directories[target]; !loaded || !c.advertises(target, source) {
					continue
				}
				c.record(target, source)
				if _, known := c.mutual[target]; !known {
					c.mutual[target] = struct{}{}
					changed = true
				}
			}
		}
	}
}

func (c *crawl) eligibleSources() []string {
	sources := make([]string, 0, len(c.mutual))
	for _, origin := range c.order {
		if _, eligible := c.mutual[origin]; eligible {
			sources = append(sources, origin)
		}
	}
	return sources
}

func (c *crawl) loadDirectories(ctx context.Context, origins []string) error {
	budget := max(c.limits.DirectoryRequests-c.directoryRequests, 0)
	if len(origins) > budget {
		origins = origins[:budget]
	}
	c.directoryRequests += len(origins)
	for _, origin := range origins {
		c.requested[origin] = struct{}{}
	}
	type loaded struct {
		origins []string
		ok      bool
	}
	results, err := parallel.Map(ctx, c.limits.Concurrency, origins, func(ctx context.Context, _ int, origin string) (loaded, error) {
		requestCtx, cancel := context.WithTimeout(ctx, c.limits.RequestTimeout)
		defer cancel()
		advertised, err := c.fetcher.ListNeighbors(requestCtx, origin)
		if errors.Is(err, ErrUnsupported) {
			return loaded{ok: true}, nil
		}
		if err != nil {
			return loaded{}, ctx.Err()
		}
		return loaded{origins: advertised, ok: true}, nil
	})
	if err != nil {
		return err
	}
	for index, result := range results {
		if !result.ok {
			c.failedRequests++
			continue
		}
		c.directories[origins[index]] = c.newDirectory(origins[index], result.origins)
	}
	return nil
}

func (c *crawl) loadProfiles(ctx context.Context) ([]Server, error) {
	origins := c.order
	if len(origins) > c.limits.ProfileRequests {
		origins = origins[:c.limits.ProfileRequests]
	}
	c.profileRequests = len(origins)
	profiles, err := parallel.Map(ctx, c.limits.Concurrency, origins, func(ctx context.Context, _ int, origin string) (*Profile, error) {
		requestCtx, cancel := context.WithTimeout(ctx, c.limits.RequestTimeout)
		defer cancel()
		profile, err := c.fetcher.GetProfile(requestCtx, origin)
		if err != nil || profile.Name == "" {
			return nil, ctx.Err()
		}
		profile.Name = truncateUTF8(profile.Name, MaxNameLength)
		profile.Version = truncateUTF8(profile.Version, MaxVersionLength)
		profile.Description = truncateUTF8(profile.Description, MaxDescriptionLength)
		if len(profile.LogoURL) > MaxImageURLLength {
			profile.LogoURL = ""
		}
		if len(profile.BannerURL) > MaxImageURLLength {
			profile.BannerURL = ""
		}
		return &profile, nil
	})
	if err != nil {
		return nil, err
	}
	listed := make(map[string]struct{}, len(origins))
	for index, profile := range profiles {
		if profile != nil {
			listed[origins[index]] = struct{}{}
		}
	}
	servers := make([]Server, 0, len(listed))
	for index, profile := range profiles {
		if profile == nil {
			c.failedRequests++
			continue
		}
		recording := c.recorded[origins[index]]
		// A recommender without a loaded profile is not in the result.
		recommendedBy := slices.DeleteFunc(slices.Clone(recording.recommendedBy), func(origin string) bool {
			_, exists := listed[origin]
			return !exists
		})
		servers = append(servers, Server{
			Origin:         origins[index],
			Profile:        *profile,
			DirectNeighbor: recording.direct,
			RecommendedBy:  recommendedBy,
		})
	}
	return servers, nil
}

// truncateUTF8 removes invalid UTF-8 and shortens value to at most maxBytes
// without splitting a character.
func truncateUTF8(value string, maxBytes int) string {
	value = strings.ToValidUTF8(value, "")
	if len(value) <= maxBytes {
		return value
	}
	cut := maxBytes
	for cut > 0 && !utf8.RuneStart(value[cut]) {
		cut--
	}
	return value[:cut]
}

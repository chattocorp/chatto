package core

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/assets"
	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/core/linkpreview"
	"hmans.de/chatto/internal/core/neighborhood"
	"hmans.de/chatto/internal/lease"
	"hmans.de/chatto/internal/parallel"
	cachestatev1 "hmans.de/chatto/internal/pb/chatto/core/cache_state/v1"
)

const (
	neighborhoodImagesBucket = "NEIGHBORHOOD_IMAGES"
	// neighborhoodDirectoryKey stores the latest discovery result in
	// MEMORY_CACHE.
	neighborhoodDirectoryKey = "neighborhood.directory"
	neighborhoodLeaseName    = "neighborhood-discovery"

	// neighborhoodRefreshAge is the maximum age of a directory before the
	// next discovery pass.
	neighborhoodRefreshAge = time.Hour
	// neighborhoodSourceChangeDelay is the minimum directory age before a
	// Neighbor change starts a new pass. It limits the pass rate during a
	// series of edits.
	neighborhoodSourceChangeDelay = 10 * time.Second
	// neighborhoodChangeDebounce is the wait after a local Neighbor change
	// before the replica checks the directory. It merges quick successive
	// edits into one pass.
	neighborhoodChangeDebounce = 3 * time.Second
	// neighborhoodIncompleteRetryAge is the maximum age of a directory from
	// a pass with failed remote requests.
	neighborhoodIncompleteRetryAge = 10 * time.Minute
	neighborhoodCheckInterval      = time.Minute
	// neighborhoodPassTimeout bounds one pass, including image downloads.
	neighborhoodPassTimeout = 20 * time.Minute
	// neighborhoodImageTTL is the object store TTL. Discovery rewrites an
	// image that it still uses after neighborhoodImageRewriteAge.
	neighborhoodImageTTL        = 7 * 24 * time.Hour
	neighborhoodImageRewriteAge = 3 * 24 * time.Hour
	// maxNeighborhoodDirectoryBytes keeps the stored directory below the
	// default NATS payload limit.
	maxNeighborhoodDirectoryBytes = 900 << 10
)

// neighborhoodImageName matches the content-addressed object names in
// NEIGHBORHOOD_IMAGES.
var neighborhoodImageName = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ErrNeighborhoodImageNotFound means that a Neighborhood image does not exist
// or has expired.
var ErrNeighborhoodImageNotFound = errors.New("Neighborhood image not found")

// neighborhoodFetcher loads remote discovery data and profile images.
type neighborhoodFetcher interface {
	neighborhood.Fetcher
	FetchImage(ctx context.Context, origin, rawURL string) ([]byte, error)
}

type neighborhoodLease interface {
	TryRun(context.Context, func(context.Context) error) (bool, error)
}

// neighborhoodDiscovery refreshes the cached Neighborhood directory. Every
// replica checks the directory age. A shared lease lets one replica run a
// pass at a time, and the shared directory age limits the cluster-wide rate.
type neighborhoodDiscovery struct {
	kv          jetstream.KeyValue
	images      jetstream.ObjectStore
	lease       neighborhoodLease
	fetcher     neighborhoodFetcher
	selfOrigins []string
	neighbors   func() []string
	assetsCfg   assets.Config
	limits      neighborhood.Limits
	logger      *log.Logger
	now         func() time.Time
	// changed receives a signal after a Neighbor mutation on this replica.
	// The buffer holds one pending signal.
	changed chan struct{}
	// checkInterval and changeDebounce override the production timings in
	// tests.
	checkInterval  time.Duration
	changeDebounce time.Duration
}

// notifyNeighborsChanged asks the worker on this replica to check the
// directory soon. Other replicas find the change at their next periodic
// check. The call never blocks.
func (d *neighborhoodDiscovery) notifyNeighborsChanged() {
	if d == nil || d.changed == nil {
		return
	}
	select {
	case d.changed <- struct{}{}:
	default:
	}
}

// Run checks the directory once per minute after boot, and soon after a
// Neighbor change on this replica. It refreshes the directory when it is
// missing, old, incomplete for ten minutes, or based on different Neighbors.
func (d *neighborhoodDiscovery) Run(ctx context.Context, bootDone <-chan struct{}) error {
	select {
	case <-bootDone:
	case <-ctx.Done():
		return ctx.Err()
	}
	checkInterval := cmp.Or(d.checkInterval, neighborhoodCheckInterval)
	changeDebounce := cmp.Or(d.changeDebounce, neighborhoodChangeDebounce)
	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()
	for {
		retryAfter, err := d.refreshIfDue(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			d.logger.Warn("Neighborhood discovery failed", "stage", "refresh", "error", err)
		}
		// A Neighbor change that arrived during the minimum pass interval is
		// checked again when that interval ends.
		var retry <-chan time.Time
		if retryAfter > 0 {
			retry = time.After(retryAfter)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		case <-retry:
		case <-d.changed:
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(changeDebounce):
			}
			// Signals from edits during the wait belong to this check.
			select {
			case <-d.changed:
			default:
			}
		}
	}
}

// refreshIfDue runs a discovery pass when the directory is due. When the
// directory still does not match the current Neighbors afterwards, it returns
// the time after which the caller checks again: the rest of the minimum pass
// interval, or the full interval while another replica holds the lease.
func (d *neighborhoodDiscovery) refreshIfDue(ctx context.Context) (time.Duration, error) {
	sources := d.neighbors()
	fingerprint := neighborhoodSourceFingerprint(sources)
	current, err := d.load(ctx)
	if err != nil {
		return 0, err
	}
	if !d.due(current, fingerprint) {
		return d.pendingChangeRetry(current, fingerprint), nil
	}
	var retryAfter time.Duration
	acquired, err := d.lease.TryRun(ctx, func(leaderCtx context.Context) error {
		// Another replica can finish a pass between the first check and
		// lease acquisition.
		current, err := d.load(leaderCtx)
		if err != nil {
			return err
		}
		if !d.due(current, fingerprint) {
			retryAfter = d.pendingChangeRetry(current, fingerprint)
			return nil
		}
		return d.refresh(leaderCtx, sources, fingerprint, current)
	})
	if err == nil && !acquired && current.GetSourceFingerprint() != fingerprint {
		// The pass on the other replica can use an older Neighbor list.
		retryAfter = neighborhoodSourceChangeDelay
	}
	return retryAfter, err
}

// pendingChangeRetry returns the rest of the minimum pass interval when the
// directory is based on different Neighbors, and zero otherwise.
func (d *neighborhoodDiscovery) pendingChangeRetry(current *cachestatev1.NeighborhoodDirectory, fingerprint string) time.Duration {
	if current == nil || current.GetSourceFingerprint() == fingerprint {
		return 0
	}
	age := d.now().Sub(current.GetRefreshedAt().AsTime())
	return max(neighborhoodSourceChangeDelay-age, time.Millisecond)
}

func (d *neighborhoodDiscovery) due(current *cachestatev1.NeighborhoodDirectory, fingerprint string) bool {
	if current == nil || current.GetRefreshedAt() == nil {
		return true
	}
	age := d.now().Sub(current.GetRefreshedAt().AsTime())
	if age >= neighborhoodRefreshAge || age < 0 {
		return true
	}
	if current.GetIncomplete() && age >= neighborhoodIncompleteRetryAge {
		return true
	}
	return current.GetSourceFingerprint() != fingerprint && age >= neighborhoodSourceChangeDelay
}

func (d *neighborhoodDiscovery) refresh(ctx context.Context, sources []string, fingerprint string, previous *cachestatev1.NeighborhoodDirectory) error {
	passCtx, cancel := context.WithTimeout(ctx, neighborhoodPassTimeout)
	defer cancel()
	started := d.now()
	result, err := neighborhood.Crawl(passCtx, d.selfOrigins, sources, d.fetcher, d.limits)
	if err != nil {
		return fmt.Errorf("discover Neighborhood: %w", err)
	}

	previousImages := make(map[string]*cachestatev1.NeighborhoodServerRecord, len(previous.GetServers()))
	for _, record := range previous.GetServers() {
		previousImages[record.GetOrigin()] = record
	}
	records, err := parallel.Map(passCtx, d.limits.Concurrency, result.Servers, func(ctx context.Context, _ int, server neighborhood.Server) (*cachestatev1.NeighborhoodServerRecord, error) {
		prior := previousImages[server.Origin]
		return &cachestatev1.NeighborhoodServerRecord{
			Origin:               server.Origin,
			Name:                 server.Profile.Name,
			Version:              server.Profile.Version,
			Description:          server.Profile.Description,
			DirectNeighbor:       server.DirectNeighbor,
			RecommendedByOrigins: server.RecommendedBy,
			Logo:                 d.storeImage(ctx, server.Origin, server.Profile.LogoURL, prior.GetLogo(), assets.ProcessAvatarImageWithConfig),
			Banner:               d.storeImage(ctx, server.Origin, server.Profile.BannerURL, prior.GetBanner(), assets.ProcessLinkPreviewImageWithConfig),
		}, nil
	})
	if err != nil {
		return fmt.Errorf("store Neighborhood images: %w", err)
	}

	directory := &cachestatev1.NeighborhoodDirectory{
		RefreshedAt:       timestamppb.New(d.now()),
		SourceFingerprint: fingerprint,
		Servers:           records,
		Incomplete:        result.FailedRequests > 0,
	}
	data, err := marshalNeighborhoodDirectory(directory)
	if err != nil {
		return err
	}
	if _, err := d.kv.Put(ctx, neighborhoodDirectoryKey, data); err != nil {
		return fmt.Errorf("store Neighborhood directory: %w", err)
	}
	d.logger.Info("Neighborhood discovery completed",
		"sources", len(sources),
		"servers", len(directory.GetServers()),
		"directory_requests", result.DirectoryRequests,
		"profile_requests", result.ProfileRequests,
		"failed_requests", result.FailedRequests,
		"duration", d.now().Sub(started).Round(time.Millisecond))
	return nil
}

// marshalNeighborhoodDirectory removes servers from the end until the value
// fits in one MEMORY_CACHE record. It also removes recommendations from the
// removed servers.
func marshalNeighborhoodDirectory(directory *cachestatev1.NeighborhoodDirectory) ([]byte, error) {
	for {
		data, err := proto.Marshal(directory)
		if err != nil {
			return nil, fmt.Errorf("marshal Neighborhood directory: %w", err)
		}
		if len(data) <= maxNeighborhoodDirectoryBytes || len(directory.Servers) == 0 {
			return data, nil
		}
		removed := directory.Servers[len(directory.Servers)-1].GetOrigin()
		directory.Servers = directory.Servers[:len(directory.Servers)-1]
		for _, server := range directory.Servers {
			server.RecommendedByOrigins = slices.DeleteFunc(server.RecommendedByOrigins, func(origin string) bool {
				return origin == removed
			})
		}
	}
}

type neighborhoodImageProcessor func(io.Reader, assets.Config) (io.Reader, error)

// storeImage returns a cached copy of one remote profile image, or nil when
// the image is absent or unusable. It reuses the previous copy while the
// source URL is unchanged.
func (d *neighborhoodDiscovery) storeImage(ctx context.Context, origin, sourceURL string, previous *cachestatev1.NeighborhoodImage, process neighborhoodImageProcessor) *cachestatev1.NeighborhoodImage {
	if sourceURL == "" {
		return nil
	}
	if previous.GetSourceUrl() == sourceURL && d.keepImage(ctx, previous.GetObjectName()) {
		return previous
	}
	requestCtx, cancel := context.WithTimeout(ctx, d.limits.RequestTimeout)
	defer cancel()
	data, err := d.fetcher.FetchImage(requestCtx, origin, sourceURL)
	if err != nil {
		return nil
	}
	processed, err := process(bytes.NewReader(data), d.assetsCfg)
	if err != nil {
		return nil
	}
	encoded, err := io.ReadAll(processed)
	if err != nil {
		return nil
	}
	sum := sha256.Sum256(encoded)
	name := hex.EncodeToString(sum[:])
	if !d.imageIsFresh(ctx, name) {
		if _, err := d.images.Put(ctx, jetstream.ObjectMeta{
			Name:    name,
			Headers: map[string][]string{"Content-Type": {"image/webp"}},
		}, bytes.NewReader(encoded)); err != nil {
			d.logger.Warn("Neighborhood image storage failed", "stage", "image_put", "error", err)
			return nil
		}
	}
	return &cachestatev1.NeighborhoodImage{SourceUrl: sourceURL, ObjectName: name}
}

// keepImage reports whether a previous image still exists. It rewrites an
// older image so that the object store TTL does not remove an image in use.
func (d *neighborhoodDiscovery) keepImage(ctx context.Context, name string) bool {
	info, err := d.images.GetInfo(ctx, name)
	if err != nil {
		return false
	}
	if d.now().Sub(info.ModTime) < neighborhoodImageRewriteAge {
		return true
	}
	data, err := d.images.GetBytes(ctx, name)
	if err != nil {
		return false
	}
	_, err = d.images.Put(ctx, jetstream.ObjectMeta{Name: name, Headers: info.Headers}, bytes.NewReader(data))
	return err == nil
}

func (d *neighborhoodDiscovery) imageIsFresh(ctx context.Context, name string) bool {
	info, err := d.images.GetInfo(ctx, name)
	return err == nil && d.now().Sub(info.ModTime) < neighborhoodImageRewriteAge
}

func (d *neighborhoodDiscovery) load(ctx context.Context) (*cachestatev1.NeighborhoodDirectory, error) {
	return loadNeighborhoodDirectory(ctx, d.kv)
}

func loadNeighborhoodDirectory(ctx context.Context, kv jetstream.KeyValue) (*cachestatev1.NeighborhoodDirectory, error) {
	entry, err := kv.Get(ctx, neighborhoodDirectoryKey)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read Neighborhood directory: %w", err)
	}
	directory := &cachestatev1.NeighborhoodDirectory{}
	if err := proto.Unmarshal(entry.Value(), directory); err != nil {
		return nil, fmt.Errorf("decode Neighborhood directory: %w", err)
	}
	return directory, nil
}

// neighborhoodSourceFingerprint identifies the Neighbor set of a discovery
// pass. Every replica projects the same Neighbors, so replicas agree on it.
// It excludes the replica's own configured origins: replicas with different
// origin settings must not start passes against each other. The hourly
// refresh applies a change to those settings.
func neighborhoodSourceFingerprint(neighbors []string) string {
	sources := slices.Sorted(slices.Values(neighbors))
	sum := sha256.Sum256([]byte(strings.Join(sources, "\n")))
	return hex.EncodeToString(sum[:])
}

// NeighborhoodDirectory returns the latest cached Neighborhood. It returns
// nil before the first completed discovery pass. The call does not contact
// remote servers.
func (c *ChattoCore) NeighborhoodDirectory(ctx context.Context) (*cachestatev1.NeighborhoodDirectory, error) {
	return loadNeighborhoodDirectory(ctx, c.storage.memoryCacheKV)
}

// NeighborhoodImagePath returns the public server-relative URL path of one
// cached Neighborhood image.
func NeighborhoodImagePath(name string) string {
	return "/assets/neighborhood/" + name
}

// OpenNeighborhoodImage returns one cached Neighborhood image. The caller
// must close the returned reader.
func (c *ChattoCore) OpenNeighborhoodImage(ctx context.Context, name string) (io.ReadCloser, *jetstream.ObjectInfo, error) {
	if !neighborhoodImageName.MatchString(name) {
		return nil, nil, ErrNeighborhoodImageNotFound
	}
	result, err := c.storage.neighborhoodImages.Get(ctx, name)
	if errors.Is(err, jetstream.ErrObjectNotFound) {
		return nil, nil, ErrNeighborhoodImageNotFound
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read Neighborhood image: %w", err)
	}
	info, err := result.Info()
	if err != nil {
		_ = result.Close()
		return nil, nil, fmt.Errorf("read Neighborhood image info: %w", err)
	}
	return result, info, nil
}

func initializeNeighborhoodDiscovery(core *ChattoCore, infra *coreInfrastructure, cfg config.CoreConfig, logger *log.Logger) error {
	discoveryLease, err := lease.New(infra.js, infra.storage.memoryCacheKV, lease.Options{
		Name:   neighborhoodLeaseName,
		Bucket: "MEMORY_CACHE",
		Logger: logger.WithPrefix("core.NeighborhoodDiscoveryLease"),
	})
	if err != nil {
		return fmt.Errorf("failed to initialize Neighborhood discovery lease: %w", err)
	}
	core.neighborhoodDiscovery = &neighborhoodDiscovery{
		kv:          infra.storage.memoryCacheKV,
		images:      infra.storage.neighborhoodImages,
		lease:       discoveryLease,
		fetcher:     neighborhood.RemoteFetcher{Client: newNeighborhoodHTTPClient()},
		selfOrigins: slices.Clone(cfg.ServerOrigins),
		neighbors: func() []string {
			neighbors := core.ConfigModel().ListNeighbors()
			origins := make([]string, 0, len(neighbors))
			for _, neighbor := range neighbors {
				origins = append(origins, neighbor.Origin)
			}
			return origins
		},
		assetsCfg: core.AssetsConfig(),
		limits:    neighborhood.DefaultLimits,
		logger:    logger.WithPrefix("core.NeighborhoodDiscovery"),
		now:       time.Now,
		changed:   make(chan struct{}, 1),
	}
	return nil
}

// newNeighborhoodHTTPClient returns the link-preview client, which rejects
// private network addresses when it connects. A redirect could leave the
// advertised origin, so the client returns it as an unsuccessful response.
func newNeighborhoodHTTPClient() *http.Client {
	client := linkpreview.NewSSRFSafeClient(2 * neighborhood.DefaultLimits.RequestTimeout)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return client
}

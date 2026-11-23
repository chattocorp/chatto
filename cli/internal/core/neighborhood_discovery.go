package core

import (
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/core/linkpreview"
	"github.com/chattocorp/chatto/cli/internal/core/neighborhood"
	cachestatev1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/cache_state/v1"
	"github.com/chattocorp/chatto/pkg/jetstreamutil"
	jobqueue "github.com/chattocorp/chatto/pkg/jobs"
)

const (
	jobTypeNeighborhoodRefresh = "neighborhood_refresh"

	// neighborhoodDirectoryKey stores the latest discovery result in
	// MEMORY_CACHE.
	neighborhoodDirectoryKey = "neighborhood.directory"

	// neighborhoodRefreshAge is the maximum age of a directory before the
	// next discovery pass.
	neighborhoodRefreshAge = time.Hour
	// neighborhoodSourceChangeDelay is the minimum directory age before a
	// Neighbor change starts a new pass. It limits the pass rate during a
	// series of edits.
	neighborhoodSourceChangeDelay = 10 * time.Second
	// neighborhoodIncompleteRetryAge is the maximum age of a directory from
	// a pass with failed remote requests.
	neighborhoodIncompleteRetryAge = 10 * time.Minute
	// neighborhoodCheckInterval is the time between checks. A check reads one
	// MEMORY_CACHE value and hashes the local Neighbor projection.
	neighborhoodCheckInterval = 5 * time.Second
	// neighborhoodFailureBackoff is the wait after a failed check or job attempt.
	neighborhoodFailureBackoff = time.Minute
	// neighborhoodPassTimeout bounds one pass.
	neighborhoodPassTimeout = 20 * time.Minute
	// maxNeighborhoodDirectoryBytes keeps the stored directory below the
	// default NATS payload limit.
	maxNeighborhoodDirectoryBytes = 900 << 10
)

// neighborhoodDiscovery schedules and performs refreshes of the shared directory.
type neighborhoodDiscovery struct {
	// Only the periodic check accesses lastEnqueued. Once JOBS accepts a
	// generation, the queue owns retries until its directory is replaced.
	lastEnqueued string
	enqueue      func(context.Context, jobqueue.Spec) error
	kv           jetstream.KeyValue
	fetcher      neighborhood.Fetcher
	selfOrigins  []string
	neighbors    func() []string
	limits       neighborhood.Limits
	logger       *log.Logger
	now          func() time.Time
	// checkInterval overrides the production cadence in tests.
	checkInterval time.Duration
}

// Run checks the directory every few seconds after boot. It schedules a refresh of the
// directory when it is missing, old, incomplete for ten minutes, or based on
// a different Neighbor set. Every replica sees Neighbor changes through its
// configuration projection, so no change signal is necessary.
func (d *neighborhoodDiscovery) Run(ctx context.Context, bootDone <-chan struct{}) error {
	select {
	case <-bootDone:
	case <-ctx.Done():
		return ctx.Err()
	}
	checkInterval := cmp.Or(d.checkInterval, neighborhoodCheckInterval)
	for {
		wait := checkInterval
		if err := d.refreshIfDue(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			d.logger.Warn("Neighborhood discovery failed", "stage", "refresh", "error", err)
			wait = neighborhoodFailureBackoff
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(wait):
		}
	}
}

// refreshIfDue hands off one job per cache generation. On a cold cache,
// establish a shared generation first. This distinguishes a cache reset from
// an earlier cold start whose job facts are still retained.
func (d *neighborhoodDiscovery) refreshIfDue(ctx context.Context) error {
	entry, current, err := loadNeighborhoodDirectory(ctx, d.kv)
	if err != nil {
		return err
	}
	if entry == nil {
		if _, err := d.kv.Create(ctx, neighborhoodDirectoryKey, nil); err != nil && !jetstreamutil.IsSequenceConflict(err) {
			return err
		}
		entry, current, err = loadNeighborhoodDirectory(ctx, d.kv)
		if err != nil || entry == nil {
			return err
		}
	}
	sources := d.neighbors()
	fingerprint := neighborhoodSourceFingerprint(sources)
	if !d.due(current, fingerprint) {
		return nil
	}
	// With no Neighbors the pass only publishes an empty directory. Use the
	// same conditional write to clear old results without scheduling work.
	if len(sources) == 0 {
		return d.refresh(ctx, sources, fingerprint, current, entry.Revision())
	}
	id := neighborhoodRefreshID(entry)
	if id == d.lastEnqueued {
		return nil
	}
	if err := d.enqueue(ctx, jobqueue.Spec{ID: id, Type: jobTypeNeighborhoodRefresh, MaxAttempts: jobqueue.UnlimitedAttempts, RetryDelay: neighborhoodFailureBackoff}); err != nil {
		return err
	}
	d.lastEnqueued = id
	return nil
}

// The broker timestamp distinguishes revisions reused after a cache reset.
func neighborhoodRefreshID(entry jetstream.KeyValueEntry) string {
	return jobqueue.ID(jobTypeNeighborhoodRefresh, fmt.Sprintf("%d:%d", entry.Revision(), entry.Created().UnixNano()))
}

func (d *neighborhoodDiscovery) performJob(ctx context.Context, job jobqueue.Job) error {
	entry, current, err := loadNeighborhoodDirectory(ctx, d.kv)
	if err != nil || entry == nil || neighborhoodRefreshID(entry) != job.ID {
		return err
	}
	sources := d.neighbors()
	return d.refresh(ctx, sources, neighborhoodSourceFingerprint(sources), current, entry.Revision())
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

func (d *neighborhoodDiscovery) refresh(ctx context.Context, sources []string, fingerprint string, previous *cachestatev1.NeighborhoodDirectory, revision uint64) error {
	passCtx, cancel := context.WithTimeout(ctx, neighborhoodPassTimeout)
	defer cancel()
	started := d.now()
	result, err := neighborhood.Crawl(passCtx, d.selfOrigins, sources, d.fetcher, d.limits)
	if err != nil {
		return fmt.Errorf("discover Neighborhood: %w", err)
	}

	records := make([]*cachestatev1.NeighborhoodServerRecord, 0, len(result.Servers))
	for _, server := range result.Servers {
		records = append(records, &cachestatev1.NeighborhoodServerRecord{
			Origin:               server.Origin,
			Name:                 server.Profile.Name,
			Version:              server.Profile.Version,
			Description:          server.Profile.Description,
			DirectNeighbor:       server.DirectNeighbor,
			RecommendedByOrigins: server.RecommendedBy,
			Logo:                 neighborhoodImage(server.Origin, server.Profile.LogoURL),
			Banner:               neighborhoodImage(server.Origin, server.Profile.BannerURL),
		})
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
	if _, err := d.kv.Update(ctx, neighborhoodDirectoryKey, data, revision); err != nil {
		if jetstreamutil.IsSequenceConflict(err) {
			return nil
		}
		return fmt.Errorf("store Neighborhood directory: %w", err)
	}
	logCompleted := d.logger.Info
	if len(sources) == 0 {
		logCompleted = d.logger.Debug
	}
	logCompleted("Neighborhood discovery completed",
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

// neighborhoodImage returns the record of one remote profile image with its absolute
// source URL, or nil when there is no usable URL. The remote image proxy
// fetches the image when a client first shows it (ADR-128).
func neighborhoodImage(origin, rawURL string) *cachestatev1.NeighborhoodImage {
	if rawURL == "" {
		return nil
	}
	sourceURL, ok := neighborhood.ResolveImageURL(origin, rawURL)
	if !ok {
		return nil
	}
	return &cachestatev1.NeighborhoodImage{SourceUrl: sourceURL}
}

func loadNeighborhoodDirectory(ctx context.Context, kv jetstream.KeyValue) (jetstream.KeyValueEntry, *cachestatev1.NeighborhoodDirectory, error) {
	entry, err := kv.Get(ctx, neighborhoodDirectoryKey)
	if errors.Is(err, jetstream.ErrKeyNotFound) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, fmt.Errorf("read Neighborhood directory: %w", err)
	}
	// An empty value identifies the first pending pass. Public readers still
	// see a missing directory until a worker publishes the result.
	if len(entry.Value()) == 0 {
		return entry, nil, nil
	}
	directory := &cachestatev1.NeighborhoodDirectory{}
	if err := proto.Unmarshal(entry.Value(), directory); err != nil {
		return nil, nil, fmt.Errorf("decode Neighborhood directory: %w", err)
	}
	return entry, directory, nil
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
	_, directory, err := loadNeighborhoodDirectory(ctx, c.storage.memoryCacheKV)
	return directory, err
}

func initializeNeighborhoodDiscovery(core *ChattoCore, infra *coreInfrastructure, cfg config.CoreConfig, logger *log.Logger) {
	core.neighborhoodDiscovery = &neighborhoodDiscovery{
		enqueue:     core.jobs.Enqueue,
		kv:          infra.storage.memoryCacheKV,
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
		limits: neighborhood.DefaultLimits,
		logger: logger.WithPrefix("core.NeighborhoodDiscovery"),
		now:    time.Now,
	}
}

// newNeighborhoodHTTPClient returns the link-preview client, which rejects
// private network addresses when it connects. A redirect could leave the
// advertised origin, so the client returns it as an unsuccessful response.
func newNeighborhoodHTTPClient() *http.Client {
	client := linkpreview.NewSSRFSafeClient(2 * neighborhood.DefaultLimits.RequestTimeout)
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return client
}

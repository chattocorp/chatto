package bleve

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/charmbracelet/log"
	"github.com/chattocorp/chatto/cli/internal/dekstore"
	"github.com/chattocorp/chatto/cli/internal/evtstream"
	"github.com/chattocorp/chatto/cli/internal/kms"
	"github.com/chattocorp/chatto/cli/internal/logbridge"
	"github.com/chattocorp/chatto/cli/internal/natsresources"
	"github.com/chattocorp/chatto/cli/internal/runtimeunit"
	"github.com/chattocorp/chatto/cli/internal/search"
	"github.com/chattocorp/chatto/pkg/events"
	"github.com/chattocorp/chatto/pkg/jetstreamutil"
	"github.com/nats-io/nats.go/micro"
)

const (
	runtimeUnitName                = "search.BleveProvider"
	searchIndexingProgressInterval = 10 * time.Second
	searchStartupPollInterval      = 10 * time.Millisecond
	// startupValidationTimeout bounds the validation query after startup
	// replay. It is longer than the query probe timeout, because the
	// validation reads one stored document from the full index.
	startupValidationTimeout = time.Minute
	// searchShutdownTimeout bounds the wait for the projector and the index
	// to stop. Together with the NATS drain, it fits within the default
	// Kubernetes termination grace period of 30 seconds.
	searchShutdownTimeout = 15 * time.Second
)

// errSearchShutdownTimeout reports index work that did not stop in time.
var errSearchShutdownTimeout = errors.New("search index work did not stop before the shutdown timeout")

// Unit runs the bundled Bleve provider either under chatto run or standalone.
type Unit struct {
	// Health receives process-local health state. The standalone
	// search-provider command sets it; chatto run leaves it nil.
	Health *Health

	// startupResultHook is a test seam for observing the NATS boundary after
	// projection startup resolves but before failure cleanup removes it.
	startupResultHook func(error)
	// newProjection, servingStarted, probeInterval, probeTimeout, and
	// shutdownTimeout are test seams. Zero values select production behavior.
	newProjection   func(*Projection)
	servingStarted  func(micro.Service)
	probeInterval   time.Duration
	probeTimeout    time.Duration
	shutdownTimeout time.Duration
}

func (Unit) Name() string { return runtimeUnitName }

func (u Unit) Run(ctx context.Context, env runtimeunit.Env) error {
	languages := env.Config.SearchProvider.LanguagesOrDefault()
	defer env.Logger.Info("Bundled search provider stopped", "stage", "shutdown")
	u.Health.setNATS(env.NC)

	evt, err := env.JS.Stream(ctx, natsresources.EVT)
	if err != nil {
		return fmt.Errorf("open EVT stream: %w", err)
	}
	encryptionBucket, err := env.JS.KeyValue(ctx, natsresources.EncryptionKeys)
	if err != nil {
		return fmt.Errorf("open ENCRYPTION_KEYS bucket: %w", err)
	}
	runtimeStateBucket, err := env.JS.KeyValue(ctx, natsresources.RuntimeState)
	if err != nil {
		return fmt.Errorf("open RUNTIME_STATE bucket: %w", err)
	}
	encryptionKeys, err := jetstreamutil.NewKeyValue(env.JS, encryptionBucket)
	if err != nil {
		return err
	}
	runtimeState, err := jetstreamutil.NewKeyValue(env.JS, runtimeStateBucket)
	if err != nil {
		return err
	}
	keyStore := kms.NewBuiltin(encryptionKeys, env.Logger)
	projection, err := NewProjection(
		env.Config.SearchProvider.DirectoryOrDefault(),
		languages,
		keyStore,
		keyStore,
		dekstore.New(runtimeState, env.Logger),
		env.Logger,
	)
	if err != nil {
		return err
	}
	if u.newProjection != nil {
		u.newProjection(projection)
	}

	projectionHandle, err := evtstream.NewProjectionHandle(env.JS, evt, projection, events.ProjectorOptions{
		Logger:              logbridge.Slog(env.Logger),
		ConsumerName:        "message_search",
		ConsumerDescription: "Message search index",
		Checkpoint:          &events.CheckpointOptions{Key: "message_search", ResolveStreamIdentity: evtstream.IdentityFromInfo},
	})
	if err != nil {
		return errors.Join(err, projection.Close())
	}
	projector := projectionHandle.Projector()
	provider := newProvider(projectionHandle)
	if u.probeTimeout > 0 {
		provider.probe.timeout = u.probeTimeout
	}
	u.Health.setProvider(provider, projection)
	serviceOptions := search.ServiceOptions{ImplementationVersion: env.Version}
	service, err := search.AddStartupStatusService(ctx, env.NC, provider, serviceOptions)
	if err != nil {
		return errors.Join(fmt.Errorf("register search provider status service: %w", err), projection.Close())
	}
	defer service.Stop()

	monitorContext, stopMonitor := context.WithCancel(ctx)
	defer stopMonitor()
	go logSearchIndexingProgress(monitorContext, projector, env.Logger, searchIndexingProgressInterval)

	projectorContext, stopProjector := context.WithCancel(ctx)
	defer stopProjector()
	projectorDone := make(chan error, 1)
	go func() {
		projectorDone <- projector.Run(projectorContext)
	}()
	shutdown := func(projectorDone <-chan error) error {
		stopProjector()
		return u.stopIndex(env.Logger, projectorDone, projection)
	}
	projectorStopped, err := waitForSearchProjectionStartup(ctx, projector, projectorDone, searchStartupPollInterval)
	if err != nil {
		if u.startupResultHook != nil {
			u.startupResultHook(err)
		}
		if projectorStopped {
			projectorDone = nil
		}
		shutdownErr := shutdown(projectorDone)
		if errors.Is(err, context.Canceled) {
			return shutdownErr
		}
		return errors.Join(err, shutdownErr)
	}

	projectorStopped, projectorErr := u.serve(ctx, env, provider, serviceOptions, projectorDone)
	if projectorStopped {
		projectorDone = nil
	}
	shutdownErr := shutdown(projectorDone)
	if errors.Is(projectorErr, context.Canceled) {
		projectorErr = nil
	}
	return errors.Join(projectorErr, shutdownErr)
}

// serve keeps the provider's query and ready status subscriptions aligned
// with its query probe. It joins the shared queues only after the startup
// validation (completeStartup) and a probe succeed, and leaves them while the
// latest probe failed or a probe is blocked, so that healthy replicas answer
// instead. A failed validation is tried again at each interval; it never
// deletes the index. It returns when ctx ends
// or the projector stops; the bool reports that the projector stopped, and
// the error is then the projector's error.
func (u Unit) serve(ctx context.Context, env runtimeunit.Env, provider *Provider, options search.ServiceOptions, projectorDone <-chan error) (bool, error) {
	interval := u.probeInterval
	if interval <= 0 {
		interval = queryProbeInterval
	}
	var serving micro.Service
	stopServing := func() {
		if serving != nil {
			_ = serving.Stop()
			serving = nil
			provider.serving.Store(false)
		}
	}
	defer stopServing()

	// validation runs completeStartup in its own goroutine, so that a blocked
	// index cannot block shutdown. At most one validation runs at a time.
	validated := false
	var validationDone chan error
	startValidation := func() {
		validationDone = make(chan error, 1)
		go func(done chan<- error) {
			validationContext, cancel := context.WithTimeout(ctx, startupValidationTimeout)
			defer cancel()
			recovered, err := provider.projection.Projection().completeStartup(validationContext)
			if err == nil && recovered {
				env.Logger.Info("Search index recovery complete; a validation query succeeded", "stage", "index_recovery_complete")
			}
			done <- err
		}(validationDone)
	}

	readyLogged := false
	reconcile := func() error {
		// NATS micro stops a service by itself after some subscription
		// errors, for example a slow consumer. Clear readiness and register
		// a new service below.
		if serving != nil && serving.Stopped() {
			serving = nil
			provider.serving.Store(false)
			env.Logger.Warn("Search provider query service stopped unexpectedly", "stage", "query_service_stopped")
		}
		snapshot, _ := provider.probe.Snapshot()
		healthy := validated && snapshot.Healthy(time.Now())
		switch {
		case healthy && serving == nil:
			service, err := search.AddService(ctx, env.NC, provider, options)
			if err != nil {
				return fmt.Errorf("register search provider query endpoint: %w", err)
			}
			serving = service
			provider.serving.Store(true)
			if u.servingStarted != nil {
				u.servingStarted(service)
			}
			if !readyLogged {
				// The operator's standalone search readiness probe consumes
				// this stage field. Emit it once, after replay, a successful
				// query probe, and query/status endpoint registration.
				env.Logger.Info("Search provider ready", "stage", "service_ready")
				readyLogged = true
			} else {
				env.Logger.Info("Search provider resumed serving queries", "stage", "query_probe_recovered")
			}
		case !healthy && serving != nil:
			stopServing()
			env.Logger.Warn("Search provider stopped serving queries",
				"stage", "query_probe_failed",
				"probe_result", snapshot.LastResult.String(),
				"consecutive_failures", snapshot.ConsecutiveFailures,
				"probe_in_flight", !snapshot.InFlightSince.IsZero())
		}
		return nil
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	probeDone := provider.probe.Start(ctx)
	startValidation()
	for {
		select {
		case <-ctx.Done():
			return false, nil
		case err := <-projectorDone:
			return true, err
		case err := <-validationDone:
			validationDone = nil
			if err != nil {
				env.Logger.Warn("Search index validation failed; the provider does not serve queries", "stage", "startup_validation_failed", "error", err)
				break
			}
			validated = true
			if err := reconcile(); err != nil {
				return false, err
			}
		case <-probeDone:
			probeDone = nil
			if err := reconcile(); err != nil {
				return false, err
			}
		case <-ticker.C:
			if err := reconcile(); err != nil {
				return false, err
			}
			if probeDone == nil {
				probeDone = provider.probe.Start(ctx)
			}
			if !validated && validationDone == nil {
				startValidation()
			}
		}
	}
}

// stopIndex waits for the cancelled projector, then closes the projection.
// projectorDone is nil when the projector already stopped. A commit that
// does not return holds the projection lock, so both waits run in one
// goroutine bounded by the shutdown timeout. On timeout, stopIndex leaves the
// projection open; the process is expected to exit.
func (u Unit) stopIndex(logger *log.Logger, projectorDone <-chan error, projection *Projection) error {
	timeout := u.shutdownTimeout
	if timeout <= 0 {
		timeout = searchShutdownTimeout
	}
	stopped := make(chan error, 1)
	go func() {
		var err error
		if projectorDone != nil {
			if err = <-projectorDone; errors.Is(err, context.Canceled) {
				err = nil
			}
		}
		stopped <- errors.Join(err, projection.Close())
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-stopped:
		return err
	case <-timer.C:
		diagnostics := projection.diagnostics.Snapshot(time.Now())
		logger.Error("Search index work did not stop before the shutdown timeout",
			"stage", "shutdown_timeout",
			"timeout", timeout,
			"write_stage", diagnostics.WriteStage,
			"write_elapsed_seconds", diagnostics.WriteElapsedSeconds,
			"write_first_seq", diagnostics.WriteFirstSeq,
			"write_last_seq", diagnostics.WriteLastSeq,
			"queries_waiting", diagnostics.QueriesWaiting,
			"queries_active", diagnostics.QueriesActive)
		return errSearchShutdownTimeout
	}
}

func waitForSearchProjectionStartup(ctx context.Context, projector *events.Projector, done <-chan error, pollInterval time.Duration) (bool, error) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		status := projector.Status()
		if status.StartupComplete {
			return false, nil
		}
		if status.Err != nil {
			return false, status.Err
		}
		select {
		case err := <-done:
			return true, err
		case <-ctx.Done():
			return false, ctx.Err()
		case <-ticker.C:
		}
	}
}

func logSearchIndexingProgress(ctx context.Context, projector *events.Projector, logger *log.Logger, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var previousMessages uint64
	var previousDuration time.Duration
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			status := projector.Status()
			if status.StartupComplete || status.Err != nil {
				return
			}
			if !status.Started {
				continue
			}
			logSearchIndexingStatus(logger, status, previousMessages, previousDuration)
			previousMessages = status.StartupMessages
			previousDuration = status.StartupDuration
		}
	}
}

func logSearchIndexingStatus(logger *log.Logger, status events.ProjectorStatus, previousMessages uint64, previousDuration time.Duration) {
	var averageRate float64
	if seconds := status.StartupDuration.Seconds(); seconds > 0 {
		averageRate = float64(status.StartupMessages) / seconds
	}
	recentMessages := status.StartupMessages - min(status.StartupMessages, previousMessages)
	recentDuration := status.StartupDuration - min(status.StartupDuration, previousDuration)
	var recentRate float64
	if seconds := recentDuration.Seconds(); seconds > 0 {
		recentRate = float64(recentMessages) / seconds
	}
	logger.Info("Search provider indexing progress",
		"stage", "startup_replay",
		"indexed_events", status.StartupMessages,
		"events_since_last_report", recentMessages,
		"events_per_second", recentRate,
		"average_events_per_second", averageRate,
		"stalled", recentDuration > 0 && recentMessages == 0,
		"current_seq", status.LastSeq,
		"target_seq", status.StartupTargetSeq,
		"elapsed", status.StartupDuration)
}

var _ runtimeunit.Unit = Unit{}

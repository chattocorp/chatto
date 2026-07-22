package bleve

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hmans.de/chatto/internal/dekstore"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/kms"
	"hmans.de/chatto/internal/runtimeunit"
	"hmans.de/chatto/internal/search"
)

const (
	runtimeUnitName                = "search.BleveProvider"
	searchIndexingProgressInterval = 10 * time.Second
	searchStartupPollInterval      = 10 * time.Millisecond
)

// Unit runs the bundled Bleve provider either under chatto run or standalone.
type Unit struct {
	// startupResultHook is a test seam for observing the NATS boundary after
	// projection startup resolves but before failure cleanup removes it.
	startupResultHook func(error)
}

func (Unit) Name() string { return runtimeUnitName }

func (u Unit) Run(ctx context.Context, env runtimeunit.Env) error {
	languages := env.Config.SearchProvider.LanguagesOrDefault()
	env.Logger.Info("Starting bundled search provider",
		"stage", "startup",
		"startup_batch_size", startupReplayBatchSize,
		"language_analyzers", languages,
		"language_analyzer_count", len(languages))
	defer env.Logger.Info("Bundled search provider stopped", "stage", "shutdown")

	evt, err := env.JS.Stream(ctx, "EVT")
	if err != nil {
		return fmt.Errorf("open EVT stream: %w", err)
	}
	encryptionKeys, err := env.JS.KeyValue(ctx, "ENCRYPTION_KEYS")
	if err != nil {
		return fmt.Errorf("open ENCRYPTION_KEYS bucket: %w", err)
	}
	runtimeState, err := env.JS.KeyValue(ctx, "RUNTIME_STATE")
	if err != nil {
		return fmt.Errorf("open RUNTIME_STATE bucket: %w", err)
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
	defer projection.Close()
	env.Logger.Info("Search index opened",
		"stage", "index_open",
		"checkpoint_contract", projection.CheckpointContractID())

	projector := events.NewProjector(env.JS, evt, projection, env.Logger)
	if err := projector.ConfigureCheckpoint("message_search"); err != nil {
		return err
	}
	provider := &Provider{Projection: projection, Projector: projector}
	service, err := search.AddStartupStatusService(ctx, env.NC, provider, search.ServiceOptions{ImplementationVersion: env.Version})
	if err != nil {
		return fmt.Errorf("register search provider status service: %w", err)
	}
	defer service.Stop()
	env.Logger.Info("Search provider status service registered",
		"stage", "status_ready",
		"status_subject", search.StartupStatusSubject)

	monitorContext, stopMonitor := context.WithCancel(ctx)
	defer stopMonitor()
	go logSearchIndexingProgress(monitorContext, projector, env.Logger, searchIndexingProgressInterval)

	projectorContext, stopProjector := context.WithCancel(ctx)
	projectorDone := make(chan error, 1)
	go func() {
		projectorDone <- projector.Run(projectorContext)
	}()
	projectorStopped, err := waitForSearchProjectionStartup(ctx, projector, projectorDone, searchStartupPollInterval)
	if err != nil {
		if u.startupResultHook != nil {
			u.startupResultHook(err)
		}
		stopProjector()
		if !projectorStopped {
			<-projectorDone
		}
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
	if err := search.AddQueryEndpoint(ctx, service, provider); err != nil {
		stopProjector()
		<-projectorDone
		return fmt.Errorf("register search provider query endpoint: %w", err)
	}
	if err := search.AddStatusEndpoint(ctx, service, provider); err != nil {
		stopProjector()
		<-projectorDone
		return fmt.Errorf("register search provider ready status endpoint: %w", err)
	}
	env.Logger.Info("Search provider service registered",
		"stage", "service_ready",
		"query_subject", search.QuerySubject,
		"status_subject", search.StatusSubject)

	err = <-projectorDone
	stopProjector()
	if errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func waitForSearchProjectionStartup(ctx context.Context, projector *events.Projector, done <-chan error, pollInterval time.Duration) (bool, error) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		status := projector.Status()
		if status.StartupComplete {
			return false, nil
		}
		if status.Failed {
			if status.Err != nil {
				return false, status.Err
			}
			return false, events.ErrProjectionFailed
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

func logSearchIndexingProgress(ctx context.Context, projector *events.Projector, logger events.Logger, interval time.Duration) {
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
			if status.StartupComplete || status.Failed {
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

func logSearchIndexingStatus(logger events.Logger, status events.ProjectorStatus, previousMessages uint64, previousDuration time.Duration) {
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

package core

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"sync"
	"time"

	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/projectionsnapshot"
)

// Keep the original lease name so mixed-version replicas coordinate snapshot
// publication during rollout.
const projectionSnapshotLeaseName = "projection-snapshot-threads"

const (
	projectionSnapshotDailyIntervalMin    = 23 * time.Hour
	projectionSnapshotDailyIntervalJitter = time.Hour
	projectionSnapshotExpiryTimeout       = 5 * time.Minute
	projectionSnapshotExpiryMaxDeletes    = 100
	projectionSnapshotExpiryMaxBytes      = 1 << 30
)

type projectionSnapshotJob struct {
	projector      *events.Projector
	repository     *projectionsnapshot.Repository
	projectionKey  string
	compatibility  string
	streamName     string
	streamIdentity string
}

type projectionSnapshotWorker struct {
	jobs          []projectionSnapshotJob
	lease         projectionSnapshotLease
	expirer       projectionSnapshotExpirer
	retention     time.Duration
	logger        events.Logger
	done          chan struct{}
	doneOnce      sync.Once
	wait          func(context.Context, time.Duration) error
	nextInterval  func() time.Duration
	expiryTimeout time.Duration
}

type projectionSnapshotLease interface {
	Run(context.Context, func(context.Context) error) error
	CheckOwnership(context.Context) error
}

type projectionSnapshotExpirer interface {
	Backend() string
	Expire(context.Context, projectionsnapshot.ExpireOptions) (projectionsnapshot.ExpireResult, error)
}

func (w *projectionSnapshotWorker) Run(ctx context.Context, bootDone <-chan struct{}) error {
	defer w.signalFirstPass()
	select {
	case <-bootDone:
	case <-ctx.Done():
		return ctx.Err()
	}
	w.logger.Debug("Projection snapshot worker waiting for lease",
		"projections", len(w.jobs),
		"stage", "lease_acquire")
	err := w.lease.Run(ctx, w.runLeader)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("Projection snapshot worker stopped without publishing all projections",
			"projections", len(w.jobs),
			"stage", "worker",
			"error", err)
	}
	return err
}

func (w *projectionSnapshotWorker) runLeader(ctx context.Context) error {
	wait := w.wait
	if wait == nil {
		wait = waitForProjectionSnapshotPass
	}
	nextInterval := w.nextInterval
	if nextInterval == nil {
		nextInterval = projectionSnapshotDailyInterval
	}
	for {
		started := time.Now()
		if err := w.generate(ctx); err != nil {
			return err
		}
		w.expire(ctx)
		w.signalFirstPass()
		delay := max(time.Until(started.Add(nextInterval())), 0)
		if err := wait(ctx, delay); err != nil {
			return err
		}
	}
}

func (w *projectionSnapshotWorker) signalFirstPass() {
	if w.done != nil {
		w.doneOnce.Do(func() { close(w.done) })
	}
}

func (w *projectionSnapshotWorker) generate(ctx context.Context) error {
	for _, job := range w.jobs {
		if err := w.generateJob(ctx, job); err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			w.logger.Warn("Projection snapshot generation failed",
				"projection", job.projectionKey,
				"backend", job.repository.Backend(),
				"stage", "generate",
				"error", err)
		}
	}
	return nil
}

func (w *projectionSnapshotWorker) expire(ctx context.Context) {
	if w.expirer == nil {
		return
	}
	timeout := w.expiryTimeout
	if timeout <= 0 {
		timeout = projectionSnapshotExpiryTimeout
	}
	expireCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	result, err := w.expirer.Expire(expireCtx, projectionsnapshot.ExpireOptions{
		Retention: w.retention, MaxDeletes: projectionSnapshotExpiryMaxDeletes, MaxDeleteBytes: projectionSnapshotExpiryMaxBytes,
	})
	if err != nil {
		w.logger.Warn("Projection snapshot S3 expiry pass failed",
			"backend", w.expirer.Backend(), "stage", "expire", "error", err,
			"scanned_objects", result.ScannedObjects, "eligible_objects", result.EligibleObjects,
			"deleted_objects", result.DeletedObjects, "deleted_bytes", result.DeletedBytes)
		return
	}
	w.logger.Info("Projection snapshot S3 expiry pass complete",
		"backend", w.expirer.Backend(), "stage", "expire", "error", nil,
		"retention", w.retention, "scanned_objects", result.ScannedObjects,
		"scanned_bytes", result.ScannedBytes, "recent_objects", result.RecentObjects,
		"eligible_objects", result.EligibleObjects, "eligible_bytes", result.EligibleBytes,
		"ignored_objects", result.IgnoredObjects, "deleted_objects", result.DeletedObjects,
		"deleted_bytes", result.DeletedBytes, "delete_limit_hit", result.DeleteLimitHit)
}

func projectionSnapshotDailyInterval() time.Duration {
	return projectionSnapshotDailyIntervalMin + time.Duration(rand.Int64N(int64(projectionSnapshotDailyIntervalJitter)+1))
}

func waitForProjectionSnapshotPass(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (w *projectionSnapshotWorker) generateJob(ctx context.Context, job projectionSnapshotJob) error {
	started := time.Now()
	status := job.projector.Status()
	if !status.StartupComplete {
		return fmt.Errorf("projection startup is not complete")
	}
	if status.LastSeq == 0 {
		w.logger.Debug("Projection snapshot generation skipped for empty EVT stream",
			"projection", job.projectionKey,
			"backend", job.repository.Backend(),
			"stage", "generate_skip")
		return nil
	}
	captured, err := job.projector.CaptureSnapshot()
	if err != nil {
		return fmt.Errorf("capture projection snapshot: %w", err)
	}
	if err := w.lease.CheckOwnership(ctx); err != nil {
		return fmt.Errorf("recheck snapshot lease before publish: %w", err)
	}
	loaded, err := job.repository.Save(ctx, projectionsnapshot.SaveInput{
		ProjectionKey:   job.projectionKey,
		CompatibilityID: job.compatibility,
		StreamName:      job.streamName,
		StreamIdentity:  job.streamIdentity,
		CutoffSequence:  captured.CutoffSequence,
		Payload:         captured.Payload,
	})
	if errors.Is(err, projectionsnapshot.ErrSnapshotRegressed) {
		w.logger.Debug("Projection snapshot generation skipped after a newer publication",
			"projection", job.projectionKey,
			"backend", job.repository.Backend(),
			"stage", "generate_skip",
			"cutoff_seq", captured.CutoffSequence)
		return nil
	}
	if err != nil {
		return err
	}
	w.logger.Info("Projection snapshot generation complete",
		"projection", job.projectionKey,
		"backend", job.repository.Backend(),
		"stage", "generate",
		"generation_id", loaded.GenerationID,
		"cutoff_seq", loaded.CutoffSequence,
		"payload_bytes", len(loaded.Payload),
		"duration", time.Since(started))
	return nil
}

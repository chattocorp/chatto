package core

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/projectionsnapshot"
)

// Keep the original lease name so mixed-version replicas coordinate snapshot
// publication during rollout.
const (
	projectionSnapshotLeaseName       = "projection-snapshot-threads"
	projectionSnapshotExpiryLeaseName = "projection-snapshot-expiry"
)

const (
	projectionSnapshotRefreshAge           = 23 * time.Hour
	projectionSnapshotRefreshCheckInterval = time.Hour
	projectionSnapshotClockSkewTolerance   = 5 * time.Minute
	projectionSnapshotExpiryInterval       = 24 * time.Hour
	projectionSnapshotExpiryTimeout        = 5 * time.Minute
	projectionSnapshotExpiryMaxDeletes     = 100
	projectionSnapshotExpiryMaxBytes       = 1 << 30
)

type projectionSnapshotJob struct {
	projector      *events.Projector
	repository     *projectionsnapshot.Repository
	projectionKey  string
	streamName     string
	streamIdentity string
}

type projectionSnapshotWorker struct {
	jobs          []projectionSnapshotJob
	lease         projectionSnapshotLease
	expiryLease   projectionSnapshotExpiryLease
	expirer       projectionSnapshotExpirer
	retention     time.Duration
	logger        events.Logger
	done          chan struct{}
	doneOnce      sync.Once
	wait          func(context.Context, time.Duration) error
	nextInterval  func() time.Duration
	expiryTimeout time.Duration
	now           func() time.Time
}

type projectionSnapshotLease interface {
	TryRun(context.Context, func(context.Context) error) (bool, error)
	CheckOwnership(context.Context) error
}

type projectionSnapshotExpiryLease interface {
	TryRunWithCooldown(context.Context, func(context.Context) error) (bool, error)
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
	wait := w.wait
	if wait == nil {
		wait = waitForProjectionSnapshotPass
	}
	nextInterval := w.nextInterval
	if nextInterval == nil {
		nextInterval = func() time.Duration { return projectionSnapshotRefreshCheckInterval }
	}
	now := w.now
	if now == nil {
		now = time.Now
	}
	bootPass := true
	for {
		started := now()
		acquired, err := w.lease.TryRun(ctx, func(leaderCtx context.Context) error {
			if err := w.generate(leaderCtx, bootPass); err != nil {
				return err
			}
			w.expireIfDue(leaderCtx)
			return nil
		})
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return err
			}
			w.logger.Warn("Projection snapshot pass failed",
				"projections", len(w.jobs),
				"stage", "worker",
				"error", err)
		}
		if acquired && err == nil {
			bootPass = false
		}
		w.signalFirstPass()
		delay := max(started.Add(nextInterval()).Sub(now()), 0)
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

func (w *projectionSnapshotWorker) generate(ctx context.Context, publishReplayDelta bool) error {
	for _, job := range w.jobs {
		if err := w.generateJob(ctx, job, publishReplayDelta); err != nil {
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

func (w *projectionSnapshotWorker) expireIfDue(ctx context.Context) {
	if w.expirer == nil || w.expiryLease == nil {
		return
	}
	acquired, err := w.expiryLease.TryRunWithCooldown(ctx, w.expire)
	if err != nil && !acquired && !errors.Is(err, context.Canceled) {
		w.logger.Warn("Projection snapshot S3 expiry claim failed",
			"backend", w.expirer.Backend(), "stage", "expire_claim", "error", err)
	}
}

func (w *projectionSnapshotWorker) expire(ctx context.Context) error {
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
		return err
	}
	w.logger.Info("Projection snapshot S3 expiry pass complete",
		"backend", w.expirer.Backend(), "stage", "expire", "error", nil,
		"retention", w.retention, "scanned_objects", result.ScannedObjects,
		"scanned_bytes", result.ScannedBytes, "recent_objects", result.RecentObjects,
		"eligible_objects", result.EligibleObjects, "eligible_bytes", result.EligibleBytes,
		"ignored_objects", result.IgnoredObjects, "deleted_objects", result.DeletedObjects,
		"deleted_bytes", result.DeletedBytes, "delete_limit_hit", result.DeleteLimitHit)
	return nil
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

func (w *projectionSnapshotWorker) generateJob(ctx context.Context, job projectionSnapshotJob, publishReplayDelta bool) error {
	now := w.now
	if now == nil {
		now = time.Now
	}
	started := now()
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
	if !projectionSnapshotRefreshDue(status, started, publishReplayDelta) {
		w.logger.Debug("Projection snapshot generation skipped for fresh restored state",
			"projection", job.projectionKey,
			"backend", job.repository.Backend(),
			"stage", "generate_skip",
			"cutoff_seq", status.LatestSnapshotSeq,
			"snapshot_age", max(started.Sub(status.LatestSnapshotAt), 0),
			"refresh_age", projectionSnapshotRefreshAge)
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
		ProjectionKey:  job.projectionKey,
		ContractID:     job.projector.SnapshotContractID(),
		StreamName:     job.streamName,
		StreamIdentity: job.streamIdentity,
		CutoffSequence: captured.CutoffSequence,
		Payload:        captured.Payload,
		RefreshAge:     projectionSnapshotRefreshAge,
		ClockSkew:      projectionSnapshotClockSkewTolerance,
	})
	if errors.Is(err, projectionsnapshot.ErrSnapshotFresh) {
		job.projector.RecordSnapshotPublication(loaded.CutoffSequence, loaded.CreatedAt)
		w.logger.Debug("Projection snapshot generation skipped after a fresh publication",
			"projection", job.projectionKey,
			"backend", job.repository.Backend(),
			"stage", "generate_skip",
			"cutoff_seq", loaded.CutoffSequence)
		return nil
	}
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
	job.projector.RecordSnapshotPublication(loaded.CutoffSequence, loaded.CreatedAt)
	w.logger.Info("Projection snapshot generation complete",
		"projection", job.projectionKey,
		"backend", job.repository.Backend(),
		"stage", "generate",
		"generation_id", loaded.GenerationID,
		"cutoff_seq", loaded.CutoffSequence,
		"payload_bytes", len(loaded.Payload),
		"duration", now().Sub(started))
	return nil
}

func projectionSnapshotRefreshDue(status events.ProjectorStatus, now time.Time, publishReplayDelta bool) bool {
	if status.LatestSnapshotAt.IsZero() {
		return true
	}
	if status.LatestSnapshotAt.After(now.Add(projectionSnapshotClockSkewTolerance)) {
		return true
	}
	if !status.LatestSnapshotAt.After(now.Add(-projectionSnapshotRefreshAge)) {
		return true
	}
	return publishReplayDelta && status.LastSeq != status.LatestSnapshotSeq
}

package core

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/projectionsnapshot"
)

const (
	projectionSnapshotCleanupLeaseName       = "projection-snapshot-cleanup"
	projectionSnapshotCleanupInitialMin      = 5 * time.Minute
	projectionSnapshotCleanupInitialJitter   = 5 * time.Minute
	projectionSnapshotCleanupInterval        = 6 * time.Hour
	projectionSnapshotCleanupFailureInterval = 30 * time.Minute
	projectionSnapshotCleanupTimeout         = 5 * time.Minute
	projectionSnapshotCleanupGracePeriod     = 24 * time.Hour
	projectionSnapshotCleanupMaxDeletes      = 100
	projectionSnapshotCleanupMaxDeleteBytes  = 1 << 30
)

type projectionSnapshotSweeper interface {
	Backend() string
	Sweep(context.Context, projectionsnapshot.SweepOptions) (projectionsnapshot.SweepResult, error)
}

type projectionSnapshotCleanupLease interface {
	Run(context.Context, func(context.Context) error) error
	CheckOwnership(context.Context) error
}

type projectionSnapshotCleanupWorker struct {
	repository     projectionSnapshotSweeper
	lease          projectionSnapshotCleanupLease
	projectionKeys []string
	logger         events.Logger
	initialDelay   func() time.Duration
	wait           func(context.Context, time.Duration) error
	sweepTimeout   time.Duration
	done           chan struct{}
}

func (w *projectionSnapshotCleanupWorker) Run(ctx context.Context, bootDone <-chan struct{}) error {
	if w.done != nil {
		defer close(w.done)
	}
	select {
	case <-bootDone:
	case <-ctx.Done():
		return ctx.Err()
	}
	w.logger.Debug("Projection snapshot cleanup worker waiting for lease",
		"backend", w.repository.Backend(),
		"stage", "cleanup_lease_acquire")
	err := w.lease.Run(ctx, w.runLeader)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("Projection snapshot cleanup worker stopped",
			"backend", w.repository.Backend(),
			"stage", "cleanup_worker",
			"error", err)
	}
	return err
}

func (w *projectionSnapshotCleanupWorker) runLeader(ctx context.Context) error {
	wait := w.wait
	if wait == nil {
		wait = waitForProjectionSnapshotCleanup
	}
	initialDelay := projectionSnapshotCleanupInitialDelay
	if w.initialDelay != nil {
		initialDelay = w.initialDelay
	}
	if err := wait(ctx, initialDelay()); err != nil {
		return err
	}

	for {
		result, err := w.sweep(ctx)
		delay := projectionSnapshotCleanupInterval
		if err != nil {
			delay = projectionSnapshotCleanupFailureInterval
			w.logger.Warn("Projection snapshot cleanup pass failed",
				"backend", w.repository.Backend(),
				"stage", "cleanup",
				"error", err,
				"scanned_objects", result.ScannedObjects,
				"eligible_objects", result.EligibleObjects,
				"deleted_objects", result.DeletedObjects,
				"deleted_bytes", result.DeletedBytes)
		} else {
			if result.DeleteLimitHit {
				delay = projectionSnapshotCleanupFailureInterval
			}
			w.logger.Info("Projection snapshot cleanup pass complete",
				"backend", w.repository.Backend(),
				"stage", "cleanup",
				"error", nil,
				"scanned_objects", result.ScannedObjects,
				"scanned_bytes", result.ScannedBytes,
				"referenced_objects", result.ReferencedObjects,
				"active_pointers", result.ActivePointers,
				"recent_objects", result.RecentObjects,
				"eligible_objects", result.EligibleObjects,
				"eligible_bytes", result.EligibleBytes,
				"ignored_objects", result.IgnoredObjects,
				"deleted_objects", result.DeletedObjects,
				"deleted_bytes", result.DeletedBytes,
				"delete_limit_hit", result.DeleteLimitHit)
		}
		if err := wait(ctx, delay); err != nil {
			return err
		}
	}
}

func (w *projectionSnapshotCleanupWorker) sweep(ctx context.Context) (projectionsnapshot.SweepResult, error) {
	timeout := w.sweepTimeout
	if timeout == 0 {
		timeout = projectionSnapshotCleanupTimeout
	}
	sweepCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return w.repository.Sweep(sweepCtx, projectionsnapshot.SweepOptions{
		ProjectionKeys: w.projectionKeys,
		GracePeriod:    projectionSnapshotCleanupGracePeriod,
		MaxDeletes:     projectionSnapshotCleanupMaxDeletes,
		MaxDeleteBytes: projectionSnapshotCleanupMaxDeleteBytes,
		BeforeDelete:   w.lease.CheckOwnership,
	})
}

func projectionSnapshotCleanupInitialDelay() time.Duration {
	return projectionSnapshotCleanupInitialMin + time.Duration(rand.Int64N(int64(projectionSnapshotCleanupInitialJitter)+1))
}

func waitForProjectionSnapshotCleanup(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

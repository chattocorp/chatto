package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/lease"
	"hmans.de/chatto/internal/projectionsnapshot"
)

// Keep the original lease name so mixed-version replicas coordinate Thread
// publications while the newer leader also publishes the v2 cohort.
const projectionSnapshotLeaseName = "projection-snapshot-threads"

type projectionSnapshotJob struct {
	projector      *events.Projector
	repository     *projectionsnapshot.Repository
	projectionKey  string
	compatibility  string
	streamName     string
	streamIdentity string
}

type projectionSnapshotWorker struct {
	jobs   []projectionSnapshotJob
	lease  *lease.Lease
	logger events.Logger
	done   chan struct{}
}

func (w *projectionSnapshotWorker) Run(ctx context.Context, bootDone <-chan struct{}) error {
	if w.done != nil {
		defer close(w.done)
	}
	select {
	case <-bootDone:
	case <-ctx.Done():
		return ctx.Err()
	}
	w.logger.Debug("Projection snapshot worker waiting for lease",
		"projections", len(w.jobs),
		"stage", "lease_acquire")
	err := w.lease.Run(ctx, w.generate)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("Projection snapshot worker stopped without publishing all projections",
			"projections", len(w.jobs),
			"stage", "worker",
			"error", err)
	}
	return err
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
	current, err := job.repository.Load(ctx, job.projectionKey, job.compatibility, job.streamName, job.streamIdentity, status.LastSeq)
	if err == nil && current.CutoffSequence >= status.LastSeq {
		w.logger.Debug("Projection snapshot already current",
			"projection", job.projectionKey,
			"backend", job.repository.Backend(),
			"stage", "generate_skip",
			"generation_id", current.GenerationID,
			"cutoff_seq", current.CutoffSequence)
		return nil
	}
	if err != nil && !errors.Is(err, projectionsnapshot.ErrSnapshotNotFound) {
		w.logger.Warn("Projection snapshot current generation could not be checked; rebuilding",
			"projection", job.projectionKey,
			"backend", job.repository.Backend(),
			"stage", "generate_check",
			"error", err)
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
	if errors.Is(err, projectionsnapshot.ErrSnapshotNotAdvanced) {
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

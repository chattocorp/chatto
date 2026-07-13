package core

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/lease"
	"hmans.de/chatto/internal/projectionsnapshot"
)

const projectionSnapshotLeaseName = "projection-snapshot-threads"

type projectionSnapshotWorker struct {
	projector     *events.Projector
	repository    *projectionsnapshot.Repository
	lease         *lease.Lease
	projectionKey string
	compatibility string
	stream        jetstream.Stream
	streamName    string
	logger        events.Logger
}

func (w *projectionSnapshotWorker) Run(ctx context.Context, bootDone <-chan struct{}) error {
	select {
	case <-bootDone:
	case <-ctx.Done():
		return ctx.Err()
	}
	w.logger.Debug("Projection snapshot worker waiting for lease",
		"projection", w.projectionKey,
		"backend", w.repository.Backend(),
		"stage", "lease_acquire")
	err := w.lease.Run(ctx, w.generate)
	if err != nil && !errors.Is(err, context.Canceled) {
		w.logger.Warn("Projection snapshot worker stopped without publishing",
			"projection", w.projectionKey,
			"backend", w.repository.Backend(),
			"stage", "worker",
			"error", err)
	}
	return err
}

func (w *projectionSnapshotWorker) generate(ctx context.Context) error {
	started := time.Now()
	status := w.projector.Status()
	if !status.StartupComplete {
		return fmt.Errorf("projection startup is not complete")
	}
	if status.LastSeq == 0 {
		w.logger.Debug("Projection snapshot generation skipped for empty EVT stream",
			"projection", w.projectionKey,
			"backend", w.repository.Backend(),
			"stage", "generate_skip")
		return nil
	}
	current, err := w.repository.Load(ctx, w.projectionKey, w.compatibility, w.streamName, status.LastSeq)
	if err == nil {
		identity, identityErr := events.StreamPositionIdentity(ctx, w.stream, current.CutoffSequence)
		switch {
		case identityErr != nil:
			err = fmt.Errorf("validate current snapshot EVT cutoff identity: %w", identityErr)
		case current.StreamIdentity != identity:
			err = fmt.Errorf("validate current snapshot EVT cutoff identity: history changed")
		}
	}
	if err == nil && current.CutoffSequence >= status.LastSeq {
		w.logger.Debug("Projection snapshot already current",
			"projection", w.projectionKey,
			"backend", w.repository.Backend(),
			"stage", "generate_skip",
			"generation_id", current.GenerationID,
			"cutoff_seq", current.CutoffSequence)
		return nil
	}
	if err != nil && !errors.Is(err, projectionsnapshot.ErrSnapshotNotFound) {
		w.logger.Warn("Projection snapshot current generation could not be checked; rebuilding",
			"projection", w.projectionKey,
			"backend", w.repository.Backend(),
			"stage", "generate_check",
			"error", err)
	}

	captured, err := w.projector.CaptureSnapshot()
	if err != nil {
		return fmt.Errorf("capture projection snapshot: %w", err)
	}
	if len(captured.Payload) == 0 {
		return fmt.Errorf("projection returned an empty snapshot")
	}
	streamIdentity, err := events.StreamPositionIdentity(ctx, w.stream, captured.CutoffSequence)
	if err != nil {
		return fmt.Errorf("fingerprint projection snapshot cutoff: %w", err)
	}
	if err := w.lease.CheckOwnership(ctx); err != nil {
		return fmt.Errorf("recheck snapshot lease before publish: %w", err)
	}
	loaded, err := w.repository.Save(ctx, projectionsnapshot.SaveInput{
		ProjectionKey:   w.projectionKey,
		CompatibilityID: w.compatibility,
		StreamName:      w.streamName,
		StreamIdentity:  streamIdentity,
		CutoffSequence:  captured.CutoffSequence,
		Payload:         captured.Payload,
	})
	if err != nil {
		return err
	}
	w.logger.Info("Projection snapshot generation complete",
		"projection", w.projectionKey,
		"backend", w.repository.Backend(),
		"stage", "generate",
		"generation_id", loaded.GenerationID,
		"cutoff_seq", loaded.CutoffSequence,
		"payload_bytes", len(loaded.Payload),
		"duration", time.Since(started))
	return nil
}

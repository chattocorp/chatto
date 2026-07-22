package events

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// ErrProjectionCheckpointInvalid asks the projector to discard a local
// checkpoint and rebuild the projection from retained EVT history. Other
// errors are treated as operational failures so transient storage trouble does
// not destroy a potentially valid index.
var ErrProjectionCheckpointInvalid = errors.New("projection checkpoint is invalid")

// ProjectionCheckpointRequest binds local derived state to one projection
// contract and one EVT stream incarnation.
type ProjectionCheckpointRequest struct {
	ProjectionKey  string
	ContractID     string
	StreamName     string
	StreamIdentity string
	FirstSequence  uint64
	LastSequence   uint64
}

// ProjectionCheckpoint identifies the highest EVT stream sequence atomically
// represented by a checkpointed projection's local state.
type ProjectionCheckpoint struct {
	CutoffSequence uint64
}

// CheckpointedProjection owns disposable local derived state. A successful
// Apply must atomically commit both its materialized changes and the supplied
// stream sequence before returning.
type CheckpointedProjection interface {
	Projection
	CheckpointContractID() string
	RestoreCheckpoint(context.Context, ProjectionCheckpointRequest) (ProjectionCheckpoint, error)
	ResetCheckpoint(context.Context, ProjectionCheckpointRequest) error
}

// ConfigureCheckpoint enables projection-owned local checkpoint restore. It
// must be called before Run and cannot be combined with ADR-050 snapshots.
func (p *Projector) ConfigureCheckpoint(key string) error {
	if key == "" {
		return fmt.Errorf("projection checkpoint key is required")
	}
	projection, ok := p.proj.(CheckpointedProjection)
	if !ok {
		return fmt.Errorf("projection %q does not support local checkpoints", key)
	}
	contractID := projection.CheckpointContractID()
	if contractID == "" {
		return fmt.Errorf("projection %q does not declare a checkpoint contract", key)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return fmt.Errorf("configure projection checkpoint after projector start")
	}
	if p.snapshotSource != nil {
		return fmt.Errorf("projection %q already uses snapshot restore", key)
	}
	if p.checkpointKey != "" {
		return fmt.Errorf("projection %q checkpoint is already configured", key)
	}
	p.checkpointKey = key
	p.checkpointContractID = contractID
	return nil
}

func (p *Projector) restoreCheckpointForRun(ctx context.Context, targetSeq uint64) error {
	p.mu.Lock()
	key := p.checkpointKey
	contractID := p.checkpointContractID
	p.mu.Unlock()
	if key == "" {
		return fmt.Errorf("projection checkpoint is not configured")
	}
	projection, ok := p.proj.(CheckpointedProjection)
	if !ok {
		return fmt.Errorf("projection %q no longer supports local checkpoints", key)
	}

	info, err := p.stream.Info(ctx)
	if err != nil {
		return fmt.Errorf("read EVT stream info for projection checkpoint: %w", err)
	}
	identity := info.Config.Metadata[EVTStreamIdentityMetadataKey]
	if !ValidStreamIdentity(identity) {
		return fmt.Errorf("projection checkpoint EVT stream identity is missing or invalid")
	}
	request := ProjectionCheckpointRequest{
		ProjectionKey:  key,
		ContractID:     contractID,
		StreamName:     info.Config.Name,
		StreamIdentity: identity,
		FirstSequence:  info.State.FirstSeq,
		LastSequence:   info.State.LastSeq,
	}

	checkpoint, restoreErr := projection.RestoreCheckpoint(ctx, request)
	invalidReason := restoreErr
	if restoreErr == nil && checkpoint.CutoffSequence > request.LastSequence {
		invalidReason = fmt.Errorf("%w: cutoff %d is newer than EVT tail %d", ErrProjectionCheckpointInvalid, checkpoint.CutoffSequence, request.LastSequence)
	}
	if restoreErr == nil && checkpoint.CutoffSequence > 0 && request.FirstSequence > checkpoint.CutoffSequence+1 {
		invalidReason = fmt.Errorf("%w: cutoff %d is behind retained EVT start %d", ErrProjectionCheckpointInvalid, checkpoint.CutoffSequence, request.FirstSequence)
	}
	if invalidReason != nil {
		if !errors.Is(invalidReason, ErrProjectionCheckpointInvalid) {
			return fmt.Errorf("restore projection checkpoint %q: %w", key, invalidReason)
		}
		p.logger.Info("Projection checkpoint invalid; replaying EVT",
			"projection", key,
			"stage", "checkpoint_restore",
			"error", invalidReason)
		if err := projection.ResetCheckpoint(ctx, request); err != nil {
			return errors.Join(invalidReason, fmt.Errorf("reset projection checkpoint %q: %w", key, err))
		}
		p.resetRestoreState()
		return nil
	}

	p.mu.Lock()
	p.restoredSeq = checkpoint.CutoffSequence
	p.checkpointRestored = checkpoint.CutoffSequence > 0
	p.checkpointCutoffSeq = checkpoint.CutoffSequence
	p.mu.Unlock()
	if checkpoint.CutoffSequence > 0 {
		p.advance(checkpoint.CutoffSequence)
	}
	p.logger.Info("Projection checkpoint restored",
		"projection", key,
		"stage", "checkpoint_restore",
		"cutoff_seq", checkpoint.CutoffSequence,
		"target_seq", targetSeq)
	return nil
}

func (p *Projector) resetRestoreState() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.lastSeq = 0
	p.restoredSeq = 0
	p.restoredGenerationID = ""
	p.snapshotRestored = false
	p.latestSnapshotSeq = 0
	p.latestSnapshotAt = time.Time{}
	p.checkpointRestored = false
	p.checkpointCutoffSeq = 0
}

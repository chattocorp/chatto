package events

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// SnapshotProjection supports serializing and restoring projection state for
// one decoded event type.
// Snapshot persistence is optional and configured separately on Projector.
type SnapshotProjection[E any] interface {
	EventProjection[E]

	// Snapshot returns a canonical serialized form of the current state. Every
	// successful payload, including nil or empty, may be persisted with the
	// current stream cutoff and must therefore restore the complete state at
	// that cutoff. Return an error when no valid snapshot can be produced.
	Snapshot() ([]byte, error)

	// Restore initializes state from a snapshot. Called once before Run starts
	// consuming. It may receive nil or empty for either a cold start or a
	// canonical empty snapshot, which must produce the same valid state.
	// Implementations must leave their prior state unchanged when returning
	// an error so the Projector can reliably fall back to cold replay.
	Restore(snapshot []byte) error
}

// SnapshotContractProjection opts a projection into persisted snapshots.
// The contract ID covers every projection-specific input that determines
// whether restoring a snapshot is equivalent to replaying the event log through its
// cutoff. Changing unrelated application versions must not invalidate it.
type SnapshotContractProjection[E any] interface {
	SnapshotProjection[E]
	SnapshotContractID() string
}

type snapshotProjectionState interface {
	Snapshot() ([]byte, error)
	Restore([]byte) error
}

type snapshotContractProjectionState interface {
	snapshotProjectionState
	SnapshotContractID() string
}

// ProjectionSnapshot is projection state restored from a source or captured
// for publication. Restored snapshots must carry the contract, stream name,
// and stream identity that were validated by the source; the Projector checks
// those bindings again before applying the payload.
type ProjectionSnapshot struct {
	GenerationID   string
	ContractID     string
	StreamName     string
	CutoffSequence uint64
	StreamIdentity string
	CreatedAt      time.Time
	Payload        []byte
}

// ProjectionSnapshotLoadRequest contains the repository lookup constraints
// owned by the Projector. Sources must reject mismatched or newer stream state
// before returning a snapshot.
type ProjectionSnapshotLoadRequest struct {
	ProjectionKey  string
	ContractID     string
	StreamName     string
	StreamIdentity string
	MaxCutoff      uint64
}

// ProjectionSnapshotSource loads disposable projection state for a specific
// projection contract and stream incarnation. Implementations must enforce
// every constraint in the request and return an error when no valid snapshot is
// available; the Projector then falls back to replaying retained stream events.
type ProjectionSnapshotSource interface {
	// LoadProjectionSnapshot returns a snapshot whose contract ID and stream
	// identity match the request and whose cutoff does not exceed MaxCutoff.
	LoadProjectionSnapshot(context.Context, ProjectionSnapshotLoadRequest) (ProjectionSnapshot, error)
}

// StreamIdentityResolver resolves an application's opaque stream incarnation
// from supplied stream information. Restore invokes it with the same fresh
// StreamInfo used to validate persisted projection state.
type StreamIdentityResolver func(*jetstream.StreamInfo) (string, error)

func resolveProjectionStreamIdentity(info *jetstream.StreamInfo, resolve StreamIdentityResolver) (string, error) {
	if resolve == nil {
		return "", fmt.Errorf("stream identity resolver is not configured")
	}
	identity, err := resolve(info)
	if err != nil {
		return "", err
	}
	if identity == "" {
		return "", fmt.Errorf("stream identity is empty")
	}
	return identity, nil
}

// ProjectionSnapshotPart is one stable, independently stored part of a
// projection component snapshot.
type ProjectionSnapshotPart struct {
	Key     string
	Payload []byte
}

// ProjectionSnapshotComponent is one independently serialized component in a
// projection snapshot cohort. Part keys are stable within its contract.
type ProjectionSnapshotComponent struct {
	Key        string
	ContractID string
	Parts      []ProjectionSnapshotPart
}

// ProjectionSnapshotComponentContract identifies one required component and
// bounds the number of payload parts that a snapshot source can load.
type ProjectionSnapshotComponentContract struct {
	Key        string
	ContractID string
	MaxParts   int
}

// ProjectionSnapshotCohort is component state captured or restored at one
// event-log cutoff. A cohort is installed as one unit.
type ProjectionSnapshotCohort struct {
	GenerationID   string
	ContractID     string
	StreamName     string
	CutoffSequence uint64
	StreamIdentity string
	CreatedAt      time.Time
	Components     []ProjectionSnapshotComponent
}

// ProjectionSnapshotCohortLoadRequest contains the repository constraints for
// one projection snapshot cohort.
type ProjectionSnapshotCohortLoadRequest struct {
	ProjectionKey  string
	ContractID     string
	StreamName     string
	StreamIdentity string
	MaxCutoff      uint64
	Components     []ProjectionSnapshotComponentContract
}

// ProjectionSnapshotCohortSource loads one complete projection snapshot
// cohort. It must not return a partial generation.
type ProjectionSnapshotCohortSource interface {
	LoadProjectionSnapshotCohort(context.Context, ProjectionSnapshotCohortLoadRequest) (ProjectionSnapshotCohort, error)
}

type snapshotCohortProjectionState interface {
	SnapshotComponents() ([]ProjectionSnapshotComponent, error)
	RestoreComponents([]ProjectionSnapshotComponent) error
	ResetComponents() error
	SnapshotCohortContractID() string
	SnapshotComponentContracts() []ProjectionSnapshotComponentContract
}

// ConfigureSnapshots enables best-effort bootstrap restore for this projector.
// The identity resolver receives the same fresh stream information used by the
// restore request. It must be called before Run. A load or restore failure is
// logged and falls back to an empty projection followed by full event replay.
func (p *Projector) ConfigureSnapshots(key string, source ProjectionSnapshotSource, resolveStreamIdentity StreamIdentityResolver) error {
	if key == "" {
		return fmt.Errorf("projection snapshot key is required")
	}
	if source == nil {
		return fmt.Errorf("projection snapshot source is nil")
	}
	if resolveStreamIdentity == nil {
		return fmt.Errorf("projection snapshot stream identity resolver is required")
	}
	configuredStreamIdentity, err := resolveProjectionStreamIdentity(p.stream.CachedInfo(), resolveStreamIdentity)
	if err != nil {
		return fmt.Errorf("resolve projection snapshot stream identity: %w", err)
	}
	contractProjection, ok := p.proj.(snapshotContractProjectionState)
	if !ok {
		return fmt.Errorf("projection %q does not declare a snapshot contract", key)
	}
	contractID := contractProjection.SnapshotContractID()
	if contractID == "" {
		return fmt.Errorf("projection %q does not declare a snapshot contract", key)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return fmt.Errorf("configure projection snapshots after projector start")
	}
	if p.checkpointKey != "" {
		return fmt.Errorf("projection %q already uses a local checkpoint", key)
	}
	if p.snapshotCohortSource != nil {
		return fmt.Errorf("projection %q already uses snapshot cohorts", key)
	}
	p.snapshotKey = key
	p.snapshotContractID = contractID
	p.snapshotSource = source
	p.snapshotIdentityResolver = resolveStreamIdentity
	p.snapshotConfiguredID = configuredStreamIdentity
	p.snapshotLoadTimeout = projectionSnapshotLoadTimeout
	return nil
}

// ConfigureSnapshotCohorts enables best-effort bootstrap restore for a
// componentized projection. The source must return one complete cohort whose
// parts share the requested event-log cutoff. A load or restore failure falls
// back to a complete cold replay.
func (p *Projector) ConfigureSnapshotCohorts(key string, source ProjectionSnapshotCohortSource, resolveStreamIdentity StreamIdentityResolver) error {
	if key == "" {
		return fmt.Errorf("projection snapshot key is required")
	}
	if source == nil {
		return fmt.Errorf("projection snapshot cohort source is nil")
	}
	if resolveStreamIdentity == nil {
		return fmt.Errorf("projection snapshot stream identity resolver is required")
	}
	configuredStreamIdentity, err := resolveProjectionStreamIdentity(p.stream.CachedInfo(), resolveStreamIdentity)
	if err != nil {
		return fmt.Errorf("resolve projection snapshot stream identity: %w", err)
	}
	projection, ok := p.proj.(snapshotCohortProjectionState)
	if !ok {
		return fmt.Errorf("projection %q does not declare snapshot cohorts", key)
	}
	contractID := projection.SnapshotCohortContractID()
	if contractID == "" {
		return fmt.Errorf("projection %q does not declare a snapshot cohort contract", key)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return fmt.Errorf("configure projection snapshots after projector start")
	}
	if p.checkpointKey != "" {
		return fmt.Errorf("projection %q already uses a local checkpoint", key)
	}
	if p.snapshotSource != nil {
		return fmt.Errorf("projection %q already uses single-payload snapshots", key)
	}
	p.snapshotKey = key
	p.snapshotCohortContractID = contractID
	p.snapshotCohortSource = source
	p.snapshotIdentityResolver = resolveStreamIdentity
	p.snapshotConfiguredID = configuredStreamIdentity
	p.snapshotLoadTimeout = projectionSnapshotLoadTimeout
	return nil
}

// SnapshotContractID returns the contract captured when snapshots were
// configured. Restore and publication must use this single value.
func (p *Projector) SnapshotContractID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.snapshotCohortContractID != "" {
		return p.snapshotCohortContractID
	}
	return p.snapshotContractID
}

// CaptureSnapshotCohort serializes every registered component and the common
// applied sequence under one apply barrier.
func (p *Projector) CaptureSnapshotCohort(ctx context.Context) (ProjectionSnapshotCohort, error) {
	p.mu.Lock()
	resolveStreamIdentity := p.snapshotIdentityResolver
	streamIdentity := p.snapshotRunStreamIdentity
	p.mu.Unlock()
	if resolveStreamIdentity != nil {
		currentIdentity, err := p.resolveCurrentStreamIdentity(ctx, resolveStreamIdentity)
		if err != nil {
			return ProjectionSnapshotCohort{}, fmt.Errorf("resolve stream identity before snapshot capture: %w", err)
		}
		if streamIdentity == "" || currentIdentity != streamIdentity {
			return ProjectionSnapshotCohort{}, fmt.Errorf("stream identity changed during projector run")
		}
	}

	var components []ProjectionSnapshotComponent
	var seq uint64
	p.applyMu.Lock()
	projection, ok := p.proj.(snapshotCohortProjectionState)
	if !ok {
		p.applyMu.Unlock()
		return ProjectionSnapshotCohort{}, fmt.Errorf("projection does not support snapshot cohorts")
	}
	var err error
	components, err = projection.SnapshotComponents()
	if err == nil {
		p.mu.Lock()
		seq = p.lastSeq
		p.mu.Unlock()
	}
	p.applyMu.Unlock()
	if err != nil {
		return ProjectionSnapshotCohort{}, err
	}

	if resolveStreamIdentity != nil {
		currentIdentity, err := p.resolveCurrentStreamIdentity(ctx, resolveStreamIdentity)
		if err != nil {
			return ProjectionSnapshotCohort{}, fmt.Errorf("resolve stream identity after snapshot capture: %w", err)
		}
		if currentIdentity != streamIdentity {
			return ProjectionSnapshotCohort{}, fmt.Errorf("stream identity changed during projector run")
		}
	}
	p.mu.Lock()
	contractID := p.snapshotCohortContractID
	p.mu.Unlock()
	return ProjectionSnapshotCohort{
		ContractID: contractID, StreamName: p.stream.CachedInfo().Config.Name,
		CutoffSequence: seq, StreamIdentity: streamIdentity, Components: components,
	}, nil
}

// CaptureSnapshot serializes projection state, the corresponding applied event
// sequence, and the stream identity bound to this run at one barrier. An empty
// payload is valid canonical state and still carries the projection's replay
// cutoff.
func (p *Projector) CaptureSnapshot(ctx context.Context) (ProjectionSnapshot, error) {
	p.mu.Lock()
	resolveStreamIdentity := p.snapshotIdentityResolver
	streamIdentity := p.snapshotRunStreamIdentity
	p.mu.Unlock()
	if resolveStreamIdentity != nil {
		currentIdentity, err := p.resolveCurrentStreamIdentity(ctx, resolveStreamIdentity)
		if err != nil {
			return ProjectionSnapshot{}, fmt.Errorf("resolve stream identity before snapshot capture: %w", err)
		}
		if streamIdentity == "" || currentIdentity != streamIdentity {
			return ProjectionSnapshot{}, fmt.Errorf("stream identity changed during projector run")
		}
	}

	payload, seq, err := func() ([]byte, uint64, error) {
		p.applyMu.Lock()
		defer p.applyMu.Unlock()
		projection, ok := p.proj.(snapshotProjectionState)
		if !ok {
			return nil, 0, fmt.Errorf("projection does not support snapshots")
		}
		payload, err := projection.Snapshot()
		if err != nil {
			return nil, 0, err
		}
		p.mu.Lock()
		seq := p.lastSeq
		p.mu.Unlock()
		return payload, seq, nil
	}()
	if err != nil {
		return ProjectionSnapshot{}, err
	}

	if resolveStreamIdentity != nil {
		currentIdentity, err := p.resolveCurrentStreamIdentity(ctx, resolveStreamIdentity)
		if err != nil {
			return ProjectionSnapshot{}, fmt.Errorf("resolve stream identity after snapshot capture: %w", err)
		}
		if currentIdentity != streamIdentity {
			return ProjectionSnapshot{}, fmt.Errorf("stream identity changed during projector run")
		}
	}
	p.mu.Lock()
	contractID := p.snapshotContractID
	p.mu.Unlock()
	streamName := p.stream.CachedInfo().Config.Name
	return ProjectionSnapshot{
		ContractID:     contractID,
		StreamName:     streamName,
		CutoffSequence: seq,
		StreamIdentity: streamIdentity,
		Payload:        payload,
	}, nil
}

func (p *Projector) resolveCurrentStreamIdentity(ctx context.Context, resolve StreamIdentityResolver) (string, error) {
	info, err := p.freshStreamInfo(ctx)
	if err != nil {
		return "", err
	}
	return resolveProjectionStreamIdentity(info, resolve)
}

// freshStreamInfo reads current stream metadata through a short-lived handle.
// The NATS client mutates a stream handle's cached info during Info. Projector
// reads can run concurrently, so they must not share that mutation.
func (p *Projector) freshStreamInfo(ctx context.Context) (*jetstream.StreamInfo, error) {
	streamName := p.stream.CachedInfo().Config.Name
	stream, err := p.js.Stream(ctx, streamName)
	if err != nil {
		return nil, err
	}
	return stream.CachedInfo(), nil
}

// RecordSnapshotPublication updates the latest persisted generation metadata
// used by the snapshot worker's refresh policy. Publication remains guarded by
// the repository's cross-replica OCC checks.
func (p *Projector) RecordSnapshotPublication(cutoff uint64, createdAt time.Time) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.latestSnapshotSeq = cutoff
	p.latestSnapshotAt = createdAt
}

func (p *Projector) restoreForRun(ctx context.Context, targetSeq uint64) error {
	coldRestore := func() error {
		if projection, ok := p.proj.(snapshotCohortProjectionState); ok {
			if err := projection.ResetComponents(); err != nil {
				return fmt.Errorf("restore empty projection components: %w", err)
			}
			p.resetRestoreState()
			return nil
		}
		if projection, ok := p.proj.(snapshotProjectionState); ok {
			if err := projection.Restore(nil); err != nil {
				return fmt.Errorf("restore empty projection: %w", err)
			}
		}
		p.resetRestoreState()
		return nil
	}

	p.mu.Lock()
	source := p.snapshotSource
	cohortSource := p.snapshotCohortSource
	checkpointKey := p.checkpointKey
	key := p.snapshotKey
	contractID := p.snapshotContractID
	resolveStreamIdentity := p.snapshotIdentityResolver
	configuredStreamIdentity := p.snapshotConfiguredID
	loadTimeout := p.snapshotLoadTimeout
	p.mu.Unlock()
	if checkpointKey != "" {
		return p.restoreCheckpointForRun(ctx, targetSeq)
	}
	if cohortSource != nil {
		return p.restoreSnapshotCohortForRun(ctx, targetSeq, coldRestore)
	}
	if source == nil {
		return coldRestore()
	}
	if loadTimeout <= 0 {
		loadTimeout = projectionSnapshotLoadTimeout
	}
	loadCtx, cancelLoad := context.WithTimeout(ctx, loadTimeout)
	defer cancelLoad()
	info, err := p.freshStreamInfo(loadCtx)
	if err != nil {
		p.mu.Lock()
		p.snapshotRunStreamIdentity = configuredStreamIdentity
		p.mu.Unlock()
		p.logger.Info("Projection snapshot stream info unavailable; replaying event log",
			"projection", key,
			"stage", "restore_stream_info",
			"error", err)
		return coldRestore()
	}
	streamIdentity, err := resolveProjectionStreamIdentity(info, resolveStreamIdentity)
	if err != nil {
		p.mu.Lock()
		p.snapshotRunStreamIdentity = configuredStreamIdentity
		p.mu.Unlock()
		p.logger.Info("Projection snapshot stream identity unavailable; replaying event log",
			"projection", key,
			"stage", "restore_stream_identity",
			"error", err)
		return coldRestore()
	}
	p.mu.Lock()
	p.snapshotRunStreamIdentity = streamIdentity
	p.mu.Unlock()
	snapshot, err := source.LoadProjectionSnapshot(loadCtx, ProjectionSnapshotLoadRequest{
		ProjectionKey:  key,
		ContractID:     contractID,
		StreamName:     info.Config.Name,
		StreamIdentity: streamIdentity,
		MaxCutoff:      targetSeq,
	})
	if err != nil {
		p.logger.Info("Projection snapshot unavailable; replaying event log",
			"projection", key,
			"stage", "restore",
			"error", err)
		return coldRestore()
	}
	if snapshot.ContractID != contractID || snapshot.StreamName != info.Config.Name || snapshot.StreamIdentity != streamIdentity {
		p.logger.Warn("Projection snapshot binding rejected; replaying event log",
			"projection", key,
			"stage", "restore_validate",
			"generation_id", snapshot.GenerationID,
			"snapshot_contract_id", snapshot.ContractID,
			"snapshot_stream_name", snapshot.StreamName,
			"snapshot_stream_identity", snapshot.StreamIdentity)
		return coldRestore()
	}
	currentIdentity, err := p.resolveCurrentStreamIdentity(loadCtx, resolveStreamIdentity)
	if err != nil {
		return fmt.Errorf("recheck projection snapshot stream identity: %w", err)
	}
	if currentIdentity != streamIdentity {
		return fmt.Errorf("projection snapshot stream identity changed while loading")
	}
	if snapshot.CutoffSequence > targetSeq {
		p.logger.Warn("Projection snapshot cutoff rejected; replaying event log",
			"projection", key,
			"stage", "restore_validate",
			"generation_id", snapshot.GenerationID,
			"cutoff_seq", snapshot.CutoffSequence,
			"target_seq", targetSeq)
		return coldRestore()
	}
	projection, ok := p.proj.(snapshotProjectionState)
	if !ok {
		return fmt.Errorf("projection %q no longer supports snapshots", key)
	}
	if err := projection.Restore(snapshot.Payload); err != nil {
		p.logger.Warn("Projection snapshot restore failed; replaying event log",
			"projection", key,
			"stage", "restore_apply",
			"generation_id", snapshot.GenerationID,
			"error", err)
		if resetErr := coldRestore(); resetErr != nil {
			return errors.Join(fmt.Errorf("restore projection snapshot: %w", err), resetErr)
		}
		return nil
	}
	p.mu.Lock()
	p.restoredSeq = snapshot.CutoffSequence
	p.restoredGenerationID = snapshot.GenerationID
	p.snapshotRestored = true
	p.latestSnapshotSeq = snapshot.CutoffSequence
	p.latestSnapshotAt = snapshot.CreatedAt
	p.mu.Unlock()
	// Restore runs after Run marks the projector started, so boot-time callers may already be
	// waiting for this sequence. Advance through the normal waiter path instead
	// of assigning lastSeq directly.
	p.advance(snapshot.CutoffSequence)
	p.logger.Info("Projection snapshot restored",
		"projection", key,
		"stage", "restore_apply",
		"generation_id", snapshot.GenerationID,
		"cutoff_seq", snapshot.CutoffSequence,
		"target_seq", targetSeq,
		"payload_bytes", len(snapshot.Payload))
	return nil
}

func (p *Projector) restoreSnapshotCohortForRun(ctx context.Context, targetSeq uint64, coldRestore func() error) error {
	projection, ok := p.proj.(snapshotCohortProjectionState)
	if !ok {
		return fmt.Errorf("projection does not support snapshot cohorts")
	}
	p.mu.Lock()
	source := p.snapshotCohortSource
	key := p.snapshotKey
	contractID := p.snapshotCohortContractID
	resolveStreamIdentity := p.snapshotIdentityResolver
	configuredStreamIdentity := p.snapshotConfiguredID
	loadTimeout := p.snapshotLoadTimeout
	p.mu.Unlock()
	if loadTimeout <= 0 {
		loadTimeout = projectionSnapshotLoadTimeout
	}
	loadCtx, cancelLoad := context.WithTimeout(ctx, loadTimeout)
	defer cancelLoad()
	info, err := p.freshStreamInfo(loadCtx)
	if err != nil {
		p.mu.Lock()
		p.snapshotRunStreamIdentity = configuredStreamIdentity
		p.mu.Unlock()
		p.logger.Info("Projection snapshot stream info unavailable; replaying event log",
			"projection", key, "stage", "restore_stream_info", "error", err)
		return coldRestore()
	}
	streamIdentity, err := resolveProjectionStreamIdentity(info, resolveStreamIdentity)
	if err != nil {
		p.mu.Lock()
		p.snapshotRunStreamIdentity = configuredStreamIdentity
		p.mu.Unlock()
		p.logger.Info("Projection snapshot stream identity unavailable; replaying event log",
			"projection", key, "stage", "restore_stream_identity", "error", err)
		return coldRestore()
	}
	p.mu.Lock()
	p.snapshotRunStreamIdentity = streamIdentity
	p.mu.Unlock()
	cohort, err := source.LoadProjectionSnapshotCohort(loadCtx, ProjectionSnapshotCohortLoadRequest{
		ProjectionKey: key, ContractID: contractID, StreamName: info.Config.Name,
		StreamIdentity: streamIdentity, MaxCutoff: targetSeq,
		Components: projection.SnapshotComponentContracts(),
	})
	if err != nil {
		p.logger.Info("Projection snapshot cohort unavailable; replaying event log",
			"projection", key, "stage", "restore", "error", err)
		return coldRestore()
	}
	if cohort.ContractID != contractID || cohort.StreamName != info.Config.Name || cohort.StreamIdentity != streamIdentity {
		p.logger.Warn("Projection snapshot cohort binding rejected; replaying event log",
			"projection", key, "stage", "restore_validate", "generation_id", cohort.GenerationID)
		return coldRestore()
	}
	currentIdentity, err := p.resolveCurrentStreamIdentity(loadCtx, resolveStreamIdentity)
	if err != nil {
		return fmt.Errorf("recheck projection snapshot cohort stream identity: %w", err)
	}
	if currentIdentity != streamIdentity {
		return fmt.Errorf("projection snapshot cohort stream identity changed while loading")
	}
	if cohort.CutoffSequence > targetSeq {
		p.logger.Warn("Projection snapshot cohort cutoff rejected; replaying event log",
			"projection", key, "stage", "restore_validate", "generation_id", cohort.GenerationID,
			"cutoff_seq", cohort.CutoffSequence, "target_seq", targetSeq)
		return coldRestore()
	}
	if err := projection.RestoreComponents(cohort.Components); err != nil {
		p.logger.Warn("Projection snapshot cohort restore failed; replaying event log",
			"projection", key, "stage", "restore_apply", "generation_id", cohort.GenerationID, "error", err)
		if resetErr := coldRestore(); resetErr != nil {
			return errors.Join(fmt.Errorf("restore projection snapshot cohort: %w", err), resetErr)
		}
		return nil
	}
	p.mu.Lock()
	p.restoredSeq = cohort.CutoffSequence
	p.restoredGenerationID = cohort.GenerationID
	p.snapshotRestored = true
	p.latestSnapshotSeq = cohort.CutoffSequence
	p.latestSnapshotAt = cohort.CreatedAt
	p.mu.Unlock()
	p.advance(cohort.CutoffSequence)
	p.logger.Info("Projection snapshot cohort restored",
		"projection", key, "stage", "restore_apply", "generation_id", cohort.GenerationID,
		"cutoff_seq", cohort.CutoffSequence, "target_seq", targetSeq, "components", len(cohort.Components))
	return nil
}

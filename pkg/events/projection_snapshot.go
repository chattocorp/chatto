package events

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// SnapshotStatePartKey is the part key of a component whose complete state is
// one payload. ComponentizedProjection components and single-payload
// projections use it. A single-payload projection also uses it as its
// component key.
const SnapshotStatePartKey = "state"

// SnapshotProjection is a projection whose complete state serializes to one
// payload. The Projector stores it as a snapshot with one component that has
// one part.
type SnapshotProjection interface {
	// SnapshotContractID covers every projection-specific input that
	// determines whether restoring a snapshot is equivalent to replaying the
	// event log through its cutoff. Changing unrelated application versions
	// must not invalidate it.
	SnapshotContractID() string

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

// ComponentSnapshotProjection is a projection whose state serializes to
// several components that are captured and restored as one unit.
// ComponentizedProjection implements it.
type ComponentSnapshotProjection interface {
	// SnapshotContractID is the restore-equivalence contract for the complete
	// component set.
	SnapshotContractID() string
	// SnapshotComponentContracts returns the exact component set that a
	// snapshot source must validate before it loads payload parts.
	SnapshotComponentContracts() []ProjectionSnapshotComponentContract
	// SnapshotComponents serializes every component.
	SnapshotComponents() ([]ProjectionSnapshotComponent, error)
	// RestoreComponents installs a complete component set. It must leave the
	// prior state unchanged when it returns an error.
	RestoreComponents([]ProjectionSnapshotComponent) error
	// ResetComponents installs every component's canonical empty state.
	ResetComponents() error
}

// ProjectionSnapshotPart is one stable, independently stored part of a
// projection snapshot component.
type ProjectionSnapshotPart struct {
	Key     string
	Payload []byte
}

// ProjectionSnapshotComponent is one independently serialized component of a
// projection snapshot. Part keys are stable within its contract.
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

// ProjectionSnapshot is projection state captured or restored at one event-log
// cutoff. Its components are installed as one unit. Restored snapshots must
// carry the contract, stream name, and stream identity that were validated by
// the source; the Projector checks those bindings again before it installs the
// components.
type ProjectionSnapshot struct {
	GenerationID   string
	ContractID     string
	StreamName     string
	CutoffSequence uint64
	StreamIdentity string
	CreatedAt      time.Time
	Components     []ProjectionSnapshotComponent
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
	// Components is the exact component set of the projection. A
	// SnapshotProjection has one component with exactly one part.
	Components []ProjectionSnapshotComponentContract
}

// ProjectionSnapshotSource loads disposable projection state for a specific
// projection contract and stream incarnation. Implementations must enforce
// every constraint in the request, must not return a partial snapshot, and
// return an error when no valid snapshot is available; the Projector then
// falls back to replaying retained stream events.
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

// projectorSnapshots is the snapshot configuration of one Projector. Fields
// are guarded by Projector.mu.
type projectorSnapshots struct {
	key        string
	contractID string
	source     ProjectionSnapshotSource
	// resolveIdentity resolves the stream identity that binds snapshots.
	resolveIdentity StreamIdentityResolver
	// configuredIdentity is the identity resolved by ConfigureSnapshots. Run
	// uses it when it cannot read fresh stream information.
	configuredIdentity string
	// runIdentity is the identity bound to the current run. Capture rejects a
	// stream whose identity differs from it.
	runIdentity string
	loadTimeout time.Duration
}

// payloadSnapshotter is the state interface of SnapshotProjection without the
// contract. A projection that implements it is reset with Restore(nil) before
// a cold replay, also when snapshots are not configured.
type payloadSnapshotter interface {
	Snapshot() ([]byte, error)
	Restore([]byte) error
}

// snapshotStateOf returns the component view of a projection's snapshot state.
func snapshotStateOf(projection SubjectProjection) (ComponentSnapshotProjection, bool) {
	if state, ok := projection.(ComponentSnapshotProjection); ok {
		return state, true
	}
	if state, ok := projection.(payloadSnapshotter); ok {
		return singlePayloadSnapshot{projection: state}, true
	}
	return nil, false
}

// singlePayloadSnapshot adapts a single-payload projection to one component
// with one part.
type singlePayloadSnapshot struct {
	projection payloadSnapshotter
}

func (s singlePayloadSnapshot) SnapshotContractID() string {
	if contract, ok := s.projection.(SnapshotProjection); ok {
		return contract.SnapshotContractID()
	}
	return ""
}

func (s singlePayloadSnapshot) SnapshotComponentContracts() []ProjectionSnapshotComponentContract {
	return []ProjectionSnapshotComponentContract{{
		Key: SnapshotStatePartKey, ContractID: s.SnapshotContractID(), MaxParts: 1,
	}}
}

func (s singlePayloadSnapshot) SnapshotComponents() ([]ProjectionSnapshotComponent, error) {
	payload, err := s.projection.Snapshot()
	if err != nil {
		return nil, err
	}
	return []ProjectionSnapshotComponent{{
		Key: SnapshotStatePartKey, ContractID: s.SnapshotContractID(),
		Parts: []ProjectionSnapshotPart{{Key: SnapshotStatePartKey, Payload: payload}},
	}}, nil
}

func (s singlePayloadSnapshot) RestoreComponents(components []ProjectionSnapshotComponent) error {
	if len(components) != 1 {
		return fmt.Errorf("projection snapshot has %d components, want 1", len(components))
	}
	payload, err := statePartPayload(components[0], SnapshotStatePartKey, s.SnapshotContractID())
	if err != nil {
		return err
	}
	return s.projection.Restore(payload)
}

func (s singlePayloadSnapshot) ResetComponents() error {
	return s.projection.Restore(nil)
}

// statePartPayload returns the payload of a component that must have key,
// contractID, and exactly one SnapshotStatePartKey part.
func statePartPayload(component ProjectionSnapshotComponent, key, contractID string) ([]byte, error) {
	if component.Key != key {
		return nil, fmt.Errorf("projection snapshot component is %q, want %q", component.Key, key)
	}
	if component.ContractID != contractID {
		return nil, fmt.Errorf("projection snapshot component %q contract does not match", key)
	}
	if len(component.Parts) != 1 || component.Parts[0].Key != SnapshotStatePartKey {
		return nil, fmt.Errorf("projection snapshot component %q has %d parts, want 1", key, len(component.Parts))
	}
	return component.Parts[0].Payload, nil
}

// ConfigureSnapshots enables best-effort bootstrap restore for a projection
// that implements SnapshotProjection or ComponentSnapshotProjection. The
// identity resolver receives the same fresh stream information used by the
// restore request. It must be called before Run and cannot be combined with a
// local checkpoint. A load or restore failure is logged and falls back to an
// empty projection followed by full event replay.
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
	configuredIdentity, err := resolveProjectionStreamIdentity(p.stream.CachedInfo(), resolveStreamIdentity)
	if err != nil {
		return fmt.Errorf("resolve projection snapshot stream identity: %w", err)
	}
	state, ok := snapshotStateOf(p.proj)
	if !ok || state.SnapshotContractID() == "" {
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
	p.snapshots = projectorSnapshots{
		key:                key,
		contractID:         state.SnapshotContractID(),
		source:             source,
		resolveIdentity:    resolveStreamIdentity,
		configuredIdentity: configuredIdentity,
		loadTimeout:        projectionSnapshotLoadTimeout,
	}
	return nil
}

// SnapshotContractID returns the contract captured when snapshots were
// configured. Restore and publication must use this single value.
func (p *Projector) SnapshotContractID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snapshots.contractID
}

// CaptureSnapshot serializes every snapshot component, the corresponding
// applied event sequence, and the stream identity bound to this run at one
// apply barrier. An empty payload is valid canonical state and still carries
// the projection's replay cutoff.
func (p *Projector) CaptureSnapshot(ctx context.Context) (ProjectionSnapshot, error) {
	p.mu.Lock()
	snapshots := p.snapshots
	p.mu.Unlock()
	state, ok := snapshotStateOf(p.proj)
	if !ok {
		return ProjectionSnapshot{}, fmt.Errorf("projection does not support snapshots")
	}
	checkIdentity := func(stage string) error {
		if snapshots.resolveIdentity == nil {
			return nil
		}
		currentIdentity, err := p.resolveCurrentStreamIdentity(ctx, snapshots.resolveIdentity)
		if err != nil {
			return fmt.Errorf("resolve stream identity %s snapshot capture: %w", stage, err)
		}
		if snapshots.runIdentity == "" || currentIdentity != snapshots.runIdentity {
			return fmt.Errorf("stream identity changed during projector run")
		}
		return nil
	}
	if err := checkIdentity("before"); err != nil {
		return ProjectionSnapshot{}, err
	}

	components, seq, err := func() ([]ProjectionSnapshotComponent, uint64, error) {
		p.applyMu.Lock()
		defer p.applyMu.Unlock()
		components, err := state.SnapshotComponents()
		if err != nil {
			return nil, 0, err
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		return components, p.lastSeq, nil
	}()
	if err != nil {
		return ProjectionSnapshot{}, err
	}

	if err := checkIdentity("after"); err != nil {
		return ProjectionSnapshot{}, err
	}
	return ProjectionSnapshot{
		ContractID:     snapshots.contractID,
		StreamName:     p.stream.CachedInfo().Config.Name,
		CutoffSequence: seq,
		StreamIdentity: snapshots.runIdentity,
		Components:     components,
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

// restoreForRun prepares projection state before Run consumes events: from a
// local checkpoint, from a snapshot, or empty for a cold replay. The caller
// holds applyMu.
func (p *Projector) restoreForRun(ctx context.Context, targetSeq uint64) error {
	p.mu.Lock()
	checkpointKey := p.checkpointKey
	snapshots := p.snapshots
	p.mu.Unlock()
	if checkpointKey != "" {
		return p.restoreCheckpointForRun(ctx, targetSeq)
	}
	if snapshots.source == nil {
		return p.resetForColdReplay()
	}
	return p.restoreSnapshotForRun(ctx, targetSeq, snapshots)
}

// resetForColdReplay installs empty projection state before a replay of the
// complete retained history.
func (p *Projector) resetForColdReplay() error {
	if state, ok := snapshotStateOf(p.proj); ok {
		if err := state.ResetComponents(); err != nil {
			return fmt.Errorf("restore empty projection: %w", err)
		}
	}
	p.resetRestoreState()
	return nil
}

func (p *Projector) setRunStreamIdentity(identity string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.snapshots.runIdentity = identity
}

func (p *Projector) restoreSnapshotForRun(ctx context.Context, targetSeq uint64, snapshots projectorSnapshots) error {
	key := snapshots.key
	state, ok := snapshotStateOf(p.proj)
	if !ok {
		return fmt.Errorf("projection %q no longer supports snapshots", key)
	}
	loadTimeout := snapshots.loadTimeout
	if loadTimeout <= 0 {
		loadTimeout = projectionSnapshotLoadTimeout
	}
	loadCtx, cancelLoad := context.WithTimeout(ctx, loadTimeout)
	defer cancelLoad()

	info, err := p.freshStreamInfo(loadCtx)
	if err != nil {
		p.setRunStreamIdentity(snapshots.configuredIdentity)
		p.logger.Info("Projection snapshot stream info unavailable; replaying event log",
			"projection", key, "stage", "restore_stream_info", "error", err)
		return p.resetForColdReplay()
	}
	streamIdentity, err := resolveProjectionStreamIdentity(info, snapshots.resolveIdentity)
	if err != nil {
		p.setRunStreamIdentity(snapshots.configuredIdentity)
		p.logger.Info("Projection snapshot stream identity unavailable; replaying event log",
			"projection", key, "stage", "restore_stream_identity", "error", err)
		return p.resetForColdReplay()
	}
	p.setRunStreamIdentity(streamIdentity)

	snapshot, err := snapshots.source.LoadProjectionSnapshot(loadCtx, ProjectionSnapshotLoadRequest{
		ProjectionKey:  key,
		ContractID:     snapshots.contractID,
		StreamName:     info.Config.Name,
		StreamIdentity: streamIdentity,
		MaxCutoff:      targetSeq,
		Components:     state.SnapshotComponentContracts(),
	})
	if err != nil {
		p.logger.Info("Projection snapshot unavailable; replaying event log",
			"projection", key, "stage", "restore", "error", err)
		return p.resetForColdReplay()
	}
	if snapshot.ContractID != snapshots.contractID || snapshot.StreamName != info.Config.Name || snapshot.StreamIdentity != streamIdentity {
		p.logger.Warn("Projection snapshot binding rejected; replaying event log",
			"projection", key,
			"stage", "restore_validate",
			"generation_id", snapshot.GenerationID,
			"snapshot_contract_id", snapshot.ContractID,
			"snapshot_stream_name", snapshot.StreamName,
			"snapshot_stream_identity", snapshot.StreamIdentity)
		return p.resetForColdReplay()
	}
	currentIdentity, err := p.resolveCurrentStreamIdentity(loadCtx, snapshots.resolveIdentity)
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
		return p.resetForColdReplay()
	}
	if err := state.RestoreComponents(snapshot.Components); err != nil {
		p.logger.Warn("Projection snapshot restore failed; replaying event log",
			"projection", key,
			"stage", "restore_apply",
			"generation_id", snapshot.GenerationID,
			"error", err)
		if resetErr := p.resetForColdReplay(); resetErr != nil {
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
	// Restore runs after Run marks the projector started, so boot-time callers
	// may already be waiting for this sequence. Advance through the normal
	// waiter path instead of assigning lastSeq directly.
	p.advance(snapshot.CutoffSequence)
	payloadBytes := 0
	for _, component := range snapshot.Components {
		for _, part := range component.Parts {
			payloadBytes += len(part.Payload)
		}
	}
	p.logger.Info("Projection snapshot restored",
		"projection", key,
		"stage", "restore_apply",
		"generation_id", snapshot.GenerationID,
		"cutoff_seq", snapshot.CutoffSequence,
		"target_seq", targetSeq,
		"components", len(snapshot.Components),
		"payload_bytes", payloadBytes)
	return nil
}

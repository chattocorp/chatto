package events

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// ErrProjectionFailed marks a projector that stopped applying events
// because its Projection.Apply returned an error.
var ErrProjectionFailed = errors.New("projection failed")

// ErrProjectionSubjectNotConsumed is returned when a caller asks a projector
// to wait for a subject outside the projection's declared filter set.
var ErrProjectionSubjectNotConsumed = errors.New("projection does not consume subject")

// ErrProjectionSequenceSubjectMismatch is returned when a caller asks a
// projector to wait for a sequence that belongs to a different subject than the
// one supplied by the caller.
var ErrProjectionSequenceSubjectMismatch = errors.New("projection wait sequence subject mismatch")

// Projection replay is a sequential bulk read. NATS defaults to a 500-message
// client buffer, which turns histories of many small EVT records into many
// latency-bound pull requests on a remote JetStream cluster. A byte window
// keeps those pulls large while bounding client-side memory.
const (
	projectionPullMaxBytes        = 16 * 1024 * 1024
	projectionSnapshotLoadTimeout = 15 * time.Second
	// Ordered pull consumers cannot issue another pull while a synchronous
	// projection Apply is running. Keep their cleanup window comfortably above
	// slow disk-backed commits so NATS cannot delete a live projector consumer.
	projectionConsumerInactiveThreshold = 5 * time.Minute
	// EVTStreamIdentityMetadataKey stores the durable stream incarnation used to
	// reject projection snapshots after EVT is deleted and recreated.
	EVTStreamIdentityMetadataKey = "chatto.evt.incarnation"
	streamIdentityPrefix         = "evt-incarnation-v1:"
)

// MemoryProjection is an embeddable base for projections whose state lives
// entirely in process memory. It contributes only a sync.RWMutex for read/write
// coordination; projections opt into snapshot persistence by implementing
// SnapshotProjection explicitly.
//
// Embed by value — the zero mutex is ready to use. Subclasses still
// implement Subjects() and Apply(). Future non-memory projection types
// (KV-backed, file-backed) would have their own embed-friendly base.
type MemoryProjection struct {
	sync.RWMutex
}

// Projection is the read side. Implementations consume events from a subject
// filter and serve reads from derived state. Most projections are in-memory;
// CheckpointedProjection supports disposable local disk-backed state.
//
// Concurrency contract: Apply is called from a single goroutine owned by
// the Projector, in stream order. Implementations don't need internal
// locking on the write path. They DO need a read lock if external code reads
// concurrently; projections typically embed a sync.RWMutex for this.
//
// Idempotency: Apply(e, n) followed by Apply(e, n) must produce the same
// state as a single Apply(e, n). Snapshot tail replay relies on this contract.
//
// Event immutability: core event protobufs are durable facts. Apply
// implementations must treat the input event as read-only, and projection
// read APIs that expose event pointers rely on callers treating those events
// as read-only as well. Projections that derive mutable current-state values
// from events should copy those values before mutating or returning them.
type Projection interface {
	// Subjects returns the subject filter(s) this projection consumes.
	// Wildcards are supported (e.g. "server.evt.room.>").
	Subjects() []string

	// Apply is called for every event matching Subjects(), in stream
	// order. seq is the stream sequence of this event.
	Apply(event *corev1.Event, seq uint64) error
}

// SequencedEvent pairs one decoded EVT event with its stable stream sequence.
// StartupBatchProjection receives these in strictly increasing stream order.
type SequencedEvent struct {
	Event    *corev1.Event
	Sequence uint64
}

// StartupBatchProjection atomically applies groups of events while a projector
// replays its captured startup history. ApplyStartupBatch must produce the same
// state as calling Apply for each item in order and must commit the final
// sequence together with every derived mutation before returning success.
// Live events continue through Apply individually after startup is current.
type StartupBatchProjection interface {
	Projection
	StartupBatchSize() int
	ApplyStartupBatch([]SequencedEvent) error
}

// SnapshotProjection supports serializing and restoring projection state.
// Snapshot persistence is optional and configured separately on Projector.
type SnapshotProjection interface {
	Projection

	// Snapshot returns a serialized form of the current state.
	// Returning (nil, nil) means "no snapshot support yet"; the Projector
	// will then skip snapshot persistence.
	Snapshot() ([]byte, error)

	// Restore initializes state from a snapshot. Called once before Run
	// starts consuming. May be called with nil/empty for cold start —
	// the projection should treat that as "no prior snapshot state."
	// Implementations must leave their prior state unchanged when returning
	// an error so the Projector can reliably fall back to cold replay.
	Restore(snapshot []byte) error
}

// SnapshotContractProjection opts a Projection into persisted snapshots.
// The contract ID covers every projection-specific input that determines
// whether restoring a snapshot is equivalent to replaying EVT through its
// cutoff. Changing unrelated Chatto versions must not invalidate it.
type SnapshotContractProjection interface {
	SnapshotProjection
	SnapshotContractID() string
}

// ProjectionSnapshot is a validated snapshot returned by a snapshot source.
type ProjectionSnapshot struct {
	GenerationID   string
	CutoffSequence uint64
	CreatedAt      time.Time
	Payload        []byte
}

// ProjectionSnapshotLoadRequest contains the encrypted repository lookup
// constraints owned by the Projector. Sources must reject mismatched or newer
// stream state before returning a snapshot.
type ProjectionSnapshotLoadRequest struct {
	ProjectionKey  string
	ContractID     string
	StreamName     string
	StreamIdentity string
	MaxCutoff      uint64
}

type ProjectionSnapshotSource interface {
	LoadProjectionSnapshot(context.Context, ProjectionSnapshotLoadRequest) (ProjectionSnapshot, error)
}

// ReplaySubjectProjection can be implemented when a projection's logical
// consumed subjects are narrower than the physical filters its ordered
// consumer should use. Waits and diagnostics still report the narrower
// Subjects contract.
type ReplaySubjectProjection interface {
	ReplaySubjects() []string
}

// StartupReplayCompleter can be implemented by projections that retain
// temporary state only while replaying the stream at process startup. The
// Projector calls CompleteStartupReplay exactly once after every event through
// the captured startup target has been applied. It is also called for an empty
// or already-current projection.
type StartupReplayCompleter interface {
	CompleteStartupReplay()
}

// Projector runs the consumer + apply loop for one projection.
type Projector struct {
	js      jetstream.JetStream
	stream  jetstream.Stream
	proj    Projection
	logger  Logger
	applyMu sync.Mutex

	subjects        []string
	replaySubjects  []string
	subjectMatchers []compiledSubjectFilter

	mu        sync.Mutex
	lastSeq   uint64
	waiters   []seqWaiter
	failedSeq uint64
	failedErr error
	failedCh  chan struct{}
	// started flips true the first time Run is invoked and stays true
	// for the projector's lifetime. WaitFor uses this to short-
	// circuit during boot-time mutations that happen before
	// ChattoCore.Run gets a chance to start the consumer (see the
	// WaitFor doc for why).
	started bool

	startupStartedAt time.Time
	startupTargetSeq uint64
	startupEndedAt   time.Time
	startupCompleted bool
	startupMessages  uint64
	startupLogged    bool
	startupBatchSize int
	startupBatch     []SequencedEvent

	snapshotKey          string
	snapshotContractID   string
	snapshotSource       ProjectionSnapshotSource
	snapshotStreamID     string
	snapshotLoadTimeout  time.Duration
	restoredSeq          uint64
	restoredGenerationID string
	snapshotRestored     bool
	latestSnapshotSeq    uint64
	latestSnapshotAt     time.Time

	checkpointKey        string
	checkpointContractID string
	checkpointRestored   bool
	checkpointCutoffSeq  uint64
}

// ProjectorStatus is a concurrency-safe snapshot of a projector's
// lifecycle state. Operators use it for diagnostics; core readiness uses
// Err to surface fatal projection failures.
type ProjectorStatus struct {
	Started bool
	LastSeq uint64

	StartupTargetSeq     uint64
	StartupComplete      bool
	StartupDuration      time.Duration
	StartupMessages      uint64
	SnapshotRestored     bool
	SnapshotCutoffSeq    uint64
	SnapshotGenerationID string
	CheckpointRestored   bool
	CheckpointCutoffSeq  uint64
	CheckpointContractID string
	LatestSnapshotSeq    uint64
	LatestSnapshotAt     time.Time

	Failed    bool
	FailedSeq uint64
	Failure   string
	Err       error
}

type seqWaiter struct {
	seq uint64
	ch  chan struct{}
}

// NewProjector binds a projection to a stream. Does not start the consumer
// — call Run for that.
func NewProjector(js jetstream.JetStream, stream jetstream.Stream, proj Projection, logger Logger) *Projector {
	subjects := append([]string(nil), proj.Subjects()...)
	replaySubjects := append([]string(nil), projectionReplaySubjects(proj, subjects)...)
	startupBatchSize := 0
	if projection, ok := proj.(StartupBatchProjection); ok {
		if size := projection.StartupBatchSize(); size > 1 {
			startupBatchSize = size
		}
	}
	return &Projector{
		js:               js,
		stream:           stream,
		proj:             proj,
		logger:           logger,
		subjects:         subjects,
		replaySubjects:   replaySubjects,
		subjectMatchers:  compileSubjectFilters(subjects),
		failedCh:         make(chan struct{}),
		startupBatchSize: startupBatchSize,
	}
}

// ConfigureSnapshots enables best-effort bootstrap restore for this projector.
// It must be called before Run. A load or restore failure is logged and falls
// back to an empty projection followed by full EVT replay.
func (p *Projector) ConfigureSnapshots(key string, source ProjectionSnapshotSource, streamIdentity string) error {
	if key == "" {
		return fmt.Errorf("projection snapshot key is required")
	}
	if source == nil {
		return fmt.Errorf("projection snapshot source is nil")
	}
	if !ValidStreamIdentity(streamIdentity) {
		return fmt.Errorf("projection snapshot EVT stream identity is invalid")
	}
	contractProjection, ok := p.proj.(SnapshotContractProjection)
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
	p.snapshotKey = key
	p.snapshotContractID = contractID
	p.snapshotSource = source
	p.snapshotStreamID = streamIdentity
	p.snapshotLoadTimeout = projectionSnapshotLoadTimeout
	return nil
}

// SnapshotContractID returns the contract captured when snapshots were
// configured. Restore and publication must use this single value.
func (p *Projector) SnapshotContractID() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.snapshotContractID
}

// CaptureSnapshot serializes projection state and the corresponding applied
// EVT sequence at one barrier. An empty protobuf payload is valid canonical
// state and still carries the projection's replay cutoff.
func (p *Projector) CaptureSnapshot() (ProjectionSnapshot, error) {
	p.applyMu.Lock()
	defer p.applyMu.Unlock()

	projection, ok := p.proj.(SnapshotProjection)
	if !ok {
		return ProjectionSnapshot{}, fmt.Errorf("projection does not support snapshots")
	}
	payload, err := projection.Snapshot()
	if err != nil {
		return ProjectionSnapshot{}, err
	}
	p.mu.Lock()
	seq := p.lastSeq
	p.mu.Unlock()
	return ProjectionSnapshot{CutoffSequence: seq, Payload: payload}, nil
}

// NewStreamIdentity deterministically derives an opaque identity for one EVT
// stream incarnation. created is used only when initializing missing metadata;
// normal restarts read the persisted identity instead.
func NewStreamIdentity(created time.Time) (string, error) {
	if created.IsZero() {
		return "", fmt.Errorf("EVT stream creation time is required")
	}
	sum := sha256.Sum256([]byte("chatto/evt-incarnation/v1\x00" + created.UTC().Format(time.RFC3339Nano)))
	return streamIdentityPrefix + hex.EncodeToString(sum[:16]), nil
}

// ValidStreamIdentity reports whether identity has Chatto's versioned EVT
// stream-incarnation format.
func ValidStreamIdentity(identity string) bool {
	if len(identity) != len(streamIdentityPrefix)+32 || !strings.HasPrefix(identity, streamIdentityPrefix) {
		return false
	}
	_, err := hex.DecodeString(identity[len(streamIdentityPrefix):])
	return err == nil
}

// StreamIdentity reads the durable incarnation identity cached when EVT was
// opened. Unlike StreamInfo.Created, this value survives process reconstruction
// and backup restore.
func StreamIdentity(stream jetstream.Stream) (string, error) {
	if stream == nil {
		return "", fmt.Errorf("EVT stream is required")
	}
	info := stream.CachedInfo()
	if info == nil {
		return "", fmt.Errorf("EVT stream info is unavailable")
	}
	identity := info.Config.Metadata[EVTStreamIdentityMetadataKey]
	if !ValidStreamIdentity(identity) {
		return "", fmt.Errorf("EVT stream identity is missing or invalid")
	}
	return identity, nil
}

// Status returns the projector's current lifecycle state. Safe to call from
// any goroutine.
func (p *Projector) Status() ProjectorStatus {
	p.mu.Lock()
	defer p.mu.Unlock()

	var snapshotCutoffSeq uint64
	if p.snapshotRestored {
		snapshotCutoffSeq = p.restoredSeq
	}
	status := ProjectorStatus{
		Started:              p.started,
		LastSeq:              p.lastSeq,
		StartupTargetSeq:     p.startupTargetSeq,
		StartupComplete:      p.startupCompleted,
		StartupMessages:      p.startupMessages,
		SnapshotRestored:     p.snapshotRestored,
		SnapshotCutoffSeq:    snapshotCutoffSeq,
		SnapshotGenerationID: p.restoredGenerationID,
		CheckpointRestored:   p.checkpointRestored,
		CheckpointCutoffSeq:  p.checkpointCutoffSeq,
		CheckpointContractID: p.checkpointContractID,
		LatestSnapshotSeq:    p.latestSnapshotSeq,
		LatestSnapshotAt:     p.latestSnapshotAt,
	}
	if !p.startupStartedAt.IsZero() {
		startupEndsAt := p.startupEndedAt
		if startupEndsAt.IsZero() {
			startupEndsAt = time.Now()
		}
		status.StartupDuration = startupEndsAt.Sub(p.startupStartedAt)
	}
	if p.failedErr != nil {
		status.Failed = true
		status.FailedSeq = p.failedSeq
		status.Failure = p.failedErr.Error()
		status.Err = p.failedErr
	}
	return status
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

// Err returns the fatal projection error, if the projector has stopped
// because it could not decode or apply an event.
func (p *Projector) Err() error {
	return p.Status().Err
}

// LastSeq returns the highest matching ordered stream sequence the projector
// has applied. Safe to call from any goroutine.
func (p *Projector) LastSeq() uint64 {
	return p.Status().LastSeq
}

// Started reports whether Run has entered its body — i.e. whether
// the projector's consumer is being set up / has been set up. Used by
// test helpers (and lifecycle code) that need to wait for projectors
// to come online before issuing reads against the projection.
func (p *Projector) Started() bool {
	return p.Status().Started
}

// Subjects returns the subject filters this projector consumes.
// The returned slice is a copy so callers cannot mutate projection state.
func (p *Projector) Subjects() []string {
	return append([]string(nil), p.subjects...)
}

// ReplaySubjects returns the physical stream filters used for replay.
func (p *Projector) ReplaySubjects() []string {
	return append([]string(nil), p.replaySubjects...)
}

// AppendAndWait publishes an event for an aggregate and blocks until
// this projection has applied it. The subject is derived from
// `agg.SubjectFor(event)`, so the caller cannot accidentally publish an
// event onto the wrong subject for its payload.
//
// This is the single-shot "publish-then read-your-writes" primitive.
// If it returns ErrConflict, state-replacement callers must re-read and
// re-compose before retrying.
//
// Returns the stream sequence the publish landed at, plus any error.
// On a publish failure the sequence is 0; on a wait failure (most
// commonly ctx cancellation) the sequence is non-zero and the event
// has already been durably published — only the local projection
// hasn't caught up.
func (p *Projector) AppendAndWait(ctx context.Context, pub *Publisher, agg Aggregate, event *corev1.Event) (uint64, error) {
	subject := agg.SubjectFor(event)
	seq, err := pub.Append(ctx, subject, event)
	if err != nil {
		return 0, err
	}
	if err := p.WaitFor(ctx, SubjectPosition(subject, seq)); err != nil {
		return seq, err
	}
	return seq, nil
}

// AppendEventuallyAndWait is AppendAndWait for append-only events that can
// safely retry the same payload after an OCC conflict. See
// Publisher.AppendEventually for the safety rule.
func (p *Projector) AppendEventuallyAndWait(ctx context.Context, pub *Publisher, agg Aggregate, event *corev1.Event) (uint64, error) {
	subject := agg.SubjectFor(event)
	seq, err := pub.AppendEventually(ctx, subject, event)
	if err != nil {
		return 0, err
	}
	if err := p.WaitFor(ctx, SubjectPosition(subject, seq)); err != nil {
		return seq, err
	}
	return seq, nil
}

// WaitFor blocks until LastSeq() >= pos.Seq or ctx is done.
//
// Used by writers that need read-your-writes consistency: capture the stream
// position for the write target, pass it here, then read from the projection.
// The stream sequence must belong to pos.SubjectFilter, and the sequence's
// actual subject must match one of this projector's subject filters.
//
// If LastSeq() is already >= pos.Seq when called, returns immediately with no
// error. Otherwise registers a waiter and blocks.
//
// Precondition: the projector's Run loop is expected to be active by
// the time any code reaches WaitFor. Callers that mutate during
// boot (ensureChannelRoomsAreInAGroup, SeedDefaultRooms) are
// orchestrated by core.Run / core.WaitForBoot to make this true.
// Calling before Run started would block forever waiting for a
// sequence that never advances — that's the symptom we want, since
// the alternative (silently skipping the wait) leaves the projection
// out of sync with the KV write and produces orphan-room bugs.
func (p *Projector) WaitFor(ctx context.Context, pos StreamPosition) error {
	if pos.IsZero() {
		return nil
	}

	if err := p.validateSeqSubject(ctx, pos); err != nil {
		return err
	}

	return p.waitForSeq(ctx, pos.Seq)
}

func (p *Projector) waitForSeq(ctx context.Context, seq uint64) error {
	p.mu.Lock()
	if p.failedErr != nil && seq >= p.failedSeq {
		err := p.failedErr
		p.mu.Unlock()
		return err
	}
	if p.lastSeq >= seq {
		p.mu.Unlock()
		return nil
	}
	ch := make(chan struct{})
	p.waiters = append(p.waiters, seqWaiter{seq: seq, ch: ch})
	// Keep waiters sorted ascending by seq so advance() can release them
	// in order and stop scanning at the first unmet seq.
	sort.Slice(p.waiters, func(i, j int) bool {
		return p.waiters[i].seq < p.waiters[j].seq
	})
	p.mu.Unlock()

	select {
	case <-ch:
		p.mu.Lock()
		err := p.failedErr
		failedSeq := p.failedSeq
		p.mu.Unlock()
		if err != nil && seq >= failedSeq {
			return err
		}
		return nil
	case <-ctx.Done():
		// Drop our waiter so we don't leak. The advance path tolerates
		// already-closed channels (it doesn't close twice), and a small
		// scan here is fine — waiters lists are short.
		p.mu.Lock()
		for i, w := range p.waiters {
			if w.ch == ch {
				p.waiters = append(p.waiters[:i], p.waiters[i+1:]...)
				break
			}
		}
		p.mu.Unlock()
		return ctx.Err()
	}
}

func (p *Projector) validateConsumesSubject(subject string) error {
	for i := range p.subjectMatchers {
		if p.subjectMatchers[i].matches(subject) {
			return nil
		}
	}
	return fmt.Errorf("%w: subject %q not matched by filters %v",
		ErrProjectionSubjectNotConsumed, subject, p.subjects)
}

func (p *Projector) validateSeqSubject(ctx context.Context, pos StreamPosition) error {
	msg, err := p.stream.GetMsg(ctx, pos.Seq)
	if err != nil {
		return fmt.Errorf("load stream sequence %d before projection wait: %w", pos.Seq, err)
	}
	if !subjectMatchesFilter(pos.SubjectFilter, msg.Subject) {
		return fmt.Errorf("%w: seq %d belongs to %q, not %q",
			ErrProjectionSequenceSubjectMismatch, pos.Seq, msg.Subject, pos.SubjectFilter)
	}
	if err := p.validateConsumesSubject(msg.Subject); err != nil {
		return err
	}
	return nil
}

func subjectMatchesFilter(filter, subject string) bool {
	return compileSubjectFilter(filter).matches(subject)
}

type compiledSubjectFilter struct {
	raw    string
	tokens []string
}

func compileSubjectFilters(filters []string) []compiledSubjectFilter {
	compiled := make([]compiledSubjectFilter, 0, len(filters))
	for _, filter := range filters {
		compiled = append(compiled, compileSubjectFilter(filter))
	}
	return compiled
}

func compileSubjectFilter(filter string) compiledSubjectFilter {
	return compiledSubjectFilter{
		raw:    filter,
		tokens: splitSubjectTokens(filter),
	}
}

func splitSubjectTokens(subject string) []string {
	if subject == "" {
		return nil
	}
	tokenCount := 1
	for i := 0; i < len(subject); i++ {
		if subject[i] == '.' {
			tokenCount++
		}
	}
	tokens := make([]string, 0, tokenCount)
	start := 0
	for i := 0; i <= len(subject); i++ {
		if i == len(subject) || subject[i] == '.' {
			tokens = append(tokens, subject[start:i])
			start = i + 1
		}
	}
	return tokens
}

func (f compiledSubjectFilter) matches(subject string) bool {
	if f.raw == "" || subject == "" {
		return false
	}
	pos := 0
	for i, token := range f.tokens {
		if token == ">" {
			return i == len(f.tokens)-1 && pos < len(subject)
		}
		if pos > len(subject) {
			return false
		}
		end := pos
		for end < len(subject) && subject[end] != '.' {
			end++
		}
		if end == pos {
			return false
		}
		if token != "*" && token != subject[pos:end] {
			return false
		}
		pos = end + 1
	}
	return pos == len(subject)+1
}

// WaitForCurrent blocks until the projection has applied the latest
// stream message currently matching its subject filters. It is intended
// for diagnostics and sequencing: call it after the projector is
// running to ensure projection reads reflect the stream as of this call.
func (p *Projector) WaitForCurrent(ctx context.Context) error {
	target, err := p.currentTarget(ctx)
	if err != nil {
		return err
	}
	if target.seq == 0 {
		return nil
	}
	return p.waitForSeq(ctx, target.seq)
}

// CurrentTargetSeq returns the highest stream sequence currently matching
// this projection's subject filters. A zero return means the stream has no
// message for any of the filters yet.
func (p *Projector) CurrentTargetSeq(ctx context.Context) (uint64, error) {
	target, err := p.currentTarget(ctx)
	return target.seq, err
}

type projectionTarget struct {
	seq uint64
}

func (p *Projector) currentTarget(ctx context.Context) (projectionTarget, error) {
	return p.targetForSubjects(ctx, p.subjects)
}

func (p *Projector) targetForSubjects(ctx context.Context, subjects []string) (projectionTarget, error) {
	var target projectionTarget
	for _, subject := range subjects {
		msg, err := p.stream.GetLastMsgForSubject(ctx, subject)
		if err != nil {
			if errors.Is(err, jetstream.ErrMsgNotFound) {
				continue
			}
			return projectionTarget{}, fmt.Errorf("last msg for subject %q: %w", subject, err)
		}
		if msg.Sequence > target.seq {
			target = projectionTarget{seq: msg.Sequence}
		}
	}
	return target, nil
}

func projectionReplaySubjects(proj Projection, subjects []string) []string {
	if replay, ok := proj.(ReplaySubjectProjection); ok {
		return replay.ReplaySubjects()
	}
	return subjects
}

func (p *Projector) consumesSubject(subject string) bool {
	for i := range p.subjectMatchers {
		if p.subjectMatchers[i].matches(subject) {
			return true
		}
	}
	return false
}

// advance updates lastSeq and releases any waiters that have now been
// reached. Called from the consumer goroutine after each successful Apply.
func (p *Projector) advance(seq uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if seq > p.lastSeq {
		p.lastSeq = seq
	}
	// Waiters are sorted ascending; pop from the front while their seq is
	// met by the new lastSeq.
	i := 0
	for ; i < len(p.waiters); i++ {
		if p.waiters[i].seq > p.lastSeq {
			break
		}
		close(p.waiters[i].ch)
	}
	if i > 0 {
		p.waiters = p.waiters[i:]
	}
}

func (p *Projector) fail(seq uint64, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failedErr == nil {
		p.failedSeq = seq
		p.failedErr = fmt.Errorf("%w at seq %d: %w", ErrProjectionFailed, seq, err)
		if p.started && p.startupEndedAt.IsZero() {
			p.startupEndedAt = time.Now()
		}
		close(p.failedCh)
	}
	for _, w := range p.waiters {
		close(w.ch)
	}
	p.waiters = nil
}

// Run starts the consumer + apply loop. Blocks until ctx is cancelled.
// Returns the context's error on shutdown.
func (p *Projector) Run(ctx context.Context) (runErr error) {
	defer func() {
		if runErr != nil && !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, context.DeadlineExceeded) {
			p.fail(0, runErr)
		}
	}()
	startedAt := time.Now()
	p.mu.Lock()
	p.started = true
	if p.startupStartedAt.IsZero() {
		p.startupStartedAt = startedAt
	}
	p.mu.Unlock()

	target, err := p.currentTarget(ctx)
	if err != nil {
		return fmt.Errorf("read projection startup target: %w", err)
	}
	if err := p.restoreForRun(ctx, target.seq); err != nil {
		return err
	}
	p.setStartupTarget(target.seq)

	consumerConfig := jetstream.OrderedConsumerConfig{
		FilterSubjects:    p.replaySubjects,
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		InactiveThreshold: projectionConsumerInactiveThreshold,
	}
	p.mu.Lock()
	restoredSeq := p.restoredSeq
	p.mu.Unlock()
	if restoredSeq > 0 {
		consumerConfig.DeliverPolicy = jetstream.DeliverByStartSequencePolicy
		consumerConfig.OptStartSeq = restoredSeq + 1
	}
	cons, err := p.stream.OrderedConsumer(ctx, consumerConfig)
	if err != nil {
		return fmt.Errorf("create ordered consumer: %w", err)
	}

	// Use Consume(handler) — NOT Messages() iterator. The iterator path
	// has an idle-cost behaviour in the SDK that adds ~5s per process to
	// our e2e test runtime (measured at 6× slowdown on membership-heavy
	// flows), even when the stream is empty. Consume(handler) on the
	// same OrderedConsumer keeps all of OC's guarantees (stream-order
	// delivery, gap detection, automatic reset) and is steady-state
	// quiet when idle. See the perf-investigation notes accompanying
	// this change.
	cc, err := cons.Consume(p.handleMessage,
		jetstream.PullMaxBytes(projectionPullMaxBytes),
		jetstream.ConsumeErrHandler(p.handleConsumeErr),
	)
	if err != nil {
		return fmt.Errorf("start consume: %w", err)
	}
	defer cc.Stop()
	p.maybeCompleteStartup(time.Now())

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-p.failedCh:
		if err := p.Err(); err != nil {
			return err
		}
		return ErrProjectionFailed
	}
}

// handleMessage is the per-event callback wired into the OrderedConsumer's
// Consume handler. It is invoked from a single goroutine the SDK owns, in
// stream order — matching the Projection.Apply concurrency contract.
//
// Errors from the projection's Apply mark the projector as failed. Waiters
// for the failed sequence (or later) return ErrProjectionFailed instead of
// reporting read-your-writes success against state that did not apply.
func (p *Projector) handleMessage(msg jetstream.Msg) {
	p.mu.Lock()
	failed := p.failedErr != nil
	p.mu.Unlock()
	if failed {
		return
	}

	seq, err := streamSequenceFromMsg(msg)
	if err != nil {
		p.logger.Error("Projection message metadata failed", "subject", msg.Subject(), "error", err)
		p.fail(0, fmt.Errorf("message metadata for subject %q: %w", msg.Subject(), err))
		return
	}

	if !p.consumesSubject(msg.Subject()) {
		return
	}
	if p.shouldSkipRestored(seq) {
		return
	}

	var event corev1.Event
	if err := proto.Unmarshal(msg.Data(), &event); err != nil {
		err = fmt.Errorf("unmarshal event on subject %q: %w", msg.Subject(), err)
		failureSeq := p.pendingStartupBatchFirstSequence(seq)
		p.logger.Error("Projection decode failed",
			"subject", msg.Subject(),
			"seq", seq,
			"error", err)
		p.fail(failureSeq, err)
		return
	}

	failureSeq, err := p.apply(&event, seq)
	if err != nil {
		p.logger.Error("Projection Apply failed",
			"subject", msg.Subject(),
			"seq", seq,
			"event_id", event.GetId(),
			"error", err)
		p.fail(failureSeq, err)
		return
	}

	p.maybeCompleteStartup(time.Now())
}

func (p *Projector) apply(event *corev1.Event, seq uint64) (uint64, error) {
	p.applyMu.Lock()
	defer p.applyMu.Unlock()
	if p.shouldSkipRestored(seq) {
		return 0, nil
	}
	if projection, ok := p.proj.(StartupBatchProjection); ok && p.shouldBatchStartup(seq) {
		p.startupBatch = append(p.startupBatch, SequencedEvent{Event: event, Sequence: seq})
		if len(p.startupBatch) < p.startupBatchSize && seq < p.startupTargetSequence() {
			return 0, nil
		}
		firstSeq := p.startupBatch[0].Sequence
		lastSeq := p.startupBatch[len(p.startupBatch)-1].Sequence
		messageCount := uint64(len(p.startupBatch))
		if err := projection.ApplyStartupBatch(p.startupBatch); err != nil {
			return firstSeq, err
		}
		p.startupBatch = p.startupBatch[:0]
		p.countStartupMessages(messageCount)
		p.advance(lastSeq)
		return 0, nil
	}
	if err := p.proj.Apply(event, seq); err != nil {
		return seq, err
	}
	p.countStartupMessages(1)
	p.advance(seq)
	return 0, nil
}

func (p *Projector) shouldBatchStartup(seq uint64) bool {
	if p.startupBatchSize <= 1 {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.started && p.startupEndedAt.IsZero() && seq <= p.startupTargetSeq
}

func (p *Projector) startupTargetSequence() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.startupTargetSeq
}

func (p *Projector) pendingStartupBatchFirstSequence(fallback uint64) uint64 {
	p.applyMu.Lock()
	defer p.applyMu.Unlock()
	if len(p.startupBatch) > 0 {
		return p.startupBatch[0].Sequence
	}
	return fallback
}

func (p *Projector) shouldSkipRestored(seq uint64) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.restoredSeq > 0 && seq <= p.restoredSeq
}

func (p *Projector) countStartupMessages(count uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started && p.startupEndedAt.IsZero() {
		p.startupMessages += count
	}
}

func (p *Projector) maybeCompleteStartup(now time.Time) {
	p.mu.Lock()
	shouldLog := false
	shouldCompleteReplay := false
	var duration time.Duration
	var targetSeq, lastSeq, messages uint64
	var projectionKey string
	if p.started && p.startupEndedAt.IsZero() && p.lastSeq >= p.startupTargetSeq {
		p.startupEndedAt = now
		p.startupCompleted = true
		shouldCompleteReplay = true
	}
	if p.started && p.startupCompleted && !p.startupLogged {
		p.startupLogged = true
		shouldLog = true
		duration = p.startupEndedAt.Sub(p.startupStartedAt)
		targetSeq = p.startupTargetSeq
		lastSeq = p.lastSeq
		messages = p.startupMessages
		projectionKey = p.checkpointKey
		if projectionKey == "" {
			projectionKey = p.snapshotKey
		}
	}
	p.mu.Unlock()

	if shouldCompleteReplay {
		if projection, ok := p.proj.(StartupReplayCompleter); ok {
			projection.CompleteStartupReplay()
		}
	}

	if shouldLog {
		var rate float64
		if seconds := duration.Seconds(); seconds > 0 {
			rate = float64(messages) / seconds
		}
		p.logger.Info("Projection startup complete",
			"projection", projectionKey,
			"duration", duration,
			"messages", messages,
			"messages_per_second", rate,
			"last_seq", lastSeq,
			"target_seq", targetSeq,
			"subjects", p.subjects,
		)
	}
}

// handleConsumeErr is invoked by the SDK when the OrderedConsumer's
// background machinery hits a transient problem (missed heartbeat,
// reset attempt, etc.). OrderedConsumer recovers internally; we log
// and stay running.
func (p *Projector) handleConsumeErr(_ jetstream.ConsumeContext, err error) {
	p.logger.Warn("Projection consumer error (auto-recovering)", "error", err)
}

func (p *Projector) setStartupTarget(seq uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.startupTargetSeq = seq
}

func (p *Projector) restoreForRun(ctx context.Context, targetSeq uint64) error {
	coldRestore := func() error {
		if projection, ok := p.proj.(SnapshotProjection); ok {
			if err := projection.Restore(nil); err != nil {
				return fmt.Errorf("restore empty projection: %w", err)
			}
		}
		p.resetRestoreState()
		return nil
	}

	p.mu.Lock()
	source := p.snapshotSource
	checkpointKey := p.checkpointKey
	key := p.snapshotKey
	contractID := p.snapshotContractID
	streamIdentity := p.snapshotStreamID
	loadTimeout := p.snapshotLoadTimeout
	p.mu.Unlock()
	if checkpointKey != "" {
		return p.restoreCheckpointForRun(ctx, targetSeq)
	}
	if source == nil {
		return coldRestore()
	}
	streamName := ""
	if info := p.stream.CachedInfo(); info != nil {
		streamName = info.Config.Name
	}
	if loadTimeout <= 0 {
		loadTimeout = projectionSnapshotLoadTimeout
	}
	loadCtx, cancelLoad := context.WithTimeout(ctx, loadTimeout)
	defer cancelLoad()
	snapshot, err := source.LoadProjectionSnapshot(loadCtx, ProjectionSnapshotLoadRequest{
		ProjectionKey:  key,
		ContractID:     contractID,
		StreamName:     streamName,
		StreamIdentity: streamIdentity,
		MaxCutoff:      targetSeq,
	})
	if err != nil {
		p.logger.Info("Projection snapshot unavailable; replaying EVT",
			"projection", key,
			"stage", "restore",
			"error", err)
		return coldRestore()
	}
	if snapshot.CutoffSequence > targetSeq {
		p.logger.Warn("Projection snapshot cutoff rejected; replaying EVT",
			"projection", key,
			"stage", "restore_validate",
			"generation_id", snapshot.GenerationID,
			"cutoff_seq", snapshot.CutoffSequence,
			"target_seq", targetSeq)
		return coldRestore()
	}
	projection, ok := p.proj.(SnapshotProjection)
	if !ok {
		return fmt.Errorf("projection %q no longer supports snapshots", key)
	}
	if err := projection.Restore(snapshot.Payload); err != nil {
		p.logger.Warn("Projection snapshot restore failed; replaying EVT",
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
	// Restore runs after markStarted, so boot-time callers may already be
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

func streamSequenceFromMsg(msg jetstream.Msg) (uint64, error) {
	return streamSequenceFromReply(msg.Reply())
}

func streamSequenceFromReply(reply string) (uint64, error) {
	const jsAckPrefix = "$JS.ACK."
	if len(reply) < len(jsAckPrefix) || reply[:len(jsAckPrefix)] != jsAckPrefix {
		return 0, fmt.Errorf("invalid JetStream ACK reply subject")
	}

	var v1Start, v1End int
	var v2Start, v2End int
	tokenStart := 0
	tokenIndex := 0
	for i := 0; i <= len(reply); i++ {
		if i != len(reply) && reply[i] != '.' {
			continue
		}
		switch tokenIndex {
		case 5:
			v1Start, v1End = tokenStart, i
		case 7:
			v2Start, v2End = tokenStart, i
		}
		tokenIndex++
		tokenStart = i + 1
	}

	switch {
	case tokenIndex == 9:
		return parseAckSequenceToken(reply[v1Start:v1End])
	case tokenIndex >= 11:
		return parseAckSequenceToken(reply[v2Start:v2End])
	default:
		return 0, fmt.Errorf("invalid JetStream ACK reply subject")
	}
}

func parseAckSequenceToken(token string) (uint64, error) {
	if token == "" {
		return 0, fmt.Errorf("invalid JetStream ACK stream sequence")
	}
	var n uint64
	for i := 0; i < len(token); i++ {
		c := token[i]
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("invalid JetStream ACK stream sequence")
		}
		digit := uint64(c - '0')
		if n > (^uint64(0)-digit)/10 {
			return 0, fmt.Errorf("invalid JetStream ACK stream sequence")
		}
		n = n*10 + digit
	}
	return n, nil
}

package events

import (
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sync/errgroup"
)

// ErrProjectionFailed marks a projector that stopped because it could not
// decode or apply an event, restore its state, or run its consumer.
// Projector.Err and ProjectorStatus.Err wrap it.
var ErrProjectionFailed = errors.New("projection failed")

// ErrProjectionSubjectNotConsumed is returned when a caller asks a projector
// to wait for a subject outside the projection's declared filter set.
var ErrProjectionSubjectNotConsumed = errors.New("projection does not consume subject")

// ErrProjectionSequenceSubjectMismatch is returned when a caller asks a
// projector to wait for a sequence that belongs to a different subject than the
// one supplied by the caller.
var ErrProjectionSequenceSubjectMismatch = errors.New("projection wait sequence subject mismatch")

// ErrProjectorAlreadyStarted is returned when Run is called more than once on
// the same projector. A projector owns one ordered consumer lifecycle and
// cannot be restarted after its context is cancelled or its run fails.
var ErrProjectorAlreadyStarted = errors.New("projector already started")

// Projection replay is a sequential bulk read. NATS defaults to a 500-message
// client buffer, which turns histories of many small event records into many
// latency-bound pull requests on a remote JetStream cluster. A byte window
// keeps those pulls large while bounding client-side memory.
const (
	projectionPullMaxBytes        = 16 * 1024 * 1024
	projectionSnapshotLoadTimeout = 15 * time.Second
	// Ordered pull consumers cannot issue another pull while a synchronous
	// projection Apply is running. Keep their cleanup window comfortably above
	// slow disk-backed commits so NATS cannot delete a live projector consumer.
	projectionConsumerInactiveThreshold = 5 * time.Minute
	projectionConsumerCleanupTimeout    = 2 * time.Second
	// projectionConsumerStopTimeout sets how long Run waits for the consume
	// goroutine to stop by itself before it stops the iterator.
	projectionConsumerStopTimeout = 2 * time.Second
	// While startup is incomplete, Run checks this often whether a deleted or
	// expired startup target prevents completion. Each check reads the last
	// message of every projection subject.
	projectionStartupReconcileInterval = 5 * time.Second
	// DefaultProjectorStallAfter is the stall threshold when
	// ProjectorOptions.StallAfter is zero.
	DefaultProjectorStallAfter = 5 * time.Minute
	// projectionSourceCheckInterval sets how often Run reads the newest
	// matching stream sequence, so that a consumer that stops delivering
	// still shows pending work.
	projectionSourceCheckInterval = 30 * time.Second
	projectionSourceCheckTimeout  = 10 * time.Second
	// projectionWaitValidationDelay sets how long WaitFor waits before it
	// reads the target message to make sure that the wait can end. Most waits
	// end before this delay, so they do not read the stream at all.
	projectionWaitValidationDelay = 100 * time.Millisecond
)

// ProjectorOperation names the work that a projector does at one moment.
// The values are stable, low-cardinality labels for diagnostics and metrics.
type ProjectorOperation string

const (
	// ProjectorIdle means that the projector waits for events.
	ProjectorIdle ProjectorOperation = ""
	// ProjectorRestoring means that Run restores a checkpoint or snapshot,
	// or resets state for a cold replay.
	ProjectorRestoring ProjectorOperation = "restore"
	// ProjectorApplying means that one event is prepared and committed.
	ProjectorApplying ProjectorOperation = "apply"
	// ProjectorApplyingStartupBatch means that a startup batch is applied.
	ProjectorApplyingStartupBatch ProjectorOperation = "startup_batch"
	// ProjectorApplyingLiveBatch means that a live batch is applied.
	ProjectorApplyingLiveBatch ProjectorOperation = "live_batch"
)

type decodedEvent interface {
	diagnosticID() string
}

type typedDecodedEvent[E any] struct {
	event E
	id    string
}

func (e typedDecodedEvent[E]) diagnosticID() string {
	return e.id
}

type sequencedDecodedEvent struct {
	event    decodedEvent
	sequence uint64
}

// Projector runs the consumer + apply loop for one projection. A Projector is
// single-run: create a new instance when the consumer lifecycle must restart.
type Projector struct {
	js      jetstream.JetStream
	stream  jetstream.Stream
	proj    SubjectProjection
	logger  *slog.Logger
	applyMu sync.Mutex
	decode  func([]byte) (decodedEvent, error)
	// prepare prepares one event under applyMu. A nil mutation means that
	// preparation already applied the event.
	prepare func(decodedEvent, string, uint64) (PreparedMutation, error)
	// applyStartupBatch applies batched startup events. It is nil unless the
	// projection implements StartupBatchEventProjection.
	applyStartupBatch func([]sequencedDecodedEvent) error
	applyLiveBatch    func([]sequencedDecodedEvent) error
	liveBatchSize     int
	liveBatchDelay    time.Duration
	liveBatch         []sequencedDecodedEvent
	liveFlushTimer    *time.Timer // Armed only while liveBatch is non-empty.
	// handledSeq is the stream sequence of the last message that
	// handleMessage accepted. Only the consume goroutine uses it.
	handledSeq uint64

	subjects        []string
	replaySubjects  []string
	subjectMatchers []compiledSubjectFilter

	// Consumer identity is application-owned diagnostic text. It does not
	// change after construction.
	consumerName        string
	consumerDescription string

	mu        sync.Mutex
	lastSeq   uint64
	waiters   []seqWaiter
	failedSeq uint64
	failedErr error
	failedCh  chan struct{}
	startupCh chan struct{}
	// started flips true the first time Run is invoked and stays true
	// for the projector's lifetime. Run uses it to reject a second call,
	// and the startup bookkeeping uses it to tell startup replay from
	// live delivery. Status reports it.
	started bool

	startupStartedAt time.Time
	startupTargetSeq uint64
	startupEndedAt   time.Time
	startupCompleted bool
	startupMessages  uint64
	startupBatchSize int
	startupBatch     []sequencedDecodedEvent
	// startupReconcileInterval sets how often Run checks whether the retained
	// startup history is complete. Zero uses
	// projectionStartupReconcileInterval.
	startupReconcileInterval time.Duration

	// snapshots and the checkpoint fields do not change after construction.
	snapshots projectorSnapshots
	// snapshotRunIdentity is the stream identity bound to the current run.
	// CaptureSnapshot rejects a stream whose identity differs from it.
	snapshotRunIdentity  string
	restoredSeq          uint64
	restoredGenerationID string
	snapshotRestored     bool
	latestSnapshotSeq    uint64
	latestSnapshotAt     time.Time

	checkpointKey              string
	checkpointContractID       string
	checkpointIdentityResolver StreamIdentityResolver
	checkpointRestored         bool
	checkpointCutoffSeq        uint64

	// Freshness tracking. All fields are guarded by mu, so Status never
	// waits for applyMu or a projection lock.
	//
	// deliveredSeq is the highest consumed sequence delivered to the handler.
	// sourceSeq is the newest matching sequence from the latest source check;
	// it can decrease when the newest event is deleted. pendingSince is the
	// later of the last committed progress and the moment work became
	// pending; it is zero while the projector is caught up.
	deliveredSeq       uint64
	sourceSeq          uint64
	pendingSince       time.Time
	lastCommitAt       time.Time
	lastCommitDuration time.Duration
	operation          ProjectorOperation
	operationStartedAt time.Time
	stallAfter         time.Duration
	// sourceCheckInterval sets how often Run reads the source sequence. Zero
	// uses projectionSourceCheckInterval.
	sourceCheckInterval time.Duration
}

// ProjectorStatus is a concurrency-safe snapshot of a projector's
// lifecycle state. Operators use it for diagnostics; application readiness
// uses Err to surface fatal projection failures. A projector has failed
// exactly when Err is not nil.
type ProjectorStatus struct {
	Started bool
	LastSeq uint64

	// StartupTargetSeq is the last matching sequence captured when Run
	// started. If that event is deleted or expires before the projector
	// applies it, startup completes at the last applied sequence and this
	// value is lowered to that sequence.
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

	// ObservedSeq is the highest matching source sequence that the
	// projector knows of: the newest delivered event, the startup target, or
	// the result of the latest periodic source check. Work is pending while
	// ObservedSeq is greater than LastSeq.
	ObservedSeq uint64
	// LastCommitAt is when the projector last committed progress. It is zero
	// before the first commit in this process.
	LastCommitAt time.Time
	// LastCommitDuration is how long the last committed apply or batch took.
	LastCommitDuration time.Duration
	// CurrentOperation is the work that runs now, or ProjectorIdle.
	CurrentOperation ProjectorOperation
	// CurrentOperationDuration is how long CurrentOperation has run.
	CurrentOperationDuration time.Duration
	// PendingFor is how long pending work has waited since the later of the
	// last committed progress and the moment the work became pending. It is
	// zero when the projector is caught up.
	PendingFor time.Duration
	// Stalled is true when a running projector without failure has pending
	// work and PendingFor reached the stall threshold. An idle, caught-up
	// projector is never stalled, and each commit restarts PendingFor, so a
	// slow but advancing replay is not stalled.
	Stalled bool

	// Err is the fatal error that stopped the projector, or nil. It wraps
	// ErrProjectionFailed and the cause: a decode, apply, restore, or other
	// run error.
	Err error
	// FailedSeq is the stream sequence that the projector could not decode
	// or apply. It is zero while Err is nil, and when the failure has no
	// sequence, for example a restore or run failure.
	FailedSeq uint64
}

type seqWaiter struct {
	seq uint64
	ch  chan struct{}
}

// ProjectorOptions configures a Projector at construction. The zero value is
// valid: it discards diagnostics, uses the SDK's consumer name, and replays the
// complete retained history on every run.
type ProjectorOptions struct {
	// Logger receives diagnostics. Nil discards them.
	Logger *slog.Logger
	// ConsumerName labels the ephemeral consumer. It must contain at most 64
	// ASCII letters, digits, hyphens, or underscores. Run adds a random
	// suffix so replicas never share a consumer. Empty uses the SDK's name.
	ConsumerName string
	// ConsumerDescription is stored in consumer metadata because the
	// ordered-consumer API has no description field.
	//
	// The consumer labels must not contain personal data or secrets. They do
	// not change snapshot identities or create a durable consumer.
	ConsumerDescription string
	// Snapshots enables snapshot restore. It cannot be combined with
	// Checkpoint.
	Snapshots *SnapshotOptions
	// Checkpoint enables local checkpoint restore. It cannot be combined
	// with Snapshots.
	Checkpoint *CheckpointOptions
	// StallAfter is how long pending work can wait without committed
	// progress before Status reports the projector as stalled. Zero uses
	// DefaultProjectorStallAfter. It must be longer than the live batch
	// delay. A negative value is invalid.
	StallAfter time.Duration
}

// NewDecodedProjector binds a non-nil pointer projection and decoder to a
// stream. It does not start the consumer; call Run for that. Requiring a
// pointer prevents the projector and application read side from receiving
// separate value copies of mutable projection state. The decoder is the only
// boundary between opaque stored records and application event values.
//
// It panics when a required argument is nil or the projection is not a
// pointer. It returns an error when opts are invalid or when the snapshot
// stream identity cannot be resolved.
func NewDecodedProjector[E any](
	js jetstream.JetStream,
	stream jetstream.Stream,
	proj EventProjection[E],
	decoder EventDecoder[E],
	opts ProjectorOptions,
) (*Projector, error) {
	// Apply changes state during preparation. Both run under the apply
	// barrier, so the result equals a prepared mutation with no commit step.
	p, err := newProjector(js, stream, proj, decoder, opts, func(event E, _ string, seq uint64) (PreparedMutation, error) {
		return nil, proj.Apply(event, seq)
	})
	if err != nil {
		return nil, err
	}
	if projection, ok := proj.(StartupBatchEventProjection[E]); ok {
		if size := projection.StartupBatchSize(); size > 1 {
			p.startupBatchSize = size
			p.applyStartupBatch = func(items []sequencedDecodedEvent) error {
				typed := make([]SequencedEventOf[E], len(items))
				for i, item := range items {
					typed[i] = SequencedEventOf[E]{
						Event:    item.event.(typedDecodedEvent[E]).event,
						Sequence: item.sequence,
					}
				}
				return projection.ApplyStartupBatch(typed)
			}
		}
	}
	if projection, ok := proj.(LiveBatchEventProjection[E]); ok {
		p.liveBatchSize = projection.LiveBatchSize()
		p.liveBatchDelay = projection.LiveBatchMaxDelay()
		if p.liveBatchSize < 1 || p.liveBatchDelay <= 0 {
			return nil, fmt.Errorf("live batch size and delay must be positive")
		}
		p.applyLiveBatch = func(items []sequencedDecodedEvent) error {
			typed := make([]SequencedEventOf[E], len(items))
			for i, item := range items {
				typed[i] = SequencedEventOf[E]{Event: item.event.(typedDecodedEvent[E]).event, Sequence: item.sequence}
			}
			return projection.ApplyLiveBatch(typed)
		}
	}
	return p, nil
}

// NewDecodedPreparedProjector binds a prepared projection and decoder to one
// ordered projector lifecycle. The projector prepares the event while it holds
// the apply barrier, commits only after preparation succeeds, and then advances
// the applied sequence before it releases the barrier. A projection that
// implements SubjectEventReducer prepares with the delivered subject.
//
// It panics and returns errors like NewDecodedProjector.
func NewDecodedPreparedProjector[E any](
	js jetstream.JetStream,
	stream jetstream.Stream,
	proj PreparedEventProjection[E],
	decoder EventDecoder[E],
	opts ProjectorOptions,
) (*Projector, error) {
	prepare := func(event E, _ string, seq uint64) (PreparedMutation, error) {
		return proj.Prepare(event, seq)
	}
	if projection, ok := proj.(SubjectEventReducer[E]); ok {
		prepare = projection.PrepareSubject
	}
	return newProjector(js, stream, proj, decoder, opts, prepare)
}

// newProjector validates its arguments and options and builds the Projector
// that prepares every decoded event with prepare.
func newProjector[E any](
	js jetstream.JetStream,
	stream jetstream.Stream,
	proj SubjectProjection,
	decoder EventDecoder[E],
	opts ProjectorOptions,
	prepare func(event E, subject string, seq uint64) (PreparedMutation, error),
) (*Projector, error) {
	if isNilProjection(proj) {
		panic("events: projector requires a non-nil projection")
	}
	if reflect.ValueOf(proj).Kind() != reflect.Pointer {
		panic("events: projector requires a pointer projection")
	}
	if decoder == nil {
		panic("events: projector requires a non-nil event decoder")
	}
	if js == nil || stream == nil {
		panic("events: projector requires a JetStream context and a stream")
	}
	if err := validateConsumerIdentity(opts.ConsumerName, opts.ConsumerDescription); err != nil {
		return nil, err
	}
	if opts.Snapshots != nil && opts.Checkpoint != nil {
		return nil, fmt.Errorf("projection cannot use both snapshots and a local checkpoint")
	}
	if opts.StallAfter < 0 {
		return nil, fmt.Errorf("projection stall threshold must not be negative")
	}
	stallAfter := opts.StallAfter
	if stallAfter == 0 {
		stallAfter = DefaultProjectorStallAfter
	}
	subjects := slices.Clone(proj.Subjects())
	p := &Projector{
		js:                  js,
		stream:              stream,
		proj:                proj,
		logger:              normalizeLogger(opts.Logger),
		consumerName:        opts.ConsumerName,
		consumerDescription: opts.ConsumerDescription,
		decode: func(data []byte) (decodedEvent, error) {
			event, err := decoder(data)
			if err != nil {
				return nil, err
			}
			return typedDecodedEvent[E]{event: event.Event, id: event.ID}, nil
		},
		prepare: func(event decodedEvent, subject string, seq uint64) (PreparedMutation, error) {
			return prepare(event.(typedDecodedEvent[E]).event, subject, seq)
		},
		subjects:        subjects,
		replaySubjects:  slices.Clone(projectionReplaySubjects(proj, subjects)),
		subjectMatchers: compileSubjectFilters(subjects),
		failedCh:        make(chan struct{}),
		startupCh:       make(chan struct{}),
		stallAfter:      stallAfter,
	}
	if opts.Snapshots != nil {
		if err := p.configureSnapshots(*opts.Snapshots); err != nil {
			return nil, err
		}
	}
	if opts.Checkpoint != nil {
		if err := p.configureCheckpoint(*opts.Checkpoint); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// Status returns the projector's current lifecycle state. Safe to call from
// any goroutine. It takes only the short status lock, never the apply
// barrier, so it returns while a commit blocks.
func (p *Projector) Status() ProjectorStatus {
	return p.statusAt(time.Now())
}

func (p *Projector) statusAt(now time.Time) ProjectorStatus {
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
		ObservedSeq:          p.observedSeqLocked(),
		LastCommitAt:         p.lastCommitAt,
		LastCommitDuration:   p.lastCommitDuration,
		CurrentOperation:     p.operation,
	}
	if p.operation != ProjectorIdle {
		status.CurrentOperationDuration = now.Sub(p.operationStartedAt)
	}
	if !p.pendingSince.IsZero() {
		status.PendingFor = now.Sub(p.pendingSince)
		status.Stalled = p.started && p.failedErr == nil && status.PendingFor >= p.stallAfter
	}
	if !p.startupStartedAt.IsZero() {
		startupEndsAt := p.startupEndedAt
		if startupEndsAt.IsZero() {
			startupEndsAt = now
		}
		status.StartupDuration = startupEndsAt.Sub(p.startupStartedAt)
	}
	if p.failedErr != nil {
		status.FailedSeq = p.failedSeq
		status.Err = p.failedErr
	}
	return status
}

// Err returns the fatal error that stopped the projector, or nil. It equals
// Status().Err.
func (p *Projector) Err() error {
	return p.Status().Err
}

// LastSeq returns the highest matching ordered stream sequence the projector
// has applied. Safe to call from any goroutine.
func (p *Projector) LastSeq() uint64 {
	return p.Status().LastSeq
}

// WithReadBarrier runs read while event application is paused. sequence is the
// exact event-log sequence represented by every model owned by this projector.
// The callback must do only bounded in-memory work and must not call this
// method recursively.
func (p *Projector) WithReadBarrier(read func(sequence uint64) error) error {
	if read == nil {
		return fmt.Errorf("projection read callback is nil")
	}
	p.applyMu.Lock()
	defer p.applyMu.Unlock()
	p.mu.Lock()
	sequence := p.lastSeq
	failedErr := p.failedErr
	p.mu.Unlock()
	if failedErr != nil {
		return failedErr
	}
	return read(sequence)
}

func (p *Projector) ownsProjection(projection SubjectProjection) bool {
	if sameProjection(p.proj, projection) {
		return true
	}
	owner, ok := p.proj.(ProjectionOwner)
	return ok && owner.OwnsProjection(projection)
}

// Started reports whether Run has entered its body — i.e. whether
// the projector's consumer is being set up / has been set up. Used by
// test helpers (and lifecycle code) that need to wait for projectors
// to come online before issuing reads against the projection.
func (p *Projector) Started() bool {
	return p.Status().Started
}

// WaitForStartup blocks until the projector has applied its captured startup
// history and completed any StartupReplayCompleter hook. It returns a fatal
// startup failure or the caller's context error instead of reporting readiness.
//
// Run must be active for startup to advance. Calling WaitForStartup before Run
// is valid and blocks until Run starts, fails, or the context ends.
func (p *Projector) WaitForStartup(ctx context.Context) error {
	select {
	case <-p.startupCh:
		return nil
	default:
	}

	p.mu.Lock()
	if p.failedErr != nil {
		err := p.failedErr
		p.mu.Unlock()
		return err
	}
	startupCh := p.startupCh
	failedCh := p.failedCh
	p.mu.Unlock()

	select {
	case <-startupCh:
		return nil
	case <-failedCh:
		select {
		case <-startupCh:
			return nil
		default:
		}
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.failedErr
	case <-ctx.Done():
		return ctx.Err()
	}
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

// WaitFor blocks until LastSeq() >= pos.Seq or ctx is done.
//
// Used by writers that need read-your-writes consistency: capture the stream
// position for the write target, pass it here, then read from the projection.
// The stream sequence must belong to pos.SubjectFilter, and the sequence's
// actual subject must match one of this projector's subject filters.
//
// A call whose LastSeq() is already at or beyond pos.Seq returns without a
// stream read. Otherwise it registers a waiter and blocks. When the wait has
// not ended after a short delay (at once when ctx has less than twice that
// delay left), WaitFor reads the message at pos.Seq from the
// stream leader. It returns an error when that message does not exist, does
// not match pos.SubjectFilter, or has a subject that the projector does not
// consume, because the projector cannot reach such a position. The read does
// not delay the end of the wait: when the projector reaches pos.Seq, fails, or
// ctx ends while the read runs, WaitFor returns at once and cancels the read.
//
// WaitFor always rejects, without a stream read, a pos.SubjectFilter that
// cannot match a subject that the projector consumes: an exact subject that
// the projector does not consume, or a wildcard filter that has no subject in
// common with the projector's subject filters. Other wrong positions are
// rejected only while the projector has not reached pos.Seq: a sequence whose
// subject does not match pos.SubjectFilter, a sequence on a subject that the
// projector does not consume, and a sequence that does not exist. When the
// projector is already past such a position, WaitFor returns nil. The
// projection then shows every consumed event through pos.Seq, so the result
// is not stale, but the caller's mistake is not reported.
//
// Precondition: the projector's Run loop is expected to be active before any
// code reaches WaitFor. Applications must order projector startup before
// mutations that require read-your-writes consistency. Calling WaitFor before
// Run starts blocks until Run advances to the target or the context ends;
// silently skipping the wait could expose stale derived state.
func (p *Projector) WaitFor(ctx context.Context, pos StreamPosition) error {
	if pos.IsZero() {
		return nil
	}
	if !p.filterCanMatchConsumed(pos.SubjectFilter) {
		return fmt.Errorf("%w: subject filter %q not matched by filters %v",
			ErrProjectionSubjectNotConsumed, pos.SubjectFilter, p.subjects)
	}
	return p.waitForSeq(ctx, pos.Seq, func(ctx context.Context) error {
		return p.validateSeqSubject(ctx, pos)
	})
}

// filterCanMatchConsumed reports whether some subject can match both filter
// and one of the projector's subject filters. It reads no stream state.
func (p *Projector) filterCanMatchConsumed(filter string) bool {
	if !subjectHasWildcard(filter) {
		return p.consumesSubject(filter)
	}
	for _, subject := range p.subjects {
		if subjectFiltersOverlap(filter, subject) {
			return true
		}
	}
	return false
}

// waitForSeq blocks until LastSeq() >= seq, the projector fails at or before
// seq, or ctx ends. A non-nil validate starts once, in its own goroutine,
// when the wait has not ended after validationDelay(ctx). Its error ends the
// wait. The wait does not wait for validate: when the wait ends first,
// waitForSeq cancels the context of validate and returns.
func (p *Projector) waitForSeq(ctx context.Context, seq uint64, validate func(context.Context) error) error {
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
	// Keep waiters sorted ascending by seq so advance() can release them
	// in order and stop scanning at the first unmet seq.
	i, _ := slices.BinarySearchFunc(p.waiters, seq, func(w seqWaiter, target uint64) int {
		return cmp.Compare(w.seq, target)
	})
	p.waiters = slices.Insert(p.waiters, i, seqWaiter{seq: seq, ch: ch})
	p.mu.Unlock()

	var validationDue <-chan time.Time
	if validate != nil {
		timer := time.NewTimer(validationDelay(ctx))
		defer timer.Stop()
		validationDue = timer.C
	}
	// validation is nil until the read starts. A nil channel never receives,
	// so the select below ignores it before that.
	var validation chan error
	validationCtx, cancelValidation := context.WithCancel(ctx)
	defer cancelValidation()
	for {
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
		case <-validationDue:
			validationDue = nil
			validation = make(chan error, 1)
			go func() { validation <- validate(validationCtx) }()
		case err := <-validation:
			validation = nil
			if err != nil {
				p.removeWaiter(ch)
				return err
			}
		case <-ctx.Done():
			p.removeWaiter(ch)
			return ctx.Err()
		}
	}
}

// validationDelay returns how long a wait waits before it validates its
// position. A context with little time left validates at once, so that the
// delay cannot turn a prompt validation error into a deadline error.
func validationDelay(ctx context.Context) time.Duration {
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < 2*projectionWaitValidationDelay {
		return 0
	}
	return projectionWaitValidationDelay
}

// removeWaiter drops the waiter of an ended wait so that it does not leak.
// The advance path tolerates already-closed channels (it doesn't close
// twice), and a small scan here is fine — waiters lists are short.
func (p *Projector) removeWaiter(ch chan struct{}) {
	p.mu.Lock()
	p.waiters = slices.DeleteFunc(p.waiters, func(w seqWaiter) bool { return w.ch == ch })
	p.mu.Unlock()
}

func (p *Projector) validateConsumesSubject(subject string) error {
	if p.consumesSubject(subject) {
		return nil
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

// WaitForCurrent blocks until the projection has applied the latest
// stream message currently matching its subject filters. It is intended
// for diagnostics and sequencing: call it after the projector is
// running to ensure projection reads reflect the stream as of this call.
// It also waits for every event that the projector has already received.
// Such an event can be newer than the latest retained message when it was
// deleted or expired after delivery.
func (p *Projector) WaitForCurrent(ctx context.Context) error {
	target, err := p.currentTarget(ctx)
	if err != nil {
		return err
	}
	p.mu.Lock()
	seq := max(target.seq, p.deliveredSeq)
	p.mu.Unlock()
	if seq == 0 {
		return nil
	}
	return p.waitForSeq(ctx, seq, nil)
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

// targetForSubjects looks up the last message for every filter concurrently.
// Each lookup is a separate JetStream request, so sequential lookups would cost
// one broker round trip per filter.
func (p *Projector) targetForSubjects(ctx context.Context, subjects []string) (projectionTarget, error) {
	sequences := make([]uint64, len(subjects))
	lookups, lookupCtx := errgroup.WithContext(ctx)
	for i, subject := range subjects {
		lookups.Go(func() error {
			msg, err := p.stream.GetLastMsgForSubject(lookupCtx, subject)
			if err != nil {
				if errors.Is(err, jetstream.ErrMsgNotFound) {
					return nil
				}
				return fmt.Errorf("last msg for subject %q: %w", subject, err)
			}
			sequences[i] = msg.Sequence
			return nil
		})
	}
	if err := lookups.Wait(); err != nil {
		return projectionTarget{}, err
	}
	var target projectionTarget
	for _, sequence := range sequences {
		if sequence > target.seq {
			target = projectionTarget{seq: sequence}
		}
	}
	return target, nil
}

func projectionReplaySubjects(proj SubjectProjection, subjects []string) []string {
	if replay, ok := proj.(ReplaySubjectProjection); ok {
		return replay.ReplaySubjects()
	}
	return subjects
}

func (p *Projector) consumesSubject(subject string) bool {
	return matchesAnySubject(p.subjectMatchers, subject)
}

// advance updates lastSeq and releases any waiters that have now been
// reached. Restore calls it for a restored cutoff.
func (p *Projector) advance(seq uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.advanceLocked(seq, time.Now())
}

// commitDone ends the current apply operation, records its duration as
// committed progress, and advances to seq. The caller must hold applyMu.
func (p *Projector) commitDone(seq uint64) {
	now := time.Now()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.operation != ProjectorIdle {
		p.lastCommitDuration = now.Sub(p.operationStartedAt)
		p.operation = ProjectorIdle
	}
	p.advanceLocked(seq, now)
}

// startOperation records the work that starts now. The caller must hold
// applyMu, so operations never overlap.
func (p *Projector) startOperation(operation ProjectorOperation) {
	now := time.Now()
	p.mu.Lock()
	p.operation = operation
	p.operationStartedAt = now
	p.mu.Unlock()
}

// endOperation records that the current operation ended without commit.
func (p *Projector) endOperation() {
	p.mu.Lock()
	p.operation = ProjectorIdle
	p.mu.Unlock()
}

// observedSeqLocked returns the highest known matching source sequence. The
// caller must hold mu.
func (p *Projector) observedSeqLocked() uint64 {
	return max(p.deliveredSeq, p.sourceSeq, p.startupTargetSeq)
}

// updatePendingLocked starts the pending interval when work becomes pending
// and clears it when the projector is caught up. progress restarts the
// interval. The caller must hold mu.
func (p *Projector) updatePendingLocked(now time.Time, progress bool) {
	if p.observedSeqLocked() <= p.lastSeq {
		p.pendingSince = time.Time{}
		return
	}
	if progress || p.pendingSince.IsZero() {
		p.pendingSince = now
	}
}

// observeDelivered records a delivered event that the projector consumes.
func (p *Projector) observeDelivered(seq uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if seq > p.deliveredSeq {
		p.deliveredSeq = seq
		p.updatePendingLocked(time.Now(), false)
	}
}

// observeSource records the newest matching source sequence.
func (p *Projector) observeSource(seq uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sourceSeq = seq
	p.updatePendingLocked(time.Now(), false)
}

func (p *Projector) advanceLocked(seq uint64, now time.Time) {
	if seq > p.lastSeq {
		p.lastSeq = seq
		p.lastCommitAt = now
		p.updatePendingLocked(now, true)
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
	p.operation = ProjectorIdle
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

// validateConsumerIdentity checks the optional consumer labels. An empty name
// uses the SDK's name, so a description needs a name.
func validateConsumerIdentity(name, description string) error {
	if name == "" && description != "" {
		return fmt.Errorf("projection consumer description requires a consumer name")
	}
	if len(name) > 64 {
		return fmt.Errorf("projection consumer name must contain at most 64 ASCII letters, digits, hyphens, or underscores")
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return fmt.Errorf("projection consumer name must contain only ASCII letters, digits, hyphens, or underscores")
		}
	}
	return nil
}

// Run starts the consumer + apply loop once. Blocks until ctx is cancelled.
// Returns ErrProjectorAlreadyStarted for a repeated or concurrent call, and
// the context's error on shutdown. On exit it stops consumption and attempts
// to delete only its own ephemeral consumer. Cleanup failure does not replace
// the run error; inactivity expiry remains the fallback after a crash or loss
// of the NATS connection.
func (p *Projector) Run(ctx context.Context) (runErr error) {
	defer func() {
		if runErr != nil && !errors.Is(runErr, ErrProjectorAlreadyStarted) && !errors.Is(runErr, context.Canceled) && !errors.Is(runErr, context.DeadlineExceeded) {
			p.fail(0, runErr)
		}
	}()
	startedAt := time.Now()
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return ErrProjectorAlreadyStarted
	}
	p.started = true
	if p.startupStartedAt.IsZero() {
		p.startupStartedAt = startedAt
	}
	p.mu.Unlock()

	target, err := p.currentTarget(ctx)
	if err != nil {
		return fmt.Errorf("read projection startup target: %w", err)
	}
	p.applyMu.Lock()
	p.startOperation(ProjectorRestoring)
	restoreErr := p.restoreForRun(ctx, target.seq)
	p.endOperation()
	p.applyMu.Unlock()
	if restoreErr != nil {
		return restoreErr
	}
	p.setStartupTarget(target.seq)

	consumerConfig := jetstream.OrderedConsumerConfig{
		FilterSubjects:    p.replaySubjects,
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		InactiveThreshold: projectionConsumerInactiveThreshold,
	}
	if p.consumerName != "" {
		consumerConfig.NamePrefix = "projection-" + p.consumerName + "-" + rand.Text()
		consumerConfig.Metadata = map[string]string{
			"projection_name":        p.consumerName,
			"projection_description": p.consumerDescription,
		}
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
	// Registered before Stop so cleanup runs after consumption stops, including
	// when Messages fails after the physical consumer was created.
	defer p.deleteProjectionConsumer(cons)

	var liveTick <-chan time.Time
	if p.applyLiveBatch != nil {
		p.liveFlushTimer = time.NewTimer(p.liveBatchDelay)
		p.liveFlushTimer.Stop()
		defer p.liveFlushTimer.Stop()
		liveTick = p.liveFlushTimer.C
	}
	// Read with the Messages iterator, not with Consume(handler). After a
	// consumer reset, for example after a reconnect, Consume starts the new
	// subscription while the handler for the previous message can still run.
	// The new subscription then delivers the next message to a second handler
	// call, which can apply it first. The iterator resets the consumer inside
	// Next. One goroutine calls Next and handles each message before it calls
	// Next again, so delivery stays serial across resets.
	messages, err := cons.Messages(jetstream.PullMaxBytes(projectionPullMaxBytes))
	if err != nil {
		return fmt.Errorf("start consume: %w", err)
	}
	consumeCtx, stopConsume := context.WithCancel(ctx)
	consumeDone := make(chan struct{})
	var consumeErr error
	go func() {
		defer close(consumeDone)
		consumeErr = p.consume(consumeCtx, messages)
	}()
	// Run returns only after the consume goroutine stops, so no event is
	// applied after Run returns. The context ends a waiting Next. Stop runs
	// after the goroutine returns because nats.go v1.54 does not synchronize
	// Stop with the consumer reset inside Next. Only when the goroutine does
	// not stop in time, for example because the reset retries while NATS is
	// unavailable, does Stop run at the same time to cancel the reset. If a
	// reset completes at that moment, nats.go v1.54 races on its current
	// subscription, and the new subscription can stay open until the
	// connection closes. nats.go v1.55 removes the race.
	defer func() {
		stopConsume()
		select {
		case <-consumeDone:
		case <-time.After(projectionConsumerStopTimeout):
		}
		messages.Stop()
		<-consumeDone
	}()
	sourceChecks, stopSourceChecks := context.WithCancel(ctx)
	sourceChecksDone := make(chan struct{})
	go func() {
		defer close(sourceChecksDone)
		p.runSourceChecks(sourceChecks)
	}()
	defer func() {
		stopSourceChecks()
		<-sourceChecksDone
	}()
	p.maybeCompleteStartup(time.Now())

	// Startup normally completes when the target event is applied. If the
	// target is deleted or expires before delivery, no event can complete
	// startup, so check the retained history while startup is incomplete.
	interval := p.startupReconcileInterval
	if interval <= 0 {
		interval = projectionStartupReconcileInterval
	}
	reconcile := time.NewTicker(interval)
	defer reconcile.Stop()
	startupDone := p.startupCh
	handledSeq := ^uint64(0)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-p.failedCh:
			if err := p.Err(); err != nil {
				return err
			}
			return ErrProjectionFailed
		case <-consumeDone:
			if consumeErr != nil {
				return consumeErr
			}
			return ctx.Err()
		case <-liveTick:
			p.applyMu.Lock()
			p.flushLiveBatch()
			p.applyMu.Unlock()
			p.maybeCompleteStartup(time.Now())
		case <-startupDone:
			reconcile.Stop()
			startupDone = nil
		case <-reconcile.C:
			handledSeq = p.reconcileStartup(ctx, handledSeq)
		}
	}
}

// reconcileStartup completes startup when every retained event through the
// startup target has been handled, but the target itself was deleted or
// expired after Run captured it. Publication only appends after the target,
// so when the newest matching event is already handled, no retained startup
// event remains. It returns the handled sequence for the next check and reads
// the stream only after replay stopped advancing since the previous check.
func (p *Projector) reconcileStartup(ctx context.Context, previousHandledSeq uint64) uint64 {
	p.applyMu.Lock()
	handledSeq, pending := p.startupProgress()
	p.applyMu.Unlock()
	if !pending || handledSeq != previousHandledSeq {
		return handledSeq
	}
	current, err := p.currentTarget(ctx)
	if err != nil {
		if ctx.Err() == nil {
			p.logger.Debug("Projection startup check failed", "error", err)
		}
		return handledSeq
	}

	p.applyMu.Lock()
	handledSeq, pending = p.startupProgress()
	if !pending || current.seq > handledSeq {
		p.applyMu.Unlock()
		return handledSeq
	}
	if failureSeq, err := p.flushStartupBatch(); err != nil {
		p.logger.Error("Projection startup batch failed", "seq", failureSeq, "error", err)
		// Fail before the barrier opens so that no later event is applied.
		p.fail(failureSeq, err)
		p.applyMu.Unlock()
		return handledSeq
	}
	p.mu.Lock()
	// An event that is delivered but not yet applied is still in decode or
	// waits for applyMu. Its handling makes progress, so do not complete
	// startup before it. The check and the lowered target use one critical
	// section: an event delivered after it is past the lowered target.
	if p.deliveredSeq > p.lastSeq {
		p.mu.Unlock()
		p.applyMu.Unlock()
		return handledSeq
	}
	originalTarget := p.startupTargetSeq
	if p.startupTargetSeq > p.lastSeq {
		p.startupTargetSeq = p.lastSeq
	}
	p.updatePendingLocked(time.Now(), false)
	lastSeq := p.lastSeq
	p.mu.Unlock()
	// Complete before the barrier opens: an event that the consumer applies
	// next is already past the lowered target.
	summary, completed := p.completeStartupLocked(time.Now())
	p.applyMu.Unlock()

	p.logger.Info("Projection startup target is no longer retained; completing startup at last applied event",
		"original_target_seq", originalTarget,
		"last_seq", lastSeq,
		"subjects", p.subjects)
	if completed {
		p.logStartupComplete(summary)
	}
	return handledSeq
}

// startupProgress returns the highest sequence that is applied or waits in
// the startup batch, and whether startup is still incomplete without failure.
// The caller must hold applyMu.
func (p *Projector) startupProgress() (uint64, bool) {
	p.mu.Lock()
	handledSeq := p.lastSeq
	pending := p.startupEndedAt.IsZero() && p.failedErr == nil
	p.mu.Unlock()
	if n := len(p.startupBatch); n > 0 {
		handledSeq = p.startupBatch[n-1].sequence
	}
	return handledSeq, pending
}

// deleteProjectionConsumer reads the current name after Stop: the SDK can
// replace an ordered consumer during recovery. Never delete by a shared prefix
// or by the initial name, which may refer to an earlier SDK generation.
func (p *Projector) deleteProjectionConsumer(consumer jetstream.Consumer) {
	info := consumer.CachedInfo()
	if info == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), projectionConsumerCleanupTimeout)
	defer cancel()
	if err := p.stream.DeleteConsumer(ctx, info.Name); err != nil && !errors.Is(err, jetstream.ErrConsumerNotFound) && !errors.Is(err, jetstream.ErrStreamNotFound) {
		p.logger.Warn("Could not delete projection consumer; inactivity expiry remains enabled", "consumer", info.Name, "error", err)
	}
}

// consume reads the ordered consumer until ctx ends or the iterator stops.
// It is the only caller of handleMessage, and it calls handleMessage for one
// message at a time. The ordered consumer returns messages in stream order
// and resets itself inside Next, between two handleMessage calls. Thus each
// event is handled completely before the next one is delivered, also across a
// reconnect or another consumer reset. consume returns nil when ctx ends and
// an error when the iterator stops for another reason.
func (p *Projector) consume(ctx context.Context, messages jetstream.MessagesContext) error {
	// Build the option once: Next is on the replay hot path.
	next := []jetstream.NextOpt{jetstream.NextContext(ctx)}
	for {
		msg, err := messages.Next(next...)
		if ctx.Err() != nil {
			return nil
		}
		if errors.Is(err, jetstream.ErrMsgIteratorClosed) {
			return fmt.Errorf("projection consumer stopped: %w", err)
		}
		if err != nil {
			// The ordered consumer resets itself for heartbeat, reconnect,
			// sequence, and deleted-consumer errors. It returns other errors,
			// for example a metadata error or a failure to start the new
			// subscription after a reset. The next call continues, or it
			// returns ErrMsgIteratorClosed, and then the projector fails.
			p.logger.Warn("Projection consumer error (auto-recovering)", "error", err)
			continue
		}
		p.handleMessage(msg)
	}
}

// handleMessage handles one delivered message. Only consume calls it, one
// message at a time and in stream order. Projection callbacks run under
// applyMu, so they also do not overlap the batch flushes that the Run
// goroutine starts, such as the live batch timer and reconcileStartup.
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

	// Delivery is serial and in stream order. Thus a sequence that is not
	// greater than the last handled one is a redelivery of a handled event.
	if seq <= p.handledSeq {
		return
	}
	p.handledSeq = seq

	if !p.consumesSubject(msg.Subject()) {
		return
	}
	if p.shouldSkipRestored(seq) {
		return
	}
	p.observeDelivered(seq)

	event, err := p.decode(msg.Data())
	if err != nil {
		err = fmt.Errorf("decode event on subject %q: %w", msg.Subject(), err)
		p.logger.Error("Projection decode failed",
			"subject", msg.Subject(),
			"seq", seq,
			"error", err)
		p.failPendingBatch(seq, err)
		return
	}

	failureSeq, err := p.applyEvent(event, msg.Subject(), seq)
	if err != nil {
		p.logger.Error("Projection Apply failed",
			"subject", msg.Subject(),
			"seq", seq,
			"event_id", event.diagnosticID(),
			"error", err)
		p.fail(failureSeq, err)
		return
	}

	p.maybeCompleteStartup(time.Now())
}

func (p *Projector) applyEvent(event decodedEvent, subject string, seq uint64) (uint64, error) {
	p.applyMu.Lock()
	defer p.applyMu.Unlock()
	if p.hasFailed() || p.shouldSkipRestored(seq) {
		// A failure recorded while this event waited for the barrier stops
		// application, as the check in handleMessage does.
		return 0, nil
	}
	if p.applyStartupBatch != nil {
		if p.shouldBatchStartup(seq) {
			p.startupBatch = append(p.startupBatch, sequencedDecodedEvent{event: event, sequence: seq})
			if len(p.startupBatch) < p.startupBatchSize && seq < p.startupTargetSequence() {
				return 0, nil
			}
			return p.flushStartupBatch()
		}
		// A deleted or expired startup target never arrives to flush the
		// batch. Apply the pending events before any later event so stream
		// order holds.
		if failureSeq, err := p.flushStartupBatch(); err != nil {
			return failureSeq, err
		}
	}
	if p.applyLiveBatch != nil && seq > p.startupTargetSequence() {
		// Ordered delivery past the target proves that all retained startup
		// events have been handled. If the target disappeared, finish at the
		// committed sequence before buffering live events. Otherwise readiness
		// and the startup hook would depend on the live batch timer.
		p.mu.Lock()
		if p.startupEndedAt.IsZero() && p.lastSeq < p.startupTargetSeq {
			p.startupTargetSeq = p.lastSeq
			p.updatePendingLocked(time.Now(), false)
		}
		p.mu.Unlock()
		if summary, completed := p.completeStartupLocked(time.Now()); completed {
			p.logStartupComplete(summary)
		}
		p.liveBatch = append(p.liveBatch, sequencedDecodedEvent{event: event, sequence: seq})
		if len(p.liveBatch) == 1 {
			p.liveFlushTimer.Reset(p.liveBatchDelay)
		}
		if len(p.liveBatch) >= p.liveBatchSize {
			return p.flushLiveBatch()
		}
		return 0, nil
	}
	p.startOperation(ProjectorApplying)
	mutation, err := p.prepare(event, subject, seq)
	if err != nil {
		return seq, err
	}
	if mutation != nil {
		mutation.Commit()
	}
	p.countStartupMessages(1)
	p.commitDone(seq)
	return 0, nil
}

// flushStartupBatch applies the pending startup batch. On failure it returns
// the first batched sequence because no event in the batch was applied. The
// caller must hold applyMu.
func (p *Projector) flushStartupBatch() (uint64, error) {
	if len(p.startupBatch) == 0 {
		return 0, nil
	}
	firstSeq := p.startupBatch[0].sequence
	lastSeq := p.startupBatch[len(p.startupBatch)-1].sequence
	messageCount := uint64(len(p.startupBatch))
	p.startOperation(ProjectorApplyingStartupBatch)
	if err := p.applyStartupBatch(p.startupBatch); err != nil {
		return firstSeq, err
	}
	p.startupBatch = p.startupBatch[:0]
	p.countStartupMessages(messageCount)
	p.commitDone(lastSeq)
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

func (p *Projector) failPendingBatch(fallback uint64, err error) {
	p.applyMu.Lock()
	defer p.applyMu.Unlock()
	if len(p.startupBatch) > 0 {
		fallback = p.startupBatch[0].sequence
	} else if len(p.liveBatch) > 0 {
		fallback = p.liveBatch[0].sequence
	}
	p.fail(fallback, err)
}

// flushLiveBatch holds the apply barrier through commit and failure recording.
// A failed batch must not be retried by the timer or a concurrent callback.
func (p *Projector) flushLiveBatch() (uint64, error) {
	if len(p.liveBatch) == 0 || p.hasFailed() {
		return 0, nil
	}
	first := p.liveBatch[0].sequence
	p.liveFlushTimer.Stop()
	last := p.liveBatch[len(p.liveBatch)-1].sequence
	p.startOperation(ProjectorApplyingLiveBatch)
	if err := p.applyLiveBatch(p.liveBatch); err != nil {
		p.fail(first, err)
		return first, err
	}
	p.countStartupMessages(uint64(len(p.liveBatch)))
	clear(p.liveBatch)
	p.liveBatch = p.liveBatch[:0]
	p.commitDone(last)
	return 0, nil
}

func (p *Projector) hasFailed() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.failedErr != nil
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

// maybeCompleteStartup completes startup when every event through the startup
// target has been applied. The caller must not hold applyMu.
func (p *Projector) maybeCompleteStartup(now time.Time) {
	select {
	case <-p.startupCh:
		return // Already complete; handleMessage calls this for every event.
	default:
	}
	p.applyMu.Lock()
	summary, completed := p.completeStartupLocked(now)
	p.applyMu.Unlock()
	if completed {
		p.logStartupComplete(summary)
	}
}

// startupSummary holds the values that the startup-complete log reports.
type startupSummary struct {
	duration          time.Duration
	lastSeq, messages uint64
	projectionKey     string
	restore           string
}

// completeStartupLocked ends startup, runs the projection's
// StartupReplayCompleter hook, and then reports startup complete and releases
// WaitForStartup callers. The caller must hold applyMu, so no event is applied
// between the end of startup and the hook.
func (p *Projector) completeStartupLocked(now time.Time) (startupSummary, bool) {
	p.mu.Lock()
	if !p.started || !p.startupEndedAt.IsZero() || p.lastSeq < p.startupTargetSeq {
		p.mu.Unlock()
		return startupSummary{}, false
	}
	p.startupEndedAt = now
	summary := startupSummary{
		duration:      now.Sub(p.startupStartedAt),
		lastSeq:       p.lastSeq,
		messages:      p.startupMessages,
		projectionKey: p.diagnosticKey(),
	}
	summary.restore = "none"
	if p.snapshotRestored {
		summary.restore = "snapshot"
	} else if p.checkpointRestored {
		summary.restore = "checkpoint"
	}
	p.mu.Unlock()

	if projection, ok := p.proj.(StartupReplayCompleter); ok {
		projection.CompleteStartupReplay()
	}
	p.mu.Lock()
	p.startupCompleted = true
	p.mu.Unlock()
	close(p.startupCh)
	return summary, true
}

// diagnosticKey names the projection in logs. It uses only construction-time
// identity, so it needs no lock.
func (p *Projector) diagnosticKey() string {
	for _, key := range []string{p.checkpointKey, p.snapshots.key, p.consumerName} {
		if key != "" {
			return key
		}
	}
	return reflect.TypeOf(p.proj).Elem().String()
}

// runSourceChecks reads the newest matching source sequence at a fixed
// interval until ctx ends, so that pending work is visible also when the
// consumer stops delivering. It logs each change of the stall state. It never
// takes applyMu, so a blocked commit cannot stop it.
func (p *Projector) runSourceChecks(ctx context.Context) {
	interval := p.sourceCheckInterval
	if interval <= 0 {
		interval = projectionSourceCheckInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	stalled := false
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		checkCtx, cancel := context.WithTimeout(ctx, projectionSourceCheckTimeout)
		target, err := p.currentTarget(checkCtx)
		cancel()
		if err == nil {
			p.observeSource(target.seq)
		} else if ctx.Err() == nil {
			p.logger.Debug("Projection source check failed", "projection", p.diagnosticKey(), "error", err)
		}
		status := p.Status()
		if status.Stalled == stalled {
			continue
		}
		stalled = status.Stalled
		attrs := []any{
			"projection", p.diagnosticKey(),
			"last_seq", status.LastSeq,
			"observed_seq", status.ObservedSeq,
			"pending_for", status.PendingFor.Round(time.Second),
			"operation", string(status.CurrentOperation),
			"operation_duration", status.CurrentOperationDuration.Round(time.Second),
		}
		if stalled {
			p.logger.Warn("Projection stalled: pending events have no committed progress", attrs...)
		} else {
			p.logger.Info("Projection no longer stalled", attrs...)
		}
	}
}

func (p *Projector) logStartupComplete(summary startupSummary) {
	p.logger.Info("Projection startup complete",
		"projection", summary.projectionKey,
		"duration", summary.duration.Round(time.Millisecond),
		"messages", summary.messages,
		"restore", summary.restore,
		"last_seq", summary.lastSeq,
	)
}

func (p *Projector) setStartupTarget(seq uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.startupTargetSeq = seq
	p.updatePendingLocked(time.Now(), false)
}

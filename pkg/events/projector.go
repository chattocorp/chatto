package events

import (
	"cmp"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"golang.org/x/sync/errgroup"
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
	// While startup is incomplete, Run checks this often whether a deleted or
	// expired startup target prevents completion. Each check reads the last
	// message of every projection subject.
	projectionStartupReconcileInterval = 5 * time.Second
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
	logger  Logger
	applyMu sync.Mutex
	decode  func([]byte) (decodedEvent, error)
	// prepare prepares one event under applyMu. A nil mutation means that
	// preparation already applied the event.
	prepare func(decodedEvent, string, uint64) (PreparedMutation, error)
	// applyStartupBatch applies batched startup events. It is nil unless the
	// projection implements StartupBatchEventProjection.
	applyStartupBatch func([]sequencedDecodedEvent) error

	subjects        []string
	replaySubjects  []string
	subjectMatchers []compiledSubjectFilter

	// Consumer identity is application-owned diagnostic text, guarded by mu
	// and frozen by Run.
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
	// for the projector's lifetime. WaitFor uses this to short-
	// circuit during boot-time mutations that happen before
	// application lifecycle gets a chance to start the consumer (see the
	// WaitFor doc for why).
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

	snapshots            projectorSnapshots
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
}

// ProjectorStatus is a concurrency-safe snapshot of a projector's
// lifecycle state. Operators use it for diagnostics; application readiness
// uses Err to surface fatal projection failures.
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

	Failed    bool
	FailedSeq uint64
	Failure   string
	Err       error
}

type seqWaiter struct {
	seq uint64
	ch  chan struct{}
}

// NewDecodedProjector binds a non-nil pointer projection and decoder to a
// stream. It does not start the consumer; call Run for that. Requiring a
// pointer prevents the projector and application read side from receiving
// separate value copies of mutable projection state. The decoder is the only
// boundary between opaque stored records and application event values.
func NewDecodedProjector[E any](
	js jetstream.JetStream,
	stream jetstream.Stream,
	proj EventProjection[E],
	decoder EventDecoder[E],
	logger Logger,
) *Projector {
	// Apply changes state during preparation. Both run under the apply
	// barrier, so the result equals a prepared mutation with no commit step.
	p := newProjector(js, stream, proj, decoder, logger, func(event E, _ string, seq uint64) (PreparedMutation, error) {
		return nil, proj.Apply(event, seq)
	})
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
	return p
}

// NewDecodedPreparedProjector binds a prepared projection and decoder to one
// ordered projector lifecycle. The projector prepares the event while it holds
// the apply barrier, commits only after preparation succeeds, and then advances
// the applied sequence before it releases the barrier. A projection that
// implements SubjectEventReducer prepares with the delivered subject.
func NewDecodedPreparedProjector[E any](
	js jetstream.JetStream,
	stream jetstream.Stream,
	proj PreparedEventProjection[E],
	decoder EventDecoder[E],
	logger Logger,
) *Projector {
	prepare := func(event E, _ string, seq uint64) (PreparedMutation, error) {
		return proj.Prepare(event, seq)
	}
	if projection, ok := proj.(SubjectEventReducer[E]); ok {
		prepare = projection.PrepareSubject
	}
	return newProjector(js, stream, proj, decoder, logger, prepare)
}

// newProjector validates a projection and decoder and builds the Projector
// that prepares every decoded event with prepare.
func newProjector[E any](
	js jetstream.JetStream,
	stream jetstream.Stream,
	proj SubjectProjection,
	decoder EventDecoder[E],
	logger Logger,
	prepare func(event E, subject string, seq uint64) (PreparedMutation, error),
) *Projector {
	if isNilProjection(proj) {
		panic("events: projector requires a non-nil projection")
	}
	if reflect.ValueOf(proj).Kind() != reflect.Pointer {
		panic("events: projector requires a pointer projection")
	}
	if decoder == nil {
		panic("events: projector requires a non-nil event decoder")
	}
	subjects := slices.Clone(proj.Subjects())
	return &Projector{
		js:     js,
		stream: stream,
		proj:   proj,
		logger: normalizeLogger(logger),
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
	}
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
// After the stream position is validated, a call whose LastSeq() is already at
// or beyond pos.Seq skips waiter registration. Otherwise it registers a waiter
// and blocks.
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
	// Keep waiters sorted ascending by seq so advance() can release them
	// in order and stop scanning at the first unmet seq.
	i, _ := slices.BinarySearchFunc(p.waiters, seq, func(w seqWaiter, target uint64) int {
		return cmp.Compare(w.seq, target)
	})
	p.waiters = slices.Insert(p.waiters, i, seqWaiter{seq: seq, ch: ch})
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
		p.waiters = slices.DeleteFunc(p.waiters, func(w seqWaiter) bool { return w.ch == ch })
		p.mu.Unlock()
		return ctx.Err()
	}
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

// ConfigureConsumerIdentity sets diagnostic labels for this projector's
// ephemeral consumer. Call it before Run. The name must contain 1 to 64 ASCII
// letters, digits, hyphens, or underscores. Neither value may contain personal
// data or secrets. The description is stored in consumer metadata because the
// ordered-consumer API has no description field.
//
// Run adds a random suffix to the name so replicas never share a consumer.
// These labels do not change snapshot identities or create a durable consumer.
func (p *Projector) ConfigureConsumerIdentity(name, description string) error {
	if len(name) == 0 || len(name) > 64 {
		return fmt.Errorf("projection consumer name must contain 1 to 64 ASCII letters, digits, hyphens, or underscores")
	}
	for _, c := range name {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_') {
			return fmt.Errorf("projection consumer name must contain only ASCII letters, digits, hyphens, or underscores")
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.started {
		return ErrProjectorAlreadyStarted
	}
	p.consumerName = name
	p.consumerDescription = description
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
	consumerName, consumerDescription := p.consumerName, p.consumerDescription
	if p.startupStartedAt.IsZero() {
		p.startupStartedAt = startedAt
	}
	p.mu.Unlock()

	target, err := p.currentTarget(ctx)
	if err != nil {
		return fmt.Errorf("read projection startup target: %w", err)
	}
	p.applyMu.Lock()
	restoreErr := p.restoreForRun(ctx, target.seq)
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
	if consumerName != "" {
		consumerConfig.NamePrefix = "projection-" + consumerName + "-" + rand.Text()
		consumerConfig.Metadata = map[string]string{
			"projection_name":        consumerName,
			"projection_description": consumerDescription,
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
	// when Consume fails after the physical consumer was created.
	defer p.deleteProjectionConsumer(cons)

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
	originalTarget := p.startupTargetSeq
	if p.startupTargetSeq > p.lastSeq {
		p.startupTargetSeq = p.lastSeq
	}
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

// handleMessage is the per-event callback wired into the OrderedConsumer's
// Consume handler. The SDK invokes it in stream order. Projection callbacks
// run under applyMu, so they never run concurrently and always see stream
// order, including when reconcileStartup flushes a startup batch from the Run
// goroutine.
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

	event, err := p.decode(msg.Data())
	if err != nil {
		err = fmt.Errorf("decode event on subject %q: %w", msg.Subject(), err)
		failureSeq := p.pendingStartupBatchFirstSequence(seq)
		p.logger.Error("Projection decode failed",
			"subject", msg.Subject(),
			"seq", seq,
			"error", err)
		p.fail(failureSeq, err)
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
	mutation, err := p.prepare(event, subject, seq)
	if err != nil {
		return seq, err
	}
	if mutation != nil {
		mutation.Commit()
	}
	p.countStartupMessages(1)
	p.advance(seq)
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
	if err := p.applyStartupBatch(p.startupBatch); err != nil {
		return firstSeq, err
	}
	p.startupBatch = p.startupBatch[:0]
	p.countStartupMessages(messageCount)
	p.advance(lastSeq)
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
		return p.startupBatch[0].sequence
	}
	return fallback
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
	duration                     time.Duration
	targetSeq, lastSeq, messages uint64
	projectionKey                string
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
		targetSeq:     p.startupTargetSeq,
		lastSeq:       p.lastSeq,
		messages:      p.startupMessages,
		projectionKey: p.checkpointKey,
	}
	if summary.projectionKey == "" {
		summary.projectionKey = p.snapshots.key
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

func (p *Projector) logStartupComplete(summary startupSummary) {
	var rate float64
	if seconds := summary.duration.Seconds(); seconds > 0 {
		rate = float64(summary.messages) / seconds
	}
	p.logger.Info("Projection startup complete",
		"projection", summary.projectionKey,
		"duration", summary.duration,
		"messages", summary.messages,
		"messages_per_second", rate,
		"last_seq", summary.lastSeq,
		"target_seq", summary.targetSeq,
		"subjects", p.subjects,
	)
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

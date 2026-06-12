package events

import (
	"context"
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

// MemoryProjection is an embeddable base for projections whose state
// lives entirely in process memory. It contributes a sync.RWMutex that
// subclasses use for read/write coordination, plus no-op
// Snapshot/Restore methods that satisfy the Projection interface until
// snapshot orchestration (ADR-033) ships.
//
// Embed by value — the zero mutex is ready to use. Subclasses still
// implement Subjects() and Apply(). Future non-memory projection types
// (KV-backed, file-backed) would have their own embed-friendly base.
type MemoryProjection struct {
	sync.RWMutex
}

// Snapshot implements Projection (no-op until ADR-033 snapshot
// orchestration ships; the Projector treats (nil, nil) as "skip
// snapshot persistence").
func (*MemoryProjection) Snapshot() ([]byte, error) { return nil, nil }

// Restore implements Projection. Called once before Run with
// nil/empty until snapshot orchestration ships.
func (*MemoryProjection) Restore(_ []byte) error { return nil }

// Projection is the read side. Implementations are in-memory Go data
// structures that consume events from a subject filter and serve reads.
//
// Concurrency contract: Apply is called from a single goroutine owned by
// the Projector, in stream order. Implementations don't need internal
// locking on the write path. They DO need a read lock if external code
// (e.g. GraphQL resolvers) reads concurrently — projections typically
// embed a sync.RWMutex for this.
//
// Idempotency: Apply(e, n) followed by Apply(e, n) must produce the same
// state as a single Apply(e, n). Snapshots aren't implemented yet, but the
// contract holds now so we don't have to revisit it later.
type Projection interface {
	// Subjects returns the subject filter(s) this projection consumes.
	// Wildcards are supported (e.g. "server.evt.room.>").
	Subjects() []string

	// Apply is called for every event matching Subjects(), in stream
	// order. seq is the stream sequence of this event.
	Apply(event *corev1.Event, seq uint64) error

	// Snapshot returns a serialized form of the current state.
	// Returning (nil, nil) means "no snapshot support yet"; the Projector
	// will then skip snapshot persistence. Interface is committed; the
	// orchestration that calls Snapshot/Restore is deferred per ADR-033.
	Snapshot() ([]byte, error)

	// Restore initializes state from a snapshot. Called once before Run
	// starts consuming. May be called with nil/empty for cold start —
	// the projection should treat that as "no prior state."
	Restore(snapshot []byte) error
}

// Projector runs the consumer + apply loop for one projection.
type Projector struct {
	js     jetstream.JetStream
	stream jetstream.Stream
	proj   Projection
	logger Logger

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
}

// ProjectorStatus is a concurrency-safe snapshot of a projector's
// lifecycle state. Operators use it for diagnostics; core readiness uses
// Err to surface fatal projection failures.
type ProjectorStatus struct {
	Started bool
	LastSeq uint64

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
	return &Projector{
		js:       js,
		stream:   stream,
		proj:     proj,
		logger:   logger,
		failedCh: make(chan struct{}),
	}
}

// Status returns the projector's current lifecycle state. Safe to call from
// any goroutine.
func (p *Projector) Status() ProjectorStatus {
	p.mu.Lock()
	defer p.mu.Unlock()

	status := ProjectorStatus{
		Started: p.started,
		LastSeq: p.lastSeq,
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

// LastSeq returns the highest stream sequence applied to the projection so
// far. Safe to call from any goroutine.
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
	return append([]string(nil), p.proj.Subjects()...)
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
	subjects := p.proj.Subjects()
	for _, filter := range subjects {
		if subjectMatchesFilter(filter, subject) {
			return nil
		}
	}
	return fmt.Errorf("%w: subject %q not matched by filters %v",
		ErrProjectionSubjectNotConsumed, subject, subjects)
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
	if filter == "" || subject == "" {
		return false
	}
	filterTokens := strings.Split(filter, ".")
	subjectTokens := strings.Split(subject, ".")

	for i, token := range filterTokens {
		if token == ">" {
			return i == len(filterTokens)-1 && len(subjectTokens) > i
		}
		if i >= len(subjectTokens) {
			return false
		}
		if token != "*" && token != subjectTokens[i] {
			return false
		}
	}
	return len(subjectTokens) == len(filterTokens)
}

// WaitForCurrent blocks until the projection has applied the latest
// stream message currently matching its subject filters. It is intended
// for diagnostics and boot verification: call it after the projector is
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
	var target projectionTarget
	for _, subject := range p.proj.Subjects() {
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
		close(p.failedCh)
	}
	for _, w := range p.waiters {
		close(w.ch)
	}
	p.waiters = nil
}

// Run starts the consumer + apply loop. Blocks until ctx is cancelled.
// Returns the context's error on shutdown.
//
// Snapshot orchestration is deferred (ADR-033). For now, Restore is always
// called with nil and the loop replays from the beginning of the stream.
func (p *Projector) Run(ctx context.Context) error {
	p.mu.Lock()
	p.started = true
	p.mu.Unlock()

	if err := p.proj.Restore(nil); err != nil {
		return fmt.Errorf("restore projection: %w", err)
	}

	cons, err := p.stream.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{
		FilterSubjects:    p.proj.Subjects(),
		DeliverPolicy:     jetstream.DeliverAllPolicy,
		InactiveThreshold: 30 * time.Second,
	})
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
		jetstream.ConsumeErrHandler(p.handleConsumeErr),
	)
	if err != nil {
		return fmt.Errorf("start consume: %w", err)
	}
	defer cc.Stop()

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

	meta, err := msg.Metadata()
	if err != nil {
		p.logger.Error("Projection message metadata failed", "subject", msg.Subject(), "error", err)
		p.fail(0, fmt.Errorf("message metadata for subject %q: %w", msg.Subject(), err))
		return
	}

	var event corev1.Event
	if err := proto.Unmarshal(msg.Data(), &event); err != nil {
		err = fmt.Errorf("unmarshal event on subject %q: %w", msg.Subject(), err)
		p.logger.Error("Projection decode failed",
			"subject", msg.Subject(),
			"seq", meta.Sequence.Stream,
			"error", err)
		p.fail(meta.Sequence.Stream, err)
		return
	}

	if err := p.proj.Apply(&event, meta.Sequence.Stream); err != nil {
		p.logger.Error("Projection Apply failed",
			"subject", msg.Subject(),
			"seq", meta.Sequence.Stream,
			"event_id", event.GetId(),
			"error", err)
		p.fail(meta.Sequence.Stream, err)
		return
	}

	p.advance(meta.Sequence.Stream)
}

// handleConsumeErr is invoked by the SDK when the OrderedConsumer's
// background machinery hits a transient problem (missed heartbeat,
// reset attempt, etc.). OrderedConsumer recovers internally; we log
// and stay running.
func (p *Projector) handleConsumeErr(_ jetstream.ConsumeContext, err error) {
	p.logger.Warn("Projection consumer error (auto-recovering)", "error", err)
}

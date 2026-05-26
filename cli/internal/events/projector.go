package events

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

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

	mu      sync.Mutex
	lastSeq uint64
	waiters []seqWaiter
	// started flips true the first time Run is invoked and stays true
	// for the projector's lifetime. WaitForSeq uses this to short-
	// circuit during boot-time mutations that happen before
	// ChattoCore.Run gets a chance to start the consumer (see the
	// WaitForSeq doc for why).
	started bool
}

type seqWaiter struct {
	seq uint64
	ch  chan struct{}
}

// NewProjector binds a projection to a stream. Does not start the consumer
// — call Run for that.
func NewProjector(js jetstream.JetStream, stream jetstream.Stream, proj Projection, logger Logger) *Projector {
	return &Projector{
		js:     js,
		stream: stream,
		proj:   proj,
		logger: logger,
	}
}

// LastSeq returns the highest stream sequence applied to the projection so
// far. Safe to call from any goroutine.
func (p *Projector) LastSeq() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastSeq
}

// Started reports whether Run has entered its body — i.e. whether
// the projector's consumer is being set up / has been set up. Used by
// test helpers (and lifecycle code) that need to wait for projectors
// to come online before issuing reads against the projection.
func (p *Projector) Started() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.started
}

// AppendAndWait publishes an event for an aggregate and blocks until
// this projection has applied it. The subject is derived from
// `agg.SubjectFor(event)`, so the caller cannot accidentally publish an
// event onto the wrong subject for its payload.
//
// This is the canonical "publish-then read-your-writes" primitive —
// every caller that needs to read its own write through the projection
// should use this rather than hand-rolling Append + WaitForSeq.
//
// Returns the stream sequence the publish landed at, plus any error.
// On a publish failure the sequence is 0; on a wait failure (most
// commonly ctx cancellation) the sequence is non-zero and the event
// has already been durably published — only the local projection
// hasn't caught up.
func (p *Projector) AppendAndWait(ctx context.Context, pub *Publisher, agg Aggregate, event *corev1.Event) (uint64, error) {
	seq, err := pub.Append(ctx, agg.SubjectFor(event), event)
	if err != nil {
		return 0, err
	}
	if err := p.WaitForSeq(ctx, seq); err != nil {
		return seq, err
	}
	return seq, nil
}

// WaitForSeq blocks until LastSeq() >= seq or ctx is done.
//
// Used by writers that need read-your-writes consistency: capture the seq
// from Publisher.Append, pass it here, then read from the projection.
//
// If LastSeq() is already >= seq when called, returns immediately with no
// error. Otherwise registers a waiter and blocks.
//
// Precondition: the projector's Run loop is expected to be active by
// the time any code reaches WaitForSeq. Callers that mutate during
// boot (ensureChannelRoomsAreInAGroup, SeedDefaultRooms) are
// orchestrated by core.Run / core.WaitForBoot to make this true.
// Calling before Run started would block forever waiting for a
// sequence that never advances — that's the symptom we want, since
// the alternative (silently skipping the wait) leaves the projection
// out of sync with the KV write and produces orphan-room bugs.
func (p *Projector) WaitForSeq(ctx context.Context, seq uint64) error {
	p.mu.Lock()
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

// WaitForCurrent blocks until the projection has applied the latest
// stream message currently matching its subject filters. It is intended
// for diagnostics and boot verification: call it after the projector is
// running to ensure projection reads reflect the stream as of this call.
func (p *Projector) WaitForCurrent(ctx context.Context) error {
	var target uint64
	for _, subject := range p.proj.Subjects() {
		msg, err := p.stream.GetLastMsgForSubject(ctx, subject)
		if err != nil {
			if errors.Is(err, jetstream.ErrMsgNotFound) {
				continue
			}
			return fmt.Errorf("last msg for subject %q: %w", subject, err)
		}
		if msg.Sequence > target {
			target = msg.Sequence
		}
	}
	if target == 0 {
		return nil
	}
	return p.WaitForSeq(ctx, target)
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

	<-ctx.Done()
	return ctx.Err()
}

// handleMessage is the per-event callback wired into the OrderedConsumer's
// Consume handler. It is invoked from a single goroutine the SDK owns, in
// stream order — matching the Projection.Apply concurrency contract.
//
// Errors from the projection's Apply are logged and swallowed per
// ADR-033's "a projection bug shouldn't stall the consumer" rule. Stream
// sequence is advanced regardless so WaitForSeq waiters unblock.
func (p *Projector) handleMessage(msg jetstream.Msg) {
	meta, err := msg.Metadata()
	if err != nil {
		p.logger.Warn("Skipping event with no metadata", "subject", msg.Subject(), "error", err)
		return
	}

	var event corev1.Event
	if err := proto.Unmarshal(msg.Data(), &event); err != nil {
		p.logger.Warn("Skipping unmarshalable event",
			"subject", msg.Subject(),
			"seq", meta.Sequence.Stream,
			"error", err)
		p.advance(meta.Sequence.Stream)
		return
	}

	if err := p.proj.Apply(&event, meta.Sequence.Stream); err != nil {
		p.logger.Error("Projection Apply failed",
			"subject", msg.Subject(),
			"seq", meta.Sequence.Stream,
			"event_id", event.GetId(),
			"error", err)
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

package events

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// These tests force an ordered consumer reset while the projector still
// handles an earlier event. A reset gives the consumer a new subscription.
// The projector must not apply a later event, release a waiter, or advance
// LastSeq before the earlier event is applied, and it must apply each event
// exactly once.

const (
	resetConsumedSubject   = "evt.reset.consumed.item"
	resetUnconsumedSubject = "evt.reset.other.item"
	// resetObservationWindow is how long a test gives the projector to do
	// something wrong while an earlier event is held. Correct code cannot
	// deliver the next event in this window, so the window never makes a
	// correct run fail or pass by chance. It only gives code without the fix
	// the time to show the defect.
	resetObservationWindow = 300 * time.Millisecond
)

// gate holds the goroutine that calls hold until the test opens it.
type gate struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func newGate() *gate {
	return &gate{entered: make(chan struct{}), release: make(chan struct{})}
}

func (g *gate) hold() {
	close(g.entered)
	<-g.release
}

func (g *gate) open() { g.once.Do(func() { close(g.release) }) }

// resetProjection records each applied event with its sequence. Apply calls
// the hook for the event, if any, before it records the event.
type resetProjection struct {
	mu      sync.Mutex
	events  []string
	seqs    []uint64
	hooks   map[string]func() error
	applied chan uint64
}

func newResetProjection() *resetProjection {
	return &resetProjection{hooks: map[string]func() error{}, applied: make(chan uint64, 100)}
}

func (*resetProjection) Subjects() []string { return []string{"evt.reset.consumed.>"} }

// ReplaySubjects is broader than Subjects, so the consumer also delivers
// messages that the projector must skip.
func (*resetProjection) ReplaySubjects() []string { return []string{"evt.reset.>"} }

func (p *resetProjection) Apply(event string, seq uint64) error {
	return p.record([]SequencedEventOf[string]{{Event: event, Sequence: seq}})
}

func (p *resetProjection) record(items []SequencedEventOf[string]) error {
	for _, item := range items {
		p.mu.Lock()
		hook := p.hooks[item.Event]
		p.mu.Unlock()
		if hook != nil {
			if err := hook(); err != nil {
				return err
			}
		}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, item := range items {
		p.events = append(p.events, item.Event)
		p.seqs = append(p.seqs, item.Sequence)
		p.applied <- item.Sequence
	}
	return nil
}

func (p *resetProjection) setHook(event string, hook func() error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.hooks[event] = hook
}

func (p *resetProjection) appliedSeqs() []uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.seqs)
}

// resetBatchProjection adds startup and live batches to resetProjection.
type resetBatchProjection struct {
	*resetProjection
	startupSize int
	liveSize    int
	liveDelay   time.Duration
}

func (p *resetBatchProjection) StartupBatchSize() int { return p.startupSize }
func (p *resetBatchProjection) ApplyStartupBatch(items []SequencedEventOf[string]) error {
	return p.record(items)
}
func (p *resetBatchProjection) LiveBatchSize() int               { return p.liveSize }
func (p *resetBatchProjection) LiveBatchMaxDelay() time.Duration { return p.liveDelay }
func (p *resetBatchProjection) ApplyLiveBatch(items []SequencedEventOf[string]) error {
	return p.record(items)
}

// resetHarness runs one projector against an embedded NATS server and lets
// the test force reconnects and gate the decoder.
type resetHarness struct {
	t          *testing.T
	ctx        context.Context
	conn       *nats.Conn
	js         jetstream.JetStream
	stream     jetstream.Stream
	log        *EncodedEventLog
	projector  *Projector
	projection *resetProjection

	mu          sync.Mutex
	decodeHooks map[string]func()
}

func newResetHarness(t *testing.T) *resetHarness {
	t.Helper()
	conn := startTestNATS(t)
	js := must(jetstream.New(conn))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	stream := must(js.CreateStream(ctx, jetstream.StreamConfig{Name: "EVENTS_RESET", Subjects: []string{"evt.reset.>"}}))
	return newResetHarnessOn(t, ctx, conn, js, stream)
}

func newResetHarnessOn(t *testing.T, ctx context.Context, conn *nats.Conn, js jetstream.JetStream, stream jetstream.Stream) *resetHarness {
	return &resetHarness{
		t: t, ctx: ctx, conn: conn, js: js, stream: stream,
		log:         NewEncodedEventLog(js, stream, nil),
		projection:  newResetProjection(),
		decodeHooks: map[string]func(){},
	}
}

// start builds the projector for projection and runs it. Gates that the
// test opens in its cleanup run before the projector stops.
func (h *resetHarness) start(projection SubjectProjection, waitForStartup bool) {
	h.t.Helper()
	decoder := func(data []byte) (DecodedEvent[string], error) {
		if string(data) == "undecodable" {
			return DecodedEvent[string]{}, errors.New("undecodable event")
		}
		return DecodedEvent[string]{Event: string(data), ID: string(data)}, nil
	}
	h.projector = must(NewDecodedProjector(h.js, h.stream, projection.(EventProjection[string]), decoder, ProjectorOptions{}))
	decode := h.projector.decode
	h.projector.decode = func(data []byte) (decodedEvent, error) {
		h.mu.Lock()
		hook := h.decodeHooks[string(data)]
		h.mu.Unlock()
		if hook != nil {
			hook()
		}
		return decode(data)
	}
	runCtx, stop := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- h.projector.Run(runCtx) }()
	h.t.Cleanup(func() {
		stop()
		if err := <-done; err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, ErrProjectionFailed) && !errors.Is(err, jetstream.ErrMsgIteratorClosed) {
			h.t.Errorf("Run: %v", err)
		}
	})
	if waitForStartup {
		if err := h.projector.WaitForStartup(h.ctx); err != nil {
			h.t.Fatal(err)
		}
	}
}

// holdDecode makes the decoder hold event until the gate opens.
func (h *resetHarness) holdDecode(event string) *gate {
	g := newGate()
	h.t.Cleanup(g.open)
	h.mu.Lock()
	h.decodeHooks[event] = g.hold
	h.mu.Unlock()
	return g
}

// holdApply makes Apply hold event until the gate opens.
func (h *resetHarness) holdApply(event string) *gate {
	g := newGate()
	h.t.Cleanup(g.open)
	h.projection.setHook(event, func() error { g.hold(); return nil })
	return g
}

func (h *resetHarness) append(subject, event string) uint64 {
	h.t.Helper()
	seq, err := h.log.AppendEventually(h.ctx, subject, EncodedRecord{ID: event, Data: []byte(event)})
	if err != nil {
		h.t.Fatal(err)
	}
	return seq
}

// reconnect forces the client to reconnect and waits until it is connected
// again. The ordered consumer resets after the reconnect.
func (h *resetHarness) reconnect() {
	h.t.Helper()
	connected := h.conn.StatusChanged(nats.CONNECTED)
	defer h.conn.RemoveStatusListener(connected)
	if err := h.conn.ForceReconnect(); err != nil {
		h.t.Fatal(err)
	}
	select {
	case <-connected:
	case <-h.ctx.Done():
		h.t.Fatal("client did not reconnect")
	}
}

// waitAsync starts wait and returns a channel with its result.
func waitAsync(wait func() error) <-chan error {
	result := make(chan error, 1)
	go func() { result <- wait() }()
	return result
}

// expectHeld fails when a waiter returns or an event is applied during the
// observation window.
func (h *resetHarness) expectHeld(what string, waiters ...<-chan error) {
	h.t.Helper()
	window := time.NewTimer(resetObservationWindow)
	defer window.Stop()
	for {
		select {
		case <-window.C:
			return
		case seq := <-h.projection.applied:
			h.t.Fatalf("seq %d applied while %s", seq, what)
		default:
		}
		for _, waiter := range waiters {
			select {
			case err := <-waiter:
				h.t.Fatalf("waiter returned %v while %s (LastSeq %d)", err, what, h.projector.LastSeq())
			default:
			}
		}
		select {
		case <-window.C:
			return
		case seq := <-h.projection.applied:
			h.t.Fatalf("seq %d applied while %s", seq, what)
		case <-time.After(time.Millisecond):
		}
	}
}

func (h *resetHarness) waitFor(seq uint64) {
	h.t.Helper()
	if err := h.projector.WaitFor(h.ctx, SubjectPosition(resetConsumedSubject, seq)); err != nil {
		h.t.Fatalf("WaitFor(%d): %v", seq, err)
	}
}

func (h *resetHarness) expectApplied(want ...uint64) {
	h.t.Helper()
	if got := h.projection.appliedSeqs(); !slices.Equal(got, want) {
		h.t.Fatalf("applied sequences = %v, want %v", got, want)
	}
}

// TestProjectorResetDoesNotOvertakeHeldEvent holds consumed event s at each
// stage of its handling, forces a reconnect, and appends s+1. Before the fix,
// s+1 was applied first when s was in its decoder: WaitFor(s) returned nil,
// and the projection saw the sequences [s+1 s].
func TestProjectorResetDoesNotOvertakeHeldEvent(t *testing.T) {
	for _, stage := range []string{"decode", "apply barrier", "apply"} {
		t.Run(stage, func(t *testing.T) {
			h := newResetHarness(t)
			h.start(h.projection, true)
			var held *gate
			var decoded chan struct{}
			switch stage {
			case "decode":
				held = h.holdDecode("s")
			case "apply barrier":
				// A read barrier holds applyMu, so s waits for it after its
				// decoder returns.
				held = newGate()
				t.Cleanup(held.open)
				barrier := waitAsync(func() error {
					return h.projector.WithReadBarrier(func(uint64) error { held.hold(); return nil })
				})
				t.Cleanup(func() { <-barrier })
				receive(t, held.entered, "read barrier")
				decoded = make(chan struct{})
				h.mu.Lock()
				h.decodeHooks["s"] = func() { close(decoded) }
				h.mu.Unlock()
			case "apply":
				held = h.holdApply("s")
			}
			s := h.append(resetConsumedSubject, "s")
			if decoded != nil {
				// s waits for applyMu once its decoder returns.
				receive(t, decoded, "decode s")
			} else {
				receive(t, held.entered, "hold s")
			}
			h.reconnect()
			next := h.append(resetConsumedSubject, "next")
			waitS := waitAsync(func() error { return h.projector.WaitFor(h.ctx, SubjectPosition(resetConsumedSubject, s)) })
			h.expectHeld("s is held", waitS)
			if got := h.projector.LastSeq(); got >= s {
				t.Fatalf("LastSeq = %d while s=%d is held", got, s)
			}
			held.open()
			if err := <-waitS; err != nil {
				t.Fatalf("WaitFor(s): %v", err)
			}
			h.waitFor(next)
			h.expectApplied(s, next)
		})
	}
}

// TestProjectorResetMoreThanOnce forces two reconnects in a row while s is in
// its decoder.
func TestProjectorResetMoreThanOnce(t *testing.T) {
	h := newResetHarness(t)
	h.start(h.projection, true)
	held := h.holdDecode("s")
	s := h.append(resetConsumedSubject, "s")
	receive(t, held.entered, "hold s")
	h.reconnect()
	first := h.append(resetConsumedSubject, "first")
	h.reconnect()
	second := h.append(resetConsumedSubject, "second")
	waitCurrent := waitAsync(func() error { return h.projector.WaitForCurrent(h.ctx) })
	h.expectHeld("s is held", waitCurrent)
	held.open()
	if err := <-waitCurrent; err != nil {
		t.Fatalf("WaitForCurrent: %v", err)
	}
	if got := h.projector.LastSeq(); got != second {
		t.Fatalf("LastSeq after WaitForCurrent = %d, want %d", got, second)
	}
	h.expectApplied(s, first, second)
}

// TestProjectorResetWhileIdle forces reconnects while no event is handled.
// The consumer resets in the iterator and delivers later events normally.
func TestProjectorResetWhileIdle(t *testing.T) {
	h := newResetHarness(t)
	h.start(h.projection, true)
	first := h.append(resetConsumedSubject, "first")
	h.waitFor(first)
	h.reconnect()
	h.reconnect()
	second := h.append(resetConsumedSubject, "second")
	h.waitFor(second)
	if err := h.projector.WaitForCurrent(h.ctx); err != nil {
		t.Fatal(err)
	}
	h.expectApplied(first, second)
}

// TestProjectorResetSkipsUnconsumedSubject delivers a message outside the
// consumed subjects across a reset. The projector skips it and applies the
// consumed events in order.
func TestProjectorResetSkipsUnconsumedSubject(t *testing.T) {
	h := newResetHarness(t)
	h.start(h.projection, true)
	held := h.holdDecode("s")
	s := h.append(resetConsumedSubject, "s")
	receive(t, held.entered, "hold s")
	h.reconnect()
	h.append(resetUnconsumedSubject, "other")
	next := h.append(resetConsumedSubject, "next")
	waitNext := waitAsync(func() error { return h.projector.WaitFor(h.ctx, SubjectPosition(resetConsumedSubject, next)) })
	h.expectHeld("s is held", waitNext)
	held.open()
	if err := <-waitNext; err != nil {
		t.Fatal(err)
	}
	h.expectApplied(s, next)
}

// TestProjectorResetFailureOnHeldEvent makes the held event fail after the
// reset. The projector reports the failure at s and applies nothing after it.
func TestProjectorResetFailureOnHeldEvent(t *testing.T) {
	for _, stage := range []string{"decode", "apply"} {
		t.Run(stage, func(t *testing.T) {
			h := newResetHarness(t)
			h.start(h.projection, true)
			event := "s"
			var held *gate
			if stage == "decode" {
				event = "undecodable"
				held = h.holdDecode(event)
			} else {
				held = newGate()
				t.Cleanup(held.open)
				h.projection.setHook(event, func() error {
					held.hold()
					return errors.New("apply failed")
				})
			}
			s := h.append(resetConsumedSubject, event)
			receive(t, held.entered, "hold s")
			h.reconnect()
			next := h.append(resetConsumedSubject, "next")
			waitNext := waitAsync(func() error { return h.projector.WaitFor(h.ctx, SubjectPosition(resetConsumedSubject, next)) })
			h.expectHeld("s is held", waitNext)
			held.open()
			if err := <-waitNext; !errors.Is(err, ErrProjectionFailed) {
				t.Fatalf("WaitFor(next) = %v, want ErrProjectionFailed", err)
			}
			status := h.projector.Status()
			if status.FailedSeq != s || status.LastSeq >= s {
				t.Fatalf("FailedSeq = %d, LastSeq = %d, want failure at %d without progress", status.FailedSeq, status.LastSeq, s)
			}
			h.expectApplied()
		})
	}
}

// TestProjectorResetDuringStartupReplay forces a reset while an event in the
// middle of the startup replay is in its decoder. The startup batch keeps
// stream order and applies each event once.
func TestProjectorResetDuringStartupReplay(t *testing.T) {
	for _, held := range []string{"middle", "target"} {
		t.Run(held, func(t *testing.T) {
			h := newResetHarness(t)
			var seqs []uint64
			for i := range 6 {
				event := fmt.Sprintf("startup-%d", i)
				switch {
				case held == "middle" && i == 2, held == "target" && i == 5:
					event = "s"
				}
				seqs = append(seqs, h.append(resetConsumedSubject, event))
			}
			gate := h.holdDecode("s")
			projection := &resetBatchProjection{resetProjection: h.projection, startupSize: 3, liveSize: 1, liveDelay: time.Hour}
			h.start(projection, false)
			receive(t, gate.entered, "hold s")
			h.reconnect()
			live := h.append(resetConsumedSubject, "live")
			waitStartup := waitAsync(func() error { return h.projector.WaitForStartup(h.ctx) })
			waitLive := waitAsync(func() error { return h.projector.WaitFor(h.ctx, SubjectPosition(resetConsumedSubject, live)) })
			waitCurrent := waitAsync(func() error { return h.projector.WaitForCurrent(h.ctx) })
			// A full startup batch before s can be applied. With s as the
			// target, the batch with the first three events is full before s.
			appliedBefore := seqs[0]
			if held == "target" {
				appliedBefore = seqs[3]
			}
			window := time.After(resetObservationWindow)
		observe:
			for {
				select {
				case seq := <-h.projection.applied:
					if seq >= appliedBefore {
						t.Fatalf("seq %d applied while s is held", seq)
					}
				case err := <-waitStartup:
					t.Fatalf("WaitForStartup returned %v while s is held", err)
				case err := <-waitLive:
					t.Fatalf("WaitFor(live) returned %v while s is held", err)
				case err := <-waitCurrent:
					t.Fatalf("WaitForCurrent returned %v while s is held", err)
				case <-window:
					break observe
				}
			}
			gate.open()
			for _, waiter := range []<-chan error{waitStartup, waitLive, waitCurrent} {
				if err := <-waiter; err != nil {
					t.Fatal(err)
				}
			}
			if status := h.projector.Status(); status.StartupTargetSeq != seqs[5] || status.StartupMessages != 6 {
				t.Fatalf("startup target %d with %d messages, want %d with 6", status.StartupTargetSeq, status.StartupMessages, seqs[5])
			}
			h.expectApplied(append(seqs, live)...)
		})
	}
}

// TestProjectorResetWithLiveBatch puts s in a live batch and forces a reset
// before the batch flushes. The flush applies s and the later event in order,
// and no waiter returns before the flush.
func TestProjectorResetWithLiveBatch(t *testing.T) {
	for _, stage := range []string{"buffered", "flush"} {
		t.Run(stage, func(t *testing.T) {
			h := newResetHarness(t)
			projection := &resetBatchProjection{resetProjection: h.projection, startupSize: 1, liveSize: 2, liveDelay: time.Hour}
			if stage == "flush" {
				projection.liveSize = 1
			}
			h.start(projection, true)
			var held *gate
			if stage == "flush" {
				held = h.holdApply("s")
			}
			s := h.append(resetConsumedSubject, "s")
			if held != nil {
				receive(t, held.entered, "hold flush")
			} else {
				waitDelivered(t, h, s)
			}
			h.reconnect()
			waitS := waitAsync(func() error { return h.projector.WaitFor(h.ctx, SubjectPosition(resetConsumedSubject, s)) })
			if held != nil {
				next := h.append(resetConsumedSubject, "next")
				h.expectHeld("s is in a live batch flush", waitS)
				held.open()
				h.waitFor(next)
				h.expectApplied(s, next)
			} else {
				h.expectHeld("s is in an unflushed live batch", waitS)
				next := h.append(resetConsumedSubject, "next")
				h.waitFor(next)
				h.expectApplied(s, next)
			}
			if err := <-waitS; err != nil {
				t.Fatal(err)
			}
		})
	}
}

// waitDelivered waits until the projector reports seq as delivered.
func waitDelivered(t *testing.T, h *resetHarness, seq uint64) {
	t.Helper()
	for h.projector.Status().ObservedSeq < seq {
		select {
		case <-h.ctx.Done():
			t.Fatalf("seq %d was not delivered", seq)
		case <-time.After(time.Millisecond):
		}
	}
}

// redeliveringMessages returns the messages of an ordered consumer and then
// returns each message a second time, as a consumer that redelivers after a
// reset would do.
type redeliveringMessages struct {
	jetstream.MessagesContext
	last jetstream.Msg
}

func (m *redeliveringMessages) Next(opts ...jetstream.NextOpt) (jetstream.Msg, error) {
	if m.last != nil {
		msg := m.last
		m.last = nil
		return msg, nil
	}
	msg, err := m.MessagesContext.Next(opts...)
	m.last = msg
	return msg, err
}

type redeliveringConsumer struct{ jetstream.Consumer }

func (c redeliveringConsumer) Messages(opts ...jetstream.PullMessagesOpt) (jetstream.MessagesContext, error) {
	messages, err := c.Consumer.Messages(opts...)
	if err != nil {
		return nil, err
	}
	return &redeliveringMessages{MessagesContext: messages}, nil
}

type redeliveringStream struct{ jetstream.Stream }

func (s redeliveringStream) OrderedConsumer(ctx context.Context, cfg jetstream.OrderedConsumerConfig) (jetstream.Consumer, error) {
	consumer, err := s.Stream.OrderedConsumer(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return redeliveringConsumer{Consumer: consumer}, nil
}

// TestProjectorIgnoresRedeliveredEvent delivers every event twice, across a
// reset too. The projector applies each event once.
func TestProjectorIgnoresRedeliveredEvent(t *testing.T) {
	h := newResetHarness(t)
	h.stream = redeliveringStream{Stream: h.stream}
	h.start(h.projection, true)
	first := h.append(resetConsumedSubject, "first")
	h.waitFor(first)
	h.reconnect()
	h.append(resetUnconsumedSubject, "other")
	second := h.append(resetConsumedSubject, "second")
	h.waitFor(second)
	third := h.append(resetConsumedSubject, "third")
	h.waitFor(third)
	h.expectApplied(first, second, third)
}

// TestProjectorFailsWhenConsumerStops closes the connection. The iterator
// stops, so the projector reports a failure instead of a silent stall.
func TestProjectorFailsWhenConsumerStops(t *testing.T) {
	h := newResetHarness(t)
	h.start(h.projection, true)
	h.conn.Close()
	for h.projector.Err() == nil {
		select {
		case <-h.ctx.Done():
			t.Fatal("projector did not fail after its consumer stopped")
		case <-time.After(time.Millisecond):
		}
	}
	if err := h.projector.Err(); !errors.Is(err, ErrProjectionFailed) || !errors.Is(err, jetstream.ErrMsgIteratorClosed) {
		t.Fatalf("Err = %v, want a failure from the stopped consumer", err)
	}
}

// TestProjectorStopsDuringEndlessReset deletes the stream, so the ordered
// consumer reset retries without end. Run still returns when its context
// ends.
func TestProjectorStopsDuringEndlessReset(t *testing.T) {
	h := newResetHarness(t)
	projector := must(NewDecodedProjector(h.js, h.stream, h.projection, func(data []byte) (DecodedEvent[string], error) {
		return DecodedEvent[string]{Event: string(data)}, nil
	}, ProjectorOptions{}))
	runCtx, stop := context.WithCancel(h.ctx)
	defer stop()
	done := waitAsync(func() error { return projector.Run(runCtx) })
	if err := projector.WaitForStartup(h.ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.js.DeleteStream(h.ctx, "EVENTS_RESET"); err != nil {
		t.Fatal(err)
	}
	stop()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run = %v, want context.Canceled", err)
		}
	case <-h.ctx.Done():
		t.Fatal("Run did not return while the consumer reset retried")
	}
}

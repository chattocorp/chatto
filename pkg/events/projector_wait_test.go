package events

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// waitTestProjection consumes evt.wait.consumed.a and evt.wait.consumed.b.
// Apply fails for the event "fail".
type waitTestProjection struct {
	mu      sync.Mutex
	applied []string
}

func (*waitTestProjection) Subjects() []string {
	return []string{"evt.wait.consumed.a", "evt.wait.consumed.b"}
}

func (p *waitTestProjection) Apply(event string, _ uint64) error {
	if event == "fail" {
		return errors.New("test apply failure")
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.applied = append(p.applied, event)
	return nil
}

func (p *waitTestProjection) appliedEvents() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.applied...)
}

// gatedStream lets a test hold the projector's GetMsg calls. While gate is
// set, each GetMsg call sends on entered and then waits until release closes
// or its context ends. It sends the context error, if any, on cancelled.
type gatedStream struct {
	jetstream.Stream
	gate      atomic.Bool
	entered   chan struct{}
	release   chan struct{}
	cancelled chan error
}

func (s *gatedStream) GetMsg(ctx context.Context, seq uint64, opts ...jetstream.GetMsgOpt) (*jetstream.RawStreamMsg, error) {
	if s.gate.Load() {
		s.entered <- struct{}{}
		select {
		case <-s.release:
		case <-ctx.Done():
			s.cancelled <- ctx.Err()
			return nil, ctx.Err()
		}
	}
	return s.Stream.GetMsg(ctx, seq, opts...)
}

type waitTestEnv struct {
	stream     *gatedStream
	eventLog   *EncodedEventLog
	projector  *Projector
	projection *waitTestProjection
	msgGets    *atomic.Int64
	run        func()
}

// newWaitTestEnv builds a projector over a fresh stream. It does not start
// Run; call env.run for that. msgGets counts the MSG.GET requests that reach
// the stream leader.
func newWaitTestEnv(t *testing.T, ctx context.Context) *waitTestEnv {
	t.Helper()
	connection := startTestNATS(t)
	js, err := jetstream.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{Name: "EVENTS_WAIT_TEST", Subjects: []string{"evt.wait.>"}})
	if err != nil {
		t.Fatal(err)
	}
	var msgGets atomic.Int64
	sub, err := connection.Subscribe("$JS.API.STREAM.MSG.GET.EVENTS_WAIT_TEST", func(*nats.Msg) { msgGets.Add(1) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	if err := connection.Flush(); err != nil {
		t.Fatal(err)
	}
	gated := &gatedStream{
		Stream:    stream,
		entered:   make(chan struct{}, 1),
		release:   make(chan struct{}),
		cancelled: make(chan error, 1),
	}
	projection := &waitTestProjection{}
	projector := must(NewDecodedProjector(js, gated, projection, func(data []byte) (DecodedEvent[string], error) {
		return DecodedEvent[string]{Event: string(data), ID: string(data)}, nil
	}, ProjectorOptions{}))
	env := &waitTestEnv{
		stream:     gated,
		eventLog:   NewEncodedEventLog(js, stream, nil),
		projector:  projector,
		projection: projection,
		msgGets:    &msgGets,
	}
	var once sync.Once
	env.run = func() {
		once.Do(func() {
			runCtx, stop := context.WithCancel(context.Background())
			done := make(chan error, 1)
			go func() { done <- projector.Run(runCtx) }()
			t.Cleanup(func() {
				stop()
				<-done
			})
		})
	}
	return env
}

// startWaitTestEnv builds the projector and waits until its startup is
// complete.
func startWaitTestEnv(t *testing.T, ctx context.Context) *waitTestEnv {
	t.Helper()
	env := newWaitTestEnv(t, ctx)
	env.run()
	if err := env.projector.WaitForStartup(ctx); err != nil {
		t.Fatal(err)
	}
	return env
}

func (e *waitTestEnv) append(t *testing.T, ctx context.Context, subject, data string) uint64 {
	t.Helper()
	seq, err := e.eventLog.AppendEventually(ctx, subject, EncodedRecord{ID: data + "-" + subject, Data: []byte(data)})
	if err != nil {
		t.Fatal(err)
	}
	return seq
}

// holdBarrier pauses event application until the returned release function
// runs, so that the projection lags.
func holdBarrier(t *testing.T, p *Projector) func() {
	t.Helper()
	held := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- p.WithReadBarrier(func(uint64) error {
			close(held)
			<-release
			return nil
		})
	}()
	<-held
	var once sync.Once
	releaseFn := func() {
		once.Do(func() {
			close(release)
			if err := <-done; err != nil {
				t.Errorf("WithReadBarrier: %v", err)
			}
		})
	}
	t.Cleanup(releaseFn)
	return releaseFn
}

// waitForWaiters blocks until the projector holds n registered waiters.
func waitForWaiters(t *testing.T, p *Projector, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		p.mu.Lock()
		got := len(p.waiters)
		p.mu.Unlock()
		if got == n {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("projector has %d waiters, want %d", got, n)
		}
		time.Sleep(time.Millisecond)
	}
}

func waitTestContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func requireNotDone(t *testing.T, done <-chan error, what string) {
	t.Helper()
	select {
	case err := <-done:
		t.Fatalf("%s returned %v too early", what, err)
	default:
	}
}

func receive[T any](t *testing.T, ch <-chan T, what string) T {
	t.Helper()
	select {
	case value := <-ch:
		return value
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
		var zero T
		return zero
	}
}

func TestProjectorWaitForReachedPositionDoesNotReadStream(t *testing.T) {
	ctx := waitTestContext(t)
	env := startWaitTestEnv(t, ctx)
	seq := env.append(t, ctx, "evt.wait.consumed.a", "first")
	if err := env.projector.WaitFor(ctx, SubjectPosition("evt.wait.consumed.a", seq)); err != nil {
		t.Fatal(err)
	}
	before := env.msgGets.Load()
	for _, pos := range []StreamPosition{
		SubjectPosition("evt.wait.consumed.a", seq),
		SubjectPosition("evt.wait.consumed.>", seq),
	} {
		if err := env.projector.WaitFor(ctx, pos); err != nil {
			t.Fatalf("WaitFor(%v): %v", pos, err)
		}
	}
	if got := env.msgGets.Load() - before; got != 0 {
		t.Fatalf("WaitFor on a reached position sent %d MSG.GET requests, want 0", got)
	}
}

func TestProjectorWaitForBlocksUntilLaggingProjectionApplies(t *testing.T) {
	ctx := waitTestContext(t)
	env := startWaitTestEnv(t, ctx)
	release := holdBarrier(t, env.projector)
	seq := env.append(t, ctx, "evt.wait.consumed.a", "lagging")
	env.stream.gate.Store(true)

	done := make(chan error, 1)
	go func() { done <- env.projector.WaitFor(ctx, SubjectPosition("evt.wait.consumed.a", seq)) }()
	// The delayed validation reads the target message. The position is
	// valid, so the wait goes on after the read.
	receive(t, env.stream.entered, "the validation read")
	env.stream.gate.Store(false)
	close(env.stream.release)
	waitForWaiters(t, env.projector, 1)
	requireNotDone(t, done, "WaitFor")
	if got := env.projector.LastSeq(); got >= seq {
		t.Fatalf("LastSeq = %d while the barrier holds, want below %d", got, seq)
	}

	release()
	if err := receive(t, done, "WaitFor"); err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if got := env.projection.appliedEvents(); len(got) != 1 || got[0] != "lagging" {
		t.Fatalf("applied events after WaitFor = %v", got)
	}
}

func TestProjectorWaitForRejectsUnreachablePositions(t *testing.T) {
	ctx := waitTestContext(t)
	env := startWaitTestEnv(t, ctx)
	consumedSeq := env.append(t, ctx, "evt.wait.consumed.a", "first")
	if err := env.projector.WaitFor(ctx, SubjectPosition("evt.wait.consumed.a", consumedSeq)); err != nil {
		t.Fatal(err)
	}
	// The projector does not receive this message, so LastSeq never reaches
	// it. Without validation, a wait for it would block until ctx ends.
	otherSeq := env.append(t, ctx, "evt.wait.other.a", "other")

	for _, tc := range []struct {
		name      string
		pos       StreamPosition
		want      error
		wantReads int64
	}{
		{"exact unconsumed subject", SubjectPosition("evt.wait.other.a", otherSeq), ErrProjectionSubjectNotConsumed, 0},
		{"filter without consumed subjects", SubjectPosition("evt.wait.other.>", otherSeq), ErrProjectionSubjectNotConsumed, 0},
		{"filter on an unconsumed subject", SubjectPosition("evt.wait.>", otherSeq), ErrProjectionSubjectNotConsumed, 1},
		{"filter that does not match the subject", SubjectPosition("evt.wait.consumed.>", otherSeq), ErrProjectionSequenceSubjectMismatch, 1},
		{"exact consumed subject on another sequence", SubjectPosition("evt.wait.consumed.b", otherSeq), ErrProjectionSequenceSubjectMismatch, 1},
		{"sequence after the stream tail", SubjectPosition("evt.wait.consumed.a", otherSeq+10), jetstream.ErrMsgNotFound, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := env.msgGets.Load()
			err := env.projector.WaitFor(ctx, tc.pos)
			if !errors.Is(err, tc.want) {
				t.Fatalf("WaitFor error = %v, want %v", err, tc.want)
			}
			if got := env.msgGets.Load() - before; got != tc.wantReads {
				t.Fatalf("WaitFor sent %d MSG.GET requests, want %d", got, tc.wantReads)
			}
			waitForWaiters(t, env.projector, 0)
		})
	}
}

func TestProjectorWaitForEndsWhenTargetAppliesDuringValidation(t *testing.T) {
	ctx := waitTestContext(t)
	env := startWaitTestEnv(t, ctx)
	release := holdBarrier(t, env.projector)
	seq := env.append(t, ctx, "evt.wait.consumed.a", "target")
	env.stream.gate.Store(true)

	done := make(chan error, 1)
	go func() { done <- env.projector.WaitFor(ctx, SubjectPosition("evt.wait.consumed.a", seq)) }()
	receive(t, env.stream.entered, "the validation read")
	// The read stays blocked. The applied target must end the wait anyway.
	release()
	if err := receive(t, done, "WaitFor"); err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if got := env.projection.appliedEvents(); len(got) != 1 || got[0] != "target" {
		t.Fatalf("applied events after WaitFor = %v", got)
	}
	if err := receive(t, env.stream.cancelled, "cancellation of the validation read"); !errors.Is(err, context.Canceled) {
		t.Fatalf("validation read ended with %v, want context.Canceled", err)
	}
	waitForWaiters(t, env.projector, 0)
}

func TestProjectorWaitForReportsFailureDuringValidation(t *testing.T) {
	ctx := waitTestContext(t)
	env := startWaitTestEnv(t, ctx)
	release := holdBarrier(t, env.projector)
	seq := env.append(t, ctx, "evt.wait.consumed.a", "fail")
	env.stream.gate.Store(true)

	done := make(chan error, 1)
	go func() { done <- env.projector.WaitFor(ctx, SubjectPosition("evt.wait.consumed.a", seq)) }()
	receive(t, env.stream.entered, "the validation read")
	// Apply fails at the target while the read stays blocked.
	release()
	if err := receive(t, done, "WaitFor"); !errors.Is(err, ErrProjectionFailed) {
		t.Fatalf("WaitFor error = %v, want ErrProjectionFailed", err)
	}
	if err := receive(t, env.stream.cancelled, "cancellation of the validation read"); !errors.Is(err, context.Canceled) {
		t.Fatalf("validation read ended with %v, want context.Canceled", err)
	}
}

func TestProjectorWaitForHonorsCancellationDuringValidation(t *testing.T) {
	ctx := waitTestContext(t)
	env := startWaitTestEnv(t, ctx)
	holdBarrier(t, env.projector)
	seq := env.append(t, ctx, "evt.wait.consumed.a", "target")
	env.stream.gate.Store(true)

	waitCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- env.projector.WaitFor(waitCtx, SubjectPosition("evt.wait.consumed.a", seq)) }()
	receive(t, env.stream.entered, "the validation read")
	cancel()
	if err := receive(t, done, "WaitFor"); !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitFor error = %v, want context.Canceled", err)
	}
	if err := receive(t, env.stream.cancelled, "cancellation of the validation read"); !errors.Is(err, context.Canceled) {
		t.Fatalf("validation read ended with %v, want context.Canceled", err)
	}
	waitForWaiters(t, env.projector, 0)
}

func TestProjectorWaitForBeforeRun(t *testing.T) {
	ctx := waitTestContext(t)
	env := newWaitTestEnv(t, ctx)
	seq := env.append(t, ctx, "evt.wait.consumed.a", "history")

	env.stream.gate.Store(true)
	done := make(chan error, 1)
	go func() { done <- env.projector.WaitFor(ctx, SubjectPosition("evt.wait.consumed.a", seq)) }()
	waitForWaiters(t, env.projector, 1)
	// Without Run, the projector cannot reach the position. The valid
	// position passes validation, and the wait goes on.
	receive(t, env.stream.entered, "the validation read")
	env.stream.gate.Store(false)
	close(env.stream.release)
	requireNotDone(t, done, "WaitFor before Run")

	env.run()
	if err := receive(t, done, "WaitFor"); err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if got := env.projection.appliedEvents(); len(got) != 1 || got[0] != "history" {
		t.Fatalf("applied events after WaitFor = %v", got)
	}
}

func TestProjectorWaitForDuringStartup(t *testing.T) {
	ctx := waitTestContext(t)
	env := newWaitTestEnv(t, ctx)
	env.append(t, ctx, "evt.wait.consumed.a", "first")
	env.append(t, ctx, "evt.wait.other.a", "other")
	seq := env.append(t, ctx, "evt.wait.consumed.b", "last")

	// The barrier holds startup replay before its first event.
	release := holdBarrier(t, env.projector)
	env.run()
	done := make(chan error, 1)
	go func() { done <- env.projector.WaitFor(ctx, SubjectPosition("evt.wait.consumed.b", seq)) }()
	waitForWaiters(t, env.projector, 1)
	requireNotDone(t, done, "WaitFor during startup")
	if env.projector.Status().StartupComplete {
		t.Fatal("startup completed while the barrier holds replay")
	}

	release()
	if err := receive(t, done, "WaitFor"); err != nil {
		t.Fatalf("WaitFor: %v", err)
	}
	if got := env.projection.appliedEvents(); len(got) != 2 || got[0] != "first" || got[1] != "last" {
		t.Fatalf("applied events after WaitFor = %v", got)
	}
}

func TestSubjectFiltersOverlap(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{"evt.user.U1.created", "evt.user.*.created", true},
		{"evt.user.*.created", "evt.user.*.deleted", false},
		{"evt.user.>", "evt.user.*.created", true},
		{"evt.user.U1.>", "evt.room.>", false},
		{"evt.>", "evt.room.*.posted", true},
		{"evt.room.>", "evt.room", false},
		{"evt.*", "evt.room.posted", false},
		{"evt.*.>", "evt.room", false},
		{"evt.*.>", "evt.room.posted", true},
		{">", "evt.room.posted", true},
		{"evt.>", "evt.>", true},
		{"", "evt.>", false},
		{"evt..x", "evt.*.x", false},
	} {
		if got := subjectFiltersOverlap(tc.a, tc.b); got != tc.want {
			t.Errorf("subjectFiltersOverlap(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
		if got := subjectFiltersOverlap(tc.b, tc.a); got != tc.want {
			t.Errorf("subjectFiltersOverlap(%q, %q) = %v, want %v", tc.b, tc.a, got, tc.want)
		}
	}
}

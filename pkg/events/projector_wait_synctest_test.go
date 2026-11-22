package events

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

// scriptedMsgStream answers GetMsg with subject or err. When release is not
// nil, GetMsg first waits until release closes or its context ends.
type scriptedMsgStream struct {
	stubStream
	subject string
	err     error
	release chan struct{}
	calls   int
}

func (s *scriptedMsgStream) GetMsg(ctx context.Context, seq uint64, _ ...jetstream.GetMsgOpt) (*jetstream.RawStreamMsg, error) {
	s.calls++
	if s.release != nil {
		select {
		case <-s.release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if s.err != nil {
		return nil, s.err
	}
	return &jetstream.RawStreamMsg{Subject: s.subject, Sequence: seq}, nil
}

// scriptedWaitProjector returns a running projector that consumes evt.a and
// reads target messages from stream. It has no consumer.
func scriptedWaitProjector(stream *scriptedMsgStream) *Projector {
	p := statusProjector(time.Minute)
	p.stream = stream
	p.subjects = []string{"evt.a"}
	p.subjectMatchers = compileSubjectFilters(p.subjects)
	return p
}

type waitResult struct{ err error }

func startWait(ctx context.Context, p *Projector, pos StreamPosition) <-chan waitResult {
	done := make(chan waitResult, 1)
	go func() { done <- waitResult{p.WaitFor(ctx, pos)} }()
	return done
}

func TestProjectorWaitForValidatesAtOnceNearDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stream := &scriptedMsgStream{err: jetstream.ErrMsgNotFound}
		p := scriptedWaitProjector(stream)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		start := time.Now()
		err := p.WaitFor(ctx, SubjectPosition("evt.a", 1))
		if !errors.Is(err, jetstream.ErrMsgNotFound) {
			t.Fatalf("WaitFor error = %v, want ErrMsgNotFound", err)
		}
		if elapsed := time.Since(start); elapsed != 0 {
			t.Fatalf("WaitFor validated after %v, want at once", elapsed)
		}
	})
}

func TestProjectorWaitForContinuesAfterSuccessfulValidation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		finish func(*Projector)
		want   error
	}{
		{"reach", func(p *Projector) { p.advance(1) }, nil},
		{"failure", func(p *Projector) { p.fail(1, errors.New("broken")) }, ErrProjectionFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				stream := &scriptedMsgStream{subject: "evt.a"}
				p := scriptedWaitProjector(stream)
				done := startWait(context.Background(), p, SubjectPosition("evt.a", 1))
				time.Sleep(projectionWaitValidationDelay)
				synctest.Wait()
				// The validation read has run and its result is processed:
				// every goroutine is blocked, and only the wait remains.
				if stream.calls != 1 {
					t.Fatalf("validation reads = %d, want 1", stream.calls)
				}
				select {
				case r := <-done:
					t.Fatalf("WaitFor returned %v after a successful validation", r.err)
				default:
				}
				tc.finish(p)
				r := <-done
				if !errors.Is(r.err, tc.want) || (tc.want == nil && r.err != nil) {
					t.Fatalf("WaitFor error = %v, want %v", r.err, tc.want)
				}
			})
		})
	}
}

func TestProjectorWaitForIgnoresLateValidationResult(t *testing.T) {
	for _, tc := range []struct {
		name   string
		finish func(*Projector, context.CancelFunc)
		want   error
	}{
		{"reach", func(p *Projector, _ context.CancelFunc) { p.advance(1) }, nil},
		{"failure", func(p *Projector, _ context.CancelFunc) { p.fail(1, errors.New("broken")) }, ErrProjectionFailed},
		{"cancellation", func(_ *Projector, cancel context.CancelFunc) { cancel() }, context.Canceled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// The read would fail, but it ends only after the wait ended.
				stream := &scriptedMsgStream{err: errors.New("late read error"), release: make(chan struct{})}
				p := scriptedWaitProjector(stream)
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				done := startWait(ctx, p, SubjectPosition("evt.a", 1))
				time.Sleep(projectionWaitValidationDelay)
				synctest.Wait()
				if stream.calls != 1 {
					t.Fatalf("validation reads = %d, want 1", stream.calls)
				}
				tc.finish(p, cancel)
				r := <-done
				if !errors.Is(r.err, tc.want) || (tc.want == nil && r.err != nil) {
					t.Fatalf("WaitFor error = %v, want %v", r.err, tc.want)
				}
				close(stream.release)
				// synctest.Test fails when the validation goroutine does not
				// end. Its late result must not block or reach a caller.
				synctest.Wait()
				p.mu.Lock()
				defer p.mu.Unlock()
				if len(p.waiters) != 0 {
					t.Fatalf("waiters after the wait = %d, want 0", len(p.waiters))
				}
			})
		})
	}
}

func TestProjectorWaitForValidationErrorEndsWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		stream := &scriptedMsgStream{subject: "evt.b"}
		p := scriptedWaitProjector(stream)
		start := time.Now()
		err := p.WaitFor(context.Background(), SubjectPosition("evt.*", 1))
		if !errors.Is(err, ErrProjectionSubjectNotConsumed) {
			t.Fatalf("WaitFor error = %v, want ErrProjectionSubjectNotConsumed", err)
		}
		if elapsed := time.Since(start); elapsed != projectionWaitValidationDelay {
			t.Fatalf("WaitFor failed after %v, want %v", elapsed, projectionWaitValidationDelay)
		}
		if len(p.waiters) != 0 {
			t.Fatalf("waiters after the wait = %d, want 0", len(p.waiters))
		}
	})
}

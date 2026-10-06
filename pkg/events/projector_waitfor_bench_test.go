package events_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	. "hmans.de/chatto/pkg/events"
)

// BenchmarkWaitForAppliedSequence measures a read-your-writes wait for an
// event that the projector already applied, the common case after a write.
func BenchmarkWaitForAppliedSequence(b *testing.B) {
	for name, filter := range map[string]func(subject string) string{
		"exact subject":   func(subject string) string { return subject },
		"wildcard filter": func(string) string { return "evt.codec.bench.>" },
	} {
		b.Run(name, func(b *testing.B) {
			js, stream := setupTestStream(b)
			ctx := testContext(b)
			const subject = "evt.codec.bench.created"
			seq, err := NewEncodedEventLog(js, stream, nil).AppendEventually(ctx, subject, EncodedRecord{ID: "bench", Data: []byte("bench:alpha")})
			if err != nil {
				b.Fatal(err)
			}
			projector := must(NewDecodedProjector(js, stream, &codecTestProjection{subject: "evt.codec.bench.>"}, decodeCodecTestEvent, ProjectorOptions{}))
			runCtx, cancel := context.WithCancel(ctx)
			b.Cleanup(cancel)
			go func() { _ = projector.Run(runCtx) }()
			if err := projector.WaitForStartup(ctx); err != nil {
				b.Fatal(err)
			}
			position := SubjectPosition(filter(subject), seq)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if err := projector.WaitFor(ctx, position); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// countingStream counts the broker reads that confirm a wait's subject.
type countingStream struct {
	jetstream.Stream
	gets atomic.Int32
}

func (s *countingStream) GetMsg(ctx context.Context, seq uint64, opts ...jetstream.GetMsgOpt) (*jetstream.RawStreamMsg, error) {
	s.gets.Add(1)
	return s.Stream.GetMsg(ctx, seq, opts...)
}

// A wait for an applied sequence on an exact subject needs no broker read. A
// wildcard filter still confirms the subject, and an exact subject that the
// projector does not consume still fails.
func TestWaitForSkipsBrokerReadForAppliedExactSubject(t *testing.T) {
	js, realStream := setupTestStream(t)
	ctx := testContext(t)
	const subject = "evt.codec.fast.created"
	seq, err := NewEncodedEventLog(js, realStream, nil).AppendEventually(ctx, subject, EncodedRecord{ID: "fast", Data: []byte("fast:alpha")})
	if err != nil {
		t.Fatal(err)
	}
	stream := &countingStream{Stream: realStream}
	projector := must(NewDecodedProjector(js, stream, &codecTestProjection{subject: "evt.codec.fast.>"}, decodeCodecTestEvent, ProjectorOptions{}))
	runCtx, cancel := context.WithCancel(ctx)
	t.Cleanup(cancel)
	go func() { _ = projector.Run(runCtx) }()
	if err := projector.WaitForStartup(ctx); err != nil {
		t.Fatal(err)
	}
	stream.gets.Store(0)

	if err := projector.WaitFor(ctx, SubjectPosition(subject, seq)); err != nil || stream.gets.Load() != 0 {
		t.Fatalf("exact applied wait = %v with %d broker reads, want nil with 0", err, stream.gets.Load())
	}
	if err := projector.WaitFor(ctx, SubjectPosition("evt.codec.fast.>", seq)); err != nil || stream.gets.Load() != 1 {
		t.Fatalf("wildcard applied wait = %v with %d broker reads, want nil with 1", err, stream.gets.Load())
	}
	if err := projector.WaitFor(ctx, SubjectPosition("evt.other.created", seq)); !errors.Is(err, ErrProjectionSubjectNotConsumed) || stream.gets.Load() != 1 {
		t.Fatalf("unconsumed exact wait = %v with %d broker reads, want ErrProjectionSubjectNotConsumed with 1", err, stream.gets.Load())
	}
}

package events

import (
	"context"
	"crypto/sha256"
	"fmt"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

// countingProjection counts applied events. With work > 0, Apply hashes the
// event work times, to simulate the decode and apply cost of a real
// projection. The replay benchmark thus measures the projector and consumer
// with and without projection work.
type countingProjection struct {
	applied int
	work    int
	sum     [sha256.Size]byte
}

func (*countingProjection) Subjects() []string { return []string{"evt.bench.>"} }

func (p *countingProjection) Apply(event string, _ uint64) error {
	p.applied++
	for range p.work {
		p.sum = sha256.Sum256(append(p.sum[:0:0], event...))
	}
	return nil
}

// BenchmarkProjectorReplay measures a cold startup replay through a real
// embedded NATS server and an ordered consumer, from Run to startup complete.
func BenchmarkProjectorReplay(b *testing.B) {
	const events = 20_000
	ctx := context.Background()
	connection := startTestNATS(b)
	js := must(jetstream.New(connection))
	stream := must(js.CreateStream(ctx, jetstream.StreamConfig{Name: "EVENTS_BENCH", Subjects: []string{"evt.bench.>"}}))
	payload := []byte("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	for i := range events {
		if _, err := js.PublishAsync(fmt.Sprintf("evt.bench.%d", i%64), payload); err != nil {
			b.Fatal(err)
		}
	}
	<-js.PublishAsyncComplete()

	for _, work := range []int{0, 8} {
		b.Run(fmt.Sprintf("work_%d", work), func(b *testing.B) {
			benchmarkProjectorReplay(b, js, stream, events, work)
		})
	}
}

func benchmarkProjectorReplay(b *testing.B, js jetstream.JetStream, stream jetstream.Stream, events, work int) {
	ctx := context.Background()
	decode := func(data []byte) (DecodedEvent[string], error) { return DecodedEvent[string]{Event: string(data)}, nil }
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		projection := &countingProjection{work: work}
		projector := must(NewDecodedProjector(js, stream, projection, decode, ProjectorOptions{}))
		runCtx, stop := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() { done <- projector.Run(runCtx) }()
		if err := projector.WaitForStartup(ctx); err != nil {
			b.Fatal(err)
		}
		stop()
		<-done
		if projection.applied != events {
			b.Fatalf("applied %d events, want %d", projection.applied, events)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(b.Elapsed().Nanoseconds())/float64(b.N*events), "ns/event")
}

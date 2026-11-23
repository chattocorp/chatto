package events

import (
	"context"
	"fmt"
	"runtime"
	"testing"

	"github.com/nats-io/nats.go/jetstream"
)

// idleProjectorHeapLimit is the largest live heap that one idle projector can
// keep in TestIdleProjectorRetainedHeap. Measured with nats.go v1.56.0, one
// idle projector keeps about 50 KiB with Consume and with the current
// iterator options. A Messages iterator with only PullMaxBytes keeps an
// 8 MB channel buffer (1,000,000 pointers). The limit is about ten times
// the current value, so normal changes pass, and well below one large
// buffer, so that regression fails.
const idleProjectorHeapLimit = 512 * 1024

// measureIdleProjectorHeap starts count projectors on one small stream, waits
// until each one completes startup, and returns the live heap that each
// running projector adds. It forces garbage collection before and after it
// starts the projectors, so only memory that the running projectors keep is
// counted. The in-process NATS server also counts: it keeps the state of each
// ordered consumer.
func measureIdleProjectorHeap(t testing.TB, count int) uint64 {
	t.Helper()
	ctx := context.Background()
	connection := startTestNATS(t)
	js := must(jetstream.New(connection))
	stream := must(js.CreateStream(ctx, jetstream.StreamConfig{Name: "EVENTS_HEAP", Subjects: []string{"evt.bench.>"}}))
	for i := range 100 {
		if _, err := js.Publish(ctx, fmt.Sprintf("evt.bench.%d", i%8), []byte("event")); err != nil {
			t.Fatal(err)
		}
	}
	decode := func(data []byte) (DecodedEvent[string], error) { return DecodedEvent[string]{Event: string(data)}, nil }

	before := liveHeap()
	runCtx, stop := context.WithCancel(ctx)
	projectors := make([]*Projector, count)
	done := make(chan error, count)
	for i := range projectors {
		projectors[i] = must(NewDecodedProjector(js, stream, &countingProjection{}, decode, ProjectorOptions{}))
		go func() { done <- projectors[i].Run(runCtx) }()
	}
	t.Cleanup(func() {
		stop()
		for range projectors {
			<-done
		}
	})
	for _, projector := range projectors {
		if err := projector.WaitForStartup(ctx); err != nil {
			t.Fatal(err)
		}
	}
	after := liveHeap()
	runtime.KeepAlive(projectors)
	if after < before {
		return 0
	}
	return (after - before) / uint64(count)
}

// liveHeap returns HeapAlloc after two forced collections. The second
// collection frees objects that finalizers of the first one released.
func liveHeap() uint64 {
	runtime.GC()
	runtime.GC()
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}

// TestIdleProjectorRetainedHeap fails when an idle projector keeps a large
// heap, for example a Messages iterator buffer sized from a large batch.
// A Chatto replica runs about eleven projectors, so the test starts eleven.
func TestIdleProjectorRetainedHeap(t *testing.T) {
	const projectors = 11
	perProjector := measureIdleProjectorHeap(t, projectors)
	t.Logf("live heap per idle projector: %d KiB (%d projectors)", perProjector/1024, projectors)
	if perProjector > idleProjectorHeapLimit {
		t.Fatalf("idle projector keeps %d bytes of live heap, limit %d", perProjector, idleProjectorHeapLimit)
	}
}

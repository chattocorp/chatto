package core

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// projectionBenchmarkStoreEnv names a copy of an embedded NATS data directory
// whose EVT stream supplies a real-world replay fixture. Never point it at a
// live server's directory: the benchmark opens the store with its own server.
// The copy holds real user data; keep it under .context/ and delete it after
// use.
const projectionBenchmarkStoreEnv = "CHATTO_BENCH_EVT_STORE_DIR"

// BenchmarkProjectionRetainedHeapFromStore reports live Go heap per
// projection after replaying a real EVT stream. It complements the synthetic
// BenchmarkProjectionRetainedHeap fixture with production event mixes. Run it
// with -benchtime=1x and CHATTO_BENCH_EVT_STORE_DIR set to a copied NATS data
// directory.
func BenchmarkProjectionRetainedHeapFromStore(b *testing.B) {
	storeDir := projectionBenchmarkStoreDir(b)
	if b.N != 1 {
		b.Skip("run with -benchtime=1x")
	}
	if os.Getenv("CHATTO_BENCH_HEAP_PROFILE_DIR") != "" {
		runtime.MemProfileRate = 1
	}
	fixture := loadProjectionBenchmarkStoreFixture(b, storeDir)

	for _, scope := range []string{"room_timeline", "threads", "reactions", "assets", "rbac", "content_keys", "infallible_content_view"} {
		b.Run(scope, func(b *testing.B) {
			if b.N != 1 {
				b.Skip("run with -benchtime=1x")
			}
			runtime.GC()
			var before runtime.MemStats
			runtime.ReadMemStats(&before)

			b.ReportAllocs()
			b.ResetTimer()
			targets, err := replayProjectionBenchmarkFixture(fixture, scope)
			b.StopTimer()
			if err != nil {
				b.Fatal(err)
			}

			runtime.GC()
			var after runtime.MemStats
			runtime.ReadMemStats(&after)
			retainedBytes := int64(after.HeapAlloc) - int64(before.HeapAlloc)
			var estimatedBytes int64
			for _, target := range targets {
				estimatedBytes += target.estimate()
			}
			b.ReportMetric(float64(retainedBytes), "retained-heap-B")
			b.ReportMetric(float64(retainedBytes)/float64(len(fixture)), "retained-heap-B/event")
			b.ReportMetric(float64(estimatedBytes), "estimated-B")
			b.ReportMetric(float64(len(fixture)), "events/replay")
			writeProjectionBenchmarkHeapProfile(b, "store-"+scope, len(fixture))
			runtime.KeepAlive(targets)
		})
	}
}

// projectionBenchmarkStoreDir returns the configured store copy or skips.
func projectionBenchmarkStoreDir(b *testing.B) string {
	b.Helper()
	configured := os.Getenv(projectionBenchmarkStoreEnv)
	if configured == "" {
		b.Skipf("set %s to a copied NATS data directory", projectionBenchmarkStoreEnv)
	}
	// Resolve relative paths against the module root, not the package
	// directory that go test runs in, and refuse missing directories so NATS
	// does not create an empty store in the source tree.
	storeDir := projectionBenchmarkOutputDirectory(b, configured)
	if info, err := os.Stat(storeDir); err != nil || !info.IsDir() {
		b.Fatalf("%s=%q is not a directory", projectionBenchmarkStoreEnv, configured)
	}
	return storeDir
}

// loadProjectionBenchmarkStoreFixture reads every EVT record from a copied
// embedded NATS data directory with an in-process server.
func loadProjectionBenchmarkStoreFixture(b *testing.B, storeDir string) []projectionBenchmarkWireEvent {
	b.Helper()
	ns, err := server.NewServer(&server.Options{
		JetStream:  true,
		DontListen: true,
		StoreDir:   storeDir,
		NoSigs:     true,
	})
	if err != nil {
		b.Fatalf("create NATS server: %v", err)
	}
	ns.Start()
	defer func() {
		ns.Shutdown()
		ns.WaitForShutdown()
	}()
	if !ns.ReadyForConnections(30 * time.Second) {
		b.Fatal("NATS server not ready")
	}
	nc, err := nats.Connect(nats.DefaultURL, nats.InProcessServer(ns))
	if err != nil {
		b.Fatalf("connect to NATS: %v", err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc)
	if err != nil {
		b.Fatalf("create JetStream context: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	stream, err := js.Stream(ctx, "EVT")
	if err != nil {
		b.Fatalf("open EVT stream: %v", err)
	}
	info, err := stream.Info(ctx)
	if err != nil {
		b.Fatalf("read EVT stream info: %v", err)
	}
	consumer, err := stream.OrderedConsumer(ctx, jetstream.OrderedConsumerConfig{})
	if err != nil {
		b.Fatalf("create EVT consumer: %v", err)
	}

	fixture := make([]projectionBenchmarkWireEvent, 0, info.State.Msgs)
	// Stop at the stream's last sequence. An empty batch also ends the read,
	// so records that expire while the benchmark runs cannot stall it.
	for len(fixture) == 0 || fixture[len(fixture)-1].seq < info.State.LastSeq {
		if err := ctx.Err(); err != nil {
			b.Fatalf("read EVT records: %v", err)
		}
		batch, err := consumer.Fetch(1_000, jetstream.FetchMaxWait(5*time.Second))
		if err != nil {
			b.Fatalf("fetch EVT records: %v", err)
		}
		received := 0
		for msg := range batch.Messages() {
			metadata, err := msg.Metadata()
			if err != nil {
				b.Fatalf("read EVT record metadata: %v", err)
			}
			fixture = append(fixture, projectionBenchmarkWireEvent{
				subject: msg.Subject(),
				data:    append([]byte(nil), msg.Data()...),
				seq:     metadata.Sequence.Stream,
			})
			received++
		}
		if err := batch.Error(); err != nil && !errors.Is(err, nats.ErrTimeout) {
			b.Fatalf("fetch EVT records: %v", err)
		}
		if received == 0 {
			break
		}
	}
	b.Logf("loaded %d EVT records (%d bytes)", len(fixture), projectionBenchmarkWireBytes(fixture))
	return fixture
}

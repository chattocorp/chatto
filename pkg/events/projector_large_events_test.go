package events

import (
	"bytes"
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

// sequenceProjection records the stream sequence and size of each applied
// event. delay makes each Apply slow, so the iterator buffer fills while the
// projector is behind.
type sequenceProjection struct {
	mu    sync.Mutex
	seqs  []uint64
	sizes []int
	delay time.Duration
}

func (*sequenceProjection) Subjects() []string { return []string{"evt.large.>"} }

func (p *sequenceProjection) Apply(event []byte, seq uint64) error {
	if p.delay > 0 {
		time.Sleep(p.delay)
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.seqs = append(p.seqs, seq)
	p.sizes = append(p.sizes, len(event))
	return nil
}

// startLargePayloadNATS starts an in-process server that accepts messages up
// to maxPayload bytes. The NATS default is 1 MiB.
func startLargePayloadNATS(t *testing.T, maxPayload int32) *nats.Conn {
	t.Helper()
	natsServer, err := server.NewServer(&server.Options{
		JetStream:  true,
		DontListen: true,
		StoreDir:   t.TempDir(),
		NoSigs:     true,
		MaxPayload: maxPayload,
	})
	if err != nil {
		t.Fatalf("create NATS server: %v", err)
	}
	natsServer.Start()
	t.Cleanup(func() {
		natsServer.Shutdown()
		natsServer.WaitForShutdown()
	})
	if !natsServer.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server did not become ready")
	}
	connection, err := nats.Connect(nats.DefaultURL, nats.InProcessServer(natsServer))
	if err != nil {
		t.Fatalf("connect to NATS server: %v", err)
	}
	t.Cleanup(connection.Close)
	return connection
}

// TestProjectorDeliversLargeEventsInOrder replays and then follows a stream
// with one event of almost 16 MiB, a burst of large events that together are
// larger than 16 MiB, and more small events than one pull batch. The
// projection is slow, so the iterator buffer fills. Every event must be
// applied once and in order, the client must not drop a message, and the
// ordered consumer must not reset.
func TestProjectorDeliversLargeEventsInOrder(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	connection := startLargePayloadNATS(t, 32*1024*1024)
	var dropped atomic.Int64
	connection.SetErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
		if err == nats.ErrSlowConsumer {
			dropped.Add(1)
		}
	})
	var consumerCreates atomic.Int64
	if _, err := connection.Subscribe("$JS.API.CONSUMER.CREATE.>", func(*nats.Msg) { consumerCreates.Add(1) }); err != nil {
		t.Fatal(err)
	}
	js := must(jetstream.New(connection))
	stream := must(js.CreateStream(ctx, jetstream.StreamConfig{
		Name:       "EVENTS_LARGE",
		Subjects:   []string{"evt.large.>"},
		MaxMsgSize: 32 * 1024 * 1024,
	}))

	var want []int
	publish := func(count, size int) {
		t.Helper()
		for range count {
			payload := bytes.Repeat([]byte{'x'}, size)
			if _, err := js.Publish(ctx, fmt.Sprintf("evt.large.%d", len(want)%4), payload); err != nil {
				t.Fatal(err)
			}
			want = append(want, size)
		}
	}
	// Replayed at startup: one event just below the earlier 16 MiB byte
	// window, 20 events of 1 MiB (20 MiB together), and 1.5 pull batches of
	// small events.
	publish(1, 16*1024*1024-4096)
	publish(20, 1024*1024)
	publish(projectionPullMaxMessages*3/2, 256)

	projection := &sequenceProjection{delay: time.Millisecond}
	decode := func(data []byte) (DecodedEvent[[]byte], error) { return DecodedEvent[[]byte]{Event: data}, nil }
	projector := must(NewDecodedProjector(js, stream, projection, decode, ProjectorOptions{}))
	runCtx, stop := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- projector.Run(runCtx) }()
	defer func() {
		stop()
		if err := <-done; err != nil && runCtx.Err() == nil {
			t.Errorf("Run: %v", err)
		}
	}()
	if err := projector.WaitForStartup(ctx); err != nil {
		t.Fatal(err)
	}

	// Delivered live: the same mix again.
	publish(1, 16*1024*1024-4096)
	publish(20, 1024*1024)
	publish(projectionPullMaxMessages*3/2, 256)
	if err := projector.WaitForCurrent(ctx); err != nil {
		t.Fatal(err)
	}

	projection.mu.Lock()
	defer projection.mu.Unlock()
	if len(projection.seqs) != len(want) {
		t.Fatalf("applied %d events, want %d", len(projection.seqs), len(want))
	}
	for i, seq := range projection.seqs {
		if seq != uint64(i+1) {
			t.Fatalf("event %d has sequence %d, want %d", i, seq, i+1)
		}
		if projection.sizes[i] != want[i] {
			t.Fatalf("event %d has %d bytes, want %d", i, projection.sizes[i], want[i])
		}
	}
	if n := dropped.Load(); n != 0 {
		t.Fatalf("client dropped messages %d times (slow consumer)", n)
	}
	if n := consumerCreates.Load(); n != 1 {
		t.Fatalf("ordered consumer created %d times, want 1 (no reset)", n)
	}
}

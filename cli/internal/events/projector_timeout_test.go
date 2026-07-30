package events

import (
	"context"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/testutil"
)

type timeoutTestProjection struct {
	count atomic.Int64
}

func (*timeoutTestProjection) Subjects() []string { return []string{"evt.timeout.>"} }
func (p *timeoutTestProjection) Apply(string, uint64) error {
	p.count.Add(1)
	return nil
}
func (*timeoutTestProjection) Snapshot() ([]byte, error) { return nil, nil }
func (*timeoutTestProjection) Restore([]byte) error      { return nil }
func (*timeoutTestProjection) SnapshotContractID() string {
	return "timeout-test-v1"
}

type timeoutSnapshotSource struct {
	canceled chan struct{}
}

func (s *timeoutSnapshotSource) LoadProjectionSnapshot(ctx context.Context, _ ProjectionSnapshotLoadRequest) (ProjectionSnapshot, error) {
	<-ctx.Done()
	close(s.canceled)
	return ProjectionSnapshot{}, ctx.Err()
}

func TestProjectorSnapshotLoadTimeoutFallsBackToColdReplay(t *testing.T) {
	_, connection := testutil.StartNATS(t)
	js, err := jetstream.New(connection)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	stream, err := js.CreateStream(ctx, jetstream.StreamConfig{
		Name:     "EVENTS_TIMEOUT_TEST",
		Subjects: []string{"evt.timeout.>"},
		Storage:  jetstream.FileStorage,
	})
	if err != nil {
		t.Fatal(err)
	}
	eventLog := NewEncodedEventLog(js, stream, log.New(io.Discard))
	if _, err := eventLog.Append(ctx, "evt.timeout.aggregate.created", EncodedRecord{ID: "timeout-event", Data: []byte("event")}); err != nil {
		t.Fatal(err)
	}

	projection := &timeoutTestProjection{}
	projector := NewDecodedProjector(
		js,
		stream,
		projection,
		func(data []byte) (DecodedEvent[string], error) {
			return DecodedEvent[string]{Event: string(data), ID: string(data)}, nil
		},
		log.New(io.Discard),
	)
	source := &timeoutSnapshotSource{canceled: make(chan struct{})}
	if err := projector.ConfigureSnapshots(
		"timeout",
		source,
		func(*jetstream.StreamInfo) (string, error) { return "timeout-stream", nil },
	); err != nil {
		t.Fatal(err)
	}
	projector.snapshotLoadTimeout = 20 * time.Millisecond

	runCtx, stop := context.WithCancel(context.Background())
	t.Cleanup(stop)
	go func() { _ = projector.Run(runCtx) }()
	deadline := time.Now().Add(2 * time.Second)
	for !projector.Status().StartupComplete && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if projection.count.Load() != 1 || projector.Status().SnapshotRestored {
		t.Fatalf("timeout fallback projection count/status = %d/%#v", projection.count.Load(), projector.Status())
	}
	select {
	case <-source.canceled:
	default:
		t.Fatal("snapshot source was not canceled at the load deadline")
	}
}

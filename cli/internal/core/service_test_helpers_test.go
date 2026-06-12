package core

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/testutil"
)

type testEventHarness struct {
	js        jetstream.JetStream
	stream    jetstream.Stream
	publisher *events.Publisher
}

func newTestEventHarness(t *testing.T) *testEventHarness {
	t.Helper()
	_, nc := testutil.StartNATS(t)
	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("jetstream.New: %v", err)
	}
	stream, err := js.CreateOrUpdateStream(testContext(t), jetstream.StreamConfig{
		Name:               "EVT",
		Subjects:           []string{"evt.>"},
		Storage:            jetstream.MemoryStorage,
		AllowAtomicPublish: true,
	})
	if err != nil {
		t.Fatalf("CreateOrUpdateStream: %v", err)
	}
	return &testEventHarness{
		js:        js,
		stream:    stream,
		publisher: events.NewPublisher(js, stream, testServiceLogger()),
	}
}

func testEventPublisher(t *testing.T) *events.Publisher {
	t.Helper()
	return newTestEventHarness(t).publisher
}

func testEventProjector(t *testing.T) *events.Projector {
	t.Helper()
	return newTestEventHarness(t).projector(NewRoomTimelineProjection())
}

func (h *testEventHarness) projector(proj events.Projection) *events.Projector {
	return events.NewProjector(h.js, h.stream, proj, testServiceLogger())
}

func startTestProjector(t *testing.T, projector *events.Projector) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- projector.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("projector did not stop within timeout")
		}
	})

	deadline := time.Now().Add(5 * time.Second)
	for !projector.Started() {
		if time.Now().After(deadline) {
			t.Fatal("projector did not start within timeout")
		}
		time.Sleep(time.Millisecond)
	}
}

func testServiceLogger() *log.Logger {
	return log.New(io.Discard)
}

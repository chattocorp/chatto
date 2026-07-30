package events_test

import (
	"context"
	"io"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"

	. "hmans.de/chatto/internal/events"
	"hmans.de/chatto/internal/testutil"
)

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func testLogger() Logger {
	return log.New(io.Discard)
}

func setupTestStream(t *testing.T) (jetstream.JetStream, jetstream.Stream) {
	t.Helper()
	_, connection := testutil.StartNATS(t)
	js, err := jetstream.New(connection)
	if err != nil {
		t.Fatalf("create JetStream context: %v", err)
	}
	stream, err := js.CreateOrUpdateStream(testContext(t), jetstream.StreamConfig{
		Name:               "EVENTS_TEST",
		Subjects:           []string{"evt.>"},
		Storage:            jetstream.FileStorage,
		AllowAtomicPublish: true,
	})
	if err != nil {
		t.Fatalf("create test stream: %v", err)
	}
	return js, stream
}

func waitFor(t *testing.T, timeout time.Duration, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatal("condition did not become true")
		}
		time.Sleep(time.Millisecond)
	}
}

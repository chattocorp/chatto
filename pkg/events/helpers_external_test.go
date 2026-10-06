package events_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

func testContext(t testing.TB) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func testLogger() *slog.Logger {
	return slog.New(slog.DiscardHandler)
}

func startTestNATS(t testing.TB) *nats.Conn {
	t.Helper()
	natsServer, err := server.NewServer(&server.Options{
		JetStream:  true,
		DontListen: true,
		StoreDir:   t.TempDir(),
		NoSigs:     true,
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

func setupTestStream(t testing.TB) (jetstream.JetStream, jetstream.Stream) {
	t.Helper()
	connection := startTestNATS(t)
	js, err := jetstream.New(connection)
	if err != nil {
		t.Fatalf("create JetStream context: %v", err)
	}
	stream, err := js.CreateOrUpdateStream(testContext(t), jetstream.StreamConfig{
		Name:               "EVENTS_TEST",
		Subjects:           []string{"evt.>"},
		Storage:            jetstream.FileStorage,
		AllowMsgTTL:        true,
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

// must returns value or panics with err. Tests use it for constructors that
// must succeed.
func must[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

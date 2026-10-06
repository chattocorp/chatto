package jetstreamutil_test

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

func testContext(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func startTestNATS(t *testing.T) *nats.Conn {
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

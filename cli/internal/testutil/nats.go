package testutil

import (
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// StartNATS starts an embedded JetStream-enabled NATS server for tests and
// connects to it in-process. It intentionally does not open a TCP listener.
func StartNATS(t testing.TB) (*server.Server, *nats.Conn) {
	t.Helper()

	ns, err := server.NewServer(&server.Options{
		JetStream:  true,
		DontListen: true,
		StoreDir:   t.TempDir(),
		NoSigs:     true,
	})
	if err != nil {
		t.Fatalf("Failed to create NATS server: %v", err)
	}

	ns.Start()
	if !ns.ReadyForConnections(5 * time.Second) {
		t.Fatal("NATS server not ready")
	}

	nc, err := nats.Connect(nats.DefaultURL, nats.InProcessServer(ns))
	if err != nil {
		t.Fatalf("Failed to connect to NATS: %v", err)
	}

	t.Cleanup(func() {
		nc.Close()
		ns.Shutdown()
		ns.WaitForShutdown()
	})

	return ns, nc
}

package events

import (
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

type discardLogger struct{}

func (discardLogger) Debug(interface{}, ...interface{}) {}
func (discardLogger) Info(interface{}, ...interface{})  {}
func (discardLogger) Warn(interface{}, ...interface{})  {}
func (discardLogger) Error(interface{}, ...interface{}) {}

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

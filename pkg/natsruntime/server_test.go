package natsruntime_test

import (
	"net"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/pkg/natsruntime"
)

func TestServerPersistsJetStreamAcrossRestart(t *testing.T) {
	storeDir := t.TempDir()
	config := privateJetStreamConfig(storeDir)

	first, err := natsruntime.Start(config)
	if err != nil {
		t.Fatalf("start first server: %v", err)
	}
	connection, err := nats.Connect(nats.DefaultURL, first.InProcessOption())
	if err != nil {
		first.Shutdown()
		t.Fatalf("connect to first server: %v", err)
	}
	js, err := jetstream.New(connection)
	if err != nil {
		connection.Close()
		first.Shutdown()
		t.Fatalf("create first JetStream client: %v", err)
	}
	stream, err := js.CreateStream(t.Context(), jetstream.StreamConfig{
		Name:     "TEST",
		Subjects: []string{"test.>"},
		Storage:  jetstream.FileStorage,
	})
	if err != nil {
		connection.Close()
		first.Shutdown()
		t.Fatalf("create stream: %v", err)
	}
	if _, err := js.Publish(t.Context(), "test.created", []byte("created")); err != nil {
		connection.Close()
		first.Shutdown()
		t.Fatalf("publish event: %v", err)
	}
	info, err := stream.Info(t.Context())
	if err != nil {
		connection.Close()
		first.Shutdown()
		t.Fatalf("read first stream info: %v", err)
	}
	if info.State.Msgs != 1 {
		connection.Close()
		first.Shutdown()
		t.Fatalf("first stream messages = %d, want 1", info.State.Msgs)
	}
	connection.Close()
	first.Shutdown()

	restarted, err := natsruntime.Start(config)
	if err != nil {
		t.Fatalf("restart server: %v", err)
	}
	t.Cleanup(restarted.Shutdown)
	restartedConnection, err := nats.Connect(nats.DefaultURL, restarted.InProcessOption())
	if err != nil {
		t.Fatalf("connect to restarted server: %v", err)
	}
	t.Cleanup(restartedConnection.Close)
	restartedJS, err := jetstream.New(restartedConnection)
	if err != nil {
		t.Fatalf("create restarted JetStream client: %v", err)
	}
	restartedStream, err := restartedJS.Stream(t.Context(), "TEST")
	if err != nil {
		t.Fatalf("open restarted stream: %v", err)
	}
	restartedInfo, err := restartedStream.Info(t.Context())
	if err != nil {
		t.Fatalf("read restarted stream info: %v", err)
	}
	if restartedInfo.State.Msgs != 1 {
		t.Fatalf("restarted stream messages = %d, want 1", restartedInfo.State.Msgs)
	}
}

func TestServerShutdownIsIdempotent(t *testing.T) {
	runtime, err := natsruntime.Start(privateJetStreamConfig(t.TempDir()))
	if err != nil {
		t.Fatalf("start server: %v", err)
	}

	runtime.Shutdown()
	runtime.Shutdown()
}

func TestStartClonesNestedOptions(t *testing.T) {
	allowedConnectionTypes := map[string]struct{}{"standard": {}}
	config := privateJetStreamConfig(t.TempDir())
	config.Options.Users = []*server.User{{
		Username:               "client",
		Password:               "password",
		AllowedConnectionTypes: allowedConnectionTypes,
	}}

	runtime, err := natsruntime.Start(config)
	if err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(runtime.Shutdown)

	if _, ok := allowedConnectionTypes["standard"]; !ok {
		t.Fatal("Start mutated the caller's nested user options")
	}
	if _, ok := allowedConnectionTypes["STANDARD"]; ok {
		t.Fatal("Start exposed nats-server normalization through caller-owned options")
	}
}

func TestStartRejectsNonPositiveReadyTimeout(t *testing.T) {
	_, err := natsruntime.Start(natsruntime.Config{
		Options: server.Options{
			JetStream:  true,
			StoreDir:   t.TempDir(),
			DontListen: true,
			NoLog:      true,
		},
	})
	if err == nil || !strings.Contains(err.Error(), "timeout must be positive") {
		t.Fatalf("start error = %v, want readiness timeout error", err)
	}
}

func TestStartCleansUpFailedServer(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve client port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port

	config := privateJetStreamConfig(t.TempDir())
	config.Options.DontListen = false
	config.Options.Host = "127.0.0.1"
	config.Options.Port = port
	config.ReadyTimeout = 100 * time.Millisecond
	_, err = natsruntime.Start(config)
	if err == nil {
		listener.Close()
		t.Fatal("start with occupied client port succeeded")
	}
	if err := listener.Close(); err != nil {
		t.Fatalf("release client port: %v", err)
	}

	restarted, err := natsruntime.Start(config)
	if err != nil {
		t.Fatalf("start after failed server cleanup: %v", err)
	}
	restarted.Shutdown()
}

func privateJetStreamConfig(storeDir string) natsruntime.Config {
	return natsruntime.Config{
		Options: server.Options{
			JetStream:  true,
			StoreDir:   storeDir,
			DontListen: true,
			NoLog:      true,
		},
		ReadyTimeout: 5 * time.Second,
	}
}

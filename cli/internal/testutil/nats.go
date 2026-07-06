package testutil

import (
	"context"
	"errors"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

var (
	sharedNATSMu     sync.Mutex
	sharedNATSOnce   sync.Once
	sharedNATSServer *server.Server
	sharedNATSStore  string
	sharedNATSErr    error
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

// StartSharedNATS returns a fresh in-process connection to a package-shared
// embedded NATS server. It removes Chatto's known JetStream resources before
// returning so callers can create a fresh ChattoCore without paying per-test
// server startup cost.
//
// This helper assumes package tests are not marked t.Parallel. It is intended
// for integration-style test helpers that already serialize access through Go's
// default test runner.
func StartSharedNATS(t testing.TB) (*server.Server, *nats.Conn) {
	t.Helper()

	ns := sharedNATS(t)
	nc, err := nats.Connect(nats.DefaultURL, nats.InProcessServer(ns))
	if err != nil {
		t.Fatalf("Failed to connect to shared NATS: %v", err)
	}
	t.Cleanup(nc.Close)

	ResetChattoJetStream(t, nc)

	return ns, nc
}

// ShutdownSharedNATS stops the package-shared embedded server and removes its
// JetStream store directory. Packages that use StartSharedNATS should call this
// from TestMain after m.Run so the store survives for the whole package test
// process but does not leak under the system temp directory.
func ShutdownSharedNATS() {
	sharedNATSMu.Lock()
	defer sharedNATSMu.Unlock()

	if sharedNATSServer != nil {
		sharedNATSServer.Shutdown()
		sharedNATSServer.WaitForShutdown()
		sharedNATSServer = nil
	}
	if sharedNATSStore != "" {
		_ = os.RemoveAll(sharedNATSStore)
		sharedNATSStore = ""
	}
	sharedNATSErr = nil
	sharedNATSOnce = sync.Once{}
}

// ResetChattoJetStream deletes the durable resources Chatto test cores create.
// It deliberately targets current Chatto resource names rather than deleting
// every stream in the account, so ad-hoc test streams remain opt-in.
func ResetChattoJetStream(t testing.TB, nc *nats.Conn) {
	t.Helper()

	sharedNATSMu.Lock()
	defer sharedNATSMu.Unlock()

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("create JetStream context: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	for _, bucket := range []string{
		"ENCRYPTION_KEYS",
		"RUNTIME_STATE",
		"MEMORY_CACHE",
	} {
		if err := js.DeleteKeyValue(ctx, bucket); err != nil && !errors.Is(err, jetstream.ErrBucketNotFound) {
			t.Fatalf("delete KV bucket %s: %v", bucket, err)
		}
	}

	for _, bucket := range []string{
		"ASSET_CACHE",
		"SERVER_ASSETS",
	} {
		if err := js.DeleteObjectStore(ctx, bucket); err != nil && !errors.Is(err, jetstream.ErrBucketNotFound) {
			t.Fatalf("delete object store %s: %v", bucket, err)
		}
	}

	for _, stream := range []string{"EVT"} {
		if err := js.DeleteStream(ctx, stream); err != nil && !errors.Is(err, jetstream.ErrStreamNotFound) {
			t.Fatalf("delete stream %s: %v", stream, err)
		}
	}

	if err := nc.FlushTimeout(2 * time.Second); err != nil {
		t.Fatalf("flush after JetStream reset: %v", err)
	}
}

// WaitForValue reads from ch until match accepts a value or timeout expires.
func WaitForValue[T any](t testing.TB, ch <-chan T, timeout time.Duration, description string, match func(T) bool) T {
	t.Helper()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	var zero T
	for {
		select {
		case value, ok := <-ch:
			if !ok {
				t.Fatalf("channel closed while waiting for %s", description)
			}
			if match(value) {
				return value
			}
		case <-timer.C:
			t.Fatalf("timeout waiting for %s", description)
			return zero
		}
	}
}

func sharedNATS(t testing.TB) *server.Server {
	t.Helper()

	sharedNATSOnce.Do(func() {
		storeDir, err := os.MkdirTemp("", "chatto-shared-nats-*")
		if err != nil {
			sharedNATSErr = err
			return
		}
		sharedNATSStore = storeDir

		sharedNATSServer, sharedNATSErr = server.NewServer(&server.Options{
			JetStream:  true,
			DontListen: true,
			StoreDir:   storeDir,
			NoSigs:     true,
		})
		if sharedNATSErr != nil {
			return
		}

		sharedNATSServer.Start()
		if !sharedNATSServer.ReadyForConnections(5 * time.Second) {
			sharedNATSErr = errors.New("shared NATS server not ready")
		}
	})

	if sharedNATSErr != nil {
		t.Fatalf("Failed to create shared NATS server: %v", sharedNATSErr)
	}

	return sharedNATSServer
}

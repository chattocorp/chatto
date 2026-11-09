package core

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/config"
	"github.com/chattocorp/chatto/cli/internal/runtimeunit"
	"github.com/chattocorp/chatto/cli/internal/testutil"
	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go/jetstream"
)

func TestChattoCoreRecoversAfterExternalNATSRestart(t *testing.T) {
	storeDir := testutil.StoreDir(t)
	ns := startRecoveryTestNATS(t, storeDir, -1)
	port := ns.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithCancel(context.Background())
	cfg := config.ChattoConfig{}
	cfg.NATS.Client.URL = fmt.Sprintf("nats://127.0.0.1:%d", port)
	nc, err := runtimeunit.ConnectToNATS(ctx, cfg, nil)
	if err != nil {
		cancel()
		stopRecoveryTestNATS(ns)
		t.Fatalf("ConnectToNATS: %v", err)
	}

	chattoCore, err := NewChattoCore(ctx, nc, config.CoreConfig{
		SecretKey: "recovery-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "recovery-signing-secret"},
	})
	if err != nil {
		cancel()
		nc.Close()
		stopRecoveryTestNATS(ns)
		t.Fatalf("NewChattoCore: %v", err)
	}
	coreDone := make(chan error, 1)
	go func() { coreDone <- chattoCore.Run(ctx) }()
	coreStopped := false

	t.Cleanup(func() {
		cancel()
		if !coreStopped {
			select {
			case <-coreDone:
			case <-time.After(5 * time.Second):
				t.Error("core.Run did not stop")
			}
		}
		nc.Close()
		stopRecoveryTestNATS(ns)
	})

	bootCtx, bootCancel := context.WithTimeout(ctx, 10*time.Second)
	defer bootCancel()
	if err := chattoCore.WaitForBoot(bootCtx); err != nil {
		t.Fatalf("WaitForBoot: %v", err)
	}
	if err := chattoCore.Ready(bootCtx); err != nil {
		t.Fatalf("Ready before restart: %v", err)
	}

	viewer, err := chattoCore.CreateUser(bootCtx, SystemActorID, "recovery-viewer", "Recovery Viewer", "password123")
	if err != nil {
		t.Fatalf("CreateUser before restart: %v", err)
	}
	if err := chattoCore.SetPresence(bootCtx, viewer.Id, PresenceStatusOnline); err != nil {
		t.Fatalf("SetPresence before restart: %v", err)
	}
	waitForRecoveryTest(t, 5*time.Second, func() bool {
		statuses, err := chattoCore.GetUserPresences(bootCtx, []string{viewer.Id})
		return err == nil && statuses[viewer.Id] == PresenceStatusOnline
	}, "presence watcher to observe the pre-restart state")
	streamCtx, stopStream := context.WithCancel(ctx)
	defer stopStream()
	events, err := chattoCore.StreamMyEventsWithOptions(streamCtx, viewer.Id, StreamMyEventsOptions{})
	if err != nil {
		t.Fatalf("StreamMyEventsWithOptions: %v", err)
	}

	stopRecoveryTestNATS(ns)
	ns = nil
	waitForRecoveryTest(t, 5*time.Second, func() bool { return nc.IsReconnecting() }, "NATS client to enter reconnecting state")
	waitForRecoveryTest(t, 5*time.Second, func() bool {
		select {
		case _, ok := <-events:
			return !ok
		default:
			return false
		}
	}, "realtime stream to close after NATS continuity loss")

	readyCtx, readyCancel := context.WithTimeout(ctx, 250*time.Millisecond)
	if err := chattoCore.Ready(readyCtx); err == nil {
		readyCancel()
		t.Fatal("Ready succeeded while NATS was unavailable")
	}
	readyCancel()

	ns = startRecoveryTestNATS(t, storeDir, port)
	waitForRecoveryTest(t, 10*time.Second, nc.IsConnected, "NATS client to reconnect")
	waitForRecoveryTest(t, 10*time.Second, func() bool {
		readyCtx, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		return chattoCore.Ready(readyCtx) == nil
	}, "Chatto core to become ready after projection recovery")
	writeCtx, writeCancel := context.WithTimeout(ctx, 10*time.Second)
	defer writeCancel()
	statuses, err := chattoCore.GetUserPresences(writeCtx, []string{viewer.Id})
	if err != nil {
		t.Fatalf("GetUserPresences after recovery: %v", err)
	}
	if statuses[viewer.Id] != PresenceStatusOffline {
		t.Fatalf("presence after volatile bucket recovery = %q, want %q", statuses[viewer.Id], PresenceStatusOffline)
	}

	recoveredStreamCtx, stopRecoveredStream := context.WithCancel(ctx)
	defer stopRecoveredStream()
	if _, err := chattoCore.StreamMyEventsWithOptions(recoveredStreamCtx, viewer.Id, StreamMyEventsOptions{}); err != nil {
		t.Fatalf("open realtime stream after recovery: %v", err)
	}
	if _, err := chattoCore.CreateUser(writeCtx, SystemActorID, "recovered-user", "Recovered User", "password123"); err != nil {
		t.Fatalf("durable write after recovery: %v", err)
	}
	select {
	case err := <-coreDone:
		t.Fatalf("core stopped during recoverable NATS restart: %v", err)
	default:
	}

	// A permanently closed client cannot recover. Core.Run must terminate so
	// the process supervisor can replace the replica even without a probe.
	nc.Close()
	select {
	case err := <-coreDone:
		coreStopped = true
		if err == nil {
			t.Fatal("core stopped without reporting the permanently closed NATS connection")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("core did not stop after its NATS connection permanently closed")
	}
}

func TestNATSRecoveryLivenessError(t *testing.T) {
	now := time.Now()
	chattoCore := &ChattoCore{}
	if err := chattoCore.NATSRecoveryLivenessError(now); err != nil {
		t.Fatalf("healthy core reported a liveness error: %v", err)
	}
	chattoCore.natsRecoveryStartedAt.Store(now.Add(-natsRecoveryLivenessLimit - time.Second).UnixNano())
	if err := chattoCore.NATSRecoveryLivenessError(now); err == nil {
		t.Fatal("stalled NATS recovery did not report a liveness error")
	}
}

func startRecoveryTestNATS(t *testing.T, storeDir string, port int) *server.Server {
	t.Helper()
	ns, err := server.NewServer(&server.Options{
		JetStream: true,
		StoreDir:  storeDir,
		Host:      "127.0.0.1",
		Port:      port,
		NoSigs:    true,
	})
	if err != nil {
		t.Fatalf("create NATS server: %v", err)
	}
	ns.Start()
	if !ns.ReadyForConnections(5 * time.Second) {
		ns.Shutdown()
		t.Fatal("NATS server did not become ready")
	}
	return ns
}

func stopRecoveryTestNATS(ns *server.Server) {
	if ns == nil {
		return
	}
	ns.Shutdown()
	ns.WaitForShutdown()
}

func waitForRecoveryTest(t *testing.T, timeout time.Duration, condition func() bool, description string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", description)
}

// Construction must survive a disconnected broker before any runtime starts.
func TestStorageStartupRecoversAfterNATSRestart(t *testing.T) {
	dir := testutil.StoreDir(t)
	ns := startRecoveryTestNATS(t, dir, -1)
	port := ns.Addr().(*net.TCPAddr).Port
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	cfg := config.ChattoConfig{}
	cfg.NATS.Client.URL = fmt.Sprintf("nats://127.0.0.1:%d", port)
	nc, err := runtimeunit.ConnectToNATS(ctx, cfg, nil)
	if err != nil {
		stopRecoveryTestNATS(ns)
		t.Fatal(err)
	}
	defer nc.Close()
	js, err := jetstream.New(nc, jetstream.WithDefaultTimeout(100*time.Millisecond))
	if err != nil {
		stopRecoveryTestNATS(ns)
		t.Fatal(err)
	}
	stopRecoveryTestNATS(ns)
	done := make(chan error, 1)
	go func() { _, err := newStorage(js, ctx, config.CoreConfig{}); done <- err }()
	// Keep provisioning pending while the broker is unavailable.
	select {
	case err := <-done:
		t.Fatalf("startup returned during outage: %v", err)
	case <-time.After(time.Second):
	}
	ns = startRecoveryTestNATS(t, dir, port)
	defer stopRecoveryTestNATS(ns)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("startup did not recover")
	}
}

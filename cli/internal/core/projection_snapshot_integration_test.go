package core

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
)

func TestExperimentalProjectionSnapshotsPersistAndRestoreThreads(t *testing.T) {
	storeDir := t.TempDir()
	ns, nc := startPersistentSnapshotNATS(t, storeDir)
	t.Cleanup(func() { stopPersistentSnapshotNATS(ns, nc) })
	ctx := testContext(t)
	cfg := config.CoreConfig{
		SecretKey: "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
		Assets: config.AssetsConfig{
			SigningSecret:  "test-signing-secret",
			StorageBackend: config.StorageBackendNATS,
		},
		Experimental: config.ExperimentalConfig{ProjectionSnapshots: true},
		Version:      "snapshot-integration-test",
	}

	first, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	created := threadCreatedEvent("THREAD-CREATED", "R1", "ROOT", "U1", 1)
	reply := postedEvent(postedOpts{envelopeID: "REPLY-1", eventID: "REPLY-1", roomID: "R1", actorID: "U2", inThread: "ROOT", at: 2})
	for _, event := range []*corev1.Event{created, reply} {
		if _, err := first.EventPublisher.AppendEventually(ctx, events.RoomAggregate("R1").SubjectFor(event), event); err != nil {
			t.Fatal(err)
		}
	}
	stopFirst := startSnapshotTestCore(t, first)
	waitForSnapshotObjects(t, ctx, first, 2)
	stopFirst()
	stopPersistentSnapshotNATS(ns, nc)

	// A persisted StreamInfo.Created timestamp changes when embedded NATS is
	// reconstructed. Restart the process against the same store so this test
	// proves snapshot identity is derived from EVT history instead.
	ns, nc = startPersistentSnapshotNATS(t, storeDir)

	second, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	stopSecond := startSnapshotTestCore(t, second)
	defer stopSecond()
	status := second.ThreadsProjector.Status()
	if !status.SnapshotRestored || status.SnapshotCutoffSeq == 0 || status.SnapshotGenerationID == "" {
		t.Fatalf("Thread projector did not restore snapshot: %#v", status)
	}
	if got := threadEventIDs(second.Threads.ThreadEvents("ROOT")); !slices.Equal(got, []string{"REPLY-1"}) {
		t.Fatalf("restored Thread events = %v", got)
	}
}

func startPersistentSnapshotNATS(t *testing.T, storeDir string) (*server.Server, *nats.Conn) {
	t.Helper()
	ns, err := server.NewServer(&server.Options{
		JetStream:  true,
		DontListen: true,
		StoreDir:   storeDir,
		NoSigs:     true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ns.Start()
	if !ns.ReadyForConnections(5 * time.Second) {
		ns.Shutdown()
		t.Fatal("persistent snapshot NATS did not become ready")
	}
	nc, err := nats.Connect(nats.DefaultURL, nats.InProcessServer(ns))
	if err != nil {
		ns.Shutdown()
		t.Fatal(err)
	}
	return ns, nc
}

func stopPersistentSnapshotNATS(ns *server.Server, nc *nats.Conn) {
	if nc != nil {
		nc.Close()
	}
	if ns != nil {
		ns.Shutdown()
		ns.WaitForShutdown()
	}
}

func TestExperimentalProjectionSnapshotsAreDisabledByDefault(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	core, err := NewChattoCore(testContext(t), nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if core.projectionSnapshotWorker != nil {
		t.Fatal("snapshot worker enabled without experimental flag")
	}
}

func startSnapshotTestCore(t *testing.T, core *ChattoCore) func() {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- core.Run(ctx) }()
	bootCtx, bootCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer bootCancel()
	if err := core.WaitForBoot(bootCtx); err != nil {
		cancel()
		t.Fatal(err)
	}
	return func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("core did not stop")
		}
	}
}

func waitForSnapshotObjects(t *testing.T, ctx context.Context, core *ChattoCore, minimum int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		objects, err := core.storage.serverAssets.List(ctx)
		if err == nil {
			count := 0
			for _, object := range objects {
				if strings.HasPrefix(object.Name, "internal/projection-snapshots/v1/") {
					count++
				}
			}
			if count >= minimum {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("snapshot objects were not published")
}

package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"hmans.de/chatto/internal/config"
	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	"hmans.de/chatto/internal/testutil"
	"hmans.de/chatto/internal/testutil/fakes3"
)

func TestProjectionSnapshotsPersistAndRestoreCohort(t *testing.T) {
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
		ProjectionSnapshots: true,
		Version:             "snapshot-integration-test",
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
	select {
	case <-first.projectionSnapshotWorker.done:
	case <-time.After(5 * time.Second):
		t.Fatal("snapshot worker did not publish the projection cohort")
	}
	firstSnapshotObjects := projectionSnapshotObjectNames(t, ctx, first)
	if len(firstSnapshotObjects) < 2 {
		t.Fatalf("snapshot worker published only %d objects", len(firstSnapshotObjects))
	}
	for _, name := range firstSnapshotObjects {
		if _, err := first.storage.serverAssets.GetInfo(ctx, name); !errors.Is(err, jetstream.ErrObjectNotFound) {
			t.Fatalf("snapshot object %q leaked into shared SERVER_ASSETS: %v", name, err)
		}
	}
	firstIdentity, err := events.StreamIdentity(first.storage.serverEvtStream)
	if err != nil {
		t.Fatal(err)
	}
	stopFirst()
	stopPersistentSnapshotNATS(ns, nc)

	// A persisted StreamInfo.Created timestamp changes when embedded NATS is
	// reconstructed. Restart the process against the same store so this test
	// proves the durable EVT metadata identity remains stable instead.
	ns, nc = startPersistentSnapshotNATS(t, storeDir)

	second, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	stopSecond := startSnapshotTestCore(t, second)
	t.Cleanup(stopSecond)
	secondIdentity, err := events.StreamIdentity(second.storage.serverEvtStream)
	if err != nil {
		t.Fatal(err)
	}
	if secondIdentity != firstIdentity {
		t.Fatalf("EVT identity changed across process restart: %q != %q", secondIdentity, firstIdentity)
	}
	status := second.ThreadsProjector.Status()
	if !status.SnapshotRestored || status.SnapshotCutoffSeq == 0 || status.SnapshotGenerationID == "" {
		t.Fatalf("Thread projector did not restore snapshot: %#v", status)
	}
	if status.StartupMessages != 0 {
		t.Fatalf("Thread projector replayed %d messages after current snapshot restore", status.StartupMessages)
	}
	for _, registration := range second.projections {
		if !registration.snapshotEnabled {
			continue
		}
		status := registration.projector.Status()
		if status.StartupTargetSeq == 0 {
			// Empty projections have no state or replay tail to accelerate, and
			// therefore do not publish or restore a zero-cutoff generation.
			continue
		}
		if !status.SnapshotRestored || status.SnapshotCutoffSeq == 0 {
			t.Errorf("%s projector did not restore its snapshot: %#v", registration.key, status)
		}
		if status.StartupMessages != 0 {
			t.Errorf("%s projector replayed %d messages after current snapshot restore", registration.key, status.StartupMessages)
		}
	}
	if got := threadEventIDs(second.Threads.ThreadEvents("ROOT")); !slices.Equal(got, []string{"REPLY-1"}) {
		t.Fatalf("restored Thread events = %v", got)
	}
	select {
	case <-second.projectionSnapshotWorker.done:
	case <-time.After(5 * time.Second):
		t.Fatal("snapshot worker did not finish its initial eligibility pass")
	}
	stopSecond()
	refreshedObjects := projectionSnapshotObjectNames(t, ctx, second)
	if len(refreshedObjects) != len(firstSnapshotObjects) {
		t.Fatalf("fresh restore changed generation count from %d to %d", len(firstSnapshotObjects), len(refreshedObjects))
	}
	for _, previous := range firstSnapshotObjects {
		if !slices.Contains(refreshedObjects, previous) {
			t.Fatalf("fresh restore replaced generation %q", previous)
		}
	}
}

func TestRestoredProjectionWithReplayDeltaPublishesAfterBoot(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	ctx := testContext(t)
	cfg := config.CoreConfig{
		SecretKey:           "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
		Assets:              config.AssetsConfig{SigningSecret: "test-signing-secret", StorageBackend: config.StorageBackendNATS},
		ProjectionSnapshots: true,
		Version:             "snapshot-delta-refresh-test",
	}

	first, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []*corev1.Event{
		threadCreatedEvent("THREAD-CREATED", "R1", "ROOT", "U1", 1),
		postedEvent(postedOpts{envelopeID: "REPLY-1", eventID: "REPLY-1", roomID: "R1", actorID: "U2", inThread: "ROOT", at: 2}),
	} {
		if _, err := first.EventPublisher.AppendEventually(ctx, events.RoomAggregate("R1").SubjectFor(event), event); err != nil {
			t.Fatal(err)
		}
	}
	stopFirst := startSnapshotTestCore(t, first)
	select {
	case <-first.projectionSnapshotWorker.done:
	case <-time.After(5 * time.Second):
		t.Fatal("initial snapshot worker did not finish")
	}
	firstStatus := first.ThreadsProjector.Status()
	firstThreadObjects := snapshotObjectsForProjection(projectionSnapshotObjectNames(t, ctx, first), "threads")
	if len(firstThreadObjects) != 1 || firstStatus.LatestSnapshotAt.IsZero() {
		t.Fatalf("initial Threads snapshot state = %#v, objects=%v", firstStatus, firstThreadObjects)
	}
	stopFirst()

	delta := postedEvent(postedOpts{envelopeID: "REPLY-2", eventID: "REPLY-2", roomID: "R1", actorID: "U3", inThread: "ROOT", at: 3})
	if _, err := first.EventPublisher.AppendEventually(ctx, events.RoomAggregate("R1").SubjectFor(delta), delta); err != nil {
		t.Fatal(err)
	}

	second, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	stopSecond := startSnapshotTestCore(t, second)
	t.Cleanup(stopSecond)
	select {
	case <-second.projectionSnapshotWorker.done:
	case <-time.After(5 * time.Second):
		t.Fatal("delta snapshot worker did not finish")
	}
	secondStatus := second.ThreadsProjector.Status()
	if !secondStatus.SnapshotRestored || secondStatus.StartupMessages == 0 {
		t.Fatalf("Threads did not restore and replay its delta: %#v", secondStatus)
	}
	if secondStatus.LatestSnapshotSeq != secondStatus.LastSeq || !secondStatus.LatestSnapshotAt.After(firstStatus.LatestSnapshotAt) {
		t.Fatalf("Threads did not publish its replayed delta: before=%#v after=%#v", firstStatus, secondStatus)
	}
	secondThreadObjects := snapshotObjectsForProjection(projectionSnapshotObjectNames(t, ctx, second), "threads")
	if len(secondThreadObjects) != 2 {
		t.Fatalf("Threads generations after delta publication = %v, want current and previous", secondThreadObjects)
	}
}

func snapshotObjectsForProjection(objects []string, projection string) []string {
	return slices.DeleteFunc(slices.Clone(objects), func(object string) bool {
		return !strings.Contains(object, "/"+projection+"/")
	})
}

func TestMissingProjectionSnapshotColdReplaysOnlyItsOwner(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	ctx := testContext(t)
	cfg := config.CoreConfig{
		SecretKey:           "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
		Assets:              config.AssetsConfig{SigningSecret: "test-signing-secret", StorageBackend: config.StorageBackendNATS},
		ProjectionSnapshots: true,
		Version:             "snapshot-independent-replay-test",
	}

	first, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range []*corev1.Event{
		threadCreatedEvent("THREAD-CREATED", "R1", "ROOT", "U1", 1),
		postedEvent(postedOpts{envelopeID: "REPLY-1", eventID: "REPLY-1", roomID: "R1", actorID: "U2", inThread: "ROOT", at: 2}),
	} {
		if _, err := first.EventPublisher.AppendEventually(ctx, events.RoomAggregate("R1").SubjectFor(event), event); err != nil {
			t.Fatal(err)
		}
	}
	stopFirst := startSnapshotTestCore(t, first)
	select {
	case <-first.projectionSnapshotWorker.done:
	case <-time.After(5 * time.Second):
		t.Fatal("snapshot worker did not publish initial generations")
	}
	store := projectionSnapshotObjectStore(t, ctx, first)
	deleted := 0
	for _, name := range projectionSnapshotObjectNames(t, ctx, first) {
		if !strings.Contains(name, "/threads/") {
			continue
		}
		if err := store.Delete(ctx, name); err != nil {
			t.Fatal(err)
		}
		deleted++
	}
	if deleted == 0 {
		t.Fatal("initial worker did not publish a Threads snapshot")
	}
	stopFirst()

	second, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	stopSecond := startSnapshotTestCore(t, second)
	t.Cleanup(stopSecond)
	threadsStatus := second.ThreadsProjector.Status()
	if threadsStatus.SnapshotRestored || threadsStatus.StartupMessages == 0 {
		t.Fatalf("Threads did not cold replay after its snapshot was removed: %#v", threadsStatus)
	}
	roomDirectoryStatus := second.RoomDirectoryProjector.Status()
	if !roomDirectoryStatus.SnapshotRestored || roomDirectoryStatus.StartupMessages != 0 {
		t.Fatalf("Room Directory did not restore independently: %#v", roomDirectoryStatus)
	}
}

func TestUserProfileSnapshotRestoresWhileAuthenticationColdReplays(t *testing.T) {
	storeDir := t.TempDir()
	ns, nc := startPersistentSnapshotNATS(t, storeDir)
	t.Cleanup(func() { stopPersistentSnapshotNATS(ns, nc) })
	ctx := testContext(t)
	cfg := config.CoreConfig{
		SecretKey:           "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
		Assets:              config.AssetsConfig{SigningSecret: "test-signing-secret", StorageBackend: config.StorageBackendNATS},
		ProjectionSnapshots: true,
		Version:             "user-snapshot-integration-test",
	}

	first, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	stopFirst := startSnapshotTestCore(t, first)
	select {
	case <-first.projectionSnapshotWorker.done:
	case <-time.After(5 * time.Second):
		t.Fatal("initial snapshot worker did not finish")
	}
	user, err := first.CreateUser(ctx, SystemActorID, "snapshot-user", "Snapshot User", "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	acquired, err := first.projectionSnapshotWorker.lease.TryRun(ctx, func(leaderCtx context.Context) error {
		return first.projectionSnapshotWorker.generate(leaderCtx, true)
	})
	if err != nil {
		t.Fatalf("generate updated user snapshot: %v", err)
	}
	if !acquired {
		t.Fatal("updated user snapshot pass did not acquire the lease")
	}
	stopFirst()
	stopPersistentSnapshotNATS(ns, nc)

	ns, nc = startPersistentSnapshotNATS(t, storeDir)
	second, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	stopSecond := startSnapshotTestCore(t, second)
	t.Cleanup(stopSecond)

	profileStatus := second.UsersProjector.Status()
	if !profileStatus.SnapshotRestored || profileStatus.SnapshotCutoffSeq == 0 {
		t.Fatalf("user profile projector did not restore snapshot: %#v", profileStatus)
	}
	authStatus := second.UserAuthProjector.Status()
	if authStatus.SnapshotRestored {
		t.Fatalf("user auth projector unexpectedly restored a snapshot: %#v", authStatus)
	}
	if authStatus.StartupMessages < 2 {
		t.Fatalf("user auth projector replayed %d messages, want account and password events", authStatus.StartupMessages)
	}
	restored, err := second.GetUser(ctx, user.GetId())
	if err != nil {
		t.Fatal(err)
	}
	if restored.GetLogin() != "snapshot-user" || restored.GetDisplayName() != "Snapshot User" {
		t.Fatalf("restored user = %#v", restored)
	}
	if err := second.VerifyUserPassword(ctx, user.GetId(), "correct horse battery staple"); err != nil {
		t.Fatalf("cold-replayed password credential did not verify: %v", err)
	}
}

func TestProjectionSnapshotsRejectRecreatedEVTHistory(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	ctx := testContext(t)
	cfg := config.CoreConfig{
		SecretKey: "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
		Assets: config.AssetsConfig{
			SigningSecret:  "test-signing-secret",
			StorageBackend: config.StorageBackendNATS,
		},
		ProjectionSnapshots: true,
		Version:             "snapshot-integration-test",
	}

	first, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	eventsToPublish := []*corev1.Event{
		threadCreatedEvent("THREAD-CREATED", "R1", "ROOT", "U1", 1),
		postedEvent(postedOpts{envelopeID: "REPLY-1", eventID: "REPLY-1", roomID: "R1", actorID: "U2", inThread: "ROOT", at: 2}),
	}
	for _, event := range eventsToPublish {
		if _, err := first.EventPublisher.AppendEventually(ctx, events.RoomAggregate("R1").SubjectFor(event), event); err != nil {
			t.Fatal(err)
		}
	}
	stopFirst := startSnapshotTestCore(t, first)
	waitForSnapshotObjects(t, ctx, first, 1)
	firstIdentity, err := events.StreamIdentity(first.storage.serverEvtStream)
	if err != nil {
		t.Fatal(err)
	}
	stopFirst()

	if err := first.js.DeleteStream(ctx, "EVT"); err != nil {
		t.Fatal(err)
	}
	recreated, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	recreatedIdentity, err := events.StreamIdentity(recreated.storage.serverEvtStream)
	if err != nil {
		t.Fatal(err)
	}
	if recreatedIdentity == firstIdentity {
		t.Fatal("recreated EVT stream reused its prior identity")
	}
	eventsToPublish[0] = threadCreatedEvent("THREAD-CREATED-DIFFERENT", "R1", "ROOT", "U9", 1)
	for _, event := range eventsToPublish {
		if _, err := recreated.EventPublisher.AppendEventually(ctx, events.RoomAggregate("R1").SubjectFor(event), event); err != nil {
			t.Fatal(err)
		}
	}
	stopRecreated := startSnapshotTestCore(t, recreated)
	defer stopRecreated()
	status := recreated.ThreadsProjector.Status()
	if status.SnapshotRestored {
		t.Fatalf("Thread projector restored snapshot from deleted EVT history: %#v", status)
	}
}

func TestConcurrentCoreInitializationConvergesOnEVTIdentity(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	ctx := testContext(t)
	cfg := config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	}
	type result struct {
		core *ChattoCore
		err  error
	}
	start := make(chan struct{})
	results := make(chan result, 2)
	for range 2 {
		go func() {
			<-start
			core, err := NewChattoCore(ctx, nc, cfg)
			results <- result{core: core, err: err}
		}()
	}
	close(start)

	cores := make([]*ChattoCore, 0, 2)
	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		cores = append(cores, result.core)
	}
	identities := make([]string, 0, len(cores))
	for _, core := range cores {
		identity, err := events.StreamIdentity(core.storage.serverEvtStream)
		if err != nil {
			t.Fatal(err)
		}
		identities = append(identities, identity)
	}
	if identities[0] != identities[1] {
		t.Fatalf("concurrent core identities did not converge: %q != %q", identities[0], identities[1])
	}

	third, err := NewChattoCore(ctx, nc, cfg)
	if err != nil {
		t.Fatal(err)
	}
	thirdIdentity, err := events.StreamIdentity(third.storage.serverEvtStream)
	if err != nil {
		t.Fatal(err)
	}
	if thirdIdentity != identities[0] {
		t.Fatalf("subsequent core changed EVT identity: %q != %q", thirdIdentity, identities[0])
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

func TestProjectionSnapshotsAreDisabledByDefault(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	core, err := NewChattoCore(testContext(t), nc, config.CoreConfig{
		SecretKey: "test-core-secret",
		Assets:    config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if core.projectionSnapshotWorker != nil {
		t.Fatal("snapshot worker enabled without projection snapshot configuration")
	}
}

func TestProjectionSnapshotNATSStoreUsesConfiguredRetention(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	retention := config.Duration(9 * 24 * time.Hour)
	core, err := NewChattoCore(testContext(t), nc, config.CoreConfig{
		SecretKey:           "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
		Assets:              config.AssetsConfig{SigningSecret: "test-signing-secret", StorageBackend: config.StorageBackendNATS},
		ProjectionSnapshots: true, ProjectionSnapshotRetention: retention,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := core.js.ObjectStore(testContext(t), projectionSnapshotObjectStoreName)
	if err != nil {
		t.Fatal(err)
	}
	status, err := store.Status(testContext(t))
	if err != nil {
		t.Fatal(err)
	}
	if status.TTL() != retention.Duration() {
		t.Fatalf("snapshot Object Store TTL = %s, want %s", status.TTL(), retention.Duration())
	}
	if core.projectionSnapshotWorker == nil || core.projectionSnapshotWorker.expirer != nil || core.projectionSnapshotWorker.expiryLease != nil {
		t.Fatal("NATS snapshot worker should use Object Store TTL without application expiry")
	}
}

func TestProjectionSnapshotS3CleanupCanBeDisabledForExternalLifecycle(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(fmt.Sprintf("enabled_%t", enabled), func(t *testing.T) {
			server := fakes3.NewServer(t)
			useSSL := false
			pathStyle := true
			_, nc := testutil.StartNATS(t)
			core, err := NewChattoCore(testContext(t), nc, config.CoreConfig{
				SecretKey:           "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f",
				ProjectionSnapshots: true, ProjectionSnapshotS3Cleanup: &enabled,
				Assets: config.AssetsConfig{
					SigningSecret: "test-signing-secret", StorageBackend: config.StorageBackendS3,
					S3: config.S3Config{Endpoint: server.EndpointHost(), Bucket: "snapshots", AccessKeyID: "key", SecretAccessKey: "secret", UseSSL: &useSSL, PathStyle: &pathStyle},
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if core.projectionSnapshotWorker == nil {
				t.Fatal("snapshot worker is disabled")
			}
			if got := core.projectionSnapshotWorker.expirer != nil; got != enabled {
				t.Fatalf("application S3 expiry enabled = %t, want %t", got, enabled)
			}
			if got := core.projectionSnapshotWorker.expiryLease != nil; got != enabled {
				t.Fatalf("application S3 expiry cooldown enabled = %t, want %t", got, enabled)
			}
		})
	}
}

func TestProjectionSnapshotInitializationFailureDoesNotPreventCoreStartup(t *testing.T) {
	_, nc := testutil.StartNATS(t)
	core, err := NewChattoCore(testContext(t), nc, config.CoreConfig{
		SecretKey:           "not-a-32-byte-hex-secret",
		ProjectionSnapshots: true,
		Assets:              config.AssetsConfig{SigningSecret: "test-signing-secret"},
	})
	if err != nil {
		t.Fatalf("optional snapshot initialization prevented core construction: %v", err)
	}
	if core.projectionSnapshotWorker != nil {
		t.Fatal("snapshot worker enabled after repository initialization failure")
	}
	stop := startSnapshotTestCore(t, core)
	stop()
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
	var stopOnce sync.Once
	return func() {
		stopOnce.Do(func() {
			cancel()
			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("core did not stop")
			}
		})
	}
}

func waitForSnapshotObjects(t *testing.T, ctx context.Context, core *ChattoCore, minimum int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		objects, err := projectionSnapshotObjectStore(t, ctx, core).List(ctx)
		if err == nil {
			count := 0
			for _, object := range objects {
				if strings.HasPrefix(object.Name, "internal/projection-snapshots/") {
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

func projectionSnapshotObjectNames(t *testing.T, ctx context.Context, core *ChattoCore) []string {
	t.Helper()
	objects, err := projectionSnapshotObjectStore(t, ctx, core).List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(objects))
	for _, object := range objects {
		if strings.HasPrefix(object.Name, "internal/projection-snapshots/") {
			names = append(names, object.Name)
		}
	}
	slices.Sort(names)
	return names
}

func projectionSnapshotObjectStore(t *testing.T, ctx context.Context, core *ChattoCore) jetstream.ObjectStore {
	t.Helper()
	store, err := core.js.ObjectStore(ctx, projectionSnapshotObjectStoreName)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

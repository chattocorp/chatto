package core

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
)

func TestReadStateIndexInitialSnapshotAndReplicaUpdates(t *testing.T) {
	core, _ := setupTestCore(t)
	kv := core.storage.runtimeStateKV
	ctx := testContext(t)

	const (
		userID      = "Uindex-user"
		roomID      = "Rindex-room"
		threadRoot  = "Eindex-thread"
		initialRoom = "Einitial-room"
		initialRoot = "Einitial-thread"
		updatedRoom = "Eupdated-room"
	)
	roomKey := roomReadEventKey(userID, roomID)
	threadKey := threadLastOpenedKey(userID, roomID, threadRoot)
	if _, err := kv.Put(ctx, roomKey, []byte(initialRoom)); err != nil {
		t.Fatalf("seed room marker: %v", err)
	}
	if _, err := kv.Put(ctx, threadKey, []byte(initialRoot)); err != nil {
		t.Fatalf("seed thread marker: %v", err)
	}

	indexes := []*ReadStateIndex{
		NewReadStateIndex(kv, testCoreLogger()),
		NewReadStateIndex(kv, testCoreLogger()),
	}
	runCtx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	for _, index := range indexes {
		index := index
		go func() {
			_ = index.Run(runCtx)
		}()
		if err := index.WaitReady(ctx); err != nil {
			t.Fatalf("wait for initial index snapshot: %v", err)
		}
		assertIndexedRoomMarker(t, ctx, index, userID, roomID, initialRoom)
		assertIndexedThreadMarker(t, ctx, index, userID, roomID, threadRoot, initialRoot)
	}

	revision, err := kv.Put(ctx, roomKey, []byte(updatedRoom))
	if err != nil {
		t.Fatalf("update room marker: %v", err)
	}
	for _, index := range indexes {
		if err := index.waitForRevision(ctx, roomKey, revision); err != nil {
			t.Fatalf("wait for replica update: %v", err)
		}
		assertIndexedRoomMarker(t, ctx, index, userID, roomID, updatedRoom)
	}

	snapshot, err := indexes[0].userSnapshot(ctx, userID)
	if err != nil {
		t.Fatalf("get user snapshot: %v", err)
	}
	snapshot.roomMarkers[roomID][0] = 'X'
	assertIndexedRoomMarker(t, ctx, indexes[0], userID, roomID, updatedRoom)

	if err := kv.Delete(ctx, threadKey); err != nil {
		t.Fatalf("delete thread marker: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for _, index := range indexes {
		for {
			_, exists, err := index.threadMarker(ctx, userID, roomID, threadRoot)
			if err != nil {
				t.Fatalf("read deleted thread marker: %v", err)
			}
			if !exists {
				break
			}
			if time.Now().After(deadline) {
				t.Fatal("deleted thread marker remained in index")
			}
			time.Sleep(time.Millisecond)
		}
	}

	revision, err = kv.Create(ctx, threadKey, []byte(updatedRoom))
	if err != nil {
		t.Fatalf("recreate thread marker: %v", err)
	}
	for _, index := range indexes {
		if err := index.waitForRevision(ctx, threadKey, revision); err != nil {
			t.Fatalf("wait for recreated thread marker: %v", err)
		}
		assertIndexedThreadMarker(t, ctx, index, userID, roomID, threadRoot, updatedRoom)
	}
}

func TestReadStateIndexRunReturnsContextCanceledOnShutdown(t *testing.T) {
	core, _ := setupTestCore(t)
	index := NewReadStateIndex(core.storage.runtimeStateKV, testCoreLogger())
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- index.Run(runCtx) }()

	if err := index.WaitReady(testContext(t)); err != nil {
		t.Fatalf("wait for index readiness: %v", err)
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run returned %v, want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not stop within timeout")
	}
}

func TestReadStateIndexDoesNotMissUpdateAtInitialSyncBoundary(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	gatedKV := &initialSyncGatedKV{
		KeyValue:        core.storage.runtimeStateKV,
		atBoundary:      make(chan struct{}),
		releaseBoundary: make(chan struct{}),
	}
	index := NewReadStateIndex(gatedKV, testCoreLogger())
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- index.Run(runCtx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("read state index did not stop within timeout")
		}
	})

	select {
	case <-gatedKV.atBoundary:
	case <-ctx.Done():
		t.Fatalf("wait for initial snapshot boundary: %v", ctx.Err())
	}

	const (
		userID  = "Uinitial-boundary"
		roomID  = "Rinitial-boundary"
		eventID = "Einitial-boundary"
	)
	key := roomReadEventKey(userID, roomID)
	revision, err := gatedKV.Put(ctx, key, []byte(eventID))
	if err != nil {
		t.Fatalf("write at initial snapshot boundary: %v", err)
	}
	close(gatedKV.releaseBoundary)

	if err := index.WaitReady(ctx); err != nil {
		t.Fatalf("wait for index readiness: %v", err)
	}
	if err := index.waitForRevision(ctx, key, revision); err != nil {
		t.Fatalf("wait for boundary update: %v", err)
	}
	assertIndexedRoomMarker(t, ctx, index, userID, roomID, eventID)
}

func TestReadStateReplicaRacesConvergeWithoutRegression(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	room, err := core.CreateRoom(ctx, SystemActorID, KindChannel, "", "read-race", "")
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	poster, err := core.CreateUser(ctx, SystemActorID, "read-race-poster", "Read Race Poster", "password123")
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := core.JoinRoom(ctx, poster.Id, KindChannel, poster.Id, room.Id); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	first, err := core.PostMessage(ctx, KindChannel, room.Id, poster.Id, "first", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage first: %v", err)
	}
	second, err := core.PostMessage(ctx, KindChannel, room.Id, poster.Id, "second", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage second: %v", err)
	}
	third, err := core.PostMessage(ctx, KindChannel, room.Id, poster.Id, "third", nil, "", "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage third: %v", err)
	}
	reply1, err := core.PostMessage(ctx, KindChannel, room.Id, poster.Id, "reply one", nil, first.Id, "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage reply one: %v", err)
	}
	reply2, err := core.PostMessage(ctx, KindChannel, room.Id, poster.Id, "reply two", nil, first.Id, "", nil, false)
	if err != nil {
		t.Fatalf("PostMessage reply two: %v", err)
	}

	replica := newReadStateReplica(t, core)
	for n := 0; n < 10; n++ {
		userID := fmt.Sprintf("Uroom-race-%d", n)
		if err := core.SetLastReadEventID(ctx, KindChannel, userID, room.Id, first.Id); err != nil {
			t.Fatalf("seed room marker %d: %v", n, err)
		}
		roomKey := roomReadEventKey(userID, room.Id)
		seed, err := core.storage.runtimeStateKV.Get(ctx, roomKey)
		if err != nil {
			t.Fatalf("get seeded room marker %d: %v", n, err)
		}
		if err := replica.readStateModel.index.waitForRevision(ctx, roomKey, seed.Revision()); err != nil {
			t.Fatalf("sync replica room marker %d: %v", n, err)
		}

		runConcurrentWrites(t,
			func() error {
				_, err := core.AdvanceLastReadEventID(ctx, KindChannel, userID, room.Id, second.Id)
				return err
			},
			func() error {
				_, err := replica.AdvanceLastReadEventID(ctx, KindChannel, userID, room.Id, third.Id)
				return err
			},
		)
		assertReplicasAtKVRevision(t, ctx, core, replica, roomKey)
		assertIndexedRoomMarker(t, ctx, core.readStateModel.index, userID, room.Id, third.Id)
		assertIndexedRoomMarker(t, ctx, replica.readStateModel.index, userID, room.Id, third.Id)

		threadUserID := fmt.Sprintf("Uthread-race-%d", n)
		runConcurrentWrites(t,
			func() error {
				_, err := core.SetThreadLastReadEventID(ctx, KindChannel, threadUserID, room.Id, first.Id, reply1.Id)
				return err
			},
			func() error {
				_, err := replica.SetThreadLastReadEventID(ctx, KindChannel, threadUserID, room.Id, first.Id, reply2.Id)
				return err
			},
		)
		threadKey := threadLastOpenedKey(threadUserID, room.Id, first.Id)
		assertReplicasAtKVRevision(t, ctx, core, replica, threadKey)
		assertIndexedThreadMarker(t, ctx, core.readStateModel.index, threadUserID, room.Id, first.Id, reply2.Id)
		assertIndexedThreadMarker(t, ctx, replica.readStateModel.index, threadUserID, room.Id, first.Id, reply2.Id)
	}
}

func TestReadStateWritesAreImmediatelyVisibleAndInitializationDoesNotOverwrite(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	replica := newReadStateReplica(t, core)

	const (
		userID       = "Uread-your-writes"
		roomID       = "Rread-your-writes"
		currentEvent = "Ecurrent-marker"
		staleEvent   = "Estale-initializer"
	)
	if err := core.SetLastReadEventID(ctx, KindChannel, userID, roomID, currentEvent); err != nil {
		t.Fatalf("SetLastReadEventID: %v", err)
	}
	got, exists, err := core.PeekLastReadEventID(ctx, userID, roomID)
	if err != nil {
		t.Fatalf("PeekLastReadEventID after write: %v", err)
	}
	if !exists || got != currentEvent {
		t.Fatalf("local room marker = (%q, %v), want (%q, true)", got, exists, currentEvent)
	}

	if err := replica.initializeLastReadEventID(ctx, userID, roomID, staleEvent); err != nil {
		t.Fatalf("initializeLastReadEventID after concurrent write: %v", err)
	}
	got, exists, err = replica.PeekLastReadEventID(ctx, userID, roomID)
	if err != nil {
		t.Fatalf("replica PeekLastReadEventID after initialization: %v", err)
	}
	if !exists || got != currentEvent {
		t.Fatalf("marker after delayed initialization = (%q, %v), want (%q, true)", got, exists, currentEvent)
	}
}

func TestRoomReadMarkerReadsDoNotHitKVPerRoom(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	const (
		userID      = "Uno-kv-reads"
		markerCount = 100
	)
	keys := make([]string, 0, markerCount)
	for n := 0; n < markerCount; n++ {
		roomID := fmt.Sprintf("Rno-kv-read-%d", n)
		key := roomReadEventKey(userID, roomID)
		revision, err := core.storage.runtimeStateKV.Put(ctx, key, []byte("Eindexed"))
		if err != nil {
			t.Fatalf("seed marker %d: %v", n, err)
		}
		if err := core.readStateModel.index.waitForRevision(ctx, key, revision); err != nil {
			t.Fatalf("wait for marker %d: %v", n, err)
		}
		keys = append(keys, roomID)
	}

	countingKV := &countingGetKV{KeyValue: core.storage.runtimeStateKV}
	core.storage.runtimeStateKV = countingKV
	for _, roomID := range keys {
		got, exists, err := core.PeekLastReadEventID(ctx, userID, roomID)
		if err != nil {
			t.Fatalf("read indexed marker %q: %v", roomID, err)
		}
		if !exists || got != "Eindexed" {
			t.Fatalf("indexed marker %q = (%q, %v), want (Eindexed, true)", roomID, got, exists)
		}
	}
	if calls := countingKV.getCalls.Load(); calls != 0 {
		t.Fatalf("room marker reads made %d KV Get calls, want 0", calls)
	}
}

func TestParseReadMarkerKeys(t *testing.T) {
	userID, roomID, ok := parseRoomReadMarkerKey("read.room.U123.R456")
	if !ok || userID != "U123" || roomID != "R456" {
		t.Fatalf("parseRoomReadMarkerKey = (%q, %q, %v)", userID, roomID, ok)
	}
	if _, _, ok := parseRoomReadMarkerKey("read.room.U123"); ok {
		t.Fatal("short room marker key parsed successfully")
	}

	userID, marker, ok := parseThreadReadMarkerKey("read.thread.U123.R456.E789")
	if !ok || userID != "U123" || marker.roomID != "R456" || marker.threadRootEventID != "E789" {
		t.Fatalf("parseThreadReadMarkerKey = (%q, %#v, %v)", userID, marker, ok)
	}
	if _, _, ok := parseThreadReadMarkerKey("read.thread.U123.R456"); ok {
		t.Fatal("short thread marker key parsed successfully")
	}
}

func TestReadStateIndexTracksBoundedRoomMarkerChangesAfterFence(t *testing.T) {
	index := NewReadStateIndex(nil, nil)
	index.readyOnce.Do(func() { close(index.ready) })
	fence, err := index.roomMarkerFence(context.Background())
	if err != nil {
		t.Fatalf("roomMarkerFence: %v", err)
	}
	for revision, key := range []string{
		"read.room.U1.R2",
		"read.room.U2.R3",
		"read.room.U1.R1",
		"read.room.U1.R2",
	} {
		index.apply(benchmarkKVEntry{key: key, value: []byte("Echanged"), revision: uint64(revision + 1)})
	}
	roomIDs, err := index.roomMarkerIDsChangedAfter(context.Background(), "U1", fence)
	if err != nil {
		t.Fatalf("roomMarkerIDsChangedAfter: %v", err)
	}
	if want := []string{"R1", "R2"}; !slices.Equal(roomIDs, want) {
		t.Fatalf("changed room IDs = %v, want %v", roomIDs, want)
	}

	for revision := 0; revision <= roomMarkerChangeLimit; revision++ {
		index.apply(benchmarkKVEntry{
			key:      fmt.Sprintf("read.room.U1.overflow-%d", revision),
			value:    []byte("Eoverflow"),
			revision: uint64(revision + 100),
		})
	}
	if _, err := index.roomMarkerIDsChangedAfter(context.Background(), "U1", fence); err == nil {
		t.Fatal("expired room-marker fence was accepted")
	}
}

func BenchmarkReadStateIndexBuild100000Markers(b *testing.B) {
	const markerCount = 100_000
	entries := make([]jetstream.KeyValueEntry, 0, markerCount)
	for n := 0; n < markerCount; n++ {
		entries = append(entries, benchmarkKVEntry{
			key:      fmt.Sprintf("read.room.U%d.R%d", n/100, n),
			value:    []byte("Ebenchmark123"),
			revision: uint64(n + 1),
		})
	}

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		index := NewReadStateIndex(nil, nil)
		for _, entry := range entries {
			index.apply(entry)
		}
		rooms, threads := index.entryCounts()
		if rooms != markerCount || threads != 0 {
			b.Fatalf("entry counts = (%d, %d), want (%d, 0)", rooms, threads, markerCount)
		}
	}
}

type initialSyncGatedKV struct {
	jetstream.KeyValue
	atBoundary      chan struct{}
	releaseBoundary chan struct{}
}

type countingGetKV struct {
	jetstream.KeyValue
	getCalls atomic.Int64
}

type benchmarkKVEntry struct {
	key      string
	value    []byte
	revision uint64
}

func (entry benchmarkKVEntry) Bucket() string                  { return "RUNTIME_STATE" }
func (entry benchmarkKVEntry) Key() string                     { return entry.key }
func (entry benchmarkKVEntry) Value() []byte                   { return entry.value }
func (entry benchmarkKVEntry) Revision() uint64                { return entry.revision }
func (entry benchmarkKVEntry) Created() time.Time              { return time.Time{} }
func (entry benchmarkKVEntry) Delta() uint64                   { return 0 }
func (entry benchmarkKVEntry) Operation() jetstream.KeyValueOp { return jetstream.KeyValuePut }

func (kv *countingGetKV) Get(ctx context.Context, key string) (jetstream.KeyValueEntry, error) {
	kv.getCalls.Add(1)
	return kv.KeyValue.Get(ctx, key)
}

func (kv *initialSyncGatedKV) WatchFiltered(ctx context.Context, keys []string, opts ...jetstream.WatchOpt) (jetstream.KeyWatcher, error) {
	watcher, err := kv.KeyValue.WatchFiltered(ctx, keys, opts...)
	if err != nil {
		return nil, err
	}
	gated := &initialSyncGatedWatcher{
		source:          watcher,
		updates:         make(chan jetstream.KeyValueEntry),
		atBoundary:      kv.atBoundary,
		releaseBoundary: kv.releaseBoundary,
		ctx:             ctx,
	}
	go gated.forward()
	return gated, nil
}

type initialSyncGatedWatcher struct {
	source          jetstream.KeyWatcher
	updates         chan jetstream.KeyValueEntry
	atBoundary      chan struct{}
	releaseBoundary chan struct{}
	ctx             context.Context
	boundaryOnce    sync.Once
}

func (w *initialSyncGatedWatcher) Updates() <-chan jetstream.KeyValueEntry {
	return w.updates
}

func (w *initialSyncGatedWatcher) Stop() error {
	return w.source.Stop()
}

func (w *initialSyncGatedWatcher) forward() {
	defer close(w.updates)
	for entry := range w.source.Updates() {
		if entry == nil {
			w.boundaryOnce.Do(func() { close(w.atBoundary) })
			select {
			case <-w.releaseBoundary:
			case <-w.ctx.Done():
				return
			}
		}
		select {
		case w.updates <- entry:
		case <-w.ctx.Done():
			return
		}
	}
}

func newReadStateReplica(t *testing.T, source *ChattoCore) *ChattoCore {
	t.Helper()
	index := NewReadStateIndex(source.storage.runtimeStateKV, testCoreLogger())
	replica := &ChattoCore{
		nc:        source.nc,
		logger:    source.logger,
		storage:   source.storage,
		roomModel: source.roomModel,
	}
	replica.readStateModel = &ReadStateModel{core: replica, index: index}

	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- index.Run(runCtx) }()
	if err := index.WaitReady(testContext(t)); err != nil {
		cancel()
		t.Fatalf("wait for replica read state index: %v", err)
	}
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if !errors.Is(err, context.Canceled) {
				t.Errorf("replica read state index stopped with %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Error("replica read state index did not stop within timeout")
		}
	})
	return replica
}

func runConcurrentWrites(t *testing.T, writes ...func() error) {
	t.Helper()
	start := make(chan struct{})
	errs := make(chan error, len(writes))
	for _, write := range writes {
		write := write
		go func() {
			<-start
			errs <- write()
		}()
	}
	close(start)
	for range writes {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent read marker write: %v", err)
		}
	}
}

func assertReplicasAtKVRevision(t *testing.T, ctx context.Context, first, second *ChattoCore, key string) {
	t.Helper()
	entry, err := first.storage.runtimeStateKV.Get(ctx, key)
	if err != nil {
		t.Fatalf("get authoritative marker %q: %v", key, err)
	}
	for _, index := range []*ReadStateIndex{first.readStateModel.index, second.readStateModel.index} {
		if err := index.waitForRevision(ctx, key, entry.Revision()); err != nil {
			t.Fatalf("wait for replica marker %q at revision %d: %v", key, entry.Revision(), err)
		}
	}
}

func assertIndexedRoomMarker(t *testing.T, ctx context.Context, index *ReadStateIndex, userID, roomID, want string) {
	t.Helper()
	entry, exists, err := index.roomMarker(ctx, userID, roomID)
	if err != nil {
		t.Fatalf("read indexed room marker: %v", err)
	}
	if !exists || string(entry.value) != want {
		t.Fatalf("indexed room marker = (%q, %v), want (%q, true)", entry.value, exists, want)
	}
}

func assertIndexedThreadMarker(t *testing.T, ctx context.Context, index *ReadStateIndex, userID, roomID, threadRoot, want string) {
	t.Helper()
	entry, exists, err := index.threadMarker(ctx, userID, roomID, threadRoot)
	if err != nil {
		t.Fatalf("read indexed thread marker: %v", err)
	}
	if !exists || string(entry.value) != want {
		t.Fatalf("indexed thread marker = (%q, %v), want (%q, true)", entry.value, exists, want)
	}
}

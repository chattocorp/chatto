package core

import (
	"context"
	"testing"
	"time"
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

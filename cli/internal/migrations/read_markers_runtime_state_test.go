package migrations

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"hmans.de/chatto/internal/testutil"
)

func TestMigrateReadMarkersToRuntimeState(t *testing.T) {
	ctx, legacy, runtimeState := setupReadMarkerMigrationKV(t)

	if _, err := legacy.Put(ctx, "room_read_event.U1.R1", []byte("Eroot1")); err != nil {
		t.Fatalf("put room marker: %v", err)
	}
	legacyThreadValue := []byte{0, 0, 0, 0, 0, 0, 0, 42}
	if _, err := legacy.Put(ctx, "thread_last_opened.U1.R1.Ethread1", legacyThreadValue); err != nil {
		t.Fatalf("put thread marker: %v", err)
	}

	if err := MigrateReadMarkersToRuntimeState(ctx, legacy, runtimeState, testLogger()); err != nil {
		t.Fatalf("migrate read markers: %v", err)
	}

	roomEntry, err := runtimeState.Get(ctx, "read.room.U1.R1")
	if err != nil {
		t.Fatalf("get migrated room marker: %v", err)
	}
	if got := string(roomEntry.Value()); got != "Eroot1" {
		t.Fatalf("room marker = %q, want Eroot1", got)
	}

	threadEntry, err := runtimeState.Get(ctx, "read.thread.U1.R1.Ethread1")
	if err != nil {
		t.Fatalf("get migrated thread marker: %v", err)
	}
	if got := string(threadEntry.Value()); got != string(legacyThreadValue) {
		t.Fatalf("thread marker bytes = %v, want %v", threadEntry.Value(), legacyThreadValue)
	}
}

func TestMigrateReadMarkersToRuntimeState_DoesNotOverwriteRuntimeState(t *testing.T) {
	ctx, legacy, runtimeState := setupReadMarkerMigrationKV(t)

	if _, err := legacy.Put(ctx, "room_read_event.U1.R1", []byte("legacy")); err != nil {
		t.Fatalf("put legacy marker: %v", err)
	}
	if _, err := runtimeState.Put(ctx, "read.room.U1.R1", []byte("newer")); err != nil {
		t.Fatalf("put runtime marker: %v", err)
	}

	if err := MigrateReadMarkersToRuntimeState(ctx, legacy, runtimeState, testLogger()); err != nil {
		t.Fatalf("migrate read markers: %v", err)
	}

	entry, err := runtimeState.Get(ctx, "read.room.U1.R1")
	if err != nil {
		t.Fatalf("get runtime marker: %v", err)
	}
	if got := string(entry.Value()); got != "newer" {
		t.Fatalf("runtime marker = %q, want newer", got)
	}
}

func TestMigrateThreadFollowsToRuntimeState(t *testing.T) {
	ctx, legacy, runtimeState := setupReadMarkerMigrationKV(t)

	if _, err := legacy.Put(ctx, "thread_follow.U1.R1.Ethread1", []byte{0x01}); err != nil {
		t.Fatalf("put legacy follow: %v", err)
	}

	if err := MigrateThreadFollowsToRuntimeState(ctx, legacy, runtimeState, testLogger()); err != nil {
		t.Fatalf("migrate thread follows: %v", err)
	}

	entry, err := runtimeState.Get(ctx, "thread_follow.U1.R1.Ethread1")
	if err != nil {
		t.Fatalf("get migrated follow: %v", err)
	}
	if got := entry.Value(); string(got) != string([]byte{0x01}) {
		t.Fatalf("thread follow bytes = %v, want [1]", got)
	}

	if _, err := legacy.Get(ctx, "thread_follow.U1.R1.Ethread1"); err != nil {
		t.Fatalf("legacy follow should be retained for rollback: %v", err)
	}
}

func TestMigrateThreadFollowsToRuntimeState_DoesNotOverwriteRuntimeState(t *testing.T) {
	ctx, legacy, runtimeState := setupReadMarkerMigrationKV(t)

	if _, err := legacy.Put(ctx, "thread_follow.U1.R1.Ethread1", []byte{0x01}); err != nil {
		t.Fatalf("put legacy follow: %v", err)
	}
	if _, err := runtimeState.Put(ctx, "thread_follow.U1.R1.Ethread1", []byte{0x02}); err != nil {
		t.Fatalf("put runtime follow: %v", err)
	}

	if err := MigrateThreadFollowsToRuntimeState(ctx, legacy, runtimeState, testLogger()); err != nil {
		t.Fatalf("migrate thread follows: %v", err)
	}

	entry, err := runtimeState.Get(ctx, "thread_follow.U1.R1.Ethread1")
	if err != nil {
		t.Fatalf("get runtime follow: %v", err)
	}
	if got := entry.Value(); string(got) != string([]byte{0x02}) {
		t.Fatalf("runtime follow bytes = %v, want [2]", got)
	}
}

func setupReadMarkerMigrationKV(t *testing.T) (context.Context, jetstream.KeyValue, jetstream.KeyValue) {
	t.Helper()

	_, nc := testutil.StartNATS(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	t.Cleanup(cancel)

	js, err := jetstream.New(nc)
	if err != nil {
		t.Fatalf("jetstream: %v", err)
	}

	legacy, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  "SERVER_RUNTIME",
		Storage: jetstream.MemoryStorage,
	})
	if err != nil {
		t.Fatalf("create legacy KV: %v", err)
	}
	runtimeState, err := js.CreateOrUpdateKeyValue(ctx, jetstream.KeyValueConfig{
		Bucket:  "RUNTIME_STATE",
		Storage: jetstream.MemoryStorage,
		History: 1,
	})
	if err != nil {
		t.Fatalf("create runtime state KV: %v", err)
	}
	return ctx, legacy, runtimeState
}

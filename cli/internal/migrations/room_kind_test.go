package migrations

import (
	"context"
	"testing"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func getRoom(t *testing.T, ctx context.Context, kv jetstream.KeyValue, key string) *corev1.Room {
	t.Helper()
	entry, err := kv.Get(ctx, key)
	if err != nil {
		t.Fatalf("get %s: %v", key, err)
	}
	var room corev1.Room
	if err := proto.Unmarshal(entry.Value(), &room); err != nil {
		t.Fatalf("unmarshal %s: %v", key, err)
	}
	return &room
}

func TestBackfillRoomKind_BackfillsLegacyRoomsFromKeyPrefix(t *testing.T) {
	ctx, kv := setupTestKV(t)

	// Legacy channel room: Kind is UNSPECIFIED (zero). The migration
	// recovers the kind from the key prefix ("room.channel.*").
	channel := &corev1.Room{Id: "Rchan", Name: "general"}
	channelData, err := proto.Marshal(channel)
	if err != nil {
		t.Fatalf("marshal channel: %v", err)
	}
	if _, err := kv.Put(ctx, "room.channel.Rchan", channelData); err != nil {
		t.Fatalf("put channel: %v", err)
	}

	// Legacy DM room.
	dm := &corev1.Room{Id: "Rdm"}
	dmData, err := proto.Marshal(dm)
	if err != nil {
		t.Fatalf("marshal dm: %v", err)
	}
	if _, err := kv.Put(ctx, "room.dm.Rdm", dmData); err != nil {
		t.Fatalf("put dm: %v", err)
	}

	// Already-migrated channel room: Kind is set; migration must not
	// touch it.
	migrated := &corev1.Room{Id: "Rmig", Kind: corev1.RoomKind_ROOM_KIND_CHANNEL}
	migratedData, err := proto.Marshal(migrated)
	if err != nil {
		t.Fatalf("marshal migrated: %v", err)
	}
	if _, err := kv.Put(ctx, "room.channel.Rmig", migratedData); err != nil {
		t.Fatalf("put migrated: %v", err)
	}
	before, err := kv.Get(ctx, "room.channel.Rmig")
	if err != nil {
		t.Fatalf("get migrated before: %v", err)
	}
	migratedRev := before.Revision()

	logger := log.New(nil)
	if err := BackfillRoomKind(ctx, kv, logger); err != nil {
		t.Fatalf("BackfillRoomKind: %v", err)
	}

	if got := getRoom(t, ctx, kv, "room.channel.Rchan"); got.Kind != corev1.RoomKind_ROOM_KIND_CHANNEL {
		t.Errorf("channel.Kind = %v, want CHANNEL", got.Kind)
	}
	if got := getRoom(t, ctx, kv, "room.dm.Rdm"); got.Kind != corev1.RoomKind_ROOM_KIND_DM {
		t.Errorf("dm.Kind = %v, want DM", got.Kind)
	}

	after, err := kv.Get(ctx, "room.channel.Rmig")
	if err != nil {
		t.Fatalf("get migrated after: %v", err)
	}
	if after.Revision() != migratedRev {
		t.Errorf("already-migrated room was rewritten (rev %d -> %d)", migratedRev, after.Revision())
	}
}

func TestBackfillRoomKind_IsIdempotent(t *testing.T) {
	ctx, kv := setupTestKV(t)

	room := &corev1.Room{Id: "Rchan"}
	data, err := proto.Marshal(room)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if _, err := kv.Put(ctx, "room.channel.Rchan", data); err != nil {
		t.Fatalf("put: %v", err)
	}

	logger := log.New(nil)
	if err := BackfillRoomKind(ctx, kv, logger); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	afterFirst, err := kv.Get(ctx, "room.channel.Rchan")
	if err != nil {
		t.Fatalf("get after first: %v", err)
	}
	revAfterFirst := afterFirst.Revision()

	if err := BackfillRoomKind(ctx, kv, logger); err != nil {
		t.Fatalf("second pass: %v", err)
	}
	afterSecond, err := kv.Get(ctx, "room.channel.Rchan")
	if err != nil {
		t.Fatalf("get after second: %v", err)
	}
	if afterSecond.Revision() != revAfterFirst {
		t.Errorf("second pass rewrote the record (rev %d -> %d) — not idempotent",
			revAfterFirst, afterSecond.Revision())
	}
}

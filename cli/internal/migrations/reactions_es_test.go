package migrations

import (
	"context"
	"encoding/binary"
	"io"
	"testing"
	"time"

	"github.com/charmbracelet/log"
	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/types/known/timestamppb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestMigrateReactionsToES_EmptyKV(t *testing.T) {
	rig := setupMessagesRig(t)
	reactionsKV := rig.reactionsKV(t)

	if err := MigrateReactionsToES(rig.ctx, rig.srcStream, reactionsKV, rig.publisher, log.New(io.Discard)); err != nil {
		t.Fatalf("migration: %v", err)
	}
	info, err := rig.evtStream.Info(rig.ctx)
	if err != nil {
		t.Fatalf("evt info: %v", err)
	}
	if info.State.Msgs != 0 {
		t.Errorf("EVT should be empty after empty reaction migration, got %d msgs", info.State.Msgs)
	}
}

func TestMigrateReactionsToES_ImportsCurrentStateAndReplays(t *testing.T) {
	rig := setupMessagesRig(t)
	reactionsKV := rig.reactionsKV(t)

	rig.publishLegacy(t, "server.room.channel.R1.msg.M1", &corev1.Event{
		Id:        "M1",
		ActorId:   "U1",
		CreatedAt: timestamppb.Now(),
		Event: &corev1.Event_MessagePosted{
			MessagePosted: &corev1.MessagePostedEvent{RoomId: "R1", EventId: "M1"},
		},
	})
	putReaction(t, rig.ctx, reactionsKV, "M1.thumbsup.U2", time.Unix(1700000000, 123))
	putReaction(t, rig.ctx, reactionsKV, "M1.heart.U3", time.Unix(1700000001, 456))

	if err := MigrateReactionsToES(rig.ctx, rig.srcStream, reactionsKV, rig.publisher, log.New(io.Discard)); err != nil {
		t.Fatalf("migration: %v", err)
	}
	info, err := rig.evtStream.Info(rig.ctx)
	if err != nil {
		t.Fatalf("evt info: %v", err)
	}
	if info.State.Msgs != 2 {
		t.Fatalf("EVT messages after first migration = %d, want 2", info.State.Msgs)
	}

	ev := rig.readEvent(t, 1)
	added := ev.GetReactionAdded()
	if added == nil {
		t.Fatalf("event 1 type = %T, want ReactionAdded", ev.GetEvent())
	}
	if added.GetRoomId() != "R1" || added.GetMessageEventId() != "M1" || ev.GetActorId() == "" || added.GetEmoji() == "" {
		t.Fatalf("unexpected migrated reaction event: actor=%q payload=%+v", ev.GetActorId(), added)
	}

	if err := MigrateReactionsToES(rig.ctx, rig.srcStream, reactionsKV, rig.publisher, log.New(io.Discard)); err != nil {
		t.Fatalf("replay migration: %v", err)
	}
	info, err = rig.evtStream.Info(rig.ctx)
	if err != nil {
		t.Fatalf("evt info after replay: %v", err)
	}
	if info.State.Msgs != 2 {
		t.Fatalf("EVT messages after replay = %d, want 2", info.State.Msgs)
	}
}

func (r *messagesTestRig) reactionsKV(t *testing.T) jetstream.KeyValue {
	t.Helper()
	kv, err := r.js.CreateOrUpdateKeyValue(r.ctx, jetstream.KeyValueConfig{
		Bucket:  "SERVER_REACTIONS_TEST",
		Storage: jetstream.MemoryStorage,
	})
	if err != nil {
		t.Fatalf("create SERVER_REACTIONS bucket: %v", err)
	}
	return kv
}

func putReaction(t *testing.T, ctx context.Context, kv jetstream.KeyValue, key string, ts time.Time) {
	t.Helper()
	value := make([]byte, 8)
	binary.BigEndian.PutUint64(value, uint64(ts.UnixNano()))
	if _, err := kv.Put(ctx, key, value); err != nil {
		t.Fatalf("put reaction %s: %v", key, err)
	}
}

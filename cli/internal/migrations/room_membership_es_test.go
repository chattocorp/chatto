package migrations

import (
	"testing"

	"github.com/nats-io/nats.go/jetstream"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func seedMembership(t *testing.T, kv jetstream.KeyValue, kind, roomID, userID string) {
	t.Helper()
	m := &corev1.RoomMembership{UserId: userID, RoomId: roomID}
	data, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("marshal membership: %v", err)
	}
	if _, err := kv.Put(t.Context(), "room_membership."+kind+"."+roomID+"."+userID, data); err != nil {
		t.Fatalf("put membership: %v", err)
	}
}

func TestMigrateRoomMembershipToES_EmptyKV(t *testing.T) {
	ctx, kv, _, publisher := setupTestES(t)
	_ = ctx

	if err := MigrateRoomMembershipToES(t.Context(), kv, publisher, testLogger()); err != nil {
		t.Fatalf("MigrateRoomMembershipToES: %v", err)
	}
	// Nothing to assert beyond "no error" — empty KV ⇒ no events.
}

func TestMigrateRoomMembershipToES_SeedsAndReplays(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

	seedMembership(t, kv, "channel", "R1", "U1")
	seedMembership(t, kv, "channel", "R1", "U2")
	seedMembership(t, kv, "channel", "R2", "U1")
	seedMembership(t, kv, "dm", "DM1", "U1")
	seedMembership(t, kv, "dm", "DM1", "U3")

	// First run: 3 aggregates seeded, 5 events emitted.
	if err := MigrateRoomMembershipToES(ctx, kv, publisher, testLogger()); err != nil {
		t.Fatalf("first run: %v", err)
	}

	cases := []struct {
		subject  string
		wantLast uint64 // expected last per-subject sequence == number of events on it
	}{
		{events.RoomAggregate("R1").Subject(), 2},
		{events.RoomAggregate("R2").Subject(), 1},
		{events.RoomAggregate("DM1").Subject(), 2},
	}
	for _, tc := range cases {
		msg, err := stream.GetLastMsgForSubject(ctx, tc.subject)
		if err != nil {
			t.Fatalf("GetLastMsgForSubject(%s): %v", tc.subject, err)
		}
		if msg.Sequence == 0 {
			t.Errorf("subject %s: expected non-zero last seq", tc.subject)
		}
	}

	info, err := stream.Info(ctx)
	if err != nil {
		t.Fatalf("stream info: %v", err)
	}
	if info.State.Msgs != 5 {
		t.Errorf("expected 5 stream messages after first run, got %d", info.State.Msgs)
	}

	// Replay: every aggregate already has events, so each AppendAt(seq=0)
	// hits ErrConflict and is skipped. No new messages should land.
	if err := MigrateRoomMembershipToES(ctx, kv, publisher, testLogger()); err != nil {
		t.Fatalf("replay run: %v", err)
	}
	infoReplay, err := stream.Info(ctx)
	if err != nil {
		t.Fatalf("stream info replay: %v", err)
	}
	if infoReplay.State.Msgs != 5 {
		t.Errorf("expected stream to still have 5 messages after replay, got %d", infoReplay.State.Msgs)
	}
}

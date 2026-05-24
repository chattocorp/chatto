package core

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// seedMembership creates a room and joins a user. Uses the regular
// JoinRoom path so KV reflects what production data looks like.
func seedMembership(t *testing.T, c *ChattoCore, ctx context.Context, kind RoomKind, roomID, userID string) {
	t.Helper()
	// Existing JoinRoom needs the room to exist and the user to exist;
	// for the migration we don't need that machinery — the migration
	// only reads the room_membership key, not Room or User. Write the
	// membership directly via the same KV pattern JoinRoom uses.
	c.storage.serverConfigKV.Put(ctx, "room_membership."+string(kind)+"."+roomID+"."+userID, mustMarshalMembership(t, &corev1.RoomMembership{UserId: userID, RoomId: roomID}))
}

func mustMarshalMembership(t *testing.T, m *corev1.RoomMembership) []byte {
	t.Helper()
	data, err := proto.Marshal(m)
	if err != nil {
		t.Fatalf("marshal membership: %v", err)
	}
	return data
}

func TestMigrateRoomMembership_EmptyKV(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	stats, err := core.MigrateRoomMembership(ctx)
	if err != nil {
		t.Fatalf("MigrateRoomMembership: %v", err)
	}
	if stats.SubjectsScanned != 0 || stats.EventsEmitted != 0 {
		t.Errorf("empty KV migration stats=%+v, want zeros", stats)
	}
}

func TestMigrateRoomMembership_MigratesAndIsReplayable(t *testing.T) {
	core, _ := setupTestCore(t)
	ctx := testContext(t)

	// Seed: two channel rooms, one DM room.
	seedMembership(t, core, ctx, KindChannel, "R1", "U1")
	seedMembership(t, core, ctx, KindChannel, "R1", "U2")
	seedMembership(t, core, ctx, KindChannel, "R2", "U1")
	seedMembership(t, core, ctx, KindDM, "DM1", "U1")
	seedMembership(t, core, ctx, KindDM, "DM1", "U3")

	// First migration run.
	stats, err := core.MigrateRoomMembership(ctx)
	if err != nil {
		t.Fatalf("MigrateRoomMembership: %v", err)
	}
	if stats.SubjectsScanned != 3 {
		t.Errorf("SubjectsScanned=%d, want 3", stats.SubjectsScanned)
	}
	if stats.SubjectsMigrated != 3 {
		t.Errorf("SubjectsMigrated=%d, want 3", stats.SubjectsMigrated)
	}
	if stats.SubjectsSkipped != 0 {
		t.Errorf("SubjectsSkipped=%d, want 0", stats.SubjectsSkipped)
	}
	if stats.EventsEmitted != 5 {
		t.Errorf("EventsEmitted=%d, want 5", stats.EventsEmitted)
	}

	// Second run on the same KV: every subject already has events,
	// so each subject hits ErrConflict on its first AppendAt and is
	// skipped. No new events should be written.
	replay, err := core.MigrateRoomMembership(ctx)
	if err != nil {
		t.Fatalf("MigrateRoomMembership replay: %v", err)
	}
	if replay.SubjectsScanned != 3 {
		t.Errorf("replay SubjectsScanned=%d, want 3", replay.SubjectsScanned)
	}
	if replay.SubjectsMigrated != 0 {
		t.Errorf("replay SubjectsMigrated=%d, want 0", replay.SubjectsMigrated)
	}
	if replay.SubjectsSkipped != 3 {
		t.Errorf("replay SubjectsSkipped=%d, want 3", replay.SubjectsSkipped)
	}
	if replay.EventsEmitted != 0 {
		t.Errorf("replay EventsEmitted=%d, want 0", replay.EventsEmitted)
	}

	// Verify last seq per subject equals the number of members.
	cases := []struct {
		subject string
		want    uint64
	}{
		{events.RoomAggregate("R1").Subject(), 2},
		{events.RoomAggregate("R2").Subject(), 1},
		{events.RoomAggregate("DM1").Subject(), 2},
	}
	for _, tc := range cases {
		msg, err := core.storage.serverEvtStream.GetLastMsgForSubject(ctx, tc.subject)
		if err != nil {
			t.Fatalf("GetLastMsgForSubject(%s): %v", tc.subject, err)
		}
		if msg.Sequence == 0 {
			t.Errorf("subject %s has zero seq", tc.subject)
		}
		// We can't directly read per-subject seq, but we can verify
		// at least the right number of messages were emitted by
		// running the projector and asserting its membership view.
		_ = tc.want
	}

	// setupTestCore already started the projector; we just wait for it
	// to catch up to the migration's emitted events.
	waitForCondition(t, 3*time.Second, func() bool {
		rooms, memberships := core.RoomMembership.Stats()
		return rooms == 3 && memberships == 5
	})

	if !core.RoomMembership.IsMember("R1", "U1") {
		t.Error("R1 should contain U1")
	}
	if !core.RoomMembership.IsMember("DM1", "U3") {
		t.Error("DM1 should contain U3")
	}
	if core.RoomMembership.IsMember("R2", "U2") {
		t.Error("R2 should NOT contain U2")
	}
}

func waitForCondition(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("condition not met within %v", timeout)
}

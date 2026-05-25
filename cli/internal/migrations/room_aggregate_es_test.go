package migrations

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func seedRoom(t *testing.T, kv jetstream.KeyValue, kind string, room *corev1.Room) {
	t.Helper()
	data, err := proto.Marshal(room)
	require.NoError(t, err)
	_, err = kv.Put(t.Context(), "room."+kind+"."+room.GetId(), data)
	require.NoError(t, err)
}

func seedMembership(t *testing.T, kv jetstream.KeyValue, kind, roomID, userID string) {
	t.Helper()
	m := &corev1.RoomMembership{UserId: userID, RoomId: roomID}
	data, err := proto.Marshal(m)
	require.NoError(t, err)
	_, err = kv.Put(t.Context(), "room_membership."+kind+"."+roomID+"."+userID, data)
	require.NoError(t, err)
}

func TestMigrateRoomAggregateToES_EmptyKV(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)
	require.NoError(t, MigrateRoomAggregateToES(ctx, kv, publisher, testLogger()))

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 0, info.State.Msgs)
}

func TestMigrateRoomAggregateToES_SeedsRoomThenMembers(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

	seedRoom(t, kv, "channel", &corev1.Room{
		Id:   "R1",
		Name: "general",
		Kind: corev1.RoomKind_ROOM_KIND_CHANNEL,
	})
	seedMembership(t, kv, "channel", "R1", "U1")
	seedMembership(t, kv, "channel", "R1", "U2")

	seedRoom(t, kv, "dm", &corev1.Room{
		Id:   "DM1",
		Kind: corev1.RoomKind_ROOM_KIND_DM,
	})
	seedMembership(t, kv, "dm", "DM1", "U3")

	seedRoom(t, kv, "channel", &corev1.Room{
		Id:       "R2",
		Name:     "archive",
		Kind:     corev1.RoomKind_ROOM_KIND_CHANNEL,
		Archived: true,
	})

	require.NoError(t, MigrateRoomAggregateToES(ctx, kv, publisher, testLogger()))

	// Stream count: R1 = RoomCreated + 2×UserJoined = 3
	//               DM1 = RoomCreated + UserJoined = 2
	//               R2 = RoomCreated + RoomArchived = 2
	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 7, info.State.Msgs)

	// First event on each subject must be RoomCreated.
	for _, roomID := range []string{"R1", "DM1", "R2"} {
		subject := events.RoomSubject(roomID)
		seq := firstSubjectSeq(t, ctx, stream, subject)
		msg, err := stream.GetMsg(ctx, seq)
		require.NoError(t, err)

		var ev corev1.Event
		require.NoError(t, proto.Unmarshal(msg.Data, &ev))
		_, ok := ev.GetEvent().(*corev1.Event_RoomCreated)
		require.True(t, ok, "expected first event on %s to be RoomCreated, got %T", subject, ev.GetEvent())
	}

	// R2 archived: last event on its subject must be RoomArchived.
	lastMsgR2, err := stream.GetLastMsgForSubject(ctx, events.RoomSubject("R2"))
	require.NoError(t, err)
	var archivedEv corev1.Event
	require.NoError(t, proto.Unmarshal(lastMsgR2.Data, &archivedEv))
	_, isArchive := archivedEv.GetEvent().(*corev1.Event_RoomArchived)
	require.True(t, isArchive, "expected last event on R2 to be RoomArchived")
}

func TestMigrateRoomAggregateToES_Replay(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

	seedRoom(t, kv, "channel", &corev1.Room{
		Id:   "R1",
		Name: "general",
		Kind: corev1.RoomKind_ROOM_KIND_CHANNEL,
	})
	seedMembership(t, kv, "channel", "R1", "U1")

	require.NoError(t, MigrateRoomAggregateToES(ctx, kv, publisher, testLogger()))
	infoFirst, err := stream.Info(ctx)
	require.NoError(t, err)

	// Replay: batch's first entry (RoomCreated, OCC seq=0) must
	// conflict; the entire room is skipped — stream stays put.
	require.NoError(t, MigrateRoomAggregateToES(ctx, kv, publisher, testLogger()))
	infoSecond, err := stream.Info(ctx)
	require.NoError(t, err)
	require.Equal(t, infoFirst.State.Msgs, infoSecond.State.Msgs, "replay must be a no-op")
}

func TestMigrateRoomAggregateToES_ChronologicalMembershipOrder(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

	seedRoom(t, kv, "channel", &corev1.Room{
		Id:   "R1",
		Name: "general",
		Kind: corev1.RoomKind_ROOM_KIND_CHANNEL,
	})
	// Seed in non-chronological order; the migration must sort by
	// KV-entry creation timestamp.
	seedMembership(t, kv, "channel", "R1", "Uearlier")
	time.Sleep(20 * time.Millisecond)
	seedMembership(t, kv, "channel", "R1", "Ulater")

	require.NoError(t, MigrateRoomAggregateToES(ctx, kv, publisher, testLogger()))

	subject := events.RoomSubject("R1")
	firstSeq := firstSubjectSeq(t, ctx, stream, subject)

	// Events at firstSeq+1 and firstSeq+2 must be the joins in
	// chronological order.
	for i, wantActor := range []string{"Uearlier", "Ulater"} {
		msg, err := stream.GetMsg(ctx, firstSeq+uint64(i+1))
		require.NoError(t, err)
		var ev corev1.Event
		require.NoError(t, proto.Unmarshal(msg.Data, &ev))
		require.Equal(t, wantActor, ev.GetActorId(), "join #%d actor", i)
	}
}

// firstSubjectSeq returns the lowest stream sequence carrying a
// message for the given subject. The test stream is small, so a
// linear walk is fine.
func firstSubjectSeq(t *testing.T, ctx context.Context, stream jetstream.Stream, subject string) uint64 {
	t.Helper()
	info, err := stream.Info(ctx)
	require.NoError(t, err)
	for seq := info.State.FirstSeq; seq <= info.State.LastSeq; seq++ {
		msg, err := stream.GetMsg(ctx, seq)
		if err != nil {
			continue
		}
		if msg.Subject == subject {
			return seq
		}
	}
	t.Fatalf("no message found on subject %s", subject)
	return 0
}

package migrations

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

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

func seedOldMembership(t *testing.T, kv jetstream.KeyValue, roomID, userID string) {
	t.Helper()
	m := &corev1.RoomMembership{UserId: userID, RoomId: roomID}
	data, err := proto.Marshal(m)
	require.NoError(t, err)
	_, err = kv.Put(t.Context(), "room_membership."+userID+"."+roomID, data)
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

	// First event on each room aggregate must be RoomCreated.
	for _, roomID := range []string{"R1", "DM1", "R2"} {
		agg := events.RoomAggregate(roomID)
		seq := firstAggregateSeq(t, ctx, stream, agg)
		msg, err := stream.GetMsg(ctx, seq)
		require.NoError(t, err)

		var ev corev1.Event
		require.NoError(t, proto.Unmarshal(msg.Data, &ev))
		_, ok := ev.GetEvent().(*corev1.Event_RoomCreated)
		require.True(t, ok, "expected first event on %s to be RoomCreated, got %T", agg.AllEventsFilter(), ev.GetEvent())
	}

	// R2 archived: the RoomArchived event must be on its own per-(agg, type) subject.
	archivedSubject := events.RoomAggregate("R2").Subject(events.EventRoomArchived)
	lastMsgR2, err := stream.GetLastMsgForSubject(ctx, archivedSubject)
	require.NoError(t, err)
	var archivedEv corev1.Event
	require.NoError(t, proto.Unmarshal(lastMsgR2.Data, &archivedEv))
	_, isArchive := archivedEv.GetEvent().(*corev1.Event_RoomArchived)
	require.True(t, isArchive, "expected last event on R2.room_archived to be RoomArchived")
}

func TestMigrateRoomAggregateToES_AcceptsOldMembershipKeyShape(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

	seedRoom(t, kv, "channel", &corev1.Room{
		Id:   "R1",
		Name: "general",
		Kind: corev1.RoomKind_ROOM_KIND_CHANNEL,
	})
	seedOldMembership(t, kv, "R1", "U1")

	require.NoError(t, MigrateRoomAggregateToES(ctx, kv, publisher, testLogger()))

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 2, info.State.Msgs)

	msg, err := stream.GetMsg(ctx, 2)
	require.NoError(t, err)
	var ev corev1.Event
	require.NoError(t, proto.Unmarshal(msg.Data, &ev))
	require.Equal(t, "U1", ev.GetActorId())
	require.Equal(t, "R1", ev.GetUserJoinedRoom().GetRoomId())
}

func TestMigrateRoomAggregateToES_DedupesOldAndKindedMembershipKeys(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

	seedRoom(t, kv, "channel", &corev1.Room{
		Id:   "R1",
		Name: "general",
		Kind: corev1.RoomKind_ROOM_KIND_CHANNEL,
	})
	seedMembership(t, kv, "channel", "R1", "U1")
	seedOldMembership(t, kv, "R1", "U1")

	require.NoError(t, MigrateRoomAggregateToES(ctx, kv, publisher, testLogger()))

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 2, info.State.Msgs)

	msg, err := stream.GetMsg(ctx, 2)
	require.NoError(t, err)
	var ev corev1.Event
	require.NoError(t, proto.Unmarshal(msg.Data, &ev))
	require.Equal(t, "U1", ev.GetActorId())
	require.Equal(t, "R1", ev.GetUserJoinedRoom().GetRoomId())
}

func TestMigrateRoomAggregateToES_BackfillsMissingMemberships(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

	seedRoom(t, kv, "channel", &corev1.Room{
		Id:   "R1",
		Name: "general",
		Kind: corev1.RoomKind_ROOM_KIND_CHANNEL,
	})
	seedMembership(t, kv, "channel", "R1", "U1")
	require.NoError(t, MigrateRoomAggregateToES(ctx, kv, publisher, testLogger()))

	seedOldMembership(t, kv, "R1", "U2")
	require.NoError(t, MigrateRoomAggregateToES(ctx, kv, publisher, testLogger()))

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 3, info.State.Msgs)

	msg, err := stream.GetMsg(ctx, 3)
	require.NoError(t, err)
	var ev corev1.Event
	require.NoError(t, proto.Unmarshal(msg.Data, &ev))
	require.Equal(t, "U2", ev.GetActorId())
	require.Equal(t, "R1", ev.GetUserJoinedRoom().GetRoomId())
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

	agg := events.RoomAggregate("R1")
	firstSeq := firstAggregateSeq(t, ctx, stream, agg)

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

func TestMigrateRoomAggregateToES_PreservesLegacyJoinEnvelope(t *testing.T) {
	ctx, kv, stream, publisher, js := setupTestESWithJS(t)
	legacyStream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     "SERVER_EVENTS_TEST",
		Subjects: []string{"server.>"},
		Storage:  jetstream.MemoryStorage,
	})
	require.NoError(t, err)

	seedRoom(t, kv, "dm", &corev1.Room{
		Id:   "DM1",
		Kind: corev1.RoomKind_ROOM_KIND_DM,
	})
	seedMembership(t, kv, "dm", "DM1", "U1")

	legacyCreatedAt := time.Date(2026, 3, 17, 13, 38, 0, 0, time.UTC)
	legacyJoin := &corev1.Event{
		Id:        "ElegacyJoinU1",
		ActorId:   "U1",
		CreatedAt: timestamppb.New(legacyCreatedAt),
		Event: &corev1.Event_UserJoinedRoom{
			UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: "DM1"},
		},
	}
	data, err := proto.Marshal(legacyJoin)
	require.NoError(t, err)
	_, err = js.Publish(ctx, "server.room.dm.DM1.meta", data)
	require.NoError(t, err)

	require.NoError(t, MigrateRoomAggregateToES(ctx, kv, publisher, testLogger(), legacyStream))

	msg, err := stream.GetMsg(ctx, 2)
	require.NoError(t, err)
	var ev corev1.Event
	require.NoError(t, proto.Unmarshal(msg.Data, &ev))
	require.Equal(t, "ElegacyJoinU1", ev.GetId())
	require.Equal(t, "U1", ev.GetActorId())
	require.True(t, ev.GetCreatedAt().AsTime().Equal(legacyCreatedAt))
	require.Equal(t, "DM1", ev.GetUserJoinedRoom().GetRoomId())
}

func TestMigrateRoomAggregateToES_PreservesLatestLegacyJoinForCurrentMembership(t *testing.T) {
	ctx, kv, stream, publisher, js := setupTestESWithJS(t)
	legacyStream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     "SERVER_EVENTS_TEST",
		Subjects: []string{"server.>"},
		Storage:  jetstream.MemoryStorage,
	})
	require.NoError(t, err)

	seedRoom(t, kv, "channel", &corev1.Room{
		Id:   "R1",
		Name: "general",
		Kind: corev1.RoomKind_ROOM_KIND_CHANNEL,
	})
	seedMembership(t, kv, "channel", "R1", "U1")

	firstJoinedAt := time.Date(2026, 3, 17, 13, 38, 0, 0, time.UTC)
	rejoinedAt := time.Date(2026, 5, 8, 9, 0, 0, 0, time.UTC)
	legacyJoins := []*corev1.Event{
		{
			Id:        "EfirstJoin",
			ActorId:   "U1",
			CreatedAt: timestamppb.New(firstJoinedAt),
			Event: &corev1.Event_UserJoinedRoom{
				UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: "R1"},
			},
		},
		{
			Id:        "Erejoin",
			ActorId:   "U1",
			CreatedAt: timestamppb.New(rejoinedAt),
			Event: &corev1.Event_UserJoinedRoom{
				UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: "R1"},
			},
		},
	}
	for _, legacyJoin := range legacyJoins {
		data, err := proto.Marshal(legacyJoin)
		require.NoError(t, err)
		_, err = js.Publish(ctx, "server.room.channel.R1.meta", data)
		require.NoError(t, err)
	}

	require.NoError(t, MigrateRoomAggregateToES(ctx, kv, publisher, testLogger(), legacyStream))

	msg, err := stream.GetMsg(ctx, 2)
	require.NoError(t, err)
	var ev corev1.Event
	require.NoError(t, proto.Unmarshal(msg.Data, &ev))
	require.Equal(t, "Erejoin", ev.GetId())
	require.Equal(t, "U1", ev.GetActorId())
	require.True(t, ev.GetCreatedAt().AsTime().Equal(rejoinedAt))
	require.Equal(t, "R1", ev.GetUserJoinedRoom().GetRoomId())
}

func TestMigrateRoomAggregateToES_DoesNotResurrectLegacyJoinWithoutMembership(t *testing.T) {
	ctx, kv, stream, publisher, js := setupTestESWithJS(t)
	legacyStream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     "SERVER_EVENTS_TEST",
		Subjects: []string{"server.>"},
		Storage:  jetstream.MemoryStorage,
	})
	require.NoError(t, err)

	seedRoom(t, kv, "channel", &corev1.Room{
		Id:   "R1",
		Name: "general",
		Kind: corev1.RoomKind_ROOM_KIND_CHANNEL,
	})

	legacyJoin := &corev1.Event{
		Id:        "EdepartedJoin",
		ActorId:   "Udeparted",
		CreatedAt: timestamppb.New(time.Date(2026, 3, 17, 13, 38, 0, 0, time.UTC)),
		Event: &corev1.Event_UserJoinedRoom{
			UserJoinedRoom: &corev1.UserJoinedRoomEvent{RoomId: "R1"},
		},
	}
	data, err := proto.Marshal(legacyJoin)
	require.NoError(t, err)
	_, err = js.Publish(ctx, "server.room.channel.R1.meta", data)
	require.NoError(t, err)

	require.NoError(t, MigrateRoomAggregateToES(ctx, kv, publisher, testLogger(), legacyStream))

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, info.State.Msgs)

	msg, err := stream.GetMsg(ctx, 1)
	require.NoError(t, err)
	var ev corev1.Event
	require.NoError(t, proto.Unmarshal(msg.Data, &ev))
	require.NotNil(t, ev.GetRoomCreated())
}

// firstAggregateSeq returns the lowest stream sequence carrying a
// message for any subject under the aggregate's full filter (i.e.
// any event on the aggregate, regardless of event type). Linear
// walk; fine for small test streams.
func firstAggregateSeq(t *testing.T, ctx context.Context, stream jetstream.Stream, agg events.Aggregate) uint64 {
	t.Helper()
	info, err := stream.Info(ctx)
	require.NoError(t, err)
	prefix := events.SubjectRoot + agg.Type + "." + agg.ID + "."
	for seq := info.State.FirstSeq; seq <= info.State.LastSeq; seq++ {
		msg, err := stream.GetMsg(ctx, seq)
		if err != nil {
			continue
		}
		if strings.HasPrefix(msg.Subject, prefix) {
			return seq
		}
	}
	t.Fatalf("no message found under aggregate filter %s", agg.AllEventsFilter())
	return 0
}

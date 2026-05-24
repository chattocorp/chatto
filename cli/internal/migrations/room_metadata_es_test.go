package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestMigrateRoomMetadataToES_EmptyKV(t *testing.T) {
	ctx, kv, _, publisher := setupTestES(t)
	require.NoError(t, MigrateRoomMetadataToES(ctx, kv, publisher, testLogger()))
}

func TestMigrateRoomMetadataToES_SeedsAndIsReplayable(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

	rooms := []*corev1.Room{
		{Id: "R1", Name: "general", Description: "default", Kind: corev1.RoomKind_ROOM_KIND_CHANNEL},
		{Id: "R2", Name: "announcements", Description: "ann", Kind: corev1.RoomKind_ROOM_KIND_CHANNEL, Archived: true},
		{Id: "DM1", Name: "", Description: "", Kind: corev1.RoomKind_ROOM_KIND_DM},
	}
	for _, r := range rooms {
		data, err := proto.Marshal(r)
		require.NoError(t, err)
		var kvKey string
		switch r.Kind {
		case corev1.RoomKind_ROOM_KIND_CHANNEL:
			kvKey = "room.channel." + r.Id
		case corev1.RoomKind_ROOM_KIND_DM:
			kvKey = "room.dm." + r.Id
		}
		_, err = kv.Put(ctx, kvKey, data)
		require.NoError(t, err)
	}

	require.NoError(t, MigrateRoomMetadataToES(ctx, kv, publisher, testLogger()))

	// R1: one RoomCreated.
	msgR1, err := stream.GetLastMsgForSubject(ctx, events.RoomAggregate("R1").Subject())
	require.NoError(t, err)
	require.NotZero(t, msgR1.Sequence)

	// R2 (archived): one RoomCreated + one RoomArchived. Last subject
	// message should be the archive event.
	msgR2, err := stream.GetLastMsgForSubject(ctx, events.RoomAggregate("R2").Subject())
	require.NoError(t, err)
	var ev corev1.Event
	require.NoError(t, proto.Unmarshal(msgR2.Data, &ev))
	_, isArchive := ev.GetEvent().(*corev1.Event_RoomArchived)
	require.True(t, isArchive, "expected last event on R2 to be RoomArchived")

	// DM: should be present too.
	_, err = stream.GetLastMsgForSubject(ctx, events.RoomAggregate("DM1").Subject())
	require.NoError(t, err)

	// Stream message count: 3 rooms + 1 archive event = 4.
	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 4, info.State.Msgs)

	// Replay: every room aggregate already has events, so the
	// AppendAt(seq=0) skips each room. No new messages emitted.
	require.NoError(t, MigrateRoomMetadataToES(ctx, kv, publisher, testLogger()))
	infoReplay, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 4, infoReplay.State.Msgs)
}

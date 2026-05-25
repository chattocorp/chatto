package migrations

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/events"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestMigrateRoomGroupsToES_EmptyKV(t *testing.T) {
	ctx, kv, _, publisher := setupTestES(t)
	require.NoError(t, MigrateRoomGroupsToES(ctx, kv, publisher, testLogger()))
}

func TestMigrateRoomGroupsToES_SeedsAndIsReplayable(t *testing.T) {
	ctx, kv, stream, publisher := setupTestES(t)

	groups := []*corev1.RoomGroup{
		{Id: "G1", Name: "Lobby", Description: "default", RoomIds: []string{"R1", "R2"}},
		{Id: "G2", Name: "Projects", Description: "", RoomIds: []string{}},
		{Id: "G3", Name: "Topics", RoomIds: []string{"R3", "R4", "R5"}},
	}
	for _, g := range groups {
		data, err := proto.Marshal(g)
		require.NoError(t, err)
		_, err = kv.Put(ctx, "room_group."+g.Id, data)
		require.NoError(t, err)
	}

	require.NoError(t, MigrateRoomGroupsToES(ctx, kv, publisher, testLogger()))

	// Each group has 1 RoomGroupCreated + N RoomAddedToGroup events.
	// Verify the last event for each group is the right shape and that
	// the per-aggregate sequence count matches.
	cases := []struct {
		groupID     string
		expectedMsg int // 1 (Created) + len(roomIDs)
	}{
		{"G1", 3},
		{"G2", 1},
		{"G3", 4},
	}
	totalMsgs := 0
	for _, tc := range cases {
		// AllEventsFilter is a wildcard pattern — matches the last
		// message under any per-(agg, event-type) subject for this group.
		filter := events.GroupAggregate(tc.groupID).AllEventsFilter()
		msg, err := stream.GetLastMsgForSubject(ctx, filter)
		require.NoError(t, err)
		require.NotZero(t, msg.Sequence)
		totalMsgs += tc.expectedMsg
	}

	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, totalMsgs, info.State.Msgs)

	// Spot-check the last event on G1 — should be the last
	// RoomAddedToGroup (R2, since R1 was added first).
	msgG1, err := stream.GetLastMsgForSubject(ctx, events.GroupAggregate("G1").AllEventsFilter())
	require.NoError(t, err)
	var ev corev1.Event
	require.NoError(t, proto.Unmarshal(msgG1.Data, &ev))
	add, ok := ev.GetEvent().(*corev1.Event_RoomAddedToGroup)
	require.True(t, ok)
	require.Equal(t, "R2", add.RoomAddedToGroup.GetRoomId())

	// Replay: groups are already on the stream, so AppendAt(seq=0)
	// conflicts for each and the migration emits nothing new.
	require.NoError(t, MigrateRoomGroupsToES(ctx, kv, publisher, testLogger()))
	infoReplay, err := stream.Info(ctx)
	require.NoError(t, err)
	require.EqualValues(t, totalMsgs, infoReplay.State.Msgs)
}

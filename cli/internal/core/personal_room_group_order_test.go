package core

import (
	"strings"
	"testing"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/cli/internal/readmodel"
	"github.com/stretchr/testify/require"
)

func TestPersonalRoomGroupOrderValidation(t *testing.T) {
	for _, ids := range [][]string{nil, {}, {"a", "b"}} {
		require.NoError(t, validatePersonalRoomGroupOrder(ids))
	}
	for _, ids := range [][]string{{""}, {"a", "a"}, {strings.Repeat("a", 129)}, make([]string, 1001)} {
		require.Error(t, validatePersonalRoomGroupOrder(ids))
	}
}

func TestPersonalRoomGroupOrderMerge(t *testing.T) {
	groups := []*DirectoryRoomGroup{
		{Group: &readmodel.RoomGroup{Id: "a"}},
		{Group: &readmodel.RoomGroup{Id: "b"}},
		{Group: &readmodel.RoomGroup{Id: "new"}},
	}
	for _, tc := range []struct {
		name        string
		order, want []string
	}{
		{"server default", nil, []string{"a", "b", "new"}},
		{"personal and new", []string{"b", "a"}, []string{"b", "a", "new"}},
		{"deleted and invisible", []string{"deleted", "hidden", "b"}, []string{"b", "a", "new"}},
		{"defensive duplicate", []string{"b", "b"}, []string{"b", "a", "new"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := mergePersonalRoomGroupOrder(groups, tc.order)
			ids := make([]string, len(got))
			for i, group := range got {
				ids[i] = group.Group.Id
			}
			require.Equal(t, tc.want, ids)
			require.Equal(t, "a", groups[0].Group.Id)
		})
	}
}

func TestPersonalRoomGroupOrderReplayAndSnapshot(t *testing.T) {
	p := NewConfigProjection()
	apply := func(ids []string) {
		require.NoError(t, p.Apply(&evtv1.Event{Event: &evtv1.Event_UserRoomGroupOrderChanged{
			UserRoomGroupOrderChanged: &evtv1.UserRoomGroupOrderChangedEvent{UserId: "u", GroupIds: ids},
		}}, 1))
	}
	apply([]string{"b", "a"})
	got := p.RoomGroupOrder("u")
	got[0] = "mutated"
	require.Equal(t, []string{"b", "a"}, p.RoomGroupOrder("u"))
	require.Empty(t, p.RoomGroupOrder("other"))
	data, err := p.Snapshot()
	require.NoError(t, err)
	restored := NewConfigProjection()
	require.NoError(t, restored.Restore(data))
	require.Equal(t, p.RoomGroupOrder("u"), restored.RoomGroupOrder("u"))
	apply([]string{"a"})
	require.Equal(t, []string{"a"}, p.RoomGroupOrder("u"))
	apply(nil)
	require.Empty(t, p.RoomGroupOrder("u"))
}

func TestPersonalRoomGroupOrderLiveClassification(t *testing.T) {
	event := &evtv1.Event{Event: &evtv1.Event_UserRoomGroupOrderChanged{
		UserRoomGroupOrderChanged: &evtv1.UserRoomGroupOrderChangedEvent{UserId: "owner", GroupIds: []string{"a"}},
	}}
	require.True(t, isDeliverableLiveEVTUserConfigEvent(event))
	require.Equal(t, "owner", userIDOfUserConfigEvent(event))
}

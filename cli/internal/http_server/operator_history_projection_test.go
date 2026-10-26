package http_server

import (
	"testing"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/stretchr/testify/require"
)

func TestHistoricalThreadAndReactionRealtimeSuppression(t *testing.T) {
	t.Parallel()
	for _, event := range []*evtv1.Event{
		{Event: &evtv1.Event_ThreadCreated{ThreadCreated: &evtv1.ThreadCreatedEvent{RoomId: "room", ThreadRootEventId: "root", HistoricalImport: true}}},
		{Event: &evtv1.Event_ReactionAdded{ReactionAdded: &evtv1.ReactionAddedEvent{RoomId: "room", MessageEventId: "root", Emoji: "heart", UserId: "user", HistoricalImport: true}}},
	} {
		require.Nil(t, projectRealtimeEvent("viewer", event))
		if thread := event.GetThreadCreated(); thread != nil {
			thread.HistoricalImport = false
		} else {
			event.GetReactionAdded().HistoricalImport = false
		}
		require.NotNil(t, projectRealtimeEvent("viewer", event))
	}
}

func TestTombstoneRetractionUsesOrdinaryRealtimeDelivery(t *testing.T) {
	t.Parallel()
	event := &evtv1.Event{Event: &evtv1.Event_MessageRetracted{
		MessageRetracted: &evtv1.MessageRetractedEvent{RoomId: "room", EventId: "root"},
	}}
	projected := projectRealtimeEvent("viewer", event)
	require.NotNil(t, projected)
	require.Equal(t, "room", projected.GetMessageRetracted().GetRoomId())
	require.Equal(t, "root", projected.GetMessageRetracted().GetMessageEventId())
}

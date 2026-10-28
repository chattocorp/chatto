package core

import (
	"testing"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/stretchr/testify/require"
)

func TestRoomMembershipEventsSetting(t *testing.T) {
	t.Parallel()
	chatto, _ := setupTestCore(t)
	ctx := testContext(t)
	manager, err := chatto.CreateUser(ctx, SystemActorID, "activity-manager", "Manager", "password123")
	require.NoError(t, err)
	room, err := chatto.CreateRoom(ctx, SystemActorID, KindChannel, "", "activity", "")
	require.NoError(t, err)
	require.True(t, room.ShowMembershipEvents)
	input := RoomUpdateInput{ActorID: manager.Id, RoomID: room.Id, ShowMembershipEvents: new(false)}
	_, err = chatto.RoomCommands().UpdateRoom(ctx, input)
	require.ErrorIs(t, err, ErrPermissionDenied)
	require.NoError(t, chatto.GrantUserRoomPermission(ctx, SystemActorID, room.Id, manager.Id, PermRoomManage))
	updated, err := chatto.RoomCommands().UpdateRoom(ctx, input)
	require.NoError(t, err)
	require.False(t, updated.ShowMembershipEvents)
	_, err = chatto.RoomCommands().UpdateRoom(ctx, input)
	require.NoError(t, err)
	facts, _, err := chatto.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventRoomMembershipEventsVisibilityChanged))
	require.NoError(t, err)
	require.Len(t, facts, 1, "retrying the same intent must not append another fact")
	require.False(t, IsVisibleRoomTimelineEntry(facts[0]))
	require.True(t, isDeliverableLiveEVTRoomEvent(facts[0]))
	require.Equal(t, room.Id, roomIDOfEvent(facts[0]))
	_, err = chatto.JoinRoom(ctx, manager.Id, KindChannel, manager.Id, room.Id)
	require.NoError(t, err)
	joins, _, err := chatto.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventUserJoinedRoom))
	require.NoError(t, err)
	require.NotEmpty(t, joins)
	require.True(t, IsVisibleRoomTimelineEntry(joins[0]), "server history still includes membership facts")
	require.True(t, isDeliverableLiveEVTRoomEvent(joins[0]))
	input.ShowMembershipEvents = new(true)
	updated, err = chatto.RoomCommands().UpdateRoom(ctx, input)
	require.NoError(t, err)
	require.True(t, updated.ShowMembershipEvents)
	dm, _, err := chatto.FindOrCreateDM(ctx, manager.Id, []string{manager.Id})
	require.NoError(t, err)
	input.RoomID = dm.Id
	_, err = chatto.RoomCommands().UpdateRoom(ctx, input)
	require.ErrorIs(t, err, ErrInvalidArgument)
}

func TestRoomMembershipEventsReplayAndSnapshot(t *testing.T) {
	t.Parallel()
	p := NewRoomDirectoryProjection()
	require.NoError(t, p.Apply(&evtv1.Event{Event: &evtv1.Event_RoomCreated{RoomCreated: &evtv1.RoomCreatedEvent{RoomId: "legacy", Kind: evtv1.RoomKind_ROOM_KIND_CHANNEL}}}, 1))
	room, ok := p.Catalog.Get("legacy")
	require.True(t, ok)
	require.True(t, room.ShowMembershipEvents, "old creation facts default to enabled")
	for i, show := range []bool{false, true} {
		require.NoError(t, p.Apply(&evtv1.Event{Event: &evtv1.Event_RoomMembershipEventsVisibilityChanged{RoomMembershipEventsVisibilityChanged: &evtv1.RoomMembershipEventsVisibilityChangedEvent{RoomId: "legacy", ShowMembershipEvents: show}}}, uint64(i+2)))
		payload, err := p.Snapshot()
		require.NoError(t, err)
		restored := NewRoomDirectoryProjection()
		require.NoError(t, restored.Restore(payload))
		room, ok := restored.Catalog.Get("legacy")
		require.True(t, ok)
		require.Equal(t, show, room.ShowMembershipEvents)
	}
}

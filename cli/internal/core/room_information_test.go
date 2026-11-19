package core

import (
	"strings"
	"testing"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/stretchr/testify/require"
)

func TestRoomInformation(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	manager, err := c.CreateUser(ctx, SystemActorID, "room-manager", "Manager", "password123")
	require.NoError(t, err)
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "room-info", "short description")
	require.NoError(t, err)
	text := "## Guidelines\n\n    indented code\n"
	input := RoomUpdateInput{ActorID: manager.Id, RoomID: room.Id, Information: &text}
	_, err = c.RoomCommands().UpdateRoom(ctx, input)
	require.ErrorIs(t, err, ErrPermissionDenied)
	require.NoError(t, c.GrantUserRoomPermission(ctx, SystemActorID, room.Id, manager.Id, PermRoomManage))
	for range 2 {
		updated, err := c.RoomCommands().UpdateRoom(ctx, input)
		require.NoError(t, err)
		require.Equal(t, text, updated.Information)
		require.Equal(t, "short description", updated.Description)
	}
	facts, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).Subject(evtstream.EventRoomInformationChanged))
	require.NoError(t, err)
	require.Len(t, facts, 1)
	require.True(t, IsVisibleRoomTimelineEntry(facts[0]))
	require.True(t, isDeliverableLiveEVTRoomEvent(facts[0]))
	require.False(t, isRoomDirectoryProjectionEvent(facts[0]), "a directory listing alone must not disclose edit activity")
	require.Equal(t, room.Id, roomIDOfEvent(facts[0]))

	directory := NewRoomDirectoryProjection()
	require.NoError(t, directory.Apply(roomCreatedEvent(room.Id, room.Name, room.Description, evtv1.RoomKind_ROOM_KIND_CHANNEL), 1))
	require.NoError(t, directory.Apply(facts[0], 2))
	snapshot, err := directory.Snapshot()
	require.NoError(t, err)
	restored := NewRoomDirectoryProjection()
	require.NoError(t, restored.Restore(snapshot))
	restoredRoom, ok := restored.Catalog.Get(room.Id)
	require.True(t, ok)
	require.Equal(t, text, restoredRoom.Information)

	input.Information = new(strings.Repeat("😀", 10001))
	_, err = c.RoomCommands().UpdateRoom(ctx, input)
	require.ErrorIs(t, err, ErrInvalidArgument)
	input.Information = new(strings.Repeat("😀", 10000))
	_, err = c.RoomCommands().UpdateRoom(ctx, input)
	require.NoError(t, err)
	input.Information = new("")
	cleared, err := c.RoomCommands().UpdateRoom(ctx, input)
	require.NoError(t, err)
	require.Empty(t, cleared.Information)
	dm, _, err := c.FindOrCreateDM(ctx, manager.Id, []string{manager.Id})
	require.NoError(t, err)
	input.RoomID = dm.Id
	_, err = c.RoomCommands().UpdateRoom(ctx, input)
	require.ErrorIs(t, err, ErrInvalidArgument)
}

package core

import (
	"context"
	"testing"
	"time"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	"github.com/chattocorp/chatto/cli/internal/evtstream"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/pkg/events"
	"github.com/stretchr/testify/require"
)

// Only explicit membership is needed for these DM tests. Other commands fail
// if they try to use the embedded, unconfigured membership operations.
type dmTestMemberships struct {
	roomCommandMemberships
	rooms *RoomModel
}

func (m dmTestMemberships) RoomMembershipExists(_ context.Context, _ RoomKind, userID, roomID string) (bool, error) {
	return m.rooms.hasExplicitRoomMembership(roomID, userID), nil
}

func TestRoomCommandModelDMWithoutCoreCallbacks(t *testing.T) {
	t.Parallel()
	core, _ := setupTestCore(t)
	ctx := testContext(t)
	commands := &RoomCommandModel{
		rooms: core.roomModel, publisher: core.EventPublisher, logger: testCoreLogger(),
		memberships: dmTestMemberships{rooms: core.roomModel},
		readState:   &ReadStateModel{index: core.readStateModel.index},
	}
	room, created, err := commands.findOrCreateDM(ctx, "U-one", []string{"U-two"})
	require.NoError(t, err)
	require.True(t, created)
	require.Equal(t, DMRoomID([]string{"U-one", "U-two"}), room.Id)
	for _, userID := range []string{"U-one", "U-two"} {
		require.True(t, commands.rooms.hasExplicitRoomMembership(room.Id, userID))
		_, exists, err := commands.readState.index.roomMarker(ctx, userID, room.Id)
		require.NoError(t, err)
		require.True(t, exists, "initial read marker")
	}
	again, created, err := commands.findOrCreateDM(ctx, "U-two", []string{"U-one"})
	require.NoError(t, err)
	require.False(t, created)
	require.Equal(t, room.Id, again.Id)
	facts, _, err := commands.publisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.Id).AllEventsFilter())
	require.NoError(t, err)
	require.Len(t, facts, 3, "one creation and two joins")
}

func TestRoomCommandModelDMConflictWaitsAndChecksMembership(t *testing.T) {
	t.Parallel()
	for _, removed := range []bool{false, true} {
		name := "member"
		if removed {
			name = "removed member"
		}
		t.Run(name, func(t *testing.T) {
			harness := newTestEventHarness(t)
			ctx := testContext(t)
			directory := NewRoomDirectoryProjection()
			projector := harness.projector(directory)
			rooms := newTestRoomModel(t, directory, projector, NewRoomGroupLayoutProjection(), nil, nil, nil, nil, nil, nil, nil)
			commands := &RoomCommandModel{rooms: rooms, publisher: harness.publisher, logger: testCoreLogger(), memberships: dmTestMemberships{rooms: rooms}}
			roomID := DMRoomID([]string{"U-one", "U-two"})
			facts := []*evtv1.Event{
				newEvent(SystemActorID, &evtv1.Event{Event: &evtv1.Event_RoomCreated{RoomCreated: &evtv1.RoomCreatedEvent{RoomId: roomID, Kind: evtv1.RoomKind_ROOM_KIND_DM}}}),
				newEvent("U-one", &evtv1.Event{Event: &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: roomID}}}),
				newEvent("U-two", &evtv1.Event{Event: &evtv1.Event_UserJoinedRoom{UserJoinedRoom: &evtv1.UserJoinedRoomEvent{RoomId: roomID}}}),
			}
			if removed {
				facts = append(facts, newEvent("U-one", &evtv1.Event{Event: &evtv1.Event_RoomMemberBanned{RoomMemberBanned: &evtv1.RoomMemberBannedEvent{RoomId: roomID, UserId: "U-one", Reason: "test removal"}}}))
			}
			entries := make([]evtstream.BatchEntry, 0, len(facts))
			for _, fact := range facts {
				entries = append(entries, evtstream.BatchEntry{Subject: evtstream.RoomAggregate(roomID).SubjectFor(fact), Event: fact})
			}
			entries[0].Expect = events.ExpectFilterSeq(evtstream.RoomAggregate(roomID).AllEventsFilter(), 0)
			_, err := harness.publisher.AppendBatch(ctx, entries)
			require.NoError(t, err)
			type result struct {
				room    *readmodel.Room
				created bool
				err     error
			}
			done := make(chan result, 1)
			go func() {
				room, created, err := commands.findOrCreateDM(ctx, "U-one", []string{"U-two"})
				done <- result{room, created, err}
			}()
			select {
			case got := <-done:
				t.Fatalf("returned before projection catch-up: %v", got.err)
			case <-time.After(25 * time.Millisecond):
			}
			startTestProjector(t, projector)
			got := <-done
			require.False(t, got.created)
			if removed {
				require.ErrorContains(t, got.err, "not a participant")
				require.Nil(t, got.room)
			} else {
				require.NoError(t, got.err)
				require.Equal(t, roomID, got.room.Id)
			}
		})
	}
}

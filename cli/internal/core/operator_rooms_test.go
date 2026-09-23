package core

import (
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func TestCreateOperatorRoomSourceRetry(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	groups, err := c.ListRoomGroupsOrdered(ctx, KindChannel)
	require.NoError(t, err)
	require.NotEmpty(t, groups)

	input := OperatorRoomCreateInput{Name: "  Imported Room  ", Description: "Imported description", Source: "discord", SourceID: "123"}
	created, err := c.CreateOperatorRoom(ctx, input)
	require.NoError(t, err)
	require.Equal(t, "Imported Room", created.GetName())
	require.Equal(t, groups[0].GetId(), created.GetGroupId())
	actual, err := c.GetRoom(ctx, KindChannel, created.GetId())
	require.NoError(t, err)
	require.Equal(t, created.GetId(), actual.GetId())
	rooms, err := c.ListRooms(ctx, KindChannel)
	require.NoError(t, err)
	require.Contains(t, roomIDs(rooms), created.GetId())

	retried, err := c.CreateOperatorRoom(ctx, input)
	require.NoError(t, err)
	require.True(t, proto.Equal(created, retried))
	events, _, err := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(created.GetId()).Subject(evtstream.EventRoomCreated))
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.Equal(t, SystemActorID, events[0].GetActorId())

	changed := input
	changed.Description = "Changed"
	_, err = c.CreateOperatorRoom(ctx, changed)
	require.ErrorIs(t, err, ErrRoomSourceConflict)
	changed = input
	changed.Name = "Other"
	_, err = c.CreateOperatorRoom(ctx, changed)
	require.ErrorIs(t, err, ErrRoomSourceConflict)
	changed = input
	changed.GroupID = groups[0].GetId()
	_, err = c.CreateOperatorRoom(ctx, changed)
	require.ErrorIs(t, err, ErrRoomSourceConflict, "omitted and explicit group flags are different input")

	// The key still names the original creation after later room changes.
	_, err = c.UpdateRoom(ctx, SystemActorID, KindChannel, created.GetId(), "Renamed", "New description")
	require.NoError(t, err)
	retried, err = c.CreateOperatorRoom(ctx, input)
	require.NoError(t, err)
	require.True(t, proto.Equal(created, retried))
	require.NoError(t, c.DeleteRoom(ctx, SystemActorID, KindChannel, created.GetId()))
	retried, err = c.CreateOperatorRoom(ctx, input)
	require.NoError(t, err)
	require.True(t, proto.Equal(created, retried))

	// An ordinary creation needs no source key and uses the same default group.
	ordinary, err := c.CreateOperatorRoom(ctx, OperatorRoomCreateInput{Name: "Ordinary operator room"})
	require.NoError(t, err)
	require.Equal(t, groups[0].GetId(), ordinary.GetGroupId())
}

func TestCreateOperatorRoomValidationAndExplicitGroup(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	group, err := c.CreateRoomGroup(ctx, SystemActorID, "Imported group", "")
	require.NoError(t, err)
	room, err := c.CreateOperatorRoom(ctx, OperatorRoomCreateInput{Name: "In group", GroupID: group.GetId()})
	require.NoError(t, err)
	require.Equal(t, group.GetId(), room.GetGroupId())

	for _, input := range []OperatorRoomCreateInput{
		{Name: ""},
		{Name: "bad\nname"},
		{Name: "valid", Description: string(make([]byte, RoomDescriptionMaxLength+1))},
		{Name: "valid", GroupID: "missing-group"},
		{Name: "valid", Source: "discord"},
		{Name: "valid", SourceID: "123"},
		{Name: "valid", Source: " ", SourceID: "123"},
		{Name: "valid", Source: "discord", SourceID: " "},
	} {
		_, err := c.CreateOperatorRoom(ctx, input)
		require.Error(t, err, "input = %+v", input)
	}
}

func TestCreateOperatorRoomSourceSurvivesSnapshotAndReplay(t *testing.T) {
	c, nc := setupTestCore(t)
	ctx := testContext(t)
	input := OperatorRoomCreateInput{Name: "Snapshot room", Source: "discord", SourceID: "456"}
	created, err := c.CreateOperatorRoom(ctx, input)
	require.NoError(t, err)
	require.NoError(t, c.DeleteRoom(ctx, SystemActorID, KindChannel, created.GetId()))

	payload, err := c.roomModel.directory.Projection().Snapshot()
	require.NoError(t, err)
	restored := NewRoomDirectoryProjection()
	require.NoError(t, restored.Restore(payload))
	claim := restored.Catalog.CreationClaimSnapshot(input.Name, "", operatorRoomDigest("room-source-v1", input.Source, input.SourceID)).sourceClaim
	require.NotNil(t, claim)
	require.True(t, proto.Equal(created, claim.createdRoom))

	second, err := NewChattoCore(ctx, nc, c.config)
	require.NoError(t, err)
	startCoreServices(t, second)
	retried, err := second.CreateOperatorRoom(ctx, input)
	require.NoError(t, err)
	require.True(t, proto.Equal(created, retried))
}

func TestCreateOperatorRoomRejectsPartialBatch(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	limitCompoundBatch(t, ctx, c, 1) // Creation needs a room fact and a group placement fact.
	input := OperatorRoomCreateInput{Name: "No partial room", Source: "discord", SourceID: "partial"}
	_, err := c.CreateOperatorRoom(ctx, input)
	require.Error(t, err)
	claim := c.roomModel.creationClaimSnapshot(input.Name, "", operatorRoomDigest("room-source-v1", input.Source, input.SourceID))
	require.Nil(t, claim.sourceClaim)
	require.Empty(t, claim.ConflictingRoomID)
}

func TestCreateOperatorRoomConcurrentSourceClaim(t *testing.T) {
	first, nc := setupTestCore(t)
	ctx := testContext(t)
	second, err := NewChattoCore(ctx, nc, first.config)
	require.NoError(t, err)
	startCoreServices(t, second)

	input := OperatorRoomCreateInput{Name: "Concurrent import", Source: "discord", SourceID: "789"}
	type createResult struct {
		roomID         string
		groupID        string
		visibleGroupID string
		err            error
	}
	var wg sync.WaitGroup
	wg.Add(2)
	results := make([]*createResult, 2)
	for i, c := range []*ChattoCore{first, second} {
		i, c := i, c
		go func() {
			defer wg.Done()
			room, err := c.CreateOperatorRoom(ctx, input)
			result := &createResult{err: err}
			if room != nil {
				result.roomID = room.GetId()
				result.groupID = room.GetGroupId()
				visible, readErr := c.GetRoom(ctx, KindChannel, room.GetId())
				result.err = readErr
				if visible != nil {
					result.visibleGroupID = visible.GetGroupId()
				}
			}
			results[i] = result
		}()
	}
	wg.Wait()
	for _, result := range results {
		require.NoError(t, result.err)
		require.NotEmpty(t, result.roomID)
		require.NotEmpty(t, result.groupID)
		require.Equal(t, result.groupID, result.visibleGroupID)
	}
	require.Equal(t, results[0].roomID, results[1].roomID)
	events, _, err := first.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(results[0].roomID).Subject(evtstream.EventRoomCreated))
	require.NoError(t, err)
	require.Len(t, events, 1)
}

func TestCreateOperatorRoomConcurrentConflictingInput(t *testing.T) {
	first, nc := setupTestCore(t)
	ctx := testContext(t)
	second, err := NewChattoCore(ctx, nc, first.config)
	require.NoError(t, err)
	startCoreServices(t, second)

	inputs := []OperatorRoomCreateInput{
		{Name: "First claim", Source: "discord", SourceID: "same"},
		{Name: "Second claim", Source: "discord", SourceID: "same"},
	}
	type createResult struct {
		roomID string
		err    error
	}
	results := make([]createResult, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	for i, c := range []*ChattoCore{first, second} {
		i, c := i, c
		go func() {
			defer wg.Done()
			room, err := c.CreateOperatorRoom(ctx, inputs[i])
			results[i].err = err
			if room != nil {
				results[i].roomID = room.GetId()
			}
		}()
	}
	wg.Wait()
	successes, conflicts := 0, 0
	for _, result := range results {
		switch {
		case result.err == nil:
			successes++
			require.NotEmpty(t, result.roomID)
		case errors.Is(result.err, ErrRoomSourceConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent result: %v", result.err)
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
}

func roomIDs(rooms []*evtv1.Room) []string {
	ids := make([]string, 0, len(rooms))
	for _, room := range rooms {
		ids = append(ids, room.GetId())
	}
	return ids
}

package core

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"hmans.de/chatto/internal/evtstream"
)

func TestOperatorRoomMemberAddAcrossReplicasAndReplay(t *testing.T) {
	first, nc := setupTestCore(t)
	ctx := testContext(t)
	user, err := first.CreateUser(ctx, SystemActorID, "operator-replica-member", "Operator Replica Member", "password")
	require.NoError(t, err)
	room, err := first.CreateRoom(ctx, SystemActorID, KindChannel, "", "operator-replica-room", "")
	require.NoError(t, err)
	second, err := NewChattoCore(ctx, nc, first.config)
	require.NoError(t, err)
	startCoreServices(t, second)

	type result struct {
		roomID string
		userID string
		err    error
	}
	results := make([]result, 2)
	var wg sync.WaitGroup
	for i, c := range []*ChattoCore{first, second} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			membership, addErr := c.AddMember(ctx, SystemActorID, KindChannel, room.GetId(), user.GetId())
			results[i].err = addErr
			if membership != nil {
				results[i].roomID = membership.GetRoomId()
				results[i].userID = membership.GetUserId()
			}
		}()
	}
	wg.Wait()
	for _, result := range results {
		require.NoError(t, result.err)
		require.Equal(t, room.GetId(), result.roomID)
		require.Equal(t, user.GetId(), result.userID)
	}

	replayed, err := NewChattoCore(ctx, nc, first.config)
	require.NoError(t, err)
	startCoreServices(t, replayed)
	membership, err := replayed.AddMember(ctx, SystemActorID, KindChannel, room.GetId(), user.GetId())
	require.NoError(t, err)
	require.Equal(t, room.GetId(), membership.GetRoomId())
	require.Equal(t, user.GetId(), membership.GetUserId())
	for _, eventType := range []string{evtstream.EventRoomMemberAdded, evtstream.EventUserJoinedRoom} {
		events, _, err := first.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.GetId()).Subject(eventType))
		require.NoError(t, err)
		require.Len(t, events, 1, "event type %s", eventType)
	}
}

func TestAddMemberRechecksRoomAfterConcurrentArchive(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := c.CreateUser(ctx, SystemActorID, "operator-archive-member", "Operator Archive Member", "password")
	require.NoError(t, err)
	room, err := c.CreateRoom(ctx, SystemActorID, KindChannel, "", "operator-archive-race", "")
	require.NoError(t, err)

	checks := 0
	_, err = c.addMember(ctx, SystemActorID, KindChannel, room.GetId(), user.GetId(), func() error {
		checks++
		if checks == 2 {
			_, archiveErr := c.ArchiveRoom(ctx, SystemActorID, KindChannel, room.GetId())
			return archiveErr
		}
		return nil
	})
	require.ErrorIs(t, err, ErrRoomArchived)
	_, err = c.GetRoomMembership(ctx, KindChannel, user.GetId(), room.GetId())
	require.Error(t, err)
	for _, eventType := range []string{evtstream.EventRoomMemberAdded, evtstream.EventUserJoinedRoom} {
		events, _, eventErr := c.EventPublisher.SubjectEvents(ctx, evtstream.RoomAggregate(room.GetId()).Subject(eventType))
		require.NoError(t, eventErr)
		require.Empty(t, events, "event type %s", eventType)
	}
}

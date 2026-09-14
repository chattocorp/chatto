//go:build bootstrap || test_endpoints

package core

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSeedDataCreatesReadableReproducibleState(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	options := SeedOptions{Seed: 42, Users: 3, Rooms: 2, Messages: 12, ThreadReplies: 5}
	first, err := c.SeedData(ctx, options)
	require.NoError(t, err)
	require.Len(t, first.Users, 3)
	require.Len(t, first.Rooms, 2)
	require.Len(t, first.Messages, 12)
	require.Equal(t, SeedVersion, first.Version)
	for _, user := range first.Users {
		roles, err := c.GetUserRoles(ctx, user.ID)
		require.NoError(t, err)
		require.NotContains(t, roles, RoleOwner)
	}
	for _, room := range first.Rooms {
		for _, user := range first.Users {
			_, err := c.GetRoomMembership(ctx, KindChannel, user.ID, room.ID)
			require.NoError(t, err)
		}
	}
	replies := map[string]int{}
	for _, message := range first.Messages {
		body, err := c.GetMessageBody(ctx, message.ID)
		require.NoError(t, err)
		require.Equal(t, message.Body, body)
		if message.ThreadRootID != "" {
			replies[message.ThreadRootID]++
		}
	}
	for _, message := range first.Messages[:7] {
		if replies[message.ID] == 0 {
			continue
		}
		metadata, err := c.GetThreadMetadata(ctx, KindChannel, message.RoomID, message.ID)
		require.NoError(t, err)
		require.EqualValues(t, replies[message.ID], metadata.ReplyCount)
	}
	otherCore, _ := setupTestCore(t)
	second, err := otherCore.SeedData(ctx, options)
	require.NoError(t, err)
	// Normalize only generated identifiers. Persisted
	// message bodies and actual author/room/thread relationships must agree.
	for i := range first.Users {
		require.Equal(t, first.Users[i].DisplayName, second.Users[i].DisplayName)
		require.Equal(t, first.Users[i].Login, second.Users[i].Login)
	}
	for i := range first.Rooms {
		require.Equal(t, first.Rooms[i].Name, second.Rooms[i].Name)
	}
	ids := map[string]string{}
	for i, user := range first.Users {
		ids[user.ID] = second.Users[i].ID
	}
	for i, room := range first.Rooms {
		ids[room.ID] = second.Rooms[i].ID
	}
	for i, message := range first.Messages {
		ids[message.ID] = second.Messages[i].ID
	}
	for i, message := range first.Messages {
		other := second.Messages[i]
		require.Equal(t, message.Body, other.Body)
		require.Equal(t, ids[message.AuthorID], other.AuthorID)
		require.Equal(t, ids[message.RoomID], other.RoomID)
		require.Equal(t, ids[message.ThreadRootID], other.ThreadRootID)
	}
	again, err := c.SeedData(ctx, options)
	require.NoError(t, err)
	for i, user := range first.Users {
		require.NotEqual(t, user.ID, again.Users[i].ID)
		require.NotEqual(t, user.Login, again.Users[i].Login)
		require.Equal(t, user.DisplayName, again.Users[i].DisplayName)
	}
	for i, message := range first.Messages {
		require.Equal(t, message.Body, again.Messages[i].Body)
	}
	count, err := c.CountUsers(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 6, count)
}

func TestSeedDataConcurrentRunsAllocateDistinctNames(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	options := SeedOptions{Users: 2, Rooms: 1, Messages: 2}
	errors := make([]error, 2)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range errors {
		wg.Go(func() {
			<-start
			_, errors[i] = c.SeedData(ctx, options)
		})
	}
	close(start)
	wg.Wait()
	for _, err := range errors {
		require.NoError(t, err)
	}
	count, err := c.CountUsers(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 4, count)
	rooms, err := c.ListRooms(ctx, KindChannel)
	require.NoError(t, err)
	require.Len(t, rooms, 2)
	require.NotEqual(t, rooms[0].Name, rooms[1].Name)
}

func TestSeedDataRejectsInvalidOptionsBeforeWriting(t *testing.T) {
	c, _ := setupTestCore(t)
	for _, options := range []SeedOptions{
		{Users: 0, Rooms: 1},
		{Users: 1, Rooms: 0},
		{Users: 5000, Rooms: 100},
		{Users: 1, Rooms: 1, Messages: -1},
		{Users: 1, Rooms: 1, Messages: 1, ThreadReplies: 1},
	} {
		_, err := c.SeedData(testContext(t), options)
		require.ErrorIs(t, err, ErrInvalidArgument)
	}
	count, err := c.CountUsers(testContext(t))
	require.NoError(t, err)
	require.Zero(t, count)
}

func TestSeedDataAcceptsEmptyHistoryAndCancellation(t *testing.T) {
	c, _ := setupTestCore(t)
	options := SeedOptions{Users: 1, Rooms: 1}
	result, err := c.SeedData(testContext(t), options)
	require.NoError(t, err)
	require.Empty(t, result.Messages)
	ctx, cancel := context.WithCancel(testContext(t))
	cancel()
	_, err = c.SeedData(ctx, options)
	require.ErrorIs(t, err, context.Canceled)
}

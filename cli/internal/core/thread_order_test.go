package core

import (
	"context"
	"slices"
	"testing"
	"time"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/stretchr/testify/require"
)

type countingThreadHydration struct {
	threadFollowIntegration
	reads int
}

func (c *countingThreadHydration) GetRoomEventByEventID(ctx context.Context, kind RoomKind, roomID, eventID string) (*evtv1.Event, error) {
	c.reads++
	return c.threadFollowIntegration.GetRoomEventByEventID(ctx, kind, roomID, eventID)
}

func TestThreadCreationOrderDoesNotHydrateUnpagedRoots(t *testing.T) {
	t.Parallel()
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	user, err := c.CreateUser(ctx, SystemActorID, "thread-sort", "Thread Sort", "password")
	require.NoError(t, err)
	room, err := c.CreateRoom(ctx, user.Id, KindChannel, "", "thread-sort", "")
	require.NoError(t, err)
	_, err = c.JoinRoom(ctx, user.Id, KindChannel, user.Id, room.Id)
	require.NoError(t, err)
	for range 3 {
		root, err := c.PostMessage(ctx, KindChannel, room.Id, user.Id, "root", nil, "", "", nil, false)
		require.NoError(t, err)
		require.NoError(t, c.FollowThread(ctx, KindChannel, user.Id, room.Id, root.Id))
	}
	hydration := &countingThreadHydration{threadFollowIntegration: c.threadFollows.integration}
	c.threadFollows.integration = hydration
	page, err := c.threadFollows.ListFollowedThreads(ctx, user.Id, false, false, 1, 0, ThreadRootAuthorAll, ThreadOrderCreated)
	require.NoError(t, err)
	require.Len(t, page.Threads, 1)
	require.Equal(t, 3, page.TotalCount)
	require.NotNil(t, page.Threads[0].RootCreatedAt)
	require.Zero(t, hydration.reads, "sorting must use projected dates without loading message bodies")
}

func TestThreadOrderDates(t *testing.T) {
	old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	recent := old.Add(time.Hour)
	threads := []*FollowedThread{
		{ThreadRootEventID: "unknown"},
		{ThreadRootEventID: "b", RootCreatedAt: &recent, ActivityAt: &old},
		{ThreadRootEventID: "old", RootCreatedAt: &old, ActivityAt: &recent},
		{ThreadRootEventID: "a", RootCreatedAt: &recent, ActivityAt: &old},
	}
	for _, test := range []struct {
		order ThreadOrder
		ids   []string
	}{
		{ThreadOrderCreated, []string{"a", "b", "old", "unknown"}},
		{ThreadOrderActivity, []string{"old", "a", "b", "unknown"}},
		{ThreadOrderDefault, []string{"old", "a", "b", "unknown"}},
	} {
		slices.SortFunc(threads, func(a, b *FollowedThread) int { return compareThreadDates(a, b, test.order) })
		var ids []string
		for _, thread := range threads {
			ids = append(ids, thread.ThreadRootEventID)
		}
		require.Equal(t, test.ids, ids)
	}
}

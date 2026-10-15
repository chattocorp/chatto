package core

import (
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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

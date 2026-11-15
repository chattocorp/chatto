package core

import (
	"sync"
	"testing"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/stretchr/testify/require"
)

func TestReleaseHighlightsConcurrentAndIdempotent(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	const user = "highlight-reader"
	initial, err := c.InitializeReleaseHighlights(ctx, user, "users", "0.5.0")
	require.NoError(t, err)
	require.Equal(t, "0.5.0", initial.GetHighlightsUsersSinceVersion())
	require.Empty(t, initial.GetDismissedReleaseHighlights())
	changed, err := c.InitializeReleaseHighlights(ctx, user, "users", "0.6.0")
	require.NoError(t, err)
	require.Equal(t, "0.5.0", changed.GetHighlightsUsersSinceVersion())
	_, err = c.InitializeReleaseHighlights(ctx, user, "admins", "0.6.0")
	require.NoError(t, err)

	start := make(chan struct{})
	failures := make(chan error, 2)
	var workers sync.WaitGroup
	for _, id := range []string{"users:help", "admins:bots"} {
		workers.Go(func() { <-start; _, err := c.DismissReleaseHighlights(ctx, user, []string{id}); failures <- err })
	}
	close(start)
	workers.Wait()
	close(failures)
	for err := range failures {
		require.NoError(t, err)
	}
	settings, err := c.DismissReleaseHighlights(ctx, user, []string{"users:help", "users:help"})
	require.NoError(t, err)
	require.Equal(t, []string{"admins:bots", "users:help"}, settings.GetDismissedReleaseHighlights())
	require.Equal(t, "0.5.0", settings.GetHighlightsUsersSinceVersion())
	require.Equal(t, "0.6.0", settings.GetHighlightsAdminsSinceVersion())
	other, err := c.GetUserSettings(ctx, "other-reader")
	require.NoError(t, err)
	require.Nil(t, other)

	// Snapshot restore must equal the live projection, including both baselines.
	data, err := c.configModel.config.Projection().Snapshot()
	require.NoError(t, err)
	restored := NewConfigProjection()
	require.NoError(t, restored.Restore(data))
	row := restored.users[user]
	require.Equal(t, []string{"admins:bots", "users:help"}, sortedMapKeys(row.dismissedReleaseHighlights))
	require.Equal(t, "0.5.0", *row.highlightsUsersSinceVersion)
	require.Equal(t, "0.6.0", *row.highlightsAdminsSinceVersion)
	require.NoError(t, restored.Apply(&evtv1.Event{Event: &evtv1.Event_UserAccountDeleted{UserAccountDeleted: &evtv1.UserAccountDeletedEvent{UserId: user}}}, 1))
	require.NotContains(t, restored.users, user)
}

func TestReleaseHighlightsValidateBeforeWrite(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	for _, ids := range [][]string{nil, {"unknown:help"}, {"users:../help"}, {"users:"}, make([]string, 101)} {
		_, err := c.DismissReleaseHighlights(ctx, "reader", ids)
		require.ErrorIs(t, err, ErrInvalidArgument)
	}
	for _, version := range []string{"", "0.5", "v0.5.0", "not-a-version"} {
		_, err := c.InitializeReleaseHighlights(ctx, "reader", "users", version)
		require.ErrorIs(t, err, ErrInvalidArgument)
	}
	_, err := c.InitializeReleaseHighlights(ctx, "reader", "unknown", "0.5.0")
	require.ErrorIs(t, err, ErrInvalidArgument)
	settings, err := c.GetUserSettings(ctx, "reader")
	require.NoError(t, err)
	require.Nil(t, settings)
}

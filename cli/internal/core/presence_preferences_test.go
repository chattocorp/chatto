package core

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

func TestPresencePreferenceOverridesEveryDevice(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := context.Background()
	p, err := c.SetPresencePreference(ctx, "user", apiv1.PresenceMode_PRESENCE_MODE_INVISIBLE, "")
	require.NoError(t, err)
	for _, explicit := range []bool{false, true} {
		require.NoError(t, c.SetPresenceWithOptions(ctx, "user", PresenceStatusOnline, explicit))
		status, err := c.GetUserPresence(ctx, "user")
		require.NoError(t, err)
		require.Equal(t, PresenceStatusOffline, status)
	}
	allowed, err := c.MayPublishTyping(ctx, "user")
	require.NoError(t, err)
	require.False(t, allowed)
	_, err = c.SetPresencePreference(ctx, "user", apiv1.PresenceMode_PRESENCE_MODE_ONLINE, "")
	require.ErrorIs(t, err, events.ErrConflict)
	p, err = c.SetPresencePreference(ctx, "user", apiv1.PresenceMode_PRESENCE_MODE_DO_NOT_DISTURB, p.Revision)
	require.NoError(t, err)
	require.NoError(t, c.SetPresenceWithOptions(ctx, "user", PresenceStatusOnline, true))
	status, err := c.GetUserPresence(ctx, "user")
	require.NoError(t, err)
	require.Equal(t, PresenceStatusDoNotDisturb, status)
	loaded, err := c.GetPresencePreference(ctx, "user")
	require.NoError(t, err)
	require.Equal(t, p, loaded)
}

func TestInvisiblePresenceHasNoPublicRefreshOrExpiryEvents(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := c.SetPresencePreference(ctx, "user", apiv1.PresenceMode_PRESENCE_MODE_INVISIBLE, "")
	require.NoError(t, err)
	sub, err := c.presenceModel.Subscribe(ctx)
	require.NoError(t, err)
	defer c.presenceModel.Unsubscribe(sub)
	require.NoError(t, c.SetPresence(ctx, "user", PresenceStatusOnline))
	require.NoError(t, c.refreshPresence(ctx, "user"))
	require.NoError(t, c.presenceModel.memoryCacheKV.Delete(ctx, presenceKey("user")))
	// Resync is a watcher barrier after the writes; no invisible transition may
	// have reached a public subscriber during the complete live lifecycle.
	require.NoError(t, c.presenceModel.Resync(ctx))
	select {
	case update := <-sub.C:
		t.Fatalf("invisible activity leaked: %#v", update)
	default:
	}
	count, err := c.LivePresenceCount(ctx)
	require.NoError(t, err)
	require.Zero(t, count)
	p, err := c.GetPresencePreference(ctx, "user")
	require.NoError(t, err)
	require.Equal(t, apiv1.PresenceMode_PRESENCE_MODE_INVISIBLE, p.Mode)
}

func TestPresencePreferenceReplayAndAccountDeletion(t *testing.T) {
	h := NewPresenceHub(nil, nil)
	h.live["user"] = PresenceStatusOnline
	event := &evtv1.Event{Id: "choice", Event: &evtv1.Event_UserPresencePreferenceChanged{UserPresencePreferenceChanged: &evtv1.UserPresencePreferenceChangedEvent{UserId: "user", Mode: evtv1.SavedPresenceMode_SAVED_PRESENCE_MODE_INVISIBLE}}}
	require.NoError(t, h.Apply(event, 1))
	require.Empty(t, h.snapshot)
	require.Equal(t, "choice", h.preference("user").Revision)
	require.NoError(t, h.Apply(&evtv1.Event{Event: &evtv1.Event_UserAccountDeleted{UserAccountDeleted: &evtv1.UserAccountDeletedEvent{UserId: "user"}}}, 2))
	require.Nil(t, h.preference("user"))
	require.Empty(t, h.live)
}

func TestPresencePreferenceSharedAcrossReplicasAndRestart(t *testing.T) {
	c, nc := setupTestCore(t)
	ctx := testContext(t)
	p, err := c.SetPresencePreference(ctx, "user", apiv1.PresenceMode_PRESENCE_MODE_INVISIBLE, "")
	require.NoError(t, err)
	replica, err := NewChattoCore(ctx, nc, c.config)
	require.NoError(t, err)
	startCoreServices(t, replica)
	loaded, err := replica.GetPresencePreference(ctx, "user")
	require.NoError(t, err)
	require.Equal(t, p, loaded)
	require.NoError(t, replica.SetPresenceWithOptions(ctx, "user", PresenceStatusOnline, true))
	for _, instance := range []*ChattoCore{c, replica} {
		status, err := instance.GetUserPresence(ctx, "user")
		require.NoError(t, err)
		require.Equal(t, PresenceStatusOffline, status)
	}
	p, err = replica.SetPresencePreference(ctx, "user", apiv1.PresenceMode_PRESENCE_MODE_DO_NOT_DISTURB, p.Revision)
	require.NoError(t, err)
	loaded, err = c.GetPresencePreference(ctx, "user")
	require.NoError(t, err)
	require.Equal(t, p, loaded)
	require.NoError(t, c.presenceModel.memoryCacheKV.Delete(ctx, presenceKey("user")))
	public, err := c.GetUserPresence(ctx, "user")
	require.NoError(t, err)
	require.Equal(t, PresenceStatusOffline, public)
	policy, err := c.notificationPresence(ctx, "user")
	require.NoError(t, err)
	require.Equal(t, PresenceStatusDoNotDisturb, policy)
}

func TestPresenceWatcherWaitsForPrivateChoiceBeforeExposingLiveness(t *testing.T) {
	s, _, _ := newTestPresenceModel(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, s.SetPresence(ctx, "user", PresenceStatusOnline))
	entered := make(chan struct{})
	release := make(chan struct{})
	s.hub.beforeLiveRead = func(ctx context.Context, userID string) error {
		close(entered)
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	t.Cleanup(func() { cancel(); <-done })
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("liveness bypassed the private-choice readiness barrier")
	}
	// Simulate the lagging replica catching up to a committed invisible choice.
	require.NoError(t, s.hub.Apply(&evtv1.Event{Id: "saved", Event: &evtv1.Event_UserPresencePreferenceChanged{UserPresencePreferenceChanged: &evtv1.UserPresencePreferenceChangedEvent{UserId: "user", Mode: evtv1.SavedPresenceMode_SAVED_PRESENCE_MODE_INVISIBLE}}}, 1))
	close(release)
	statuses, err := s.GetUserPresences(ctx, []string{"user"})
	require.NoError(t, err)
	require.Equal(t, PresenceStatusOffline, statuses["user"])
}

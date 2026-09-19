package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	pubsubv1 "hmans.de/chatto/internal/pb/chatto/core/pubsub/v1"
	runtimestatev1 "hmans.de/chatto/internal/pb/chatto/core/runtime_state/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
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

func TestPresencePreferenceRejectsDelayedReadsAndClearsDeletedState(t *testing.T) {
	h := NewPresenceHub(nil, nil, nil)
	h.live["user"] = PresenceStatusOnline
	h.applyPreference("user", &apiv1.PresencePreference{Mode: apiv1.PresenceMode_PRESENCE_MODE_INVISIBLE, Revision: "choice"}, 2)
	h.applyPreference("user", &apiv1.PresencePreference{Mode: apiv1.PresenceMode_PRESENCE_MODE_ONLINE, Revision: "old"}, 1)
	require.Empty(t, h.snapshot)
	require.Equal(t, "choice", h.preference("user").Revision)
	h.applyPreference("user", nil, 3)
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
	s.hub.applyPreference("user", &apiv1.PresencePreference{Mode: apiv1.PresenceMode_PRESENCE_MODE_INVISIBLE, Revision: "saved"}, 1)
	close(release)
	statuses, err := s.GetUserPresences(ctx, []string{"user"})
	require.NoError(t, err)
	require.Equal(t, PresenceStatusOffline, statuses["user"])
}

func TestPresenceChoicesReplaceRuntimeStateWithoutWritingEVT(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	evt, err := c.js.Stream(ctx, "EVT")
	require.NoError(t, err)
	before, err := evt.Info(ctx)
	require.NoError(t, err)
	var revision string
	for _, mode := range []apiv1.PresenceMode{apiv1.PresenceMode_PRESENCE_MODE_INVISIBLE, apiv1.PresenceMode_PRESENCE_MODE_ONLINE, apiv1.PresenceMode_PRESENCE_MODE_AWAY, apiv1.PresenceMode_PRESENCE_MODE_DO_NOT_DISTURB} {
		choice, err := c.SetPresencePreference(ctx, "user", mode, revision)
		require.NoError(t, err)
		revision = choice.Revision
		require.NoError(t, c.SetPresence(ctx, "user", PresenceStatusOnline))
	}
	after, err := evt.Info(ctx)
	require.NoError(t, err)
	require.Equal(t, before.State.LastSeq, after.State.LastSeq, "presence must never append to EVT")
	history, err := c.storage.runtimeStateKV.History(ctx, presenceKey("user"))
	require.NoError(t, err)
	require.Len(t, history, 1, "presence must retain only its current value")
	var stored runtimestatev1.PresencePreference
	require.NoError(t, proto.Unmarshal(history[0].Value(), &stored))
	require.Equal(t, runtimestatev1.PresenceMode_PRESENCE_MODE_DO_NOT_DISTURB, stored.Mode)
	stream, err := c.js.Stream(ctx, "KV_RUNTIME_STATE")
	require.NoError(t, err)
	info, err := stream.Info(ctx)
	require.NoError(t, err)
	require.Equal(t, jetstream.FileStorage, info.Config.Storage)
	require.EqualValues(t, 1, info.Config.MaxMsgsPerSubject)
}

func TestPresencePreferenceSignalIsPrivateAndTransient(t *testing.T) {
	c, _ := setupTestCore(t)
	event := newPubSubEvent("owner", &pubsubv1.PubSubEvent{Event: &pubsubv1.PubSubEvent_ViewerPresencePreferenceChanged{ViewerPresencePreferenceChanged: &realtimev1.ViewerPresencePreferenceChangedEvent{}}})
	subject, err := userPubSubEventPublication("owner", event).subject()
	require.NoError(t, err)
	require.Equal(t, "live.sync.user.owner.presence_preference", subject)
	for _, viewer := range []string{"owner", "other"} {
		_, allowed := c.filterPubSubEvent(testContext(t), viewer, nil, &nats.Msg{Subject: subject}, event)
		require.Equal(t, viewer == "owner", allowed)
	}
}

func TestPresencePreferenceConcurrentReplacementAcrossReplicas(t *testing.T) {
	c, nc := setupTestCore(t)
	ctx := testContext(t)
	replica, err := NewChattoCore(ctx, nc, c.config)
	require.NoError(t, err)
	startCoreServices(t, replica)
	initial, err := c.SetPresencePreference(ctx, "user", apiv1.PresenceMode_PRESENCE_MODE_ONLINE, "")
	require.NoError(t, err)
	start := make(chan struct{})
	results := make(chan error, 2)
	for index, instance := range []*ChattoCore{c, replica} {
		go func() {
			<-start
			mode := apiv1.PresenceMode_PRESENCE_MODE_INVISIBLE
			if index == 1 {
				mode = apiv1.PresenceMode_PRESENCE_MODE_DO_NOT_DISTURB
			}
			_, err := instance.SetPresencePreference(ctx, "user", mode, initial.Revision)
			results <- err
		}()
	}
	close(start)
	var successes, conflicts int
	for range 2 {
		err := <-results
		if err == nil {
			successes++
		} else if errors.Is(err, events.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	require.Equal(t, 1, successes)
	require.Equal(t, 1, conflicts)
	first, err := c.GetPresencePreference(ctx, "user")
	require.NoError(t, err)
	second, err := replica.GetPresencePreference(ctx, "user")
	require.NoError(t, err)
	require.Equal(t, first, second)
}

func TestPresencePreferenceKVDeletionClearsWatcherAndSurvivesResync(t *testing.T) {
	c, _ := setupTestCore(t)
	ctx := testContext(t)
	_, err := c.SetPresencePreference(ctx, "user", apiv1.PresenceMode_PRESENCE_MODE_INVISIBLE, "")
	require.NoError(t, err)
	require.NoError(t, c.presenceModel.forgetUser(ctx, "user"))
	require.NoError(t, c.presenceModel.waitPreferencesCurrent(ctx))
	require.Nil(t, c.presenceModel.hub.preference("user"))
	require.NoError(t, c.presenceModel.Resync(ctx))
	choice, err := c.GetPresencePreference(ctx, "user")
	require.NoError(t, err)
	require.Nil(t, choice)
}

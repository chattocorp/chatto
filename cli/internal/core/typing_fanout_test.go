package core

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	pubsubv1 "hmans.de/chatto/internal/pb/chatto/core/pubsub/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

// filterPubSubEvent applies every live sync delivery rule for one recipient,
// as MyEventsHub does for each event.
func (c *ChattoCore) filterPubSubEvent(ctx context.Context, userID string, memberRooms map[string]struct{}, msg *nats.Msg, event *pubsubv1.PubSubEvent) (EventEnvelope, bool) {
	s := c.myEventsModel
	delivery, ok := s.preparePubSubEvent(msg, event)
	if !ok {
		return nil, false
	}
	if delivery.typing() && !s.typingSenderVisible(ctx, delivery) {
		return nil, false
	}
	return s.filterPreparedPubSubEvent(ctx, userID, memberRooms, delivery)
}

// typingLookupSentinelSubject shares the counting subscription with the
// preference reads, but no stream has this name.
const typingLookupSentinelSubject = "$JS.API.STREAM.MSG.GET.CHATTO_TEST_SENTINEL"

type typingFanoutFixture struct {
	core    *ChattoCore
	nc      *nats.Conn
	room    string
	author  string
	viewers []string
	streams []<-chan EventEnvelope
	lookups map[string]*atomic.Int64
	marks   chan struct{}
}

// newTypingFanoutFixture streams live events for viewerCount room members and
// counts authoritative presence-preference reads per user.
func newTypingFanoutFixture(t *testing.T, viewerCount int) *typingFanoutFixture {
	t.Helper()
	c, nc := setupTestCore(t)
	ctx := testContext(t)
	author, err := c.CreateUser(ctx, SystemActorID, "typing-author", "Typing Author", "password123")
	require.NoError(t, err)
	room, err := c.CreateRoom(ctx, author.Id, KindChannel, "", "typing-room", "")
	require.NoError(t, err)
	_, err = c.JoinRoom(ctx, author.Id, KindChannel, author.Id, room.Id)
	require.NoError(t, err)

	f := &typingFanoutFixture{core: c, nc: nc, room: room.Id, author: author.Id, lookups: make(map[string]*atomic.Int64), marks: make(chan struct{}, 1)}
	for i := range viewerCount {
		viewer, err := c.CreateUser(ctx, SystemActorID, "typing-viewer-"+string(rune('a'+i)), "Typing Viewer", "password123")
		require.NoError(t, err)
		_, err = c.JoinRoom(ctx, viewer.Id, KindChannel, viewer.Id, room.Id)
		require.NoError(t, err)
		f.viewers = append(f.viewers, viewer.Id)
	}
	for _, userID := range append([]string{author.Id}, f.viewers...) {
		f.lookups[userID] = new(atomic.Int64)
	}
	// MayPublishTyping reads the leader-routed KV record through the stream
	// message-get API. Count those requests for each user.
	sub, err := nc.Subscribe("$JS.API.STREAM.MSG.GET.*", func(msg *nats.Msg) {
		if msg.Subject == typingLookupSentinelSubject {
			f.marks <- struct{}{}
			return
		}
		for userID, count := range f.lookups {
			if strings.Contains(string(msg.Data), `"$KV.RUNTIME_STATE.`+presenceKey(userID)+`"`) {
				count.Add(1)
			}
		}
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	require.NoError(t, nc.Flush())

	select {
	case <-c.myEventsModel.hub.ready:
	case <-ctx.Done():
		t.Fatal("myEvents hub did not become ready")
	}
	for _, viewerID := range f.viewers {
		stream, err := c.StreamMyEventsWithOptions(ctx, viewerID, StreamMyEventsOptions{})
		require.NoError(t, err)
		f.streams = append(f.streams, stream)
	}
	return f
}

// lookupCount returns the preference reads for userID that the hub sent before
// this call. The core shares nc, and one subscription receives its messages in
// order, so the callback has counted every earlier read once it sees the
// sentinel.
func (f *typingFanoutFixture) lookupCount(t *testing.T, userID string) int64 {
	t.Helper()
	require.NoError(t, f.nc.Publish(typingLookupSentinelSubject, nil))
	select {
	case <-f.marks:
	case <-time.After(2 * time.Second):
		t.Fatal("lookup sentinel was not delivered")
	}
	return f.lookups[userID].Load()
}

// publishTyping publishes the pubsub fact directly, so that only the hub reads
// the sender's preference.
func (f *typingFanoutFixture) publishTyping(t *testing.T, actorID string) string {
	t.Helper()
	event := newPubSubEvent(actorID, &pubsubv1.PubSubEvent{Event: &pubsubv1.PubSubEvent_UserTyping{
		UserTyping: &realtimev1.UserTypingEvent{RoomId: f.room},
	}})
	require.NoError(t, f.core.publishRoomPubSubEvent(testContext(t), KindChannel, f.room, event))
	return event.GetId()
}

// nextTyping returns the ID of the next typing event on stream.
func nextTyping(t *testing.T, stream <-chan EventEnvelope) string {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case envelope, ok := <-stream:
			require.True(t, ok, "event stream closed")
			if envelope.PubSubEvent().GetUserTyping() != nil {
				return envelope.ID()
			}
		case <-timer.C:
			t.Fatal("typing event was not delivered")
		}
	}
}

func TestMyEventsHubChecksTypingPrivacyOncePerEvent(t *testing.T) {
	f := newTypingFanoutFixture(t, 3)

	eventID := f.publishTyping(t, f.author)
	for _, stream := range f.streams {
		require.Equal(t, eventID, nextTyping(t, stream))
	}
	require.EqualValues(t, 1, f.lookupCount(t, f.author), "one privacy read per typing event, not one per recipient")
}

func TestMyEventsHubDropsTypingFromHiddenSender(t *testing.T) {
	f := newTypingFanoutFixture(t, 2)
	_, err := f.core.SetPresencePreference(testContext(t), f.author, apiv1.PresenceStatus_PRESENCE_STATUS_OFFLINE, "")
	require.NoError(t, err)

	f.publishTyping(t, f.author)
	// The hub handles live sync events in order. The first typing event that
	// the other viewer receives must therefore be this marker.
	marker := f.publishTyping(t, f.viewers[1])
	require.Equal(t, marker, nextTyping(t, f.streams[0]))
}

func TestMyEventsHubSkipsTypingPrivacyReadWithoutAudience(t *testing.T) {
	f := newTypingFanoutFixture(t, 1)
	err := f.core.RoomCommands().LeaveRoom(testContext(t), RoomIDInput{ActorID: f.viewers[0], RoomID: f.room})
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		f.core.myEventsModel.hub.mu.Lock()
		defer f.core.myEventsModel.hub.mu.Unlock()
		state := f.core.myEventsModel.hub.users[f.viewers[0]]
		if state == nil {
			return false
		}
		_, member := state.memberRooms[f.room]
		return !member
	}, 2*time.Second, 10*time.Millisecond)

	f.publishTyping(t, f.author)
	// Nobody on this replica is a member of the room, so the hub must not read
	// the sender's preference. A user-scoped event acts as an ordering marker.
	marker := newPubSubEvent(f.viewers[0], &pubsubv1.PubSubEvent{Event: &pubsubv1.PubSubEvent_ViewerPresencePreferenceChanged{
		ViewerPresencePreferenceChanged: &realtimev1.ViewerPresencePreferenceChangedEvent{},
	}})
	require.NoError(t, f.core.publishUserPubSubEvent(testContext(t), f.viewers[0], marker))
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case envelope := <-f.streams[0]:
			if envelope.ID() == marker.GetId() {
				require.Zero(t, f.lookupCount(t, f.author))
				return
			}
		case <-timer.C:
			t.Fatal("marker event was not delivered")
		}
	}
}

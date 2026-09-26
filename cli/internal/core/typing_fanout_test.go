package core

import (
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

// newTypingFanoutFixture streams live events for members room members and for
// outsiders users who never join the room. It counts authoritative
// presence-preference reads per user.
func newTypingFanoutFixture(t *testing.T, members, outsiders int) *typingFanoutFixture {
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
	for i := range members + outsiders {
		viewer, err := c.CreateUser(ctx, SystemActorID, "typing-viewer-"+string(rune('a'+i)), "Typing Viewer", "password123")
		require.NoError(t, err)
		if i < members {
			_, err = c.JoinRoom(ctx, viewer.Id, KindChannel, viewer.Id, room.Id)
			require.NoError(t, err)
		}
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

// nextEvent returns the ID of the next event on stream that match accepts.
func nextEvent(t *testing.T, stream <-chan EventEnvelope, match func(EventEnvelope) bool) string {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case envelope, ok := <-stream:
			require.True(t, ok, "event stream closed")
			if match(envelope) {
				return envelope.ID()
			}
		case <-timer.C:
			t.Fatal("expected event was not delivered")
		}
	}
}

// nextTyping returns the ID of the next typing event on stream.
func nextTyping(t *testing.T, stream <-chan EventEnvelope) string {
	t.Helper()
	return nextEvent(t, stream, func(envelope EventEnvelope) bool {
		return envelope.PubSubEvent().GetUserTyping() != nil
	})
}

func TestMyEventsHubChecksTypingPrivacyOncePerEvent(t *testing.T) {
	f := newTypingFanoutFixture(t, 3, 0)

	eventID := f.publishTyping(t, f.author)
	for _, stream := range f.streams {
		require.Equal(t, eventID, nextTyping(t, stream))
	}
	require.EqualValues(t, 1, f.lookupCount(t, f.author), "one privacy read per typing event, not one per recipient")
}

func TestMyEventsHubDropsTypingFromHiddenSender(t *testing.T) {
	f := newTypingFanoutFixture(t, 2, 0)
	_, err := f.core.SetPresencePreference(testContext(t), f.author, apiv1.PresenceStatus_PRESENCE_STATUS_OFFLINE, "")
	require.NoError(t, err)

	f.publishTyping(t, f.author)
	// The hub handles live sync events in order. The first typing event that
	// the other viewer receives must therefore be this marker.
	marker := f.publishTyping(t, f.viewers[1])
	require.Equal(t, marker, nextTyping(t, f.streams[0]))
}

func TestMyEventsHubSkipsTypingPrivacyReadWithoutAudience(t *testing.T) {
	f := newTypingFanoutFixture(t, 0, 1)
	outsider := f.viewers[0]

	f.publishTyping(t, f.author)
	// Nobody on this process is a member of the room, so the hub must not read
	// the sender's preference. A user-scoped event acts as an ordering marker.
	marker := newPubSubEvent(outsider, &pubsubv1.PubSubEvent{Event: &pubsubv1.PubSubEvent_ViewerPresencePreferenceChanged{
		ViewerPresencePreferenceChanged: &realtimev1.ViewerPresencePreferenceChangedEvent{},
	}})
	require.NoError(t, f.core.publishUserPubSubEvent(testContext(t), outsider, marker))
	require.Equal(t, marker.GetId(), nextEvent(t, f.streams[0], func(envelope EventEnvelope) bool {
		return envelope.ID() == marker.GetId()
	}))
	require.Zero(t, f.lookupCount(t, f.author))
}

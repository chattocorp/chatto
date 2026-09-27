package core

import (
	"context"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/stretchr/testify/require"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	notificationv1 "hmans.de/chatto/internal/pb/chatto/core/notification/v1"
)

// streamSequenceReadSentinelSubject shares the counting subscription with the
// stream reads, but no stream has this name.
const streamSequenceReadSentinelSubject = "$JS.API.STREAM.MSG.GET.CHATTO_TEST_SENTINEL"

// TestNotificationVisibleOccurrencesReadsStreamOncePerBatch proves that the
// visibility filter does not load one event-stream message per occurrence.
// Long notification lists made ListNotificationOccurrences latency grow with
// one stream round trip per retained occurrence.
func TestNotificationVisibleOccurrencesReadsStreamOncePerBatch(t *testing.T) {
	chattoCore, nc := setupTestCore(t)
	reader, _, occurrences := setupNotificationVisibilityFixture(t, chattoCore, 20)

	// Let background notification work settle before the reads are counted.
	require.NoError(t, chattoCore.NotificationOccurrences().WaitCurrent(testContext(t)))

	var sequenceReads atomic.Int64
	marks := make(chan struct{}, 1)
	// One subscription runs its handler in delivery order, so the sentinel is
	// handled after every earlier request. Other workers read other streams in
	// the background. Only EVT reads can come from the visibility filter.
	sub, err := nc.Subscribe("$JS.API.>", func(msg *nats.Msg) {
		switch msg.Subject {
		case streamSequenceReadSentinelSubject:
			marks <- struct{}{}
		case "$JS.API.STREAM.MSG.GET.EVT", "$JS.API.DIRECT.GET.EVT":
			var request struct {
				Seq uint64 `json:"seq"`
			}
			if json.Unmarshal(msg.Data, &request) == nil && request.Seq != 0 {
				sequenceReads.Add(1)
			}
		}
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	require.NoError(t, nc.Flush())
	readsSoFar := func() int64 {
		t.Helper()
		require.NoError(t, nc.Publish(streamSequenceReadSentinelSubject, nil))
		select {
		case <-marks:
		case <-time.After(2 * time.Second):
			t.Fatal("stream read sentinel was not delivered")
		}
		return sequenceReads.Load()
	}
	readsFor := func(batch []*notificationv1.NotificationOccurrence) int64 {
		t.Helper()
		before := readsSoFar()
		visible, err := chattoCore.NotificationOccurrences().VisibleOccurrences(testContext(t), reader, batch)
		require.NoError(t, err)
		require.Len(t, visible, len(batch))
		return readsSoFar() - before
	}

	single := readsFor(occurrences[:1])
	all := readsFor(occurrences)
	require.LessOrEqual(t, all, single+1, "visibility filter read the event stream once per occurrence")
}

// TestNotificationVisibleOccurrencesFiltersMixedBatchInOrder checks a batch
// that spans more than one content-view barrier chunk. It must omit targets in
// a room that the reader left and deleted targets, and keep the input order.
func TestNotificationVisibleOccurrencesFiltersMixedBatchInOrder(t *testing.T) {
	chattoCore, _ := setupTestCore(t)
	ctx := testContext(t)
	reader, poster, occurrences := setupNotificationVisibilityFixture(t, chattoCore, notificationVisibilityChunkSize+44)
	leftRoom := NotificationOccurrenceMessageReference(occurrences[0]).GetRoomId()
	require.NoError(t, chattoCore.LeaveRoom(ctx, reader, KindChannel, reader, leftRoom))

	want := make([]string, 0, len(occurrences))
	for index, occurrence := range occurrences {
		message := NotificationOccurrenceMessageReference(occurrence)
		switch {
		case message.GetRoomId() == leftRoom:
		case index%7 == 1:
			require.NoError(t, chattoCore.DeleteMessage(ctx, poster, KindChannel, message.GetRoomId(), message.GetEventId()))
		default:
			want = append(want, occurrence.GetId())
		}
	}

	visible, err := chattoCore.NotificationOccurrences().VisibleOccurrences(ctx, reader, occurrences)
	require.NoError(t, err)
	got := make([]string, 0, len(visible))
	for _, occurrence := range visible {
		got = append(got, occurrence.GetId())
	}
	require.Equal(t, want, got)
}

// BenchmarkNotificationVisibleOccurrences measures the visibility filter that
// ListNotificationOccurrences applies to a recipient's complete occurrence
// list before it pages the result.
func BenchmarkNotificationVisibleOccurrences(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprintf("occurrences_%d", count), func(b *testing.B) {
			chattoCore := setupTestCoreWithEncryption(b)
			reader, _, occurrences := setupNotificationVisibilityFixture(b, chattoCore, count)
			ctx := context.Background()
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				visible, err := chattoCore.NotificationOccurrences().VisibleOccurrences(ctx, reader, occurrences)
				require.NoError(b, err)
				require.Len(b, visible, count)
			}
		})
	}
}

// setupNotificationVisibilityFixture creates count visible occurrences for one
// reader. Their targets are spread over several rooms. It returns the reader
// ID, the poster ID, and the occurrences in list order.
func setupNotificationVisibilityFixture(tb testing.TB, chattoCore *ChattoCore, count int) (string, string, []*notificationv1.NotificationOccurrence) {
	tb.Helper()
	ctx := context.Background()
	poster, err := chattoCore.CreateUser(ctx, SystemActorID, "visibility-poster", "Visibility Poster", "password")
	require.NoError(tb, err)
	reader, err := chattoCore.CreateUser(ctx, SystemActorID, "visibility-reader", "Visibility Reader", "password")
	require.NoError(tb, err)

	const roomCount = 10
	rooms := make([]string, 0, roomCount)
	for i := 0; i < roomCount; i++ {
		room, err := chattoCore.CreateRoom(ctx, poster.Id, KindChannel, "", fmt.Sprintf("visibility-%d", i), "")
		require.NoError(tb, err)
		for _, userID := range []string{poster.Id, reader.Id} {
			_, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id)
			require.NoError(tb, err)
		}
		rooms = append(rooms, room.Id)
	}

	inputs := make([]CreateNotificationOccurrenceInput, 0, count)
	for i := 0; i < count; i++ {
		roomID := rooms[i%len(rooms)]
		posted, err := chattoCore.PostMessage(ctx, KindChannel, roomID, poster.Id, fmt.Sprintf("message %d", i), nil, "", "", nil, false)
		require.NoError(tb, err)
		entry, ok := chattoCore.roomModel.timelineEntry(posted.GetId())
		require.True(tb, ok)
		inputs = append(inputs, CreateNotificationOccurrenceInput{
			RecipientID: reader.Id, SourceEventID: posted.GetId(), SourceCreated: posted.GetCreatedAt().AsTime(), ActorID: poster.Id,
			Signal:               testNotificationSignal(notificationTestSignalDirectMention, roomID, posted.GetId()),
			Mode:                 evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION,
			AttentionLevel:       notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
			SourceStreamSequence: entry.StreamSeq, SkipReadLookup: true,
		})
	}
	require.NoError(tb, chattoCore.NotificationOccurrences().CreateMany(ctx, inputs))
	occurrences, err := chattoCore.NotificationOccurrences().List(ctx, reader.Id)
	require.NoError(tb, err)
	require.Len(tb, occurrences, count)
	return reader.Id, poster.Id, occurrences
}

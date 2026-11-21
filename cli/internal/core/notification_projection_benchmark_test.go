package core

import (
	"fmt"
	"runtime"
	"testing"
	"time"

	notificationv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/notification/v1"

	"google.golang.org/protobuf/proto"
)

// newNotificationBenchmarkFixture returns serialized NOTIFICATIONS records for
// count occurrences. The mix follows a busy server: few recipients, rooms and
// actors against many occurrences, with thread and reaction signals, read
// transitions and dismissals. The result is deterministic.
func newNotificationBenchmarkFixture(tb testing.TB, count int, now time.Time) [][]byte {
	tb.Helper()
	id := func(prefix string, n int) string { return fmt.Sprintf("%s%026d", prefix, n) }
	var records [][]byte
	add := func(event *notificationv1.NotificationEvent) {
		data, err := proto.Marshal(event)
		if err != nil {
			tb.Fatal(err)
		}
		records = append(records, data)
	}
	for i := range count {
		recipient := id("U", i%200)
		source := id("E", i)
		message := &notificationv1.NotificationMessageReference{RoomId: id("R", i%40), EventId: source}
		if i%2 == 0 {
			message.ThreadRootEventId = new(id("E", i/20))
		}
		signal := &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_RoomMessageReceived{
			RoomMessageReceived: &notificationv1.RoomMessageReceived{Message: message},
		}}
		kind := "room_message"
		switch i % 10 {
		case 0:
			kind = "reaction"
			signal = &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_ReactionReceived{
				ReactionReceived: &notificationv1.ReactionReceived{Message: message, Emoji: "👍"},
			}}
		case 1:
			kind = "direct_mention"
			signal = &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_DirectMentionReceived{
				DirectMentionReceived: &notificationv1.DirectMentionReceived{Message: message},
			}}
		}
		created := now.Add(-time.Duration(count-i) * time.Second)
		expires := timestamp(created.Add(90 * 24 * time.Hour))
		signalled := &notificationv1.NotificationSignalled{
			SourceEventId:        source,
			SourceCreatedAt:      timestamp(created),
			ActorId:              id("U", (i+7)%200),
			Signal:               signal,
			SourceStreamSequence: uint64(i + 1),
			AttentionLevel:       notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_AMBIENT,
		}
		if i%10 == 1 {
			signalled.AttentionLevel = notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT
			signalled.AlertExpiresAt = timestamp(created.Add(time.Hour))
		}
		notificationID := notificationOccurrenceID(recipient, source, kind)
		add(&notificationv1.NotificationEvent{
			Id: notificationLifecycleEventID("signalled", notificationID), RecipientId: recipient, NotificationId: notificationID,
			OccurredAt: timestamp(created), ExpiresAt: expires,
			Event: &notificationv1.NotificationEvent_Signalled{Signalled: signalled},
		})
		switch {
		case i%20 == 3:
			add(&notificationv1.NotificationEvent{
				Id: notificationLifecycleEventID("removed", notificationID), RecipientId: recipient, NotificationId: notificationID,
				OccurredAt: timestamp(created), ExpiresAt: expires,
				Event: &notificationv1.NotificationEvent_Removed{Removed: &notificationv1.NotificationRemoved{}},
			})
		case i%3 == 0:
			add(&notificationv1.NotificationEvent{
				Id: notificationLifecycleEventID("read", notificationID), RecipientId: recipient, NotificationId: notificationID,
				OccurredAt: timestamp(created), ExpiresAt: expires,
				Event: &notificationv1.NotificationEvent_Read{Read: &notificationv1.NotificationRead{}},
			})
		}
	}
	return records
}

// replayNotificationBenchmarkFixture decodes and applies every record, as the
// projector does.
func replayNotificationBenchmarkFixture(tb testing.TB, records [][]byte, now time.Time) *NotificationProjection {
	tb.Helper()
	projection := NewNotificationProjection(nil)
	projection.now = func() time.Time { return now }
	for i, data := range records {
		event := &notificationv1.NotificationEvent{}
		if err := proto.Unmarshal(data, event); err != nil {
			tb.Fatal(err)
		}
		if err := projection.Apply(event, uint64(i+1)); err != nil {
			tb.Fatal(err)
		}
	}
	return projection
}

// BenchmarkNotificationProjectionRetainedHeap measures the Go heap that the
// Notifications projection retains after a replay and a forced GC, and the
// replay time. Run it with -benchtime=1x.
func BenchmarkNotificationProjectionRetainedHeap(b *testing.B) {
	if b.N != 1 {
		b.Skip("run with -benchtime=1x")
	}
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, count := range []int{10_000, 50_000} {
		b.Run(fmt.Sprintf("occurrences_%d", count), func(b *testing.B) {
			if b.N != 1 {
				b.Skip("run with -benchtime=1x")
			}
			records := newNotificationBenchmarkFixture(b, count, now)

			runtime.GC()
			var before runtime.MemStats
			runtime.ReadMemStats(&before)

			b.ReportAllocs()
			b.ResetTimer()
			projection := replayNotificationBenchmarkFixture(b, records, now)
			b.StopTimer()

			runtime.GC()
			var after runtime.MemStats
			runtime.ReadMemStats(&after)
			retainedBytes := int64(after.HeapAlloc) - int64(before.HeapAlloc)
			entries, estimatedBytes, _ := projection.adminProjectionEstimate()
			b.ReportMetric(float64(retainedBytes), "retained-heap-B")
			b.ReportMetric(float64(retainedBytes)/float64(entries), "retained-heap-B/entry")
			b.ReportMetric(float64(estimatedBytes), "estimated-B")
			runtime.KeepAlive(records)
			runtime.KeepAlive(projection)
		})
	}
}

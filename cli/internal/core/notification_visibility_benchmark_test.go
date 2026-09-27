package core

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	notificationv1 "hmans.de/chatto/internal/pb/chatto/core/notification/v1"
)

// BenchmarkNotificationVisibleOccurrences measures the visibility filter that
// ListNotificationOccurrences applies to a recipient's complete occurrence
// list before it pages the result. The work must not scale with one stream
// round trip per occurrence.
func BenchmarkNotificationVisibleOccurrences(b *testing.B) {
	for _, count := range []int{100, 1000} {
		b.Run(fmt.Sprintf("occurrences_%d", count), func(b *testing.B) {
			chattoCore, reader, occurrences := setupNotificationVisibilityBenchmark(b, count)
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

func setupNotificationVisibilityBenchmark(b *testing.B, count int) (*ChattoCore, string, []*notificationv1.NotificationOccurrence) {
	b.Helper()
	chattoCore := setupTestCoreWithEncryption(b)
	ctx := context.Background()
	poster, err := chattoCore.CreateUser(ctx, SystemActorID, "visibility-poster", "Visibility Poster", "password")
	require.NoError(b, err)
	reader, err := chattoCore.CreateUser(ctx, SystemActorID, "visibility-reader", "Visibility Reader", "password")
	require.NoError(b, err)

	const roomCount = 10
	rooms := make([]string, 0, roomCount)
	for i := 0; i < roomCount; i++ {
		room, err := chattoCore.CreateRoom(ctx, poster.Id, KindChannel, "", fmt.Sprintf("visibility-%d", i), "")
		require.NoError(b, err)
		for _, userID := range []string{poster.Id, reader.Id} {
			_, err := chattoCore.JoinRoom(ctx, userID, KindChannel, userID, room.Id)
			require.NoError(b, err)
		}
		rooms = append(rooms, room.Id)
	}

	inputs := make([]CreateNotificationOccurrenceInput, 0, count)
	for i := 0; i < count; i++ {
		roomID := rooms[i%len(rooms)]
		posted, err := chattoCore.PostMessage(ctx, KindChannel, roomID, poster.Id, fmt.Sprintf("message %d", i), nil, "", "", nil, false)
		require.NoError(b, err)
		entry, ok := chattoCore.roomModel.timelineEntry(posted.GetId())
		require.True(b, ok)
		inputs = append(inputs, CreateNotificationOccurrenceInput{
			RecipientID: reader.Id, SourceEventID: posted.GetId(), SourceCreated: posted.GetCreatedAt().AsTime(), ActorID: poster.Id,
			Signal:               testNotificationSignal(notificationTestSignalDirectMention, roomID, posted.GetId()),
			Mode:                 evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION,
			AttentionLevel:       notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
			SourceStreamSequence: entry.StreamSeq, SkipReadLookup: true,
		})
	}
	require.NoError(b, chattoCore.NotificationOccurrences().CreateMany(ctx, inputs))
	occurrences, err := chattoCore.NotificationOccurrences().List(ctx, reader.Id)
	require.NoError(b, err)
	require.Len(b, occurrences, count)
	return chattoCore, reader.Id, occurrences
}

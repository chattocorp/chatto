package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	notificationv1 "hmans.de/chatto/internal/pb/chatto/core/notification/v1"
)

// A fresh core must finish a read whose durable boundary was saved before
// the process stopped, without clearing activity beyond that boundary.
func TestNotificationReadRepairAfterCoreRestart(t *testing.T) {
	t.Parallel()
	for _, thread := range []bool{false, true} {
		name := "room"
		if thread {
			name = "thread"
		}
		t.Run(name, func(t *testing.T) {
			ctx := testContext(t)
			first, nc := newTestCore(t)
			stop := startSnapshotTestCore(t, first)
			t.Cleanup(stop)
			room, err := first.CreateRoom(ctx, SystemActorID, KindChannel, "", "Read recovery", "")
			require.NoError(t, err)
			poster, err := first.CreateUser(ctx, SystemActorID, "poster", "Poster", "password123")
			require.NoError(t, err)
			reader, err := first.CreateUser(ctx, SystemActorID, "reader", "Reader", "password123")
			require.NoError(t, err)
			for _, id := range []string{poster.Id, reader.Id} {
				_, err := first.JoinRoom(ctx, id, KindChannel, id, room.Id)
				require.NoError(t, err)
			}
			root, err := first.PostMessage(ctx, KindChannel, room.Id, poster.Id, "root", nil, "", "", nil, false)
			require.NoError(t, err)
			threadID := ""
			if thread {
				threadID = root.Id
			}
			create := func(body string) (*evtv1.Event, *notificationv1.NotificationOccurrence) {
				t.Helper()
				event, err := first.PostMessage(ctx, KindChannel, room.Id, poster.Id, body, nil, threadID, "", nil, false)
				require.NoError(t, err)
				entry, ok := first.roomModel.timelineEntry(event.Id)
				require.True(t, ok)
				signal := testNotificationSignal(notificationTestSignalDirectMention, room.Id, event.Id)
				if thread {
					signal.GetDirectMentionReceived().Message.ThreadRootEventId = new(threadID)
				}
				occurrence, _, err := first.NotificationOccurrences().Create(ctx, CreateNotificationOccurrenceInput{
					RecipientID: reader.Id, ActorID: poster.Id, SourceEventID: event.Id,
					SourceCreated: event.CreatedAt.AsTime(), SourceStreamSequence: entry.StreamSeq,
					Signal: signal, Mode: evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION,
					AttentionLevel: notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
				})
				require.NoError(t, err)
				return event, occurrence
			}
			covered, coveredOccurrence := create("seen before interruption")
			_, newerOccurrence := create("not seen")
			if thread {
				_, err = first.SetThreadLastReadEventID(ctx, KindChannel, reader.Id, room.Id, threadID, covered.Id)
			} else {
				_, err = first.AdvanceLastReadEventID(ctx, KindChannel, reader.Id, room.Id, covered.Id)
			}
			require.NoError(t, err)
			entry, ok := first.roomModel.timelineEntry(covered.Id)
			require.True(t, ok)
			stop()
			// Install exactly the durable half of the handshake while all workers
			// are stopped. No live watcher can repair it before the restart.
			_, err = first.storage.runtimeStateKV.Put(ctx, notificationReadBoundaryKey(reader.Id, room.Id, threadID),
				encodeNotificationReadBoundary(notificationReadBoundary{targetSequence: entry.StreamSeq, observedSequence: entry.StreamSeq}))
			require.NoError(t, err)
			stored, err := first.NotificationOccurrences().Get(ctx, reader.Id, coveredOccurrence.Id)
			require.NoError(t, err)
			require.False(t, stored.GetRead())

			second, err := NewChattoCore(ctx, nc, first.config)
			require.NoError(t, err)
			stopSecond := startSnapshotTestCore(t, second)
			defer stopSecond()
			require.Eventually(t, func() bool {
				stored, err := second.NotificationOccurrences().Get(ctx, reader.Id, coveredOccurrence.Id)
				return err == nil && stored.GetRead()
			}, 5*time.Second, 10*time.Millisecond)
			stored, err = second.NotificationOccurrences().Get(ctx, reader.Id, newerOccurrence.Id)
			require.NoError(t, err)
			require.False(t, stored.GetRead(), "recovery must preserve newer unread activity")
			if thread {
				marker, err := second.GetThreadLastOpened(ctx, KindChannel, reader.Id, room.Id, threadID)
				require.NoError(t, err)
				require.Equal(t, covered.CreatedAt.AsTime(), marker)
			} else {
				marker, _, err := second.PeekLastReadEventID(ctx, reader.Id, room.Id)
				require.NoError(t, err)
				require.Equal(t, covered.Id, marker)
			}
			repaired, err := second.NotificationOccurrences().reconcileCoveredUnread(ctx)
			require.NoError(t, err)
			require.Zero(t, repaired, "recovery must be idempotent")
		})
	}
}

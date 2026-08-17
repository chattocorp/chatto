package connectapi

import (
	"context"
	"testing"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func TestNotificationAssemblerIgnoresUnsupportedTarget(t *testing.T) {
	got, err := (&notificationAssembler{}).occurrenceWithPresentation(
		context.Background(),
		&corev1.NotificationOccurrence{Target: &corev1.NotificationTarget{}},
		"",
	)
	if err != nil || got != nil {
		t.Fatalf("occurrenceWithPresentation = (%v, %v), want nil, nil", got, err)
	}
}

func TestNotificationSummaryCountsAttentionAcrossCompleteOccurrenceSet(t *testing.T) {
	expires := timestamppb.New(time.Now().Add(time.Hour))
	occurrence := func(roomID string, state corev1.NotificationInboxState, level corev1.NotificationAttentionLevel, reason corev1.NotificationReason) *corev1.NotificationOccurrence {
		return &corev1.NotificationOccurrence{
			Target:         testNotificationRoomMessageTarget(roomID, "event"),
			InboxState:     state,
			AttentionLevel: level,
			Reasons:        []*corev1.NotificationReasonMatch{{Reason: reason}},
			ExpiresAt:      expires,
		}
	}

	summary := notificationSummary([]*corev1.NotificationOccurrence{
		occurrence("room-a", corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD, corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_AMBIENT, corev1.NotificationReason_NOTIFICATION_REASON_REACTION),
		occurrence("room-a", corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD, corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT, corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION),
		occurrence("room-b", corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD, corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_UNSPECIFIED, corev1.NotificationReason_NOTIFICATION_REASON_REPLY),
		occurrence("room-b", corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_READ, corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT, corev1.NotificationReason_NOTIFICATION_REASON_REPLY),
	})

	if summary.unreadCount != 3 || summary.importantUnreadCount != 2 {
		t.Fatalf("summary counts = (%d, %d), want (3, 2)", summary.unreadCount, summary.importantUnreadCount)
	}
	if len(summary.roomCounts) != 2 {
		t.Fatalf("room counts = %d, want 2", len(summary.roomCounts))
	}
	if got := summary.roomCounts[0]; got.GetRoomId() != "room-a" || got.GetUnreadCount() != 2 || got.GetImportantUnreadCount() != 1 {
		t.Fatalf("room-a summary = %+v, want unread=2 important=1", got)
	}
	if got := summary.roomCounts[1]; got.GetRoomId() != "room-b" || got.GetUnreadCount() != 1 || got.GetImportantUnreadCount() != 1 {
		t.Fatalf("room-b summary = %+v, want unread=1 important=1", got)
	}
}

func testNotificationRoomMessageTarget(roomID, eventID string) *corev1.NotificationTarget {
	return &corev1.NotificationTarget{
		Kind: &corev1.NotificationTarget_RoomMessage{
			RoomMessage: &corev1.NotificationRoomMessageTarget{
				RoomId:  roomID,
				EventId: eventID,
			},
		},
	}
}

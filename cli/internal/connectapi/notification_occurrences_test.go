package connectapi

import (
	"context"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
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

func TestVisibleNotificationOccurrencesPreservesUnsupportedFutureTarget(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("future-target-room")
	posted := env.post(room.GetId(), env.viewer.GetId(), "future target", "")
	stored, created, err := env.core.NotificationOccurrences().Create(env.ctx, core.CreateNotificationOccurrenceInput{
		RecipientID:   env.viewer.GetId(),
		SourceEventID: posted.GetId(),
		SourceCreated: posted.GetCreatedAt().AsTime(),
		ActorID:       env.viewer.GetId(),
		Target:        testNotificationRoomMessageTarget(room.GetId(), posted.GetId()),
		Reasons: []*corev1.NotificationReasonMatch{{
			Reason:    corev1.NotificationReason_NOTIFICATION_REASON_DIRECT_MENTION,
			Intensity: corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
		}},
		SkipReadLookup: true,
	})
	if err != nil || !created {
		t.Fatalf("Create occurrence = (%v, %v, %v), want created", stored, created, err)
	}
	future := proto.Clone(stored).(*corev1.NotificationOccurrence)
	future.Target = &corev1.NotificationTarget{}
	future.Target.ProtoReflect().SetUnknown([]byte{0x12, 0x00})

	visible, err := env.notifications.visibleNotificationOccurrences(env.ctx, env.viewer.GetId(), []*corev1.NotificationOccurrence{future})
	if err != nil || len(visible) != 0 {
		t.Fatalf("visible future occurrences = (%v, %v), want empty without error", visible, err)
	}
	if err := requireSupportedNotificationTargets(stored, future); connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Fatalf("mixed supported targets code = %v, want unimplemented", connect.CodeOf(err))
	}
	if deleted, err := env.notifications.deleteVisibleNotificationOccurrences(env.ctx, env.viewer.GetId(), []*corev1.NotificationOccurrence{future}); connect.CodeOf(err) != connect.CodeUnimplemented || deleted != 0 {
		t.Fatalf("delete future occurrence = (%d, %v), want zero and unimplemented", deleted, err)
	}
	if current, err := env.core.NotificationOccurrences().Get(env.ctx, env.viewer.GetId(), stored.GetId()); err != nil || current.GetRemovalReason() != corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_UNSPECIFIED {
		t.Fatalf("stored future occurrence was mutated = (%v, %v)", current, err)
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

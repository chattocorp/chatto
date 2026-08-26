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

type notificationTestSignalKind string

const (
	notificationTestSignalDirectMessage  notificationTestSignalKind = "direct_message"
	notificationTestSignalDirectMention  notificationTestSignalKind = "direct_mention"
	notificationTestSignalReply          notificationTestSignalKind = "reply"
	notificationTestSignalFollowedThread notificationTestSignalKind = "followed_thread"
	notificationTestSignalFollowedRoom   notificationTestSignalKind = "followed_room"
	notificationTestSignalReaction       notificationTestSignalKind = "reaction"
)

func TestNotificationAssemblerIgnoresUnsupportedSignal(t *testing.T) {
	got, err := (&notificationAssembler{}).occurrenceWithPresentation(
		context.Background(),
		&corev1.NotificationOccurrence{
			Signal:         &corev1.NotificationSignal{},
			AttentionLevel: corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
		},
		"",
	)
	if err != nil || got != nil {
		t.Fatalf("occurrenceWithPresentation = (%v, %v), want nil, nil", got, err)
	}
}

func TestNotificationAssemblerTreatsUnknownAttentionAsImportant(t *testing.T) {
	got := apiNotificationAttentionLevel(corev1.NotificationAttentionLevel(99))
	if got != 2 { // NOTIFICATION_ATTENTION_LEVEL_IMPORTANT
		t.Fatalf("unknown attention = %v, want conservative Important", got)
	}
}

func TestVisibleNotificationOccurrencesPreservesUnsupportedFutureSignal(t *testing.T) {
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("future-target-room")
	posted := env.post(room.GetId(), env.viewer.GetId(), "future target", "")
	stored, created, err := env.core.NotificationOccurrences().Create(env.ctx, core.CreateNotificationOccurrenceInput{
		RecipientID:    env.viewer.GetId(),
		SourceEventID:  posted.GetId(),
		SourceCreated:  posted.GetCreatedAt().AsTime(),
		ActorID:        env.viewer.GetId(),
		Signal:         testNotificationSignal(notificationTestSignalDirectMention, room.GetId(), posted.GetId()),
		Mode:           corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION,
		AttentionLevel: corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
		SkipReadLookup: true,
	})
	if err != nil || !created {
		t.Fatalf("Create occurrence = (%v, %v, %v), want created", stored, created, err)
	}
	future := proto.Clone(stored).(*corev1.NotificationOccurrence)
	future.Signal = &corev1.NotificationSignal{}
	future.Signal.ProtoReflect().SetUnknown([]byte{0x80, 0x06, 0x01})

	visible, err := env.notifications.visibleNotificationOccurrences(env.ctx, env.viewer.GetId(), []*corev1.NotificationOccurrence{future})
	if err != nil || len(visible) != 0 {
		t.Fatalf("visible future occurrences = (%v, %v), want empty without error", visible, err)
	}
	if err := requireSupportedNotificationSignals(stored, future); connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Fatalf("mixed supported signals code = %v, want unimplemented", connect.CodeOf(err))
	}
	if deleted, err := env.notifications.deleteVisibleNotificationOccurrences(env.ctx, env.viewer.GetId(), []*corev1.NotificationOccurrence{future}); connect.CodeOf(err) != connect.CodeUnimplemented || deleted != 0 {
		t.Fatalf("delete future occurrence = (%d, %v), want zero and unimplemented", deleted, err)
	}
	if current, err := env.core.NotificationOccurrences().Get(env.ctx, env.viewer.GetId(), stored.GetId()); err != nil || current.GetId() != stored.GetId() {
		t.Fatalf("stored future occurrence was mutated = (%v, %v)", current, err)
	}
}

func TestNotificationSummaryCountsAttentionAcrossCompleteOccurrenceSet(t *testing.T) {
	expires := timestamppb.New(time.Now().Add(time.Hour))
	occurrence := func(roomID string, read bool, level corev1.NotificationAttentionLevel, reason notificationTestSignalKind) *corev1.NotificationOccurrence {
		return &corev1.NotificationOccurrence{
			Signal:         testNotificationSignal(reason, roomID, "event"),
			Read:           read,
			AttentionLevel: level,
			ExpiresAt:      expires,
		}
	}

	summary := notificationSummary([]*corev1.NotificationOccurrence{
		occurrence("room-a", false, corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_AMBIENT, notificationTestSignalReaction),
		occurrence("room-a", false, corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT, notificationTestSignalDirectMention),
		occurrence("room-b", false, corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT, notificationTestSignalReply),
		occurrence("room-b", true, corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT, notificationTestSignalReply),
		occurrence("room-c", false, corev1.NotificationAttentionLevel(99), notificationTestSignalReply),
	})

	if summary.unreadCount != 4 || summary.importantUnreadCount != 3 {
		t.Fatalf("summary counts = (%d, %d), want (4, 3)", summary.unreadCount, summary.importantUnreadCount)
	}
	if len(summary.roomCounts) != 3 {
		t.Fatalf("room counts = %d, want 3", len(summary.roomCounts))
	}
	if got := summary.roomCounts[0]; got.GetRoomId() != "room-a" || got.GetUnreadCount() != 2 || got.GetImportantUnreadCount() != 1 {
		t.Fatalf("room-a summary = %+v, want unread=2 important=1", got)
	}
	if got := summary.roomCounts[1]; got.GetRoomId() != "room-b" || got.GetUnreadCount() != 1 || got.GetImportantUnreadCount() != 1 {
		t.Fatalf("room-b summary = %+v, want unread=1 important=1", got)
	}
	if got := summary.roomCounts[2]; got.GetRoomId() != "room-c" || got.GetUnreadCount() != 1 || got.GetImportantUnreadCount() != 1 {
		t.Fatalf("room-c summary = %+v, want unknown level counted as important", got)
	}
}

func testNotificationRoomMessageTarget(roomID, eventID string) *corev1.NotificationMessageReference {
	return &corev1.NotificationMessageReference{RoomId: roomID, EventId: eventID}
}

func testNotificationSignal(kind notificationTestSignalKind, roomID, eventID string) *corev1.NotificationSignal {
	message := testNotificationRoomMessageTarget(roomID, eventID)
	return testNotificationSignalWithMessage(kind, message)
}

func testNotificationSignalWithMessage(kind notificationTestSignalKind, message *corev1.NotificationMessageReference) *corev1.NotificationSignal {
	signal := &corev1.NotificationSignal{}
	switch kind {
	case notificationTestSignalDirectMessage:
		signal.Kind = &corev1.NotificationSignal_DirectMessageReceived{DirectMessageReceived: &corev1.DirectMessageReceived{Message: message}}
	case notificationTestSignalReaction:
		signal.Kind = &corev1.NotificationSignal_ReactionReceived{ReactionReceived: &corev1.ReactionReceived{Message: message}}
	case notificationTestSignalReply:
		signal.Kind = &corev1.NotificationSignal_ReplyReceived{ReplyReceived: &corev1.ReplyReceived{Message: message}}
	case notificationTestSignalFollowedThread:
		signal.Kind = &corev1.NotificationSignal_FollowedThreadActivity{FollowedThreadActivity: &corev1.FollowedThreadActivity{Message: message}}
	case notificationTestSignalFollowedRoom:
		signal.Kind = &corev1.NotificationSignal_FollowedRoomActivity{FollowedRoomActivity: &corev1.FollowedRoomActivity{Message: message}}
	default:
		signal.Kind = &corev1.NotificationSignal_DirectMentionReceived{DirectMentionReceived: &corev1.DirectMentionReceived{Message: message}}
	}
	return signal
}

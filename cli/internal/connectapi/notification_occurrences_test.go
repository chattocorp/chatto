package connectapi

import (
	"context"
	"hmans.de/chatto/internal/pb/chatto/core/notification/v1"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

type notificationTestSignalKind string

const (
	notificationTestSignalDirectMessage  notificationTestSignalKind = "direct_message"
	notificationTestSignalDirectMention  notificationTestSignalKind = "direct_mention"
	notificationTestSignalReply          notificationTestSignalKind = "reply"
	notificationTestSignalFollowedThread notificationTestSignalKind = "followed_thread"
	notificationTestSignalFollowedRoom   notificationTestSignalKind = "followed_room"
	notificationTestSignalReaction       notificationTestSignalKind = "reaction"
	notificationTestSignalRoomMessage    notificationTestSignalKind = "room_message"
)

func TestNotificationAssemblerIgnoresUnsupportedSignal(t *testing.T) {
	got, err := (&notificationAssembler{}).occurrenceWithPresentation(
		context.Background(),
		&notificationv1.NotificationOccurrence{
			Signal:         &notificationv1.NotificationSignal{},
			AttentionLevel: notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
		},
		"",
	)
	if err != nil || got != nil {
		t.Fatalf("occurrenceWithPresentation = (%v, %v), want nil, nil", got, err)
	}
}

func TestNotificationAssemblerTreatsUnknownAttentionAsImportant(t *testing.T) {
	got := apiNotificationAttentionLevel(notificationv1.NotificationAttentionLevel(99))
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
		Mode:           evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION,
		AttentionLevel: notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
		SkipReadLookup: true,
	})
	if err != nil || !created {
		t.Fatalf("Create occurrence = (%v, %v, %v), want created", stored, created, err)
	}
	future := proto.Clone(stored).(*notificationv1.NotificationOccurrence)
	future.Signal = &notificationv1.NotificationSignal{}
	future.Signal.ProtoReflect().SetUnknown([]byte{0x80, 0x06, 0x01})

	visible, err := env.notifications.visibleNotificationOccurrences(env.ctx, env.viewer.GetId(), []*notificationv1.NotificationOccurrence{future})
	if err != nil || len(visible) != 0 {
		t.Fatalf("visible future occurrences = (%v, %v), want empty without error", visible, err)
	}
	if err := requireSupportedNotificationSignals(stored, future); connect.CodeOf(err) != connect.CodeUnimplemented {
		t.Fatalf("mixed supported signals code = %v, want unimplemented", connect.CodeOf(err))
	}
	if deleted, err := env.notifications.deleteVisibleNotificationOccurrences(env.ctx, env.viewer.GetId(), []*notificationv1.NotificationOccurrence{future}); connect.CodeOf(err) != connect.CodeUnimplemented || deleted != 0 {
		t.Fatalf("delete future occurrence = (%d, %v), want zero and unimplemented", deleted, err)
	}
	if current, err := env.core.NotificationOccurrences().Get(env.ctx, env.viewer.GetId(), stored.GetId()); err != nil || current.GetId() != stored.GetId() {
		t.Fatalf("stored future occurrence was mutated = (%v, %v)", current, err)
	}
}

func TestNotificationSummaryCountsAttentionAcrossCompleteOccurrenceSet(t *testing.T) {
	expires := timestamppb.New(time.Now().Add(time.Hour))
	occurrence := func(roomID string, read bool, level notificationv1.NotificationAttentionLevel, reason notificationTestSignalKind) *notificationv1.NotificationOccurrence {
		return &notificationv1.NotificationOccurrence{
			Signal:         testNotificationSignal(reason, roomID, "event"),
			Read:           read,
			AttentionLevel: level,
			ExpiresAt:      expires,
		}
	}

	summary := notificationSummary([]*notificationv1.NotificationOccurrence{
		occurrence("room-a", false, notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_AMBIENT, notificationTestSignalReaction),
		occurrence("room-a", false, notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT, notificationTestSignalDirectMention),
		occurrence("room-b", false, notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT, notificationTestSignalReply),
		occurrence("room-b", true, notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT, notificationTestSignalReply),
		occurrence("room-c", false, notificationv1.NotificationAttentionLevel(99), notificationTestSignalReply),
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

func testNotificationRoomMessageTarget(roomID, eventID string) *notificationv1.NotificationMessageReference {
	return &notificationv1.NotificationMessageReference{RoomId: roomID, EventId: eventID}
}

func testNotificationSignal(kind notificationTestSignalKind, roomID, eventID string) *notificationv1.NotificationSignal {
	message := testNotificationRoomMessageTarget(roomID, eventID)
	return testNotificationSignalWithMessage(kind, message)
}

func testNotificationSignalWithMessage(kind notificationTestSignalKind, message *notificationv1.NotificationMessageReference) *notificationv1.NotificationSignal {
	signal := &notificationv1.NotificationSignal{}
	switch kind {
	case notificationTestSignalDirectMessage:
		signal.Kind = &notificationv1.NotificationSignal_DirectMessageReceived{DirectMessageReceived: &notificationv1.DirectMessageReceived{Message: message}}
	case notificationTestSignalReaction:
		signal.Kind = &notificationv1.NotificationSignal_ReactionReceived{ReactionReceived: &notificationv1.ReactionReceived{Message: message}}
	case notificationTestSignalReply:
		signal.Kind = &notificationv1.NotificationSignal_ReplyReceived{ReplyReceived: &notificationv1.ReplyReceived{Message: message}}
	case notificationTestSignalFollowedThread:
		signal.Kind = &notificationv1.NotificationSignal_FollowedThreadActivity{FollowedThreadActivity: &notificationv1.FollowedThreadActivity{Message: message}}
	case notificationTestSignalFollowedRoom:
		signal.Kind = &notificationv1.NotificationSignal_FollowedRoomActivity{FollowedRoomActivity: &notificationv1.FollowedRoomActivity{Message: message}}
	case notificationTestSignalRoomMessage:
		signal.Kind = &notificationv1.NotificationSignal_RoomMessageReceived{RoomMessageReceived: &notificationv1.RoomMessageReceived{Message: message}}
	default:
		signal.Kind = &notificationv1.NotificationSignal_DirectMentionReceived{DirectMentionReceived: &notificationv1.DirectMentionReceived{Message: message}}
	}
	return signal
}

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

func TestNotificationAssemblerIgnoresUnsupportedSignal(t *testing.T) {
	got, err := (&notificationAssembler{}).occurrenceWithPresentation(
		context.Background(),
		&corev1.NotificationOccurrence{Signal: &corev1.NotificationSignal{}},
		"",
	)
	if err != nil || got != nil {
		t.Fatalf("occurrenceWithPresentation = (%v, %v), want nil, nil", got, err)
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
		Signal:         testNotificationSignal(corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_DIRECT_MENTION, room.GetId(), posted.GetId()),
		Intensity:      corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_BADGE,
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
	occurrence := func(roomID string, state corev1.NotificationReadState, level corev1.NotificationAttentionLevel, reason corev1.NotificationPolicyKind) *corev1.NotificationOccurrence {
		return &corev1.NotificationOccurrence{
			Signal:         testNotificationSignal(reason, roomID, "event"),
			ReadState:      state,
			AttentionLevel: level,
			ExpiresAt:      expires,
		}
	}

	summary := notificationSummary([]*corev1.NotificationOccurrence{
		occurrence("room-a", corev1.NotificationReadState_NOTIFICATION_READ_STATE_UNREAD, corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_AMBIENT, corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_REACTION),
		occurrence("room-a", corev1.NotificationReadState_NOTIFICATION_READ_STATE_UNREAD, corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT, corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_DIRECT_MENTION),
		occurrence("room-b", corev1.NotificationReadState_NOTIFICATION_READ_STATE_UNREAD, corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_UNSPECIFIED, corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_REPLY),
		occurrence("room-b", corev1.NotificationReadState_NOTIFICATION_READ_STATE_READ, corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT, corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_REPLY),
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

func testNotificationRoomMessageTarget(roomID, eventID string) *corev1.NotificationMessageReference {
	return &corev1.NotificationMessageReference{RoomId: roomID, EventId: eventID}
}

func testNotificationSignal(kind corev1.NotificationPolicyKind, roomID, eventID string) *corev1.NotificationSignal {
	message := testNotificationRoomMessageTarget(roomID, eventID)
	return testNotificationSignalWithMessage(kind, message)
}

func testNotificationSignalWithMessage(kind corev1.NotificationPolicyKind, message *corev1.NotificationMessageReference) *corev1.NotificationSignal {
	signal := &corev1.NotificationSignal{}
	switch kind {
	case corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_DIRECT_MESSAGE:
		signal.Kind = &corev1.NotificationSignal_DirectMessageReceived{DirectMessageReceived: &corev1.DirectMessageReceived{Message: message}}
	case corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_REACTION:
		signal.Kind = &corev1.NotificationSignal_ReactionReceived{ReactionReceived: &corev1.ReactionReceived{Message: message}}
	case corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_REPLY:
		signal.Kind = &corev1.NotificationSignal_ReplyReceived{ReplyReceived: &corev1.ReplyReceived{Message: message}}
	case corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_FOLLOWED_THREAD:
		signal.Kind = &corev1.NotificationSignal_FollowedThreadActivity{FollowedThreadActivity: &corev1.FollowedThreadActivity{Message: message}}
	case corev1.NotificationPolicyKind_NOTIFICATION_POLICY_KIND_FOLLOWED_ROOM:
		signal.Kind = &corev1.NotificationSignal_FollowedRoomActivity{FollowedRoomActivity: &corev1.FollowedRoomActivity{Message: message}}
	default:
		signal.Kind = &corev1.NotificationSignal_DirectMentionReceived{DirectMentionReceived: &corev1.DirectMentionReceived{Message: message}}
	}
	return signal
}

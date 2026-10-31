package connectapi

import (
	"context"
	"github.com/chattocorp/chatto/cli/internal/pb/chatto/core/notification/v1"
	"strings"
	"testing"
	"time"

	"connectrpc.com/connect"
	"github.com/chattocorp/chatto/cli/internal/core"
	apiv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/api/v1"
	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	"github.com/chattocorp/chatto/cli/internal/push"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestNotificationContentMatchesWebPushAndHidesDeletedMessages(t *testing.T) {
	t.Parallel()
	env := newConnectAPITestEnv(t)
	room := env.createJoinedRoom("notification-content")
	body := strings.Repeat("Preview 😀 ", 20)
	posted := env.post(room.GetId(), env.viewer.GetId(), body, "")
	stored, _, err := env.core.NotificationOccurrences().Create(env.ctx, core.CreateNotificationOccurrenceInput{
		RecipientID: env.viewer.GetId(), SourceEventID: posted.GetId(),
		SourceCreated: posted.GetCreatedAt().AsTime(), ActorID: env.viewer.GetId(),
		Signal:         testNotificationSignal(notificationTestSignalDirectMention, room.GetId(), posted.GetId()),
		Mode:           evtv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION,
		AttentionLevel: notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
		SkipReadLookup: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx := withCaller(env.ctx, env.viewer)
	response, err := env.notifications.GetNotificationOccurrence(ctx, connect.NewRequest(&apiv1.GetNotificationOccurrenceRequest{NotificationId: stored.GetId()}))
	if err != nil {
		t.Fatal(err)
	}
	want := push.BuildPayloadFromOccurrence(stored, env.viewer.DisplayName, "", &push.PayloadContext{RoomName: room.GetName(), MessagePreview: body})
	content := response.Msg.GetOccurrence().GetContent()
	if content.GetTitle() != want.Title || content.GetBody() != want.Body || content.GetBody() == "" {
		t.Fatalf("API content differs from Web Push: %v", content)
	}
	batch, err := newNotificationAssembler(env.api).occurrences(env.ctx, []*notificationv1.NotificationOccurrence{stored})
	if err != nil || len(batch) != 1 || !proto.Equal(batch[0].GetContent(), content) {
		t.Fatalf("batch content differs: %v, %v", batch, err)
	}
	if err := env.core.DeleteMessage(env.ctx, env.viewer.Id, core.KindChannel, room.Id, posted.Id); err != nil {
		t.Fatal(err)
	}
	// Hydration must not expose the retained occurrence after its target is deleted.
	batch, err = newNotificationAssembler(env.api).occurrences(env.ctx, []*notificationv1.NotificationOccurrence{stored})
	if err != nil || len(batch) != 0 {
		t.Fatalf("deleted content returned: %v, %v", batch, err)
	}
	single, err := newNotificationAssembler(env.api).occurrence(env.ctx, stored)
	if err != nil || single != nil {
		t.Fatalf("deleted single content returned: %v, %v", single, err)
	}
}

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
	t.Parallel()

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
	t.Parallel()

	got := apiNotificationAttentionLevel(notificationv1.NotificationAttentionLevel(99))
	if got != 2 { // NOTIFICATION_ATTENTION_LEVEL_IMPORTANT
		t.Fatalf("unknown attention = %v, want conservative Important", got)
	}
}

func TestVisibleNotificationOccurrencesPreservesUnsupportedFutureSignal(t *testing.T) {
	t.Parallel()

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
	if err := requireSupportedNotificationSignals(stored, future); errorCode(err) != connect.CodeUnimplemented {
		t.Fatalf("mixed supported signals code = %v, want unimplemented", errorCode(err))
	}
	if deleted, err := env.notifications.deleteVisibleNotificationOccurrences(env.ctx, env.viewer.GetId(), []*notificationv1.NotificationOccurrence{future}); errorCode(err) != connect.CodeUnimplemented || deleted != 0 {
		t.Fatalf("delete future occurrence = (%d, %v), want zero and unimplemented", deleted, err)
	}
	if current, err := env.core.NotificationOccurrences().Get(env.ctx, env.viewer.GetId(), stored.GetId()); err != nil || current.GetId() != stored.GetId() {
		t.Fatalf("stored future occurrence was mutated = (%v, %v)", current, err)
	}
}

func TestNotificationSummaryCountsAttentionAcrossCompleteOccurrenceSet(t *testing.T) {
	t.Parallel()

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

func TestNotificationSummaryIncludesRoomOnlyInvitation(t *testing.T) {
	t.Parallel()
	invitation := &notificationv1.NotificationOccurrence{
		Signal: &notificationv1.NotificationSignal{Kind: &notificationv1.NotificationSignal_RoomInvitationReceived{
			RoomInvitationReceived: &notificationv1.RoomInvitationReceived{RoomId: "invited-room", InvitationId: "invite"},
		}},
		AttentionLevel: notificationv1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT,
		ExpiresAt:      timestamppb.New(time.Now().Add(time.Hour)),
	}
	summary := notificationSummary([]*notificationv1.NotificationOccurrence{invitation})
	if summary.unreadCount != 1 || summary.importantUnreadCount != 1 || len(summary.roomCounts) != 1 || summary.roomCounts[0].GetRoomId() != "invited-room" {
		t.Fatalf("invitation summary = %+v", summary)
	}
	invitation.Read = true
	summary = notificationSummary([]*notificationv1.NotificationOccurrence{invitation})
	if summary.unreadCount != 0 || len(summary.roomCounts) != 0 {
		t.Fatalf("read invitation summary = %+v", summary)
	}
}

package core

import (
	"context"
	"testing"

	"google.golang.org/protobuf/types/known/fieldmaskpb"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type notificationTestSignalKind string

const (
	notificationTestSignalDirectMessage  notificationTestSignalKind = "direct_message_received"
	notificationTestSignalDirectMention  notificationTestSignalKind = "direct_mention_received"
	notificationTestSignalReply          notificationTestSignalKind = "reply_received"
	notificationTestSignalRoleMention    notificationTestSignalKind = "role_mention_received"
	notificationTestSignalHere           notificationTestSignalKind = "here_mention_received"
	notificationTestSignalAll            notificationTestSignalKind = "all_mention_received"
	notificationTestSignalFollowedThread notificationTestSignalKind = "followed_thread_activity"
	notificationTestSignalFollowedRoom   notificationTestSignalKind = "followed_room_activity"
	notificationTestSignalReaction       notificationTestSignalKind = "reaction_received"
)

func testNotificationOccurrences(t *testing.T, chattoCore *ChattoCore, userID string) []*corev1.NotificationOccurrence {
	t.Helper()
	items, err := chattoCore.NotificationOccurrences().List(testContext(t), userID)
	if err != nil {
		t.Fatalf("List notification occurrences: %v", err)
	}
	return items
}

func testOccurrenceHasKind(occurrence *corev1.NotificationOccurrence, kind notificationTestSignalKind) bool {
	return notificationSignalIdentity(occurrence.GetSignal()) == string(kind)
}

func testOccurrencesHaveKinds(occurrences []*corev1.NotificationOccurrence, kinds ...notificationTestSignalKind) bool {
	seen := make(map[notificationTestSignalKind]int)
	for _, occurrence := range occurrences {
		seen[notificationTestSignalKind(notificationSignalIdentity(occurrence.GetSignal()))]++
	}
	for _, kind := range kinds {
		if seen[kind] == 0 {
			return false
		}
		seen[kind]--
	}
	return true
}

func newNotificationRoomMessageTarget(roomID, eventID string) *corev1.NotificationMessageReference {
	return newNotificationMessageReference(roomID, eventID)
}

func testNotificationSignal(kind notificationTestSignalKind, roomID, eventID string) *corev1.NotificationSignal {
	message := newNotificationMessageReference(roomID, eventID)
	switch kind {
	case notificationTestSignalDirectMessage:
		return &corev1.NotificationSignal{Kind: &corev1.NotificationSignal_DirectMessageReceived{DirectMessageReceived: &corev1.DirectMessageReceived{Message: message}}}
	case notificationTestSignalDirectMention:
		return &corev1.NotificationSignal{Kind: &corev1.NotificationSignal_DirectMentionReceived{DirectMentionReceived: &corev1.DirectMentionReceived{Message: message}}}
	case notificationTestSignalReply:
		return &corev1.NotificationSignal{Kind: &corev1.NotificationSignal_ReplyReceived{ReplyReceived: &corev1.ReplyReceived{Message: message}}}
	case notificationTestSignalRoleMention:
		return &corev1.NotificationSignal{Kind: &corev1.NotificationSignal_RoleMentionReceived{RoleMentionReceived: &corev1.RoleMentionReceived{Message: message}}}
	case notificationTestSignalHere:
		return &corev1.NotificationSignal{Kind: &corev1.NotificationSignal_HereMentionReceived{HereMentionReceived: &corev1.HereMentionReceived{Message: message}}}
	case notificationTestSignalAll:
		return &corev1.NotificationSignal{Kind: &corev1.NotificationSignal_AllMentionReceived{AllMentionReceived: &corev1.AllMentionReceived{Message: message}}}
	case notificationTestSignalFollowedThread:
		return &corev1.NotificationSignal{Kind: &corev1.NotificationSignal_FollowedThreadActivity{FollowedThreadActivity: &corev1.FollowedThreadActivity{Message: message}}}
	case notificationTestSignalFollowedRoom:
		return &corev1.NotificationSignal{Kind: &corev1.NotificationSignal_FollowedRoomActivity{FollowedRoomActivity: &corev1.FollowedRoomActivity{Message: message}}}
	case notificationTestSignalReaction:
		return &corev1.NotificationSignal{Kind: &corev1.NotificationSignal_ReactionReceived{ReactionReceived: &corev1.ReactionReceived{Message: message}}}
	default:
		return nil
	}
}

func testNotificationPolicyPatch(kind notificationTestSignalKind, mode corev1.NotificationDeliveryMode) (*corev1.NotificationDeliveryModes, *fieldmaskpb.FieldMask) {
	modes := &corev1.NotificationDeliveryModes{}
	var path string
	var target **corev1.NotificationDeliveryMode
	switch kind {
	case notificationTestSignalDirectMessage:
		path, target = "direct_messages", &modes.DirectMessages
	case notificationTestSignalDirectMention:
		path, target = "direct_mentions", &modes.DirectMentions
	case notificationTestSignalReply:
		path, target = "replies", &modes.Replies
	case notificationTestSignalRoleMention:
		path, target = "role_mentions", &modes.RoleMentions
	case notificationTestSignalHere:
		path, target = "here_mentions", &modes.HereMentions
	case notificationTestSignalAll:
		path, target = "all_mentions", &modes.AllMentions
	case notificationTestSignalFollowedThread:
		path, target = "followed_threads", &modes.FollowedThreads
	case notificationTestSignalFollowedRoom:
		path, target = "followed_rooms", &modes.FollowedRooms
	case notificationTestSignalReaction:
		path, target = "reactions", &modes.Reactions
	default:
		return modes, &fieldmaskpb.FieldMask{Paths: []string{string(kind)}}
	}
	if mode != corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED {
		*target = mode.Enum()
	}
	return modes, &fieldmaskpb.FieldMask{Paths: []string{path}}
}

func (s *NotificationPolicyModel) SetServerNotificationMode(ctx context.Context, actorID string, kind notificationTestSignalKind, mode corev1.NotificationDeliveryMode) (*NotificationPolicy, error) {
	patch, mask := testNotificationPolicyPatch(kind, mode)
	return s.UpdateNotificationPolicy(ctx, actorID, "", patch, mask)
}

func (s *NotificationPolicyModel) SetRoomNotificationMode(ctx context.Context, actorID, roomID string, kind notificationTestSignalKind, mode corev1.NotificationDeliveryMode) (*NotificationPolicy, error) {
	patch, mask := testNotificationPolicyPatch(kind, mode)
	return s.UpdateNotificationPolicy(ctx, actorID, roomID, patch, mask)
}

func (cm *ConfigModel) notificationRoomMode(userID, roomID string, kind notificationTestSignalKind) corev1.NotificationDeliveryMode {
	return notificationModeForSignal(cm.notificationRoomModes(userID, roomID), testNotificationSignal(kind, roomID, "test"))
}

func testUnsupportedNotificationSignal() *corev1.NotificationSignal {
	signal := &corev1.NotificationSignal{}
	signal.ProtoReflect().SetUnknown([]byte{0x80, 0x06, 0x01})
	return signal
}

func testDeleteAllNotificationOccurrences(t *testing.T, chattoCore *ChattoCore, userID string) {
	t.Helper()
	for _, occurrence := range testNotificationOccurrences(t, chattoCore, userID) {
		if _, err := chattoCore.NotificationOccurrences().Delete(testContext(t), userID, occurrence.GetId()); err != nil {
			t.Fatalf("delete notification occurrence: %v", err)
		}
	}
}

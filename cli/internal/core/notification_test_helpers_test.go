package core

import (
	"testing"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func testNotificationOccurrences(t *testing.T, chattoCore *ChattoCore, userID string) []*corev1.NotificationOccurrence {
	t.Helper()
	items, err := chattoCore.NotificationOccurrences().List(testContext(t), userID)
	if err != nil {
		t.Fatalf("List notification occurrences: %v", err)
	}
	return items
}

func testOccurrenceHasKind(occurrence *corev1.NotificationOccurrence, kind corev1.NotificationPolicyKind) bool {
	return notificationSignalPolicyKind(occurrence.GetSignal()) == kind
}

func testOccurrencesHaveKinds(occurrences []*corev1.NotificationOccurrence, kinds ...corev1.NotificationPolicyKind) bool {
	seen := make(map[corev1.NotificationPolicyKind]int)
	for _, occurrence := range occurrences {
		seen[notificationSignalPolicyKind(occurrence.GetSignal())]++
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

func testNotificationSignal(kind corev1.NotificationPolicyKind, roomID, eventID string) *corev1.NotificationSignal {
	return notificationSignalForPolicyKind(kind, newNotificationMessageReference(roomID, eventID), "")
}

func testUnsupportedNotificationSignal() *corev1.NotificationSignal {
	signal := &corev1.NotificationSignal{}
	signal.ProtoReflect().SetUnknown([]byte{0x80, 0x06, 0x01})
	return signal
}

func testDeleteAllNotificationOccurrences(t *testing.T, chattoCore *ChattoCore, userID string) {
	t.Helper()
	for _, occurrence := range testNotificationOccurrences(t, chattoCore, userID) {
		if _, err := chattoCore.NotificationOccurrences().Delete(testContext(t), userID, occurrence.GetId(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_DELETED); err != nil {
			t.Fatalf("delete notification occurrence: %v", err)
		}
	}
}

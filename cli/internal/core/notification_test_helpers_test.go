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

func testOccurrenceHasKind(occurrence *corev1.NotificationOccurrence, kind corev1.NotificationPreferenceCategory) bool {
	return notificationSignalPreferenceCategory(occurrence.GetSignal()) == kind
}

func testOccurrencesHaveKinds(occurrences []*corev1.NotificationOccurrence, kinds ...corev1.NotificationPreferenceCategory) bool {
	seen := make(map[corev1.NotificationPreferenceCategory]int)
	for _, occurrence := range occurrences {
		seen[notificationSignalPreferenceCategory(occurrence.GetSignal())]++
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

func testNotificationSignal(kind corev1.NotificationPreferenceCategory, roomID, eventID string) *corev1.NotificationSignal {
	return notificationSignalForPreferenceCategory(kind, newNotificationMessageReference(roomID, eventID), "")
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

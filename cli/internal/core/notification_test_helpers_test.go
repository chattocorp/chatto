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

func testOccurrenceHasReason(occurrence *corev1.NotificationOccurrence, reason corev1.NotificationReason) bool {
	for _, match := range occurrence.GetReasons() {
		if match.GetReason() == reason {
			return true
		}
	}
	return false
}

func testDeleteAllNotificationOccurrences(t *testing.T, chattoCore *ChattoCore, userID string) {
	t.Helper()
	for _, occurrence := range testNotificationOccurrences(t, chattoCore, userID) {
		if _, err := chattoCore.NotificationOccurrences().Delete(testContext(t), userID, occurrence.GetId(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_DELETED); err != nil {
			t.Fatalf("delete notification occurrence: %v", err)
		}
	}
}

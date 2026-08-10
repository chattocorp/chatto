package core

import (
	"testing"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func testNotificationOccurrences(t *testing.T, chattoCore *ChattoCore, userID string) []*corev1.NotificationOccurrence {
	t.Helper()
	items, err := chattoCore.NotificationOccurrences().List(testContext(t), userID, NotificationOccurrenceViewInbox)
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

func testMoveAllNotificationOccurrencesDone(t *testing.T, chattoCore *ChattoCore, userID string) {
	t.Helper()
	done := corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_DONE
	for _, occurrence := range testNotificationOccurrences(t, chattoCore, userID) {
		if _, err := chattoCore.NotificationOccurrences().Update(testContext(t), userID, occurrence.GetId(), UpdateNotificationOccurrenceInput{InboxState: &done}); err != nil {
			t.Fatalf("move notification occurrence Done: %v", err)
		}
	}
}

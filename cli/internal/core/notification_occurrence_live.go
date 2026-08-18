package core

import (
	"context"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func (c *ChattoCore) publishNotificationOccurrencesInvalidated(ctx context.Context, occurrence *corev1.NotificationOccurrence, alertCandidate bool) {
	if c == nil || occurrence == nil || occurrence.GetRecipientId() == "" {
		return
	}
	var candidateID *string
	if alertCandidate && NotificationAlertPending(occurrence) && !c.suppressesNotificationAlertsForPresence(ctx, occurrence.GetRecipientId()) {
		candidateID = proto.String(occurrence.GetId())
	}
	event := newLiveEvent(occurrence.GetActorId(), &corev1.LiveEvent{
		Event: &corev1.LiveEvent_NotificationOccurrencesInvalidated{
			NotificationOccurrencesInvalidated: &corev1.NotificationOccurrencesInvalidatedEvent{
				AlertCandidateNotificationId: candidateID,
			},
		},
	})
	if err := c.publishLiveEvent(ctx, subjects.LiveSyncUserEvent(occurrence.GetRecipientId(), "notification_v2"), event); err != nil {
		c.logger.Warn("Failed to publish notification occurrence invalidation",
			"notification_id", occurrence.GetId(),
			"recipient_id", occurrence.GetRecipientId(),
			"error", err)
	}
}

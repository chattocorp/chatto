package core

import (
	"context"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func (c *ChattoCore) publishNotificationOccurrenceChanged(ctx context.Context, occurrence *corev1.NotificationOccurrence, created, deleted bool) {
	if c == nil || occurrence == nil || occurrence.GetRecipientId() == "" {
		return
	}
	alert := created && occurrence.GetIntensity() == corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT &&
		occurrence.GetInboxState() == corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD &&
		occurrence.GetAlertState() == corev1.NotificationAlertState_NOTIFICATION_ALERT_STATE_PENDING &&
		!c.suppressesNotificationAlertsForPresence(ctx, occurrence.GetRecipientId())
	event := newLiveEvent(occurrence.GetActorId(), &corev1.LiveEvent{
		Event: &corev1.LiveEvent_NotificationOccurrenceChanged{
			NotificationOccurrenceChanged: &corev1.NotificationOccurrenceChangedEvent{
				NotificationId: occurrence.GetId(),
				Created:        created,
				Deleted:        deleted,
				Alert:          alert,
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

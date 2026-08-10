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
	alert := created && occurrence.GetStrongestIntensity() == corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_ALERT &&
		!c.suppressesNotificationAlertsForPresence(ctx, occurrence.GetRecipientId())
	revision := uint64(0)
	if entry, exists, err := c.notificationOccurrences.index.occurrenceBySource(ctx, occurrence.GetRecipientId(), occurrence.GetSourceEventId()); err == nil && exists {
		revision = entry.revision
	}
	event := newLiveEvent(occurrence.GetActorId(), &corev1.LiveEvent{
		Event: &corev1.LiveEvent_NotificationOccurrenceChanged{
			NotificationOccurrenceChanged: &corev1.NotificationOccurrenceChangedEvent{
				NotificationId:       occurrence.GetId(),
				Created:              created,
				Deleted:              deleted,
				Alert:                alert,
				SourceEventId:        occurrence.GetSourceEventId(),
				RuntimeStateRevision: revision,
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

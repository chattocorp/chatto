package core

import (
	"context"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func (c *ChattoCore) publishNotificationOccurrencesInvalidated(ctx context.Context, occurrence *corev1.NotificationOccurrence, alertCandidate bool) {
	c.publishNotificationOccurrenceInvalidations(ctx, []*corev1.NotificationOccurrence{occurrence}, alertCandidate)
}

func (c *ChattoCore) publishNotificationOccurrenceInvalidations(ctx context.Context, occurrences []*corev1.NotificationOccurrence, alertCandidate bool) {
	if c == nil {
		return
	}
	alertRecipients := make([]string, 0, len(occurrences))
	if alertCandidate {
		for _, occurrence := range occurrences {
			if occurrence != nil && occurrence.GetRecipientId() != "" && NotificationAlertPending(occurrence) {
				alertRecipients = append(alertRecipients, occurrence.GetRecipientId())
			}
		}
	}
	var presences map[string]string
	if len(alertRecipients) > 0 {
		var err error
		presences, err = c.GetUserPresences(ctx, alertRecipients)
		if err != nil {
			c.logger.Warn("Failed to get presence for notification suppression", "error", err)
			presences = nil
		}
	}
	publications := make([]liveEventPublication, 0, len(occurrences))
	for _, occurrence := range occurrences {
		if occurrence == nil || occurrence.GetRecipientId() == "" {
			continue
		}
		var candidateID *string
		if alertCandidate && NotificationAlertPending(occurrence) && presences[occurrence.GetRecipientId()] != PresenceStatusDoNotDisturb {
			candidateID = proto.String(occurrence.GetId())
		}
		publications = append(publications, liveEventPublication{
			subject: subjects.LiveSyncUserEvent(occurrence.GetRecipientId(), "notification_v2"),
			event: newLiveEvent(occurrence.GetActorId(), &corev1.LiveEvent{
				Event: &corev1.LiveEvent_NotificationOccurrencesInvalidated{
					NotificationOccurrencesInvalidated: &corev1.NotificationOccurrencesInvalidatedEvent{
						AlertCandidateNotificationId: candidateID,
					},
				},
			}),
		})
	}
	if err := c.publishLiveEvents(ctx, publications); err != nil {
		c.logger.Warn("Failed to publish notification occurrence invalidations",
			"count", len(publications), "error", err)
	}
}

package core

import (
	"context"
	"hmans.de/chatto/internal/pb/chatto/core/live/v1"
	"hmans.de/chatto/internal/pb/chatto/core/notification/v1"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/core/subjects"
)

func (c *ChattoCore) publishNotificationOccurrencesInvalidated(ctx context.Context, occurrence *notificationv1.NotificationOccurrence, creationCandidate bool) {
	c.publishNotificationOccurrenceInvalidations(ctx, []*notificationv1.NotificationOccurrence{occurrence}, creationCandidate)
}

func (c *ChattoCore) publishNotificationOccurrenceInvalidations(ctx context.Context, occurrences []*notificationv1.NotificationOccurrence, creationCandidate bool) {
	if c == nil {
		return
	}
	soundRecipients := make([]string, 0, len(occurrences))
	if creationCandidate {
		for _, occurrence := range occurrences {
			if occurrence != nil && occurrence.GetRecipientId() != "" && !occurrence.GetRead() {
				soundRecipients = append(soundRecipients, occurrence.GetRecipientId())
			}
		}
	}
	var presences map[string]string
	if len(soundRecipients) > 0 {
		var err error
		presences, err = c.GetUserPresences(ctx, soundRecipients)
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
		var alertCandidateID, soundCandidateID *string
		if creationCandidate && !occurrence.GetRead() && presences[occurrence.GetRecipientId()] != PresenceStatusDoNotDisturb {
			soundCandidateID = proto.String(occurrence.GetId())
			if NotificationAlertPending(occurrence) {
				// Preserve the legacy candidate for older replicas, which only play
				// sound for push-eligible notification occurrences.
				alertCandidateID = proto.String(occurrence.GetId())
			}
		}
		publications = append(publications, liveEventPublication{
			subject: subjects.LiveSyncUserEvent(occurrence.GetRecipientId(), "notification_v2"),
			event: newLiveEvent(occurrence.GetActorId(), &livev1.LiveEvent{
				Event: &livev1.LiveEvent_NotificationOccurrencesInvalidated{
					NotificationOccurrencesInvalidated: &livev1.NotificationOccurrencesInvalidatedEvent{
						AlertCandidateNotificationId: alertCandidateID,
						SoundCandidateNotificationId: soundCandidateID,
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

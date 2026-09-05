package core

import (
	"context"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/pb/chatto/core/notification/v1"
	pubsubv1 "hmans.de/chatto/internal/pb/chatto/core/pubsub/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

// publishNotificationOccurrenceInvalidations reports changes to recipient state.
// Creation IDs describe facts, independent of read state or delivery policy.
func (c *ChattoCore) publishNotificationOccurrenceInvalidations(ctx context.Context, occurrences []*notificationv1.NotificationOccurrence, created bool) {
	if c == nil {
		return
	}
	publications := make([]pubsubEventPublication, 0, len(occurrences))
	for _, occurrence := range occurrences {
		if occurrence == nil || occurrence.GetRecipientId() == "" {
			continue
		}
		var createdID *string
		if created {
			createdID = proto.String(occurrence.GetId())
		}
		publications = append(publications, userPubSubEventPublication(
			occurrence.GetRecipientId(),
			newPubSubEvent(occurrence.GetActorId(), &pubsubv1.PubSubEvent{
				Event: &pubsubv1.PubSubEvent_NotificationOccurrencesChanged{
					NotificationOccurrencesChanged: &realtimev1.NotificationOccurrencesChangedEvent{
						CreatedNotificationId: createdID,
					},
				},
			}),
		))
	}
	if err := c.publishPubSubEvents(ctx, publications); err != nil {
		c.logger.Warn("Failed to publish notification occurrence invalidations",
			"count", len(publications), "error", err)
	}
}

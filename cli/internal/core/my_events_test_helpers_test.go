package core

import (
	"context"

	"github.com/nats-io/nats.go"
	pubsubv1 "hmans.de/chatto/internal/pb/chatto/core/pubsub/v1"
)

// filterPubSubEvent applies the live sync delivery rules to one recipient.
// It combines the steps that MyEventsHub.handlePubSub runs, except the
// audience check, which does not apply to a single named recipient.
func (c *ChattoCore) filterPubSubEvent(ctx context.Context, userID string, memberRooms map[string]struct{}, msg *nats.Msg, event *pubsubv1.PubSubEvent) (EventEnvelope, bool) {
	s := c.myEventsModel
	delivery, ok := s.preparePubSubEvent(msg, event)
	if !ok {
		return nil, false
	}
	if delivery.roomID != "" && !s.typingSenderVisible(ctx, event.ActorId) {
		return nil, false
	}
	return s.filterPreparedPubSubEvent(ctx, userID, memberRooms, delivery)
}

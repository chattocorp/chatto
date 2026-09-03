package core

import (
	"context"

	pubsubv1 "hmans.de/chatto/internal/pb/chatto/core/pubsub/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

// PublishTypingIndicator publishes a typing indicator event to other users in the room.
// This is a live-only event (bypasses JetStream storage).
// The threadRootEventID is optional - if set, indicates typing in a thread; if nil, typing in main room.
//
// Authorization: Caller must verify room membership before calling.
func (c *ChattoCore) PublishTypingIndicator(ctx context.Context, actorID string, kind RoomKind, roomID string, threadRootEventID *string) error {
	typingEvent := &realtimev1.UserTypingEvent{
		RoomId: roomID,
	}
	if threadRootEventID != nil {
		typingEvent.ThreadRootEventId = threadRootEventID
	}

	event := newPubSubEvent(actorID, &pubsubv1.PubSubEvent{
		Event: &pubsubv1.PubSubEvent_UserTyping{
			UserTyping: typingEvent,
		},
	})

	if err := c.publishRoomPubSubEvent(ctx, kind, roomID, event); err != nil {
		c.logger.Warn("Failed to publish typing indicator", "error", err)
		return err
	}

	return nil
}

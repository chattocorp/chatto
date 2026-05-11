package core

import (
	"context"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// PublishTypingIndicator publishes a typing indicator event to other users in the room.
// This is a live-only event (bypasses JetStream storage).
// The threadRootEventID is optional - if set, indicates typing in a thread; if nil, typing in main room.
//
// Authorization: Caller must verify room membership before calling.
func (c *ChattoCore) PublishTypingIndicator(ctx context.Context, actorID, spaceID, roomID string, threadRootEventID *string) error {
	typingEvent := &corev1.UserTypingEvent{
		SpaceId: spaceID,
		RoomId:  roomID,
	}
	if threadRootEventID != nil {
		typingEvent.ThreadRootEventId = threadRootEventID
	}

	event := newEvent(actorID, &corev1.Event{
		Event: &corev1.Event_UserTyping{
			UserTyping: typingEvent,
		},
	})

	// Publish directly to live subject (bypass JetStream)
	subject := subjects.LiveRoomEvent(kindForSpace(spaceID), roomID, "user_typing")
	if err := c.publishLiveServerEvent(ctx, subject, event); err != nil {
		c.logger.Warn("Failed to publish typing indicator", "error", err)
		return err
	}

	return nil
}

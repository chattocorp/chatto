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
func (c *ChattoCore) PublishTypingIndicator(ctx context.Context, actorID string, kind RoomKind, roomID string, threadRootEventID *string) error {
	typingEvent := &corev1.UserTypingEvent{
		RoomId: roomID,
	}
	if threadRootEventID != nil {
		typingEvent.ThreadRootEventId = threadRootEventID
	}

	event := newLiveEvent(actorID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_UserTyping{
			UserTyping: typingEvent,
		},
	})

	// Publish directly to live subject (bypass JetStream)
	subject := subjects.LiveSyncRoomEvent(string(kind), roomID, "user_typing")
	if err := c.publishLiveEvent(ctx, subject, event); err != nil {
		c.logger.Warn("Failed to publish typing indicator", "error", err)
		return err
	}

	return nil
}

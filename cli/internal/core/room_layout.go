package core

import (
	"context"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// PublishRoomGroupsUpdated publishes a live event notifying clients that the
// channel-room groups (their ordering, names, or membership) changed.
// Authorization: published to the deployment-scoped config subject, delivered
// to all authenticated users via the existing live-event authorization filter.
func (c *ChattoCore) PublishRoomGroupsUpdated(ctx context.Context, actorID string, kind RoomKind) error {
	event := newLiveEvent(actorID, &corev1.LiveEvent{
		Event: &corev1.LiveEvent_RoomGroupsUpdated{
			RoomGroupsUpdated: &corev1.RoomGroupsUpdatedEvent{},
		},
	})

	subject := subjects.LiveSyncConfigEvent("room_groups_updated")
	return c.publishLiveEvent(ctx, subject, event)
}

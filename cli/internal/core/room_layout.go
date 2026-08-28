package core

import (
	"context"
	"hmans.de/chatto/internal/pb/chatto/core/live/v1"

	"hmans.de/chatto/internal/core/subjects"
)

// PublishRoomGroupsUpdated publishes a live event notifying clients that the
// channel-room groups (their ordering, names, or membership) changed.
// Authorization: published to the deployment-scoped config subject, delivered
// to all authenticated users via the existing live-event authorization filter.
func (c *ChattoCore) PublishRoomGroupsUpdated(ctx context.Context, actorID string, kind RoomKind) error {
	event := newLiveEvent(actorID, &livev1.LiveEvent{
		Event: &livev1.LiveEvent_RoomGroupsUpdated{
			RoomGroupsUpdated: &livev1.RoomGroupsUpdatedEvent{},
		},
	})

	subject := subjects.LiveSyncConfigEvent("room_groups_updated")
	return c.publishLiveEvent(ctx, subject, event)
}

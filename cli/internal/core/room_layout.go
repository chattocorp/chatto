package core

import (
	"context"
	"slices"

	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// roomLayoutKey is the KV key for the room layout document within the space CONFIG bucket.
const roomLayoutKey = "room_layout"

// maxLayoutRetries is the maximum number of OCC retry attempts for room layout updates.
const maxLayoutRetries = 5

// removeRoomFromLayout removes a room ID from every group document
// (best-effort). Called when a room is deleted to keep group docs
// consistent. The layout's `group_ids` ordering is not touched —
// only per-group `room_ids` lists.
func (c *ChattoCore) removeRoomFromLayout(ctx context.Context, kind RoomKind, roomID string) {
	if kind != KindChannel {
		return
	}
	docs, err := c.listAllRoomGroupDocs(ctx)
	if err != nil {
		c.logger.Warn("removeRoomFromLayout: list groups", "error", err)
		return
	}
	for groupID, g := range docs {
		if !slices.Contains(g.RoomIds, roomID) {
			continue
		}
		if err := c.mutateRoomGroup(ctx, groupID, func(g *corev1.RoomGroup) error {
			g.RoomIds = slices.DeleteFunc(g.RoomIds, func(id string) bool { return id == roomID })
			return nil
		}); err != nil {
			c.logger.Warn("removeRoomFromLayout: prune group",
				"error", err, "group_id", groupID, "room_id", roomID)
		}
	}
}

// PublishRoomGroupsUpdated publishes a live event notifying clients that the
// channel-room groups (their ordering, names, or membership) changed.
// Authorization: published to the deployment-scoped config subject, delivered
// to all authenticated users via the existing live-event authorization filter.
func (c *ChattoCore) PublishRoomGroupsUpdated(ctx context.Context, actorID string, kind RoomKind) error {
	event := &corev1.Event{
		CreatedAt: timestamppb.Now(),
		ActorId:   actorID,
		Event: &corev1.Event_RoomGroupsUpdated{
			RoomGroupsUpdated: &corev1.RoomGroupsUpdatedEvent{
				SpaceId: SpaceIDForKind(kind),
			},
		},
	}

	subject := subjects.LiveConfigEvent("room_groups_updated")
	return c.publishLiveEvent(ctx, subject, event)
}

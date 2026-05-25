package core

import (
	"context"

	"google.golang.org/protobuf/types/known/timestamppb"

	"hmans.de/chatto/internal/core/subjects"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// roomLayoutKey identifies the legacy KV key that used to hold the
// room layout document. Phase 6 stopped writing it; the constant
// stays around because the boot-time migration in
// internal/migrations/room_layout_es.go still reads from it. KV
// itself is retained for rollback per ADR-035's deferred phase 7.
const roomLayoutKey = "room_layout"

// PublishRoomGroupsUpdated publishes a live event notifying clients that the
// channel-room groups (their ordering, names, or membership) changed.
// Authorization: published to the deployment-scoped config subject, delivered
// to all authenticated users via the existing live-event authorization filter.
func (c *ChattoCore) PublishRoomGroupsUpdated(ctx context.Context, actorID string, kind RoomKind) error {
	event := &corev1.Event{
		CreatedAt: timestamppb.Now(),
		ActorId:   actorID,
		Event: &corev1.Event_RoomGroupsUpdated{
			RoomGroupsUpdated: &corev1.RoomGroupsUpdatedEvent{},
		},
	}

	subject := subjects.LiveConfigEvent("room_groups_updated")
	return c.publishLiveEvent(ctx, subject, event)
}

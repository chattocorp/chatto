package graph

import (
	"fmt"

	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// convertNotification converts a protobuf Notification to a GraphQL NotificationItem.
func convertNotification(notif *corev1.Notification) (model.NotificationItem, error) {
	switch n := notif.Notification.(type) {
	case *corev1.Notification_DmMessage:
		return &model.DMMessageNotificationItem{
			ID:        notif.Id,
			CreatedAt: notif.CreatedAt,
			ActorID:   notif.ActorId,
			RoomID:    n.DmMessage.RoomId,
		}, nil

	case *corev1.Notification_Mention:
		var inThread *string
		if n.Mention.InThread != "" {
			inThread = &n.Mention.InThread
		}
		return &model.MentionNotificationItem{
			ID:        notif.Id,
			CreatedAt: notif.CreatedAt,
			ActorID:   notif.ActorId,
			SpaceID:   n.Mention.SpaceId,
			RoomID:    n.Mention.RoomId,
			EventID:   n.Mention.EventId,
			InThread:  inThread,
		}, nil

	case *corev1.Notification_Reply:
		var inThread *string
		if n.Reply.InThread != "" {
			inThread = &n.Reply.InThread
		}
		return &model.ReplyNotificationItem{
			ID:          notif.Id,
			CreatedAt:   notif.CreatedAt,
			ActorID:     notif.ActorId,
			SpaceID:     n.Reply.SpaceId,
			RoomID:      n.Reply.RoomId,
			EventID:     n.Reply.EventId,
			InReplyToID: n.Reply.InReplyToId,
			InThread:    inThread,
		}, nil

	case *corev1.Notification_RoomMessage:
		return &model.RoomMessageNotificationItem{
			ID:        notif.Id,
			CreatedAt: notif.CreatedAt,
			ActorID:   notif.ActorId,
			SpaceID:   n.RoomMessage.SpaceId,
			RoomID:    n.RoomMessage.RoomId,
			EventID:   n.RoomMessage.EventId,
		}, nil

	default:
		return nil, fmt.Errorf("unknown notification type: %T", notif.Notification)
	}
}

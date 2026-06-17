package graph

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/graph/model"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

func emptyNotificationsConnection() *model.NotificationsConnection {
	return &model.NotificationsConnection{
		Items:      []model.NotificationItem{},
		TotalCount: 0,
		HasMore:    false,
	}
}

func (r *Resolver) resolveNotificationsConnection(ctx context.Context, userID string, limit *int32, offset *int32, matches func(*corev1.Notification) bool) (*model.NotificationsConnection, error) {
	notifications, err := r.getNotifications(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get notifications: %w", err)
	}

	filtered := notifications
	if matches != nil {
		filtered = make([]*corev1.Notification, 0, len(notifications))
		for _, notif := range notifications {
			if matches(notif) {
				filtered = append(filtered, notif)
			}
		}
	}

	limitVal, offsetVal := paginationArgs(limit, offset, 50, 100)
	page, totalCount, hasMore := paginateSlice(filtered, limitVal, offsetVal)

	items := make([]model.NotificationItem, 0, len(page))
	for _, notif := range page {
		item, err := convertNotification(notif)
		if err != nil {
			r.logger.Warn("Failed to convert notification", "id", notif.Id, "error", err)
			continue
		}
		items = append(items, item)
	}

	return &model.NotificationsConnection{
		Items:      items,
		TotalCount: int32(totalCount),
		HasMore:    hasMore,
	}, nil
}

func notificationTargetRoomID(notif *corev1.Notification) string {
	if notif == nil {
		return ""
	}
	switch n := notif.Notification.(type) {
	case *corev1.Notification_DmMessage:
		return n.DmMessage.GetRoomId()
	case *corev1.Notification_Mention:
		return n.Mention.GetRoomId()
	case *corev1.Notification_Reply:
		return n.Reply.GetRoomId()
	case *corev1.Notification_RoomMessage:
		return n.RoomMessage.GetRoomId()
	default:
		return ""
	}
}

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
		var threadRootEventID *string
		if n.Mention.InThread != "" {
			threadRootEventID = &n.Mention.InThread
		}
		return &model.MentionNotificationItem{
			ID:                notif.Id,
			CreatedAt:         notif.CreatedAt,
			ActorID:           notif.ActorId,
			RoomID:            n.Mention.RoomId,
			EventID:           n.Mention.EventId,
			ThreadRootEventID: threadRootEventID,
		}, nil

	case *corev1.Notification_Reply:
		var threadRootEventID *string
		if n.Reply.InThread != "" {
			threadRootEventID = &n.Reply.InThread
		}
		return &model.ReplyNotificationItem{
			ID:                notif.Id,
			CreatedAt:         notif.CreatedAt,
			ActorID:           notif.ActorId,
			RoomID:            n.Reply.RoomId,
			EventID:           n.Reply.EventId,
			InReplyToID:       n.Reply.InReplyToId,
			ThreadRootEventID: threadRootEventID,
		}, nil

	case *corev1.Notification_RoomMessage:
		return &model.RoomMessageNotificationItem{
			ID:        notif.Id,
			CreatedAt: notif.CreatedAt,
			ActorID:   notif.ActorId,
			RoomID:    n.RoomMessage.RoomId,
			EventID:   n.RoomMessage.EventId,
		}, nil

	default:
		return nil, fmt.Errorf("unknown notification type: %T", notif.Notification)
	}
}

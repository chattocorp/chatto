package http_server

import (
	"context"
	"fmt"

	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	wirev1 "hmans.de/chatto/internal/pb/chatto/wire/v1"
)

func (c *wireConn) handleWireListNotifications(ctx context.Context, userID, requestID string, body *apiv1.ListNotificationsRequest) (*apiv1.ListNotificationsResponse, *wirev1.WireError) {
	notifications, err := c.server.core.GetNotifications(ctx, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}

	limit, offset := wirePaginationArgs(int(body.GetLimit()), int(body.GetOffset()), 50, 100)
	totalCount := len(notifications)
	end := offset + limit
	if offset > totalCount {
		offset = totalCount
	}
	if end > totalCount {
		end = totalCount
	}

	page := notifications[offset:end]
	items := make([]*apiv1.NotificationItemView, 0, len(page))
	for _, notif := range page {
		item, err := c.notificationItemView(ctx, notif)
		if err != nil {
			c.server.logger.Warn("Failed to convert notification for wire response", "notification_id", notif.GetId(), "error", err)
			continue
		}
		if item != nil {
			items = append(items, item)
		}
	}

	serverName := "Chatto"
	if cm := c.server.core.ConfigManager(); cm != nil {
		name, err := cm.GetEffectiveServerName(ctx)
		if err != nil {
			return nil, c.errorFromRequestErr(requestID, err)
		}
		serverName = name
	}

	return &apiv1.ListNotificationsResponse{
		Items:      items,
		TotalCount: int32(totalCount),
		HasMore:    offset+len(page) < totalCount,
		ServerName: serverName,
	}, nil
}

func (c *wireConn) handleWireHasNotifications(ctx context.Context, userID, requestID string) (*apiv1.HasNotificationsResponse, *wirev1.WireError) {
	hasNotifications, err := c.server.core.HasUnreadNotifications(ctx, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.HasNotificationsResponse{HasNotifications: hasNotifications}, nil
}

func (c *wireConn) handleWireDismissNotification(ctx context.Context, userID, requestID string, body *apiv1.DismissNotificationRequest) (*apiv1.DismissNotificationResponse, *wirev1.WireError) {
	if body.GetNotificationId() == "" {
		return nil, c.errorFromRequestErr(requestID, fmt.Errorf("%w: notification_id is required", errWireInvalidArgument))
	}
	dismissed, err := c.server.core.DismissNotification(ctx, userID, body.GetNotificationId())
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.DismissNotificationResponse{Dismissed: dismissed}, nil
}

func (c *wireConn) handleWireDismissAllNotifications(ctx context.Context, userID, requestID string) (*apiv1.DismissAllNotificationsResponse, *wirev1.WireError) {
	count, err := c.server.core.DismissAllNotifications(ctx, userID)
	if err != nil {
		return nil, c.errorFromRequestErr(requestID, err)
	}
	return &apiv1.DismissAllNotificationsResponse{DismissedCount: int32(count)}, nil
}

func (c *wireConn) notificationItemView(ctx context.Context, notif *corev1.Notification) (*apiv1.NotificationItemView, error) {
	if notif == nil {
		return nil, nil
	}

	view := &apiv1.NotificationItemView{
		Id:        notif.GetId(),
		CreatedAt: cloneTimestamp(notif.GetCreatedAt()),
		Actor:     c.optionalUser(ctx, notif.GetActorId()),
	}

	switch n := notif.GetNotification().(type) {
	case *corev1.Notification_DmMessage:
		view.Kind = apiv1.NotificationKind_NOTIFICATION_KIND_DM_MESSAGE
		view.RoomId = n.DmMessage.GetRoomId()
		view.EventId = optionalString(n.DmMessage.GetEventId())
		view.Summary = notificationSummary(view.Actor, "sent you a message", "New message")
		c.hydrateNotificationRoom(ctx, view, core.KindDM)
	case *corev1.Notification_Mention:
		view.Kind = apiv1.NotificationKind_NOTIFICATION_KIND_MENTION
		view.RoomId = n.Mention.GetRoomId()
		view.EventId = optionalString(n.Mention.GetEventId())
		view.ThreadRootEventId = optionalString(n.Mention.GetInThread())
		view.Summary = notificationSummary(view.Actor, "mentioned you", "You were mentioned")
		c.hydrateNotificationRoom(ctx, view, "")
	case *corev1.Notification_Reply:
		view.Kind = apiv1.NotificationKind_NOTIFICATION_KIND_REPLY
		view.RoomId = n.Reply.GetRoomId()
		view.EventId = optionalString(n.Reply.GetEventId())
		view.InReplyToId = optionalString(n.Reply.GetInReplyToId())
		view.ThreadRootEventId = optionalString(n.Reply.GetInThread())
		view.Summary = notificationSummary(view.Actor, "replied to your message", "New reply to your message")
		c.hydrateNotificationRoom(ctx, view, "")
	case *corev1.Notification_RoomMessage:
		view.Kind = apiv1.NotificationKind_NOTIFICATION_KIND_ROOM_MESSAGE
		view.RoomId = n.RoomMessage.GetRoomId()
		view.EventId = optionalString(n.RoomMessage.GetEventId())
		view.Summary = notificationSummary(view.Actor, "posted a message", "New message")
		c.hydrateNotificationRoom(ctx, view, core.KindChannel)
	default:
		return nil, fmt.Errorf("unknown notification type: %T", notif.GetNotification())
	}

	return view, nil
}

func (c *wireConn) hydrateNotificationRoom(ctx context.Context, view *apiv1.NotificationItemView, kind core.RoomKind) {
	if view == nil || view.GetRoomId() == "" {
		return
	}

	var room *corev1.Room
	var err error
	if kind == "" {
		room, err = c.server.core.FindRoomByID(ctx, view.GetRoomId())
	} else {
		room, err = c.server.core.GetRoom(ctx, kind, view.GetRoomId())
	}
	if err != nil || room == nil {
		return
	}
	view.RoomName = room.GetName()
}

func notificationSummary(actor *corev1.User, suffix, fallback string) string {
	if actor == nil || actor.GetDisplayName() == "" {
		return fallback
	}
	return fmt.Sprintf("%s %s", actor.GetDisplayName(), suffix)
}

func wirePaginationArgs(limit, offset, defaultLimit, maxLimit int) (int, int) {
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}

package connectapi

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	defaultNotificationLimit = 50
	maxNotificationLimit     = 100
)

type notificationService struct {
	api *API
}

func (s *notificationService) ListNotifications(ctx context.Context, req *connect.Request[apiv1.ListNotificationsRequest]) (*connect.Response[apiv1.ListNotificationsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	return s.notificationPage(ctx, caller.UserID, req.Msg.GetPage(), nil)
}

func (s *notificationService) ListRoomNotifications(ctx context.Context, req *connect.Request[apiv1.ListRoomNotificationsRequest]) (*connect.Response[apiv1.ListRoomNotificationsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	notifications, err := s.api.core.GetRoomNotificationsForMember(ctx, caller.UserID, req.Msg.GetRoomId())
	if err != nil {
		return nil, connectError(err)
	}
	page, err := newNotificationAssembler(s.api).pageFromList(ctx, notifications, req.Msg.GetPage())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.ListRoomNotificationsResponse{
		Items:      page.GetItems(),
		ServerName: page.GetServerName(),
		Page:       page.GetPage(),
	}), nil
}

func (s *notificationService) HasNotifications(ctx context.Context, _ *connect.Request[apiv1.HasNotificationsRequest]) (*connect.Response[apiv1.HasNotificationsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	has, err := s.api.core.HasUnreadNotifications(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.HasNotificationsResponse{HasNotifications: has}), nil
}

func (s *notificationService) ListNotificationCounts(ctx context.Context, _ *connect.Request[apiv1.ListNotificationCountsRequest]) (*connect.Response[apiv1.ListNotificationCountsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	notifications, err := s.api.core.GetNotifications(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	countsByRoom := make(map[string]int32)
	for _, notification := range notifications {
		roomID := notificationTargetRoomID(notification)
		if roomID == "" {
			continue
		}
		countsByRoom[roomID]++
	}
	roomCounts := make([]*apiv1.RoomNotificationCount, 0, len(countsByRoom))
	for roomID, count := range countsByRoom {
		roomCounts = append(roomCounts, &apiv1.RoomNotificationCount{
			RoomId:     roomID,
			TotalCount: count,
		})
	}
	return connect.NewResponse(&apiv1.ListNotificationCountsResponse{RoomCounts: roomCounts}), nil
}

func (s *notificationService) DismissNotification(ctx context.Context, req *connect.Request[apiv1.DismissNotificationRequest]) (*connect.Response[apiv1.DismissNotificationResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if req.Msg.NotificationId == "" {
		return nil, invalidArgument("notification_id is required")
	}
	dismissed, err := s.api.core.DismissNotification(ctx, caller.UserID, req.Msg.NotificationId)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.DismissNotificationResponse{Dismissed: dismissed}), nil
}

func (s *notificationService) DismissAllNotifications(ctx context.Context, _ *connect.Request[apiv1.DismissAllNotificationsRequest]) (*connect.Response[apiv1.DismissAllNotificationsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	count, err := s.api.core.DismissAllNotifications(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.DismissAllNotificationsResponse{DismissedCount: int32(count)}), nil
}

func (s *notificationService) notificationPage(ctx context.Context, userID string, pageRequest *apiv1.PageRequest, matches func(*corev1.Notification) bool) (*connect.Response[apiv1.ListNotificationsResponse], error) {
	notifications, err := s.api.core.GetNotifications(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}

	filtered := notifications
	if matches != nil {
		filtered = make([]*corev1.Notification, 0, len(notifications))
		for _, notification := range notifications {
			if matches(notification) {
				filtered = append(filtered, notification)
			}
		}
	}

	page, err := newNotificationAssembler(s.api).pageFromList(ctx, filtered, pageRequest)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(page), nil
}

func notificationSummary(actor *apiv1.UserProfile, notification *corev1.Notification) string {
	actorName := ""
	if actor != nil && actor.GetUser() != nil {
		actorName = actor.GetUser().GetDisplayName()
	}
	switch notification.GetNotification().(type) {
	case *corev1.Notification_DmMessage:
		if actorName == "" {
			return "New message"
		}
		return fmt.Sprintf("%s sent you a message", actorName)
	case *corev1.Notification_Mention:
		if actorName == "" {
			return "You were mentioned"
		}
		return fmt.Sprintf("%s mentioned you", actorName)
	case *corev1.Notification_Reply:
		if actorName == "" {
			return "New reply to your message"
		}
		return fmt.Sprintf("%s replied to your message", actorName)
	case *corev1.Notification_RoomMessage:
		if actorName == "" {
			return "New message"
		}
		return fmt.Sprintf("%s posted a message", actorName)
	default:
		return "New notification"
	}
}

func notificationTargetRoomID(notification *corev1.Notification) string {
	if notification == nil {
		return ""
	}
	switch payload := notification.GetNotification().(type) {
	case *corev1.Notification_DmMessage:
		return payload.DmMessage.GetRoomId()
	case *corev1.Notification_Mention:
		return payload.Mention.GetRoomId()
	case *corev1.Notification_Reply:
		return payload.Reply.GetRoomId()
	case *corev1.Notification_RoomMessage:
		return payload.RoomMessage.GetRoomId()
	default:
		return ""
	}
}

func paginateNotifications(notifications []*corev1.Notification, limit, offset int) ([]*corev1.Notification, int, bool) {
	total := len(notifications)
	if offset >= total {
		return []*corev1.Notification{}, total, false
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return notifications[offset:end], total, end < total
}

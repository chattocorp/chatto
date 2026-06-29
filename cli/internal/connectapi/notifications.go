package connectapi

import (
	"context"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
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
	page, err := s.notificationPageFromList(ctx, notifications, req.Msg.GetPage())
	if err != nil {
		return nil, err
	}
	return connect.NewResponse(&apiv1.ListRoomNotificationsResponse{
		Items:      page.Msg.GetItems(),
		ServerName: page.Msg.GetServerName(),
		Page:       page.Msg.GetPage(),
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

	return s.notificationPageFromList(ctx, filtered, pageRequest)
}

func (s *notificationService) notificationPageFromList(ctx context.Context, notifications []*corev1.Notification, pageRequest *apiv1.PageRequest) (*connect.Response[apiv1.ListNotificationsResponse], error) {
	limitVal, offsetVal := apiPagination(pageRequest, defaultNotificationLimit, maxNotificationLimit)
	page, totalCount, hasMore := paginateNotifications(notifications, limitVal, offsetVal)
	items := make([]*apiv1.NotificationItem, 0, len(page))
	for _, notification := range page {
		item, err := s.apiNotificationItem(ctx, notification)
		if err != nil {
			return nil, err
		}
		if item != nil {
			items = append(items, item)
		}
	}

	response := s.emptyPage(ctx)
	response.Items = items
	response.Page = apiPageInfo(totalCount, hasMore)
	return connect.NewResponse(response), nil
}

func (s *notificationService) emptyPage(ctx context.Context) *apiv1.ListNotificationsResponse {
	name := "Chatto"
	if cm := s.api.core.ConfigManager(); cm != nil {
		if configuredName, err := cm.GetEffectiveServerName(ctx); err == nil && configuredName != "" {
			name = configuredName
		}
	}
	return &apiv1.ListNotificationsResponse{
		Items:      []*apiv1.NotificationItem{},
		ServerName: name,
	}
}

func (s *notificationService) apiNotificationItem(ctx context.Context, notification *corev1.Notification) (*apiv1.NotificationItem, error) {
	if notification == nil {
		return nil, nil
	}

	actor, err := s.notificationActor(ctx, notification.GetActorId())
	if err != nil {
		return nil, err
	}
	item := &apiv1.NotificationItem{
		Id:        notification.GetId(),
		CreatedAt: notification.GetCreatedAt(),
		Actor:     actor,
		Summary:   notificationSummary(actor, notification),
	}

	switch payload := notification.GetNotification().(type) {
	case *corev1.Notification_DmMessage:
		item.Kind = &apiv1.NotificationItem_DirectMessage{
			DirectMessage: &apiv1.DirectMessageNotification{
				RoomId:  payload.DmMessage.GetRoomId(),
				EventId: payload.DmMessage.GetEventId(),
			},
		}
	case *corev1.Notification_Mention:
		room, err := s.notificationRoom(ctx, payload.Mention.GetRoomId())
		if err != nil {
			return nil, err
		}
		mention := &apiv1.MentionNotification{
			Room:    room,
			EventId: payload.Mention.GetEventId(),
		}
		if threadID := payload.Mention.GetInThread(); threadID != "" {
			mention.ThreadRootEventId = &threadID
		}
		item.Kind = &apiv1.NotificationItem_Mention{Mention: mention}
	case *corev1.Notification_Reply:
		room, err := s.notificationRoom(ctx, payload.Reply.GetRoomId())
		if err != nil {
			return nil, err
		}
		reply := &apiv1.ReplyNotification{
			Room:        room,
			EventId:     payload.Reply.GetEventId(),
			InReplyToId: payload.Reply.GetInReplyToId(),
		}
		if threadID := payload.Reply.GetInThread(); threadID != "" {
			reply.ThreadRootEventId = &threadID
		}
		item.Kind = &apiv1.NotificationItem_Reply{Reply: reply}
	case *corev1.Notification_RoomMessage:
		room, err := s.notificationRoom(ctx, payload.RoomMessage.GetRoomId())
		if err != nil {
			return nil, err
		}
		item.Kind = &apiv1.NotificationItem_RoomMessage{
			RoomMessage: &apiv1.RoomMessageNotification{
				Room:    room,
				EventId: payload.RoomMessage.GetEventId(),
			},
		}
	default:
		return nil, connectError(fmt.Errorf("unknown notification type %T", notification.GetNotification()))
	}

	return item, nil
}

func (s *notificationService) notificationActor(ctx context.Context, userID string) (*apiv1.UserProfile, error) {
	if userID == "" {
		return nil, nil
	}
	user, err := s.api.core.GetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, nil
		}
		return nil, connectError(err)
	}
	presence, err := s.api.core.GetUserPresence(ctx, userID)
	if err != nil {
		return nil, connectError(err)
	}
	actor := &apiv1.UserProfile{
		User: &apiv1.User{
			Id:          user.GetId(),
			Login:       user.GetLogin(),
			DisplayName: user.GetDisplayName(),
			Deleted:     user.GetDeleted(),
		},
		PresenceStatus: corePresenceStatusToAPI(presence),
		CustomStatus:   coreCustomStatusToAPI(user.GetCustomStatus()),
	}
	if avatarURL, err := s.api.core.GetUserAvatarURL(ctx, userID, nil, nil, ""); err != nil {
		return nil, connectError(err)
	} else if avatarURL != "" {
		actor.User.AvatarUrl = stringPtr(s.api.absolutizeAssetURL(ctx, avatarURL))
	}
	return actor, nil
}

func (s *notificationService) notificationRoom(ctx context.Context, roomID string) (*apiv1.NotificationRoom, error) {
	if roomID == "" {
		return nil, nil
	}
	room, err := s.api.core.FindRoomByID(ctx, roomID)
	if err != nil {
		return nil, connectError(err)
	}
	return &apiv1.NotificationRoom{
		Id:   room.GetId(),
		Name: room.GetName(),
	}, nil
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

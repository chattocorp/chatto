package connectapi

import (
	"context"
	"errors"
	"fmt"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/parallel"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

type notificationAssembler struct {
	api *API
}

func newNotificationAssembler(api *API) *notificationAssembler {
	return &notificationAssembler{api: api}
}

func (a *notificationAssembler) pageFromList(ctx context.Context, notifications []*corev1.Notification, pageRequest *apiv1.PageRequest) (*apiv1.ListNotificationsResponse, error) {
	limitVal, offsetVal := apiPagination(pageRequest, defaultNotificationLimit, maxNotificationLimit)
	page, totalCount, hasMore := paginateNotifications(notifications, limitVal, offsetVal)
	hydrated, err := parallel.MapNonNil(ctx, maxConnectAPIHydrationConcurrency, page, func(ctx context.Context, _ int, notification *corev1.Notification) (*apiv1.NotificationItem, error) {
		return a.item(ctx, notification)
	})
	if err != nil {
		return nil, err
	}

	response := a.emptyPage(ctx)
	response.Notifications = hydrated
	response.Page = apiPageInfo(totalCount, hasMore)
	return response, nil
}

func (a *notificationAssembler) emptyPage(_ context.Context) *apiv1.ListNotificationsResponse {
	return &apiv1.ListNotificationsResponse{
		Notifications: []*apiv1.NotificationItem{},
	}
}

func (a *notificationAssembler) item(ctx context.Context, notification *corev1.Notification) (*apiv1.NotificationItem, error) {
	if notification == nil {
		return nil, nil
	}

	actor, err := a.actor(ctx, notification.GetActorId())
	if err != nil {
		return nil, err
	}
	item := &apiv1.NotificationItem{
		Id:        notification.GetId(),
		CreatedAt: notification.GetCreatedAt(),
		Actor:     actor,
	}

	switch payload := notification.GetNotification().(type) {
	case *corev1.Notification_DmMessage:
		room, err := a.room(ctx, payload.DmMessage.GetRoomId())
		if err != nil {
			return nil, err
		}
		item.Kind = &apiv1.NotificationItem_DirectMessage{
			DirectMessage: &apiv1.DirectMessageNotification{
				EventId: payload.DmMessage.GetEventId(),
				Room:    room,
			},
		}
	case *corev1.Notification_Mention:
		room, err := a.room(ctx, payload.Mention.GetRoomId())
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
		room, err := a.room(ctx, payload.Reply.GetRoomId())
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
		room, err := a.room(ctx, payload.RoomMessage.GetRoomId())
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
		return nil, fmt.Errorf("unknown notification type %T", notification.GetNotification())
	}

	return item, nil
}

func (a *notificationAssembler) actor(ctx context.Context, userID string) (*apiv1.User, error) {
	if userID == "" {
		return nil, nil
	}
	user, err := a.api.core.GetUser(ctx, userID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	actor, err := (&userService{api: a.api}).userSummary(ctx, user, nil)
	if err != nil {
		return nil, err
	}
	return actor, nil
}

func (a *notificationAssembler) room(ctx context.Context, roomID string) (*apiv1.RoomSummary, error) {
	if roomID == "" {
		return nil, nil
	}
	room, err := a.api.core.FindRoomByID(ctx, roomID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return &apiv1.RoomSummary{Id: roomID}, nil
		}
		return nil, err
	}
	return apiRoomSummary(room), nil
}

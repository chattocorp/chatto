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
	items, err := parallel.MapNonNil(ctx, maxConnectAPIHydrationConcurrency, page, func(ctx context.Context, _ int, notification *corev1.Notification) (*apiv1.NotificationItem, error) {
		return a.item(ctx, notification)
	})
	if err != nil {
		return nil, err
	}

	response := a.emptyPage(ctx)
	response.Items = items
	response.Page = apiPageInfo(totalCount, hasMore)
	return response, nil
}

func (a *notificationAssembler) emptyPage(ctx context.Context) *apiv1.ListNotificationsResponse {
	name := "Chatto"
	if cm := a.api.core.ConfigManager(); cm != nil {
		if configuredName, err := cm.GetEffectiveServerName(ctx); err == nil && configuredName != "" {
			name = configuredName
		}
	}
	return &apiv1.ListNotificationsResponse{
		Items:      []*apiv1.NotificationItem{},
		ServerName: name,
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

func (a *notificationAssembler) actor(ctx context.Context, userID string) (*apiv1.UserProfile, error) {
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
	presence, err := a.api.core.GetUserPresence(ctx, userID)
	if err != nil {
		return nil, err
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
	if avatarURL, err := a.api.core.GetUserAvatarURL(ctx, userID, nil, nil, ""); err != nil {
		return nil, err
	} else if avatarURL != "" {
		actor.User.AvatarUrl = stringPtr(a.api.absolutizeAssetURL(ctx, avatarURL))
	}
	return actor, nil
}

func (a *notificationAssembler) room(ctx context.Context, roomID string) (*apiv1.NotificationRoom, error) {
	if roomID == "" {
		return nil, nil
	}
	room, err := a.api.core.FindRoomByID(ctx, roomID)
	if err != nil {
		return nil, err
	}
	return &apiv1.NotificationRoom{
		Id:   room.GetId(),
		Name: room.GetName(),
	}, nil
}

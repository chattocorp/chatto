package connectapi

import (
	"context"
	"errors"

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

func (a *notificationAssembler) actor(ctx context.Context, userID, presence string) (*apiv1.User, error) {
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
	return userSummaryWithPresence(ctx, a.api, user, nil, presence)
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

func (a *notificationAssembler) occurrence(ctx context.Context, occurrence *corev1.NotificationOccurrence) (*apiv1.NotificationOccurrence, error) {
	if occurrence == nil {
		return nil, nil
	}
	presence, err := a.api.core.GetUserPresence(ctx, occurrence.GetActorId())
	if err != nil {
		return nil, err
	}
	return a.occurrenceWithPresentation(ctx, occurrence, presence)
}

func (a *notificationAssembler) occurrenceWithPresentation(ctx context.Context, occurrence *corev1.NotificationOccurrence, presence string) (*apiv1.NotificationOccurrence, error) {
	if occurrence == nil {
		return nil, nil
	}
	actor, err := a.actor(ctx, occurrence.GetActorId(), presence)
	if err != nil {
		return nil, err
	}
	room, err := a.room(ctx, occurrence.GetTarget().GetRoomId())
	if err != nil {
		return nil, err
	}
	target := &apiv1.NotificationTarget{Room: room, EventId: occurrence.GetTarget().GetEventId()}
	if value := occurrence.GetTarget().GetThreadRootEventId(); value != "" {
		target.ThreadRootEventId = &value
	}
	if value := occurrence.GetTarget().GetParentEventId(); value != "" {
		target.ParentEventId = &value
	}
	reasons := make([]*apiv1.NotificationReasonMatch, 0, len(occurrence.GetReasons()))
	for _, match := range occurrence.GetReasons() {
		reasons = append(reasons, &apiv1.NotificationReasonMatch{
			Reason:    apiv1.NotificationReason(match.GetReason()),
			Intensity: apiv1.NotificationDeliveryIntensity(match.GetIntensity()),
		})
	}
	return &apiv1.NotificationOccurrence{
		Id:                 occurrence.GetId(),
		SourceEventId:      occurrence.GetSourceEventId(),
		CreatedAt:          occurrence.GetSourceCreatedAt(),
		Actor:              actor,
		Target:             target,
		Reasons:            reasons,
		StrongestIntensity: apiv1.NotificationDeliveryIntensity(occurrence.GetStrongestIntensity()),
		AttentionLevel:     apiv1.NotificationAttentionLevel(core.NotificationOccurrenceAttentionLevel(occurrence)),
		Unread:             occurrence.GetInboxState() == corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD,
		ReactionEmoji:      occurrence.GetReactionEmoji(),
		ExpiresAt:          occurrence.GetExpiresAt(),
	}, nil
}

func (a *notificationAssembler) occurrences(ctx context.Context, occurrences []*corev1.NotificationOccurrence) ([]*apiv1.NotificationOccurrence, error) {
	actorIDs := make([]string, 0)
	for _, occurrence := range occurrences {
		if actorID := occurrence.GetActorId(); actorID != "" {
			actorIDs = append(actorIDs, actorID)
		}
	}
	presences, err := a.api.core.GetUserPresences(ctx, actorIDs)
	if err != nil {
		return nil, err
	}
	return parallel.MapNonNil(ctx, maxConnectAPIHydrationConcurrency, occurrences, func(ctx context.Context, _ int, occurrence *corev1.NotificationOccurrence) (*apiv1.NotificationOccurrence, error) {
		return a.occurrenceWithPresentation(
			ctx,
			occurrence,
			presences[occurrence.GetActorId()],
		)
	})
}

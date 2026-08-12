package connectapi

import (
	"context"
	"errors"
	"strings"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/parallel"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	notificationThreadRootExcerptMaxRunes = 180
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
	excerpt, err := a.threadRootExcerpt(ctx, occurrence.GetTarget().GetThreadRootEventId())
	if err != nil {
		return nil, err
	}
	return a.occurrenceWithPresentation(ctx, occurrence, presence, excerpt)
}

func (a *notificationAssembler) occurrenceWithPresentation(ctx context.Context, occurrence *corev1.NotificationOccurrence, presence string, threadRootExcerpt *string) (*apiv1.NotificationOccurrence, error) {
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
		Id:                       occurrence.GetId(),
		SourceEventId:            occurrence.GetSourceEventId(),
		CreatedAt:                occurrence.GetSourceCreatedAt(),
		Actor:                    actor,
		Target:                   target,
		Reasons:                  reasons,
		StrongestIntensity:       apiv1.NotificationDeliveryIntensity(occurrence.GetStrongestIntensity()),
		Unread:                   occurrence.GetInboxState() == corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD,
		ReactionEmoji:            occurrence.GetReactionEmoji(),
		ExpiresAt:                occurrence.GetExpiresAt(),
		ThreadRootMessageExcerpt: threadRootExcerpt,
	}, nil
}

func (a *notificationAssembler) occurrences(ctx context.Context, occurrences []*corev1.NotificationOccurrence) ([]*apiv1.NotificationOccurrence, error) {
	actorIDs := make([]string, 0)
	threadRootIDs := make(map[string]struct{})
	for _, occurrence := range occurrences {
		if actorID := occurrence.GetActorId(); actorID != "" {
			actorIDs = append(actorIDs, actorID)
		}
		if threadRootID := occurrence.GetTarget().GetThreadRootEventId(); threadRootID != "" {
			threadRootIDs[threadRootID] = struct{}{}
		}
	}
	presences, err := a.api.core.GetUserPresences(ctx, actorIDs)
	if err != nil {
		return nil, err
	}
	threadRootIDList := make([]string, 0, len(threadRootIDs))
	for threadRootID := range threadRootIDs {
		threadRootIDList = append(threadRootIDList, threadRootID)
	}
	excerpts, err := parallel.Map(ctx, maxConnectAPIHydrationConcurrency, threadRootIDList, func(ctx context.Context, _ int, threadRootID string) (*string, error) {
		return a.threadRootExcerpt(ctx, threadRootID)
	})
	if err != nil {
		return nil, err
	}
	excerptsByThreadRootID := make(map[string]*string, len(threadRootIDList))
	for index, threadRootID := range threadRootIDList {
		excerptsByThreadRootID[threadRootID] = excerpts[index]
	}
	return parallel.MapNonNil(ctx, maxConnectAPIHydrationConcurrency, occurrences, func(ctx context.Context, _ int, occurrence *corev1.NotificationOccurrence) (*apiv1.NotificationOccurrence, error) {
		return a.occurrenceWithPresentation(
			ctx,
			occurrence,
			presences[occurrence.GetActorId()],
			excerptsByThreadRootID[occurrence.GetTarget().GetThreadRootEventId()],
		)
	})
}

// threadRootExcerpt hydrates presentation text only after the containing
// occurrence passed current target visibility checks. The excerpt is never
// copied into persisted notification state.
func (a *notificationAssembler) threadRootExcerpt(ctx context.Context, threadRootEventID string) (*string, error) {
	if threadRootEventID == "" {
		return nil, nil
	}
	body, err := a.api.core.GetFullMessageBody(ctx, threadRootEventID)
	if err != nil {
		if errors.Is(err, core.ErrMessageBodyCorrupt) {
			return nil, nil
		}
		return nil, err
	}
	if body == nil {
		return nil, nil
	}
	excerpt := notificationThreadRootExcerpt(body.Body)
	if excerpt == "" {
		return nil, nil
	}
	return &excerpt, nil
}

func notificationThreadRootExcerpt(body string) string {
	excerpt := strings.Join(strings.Fields(body), " ")
	runes := []rune(excerpt)
	if len(runes) > notificationThreadRootExcerptMaxRunes {
		return strings.TrimSpace(string(runes[:notificationThreadRootExcerptMaxRunes-1])) + "…"
	}
	return excerpt
}

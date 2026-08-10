package connectapi

import (
	"context"
	"sort"

	"hmans.de/chatto/internal/core"
	"hmans.de/chatto/internal/parallel"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const notificationGroupOccurrencePreviewLimit = 20

func (a *notificationAssembler) occurrence(ctx context.Context, occurrence *corev1.NotificationOccurrence) (*apiv1.NotificationOccurrence, error) {
	if occurrence == nil {
		return nil, nil
	}
	presence, err := a.api.core.GetUserPresence(ctx, occurrence.GetActorId())
	if err != nil {
		return nil, err
	}
	return a.occurrenceWithPresence(ctx, occurrence, presence)
}

func (a *notificationAssembler) occurrenceWithPresence(ctx context.Context, occurrence *corev1.NotificationOccurrence, presence string) (*apiv1.NotificationOccurrence, error) {
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
		InboxState:         apiv1.NotificationInboxState(occurrence.GetInboxState()),
		ExpiresAt:          occurrence.GetExpiresAt(),
	}, nil
}

func (a *notificationAssembler) occurrences(ctx context.Context, occurrences []*corev1.NotificationOccurrence) ([]*apiv1.NotificationOccurrence, error) {
	actorIDs := make([]string, 0, len(occurrences))
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
		return a.occurrenceWithPresence(ctx, occurrence, presences[occurrence.GetActorId()])
	})
}

func (a *notificationAssembler) groups(ctx context.Context, groups []core.NotificationOccurrenceGroup) ([]*apiv1.NotificationGroup, error) {
	actorIDs := make([]string, 0)
	for _, group := range groups {
		for _, occurrence := range group.Occurrences {
			if actorID := occurrence.GetActorId(); actorID != "" {
				actorIDs = append(actorIDs, actorID)
			}
		}
	}
	presences, err := a.api.core.GetUserPresences(ctx, actorIDs)
	if err != nil {
		return nil, err
	}
	return parallel.MapNonNil(ctx, maxConnectAPIHydrationConcurrency, groups, func(ctx context.Context, _ int, group core.NotificationOccurrenceGroup) (*apiv1.NotificationGroup, error) {
		return a.groupWithPresences(ctx, group, presences)
	})
}

func (a *notificationAssembler) groupWithPresences(ctx context.Context, group core.NotificationOccurrenceGroup, presences map[string]string) (*apiv1.NotificationGroup, error) {
	if len(group.Occurrences) == 0 {
		return nil, nil
	}
	unread := false
	canUnsubscribe := false
	strongest := corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_UNSPECIFIED
	reasonSet := make(map[corev1.NotificationReason]struct{})
	var openOccurrence *corev1.NotificationOccurrence
	var nextExpiry *corev1.NotificationOccurrence
	for _, occurrence := range group.Occurrences {
		if occurrence.GetInboxState() == corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD {
			unread = true
			if openOccurrence == nil {
				openOccurrence = occurrence
			}
		}
		if nextExpiry == nil || occurrence.GetExpiresAt().AsTime().Before(nextExpiry.GetExpiresAt().AsTime()) {
			nextExpiry = occurrence
		}
		if occurrence.GetStrongestIntensity() > strongest {
			strongest = occurrence.GetStrongestIntensity()
		}
		for _, match := range occurrence.GetReasons() {
			reasonSet[match.GetReason()] = struct{}{}
			active := match.GetIntensity() > corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_OFF
			canUnsubscribe = canUnsubscribe || active && (match.GetReason() == corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_THREAD || match.GetReason() == corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_ROOM)
		}
	}
	if openOccurrence == nil {
		openOccurrence = group.Occurrences[0]
	}
	previewCount := min(len(group.Occurrences), notificationGroupOccurrencePreviewLimit)
	preview := append([]*corev1.NotificationOccurrence(nil), group.Occurrences[:previewCount]...)
	openInPreview := false
	for _, occurrence := range preview {
		if occurrence.GetId() == openOccurrence.GetId() {
			openInPreview = true
			break
		}
	}
	if !openInPreview {
		preview[len(preview)-1] = openOccurrence
	}
	items, err := parallel.MapNonNil(ctx, maxConnectAPIHydrationConcurrency, preview, func(ctx context.Context, _ int, occurrence *corev1.NotificationOccurrence) (*apiv1.NotificationOccurrence, error) {
		return a.occurrenceWithPresence(ctx, occurrence, presences[occurrence.GetActorId()])
	})
	if err != nil {
		return nil, err
	}
	openPreviewIndex := 0
	for index, occurrence := range preview {
		if occurrence.GetId() == openOccurrence.GetId() {
			openPreviewIndex = index
		}
	}
	reasons := make([]apiv1.NotificationReason, 0, len(reasonSet))
	for reason := range reasonSet {
		reasons = append(reasons, apiv1.NotificationReason(reason))
	}
	sort.Slice(reasons, func(i, j int) bool { return reasons[i] < reasons[j] })
	return &apiv1.NotificationGroup{
		Id:                 group.ID,
		Occurrences:        items,
		OpenTarget:         items[openPreviewIndex].GetTarget(),
		Unread:             unread,
		OccurrenceCount:    int32(len(group.Occurrences)),
		LatestAt:           items[0].GetCreatedAt(),
		StrongestIntensity: apiv1.NotificationDeliveryIntensity(strongest),
		Reasons:            reasons,
		CanUnsubscribe:     canUnsubscribe,
		NextExpiryAt:       nextExpiry.GetExpiresAt(),
		OpenNotificationId: openOccurrence.GetId(),
	}, nil
}

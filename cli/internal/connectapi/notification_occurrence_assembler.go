package connectapi

import (
	"context"
	"sort"

	"hmans.de/chatto/internal/core"
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
		Saved:              occurrence.GetSaved(),
		ExpiresAt:          occurrence.GetExpiresAt(),
	}, nil
}

func (a *notificationAssembler) group(ctx context.Context, group core.NotificationOccurrenceGroup) (*apiv1.NotificationGroup, error) {
	if len(group.Occurrences) == 0 {
		return nil, nil
	}
	unread := false
	allSaved := true
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
		allSaved = allSaved && occurrence.GetSaved()
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
	items := make([]*apiv1.NotificationOccurrence, 0, len(preview))
	openPreviewIndex := 0
	for _, occurrence := range preview {
		item, err := a.occurrence(ctx, occurrence)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
		if occurrence.GetId() == openOccurrence.GetId() {
			openPreviewIndex = len(items) - 1
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
		AllSaved:           allSaved,
		CanUnsubscribe:     canUnsubscribe,
		NextExpiryAt:       nextExpiry.GetExpiresAt(),
		OpenNotificationId: openOccurrence.GetId(),
	}, nil
}

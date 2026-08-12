package connectapi

import (
	"context"
	"sort"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
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

func (s *notificationService) waitForCurrentOccurrences(ctx context.Context) error {
	if err := s.api.core.NotificationOccurrences().WaitCurrent(ctx); err != nil {
		return connectError(err)
	}
	return nil
}

func (s *notificationService) ListNotificationOccurrences(ctx context.Context, req *connect.Request[apiv1.ListNotificationOccurrencesRequest]) (*connect.Response[apiv1.ListNotificationOccurrencesResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.waitForCurrentOccurrences(ctx); err != nil {
		return nil, err
	}
	occurrences, err := s.api.core.NotificationOccurrences().List(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	occurrences, err = s.visibleNotificationOccurrences(ctx, caller.UserID, occurrences)
	if err != nil {
		return nil, connectError(err)
	}
	limit, offset := apiPagination(req.Msg.GetPage(), defaultNotificationLimit, maxNotificationLimit)
	total := len(occurrences)
	if offset > total {
		offset = total
	}
	end := min(total, offset+limit)
	page := occurrences[offset:end]
	assembler := newNotificationAssembler(s.api)
	hydrated, err := assembler.occurrences(ctx, page)
	if err != nil {
		return nil, connectError(err)
	}
	unreadCount, nextExpiryAt, roomCounts := notificationSummary(occurrences)
	return connect.NewResponse(&apiv1.ListNotificationOccurrencesResponse{
		Occurrences:      hydrated,
		Page:             apiPageInfo(total, end < total),
		UnreadCount:      unreadCount,
		NextExpiryAt:     nextExpiryAt,
		RoomUnreadCounts: roomCounts,
	}), nil
}

func notificationSummary(occurrences []*corev1.NotificationOccurrence) (int32, *timestamppb.Timestamp, []*apiv1.NotificationRoomUnreadCount) {
	unreadCount := int32(0)
	roomCounts := make(map[string]int32)
	var nextExpiryAt *timestamppb.Timestamp
	for _, occurrence := range occurrences {
		if nextExpiryAt == nil || occurrence.GetExpiresAt().AsTime().Before(nextExpiryAt.AsTime()) {
			nextExpiryAt = occurrence.GetExpiresAt()
		}
		if occurrence.GetInboxState() == corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD {
			unreadCount++
			if roomID := occurrence.GetTarget().GetRoomId(); roomID != "" {
				roomCounts[roomID]++
			}
		}
	}
	roomIDs := make([]string, 0, len(roomCounts))
	for roomID := range roomCounts {
		roomIDs = append(roomIDs, roomID)
	}
	sort.Strings(roomIDs)
	result := make([]*apiv1.NotificationRoomUnreadCount, 0, len(roomIDs))
	for _, roomID := range roomIDs {
		result = append(result, &apiv1.NotificationRoomUnreadCount{
			RoomId:      roomID,
			UnreadCount: roomCounts[roomID],
		})
	}
	return unreadCount, nextExpiryAt, result
}

func (s *notificationService) visibleNotificationOccurrences(ctx context.Context, userID string, occurrences []*corev1.NotificationOccurrence) ([]*corev1.NotificationOccurrence, error) {
	allowedOccurrences, err := s.api.core.NotificationOccurrences().VisibleOccurrences(ctx, userID, occurrences)
	if err != nil {
		return nil, err
	}
	allowedIDs := make(map[string]struct{}, len(allowedOccurrences))
	for _, occurrence := range allowedOccurrences {
		allowedIDs[occurrence.GetId()] = struct{}{}
	}
	visible := make([]*corev1.NotificationOccurrence, 0, len(allowedOccurrences))
	for _, occurrence := range occurrences {
		if _, allowed := allowedIDs[occurrence.GetId()]; !allowed {
			if _, err := s.api.core.NotificationOccurrences().Delete(ctx, userID, occurrence.GetId(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_VISIBILITY_LOST); err != nil {
				return nil, err
			}
			continue
		}
		visible = append(visible, occurrence)
	}
	return visible, nil
}

func (s *notificationService) notificationOccurrenceVisible(ctx context.Context, userID string, occurrence *corev1.NotificationOccurrence) (bool, error) {
	visible, err := s.api.core.NotificationOccurrences().VisibleOccurrences(ctx, userID, []*corev1.NotificationOccurrence{occurrence})
	return len(visible) == 1, err
}

func (s *notificationService) MarkNotificationRead(ctx context.Context, req *connect.Request[apiv1.MarkNotificationReadRequest]) (*connect.Response[apiv1.MarkNotificationReadResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.waitForCurrentOccurrences(ctx); err != nil {
		return nil, err
	}
	existing, err := s.api.core.NotificationOccurrences().Get(ctx, caller.UserID, req.Msg.GetNotificationId())
	if err != nil {
		return nil, connectError(err)
	}
	visible, err := s.notificationOccurrenceVisible(ctx, caller.UserID, existing)
	if err != nil {
		return nil, connectError(err)
	}
	if !visible {
		_, _ = s.api.core.NotificationOccurrences().Delete(ctx, caller.UserID, existing.GetId(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_VISIBILITY_LOST)
		return nil, connectError(core.ErrNotFound)
	}
	occurrence, err := s.api.core.NotificationOccurrences().MarkRead(ctx, caller.UserID, req.Msg.GetNotificationId())
	if err != nil {
		return nil, connectError(err)
	}
	item, err := newNotificationAssembler(s.api).occurrence(ctx, occurrence)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.MarkNotificationReadResponse{Notification: item}), nil
}

func (s *notificationService) DeleteNotificationOccurrence(ctx context.Context, req *connect.Request[apiv1.DeleteNotificationOccurrenceRequest]) (*connect.Response[apiv1.DeleteNotificationOccurrenceResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.waitForCurrentOccurrences(ctx); err != nil {
		return nil, err
	}
	deleted, err := s.api.core.NotificationOccurrences().Delete(
		ctx,
		caller.UserID,
		req.Msg.GetNotificationId(),
		corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_DELETED,
	)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.DeleteNotificationOccurrenceResponse{Deleted: deleted}), nil
}

func (s *notificationService) BatchDeleteNotificationOccurrences(ctx context.Context, req *connect.Request[apiv1.BatchDeleteNotificationOccurrencesRequest]) (*connect.Response[apiv1.BatchDeleteNotificationOccurrencesResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.waitForCurrentOccurrences(ctx); err != nil {
		return nil, err
	}
	count, err := s.api.core.NotificationOccurrences().DeleteMany(ctx, caller.UserID, req.Msg.GetNotificationIds())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.BatchDeleteNotificationOccurrencesResponse{DeletedCount: int32(count)}), nil
}

func (s *notificationService) DeleteAllNotificationOccurrences(ctx context.Context, _ *connect.Request[apiv1.DeleteAllNotificationOccurrencesRequest]) (*connect.Response[apiv1.DeleteAllNotificationOccurrencesResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.waitForCurrentOccurrences(ctx); err != nil {
		return nil, err
	}
	count, err := s.api.core.NotificationOccurrences().DeleteAll(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.DeleteAllNotificationOccurrencesResponse{DeletedCount: int32(count)}), nil
}

func apiNotificationPolicy(roomID string, policy []core.NotificationPolicyPreference) (*apiv1.GetNotificationPolicyResponse, *apiv1.SetNotificationPolicyPreferenceResponse) {
	preferences := make([]*apiv1.NotificationPolicyPreference, 0, len(policy))
	for _, preference := range policy {
		preferences = append(preferences, &apiv1.NotificationPolicyPreference{
			Reason:             apiv1.NotificationReason(preference.Reason),
			ServerIntensity:    apiv1.NotificationDeliveryIntensity(preference.ServerIntensity),
			RoomIntensity:      apiv1.NotificationDeliveryIntensity(preference.RoomIntensity),
			EffectiveIntensity: apiv1.NotificationDeliveryIntensity(preference.Effective),
		})
	}
	get := &apiv1.GetNotificationPolicyResponse{Preferences: preferences}
	set := &apiv1.SetNotificationPolicyPreferenceResponse{Preferences: preferences}
	if roomID != "" {
		get.RoomId = &roomID
		set.RoomId = &roomID
	}
	return get, set
}

func (s *notificationService) GetNotificationPolicy(ctx context.Context, req *connect.Request[apiv1.GetNotificationPolicyRequest]) (*connect.Response[apiv1.GetNotificationPolicyResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	roomID := req.Msg.GetRoomId()
	policy, err := s.api.core.NotificationPolicy().GetNotificationPolicy(ctx, caller.UserID, roomID)
	if err != nil {
		return nil, connectError(err)
	}
	response, _ := apiNotificationPolicy(roomID, policy)
	return connect.NewResponse(response), nil
}

func (s *notificationService) SetNotificationPolicyPreference(ctx context.Context, req *connect.Request[apiv1.SetNotificationPolicyPreferenceRequest]) (*connect.Response[apiv1.SetNotificationPolicyPreferenceResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	roomID := req.Msg.GetRoomId()
	reason := corev1.NotificationReason(req.Msg.GetReason())
	intensity := corev1.NotificationDeliveryIntensity(req.Msg.GetIntensity())
	var policy []core.NotificationPolicyPreference
	if roomID == "" {
		policy, err = s.api.core.NotificationPolicy().SetServerNotificationIntensity(ctx, caller.UserID, reason, intensity)
	} else {
		policy, err = s.api.core.NotificationPolicy().SetRoomNotificationIntensity(ctx, caller.UserID, roomID, reason, intensity)
	}
	if err != nil {
		return nil, connectError(err)
	}
	_, response := apiNotificationPolicy(roomID, policy)
	return connect.NewResponse(response), nil
}

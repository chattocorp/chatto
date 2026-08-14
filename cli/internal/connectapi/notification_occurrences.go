package connectapi

import (
	"context"
	"errors"
	"sort"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/proto"
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
	// assembleOccurrence is an unexported failure-injection seam for proving
	// that response hydration completes before a triage mutation commits.
	assembleOccurrence func(context.Context, *corev1.NotificationOccurrence) (*apiv1.NotificationOccurrence, error)
}

func (s *notificationService) hydratedOccurrence(ctx context.Context, occurrence *corev1.NotificationOccurrence) (*apiv1.NotificationOccurrence, error) {
	if s.assembleOccurrence != nil {
		return s.assembleOccurrence(ctx, occurrence)
	}
	return newNotificationAssembler(s.api).occurrence(ctx, occurrence)
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
	summary := notificationSummary(occurrences)
	return connect.NewResponse(&apiv1.ListNotificationOccurrencesResponse{
		Occurrences:          hydrated,
		Page:                 apiPageInfo(total, end < total),
		UnreadCount:          summary.unreadCount,
		NextExpiryAt:         summary.nextExpiryAt,
		RoomUnreadCounts:     summary.roomCounts,
		ImportantUnreadCount: proto.Int32(summary.importantUnreadCount),
	}), nil
}

type notificationOccurrenceSummary struct {
	unreadCount          int32
	importantUnreadCount int32
	nextExpiryAt         *timestamppb.Timestamp
	roomCounts           []*apiv1.NotificationRoomUnreadCount
}

type notificationRoomSummary struct {
	unreadCount          int32
	importantUnreadCount int32
}

func notificationSummary(occurrences []*corev1.NotificationOccurrence) notificationOccurrenceSummary {
	summary := notificationOccurrenceSummary{}
	roomCounts := make(map[string]notificationRoomSummary)
	for _, occurrence := range occurrences {
		if summary.nextExpiryAt == nil || occurrence.GetExpiresAt().AsTime().Before(summary.nextExpiryAt.AsTime()) {
			summary.nextExpiryAt = occurrence.GetExpiresAt()
		}
		if occurrence.GetInboxState() == corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD {
			summary.unreadCount++
			important := core.NotificationOccurrenceAttentionLevel(occurrence) == corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_IMPORTANT
			if important {
				summary.importantUnreadCount++
			}
			if roomID := occurrence.GetTarget().GetRoomId(); roomID != "" {
				room := roomCounts[roomID]
				room.unreadCount++
				if important {
					room.importantUnreadCount++
				}
				roomCounts[roomID] = room
			}
		}
	}
	roomIDs := make([]string, 0, len(roomCounts))
	for roomID := range roomCounts {
		roomIDs = append(roomIDs, roomID)
	}
	sort.Strings(roomIDs)
	summary.roomCounts = make([]*apiv1.NotificationRoomUnreadCount, 0, len(roomIDs))
	for _, roomID := range roomIDs {
		room := roomCounts[roomID]
		summary.roomCounts = append(summary.roomCounts, &apiv1.NotificationRoomUnreadCount{
			RoomId:               roomID,
			UnreadCount:          room.unreadCount,
			ImportantUnreadCount: proto.Int32(room.importantUnreadCount),
		})
	}
	return summary
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

func (s *notificationService) deleteVisibleNotificationOccurrences(ctx context.Context, userID string, occurrences []*corev1.NotificationOccurrence) (int, error) {
	visible, err := s.visibleNotificationOccurrences(ctx, userID, occurrences)
	if err != nil {
		return 0, err
	}
	ids := make([]string, 0, len(visible))
	for _, occurrence := range visible {
		ids = append(ids, occurrence.GetId())
	}
	return s.api.core.NotificationOccurrences().DeleteMany(ctx, userID, ids)
}

func (s *notificationService) notificationOccurrencesByID(ctx context.Context, userID string, occurrenceIDs []string) ([]*corev1.NotificationOccurrence, error) {
	occurrences := make([]*corev1.NotificationOccurrence, 0, len(occurrenceIDs))
	seen := make(map[string]struct{}, len(occurrenceIDs))
	for _, occurrenceID := range occurrenceIDs {
		if _, duplicate := seen[occurrenceID]; duplicate {
			continue
		}
		seen[occurrenceID] = struct{}{}
		occurrence, err := s.api.core.NotificationOccurrences().Get(ctx, userID, occurrenceID)
		if errors.Is(err, core.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		occurrences = append(occurrences, occurrence)
	}
	return occurrences, nil
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
	// Hydration may consult unrelated projections and fail. Complete it before
	// committing the triage mutation so an error response never ambiguously
	// means that the server may already have marked the occurrence read.
	item, err := s.hydratedOccurrence(ctx, existing)
	if err != nil {
		return nil, connectError(err)
	}
	_, err = s.api.core.NotificationOccurrences().MarkRead(ctx, caller.UserID, req.Msg.GetNotificationId())
	if err != nil {
		return nil, connectError(err)
	}
	item.Unread = false
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
	existing, err := s.api.core.NotificationOccurrences().Get(ctx, caller.UserID, req.Msg.GetNotificationId())
	if errors.Is(err, core.ErrNotFound) {
		return connect.NewResponse(&apiv1.DeleteNotificationOccurrenceResponse{}), nil
	}
	if err != nil {
		return nil, connectError(err)
	}
	deleted, err := s.deleteVisibleNotificationOccurrences(ctx, caller.UserID, []*corev1.NotificationOccurrence{existing})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.DeleteNotificationOccurrenceResponse{Deleted: deleted == 1}), nil
}

func (s *notificationService) BatchDeleteNotificationOccurrences(ctx context.Context, req *connect.Request[apiv1.BatchDeleteNotificationOccurrencesRequest]) (*connect.Response[apiv1.BatchDeleteNotificationOccurrencesResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.waitForCurrentOccurrences(ctx); err != nil {
		return nil, err
	}
	occurrences, err := s.notificationOccurrencesByID(ctx, caller.UserID, req.Msg.GetNotificationIds())
	if err != nil {
		return nil, connectError(err)
	}
	count, err := s.deleteVisibleNotificationOccurrences(ctx, caller.UserID, occurrences)
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
	occurrences, err := s.api.core.NotificationOccurrences().List(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	count, err := s.deleteVisibleNotificationOccurrences(ctx, caller.UserID, occurrences)
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

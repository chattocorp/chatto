package connectapi

import (
	"context"
	"errors"
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

func (s *notificationService) GetNotificationOccurrence(ctx context.Context, req *connect.Request[apiv1.GetNotificationOccurrenceRequest]) (*connect.Response[apiv1.GetNotificationOccurrenceResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.waitForCurrentOccurrences(ctx); err != nil {
		return nil, err
	}
	occurrence, err := s.api.core.NotificationOccurrences().Get(ctx, caller.UserID, req.Msg.GetNotificationId())
	if err != nil {
		return nil, connectError(err)
	}
	if err := requireSupportedNotificationSignals(occurrence); err != nil {
		return nil, err
	}
	visible, err := s.notificationOccurrenceVisible(ctx, caller.UserID, occurrence)
	if err != nil {
		return nil, connectError(err)
	}
	if !visible {
		_, _ = s.api.core.NotificationOccurrences().Delete(ctx, caller.UserID, occurrence.GetId())
		return nil, connectError(core.ErrNotFound)
	}
	hydrated, err := s.hydratedOccurrence(ctx, occurrence)
	if err != nil {
		return nil, connectError(err)
	}
	if hydrated == nil {
		return nil, connectError(core.ErrNotFound)
	}
	return connect.NewResponse(&apiv1.GetNotificationOccurrenceResponse{Occurrence: hydrated}), nil
}

func (s *notificationService) BatchGetNotificationOccurrences(ctx context.Context, req *connect.Request[apiv1.BatchGetNotificationOccurrencesRequest]) (*connect.Response[apiv1.BatchGetNotificationOccurrencesResponse], error) {
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
	if err := requireSupportedNotificationSignals(occurrences...); err != nil {
		return nil, err
	}
	visible, err := s.visibleNotificationOccurrences(ctx, caller.UserID, occurrences)
	if err != nil {
		return nil, connectError(err)
	}
	hydrated, err := newNotificationAssembler(s.api).occurrences(ctx, visible)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.BatchGetNotificationOccurrencesResponse{Occurrences: hydrated}), nil
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
	if err := requireSupportedNotificationSignals(occurrences...); err != nil {
		return nil, err
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
		ImportantUnreadCount: summary.importantUnreadCount,
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
		if !occurrence.GetRead() {
			summary.unreadCount++
			// Unknown future levels are conservatively Important.
			important := occurrence.GetAttentionLevel() != corev1.NotificationAttentionLevel_NOTIFICATION_ATTENTION_LEVEL_AMBIENT
			if important {
				summary.importantUnreadCount++
			}
			if message := core.NotificationOccurrenceMessageReference(occurrence); message != nil && message.GetRoomId() != "" {
				roomID := message.GetRoomId()
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
			ImportantUnreadCount: room.importantUnreadCount,
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
			if core.NotificationOccurrenceHasUnsupportedSignal(occurrence) {
				continue
			}
			if _, err := s.api.core.NotificationOccurrences().Delete(ctx, userID, occurrence.GetId()); err != nil {
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
	if err := requireSupportedNotificationSignals(occurrences...); err != nil {
		return 0, err
	}
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

func requireSupportedNotificationSignals(occurrences ...*corev1.NotificationOccurrence) error {
	for _, occurrence := range occurrences {
		if core.NotificationOccurrenceHasUnsupportedSignal(occurrence) {
			return connect.NewError(
				connect.CodeUnimplemented,
				errors.New("notification signal is not supported by this server version"),
			)
		}
	}
	return nil
}

func (s *notificationService) notificationOccurrencesByID(ctx context.Context, userID string, occurrenceIDs []string) ([]*corev1.NotificationOccurrence, error) {
	all, err := s.api.core.NotificationOccurrences().List(ctx, userID)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]*corev1.NotificationOccurrence, len(all))
	for _, occurrence := range all {
		byID[occurrence.GetId()] = occurrence
	}
	occurrences := make([]*corev1.NotificationOccurrence, 0, len(occurrenceIDs))
	seen := make(map[string]struct{}, len(occurrenceIDs))
	for _, occurrenceID := range occurrenceIDs {
		if _, duplicate := seen[occurrenceID]; duplicate {
			continue
		}
		seen[occurrenceID] = struct{}{}
		if occurrence := byID[occurrenceID]; occurrence != nil {
			occurrences = append(occurrences, occurrence)
		}
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
	if err := requireSupportedNotificationSignals(existing); err != nil {
		return nil, err
	}
	visible, err := s.notificationOccurrenceVisible(ctx, caller.UserID, existing)
	if err != nil {
		return nil, connectError(err)
	}
	if !visible {
		_, _ = s.api.core.NotificationOccurrences().Delete(ctx, caller.UserID, existing.GetId())
		return nil, connectError(core.ErrNotFound)
	}
	// Hydration may consult unrelated projections and fail. Complete it before
	// committing the triage mutation so an error response never ambiguously
	// means that the server may already have marked the occurrence read.
	item, err := s.hydratedOccurrence(ctx, existing)
	if err != nil {
		return nil, connectError(err)
	}
	if item == nil {
		return nil, connectError(core.ErrNotFound)
	}
	_, err = s.api.core.NotificationOccurrences().MarkRead(ctx, caller.UserID, req.Msg.GetNotificationId())
	if err != nil {
		return nil, connectError(err)
	}
	item.Unread = false
	return connect.NewResponse(&apiv1.MarkNotificationReadResponse{Occurrence: item}), nil
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

func apiNotificationDeliveryMode(mode corev1.NotificationDeliveryMode) (apiv1.NotificationDeliveryMode, error) {
	switch mode {
	case corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF:
		return apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF, nil
	case corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE:
		return apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE, nil
	case corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION:
		return apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION, nil
	case corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION:
		return apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION, nil
	default:
		return apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED, core.ErrInvalidArgument
	}
}

func apiNotificationDeliveryModes(modes *corev1.NotificationDeliveryModes) (*apiv1.NotificationDeliveryModes, error) {
	if modes == nil {
		return &apiv1.NotificationDeliveryModes{}, nil
	}
	result := &apiv1.NotificationDeliveryModes{}
	set := func(source *corev1.NotificationDeliveryMode, target **apiv1.NotificationDeliveryMode) error {
		if source == nil {
			return nil
		}
		mapped, err := apiNotificationDeliveryMode(*source)
		if err != nil {
			return err
		}
		*target = &mapped
		return nil
	}
	for _, pair := range []struct {
		source *corev1.NotificationDeliveryMode
		target **apiv1.NotificationDeliveryMode
	}{{modes.DirectMessages, &result.DirectMessages}, {modes.DirectMentions, &result.DirectMentions}, {modes.Replies, &result.Replies}, {modes.RoleMentions, &result.RoleMentions}, {modes.HereMentions, &result.HereMentions}, {modes.AllMentions, &result.AllMentions}, {modes.FollowedThreads, &result.FollowedThreads}, {modes.FollowedRooms, &result.FollowedRooms}, {modes.Reactions, &result.Reactions}, {modes.RoomMessages, &result.RoomMessages}} {
		if err := set(pair.source, pair.target); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func coreNotificationDeliveryModes(modes *apiv1.NotificationDeliveryModes) (*corev1.NotificationDeliveryModes, error) {
	if modes == nil {
		return &corev1.NotificationDeliveryModes{}, nil
	}
	result := &corev1.NotificationDeliveryModes{}
	set := func(source *apiv1.NotificationDeliveryMode, target **corev1.NotificationDeliveryMode) error {
		if source == nil {
			return nil
		}
		mapped, err := coreNotificationDeliveryMode(*source)
		if err != nil {
			return err
		}
		*target = &mapped
		return nil
	}
	for _, pair := range []struct {
		source *apiv1.NotificationDeliveryMode
		target **corev1.NotificationDeliveryMode
	}{{modes.DirectMessages, &result.DirectMessages}, {modes.DirectMentions, &result.DirectMentions}, {modes.Replies, &result.Replies}, {modes.RoleMentions, &result.RoleMentions}, {modes.HereMentions, &result.HereMentions}, {modes.AllMentions, &result.AllMentions}, {modes.FollowedThreads, &result.FollowedThreads}, {modes.FollowedRooms, &result.FollowedRooms}, {modes.Reactions, &result.Reactions}, {modes.RoomMessages, &result.RoomMessages}} {
		if err := set(pair.source, pair.target); err != nil {
			return nil, err
		}
	}
	return result, nil
}

func apiNotificationPolicy(policy *core.NotificationPolicy) (*apiv1.NotificationPolicy, error) {
	if policy == nil {
		return nil, core.ErrInvalidArgument
	}
	overrides, err := apiNotificationDeliveryModes(policy.Overrides)
	if err != nil {
		return nil, err
	}
	effective, err := apiNotificationDeliveryModes(policy.Effective)
	if err != nil {
		return nil, err
	}
	return &apiv1.NotificationPolicy{Overrides: overrides, Effective: effective}, nil
}

func coreNotificationDeliveryMode(mode apiv1.NotificationDeliveryMode) (corev1.NotificationDeliveryMode, error) {
	switch mode {
	case apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF:
		return corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_OFF, nil
	case apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE:
		return corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNREAD_BADGE, nil
	case apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION:
		return corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_IN_APP_NOTIFICATION, nil
	case apiv1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION:
		return corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_PUSH_NOTIFICATION, nil
	default:
		return corev1.NotificationDeliveryMode_NOTIFICATION_DELIVERY_MODE_UNSPECIFIED, core.ErrInvalidArgument
	}
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
	mapped, err := apiNotificationPolicy(policy)
	if err != nil {
		return nil, connectError(err)
	}
	response := &apiv1.GetNotificationPolicyResponse{Policy: mapped}
	if roomID != "" {
		response.RoomId = &roomID
	}
	return connect.NewResponse(response), nil
}

func (s *notificationService) UpdateNotificationPolicy(ctx context.Context, req *connect.Request[apiv1.UpdateNotificationPolicyRequest]) (*connect.Response[apiv1.UpdateNotificationPolicyResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	roomID := req.Msg.GetRoomId()
	overrides, err := coreNotificationDeliveryModes(req.Msg.GetOverrides())
	if err != nil {
		return nil, connectError(err)
	}
	policy, err := s.api.core.NotificationPolicy().UpdateNotificationPolicy(ctx, caller.UserID, roomID, overrides, req.Msg.GetUpdateMask())
	if err != nil {
		return nil, connectError(err)
	}
	mapped, err := apiNotificationPolicy(policy)
	if err != nil {
		return nil, connectError(err)
	}
	response := &apiv1.UpdateNotificationPolicyResponse{Policy: mapped}
	if roomID != "" {
		response.RoomId = &roomID
	}
	return connect.NewResponse(response), nil
}

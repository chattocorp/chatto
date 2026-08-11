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

func (s *notificationService) ListNotificationGroups(ctx context.Context, req *connect.Request[apiv1.ListNotificationGroupsRequest]) (*connect.Response[apiv1.ListNotificationGroupsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.waitForCurrentOccurrences(ctx); err != nil {
		return nil, err
	}
	groups, err := s.api.core.NotificationOccurrences().Groups(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	limit, offset := apiPagination(req.Msg.GetPage(), defaultNotificationLimit, maxNotificationLimit)
	page, total, hasMore, err := s.visibleNotificationPage(ctx, caller.UserID, groups, limit, offset)
	if err != nil {
		return nil, connectError(err)
	}
	assembler := newNotificationAssembler(s.api)
	hydrated, err := assembler.groups(ctx, page)
	if err != nil {
		return nil, connectError(err)
	}
	// Visibility filtering may have tombstoned stale rows. Re-read the
	// local occurrence index so summary counts reflect those writes without
	// performing another projection fence or exhaustive target validation.
	groups, err = s.api.core.NotificationOccurrences().Groups(ctx, caller.UserID)
	if err != nil {
		return nil, connectError(err)
	}
	unreadGroupCount, nextExpiryAt, roomCounts := notificationSummary(groups)
	return connect.NewResponse(&apiv1.ListNotificationGroupsResponse{
		Groups:                hydrated,
		Page:                  apiPageInfo(total, hasMore),
		UnreadGroupCount:      unreadGroupCount,
		NextExpiryAt:          nextExpiryAt,
		RoomUnreadGroupCounts: roomCounts,
	}), nil
}

// visibleNotificationPage validates only the visible prefix needed for an
// offset page. It grows by one page when stale rows are filtered, instead of
// fencing and revalidating the entire 90-day list for every request.
func (s *notificationService) visibleNotificationPage(ctx context.Context, userID string, groups []core.NotificationOccurrenceGroup, limit, offset int) ([]core.NotificationOccurrenceGroup, int, bool, error) {
	if offset >= len(groups) {
		return []core.NotificationOccurrenceGroup{}, len(groups), false, nil
	}
	targetCount := offset + limit + 1
	scanEnd := min(len(groups), targetCount)
	scanStart := 0
	visible := make([]core.NotificationOccurrenceGroup, 0, targetCount)
	for {
		batch, err := s.visibleNotificationGroups(ctx, userID, groups[scanStart:scanEnd])
		if err != nil {
			return nil, 0, false, err
		}
		visible = append(visible, batch...)
		if len(visible) >= targetCount || scanEnd == len(groups) {
			break
		}
		scanStart = scanEnd
		scanEnd = min(len(groups), scanEnd+max(limit, defaultNotificationLimit))
	}
	total := len(groups) - (scanEnd - len(visible))
	if offset >= len(visible) {
		return []core.NotificationOccurrenceGroup{}, total, scanEnd < len(groups), nil
	}
	end := min(len(visible), offset+limit)
	hasMore := len(visible) > end || scanEnd < len(groups)
	return visible[offset:end], total, hasMore, nil
}

func notificationSummary(groups []core.NotificationOccurrenceGroup) (int32, *timestamppb.Timestamp, []*apiv1.NotificationRoomUnreadGroupCount) {
	unreadGroupCount := int32(0)
	roomCounts := make(map[string]int32)
	var nextExpiryAt *timestamppb.Timestamp
	for _, group := range groups {
		groupUnread := false
		roomID := ""
		for _, occurrence := range group.Occurrences {
			if roomID == "" {
				roomID = occurrence.GetTarget().GetRoomId()
			}
			if nextExpiryAt == nil || occurrence.GetExpiresAt().AsTime().Before(nextExpiryAt.AsTime()) {
				nextExpiryAt = occurrence.GetExpiresAt()
			}
			groupUnread = groupUnread || occurrence.GetInboxState() == corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD
		}
		if groupUnread {
			unreadGroupCount++
			if roomID != "" {
				roomCounts[roomID]++
			}
		}
	}
	roomIDs := make([]string, 0, len(roomCounts))
	for roomID := range roomCounts {
		roomIDs = append(roomIDs, roomID)
	}
	sort.Strings(roomIDs)
	result := make([]*apiv1.NotificationRoomUnreadGroupCount, 0, len(roomIDs))
	for _, roomID := range roomIDs {
		result = append(result, &apiv1.NotificationRoomUnreadGroupCount{
			RoomId:           roomID,
			UnreadGroupCount: roomCounts[roomID],
		})
	}
	return unreadGroupCount, nextExpiryAt, result
}

func (s *notificationService) visibleNotificationGroups(ctx context.Context, userID string, groups []core.NotificationOccurrenceGroup) ([]core.NotificationOccurrenceGroup, error) {
	occurrences := make([]*corev1.NotificationOccurrence, 0)
	for _, group := range groups {
		occurrences = append(occurrences, group.Occurrences...)
	}
	allowedOccurrences, err := s.api.core.NotificationOccurrences().VisibleOccurrences(ctx, userID, occurrences)
	if err != nil {
		return nil, err
	}
	allowedIDs := make(map[string]struct{}, len(allowedOccurrences))
	for _, occurrence := range allowedOccurrences {
		allowedIDs[occurrence.GetId()] = struct{}{}
	}
	visible := make([]core.NotificationOccurrenceGroup, 0, len(groups))
	for _, group := range groups {
		visibleOccurrences := make([]*corev1.NotificationOccurrence, 0, len(group.Occurrences))
		for _, occurrence := range group.Occurrences {
			if _, allowed := allowedIDs[occurrence.GetId()]; !allowed {
				if _, err := s.api.core.NotificationOccurrences().Delete(ctx, userID, occurrence.GetId(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_VISIBILITY_LOST); err != nil {
					return nil, err
				}
				continue
			}
			visibleOccurrences = append(visibleOccurrences, occurrence)
		}
		if len(visibleOccurrences) == 0 {
			continue
		}
		group.Occurrences = visibleOccurrences
		visible = append(visible, group)
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

func (s *notificationService) requireVisibleNotificationGroup(ctx context.Context, userID, groupID string) error {
	groups, err := s.api.core.NotificationOccurrences().Groups(ctx, userID)
	if err != nil {
		return err
	}
	for _, group := range groups {
		if group.ID != groupID || len(group.Occurrences) == 0 {
			continue
		}
		visible, err := s.api.core.NotificationOccurrences().VisibleOccurrences(ctx, userID, group.Occurrences)
		if err != nil {
			return err
		}
		visibleIDs := make(map[string]struct{}, len(visible))
		for _, occurrence := range visible {
			visibleIDs[occurrence.GetId()] = struct{}{}
		}
		for _, occurrence := range group.Occurrences {
			if _, allowed := visibleIDs[occurrence.GetId()]; !allowed {
				if _, err := s.api.core.NotificationOccurrences().Delete(ctx, userID, occurrence.GetId(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_VISIBILITY_LOST); err != nil {
					return err
				}
			}
		}
		if len(visible) > 0 {
			return nil
		}
		return core.ErrNotFound
	}
	return core.ErrNotFound
}

func (s *notificationService) DeleteNotificationGroup(ctx context.Context, req *connect.Request[apiv1.DeleteNotificationGroupRequest]) (*connect.Response[apiv1.DeleteNotificationGroupResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.waitForCurrentOccurrences(ctx); err != nil {
		return nil, err
	}
	if err := s.requireVisibleNotificationGroup(ctx, caller.UserID, req.Msg.GetGroupId()); err != nil {
		return nil, connectError(err)
	}
	count, err := s.api.core.NotificationOccurrences().DeleteGroup(ctx, caller.UserID, req.Msg.GetGroupId())
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.DeleteNotificationGroupResponse{DeletedCount: int32(count)}), nil
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

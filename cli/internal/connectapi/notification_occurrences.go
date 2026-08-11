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

func notificationOccurrenceView(view apiv1.NotificationView) (core.NotificationOccurrenceView, error) {
	switch view {
	case apiv1.NotificationView_NOTIFICATION_VIEW_UNSPECIFIED,
		apiv1.NotificationView_NOTIFICATION_VIEW_INBOX:
		return core.NotificationOccurrenceViewInbox, nil
	case apiv1.NotificationView_NOTIFICATION_VIEW_DONE:
		return core.NotificationOccurrenceViewDone, nil
	default:
		return 0, core.ErrInvalidArgument
	}
}

func (s *notificationService) ListNotificationGroups(ctx context.Context, req *connect.Request[apiv1.ListNotificationGroupsRequest]) (*connect.Response[apiv1.ListNotificationGroupsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	view, err := notificationOccurrenceView(req.Msg.GetView())
	if err != nil {
		return nil, connectError(err)
	}
	groups, err := s.api.core.NotificationOccurrences().Groups(ctx, caller.UserID, view)
	if err != nil {
		return nil, connectError(err)
	}
	groups, err = s.visibleNotificationGroups(ctx, caller.UserID, groups)
	if err != nil {
		return nil, connectError(err)
	}
	limit, offset := apiPagination(req.Msg.GetPage(), defaultNotificationLimit, maxNotificationLimit)
	page, total, hasMore := apiSlicePage(groups, limit, offset)
	assembler := newNotificationAssembler(s.api)
	hydrated, err := assembler.groups(ctx, page)
	if err != nil {
		return nil, connectError(err)
	}
	inboxGroups := groups
	if view != core.NotificationOccurrenceViewInbox {
		inboxGroups, err = s.api.core.NotificationOccurrences().Groups(ctx, caller.UserID, core.NotificationOccurrenceViewInbox)
		if err == nil {
			inboxGroups, err = s.visibleNotificationGroups(ctx, caller.UserID, inboxGroups)
		}
		if err != nil {
			return nil, connectError(err)
		}
	}
	unreadGroupCount, nextInboxExpiryAt, roomCounts := notificationInboxSummary(inboxGroups)
	return connect.NewResponse(&apiv1.ListNotificationGroupsResponse{
		Groups:                hydrated,
		Page:                  apiPageInfo(total, hasMore),
		UnreadGroupCount:      unreadGroupCount,
		NextInboxExpiryAt:     nextInboxExpiryAt,
		RoomUnreadGroupCounts: roomCounts,
	}), nil
}

func notificationInboxSummary(groups []core.NotificationOccurrenceGroup) (int32, *timestamppb.Timestamp, []*apiv1.NotificationRoomUnreadGroupCount) {
	unreadGroupCount := int32(0)
	roomCounts := make(map[string]int32)
	var nextInboxExpiryAt *timestamppb.Timestamp
	for _, group := range groups {
		groupUnread := false
		roomID := ""
		for _, occurrence := range group.Occurrences {
			if roomID == "" {
				roomID = occurrence.GetTarget().GetRoomId()
			}
			if nextInboxExpiryAt == nil || occurrence.GetExpiresAt().AsTime().Before(nextInboxExpiryAt.AsTime()) {
				nextInboxExpiryAt = occurrence.GetExpiresAt()
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
	return unreadGroupCount, nextInboxExpiryAt, result
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
				_, _ = s.api.core.NotificationOccurrences().Delete(ctx, userID, occurrence.GetId(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_VISIBILITY_LOST)
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

func occurrenceUpdate(inboxState *apiv1.NotificationInboxState) core.UpdateNotificationOccurrenceInput {
	input := core.UpdateNotificationOccurrenceInput{}
	if inboxState != nil {
		value := corev1.NotificationInboxState(*inboxState)
		input.InboxState = &value
	}
	return input
}

func (s *notificationService) UpdateNotificationOccurrence(ctx context.Context, req *connect.Request[apiv1.UpdateNotificationOccurrenceRequest]) (*connect.Response[apiv1.UpdateNotificationOccurrenceResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
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
	occurrence, err := s.api.core.NotificationOccurrences().Update(ctx, caller.UserID, req.Msg.GetNotificationId(), occurrenceUpdate(req.Msg.InboxState))
	if err != nil {
		return nil, connectError(err)
	}
	item, err := newNotificationAssembler(s.api).occurrence(ctx, occurrence)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.UpdateNotificationOccurrenceResponse{Notification: item}), nil
}

func (s *notificationService) UpdateNotificationGroup(ctx context.Context, req *connect.Request[apiv1.UpdateNotificationGroupRequest]) (*connect.Response[apiv1.UpdateNotificationGroupResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	view, err := notificationOccurrenceView(req.Msg.GetView())
	if err != nil {
		return nil, connectError(err)
	}
	if err := s.requireVisibleNotificationGroup(ctx, caller.UserID, req.Msg.GetGroupId(), view); err != nil {
		return nil, connectError(err)
	}
	updated, err := s.api.core.NotificationOccurrences().UpdateGroup(ctx, caller.UserID, req.Msg.GetGroupId(), view, occurrenceUpdate(req.Msg.InboxState))
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.UpdateNotificationGroupResponse{UpdatedCount: int32(len(updated))}), nil
}

func (s *notificationService) requireVisibleNotificationGroup(ctx context.Context, userID, groupID string, view core.NotificationOccurrenceView) error {
	groups, err := s.api.core.NotificationOccurrences().Groups(ctx, userID, view)
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
				_, _ = s.api.core.NotificationOccurrences().Delete(ctx, userID, occurrence.GetId(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_VISIBILITY_LOST)
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
	view, err := notificationOccurrenceView(req.Msg.GetView())
	if err != nil {
		return nil, connectError(err)
	}
	count, err := s.api.core.NotificationOccurrences().DeleteGroup(ctx, caller.UserID, req.Msg.GetGroupId(), view)
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

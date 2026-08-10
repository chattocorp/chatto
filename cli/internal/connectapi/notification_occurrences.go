package connectapi

import (
	"context"
	"errors"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

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
	unreadGroupCount := 0
	var nextInboxExpiryAt *timestamppb.Timestamp
	for _, group := range inboxGroups {
		groupUnread := false
		for _, occurrence := range group.Occurrences {
			if nextInboxExpiryAt == nil || occurrence.GetExpiresAt().AsTime().Before(nextInboxExpiryAt.AsTime()) {
				nextInboxExpiryAt = occurrence.GetExpiresAt()
			}
			groupUnread = groupUnread || occurrence.GetInboxState() == corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_UNREAD
		}
		if groupUnread {
			unreadGroupCount++
		}
	}
	return connect.NewResponse(&apiv1.ListNotificationGroupsResponse{
		Groups:            hydrated,
		Page:              apiPageInfo(total, hasMore),
		UnreadGroupCount:  int32(unreadGroupCount),
		NextInboxExpiryAt: nextInboxExpiryAt,
	}), nil
}

func (s *notificationService) ListNotificationOccurrences(ctx context.Context, req *connect.Request[apiv1.ListNotificationOccurrencesRequest]) (*connect.Response[apiv1.ListNotificationOccurrencesResponse], error) {
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
	var occurrences []*corev1.NotificationOccurrence
	for _, group := range groups {
		if group.ID == req.Msg.GetGroupId() {
			occurrences = group.Occurrences
			break
		}
	}
	if occurrences == nil {
		return nil, connectError(core.ErrNotFound)
	}
	limit, offset := apiPagination(req.Msg.GetPage(), defaultNotificationLimit, maxNotificationLimit)
	page, total, hasMore := apiSlicePage(occurrences, limit, offset)
	assembler := newNotificationAssembler(s.api)
	hydrated, err := assembler.occurrences(ctx, page)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.ListNotificationOccurrencesResponse{
		Notifications: hydrated,
		Page:          apiPageInfo(total, hasMore),
	}), nil
}

func (s *notificationService) GetNotificationOccurrence(ctx context.Context, req *connect.Request[apiv1.GetNotificationOccurrenceRequest]) (*connect.Response[apiv1.GetNotificationOccurrenceResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	occurrence, err := s.api.core.NotificationOccurrences().Get(ctx, caller.UserID, req.Msg.GetNotificationId())
	if err != nil {
		return nil, connectError(err)
	}
	visible, err := s.notificationOccurrenceVisible(ctx, caller.UserID, occurrence)
	if err != nil {
		return nil, connectError(err)
	}
	if !visible {
		_, _ = s.api.core.NotificationOccurrences().Delete(ctx, caller.UserID, occurrence.GetId(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_VISIBILITY_LOST)
		return nil, connectError(core.ErrNotFound)
	}
	item, err := newNotificationAssembler(s.api).occurrence(ctx, occurrence)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.GetNotificationOccurrenceResponse{Notification: item}), nil
}

func (s *notificationService) visibleNotificationGroups(ctx context.Context, userID string, groups []core.NotificationOccurrenceGroup) ([]core.NotificationOccurrenceGroup, error) {
	visible := make([]core.NotificationOccurrenceGroup, 0, len(groups))
	for _, group := range groups {
		if len(group.Occurrences) == 0 {
			continue
		}
		allowed, err := s.notificationOccurrenceVisible(ctx, userID, group.Occurrences[0])
		if err != nil {
			return nil, err
		}
		if !allowed {
			for _, occurrence := range group.Occurrences {
				_, _ = s.api.core.NotificationOccurrences().Delete(ctx, userID, occurrence.GetId(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_VISIBILITY_LOST)
			}
			continue
		}
		visible = append(visible, group)
	}
	return visible, nil
}

func (s *notificationService) notificationOccurrenceVisible(ctx context.Context, userID string, occurrence *corev1.NotificationOccurrence) (bool, error) {
	room, err := s.api.core.FindRoomByID(ctx, occurrence.GetTarget().GetRoomId())
	if errors.Is(err, core.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	member, err := s.api.core.RoomMembershipExists(ctx, core.KindOfRoom(room), userID, room.GetId())
	return member, err
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
	if err != nil || !visible {
		if !visible {
			_, _ = s.api.core.NotificationOccurrences().Delete(ctx, caller.UserID, existing.GetId(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_VISIBILITY_LOST)
			err = core.ErrNotFound
		}
		return nil, connectError(err)
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

func (s *notificationService) DeleteNotificationOccurrence(ctx context.Context, req *connect.Request[apiv1.DeleteNotificationOccurrenceRequest]) (*connect.Response[apiv1.DeleteNotificationOccurrenceResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	deleted, err := s.api.core.NotificationOccurrences().Delete(ctx, caller.UserID, req.Msg.GetNotificationId(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_DELETED)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.DeleteNotificationOccurrenceResponse{Deleted: deleted}), nil
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
		visible, err := s.notificationOccurrenceVisible(ctx, userID, group.Occurrences[0])
		if err != nil {
			return err
		}
		if visible {
			return nil
		}
		for _, occurrence := range group.Occurrences {
			_, _ = s.api.core.NotificationOccurrences().Delete(ctx, userID, occurrence.GetId(), corev1.NotificationRemovalReason_NOTIFICATION_REMOVAL_REASON_VISIBILITY_LOST)
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

func (s *notificationService) UnsubscribeNotificationGroup(ctx context.Context, req *connect.Request[apiv1.UnsubscribeNotificationGroupRequest]) (*connect.Response[apiv1.UnsubscribeNotificationGroupResponse], error) {
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
	groups, err := s.api.core.NotificationOccurrences().Groups(ctx, caller.UserID, view)
	if err != nil {
		return nil, connectError(err)
	}
	var selected *core.NotificationOccurrenceGroup
	for index := range groups {
		if groups[index].ID == req.Msg.GetGroupId() {
			selected = &groups[index]
			break
		}
	}
	if selected == nil || len(selected.Occurrences) == 0 {
		return nil, connectError(core.ErrNotFound)
	}
	target := selected.Occurrences[0].GetTarget()
	followedThread := false
	followedRoom := false
	for _, occurrence := range selected.Occurrences {
		for _, match := range occurrence.GetReasons() {
			active := match.GetIntensity() > corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_OFF
			followedThread = followedThread || active && match.GetReason() == corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_THREAD
			followedRoom = followedRoom || active && match.GetReason() == corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_ROOM
		}
	}
	switch {
	case followedThread && target.GetThreadRootEventId() != "":
		if err := s.api.core.ThreadFollows().UnfollowThread(ctx, caller.UserID, target.GetRoomId(), target.GetThreadRootEventId()); err != nil {
			return nil, connectError(err)
		}
	case followedRoom:
		if _, err := s.api.core.NotificationPreferences().SetRoomNotificationIntensity(
			ctx,
			caller.UserID,
			target.GetRoomId(),
			corev1.NotificationReason_NOTIFICATION_REASON_FOLLOWED_ROOM,
			corev1.NotificationDeliveryIntensity_NOTIFICATION_DELIVERY_INTENSITY_OFF,
		); err != nil {
			return nil, connectError(err)
		}
	default:
		return nil, connectError(core.ErrInvalidArgument)
	}
	done := corev1.NotificationInboxState_NOTIFICATION_INBOX_STATE_DONE
	updated, err := s.api.core.NotificationOccurrences().UpdateGroup(ctx, caller.UserID, req.Msg.GetGroupId(), view, core.UpdateNotificationOccurrenceInput{InboxState: &done})
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.UnsubscribeNotificationGroupResponse{UpdatedCount: int32(len(updated))}), nil
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
	policy, err := s.api.core.NotificationPreferences().GetNotificationPolicy(ctx, caller.UserID, roomID)
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
		policy, err = s.api.core.NotificationPreferences().SetServerNotificationIntensity(ctx, caller.UserID, reason, intensity)
	} else {
		policy, err = s.api.core.NotificationPreferences().SetRoomNotificationIntensity(ctx, caller.UserID, roomID, reason, intensity)
	}
	if err != nil {
		return nil, connectError(err)
	}
	_, response := apiNotificationPolicy(roomID, policy)
	return connect.NewResponse(response), nil
}

package http_server

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/connectapi"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

func (s *HTTPServer) realtimeProjectionSnapshotFrames(ctx context.Context, userID string, timelineRoomIDs []string) ([]*realtimev1.RealtimeServerFrame, error) {
	frames := make([]*realtimev1.RealtimeServerFrame, 0)
	err := s.writeRealtimeProjectionSnapshot(ctx, userID, timelineRoomIDs, func(frame *realtimev1.RealtimeServerFrame) error {
		frames = append(frames, frame)
		return nil
	})
	return frames, err
}

// writeRealtimeProjectionSnapshot emits the compacted prefix incrementally so
// the transport does not retain a second frame graph for every decrypted room
// timeline while a reset is in flight.
func (s *HTTPServer) writeRealtimeProjectionSnapshot(ctx context.Context, userID string, timelineRoomIDs []string, writeFrame func(*realtimev1.RealtimeServerFrame) error) error {
	if s.connectAPI == nil {
		return errors.New("Connect API is unavailable")
	}
	snapshot, err := s.connectAPI.BuildRealtimeProjectionSnapshot(ctx, userID, timelineRoomIDs)
	if err != nil {
		return err
	}

	var writeErr error
	appendOperation := func(operation *realtimev1.RealtimeProjectionOperation) {
		if writeErr != nil {
			return
		}
		writeErr = writeFrame(realtimeProjectionServerFrame(&realtimev1.RealtimeProjectionEvent{
			Id:         core.NewEventID(),
			CreatedAt:  timestamppb.Now(),
			Operations: []*realtimev1.RealtimeProjectionOperation{operation},
		}))
	}
	appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_Reset_{
		Reset_: &realtimev1.RealtimeProjectionReset{},
	}})
	appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_ServerUpsert{
		ServerUpsert: snapshot.Server,
	}})
	appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_ServerStateUpsert{
		ServerStateUpsert: realtimeProjectionServerState(snapshot.ServerState),
	}})
	appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_ViewerUpsert{
		ViewerUpsert: snapshot.Viewer,
	}})
	for _, user := range snapshot.Users {
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_UserUpsert{UserUpsert: user}})
	}
	for _, room := range snapshot.Rooms {
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomUpsert{RoomUpsert: realtimeProjectionRoom(room)}})
	}
	appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomGroupsReplace{
		RoomGroupsReplace: &realtimev1.RealtimeProjectionRoomGroupsReplace{Groups: snapshot.RoomGroups},
	}})
	appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_NotificationsReplace{
		NotificationsReplace: realtimeProjectionNotifications(snapshot.Notifications),
	}})
	appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_ActiveCallsReplace{
		ActiveCallsReplace: &realtimev1.RealtimeProjectionActiveCallsReplace{Calls: snapshot.ActiveCalls},
	}})
	for _, timeline := range snapshot.Timelines {
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomTimelineReplace{
			RoomTimelineReplace: &realtimev1.RealtimeProjectionRoomTimelineReplace{RoomId: timeline.RoomID, Page: timeline.Page, EventCursors: timeline.EventCursors},
		}})
	}
	return writeErr
}

func realtimeProjectionServerFrame(event *realtimev1.RealtimeProjectionEvent) *realtimev1.RealtimeServerFrame {
	return &realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_ProjectionEvent{ProjectionEvent: event}}
}

func (s *HTTPServer) realtimeProjectionRoomTimelineFrame(ctx context.Context, viewerID, roomID string) (*realtimev1.RealtimeServerFrame, error) {
	room, err := s.connectAPI.BuildRealtimeProjectionRoom(ctx, viewerID, roomID)
	if err != nil {
		return nil, err
	}
	if !room.Room.GetViewerState().GetIsMember() {
		return nil, core.ErrNotRoomMember
	}
	timeline, err := s.connectAPI.BuildRealtimeProjectionRoomTimeline(ctx, viewerID, roomID)
	if err != nil {
		return nil, err
	}
	return realtimeProjectionServerFrame(&realtimev1.RealtimeProjectionEvent{
		Id:        core.NewEventID(),
		CreatedAt: timestamppb.Now(),
		Operations: []*realtimev1.RealtimeProjectionOperation{
			{Operation: &realtimev1.RealtimeProjectionOperation_RoomUpsert{RoomUpsert: realtimeProjectionRoom(room)}},
			{Operation: &realtimev1.RealtimeProjectionOperation_RoomTimelineReplace{
				RoomTimelineReplace: &realtimev1.RealtimeProjectionRoomTimelineReplace{
					RoomId: roomID, Page: timeline.Page, EventCursors: timeline.EventCursors,
				},
			}},
		},
	}), nil
}

// realtimeProjectionReconciliationFrame captures latest-value viewer state
// that is not fully represented by an EVT gap: room/thread read markers,
// pending notifications, and presence. Viewer config is included as a cheap
// authoritative replacement so all self-only fields converge together.
func (s *HTTPServer) realtimeProjectionReconciliationFrame(ctx context.Context, userID string) (*realtimev1.RealtimeServerFrame, error) {
	viewer, err := s.connectAPI.BuildRealtimeProjectionViewer(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("assemble viewer reconciliation: %w", err)
	}
	roomStates, err := s.connectAPI.BuildRealtimeProjectionRoomViewerStates(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("assemble room viewer-state reconciliation: %w", err)
	}
	threadStates, err := s.connectAPI.BuildRealtimeProjectionThreadViewerStates(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("assemble thread viewer-state reconciliation: %w", err)
	}
	notifications, err := s.connectAPI.BuildRealtimeProjectionNotifications(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("assemble notification reconciliation: %w", err)
	}
	presences, err := s.connectAPI.BuildRealtimeProjectionPresences(ctx)
	if err != nil {
		return nil, fmt.Errorf("assemble presence reconciliation: %w", err)
	}

	operations := make([]*realtimev1.RealtimeProjectionOperation, 0, 4+len(roomStates))
	operations = append(operations, &realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_ViewerUpsert{
		ViewerUpsert: viewer,
	}})
	for _, state := range roomStates {
		operations = append(operations, &realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomViewerStateReplace{
			RoomViewerStateReplace: &realtimev1.RealtimeProjectionRoomViewerStateReplace{
				RoomId: state.RoomID, ViewerState: state.ViewerState,
			},
		}})
	}
	operations = append(operations,
		&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_ThreadViewerStatesReplace{
			ThreadViewerStatesReplace: realtimeProjectionThreadViewerStates(threadStates),
		}},
		&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_NotificationsReplace{
			NotificationsReplace: realtimeProjectionNotifications(notifications),
		}},
		&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_PresencesReplace{
			PresencesReplace: &realtimev1.RealtimeProjectionPresencesReplace{Statuses: presences},
		}},
	)

	return realtimeProjectionServerFrame(&realtimev1.RealtimeProjectionEvent{
		Id: core.NewEventID(), CreatedAt: timestamppb.Now(), Operations: operations,
	}), nil
}

func (s *HTTPServer) realtimeProjectionFrameForEvent(ctx context.Context, viewerID string, event core.EventEnvelope) (*realtimev1.RealtimeServerFrame, bool, error) {
	return s.realtimeProjectionFrameForEventWithRooms(ctx, viewerID, event, nil)
}

// realtimeProjectionFrameForEventWithRooms maps every durable fact so its
// cursor can advance, but only materialises timeline payloads for rooms the
// connection says it retains. A nil set preserves the unfiltered test/helper
// behavior; a non-nil empty set means no timeline is retained.
func (s *HTTPServer) realtimeProjectionFrameForEventWithRooms(ctx context.Context, viewerID string, event core.EventEnvelope, retainedRooms map[string]struct{}) (*realtimev1.RealtimeServerFrame, bool, error) {
	evt := event.EVTEvent()
	if core.IsRBACEvent(evt) {
		return &realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
			Close: &realtimev1.RealtimeClose{
				Code: "projection_reset_required", Message: "authorization changed", Reconnect: true,
			},
		}}, true, nil
	}
	projection := &realtimev1.RealtimeProjectionEvent{
		Id:        event.ID(),
		CreatedAt: event.CreatedAt(),
		ActorId:   optionalRealtimeString(event.ActorID()),
	}
	if event.DeliverySeq() > 0 {
		cursor, err := s.core.RealtimeCursorForSequence(viewerID, event.DeliverySeq())
		if err != nil {
			return nil, false, err
		}
		projection.ResumeCursor = &cursor
	}

	appendOperation := func(operation *realtimev1.RealtimeProjectionOperation) {
		projection.Operations = append(projection.Operations, operation)
	}
	retainsTimeline := func(roomID string) bool {
		if retainedRooms == nil {
			return true
		}
		_, ok := retainedRooms[roomID]
		return ok
	}
	if evt == nil {
		live := event.LiveEvent()
		if live == nil {
			return nil, false, nil
		}
		switch payload := live.GetEvent().(type) {
		case *corev1.LiveEvent_ServerUpdated:
			server, err := s.connectAPI.BuildRealtimeProjectionServer(ctx)
			if err != nil {
				return nil, false, err
			}
			appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_ServerUpsert{ServerUpsert: server}})
			serverState := s.connectAPI.BuildRealtimeProjectionServerState()
			appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_ServerStateUpsert{
				ServerStateUpsert: realtimeProjectionServerState(serverState),
			}})
		case *corev1.LiveEvent_UserProfileUpdated:
			if err := s.appendRealtimeProjectionUser(ctx, viewerID, payload.UserProfileUpdated.GetUserId(), appendOperation); err != nil {
				return nil, false, err
			}
		case *corev1.LiveEvent_ServerMemberDeleted:
			appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_UserRemove{
				UserRemove: &realtimev1.RealtimeProjectionUserRemove{UserId: payload.ServerMemberDeleted.GetUserId()},
			}})
		case *corev1.LiveEvent_RoomGroupsUpdated:
			groups, err := s.connectAPI.BuildRealtimeProjectionRoomGroups(ctx, viewerID)
			if err != nil {
				return nil, false, err
			}
			appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomGroupsReplace{
				RoomGroupsReplace: &realtimev1.RealtimeProjectionRoomGroupsReplace{Groups: groups},
			}})
		case *corev1.LiveEvent_ServerUserPreferencesUpdated:
			viewer, err := s.connectAPI.BuildRealtimeProjectionViewer(ctx, viewerID)
			if err != nil {
				return nil, false, err
			}
			appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_ViewerUpsert{ViewerUpsert: viewer}})
		case *corev1.LiveEvent_NotificationLevelChanged:
			viewer, err := s.connectAPI.BuildRealtimeProjectionViewer(ctx, viewerID)
			if err != nil {
				return nil, false, err
			}
			appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_ViewerUpsert{ViewerUpsert: viewer}})
		case *corev1.LiveEvent_NotificationCreated:
			notifications, err := s.connectAPI.BuildRealtimeProjectionNotifications(ctx, viewerID)
			if err != nil {
				return nil, false, err
			}
			replacement := realtimeProjectionNotifications(notifications)
			replacement.Change = &realtimev1.RealtimeProjectionNotificationChange{
				Action:         realtimev1.RealtimeProjectionNotificationAction_REALTIME_PROJECTION_NOTIFICATION_ACTION_CREATED,
				NotificationId: payload.NotificationCreated.GetNotificationId(),
				Silent:         payload.NotificationCreated.GetSilent(),
			}
			appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_NotificationsReplace{
				NotificationsReplace: replacement,
			}})
			// Reply notifications are also the live signal that a followed
			// thread became unread. Replace the complete latest-value set so
			// unretained rooms and the My Threads view converge without a
			// ConnectRPC refresh.
			if payload.NotificationCreated.GetInReplyToId() != "" {
				threadStates, err := s.connectAPI.BuildRealtimeProjectionThreadViewerStates(ctx, viewerID)
				if err != nil {
					return nil, false, err
				}
				appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_ThreadViewerStatesReplace{
					ThreadViewerStatesReplace: realtimeProjectionThreadViewerStates(threadStates),
				}})
			}
		case *corev1.LiveEvent_NotificationDismissed:
			notifications, err := s.connectAPI.BuildRealtimeProjectionNotifications(ctx, viewerID)
			if err != nil {
				return nil, false, err
			}
			replacement := realtimeProjectionNotifications(notifications)
			replacement.Change = &realtimev1.RealtimeProjectionNotificationChange{
				Action:         realtimev1.RealtimeProjectionNotificationAction_REALTIME_PROJECTION_NOTIFICATION_ACTION_DISMISSED,
				NotificationId: payload.NotificationDismissed.GetNotificationId(),
			}
			appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_NotificationsReplace{
				NotificationsReplace: replacement,
			}})
		case *corev1.LiveEvent_RoomMarkedAsRead:
			roomID := payload.RoomMarkedAsRead.GetRoomId()
			viewerState, err := s.connectAPI.BuildRealtimeProjectionRoomViewerState(ctx, viewerID, roomID)
			if err != nil {
				return nil, false, err
			}
			appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomViewerStateReplace{
				RoomViewerStateReplace: &realtimev1.RealtimeProjectionRoomViewerStateReplace{
					RoomId: roomID, ViewerState: viewerState,
				},
			}})
			notifications, err := s.connectAPI.BuildRealtimeProjectionNotifications(ctx, viewerID)
			if err != nil {
				return nil, false, err
			}
			appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_NotificationsReplace{
				NotificationsReplace: realtimeProjectionNotifications(notifications),
			}})
		case *corev1.LiveEvent_ThreadFollowChanged:
			thread := payload.ThreadFollowChanged
			threadStates, err := s.connectAPI.BuildRealtimeProjectionThreadViewerStates(ctx, viewerID)
			if err != nil {
				return nil, false, err
			}
			appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_ThreadViewerStatesReplace{
				ThreadViewerStatesReplace: realtimeProjectionThreadViewerStates(threadStates),
			}})
			if retainsTimeline(thread.GetRoomId()) {
				timelineEvent, includes, eventCursor, err := s.connectAPI.BuildRealtimeProjectionTimelineEvent(ctx, viewerID, thread.GetRoomId(), thread.GetThreadRootEventId())
				if err != nil {
					return nil, false, err
				}
				appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomTimelineEventUpsert{
					RoomTimelineEventUpsert: &realtimev1.RealtimeProjectionRoomTimelineEventUpsert{
						RoomId: thread.GetRoomId(), Event: timelineEvent, Includes: includes, EventCursor: eventCursor,
					},
				}})
			}
		default:
			return nil, false, nil
		}
		return realtimeProjectionServerFrame(projection), true, nil
	}
	appendTimeline := func(roomID, messageEventID string, reaction *realtimev1.RealtimeProjectionReactionChange, retainDeletedRow ...bool) error {
		if !retainsTimeline(roomID) {
			return nil
		}
		if s.core.IsHiddenChannelEcho(messageEventID) {
			appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomTimelineEventRemove{
				RoomTimelineEventRemove: &realtimev1.RealtimeProjectionRoomTimelineEventRemove{RoomId: roomID, EventId: messageEventID},
			}})
			return nil
		}
		timelineEvent, includes, eventCursor, err := s.connectAPI.BuildRealtimeProjectionTimelineEvent(ctx, viewerID, roomID, messageEventID)
		if err != nil {
			return err
		}
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomTimelineEventUpsert{
			RoomTimelineEventUpsert: &realtimev1.RealtimeProjectionRoomTimelineEventUpsert{
				RoomId: roomID, Event: timelineEvent, Includes: includes, ReactionChange: reaction,
				RetainDeletedRow: len(retainDeletedRow) > 0 && retainDeletedRow[0], EventCursor: eventCursor,
			},
		}})
		return nil
	}
	appendTimelineRemove := func(roomID, eventID string) {
		if !retainsTimeline(roomID) {
			return
		}
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomTimelineEventRemove{
			RoomTimelineEventRemove: &realtimev1.RealtimeProjectionRoomTimelineEventRemove{RoomId: roomID, EventId: eventID},
		}})
	}
	appendSearchRefreshFence := func(roomID string) error {
		if !s.config.Search.Enabled {
			return nil
		}
		serverState := s.connectAPI.BuildRealtimeProjectionServerState()
		// Reuse an operation understood by every projection-v1 client. New
		// clients treat this as a search refresh fence; older clients safely
		// reapply the same server state and advance their cursor.
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_ServerStateUpsert{
			ServerStateUpsert: realtimeProjectionServerState(serverState),
		}})
		return nil
	}
	appendRoomViewerState := func(roomID string) error {
		viewerState, err := s.connectAPI.BuildRealtimeProjectionRoomViewerState(ctx, viewerID, roomID)
		if err != nil {
			return err
		}
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomViewerStateReplace{
			RoomViewerStateReplace: &realtimev1.RealtimeProjectionRoomViewerStateReplace{
				RoomId: roomID, ViewerState: viewerState,
			},
		}})
		return nil
	}
	appendRoomResult := func(roomID string) (*connectapi.RealtimeProjectionRoom, error) {
		var room *connectapi.RealtimeProjectionRoom
		var err error
		if retainsTimeline(roomID) {
			room, err = s.connectAPI.BuildRealtimeProjectionRoom(ctx, viewerID, roomID)
		} else {
			room, err = s.connectAPI.BuildRealtimeProjectionRoomSummary(ctx, viewerID, roomID)
		}
		if errors.Is(err, core.ErrNotFound) || errors.Is(err, core.ErrPermissionDenied) || room == nil {
			appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomRemove{
				RoomRemove: &realtimev1.RealtimeProjectionRoomRemove{RoomId: roomID},
			}})
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("hydrate realtime room %q: %w", roomID, err)
		}
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomUpsert{RoomUpsert: realtimeProjectionRoom(room)}})
		return room, nil
	}
	appendRoom := func(roomID string) error {
		_, err := appendRoomResult(roomID)
		return err
	}
	appendRoomTimeline := func(roomID string) error {
		if !retainsTimeline(roomID) {
			return nil
		}
		timeline, err := s.connectAPI.BuildRealtimeProjectionRoomTimeline(ctx, viewerID, roomID)
		if err != nil {
			return fmt.Errorf("hydrate realtime room timeline %q: %w", roomID, err)
		}
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomTimelineReplace{
			RoomTimelineReplace: &realtimev1.RealtimeProjectionRoomTimelineReplace{RoomId: roomID, Page: timeline.Page, EventCursors: timeline.EventCursors},
		}})
		return nil
	}
	appendRoomTimelineIfMember := func(roomID string) error {
		if !retainsTimeline(roomID) {
			return nil
		}
		viewerState, err := s.connectAPI.BuildRealtimeProjectionRoomViewerState(ctx, viewerID, roomID)
		if errors.Is(err, core.ErrNotFound) || errors.Is(err, core.ErrPermissionDenied) {
			return nil
		}
		if err != nil {
			return err
		}
		if !viewerState.GetIsMember() {
			return nil
		}
		return appendRoomTimeline(roomID)
	}
	appendRoomTimelineClear := func(roomID string) {
		if !retainsTimeline(roomID) {
			return
		}
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomTimelineReplace{
			RoomTimelineReplace: &realtimev1.RealtimeProjectionRoomTimelineReplace{RoomId: roomID, Page: &apiv1.RoomTimelinePage{}},
		}})
	}
	appendViewerSensitiveResources := func() error {
		calls, err := s.connectAPI.BuildRealtimeProjectionActiveCalls(ctx, viewerID)
		if err != nil {
			return fmt.Errorf("assemble active calls after room access change: %w", err)
		}
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_ActiveCallsReplace{
			ActiveCallsReplace: &realtimev1.RealtimeProjectionActiveCallsReplace{Calls: calls},
		}})
		notifications, err := s.connectAPI.BuildRealtimeProjectionNotifications(ctx, viewerID)
		if err != nil {
			return fmt.Errorf("assemble notifications after room access change: %w", err)
		}
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_NotificationsReplace{
			NotificationsReplace: realtimeProjectionNotifications(notifications),
		}})
		return nil
	}
	appendSourceTimeline := func(roomID string) error {
		if !retainsTimeline(roomID) {
			return nil
		}
		timelineEvent, includes, eventCursor, err := s.connectAPI.BuildRealtimeProjectionSourceTimelineEvent(ctx, viewerID, roomID, evt)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) || errors.Is(err, core.ErrPermissionDenied) {
				return nil
			}
			return err
		}
		if timelineEvent == nil {
			return nil
		}
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomTimelineEventUpsert{
			RoomTimelineEventUpsert: &realtimev1.RealtimeProjectionRoomTimelineEventUpsert{RoomId: roomID, Event: timelineEvent, Includes: includes, EventCursor: eventCursor},
		}})
		return nil
	}

	switch payload := evt.GetEvent().(type) {
	case *corev1.Event_MessagePosted:
		roomID := payload.MessagePosted.GetRoomId()
		// Refresh lightweight room state when no timeline is retained. Retained
		// rooms already carry their activity through the timeline mutation.
		if !retainsTimeline(roomID) {
			if err := appendRoom(roomID); err != nil {
				return nil, false, err
			}
		}
		if err := appendRoomViewerState(roomID); err != nil {
			return nil, false, err
		}
		if payload.MessagePosted.GetInThread() == "" {
			appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomActivity{
				RoomActivity: &realtimev1.RealtimeProjectionRoomActivity{RoomId: roomID},
			}})
		}
		if err := appendTimeline(roomID, evt.GetId(), nil); err != nil {
			return nil, false, err
		}
		// Deliver the reply before the authoritative root summary. Existing
		// reducers optimistically increment a root when ingesting a reply; the
		// following root upsert then converges that count instead of doubling it.
		if rootID := payload.MessagePosted.GetInThread(); rootID != "" {
			if err := appendTimeline(roomID, rootID, nil); err != nil {
				return nil, false, err
			}
		}
	case *corev1.Event_MessageEdited:
		roomID := payload.MessageEdited.GetRoomId()
		eventID := payload.MessageEdited.GetEventId()
		if err := appendSearchRefreshFence(roomID); err != nil {
			return nil, false, err
		}
		if s.core.IsHiddenChannelEcho(eventID) {
			appendTimelineRemove(roomID, eventID)
		} else if err := appendTimeline(roomID, eventID, nil); err != nil {
			return nil, false, err
		}
	case *corev1.Event_MessageRetracted:
		roomID := payload.MessageRetracted.GetRoomId()
		eventID := payload.MessageRetracted.GetEventId()
		if err := appendSearchRefreshFence(roomID); err != nil {
			return nil, false, err
		}
		if s.core.IsHiddenChannelEcho(eventID) {
			// A directly retracted channel echo is a projection artifact, not a
			// deleted-message tombstone. Its current authoritative state is absence.
			appendTimelineRemove(roomID, eventID)
		} else if err := appendTimeline(roomID, eventID, nil); err != nil {
			return nil, false, err
		} else if echoID, ok := s.core.LinkedChannelEchoEventID(eventID); ok {
			// Retracting the canonical reply tombstones its still-visible room
			// echo through projection state even though the durable fact names
			// only the canonical message.
			if err := appendTimeline(roomID, echoID, nil, true); err != nil {
				return nil, false, err
			}
		}
	case *corev1.Event_ReactionAdded:
		reaction := payload.ReactionAdded
		messageID := s.core.CanonicalReactionMessageEventID(reaction.GetRoomId(), reaction.GetMessageEventId())
		if err := appendTimeline(reaction.GetRoomId(), messageID, &realtimev1.RealtimeProjectionReactionChange{
			Action:         realtimev1.RealtimeProjectionReactionAction_REALTIME_PROJECTION_REACTION_ACTION_ADDED,
			MessageEventId: messageID, Emoji: reaction.GetEmoji(), UserId: evt.GetActorId(),
		}); err != nil {
			return nil, false, err
		}
		if echoID, ok := s.core.ChannelEchoEventID(messageID); ok {
			if err := appendTimeline(reaction.GetRoomId(), echoID, nil); err != nil {
				return nil, false, err
			}
		}
	case *corev1.Event_ReactionRemoved:
		reaction := payload.ReactionRemoved
		messageID := s.core.CanonicalReactionMessageEventID(reaction.GetRoomId(), reaction.GetMessageEventId())
		if err := appendTimeline(reaction.GetRoomId(), messageID, &realtimev1.RealtimeProjectionReactionChange{
			Action:         realtimev1.RealtimeProjectionReactionAction_REALTIME_PROJECTION_REACTION_ACTION_REMOVED,
			MessageEventId: messageID, Emoji: reaction.GetEmoji(), UserId: evt.GetActorId(),
		}); err != nil {
			return nil, false, err
		}
		if echoID, ok := s.core.ChannelEchoEventID(messageID); ok {
			if err := appendTimeline(reaction.GetRoomId(), echoID, nil); err != nil {
				return nil, false, err
			}
		}
	case *corev1.Event_AssetProcessingStarted,
		*corev1.Event_AssetProcessingSucceeded,
		*corev1.Event_AssetProcessingFailed,
		*corev1.Event_AssetDeleted:
		roomID, messageEventID, ok := s.core.AssetEventTimelineTarget(evt)
		if !ok {
			return nil, false, nil
		}
		if err := appendTimeline(roomID, messageEventID, nil); err != nil {
			return nil, false, err
		}
		if echoID, ok := s.core.ChannelEchoEventID(messageEventID); ok {
			if err := appendTimeline(roomID, echoID, nil); err != nil {
				return nil, false, err
			}
		}
	case *corev1.Event_VoiceCallStarted,
		*corev1.Event_VoiceCallParticipantJoined,
		*corev1.Event_VoiceCallParticipantLeft,
		*corev1.Event_VoiceCallEnded:
		calls, err := s.connectAPI.BuildRealtimeProjectionActiveCalls(ctx, viewerID)
		if err != nil {
			return nil, false, err
		}
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_ActiveCallsReplace{
			ActiveCallsReplace: &realtimev1.RealtimeProjectionActiveCallsReplace{Calls: calls},
		}})
	case *corev1.Event_RoomDeleted:
		if err := appendViewerSensitiveResources(); err != nil {
			return nil, false, err
		}
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomRemove{
			RoomRemove: &realtimev1.RealtimeProjectionRoomRemove{RoomId: payload.RoomDeleted.GetRoomId()},
		}})
	case *corev1.Event_RoomCreated:
		roomID := payload.RoomCreated.GetRoomId()
		if err := appendRoom(roomID); err != nil {
			return nil, false, err
		}
		if err := appendSourceTimeline(roomID); err != nil {
			return nil, false, err
		}
	case *corev1.Event_RoomUpdated:
		roomID := payload.RoomUpdated.GetRoomId()
		if err := appendRoom(roomID); err != nil {
			return nil, false, err
		}
		if err := appendSourceTimeline(roomID); err != nil {
			return nil, false, err
		}
	case *corev1.Event_RoomArchived:
		roomID := payload.RoomArchived.GetRoomId()
		if err := appendViewerSensitiveResources(); err != nil {
			return nil, false, err
		}
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomRemove{
			RoomRemove: &realtimev1.RealtimeProjectionRoomRemove{RoomId: roomID},
		}})
	case *corev1.Event_RoomUnarchived:
		roomID := payload.RoomUnarchived.GetRoomId()
		if err := appendRoom(roomID); err != nil {
			return nil, false, err
		}
		if err := appendRoomTimelineIfMember(roomID); err != nil {
			return nil, false, err
		}
		if err := appendSourceTimeline(roomID); err != nil {
			return nil, false, err
		}
	case *corev1.Event_RoomUniversalChanged:
		roomID := payload.RoomUniversalChanged.GetRoomId()
		room, err := appendRoomResult(roomID)
		if err != nil {
			return nil, false, err
		}
		if room == nil {
			// room_remove authoritatively evicts any retained timeline.
			break
		}
		if room.Room.GetViewerState().GetIsMember() {
			// Retained rooms regain their current authorised window immediately.
			// Unretained rooms remain lazy because appendRoomTimeline is filtered
			// through the connection's retained-room set.
			if err := appendRoomTimeline(roomID); err != nil {
				return nil, false, err
			}
		} else {
			// A universal-membership revocation must remove already-decrypted
			// timeline state in the same ordered projection event as metadata.
			appendRoomTimelineClear(roomID)
			if err := appendViewerSensitiveResources(); err != nil {
				return nil, false, err
			}
		}
	case *corev1.Event_UserJoinedRoom:
		roomID := payload.UserJoinedRoom.GetRoomId()
		if err := appendRoom(roomID); err != nil {
			return nil, false, err
		}
		if evt.GetActorId() == viewerID {
			if err := appendRoomTimeline(roomID); err != nil {
				return nil, false, err
			}
		}
		if err := appendSourceTimeline(roomID); err != nil {
			return nil, false, err
		}
	case *corev1.Event_UserLeftRoom:
		roomID := payload.UserLeftRoom.GetRoomId()
		if err := appendRoom(roomID); err != nil {
			return nil, false, err
		}
		if evt.GetActorId() == viewerID {
			appendRoomTimelineClear(roomID)
			if err := appendViewerSensitiveResources(); err != nil {
				return nil, false, err
			}
		}
		if err := appendSourceTimeline(roomID); err != nil {
			return nil, false, err
		}
	case *corev1.Event_RoomMemberAdded:
		roomID := payload.RoomMemberAdded.GetRoomId()
		if err := appendRoom(roomID); err != nil {
			return nil, false, err
		}
		if payload.RoomMemberAdded.GetUserId() == viewerID {
			if err := appendRoomTimeline(roomID); err != nil {
				return nil, false, err
			}
		}
	case *corev1.Event_RoomMemberRemoved:
		roomID := payload.RoomMemberRemoved.GetRoomId()
		if err := appendRoom(roomID); err != nil {
			return nil, false, err
		}
		if payload.RoomMemberRemoved.GetUserId() == viewerID {
			appendRoomTimelineClear(roomID)
			if err := appendViewerSensitiveResources(); err != nil {
				return nil, false, err
			}
		}
	case *corev1.Event_RoomMemberBanned:
		roomID := payload.RoomMemberBanned.GetRoomId()
		if err := appendRoom(roomID); err != nil {
			return nil, false, err
		}
		if payload.RoomMemberBanned.GetUserId() == viewerID {
			appendRoomTimelineClear(roomID)
			if err := appendViewerSensitiveResources(); err != nil {
				return nil, false, err
			}
		}
	case *corev1.Event_ThreadCreated:
		thread := payload.ThreadCreated
		if err := appendTimeline(thread.GetRoomId(), thread.GetThreadRootEventId(), nil); err != nil {
			return nil, false, err
		}
	case *corev1.Event_UserCustomStatusSet:
		if err := s.appendRealtimeProjectionUser(ctx, viewerID, payload.UserCustomStatusSet.GetUserId(), appendOperation); err != nil {
			return nil, false, err
		}
	case *corev1.Event_UserCustomStatusCleared:
		if err := s.appendRealtimeProjectionUser(ctx, viewerID, payload.UserCustomStatusCleared.GetUserId(), appendOperation); err != nil {
			return nil, false, err
		}
	case *corev1.Event_UserAccountCreated:
		if err := s.appendRealtimeProjectionUser(ctx, viewerID, payload.UserAccountCreated.GetUserId(), appendOperation); err != nil {
			return nil, false, err
		}
	case *corev1.Event_UserLoginChanged:
		if err := s.appendRealtimeProjectionUser(ctx, viewerID, payload.UserLoginChanged.GetUserId(), appendOperation); err != nil {
			return nil, false, err
		}
	case *corev1.Event_UserDisplayNameChanged:
		if err := s.appendRealtimeProjectionUser(ctx, viewerID, payload.UserDisplayNameChanged.GetUserId(), appendOperation); err != nil {
			return nil, false, err
		}
	case *corev1.Event_UserAvatarSet:
		if err := s.appendRealtimeProjectionUser(ctx, viewerID, payload.UserAvatarSet.GetUserId(), appendOperation); err != nil {
			return nil, false, err
		}
	case *corev1.Event_UserAvatarCleared:
		if err := s.appendRealtimeProjectionUser(ctx, viewerID, payload.UserAvatarCleared.GetUserId(), appendOperation); err != nil {
			return nil, false, err
		}
	case *corev1.Event_UserAccountDeleted:
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_UserRemove{
			UserRemove: &realtimev1.RealtimeProjectionUserRemove{UserId: payload.UserAccountDeleted.GetUserId()},
		}})
	default:
		return nil, false, nil
	}

	// Recognized durable facts may intentionally produce no operations when
	// they only affect a room timeline this connection has not materialised.
	// Keep the empty envelope so the client can safely advance its one cursor.
	return realtimeProjectionServerFrame(projection), true, nil
}

func realtimeProjectionServerState(state *connectapi.RealtimeProjectionServerState) *realtimev1.RealtimeProjectionServerState {
	if state == nil {
		return &realtimev1.RealtimeProjectionServerState{}
	}
	out := &realtimev1.RealtimeProjectionServerState{Runtime: state.Runtime}
	if state.MOTD != "" {
		out.Motd = &state.MOTD
	}
	return out
}

func realtimeProjectionRoom(room *connectapi.RealtimeProjectionRoom) *realtimev1.RealtimeProjectionRoom {
	if room == nil {
		return &realtimev1.RealtimeProjectionRoom{}
	}
	return &realtimev1.RealtimeProjectionRoom{
		Room:                    room.Room,
		MemberUserIds:           append([]string(nil), room.MemberUserIDs...),
		ViewerNotificationCount: room.ViewerNotificationCount,
		HasMessageHistory:       room.HasMessageHistory,
	}
}

func realtimeProjectionNotifications(notifications *connectapi.RealtimeProjectionNotifications) *realtimev1.RealtimeProjectionNotificationsReplace {
	if notifications == nil {
		return &realtimev1.RealtimeProjectionNotificationsReplace{}
	}
	return &realtimev1.RealtimeProjectionNotificationsReplace{
		Page:       notifications.Page,
		RoomCounts: notifications.RoomCounts,
	}
}

func realtimeProjectionThreadViewerStates(states []*connectapi.RealtimeProjectionThreadViewerState) *realtimev1.RealtimeProjectionThreadViewerStatesReplace {
	out := &realtimev1.RealtimeProjectionThreadViewerStatesReplace{States: make([]*realtimev1.RealtimeProjectionThreadViewerState, 0, len(states))}
	for _, state := range states {
		if state == nil {
			continue
		}
		out.States = append(out.States, &realtimev1.RealtimeProjectionThreadViewerState{
			RoomId: state.RoomID, ThreadRootEventId: state.ThreadRootEventID, ViewerState: state.ViewerState,
		})
	}
	return out
}

func (s *HTTPServer) appendRealtimeProjectionUser(
	ctx context.Context,
	viewerID, userID string,
	appendOperation func(*realtimev1.RealtimeProjectionOperation),
) error {
	user, err := s.connectAPI.BuildRealtimeProjectionUser(ctx, userID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_UserRemove{
				UserRemove: &realtimev1.RealtimeProjectionUserRemove{UserId: userID},
			}})
			return nil
		}
		return fmt.Errorf("hydrate realtime user %q for viewer %q: %w", userID, viewerID, err)
	}
	appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_UserUpsert{UserUpsert: user}})
	return nil
}

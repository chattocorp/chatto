package http_server

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"

	"hmans.de/chatto/internal/connectapi"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	realtimev1 "hmans.de/chatto/internal/pb/chatto/realtime/v1"
)

func (s *HTTPServer) realtimeProjectionSnapshotFrames(ctx context.Context, userID string, timelineRoomIDs []string) ([]*realtimev1.RealtimeServerFrame, error) {
	frames := make([]*realtimev1.RealtimeServerFrame, 0)
	_, err := s.writeRealtimeProjectionSnapshot(ctx, userID, timelineRoomIDs, func(frame *realtimev1.RealtimeServerFrame) error {
		frames = append(frames, frame)
		return nil
	})
	return frames, err
}

// writeRealtimeProjectionSnapshot emits the current-state snapshot incrementally so
// the transport does not retain a second frame graph for every decrypted room
// timeline while a reset is in flight.
func (s *HTTPServer) writeRealtimeProjectionSnapshot(ctx context.Context, userID string, timelineRoomIDs []string, writeFrame func(*realtimev1.RealtimeServerFrame) error) (uint64, error) {
	if s.connectAPI == nil {
		return 0, errors.New("Connect API is unavailable")
	}
	snapshot, err := s.connectAPI.BuildRealtimeProjectionSnapshot(ctx, userID, timelineRoomIDs)
	if err != nil {
		return 0, err
	}

	var writeErr error
	appendState := func(state *realtimev1.RealtimeStateItem) {
		if writeErr != nil {
			return
		}
		writeErr = writeFrame(realtimeStateServerFrame(state))
	}
	appendState(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_Server{
		Server: snapshot.Server,
	}})
	appendState(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_ServerState{
		ServerState: realtimeProjectionServerState(snapshot.ServerState),
	}})
	appendState(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_Viewer{
		Viewer: snapshot.Viewer,
	}})
	for _, user := range snapshot.Users {
		appendState(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_User{User: user}})
	}
	for _, room := range snapshot.Rooms {
		appendState(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_Room{Room: realtimeProjectionRoom(room)}})
	}
	appendState(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_RoomGroups{
		RoomGroups: &realtimev1.RealtimeRoomGroupsState{Groups: snapshot.RoomGroups},
	}})
	appendState(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_Notifications{
		Notifications: realtimeProjectionNotificationOccurrences(snapshot.Notifications),
	}})
	appendState(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_ActiveCalls{
		ActiveCalls: &realtimev1.RealtimeActiveCallsState{Calls: snapshot.ActiveCalls},
	}})
	for _, timeline := range snapshot.Timelines {
		appendState(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_RoomTimeline{
			RoomTimeline: &realtimev1.RealtimeRoomTimelineState{RoomId: timeline.RoomID, Page: timeline.Page, EventCursors: timeline.EventCursors},
		}})
	}
	return snapshot.RoomMarkerFence, writeErr
}

func realtimeProjectionServerFrame(event *realtimev1.RealtimeEvent) *realtimev1.RealtimeServerFrame {
	return &realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Event{Event: event}}
}

func realtimeStateServerFrame(state *realtimev1.RealtimeStateItem) *realtimev1.RealtimeServerFrame {
	return &realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_State{State: state}}
}

func realtimeProjectionRoomViewerOperation(roomID string, viewerState *apiv1.RoomViewerState) *realtimev1.RealtimeStateItem {
	return &realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_RoomViewerActivity{
		RoomViewerActivity: &realtimev1.RealtimeRoomViewerActivityState{
			RoomId: roomID, HasUnread: viewerState.GetHasUnread(), SlowModeNextPostAt: viewerState.GetSlowModeNextPostAt(),
		},
	}}
}

func (s *HTTPServer) realtimeProjectionRoomFrames(ctx context.Context, viewerID, roomID string) ([]*realtimev1.RealtimeServerFrame, error) {
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
	return []*realtimev1.RealtimeServerFrame{
		realtimeStateServerFrame(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_Room{
			Room: realtimeProjectionRoom(room),
		}}),
		realtimeStateServerFrame(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_RoomTimeline{
			RoomTimeline: &realtimev1.RealtimeRoomTimelineState{
				RoomId: roomID, Page: timeline.Page, EventCursors: timeline.EventCursors,
			},
		}}),
	}, nil
}

// realtimeProjectionReconciliationFrame captures latest-value viewer state
// that is not fully represented by an EVT gap: room/thread read markers,
// notification list state, and presence. Viewer config is included as a cheap
// authoritative replacement so all self-only fields converge together.
// Room viewer state is needed after incremental replay. A current-state snapshot
// supplies it in snapshot room upserts and repairs only markers that changed
// while that snapshot was assembled.
func (s *HTTPServer) realtimeProjectionReconciliationFrames(ctx context.Context, userID string, roomMarkerFence *uint64) ([]*realtimev1.RealtimeServerFrame, error) {
	viewer, err := s.connectAPI.BuildRealtimeProjectionViewer(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("assemble viewer reconciliation: %w", err)
	}
	var roomStates []*connectapi.RealtimeProjectionRoomViewerState
	if roomMarkerFence == nil {
		roomStates, err = s.connectAPI.BuildRealtimeProjectionRoomViewerStates(ctx, userID)
		if err != nil {
			return nil, fmt.Errorf("assemble room viewer-state reconciliation: %w", err)
		}
	} else {
		roomIDs, changedErr := s.core.ReadState().RoomMarkerIDsChangedAfter(ctx, userID, *roomMarkerFence)
		if changedErr != nil {
			return nil, fmt.Errorf("identify concurrent room viewer-state changes: %w", changedErr)
		}
		roomStates = make([]*connectapi.RealtimeProjectionRoomViewerState, 0, len(roomIDs))
		for _, roomID := range roomIDs {
			viewerState, stateErr := s.connectAPI.BuildRealtimeProjectionRoomViewerState(ctx, userID, roomID)
			if stateErr != nil {
				if errors.Is(stateErr, core.ErrNotFound) || errors.Is(stateErr, core.ErrPermissionDenied) || errors.Is(stateErr, core.ErrNotRoomMember) {
					continue
				}
				return nil, fmt.Errorf("assemble changed room %q viewer-state reconciliation: %w", roomID, stateErr)
			}
			roomStates = append(roomStates, &connectapi.RealtimeProjectionRoomViewerState{RoomID: roomID, ViewerState: viewerState})
		}
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

	operations := make([]*realtimev1.RealtimeStateItem, 0, 4+len(roomStates))
	operations = append(operations, &realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_Viewer{
		Viewer: viewer,
	}})
	for _, state := range roomStates {
		operations = append(operations, realtimeProjectionRoomViewerOperation(state.RoomID, state.ViewerState))
	}
	operations = append(operations,
		&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_ThreadViewerStates{
			ThreadViewerStates: realtimeProjectionThreadViewerStates(threadStates),
		}},
		&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_Notifications{
			Notifications: realtimeProjectionNotificationOccurrences(notifications),
		}},
		&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_Presences{
			Presences: &realtimev1.RealtimePresencesState{Statuses: presences},
		}},
	)

	frames := make([]*realtimev1.RealtimeServerFrame, 0, len(operations))
	for _, operation := range operations {
		frames = append(frames, realtimeStateServerFrame(operation))
	}
	return frames, nil
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
	advanceWithoutReset := false
	if core.IsRBACEvent(evt) {
		var err error
		advanceWithoutReset, err = s.canAdvanceSelfAuthoredRBAC(ctx, viewerID, evt)
		if err != nil {
			return nil, false, err
		}
	}
	if core.IsRBACEvent(evt) && !advanceWithoutReset {
		return &realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_Close{
			Close: &realtimev1.RealtimeClose{
				Code: "projection_reset_required", Message: "authorization changed", Reconnect: true,
			},
		}}, true, nil
	}
	canonical := event.CanonicalEvent()
	var deliveryEvent *evtv1.Event
	if isRealtimePublicEvent(canonical) {
		deliveryEvent = proto.Clone(canonical).(*evtv1.Event)
		if err := s.core.PopulateEventPlaintext(ctx, deliveryEvent); err != nil {
			return nil, false, err
		}
	}
	projection := &realtimev1.RealtimeEvent{Event: projectRealtimeEvent(deliveryEvent)}
	if event.DeliverySeq() > 0 {
		cursor, err := s.core.RealtimeCursorForSequence(viewerID, event.DeliverySeq())
		if err != nil {
			return nil, false, err
		}
		projection.ResumeCursor = &cursor
	}
	if advanceWithoutReset {
		return realtimeProjectionServerFrame(projection), true, nil
	}
	if roomID, protected := s.core.MessageReadProtectedEventRoomID(evt); protected {
		kind, err := s.core.FindRoomKind(ctx, roomID)
		if err != nil {
			if errors.Is(err, core.ErrNotFound) {
				return realtimeProjectionServerFrame(projection), true, nil
			}
			return nil, false, err
		}
		isMember, err := s.core.RoomMembershipExists(ctx, kind, viewerID, roomID)
		if err != nil {
			return nil, false, err
		}
		if !isMember {
			return realtimeProjectionServerFrame(projection), true, nil
		}
		canRead, err := s.core.CanReadMessageEvent(ctx, viewerID, kind, roomID, evt)
		if err != nil {
			return nil, false, err
		}
		if !canRead {
			return realtimeProjectionServerFrame(projection), true, nil
		}
	}
	if evt != nil && projection.GetEvent() == nil {
		return nil, false, nil
	}

	appendOperation := func(operation *realtimev1.RealtimeStateItem) {
		projection.State = append(projection.State, operation)
	}
	retainsTimeline := func(roomID string) bool {
		if retainedRooms == nil {
			return true
		}
		_, ok := retainedRooms[roomID]
		return ok
	}
	if evt == nil {
		if canonical == nil {
			return nil, false, nil
		}
		switch payload := canonical.GetEvent().(type) {
		case *evtv1.Event_ServerUpdatedSync:
			server, err := s.connectAPI.BuildRealtimeProjectionServer(ctx)
			if err != nil {
				return nil, false, err
			}
			appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_Server{Server: server}})
			serverState := s.connectAPI.BuildRealtimeProjectionServerState()
			appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_ServerState{
				ServerState: realtimeProjectionServerState(serverState),
			}})
		case *evtv1.Event_UserProfileSync:
			if err := s.appendRealtimeProjectionUser(ctx, viewerID, payload.UserProfileSync.GetUserId(), appendOperation); err != nil {
				return nil, false, err
			}
		case *evtv1.Event_ServerMemberDeletedSync:
			appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_UserRemoved{
				UserRemoved: &realtimev1.RealtimeUserRemovedState{UserId: payload.ServerMemberDeletedSync.GetUserId()},
			}})
		case *evtv1.Event_RoomGroupsUpdatedSync:
			groups, err := s.connectAPI.BuildRealtimeProjectionRoomGroups(ctx, viewerID)
			if err != nil {
				return nil, false, err
			}
			appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_RoomGroups{
				RoomGroups: &realtimev1.RealtimeRoomGroupsState{Groups: groups},
			}})
		case *evtv1.Event_ServerUserPreferencesSync:
			viewer, err := s.connectAPI.BuildRealtimeProjectionViewer(ctx, viewerID)
			if err != nil {
				return nil, false, err
			}
			appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_Viewer{Viewer: viewer}})
		case *evtv1.Event_NotificationOccurrencesInvalidated:
			invalidation := payload.NotificationOccurrencesInvalidated
			if err := s.core.NotificationOccurrences().Resync(ctx); err != nil {
				return nil, false, err
			}
			candidateID := invalidation.GetSoundCandidateNotificationId()
			if candidateID == "" {
				// Accept legacy publishers during a rolling replacement.
				candidateID = invalidation.GetAlertCandidateNotificationId()
			}
			soundEligible := false
			if candidateID != "" {
				current, err := s.core.NotificationOccurrences().Get(ctx, viewerID, candidateID)
				if err == nil && !current.GetRead() {
					soundEligible, err = s.core.NotificationSoundEligible(ctx, current)
					if err != nil {
						return nil, false, err
					}
				} else if err != nil && !errors.Is(err, core.ErrNotFound) {
					return nil, false, err
				}
			}
			notifications, err := s.connectAPI.BuildRealtimeProjectionNotifications(ctx, viewerID)
			if err != nil {
				return nil, false, err
			}
			replacement := realtimeProjectionNotificationOccurrences(notifications)
			if soundEligible && notificationReplacementContains(replacement, candidateID) {
				replacement.PlayNotificationSound = true
			}
			appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_Notifications{
				Notifications: replacement,
			}})
		case *evtv1.Event_RoomMarkedAsReadSync:
			roomID := payload.RoomMarkedAsReadSync.GetRoomId()
			viewerState, err := s.connectAPI.BuildRealtimeProjectionRoomViewerState(ctx, viewerID, roomID)
			if err != nil {
				return nil, false, err
			}
			appendOperation(realtimeProjectionRoomViewerOperation(roomID, viewerState))
			notifications, err := s.connectAPI.BuildRealtimeProjectionNotifications(ctx, viewerID)
			if err != nil {
				return nil, false, err
			}
			appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_Notifications{
				Notifications: realtimeProjectionNotificationOccurrences(notifications),
			}})
		case *evtv1.Event_NotificationUnreadChanged:
			invalidation := payload.NotificationUnreadChanged
			roomID := invalidation.GetRoomId()
			viewerState, err := s.connectAPI.BuildRealtimeProjectionRoomViewerState(ctx, viewerID, roomID)
			if err != nil {
				if errors.Is(err, core.ErrNotFound) || errors.Is(err, core.ErrPermissionDenied) {
					return realtimeProjectionServerFrame(projection), true, nil
				}
				return nil, false, err
			}
			appendOperation(realtimeProjectionRoomViewerOperation(roomID, viewerState))
		case *evtv1.Event_ThreadFollowChangedSync:
			thread := payload.ThreadFollowChangedSync
			threadStates, err := s.connectAPI.BuildRealtimeProjectionThreadViewerStates(ctx, viewerID)
			if err != nil {
				return nil, false, err
			}
			appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_ThreadViewerStates{
				ThreadViewerStates: realtimeProjectionThreadViewerStates(threadStates),
			}})
			if retainsTimeline(thread.GetRoomId()) {
				timelineEvent, includes, eventCursor, err := s.connectAPI.BuildRealtimeProjectionTimelineEvent(ctx, viewerID, thread.GetRoomId(), thread.GetThreadRootEventId())
				if err != nil {
					if errors.Is(err, core.ErrPermissionDenied) {
						return realtimeProjectionServerFrame(projection), true, nil
					}
					return nil, false, err
				}
				appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_RoomTimelineEvent{
					RoomTimelineEvent: &realtimev1.RealtimeRoomTimelineEventState{
						RoomId: thread.GetRoomId(), Event: timelineEvent, Includes: includes, EventCursor: eventCursor,
					},
				}})
			}
		default:
			return nil, false, nil
		}
		return realtimeProjectionServerFrame(projection), true, nil
	}
	appendTimeline := func(roomID, messageEventID string, retainDeletedRow ...bool) error {
		if !retainsTimeline(roomID) {
			return nil
		}
		if s.core.IsHiddenChannelEcho(messageEventID) {
			appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_RoomTimelineEventRemoved{
				RoomTimelineEventRemoved: &realtimev1.RealtimeRoomTimelineEventRemovedState{RoomId: roomID, EventId: messageEventID},
			}})
			return nil
		}
		timelineEvent, includes, eventCursor, err := s.connectAPI.BuildRealtimeProjectionTimelineEvent(ctx, viewerID, roomID, messageEventID)
		if err != nil {
			if errors.Is(err, core.ErrPermissionDenied) {
				return nil
			}
			return err
		}
		appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_RoomTimelineEvent{
			RoomTimelineEvent: &realtimev1.RealtimeRoomTimelineEventState{
				RoomId: roomID, Event: timelineEvent, Includes: includes,
				RetainDeletedRow: len(retainDeletedRow) > 0 && retainDeletedRow[0], EventCursor: eventCursor,
			},
		}})
		return nil
	}
	appendTimelineRemove := func(roomID, eventID string) {
		if !retainsTimeline(roomID) {
			return
		}
		appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_RoomTimelineEventRemoved{
			RoomTimelineEventRemoved: &realtimev1.RealtimeRoomTimelineEventRemovedState{RoomId: roomID, EventId: eventID},
		}})
	}
	appendSearchRefreshFence := func(roomID string) error {
		if !s.config.Search.Enabled {
			return nil
		}
		serverState := s.connectAPI.BuildRealtimeProjectionServerState()
		// Repeating the current server state gives projection clients an ordered
		// fence at which they can refresh search data.
		appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_ServerState{
			ServerState: realtimeProjectionServerState(serverState),
		}})
		return nil
	}
	appendFollowedThreadRefresh := func(roomID string) error {
		rootID, ok := s.core.MessageEventThreadRoot(roomID, evt)
		if !ok {
			return nil
		}
		kind, err := s.core.FindRoomKind(ctx, roomID)
		if err != nil {
			return err
		}
		following, err := s.core.IsFollowingThread(ctx, kind, viewerID, roomID, rootID)
		if err != nil {
			return err
		}
		if !following {
			return nil
		}
		threadStates, err := s.connectAPI.BuildRealtimeProjectionThreadViewerStates(ctx, viewerID)
		if err != nil {
			return err
		}
		appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_ThreadViewerStates{
			ThreadViewerStates: realtimeProjectionThreadViewerStates(threadStates),
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
			appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_RoomRemoved{
				RoomRemoved: &realtimev1.RealtimeRoomRemovedState{RoomId: roomID},
			}})
			return nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("hydrate realtime room %q: %w", roomID, err)
		}
		appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_Room{Room: realtimeProjectionRoom(room)}})
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
			if errors.Is(err, core.ErrPermissionDenied) {
				return nil
			}
			return fmt.Errorf("hydrate realtime room timeline %q: %w", roomID, err)
		}
		appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_RoomTimeline{
			RoomTimeline: &realtimev1.RealtimeRoomTimelineState{RoomId: roomID, Page: timeline.Page, EventCursors: timeline.EventCursors},
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
		appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_RoomTimeline{
			RoomTimeline: &realtimev1.RealtimeRoomTimelineState{RoomId: roomID, Page: &apiv1.RoomTimelinePage{}},
		}})
	}
	appendViewerSensitiveResources := func() error {
		calls, err := s.connectAPI.BuildRealtimeProjectionActiveCalls(ctx, viewerID)
		if err != nil {
			return fmt.Errorf("assemble active calls after room access change: %w", err)
		}
		appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_ActiveCalls{
			ActiveCalls: &realtimev1.RealtimeActiveCallsState{Calls: calls},
		}})
		notifications, err := s.connectAPI.BuildRealtimeProjectionNotifications(ctx, viewerID)
		if err != nil {
			return fmt.Errorf("assemble notifications after room access change: %w", err)
		}
		appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_Notifications{
			Notifications: realtimeProjectionNotificationOccurrences(notifications),
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
		appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_RoomTimelineEvent{
			RoomTimelineEvent: &realtimev1.RealtimeRoomTimelineEventState{RoomId: roomID, Event: timelineEvent, Includes: includes, EventCursor: eventCursor},
		}})
		return nil
	}

	switch payload := evt.GetEvent().(type) {
	case *evtv1.Event_MessagePosted:
		roomID := payload.MessagePosted.GetRoomId()
		if payload.MessagePosted.GetInThread() == "" {
			viewerState, err := s.connectAPI.BuildRealtimeProjectionRoomViewerState(ctx, viewerID, roomID)
			if err != nil && !errors.Is(err, core.ErrNotFound) && !errors.Is(err, core.ErrPermissionDenied) {
				return nil, false, err
			}
			if err == nil {
				appendOperation(realtimeProjectionRoomViewerOperation(roomID, viewerState))
			}
		}
		if err := appendTimeline(roomID, evt.GetId()); err != nil {
			return nil, false, err
		}
		// Deliver the reply before the authoritative root summary. Existing
		// reducers optimistically increment a root when ingesting a reply; the
		// following root upsert then converges that count instead of doubling it.
		if rootID := payload.MessagePosted.GetInThread(); rootID != "" {
			if err := appendTimeline(roomID, rootID); err != nil {
				return nil, false, err
			}
			// Thread unread state is independent of notification policy. An
			// existing follower whose FOLLOWED_THREAD delivery mode is Off receives
			// no occurrence invalidation, so reconcile from the durable message
			// fact whenever this viewer actually follows the affected thread.
			kind, err := s.core.FindRoomKind(ctx, roomID)
			if err != nil {
				return nil, false, err
			}
			following, err := s.core.IsFollowingThread(ctx, kind, viewerID, roomID, rootID)
			if err != nil {
				return nil, false, err
			}
			if following {
				threadStates, err := s.connectAPI.BuildRealtimeProjectionThreadViewerStates(ctx, viewerID)
				if err != nil {
					return nil, false, err
				}
				found := false
				for _, state := range threadStates {
					if state != nil && state.RoomID == roomID && state.ThreadRootEventID == rootID {
						found = true
						if state.ViewerState == nil {
							state.ViewerState = &apiv1.ThreadViewerState{}
						}
						isFollowing := true
						state.ViewerState.IsFollowing = &isFollowing
						if evt.GetActorId() != viewerID {
							hasUnread := true
							state.ViewerState.HasUnreadReplies = &hasUnread
						}
						break
					}
				}
				if !found {
					isFollowing := true
					hasUnread := evt.GetActorId() != viewerID
					threadStates = append(threadStates, &connectapi.RealtimeProjectionThreadViewerState{
						RoomID:            roomID,
						ThreadRootEventID: rootID,
						ViewerState: &apiv1.ThreadViewerState{
							IsFollowing:      &isFollowing,
							HasUnreadReplies: &hasUnread,
						},
					})
				}
				appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_ThreadViewerStates{
					ThreadViewerStates: realtimeProjectionThreadViewerStates(threadStates),
				}})
			}
		}
	case *evtv1.Event_MessageEdited:
		roomID := payload.MessageEdited.GetRoomId()
		eventID := payload.MessageEdited.GetEventId()
		if err := appendSearchRefreshFence(roomID); err != nil {
			return nil, false, err
		}
		if s.core.IsHiddenChannelEcho(eventID) {
			appendTimelineRemove(roomID, eventID)
		} else if err := appendTimeline(roomID, eventID); err != nil {
			return nil, false, err
		}
		if err := appendFollowedThreadRefresh(roomID); err != nil {
			return nil, false, err
		}
	case *evtv1.Event_MessageRetracted:
		roomID := payload.MessageRetracted.GetRoomId()
		eventID := payload.MessageRetracted.GetEventId()
		if err := appendSearchRefreshFence(roomID); err != nil {
			return nil, false, err
		}
		hiddenChannelEcho := s.core.IsHiddenChannelEcho(eventID)
		if hiddenChannelEcho {
			// A directly retracted channel echo is a projection artifact, not a
			// deleted-message tombstone. Its current authoritative state is absence.
			appendTimelineRemove(roomID, eventID)
		} else if err := appendTimeline(roomID, eventID); err != nil {
			return nil, false, err
		} else if echoID, ok := s.core.LinkedChannelEchoEventID(eventID); ok {
			// Retracting the canonical reply tombstones its still-visible room
			// echo through projection state even though the durable fact names
			// only the canonical message.
			if err := appendTimeline(roomID, echoID, true); err != nil {
				return nil, false, err
			}
		}
		if err := appendFollowedThreadRefresh(roomID); err != nil {
			return nil, false, err
		}
	case *evtv1.Event_MessagePinned:
	case *evtv1.Event_MessageUnpinned:
	case *evtv1.Event_ReactionAdded:
		reaction := payload.ReactionAdded
		messageID := s.core.CanonicalReactionMessageEventID(reaction.GetRoomId(), reaction.GetMessageEventId())
		if err := appendTimeline(reaction.GetRoomId(), messageID); err != nil {
			return nil, false, err
		}
		if echoID, ok := s.core.ChannelEchoEventID(messageID); ok {
			if err := appendTimeline(reaction.GetRoomId(), echoID); err != nil {
				return nil, false, err
			}
		}
	case *evtv1.Event_ReactionRemoved:
		reaction := payload.ReactionRemoved
		messageID := s.core.CanonicalReactionMessageEventID(reaction.GetRoomId(), reaction.GetMessageEventId())
		if err := appendTimeline(reaction.GetRoomId(), messageID); err != nil {
			return nil, false, err
		}
		if echoID, ok := s.core.ChannelEchoEventID(messageID); ok {
			if err := appendTimeline(reaction.GetRoomId(), echoID); err != nil {
				return nil, false, err
			}
		}
	case *evtv1.Event_AssetProcessingStarted,
		*evtv1.Event_AssetProcessingSucceeded,
		*evtv1.Event_AssetProcessingFailed,
		*evtv1.Event_AssetDeleted:
		roomID, messageEventID, ok := s.core.AssetEventTimelineTarget(evt)
		if !ok {
			// The owning message may already have removed this asset. Its
			// MessageEdited event is then the authoritative timeline update, but
			// this durable lifecycle fact must still advance the replay cursor.
			break
		}
		if err := appendTimeline(roomID, messageEventID); err != nil {
			return nil, false, err
		}
		if echoID, ok := s.core.ChannelEchoEventID(messageEventID); ok {
			if err := appendTimeline(roomID, echoID); err != nil {
				return nil, false, err
			}
		}
	case *evtv1.Event_VoiceCallStarted,
		*evtv1.Event_VoiceCallEnded:
		calls, err := s.connectAPI.BuildRealtimeProjectionActiveCalls(ctx, viewerID)
		if err != nil {
			return nil, false, err
		}
		appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_ActiveCalls{
			ActiveCalls: &realtimev1.RealtimeActiveCallsState{Calls: calls},
		}})
		if err := appendSourceTimeline(core.RoomIDOfEvent(evt)); err != nil {
			return nil, false, err
		}
	case *evtv1.Event_VoiceCallParticipantJoined,
		*evtv1.Event_VoiceCallParticipantLeft:
		calls, err := s.connectAPI.BuildRealtimeProjectionActiveCalls(ctx, viewerID)
		if err != nil {
			return nil, false, err
		}
		appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_ActiveCalls{
			ActiveCalls: &realtimev1.RealtimeActiveCallsState{Calls: calls},
		}})
	case *evtv1.Event_RoomDeleted:
		if err := appendViewerSensitiveResources(); err != nil {
			return nil, false, err
		}
		appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_RoomRemoved{
			RoomRemoved: &realtimev1.RealtimeRoomRemovedState{RoomId: payload.RoomDeleted.GetRoomId()},
		}})
	case *evtv1.Event_RoomCreated:
		roomID := payload.RoomCreated.GetRoomId()
		if err := appendRoom(roomID); err != nil {
			return nil, false, err
		}
		if err := appendSourceTimeline(roomID); err != nil {
			return nil, false, err
		}
	case *evtv1.Event_RoomUpdated:
		roomID := payload.RoomUpdated.GetRoomId()
		if err := appendRoom(roomID); err != nil {
			return nil, false, err
		}
		if err := appendSourceTimeline(roomID); err != nil {
			return nil, false, err
		}
	case *evtv1.Event_RoomArchived:
		roomID := payload.RoomArchived.GetRoomId()
		if err := appendViewerSensitiveResources(); err != nil {
			return nil, false, err
		}
		appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_RoomRemoved{
			RoomRemoved: &realtimev1.RealtimeRoomRemovedState{RoomId: roomID},
		}})
	case *evtv1.Event_RoomUnarchived:
		roomID := payload.RoomUnarchived.GetRoomId()
		if err := appendRoom(roomID); err != nil {
			return nil, false, err
		}
		if err := appendRoomTimelineIfMember(roomID); err != nil {
			return nil, false, err
		}
		if err := appendViewerSensitiveResources(); err != nil {
			return nil, false, err
		}
		if err := appendSourceTimeline(roomID); err != nil {
			return nil, false, err
		}
	case *evtv1.Event_RoomUniversalChanged:
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
			// timeline state in the same ordered realtime event as metadata.
			appendRoomTimelineClear(roomID)
		}
		if err := appendViewerSensitiveResources(); err != nil {
			return nil, false, err
		}
	case *evtv1.Event_RoomSlowModeChanged:
		if err := appendRoom(payload.RoomSlowModeChanged.GetRoomId()); err != nil {
			return nil, false, err
		}
	case *evtv1.Event_RoomThreadingModeChanged:
		roomID := payload.RoomThreadingModeChanged.GetRoomId()
		if err := appendRoom(roomID); err != nil {
			return nil, false, err
		}
		if err := appendSourceTimeline(roomID); err != nil {
			return nil, false, err
		}
	case *evtv1.Event_UserJoinedRoom:
		roomID := payload.UserJoinedRoom.GetRoomId()
		if err := appendRoom(roomID); err != nil {
			return nil, false, err
		}
		if evt.GetActorId() == viewerID {
			if err := appendRoomTimeline(roomID); err != nil {
				return nil, false, err
			}
			if err := appendViewerSensitiveResources(); err != nil {
				return nil, false, err
			}
		}
		if err := appendSourceTimeline(roomID); err != nil {
			return nil, false, err
		}
	case *evtv1.Event_UserLeftRoom:
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
	case *evtv1.Event_RoomMemberAdded:
		roomID := payload.RoomMemberAdded.GetRoomId()
		if err := appendRoom(roomID); err != nil {
			return nil, false, err
		}
		// AddMember publishes this audit fact before the authoritative
		// UserJoinedRoom membership fact. The following fact waits for the
		// membership projection and materialises a retained timeline, so doing
		// it here can race authorization and close the realtime socket.
	case *evtv1.Event_RoomMemberRemoved:
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
	case *evtv1.Event_RoomMemberBanned:
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
	case *evtv1.Event_RoomMemberUnbanned:
		if err := appendRoom(payload.RoomMemberUnbanned.GetRoomId()); err != nil {
			return nil, false, err
		}
	case *evtv1.Event_ThreadCreated:
		thread := payload.ThreadCreated
		if err := appendTimeline(thread.GetRoomId(), thread.GetThreadRootEventId()); err != nil {
			return nil, false, err
		}
	case *evtv1.Event_UserCustomStatusSet:
		if err := s.appendRealtimeProjectionUser(ctx, viewerID, payload.UserCustomStatusSet.GetUserId(), appendOperation); err != nil {
			return nil, false, err
		}
	case *evtv1.Event_UserCustomStatusCleared:
		if err := s.appendRealtimeProjectionUser(ctx, viewerID, payload.UserCustomStatusCleared.GetUserId(), appendOperation); err != nil {
			return nil, false, err
		}
	case *evtv1.Event_UserAccountCreated:
		if err := s.appendRealtimeProjectionUser(ctx, viewerID, payload.UserAccountCreated.GetUserId(), appendOperation); err != nil {
			return nil, false, err
		}
	case *evtv1.Event_UserLoginChanged:
		if err := s.appendRealtimeProjectionUser(ctx, viewerID, payload.UserLoginChanged.GetUserId(), appendOperation); err != nil {
			return nil, false, err
		}
	case *evtv1.Event_UserDisplayNameChanged:
		if err := s.appendRealtimeProjectionUser(ctx, viewerID, payload.UserDisplayNameChanged.GetUserId(), appendOperation); err != nil {
			return nil, false, err
		}
	case *evtv1.Event_UserBioChanged:
		if err := s.appendRealtimeProjectionUser(ctx, viewerID, payload.UserBioChanged.GetUserId(), appendOperation); err != nil {
			return nil, false, err
		}
	case *evtv1.Event_UserAvatarSet:
		if err := s.appendRealtimeProjectionUser(ctx, viewerID, payload.UserAvatarSet.GetUserId(), appendOperation); err != nil {
			return nil, false, err
		}
	case *evtv1.Event_UserAvatarCleared:
		if err := s.appendRealtimeProjectionUser(ctx, viewerID, payload.UserAvatarCleared.GetUserId(), appendOperation); err != nil {
			return nil, false, err
		}
	case *evtv1.Event_UserAccountDeleted:
		appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_UserRemoved{
			UserRemoved: &realtimev1.RealtimeUserRemovedState{UserId: payload.UserAccountDeleted.GetUserId()},
		}})
	default:
		return nil, false, nil
	}

	// Recognized durable facts may intentionally produce no operations when
	// they only affect a room timeline this connection has not materialised.
	// Keep the empty envelope so the client can safely advance its one cursor.
	return realtimeProjectionServerFrame(projection), true, nil
}

// canAdvanceSelfAuthoredRBAC identifies self-authored RBAC mutations that
// cannot change the current viewer's authorization. Effective owners retain
// every known permission after any RBAC edit. A human's direct permission
// update for their bot also cannot change the human's authorization. Other
// subscribers still reconnect and rebuild from current authorization.
func (s *HTTPServer) canAdvanceSelfAuthoredRBAC(ctx context.Context, viewerID string, event *evtv1.Event) (bool, error) {
	if event == nil || viewerID == "" || event.GetActorId() != viewerID {
		return false, nil
	}
	isOwner, err := s.core.IsServerOwner(ctx, viewerID)
	if err != nil {
		return false, fmt.Errorf("resolve effective owner for realtime RBAC delivery: %w", err)
	}
	if isOwner {
		return true, nil
	}
	var subject *evtv1.RbacPermissionSubject
	switch payload := event.GetEvent().(type) {
	case *evtv1.Event_RbacPermissionGranted:
		subject = payload.RbacPermissionGranted.GetSubject()
	case *evtv1.Event_RbacPermissionDenied:
		subject = payload.RbacPermissionDenied.GetSubject()
	case *evtv1.Event_RbacPermissionCleared:
		subject = payload.RbacPermissionCleared.GetSubject()
	default:
		return false, nil
	}
	if subject.GetKind() != evtv1.RbacPermissionSubjectKind_RBAC_PERMISSION_SUBJECT_KIND_USER || subject.GetId() == viewerID {
		return false, nil
	}
	target, err := s.core.GetUser(ctx, subject.GetId())
	if errors.Is(err, core.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("resolve RBAC permission target for realtime delivery: %w", err)
	}
	return target.GetIsBot(), nil
}

func realtimeProjectionServerState(state *connectapi.RealtimeProjectionServerState) *realtimev1.RealtimeServerState {
	if state == nil {
		return &realtimev1.RealtimeServerState{}
	}
	out := &realtimev1.RealtimeServerState{Runtime: state.Runtime}
	if state.MOTD != "" {
		out.Motd = &state.MOTD
	}
	return out
}

func realtimeProjectionRoom(room *connectapi.RealtimeProjectionRoom) *realtimev1.RealtimeRoomState {
	if room == nil {
		return &realtimev1.RealtimeRoomState{}
	}
	return &realtimev1.RealtimeRoomState{
		Room:              room.Room,
		MemberUserIds:     append([]string(nil), room.MemberUserIDs...),
		HasMessageHistory: room.HasMessageHistory,
	}
}

func realtimeProjectionNotificationOccurrences(notifications *connectapi.RealtimeProjectionNotifications) *realtimev1.RealtimeNotificationsState {
	if notifications == nil {
		return &realtimev1.RealtimeNotificationsState{}
	}
	return &realtimev1.RealtimeNotificationsState{
		Occurrences: notifications.Occurrences,
	}
}

func notificationReplacementContains(replacement *realtimev1.RealtimeNotificationsState, notificationID string) bool {
	if replacement == nil || notificationID == "" || replacement.GetOccurrences() == nil {
		return false
	}
	for _, occurrence := range replacement.GetOccurrences().GetOccurrences() {
		if occurrence.GetId() == notificationID {
			return true
		}
	}
	return false
}

func realtimeProjectionThreadViewerStates(states []*connectapi.RealtimeProjectionThreadViewerState) *realtimev1.RealtimeThreadViewerStatesState {
	out := &realtimev1.RealtimeThreadViewerStatesState{States: make([]*realtimev1.RealtimeThreadViewerState, 0, len(states))}
	for _, state := range states {
		if state == nil {
			continue
		}
		out.States = append(out.States, &realtimev1.RealtimeThreadViewerState{
			RoomId: state.RoomID, ThreadRootEventId: state.ThreadRootEventID, ViewerState: state.ViewerState,
		})
	}
	return out
}

func (s *HTTPServer) appendRealtimeProjectionUser(
	ctx context.Context,
	viewerID, userID string,
	appendOperation func(*realtimev1.RealtimeStateItem),
) error {
	user, err := s.connectAPI.BuildRealtimeProjectionUser(ctx, userID)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_UserRemoved{
				UserRemoved: &realtimev1.RealtimeUserRemovedState{UserId: userID},
			}})
			return nil
		}
		return fmt.Errorf("hydrate realtime user %q for viewer %q: %w", userID, viewerID, err)
	}
	appendOperation(&realtimev1.RealtimeStateItem{State: &realtimev1.RealtimeStateItem_User{User: user}})
	return nil
}

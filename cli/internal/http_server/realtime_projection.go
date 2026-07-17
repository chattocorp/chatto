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

func (s *HTTPServer) realtimeProjectionSnapshotFrames(ctx context.Context, userID string) ([]*realtimev1.RealtimeServerFrame, error) {
	if s.connectAPI == nil {
		return nil, errors.New("Connect API is unavailable")
	}
	snapshot, err := s.connectAPI.BuildRealtimeProjectionSnapshot(ctx, userID)
	if err != nil {
		return nil, err
	}

	frames := make([]*realtimev1.RealtimeServerFrame, 0, 4+len(snapshot.Users)+len(snapshot.Rooms)+len(snapshot.Timelines))
	appendOperation := func(operation *realtimev1.RealtimeProjectionOperation) {
		frames = append(frames, realtimeProjectionServerFrame(&realtimev1.RealtimeProjectionEvent{
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
	for _, timeline := range snapshot.Timelines {
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomTimelineReplace{
			RoomTimelineReplace: &realtimev1.RealtimeProjectionRoomTimelineReplace{RoomId: timeline.RoomID, Page: timeline.Page, EventCursors: timeline.EventCursors},
		}})
	}
	return frames, nil
}

func realtimeProjectionServerFrame(event *realtimev1.RealtimeProjectionEvent) *realtimev1.RealtimeServerFrame {
	return &realtimev1.RealtimeServerFrame{Frame: &realtimev1.RealtimeServerFrame_ProjectionEvent{ProjectionEvent: event}}
}

func (s *HTTPServer) realtimeProjectionFrameForEvent(ctx context.Context, viewerID string, event core.EventEnvelope) (*realtimev1.RealtimeServerFrame, bool, error) {
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
		cursor, err := s.core.RealtimeCursorForSequence(event.DeliverySeq())
		if err != nil {
			return nil, false, err
		}
		projection.ResumeCursor = &cursor
	}

	appendOperation := func(operation *realtimev1.RealtimeProjectionOperation) {
		projection.Operations = append(projection.Operations, operation)
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
			serverState, err := s.connectAPI.BuildRealtimeProjectionServerState(ctx)
			if err != nil {
				return nil, false, err
			}
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
		default:
			return nil, false, nil
		}
		return realtimeProjectionServerFrame(projection), true, nil
	}
	appendTimeline := func(roomID, messageEventID string, reaction *realtimev1.RealtimeProjectionReactionChange, retainDeletedRow ...bool) error {
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
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomTimelineEventRemove{
			RoomTimelineEventRemove: &realtimev1.RealtimeProjectionRoomTimelineEventRemove{RoomId: roomID, EventId: eventID},
		}})
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
	appendRoom := func(roomID string) error {
		room, err := s.connectAPI.BuildRealtimeProjectionRoom(ctx, viewerID, roomID)
		if errors.Is(err, core.ErrNotFound) || errors.Is(err, core.ErrPermissionDenied) || room == nil {
			appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomRemove{
				RoomRemove: &realtimev1.RealtimeProjectionRoomRemove{RoomId: roomID},
			}})
			return nil
		}
		if err != nil {
			return fmt.Errorf("hydrate realtime room %q: %w", roomID, err)
		}
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomUpsert{RoomUpsert: realtimeProjectionRoom(room)}})
		return nil
	}
	appendRoomTimeline := func(roomID string) error {
		timeline, err := s.connectAPI.BuildRealtimeProjectionRoomTimeline(ctx, viewerID, roomID)
		if err != nil {
			return fmt.Errorf("hydrate realtime room timeline %q: %w", roomID, err)
		}
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomTimelineReplace{
			RoomTimelineReplace: &realtimev1.RealtimeProjectionRoomTimelineReplace{RoomId: roomID, Page: timeline.Page, EventCursors: timeline.EventCursors},
		}})
		return nil
	}
	appendRoomTimelineClear := func(roomID string) {
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomTimelineReplace{
			RoomTimelineReplace: &realtimev1.RealtimeProjectionRoomTimelineReplace{RoomId: roomID, Page: &apiv1.RoomTimelinePage{}},
		}})
	}
	appendSourceTimeline := func(roomID string) error {
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
		if err := appendRoomViewerState(payload.MessagePosted.GetRoomId()); err != nil {
			return nil, false, err
		}
		if err := appendTimeline(payload.MessagePosted.GetRoomId(), evt.GetId(), nil); err != nil {
			return nil, false, err
		}
		// Deliver the reply before the authoritative root summary. Existing
		// reducers optimistically increment a root when ingesting a reply; the
		// following root upsert then converges that count instead of doubling it.
		if rootID := payload.MessagePosted.GetInThread(); rootID != "" {
			if err := appendTimeline(payload.MessagePosted.GetRoomId(), rootID, nil); err != nil {
				return nil, false, err
			}
		}
	case *corev1.Event_MessageEdited:
		roomID := payload.MessageEdited.GetRoomId()
		eventID := payload.MessageEdited.GetEventId()
		if s.core.IsHiddenChannelEcho(eventID) {
			appendTimelineRemove(roomID, eventID)
		} else if err := appendTimeline(roomID, eventID, nil); err != nil {
			return nil, false, err
		}
	case *corev1.Event_MessageRetracted:
		roomID := payload.MessageRetracted.GetRoomId()
		eventID := payload.MessageRetracted.GetEventId()
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
	case *corev1.Event_RoomDeleted:
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
		appendOperation(&realtimev1.RealtimeProjectionOperation{Operation: &realtimev1.RealtimeProjectionOperation_RoomRemove{
			RoomRemove: &realtimev1.RealtimeProjectionRoomRemove{RoomId: roomID},
		}})
	case *corev1.Event_RoomUnarchived:
		roomID := payload.RoomUnarchived.GetRoomId()
		if err := appendRoom(roomID); err != nil {
			return nil, false, err
		}
		if err := appendSourceTimeline(roomID); err != nil {
			return nil, false, err
		}
	case *corev1.Event_RoomUniversalChanged:
		roomID := payload.RoomUniversalChanged.GetRoomId()
		if err := appendRoom(roomID); err != nil {
			return nil, false, err
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
		}
	case *corev1.Event_RoomMemberBanned:
		roomID := payload.RoomMemberBanned.GetRoomId()
		if err := appendRoom(roomID); err != nil {
			return nil, false, err
		}
		if payload.RoomMemberBanned.GetUserId() == viewerID {
			appendRoomTimelineClear(roomID)
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

	if len(projection.Operations) == 0 {
		return nil, false, nil
	}
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

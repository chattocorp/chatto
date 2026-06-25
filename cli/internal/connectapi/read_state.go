package connectapi

import (
	"context"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

type readStateService struct {
	api *API
}

func (s *readStateService) MarkRoomAsRead(ctx context.Context, req *connect.Request[apiv1.MarkRoomAsReadRequest]) (*connect.Response[apiv1.MarkRoomAsReadResponse], error) {
	user, room, err := s.api.requireRoomMember(ctx, req.Msg.RoomId)
	if err != nil {
		return nil, err
	}
	kind := core.KindOfRoom(room)

	previousEventID, err := s.api.core.GetLastReadEventID(ctx, kind, user.Id, room.Id)
	if err != nil {
		return nil, connectError(err)
	}

	var (
		lastEventID string
		lastTime    time.Time
		hasLast     bool
	)
	if req.Msg.UpToEventId != "" {
		targetEventID, targetTime, found, terr := s.api.roomReadAnchor(ctx, room, req.Msg.UpToEventId)
		if terr != nil {
			return nil, terr
		}
		if found && !targetTime.IsZero() {
			lastEventID = targetEventID
			lastTime = targetTime
			hasLast = true
		}
	}
	if !hasLast {
		lastEventID, lastTime, hasLast, err = s.api.core.GetRoomLastEvent(ctx, kind, room.Id)
		if err != nil {
			return nil, connectError(err)
		}
	}

	if hasLast {
		advance, err := s.api.core.AdvanceLastReadEventID(ctx, kind, user.Id, room.Id, lastEventID)
		if err != nil {
			return nil, connectError(err)
		}
		if advance.CurrentEventID != "" {
			lastEventID = advance.CurrentEventID
			lastTime = advance.CurrentTime
		}
	}

	s.api.core.NotifyRoomMarkedAsRead(ctx, user.Id, kind, room.Id)

	resp := &apiv1.MarkRoomAsReadResponse{}
	if hasLast && !lastTime.IsZero() {
		resp.LastReadAt = timestamppb.New(lastTime)
	}
	if previousEventID != "" {
		if t, err := s.api.core.GetEventTimestamp(ctx, kind, room.Id, previousEventID); err == nil && !t.IsZero() {
			resp.PreviousLastReadAt = timestamppb.New(t)
		}
	}
	return connect.NewResponse(resp), nil
}

func (s *readStateService) MarkThreadAsRead(ctx context.Context, req *connect.Request[apiv1.MarkThreadAsReadRequest]) (*connect.Response[apiv1.MarkThreadAsReadResponse], error) {
	user, room, err := s.api.requireRoomMember(ctx, req.Msg.RoomId)
	if err != nil {
		return nil, err
	}
	kind := core.KindOfRoom(room)
	if _, err := s.api.requireThreadRoot(ctx, room, req.Msg.ThreadRootEventId); err != nil {
		return nil, err
	}

	markerEventID := ""
	if req.Msg.UpToEventId != "" {
		eventID, eerr := s.api.threadReadAnchor(ctx, room, req.Msg.ThreadRootEventId, req.Msg.UpToEventId)
		if eerr != nil {
			return nil, eerr
		}
		markerEventID = eventID
	} else {
		events, eerr := s.api.core.GetThreadEvents(ctx, kind, room.Id, req.Msg.ThreadRootEventId)
		if eerr != nil {
			return nil, connectError(eerr)
		}
		for i := len(events) - 1; i >= 0; i-- {
			if events[i] != nil && events[i].GetMessagePosted() != nil {
				markerEventID = events[i].Id
				break
			}
		}
	}

	previousReadAt, err := s.api.core.SetThreadLastReadEventID(ctx, kind, user.Id, room.Id, req.Msg.ThreadRootEventId, markerEventID)
	if err != nil {
		return nil, connectError(err)
	}

	resp := &apiv1.MarkThreadAsReadResponse{}
	if !previousReadAt.IsZero() {
		resp.PreviousReadAt = timestamppb.New(previousReadAt)
	}
	return connect.NewResponse(resp), nil
}

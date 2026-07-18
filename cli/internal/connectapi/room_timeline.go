package connectapi

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

const (
	roomTimelineCursorOpaquePrefix = "tl:"
	roomTimelineCursorVersion      = byte(1)
	roomTimelineCursorSize         = 9
	roomTimelineCursorPurpose      = "room-timeline-v1"
)

func (s *roomService) GetRoomEvents(ctx context.Context, req *connect.Request[apiv1.GetRoomEventsRequest]) (*connect.Response[apiv1.GetRoomEventsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	afterSeq, beforeSeq, err := s.api.roomTimelineCursorBounds(caller.UserID, req.Msg.RoomId, "", req.Msg.Cursor)
	if err != nil {
		return nil, err
	}

	input := core.RoomTimelineEventsInput{
		ActorID:   caller.UserID,
		RoomID:    req.Msg.RoomId,
		Limit:     int(req.Msg.Limit),
		AfterSeq:  afterSeq,
		BeforeSeq: beforeSeq,
	}

	result, err := s.api.core.RoomTimelineReads().GetRoomEvents(ctx, input)
	if err != nil {
		return nil, connectError(err)
	}

	page := result.Page
	resp, err := newRoomTimelineAssembler(s.api).buildPage(ctx, caller.UserID, result.Kind, page.Events, page.HasOlder, page.HasNewer)
	if err != nil {
		return nil, connectError(err)
	}
	resp.StartCursor, err = s.api.formatRoomTimelineCursor(caller.UserID, req.Msg.RoomId, "", page.StartCursorSeq)
	if err != nil {
		return nil, connectError(err)
	}
	resp.EndCursor, err = s.api.formatRoomTimelineCursor(caller.UserID, req.Msg.RoomId, "", page.EndCursorSeq)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.GetRoomEventsResponse{Page: resp}), nil
}

func (s *roomService) GetRoomEventsAround(ctx context.Context, req *connect.Request[apiv1.GetRoomEventsAroundRequest]) (*connect.Response[apiv1.GetRoomEventsAroundResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	result, err := s.api.core.RoomTimelineReads().GetRoomEventsAround(ctx, caller.UserID, req.Msg.RoomId, req.Msg.EventId, int(req.Msg.Limit))
	if err != nil {
		return nil, connectError(err)
	}
	around := result.Result
	page, err := newRoomTimelineAssembler(s.api).buildPage(ctx, caller.UserID, result.Kind, around.Events, around.HasOlder, around.HasNewer)
	if err != nil {
		return nil, connectError(err)
	}
	if len(around.Events) > 0 {
		page.StartCursor, err = s.api.formatRoomTimelineCursor(caller.UserID, req.Msg.RoomId, "", around.Events[0].Sequence)
		if err != nil {
			return nil, connectError(err)
		}
		page.EndCursor, err = s.api.formatRoomTimelineCursor(caller.UserID, req.Msg.RoomId, "", around.Events[len(around.Events)-1].Sequence)
		if err != nil {
			return nil, connectError(err)
		}
	}

	return connect.NewResponse(&apiv1.GetRoomEventsAroundResponse{
		Page:        page,
		TargetIndex: int32(around.TargetIndex),
	}), nil
}

func (s *messageService) GetMessage(ctx context.Context, req *connect.Request[apiv1.GetMessageRequest]) (*connect.Response[apiv1.GetMessageResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	result, err := s.api.core.RoomTimelineReads().GetMessage(ctx, caller.UserID, req.Msg.RoomId, req.Msg.EventId)
	if err != nil {
		return nil, connectError(err)
	}
	events, _, err := newRoomTimelineAssembler(s.api).hydrateEvents(ctx, caller.UserID, result.Kind, []*core.RoomEvent{{Event: result.Event}})
	if err != nil {
		return nil, connectError(err)
	}
	var message *apiv1.Message
	if len(events) > 0 {
		message = messageFromTimelineEvent(events[0])
	}

	return connect.NewResponse(&apiv1.GetMessageResponse{
		Message: message,
	}), nil
}

func (s *messageService) BatchGetMessages(ctx context.Context, req *connect.Request[apiv1.BatchGetMessagesRequest]) (*connect.Response[apiv1.BatchGetMessagesResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	result, err := s.api.core.RoomTimelineReads().BatchGetMessages(ctx, caller.UserID, req.Msg.RoomId, req.Msg.GetEventIds())
	if err != nil {
		return nil, connectError(err)
	}

	events := make([]*core.RoomEvent, 0, len(result.Events))
	for _, event := range result.Events {
		events = append(events, &core.RoomEvent{Event: event})
	}
	apiEvents, _, err := newRoomTimelineAssembler(s.api).hydrateEvents(ctx, caller.UserID, result.Kind, events)
	if err != nil {
		return nil, connectError(err)
	}
	messages := make([]*apiv1.Message, 0, len(apiEvents))
	for _, event := range apiEvents {
		if message := messageFromTimelineEvent(event); message != nil {
			messages = append(messages, message)
		}
	}

	return connect.NewResponse(&apiv1.BatchGetMessagesResponse{
		Messages: messages,
	}), nil
}

func (s *threadService) GetThreadEvents(ctx context.Context, req *connect.Request[apiv1.GetThreadEventsRequest]) (*connect.Response[apiv1.GetThreadEventsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	afterSeq, beforeSeq, err := s.api.roomTimelineCursorBounds(caller.UserID, req.Msg.RoomId, req.Msg.ThreadRootEventId, req.Msg.Cursor)
	if err != nil {
		return nil, err
	}

	input := core.ThreadTimelineEventsInput{
		ActorID:           caller.UserID,
		RoomID:            req.Msg.RoomId,
		ThreadRootEventID: req.Msg.ThreadRootEventId,
		Limit:             int(req.Msg.Limit),
		AfterSeq:          afterSeq,
		BeforeSeq:         beforeSeq,
	}

	result, err := s.api.core.RoomTimelineReads().GetThreadEvents(ctx, input)
	if err != nil {
		return nil, connectError(err)
	}

	page, err := newRoomTimelineAssembler(s.api).buildThreadPage(ctx, caller.UserID, req.Msg.RoomId, req.Msg.ThreadRootEventId, result.Kind, result.Root, result.Replies, result.IncludeRoot)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.GetThreadEventsResponse{Page: page}), nil
}

func (s *threadService) GetThreadEventsAround(ctx context.Context, req *connect.Request[apiv1.GetThreadEventsAroundRequest]) (*connect.Response[apiv1.GetThreadEventsAroundResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	result, err := s.api.core.RoomTimelineReads().GetThreadEventsAround(ctx, caller.UserID, req.Msg.RoomId, req.Msg.ThreadRootEventId, req.Msg.EventId, int(req.Msg.Limit))
	if err != nil {
		return nil, connectError(err)
	}
	page, err := newRoomTimelineAssembler(s.api).buildThreadPage(ctx, caller.UserID, req.Msg.RoomId, req.Msg.ThreadRootEventId, result.Kind, result.Root, result.Replies, true)
	if err != nil {
		return nil, connectError(err)
	}

	return connect.NewResponse(&apiv1.GetThreadEventsAroundResponse{
		Page:        page,
		TargetIndex: int32(result.TargetIndex),
	}), nil
}

func (a *API) formatRoomTimelineCursor(viewerID, roomID, threadRootEventID string, seq uint64) (string, error) {
	if seq == 0 {
		return "", nil
	}
	buf := make([]byte, roomTimelineCursorSize)
	buf[0] = roomTimelineCursorVersion
	binary.BigEndian.PutUint64(buf[1:], seq)
	token, err := a.core.SealPublicCursor(roomTimelineCursorPurpose, roomTimelineCursorScope(viewerID, roomID, threadRootEventID), buf)
	if err != nil {
		return "", fmt.Errorf("seal room timeline cursor: %w", err)
	}
	return roomTimelineCursorOpaquePrefix + token, nil
}

func (a *API) parseRoomTimelineCursor(viewerID, roomID, threadRootEventID, cursor string) (uint64, error) {
	if cursor == "" {
		return 0, nil
	}
	encoded, ok := strings.CutPrefix(cursor, roomTimelineCursorOpaquePrefix)
	if !ok {
		return 0, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid cursor format"))
	}
	raw, err := a.core.OpenPublicCursor(roomTimelineCursorPurpose, roomTimelineCursorScope(viewerID, roomID, threadRootEventID), encoded)
	if err != nil {
		return 0, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cursor encoding: %w", err))
	}
	if len(raw) != roomTimelineCursorSize || raw[0] != roomTimelineCursorVersion {
		return 0, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid cursor format"))
	}
	return binary.BigEndian.Uint64(raw[1:]), nil
}

func (a *API) roomTimelineCursorBounds(viewerID, roomID, threadRootEventID string, cursor any) (afterSeq, beforeSeq *uint64, err error) {
	switch cursor := cursor.(type) {
	case nil:
		return nil, nil, nil
	case *apiv1.GetRoomEventsRequest_After:
		seq, err := a.parseRoomTimelineCursor(viewerID, roomID, threadRootEventID, cursor.After)
		if err != nil {
			return nil, nil, err
		}
		return &seq, nil, nil
	case *apiv1.GetRoomEventsRequest_Before:
		seq, err := a.parseRoomTimelineCursor(viewerID, roomID, threadRootEventID, cursor.Before)
		if err != nil {
			return nil, nil, err
		}
		return nil, &seq, nil
	case *apiv1.GetThreadEventsRequest_After:
		seq, err := a.parseRoomTimelineCursor(viewerID, roomID, threadRootEventID, cursor.After)
		if err != nil {
			return nil, nil, err
		}
		return &seq, nil, nil
	case *apiv1.GetThreadEventsRequest_Before:
		seq, err := a.parseRoomTimelineCursor(viewerID, roomID, threadRootEventID, cursor.Before)
		if err != nil {
			return nil, nil, err
		}
		return nil, &seq, nil
	default:
		return nil, nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unsupported cursor type %T", cursor))
	}
}

// roomTimelineCursorScope binds an opaque sequence boundary to the exact
// authenticated timeline that issued it. IDs are server-generated and cannot
// contain NUL, which keeps the authenticated tuple unambiguous without placing
// any of its values inside the token.
func roomTimelineCursorScope(viewerID, roomID, threadRootEventID string) string {
	kind := "room"
	if threadRootEventID != "" {
		kind = "thread"
	}
	return kind + "\x00" + viewerID + "\x00" + roomID + "\x00" + threadRootEventID
}

func firstN(values []string, n int) []string {
	if len(values) <= n {
		return append([]string(nil), values...)
	}
	return append([]string(nil), values[:n]...)
}

func containsString(values []string, needle string) bool {
	for _, value := range values {
		if value == needle {
			return true
		}
	}
	return false
}

package connectapi

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"connectrpc.com/connect"
	"hmans.de/chatto/internal/core"
	apiv1 "hmans.de/chatto/internal/pb/chatto/api/v1"
)

const (
	roomTimelineCursorSeqPrefix    = "seq:"
	roomTimelineCursorOpaquePrefix = "tl:"
	roomTimelineCursorVersion      = byte(1)
	roomTimelineCursorSize         = 9
)

type roomTimelineService struct {
	api *API
}

func (s *roomTimelineService) GetRoomEvents(ctx context.Context, req *connect.Request[apiv1.GetRoomEventsRequest]) (*connect.Response[apiv1.GetRoomEventsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	afterSeq, beforeSeq, err := roomTimelineCursorBounds(req.Msg.Cursor)
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
	resp.StartCursor = formatRoomTimelineCursor(page.StartCursorSeq)
	resp.EndCursor = formatRoomTimelineCursor(page.EndCursorSeq)
	return connect.NewResponse(&apiv1.GetRoomEventsResponse{Page: resp}), nil
}

func (s *roomTimelineService) GetRoomEventsAround(ctx context.Context, req *connect.Request[apiv1.GetRoomEventsAroundRequest]) (*connect.Response[apiv1.GetRoomEventsAroundResponse], error) {
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
		page.StartCursor = formatRoomTimelineCursor(around.Events[0].Sequence)
		page.EndCursor = formatRoomTimelineCursor(around.Events[len(around.Events)-1].Sequence)
	}

	return connect.NewResponse(&apiv1.GetRoomEventsAroundResponse{
		Page:        page,
		TargetIndex: int32(around.TargetIndex),
	}), nil
}

func (s *roomTimelineService) ResolveMessageLinkTarget(ctx context.Context, req *connect.Request[apiv1.ResolveMessageLinkTargetRequest]) (*connect.Response[apiv1.ResolveMessageLinkTargetResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	result, err := s.api.core.RoomTimelineReads().ResolveMessageLinkTarget(ctx, caller.UserID, req.Msg.RoomId, req.Msg.EventId)
	if err != nil {
		return nil, connectError(err)
	}
	event, includes, err := newRoomTimelineAssembler(s.api).hydrateEvent(ctx, caller.UserID, result.Kind, result.Event)
	if err != nil {
		return nil, connectError(err)
	}

	return connect.NewResponse(&apiv1.ResolveMessageLinkTargetResponse{
		Event:             event,
		ThreadRootEventId: result.ThreadRootEventID,
		Includes:          includes,
	}), nil
}

func (s *roomTimelineService) GetThreadEvents(ctx context.Context, req *connect.Request[apiv1.GetThreadEventsRequest]) (*connect.Response[apiv1.GetThreadEventsResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	afterSeq, beforeSeq, err := roomTimelineCursorBounds(req.Msg.Cursor)
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

	page, err := newRoomTimelineAssembler(s.api).buildThreadPage(ctx, caller.UserID, result.Kind, result.Root, result.Replies, result.IncludeRoot)
	if err != nil {
		return nil, connectError(err)
	}
	return connect.NewResponse(&apiv1.GetThreadEventsResponse{Page: page}), nil
}

func (s *roomTimelineService) GetThreadEventsAround(ctx context.Context, req *connect.Request[apiv1.GetThreadEventsAroundRequest]) (*connect.Response[apiv1.GetThreadEventsAroundResponse], error) {
	caller, err := requireCaller(ctx)
	if err != nil {
		return nil, err
	}
	result, err := s.api.core.RoomTimelineReads().GetThreadEventsAround(ctx, caller.UserID, req.Msg.RoomId, req.Msg.ThreadRootEventId, req.Msg.EventId, int(req.Msg.Limit))
	if err != nil {
		return nil, connectError(err)
	}
	page, err := newRoomTimelineAssembler(s.api).buildThreadPage(ctx, caller.UserID, result.Kind, result.Root, result.Replies, true)
	if err != nil {
		return nil, connectError(err)
	}

	return connect.NewResponse(&apiv1.GetThreadEventsAroundResponse{
		Page:        page,
		TargetIndex: int32(result.TargetIndex),
	}), nil
}

func formatRoomTimelineCursor(seq uint64) string {
	if seq == 0 {
		return ""
	}
	buf := make([]byte, roomTimelineCursorSize)
	buf[0] = roomTimelineCursorVersion
	binary.BigEndian.PutUint64(buf[1:], seq)
	return roomTimelineCursorOpaquePrefix + base64.RawURLEncoding.EncodeToString(buf)
}

func parseRoomTimelineCursor(cursor string) (uint64, error) {
	if cursor == "" {
		return 0, nil
	}
	if rest, ok := strings.CutPrefix(cursor, roomTimelineCursorSeqPrefix); ok {
		seq, err := strconv.ParseUint(rest, 10, 64)
		if err != nil {
			return 0, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cursor sequence: %w", err))
		}
		return seq, nil
	}
	encoded, ok := strings.CutPrefix(cursor, roomTimelineCursorOpaquePrefix)
	if !ok {
		return 0, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid cursor format"))
	}
	raw, err := base64.RawURLEncoding.DecodeString(encoded)
	if err != nil {
		return 0, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid cursor encoding: %w", err))
	}
	if len(raw) != roomTimelineCursorSize || raw[0] != roomTimelineCursorVersion {
		return 0, connect.NewError(connect.CodeInvalidArgument, errors.New("invalid cursor format"))
	}
	return binary.BigEndian.Uint64(raw[1:]), nil
}

func roomTimelineCursorBounds(cursor any) (afterSeq, beforeSeq *uint64, err error) {
	switch cursor := cursor.(type) {
	case nil:
		return nil, nil, nil
	case *apiv1.GetRoomEventsRequest_After:
		seq, err := parseRoomTimelineCursor(cursor.After)
		if err != nil {
			return nil, nil, err
		}
		return &seq, nil, nil
	case *apiv1.GetRoomEventsRequest_Before:
		seq, err := parseRoomTimelineCursor(cursor.Before)
		if err != nil {
			return nil, nil, err
		}
		return nil, &seq, nil
	case *apiv1.GetThreadEventsRequest_After:
		seq, err := parseRoomTimelineCursor(cursor.After)
		if err != nil {
			return nil, nil, err
		}
		return &seq, nil, nil
	case *apiv1.GetThreadEventsRequest_Before:
		seq, err := parseRoomTimelineCursor(cursor.Before)
		if err != nil {
			return nil, nil, err
		}
		return nil, &seq, nil
	default:
		return nil, nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unsupported cursor type %T", cursor))
	}
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

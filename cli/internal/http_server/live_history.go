package http_server

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/protobuf/proto"
	"hmans.de/chatto/internal/core"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

const (
	clientLiveRequestRoomEvents         = "room.events"
	clientLiveRequestRoomEventsAround   = "room.eventsAround"
	clientLiveRequestRoomEvent          = "room.event"
	clientLiveRequestThreadEvents       = "thread.events"
	clientLiveRequestThreadEventsAround = "thread.eventsAround"

	defaultClientLiveRoomEventsLimit = 50
	maxClientLiveRoomEventsLimit     = 500
)

func (s *clientLiveSession) handleHistoryRequest(ctx context.Context, requestID uint64, req *corev1.ClientLiveRequest) (bool, string) {
	switch req.GetType() {
	case clientLiveRequestRoomEvents:
		var payload corev1.ClientRoomEventsRequest
		if !s.unmarshalRequest(requestID, req, &payload) {
			return true, "invalid_request"
		}
		response, err := s.handleRoomEventsRequest(ctx, &payload)
		return true, s.respondHistory(ctx, requestID, req.GetType(), response, err)
	case clientLiveRequestRoomEventsAround:
		var payload corev1.ClientRoomEventsAroundRequest
		if !s.unmarshalRequest(requestID, req, &payload) {
			return true, "invalid_request"
		}
		response, err := s.handleRoomEventsAroundRequest(ctx, &payload)
		return true, s.respondHistory(ctx, requestID, req.GetType(), response, err)
	case clientLiveRequestRoomEvent:
		var payload corev1.ClientRoomEventRequest
		if !s.unmarshalRequest(requestID, req, &payload) {
			return true, "invalid_request"
		}
		response, err := s.handleRoomEventRequest(ctx, &payload)
		return true, s.respondHistory(ctx, requestID, req.GetType(), response, err)
	case clientLiveRequestThreadEvents:
		var payload corev1.ClientThreadEventsRequest
		if !s.unmarshalRequest(requestID, req, &payload) {
			return true, "invalid_request"
		}
		response, err := s.handleThreadEventsRequest(ctx, &payload)
		return true, s.respondHistory(ctx, requestID, req.GetType(), response, err)
	case clientLiveRequestThreadEventsAround:
		var payload corev1.ClientThreadEventsAroundRequest
		if !s.unmarshalRequest(requestID, req, &payload) {
			return true, "invalid_request"
		}
		response, err := s.handleThreadEventsAroundRequest(ctx, &payload)
		return true, s.respondHistory(ctx, requestID, req.GetType(), response, err)
	default:
		return false, ""
	}
}

func (s *clientLiveSession) unmarshalRequest(requestID uint64, req *corev1.ClientLiveRequest, msg proto.Message) bool {
	if err := proto.Unmarshal(req.GetPayload(), msg); err != nil {
		s.enqueueError(requestID, "invalid_request", "invalid request payload", false)
		return false
	}
	return true
}

func (s *clientLiveSession) respondHistory(ctx context.Context, requestID uint64, responseType string, payload proto.Message, err error) string {
	if err != nil {
		return s.respondHistoryError(requestID, err)
	}
	raw, err := proto.Marshal(payload)
	if err != nil {
		s.server.logger.Warn("Failed to marshal client live response", "type", responseType, "error", err)
		s.enqueueError(requestID, "encode_failed", "failed to encode response", false)
		return "encode_failed"
	}
	select {
	case <-ctx.Done():
		return "cancelled"
	default:
		s.enqueue(&corev1.ClientLiveServerFrame{
			RequestId: requestID,
			Payload: &corev1.ClientLiveServerFrame_Response{
				Response: &corev1.ClientLiveResponse{Type: responseType, Payload: raw},
			},
		})
		return "ok"
	}
}

func (s *clientLiveSession) respondHistoryError(requestID uint64, err error) string {
	switch {
	case errors.Is(err, core.ErrNotRoomMember):
		s.enqueueError(requestID, "forbidden", "not a member of this room", false)
		return "forbidden"
	case errors.Is(err, core.ErrMessageNotFound):
		s.enqueueError(requestID, "not_found", "message not found", false)
		return "not_found"
	default:
		s.server.logger.Warn("Client live history request failed", "error", err)
		s.enqueueError(requestID, "request_failed", "request failed", false)
		return "request_failed"
	}
}

func (s *clientLiveSession) handleRoomEventsRequest(ctx context.Context, req *corev1.ClientRoomEventsRequest) (*corev1.ClientRoomEventsPage, error) {
	kind, err := s.authorizeRoomHistory(ctx, req.GetRoomId())
	if err != nil {
		return nil, err
	}

	limit := clientLiveRoomEventsLimit(req.GetLimit())
	var result *core.RoomEventsResult
	if req.GetAfterSeq() > 0 {
		result, err = s.server.core.GetRoomEventsAfter(ctx, kind, req.GetRoomId(), req.GetAfterSeq(), limit)
	} else {
		var beforeSeq *uint64
		if req.GetBeforeSeq() > 0 {
			seq := req.GetBeforeSeq()
			beforeSeq = &seq
		}
		result, err = s.server.core.GetRoomEvents(ctx, kind, req.GetRoomId(), limit, beforeSeq)
	}
	if err != nil {
		return nil, err
	}
	return s.liveRoomEventsPage(ctx, result)
}

func (s *clientLiveSession) handleRoomEventsAroundRequest(ctx context.Context, req *corev1.ClientRoomEventsAroundRequest) (*corev1.ClientRoomEventsAroundPage, error) {
	kind, err := s.authorizeRoomHistory(ctx, req.GetRoomId())
	if err != nil {
		return nil, err
	}
	result, err := s.server.core.GetRoomEventsAround(ctx, kind, req.GetRoomId(), req.GetEventId(), clientLiveRoomEventsLimit(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	items, err := s.liveRoomEventItems(ctx, result.Events)
	if err != nil {
		return nil, err
	}
	page := &corev1.ClientRoomEventsAroundPage{
		Events:      items,
		TargetIndex: int32(result.TargetIndex),
		HasOlder:    result.HasOlder,
		HasNewer:    result.HasNewer,
	}
	if len(result.Events) > 0 {
		page.StartCursorSeq = result.Events[0].Sequence
		page.EndCursorSeq = result.Events[len(result.Events)-1].Sequence
	}
	return page, nil
}

func (s *clientLiveSession) handleRoomEventRequest(ctx context.Context, req *corev1.ClientRoomEventRequest) (*corev1.ClientRoomEventResponse, error) {
	kind, err := s.authorizeRoomHistory(ctx, req.GetRoomId())
	if err != nil {
		return nil, err
	}
	event, err := s.server.core.GetRoomEventByEventID(ctx, kind, req.GetRoomId(), req.GetEventId())
	if err != nil {
		return nil, err
	}
	if event == nil {
		return &corev1.ClientRoomEventResponse{}, nil
	}
	seq, err := s.server.core.GetEventSequence(ctx, kind, req.GetRoomId(), req.GetEventId())
	if err != nil {
		return nil, err
	}
	item, err := s.liveRoomEventItem(ctx, &core.RoomEvent{Event: event, Sequence: seq})
	if err != nil {
		return nil, err
	}
	return &corev1.ClientRoomEventResponse{Event: item}, nil
}

func (s *clientLiveSession) handleThreadEventsRequest(ctx context.Context, req *corev1.ClientThreadEventsRequest) (*corev1.ClientRoomEventsPage, error) {
	kind, err := s.authorizeRoomHistory(ctx, req.GetRoomId())
	if err != nil {
		return nil, err
	}

	limit := clientLiveRoomEventsLimit(req.GetLimit())
	var beforeSeq *uint64
	if req.GetBeforeSeq() > 0 {
		seq := req.GetBeforeSeq()
		beforeSeq = &seq
	}
	var afterSeq *uint64
	if req.GetAfterSeq() > 0 {
		seq := req.GetAfterSeq()
		afterSeq = &seq
	}

	replies, err := s.server.core.GetThreadReplyEvents(ctx, kind, req.GetRoomId(), req.GetThreadRootEventId(), limit, beforeSeq, afterSeq)
	if err != nil {
		return nil, err
	}
	page, err := s.liveRoomEventsPage(ctx, replies)
	if err != nil {
		return nil, err
	}
	if beforeSeq != nil || afterSeq != nil {
		return page, nil
	}

	root, err := s.threadRootItem(ctx, kind, req.GetRoomId(), req.GetThreadRootEventId())
	if err != nil {
		return nil, err
	}
	if root != nil {
		page.Events = append([]*corev1.ClientRoomEventItem{root}, page.Events...)
	}
	return page, nil
}

func (s *clientLiveSession) handleThreadEventsAroundRequest(ctx context.Context, req *corev1.ClientThreadEventsAroundRequest) (*corev1.ClientRoomEventsPage, error) {
	kind, err := s.authorizeRoomHistory(ctx, req.GetRoomId())
	if err != nil {
		return nil, err
	}
	replies, err := s.server.core.GetThreadReplyEventsAround(ctx, kind, req.GetRoomId(), req.GetThreadRootEventId(), req.GetAnchorEventId(), clientLiveRoomEventsLimit(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	page, err := s.liveRoomEventsPage(ctx, replies)
	if err != nil {
		return nil, err
	}
	root, err := s.threadRootItem(ctx, kind, req.GetRoomId(), req.GetThreadRootEventId())
	if err != nil {
		return nil, err
	}
	if root != nil {
		page.Events = append([]*corev1.ClientRoomEventItem{root}, page.Events...)
	}
	return page, nil
}

func (s *clientLiveSession) authorizeRoomHistory(ctx context.Context, roomID string) (core.RoomKind, error) {
	if roomID == "" {
		return "", fmt.Errorf("room id is required")
	}
	kind, err := s.server.core.FindRoomKind(ctx, roomID)
	if err != nil {
		return "", err
	}
	member, err := s.server.core.RoomMembershipExists(ctx, kind, s.userID, roomID)
	if err != nil {
		return "", err
	}
	if !member {
		return "", core.ErrNotRoomMember
	}
	return kind, nil
}

func (s *clientLiveSession) liveRoomEventsPage(ctx context.Context, result *core.RoomEventsResult) (*corev1.ClientRoomEventsPage, error) {
	if result == nil {
		return &corev1.ClientRoomEventsPage{}, nil
	}
	items, err := s.liveRoomEventItems(ctx, result.Events)
	if err != nil {
		return nil, err
	}
	return &corev1.ClientRoomEventsPage{
		Events:         items,
		StartCursorSeq: result.StartCursorSeq,
		EndCursorSeq:   result.EndCursorSeq,
		HasOlder:       result.HasOlder,
		HasNewer:       result.HasNewer,
	}, nil
}

func (s *clientLiveSession) liveRoomEventItems(ctx context.Context, events []*core.RoomEvent) ([]*corev1.ClientRoomEventItem, error) {
	items := make([]*corev1.ClientRoomEventItem, 0, len(events))
	for _, event := range events {
		item, err := s.liveRoomEventItem(ctx, event)
		if err != nil {
			return nil, err
		}
		if item != nil {
			items = append(items, item)
		}
	}
	return items, nil
}

func (s *clientLiveSession) liveRoomEventItem(ctx context.Context, event *core.RoomEvent) (*corev1.ClientRoomEventItem, error) {
	if event == nil || event.Event == nil {
		return nil, nil
	}
	envelope := core.NewEVTEventEnvelopeWithDeliverySeq(event.Event, event.Sequence)
	live, err := s.clientLiveEventForEVT(ctx, envelope, event.Event)
	if err != nil {
		return nil, err
	}
	roomEvent := live.GetRoomEvent()
	if roomEvent == nil {
		return nil, fmt.Errorf("event %s is not a live room event", event.Event.GetId())
	}
	return &corev1.ClientRoomEventItem{
		Event:          roomEvent,
		StreamSequence: event.Sequence,
	}, nil
}

func (s *clientLiveSession) threadRootItem(ctx context.Context, kind core.RoomKind, roomID, threadRootEventID string) (*corev1.ClientRoomEventItem, error) {
	root, err := s.server.core.GetRoomEventByEventID(ctx, kind, roomID, threadRootEventID)
	if err != nil {
		return nil, err
	}
	if root == nil {
		return nil, core.ErrMessageNotFound
	}
	seq, err := s.server.core.GetEventSequence(ctx, kind, roomID, threadRootEventID)
	if err != nil {
		return nil, err
	}
	return s.liveRoomEventItem(ctx, &core.RoomEvent{Event: root, Sequence: seq})
}

func clientLiveRoomEventsLimit(limit int32) int {
	if limit <= 0 {
		return defaultClientLiveRoomEventsLimit
	}
	if limit > maxClientLiveRoomEventsLimit {
		return maxClientLiveRoomEventsLimit
	}
	return int(limit)
}

package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// RoomTimelineReads returns the operation-level model for user-facing room
// and thread timeline reads.
func (c *ChattoCore) RoomTimelineReads() *RoomTimelineReadModel {
	return c.roomTimelineReads
}

// RoomTimelineReadModel owns public timeline read authorization and target
// validation. It returns core event pages; transports remain responsible for
// cursor encoding and public DTO hydration.
type RoomTimelineReadModel struct {
	core *ChattoCore
}

type RoomTimelineEventsInput struct {
	ActorID   string
	RoomID    string
	Limit     int
	BeforeSeq *uint64
	AfterSeq  *uint64
}

type RoomTimelineEventsResult struct {
	Kind RoomKind
	Page *RoomEventsResult
}

type RoomTimelineAroundResult struct {
	Kind   RoomKind
	Result *RoomEventsAroundResult
}

type MessageReadResult struct {
	Kind  RoomKind
	Event *corev1.Event
}

type BatchMessagesReadResult struct {
	Kind   RoomKind
	Events []*corev1.Event
}

type ThreadTimelineEventsInput struct {
	ActorID           string
	RoomID            string
	ThreadRootEventID string
	Limit             int
	BeforeSeq         *uint64
	AfterSeq          *uint64
}

type ThreadTimelineEventsResult struct {
	Kind        RoomKind
	Root        *RoomEvent
	Replies     *RoomEventsResult
	IncludeRoot bool
}

type ThreadTimelineAroundResult struct {
	Kind        RoomKind
	Root        *RoomEvent
	Replies     *RoomEventsResult
	TargetIndex int
}

func (s *RoomTimelineReadModel) GetRoomEvents(ctx context.Context, input RoomTimelineEventsInput) (*RoomTimelineEventsResult, error) {
	room, kind, err := s.core.requireRoomMember(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return nil, err
	}

	var page *RoomEventsResult
	switch {
	case input.AfterSeq != nil:
		page, err = s.core.GetRoomEventsAfter(ctx, kind, room.Id, *input.AfterSeq, input.Limit)
	case input.BeforeSeq != nil:
		page, err = s.core.GetRoomEvents(ctx, kind, room.Id, input.Limit, input.BeforeSeq)
	default:
		page, err = s.core.GetRoomEvents(ctx, kind, room.Id, input.Limit, nil)
	}
	if err != nil {
		return nil, err
	}
	return &RoomTimelineEventsResult{Kind: kind, Page: page}, nil
}

func (s *RoomTimelineReadModel) GetRoomEventsAround(ctx context.Context, actorID, roomID, eventID string, limit int) (*RoomTimelineAroundResult, error) {
	room, kind, err := s.core.requireRoomMember(ctx, actorID, roomID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(eventID) == "" {
		return nil, invalidArgument("event_id is required")
	}

	result, err := s.core.GetRoomEventsAround(ctx, kind, room.Id, eventID, limit)
	if err != nil {
		return nil, err
	}
	return &RoomTimelineAroundResult{Kind: kind, Result: result}, nil
}

func (s *RoomTimelineReadModel) GetMessage(ctx context.Context, actorID, roomID, eventID string) (*MessageReadResult, error) {
	room, kind, err := s.core.requireRoomMember(ctx, actorID, roomID)
	if err != nil {
		return nil, err
	}
	event, err := s.messageEvent(ctx, kind, room.Id, eventID)
	if err != nil {
		return nil, err
	}
	return &MessageReadResult{Kind: kind, Event: event}, nil
}

func (s *RoomTimelineReadModel) BatchGetMessages(ctx context.Context, actorID, roomID string, eventIDs []string) (*BatchMessagesReadResult, error) {
	room, kind, err := s.core.requireRoomMember(ctx, actorID, roomID)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{}, len(eventIDs))
	events := make([]*corev1.Event, 0, len(eventIDs))
	for _, eventID := range eventIDs {
		if _, ok := seen[eventID]; ok {
			continue
		}
		seen[eventID] = struct{}{}

		event, err := s.messageEvent(ctx, kind, room.Id, eventID)
		if err != nil {
			if errors.Is(err, ErrMessageNotFound) {
				continue
			}
			return nil, err
		}
		events = append(events, event)
	}
	return &BatchMessagesReadResult{Kind: kind, Events: events}, nil
}

func (s *RoomTimelineReadModel) GetThreadEvents(ctx context.Context, input ThreadTimelineEventsInput) (*ThreadTimelineEventsResult, error) {
	room, kind, err := s.core.requireRoomMember(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return nil, err
	}
	root, err := s.threadRootEvent(ctx, kind, room.Id, input.ThreadRootEventID)
	if err != nil {
		return nil, err
	}

	includeRoot := true
	var replies *RoomEventsResult
	switch {
	case input.AfterSeq != nil:
		includeRoot = false
		replies, err = s.core.GetThreadReplyEvents(ctx, kind, room.Id, root.Event.Id, input.Limit, nil, input.AfterSeq)
	case input.BeforeSeq != nil:
		includeRoot = false
		replies, err = s.core.GetThreadReplyEvents(ctx, kind, room.Id, root.Event.Id, input.Limit, input.BeforeSeq, nil)
	default:
		replies, err = s.core.GetThreadReplyEvents(ctx, kind, room.Id, root.Event.Id, input.Limit, nil, nil)
	}
	if err != nil {
		return nil, err
	}
	return &ThreadTimelineEventsResult{
		Kind:        kind,
		Root:        root,
		Replies:     replies,
		IncludeRoot: includeRoot,
	}, nil
}

func (s *RoomTimelineReadModel) GetThreadEventsAround(ctx context.Context, actorID, roomID, threadRootEventID, eventID string, limit int) (*ThreadTimelineAroundResult, error) {
	room, kind, err := s.core.requireRoomMember(ctx, actorID, roomID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(eventID) == "" {
		return nil, invalidArgument("event_id is required")
	}
	root, err := s.threadRootEvent(ctx, kind, room.Id, threadRootEventID)
	if err != nil {
		return nil, err
	}

	replies, err := s.core.GetThreadReplyEventsAround(ctx, kind, room.Id, root.Event.Id, eventID, limit)
	if err != nil {
		return nil, err
	}
	return &ThreadTimelineAroundResult{
		Kind:        kind,
		Root:        root,
		Replies:     replies,
		TargetIndex: threadTimelineTargetIndex(root.Event.Id, eventID, replies.Events),
	}, nil
}

func (s *RoomTimelineReadModel) threadRootEvent(ctx context.Context, kind RoomKind, roomID, threadRootEventID string) (*RoomEvent, error) {
	event, err := s.core.requireThreadRoot(ctx, kind, roomID, threadRootEventID)
	if err != nil {
		return nil, err
	}
	seq, err := s.core.GetEventSequence(ctx, kind, roomID, threadRootEventID)
	if err != nil {
		return nil, err
	}
	if seq == 0 {
		return nil, fmt.Errorf("thread root event not found: %w", ErrNotFound)
	}
	return &RoomEvent{Event: event, Sequence: seq}, nil
}

func (s *RoomTimelineReadModel) messageEvent(ctx context.Context, kind RoomKind, roomID, eventID string) (*corev1.Event, error) {
	if strings.TrimSpace(eventID) == "" {
		return nil, invalidArgument("event_id is required")
	}
	event, err := s.core.GetRoomEventByEventID(ctx, kind, roomID, eventID)
	if err != nil {
		return nil, err
	}
	if event == nil || event.GetMessagePosted() == nil {
		return nil, ErrMessageNotFound
	}
	body, err := s.core.GetFullMessageBodyByEventID(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, ErrMessageNotFound
	}
	return event, nil
}

func threadTimelineTargetIndex(rootEventID, targetEventID string, replies []*RoomEvent) int {
	if targetEventID == rootEventID {
		return 0
	}
	for i, event := range replies {
		if event != nil && event.Event != nil && event.Event.Id == targetEventID {
			return i + 1
		}
	}
	return 0
}

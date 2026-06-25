package core

import (
	"context"
	"fmt"
	"strings"
)

// RoomTimelineReads returns the operation-level service for user-facing room
// and thread timeline reads.
func (c *ChattoCore) RoomTimelineReads() *RoomTimelineReadService {
	return c.roomTimelineReads
}

// RoomTimelineReadService owns public timeline read authorization and target
// validation. It returns core event pages; transports remain responsible for
// cursor encoding and public DTO hydration.
type RoomTimelineReadService struct {
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

func (s *RoomTimelineReadService) GetRoomEvents(ctx context.Context, input RoomTimelineEventsInput) (*RoomTimelineEventsResult, error) {
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

func (s *RoomTimelineReadService) GetRoomEventsAround(ctx context.Context, actorID, roomID, eventID string, limit int) (*RoomTimelineAroundResult, error) {
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

func (s *RoomTimelineReadService) GetThreadEvents(ctx context.Context, input ThreadTimelineEventsInput) (*ThreadTimelineEventsResult, error) {
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

func (s *RoomTimelineReadService) GetThreadEventsAround(ctx context.Context, actorID, roomID, threadRootEventID, eventID string, limit int) (*ThreadTimelineAroundResult, error) {
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

func (s *RoomTimelineReadService) threadRootEvent(ctx context.Context, kind RoomKind, roomID, threadRootEventID string) (*RoomEvent, error) {
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

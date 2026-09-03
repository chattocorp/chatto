package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"hmans.de/chatto/internal/encryption"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
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
	core  *ChattoCore
	rooms *RoomModel
}

// MessageHydrationState returns detached projection metadata for rendering one
// message. Timeline authorization belongs to the operation that supplied the
// message event; this method only interprets already-authorized projected
// state.
func (s *RoomTimelineReadModel) MessageHydrationState(eventID string) (RoomTimelineMessageHydrationState, error) {
	if s == nil || !s.rooms.hasTimeline() {
		return RoomTimelineMessageHydrationState{}, errors.New("room model unavailable")
	}
	return s.rooms.messageHydrationState(eventID), nil
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
	Event *evtv1.Event
}

type BatchMessagesReadResult struct {
	Kind   RoomKind
	Events []*evtv1.Event
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
	room, kind, err := s.core.requireRoomMessageReader(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return nil, err
	}
	visible, err := s.roomTimelineVisibility(ctx, input.ActorID, kind, room.Id)
	if err != nil {
		return nil, err
	}

	var page *RoomEventsResult
	switch {
	case input.AfterSeq != nil:
		page, err = s.core.getRoomEventsAfter(ctx, kind, room.Id, *input.AfterSeq, input.Limit, visible)
	case input.BeforeSeq != nil:
		page, err = s.core.getRoomEvents(ctx, kind, room.Id, input.Limit, input.BeforeSeq, visible)
	default:
		page, err = s.core.getRoomEvents(ctx, kind, room.Id, input.Limit, nil, visible)
	}
	if err != nil {
		return nil, err
	}
	return &RoomTimelineEventsResult{Kind: kind, Page: page}, nil
}

func (s *RoomTimelineReadModel) GetRoomEventsAround(ctx context.Context, actorID, roomID, eventID string, limit int) (*RoomTimelineAroundResult, error) {
	room, kind, err := s.core.requireRoomMessageReader(ctx, actorID, roomID)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(eventID) == "" {
		return nil, invalidArgument("event_id is required")
	}
	visible, err := s.roomTimelineVisibility(ctx, actorID, kind, room.Id)
	if err != nil {
		return nil, err
	}

	result, err := s.core.getRoomEventsAround(ctx, kind, room.Id, eventID, limit, visible)
	if err != nil {
		return nil, err
	}
	return &RoomTimelineAroundResult{Kind: kind, Result: result}, nil
}

func (s *RoomTimelineReadModel) GetMessage(ctx context.Context, actorID, roomID, eventID string) (*MessageReadResult, error) {
	room, kind, err := s.core.requireRoomMessageReader(ctx, actorID, roomID)
	if err != nil {
		return nil, err
	}
	if _, err := s.timelineMessageEntry(room.Id, eventID, false); err != nil {
		return nil, err
	}
	allowed, err := s.core.CanReadMessage(ctx, actorID, kind, room.Id, eventID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrPermissionDenied
	}
	event, err := s.messageEvent(ctx, kind, room.Id, eventID)
	if err != nil {
		return nil, err
	}
	return &MessageReadResult{Kind: kind, Event: event}, nil
}

// GetTimelineEvent returns a message's source event after applying current room
// membership and message-read authorization. Unlike GetMessage, it deliberately permits a
// deleted message whose encrypted body has already been erased so transports
// can hydrate the durable timeline tombstone.
func (s *RoomTimelineReadModel) GetTimelineEvent(ctx context.Context, actorID, roomID, eventID string) (*MessageReadResult, error) {
	room, kind, err := s.core.requireRoomMessageReader(ctx, actorID, roomID)
	if err != nil {
		return nil, err
	}
	if _, err := s.timelineMessageEntry(room.Id, eventID, true); err != nil {
		return nil, err
	}
	allowed, err := s.core.CanReadMessage(ctx, actorID, kind, room.Id, eventID)
	if err != nil {
		return nil, err
	}
	if !allowed {
		return nil, ErrPermissionDenied
	}
	event, err := s.timelineMessageEvent(ctx, kind, room.Id, eventID)
	if err != nil {
		return nil, err
	}
	return &MessageReadResult{Kind: kind, Event: event}, nil
}

func (s *RoomTimelineReadModel) BatchGetMessages(ctx context.Context, actorID, roomID string, eventIDs []string) (*BatchMessagesReadResult, error) {
	room, kind, err := s.core.requireRoomMessageReader(ctx, actorID, roomID)
	if err != nil {
		return nil, err
	}

	for attempt := 0; attempt < maxTimelineHydrationAttempts; attempt++ {
		seen := make(map[string]struct{}, len(eventIDs))
		entries := make([]*TimelineEntry, 0, len(eventIDs))
		bodyReferences := make([]TimelineBodyReference, 0, len(eventIDs))
		for _, eventID := range eventIDs {
			if _, ok := seen[eventID]; ok {
				continue
			}
			seen[eventID] = struct{}{}

			entry, err := s.timelineMessageEntry(room.Id, eventID, false)
			if errors.Is(err, ErrMessageNotFound) {
				continue
			}
			if err != nil {
				return nil, err
			}
			allowed, err := s.core.CanReadMessage(ctx, actorID, kind, room.Id, eventID)
			if err != nil {
				return nil, err
			}
			if !allowed {
				continue
			}
			bodyReference, retracted, known := s.core.roomModel.latestBodyReference(eventID)
			if !known || retracted || bodyReference.StreamSeq == 0 {
				continue
			}
			entries = append(entries, entry)
			bodyReferences = append(bodyReferences, bodyReference)
		}

		bodies, err := s.core.hydrateCurrentMessageBodies(ctx, bodyReferences)
		if errors.Is(err, errTimelineReadPlanStale) {
			continue
		}
		if err != nil {
			return nil, err
		}
		readableEntries := make([]*TimelineEntry, 0, len(entries))
		readableBodyReferences := make([]TimelineBodyReference, 0, len(entries))
		for i, body := range bodies {
			_, err := s.core.decryptMessageBody(ctx, entries[i].EventID, entries[i].RoomID, body)
			if errors.Is(err, encryption.ErrKeyNotFound) {
				continue
			}
			if err != nil {
				return nil, fmt.Errorf("failed to decrypt message body: %w", err)
			}
			readableEntries = append(readableEntries, entries[i])
			readableBodyReferences = append(readableBodyReferences, bodyReferences[i])
		}
		hydrated, err := s.core.hydrateTimelineEntries(ctx, readableEntries)
		if errors.Is(err, errTimelineReadPlanStale) {
			continue
		}
		if err != nil {
			return nil, err
		}
		current := true
		for _, reference := range readableBodyReferences {
			if !s.core.roomModel.timeline.Projection().BodyReferenceCurrent(reference) {
				current = false
				break
			}
		}
		if !current {
			continue
		}
		events := make([]*evtv1.Event, len(hydrated))
		for i, event := range hydrated {
			events[i] = event.Event
		}
		return &BatchMessagesReadResult{Kind: kind, Events: events}, nil
	}
	return nil, errTimelineReadPlanStale
}

func (s *RoomTimelineReadModel) GetThreadEvents(ctx context.Context, input ThreadTimelineEventsInput) (*ThreadTimelineEventsResult, error) {
	room, kind, err := s.core.requireThreadMessageReader(ctx, input.ActorID, input.RoomID, input.ThreadRootEventID)
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
	room, kind, err := s.core.requireThreadMessageReader(ctx, actorID, roomID, threadRootEventID)
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

func (s *RoomTimelineReadModel) roomTimelineVisibility(ctx context.Context, actorID string, kind RoomKind, roomID string) (func(*TimelineEntry) bool, error) {
	broad, err := s.core.CanReadMessages(ctx, actorID, kind, roomID)
	if err != nil {
		return nil, err
	}
	if broad || kind == KindDM {
		return nil, nil
	}
	interactions, err := s.core.CanReadMessageInteractions(ctx, actorID, kind, roomID)
	if err != nil {
		return nil, err
	}
	if !interactions {
		return nil, ErrPermissionDenied
	}
	return func(entry *TimelineEntry) bool {
		if entry == nil || entry.ThreadRootEventID == "" {
			return false
		}
		return s.core.roomModel.hasThreadInteraction(actorID, roomID, entry.ThreadRootEventID)
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

func (s *RoomTimelineReadModel) messageEvent(ctx context.Context, kind RoomKind, roomID, eventID string) (*evtv1.Event, error) {
	event, err := s.timelineMessageEvent(ctx, kind, roomID, eventID)
	if err != nil {
		return nil, err
	}
	body, err := s.core.GetFullMessageBody(ctx, eventID)
	if err != nil {
		return nil, err
	}
	if body == nil {
		return nil, ErrMessageNotFound
	}
	return event, nil
}

func (s *RoomTimelineReadModel) timelineMessageEvent(ctx context.Context, kind RoomKind, roomID, eventID string) (*evtv1.Event, error) {
	if _, err := s.timelineMessageEntry(roomID, eventID, true); err != nil {
		return nil, err
	}
	event, err := s.core.GetRoomEventByEventID(ctx, kind, roomID, eventID)
	if err != nil {
		return nil, err
	}
	if event == nil || event.GetMessagePosted() == nil {
		return nil, ErrMessageNotFound
	}
	return event, nil
}

func (s *RoomTimelineReadModel) timelineMessageEntry(roomID, eventID string, allowRetracted bool) (*TimelineEntry, error) {
	if strings.TrimSpace(eventID) == "" {
		return nil, invalidArgument("event_id is required")
	}
	entry, ok := s.core.roomModel.timelineEntry(eventID)
	if !ok || entry == nil || !entry.IsMessagePost() || entry.RoomID != roomID || s.core.roomModel.isHiddenEcho(eventID) {
		return nil, ErrMessageNotFound
	}
	if !allowRetracted {
		reference, retracted, known := s.core.roomModel.latestBodyReference(eventID)
		if !known || retracted || reference.StreamSeq == 0 {
			return nil, ErrMessageNotFound
		}
	}
	return entry, nil
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

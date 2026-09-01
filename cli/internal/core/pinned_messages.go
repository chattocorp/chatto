package core

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
	"hmans.de/chatto/pkg/events"
)

const maxPinnedMessageMutationAttempts = 5

// PinnedMessageMutationInput identifies one channel message pin association.
type PinnedMessageMutationInput struct {
	ActorID        string
	RoomID         string
	MessageEventID string
}

// PinnedMessageListInput describes one member-visible pin page.
type PinnedMessageListInput struct {
	ActorID string
	RoomID  string
	Limit   int
	Offset  int
}

// PinnedMessageItem pairs current pin metadata with the canonical message fact.
type PinnedMessageItem struct {
	Pin   PinnedMessageState
	Event *evtv1.Event
}

// PinnedMessageListResult is a stable newest-first page of active room pins.
type PinnedMessageListResult struct {
	Items            []PinnedMessageItem
	TotalCount       int
	HasMore          bool
	LatestPinEventID string
}

// ListPinnedMessages returns active pins for a channel room. A member with
// message-read authority may read them; direct-message rooms do not support
// pins.
func (s *RoomTimelineReadModel) ListPinnedMessages(ctx context.Context, input PinnedMessageListInput) (*PinnedMessageListResult, error) {
	room, kind, err := s.core.requireRoomMessageReader(ctx, input.ActorID, input.RoomID)
	if err != nil {
		return nil, err
	}
	if kind == KindDM {
		return nil, invalidArgument("DM rooms do not support pinned messages")
	}
	pins, latestPinEventID := s.core.roomModel.pinnedMessagesWithLatest(room.GetId())
	broad, err := s.core.CanReadMessages(ctx, input.ActorID, kind, room.GetId())
	if err != nil {
		return nil, err
	}
	readablePins := make([]PinnedMessageState, 0, len(pins))
	for _, pin := range pins {
		allowed, err := s.core.CanReadMessage(ctx, input.ActorID, kind, room.GetId(), pin.MessageEventID)
		if err != nil {
			return nil, err
		}
		if allowed {
			readablePins = append(readablePins, pin)
		}
	}
	pins = readablePins
	if !broad {
		latestPinEventID = ""
		if len(pins) > 0 {
			latestPinEventID = pins[0].PinEventID
		}
	}
	total := len(pins)
	start := min(max(input.Offset, 0), total)
	end := total
	if input.Limit > 0 {
		end = min(start+input.Limit, total)
	}
	items := make([]PinnedMessageItem, 0, end-start)
	for _, pin := range pins[start:end] {
		entry, ok := s.core.roomModel.timelineEntry(pin.MessageEventID)
		if !ok || entry == nil || entry.Event == nil || entry.Event.GetMessagePosted() == nil {
			continue
		}
		items = append(items, PinnedMessageItem{Pin: pin, Event: entry.Event})
	}
	return &PinnedMessageListResult{Items: items, TotalCount: total, HasMore: end < total, LatestPinEventID: latestPinEventID}, nil
}

// CreatePinnedMessage adds a canonical message to a channel's current pin set.
// The operation is idempotent and rechecks room.manage in each OCC attempt.
func (s *RoomCommandModel) CreatePinnedMessage(ctx context.Context, input PinnedMessageMutationInput) (PinnedMessageState, error) {
	return s.mutatePinnedMessage(ctx, input, true)
}

// DeletePinnedMessage removes a canonical message from a channel's current pin
// set. Removing an absent pin is a successful no-op.
func (s *RoomCommandModel) DeletePinnedMessage(ctx context.Context, input PinnedMessageMutationInput) (bool, error) {
	_, err := s.mutatePinnedMessage(ctx, input, false)
	return err == nil, err
}

func (s *RoomCommandModel) mutatePinnedMessage(ctx context.Context, input PinnedMessageMutationInput, create bool) (PinnedMessageState, error) {
	if strings.TrimSpace(input.MessageEventID) == "" {
		return PinnedMessageState{}, invalidArgument("message_event_id is required")
	}
	aggregate := evtstream.RoomAggregate(input.RoomID)
	filter := aggregate.AllEventsFilter()
	var event *evtv1.Event
	if create {
		event = newEvent(input.ActorID, &evtv1.Event{Event: &evtv1.Event_MessagePinned{MessagePinned: &evtv1.MessagePinnedEvent{RoomId: input.RoomID, MessageEventId: input.MessageEventID}}})
	} else {
		event = newEvent(input.ActorID, &evtv1.Event{Event: &evtv1.Event_MessageUnpinned{MessageUnpinned: &evtv1.MessageUnpinnedEvent{RoomId: input.RoomID, MessageEventId: input.MessageEventID}}})
	}

	for attempt := 0; attempt < maxPinnedMessageMutationAttempts; attempt++ {
		var kind RoomKind
		prepared, err := s.core.prepareMessageAppendAttempt(ctx, aggregate, func(ctx context.Context) error {
			room, memberKind, memberErr := s.core.requireRoomMember(ctx, input.ActorID, input.RoomID)
			if memberErr != nil {
				return memberErr
			}
			var authorizeErr error
			kind, authorizeErr = s.authorizeRoomManage(ctx, input.ActorID, input.RoomID)
			if authorizeErr != nil {
				return authorizeErr
			}
			if kind != memberKind {
				return ErrNotRoomMember
			}
			if room.GetArchived() {
				return ErrRoomArchived
			}
			if create {
				canRead, readErr := s.core.CanReadMessage(ctx, input.ActorID, memberKind, room.GetId(), input.MessageEventID)
				if readErr != nil {
					return readErr
				}
				if !canRead {
					return ErrPermissionDenied
				}
			}
			return nil
		})
		if err != nil {
			return PinnedMessageState{}, err
		}

		canonicalID, err := s.core.canonicalReactionMessageEventID(input.RoomID, input.MessageEventID)
		if err != nil {
			return PinnedMessageState{}, err
		}
		if canonicalID == "" {
			return PinnedMessageState{}, ErrMessageNotFound
		}
		if event.GetMessagePinned() != nil {
			event.GetMessagePinned().MessageEventId = canonicalID
		} else {
			event.GetMessageUnpinned().MessageEventId = canonicalID
		}
		current, exists := s.core.roomModel.pinnedMessage(input.RoomID, canonicalID)
		if create {
			entry, ok := s.core.roomModel.timelineEntry(canonicalID)
			if !ok || entry == nil || entry.Event == nil || entry.Event.GetMessagePosted() == nil {
				return PinnedMessageState{}, ErrMessageNotFound
			}
			if _, retracted, ok := s.core.roomModel.timeline.Projection().LatestBody(canonicalID); !ok || retracted {
				return PinnedMessageState{}, ErrMessageNotFound
			}
			if exists {
				return current, nil
			}
		} else if !exists {
			return PinnedMessageState{}, nil
		}

		sequences, err := s.core.EventPublisher.AppendBatch(ctx, []evtstream.BatchEntry{{
			Subject: aggregate.SubjectFor(event), Event: event, HasOCC: true,
			ExpectedSeq: prepared.roomSeq, FilterSubject: filter,
		}})
		if errors.Is(err, events.ErrConflict) {
			continue
		}
		if err != nil {
			return PinnedMessageState{}, err
		}
		if len(sequences) < 1 {
			return PinnedMessageState{}, errors.New("pinned message mutation committed no domain event")
		}
		if err := s.core.roomModel.waitForTimeline(ctx, events.SubjectPosition(aggregate.SubjectFor(event), sequences[0])); err != nil {
			return PinnedMessageState{}, fmt.Errorf("wait for pinned message projection: %w", err)
		}
		if create {
			return PinnedMessageState{
				PinEventID:     event.GetId(),
				PinSequence:    sequences[0],
				RoomID:         input.RoomID,
				MessageEventID: canonicalID,
			}, nil
		}
		return PinnedMessageState{}, nil
	}
	return PinnedMessageState{}, fmt.Errorf("pinned message mutation exceeded retry limit: %w", events.ErrConflict)
}

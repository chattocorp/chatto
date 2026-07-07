package core

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// MarkRoomAsReadResult describes the timestamp response for a room-level read
// marker update.
type MarkRoomAsReadResult struct {
	LastReadAt         time.Time
	PreviousLastReadAt time.Time
}

// MarkThreadAsReadResult describes the timestamp response for a thread-level
// read marker update.
type MarkThreadAsReadResult struct {
	PreviousReadAt time.Time
}

// ReadState returns the operation-level model for user-facing read marker
// updates. Public transports should authenticate at the edge, pass the actor
// ID here, and let this model own membership checks and anchor validation.
func (c *ChattoCore) ReadState() *ReadStateModel {
	return c.readStateModel
}

// ReadStateModel owns user-facing read marker mutations. Lower-level marker
// helpers stay available for trusted/internal callers, while this model keeps
// public API authorization and anchor semantics in one place.
type ReadStateModel struct {
	core *ChattoCore
}

func (s *ReadStateModel) MarkRoomAsRead(ctx context.Context, actorID, roomID, upToEventID string) (*MarkRoomAsReadResult, error) {
	room, kind, err := s.core.requireRoomMember(ctx, actorID, roomID)
	if err != nil {
		return nil, err
	}

	var (
		lastEventID string
		lastTime    time.Time
		hasLast     bool
	)
	if strings.TrimSpace(upToEventID) != "" {
		targetEventID, targetTime, found, err := s.roomReadAnchor(ctx, kind, room.Id, upToEventID)
		if err != nil {
			return nil, err
		}
		if found && !targetTime.IsZero() {
			lastEventID = targetEventID
			lastTime = targetTime
			hasLast = true
		}
	}
	if !hasLast {
		lastEventID, lastTime, hasLast, err = s.core.GetRoomLastEvent(ctx, kind, room.Id)
		if err != nil {
			return nil, err
		}
	}

	previousEventID, _, err := s.core.PeekLastReadEventID(ctx, actorID, room.Id)
	if err != nil {
		return nil, err
	}

	markerUpdated := false
	if hasLast {
		advance, err := s.core.AdvanceLastReadEventID(ctx, kind, actorID, room.Id, lastEventID)
		if err != nil {
			return nil, err
		}
		markerUpdated = advance.Updated
		if advance.CurrentEventID != "" {
			lastEventID = advance.CurrentEventID
			lastTime = advance.CurrentTime
		}
	}

	dismissedNotifications := 0
	if hasLast && !lastTime.IsZero() {
		dismissedNotifications = s.core.DismissRoomReadNotifications(ctx, kind, actorID, room.Id, lastTime)
	}
	if markerUpdated || dismissedNotifications > 0 {
		s.core.NotifyRoomMarkedAsRead(ctx, actorID, kind, room.Id)
	}

	result := &MarkRoomAsReadResult{}
	if hasLast && !lastTime.IsZero() {
		result.LastReadAt = lastTime
	}
	if previousEventID != "" {
		if previousTime, err := s.core.GetEventTimestamp(ctx, kind, room.Id, previousEventID); err == nil && !previousTime.IsZero() {
			result.PreviousLastReadAt = previousTime
		}
	}
	return result, nil
}

func (s *ReadStateModel) MarkThreadAsRead(ctx context.Context, actorID, roomID, threadRootEventID, upToEventID string) (*MarkThreadAsReadResult, error) {
	room, kind, err := s.core.requireRoomMember(ctx, actorID, roomID)
	if err != nil {
		return nil, err
	}
	if _, err := s.core.requireThreadRoot(ctx, kind, room.Id, threadRootEventID); err != nil {
		return nil, err
	}

	markerEventID := ""
	if strings.TrimSpace(upToEventID) != "" {
		eventID, err := s.threadReadAnchor(ctx, kind, room.Id, threadRootEventID, upToEventID)
		if err != nil {
			return nil, err
		}
		markerEventID = eventID
	} else {
		events, err := s.core.GetThreadEvents(ctx, kind, room.Id, threadRootEventID)
		if err != nil {
			return nil, err
		}
		for i := len(events) - 1; i >= 0; i-- {
			if events[i] != nil && events[i].GetMessagePosted() != nil {
				markerEventID = events[i].Id
				break
			}
		}
	}

	previousReadAt, err := s.core.SetThreadLastReadEventID(ctx, kind, actorID, room.Id, threadRootEventID, markerEventID)
	if err != nil {
		return nil, err
	}
	if markerEventID != "" {
		if markerTime, err := s.core.GetEventTimestamp(ctx, kind, room.Id, markerEventID); err == nil && !markerTime.IsZero() {
			s.core.DismissThreadReadNotifications(ctx, kind, actorID, room.Id, threadRootEventID, markerTime)
		}
	}
	return &MarkThreadAsReadResult{PreviousReadAt: previousReadAt}, nil
}

func (s *ReadStateModel) roomReadAnchor(ctx context.Context, kind RoomKind, roomID, eventID string) (eventIDOut string, ts time.Time, found bool, err error) {
	if strings.TrimSpace(eventID) == "" {
		return "", time.Time{}, false, nil
	}
	event, err := s.core.GetRoomEventByEventID(ctx, kind, roomID, eventID)
	if err != nil {
		return "", time.Time{}, false, err
	}
	if event == nil {
		return "", time.Time{}, false, fmt.Errorf("up_to_event_id event not found: %w", ErrNotFound)
	}
	message := event.GetMessagePosted()
	if message == nil || message.GetInThread() != "" || message.GetEchoOfEventId() != "" {
		return "", time.Time{}, false, invalidArgument("up_to_event_id must identify a root message in the room timeline")
	}
	if createdAt := event.GetCreatedAt(); createdAt != nil {
		return event.Id, createdAt.AsTime(), true, nil
	}
	return event.Id, time.Time{}, true, nil
}

func (s *ReadStateModel) threadReadAnchor(ctx context.Context, kind RoomKind, roomID, threadRootEventID, eventID string) (string, error) {
	if strings.TrimSpace(eventID) == "" {
		return "", nil
	}
	event, err := s.core.GetRoomEventByEventID(ctx, kind, roomID, eventID)
	if err != nil {
		return "", err
	}
	if event == nil {
		return "", fmt.Errorf("up_to_event_id event not found: %w", ErrNotFound)
	}
	message := event.GetMessagePosted()
	if message == nil {
		return "", invalidArgument("up_to_event_id must identify a message in the thread")
	}
	if event.Id == threadRootEventID {
		if message.GetInThread() == "" && message.GetEchoOfEventId() == "" {
			return event.Id, nil
		}
		return "", invalidArgument("up_to_event_id must identify a message in the thread")
	}
	if message.GetInThread() != threadRootEventID {
		return "", invalidArgument("up_to_event_id must identify a message in the thread")
	}
	return event.Id, nil
}

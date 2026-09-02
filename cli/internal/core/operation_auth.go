package core

import (
	"context"
	"fmt"
	"strings"

	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func requireAuthenticatedActor(actorID string) error {
	if strings.TrimSpace(actorID) == "" {
		return ErrNotAuthenticated
	}
	return nil
}

func (c *ChattoCore) requireRoomMember(ctx context.Context, actorID, roomID string) (*evtv1.Room, RoomKind, error) {
	if c.contentView == nil {
		return c.requireRoomMemberInView(ctx, actorID, roomID)
	}
	var room *evtv1.Room
	var kind RoomKind
	err := c.contentView.Read(func(uint64) error {
		var readErr error
		room, kind, readErr = c.requireRoomMemberInView(ctx, actorID, roomID)
		return readErr
	})
	return room, kind, err
}

func (c *ChattoCore) requireRoomMemberInView(ctx context.Context, actorID, roomID string) (*evtv1.Room, RoomKind, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, KindChannel, err
	}
	if strings.TrimSpace(roomID) == "" {
		return nil, KindChannel, invalidArgument("room_id is required")
	}

	room, err := c.FindRoomByID(ctx, roomID)
	if err != nil {
		return nil, KindChannel, err
	}
	kind := KindOfRoom(room)
	isMember, err := c.roomMembershipExists(ctx, kind, actorID, room.Id)
	if err != nil {
		return nil, KindChannel, err
	}
	if !isMember {
		return nil, KindChannel, ErrNotRoomMember
	}
	return room, kind, nil
}

// requireRoomMessageReader preserves membership as the first privacy boundary,
// then requires at least one configured room message-read mode. A caller that
// has only interaction-scoped access must still filter or authorize the target
// thread or message.
func (c *ChattoCore) requireRoomMessageReader(ctx context.Context, actorID, roomID string) (*evtv1.Room, RoomKind, error) {
	if c.contentView == nil {
		return c.requireRoomMessageReaderInView(ctx, actorID, roomID)
	}
	var room *evtv1.Room
	var kind RoomKind
	err := c.contentView.Read(func(uint64) error {
		var readErr error
		room, kind, readErr = c.requireRoomMessageReaderInView(ctx, actorID, roomID)
		return readErr
	})
	return room, kind, err
}

func (c *ChattoCore) requireRoomMessageReaderInView(ctx context.Context, actorID, roomID string) (*evtv1.Room, RoomKind, error) {
	room, kind, err := c.requireRoomMemberInView(ctx, actorID, roomID)
	if err != nil {
		return nil, KindChannel, err
	}
	allowed, err := c.canAccessRoomMessages(ctx, actorID, kind, room.GetId())
	if err != nil {
		return nil, KindChannel, err
	}
	if !allowed {
		return nil, KindChannel, ErrPermissionDenied
	}
	return room, kind, nil
}

func (c *ChattoCore) requireThreadMessageReader(ctx context.Context, actorID, roomID, threadRootEventID string) (*evtv1.Room, RoomKind, error) {
	if c.contentView == nil {
		return c.requireThreadMessageReaderInView(ctx, actorID, roomID, threadRootEventID)
	}
	var room *evtv1.Room
	var kind RoomKind
	err := c.contentView.Read(func(uint64) error {
		var readErr error
		room, kind, readErr = c.requireThreadMessageReaderInView(ctx, actorID, roomID, threadRootEventID)
		return readErr
	})
	return room, kind, err
}

func (c *ChattoCore) requireThreadMessageReaderInView(ctx context.Context, actorID, roomID, threadRootEventID string) (*evtv1.Room, RoomKind, error) {
	room, kind, err := c.requireRoomMemberInView(ctx, actorID, roomID)
	if err != nil {
		return nil, KindChannel, err
	}
	allowed, err := c.canReadThreadMessages(ctx, actorID, kind, room.GetId(), threadRootEventID)
	if err != nil {
		return nil, KindChannel, err
	}
	if !allowed {
		return nil, KindChannel, ErrPermissionDenied
	}
	return room, kind, nil
}

func (c *ChattoCore) requireMessageReader(ctx context.Context, actorID, roomID, messageEventID string) (*evtv1.Room, RoomKind, error) {
	if c.contentView == nil {
		return c.requireMessageReaderInView(ctx, actorID, roomID, messageEventID)
	}
	var room *evtv1.Room
	var kind RoomKind
	err := c.contentView.Read(func(uint64) error {
		var readErr error
		room, kind, readErr = c.requireMessageReaderInView(ctx, actorID, roomID, messageEventID)
		return readErr
	})
	return room, kind, err
}

func (c *ChattoCore) requireMessageReaderInView(ctx context.Context, actorID, roomID, messageEventID string) (*evtv1.Room, RoomKind, error) {
	room, kind, err := c.requireRoomMemberInView(ctx, actorID, roomID)
	if err != nil {
		return nil, KindChannel, err
	}
	allowed, err := c.canReadMessage(ctx, actorID, kind, room.GetId(), messageEventID)
	if err != nil {
		return nil, KindChannel, err
	}
	if !allowed {
		return nil, KindChannel, ErrPermissionDenied
	}
	return room, kind, nil
}

func (c *ChattoCore) requireThreadRoot(ctx context.Context, kind RoomKind, roomID, threadRootEventID string) (*evtv1.Event, error) {
	if strings.TrimSpace(threadRootEventID) == "" {
		return nil, invalidArgument("thread_root_event_id is required")
	}
	event, err := c.GetRoomEventByEventID(ctx, kind, roomID, threadRootEventID)
	if err != nil {
		return nil, err
	}
	if event == nil {
		return nil, fmt.Errorf("thread root event not found: %w", ErrNotFound)
	}
	message := event.GetMessagePosted()
	if message == nil || message.GetInThread() != "" || message.GetEchoOfEventId() != "" {
		return nil, invalidArgument("thread_root_event_id must identify a root message")
	}
	return event, nil
}

package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

func requireAuthenticatedActor(actorID string) error {
	if strings.TrimSpace(actorID) == "" {
		return ErrNotAuthenticated
	}
	return nil
}

func (a *AuthorizationModel) requireRoomMember(ctx context.Context, actorID, roomID string) (*readmodel.Room, RoomKind, error) {
	if a.view == nil {
		return a.requireRoomMemberInView(ctx, actorID, roomID)
	}
	var room *readmodel.Room
	var kind RoomKind
	err := a.view.ReadContext(ctx, func(readCtx context.Context, _ uint64) error {
		var readErr error
		room, kind, readErr = a.requireRoomMemberInView(readCtx, actorID, roomID)
		return readErr
	})
	return room, kind, err
}

func (a *AuthorizationModel) requireRoomMemberInView(ctx context.Context, actorID, roomID string) (*readmodel.Room, RoomKind, error) {
	if err := requireAuthenticatedActor(actorID); err != nil {
		return nil, KindChannel, err
	}
	if strings.TrimSpace(roomID) == "" {
		return nil, KindChannel, invalidArgument("room_id is required")
	}

	room, err := a.rooms.FindRoomByID(ctx, roomID)
	if err != nil {
		return nil, KindChannel, err
	}
	kind := KindOfRoom(room)
	isMember, err := a.roomMembershipExists(ctx, kind, actorID, room.Id)
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
func (a *AuthorizationModel) requireRoomMessageReader(ctx context.Context, actorID, roomID string) (*readmodel.Room, RoomKind, error) {
	if a.view == nil {
		return a.requireRoomMessageReaderInView(ctx, actorID, roomID)
	}
	var room *readmodel.Room
	var kind RoomKind
	err := a.view.ReadContext(ctx, func(readCtx context.Context, _ uint64) error {
		var readErr error
		room, kind, readErr = a.requireRoomMessageReaderInView(readCtx, actorID, roomID)
		return readErr
	})
	return room, kind, err
}

func (a *AuthorizationModel) requireRoomMessageReaderInView(ctx context.Context, actorID, roomID string) (*readmodel.Room, RoomKind, error) {
	room, kind, err := a.requireRoomMemberInView(ctx, actorID, roomID)
	if err != nil {
		return nil, KindChannel, err
	}
	allowed, err := a.canAccessRoomMessages(ctx, actorID, kind, room.GetId())
	if err != nil {
		return nil, KindChannel, err
	}
	if !allowed {
		return nil, KindChannel, ErrPermissionDenied
	}
	return room, kind, nil
}

func (a *AuthorizationModel) requireThreadMessageReader(ctx context.Context, actorID, roomID, threadRootEventID string) (*readmodel.Room, RoomKind, error) {
	if a.view == nil {
		return a.requireThreadMessageReaderInView(ctx, actorID, roomID, threadRootEventID)
	}
	var room *readmodel.Room
	var kind RoomKind
	err := a.view.ReadContext(ctx, func(readCtx context.Context, _ uint64) error {
		var readErr error
		room, kind, readErr = a.requireThreadMessageReaderInView(readCtx, actorID, roomID, threadRootEventID)
		return readErr
	})
	return room, kind, err
}

func (a *AuthorizationModel) requireThreadMessageReaderInView(ctx context.Context, actorID, roomID, threadRootEventID string) (*readmodel.Room, RoomKind, error) {
	room, kind, err := a.requireRoomMemberInView(ctx, actorID, roomID)
	if err != nil {
		return nil, KindChannel, err
	}
	allowed, err := a.canReadThreadMessages(ctx, actorID, kind, room.GetId(), threadRootEventID)
	if err != nil {
		return nil, KindChannel, err
	}
	if !allowed {
		return nil, KindChannel, ErrPermissionDenied
	}
	return room, kind, nil
}

func (a *AuthorizationModel) requireMessageReader(ctx context.Context, actorID, roomID, messageEventID string) (*readmodel.Room, RoomKind, error) {
	if a.view == nil {
		return a.requireMessageReaderInView(ctx, actorID, roomID, messageEventID)
	}
	var room *readmodel.Room
	var kind RoomKind
	err := a.view.ReadContext(ctx, func(readCtx context.Context, _ uint64) error {
		var readErr error
		room, kind, readErr = a.requireMessageReaderInView(readCtx, actorID, roomID, messageEventID)
		return readErr
	})
	return room, kind, err
}

func (a *AuthorizationModel) requireMessageReaderInView(ctx context.Context, actorID, roomID, messageEventID string) (*readmodel.Room, RoomKind, error) {
	room, kind, err := a.requireRoomMemberInView(ctx, actorID, roomID)
	if err != nil {
		return nil, KindChannel, err
	}
	allowed, err := a.canReadMessage(ctx, actorID, kind, room.GetId(), messageEventID)
	if err != nil {
		return nil, KindChannel, err
	}
	if !allowed {
		return nil, KindChannel, ErrPermissionDenied
	}
	return room, kind, nil
}

func (a *AuthorizationModel) requireThreadRoot(ctx context.Context, kind RoomKind, roomID, threadRootEventID string) (*evtv1.Event, error) {
	if strings.TrimSpace(threadRootEventID) == "" {
		return nil, invalidArgument("thread_root_event_id is required")
	}
	event, err := a.messages.GetRoomEventByEventID(ctx, kind, roomID, threadRootEventID)
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

package core

import (
	"context"

	"github.com/chattocorp/chatto/cli/internal/readmodel"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

// requireRoomMember delegates to shared authorization policy.
func (c *ChattoCore) requireRoomMember(ctx context.Context, actorID, roomID string) (*readmodel.Room, RoomKind, error) {
	return c.authorization.requireRoomMember(ctx, actorID, roomID)
}

// requireRoomMessageReader delegates to shared authorization policy.
func (c *ChattoCore) requireRoomMessageReader(ctx context.Context, actorID, roomID string) (*readmodel.Room, RoomKind, error) {
	return c.authorization.requireRoomMessageReader(ctx, actorID, roomID)
}

// requireThreadMessageReader delegates to shared authorization policy.
func (c *ChattoCore) requireThreadMessageReader(ctx context.Context, actorID, roomID, threadRootEventID string) (*readmodel.Room, RoomKind, error) {
	return c.authorization.requireThreadMessageReader(ctx, actorID, roomID, threadRootEventID)
}

// requireMessageReader delegates to shared authorization policy.
func (c *ChattoCore) requireMessageReader(ctx context.Context, actorID, roomID, messageEventID string) (*readmodel.Room, RoomKind, error) {
	return c.authorization.requireMessageReader(ctx, actorID, roomID, messageEventID)
}

// requireThreadRoot delegates to shared authorization policy.
func (c *ChattoCore) requireThreadRoot(ctx context.Context, kind RoomKind, roomID, threadRootEventID string) (*evtv1.Event, error) {
	return c.authorization.requireThreadRoot(ctx, kind, roomID, threadRootEventID)
}

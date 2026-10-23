package core

import (
	"context"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
)

// authorizationMessages supplies hydrated message facts for authorization.
// Core retains message hydration and canonical identity integration (ADR-115).
type authorizationMessages interface {
	GetRoomEventByEventID(ctx context.Context, kind RoomKind, roomID, eventID string) (*evtv1.Event, error)
	MessageEventThreadRoot(roomID string, event *evtv1.Event) (string, bool)
}

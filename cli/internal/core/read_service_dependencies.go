package core

import (
	"context"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	pubsubv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/pubsub/v1"
)

// readStateIntegration supplies the timeline hydration and live delivery that
// ReadStateModel uses but does not own. ChattoCore implements it.
type readStateIntegration interface {
	GetRoomEventByEventID(ctx context.Context, kind RoomKind, roomID, eventID string) (*evtv1.Event, error)
	GetThreadEvents(ctx context.Context, kind RoomKind, room_id string, threadRootEventId string) ([]*evtv1.Event, error)
	NotifyNotificationUnreadStateChanged(ctx context.Context, userID, actorID, roomID, threadRootEventID string)
	publishUserPubSubEvent(ctx context.Context, userID string, event *pubsubv1.PubSubEvent) error
}

// threadFollowIntegration supplies the event hydration and live delivery that
// ThreadFollowModel uses but does not own. ChattoCore implements it.
type threadFollowIntegration interface {
	GetRoomEventByEventID(ctx context.Context, kind RoomKind, roomID, eventID string) (*evtv1.Event, error)
	NotifyNotificationUnreadStateChanged(ctx context.Context, userID, actorID, roomID, threadRootEventID string)
}

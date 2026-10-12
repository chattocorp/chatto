package core

import (
	"context"

	evtv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/evt/v1"
	pubsubv1 "github.com/chattocorp/chatto/cli/internal/pb/chatto/core/pubsub/v1"
)

// readStateModelTimeline supplies timeline operations to ReadStateModel.
type readStateModelTimeline interface {
	GetRoomEventByEventID(ctx context.Context, kind RoomKind, roomID, eventID string) (*evtv1.Event, error)
	GetThreadEvents(ctx context.Context, kind RoomKind, room_id string, threadRootEventId string) ([]*evtv1.Event, error)
}

// threadFollowModelTimeline supplies timeline operations to ThreadFollowModel.
type threadFollowModelTimeline interface {
	GetRoomEventByEventID(ctx context.Context, kind RoomKind, roomID, eventID string) (*evtv1.Event, error)
	GetThreadMetadata(ctx context.Context, kind RoomKind, roomID string, rootEventId string) (*ThreadMetadata, error)
}

// readStateModelEffects supplies effects operations to ReadStateModel.
type readStateModelEffects interface {
	NotifyNotificationUnreadStateChanged(ctx context.Context, userID, actorID, roomID, threadRootEventID string)
	publishUserPubSubEvent(ctx context.Context, userID string, event *pubsubv1.PubSubEvent) error
}

// threadFollowModelEffects supplies effects operations to ThreadFollowModel.
type threadFollowModelEffects interface {
	NotifyNotificationUnreadStateChanged(ctx context.Context, userID, actorID, roomID, threadRootEventID string)
}

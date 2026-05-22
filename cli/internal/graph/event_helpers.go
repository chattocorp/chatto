package graph

import (
	"context"
	"errors"
	"fmt"

	"hmans.de/chatto/internal/core"
	corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"
)

// unwrapEvent extracts the concrete event payload from the proto
// Event oneof wrapper. Returns nil for an empty envelope or an
// unknown variant.
//
// For message events the wrapper's `Id` is copied into the payload's
// `EventId` field so nested resolvers (reactions, thread metadata) can
// reach it without re-traversing the envelope.
func unwrapEvent(event *corev1.Event) any {
	if event == nil || event.Event == nil {
		return nil
	}

	switch e := event.Event.(type) {
	// ---- Room lifecycle ----
	case *corev1.Event_RoomCreated:
		return e.RoomCreated
	case *corev1.Event_RoomUpdated:
		return e.RoomUpdated
	case *corev1.Event_RoomDeleted:
		return e.RoomDeleted
	case *corev1.Event_RoomArchived:
		return e.RoomArchived
	case *corev1.Event_RoomUnarchived:
		return e.RoomUnarchived

	// ---- Room membership ----
	case *corev1.Event_UserJoinedRoom:
		return e.UserJoinedRoom
	case *corev1.Event_UserLeftRoom:
		return e.UserLeftRoom
	case *corev1.Event_SpaceMemberDeleted:
		return e.SpaceMemberDeleted

	// ---- Messages ----
	case *corev1.Event_MessagePosted:
		// Populate EventId from wrapper for nested resolvers (reactions, thread metadata).
		e.MessagePosted.EventId = event.Id
		return e.MessagePosted
	case *corev1.Event_MessageUpdated:
		e.MessageUpdated.EventId = event.Id
		return e.MessageUpdated
	case *corev1.Event_MessageDeleted:
		return e.MessageDeleted

	// ---- Reactions ----
	case *corev1.Event_ReactionAdded:
		return e.ReactionAdded
	case *corev1.Event_ReactionRemoved:
		return e.ReactionRemoved

	// ---- Typing indicators ----
	case *corev1.Event_UserTyping:
		return e.UserTyping

	// ---- Video processing ----
	case *corev1.Event_VideoProcessingCompleted:
		return e.VideoProcessingCompleted

	// ---- Presence ----
	case *corev1.Event_PresenceChanged:
		return e.PresenceChanged

	// ---- Voice calls ----
	case *corev1.Event_CallParticipantJoined:
		return e.CallParticipantJoined
	case *corev1.Event_CallParticipantLeft:
		return e.CallParticipantLeft

	// ---- Subscription liveness ----
	case *corev1.Event_Heartbeat:
		return e.Heartbeat

	// ---- Server config ----
	case *corev1.Event_ConfigUpdated:
		return e.ConfigUpdated

	// ---- User lifecycle ----
	case *corev1.Event_UserCreated:
		return e.UserCreated
	case *corev1.Event_UserDeleted:
		return e.UserDeleted
	case *corev1.Event_UserProfileUpdated:
		return e.UserProfileUpdated
	case *corev1.Event_ServerUserPreferencesUpdated:
		return e.ServerUserPreferencesUpdated

	// ---- Notification level ----
	case *corev1.Event_NotificationLevelChanged:
		return e.NotificationLevelChanged

	// ---- Server lifecycle ----
	case *corev1.Event_ServerUpdated:
		return e.ServerUpdated
	// ServerCreated / ServerDeleted are intentionally dropped at the GraphQL
	// gateway: the server can't be created or deleted via the API anymore.

	// ---- Notifications ----
	case *corev1.Event_MentionNotification:
		return e.MentionNotification
	case *corev1.Event_NewDirectMessageNotification:
		return e.NewDirectMessageNotification
	case *corev1.Event_NotificationCreated:
		return e.NotificationCreated
	case *corev1.Event_NotificationDismissed:
		return e.NotificationDismissed

	// ---- Server unread ----
	case *corev1.Event_RoomMarkedAsRead:
		return e.RoomMarkedAsRead
	case *corev1.Event_MentionStatusCleared:
		return e.MentionStatusCleared

	// ---- Thread follow ----
	case *corev1.Event_ThreadFollowChanged:
		return e.ThreadFollowChanged

	// ---- Room sets ----
	case *corev1.Event_RoomGroupsUpdated:
		return e.RoomGroupsUpdated

	// ---- Session termination ----
	case *corev1.Event_SessionTerminated:
		return e.SessionTerminated

	default:
		return nil
	}
}

// resolveEventActor loads the actor User for an event envelope.
// Returns nil (without error) for system-authored events (empty ActorId)
// and for actors whose accounts have been deleted.
func (r *Resolver) resolveEventActor(ctx context.Context, event *corev1.Event) (*corev1.User, error) {
	if event.ActorId == "" {
		return nil, nil
	}
	user, err := r.getUser(ctx, event.ActorId)
	if err != nil {
		if errors.Is(err, core.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return user, nil
}

// unwrapEventAs unwraps a proto Event and asserts the payload to the
// requested GraphQL union interface (model.RoomEventType or
// model.ServerEventType). Returns a typed error for nil payloads and
// for variants that don't belong to the requested union — the latter
// is normal at the room-history boundary, where deployment-event
// variants in the proto can't appear in stored room history but the
// type system requires the assertion anyway.
func unwrapEventAs[T any](event *corev1.Event, unionName string) (T, error) {
	var zero T
	unwrapped := unwrapEvent(event)
	if unwrapped == nil {
		return zero, fmt.Errorf("unknown event variant")
	}
	typed, ok := unwrapped.(T)
	if !ok {
		return zero, fmt.Errorf("event does not implement %s: %T", unionName, unwrapped)
	}
	return typed, nil
}


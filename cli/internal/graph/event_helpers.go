package graph

import corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"

// SpaceScoped represents events that belong to a specific space.
// This interface matches the protoc-generated GetSpaceId() methods.
type SpaceScoped interface {
	GetSpaceId() string
}

// RoomScoped represents events that belong to a specific room.
// This interface matches the protoc-generated GetRoomId() methods.
type RoomScoped interface {
	GetRoomId() string
}

// unwrapSpaceEvent extracts the concrete event from the SpaceEvent oneof wrapper.
// For message events, it populates EventId from the wrapper for nested resolvers.
func unwrapSpaceEvent(event *corev1.SpaceEvent) any {
	if event == nil || event.Event == nil {
		return nil
	}

	switch e := event.Event.(type) {
	// Room lifecycle events
	case *corev1.SpaceEvent_RoomCreated:
		return e.RoomCreated
	case *corev1.SpaceEvent_RoomUpdated:
		return e.RoomUpdated
	case *corev1.SpaceEvent_RoomDeleted:
		return e.RoomDeleted
	case *corev1.SpaceEvent_RoomArchived:
		return e.RoomArchived
	case *corev1.SpaceEvent_RoomUnarchived:
		return e.RoomUnarchived

	// Room membership events
	case *corev1.SpaceEvent_UserJoinedRoom:
		return e.UserJoinedRoom
	case *corev1.SpaceEvent_UserLeftRoom:
		return e.UserLeftRoom
	case *corev1.SpaceEvent_SpaceMemberDeleted:
		return e.SpaceMemberDeleted

	// Message events
	case *corev1.SpaceEvent_MessagePosted:
		// Populate EventId from wrapper for nested resolvers (reactions, thread metadata)
		e.MessagePosted.EventId = event.Id
		return e.MessagePosted
	case *corev1.SpaceEvent_MessageUpdated:
		e.MessageUpdated.EventId = event.Id
		return e.MessageUpdated
	case *corev1.SpaceEvent_MessageDeleted:
		return e.MessageDeleted

	// Reaction events
	case *corev1.SpaceEvent_ReactionAdded:
		return e.ReactionAdded
	case *corev1.SpaceEvent_ReactionRemoved:
		return e.ReactionRemoved

	// Typing indicator events
	case *corev1.SpaceEvent_UserTyping:
		return e.UserTyping

	// Video processing events
	case *corev1.SpaceEvent_VideoProcessingCompleted:
		return e.VideoProcessingCompleted

	// Presence events
	case *corev1.SpaceEvent_PresenceChanged:
		return e.PresenceChanged

	// Voice call events
	case *corev1.SpaceEvent_CallParticipantJoined:
		return e.CallParticipantJoined
	case *corev1.SpaceEvent_CallParticipantLeft:
		return e.CallParticipantLeft

	default:
		return nil
	}
}

// unwrapInstanceEvent extracts the concrete event from the InstanceEvent oneof wrapper.
func unwrapInstanceEvent(event *corev1.InstanceEvent) any {
	if event == nil || event.Event == nil {
		return nil
	}

	switch e := event.Event.(type) {
	// Instance config events
	case *corev1.InstanceEvent_ConfigUpdated:
		return e.ConfigUpdated

	// User lifecycle events
	case *corev1.InstanceEvent_UserCreated:
		return e.UserCreated
	case *corev1.InstanceEvent_UserDeleted:
		return e.UserDeleted
	case *corev1.InstanceEvent_UserProfileUpdated:
		return e.UserProfileUpdated
	case *corev1.InstanceEvent_InstanceUserPreferencesUpdated:
		return e.InstanceUserPreferencesUpdated

	// Notification level events
	case *corev1.InstanceEvent_NotificationLevelChanged:
		return e.NotificationLevelChanged

	// Server membership events (instance-level)
	case *corev1.InstanceEvent_UserJoinedSpace:
		return e.UserJoinedSpace
	case *corev1.InstanceEvent_UserLeftSpace:
		return e.UserLeftSpace

	// Server lifecycle events
	case *corev1.InstanceEvent_SpaceUpdated:
		return e.SpaceUpdated
	// SpaceCreated / SpaceDeleted are intentionally dropped at the GraphQL
	// gateway: the server can't be created or deleted via the API anymore.

	// Notification events
	case *corev1.InstanceEvent_MentionNotification:
		return e.MentionNotification
	case *corev1.InstanceEvent_NewDirectMessageNotification:
		return e.NewDirectMessageNotification
	case *corev1.InstanceEvent_NotificationCreated:
		return e.NotificationCreated
	case *corev1.InstanceEvent_NotificationDismissed:
		return e.NotificationDismissed

	// Server unread events
	case *corev1.InstanceEvent_NewMessageInSpace:
		return e.NewMessageInSpace
	case *corev1.InstanceEvent_RoomMarkedAsRead:
		return e.RoomMarkedAsRead

	// Thread follow events
	case *corev1.InstanceEvent_ThreadFollowChanged:
		return e.ThreadFollowChanged

	// Room layout events
	case *corev1.InstanceEvent_RoomLayoutUpdated:
		return e.RoomLayoutUpdated

	// Session termination events
	case *corev1.InstanceEvent_SessionTerminated:
		return e.SessionTerminated

	default:
		return nil
	}
}

// GetEventSpaceID extracts the space_id from a SpaceEvent if present.
// Returns nil if the event doesn't have a space_id field.
func GetEventSpaceID(event *corev1.SpaceEvent) *string {
	concrete := unwrapSpaceEvent(event)
	if scoped, ok := concrete.(SpaceScoped); ok {
		id := scoped.GetSpaceId()
		return &id
	}
	return nil
}

// GetEventRoomID extracts the room_id from a SpaceEvent if present.
// Returns nil if the event doesn't have a room_id field.
func GetEventRoomID(event *corev1.SpaceEvent) *string {
	concrete := unwrapSpaceEvent(event)
	if scoped, ok := concrete.(RoomScoped); ok {
		id := scoped.GetRoomId()
		return &id
	}
	return nil
}

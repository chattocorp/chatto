package core

import (
	"hmans.de/chatto/internal/evtstream"
	evtv1 "hmans.de/chatto/internal/pb/chatto/core/evt/v1"
)

func messageAuthorID(event *evtv1.Event) string {
	if event != nil {
		return event.GetActorId()
	}
	return ""
}

func roomIDOfEvent(event *evtv1.Event) string {
	if event == nil {
		return ""
	}
	switch e := event.GetEvent().(type) {
	case *evtv1.Event_RoomCreated:
		return e.RoomCreated.GetRoomId()
	case *evtv1.Event_RoomUpdated:
		return e.RoomUpdated.GetRoomId()
	case *evtv1.Event_RoomDeleted:
		return e.RoomDeleted.GetRoomId()
	case *evtv1.Event_RoomArchived:
		return e.RoomArchived.GetRoomId()
	case *evtv1.Event_RoomUnarchived:
		return e.RoomUnarchived.GetRoomId()
	case *evtv1.Event_RoomUniversalChanged:
		return e.RoomUniversalChanged.GetRoomId()
	case *evtv1.Event_RoomSlowModeChanged:
		return e.RoomSlowModeChanged.GetRoomId()
	case *evtv1.Event_RoomThreadingModeChanged:
		return e.RoomThreadingModeChanged.GetRoomId()
	case *evtv1.Event_UserJoinedRoom:
		return e.UserJoinedRoom.GetRoomId()
	case *evtv1.Event_UserLeftRoom:
		return e.UserLeftRoom.GetRoomId()
	case *evtv1.Event_RoomMemberBanned:
		return e.RoomMemberBanned.GetRoomId()
	case *evtv1.Event_RoomMemberUnbanned:
		return e.RoomMemberUnbanned.GetRoomId()
	case *evtv1.Event_RoomMemberAdded:
		return e.RoomMemberAdded.GetRoomId()
	case *evtv1.Event_RoomMemberRemoved:
		return e.RoomMemberRemoved.GetRoomId()
	case *evtv1.Event_MessagePosted:
		return e.MessagePosted.GetRoomId()
	case *evtv1.Event_MessageEdited:
		return e.MessageEdited.GetRoomId()
	case *evtv1.Event_MessageRetracted:
		return e.MessageRetracted.GetRoomId()
	case *evtv1.Event_MessageBody:
		return e.MessageBody.GetRoomId()
	case *evtv1.Event_MessagePinned:
		return e.MessagePinned.GetRoomId()
	case *evtv1.Event_MessageUnpinned:
		return e.MessageUnpinned.GetRoomId()
	case *evtv1.Event_ThreadCreated:
		return e.ThreadCreated.GetRoomId()
	case *evtv1.Event_ThreadFollowed:
		return e.ThreadFollowed.GetRoomId()
	case *evtv1.Event_ThreadUnfollowed:
		return e.ThreadUnfollowed.GetRoomId()
	case *evtv1.Event_AssetCreated:
		return ""
	case *evtv1.Event_ReactionAdded:
		return e.ReactionAdded.GetRoomId()
	case *evtv1.Event_ReactionRemoved:
		return e.ReactionRemoved.GetRoomId()
	case *evtv1.Event_VoiceCallParticipantJoined:
		return e.VoiceCallParticipantJoined.GetRoomId()
	case *evtv1.Event_VoiceCallParticipantLeft:
		return e.VoiceCallParticipantLeft.GetRoomId()
	case *evtv1.Event_VoiceCallStarted:
		return e.VoiceCallStarted.GetRoomId()
	case *evtv1.Event_VoiceCallEnded:
		return e.VoiceCallEnded.GetRoomId()
	}
	return ""
}

// RoomIDOfEvent returns the room aggregate ID carried by a durable event.
func RoomIDOfEvent(event *evtv1.Event) string {
	return roomIDOfEvent(event)
}

// MessageReadProtectedEventRoomID identifies a durable fact whose public
// delivery can expose message content or message-specific metadata.
func (c *ChattoCore) MessageReadProtectedEventRoomID(event *evtv1.Event) (string, bool) {
	if event == nil {
		return "", false
	}
	switch event.GetEvent().(type) {
	case *evtv1.Event_MessagePosted,
		*evtv1.Event_MessageEdited,
		*evtv1.Event_MessageRetracted,
		*evtv1.Event_MessageBody,
		*evtv1.Event_MessagePinned,
		*evtv1.Event_MessageUnpinned,
		*evtv1.Event_ThreadCreated,
		*evtv1.Event_ThreadFollowed,
		*evtv1.Event_ThreadUnfollowed,
		*evtv1.Event_ReactionAdded,
		*evtv1.Event_ReactionRemoved:
		roomID := roomIDOfEvent(event)
		return roomID, roomID != ""
	case *evtv1.Event_AssetCreated:
		roomID := event.GetAssetCreated().GetRoomId()
		return roomID, roomID != ""
	case *evtv1.Event_AssetAttached:
		roomID := event.GetAssetAttached().GetRoomId()
		return roomID, roomID != ""
	case *evtv1.Event_AssetProcessingStarted,
		*evtv1.Event_AssetProcessingSucceeded,
		*evtv1.Event_AssetProcessingFailed,
		*evtv1.Event_AssetDeleted:
		roomID, _, ok := c.AssetEventTimelineTarget(event)
		return roomID, ok
	default:
		return "", false
	}
}

// MessageEventThreadRoot resolves the canonical channel-room thread affected
// by one message-derived fact. The Threads and Assets projections must be
// current through the fact before callers use this result for authorization.
func (c *ChattoCore) MessageEventThreadRoot(roomID string, event *evtv1.Event) (string, bool) {
	if event == nil || roomID == "" {
		return "", false
	}
	messageRoot := func(eventID string) (string, bool) {
		if eventID == "" {
			return "", false
		}
		if rootID, ok := c.roomModel.threadRootForMessage(roomID, eventID); ok {
			return rootID, true
		}
		canonicalID, err := c.canonicalReactionMessageEventID(roomID, eventID)
		if err != nil || canonicalID == "" || canonicalID == eventID {
			return "", false
		}
		return c.roomModel.threadRootForMessage(roomID, canonicalID)
	}

	messageEventID, ok := c.MessageEventSourceMessageID(roomID, event)
	if !ok {
		return "", false
	}
	return messageRoot(messageEventID)
}

// MessageEventSourceMessageID returns the message that owns one protected
// message-derived fact. Asset creation has no message owner until attachment
// and therefore returns false.
func (c *ChattoCore) MessageEventSourceMessageID(roomID string, event *evtv1.Event) (string, bool) {
	if event == nil || roomID == "" {
		return "", false
	}
	protectedRoomID, protected := c.MessageReadProtectedEventRoomID(event)
	if !protected || protectedRoomID != roomID {
		return "", false
	}
	var messageEventID string
	switch payload := event.GetEvent().(type) {
	case *evtv1.Event_MessagePosted:
		messageEventID = event.GetId()
	case *evtv1.Event_MessageBody:
		messageEventID = payload.MessageBody.GetEventId()
	case *evtv1.Event_MessageEdited:
		messageEventID = payload.MessageEdited.GetEventId()
	case *evtv1.Event_MessageRetracted:
		messageEventID = payload.MessageRetracted.GetEventId()
	case *evtv1.Event_MessagePinned:
		messageEventID = payload.MessagePinned.GetMessageEventId()
	case *evtv1.Event_MessageUnpinned:
		messageEventID = payload.MessageUnpinned.GetMessageEventId()
	case *evtv1.Event_ThreadCreated:
		messageEventID = payload.ThreadCreated.GetThreadRootEventId()
	case *evtv1.Event_ThreadFollowed:
		messageEventID = payload.ThreadFollowed.GetThreadRootEventId()
	case *evtv1.Event_ThreadUnfollowed:
		messageEventID = payload.ThreadUnfollowed.GetThreadRootEventId()
	case *evtv1.Event_ReactionAdded:
		messageEventID = payload.ReactionAdded.GetMessageEventId()
	case *evtv1.Event_ReactionRemoved:
		messageEventID = payload.ReactionRemoved.GetMessageEventId()
	case *evtv1.Event_AssetAttached:
		messageEventID = payload.AssetAttached.GetMessageEventId()
	case *evtv1.Event_AssetProcessingStarted,
		*evtv1.Event_AssetProcessingSucceeded,
		*evtv1.Event_AssetProcessingFailed,
		*evtv1.Event_AssetDeleted:
		assetRoomID, targetEventID, ok := c.AssetEventTimelineTarget(event)
		if !ok || assetRoomID != roomID {
			return "", false
		}
		messageEventID = targetEventID
	default:
		return "", false
	}
	return messageEventID, messageEventID != ""
}

func assetCreatedRoomID(event *evtv1.AssetCreatedEvent) string {
	if event == nil {
		return ""
	}
	return event.GetRoomId()
}

func assetIDOfLifecycleEvent(event *evtv1.Event) string {
	if event == nil {
		return ""
	}
	switch ev := event.GetEvent().(type) {
	case *evtv1.Event_AssetCreated:
		if ev.AssetCreated.GetAsset() == nil {
			return ""
		}
		return ev.AssetCreated.GetAsset().GetId()
	case *evtv1.Event_AssetProcessingStarted:
		return ev.AssetProcessingStarted.GetAssetId()
	case *evtv1.Event_AssetProcessingSucceeded:
		return ev.AssetProcessingSucceeded.GetAssetId()
	case *evtv1.Event_AssetProcessingFailed:
		return ev.AssetProcessingFailed.GetAssetId()
	case *evtv1.Event_AssetDeleted:
		return ev.AssetDeleted.GetAssetId()
	case *evtv1.Event_AssetAttached:
		return ev.AssetAttached.GetAssetId()
	default:
		return ""
	}
}

func isAssetLifecycleEvent(event *evtv1.Event) bool {
	switch event.GetEvent().(type) {
	case *evtv1.Event_AssetCreated,
		*evtv1.Event_AssetProcessingStarted,
		*evtv1.Event_AssetProcessingSucceeded,
		*evtv1.Event_AssetProcessingFailed,
		*evtv1.Event_AssetDeleted,
		*evtv1.Event_AssetAttached:
		return true
	default:
		return false
	}
}

// isVisibleRoomTimelineEntry reports whether a timeline entry should surface
// in the room-level view (GetRoomEvents and friends).
//
// Hidden:
//
//   - Thread replies (MessagePostedEvent with in_thread != "") — served via
//     GetThreadEvents.
//
//   - MessageEditedEvent / MessageRetractedEvent — folded onto the original
//     post via projection.LatestBody; not surfaced as separate timeline
//     entries.
//
//   - ReactionAddedEvent / ReactionRemovedEvent — folded into the reaction
//     projection.
//
//   - RoomMemberBannedEvent / RoomMemberUnbannedEvent and
//     RoomMemberAddedEvent / RoomMemberRemovedEvent — moderation audit facts,
//     not displayed as chat timeline items.
//
//   - Voice call participant events — projected into call state and delivered
//     live, but not displayed as chat timeline items.
//
// Visible: root messages, room lifecycle (created/updated/archived/
// unarchived/deleted), Threading Mode changes, memberships (user_joined /
// user_left), and voice call lifecycle (started / ended).
func isVisibleRoomTimelineEntry(event *evtv1.Event) bool {
	if event == nil {
		return false
	}
	switch e := event.GetEvent().(type) {
	case *evtv1.Event_MessagePosted:
		return e.MessagePosted.GetInThread() == ""
	case *evtv1.Event_RoomCreated,
		*evtv1.Event_RoomUpdated,
		*evtv1.Event_RoomDeleted,
		*evtv1.Event_RoomArchived,
		*evtv1.Event_RoomUnarchived,
		*evtv1.Event_RoomThreadingModeChanged,
		*evtv1.Event_UserJoinedRoom,
		*evtv1.Event_UserLeftRoom,
		*evtv1.Event_VoiceCallStarted,
		*evtv1.Event_VoiceCallEnded:
		return true
	case *evtv1.Event_MessageEdited, *evtv1.Event_MessageRetracted,
		*evtv1.Event_MessagePinned, *evtv1.Event_MessageUnpinned,
		*evtv1.Event_ThreadCreated,
		*evtv1.Event_RoomUniversalChanged,
		*evtv1.Event_RoomSlowModeChanged,
		*evtv1.Event_RoomMemberBanned, *evtv1.Event_RoomMemberUnbanned,
		*evtv1.Event_RoomMemberAdded, *evtv1.Event_RoomMemberRemoved,
		*evtv1.Event_AssetCreated, *evtv1.Event_AssetDeleted, *evtv1.Event_AssetAttached,
		*evtv1.Event_AssetProcessingStarted,
		*evtv1.Event_AssetProcessingSucceeded, *evtv1.Event_AssetProcessingFailed,
		*evtv1.Event_ReactionAdded, *evtv1.Event_ReactionRemoved,
		*evtv1.Event_VoiceCallParticipantJoined,
		*evtv1.Event_VoiceCallParticipantLeft:
		return false
	}
	return false
}

// IsVisibleRoomTimelineEntry reports whether an event should surface in the
// public room timeline.
func IsVisibleRoomTimelineEntry(event *evtv1.Event) bool {
	return isVisibleRoomTimelineEntry(event)
}

func isDeliverableLiveEVTRoomEvent(event *evtv1.Event) bool {
	return isDeliverableLiveEVTRoomEventType(evtstream.EventTypeOf(event))
}

func isDeliverableLiveEVTRoomEventType(eventType string) bool {
	switch eventType {
	case evtstream.EventRoomCreated,
		evtstream.EventRoomUpdated,
		evtstream.EventRoomDeleted,
		evtstream.EventRoomArchived,
		evtstream.EventRoomUnarchived,
		evtstream.EventRoomUniversalChanged,
		evtstream.EventRoomSlowModeChanged,
		evtstream.EventRoomThreadingModeChanged,
		evtstream.EventUserJoinedRoom,
		evtstream.EventUserLeftRoom,
		evtstream.EventRoomMemberAdded,
		evtstream.EventRoomMemberRemoved,
		evtstream.EventRoomMemberBanned,
		evtstream.EventThreadCreated,
		evtstream.EventMessagePosted,
		evtstream.EventMessageEdited,
		evtstream.EventMessageRetracted,
		evtstream.EventMessagePinned,
		evtstream.EventMessageUnpinned,
		evtstream.EventReactionAdded,
		evtstream.EventReactionRemoved,
		evtstream.EventAssetProcessingStarted,
		evtstream.EventAssetProcessingSucceeded,
		evtstream.EventAssetProcessingFailed,
		evtstream.EventAssetDeleted,
		evtstream.EventCallStarted,
		evtstream.EventCallParticipantJoined,
		evtstream.EventCallParticipantLeft,
		evtstream.EventCallEnded:
		return true
	default:
		return false
	}
}

func isDeliverableLiveEVTAssetEvent(event *evtv1.Event) bool {
	return isDeliverableLiveEVTAssetEventType(evtstream.EventTypeOf(event))
}

func isDeliverableLiveEVTAssetEventType(eventType string) bool {
	switch eventType {
	case evtstream.EventAssetProcessingStarted,
		evtstream.EventAssetProcessingSucceeded,
		evtstream.EventAssetProcessingFailed,
		evtstream.EventAssetDeleted:
		return true
	default:
		return false
	}
}

func isDeliverableLiveEVTUserEvent(event *evtv1.Event) bool {
	return isDeliverableLiveEVTUserEventType(evtstream.EventTypeOf(event))
}

// IsRBACEvent reports whether event changes roles or permission resolution.
// Realtime protocol v2 uses this to invalidate an authorization-dependent
// client projection without exposing the internal RBAC payload.
func IsRBACEvent(event *evtv1.Event) bool {
	if event == nil {
		return false
	}
	switch event.GetEvent().(type) {
	case *evtv1.Event_RbacRoleCreated,
		*evtv1.Event_RbacRoleDisplayNameChanged,
		*evtv1.Event_RbacRoleDescriptionChanged,
		*evtv1.Event_RbacRolePingableChanged,
		*evtv1.Event_RbacRoleDeleted,
		*evtv1.Event_RbacRolesReordered,
		*evtv1.Event_RbacRoleAssigned,
		*evtv1.Event_RbacRoleRevoked,
		*evtv1.Event_RbacPermissionGranted,
		*evtv1.Event_RbacPermissionDenied,
		*evtv1.Event_RbacPermissionCleared:
		return true
	default:
		return false
	}
}

func isDeliverableLiveEVTUserEventType(eventType string) bool {
	switch eventType {
	case evtstream.EventUserAccountCreated,
		evtstream.EventUserLoginChanged,
		evtstream.EventUserDisplayNameChanged,
		evtstream.EventUserBioChanged,
		evtstream.EventUserAvatarSet,
		evtstream.EventUserAvatarCleared,
		evtstream.EventUserAccountDeleted,
		evtstream.EventUserKeyShreddingRequested,
		evtstream.EventUserKeyShredded,
		evtstream.EventUserCustomStatusSet,
		evtstream.EventUserCustomStatusCleared:
		return true
	default:
		return false
	}
}

func eventNeedsReactionProjection(event *evtv1.Event) bool {
	switch event.GetEvent().(type) {
	case *evtv1.Event_ReactionAdded, *evtv1.Event_ReactionRemoved:
		return true
	default:
		return false
	}
}

func eventNeedsThreadProjection(event *evtv1.Event) bool {
	switch event.GetEvent().(type) {
	case *evtv1.Event_RoomCreated, *evtv1.Event_RoomDeleted,
		*evtv1.Event_ThreadCreated, *evtv1.Event_ThreadFollowed, *evtv1.Event_ThreadUnfollowed:
		return true
	case *evtv1.Event_MessagePosted:
		return true
	case *evtv1.Event_MessageEdited, *evtv1.Event_MessageRetracted:
		return true
	case *evtv1.Event_UserKeyShreddingRequested, *evtv1.Event_UserKeyShredded:
		return true
	default:
		return false
	}
}

func eventNeedsRoomDirectoryProjection(event *evtv1.Event) bool {
	switch event.GetEvent().(type) {
	case *evtv1.Event_UserJoinedRoom,
		*evtv1.Event_UserLeftRoom,
		*evtv1.Event_RoomMemberBanned,
		*evtv1.Event_RoomMemberUnbanned,
		*evtv1.Event_RoomCreated,
		*evtv1.Event_RoomUpdated,
		*evtv1.Event_RoomArchived,
		*evtv1.Event_RoomUnarchived,
		*evtv1.Event_RoomUniversalChanged,
		*evtv1.Event_RoomSlowModeChanged,
		*evtv1.Event_RoomThreadingModeChanged,
		*evtv1.Event_RoomDeleted:
		return true
	default:
		return false
	}
}

func eventNeedsCallStateProjection(event *evtv1.Event) bool {
	switch event.GetEvent().(type) {
	case *evtv1.Event_UserLeftRoom,
		*evtv1.Event_VoiceCallStarted,
		*evtv1.Event_VoiceCallParticipantJoined,
		*evtv1.Event_VoiceCallParticipantLeft,
		*evtv1.Event_VoiceCallEnded:
		return true
	default:
		return false
	}
}

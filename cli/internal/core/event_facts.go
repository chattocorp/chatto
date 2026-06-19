package core

import corev1 "hmans.de/chatto/internal/pb/chatto/core/v1"

func messageAuthorID(event *corev1.Event) string {
	if event != nil {
		return event.GetActorId()
	}
	return ""
}

func roomIDOfEvent(event *corev1.Event) string {
	if event == nil {
		return ""
	}
	switch e := event.GetEvent().(type) {
	case *corev1.Event_RoomCreated:
		return e.RoomCreated.GetRoomId()
	case *corev1.Event_RoomUpdated:
		return e.RoomUpdated.GetRoomId()
	case *corev1.Event_RoomDeleted:
		return e.RoomDeleted.GetRoomId()
	case *corev1.Event_RoomArchived:
		return e.RoomArchived.GetRoomId()
	case *corev1.Event_RoomUnarchived:
		return e.RoomUnarchived.GetRoomId()
	case *corev1.Event_UserJoinedRoom:
		return e.UserJoinedRoom.GetRoomId()
	case *corev1.Event_UserLeftRoom:
		return e.UserLeftRoom.GetRoomId()
	case *corev1.Event_RoomMemberBanned:
		return e.RoomMemberBanned.GetRoomId()
	case *corev1.Event_RoomMemberUnbanned:
		return e.RoomMemberUnbanned.GetRoomId()
	case *corev1.Event_MessagePosted:
		return e.MessagePosted.GetRoomId()
	case *corev1.Event_MessageEdited:
		return e.MessageEdited.GetRoomId()
	case *corev1.Event_MessageRetracted:
		return e.MessageRetracted.GetRoomId()
	case *corev1.Event_MessageBody:
		return e.MessageBody.GetRoomId()
	case *corev1.Event_ThreadCreated:
		return e.ThreadCreated.GetRoomId()
	case *corev1.Event_AssetCreated:
		return ""
	case *corev1.Event_ReactionAdded:
		return e.ReactionAdded.GetRoomId()
	case *corev1.Event_ReactionRemoved:
		return e.ReactionRemoved.GetRoomId()
	case *corev1.Event_VoiceCallParticipantJoined:
		return e.VoiceCallParticipantJoined.GetRoomId()
	case *corev1.Event_VoiceCallParticipantLeft:
		return e.VoiceCallParticipantLeft.GetRoomId()
	case *corev1.Event_VoiceCallStarted:
		return e.VoiceCallStarted.GetRoomId()
	case *corev1.Event_VoiceCallEnded:
		return e.VoiceCallEnded.GetRoomId()
	}
	return ""
}

func assetCreatedRoomID(event *corev1.AssetCreatedEvent) string {
	if event == nil {
		return ""
	}
	return event.GetRoomId()
}

func assetIDOfLifecycleEvent(event *corev1.Event) string {
	if event == nil {
		return ""
	}
	switch ev := event.GetEvent().(type) {
	case *corev1.Event_AssetCreated:
		if ev.AssetCreated.GetAsset() == nil {
			return ""
		}
		return ev.AssetCreated.GetAsset().GetId()
	case *corev1.Event_AssetProcessingStarted:
		return ev.AssetProcessingStarted.GetAssetId()
	case *corev1.Event_AssetProcessingSucceeded:
		return ev.AssetProcessingSucceeded.GetAssetId()
	case *corev1.Event_AssetProcessingFailed:
		return ev.AssetProcessingFailed.GetAssetId()
	case *corev1.Event_AssetDeleted:
		return ev.AssetDeleted.GetAssetId()
	default:
		return ""
	}
}

func isAssetLifecycleEvent(event *corev1.Event) bool {
	switch event.GetEvent().(type) {
	case *corev1.Event_AssetCreated,
		*corev1.Event_AssetProcessingStarted,
		*corev1.Event_AssetProcessingSucceeded,
		*corev1.Event_AssetProcessingFailed,
		*corev1.Event_AssetDeleted:
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
//   - RoomMemberBannedEvent / RoomMemberUnbannedEvent — moderation audit facts,
//     projected for admin surfaces but not displayed as chat timeline items.
//
//   - Voice call lifecycle and participant events — projected into call state
//     and delivered live, but not displayed as chat timeline items.
//
// Visible: root messages, room lifecycle (created/updated/archived/
// unarchived/deleted), and memberships (user_joined / user_left).
func isVisibleRoomTimelineEntry(event *corev1.Event) bool {
	if event == nil {
		return false
	}
	switch e := event.GetEvent().(type) {
	case *corev1.Event_MessagePosted:
		return e.MessagePosted.GetInThread() == ""
	case *corev1.Event_MessageEdited, *corev1.Event_MessageRetracted,
		*corev1.Event_ThreadCreated,
		*corev1.Event_RoomMemberBanned, *corev1.Event_RoomMemberUnbanned,
		*corev1.Event_AssetCreated, *corev1.Event_AssetDeleted,
		*corev1.Event_AssetProcessingStarted,
		*corev1.Event_AssetProcessingSucceeded, *corev1.Event_AssetProcessingFailed,
		*corev1.Event_ReactionAdded, *corev1.Event_ReactionRemoved,
		*corev1.Event_VoiceCallStarted, *corev1.Event_VoiceCallParticipantJoined,
		*corev1.Event_VoiceCallParticipantLeft, *corev1.Event_VoiceCallEnded:
		return false
	}
	return true
}

func isDeliverableLiveEVTRoomEvent(event *corev1.Event) bool {
	switch event.GetEvent().(type) {
	case *corev1.Event_RoomCreated,
		*corev1.Event_RoomUpdated,
		*corev1.Event_RoomDeleted,
		*corev1.Event_RoomArchived,
		*corev1.Event_RoomUnarchived,
		*corev1.Event_UserJoinedRoom,
		*corev1.Event_UserLeftRoom,
		*corev1.Event_ThreadCreated,
		*corev1.Event_MessagePosted,
		*corev1.Event_MessageEdited,
		*corev1.Event_MessageRetracted,
		*corev1.Event_ReactionAdded,
		*corev1.Event_ReactionRemoved,
		*corev1.Event_AssetProcessingStarted,
		*corev1.Event_AssetProcessingSucceeded,
		*corev1.Event_AssetProcessingFailed,
		*corev1.Event_AssetDeleted,
		*corev1.Event_VoiceCallStarted,
		*corev1.Event_VoiceCallParticipantJoined,
		*corev1.Event_VoiceCallParticipantLeft,
		*corev1.Event_VoiceCallEnded:
		return true
	default:
		return false
	}
}

func isDeliverableLiveEVTAssetEvent(event *corev1.Event) bool {
	switch event.GetEvent().(type) {
	case *corev1.Event_AssetProcessingStarted,
		*corev1.Event_AssetProcessingSucceeded,
		*corev1.Event_AssetProcessingFailed,
		*corev1.Event_AssetDeleted:
		return true
	default:
		return false
	}
}

func eventNeedsReactionProjection(event *corev1.Event) bool {
	switch event.GetEvent().(type) {
	case *corev1.Event_ReactionAdded, *corev1.Event_ReactionRemoved:
		return true
	default:
		return false
	}
}

func eventNeedsRoomDirectoryProjection(event *corev1.Event) bool {
	switch event.GetEvent().(type) {
	case *corev1.Event_UserJoinedRoom,
		*corev1.Event_UserLeftRoom,
		*corev1.Event_RoomMemberBanned,
		*corev1.Event_RoomMemberUnbanned,
		*corev1.Event_RoomCreated,
		*corev1.Event_RoomUpdated,
		*corev1.Event_RoomArchived,
		*corev1.Event_RoomUnarchived,
		*corev1.Event_RoomDeleted:
		return true
	default:
		return false
	}
}

func eventNeedsCallStateProjection(event *corev1.Event) bool {
	switch event.GetEvent().(type) {
	case *corev1.Event_VoiceCallStarted,
		*corev1.Event_VoiceCallParticipantJoined,
		*corev1.Event_VoiceCallParticipantLeft,
		*corev1.Event_VoiceCallEnded:
		return true
	default:
		return false
	}
}
